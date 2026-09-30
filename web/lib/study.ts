// Shared model and helpers of the "Learn" section: study topics, official
// exam structures, progress and practice-exam construction.
//
// Progress lives in the learner's account (/api/me/study); localStorage is a
// cache so the page renders instantly and keeps working if a save fails.
// The first sync in a browser uploads what was stored there before progress
// moved to the server, so nothing studied earlier is lost.

import { api } from "@/lib/api";

export interface Concept {
  id: string;
  term: string;
  what: string;
  points: string[];
  tip: string;
}
export interface Question {
  q: string;
  options: string[];
  answers: number[];
  why: string;
}
export interface Topic {
  id: string;
  branch: string;
  name: string;
  intro: string;
  concepts: Concept[];
  scenarios: Question[];
  quiz: Question[];
}
export interface Domain {
  name: string;
  weight: number;
  topics: string[];
  objectives: string[];
}
export interface ExamInfo {
  cert: string;
  minutes: number;
  questions: string;
  fee: string;
  validity: string;
  experience: string;
  format: string;
  domains: Domain[];
}
export interface Cert {
  id: string;
  name: string;
  level: string;
  guide: string;
}

export interface TopicProgress {
  seen?: string[];
  known?: string[];
  best?: number;
}
export type Progress = Record<string, TopicProgress>;

export interface ExamRecord {
  cert: string;
  at: number;
  score: number;
  total: number;
  full: boolean;
  seconds: number;
  domains: { name: string; ok: number; total: number }[];
}

function read<T>(key: string, fallback: T): T {
  try {
    const v = localStorage.getItem(key);
    return v ? (JSON.parse(v) as T) : fallback;
  } catch {
    return fallback;
  }
}
function write(key: string, v: unknown) {
  try {
    localStorage.setItem(key, JSON.stringify(v));
  } catch {
    /* private mode: progress is a convenience only */
  }
}

export const loadProgress = () => read<Progress>("cm-learn", {});
export const loadHistory = () => read<ExamRecord[]>("cm-learn-exams", []);

// saveProgress caches the progress and saves the topics that changed.
export function saveProgress(p: Progress, changed?: string[]) {
  write("cm-learn", p);
  const body = Object.fromEntries((changed ?? Object.keys(p)).filter((id) => p[id]).map((id) => [id, p[id]]));
  if (Object.keys(body).length) api("/api/me/study/progress", { body }).catch(() => {});
}

export function addHistory(r: ExamRecord) {
  write("cm-learn-exams", [r, ...loadHistory().filter((x) => !(x.at === r.at && x.cert === r.cert))].slice(0, 50));
  api("/api/me/study/exams", { body: r }).catch(() => {});
}

// Practice exams in progress: cached locally, mirrored to the account.
let serverRuns: Record<string, unknown> = {};
const runTimers: Record<string, ReturnType<typeof setTimeout>> = {};
export function loadRun<T>(key: string): T | null {
  return read<T | null>(key, null) ?? ((serverRuns[key] as T) || null);
}
export function saveRun(key: string, run: unknown | null) {
  clearTimeout(runTimers[key]);
  if (run) {
    write(key, run);
    serverRuns[key] = run;
    runTimers[key] = setTimeout(() => api(`/api/me/study/runs/${key}`, { method: "PUT", body: run }).catch(() => {}), 800);
  } else {
    try {
      localStorage.removeItem(key);
    } catch {
      /* ignore */
    }
    delete serverRuns[key];
    api(`/api/me/study/runs/${key}`, { method: "DELETE" }).catch(() => {});
  }
}

interface StudyDoc {
  user: string;
  progress: Progress;
  exams: ExamRecord[];
  runs: Record<string, unknown>;
}

// syncStudy loads the account's progress into the local cache. The first
// time in a browser (or after a logout), progress cached there by this same
// learner before the account kept it is uploaded and merged; a different
// learner's cache is discarded instead.
export async function syncStudy(): Promise<{ progress: Progress; history: ExamRecord[] }> {
  const doc = await api<StudyDoc>("/api/me/study");
  const owner = read<string>("cm-learn-owner", "");
  let progress = doc.progress ?? {};
  let history = doc.exams ?? [];
  const runs = doc.runs ?? {};
  if (!owner) {
    const local = loadProgress();
    const merged: Progress = { ...progress };
    const changed: string[] = [];
    for (const [id, lp] of Object.entries(local)) {
      const sp = progress[id] ?? {};
      const m: TopicProgress = {
        seen: Array.from(new Set([...(sp.seen ?? []), ...(lp.seen ?? [])])),
        known: Array.from(new Set([...(sp.known ?? []), ...(lp.known ?? [])])),
        best: Math.max(sp.best ?? 0, lp.best ?? 0) || (sp.best ?? lp.best),
      };
      if (JSON.stringify(m) !== JSON.stringify(sp)) changed.push(id);
      merged[id] = m;
    }
    if (changed.length) await api("/api/me/study/progress", { body: Object.fromEntries(changed.map((id) => [id, merged[id]])) }).catch(() => {});
    for (const r of loadHistory()) {
      if (!history.some((x) => x.at === r.at && x.cert === r.cert)) {
        await api("/api/me/study/exams", { body: r }).catch(() => {});
        history = [r, ...history];
      }
    }
    history.sort((a, b) => b.at - a.at);
    progress = merged;
  }
  // Runs: the account is the reference. A run cached here but missing there
  // was either handed in elsewhere (drop it) or not uploaded yet (upload it).
  for (const k of Object.keys(localStorage).filter((k) => k.startsWith("cm-exam-run-"))) {
    const local = read<{ started?: number } | null>(k, null);
    const [, cert, mode] = /^cm-exam-run-(\w+)-(full|quick)$/.exec(k) ?? [];
    if (owner && owner !== doc.user) localStorage.removeItem(k);
    else if (!runs[k] && local) {
      const finished = history.some((h) => h.cert === cert && h.full === (mode === "full") && h.at >= (local.started ?? 0));
      if (finished) localStorage.removeItem(k);
      else {
        runs[k] = local;
        api(`/api/me/study/runs/${k}`, { method: "PUT", body: local }).catch(() => {});
      }
    }
  }
  serverRuns = { ...runs };
  write("cm-learn-owner", doc.user);
  write("cm-learn", progress);
  write("cm-learn-exams", history.slice(0, 50));
  return { progress, history: history.slice(0, 50) };
}

export function shuffle<T>(a: T[]): T[] {
  const b = [...a];
  for (let i = b.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [b[i], b[j]] = [b[j], b[i]];
  }
  return b;
}

// Shuffled copy of a question whose answer indexes follow their options.
export function mix(q: Question): Question {
  const order = shuffle(q.options.map((_, i) => i));
  return { ...q, options: order.map((i) => q.options[i]), answers: q.answers.map((a) => order.indexOf(a)).sort() };
}

export function isRight(q: Question, sel: number[]) {
  return sel.length === q.answers.length && q.answers.every((a) => sel.includes(a));
}

// Topic readiness 0..1: concepts understood (40 %) + best quiz score (60 %).
export function readiness(t: Topic, p?: TopicProgress) {
  const seen = (p?.seen ?? []).filter((id) => t.concepts.some((c) => c.id === id)).length;
  return 0.4 * (seen / Math.max(1, t.concepts.length)) + 0.6 * ((p?.best ?? 0) / 100);
}

// Weight of each topic in an exam: each domain shares its weight among its topics.
export function topicWeights(ex: ExamInfo): Record<string, number> {
  const w: Record<string, number> = {};
  for (const d of ex.domains) for (const id of d.topics) w[id] = (w[id] ?? 0) + d.weight / d.topics.length;
  return w;
}

export interface ExamQuestion extends Question {
  domain: number;
}

// buildExam picks n questions spread across the official domains by weight
// (largest remainder), never repeating one, then fills any gap from the
// certification's other topics.
export function buildExam(ex: ExamInfo, byId: Record<string, Topic>, n: number): ExamQuestion[] {
  const raw = ex.domains.map((d) => (n * d.weight) / 100);
  const counts = raw.map(Math.floor);
  const order = raw.map((r, i) => [r - Math.floor(r), i]).sort((a, b) => b[0] - a[0]);
  for (let k = 0; counts.reduce((a, b) => a + b, 0) < n; k++) counts[order[k % order.length][1]]++;
  const used = new Set<Question>();
  const out: ExamQuestion[] = [];
  const pool = (ids: string[]) => shuffle(ids.flatMap((id) => (byId[id] ? [...byId[id].quiz, ...byId[id].scenarios] : [])).filter((q) => !used.has(q)));
  ex.domains.forEach((d, i) => {
    for (const q of pool(d.topics).slice(0, counts[i])) {
      used.add(q);
      out.push({ ...mix(q), domain: i });
    }
  });
  const all = ex.domains.flatMap((d, i) => d.topics.map((id) => [id, i] as [string, number]));
  for (const [id, i] of shuffle(all)) {
    if (out.length >= n) break;
    const q = pool([id])[0];
    if (q) {
      used.add(q);
      out.push({ ...mix(q), domain: i });
    }
  }
  return shuffle(out).slice(0, n);
}

export const fmtTime = (s: number) => {
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const ss = s % 60;
  return (h ? h + ":" + String(m).padStart(2, "0") : String(m)) + ":" + String(ss).padStart(2, "0");
};
