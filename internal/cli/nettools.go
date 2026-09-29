package cli

import (
	"encoding/base64"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

// IdentityToken builds a simulated OIDC token for a principal.
func IdentityToken(principal string) string {
	return "eyJhbGciOiJSUzI1NiJ9.sim." + base64.RawURLEncoding.EncodeToString([]byte(principal)) + ".sig"
}

func principalFromToken(tok string) string {
	parts := strings.Split(tok, ".")
	if len(parts) >= 3 && parts[1] == "sim" {
		if b, err := base64.RawURLEncoding.DecodeString(parts[2]); err == nil {
			return string(b)
		}
	}
	return ""
}

func (s *Session) curl(args []string, from sim.Endpoint, defaultPrincipal string) (string, error) {
	_, f := parseArgs(args[1:])
	var url string
	headers := []string{}
	var outFile string
	var writeOut string
	showHead := false
	fail2 := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-H" || a == "--header":
			if i+1 < len(args) {
				headers = append(headers, args[i+1])
				i++
			}
		case a == "-o" || a == "--output" || a == "-O":
			if a != "-O" && i+1 < len(args) {
				outFile = args[i+1]
				i++
			}
		case a == "-w" || a == "--write-out":
			if i+1 < len(args) {
				writeOut = args[i+1]
				i++
			}
		case a == "-X" || a == "-d" || a == "--data" || a == "--max-time" || a == "-m" || a == "--connect-timeout" || a == "-u" || a == "-T":
			i++
		case a == "-I" || a == "--head" || a == "-i":
			showHead = true
		case a == "-f" || a == "--fail" || a == "-fsSL" || a == "-fsS" || a == "-sf":
			fail2 = true
		case strings.HasPrefix(a, "-"):
		default:
			if url == "" {
				url = a
			}
		}
	}
	_ = f
	if url == "" {
		return "", fail(2, "curl: no URL specified!")
	}
	principal := defaultPrincipal
	for _, h := range headers {
		if strings.HasPrefix(strings.ToLower(h), "authorization: bearer ") {
			if p := principalFromToken(strings.TrimSpace(h[len("authorization: bearer "):])); p != "" {
				principal = p
			}
		}
	}
	if strings.Contains(url, "metadata.google.internal") || strings.Contains(url, "169.254.169.254") {
		if from.Kind != "vm" {
			return "", fail(6, "curl: (6) Could not resolve host: metadata.google.internal")
		}
		return s.metadata(from, url, headers)
	}
	res := s.State.HTTP(sim.HTTPRequest{From: from, Principal: principal, URL: url, SourceIP: from.IP})
	if res.Status == 0 {
		code := 7
		if strings.Contains(res.Error, "timed out") {
			code = 28
		} else if strings.Contains(res.Error, "resolve") {
			code = 6
		}
		return "", fail(code, "curl: (%d) %s", code, res.Error)
	}
	body := res.Body + "\n"
	if showHead {
		body = fmt.Sprintf("HTTP/1.1 %d %s\ncontent-type: text/html; charset=utf-8\nvia: 1.1 google\n\n", res.Status, statusText(res.Status))
	}
	if writeOut != "" {
		wo := strings.ReplaceAll(writeOut, "%{http_code}", strconv.Itoa(res.Status))
		wo = strings.ReplaceAll(wo, `\n`, "\n")
		if outFile == "/dev/null" {
			body = wo
		} else {
			body += wo
		}
	}
	if outFile != "" && outFile != "/dev/null" {
		s.Files[s.path(outFile)] = res.Body
		body = ""
	}
	if fail2 && res.Status >= 400 {
		return "", fail(22, "curl: (22) The requested URL returned error: %d", res.Status)
	}
	return body, nil
}

func statusText(c int) string {
	switch c {
	case 200:
		return "OK"
	case 403:
		return "Forbidden"
	case 404:
		return "Not Found"
	case 500:
		return "Internal Server Error"
	case 502:
		return "Bad Gateway"
	case 503:
		return "Service Unavailable"
	case 504:
		return "Gateway Timeout"
	}
	return ""
}

func (s *Session) metadata(from sim.Endpoint, url string, headers []string) (string, error) {
	ok := false
	for _, h := range headers {
		if strings.EqualFold(strings.ReplaceAll(h, " ", ""), "metadata-flavor:google") {
			ok = true
		}
	}
	if !ok {
		return "", fail(22, "Missing Metadata-Flavor:Google header.")
	}
	p := s.State.Projects[from.Project]
	vm := p.Instances[from.Name]
	switch {
	case strings.Contains(url, "service-accounts/default/email"):
		return vm.ServiceAccount + "\n", nil
	case strings.Contains(url, "service-accounts/default/scopes"):
		return strings.Join(vm.Scopes, "\n") + "\n", nil
	case strings.Contains(url, "project/project-id"):
		return p.ID + "\n", nil
	case strings.Contains(url, "instance/zone"):
		return "projects/" + p.Number + "/zones/" + vm.Zone + "\n", nil
	case strings.Contains(url, "instance/tags"):
		return fmt.Sprintf("%q\n", vm.Tags), nil
	case strings.Contains(url, "service-accounts/default/token"):
		return `{"access_token":"ya29.sim-` + s.State.ID(20) + `","expires_in":3599,"token_type":"Bearer"}` + "\n", nil
	case strings.Contains(url, "instance/attributes/"):
		k := url[strings.LastIndex(url, "/")+1:]
		return vm.Metadata[k] + "\n", nil
	}
	return "", fail(22, "curl: (22) The requested URL returned error: 404")
}

func (s *Session) nc(args []string, from sim.Endpoint) (string, error) {
	var pos []string
	for _, a := range args[1:] {
		if !strings.HasPrefix(a, "-") {
			pos = append(pos, a)
		}
	}
	if len(pos) < 2 {
		return "", fail(1, "usage: nc -zv HOST PORT")
	}
	port, err := strconv.Atoi(pos[1])
	if err != nil {
		return "", fail(1, "nc: port number invalid: %s", pos[1])
	}
	ok, _, msg := s.State.TCPConnect(from, pos[0], port)
	if !ok {
		return "", fail(1, "nc: %s", msg)
	}
	return msg + "\n", nil
}

func (s *Session) ping(args []string, from sim.Endpoint) (string, error) {
	host := ""
	for i := 1; i < len(args); i++ {
		if args[i] == "-c" || args[i] == "-W" || args[i] == "-i" {
			i++
			continue
		}
		if !strings.HasPrefix(args[i], "-") {
			host = args[i]
		}
	}
	ok, out := s.State.Ping(from, host)
	if !ok {
		return out + "\n", fail(1, "")
	}
	return out + "\n", nil
}

func (s *Session) dig(args []string, from sim.Endpoint) (string, error) {
	host := ""
	short := false
	for _, a := range args[1:] {
		if a == "+short" {
			short = true
		} else if !strings.HasPrefix(a, "+") && !strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "@") && a != "A" {
			host = a
		}
	}
	if host == "" {
		return "", fail(1, "usage: dig HOST")
	}
	ip, ok := s.State.ResolveHost(host, from)
	if !ok {
		if args[0] == "dig" {
			return fmt.Sprintf(";; ->>HEADER<<- opcode: QUERY, status: NXDOMAIN\n;; QUESTION SECTION:\n;%s.\t\tIN\tA\n", host), nil
		}
		return "", fail(1, "** server can't find %s: NXDOMAIN", host)
	}
	if short {
		return ip + "\n", nil
	}
	if args[0] == "dig" {
		return fmt.Sprintf(";; ->>HEADER<<- opcode: QUERY, status: NOERROR\n;; ANSWER SECTION:\n%s.\t300\tIN\tA\t%s\n", host, ip), nil
	}
	return fmt.Sprintf("Name:\t%s\nAddress: %s\n", host, ip), nil
}

func (s *Session) psql(args []string) (string, error) {
	return s.psqlFrom(args, sim.InternetEndpoint(sim.StudentIP))
}

func (s *Session) psqlFrom(args []string, from sim.Endpoint) (string, error) {
	host, user, db, cmd := "", "postgres", "postgres", ""
	isMy := args[0] == "mysql"
	port := 5432
	if isMy {
		user, db, port = "root", "", 3306
	}
	for i := 1; i < len(args); i++ {
		a := args[i]
		next := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch {
		case a == "-h" || a == "--host":
			host = next()
		case a == "-U" || a == "--username":
			user = next()
		case a == "-d" || a == "--dbname":
			db = next()
		case a == "-c" || a == "--command" || a == "-e" || a == "--execute":
			cmd = next()
		case a == "-u" || a == "--user":
			user = next()
		case a == "-D" || a == "--database":
			db = next()
		case strings.HasPrefix(a, "--user="):
			user = strings.TrimPrefix(a, "--user=")
		case strings.HasPrefix(a, "--database="):
			db = strings.TrimPrefix(a, "--database=")
		case strings.HasPrefix(a, "--password") || isMy && strings.HasPrefix(a, "-p"):
			if isMy && len(a) > 2 && strings.HasPrefix(a, "-p") {
				s.Env["MYSQL_PWD"] = a[2:]
			}
		case a == "-P" || a == "--port":
			next()
		case a == "-p":
			next()
		case !strings.HasPrefix(a, "-") && isMy && db == "":
			db = a
		case strings.HasPrefix(a, "host="):
			for _, kv := range strings.Fields(a) {
				k, v, _ := strings.Cut(kv, "=")
				switch k {
				case "host":
					host = v
				case "user":
					user = v
				case "dbname":
					db = v
				}
			}
		}
	}
	if host == "" {
		return "", fail(2, "psql: error: connection to server on socket \"/var/run/postgresql/.s.PGSQL.5432\" failed: No such file or directory")
	}
	ok, _, msg := s.State.TCPConnect(from, host, port)
	if !ok {
		if isMy {
			return "", fail(1, "ERROR 2003 (HY000): Can't connect to MySQL server on '%s:3306' (%s)", host, strings.TrimPrefix(msg, "connect to "))
		}
		return "", fail(2, "psql: error: connection to server at \"%s\", port 5432 failed: %s", host, strings.TrimPrefix(msg, "connect to "))
	}
	ip, _ := s.State.ResolveHost(host, from)
	ep, found := s.State.FindIP(ip)
	if !found || ep.Kind != "sql" {
		return fmt.Sprintf("psql (15.8)\nConnected to %s as %s.\n", host, user), nil
	}
	in := s.State.Projects[ep.Project].SQLInstances[ep.Name]
	if _, ok := in.Users[user]; !ok {
		return "", fail(2, "psql: error: connection to server at \"%s\", port 5432 failed: FATAL:  password authentication failed for user \"%s\"", host, user)
	}
	if pw := s.Env["PGPASSWORD"]; pw != "" && pw != in.Users[user] {
		return "", fail(2, "psql: error: FATAL:  password authentication failed for user \"%s\"", user)
	}
	if isMy && db == "" {
		db = "mysql"
	}
	if !contains(in.Databases, db) && db != "postgres" && db != "mysql" {
		return "", fail(2, "psql: error: FATAL:  database \"%s\" does not exist", db)
	}
	if cmd == "" {
		kind := "psql"
		if isMy {
			kind = "mysql"
		}
		if from.Kind == "internet" || from.Kind == "" {
			s.Remote = &Remote{Kind: kind, Project: ep.Project, Instance: ep.Name, Host: host, DB: db, User: user}
			if isMy {
				return "Welcome to the MySQL monitor.  Commands end with ;\nServer version: 8.0.39-google (Google)\n\n", nil
			}
			return "psql (15.8)\nSSL connection (protocol: TLSv1.3)\nType \"help\" for help.\n\n", nil
		}
		return fmt.Sprintf("psql (15.8)\n%s=> (interactive mode is only available from Cloud Shell; use -c \"SQL\")\n", db), nil
	}
	return s.State.ApplySQL(in, db, cmd) + "\n", nil
}

// vmExec runs a command line inside a VM (via gcloud compute ssh --command).
func (s *Session) vmExec(p *sim.Project, vm *sim.Instance, line string) (string, error) {
	from := s.State.VMEndpoint(p.ID, vm)
	var out strings.Builder
	var lastErr error
	cmds, ops := splitTop(line, []string{"&&", "||", ";"})
	for i, c := range cmds {
		if i > 0 {
			if ops[i-1] == "&&" && lastErr != nil {
				continue
			}
			if ops[i-1] == "||" && lastErr == nil {
				continue
			}
		}
		stages, _ := splitTop(c, []string{"|"})
		input := ""
		var stageErr error
		for _, st := range stages {
			toks, err := s.tokenize(strings.TrimSpace(st))
			if err != nil || len(toks) == 0 {
				continue
			}
			s.vmRoot = false
			if toks[0] == "sudo" {
				s.vmRoot = true
				toks = toks[1:]
				if len(toks) > 0 && toks[0] == "-u" {
					toks = toks[2:]
				}
			}
			if len(toks) == 0 {
				continue
			}
			o, err := s.vmCommand(p, vm, from, toks, input)
			input = o
			stageErr = err
		}
		out.WriteString(input)
		lastErr = stageErr
		if stageErr != nil && stageErr.Error() != "" {
			out.WriteString(stageErr.Error() + "\n")
		}
	}
	if lastErr != nil {
		return "", fail(1, "%s", strings.TrimRight(out.String(), "\n"))
	}
	return out.String(), nil
}

func (s *Session) vmCommand(p *sim.Project, vm *sim.Instance, from sim.Endpoint, toks []string, stdin string) (string, error) {
	switch toks[0] {
	case "curl", "wget":
		return s.curl(toks, from, "serviceAccount:"+vm.ServiceAccount)
	case "nc", "ncat", "telnet":
		return s.nc(toks, from)
	case "ping":
		return s.ping(toks, from)
	case "dig", "nslookup", "host":
		return s.dig(toks, from)
	case "psql":
		return s.psqlFrom(toks, from)
	case "hostname":
		return vm.Name + "\n", nil
	case "ip":
		return fmt.Sprintf("2: ens4: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1460\n    inet %s/32 scope global dynamic ens4\n", vm.InternalIP), nil
	case "apt", "apt-get", "yum", "dnf":
		if ok, why := s.State.InternetPath(from); !ok {
			return "", fail(100, "Err:1 http://deb.debian.org/debian bookworm InRelease\n  Could not connect to deb.debian.org:80 (151.101.2.132), connection timed out\nE: Unable to fetch some archives (%s)", why)
		}
		if len(toks) > 1 && toks[1] == "install" {
			for _, pkg := range toks[2:] {
				if strings.HasPrefix(pkg, "-") {
					continue
				}
				if !strings.Contains(vm.Metadata["sim-preinstalled"], pkg) {
					if vm.Metadata["sim-preinstalled"] != "" {
						vm.Metadata["sim-preinstalled"] += ","
					}
					vm.Metadata["sim-preinstalled"] += pkg
				}
				if !strings.Contains(vm.Metadata["startup-script"], "install -y "+pkg) {
					vm.Metadata["startup-script"] += "\napt-get install -y " + pkg
				}
			}
			return "Reading package lists... Done\nSetting up packages ... done.\n", nil
		}
		return "Hit:1 http://deb.debian.org/debian bookworm InRelease\nReading package lists... Done\n", nil
	case "systemctl", "service":
		verb, svc := "", ""
		if toks[0] == "service" && len(toks) >= 3 {
			svc, verb = toks[1], toks[2]
		} else if len(toks) >= 3 {
			verb, svc = toks[1], toks[2]
		}
		svc = strings.TrimSuffix(svc, ".service")
		ls, _ := s.State.VMListeners(p.ID, vm)
		var l *sim.Listener
		for i := range ls {
			if ls[i].Name == svc || (svc == "postgresql" && ls[i].Name == "postgresql") || strings.HasPrefix(ls[i].Name, svc) {
				l = &ls[i]
			}
		}
		key := fmt.Sprintf("svc:%s/%s/%s", p.ID, vm.Name, svc)
		if l == nil {
			return "", fail(4, "Unit %s.service could not be found.", svc)
		}
		switch verb {
		case "status":
			st := "active (running)"
			if !l.Running {
				st = "failed (Result: exit-code) — " + l.Error
			}
			return fmt.Sprintf("● %s.service\n     Loaded: loaded (/lib/systemd/system/%s.service; enabled)\n     Active: %s\n", svc, svc, st), nil
		case "start", "restart":
			delete(s.State.Extra, key)
			delete(s.State.Extra, key+":why")
			if ls2, _ := s.State.VMListeners(p.ID, vm); true {
				for _, x := range ls2 {
					if x.Name == l.Name && !x.Running {
						return "", fail(1, "Job for %s.service failed because the control process exited with error code.\nSee \"systemctl status %s.service\" and \"journalctl -xeu %s.service\" for details.", svc, svc, svc)
					}
				}
			}
			return "", nil
		case "stop":
			s.State.Extra[key] = "stopped"
			return "", nil
		}
		return "", nil
	case "ss", "netstat":
		ls, _ := s.State.VMListeners(p.ID, vm)
		var b strings.Builder
		b.WriteString("State   Recv-Q  Send-Q  Local Address:Port  Peer Address:Port  Process\nLISTEN  0       128     0.0.0.0:22          0.0.0.0:*          sshd\n")
		for _, l := range ls {
			if l.Running {
				b.WriteString(fmt.Sprintf("LISTEN  0       511     %s:%d  0.0.0.0:*  %s\n", l.Bind, l.Port, l.Name))
			}
		}
		return b.String(), nil
	case "ps":
		ls, _ := s.State.VMListeners(p.ID, vm)
		var b strings.Builder
		b.WriteString("USER  PID  COMMAND\nroot  1    /sbin/init\nroot  512  /usr/bin/google_guest_agent\n")
		for i, l := range ls {
			if l.Running {
				b.WriteString(fmt.Sprintf("root  %d  %s\n", 900+i, l.Name))
			}
		}
		return b.String(), nil
	case "journalctl":
		_, console := s.State.VMListeners(p.ID, vm)
		return strings.Join(console, "\n") + "\n", nil
	case "ls", "chmod", "chown", "chgrp", "df", "du", "rm", "truncate", "find", "stat", "id", "usermod", "groups", "logrotate":
		return s.vmOSCommand(vm, toks)
	case "cat", "grep", "head", "tail", "wc", "awk", "echo":
		if toks[0] == "cat" && len(toks) > 1 && vm.OS != nil {
			if f := vm.OS.Files[toks[1]]; f != nil {
				user := "student"
				if s.vmRoot {
					user = "root"
				}
				if f.Dir {
					return "", fail(1, "cat: %s: Is a directory", toks[1])
				}
				if !vm.OS.CanRead(user, toks[1]) {
					return "", fail(1, "cat: %s: Permission denied", toks[1])
				}
				if f.Content == "" && f.SizeMB > 0 {
					return fmt.Sprintf("(%.0f MB of %s data)\n", f.SizeMB, path.Base(toks[1])), nil
				}
				return f.Content, nil
			}
		}
		if toks[0] == "cat" && len(toks) > 1 {
			switch toks[1] {
			case "/etc/hosts":
				return fmt.Sprintf("127.0.0.1 localhost\n%s %s.%s.c.%s.internal %s\n169.254.169.254 metadata.google.internal\n", vm.InternalIP, vm.Name, vm.Zone, p.ID, vm.Name), nil
			case "/etc/resolv.conf":
				return "nameserver 169.254.169.254\nsearch " + vm.Zone + ".c." + p.ID + ".internal c." + p.ID + ".internal google.internal\n", nil
			}
		}
		return s.run(toks, stdin)
	case "gsutil", "gcloud", "bq":
		if ok, why := s.State.GoogleAPIAccess(from); !ok {
			return "", fail(1, "ERROR: gcloud crashed (TransportError): HTTPSConnectionPool(host='oauth2.googleapis.com', port=443): Max retries exceeded (Connection timed out) — %s", why)
		}
		if vm.ServiceAccount == "" {
			return "", fail(1, "ERROR: (gcloud) You do not currently have an active account selected (the instance has no service account).")
		}
		line := strings.Join(toks, " ")
		isStorage := toks[0] == "gsutil" || strings.Contains(line, " storage ")
		write := isStorage && (strings.Contains(line, " cp ") && strings.Contains(strings.SplitN(line, " cp ", 2)[1], "gs://") && !strings.HasPrefix(strings.Fields(strings.SplitN(line, " cp ", 2)[1])[0], "gs://") || strings.Contains(line, " rm ") || strings.Contains(line, " mb "))
		allowed := false
		for _, sc := range vm.Scopes {
			switch sc {
			case "cloud-platform":
				allowed = true
			case "storage-rw", "storage-full":
				allowed = allowed || isStorage
			case "storage-ro", "default":
				allowed = allowed || (isStorage && !write)
			case "bigquery":
				allowed = allowed || toks[0] == "bq"
			case "pubsub":
				allowed = allowed || strings.Contains(line, " pubsub ")
			}
		}
		if !allowed {
			return "", fail(1, "ERROR: (gcloud) HTTPError 403: Request had insufficient authentication scopes. (instance access scopes: %s)", strings.Join(vm.Scopes, ","))
		}
		sub := NewSession(s.State, p.ID, vm.ServiceAccount)
		sub.Zone, sub.Region, sub.NoTick = vm.Zone, sim.RegionOf(vm.Zone), true
		for k, v := range s.Files {
			sub.Files[k] = v
		}
		return sub.run(toks, stdin)
	case "docker":
		return "CONTAINER ID   IMAGE   STATUS\n", nil
	case "whoami":
		return "student\n", nil
	case "true":
		return "", nil
	}
	return "", fail(127, "bash: %s: command not found (on %s)", toks[0], vm.Name)
}
