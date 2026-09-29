package cli

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

// redis-cli against Memorystore. Instances only have a private IP in their
// VPC, so the client works from a VM in that network (after installing
// redis-tools), not from Cloud Shell.

func (s *Session) findRedis(host string) *sim.RedisInstance {
	for _, pid := range sim.SortedKeys(s.State.Projects) {
		for _, r := range s.State.Projects[pid].Redis {
			if r.Host == host {
				return r
			}
		}
	}
	return nil
}

func (s *Session) redisCLI(vm *sim.Instance, toks []string) (string, error) {
	if !strings.Contains(vm.Metadata["sim-preinstalled"], "redis-tools") && !strings.Contains(vm.Metadata["sim-preinstalled"], "redis") {
		return "", fail(127, "bash: redis-cli: command not found (install it with: sudo apt-get install -y redis-tools)")
	}
	host, port, auth := "127.0.0.1", "6379", ""
	var cmd []string
	for i := 1; i < len(toks); i++ {
		switch a := toks[i]; {
		case a == "-h" && i+1 < len(toks):
			host = toks[i+1]
			i++
		case a == "-p" && i+1 < len(toks):
			port = toks[i+1]
			i++
		case a == "-a" && i+1 < len(toks):
			auth = toks[i+1]
			i++
		case a == "--no-auth-warning", a == "--raw":
		default:
			cmd = append(cmd, a)
		}
	}
	r := s.findRedis(host)
	if r == nil || port != "6379" || r.Network != vm.Network || r.State != "READY" {
		return "", fail(1, "Could not connect to Redis at %s:%s: Connection timed out", host, port)
	}
	authed := !r.AuthEnabled || auth == r.AuthString
	if auth != "" && !authed {
		return "", fail(1, "AUTH failed: WRONGPASS invalid username-password pair or user is disabled.")
	}
	if len(cmd) == 0 {
		if s.Remote == nil || s.Remote.Kind != "ssh" {
			return "", fail(1, "redis-cli needs an interactive terminal here: open one with gcloud compute ssh VM, or pass the command (redis-cli -h %s PING)", host)
		}
		s.Remote.Redis, s.Remote.RedisAuth = host, authed
		return "", nil
	}
	return redisExec(r, &authed, cmd) + "\n", nil
}

// redisExec runs one Redis command and returns the reply as redis-cli prints it.
func redisExec(r *sim.RedisInstance, authed *bool, args []string) string {
	if r.Data == nil {
		r.Data = map[string]string{}
	}
	name := strings.ToUpper(args[0])
	arg := func(i int) string {
		if i < len(args) {
			return args[i]
		}
		return ""
	}
	if name == "AUTH" {
		if arg(len(args)-1) == r.AuthString && r.AuthEnabled {
			*authed = true
			return "OK"
		}
		return "(error) WRONGPASS invalid username-password pair or user is disabled."
	}
	if !*authed {
		return "(error) NOAUTH Authentication required."
	}
	wrong := fmt.Sprintf("(error) ERR wrong number of arguments for '%s' command", strings.ToLower(name))
	switch name {
	case "PING":
		if len(args) > 1 {
			return fmt.Sprintf("%q", args[1])
		}
		return "PONG"
	case "SET":
		if len(args) < 3 {
			return wrong
		}
		if len(args) > 3 && strings.EqualFold(args[3], "NX") {
			if _, ok := r.Data[args[1]]; ok {
				return "(nil)"
			}
		}
		r.Data[args[1]] = args[2]
		return "OK"
	case "GET":
		if len(args) != 2 {
			return wrong
		}
		v, ok := r.Data[args[1]]
		if !ok {
			return "(nil)"
		}
		return fmt.Sprintf("%q", v)
	case "DEL", "UNLINK":
		n := 0
		for _, k := range args[1:] {
			if _, ok := r.Data[k]; ok {
				delete(r.Data, k)
				n++
			}
		}
		return fmt.Sprintf("(integer) %d", n)
	case "EXISTS":
		n := 0
		for _, k := range args[1:] {
			if _, ok := r.Data[k]; ok {
				n++
			}
		}
		return fmt.Sprintf("(integer) %d", n)
	case "INCR", "DECR", "INCRBY", "DECRBY":
		if len(args) < 2 {
			return wrong
		}
		cur, err := strconv.Atoi(r.Data[args[1]])
		if err != nil && r.Data[args[1]] != "" {
			return "(error) ERR value is not an integer or out of range"
		}
		d := 1
		if len(args) > 2 {
			d, _ = strconv.Atoi(args[2])
		}
		if strings.HasPrefix(name, "DECR") {
			d = -d
		}
		cur += d
		r.Data[args[1]] = strconv.Itoa(cur)
		return fmt.Sprintf("(integer) %d", cur)
	case "KEYS":
		pat := arg(1)
		var keys []string
		for k := range r.Data {
			if ok, _ := path.Match(pat, k); ok {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			return "(empty array)"
		}
		var b []string
		for i, k := range keys {
			b = append(b, fmt.Sprintf("%d) %q", i+1, k))
		}
		return strings.Join(b, "\n")
	case "DBSIZE":
		return fmt.Sprintf("(integer) %d", len(r.Data))
	case "FLUSHALL", "FLUSHDB":
		r.Data = map[string]string{}
		return "OK"
	case "EXPIRE":
		if _, ok := r.Data[arg(1)]; ok {
			return "(integer) 1"
		}
		return "(integer) 0"
	case "TTL":
		if _, ok := r.Data[arg(1)]; ok {
			return "(integer) -1"
		}
		return "(integer) -2"
	case "INFO":
		ver := strings.ReplaceAll(strings.TrimPrefix(strings.ToLower(r.Version), "redis_"), "_", ".") + ".0"
		return fmt.Sprintf("# Server\nredis_version:%s\nredis_mode:standalone\ntcp_port:6379\n# Memory\nused_memory_human:%.2fM\nmaxmemory_human:%dG\n# Keyspace\ndb0:keys=%d,expires=0", ver, 1.1+float64(len(r.Data))/100, r.SizeGB, len(r.Data))
	case "CONFIG":
		return "(error) ERR unknown command 'CONFIG' — Memorystore blocks CONFIG; use gcloud redis instances update --update-redis-config"
	}
	return fmt.Sprintf("(error) ERR unknown command '%s', with args beginning with: %s", args[0], strings.Join(args[1:], " "))
}
