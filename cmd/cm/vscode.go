package main

// cm vscode writes the editor configuration for a folder:
//
//   - .vscode/settings.json: two terminal profiles, «Cloud Mastery» (the
//     interactive Cloud Shell) and «Cloud Mastery (gcloud)» (your usual shell
//     with gcloud, kubectl… going to the simulator);
//   - .vscode/mcp.json: the MCP server for VS Code's agent.
//
// Existing settings are kept: the entries are merged. A settings file with
// comments is not rewritten; the snippet to paste is printed instead.
// Antigravity is built on VS Code and reads the same terminal profiles; its
// agent's MCP servers live in its own config, whose snippet is printed.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func osKey() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "osx"
	}
	return "linux"
}

func cmdVSCode(args []string) error {
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	shims := shimDir()
	if err := writeShims(shims, exe); err != nil {
		return err
	}
	shell, sep := "bash", ":"
	switch runtime.GOOS {
	case "windows":
		shell, sep = "powershell.exe", ";"
	case "darwin":
		shell = "zsh"
	}
	profiles := map[string]any{
		"Cloud Mastery":          map[string]any{"path": exe, "args": []string{"shell"}, "icon": "cloud"},
		"Cloud Mastery (gcloud)": map[string]any{"path": shell, "env": map[string]string{"PATH": shims + sep + "${env:PATH}"}, "icon": "cloud", "overrideName": true},
	}
	vs := filepath.Join(dir, ".vscode")
	if err := os.MkdirAll(vs, 0o755); err != nil {
		return err
	}
	key := "terminal.integrated.profiles." + osKey()
	okSettings := merge(filepath.Join(vs, "settings.json"), func(m map[string]any) {
		cur, _ := m[key].(map[string]any)
		if cur == nil {
			cur = map[string]any{}
		}
		for k, v := range profiles {
			cur[k] = v
		}
		m[key] = cur
	})
	mcpServer := map[string]any{"type": "stdio", "command": exe, "args": []string{"mcp"}}
	okMCP := merge(filepath.Join(vs, "mcp.json"), func(m map[string]any) {
		cur, _ := m["servers"].(map[string]any)
		if cur == nil {
			cur = map[string]any{}
		}
		cur["cloudmastery"] = mcpServer
		m["servers"] = cur
	})
	show := func(v any) string { b, _ := json.MarshalIndent(v, "", "  "); return string(b) }
	if okSettings == nil {
		fmt.Printf(msg("✓ %s: perfiles de terminal «Cloud Mastery» y «Cloud Mastery (gcloud)»\n", "✓ %s: terminal profiles \"Cloud Mastery\" and \"Cloud Mastery (gcloud)\"\n"), filepath.Join(vs, "settings.json"))
	} else {
		fmt.Printf(msg("! No toco %s (%v). Añade esto a tu configuración:\n%s\n", "! Leaving %s untouched (%v). Add this to your settings:\n%s\n"), filepath.Join(vs, "settings.json"), okSettings, show(map[string]any{key: profiles}))
	}
	if okMCP == nil {
		fmt.Printf(msg("✓ %s: servidor MCP «cloudmastery» para el agente de VS Code\n", "✓ %s: MCP server \"cloudmastery\" for VS Code's agent\n"), filepath.Join(vs, "mcp.json"))
	} else {
		fmt.Printf(msg("! No toco %s (%v). Añade esto:\n%s\n", "! Leaving %s untouched (%v). Add this:\n%s\n"), filepath.Join(vs, "mcp.json"), okMCP, show(map[string]any{"servers": map[string]any{"cloudmastery": mcpServer}}))
	}
	fmt.Println()
	fmt.Println(msg("Antigravity: abre el panel del agente › … › MCP Servers › Manage MCP Servers › View raw config y añade:",
		"Antigravity: open the agent panel › … › MCP Servers › Manage MCP Servers › View raw config and add:"))
	fmt.Println(show(map[string]any{"mcpServers": map[string]any{"cloudmastery": map[string]any{"command": exe, "args": []string{"mcp"}}}}))
	fmt.Println(msg("\nLuego: «cm login» (una vez) y abre una terminal «Cloud Mastery».", "\nThen: \"cm login\" (once) and open a \"Cloud Mastery\" terminal."))
	return nil
}

// merge edits a JSON file in place (creating it if needed). It refuses to
// rewrite a file it cannot parse as plain JSON (e.g. with comments).
func merge(path string, edit func(map[string]any)) error {
	m := map[string]any{}
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		if err := json.Unmarshal(b, &m); err != nil {
			return errors.New(msg("tiene comentarios o no es JSON simple", "it has comments or is not plain JSON"))
		}
	}
	edit(m)
	b, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
