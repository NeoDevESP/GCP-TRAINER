"use client";

import { useEffect, useId, useRef, useState } from "react";

// Mermaid renders a diagram definition client-side. securityLevel "strict"
// keeps labels from injecting HTML.
export default function Mermaid({ chart }: { chart: string }) {
  const ref = useRef<HTMLDivElement>(null);
  const id = "m" + useId().replace(/[^a-zA-Z0-9]/g, "");
  const [err, setErr] = useState("");
  useEffect(() => {
    let cancelled = false;
    (async () => {
      if (!chart || !ref.current) return;
      const mermaid = (await import("mermaid")).default;
      const dark = typeof window !== "undefined" && window.matchMedia?.("(prefers-color-scheme: dark)").matches;
      mermaid.initialize({ startOnLoad: false, securityLevel: "strict", theme: dark ? "dark" : "default" });
      try {
        const { svg } = await mermaid.render(id, chart);
        if (!cancelled && ref.current) {
          ref.current.innerHTML = svg;
          setErr("");
        }
      } catch (e: any) {
        if (!cancelled) setErr(String(e?.message ?? e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [chart, id]);
  return (
    <div>
      <div ref={ref} className="mermaid" />
      {err && <pre className="error small">{err}</pre>}
    </div>
  );
}
