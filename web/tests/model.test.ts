// Unit tests of the learning model. Run: npm test (node --test, no deps).
import { test } from "node:test";
import assert from "node:assert/strict";
import { addScore, examPassed, isDemonstrated, isDone, labProgress, recentScore, topicMastered, topicReadiness } from "../lib/model.ts";

const at = (d: number) => new Date(Date.UTC(2026, 0, d)).toISOString();

test("no attempts is not started; a running one is in progress", () => {
  assert.equal(labProgress([]).status, "not_started");
  assert.equal(labProgress([{ labId: "a", status: "running", started: at(1) }]).status, "in_progress");
});

test("a failed hand-in is failed, never done", () => {
  const p = labProgress([{ labId: "a", status: "submitted", passed: false, score: 40, finished: at(1) }]);
  assert.equal(p.status, "failed");
  assert.equal(isDone(p.status), false);
});

test("passing with the solution or the tutor is completed, not passed", () => {
  const sol = labProgress([{ labId: "a", status: "submitted", passed: true, score: 100, solutionShown: true, finished: at(1) }]);
  const tut = labProgress([{ labId: "a", status: "submitted", passed: true, score: 100, tutor: true, finished: at(1) }]);
  for (const p of [sol, tut]) {
    assert.equal(p.status, "completed");
    assert.equal(isDone(p.status), true);
    assert.equal(isDemonstrated(p.status), false);
  }
});

test("passing alone (even with hints) is passed", () => {
  const p = labProgress([{ labId: "a", status: "submitted", passed: true, score: 85, hintsUsed: [0], finished: at(1) }]);
  assert.equal(p.status, "passed");
  assert.equal(p.independent, true);
});

test("a recent failure after a pass needs review and shows the last score", () => {
  const p = labProgress([
    { labId: "a", status: "submitted", passed: true, score: 95, finished: at(1) },
    { labId: "a", status: "submitted", passed: false, score: 30, finished: at(20) },
  ]);
  assert.equal(p.status, "needs_review");
  assert.equal(p.last, 30);
  assert.equal(p.best, 95);
});

test("a failed practice exam is not passed", () => {
  assert.equal(examPassed({ score: 20, total: 50 }), false);
  assert.equal(examPassed({ score: 35, total: 50 }), true);
  assert.equal(examPassed({ score: 0, total: 0 }), false);
});

test("the recent score shows a drop the best score hides", () => {
  let p = addScore({}, 100, 1);
  p = addScore(p, 40, 2);
  assert.equal(p.best, 100);
  assert.ok((recentScore(p) ?? 0) < 70);
});

test("reading every concept never makes a topic ready on its own", () => {
  assert.ok(topicReadiness(10, 10, {}) <= 0.1);
});

test("mastery needs two strong quizzes on different days", () => {
  const day = 86400000;
  let p = addScore({}, 100, 0);
  assert.equal(topicMastered(p), false);
  p = addScore(p, 100, 3600000);
  assert.equal(topicMastered(p), false, "same day does not prove recall");
  p = addScore(p, 90, 2 * day);
  assert.equal(topicMastered(p), true);
  p = addScore(p, 50, 3 * day);
  assert.equal(topicMastered(p), false, "a recent drop removes mastery");
});

import { schedule, type PlanEntry } from "../lib/model.ts";

test("the schedule never exceeds the daily budget (except one long activity alone)", () => {
  const e = (key: string, minutes: number, priority: PlanEntry["priority"] = "essential", done = false): PlanEntry => ({ key, minutes, priority, done });
  const entries = [e("a", 30), e("b", 40), e("c", 50, "recommended"), e("d", 200), e("e", 20, "optional"), e("f", 10, "essential", true), e("g", 60)];
  const s = schedule(entries, { minutesPerDay: 90, recommended: true, optional: false });
  for (const day of s) assert.ok(day.minutes <= 90 || (day.long && day.entries.length === 1), JSON.stringify(day));
  const keys = s.flatMap((d) => d.entries.map((x) => x.key));
  assert.deepEqual(keys, ["a", "b", "c", "d", "g"], "keeps order, skips done and optional");
  assert.equal(s.find((d) => d.long)?.entries[0].key, "d");
  const lean = schedule(entries, { minutesPerDay: 90, recommended: false, optional: true });
  assert.deepEqual(lean.flatMap((d) => d.entries.map((x) => x.key)), ["a", "b", "d", "e", "g"]);
});

import { mergeRun } from "../lib/model.ts";

test("merging runs after a conflict keeps both devices' work", () => {
  const server = { history: [{ at: 1, score: 50 }, { at: 3, score: 80 }], checked: { a: true }, current: { id: "srv" } };
  const local = { history: [{ at: 2, score: 70 }, { at: 3, score: 80 }], checked: { b: true, c: false }, current: { id: "here" } };
  const m = mergeRun(server, local);
  assert.deepEqual(m.history.map((h: any) => h.at), [3, 2, 1]);
  assert.deepEqual(m.checked, { a: true, b: true });
  assert.equal(m.current.id, "here");
  assert.equal(mergeRun(null, local), local);
});

import { placement } from "../lib/model.ts";

test("the starting diagnosis places the learner and never rewards blanks", () => {
  assert.equal(placement(0, 6), "zero");
  assert.equal(placement(2, 6), "zero");
  assert.equal(placement(3, 6), "basics");
  assert.equal(placement(4, 6), "basics");
  assert.equal(placement(5, 6), "intermediate");
  assert.equal(placement(6, 6), "intermediate");
  assert.equal(placement(0, 0), "zero");
});
