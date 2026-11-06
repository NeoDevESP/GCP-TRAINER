package main

// MCP server (Model Context Protocol, JSON-RPC 2.0 over stdio, one message
// per line). Editors such as VS Code and Antigravity start "cm mcp" and their
// agents get tools to work in the simulated Google Cloud: open a free console,
// run commands, read and write Cloud Shell files, save the world. It uses the
// same sign-in and the same session as the cm command, so you and the agent
// work on the same project.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

type rpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func obj(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

var mcpTools = []mcpTool{
	{"run_command", "Run one command in Cloud Mastery's simulated Google Cloud (not real Google Cloud): gcloud, kubectl, gsutil, bq, gam, terraform, curl, ssh into simulated VMs and the platform helpers (why <resource>, whatif <command>, help). Returns the output and exit code. Opens a free console if none is open.",
		obj(map[string]any{"command": map[string]any{"type": "string", "description": "The command line, e.g. gcloud compute instances list"}}, "command")},
	{"start_free_console", "Open a new free console (no mission, no score) from a starting world, or resume the saved one. Replaces the free console that is open.",
		obj(map[string]any{
			"world":  map[string]any{"type": "string", "description": "empty, default-network, zentra, bank-atlas, three-tier or shop (see list_worlds)"},
			"resume": map[string]any{"type": "boolean", "description": "Continue the saved world instead"},
		})},
	{"list_worlds", "List the starting worlds of the free console and whether a saved world exists.", obj(map[string]any{})},
	{"session_info", "Show the session in use: title, project, status and the address to open it in the browser.", obj(map[string]any{})},
	{"save_world", "Save the free console world so it can be resumed later.", obj(map[string]any{})},
	{"write_file", "Write a file in the Cloud Shell home of the session (e.g. main.tf, app.yaml).",
		obj(map[string]any{"path": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}}, "path", "content")},
	{"list_files", "List the files in the Cloud Shell home of the session, with their content.", obj(map[string]any{})},
}

func serveMCP(r io.Reader, w io.Writer, c *Config) error {
	in := bufio.NewScanner(r)
	in.Buffer(make([]byte, 4<<20), 4<<20)
	enc := json.NewEncoder(w)
	reply := func(id json.RawMessage, result any) {
		_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	}
	fail := func(id json.RawMessage, code int, m string) {
		_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": m}})
	}
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var m rpcMsg
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			fail(json.RawMessage("null"), -32700, "parse error")
			continue
		}
		if len(m.ID) == 0 || string(m.ID) == "null" {
			continue // a notification (initialized, cancelled…): no answer
		}
		switch m.Method {
		case "initialize":
			var p struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			_ = json.Unmarshal(m.Params, &p)
			if p.ProtocolVersion == "" {
				p.ProtocolVersion = "2025-06-18"
			}
			reply(m.ID, map[string]any{
				"protocolVersion": p.ProtocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "cloudmastery", "version": "2.1"},
				"instructions":    "Cloud Mastery simulates Google Cloud for training. Commands run in a simulated project, never in real Google Cloud. Use run_command for gcloud/kubectl/etc.; the web console shows the same state.",
			})
		case "ping":
			reply(m.ID, map[string]any{})
		case "tools/list":
			reply(m.ID, map[string]any{"tools": mcpTools})
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			if err := json.Unmarshal(m.Params, &p); err != nil {
				fail(m.ID, -32602, "invalid params")
				continue
			}
			text, isErr := callTool(c, p.Name, p.Arguments)
			reply(m.ID, map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": isErr})
		default:
			fail(m.ID, -32601, "method not found: "+m.Method)
		}
	}
	return in.Err()
}

func str(a map[string]any, k string) string {
	s, _ := a[k].(string)
	return s
}

// callTool runs one tool; errors become tool results the agent can read.
func callTool(c *Config, name string, a map[string]any) (string, bool) {
	*c = reloadSession(*c) // the cm command may have changed the session meanwhile
	errText := func(err error) (string, bool) { return "Error: " + err.Error(), true }
	switch name {
	case "run_command":
		cmd := strings.TrimSpace(str(a, "command"))
		if cmd == "" {
			return "Error: command is required", true
		}
		sid, err := session(c, true)
		if err != nil {
			return errText(err)
		}
		r, err := execLine(*c, sid, cmd)
		if err != nil {
			return errText(err)
		}
		return fmt.Sprintf("%s\n[exit code %d]", strings.TrimRight(r.Output, "\n"), r.Exit), r.Exit != 0
	case "start_free_console":
		world := str(a, "world")
		if world == "" {
			world = "default-network"
		}
		resume, _ := a["resume"].(bool)
		s, err := startSandbox(c, world, resume)
		if err != nil {
			return errText(err)
		}
		return fmt.Sprintf("Free console open: %s (project %s, session %s).\nBrowser: %s/lab?session=%s&id=%s", s.Title, s.Project, s.ID, c.Server, s.ID, s.LabID), false
	case "list_worlds":
		var r struct {
			Worlds []map[string]string `json:"worlds"`
			Saved  *struct {
				World   string    `json:"world"`
				SavedAt time.Time `json:"savedAt"`
			} `json:"saved"`
		}
		if err := c.call("GET", "/api/sandbox", nil, &r); err != nil {
			return errText(err)
		}
		var b strings.Builder
		for _, w := range r.Worlds {
			fmt.Fprintf(&b, "- %s: %s — %s\n", w["id"], w["name"], w["description"])
		}
		if r.Saved != nil {
			fmt.Fprintf(&b, "Saved world: %s (%s). Resume with start_free_console {resume: true}.\n", r.Saved.World, r.Saved.SavedAt.Format(time.RFC3339))
		}
		return b.String(), false
	case "session_info":
		sid, err := session(c, false)
		if err != nil {
			return errText(err)
		}
		var s sessionInfo
		if err := c.call("GET", "/api/sessions/"+sid, nil, &s); err != nil {
			return errText(err)
		}
		return fmt.Sprintf("%s · project %s · %s\nBrowser: %s/lab?session=%s&id=%s", s.Title, s.Project, s.Status, c.Server, s.ID, s.LabID), false
	case "save_world":
		var r struct {
			SavedAt time.Time `json:"savedAt"`
		}
		if err := c.call("POST", "/api/sandbox/save", map[string]any{}, &r); err != nil {
			return errText(err)
		}
		return "Saved at " + r.SavedAt.Format(time.RFC3339), false
	case "write_file":
		sid, err := session(c, true)
		if err != nil {
			return errText(err)
		}
		if err := c.call("PUT", "/api/sessions/"+sid+"/files", map[string]string{"Path": str(a, "path"), "Content": str(a, "content")}, nil); err != nil {
			return errText(err)
		}
		return "Written ~/" + strings.TrimPrefix(str(a, "path"), "~/"), false
	case "list_files":
		sid, err := session(c, false)
		if err != nil {
			return errText(err)
		}
		files := map[string]string{}
		if err := c.call("GET", "/api/sessions/"+sid+"/views/files", nil, &files); err != nil {
			return errText(err)
		}
		names := make([]string, 0, len(files))
		for k := range files {
			names = append(names, k)
		}
		sort.Strings(names)
		var b strings.Builder
		for _, k := range names {
			fmt.Fprintf(&b, "=== %s\n%s\n", k, files[k])
		}
		if b.Len() == 0 {
			return "No files.", false
		}
		return b.String(), false
	}
	return "Error: unknown tool " + name, true
}

// reloadSession picks up a sign-in or session changed by the cm command.
func reloadSession(c Config) Config {
	n := loadConfig()
	if n.Token != "" {
		c.Token, c.Server = n.Token, n.Server
	}
	if n.Session != "" {
		c.Session = n.Session
	}
	return c
}
