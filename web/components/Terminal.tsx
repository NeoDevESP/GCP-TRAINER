"use client";

import { useEffect, useRef } from "react";
import { api } from "@/lib/api";
import { getLang, translate } from "@/lib/i18n";

// Terminal is an xterm.js front-end with local line editing. Complete lines
// (including heredocs and for/if blocks) are sent to the session's exec API,
// which runs them in the same shell the grader observes.

function needsMore(buf: string): boolean {
  const lines = buf.split("\n");
  const last = lines[lines.length - 1];
  if (last.endsWith("\\")) return true;
  const hd = buf.match(/<<-?\s*['"]?(\w+)['"]?/);
  if (hd && !lines.slice(1).some((l) => l.trim() === hd[1])) return true;
  let depth = 0;
  for (const l of lines) {
    const t = l.trim();
    if (/^(for|while|until)\b/.test(t) || /;\s*do\s*$/.test(t)) depth += /^(for|while|until)\b/.test(t) ? 1 : 0;
    if (/^if\b/.test(t)) depth++;
    if (/^(done|fi)\b/.test(t) || /;\s*(done|fi)\s*$/.test(t)) depth--;
  }
  return depth > 0;
}

export default function Terminal({
  sessionId,
  prompt = "student@cloudshell:~$ ",
  onCommand,
  banner,
  screenReader = false,
}: {
  sessionId: string;
  prompt?: string;
  onCommand?: (line: string, exit: number) => void;
  banner?: string;
  screenReader?: boolean;
}) {
  const host = useRef<HTMLDivElement>(null);
  const onCmd = useRef(onCommand);
  onCmd.current = onCommand;

  useEffect(() => {
    if (!host.current || !sessionId) return;
    let disposed = false;
    let cleanup = () => {};
    (async () => {
      const { Terminal: XTerm } = await import("@xterm/xterm");
      const { FitAddon } = await import("@xterm/addon-fit");
      if (disposed || !host.current) return;
      const reduceMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
      const term = new XTerm({
        cursorBlink: !reduceMotion,
        screenReaderMode: screenReader,
        convertEol: true,
        fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
        fontSize: 13,
        theme: { background: "#0f1419", foreground: "#d7dde6" },
        scrollback: 5000,
      });
      const fit = new FitAddon();
      term.loadAddon(fit);
      term.open(host.current);
      fit.fit();
      const ro = new ResizeObserver(() => {
        try {
          fit.fit();
        } catch {
          /* hidden */
        }
      });
      ro.observe(host.current);

      const history: string[] = [];
      let hIdx = 0;
      let line = "";
      let cursor = 0;
      let pending = ""; // accumulated multi-line command
      let busy = false;

      const curPrompt = () => (pending ? "> " : prompt);
      const redraw = () => {
        term.write("\r\x1b[2K" + curPrompt() + line);
        const back = line.length - cursor;
        if (back > 0) term.write(`\x1b[${back}D`);
      };
      if (banner) term.writeln(banner);
      term.write(prompt);

      const run = async (cmd: string) => {
        busy = true;
        try {
          const res = await api<{ output: string; exit: number }>(`/api/sessions/${sessionId}/exec`, { body: { line: cmd } });
          if (res.output) {
            term.write(res.output.endsWith("\n") ? res.output : res.output + "\n");
          }
          onCmd.current?.(cmd, res.exit);
        } catch (e: any) {
          term.write(`\x1b[31m${e.message ?? e}\x1b[0m\n`);
        } finally {
          busy = false;
          term.write(prompt);
        }
      };

      const submit = () => {
        term.write("\r\n");
        const full = pending ? pending + "\n" + line : line;
        line = "";
        cursor = 0;
        if (needsMore(full)) {
          pending = full;
          term.write("> ");
          return;
        }
        pending = "";
        const cmd = full.trim();
        if (!cmd) {
          term.write(prompt);
          return;
        }
        if (cmd === "clear") {
          term.clear();
          term.write(prompt);
          return;
        }
        history.push(full);
        hIdx = history.length;
        run(full);
      };

      const insert = (s: string) => {
        line = line.slice(0, cursor) + s + line.slice(cursor);
        cursor += s.length;
        redraw();
      };

      const sub = term.onData((data) => {
        if (busy) return;
        // pasted text with newlines: feed line by line
        if (data.length > 1 && /[\r\n]/.test(data)) {
          const parts = data.replace(/\r\n?/g, "\n").split("\n");
          parts.forEach((p, i) => {
            insert(p);
            if (i < parts.length - 1) {
              const full = pending ? pending + "\n" + line : line;
              term.write("\r\n");
              line = "";
              cursor = 0;
              pending = full;
            }
          });
          if (!needsMore(pending) && pending && !line) {
            const cmd = pending;
            pending = "";
            history.push(cmd);
            hIdx = history.length;
            run(cmd);
          } else if (pending) {
            redraw();
          }
          return;
        }
        switch (data) {
          case "\r":
            submit();
            return;
          case "\x7f":
            if (cursor > 0) {
              line = line.slice(0, cursor - 1) + line.slice(cursor);
              cursor--;
              redraw();
            }
            return;
          case "\x03":
            term.write("^C\r\n");
            line = "";
            cursor = 0;
            pending = "";
            term.write(prompt);
            return;
          case "\x0c":
            term.clear();
            redraw();
            return;
          case "\x1b[A":
            if (hIdx > 0) {
              hIdx--;
              line = history[hIdx].split("\n")[0];
              cursor = line.length;
              redraw();
            }
            return;
          case "\x1b[B":
            if (hIdx < history.length - 1) {
              hIdx++;
              line = history[hIdx].split("\n")[0];
            } else {
              hIdx = history.length;
              line = "";
            }
            cursor = line.length;
            redraw();
            return;
          case "\x1b[D":
            if (cursor > 0) {
              cursor--;
              term.write(data);
            }
            return;
          case "\x1b[C":
            if (cursor < line.length) {
              cursor++;
              term.write(data);
            }
            return;
          case "\x01":
            cursor = 0;
            redraw();
            return;
          case "\x05":
            cursor = line.length;
            redraw();
            return;
        }
        if (data >= " " || data === "\t") insert(data === "\t" ? "  " : data);
      });
      term.focus();
      cleanup = () => {
        sub.dispose();
        ro.disconnect();
        term.dispose();
      };
    })();
    return () => {
      disposed = true;
      cleanup();
    };
  }, [sessionId, prompt, banner, screenReader]);

  return <div ref={host} role="application" aria-label={translate(getLang(), "Terminal de Cloud Shell. Escribe comandos y pulsa Intro.")} style={{ width: "100%", height: "100%" }} />;
}
