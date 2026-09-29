"use client";

import dynamic from "next/dynamic";
import { getLang, translate } from "@/lib/i18n";

const Monaco = dynamic(() => import("@monaco-editor/react"), { ssr: false, loading: () => <p className="muted">{translate(getLang(), "Cargando el editor…")}</p> });

function language(path: string): string {
  if (/\.(tf|hcl)$/.test(path)) return "hcl";
  if (/\.ya?ml$/.test(path)) return "yaml";
  if (/\.json$/.test(path)) return "json";
  if (/\.(sh|bash)$/.test(path)) return "shell";
  if (/Dockerfile$/.test(path)) return "dockerfile";
  if (/\.py$/.test(path)) return "python";
  if (/\.(js|mjs)$/.test(path)) return "javascript";
  if (/\.go$/.test(path)) return "go";
  if (/\.sql$/.test(path)) return "sql";
  if (/\.md$/.test(path)) return "markdown";
  return "plaintext";
}

export default function CodeEditor({ path, value, onChange }: { path: string; value: string; onChange: (v: string) => void }) {
  const dark = typeof window !== "undefined" && window.matchMedia?.("(prefers-color-scheme: dark)").matches;
  return (
    <Monaco
      height="100%"
      path={path}
      language={language(path)}
      value={value}
      theme={dark ? "vs-dark" : "light"}
      onChange={(v) => onChange(v ?? "")}
      options={{ minimap: { enabled: false }, fontSize: 13, scrollBeyondLastLine: false, automaticLayout: true, tabSize: 2 }}
    />
  );
}
