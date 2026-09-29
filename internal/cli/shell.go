// Package cli implements the browser terminal: a small POSIX-like shell and
// simulated gcloud, gsutil, bq, kubectl, terraform, git, docker and network
// tools operating on the F0 simulator state.
package cli

import (
	"encoding/base64"
	"fmt"
	"github.com/neodevesp/gcp-trainer/internal/desk"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

// Policy constrains what a student may do in a lab (anti-abuse, cost control).
type Policy struct {
	AllowGPUs         bool     `json:"allowGpus" yaml:"allowGpus"`
	MaxInstances      int      `json:"maxInstances" yaml:"maxInstances"`
	AllowedRegions    []string `json:"allowedRegions" yaml:"allowedRegions"`
	DeniedCommands    []string `json:"deniedCommands" yaml:"deniedCommands"`
	MaxMachineCPUs    int      `json:"maxMachineCpus" yaml:"maxMachineCpus"`
	ReadOnlyResources []string `json:"readOnlyResources" yaml:"readOnlyResources"`
}

// ExecRecord is telemetry for one command line.
type ExecRecord struct {
	Line   string `json:"line"`
	Exit   int    `json:"exit"`
	Tool   string `json:"tool"`
	Output string `json:"output,omitempty"`
	At     string `json:"at"`
}

// Session is one terminal attached to a simulated world.
type Session struct {
	State       *sim.State        `json:"-"`
	Project     string            `json:"project"`
	Region      string            `json:"region"`
	Zone        string            `json:"zone"`
	Account     string            `json:"account"`
	Impersonate string            `json:"impersonate"`
	Files       map[string]string `json:"files"`
	Env         map[string]string `json:"env"`
	Records     []ExecRecord      `json:"records"`
	Kube        KubeContext       `json:"kube"`
	Git         GitState          `json:"git"`
	Policy      Policy            `json:"policy"`
	DockerAuth  map[string]bool   `json:"dockerAuth"`
	LocalImages map[string]string `json:"localImages"` // tag -> behaviour
	NoTick      bool              `json:"-"`
	Credentials map[string]string `json:"credentials"`         // activated SA key files
	Desk        *desk.Desk        `json:"desk,omitempty"`      // ticket and simulated actors
	Chaos       []ChaosRun        `json:"chaos,omitempty"`     // chaos experiments run in this session
	Interview   *Interview        `json:"interview,omitempty"` // interview mode state
	vmRoot      bool              // current VM command runs with sudo
	// Interceptor lets higher fidelity layers (F1 emulators, F2 real GCP)
	// take over a command before the simulator handles it.
	Interceptor func(s *Session, args []string, stdin string) (handled bool, out string, err error) `json:"-"`
}

// KubeContext is the current kubectl context.
type KubeContext struct {
	Project   string `json:"project"`
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
}

// GitState models a local git working copy.
type GitState struct {
	Initialized bool              `json:"initialized"`
	Branch      string            `json:"branch"`
	Remote      string            `json:"remote"`
	Staged      map[string]string `json:"staged"`
	Commits     []sim.GitCommit   `json:"commits"`
}

// NewSession creates a terminal for a state.
func NewSession(st *sim.State, project, account string) *Session {
	return &Session{State: st, Project: project, Account: account, Files: map[string]string{}, Env: map[string]string{"HOME": "/home/student", "USER": "student", "GOOGLE_CLOUD_PROJECT": project, "DEVSHELL_PROJECT_ID": project},
		DockerAuth: map[string]bool{}, LocalImages: map[string]string{}, Credentials: map[string]string{}, Git: GitState{Staged: map[string]string{}}}
}

// Principal returns the IAM principal executing commands.
func (s *Session) Principal() string {
	if s.Impersonate != "" {
		return "serviceAccount:" + s.Impersonate
	}
	if strings.HasSuffix(s.Account, ".gserviceaccount.com") {
		return "serviceAccount:" + s.Account
	}
	return "user:" + s.Account
}

// Result is the outcome of a command line.
type Result struct {
	Output string `json:"output"`
	Exit   int    `json:"exit"`
}

// exitErr carries a non-zero exit code and message.
type exitErr struct {
	code int
	msg  string
}

func (e *exitErr) Error() string { return e.msg }

// Fail builds an error with an exit code (exported for fidelity adapters).
func Fail(code int, format string, a ...any) error { return fail(code, format, a...) }

// RunArgs executes an already tokenised command inside the session.
func (s *Session) RunArgs(args []string, stdin string) (string, error) { return s.run(args, stdin) }

func fail(code int, format string, a ...any) error {
	return &exitErr{code: code, msg: fmt.Sprintf(format, a...)}
}

// Exec runs an input (possibly multi-line, with heredocs) and returns output.
func (s *Session) Exec(input string) Result {
	var out strings.Builder
	exit := 0
	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		for strings.HasSuffix(line, "\\") && i+1 < len(lines) {
			i++
			line = strings.TrimSuffix(line, "\\") + " " + strings.TrimSpace(lines[i])
		}
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if w := firstWord(trim); w == "for" || w == "if" {
			block := line
			for openBlocks(block) > 0 && i+1 < len(lines) {
				i++
				block += "\n" + lines[i]
			}
			r := s.runStatements(statements(block))
			out.WriteString(r.Output)
			exit = r.Exit
			s.Records = append(s.Records, ExecRecord{Line: strings.TrimSpace(block), Exit: r.Exit, Tool: w, Output: truncate(r.Output, 400), At: s.State.Now()})
			if !s.NoTick {
				s.State.Step(1)
			}
			continue
		}
		stdin := ""
		if m := reHeredoc.FindStringSubmatch(line); m != nil {
			delim := m[1]
			var body []string
			for i+1 < len(lines) {
				i++
				if strings.TrimSpace(lines[i]) == delim {
					break
				}
				body = append(body, lines[i])
			}
			stdin = strings.Join(body, "\n") + "\n"
			line = reHeredoc.ReplaceAllString(line, "")
		}
		r := s.execLine(line, stdin)
		out.WriteString(r.Output)
		exit = r.Exit
		s.Records = append(s.Records, ExecRecord{Line: strings.TrimSpace(line), Exit: r.Exit, Tool: firstWord(line), Output: truncate(r.Output, 400), At: s.State.Now()})
		if !s.NoTick {
			s.State.Step(1)
		}
	}
	return Result{Output: out.String(), Exit: exit}
}

var reHeredoc = regexp.MustCompile(`<<-?\s*['"]?([A-Za-z_]+)['"]?`)

func firstWord(l string) string {
	f := strings.Fields(l)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// splitTop splits by separators outside quotes and $( ).
func splitTop(s string, seps []string) ([]string, []string) {
	var parts, ops []string
	depth := 0
	var q byte
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if q != 0 {
			if c == q {
				q = 0
			} else if c == '\\' && q == '"' {
				i++
			}
			continue
		}
		if c == '\'' || c == '"' {
			q = c
			continue
		}
		if c == '$' && i+1 < len(s) && s[i+1] == '(' {
			depth++
			i++
			continue
		}
		if c == ')' && depth > 0 {
			depth--
			continue
		}
		if depth > 0 {
			continue
		}
		for _, sep := range seps {
			if strings.HasPrefix(s[i:], sep) {
				// avoid treating "||" as "|"
				if sep == "|" && (strings.HasPrefix(s[i:], "||") || (i > 0 && s[i-1] == '|')) {
					continue
				}
				if sep == "&&" || sep == "||" || sep == ";" || sep == "|" {
					parts = append(parts, s[start:i])
					ops = append(ops, sep)
					i += len(sep) - 1
					start = i + 1
					goto next
				}
			}
		}
	next:
	}
	parts = append(parts, s[start:])
	return parts, ops
}

func (s *Session) execLine(line, stdin string) Result {
	cmds, ops := splitTop(line, []string{"&&", "||", ";"})
	var out strings.Builder
	last := Result{}
	for i, c := range cmds {
		if i > 0 {
			op := ops[i-1]
			if op == "&&" && last.Exit != 0 {
				continue
			}
			if op == "||" && last.Exit == 0 {
				continue
			}
		}
		if strings.TrimSpace(c) == "" {
			continue
		}
		last = s.execPipeline(c, stdin)
		out.WriteString(last.Output)
	}
	return Result{Output: out.String(), Exit: last.Exit}
}

func (s *Session) execPipeline(p, stdin string) Result {
	stages, _ := splitTop(p, []string{"|"})
	input := stdin
	var r Result
	for _, st := range stages {
		r = s.execSimple(strings.TrimSpace(st), input)
		input = r.Output
	}
	return r
}

// tokenize splits a command into words honouring quotes and expanding
// variables and command substitutions.
func (s *Session) tokenize(cmd string) ([]string, error) {
	var toks []string
	var cur strings.Builder
	inTok := false
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case c == '\'':
			j := strings.IndexByte(cmd[i+1:], '\'')
			if j < 0 {
				return nil, fail(2, "bash: unexpected EOF while looking for matching `''")
			}
			cur.WriteString(cmd[i+1 : i+1+j])
			i += j + 1
			inTok = true
		case c == '"':
			j := i + 1
			var b strings.Builder
			for j < len(cmd) && cmd[j] != '"' {
				if cmd[j] == '\\' && j+1 < len(cmd) && (cmd[j+1] == '"' || cmd[j+1] == '\\' || cmd[j+1] == '$') {
					b.WriteByte(cmd[j+1])
					j += 2
					continue
				}
				b.WriteByte(cmd[j])
				j++
			}
			if j >= len(cmd) {
				return nil, fail(2, "bash: unexpected EOF while looking for matching `\"'")
			}
			cur.WriteString(s.expand(b.String()))
			i = j
			inTok = true
		case c == ' ' || c == '\t':
			if inTok {
				toks = append(toks, cur.String())
				cur.Reset()
				inTok = false
			}
		case c == '\\' && i+1 < len(cmd):
			cur.WriteByte(cmd[i+1])
			i++
			inTok = true
		case c == '$':
			// find extent of expansion
			j := i + 1
			if j < len(cmd) && cmd[j] == '(' {
				depth := 1
				k := j + 1
				for k < len(cmd) && depth > 0 {
					if cmd[k] == '(' {
						depth++
					} else if cmd[k] == ')' {
						depth--
					}
					k++
				}
				cur.WriteString(s.expand(cmd[i:k]))
				i = k - 1
			} else if j < len(cmd) && cmd[j] == '{' {
				k := strings.IndexByte(cmd[j:], '}')
				if k < 0 {
					k = len(cmd) - j - 1
				}
				cur.WriteString(s.expand(cmd[i : j+k+1]))
				i = j + k
			} else {
				k := j
				for k < len(cmd) && (isIdent(cmd[k]) || (k == j && cmd[k] == '?')) {
					k++
				}
				cur.WriteString(s.expand(cmd[i:k]))
				i = k - 1
			}
			inTok = true
		default:
			cur.WriteByte(c)
			inTok = true
		}
	}
	if inTok {
		toks = append(toks, cur.String())
	}
	return toks, nil
}

func isIdent(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

var reVar = regexp.MustCompile(`\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?`)

func (s *Session) expand(str string) string {
	// command substitution
	for {
		i := strings.Index(str, "$(")
		if i < 0 {
			break
		}
		depth := 1
		k := i + 2
		for k < len(str) && depth > 0 {
			if str[k] == '(' {
				depth++
			} else if str[k] == ')' {
				depth--
			}
			k++
		}
		inner := str[i+2 : k-1]
		r := s.execLine(inner, "")
		str = str[:i] + strings.TrimRight(r.Output, "\n") + str[k:]
	}
	return reVar.ReplaceAllStringFunc(str, func(m string) string {
		name := reVar.FindStringSubmatch(m)[1]
		switch name {
		case "PROJECT_ID", "GOOGLE_CLOUD_PROJECT", "DEVSHELL_PROJECT_ID":
			if v, ok := s.Env[name]; ok {
				return v
			}
			return s.Project
		}
		return s.Env[name]
	})
}

// execSimple runs a single command with redirections.
func (s *Session) execSimple(cmd, stdin string) Result {
	toks, err := s.tokenize(cmd)
	if err != nil {
		return Result{Output: err.Error() + "\n", Exit: 2}
	}
	if len(toks) == 0 {
		return Result{}
	}
	// variable assignment
	if m := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)=(.*)$`).FindStringSubmatch(toks[0]); m != nil && len(toks) == 1 {
		s.Env[m[1]] = m[2]
		return Result{}
	}
	// redirections
	var args []string
	redirect, appendMode := "", false
	mergeErr := false
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch {
		case t == ">" || t == ">>":
			if i+1 < len(toks) {
				redirect, appendMode = toks[i+1], t == ">>"
				i++
			}
		case strings.HasPrefix(t, ">>") && len(t) > 2:
			redirect, appendMode = t[2:], true
		case strings.HasPrefix(t, ">") && len(t) > 1 && t != ">&2":
			redirect = t[1:]
		case t == "2>&1":
			mergeErr = true
		case t == "2>/dev/null":
		case t == "<" && i+1 < len(toks):
			stdin = s.Files[s.path(toks[i+1])]
			i++
		default:
			args = append(args, t)
		}
	}
	_ = mergeErr
	if len(args) == 0 {
		return Result{}
	}
	out, err := s.run(args, stdin)
	res := Result{Output: out}
	if err != nil {
		code := 1
		if ee, ok := err.(*exitErr); ok {
			code = ee.code
		}
		msg := err.Error()
		if msg != "" && !strings.HasSuffix(msg, "\n") {
			msg += "\n"
		}
		res.Output += msg
		res.Exit = code
	}
	if redirect != "" && redirect != "/dev/null" {
		p := s.path(redirect)
		if appendMode {
			s.Files[p] += out
		} else {
			s.Files[p] = out
		}
		res.Output = strings.TrimPrefix(res.Output, out)
	} else if redirect == "/dev/null" {
		res.Output = strings.TrimPrefix(res.Output, out)
	}
	return res
}

func (s *Session) path(p string) string {
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "~/")
	p = strings.TrimPrefix(p, "/home/student/")
	return p
}

func (s *Session) checkDenied(args []string) error {
	line := strings.Join(args, " ")
	for _, d := range s.Policy.DeniedCommands {
		if strings.HasPrefix(line, d) {
			return fail(1, "ERROR: this action is disabled in this lab by the platform policy (%s)", d)
		}
	}
	return nil
}

func (s *Session) run(args []string, stdin string) (string, error) {
	if err := s.checkDenied(args); err != nil {
		return "", err
	}
	if s.Interceptor != nil {
		if handled, out, err := s.Interceptor(s, args, stdin); handled {
			return out, err
		}
	}
	switch args[0] {
	case "gcloud":
		return s.gcloud(args[1:], stdin)
	case "gsutil":
		return s.gsutil(args[1:], stdin)
	case "bq":
		return s.bq(args[1:], stdin)
	case "kubectl":
		return s.kubectl(args[1:], stdin)
	case "terraform", "tf":
		return s.terraform(args[1:])
	case "git":
		return s.git(args[1:])
	case "docker":
		return s.docker(args[1:])
	case "curl", "wget":
		return s.curl(args, sim.InternetEndpoint(sim.StudentIP), "")
	case "nc", "ncat", "telnet":
		return s.nc(args, sim.InternetEndpoint(sim.StudentIP))
	case "ping":
		return s.ping(args, sim.InternetEndpoint(sim.StudentIP))
	case "dig", "nslookup", "host":
		return s.dig(args, sim.Endpoint{Kind: "internet"})
	case "psql", "mysql":
		return s.psql(args)
	case "echo":
		a := args[1:]
		nl := "\n"
		if len(a) > 0 && a[0] == "-n" {
			a, nl = a[1:], ""
		}
		if len(a) > 0 && a[0] == "-e" {
			a = a[1:]
			return strings.ReplaceAll(strings.Join(a, " "), `\n`, "\n") + nl, nil
		}
		return strings.Join(a, " ") + nl, nil
	case "printf":
		if len(args) < 2 {
			return "", nil
		}
		f := strings.ReplaceAll(strings.ReplaceAll(args[1], `\n`, "\n"), `\t`, "\t")
		vals := make([]any, 0)
		for _, a := range args[2:] {
			vals = append(vals, a)
		}
		return fmt.Sprintf(strings.ReplaceAll(f, "%s", "%v"), vals...), nil
	case "cat":
		if len(args) == 1 {
			return stdin, nil
		}
		var b strings.Builder
		for _, f := range args[1:] {
			if strings.HasPrefix(f, "-") {
				continue
			}
			c, ok := s.Files[s.path(f)]
			if !ok {
				return b.String(), fail(1, "cat: %s: No such file or directory", f)
			}
			b.WriteString(c)
		}
		return b.String(), nil
	case "ls":
		keys := sim.SortedKeys(s.Files)
		prefix := ""
		for _, a := range args[1:] {
			if !strings.HasPrefix(a, "-") {
				prefix = strings.TrimSuffix(s.path(a), "/") + "/"
			}
		}
		var out []string
		for _, k := range keys {
			if prefix == "" || strings.HasPrefix(k, prefix) {
				out = append(out, strings.TrimPrefix(k, prefix))
			}
		}
		if len(out) == 0 {
			return "", nil
		}
		return strings.Join(out, "\n") + "\n", nil
	case "rm":
		for _, f := range args[1:] {
			if strings.HasPrefix(f, "-") {
				continue
			}
			p := s.path(f)
			delete(s.Files, p)
			for k := range s.Files {
				if strings.HasPrefix(k, strings.TrimSuffix(p, "/")+"/") {
					delete(s.Files, k)
				}
			}
		}
		return "", nil
	case "touch":
		for _, f := range args[1:] {
			if _, ok := s.Files[s.path(f)]; !ok {
				s.Files[s.path(f)] = ""
			}
		}
		return "", nil
	case "mkdir", "cd", "chmod", "source", ".", "set", "clear", "true", ":", "alias":
		return "", nil
	case "false":
		return "", fail(1, "")
	case "pwd":
		return "/home/student\n", nil
	case "whoami":
		return "student\n", nil
	case "hostname":
		return "cs-" + s.State.ID(8) + "-default\n", nil
	case "date":
		return s.State.Clock.Format("Mon Jan 2 15:04:05 UTC 2006") + "\n", nil
	case "export":
		for _, a := range args[1:] {
			if kv := strings.SplitN(a, "=", 2); len(kv) == 2 {
				s.Env[kv[0]] = kv[1]
			}
		}
		return "", nil
	case "unset":
		for _, a := range args[1:] {
			delete(s.Env, a)
		}
		return "", nil
	case "env", "printenv":
		var b strings.Builder
		for _, k := range sim.SortedKeys(s.Env) {
			b.WriteString(k + "=" + s.Env[k] + "\n")
		}
		return b.String(), nil
	case "history":
		var b strings.Builder
		for i, r := range s.Records {
			b.WriteString(fmt.Sprintf("%5d  %s\n", i+1, r.Line))
		}
		return b.String(), nil
	case "sleep":
		n := 60
		if len(args) > 1 {
			n, _ = strconv.Atoi(strings.TrimSuffix(args[1], "s"))
		}
		ticks := max(1, n/60)
		s.State.Step(ticks)
		return "", nil
	case "grep", "egrep":
		return grep(args[1:], stdin, s)
	case "head", "tail":
		n := 10
		for i := 1; i < len(args); i++ {
			if args[i] == "-n" && i+1 < len(args) {
				n, _ = strconv.Atoi(args[i+1])
				i++
			} else if strings.HasPrefix(args[i], "-") {
				n, _ = strconv.Atoi(strings.TrimPrefix(args[i], "-"))
			}
		}
		ls := splitLines(stdin)
		if args[0] == "head" && len(ls) > n {
			ls = ls[:n]
		} else if args[0] == "tail" && len(ls) > n {
			ls = ls[len(ls)-n:]
		}
		return joinLines(ls), nil
	case "sed":
		return s.sed(args, stdin)
	case "wc":
		ls := splitLines(stdin)
		return fmt.Sprintf("%d\n", len(ls)), nil
	case "sort":
		ls := splitLines(stdin)
		sort.Strings(ls)
		return joinLines(ls), nil
	case "uniq":
		ls := splitLines(stdin)
		var out []string
		for i, l := range ls {
			if i == 0 || l != ls[i-1] {
				out = append(out, l)
			}
		}
		return joinLines(out), nil
	case "awk":
		return awk(args[1:], stdin)
	case "cut":
		return cut(args[1:], stdin)
	case "tee":
		for _, f := range args[1:] {
			if !strings.HasPrefix(f, "-") {
				s.Files[s.path(f)] = stdin
			}
		}
		return stdin, nil
	case "base64":
		if len(args) > 1 && (args[1] == "-d" || args[1] == "--decode") {
			b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(stdin))
			if err != nil {
				return "", fail(1, "base64: invalid input")
			}
			return string(b), nil
		}
		return base64.StdEncoding.EncodeToString([]byte(stdin)) + "\n", nil
	case "jq":
		return jq(args[1:], stdin)
	case "openssl":
		if len(args) >= 3 && args[1] == "rand" {
			return s.State.ID(32) + "\n", nil
		}
		return "", fail(1, "openssl: only `openssl rand -hex N` is supported in the simulator")
	case "ssh", "scp":
		return "", fail(1, "Use `gcloud compute ssh INSTANCE --command=\"...\"` to run commands on a VM.")
	case "help":
		return helpText, nil
	case "ticket", "ask", "team":
		return s.deskCmd(args, stdin)
	case "why":
		return s.whyCmd(args)
	case "whatif":
		return s.whatifCmd(args, stdin)
	case "arch":
		return s.archCmd(args)
	case "chaos":
		return s.chaosCmd(args)
	case "interview", "answer":
		return s.interviewCmd(args)
	case "watch":
		return s.run(args[1:], stdin)
	case "uuidgen":
		return s.State.ID(8) + "-" + s.State.ID(4) + "-" + s.State.ID(4) + "-" + s.State.ID(12) + "\n", nil
	}
	return "", fail(127, "bash: %s: command not found", args[0])
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func joinLines(ls []string) string {
	if len(ls) == 0 {
		return ""
	}
	return strings.Join(ls, "\n") + "\n"
}

func grep(args []string, in string, s *Session) (string, error) {
	inv, ci, count, quiet := false, false, false, false
	var pat string
	var files []string
	for _, a := range args {
		switch {
		case a == "-v":
			inv = true
		case a == "-i":
			ci = true
		case a == "-c":
			count = true
		case a == "-q":
			quiet = true
		case a == "-E" || a == "-e" || a == "-n" || a == "-w":
		case strings.HasPrefix(a, "-") && len(a) > 1 && pat == "":
			for _, c := range a[1:] {
				switch c {
				case 'v':
					inv = true
				case 'i':
					ci = true
				case 'c':
					count = true
				case 'q':
					quiet = true
				}
			}
		case pat == "":
			pat = a
		default:
			files = append(files, a)
		}
	}
	if len(files) > 0 {
		in = ""
		for _, f := range files {
			in += s.Files[s.path(f)]
		}
	}
	expr := pat
	if ci {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		re = regexp.MustCompile(regexp.QuoteMeta(pat))
	}
	var out []string
	for _, l := range splitLines(in) {
		if re.MatchString(l) != inv {
			out = append(out, l)
		}
	}
	if count {
		return fmt.Sprintf("%d\n", len(out)), nil
	}
	if len(out) == 0 {
		return "", fail(1, "")
	}
	if quiet {
		return "", nil
	}
	return joinLines(out), nil
}

func awk(args []string, in string) (string, error) {
	prog := ""
	sep := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "-F" && i+1 < len(args) {
			sep = args[i+1]
			i++
		} else if strings.HasPrefix(args[i], "-F") {
			sep = args[i][2:]
		} else {
			prog = args[i]
		}
	}
	m := regexp.MustCompile(`print\s+(.+?)\s*}`).FindStringSubmatch(prog)
	if m == nil {
		return in, nil
	}
	fields := strings.Split(m[1], ",")
	var out []string
	for _, l := range splitLines(in) {
		var f []string
		if sep != "" {
			f = strings.Split(l, sep)
		} else {
			f = strings.Fields(l)
		}
		var parts []string
		for _, fd := range fields {
			fd = strings.TrimSpace(fd)
			if fd == "$0" {
				parts = append(parts, l)
				continue
			}
			if strings.HasPrefix(fd, "$") {
				n, _ := strconv.Atoi(fd[1:])
				if n >= 1 && n <= len(f) {
					parts = append(parts, f[n-1])
				}
			} else {
				parts = append(parts, strings.Trim(fd, `"`))
			}
		}
		out = append(out, strings.Join(parts, " "))
	}
	return joinLines(out), nil
}

func cut(args []string, in string) (string, error) {
	d, fld := "\t", 1
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-d" && i+1 < len(args):
			d = args[i+1]
			i++
		case strings.HasPrefix(args[i], "-d"):
			d = args[i][2:]
		case args[i] == "-f" && i+1 < len(args):
			fld, _ = strconv.Atoi(args[i+1])
			i++
		case strings.HasPrefix(args[i], "-f"):
			fld, _ = strconv.Atoi(args[i][2:])
		}
	}
	var out []string
	for _, l := range splitLines(in) {
		p := strings.Split(l, d)
		if fld >= 1 && fld <= len(p) {
			out = append(out, p[fld-1])
		}
	}
	return joinLines(out), nil
}

func nowStamp() string { return time.Now().UTC().Format(time.RFC3339) }

const helpText = `GCP Lab Simulator terminal (F0). Available tools:
  gcloud    compute, network, storage, iam, run, sql, pubsub, secrets, kms,
            container, artifacts, builds, deploy, logging, monitoring, dns,
            ai, scc, services, config, auth, projects, folders, organizations,
            org-policies, billing, recommender
  gsutil    mb, ls, cp, cat, rm, iam ch, lifecycle, versioning, pap, ubla
  bq        mk, ls, show, query (--dry_run), rm
  kubectl   apply, get, describe, logs, scale, set, rollout, autoscale,
            expose, create, delete, run, exec, top, auth can-i
  terraform init, validate, plan, apply, destroy, output, fmt, show,
            import, state (list|show|rm|mv), -target, -refresh-only
  git, docker, curl, nc, ping, dig, psql
  desk      ticket (show|comment|update|resolve|escalate), team, ask WHO "question"
  mentor    why TARGET (causal chain of a request), whatif COMMAND (impact preview)
  arch      arch evaluate design.yaml requirements.yaml (architecture simulator)
  chaos     chaos run zone-outage|region-outage|kill-instance|stop-service --target=...,
            chaos history
  finops    billing report, gcloud recommender recommendations list --recommender=ID
  interview interview (next question), answer ID N[,M] ["justification"]
  vm (ssh)  ls -l, stat, id, groups, usermod, chmod, chown, df -h, du, find,
            truncate, logrotate, systemctl, journalctl, sudo
  shell     echo, cat, ls, rm, export, env, grep, head, tail, wc, awk, cut, sed,
            jq (incl. select, keys), base64, sleep (advances simulated time), history
Tips: pipes (|), &&, ||, ;, > and >> redirections, heredocs (<<EOF),
$(command) substitution and for/while/if blocks are supported. Use the Files
tab to edit files.
`

// Clone copies a session onto another state (grading, what-if analysis).
func (s *Session) Clone(st *sim.State) *Session {
	c := *s
	c.State = st
	c.NoTick = true
	c.Files = map[string]string{}
	for k, v := range s.Files {
		c.Files[k] = v
	}
	c.Env = map[string]string{}
	for k, v := range s.Env {
		c.Env[k] = v
	}
	c.Records = append([]ExecRecord{}, s.Records...)
	c.Desk = s.Desk.Clone()
	c.Chaos = append([]ChaosRun{}, s.Chaos...)
	if s.Interview != nil {
		iv := *s.Interview
		iv.Answers, iv.Justifications = map[string][]int{}, map[string]string{}
		for k, v := range s.Interview.Answers {
			iv.Answers[k] = v
		}
		for k, v := range s.Interview.Justifications {
			iv.Justifications[k] = v
		}
		c.Interview = &iv
	}
	return &c
}
