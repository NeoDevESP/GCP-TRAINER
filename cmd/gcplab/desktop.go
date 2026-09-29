package main

// Desktop mode: the Windows installer's shortcut runs `gcplab -desktop` (and a
// bare double-click on Windows does the same). The platform listens on
// 127.0.0.1 only, keeps its data in the user's profile, opens the browser and
// shows a console window that stops it when closed.

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const desktopBasePort = 8765

type desktop struct {
	url     string
	dataDir string
	logFile *os.File
}

// wantsDesktop reports whether the process runs as the local desktop app.
func wantsDesktop(args []string) bool {
	for _, a := range args[1:] {
		if a == "-desktop" || a == "--desktop" {
			return true
		}
	}
	// Double-clicking gcplab.exe: no arguments and no server configuration.
	return runtime.GOOS == "windows" && len(args) == 1 && os.Getenv("ADDR") == "" && os.Getenv("PORT") == "" && os.Getenv("DATABASE_URL") == ""
}

func setDefault(k, v string) {
	if os.Getenv(k) == "" {
		os.Setenv(k, v)
	}
}

// desktopDataDir is %LOCALAPPDATA%\CloudMastery on Windows and the user's
// config directory elsewhere (DESKTOP_DATA_DIR overrides it).
func desktopDataDir() (string, error) {
	if d := os.Getenv("DESKTOP_DATA_DIR"); d != "" {
		return d, os.MkdirAll(d, 0o700)
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		var err error
		if base, err = os.UserConfigDir(); err != nil {
			return "", err
		}
	}
	d := filepath.Join(base, "CloudMastery")
	return d, os.MkdirAll(d, 0o700)
}

// runningInstance reports whether Cloud Mastery already answers on the port.
func runningInstance(port int) bool {
	c := http.Client{Timeout: 2 * time.Second}
	r, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/api/health", port))
	if err != nil {
		return false
	}
	defer r.Body.Close()
	var b [256]byte
	n, _ := r.Body.Read(b[:])
	return r.StatusCode == http.StatusOK && strings.Contains(string(b[:n]), `"labs"`)
}

// setupDesktop configures the environment for desktop mode. It returns
// exit=true when another instance was found (the browser has been opened).
func setupDesktop() (d *desktop, exit bool, err error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, false, err
	}
	dir := filepath.Dir(exe)
	setDefault("CONTENT_DIR", filepath.Join(dir, "content"))
	setDefault("WEB_DIR", filepath.Join(dir, "web"))
	d = &desktop{}
	if d.dataDir, err = desktopDataDir(); err != nil {
		return nil, false, err
	}
	setDefault("DATA_FILE", filepath.Join(d.dataDir, "gcplab.json"))

	// A stable signing secret keeps the learner signed in across restarts.
	if os.Getenv("JWT_SECRET") == "" {
		keyFile := filepath.Join(d.dataDir, "secret.key")
		key, rerr := os.ReadFile(keyFile)
		if rerr != nil || len(strings.TrimSpace(string(key))) < 32 {
			b := make([]byte, 32)
			if _, err := rand.Read(b); err != nil {
				return nil, false, err
			}
			key = []byte(hex.EncodeToString(b))
			if err := os.WriteFile(keyFile, key, 0o600); err != nil {
				return nil, false, err
			}
		}
		os.Setenv("JWT_SECRET", strings.TrimSpace(string(key)))
	}

	// Same port every time (the browser keeps the session per address); if
	// another program holds it, try the next ones.
	base := desktopBasePort
	if p, perr := strconv.Atoi(os.Getenv("DESKTOP_PORT")); perr == nil && p > 0 {
		base = p
	}
	port := 0
	for p := base; p < base+10; p++ {
		if runningInstance(p) {
			d.url = fmt.Sprintf("http://127.0.0.1:%d/", p)
			fmt.Println("Cloud Mastery ya está en marcha:", d.url)
			openBrowser(d.url)
			return d, true, nil
		}
		if l, lerr := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p)); lerr == nil {
			l.Close()
			port = p
			break
		}
	}
	if port == 0 {
		return nil, false, fmt.Errorf("los puertos %d-%d están ocupados; define DESKTOP_PORT con otro puerto", base, base+9)
	}
	os.Setenv("ADDR", fmt.Sprintf("127.0.0.1:%d", port))
	d.url = fmt.Sprintf("http://127.0.0.1:%d/", port)

	// Technical logs go to a file; the console shows a short message.
	if d.logFile, err = os.OpenFile(filepath.Join(d.dataDir, "gcplab.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); err != nil {
		return nil, false, err
	}
	return d, false, nil
}

// logger returns the structured logger of desktop mode.
func (d *desktop) logger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(d.logFile, nil))
}

// announce waits for the server, prints how to use it and opens the browser.
func (d *desktop) announce(port string) {
	p, _ := strconv.Atoi(port)
	for i := 0; i < 100 && !runningInstance(p); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Println()
	fmt.Println("  Cloud Mastery está en marcha")
	fmt.Println()
	fmt.Println("  Abre en el navegador:  ", d.url)
	fmt.Println("  Tus datos se guardan en:", d.dataDir)
	fmt.Println()
	fmt.Println("  Deja esta ventana abierta mientras practicas.")
	fmt.Println("  Ciérrala (o pulsa Ctrl+C) para salir.")
	fmt.Println()
	openBrowser(d.url)
}

func openBrowser(url string) {
	if os.Getenv("DESKTOP_NO_BROWSER") != "" {
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

// pauseOnError keeps the console open on Windows so the message can be read.
func pauseOnError() {
	if runtime.GOOS == "windows" {
		fmt.Println("Pulsa Intro para cerrar esta ventana.")
		_, _ = fmt.Scanln()
	}
}

// fail shows a startup error in the console (desktop mode only).
func (d *desktop) fail(what string, err error) {
	if d == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "Cloud Mastery no ha podido arrancar (%s): %v\n", what, err)
	pauseOnError()
}
