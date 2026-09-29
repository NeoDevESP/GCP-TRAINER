package cli

import (
	"fmt"
	"strings"
)

// Remote is an interactive session opened from Cloud Shell: an SSH login to
// a VM (gcloud compute ssh) or a database client (gcloud sql connect, psql,
// mysql). While it is open, every line goes to it; exit returns to Cloud
// Shell. Lines are recorded as their non-interactive equivalent so grading
// sees the same commands.
type Remote struct {
	Kind     string `json:"kind"` // ssh, psql, mysql
	Project  string `json:"project"`
	VM       string `json:"vm,omitempty"`
	Zone     string `json:"zone,omitempty"`
	Instance string `json:"instance,omitempty"`
	Host     string `json:"host,omitempty"`
	DB       string `json:"db,omitempty"`
	User     string `json:"user,omitempty"`
	Buf      string `json:"buf,omitempty"`
	// Redis is set while redis-cli runs interactively inside the SSH session.
	Redis     string `json:"redis,omitempty"`
	RedisAuth bool   `json:"redisAuth,omitempty"`
}

// Prompt is the prompt the terminal shows for the next line.
func (s *Session) Prompt() string {
	r := s.Remote
	switch {
	case r == nil:
		return fmt.Sprintf("student@cloudshell:~ (%s)$ ", s.Project)
	case r.Kind == "ssh" && r.Redis != "":
		return r.Redis + ":6379> "
	case r.Kind == "ssh":
		if s.RootShell {
			return fmt.Sprintf("root@%s:~# ", r.VM)
		}
		return fmt.Sprintf("student@%s:~$ ", r.VM)
	case r.Kind == "mysql":
		if strings.TrimSpace(r.Buf) != "" {
			return "    -> "
		}
		return "mysql> "
	}
	if strings.TrimSpace(r.Buf) != "" {
		return r.DB + "-> "
	}
	return r.DB + "=> "
}

func (s *Session) remoteExec(input string) Result {
	var out strings.Builder
	exit := 0
	for _, line := range strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n") {
		if s.Remote == nil {
			// exit closed the session: the rest runs in Cloud Shell.
			r := s.Exec(line)
			out.WriteString(r.Output)
			exit = r.Exit
			continue
		}
		o, code, rec := s.remoteLine(line)
		out.WriteString(o)
		exit = code
		if rec != "" {
			s.Records = append(s.Records, ExecRecord{Line: rec, Exit: code, Tool: firstWord(rec), Output: truncate(o, 400), At: s.State.Now()})
			if !s.NoTick {
				s.State.Step(1)
			}
		}
	}
	return Result{Output: out.String(), Exit: exit, Prompt: s.Prompt()}
}

// remoteLine runs one line in the remote session and returns its output,
// exit code and the command to record.
func (s *Session) remoteLine(line string) (string, int, string) {
	r := s.Remote
	trim := strings.TrimSpace(line)
	switch r.Kind {
	case "ssh":
		if trim == "" {
			return "", 0, ""
		}
		if r.Redis != "" {
			if l := strings.ToLower(trim); l == "exit" || l == "quit" {
				r.Redis = ""
				return "", 0, ""
			}
			inst := s.findRedis(r.Redis)
			if inst == nil {
				r.Redis = ""
				return "Error: Connection reset by peer\n", 1, ""
			}
			args, err := s.tokenize(trim)
			if err != nil || len(args) == 0 {
				return "Invalid argument(s)\n", 1, ""
			}
			o := redisExec(inst, &r.RedisAuth, args)
			return o + "\n", 0, fmt.Sprintf("gcloud compute ssh %s --zone=%s --command=%s", r.VM, r.Zone, shellQuote("redis-cli -h "+r.Redis+" "+trim))
		}
		if trim == "exit" || trim == "logout" {
			if s.RootShell {
				s.RootShell = false
				return "logout\n", 0, ""
			}
			s.Remote = nil
			return fmt.Sprintf("logout\nConnection to %s closed.\n", r.VM), 0, ""
		}
		if trim == "sudo -i" || trim == "sudo su" || trim == "sudo su -" || trim == "sudo -s" {
			s.RootShell = true
			return "", 0, ""
		}
		p := s.State.Projects[r.Project]
		vm := p.Instances[r.VM]
		if vm == nil {
			s.Remote = nil
			return fmt.Sprintf("Connection to %s closed by remote host.\n", r.VM), 255, ""
		}
		if vm.Status != "RUNNING" {
			s.Remote = nil
			return fmt.Sprintf("Connection to %s closed by remote host.\nConnection to %s closed.\n", r.VM, r.VM), 255, ""
		}
		cmd := trim
		if s.RootShell && !strings.HasPrefix(cmd, "sudo ") {
			cmd = "sudo " + cmd
		}
		o, err := s.vmExec(p, vm, cmd)
		code := 0
		if err != nil {
			code = 1
			if ee, ok := err.(*exitErr); ok {
				code = ee.code
				o += ee.msg
			} else {
				o += err.Error()
			}
			if o != "" && !strings.HasSuffix(o, "\n") {
				o += "\n"
			}
		}
		return o, code, fmt.Sprintf("gcloud compute ssh %s --zone=%s --command=%s", r.VM, r.Zone, shellQuote(cmd))
	case "psql", "mysql":
		lower := strings.ToLower(trim)
		if r.Buf == "" && (lower == `\q` || lower == "exit" || lower == "quit" || lower == `\quit`) {
			s.Remote = nil
			if r.Kind == "mysql" {
				return "Bye\n", 0, ""
			}
			return "", 0, ""
		}
		if trim == "" && r.Buf == "" {
			return "", 0, ""
		}
		if r.Buf == "" && strings.HasPrefix(trim, `\`) {
			return s.remoteSQL(trim)
		}
		r.Buf += line + "\n"
		if !strings.HasSuffix(strings.TrimSpace(r.Buf), ";") && !(r.Kind == "mysql" && strings.HasPrefix(lower, "use ")) {
			return "", 0, ""
		}
		stmt := strings.TrimSpace(r.Buf)
		r.Buf = ""
		return s.remoteSQL(stmt)
	}
	s.Remote = nil
	return "", 0, ""
}

func (s *Session) remoteSQL(stmt string) (string, int, string) {
	r := s.Remote
	p := s.State.Projects[r.Project]
	in := p.SQLInstances[r.Instance]
	if in == nil || in.State != "RUNNABLE" {
		s.Remote = nil
		return "server closed the connection unexpectedly\nThe connection to the server was lost.\n", 2, ""
	}
	out, db := s.State.RunSQL(in, r.DB, stmt)
	r.DB = db
	code := 0
	if strings.HasPrefix(out, "ERROR") {
		code = 1
	}
	if out != "" {
		out += "\n"
	}
	sql := shellQuote(strings.ReplaceAll(stmt, "\n", " "))
	if r.Kind == "mysql" {
		return out, code, fmt.Sprintf("mysql -h %s -u %s %s -e %s", r.Host, r.User, r.DB, sql)
	}
	return out, code, fmt.Sprintf("psql -h %s -U %s -d %s -c %s", r.Host, r.User, r.DB, sql)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
