// Command gcplab runs the GCP Lab Simulator platform.
//
//	MODE=all       learning plane + lab plane in one process (default)
//	MODE=api       learning plane only; lab plane reached at LABPLANE_URL
//	MODE=labplane  lab plane only (internal service)
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/api"
	"github.com/neodevesp/gcp-trainer/internal/fidelity"
	"github.com/neodevesp/gcp-trainer/internal/grader"
	"github.com/neodevesp/gcp-trainer/internal/learning"
	"github.com/neodevesp/gcp-trainer/internal/orchestrator"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
	"github.com/neodevesp/gcp-trainer/internal/store"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	content := env("CONTENT_DIR", "content")
	scenario.BaselineDir = filepath.Join(content, "baselines")
	grader.PolicyDir = filepath.Join(content, "policies")
	cat, err := learning.LoadCatalog(content)
	if err != nil {
		log.Error("load catalog", "err", err)
		os.Exit(1)
	}
	if p := cat.Validate(); len(p) > 0 {
		log.Warn("content problems", "problems", p)
	}
	var st store.Store
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		st, err = store.NewPostgres(context.Background(), dsn)
	} else {
		st, err = store.NewMemory(env("DATA_FILE", "data/gcplab.json"))
	}
	if err != nil {
		log.Error("store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	// Fidelity router: F0 always; F1 when emulators are configured; F2 when a pool is configured.
	var runtimes []fidelity.Runtime
	f1 := fidelity.F1ConfigFromEnv()
	if f1.PubSubHost != "" || f1.GCSHost != "" || f1.Kubeconfig != "" {
		runtimes = append(runtimes, fidelity.F1Runtime{Cfg: f1})
	}
	var pool *fidelity.Pool
	if projects := os.Getenv("F2_PROJECTS"); projects != "" {
		var driver fidelity.Driver = fidelity.NewSimDriver()
		if env("F2_DRIVER", "sim") == "gcloud" {
			driver = &fidelity.GcloudDriver{AdminSA: os.Getenv("F2_ADMIN_SA"), DryRun: os.Getenv("F2_DRY_RUN") == "1", Log: func(s string) { log.Info("gcloud", "cmd", s) }}
		}
		pool = fidelity.NewPool(driver, strings.Split(projects, ","))
		runtimes = append(runtimes, &fidelity.F2Runtime{Pool: pool, MemberFor: func(u string) string {
			if sa := os.Getenv("F2_LAB_SA"); sa != "" {
				return "serviceAccount:" + sa
			}
			return "user:lab-" + u + "@gcplab.dev"
		}})
		go func() {
			for range time.Tick(time.Minute) {
				if n, errs := pool.Janitor(context.Background()); n > 0 || len(errs) > 0 {
					log.Info("pool janitor", "reclaimed", n, "errors", len(errs))
				}
			}
		}()
	}
	router := fidelity.NewRouter(runtimes...)

	mode := env("MODE", "all")
	addr := env("ADDR", ":8080")
	var handler http.Handler
	var svc *orchestrator.Service
	if mode == "all" || mode == "labplane" {
		svc = orchestrator.New(cat.Labs, router, st)
		svc.GraderURL = os.Getenv("GRADER_URL")
		svc.RunJanitor(30 * time.Second)
		defer svc.Close()
	}
	if mode == "labplane" {
		handler = orchestrator.Handler(svc, os.Getenv("LABPLANE_TOKEN"))
	} else {
		var lp orchestrator.LabPlane = svc
		if mode == "api" {
			lp = &orchestrator.Client{Base: os.Getenv("LABPLANE_URL"), Token: os.Getenv("LABPLANE_TOKEN")}
		}
		secret := os.Getenv("JWT_SECRET")
		if secret == "" {
			secret = "dev-only-" + strconv.FormatInt(time.Now().Unix(), 36)
			log.Warn("JWT_SECRET not set: tokens will not survive restarts")
		}
		f2m, _ := strconv.Atoi(env("F2_MONTHLY", "10"))
		srv := &api.Server{Cat: cat, Engine: &learning.Engine{Cat: cat}, Store: st, Labs: lp, Tokens: &learning.Tokens{Secret: []byte(secret), TTL: 12 * time.Hour},
			Pool: pool, WebDir: env("WEB_DIR", "web/out"), Log: log, F2Monthly: f2m}
		if iss := os.Getenv("OIDC_ISSUER"); iss != "" {
			srv.OIDC = &learning.OIDC{Issuer: iss, ClientID: os.Getenv("OIDC_CLIENT_ID"), ClientSecret: os.Getenv("OIDC_CLIENT_SECRET"), RedirectURL: os.Getenv("OIDC_REDIRECT_URL")}
		}
		if svc != nil {
			svc.OnExpire = srv.OnExpire
		}
		handler = srv.Handler()
	}
	server := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Info("listening", "addr", addr, "mode", mode, "labs", len(cat.Labs))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("serve", "err", err)
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
