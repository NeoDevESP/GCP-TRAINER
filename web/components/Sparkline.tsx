import { getLang, translate } from "@/lib/i18n";

export default function Sparkline({ points, width = 220, height = 40 }: { points: { t: string; v: number }[]; width?: number; height?: number }) {
  if (!points?.length) return <span className="muted small">{translate(getLang(), "sin datos")}</span>;
  const vs = points.map((p) => p.v);
  const min = Math.min(...vs);
  const max = Math.max(...vs);
  const span = max - min || 1;
  const d = points
    .map((p, i) => {
      const x = points.length === 1 ? width / 2 : (i / (points.length - 1)) * (width - 4) + 2;
      const y = height - 2 - ((p.v - min) / span) * (height - 4);
      return `${i ? "L" : "M"}${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(" ");
  return (
    <svg width={width} height={height} role="img" aria-label={`min ${min} max ${max}`}>
      <path d={d} fill="none" stroke="var(--accent)" strokeWidth="1.5" />
    </svg>
  );
}
