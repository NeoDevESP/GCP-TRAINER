"use client";

// Global search (Ctrl+K, ⌘K or /): pages, labs and lessons in one list,
// opened with the keyboard. It is a modal dialog with a combobox: arrows move,
// Enter opens, Escape closes and focus returns where it was.

import { useEffect, useMemo, useRef, useState } from "react";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import type { LabSummary } from "@/lib/types";

export interface SearchItem {
  label: string;
  hint: string; // what kind of thing it is
  href: string;
}

const norm = (s: string) => s.normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase();

let cache: SearchItem[] | null = null;

async function loadItems(t: (s: string) => string): Promise<SearchItem[]> {
  if (cache) return cache;
  const [cat, learn] = await Promise.all([
    api<{ labs: LabSummary[] }>("/api/catalog").catch(() => ({ labs: [] as LabSummary[] })),
    api<{ topics: { id: string; name: string }[] }>("/api/learn").catch(() => ({ topics: [] })),
  ]);
  cache = [
    ...cat.labs.map((l) => ({ label: l.title, hint: t("Laboratorio"), href: `/lab?id=${encodeURIComponent(l.id)}` })),
    ...learn.topics.map((x) => ({ label: x.name, hint: t("Lección"), href: `/learn?topic=${x.id}` })),
  ];
  return cache;
}

export default function CommandSearch({ open, onClose, pages }: { open: boolean; onClose: () => void; pages: SearchItem[] }) {
  const { t } = useI18n();
  const [q, setQ] = useState("");
  const [items, setItems] = useState<SearchItem[]>([]);
  const [sel, setSel] = useState(0);
  const input = useRef<HTMLInputElement>(null);
  const back = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!open) return;
    back.current = document.activeElement as HTMLElement;
    setQ("");
    setSel(0);
    setTimeout(() => input.current?.focus(), 0);
    loadItems(t).then(setItems);
    return () => back.current?.focus?.();
  }, [open, t]);

  const results = useMemo(() => {
    const all = [...pages, ...items];
    const words = norm(q).split(/\s+/).filter(Boolean);
    if (!words.length) return pages.slice(0, 12);
    return all.filter((it) => words.every((w) => norm(it.label + " " + it.hint).includes(w))).slice(0, 30);
  }, [q, items, pages]);

  if (!open) return null;
  const go = (it?: SearchItem) => {
    if (!it) return;
    onClose();
    window.location.href = it.href;
  };
  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") {
      e.preventDefault();
      onClose();
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      setSel((s) => Math.min(results.length - 1, s + 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSel((s) => Math.max(0, s - 1));
    } else if (e.key === "Enter") {
      e.preventDefault();
      go(results[sel]);
    } else if (e.key === "Tab") {
      e.preventDefault(); // focus stays in the dialog
    }
  };
  return (
    <div className="cs-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="cs-dialog" role="dialog" aria-modal="true" aria-label={t("Buscar")} onKeyDown={onKey}>
        <input
          ref={input}
          className="cs-input"
          role="combobox"
          aria-expanded="true"
          aria-controls="cs-list"
          aria-activedescendant={results[sel] ? `cs-${sel}` : undefined}
          aria-autocomplete="list"
          placeholder={t("Busca una página, un laboratorio o una lección…")}
          value={q}
          onChange={(e) => {
            setQ(e.target.value);
            setSel(0);
          }}
        />
        <ul id="cs-list" role="listbox" className="cs-list" aria-label={t("Resultados")}>
          {results.map((it, i) => (
            <li
              key={it.href + i}
              id={`cs-${i}`}
              role="option"
              aria-selected={i === sel}
              className={i === sel ? "on" : ""}
              onMouseEnter={() => setSel(i)}
              onMouseDown={(e) => {
                e.preventDefault();
                go(it);
              }}
            >
              <span>{it.label}</span>
              <span className="cs-hint">{it.hint}</span>
            </li>
          ))}
          {!results.length && <li className="cs-empty" role="presentation">{t("Nada coincide con «{q}».", { q })}</li>}
        </ul>
        <p className="cs-foot" aria-hidden="true">↑↓ {t("moverse")} · Enter {t("abrir")} · Esc {t("cerrar")}</p>
      </div>
    </div>
  );
}
