// Learning model shared by every page: the state of an activity, what counts
// as passing, and how a topic's score evolves. Pure functions without imports
// so they run under `node --test` (see web/tests/model.test.ts).
//
// Statuses:
//   not_started   never opened
//   in_progress   a session is running (or opened and never handed in)
//   failed        handed in, never passed
//   completed     finished with help that reveals the answer (tutor
//                 walkthrough or solution shown): done, not demonstrated
//   passed        passed on your own (hints allowed, no solution)
//   needs_review  passed before, but the most recent attempt failed

export type ActivityStatus = "not_started" | "in_progress" | "completed" | "passed" | "failed" | "needs_review";

export interface AttemptLite {
  labId: string;
  status?: string; // running, submitted, expired, abandoned
  passed?: boolean;
  score?: number;
  tutor?: boolean;
  solutionShown?: boolean;
  hintsUsed?: number[] | null;
  started?: string;
  finished?: string | null;
  mode?: string;
}

/** helped: the attempt saw the answer (tutor walkthrough or full solution). */
export const helped = (a: AttemptLite) => !!(a.tutor || a.solutionShown);

const time = (a: AttemptLite) => Date.parse(a.finished ?? a.started ?? "") || 0;

export interface LabProgress {
  status: ActivityStatus;
  /** latest handed-in score (not the best: a recent drop must show) */
  last?: number;
  best?: number;
  attempts: number;
  /** passed at least once without seeing the answer */
  independent: boolean;
}

/**
 * labProgress derives the status of one lab from its attempts. Guided
 * workshops are designed to be followed with the tutor, so finishing one with
 * help still counts as completed (never as passed).
 */
export function labProgress(atts: AttemptLite[]): LabProgress {
  const subs = atts.filter((a) => a.status === "submitted" || a.finished).sort((a, b) => time(a) - time(b));
  const running = atts.some((a) => a.status === "running");
  const res: LabProgress = { status: "not_started", attempts: subs.length, independent: false };
  if (!subs.length) {
    res.status = running || atts.length ? "in_progress" : "not_started";
    return res;
  }
  const lastSub = subs[subs.length - 1];
  res.last = lastSub.score ?? 0;
  res.best = Math.max(...subs.map((a) => a.score ?? 0));
  const indep = subs.some((a) => a.passed && !helped(a));
  const anyPass = subs.some((a) => a.passed);
  res.independent = indep;
  if (!anyPass) res.status = running ? "in_progress" : "failed";
  else if (!lastSub.passed) res.status = "needs_review";
  else if (indep) res.status = "passed";
  else res.status = "completed";
  return res;
}

/** progressByLab groups attempts by lab and derives each status. */
export function progressByLab(atts: AttemptLite[]): Record<string, LabProgress> {
  const by: Record<string, AttemptLite[]> = {};
  for (const a of atts) (by[a.labId] = by[a.labId] ?? []).push(a);
  return Object.fromEntries(Object.entries(by).map(([id, as]) => [id, labProgress(as)]));
}

/** counts as done for moving along a route (help allowed). */
export const isDone = (s?: ActivityStatus) => s === "passed" || s === "completed" || s === "needs_review";
/** counts as demonstrated (authentic progress). */
export const isDemonstrated = (s?: ActivityStatus) => s === "passed";

// ---- Practice exams --------------------------------------------------------

/** Pass mark of the practice certification exams (official exams don't publish one; 70 % is the usual target). */
export const EXAM_PASS = 0.7;
export const examPassed = (r: { score: number; total: number }) => r.total > 0 && r.score / r.total >= EXAM_PASS;

// ---- Support shifts -----------------------------------------------------------

/** A shift passes at 70 points with at least one ticket solved. */
export const SHIFT_PASS = 70;
export const shiftPassed = (h: { score: number; passed?: boolean }) => h.passed ?? h.score >= SHIFT_PASS;

// ---- Topics (theory) ---------------------------------------------------------

export interface QuizScore {
  at: number;
  score: number;
}
export interface TopicScores {
  best?: number;
  scores?: QuizScore[];
}

/** TOPIC_PASS: score of the most recent quiz needed to consider a topic learnt. */
export const TOPIC_PASS = 70;

/** lastScore is the most recent quiz score (older data only kept the best). */
export function lastScore(p?: TopicScores): number | undefined {
  const s = p?.scores;
  if (s && s.length) return [...s].sort((a, b) => a.at - b.at)[s.length - 1].score;
  return p?.best;
}

/**
 * recentScore weighs the last quizzes so a recent drop shows: the latest
 * counts 50 %, the one before 30 %, the one before that 20 %.
 */
export function recentScore(p?: TopicScores): number | undefined {
  const s = [...(p?.scores ?? [])].sort((a, b) => b.at - a.at).slice(0, 3);
  if (!s.length) return p?.best;
  const w = [0.5, 0.3, 0.2].slice(0, s.length);
  const sum = w.reduce((a, b) => a + b, 0);
  return Math.round(s.reduce((acc, x, i) => acc + x.score * w[i], 0) / sum);
}

/** addScore appends a quiz result (kept: the last 12). */
export function addScore(p: TopicScores, score: number, at = Date.now()): TopicScores {
  const scores = [...(p.scores ?? []), { at, score }].sort((a, b) => a.at - b.at).slice(-12);
  return { ...p, scores, best: Math.max(p.best ?? 0, score) };
}

/**
 * topicReadiness 0..1. Reading the concepts only opens the door (10 %); the
 * rest is the recent quiz performance. Marking concepts as understood never
 * makes a topic "mastered" on its own.
 */
export function topicReadiness(conceptsSeen: number, conceptsTotal: number, p?: TopicScores): number {
  const read = conceptsTotal ? Math.min(1, conceptsSeen / conceptsTotal) : 0;
  return 0.1 * read + 0.9 * ((recentScore(p) ?? 0) / 100);
}

/**
 * topicMastered: two quizzes on different days, the most recent ones at 85 %
 * or more (recall over time, not one lucky run).
 */
export function topicMastered(p?: TopicScores): boolean {
  const s = [...(p?.scores ?? [])].sort((a, b) => a.at - b.at);
  if (s.length < 2) return false;
  const last = s[s.length - 1];
  const prevDay = s.slice(0, -1).reverse().find((x) => Math.abs(last.at - x.at) >= 20 * 3600 * 1000);
  return !!prevDay && last.score >= 85 && prevDay.score >= 85 && (recentScore(p) ?? 0) >= 85;
}
