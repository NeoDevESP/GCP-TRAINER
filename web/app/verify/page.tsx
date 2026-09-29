"use client";

import { useState } from "react";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { dimension, label } from "@/lib/labels";
import { LangSwitch } from "@/components/Nav";

export default function Verify() {
  const { t: tr, lang } = useI18n();
  const [text, setText] = useState("");
  const [res, setRes] = useState<any>(null);
  const [err, setErr] = useState("");
  const [t, setT] = useState<any>(null);
  const check = async () => {
    setErr("");
    setRes(null);
    try {
      const parsed = JSON.parse(text);
      setT(parsed);
      setRes(await api("/api/auth/verify-transcript", { body: parsed }));
    } catch (e: any) {
      setErr(e.message);
    }
  };
  return (
    <main id="main" className="page" style={{ maxWidth: 820 }}>
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <LangSwitch />
      </div>
      <h1>{tr("Verificar un expediente")}</h1>
      <p className="muted">
        {tr(
          "El alumnado puede descargar un expediente firmado con sus competencias, su etapa profesional y los proyectos finales completados. Pégalo aquí para comprobar que lo emitió esta plataforma y que no se ha modificado.",
        )}
      </p>
      <textarea style={{ minHeight: 220, fontFamily: "var(--mono)" }} value={text} onChange={(e) => setText(e.target.value)} placeholder={tr("{ … JSON del expediente … }")} />
      <div className="row" style={{ marginTop: 8 }}>
        <button className="btn" onClick={check} disabled={!text.trim()}>{tr("Verificar")}</button>
        <a href="/">{tr("Volver")}</a>
      </div>
      {err && <p className="error">{err}</p>}
      {res && (
        <div className="card" style={{ marginTop: 16, borderColor: res.valid ? "var(--ok)" : "var(--bad)" }}>
          <h2>{res.valid ? tr("✓ Expediente válido") : tr("✗ Firma no válida")}</h2>
          {res.valid && (
            <>
              <p>
                <strong>{res.user}</strong> — {tr("etapa profesional")} <strong>{res.careerStage}</strong>,{" "}
                {tr("emitido el {date}", { date: new Date(res.issued).toLocaleString(lang === "en" ? "en-GB" : "es-ES") })}.
              </p>
              <p className="small muted">
                {tr("Autonomía: {a} · {n} laboratorios superados · especializaciones: {s}", {
                  a: t?.autonomy ?? "",
                  n: t?.labsPassed?.length ?? 0,
                  s: t?.specializations?.join(", ") || tr("ninguna"),
                })}
              </p>
              {t?.dimensions && (
                <table>
                  <tbody>
                    {Object.entries(t.dimensions).map(([k, v]: any) => (
                      <tr key={k}><td>{label(dimension, k, tr)}</td><td>{Math.round(v * 100)}%</td></tr>
                    ))}
                  </tbody>
                </table>
              )}
            </>
          )}
        </div>
      )}
    </main>
  );
}
