package main

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestWantsDesktop(t *testing.T) {
	if !wantsDesktop([]string{"gcplab", "-desktop"}) || !wantsDesktop([]string{"gcplab", "--desktop"}) {
		t.Fatal("-desktop must enable desktop mode")
	}
	if wantsDesktop([]string{"gcplab", "-other"}) {
		t.Fatal("other arguments must not enable desktop mode")
	}
}

func TestSetupDesktopKeepsSecretAndSkipsBusyPort(t *testing.T) {
	dir := t.TempDir()
	busy, err := net.Listen("tcp", "127.0.0.1:0") // another program on the first port
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	port := busy.Addr().(*net.TCPAddr).Port
	for _, k := range []string{"CONTENT_DIR", "WEB_DIR", "DATA_FILE", "JWT_SECRET", "ADDR"} {
		t.Setenv(k, "")
	}
	t.Setenv("DESKTOP_DATA_DIR", dir)
	t.Setenv("DESKTOP_PORT", strconv.Itoa(port))
	t.Setenv("DESKTOP_NO_BROWSER", "1")

	d, exit, err := setupDesktop()
	if err != nil || exit {
		t.Fatalf("setup: exit=%v err=%v", exit, err)
	}
	defer d.logFile.Close()
	secret := os.Getenv("JWT_SECRET")
	if len(secret) < 32 {
		t.Fatalf("secret too short: %q", secret)
	}
	if want := "127.0.0.1:" + strconv.Itoa(port+1); os.Getenv("ADDR") != want {
		t.Fatalf("ADDR = %s, want %s (next free port)", os.Getenv("ADDR"), want)
	}
	if !strings.HasPrefix(os.Getenv("DATA_FILE"), dir) {
		t.Fatalf("data must live in the data dir, got %s", os.Getenv("DATA_FILE"))
	}

	// A second launch reuses the same signing key, so sessions survive restarts.
	os.Setenv("JWT_SECRET", "")
	d2, _, err := setupDesktop()
	if err != nil {
		t.Fatal(err)
	}
	defer d2.logFile.Close()
	if os.Getenv("JWT_SECRET") != secret {
		t.Fatal("the signing key must be stable across launches")
	}
}

func TestPortableLayout(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin", "linux-x64")
	for _, d := range []string{bin, filepath.Join(root, "content")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if got := appRoot(bin); got != root {
		t.Fatalf("binary in bin/<platform>: root %q, want %q", got, root)
	}
	if got := appRoot(root); got != root {
		t.Fatalf("binary next to content: root %q, want %q", got, root)
	}
	t.Setenv("DESKTOP_DATA_DIR", "")
	setPortableDataDir(root)
	if got := os.Getenv("DESKTOP_DATA_DIR"); got != "" {
		t.Fatalf("no marker, got %q", got)
	}
	if err := os.WriteFile(filepath.Join(root, "portable.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	setPortableDataDir(root)
	if got, want := os.Getenv("DESKTOP_DATA_DIR"), filepath.Join(root, "datos"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
