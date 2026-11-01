"use client";

// SaveStatus shows what happens to the learner's progress: saving, saved,
// offline (kept on this device and sent when the connection returns) or an
// error with a retry button. It is announced to screen readers (polite).

import { useEffect, useState } from "react";
import { useI18n } from "@/lib/i18n";
import { flush, onSaveStatus, type SaveStatus as S } from "@/lib/sync";

export default function SaveStatus() {
  const { t } = useI18n();
  const [s, setS] = useState<S>({ state: "idle", pending: 0 });
  const [showSaved, setShowSaved] = useState(false);
  useEffect(() => onSaveStatus(setS), []);
  useEffect(() => {
    if (s.state !== "saved") return;
    setShowSaved(true);
    const id = setTimeout(() => setShowSaved(false), 2500);
    return () => clearTimeout(id);
  }, [s.state, s.at]);
  const visible = s.state === "saving" || s.state === "offline" || s.state === "error" || (s.state === "saved" && showSaved);
  return (
    <div className={`save-status ${s.state}${visible ? " show" : ""}`} role="status" aria-live="polite">
      {visible && (
        <>
          <span className="save-dot" aria-hidden="true" />
          <span>
            {s.state === "saving" && t("Guardando tu progreso…")}
            {s.state === "saved" && t("Progreso guardado")}
            {s.state === "offline" && t("Sin conexión: tu progreso queda en este dispositivo y se enviará al volver ({n} pendientes)", { n: s.pending })}
            {s.state === "error" && (s.error ? t(s.error, { msg: s.detail ?? "" }) : t("No se ha podido guardar"))}
          </span>
          {(s.state === "error" || s.state === "offline") && s.pending > 0 && (
            <button type="button" className="save-retry" onClick={() => void flush()}>
              {t("Reintentar ahora")}
            </button>
          )}
        </>
      )}
    </div>
  );
}
