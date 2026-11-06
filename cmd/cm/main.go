// Command cm connects your editor to Cloud Mastery's simulated Google Cloud.
//
// The terminal of VS Code, Antigravity or any editor can run commands in the
// free console (or in a lab you have open in the browser): gcloud, kubectl,
// gsutil, bq… run against the simulator, on the same state the web console
// shows. cm is also an MCP server, so the agents of those editors can use the
// simulator as a tool.
//
//	cm login                      sign in (asks for the password)
//	cm sandbox [-world zentra]    open a free console (or -resume the saved one)
//	cm attach <session>           work on a lab you have open in the browser
//	cm shell                      interactive Cloud Shell in this terminal
//	cm run -- gcloud …            one command (what the shims call)
//	cm push <file|dir> [path]     copy files into the Cloud Shell home
//	cm pull [dir]                 copy the Cloud Shell files here
//	cm save                       save the free console world
//	cm shims                      gcloud/kubectl/gsutil/bq/gam that go to the simulator
//	cm vscode [dir]               terminal profile + MCP config for VS Code
//	cm mcp                        MCP server on stdio (for editors' agents)
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"golang.org/x/term"
)

const defaultServer = "http://127.0.0.1:8765" // the desktop app and the portable ZIP

// Config is kept in the user's config folder, readable only by them.
type Config struct {
	Server  string `json:"server"`
	Token   string `json:"token,omitempty"`
	Session string `json:"session,omitempty"`
	Email   string `json:"email,omitempty"`
}

var cfgPath = func() string {
	if p := os.Getenv("CM_CONFIG"); p != "" {
		return p
	}
	d, err := os.UserConfigDir()
	if err != nil {
		d = "."
	}
	return filepath.Join(d, "cloudmastery", "cm.json")
}

func loadConfig() Config {
	var c Config
	if b, err := os.ReadFile(cfgPath()); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	if s := os.Getenv("CM_SERVER"); s != "" {
		c.Server = s
	}
	if c.Server == "" {
		c.Server = defaultServer
	}
	c.Server = strings.TrimRight(c.Server, "/")
	return c
}

func (c Config) save() error {
	p := cfgPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(p, b, 0o600)
}

// ---- API client -------------------------------------------------------------

type apiError struct {
	Status int
	Msg    string
}

func (e *apiError) Error() string { return e.Msg }

var httpClient = &http.Client{Timeout: 5 * time.Minute}

func (c Config) call(method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.Server+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Lang", lang())
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf(msg("no se puede conectar con %s: ¿está Cloud Mastery abierto? (%v)", "cannot reach %s: is Cloud Mastery running? (%v)"), c.Server, err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = res.Status
		}
		if res.StatusCode == 401 {
			e.Error = msg("sesión caducada o no iniciada: ejecuta «cm login»", "not signed in or session expired: run \"cm login\"")
		}
		return &apiError{res.StatusCode, e.Error}
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

func lang() string {
	l := strings.ToLower(os.Getenv("CM_LANG") + os.Getenv("LANG"))
	if strings.HasPrefix(l, "en") {
		return "en"
	}
	return "es"
}

func msg(es, en string) string {
	if lang() == "en" {
		return en
	}
	return es
}

// ---- sessions -----------------------------------------------------------------

type sessionInfo struct {
	ID      string `json:"id"`
	LabID   string `json:"labId"`
	Project string `json:"project"`
	Title   string `json:"title"`
	Status  string `json:"status"`
}

type execResult struct {
	Output string `json:"output"`
	Exit   int    `json:"exit"`
	Prompt string `json:"prompt"`
}

func startSandbox(c *Config, world string, resume bool) (*sessionInfo, error) {
	body := map[string]any{"world": world}
	if resume {
		body = map[string]any{"resume": true}
	}
	var s sessionInfo
	if err := c.call("POST", "/api/sandbox/start", body, &s); err != nil {
		return nil, err
	}
	c.Session = s.ID
	return &s, c.save()
}

// session returns the session to work on; with ensure, it opens a free
// console when there is none (or it has closed).
func session(c *Config, ensure bool) (string, error) {
	if c.Session != "" {
		var s sessionInfo
		err := c.call("GET", "/api/sessions/"+c.Session, nil, &s)
		if err == nil && (s.Status == "running" || s.Status == "") {
			return c.Session, nil
		}
		var ae *apiError
		if err != nil && !(errors.As(err, &ae) && (ae.Status == 404 || ae.Status == 403 || ae.Status == 410)) {
			return "", err
		}
	}
	if !ensure {
		return "", errors.New(msg("no hay sesión: abre una consola libre con «cm sandbox» o conéctate a un laboratorio con «cm attach <sesión>»", "no session: open a free console with \"cm sandbox\" or attach to a lab with \"cm attach <session>\""))
	}
	s, err := startSandbox(c, "default-network", false)
	if err != nil {
		return "", err
	}
	return s.ID, nil
}

func execLine(c Config, sid, line string) (*execResult, error) {
	var r execResult
	if err := c.call("POST", "/api/sessions/"+sid+"/exec", map[string]string{"line": line}, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// quote rebuilds a command line from arguments (for the shims).
func quote(args []string) string {
	var out []string
	for _, a := range args {
		if a != "" && !strings.ContainsAny(a, " \t\n'\"\\$`|&;<>()*?[]{}!#~") {
			out = append(out, a)
			continue
		}
		out = append(out, "'"+strings.ReplaceAll(a, "'", `'\''`)+"'")
	}
	return strings.Join(out, " ")
}

// ---- commands -------------------------------------------------------------------

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	c := loadConfig()
	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "login":
		err = cmdLogin(&c, args)
	case "logout":
		c.Token, c.Session = "", ""
		err = c.save()
	case "worlds":
		err = cmdWorlds(c)
	case "sandbox":
		err = cmdSandbox(&c, args)
	case "attach":
		err = cmdAttach(&c, args)
	case "status":
		err = cmdStatus(c)
	case "shell":
		err = cmdShell(&c)
	case "run":
		var code int
		code, err = cmdRun(&c, args)
		if err == nil {
			os.Exit(code)
		}
	case "push":
		err = cmdPush(&c, args)
	case "pull":
		err = cmdPull(&c, args)
	case "save":
		var r struct {
			SavedAt time.Time `json:"savedAt"`
		}
		if err = c.call("POST", "/api/sandbox/save", map[string]any{}, &r); err == nil {
			fmt.Println(msg("Mundo guardado a las ", "World saved at ") + r.SavedAt.Local().Format("15:04"))
		}
	case "shims":
		err = cmdShims(args)
	case "vscode":
		err = cmdVSCode(args)
	case "mcp":
		err = serveMCP(os.Stdin, os.Stdout, &c)
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cm:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, msg(`cm conecta tu editor con el Google Cloud simulado de Cloud Mastery.

  cm login [-server URL] [-email correo]   iniciar sesión (pide la contraseña; o CM_PASSWORD)
  cm sandbox [-world ID | -resume]         abrir una consola libre (cm worlds lista los mundos)
  cm attach <sesión>                       trabajar en un laboratorio abierto en el navegador
  cm status                                servidor, cuenta y sesión actuales
  cm shell                                 Cloud Shell interactiva en esta terminal
  cm run -- gcloud compute instances list  un comando suelto
  cm push <archivo|carpeta> [ruta]         copiar archivos al home de Cloud Shell
  cm pull [carpeta]                        traer aquí los archivos de Cloud Shell
  cm save                                  guardar el mundo de la consola libre
  cm shims [carpeta]                       gcloud, kubectl, gsutil, bq y gam que van al simulador
  cm vscode [carpeta]                      perfil de terminal y MCP para VS Code (y la config de Antigravity)
  cm mcp                                   servidor MCP por stdio, para los agentes del editor

Servidor por defecto: `+defaultServer+` (la app de escritorio). Cambia con -server o CM_SERVER.
`, `cm connects your editor to Cloud Mastery's simulated Google Cloud.

  cm login [-server URL] [-email address]  sign in (asks for the password; or CM_PASSWORD)
  cm sandbox [-world ID | -resume]         open a free console (cm worlds lists the worlds)
  cm attach <session>                      work on a lab open in the browser
  cm status                                current server, account and session
  cm shell                                 interactive Cloud Shell in this terminal
  cm run -- gcloud compute instances list  a single command
  cm push <file|dir> [path]                copy files into the Cloud Shell home
  cm pull [dir]                            copy the Cloud Shell files here
  cm save                                  save the free console world
  cm shims [dir]                           gcloud, kubectl, gsutil, bq and gam that go to the simulator
  cm vscode [dir]                          terminal profile and MCP for VS Code (and Antigravity's config)
  cm mcp                                   MCP server on stdio, for the editor's agents

Default server: `+defaultServer+` (the desktop app). Change it with -server or CM_SERVER.
`))
}

func cmdLogin(c *Config, args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	server := fs.String("server", c.Server, "")
	email := fs.String("email", c.Email, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c.Server = strings.TrimRight(*server, "/")
	in := bufio.NewReader(os.Stdin)
	if *email == "" {
		fmt.Fprint(os.Stderr, msg("Correo: ", "Email: "))
		l, _ := in.ReadString('\n')
		*email = strings.TrimSpace(l)
	}
	pw := os.Getenv("CM_PASSWORD")
	if pw == "" {
		fmt.Fprint(os.Stderr, msg("Contraseña: ", "Password: "))
		if term.IsTerminal(int(os.Stdin.Fd())) {
			b, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr)
			if err != nil {
				return err
			}
			pw = string(b)
		} else {
			l, _ := in.ReadString('\n')
			pw = strings.TrimSpace(l)
		}
	}
	var r struct {
		Token string `json:"token"`
		User  struct {
			Name string `json:"name"`
		} `json:"user"`
	}
	c.Token = ""
	if err := c.call("POST", "/api/auth/login", map[string]string{"email": *email, "password": pw}, &r); err != nil {
		return err
	}
	c.Token, c.Email, c.Session = r.Token, *email, ""
	if err := c.save(); err != nil {
		return err
	}
	fmt.Printf(msg("Hola, %s. Sesión iniciada en %s.\n", "Hi %s. Signed in to %s.\n"), r.User.Name, c.Server)
	return nil
}

func cmdWorlds(c Config) error {
	var r struct {
		Worlds []map[string]string `json:"worlds"`
		Saved  *struct {
			World string `json:"world"`
		} `json:"saved"`
	}
	if err := c.call("GET", "/api/sandbox", nil, &r); err != nil {
		return err
	}
	for _, w := range r.Worlds {
		fmt.Printf("  %-16s %s — %s\n", w["id"], w["name"], w["description"])
	}
	if r.Saved != nil {
		fmt.Printf(msg("\nTienes guardado un mundo «%s»: cm sandbox -resume\n", "\nYou have a saved \"%s\" world: cm sandbox -resume\n"), r.Saved.World)
	}
	return nil
}

func cmdSandbox(c *Config, args []string) error {
	fs := flag.NewFlagSet("sandbox", flag.ContinueOnError)
	world := fs.String("world", "default-network", "")
	resume := fs.Bool("resume", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := startSandbox(c, *world, *resume)
	if err != nil {
		return err
	}
	fmt.Printf(msg("Consola libre abierta: %s (proyecto %s).\nEn el navegador: %s/lab?session=%s&id=%s\n", "Free console open: %s (project %s).\nIn the browser: %s/lab?session=%s&id=%s\n"), s.Title, s.Project, c.Server, s.ID, s.LabID)
	return nil
}

func cmdAttach(c *Config, args []string) error {
	if len(args) != 1 {
		return errors.New(msg("uso: cm attach <sesión> (la ves en la dirección del laboratorio: ?session=…)", "usage: cm attach <session> (it is in the lab's address: ?session=…)"))
	}
	sid := args[0]
	if i := strings.Index(sid, "session="); i >= 0 { // a whole lab URL works too
		sid = strings.SplitN(sid[i+8:], "&", 2)[0]
	}
	var s sessionInfo
	if err := c.call("GET", "/api/sessions/"+sid, nil, &s); err != nil {
		return err
	}
	c.Session = s.ID
	if err := c.save(); err != nil {
		return err
	}
	fmt.Printf(msg("Conectado a «%s» (proyecto %s).\n", "Attached to \"%s\" (project %s).\n"), s.Title, s.Project)
	return nil
}

func cmdStatus(c Config) error {
	fmt.Println(msg("Servidor: ", "Server: ") + c.Server)
	if c.Token == "" {
		fmt.Println(msg("Sin sesión iniciada: cm login", "Not signed in: cm login"))
		return nil
	}
	fmt.Println(msg("Cuenta: ", "Account: ") + c.Email)
	if c.Session == "" {
		fmt.Println(msg("Sin consola: cm sandbox", "No console: cm sandbox"))
		return nil
	}
	var s sessionInfo
	if err := c.call("GET", "/api/sessions/"+c.Session, nil, &s); err != nil {
		return err
	}
	fmt.Printf(msg("Sesión: %s · %s · proyecto %s · %s\n", "Session: %s · %s · project %s · %s\n"), s.ID, s.Title, s.Project, s.Status)
	return nil
}

func cmdRun(c *Config, args []string) (int, error) {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return 2, errors.New("uso: cm run -- <comando>")
	}
	sid, err := session(c, true)
	if err != nil {
		return 1, err
	}
	line := quote(args)
	if len(args) == 1 {
		line = args[0] // a whole command line in one argument
	}
	r, err := execLine(*c, sid, line)
	if err != nil {
		return 1, err
	}
	fmt.Print(r.Output)
	if r.Output != "" && !strings.HasSuffix(r.Output, "\n") {
		fmt.Println()
	}
	return r.Exit, nil
}

func cmdShell(c *Config) error {
	sid, err := session(c, true)
	if err != nil {
		return err
	}
	var s sessionInfo
	_ = c.call("GET", "/api/sessions/"+sid, nil, &s)
	fmt.Printf(msg("Cloud Shell de Cloud Mastery · %s · proyecto %s\nEscribe «help» para ver los comandos y «exit» para salir.\n\n", "Cloud Mastery Cloud Shell · %s · project %s\nType \"help\" for the commands and \"exit\" to leave.\n\n"), s.Title, s.Project)
	prompt := fmt.Sprintf("student@cloudshell:~ (%s)$ ", s.Project)
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	for {
		fmt.Print(prompt)
		if !in.Scan() {
			fmt.Println()
			return nil
		}
		line := strings.TrimSpace(in.Text())
		if line == "exit" || line == "logout" {
			if !strings.Contains(prompt, "cloudshell") { // inside a VM or a database client: leave it
				if r, err := execLine(*c, sid, line); err == nil && r.Prompt != "" {
					fmt.Print(r.Output)
					prompt = r.Prompt
					continue
				}
			}
			return nil
		}
		if line == "" {
			continue
		}
		r, err := execLine(*c, sid, line)
		if err != nil {
			fmt.Fprintln(os.Stderr, "cm:", err)
			continue
		}
		fmt.Print(r.Output)
		if r.Output != "" && !strings.HasSuffix(r.Output, "\n") {
			fmt.Println()
		}
		if r.Prompt != "" {
			prompt = r.Prompt
		}
	}
}

func cmdPush(c *Config, args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errors.New(msg("uso: cm push <archivo|carpeta> [ruta en Cloud Shell]", "usage: cm push <file|dir> [path in Cloud Shell]"))
	}
	sid, err := session(c, true)
	if err != nil {
		return err
	}
	src := args[0]
	dst := filepath.Base(filepath.Clean(src))
	if len(args) == 2 {
		dst = args[1]
	}
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	n := 0
	put := func(local, remote string) error {
		b, err := os.ReadFile(local)
		if err != nil {
			return err
		}
		if len(b) > 400<<10 {
			fmt.Fprintf(os.Stderr, msg("se omite %s: más de 400 KB\n", "skipping %s: over 400 KB\n"), local)
			return nil
		}
		n++
		return c.call("PUT", "/api/sessions/"+sid+"/files", map[string]string{"Path": filepath.ToSlash(remote), "Content": string(b)}, nil)
	}
	if !st.IsDir() {
		err = put(src, dst)
	} else {
		err = filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if p != src && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(src, p)
			return put(p, filepath.Join(dst, rel))
		})
	}
	if err == nil {
		fmt.Printf(msg("%d archivo(s) copiados a Cloud Shell (~/%s).\n", "%d file(s) copied to Cloud Shell (~/%s).\n"), n, filepath.ToSlash(dst))
	}
	return err
}

func cmdPull(c *Config, args []string) error {
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}
	sid, err := session(c, false)
	if err != nil {
		return err
	}
	files := map[string]string{}
	if err := c.call("GET", "/api/sessions/"+sid+"/views/files", nil, &files); err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for k := range files {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		rel := filepath.FromSlash(strings.TrimPrefix(strings.TrimPrefix(k, "~/"), "/"))
		if strings.Contains(rel, "..") {
			continue // never write outside the folder
		}
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(files[k]), 0o644); err != nil {
			return err
		}
		fmt.Println("  " + p)
	}
	fmt.Printf(msg("%d archivo(s) traídos de Cloud Shell.\n", "%d file(s) copied from Cloud Shell.\n"), len(names))
	return nil
}

// shims are tiny gcloud/kubectl/… commands that forward to "cm run": put
// their folder first in the PATH of a terminal and the usual commands go to
// the simulator (only in that terminal).
var shimTools = []string{"gcloud", "kubectl", "gsutil", "bq", "gam", "terraform"}

func shimDir() string {
	return filepath.Join(filepath.Dir(cfgPath()), "bin")
}

func cmdShims(args []string) error {
	dir := shimDir()
	if len(args) == 1 {
		dir = args[0]
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := writeShims(dir, exe); err != nil {
		return err
	}
	fmt.Printf(msg("Atajos creados en %s.\nPara usarlos en una terminal: ", "Shims written to %s.\nTo use them in a terminal: "), dir)
	if runtime.GOOS == "windows" {
		fmt.Printf("$env:Path = \"%s;\" + $env:Path\n", dir)
	} else {
		fmt.Printf("export PATH=\"%s:$PATH\"\n", dir)
	}
	return nil
}

func writeShims(dir, exe string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, t := range shimTools {
		var err error
		if runtime.GOOS == "windows" {
			err = os.WriteFile(filepath.Join(dir, t+".cmd"), []byte("@\""+exe+"\" run -- "+t+" %*\r\n"), 0o755)
		} else {
			err = os.WriteFile(filepath.Join(dir, t), []byte("#!/bin/sh\n# Cloud Mastery: runs in the simulator, not in real Google Cloud\nexec \""+exe+"\" run -- "+t+" \"$@\"\n"), 0o755)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
