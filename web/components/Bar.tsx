export default function Bar({ value, max = 100, label }: { value: number; max?: number; label?: string }) {
  const pct = Math.max(0, Math.min(100, (value / (max || 1)) * 100));
  const cls = pct >= 80 ? "ok" : pct >= 50 ? "" : pct >= 25 ? "warn" : "bad";
  return (
    <div title={label ?? `${Math.round(pct)}%`}>
      <div className={`bar ${cls}`}>
        <div style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}
