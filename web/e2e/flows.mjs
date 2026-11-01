// End-to-end flows in a real browser (Playwright + Chromium).
//
//   node web/e2e/flows.mjs http://127.0.0.1:8099
//
// Needs the Go server running with the built web (see README) and Playwright
// installed. Each flow prints PASS/FAIL; the exit code is the number of
// failures. Flows use fresh accounts, so they can run against any instance.

import { createRequire } from "node:module";
const require = createRequire(import.meta.url);
const { chromium } = require("playwright");

const base = process.argv[2] ?? "http://127.0.0.1:8099";
const results = [];
const check = (name, ok, detail = "") => {
  results.push({ name, ok });
  console.log(`${ok ? "PASS" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);
};

async function account(tag) {
  const r = await fetch(base + "/api/auth/register", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ email: `${tag}-${Date.now()}@e2e.dev`, name: "E2E " + tag, password: "pass-12345678" }) });
  return (await r.json()).token;
}
const call = (tok, path, body, method) => fetch(base + path, { method: method ?? (body ? "POST" : "GET"), headers: { "content-type": "application/json", authorization: "Bearer " + tok }, body: body ? JSON.stringify(body) : undefined });

async function page(browser, tok, opts = {}) {
  const ctx = await browser.newContext({ viewport: opts.phone ? { width: 390, height: 844 } : { width: 1280, height: 900 } });
  if (tok) await ctx.addInitScript((t) => localStorage.setItem("gcplab.token", t), tok);
  const p = await ctx.newPage();
  const errors = [];
  p.on("pageerror", (e) => errors.push(e.message));
  return { ctx, p, errors };
}

const flows = {
  // The plan uses the chosen daily time and keeps it after a reload (saved on the account).
  async planPreferences(browser) {
    const tok = await account("plan");
    const { ctx, p } = await page(browser, tok);
    await p.goto(base + "/plan", { waitUntil: "networkidle" });
    await p.selectOption("#pl-budget", "60");
    await p.waitForTimeout(2500);
    const s1 = await p.locator(".pl-summary").textContent();
    await p.reload({ waitUntil: "networkidle" });
    await p.waitForTimeout(800);
    const v = await p.locator("#pl-budget").inputValue();
    check("plan: the daily time is used and survives a reload", v === "60" && /1 h/.test(s1 ?? ""), `budget=${v} summary="${s1}"`);
    const over = await p.evaluate(() => [...document.querySelectorAll(".pl-day")].length);
    check("plan: the work is split into several sessions", over > 1, `${over} sessions`);
    await ctx.close();
  },

  // A save made while offline is kept and sent when the connection returns.
  async offlineSave(browser) {
    const tok = await account("off");
    const { ctx, p } = await page(browser, tok);
    await p.goto(base + "/plan", { waitUntil: "networkidle" });
    await ctx.setOffline(true);
    await p.selectOption("#pl-budget", "30");
    await p.waitForTimeout(1500);
    const offline = await p.locator(".save-status").textContent();
    await ctx.setOffline(false);
    await p.evaluate(() => window.dispatchEvent(new Event("online")));
    await p.waitForTimeout(3000);
    const r = await (await call(tok, "/api/me/study")).json();
    const saved = JSON.stringify(r.runs?.["cm-plan"] ?? {}).includes('"minutesPerDay":30');
    check("save: offline shows a clear message", /conexión|offline/i.test(offline ?? ""), offline ?? "");
    check("save: the offline change reaches the server once online", saved);
    await ctx.close();
  },

  // A failed practice exam never ticks the plan; a passed one does.
  async examPassFail(browser) {
    const tok = await account("exam");
    await call(tok, "/api/me/study/exams", { cert: "ACE", at: Date.now() - 5000, score: 20, total: 50, full: true, seconds: 3600, domains: [] });
    const { ctx, p } = await page(browser, tok);
    await p.goto(base + "/plan", { waitUntil: "networkidle" });
    await p.locator("details.card summary").first().click();
    const failedDone = await p.locator("li.done", { hasText: "Simulacro completo del ACE" }).count();
    check("exam: a failed practice exam does not count as done", failedDone === 0);
    await call(tok, "/api/me/study/exams", { cert: "ACE", at: Date.now(), score: 40, total: 50, full: true, seconds: 3600, domains: [] });
    await p.reload({ waitUntil: "networkidle" });
    await p.locator("details.card summary").first().click();
    const passedDone = await p.locator("li.done", { hasText: "Simulacro completo del ACE" }).count();
    check("exam: a passed practice exam counts", passedDone === 1);
    await ctx.close();
  },
};

const only = process.argv[3];
const browser = await chromium.launch();
for (const [name, fn] of Object.entries(flows)) {
  if (only && name !== only) continue;
  try {
    await fn(browser);
  } catch (e) {
    check(name, false, e.message.split("\n")[0]);
  }
}
await browser.close();
const failed = results.filter((r) => !r.ok).length;
console.log(`\n${results.length - failed}/${results.length} checks passed`);
process.exit(failed);
