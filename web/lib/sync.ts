// Save queue for the learner's progress. Every save goes through here:
//
// - it is written first to an outbox in localStorage, so a reload, a closed
//   tab or a network failure never loses it (pending saves are sent when the
//   app opens again or the connection comes back);
// - it is retried with growing waits (2 s, 4 s, 8 s… up to 1 min) on network
//   errors, timeouts and server errors; a save the server rejects for good
//   (4xx) is dropped and reported, never retried forever;
// - study runs carry the revision last seen (X-Run-Base): when another device
//   saved in between (409) both copies are merged (lib/model.ts mergeRun) and
//   saved again;
// - its state (saving, saved, offline, error) is shown by <SaveStatus/>.

import { api, ApiError } from "@/lib/api";
import { mergeRun } from "@/lib/model";
import { k } from "@/lib/i18n";

export type SaveState = "idle" | "saving" | "saved" | "offline" | "error";
export interface SaveStatus {
  state: SaveState;
  pending: number;
  /** Spanish key of the last error (translated by the indicator) */
  error?: string;
  /** server message that goes with the error */
  detail?: string;
  at?: number;
}

interface Op {
  id: string; // a newer save with the same id replaces an older pending one
  path: string;
  method: string;
  body?: unknown;
  run?: string; // run key: send X-Run-Base and merge on conflict
  tries: number;
  seq: number; // unique per enqueued save
}

const OUTBOX = "cm-outbox";
let status: SaveStatus = { state: "idle", pending: 0 };
const listeners = new Set<(s: SaveStatus) => void>();
let flushing = false;
let timer: ReturnType<typeof setTimeout> | undefined;
const revs: Record<string, number> = {};

function readBox(): Op[] {
  try {
    return JSON.parse(localStorage.getItem(OUTBOX) ?? "[]") as Op[];
  } catch {
    return [];
  }
}
function writeBox(ops: Op[]) {
  try {
    if (ops.length) localStorage.setItem(OUTBOX, JSON.stringify(ops));
    else localStorage.removeItem(OUTBOX);
  } catch {
    /* storage unavailable: the queue lives in memory only */
    memBox = ops;
  }
}
let memBox: Op[] | null = null;
const box = () => memBox ?? readBox();

function set(s: Partial<SaveStatus>) {
  status = { ...status, ...s, pending: box().length };
  for (const l of listeners) l(status);
}

export const saveStatus = () => status;
export function onSaveStatus(fn: (s: SaveStatus) => void) {
  listeners.add(fn);
  fn(status);
  return () => {
    listeners.delete(fn);
  };
}

/** setRunRevs records the revisions of the runs as loaded from the server. */
export function setRunRevs(r: Record<string, number> | undefined) {
  Object.assign(revs, r ?? {});
}

/** enqueue a save; it is sent as soon as possible and survives reloads. */
let seq = Date.now();
export function enqueue(op: Omit<Op, "tries" | "seq">) {
  const ops = box().filter((o) => o.id !== op.id);
  ops.push({ ...op, tries: 0, seq: ++seq });
  writeBox(ops);
  set({ state: "saving" });
  void flush();
}

const transient = (e: unknown) => !(e instanceof ApiError) || e.status >= 500 || e.status === 408 || e.status === 429;

/** flush sends the pending saves in order. */
export async function flush(): Promise<void> {
  if (flushing) return;
  if (typeof navigator !== "undefined" && navigator.onLine === false) {
    if (box().length) set({ state: "offline" });
    return;
  }
  flushing = true;
  clearTimeout(timer);
  try {
    for (;;) {
      const ops = box();
      if (!ops.length) {
        set({ state: status.state === "error" ? "error" : "saved", at: Date.now() });
        return;
      }
      const op = ops[0];
      set({ state: "saving" });
      try {
        const headers: Record<string, string> = {};
        if (op.run) headers["X-Run-Base"] = String(revs[op.run] ?? 0);
        const res = await api<{ rev?: number }>(op.path, { method: op.method, body: op.body, headers });
        if (op.run && typeof res?.rev === "number") revs[op.run] = res.rev;
        writeBox(box().filter((o) => o.seq !== op.seq));
        set({ state: "saving", error: undefined, detail: undefined });
      } catch (e) {
        if (e instanceof ApiError && e.status === 409 && op.run) {
          // another device saved this run: merge and save again on top of it
          revs[op.run] = Number(e.data?.rev ?? 0);
          const merged = mergeRun(e.data?.run, (op.body ?? {}) as Record<string, unknown>);
          writeBox(box().map((o) => (o.seq === op.seq ? { ...o, body: merged } : o)));
          try {
            localStorage.setItem(op.run, JSON.stringify(merged));
          } catch {
            /* cache only */
          }
          window.dispatchEvent(new CustomEvent("cm-run-merged", { detail: { key: op.run, run: merged } }));
          continue;
        }
        if (!transient(e)) {
          // the server refuses it for good: drop it and say so
          writeBox(box().filter((o) => o.seq !== op.seq));
          set({ state: "error", error: k("Un cambio no se pudo guardar: {msg}"), detail: (e as Error).message });
          continue;
        }
        const tries = op.tries + 1;
        writeBox(box().map((o) => (o.seq === op.seq ? { ...o, tries } : o)));
        const offline = typeof navigator !== "undefined" && navigator.onLine === false;
        set({ state: offline ? "offline" : "error", error: offline ? undefined : k("Sin conexión con el servidor; reintentando…") });
        timer = setTimeout(() => void flush(), Math.min(60000, 2000 * 2 ** Math.min(5, tries - 1)));
        return;
      }
    }
  } finally {
    flushing = false;
  }
}

/** hasPending: there are saves not confirmed by the server yet. */
export const hasPending = () => box().length > 0;

if (typeof window !== "undefined") {
  window.addEventListener("online", () => void flush());
  // send what was left from a previous visit
  setTimeout(() => {
    if (box().length) {
      set({ state: "saving" });
      void flush();
    }
  }, 500);
  // pending saves stay in the outbox: closing the tab loses nothing, they are
  // sent on the next visit
  window.addEventListener("pagehide", () => void flush());
}
