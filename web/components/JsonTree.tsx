// JsonTree renders nested JSON as collapsible sections; leaves are shown
// inline. Top-level keys with empty values are hidden to reduce noise.

function isEmpty(v: any): boolean {
  return v == null || (Array.isArray(v) && v.length === 0) || (typeof v === "object" && Object.keys(v).length === 0);
}

function Leaf({ v }: { v: any }) {
  if (v === null || v === undefined) return <span className="muted">null</span>;
  if (typeof v === "string") return <span style={{ color: "var(--ok)" }}>&quot;{v}&quot;</span>;
  return <span style={{ color: "var(--accent)" }}>{String(v)}</span>;
}

function Node({ k, v, depth }: { k: string; v: any; depth: number }) {
  if (v === null || typeof v !== "object") {
    return (
      <div className="small" style={{ fontFamily: "var(--mono)" }}>
        <span className="muted">{k}: </span>
        <Leaf v={v} />
      </div>
    );
  }
  const entries = Array.isArray(v) ? v.map((x, i) => [String(i), x] as const) : Object.entries(v);
  const label = Array.isArray(v) ? `[${v.length}]` : `{${entries.length}}`;
  return (
    <details open={depth < 1} style={{ marginLeft: depth ? 12 : 0 }}>
      <summary className="small" style={{ fontFamily: "var(--mono)", cursor: "pointer" }}>
        {k} <span className="muted">{label}</span>
      </summary>
      <div style={{ marginLeft: 12 }}>
        {entries.map(([ck, cv]) => (
          <Node key={ck} k={ck} v={cv} depth={depth + 1} />
        ))}
      </div>
    </details>
  );
}

export default function JsonTree({ data, hideEmpty = true }: { data: any; hideEmpty?: boolean }) {
  if (data == null || typeof data !== "object") return <Leaf v={data} />;
  const entries = Object.entries(data).filter(([, v]) => !hideEmpty || !isEmpty(v));
  if (!entries.length) return <p className="muted">Nothing here yet.</p>;
  return (
    <div>
      {entries.map(([k, v]) => (
        <Node key={k} k={k} v={v} depth={0} />
      ))}
    </div>
  );
}
