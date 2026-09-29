"use client";

import { useEffect, useState } from "react";
import Nav from "@/components/Nav";
import { api } from "@/lib/api";
import { useAuth } from "@/components/useAuth";
import { useI18n } from "@/lib/i18n";

export default function Leaderboard() {
  useAuth();
  const { t, lang } = useI18n();
  const [d, setD] = useState<any>(null);
  const [err, setErr] = useState("");
  const load = () => api("/api/leaderboard").then(setD).catch((e) => setErr(e.message));
  useEffect(() => {
    load();
  }, [lang]);
  const toggle = async () => {
    await api("/api/me/leaderboard", { body: { optIn: !d.optIn } });
    load();
  };
  return (
    <>
      <Nav active="/leaderboard" />
      <main id="main" className="page" style={{ maxWidth: 800 }}>
        <h1>{t("Liga semanal")}</h1>
        <p className="muted">
          {t("Las ligas son voluntarias y ordenan la XP de esta semana ponderada por dificultad: los laboratorios difíciles cuentan más que repetir los fáciles.")}
        </p>
        {err && <p className="error">{err}</p>}
        {d && (
          <div className="card">
            <div className="row" style={{ justifyContent: "space-between" }}>
              <h2 style={{ margin: 0 }}>{t("Liga {name}", { name: d.league })}</h2>
              <button className="btn secondary" onClick={toggle}>{d.optIn ? t("Salir de la clasificación") : t("Unirme a la clasificación")}</button>
            </div>
            <table style={{ marginTop: 10 }}>
              <thead><tr><th>#</th><th>{t("Nombre")}</th><th>{t("Nivel")}</th><th>{t("Puntos")}</th></tr></thead>
              <tbody>
                {(d.entries ?? []).map((e: any, i: number) => (
                  <tr key={e.userId}><td>{i + 1}</td><td>{e.name}</td><td>{e.level}</td><td>{Math.round(e.points)}</td></tr>
                ))}
              </tbody>
            </table>
            {!d.entries?.length && <p className="muted">{t("Nadie ha puntuado en esta liga esta semana.")}</p>}
          </div>
        )}
      </main>
    </>
  );
}
