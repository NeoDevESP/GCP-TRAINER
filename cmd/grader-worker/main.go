// Command grader-worker grades lab attempts in an isolated workload. It
// executes functional probes against environments students may have
// tampered with, so it is deployed separately from the web application.
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/grader"
	"github.com/neodevesp/gcp-trainer/internal/orchestrator"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

func main() {
	content := os.Getenv("CONTENT_DIR")
	if content == "" {
		content = "content"
	}
	scenario.BaselineDir = filepath.Join(content, "baselines")
	grader.PolicyDir = filepath.Join(content, "policies")
	labs, err := scenario.LoadAll(filepath.Join(content, "labs"))
	if err != nil {
		log.Fatal(err)
	}
	byID := map[string]*scenario.Lab{}
	for _, l := range labs {
		byID[l.ID] = l
	}
	mux := http.NewServeMux()
	lib, err := scenario.LoadLibrary(filepath.Join(content, "failures"))
	if err != nil {
		log.Printf("failure library not loaded: %v", err)
	}
	mux.HandleFunc("POST /grade", orchestrator.HandleGrade(byID, lib))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8090"
	}
	log.Printf("grader worker on %s (%d labs)", addr, len(labs))
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
