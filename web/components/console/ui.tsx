"use client";

// Building blocks of the graphical console, modelled on the Google Cloud
// console: list pages with a toolbar and a filter bar, full-page creation
// forms with an "Equivalent code" panel, side panels and confirmation
// dialogs. Every change runs the equivalent command in Cloud Shell.

import { useEffect, useRef, useState, type ReactNode } from "react";
import { useI18n } from "@/lib/i18n";
import { Icon } from "./icons";

/** q quotes a shell argument when it needs it. */
export function q(v: string): string {
  if (v === "") return "''";
  return /^[A-Za-z0-9_@%+=:,./-]+$/.test(v) ? v : `'${v.replace(/'/g, `'\\''`)}'`;
}

/** locFlag returns the gcloud location flag for a zone or a region. */
export const locFlag = (loc: string) => (/-[a-z]$/.test(loc) ? `--zone=${loc}` : `--region=${loc}`);

export const REGIONS = ["europe-west1", "europe-southwest1", "europe-west4", "us-central1", "us-east1"];
export const ZONES = REGIONS.flatMap((r) => ["b", "c", "d"].map((z) => `${r}-${z}`));

export function Status({ state }: { state?: string }) {
  const { t } = useI18n();
  const s = (state ?? "").toUpperCase();
  const ok = !s || ["RUNNING", "RUNNABLE", "READY", "ACTIVE", "ENABLED", "SERVING", "TRUE"].includes(s);
  const off = ["TERMINATED", "STOPPED", "SUSPENDED", "DISABLED", "FALSE"].includes(s);
  const bad = ["FAILED", "ERROR", "CRASHLOOPBACKOFF", "DEGRADED"].includes(s);
  const text = ok ? t("En ejecución") : off ? t("Detenido") : s;
  return (
    <span className={`cc-status ${ok ? "ok" : off ? "off" : bad ? "bad" : "busy"}`} title={text} role="img" aria-label={text}>
      {ok ? (
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 16.2 4.8 12l-1.4 1.4L9 19 21 7l-1.4-1.4z" fill="currentColor" /></svg>
      ) : off ? (
        <svg viewBox="0 0 24 24" aria-hidden="true"><rect x="7" y="7" width="10" height="10" fill="currentColor" /></svg>
      ) : bad ? (
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M11 7h2v7h-2zm0 9h2v2h-2z" fill="currentColor" /></svg>
      ) : (
        <svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="3" fill="currentColor" /></svg>
      )}
    </span>
  );
}

export function Pill({ children, tone }: { children: ReactNode; tone?: "ok" | "warn" | "bad" }) {
  return <span className={`cc-pill ${tone ?? ""}`}>{children}</span>;
}

export function Page({ title, intro, actions, children }: { title: string; intro?: ReactNode; actions?: ReactNode; children: ReactNode }) {
  return (
    <section className="cc-page" aria-label={title}>
      <div className="cc-page-head">
        <h1>{title}</h1>
        {actions && <div className="cc-toolbar">{actions}</div>}
      </div>
      {intro && <div className="cc-intro">{intro}</div>}
      {children}
    </section>
  );
}

export function Tool({ icon, label, onClick, disabled, danger }: { icon: string; label: string; onClick: () => void; disabled?: boolean; danger?: boolean }) {
  return (
    <button type="button" className={`cc-tool ${danger ? "danger" : ""}`} onClick={onClick} disabled={disabled}>
      <Icon name={icon} size={18} />
      {label}
    </button>
  );
}

export type Col<T> = { key: string; label: string; render: (row: T) => ReactNode; text?: (row: T) => string };

export function Table<T>({
  caption,
  cols,
  rows,
  rowKey,
  selected,
  onSelect,
  empty,
  filter = true,
}: {
  caption: string;
  cols: Col<T>[];
  rows: T[];
  rowKey: (r: T) => string;
  selected?: string[];
  onSelect?: (keys: string[]) => void;
  empty: string;
  filter?: boolean;
}) {
  const { t } = useI18n();
  const [f, setF] = useState("");
  const needle = f.trim().toLowerCase();
  const shown = needle ? rows.filter((r) => (rowKey(r) + " " + cols.map((c) => c.text?.(r) ?? "").join(" ")).toLowerCase().includes(needle)) : rows;
  const sel = new Set(selected ?? []);
  const all = shown.length > 0 && shown.every((r) => sel.has(rowKey(r)));
  return (
    <div className="cc-table-wrap">
      {filter && (
        <div className="cc-filter">
          <span aria-hidden="true" className="cc-filter-icon">
            <svg viewBox="0 0 24 24"><path d="M10 18h4v-2h-4v2zM3 6v2h18V6H3zm3 7h12v-2H6v2z" fill="currentColor" /></svg>
          </span>
          <span className="cc-filter-label">{t("Filtrar")}</span>
          <input value={f} onChange={(e) => setF(e.target.value)} placeholder={t("Introduce el nombre o el valor de una propiedad")} aria-label={t("Filtrar {what}", { what: caption })} />
        </div>
      )}
      <div className="cc-table-scroll">
        <table className="cc-table">
          <caption className="sr-only">{caption}</caption>
          <thead>
            <tr>
              {onSelect && (
                <th className="cc-sel">
                  <input type="checkbox" aria-label={t("Seleccionar todo")} checked={all} onChange={() => onSelect(all ? [] : shown.map(rowKey))} />
                </th>
              )}
              {cols.map((c) => <th key={c.key} scope="col">{c.label}</th>)}
            </tr>
          </thead>
          <tbody>
            {shown.map((r) => {
              const k = rowKey(r);
              return (
                <tr key={k} className={sel.has(k) ? "selected" : ""}>
                  {onSelect && (
                    <td className="cc-sel">
                      <input
                        type="checkbox"
                        aria-label={t("Seleccionar {name}", { name: k })}
                        checked={sel.has(k)}
                        onChange={() => onSelect(sel.has(k) ? [...sel].filter((x) => x !== k) : [...sel, k])}
                      />
                    </td>
                  )}
                  {cols.map((c) => <td key={c.key}>{c.render(r)}</td>)}
                </tr>
              );
            })}
          </tbody>
        </table>
        {!shown.length && <p className="cc-empty">{rows.length ? t("Ninguna fila coincide con el filtro.") : empty}</p>}
      </div>
    </div>
  );
}

/** CommandLine shows the command a form or a dialog is about to run. */
export function CommandLine({ cmd }: { cmd: string }) {
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);
  return (
    <div className="cc-cmd">
      <pre>{cmd}</pre>
      <button
        type="button"
        className="cc-text-btn"
        onClick={() => {
          navigator.clipboard?.writeText(cmd).then(() => {
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
          }, () => {});
        }}
      >
        {copied ? t("Copiado") : t("Copiar")}
      </button>
    </div>
  );
}

function useDialog(onClose: () => void) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const prev = document.activeElement as HTMLElement | null;
    ref.current?.querySelector<HTMLElement>("input, select, textarea, button.cc-btn")?.focus();
    const key = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", key);
    return () => {
      window.removeEventListener("keydown", key);
      prev?.focus?.();
    };
  }, [onClose]);
  return ref;
}

export type Field = {
  id: string;
  label: string;
  type?: "text" | "number" | "select" | "check" | "password" | "textarea";
  options?: (string | [string, string])[];
  help?: string;
  placeholder?: string;
  required?: boolean;
  /** section heading shown before the field */
  section?: string;
  /** show only when the predicate holds for the current values */
  when?: (v: Record<string, any>) => boolean;
};

function Fields({ fields, v, set }: { fields: Field[]; v: Record<string, any>; set: (id: string, val: any) => void }) {
  return (
    <>
      {fields.map((f) => {
        const id = `cc-f-${f.id}`;
        const head = f.section ? <h2 className="cc-form-section">{f.section}</h2> : null;
        if (f.type === "check") {
          return (
            <div key={f.id}>
              {head}
              <label className="cc-check" htmlFor={id}>
                <input id={id} type="checkbox" checked={!!v[f.id]} onChange={(e) => set(f.id, e.target.checked)} />
                <span>
                  {f.label}
                  {f.help && <span className="cc-help">{f.help}</span>}
                </span>
              </label>
            </div>
          );
        }
        return (
          <div key={f.id}>
            {head}
            <div className="cc-field">
              <label htmlFor={id}>
                {f.label}
                {f.required && <span aria-hidden="true"> *</span>}
              </label>
              {f.type === "select" ? (
                <select id={id} value={v[f.id] ?? ""} onChange={(e) => set(f.id, e.target.value)}>
                  {(f.options ?? []).map((o) => {
                    const [val, lab] = Array.isArray(o) ? o : [o, o];
                    return <option key={val} value={val}>{lab}</option>;
                  })}
                </select>
              ) : f.type === "textarea" ? (
                <textarea id={id} value={v[f.id] ?? ""} placeholder={f.placeholder} onChange={(e) => set(f.id, e.target.value)} />
              ) : (
                <input id={id} type={f.type ?? "text"} value={v[f.id] ?? ""} placeholder={f.placeholder} required={f.required} autoComplete="off" onChange={(e) => set(f.id, e.target.value)} />
              )}
            </div>
            {f.help && <div className="cc-help">{f.help}</div>}
          </div>
        );
      })}
    </>
  );
}

export type FormProps = {
  title: string;
  fields: Field[];
  initial: Record<string, any>;
  build: (v: Record<string, any>) => string;
  submit: string;
  onClose: () => void;
  onRun: (cmd: string) => Promise<boolean>;
  onPaste: (cmd: string) => void;
};

function useForm({ fields, initial, build, onRun, onClose }: FormProps) {
  const [v, setV] = useState<Record<string, any>>(initial);
  const [busy, setBusy] = useState(false);
  const shown = fields.filter((f) => !f.when || f.when(v));
  const missing = shown.some((f) => f.required && !String(v[f.id] ?? "").trim());
  const cmd = build(v);
  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (missing || busy) return;
    setBusy(true);
    const ok = await onRun(cmd);
    setBusy(false);
    if (ok) onClose();
  };
  return { v, set: (id: string, val: any) => setV((x) => ({ ...x, [id]: val })), shown, missing, cmd, busy, submit };
}

/** CreatePage is the full-page creation form with the equivalent code on the right. */
export function CreatePage(props: FormProps) {
  const { t } = useI18n();
  const f = useForm(props);
  const [code, setCode] = useState(true);
  const ref = useRef<HTMLFormElement>(null);
  useEffect(() => {
    ref.current?.querySelector<HTMLElement>("input, select")?.focus();
  }, []);
  return (
    <div className={`cc-create ${code ? "" : "no-code"}`}>
      <form className="cc-create-form" onSubmit={f.submit} ref={ref}>
        <div className="cc-create-head">
          <button type="button" className="cc-icon-btn" onClick={props.onClose} aria-label={t("Volver")}><Icon name="back" /></button>
          <h1>{props.title}</h1>
          <button type="button" className="cc-text-btn" onClick={() => setCode(!code)} aria-expanded={code}>
            {code ? t("Ocultar el código equivalente") : t("Código equivalente")}
          </button>
        </div>
        <div className="cc-create-fields">
          <Fields fields={f.shown} v={f.v} set={f.set} />
        </div>
        <div className="cc-create-actions">
          <button className="cc-btn primary" disabled={f.missing || f.busy}>{f.busy ? t("Ejecutando…") : props.submit}</button>
          <button type="button" className="cc-btn" onClick={props.onClose}>{t("Cancelar")}</button>
        </div>
      </form>
      {code && (
        <aside className="cc-code" aria-label={t("Código equivalente")}>
          <h2>{t("Código equivalente")}</h2>
          <div className="cc-code-tabs" role="presentation"><span className="active">{t("Línea de comandos")}</span></div>
          <p className="cc-help">{t("Esto es lo que se ejecutará en Cloud Shell al pulsar «{b}».", { b: props.submit })}</p>
          <CommandLine cmd={f.cmd} />
          <button type="button" className="cc-btn" disabled={f.missing} onClick={() => { props.onPaste(f.cmd); props.onClose(); }}>
            {t("Pegar en la terminal")}
          </button>
        </aside>
      )}
    </div>
  );
}

/** Drawer is the right-hand side panel (e.g. IAM «Conceder acceso»). */
export function Drawer(props: FormProps) {
  const { t } = useI18n();
  const f = useForm(props);
  const ref = useDialog(props.onClose);
  return (
    <div className="cc-overlay" onMouseDown={(e) => e.target === e.currentTarget && props.onClose()}>
      <div className="cc-drawer" role="dialog" aria-modal="true" aria-labelledby="cc-drawer-title" ref={ref}>
        <div className="cc-drawer-head">
          <h2 id="cc-drawer-title">{props.title}</h2>
          <button type="button" className="cc-icon-btn" onClick={props.onClose} aria-label={t("Cerrar")}><Icon name="close" /></button>
        </div>
        <form className="cc-drawer-body" onSubmit={f.submit}>
          <Fields fields={f.shown} v={f.v} set={f.set} />
          <div>
            <div className="cc-field-title">{t("Código equivalente")}</div>
            <CommandLine cmd={f.cmd} />
          </div>
          <div className="cc-create-actions">
            <button className="cc-btn primary" disabled={f.missing || f.busy}>{f.busy ? t("Ejecutando…") : props.submit}</button>
            <button type="button" className="cc-btn" onClick={props.onClose}>{t("Cancelar")}</button>
            <button type="button" className="cc-btn" disabled={f.missing} onClick={() => { props.onPaste(f.cmd); props.onClose(); }}>{t("Pegar en la terminal")}</button>
          </div>
        </form>
      </div>
    </div>
  );
}

/** Confirm asks before a destructive action and shows its commands. */
export function Confirm({ title, text, cmds, confirm, onClose, onRun }: { title: string; text: string; cmds: string[]; confirm: string; onClose: () => void; onRun: () => Promise<void> }) {
  const { t } = useI18n();
  const [busy, setBusy] = useState(false);
  const ref = useDialog(onClose);
  return (
    <div className="cc-overlay center" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="cc-dialog" role="alertdialog" aria-modal="true" aria-labelledby="cc-confirm-title" ref={ref}>
        <h2 id="cc-confirm-title">{title}</h2>
        <p>{text}</p>
        <div className="cc-field-title">{t("Código equivalente")}</div>
        <CommandLine cmd={cmds.join("\n")} />
        <div className="cc-dialog-actions">
          <button type="button" className="cc-btn text" onClick={onClose}>{t("Cancelar")}</button>
          <button
            type="button"
            className="cc-btn text danger"
            disabled={busy}
            onClick={async () => {
              setBusy(true);
              await onRun();
              setBusy(false);
              onClose();
            }}
          >
            {busy ? t("Ejecutando…") : confirm}
          </button>
        </div>
      </div>
    </div>
  );
}
