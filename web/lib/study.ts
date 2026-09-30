// Shared model and helpers of the "Learn" section: study topics, official
// exam structures, per-browser progress and practice-exam construction.

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
export const saveProgress = (p: Progress) => write("cm-learn", p);
export const loadHistory = () => read<ExamRecord[]>("cm-learn-exams", []);
export function addHistory(r: ExamRecord) {
  write("cm-learn-exams", [r, ...loadHistory()].slice(0, 50));
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
