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

// Commands go through the session API: the console and Cloud Shell share that
// same simulator state, so the page sees exactly what a typed command does.
// the first-visit tour of the lab covers the page: skip it like a person would
async function skipTour(p) {
  // the tour can appear a moment after the page loads
  const skip = p.getByRole("button", { name: "Saltar" });
  await skip.waitFor({ state: "visible", timeout: 3000 }).then(() => skip.click()).catch(() => {});
}
const exec = (tok, sid, line) => call(tok, `/api/sessions/${sid}/exec`, { line });

Object.assign(flows, {
  // Hints come in five levels, each named, and none before the 4th shows a change.
  async hintLadder(browser) {
    const tok = await account("hint");
    const s = await (await call(tok, "/api/labs/ace-d09-health-check/start", { mode: "assisted" })).json();
    const { ctx, p, errors } = await page(browser, tok);
    await p.goto(`${base}/lab?session=${s.id}&id=ace-d09-health-check`, { waitUntil: "networkidle" });
    await skipTour(p);
    const btn = p.getByRole("button", { name: /Pedir una pista|Siguiente pista/ });
    const seen = [];
    for (let i = 0; i < 5; i++) {
      if (!(await btn.isEnabled())) break;
      await btn.click();
      await p.waitForFunction((n) => document.querySelectorAll(".lp-hint").length >= n, i + 1);
      seen.push(await p.locator(".lp-hint").nth(i).innerText());
    }
    // read the labels after all five are shown: each must still say "of 5"
    const labels = (await p.locator(".lp-hint .cc-pill").allInnerTexts()).map((x) => x.split("·")[0].trim());
    const names = ["Pregunta guía", "Concepto", "Dónde mirar", "Comando de ejemplo", "Solución completa"];
    check("hints: five levels in order, each 'of 5'", labels.length === 5 && labels.every((l, i) => l === `Nivel ${i + 1} de 5: ${names[i]}`), labels.join(" | "));
    check("hints: the first three levels show no change", !seen.slice(0, 3).some((x) => /firewall-rules update/.test(x)));
    check("hints: the button stops after the last level", !(await btn.isEnabled()));
    const atts = await (await call(tok, "/api/me/attempts")).json();
    const a = (Array.isArray(atts) ? atts : atts.attempts ?? []).find((x) => x.labId === "ace-d09-health-check");
    check("hints: the five levels used are recorded in the attempt", a?.hintsUsed?.length === 5 && a?.hintCost === 32, `hintsUsed=${a?.hintsUsed?.length} hintCost=${a?.hintCost}`);
    check("hints: no page errors", errors.length === 0, errors.join("; "));
    await ctx.close();
  },

  // Independent mode gives no hints, in the page and in the API.
  async independentNoHints(browser) {
    const tok = await account("ind");
    const s = await (await call(tok, "/api/labs/ace-d09-health-check/start", { mode: "independent" })).json();
    const r = await call(tok, `/api/sessions/${s.id}/hint`, {});
    const { ctx, p } = await page(browser, tok);
    await p.goto(`${base}/lab?session=${s.id}&id=ace-d09-health-check`, { waitUntil: "networkidle" });
    await skipTour(p);
    const btns = await p.getByRole("button", { name: /Pedir una pista|Siguiente pista/ }).count();
    check("independent: the API refuses hints (403)", r.status === 403, `status ${r.status}`);
    check("independent: the page offers no hint button", btns === 0);
    await ctx.close();
  },

  // A blind fix is handed in: the result names the kind of mistake, and the
  // post-incident report is scored part by part against the session.
  async diagnosisAndReport(browser) {
    const tok = await account("diag");
    const s = await (await call(tok, "/api/labs/ace-d09-health-check/start", { mode: "assisted" })).json();
    // the rule name changes with each variant of the lab
    const rule = s.params?.rule ?? "fw-hc-probes";
    const ran = await (await exec(tok, s.id, `gcloud compute firewall-rules update ${rule} --source-ranges=0.0.0.0/0`)).json();
    check("diagnosis: the blind change was applied", ran.exit === 0, ran.output?.slice(0, 80));
    const { ctx, p, errors } = await page(browser, tok);
    p.on("dialog", (d) => d.accept());
    await p.goto(`${base}/lab?session=${s.id}&id=ace-d09-health-check`, { waitUntil: "networkidle" });
    await skipTour(p);
    await p.getByRole("tab", { name: "Entrega" }).click();
    const report = p.locator("details.lp-report");
    check("report: incidents offer the post-incident report", (await report.count()) === 1);
    if (!(await report.getAttribute("open"))) await report.locator("summary").click();
    await p.fill("#rp-symptom", "La web devuelve 502 a todos los clientes desde esta mañana");
    await p.fill("#rp-evidenceSeen", "No miré nada, cambié el cortafuegos directamente");
    await p.fill("#rp-action", `Abrí ${rule} a 0.0.0.0/0 para que pasara todo`);
    await p.getByRole("button", { name: "Enviar", exact: true }).click();
    await p.waitForSelector(".res-diag", { timeout: 30000 });
    const diag = await p.locator(".res-diag").innerText();
    check("diagnosis: names 'changed things before looking'", /Cambiaste cosas antes de mirar/.test(diag));
    check("diagnosis: names the risky change", /cambio arriesgado/.test(diag));
    check("diagnosis: names the missing validation", /No comprobaste el resultado/.test(diag), diag.replace(/\s+/g, " ").slice(0, 400));
    const rep = await p.locator("#res-report-title").innerText();
    const ev = await p.locator(".res-card", { hasText: "Informe post-incidente" }).innerText();
    check("report: scored apart, evidence not backed by the session scores low", /Informe post-incidente · \d+ %/.test(rep) && /no consultaste nada/.test(ev), rep);
    check("diagnosis: no page errors", errors.length === 0, errors.join("; "));
    await ctx.close();
  },

  // The dashboard shows the five competencies, with "—" where nothing was measured.
  async competencies(browser) {
    const tok = await account("comp");
    const s = await (await call(tok, "/api/labs/ace-d09-health-check/start", { mode: "assisted" })).json();
    await exec(tok, s.id, "gcloud compute backend-services list");
    await exec(tok, s.id, `gcloud compute firewall-rules update ${s.params?.rule} --source-ranges=35.191.0.0/16,130.211.0.0/22`);
    await call(tok, `/api/sessions/${s.id}/submit`, { evidence: {} });
    // a quiz taken now (5 min towards the weekly goal)
    const topics = (await (await call(tok, "/api/learn")).json()).topics;
    await call(tok, "/api/me/study/progress", { [topics[0].id]: { scores: [{ at: Date.now(), score: 80 }], best: 80 } });
    const { ctx, p } = await page(browser, tok);
    await p.goto(base + "/dashboard", { waitUntil: "networkidle" });
    await p.getByText("Estadísticas detalladas", { exact: false }).click();
    const head = await p.locator(".comp-table thead").innerText().catch(() => "");
    const cells = await p.locator(".comp-table tbody tr").first().innerText().catch(() => "");
    check("competencies: five columns on the dashboard", ["Comprensión", "Ejecución", "Autonomía", "Retención", "Transferencia"].every((c) => head.includes(c)), head.replace(/\s+/g, " "));
    const week = await p.locator(".db-today .cm-card", { hasText: "Objetivo semanal" }).innerText().catch(() => "");
    check("weekly goal: counts this week's practice (the quiz: 5 min)", /\b([5-9]|[1-9]\d+) de 600 min esta semana/.test(week), week.replace(/\s+/g, " "));
    check("competencies: unmeasured shown as —, scores as 0..100 (not ×100)", cells.includes("—") && !/\d{4}/.test(cells), cells.replace(/\s+/g, " "));
    await ctx.close();
  },
});

// ---- 2.0 shell: onboarding, navigation, search, shortcuts, theme, language, a11y

async function uiRegister(p, tag) {
  await p.goto(base + "/", { waitUntil: "networkidle" });
  await p.getByRole("button", { name: "Crear cuenta" }).first().click();
  await p.fill("#email", `${tag}-${Date.now()}@e2e.dev`);
  await p.fill("#name", "Lucía E2E");
  await p.fill("#pw", "pass-12345678");
  await p.getByRole("button", { name: "Crear cuenta" }).last().click();
}

Object.assign(flows, {
  // A new account lands on the welcome, takes the diagnosis and gets a plan
  // that matches: someone who knows the basics skips Week 0.
  async onboarding(browser) {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    const p = await ctx.newPage();
    const errors = [];
    p.on("pageerror", (e) => errors.push(e.message));
    await uiRegister(p, "ob");
    await p.waitForURL("**/start", { timeout: 15000 });
    check("onboarding: a new account opens the welcome", true);
    await p.getByLabel(/Aprobar la certificación/).check();
    await p.getByText("1,5 h").click();
    await p.getByRole("button", { name: /Siguiente: diagnóstico/ }).click();
    // right answers to 4 of 6, "I don't know" to the rest → basics
    const right = ["El contenedor donde viven", "Que está en una zona de la región", "Qué identidad hace la llamada", "Cloud NAT para esa red"];
    for (const r of right) await p.getByLabel(new RegExp(r)).check();
    const blanks = await p.getByLabel("No lo sé").all();
    await blanks[4].check();
    await blanks[5].check();
    await p.getByRole("button", { name: /Ver mi punto de partida/ }).click();
    const h1 = await p.locator("h1").innerText();
    check("onboarding: 4 of 6 places the learner on 'basics'", /Ya tienes una base/.test(h1), h1);
    await p.waitForTimeout(2500);
    // the browser's own session (HttpOnly cookie) reads the account
    const study = await (await p.request.get(base + "/api/me/study")).json();
    const prefs = study.runs?.["cm-plan"]?.prefs ?? {};
    check("onboarding: the plan gets the chosen time and skips Week 0", prefs.minutesPerDay === 90 && prefs.skipBasics === true, JSON.stringify(prefs));
    check("onboarding: the result is saved on the account", study.runs?.["cm-onboarding"]?.level === "basics");
    await p.getByRole("link", { name: "Empezar ahora" }).click();
    await p.waitForURL(/lab\?id=zentra-s01-triage/);
    check("onboarding: 'start now' opens the first activity of the level", true);
    check("onboarding: no page errors", errors.length === 0, errors.join("; "));
    await ctx.close();
  },

  // Five destinations everywhere; the phone bar does not overflow.
  async navigation(browser) {
    const tok = await account("nav");
    for (const phone of [false, true]) {
      const { ctx, p } = await page(browser, tok, { phone });
      await p.goto(base + "/guide", { waitUntil: "networkidle" });
      const dests = await p.locator(".cm-rail-links a").allInnerTexts();
      const cur = await p.locator('.cm-rail-links a[aria-current="page"]').innerText();
      const tabs = await p.locator(".cm-tabs a").allInnerTexts();
      const ov = await p.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
      const tag = phone ? "phone" : "desktop";
      check(`nav (${tag}): five destinations`, dests.length === 5 && dests.join("|") === "Inicio|Aprender|Practicar|Trabajo|Perfil", dests.join("|"));
      check(`nav (${tag}): the current destination and its pages are marked`, cur === "Aprender" && tabs.includes("Guía de diagnóstico"), `${cur} / ${tabs.join(", ")}`);
      check(`nav (${tag}): no horizontal overflow`, ov <= 0, `${ov}px`);
      await ctx.close();
    }
  },

  // Ctrl+K opens the search, typing filters, Enter goes; g+letter jumps.
  async searchAndShortcuts(browser) {
    const tok = await account("kbd");
    const { ctx, p } = await page(browser, tok);
    await p.goto(base + "/dashboard", { waitUntil: "networkidle" });
    await p.keyboard.press("Control+k");
    const dlg = p.getByRole("dialog", { name: "Buscar" });
    check("search: Ctrl+K opens the search dialog", await dlg.isVisible());
    await p.keyboard.type("emparejamiento");
    await p.waitForSelector(".cs-list li[aria-selected=true]");
    const first = await p.locator(".cs-list li[aria-selected=true]").innerText();
    await p.keyboard.press("Enter");
    await p.waitForURL(/\/lab\?id=/);
    check("search: Enter opens the selected lab", /emparejamiento/i.test(first), first.replace(/\s+/g, " "));
    await p.goto(base + "/dashboard", { waitUntil: "networkidle" });
    await p.keyboard.press("g");
    await p.keyboard.press("a");
    await p.waitForURL(/\/learn/);
    check("shortcuts: g a goes to Aprender", true);
    await p.keyboard.press("?");
    check("shortcuts: ? lists the shortcuts", await p.getByRole("dialog", { name: "Atajos de teclado" }).isVisible());
    await p.keyboard.press("Escape");
    await ctx.close();
  },

  // Theme and density are kept after a reload and applied before paint.
  async themeAndLanguage(browser) {
    const tok = await account("thm");
    const { ctx, p } = await page(browser, tok, { phone: true });
    await p.goto(base + "/dashboard", { waitUntil: "networkidle" });
    await p.locator(".cm-avatar").first().click();
    await p.getByRole("radio", { name: "Oscuro" }).click();
    await p.getByRole("radio", { name: "Compacta" }).click();
    await p.reload({ waitUntil: "domcontentloaded" });
    // set before paint, by the inline boot script
    const attrs = await p.evaluate(() => [document.documentElement.dataset.theme, document.documentElement.dataset.density]);
    await p.waitForSelector("body.cm");
    const bg = await p.evaluate(() => getComputedStyle(document.body).backgroundColor);
    check("theme: dark and compact survive a reload", attrs[0] === "dark" && attrs[1] === "compact", attrs.join(","));
    check("theme: the dark palette is applied", bg === "rgb(17, 19, 24)", bg);
    // language from the account menu, on a phone
    await p.waitForLoadState("networkidle");
    await p.locator(".cm-avatar").first().click();
    await p.getByRole("button", { name: "EN", exact: true }).click();
    await p.waitForLoadState("networkidle");
    await p.waitForTimeout(500);
    const lang = await p.evaluate(() => document.documentElement.lang);
    const home = await p.locator(".cm-rail-links a").first().innerText();
    check("language: switched on a phone, <html lang> follows", lang === "en" && home === "Start", `${lang} ${home}`);
    await ctx.close();
  },

  // axe-core (WCAG 2.x A/AA rules) on the main pages, light and dark.
  async accessibility(browser) {
    let axe;
    try {
      axe = require("axe-core").source;
    } catch {
      check("a11y: axe-core installed (npm i in web/)", false);
      return;
    }
    const tok = await account("a11y");
    for (const scheme of ["light", "dark"]) {
      const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 }, colorScheme: scheme });
      await ctx.addInitScript((t) => localStorage.setItem("gcplab.token", t), tok);
      const p = await ctx.newPage();
      for (const path of ["/dashboard", "/plan", "/learn", "/start", "/shift", "/skills", "/catalog", "/zentra", "/incidents", "/company", "/guide", "/leaderboard", "/sandbox", "/"]) {
        await p.goto(base + path, { waitUntil: "networkidle" });
        await p.waitForTimeout(400);
        await p.addScriptTag({ content: axe });
        const res = await p.evaluate(async () => (await window.axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"] } })).violations.map((v) => `${v.id}(${v.nodes.length})`));
        check(`a11y ${scheme} ${path}: no WCAG A/AA violations`, res.length === 0, res.join(", "));
      }
      // the lab workspace (console replica, panel and Cloud Shell)
      const lab = await (await call(tok, "/api/labs/ace-d09-health-check/start", { mode: "assisted" })).json();
      await p.goto(`${base}/lab?session=${lab.id}&id=ace-d09-health-check`, { waitUntil: "networkidle" });
      await skipTour(p);
      await p.waitForTimeout(400);
      await p.addScriptTag({ content: axe });
      const lv = await p.evaluate(async () => (await window.axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"] } })).violations.map((v) => `${v.id}(${v.nodes.length})`));
      check(`a11y ${scheme} lab workspace: no WCAG A/AA violations`, lv.length === 0, lv.join(", "));
      await ctx.close();
    }
  },
});

Object.assign(flows, {
  // The session is an HttpOnly cookie: scripts never see the token, the
  // pages carry a CSP, and signing out ends the session on the server.
  async sessionSecurity(browser) {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    const p = await ctx.newPage();
    const csp = [];
    p.on("console", (m) => /Content Security Policy|Refused to/i.test(m.text()) && csp.push(m.text().slice(0, 160)));
    await uiRegister(p, "sec");
    await p.waitForURL("**/start");
    const stored = await p.evaluate(() => Object.keys(localStorage).filter((k) => /token/i.test(k) && localStorage.getItem(k)));
    const cookies = await ctx.cookies();
    const sess = cookies.find((c) => c.name === "cm_session");
    check("session: no token in localStorage", stored.length === 0, stored.join(","));
    check("session: HttpOnly, SameSite=Strict cookie", !!sess && sess.httpOnly && sess.sameSite === "Strict");
    check("session: document.cookie cannot read it", !(await p.evaluate(() => document.cookie)).includes("cm_session"));
    const head = await p.request.get(base + "/dashboard");
    check("headers: CSP and frame protection on pages", /default-src 'self'/.test(head.headers()["content-security-policy"] ?? "") && head.headers()["x-frame-options"] === "DENY");
    // a forged cross-site change (no CSRF header) is refused, the session stays
    const forged = await p.request.put(base + "/api/me/study/runs/cm-plan", { data: { x: 1 } });
    check("csrf: a change without the client header is refused (403)", forged.status() === 403, String(forged.status()));
    // pages with Mermaid, video, the lab and the editor load without CSP blocks
    for (const path of ["/dashboard", "/learn", "/zentra", "/guide", "/catalog"]) await p.goto(base + path, { waitUntil: "networkidle" });
    check("csp: main pages load with no CSP violation", csp.length === 0, csp.join(" | "));
    await p.locator(".cm-avatar").first().click();
    await p.getByRole("button", { name: "Cerrar sesión" }).click();
    await p.waitForURL(base + "/");
    const after = await p.request.get(base + "/api/me");
    check("session: signing out removes the session cookie", after.status() === 401, String(after.status()));
    await ctx.close();
  },
});

Object.assign(flows, {
  // Free console: open a world from Practicar, type in Cloud Shell, save it,
  // and continue it later. No hand-in, no teacher, nothing in the progress.
  async freeConsole(browser) {
    const tok = await account("free");
    const { ctx, p, errors } = await page(browser, tok);
    p.on("dialog", (d) => d.accept());
    await p.goto(base + "/catalog", { waitUntil: "networkidle" });
    await p.locator(".cm-tabs a", { hasText: "Consola libre" }).click();
    await p.waitForURL("**/sandbox");
    await p.getByRole("button", { name: "Abrir «Proyecto nuevo»" }).click();
    await p.waitForURL(/\/lab\?session=.*id=sandbox-default-network/, { timeout: 30000 });
    await skipTour(p);
    const sid = new URL(p.url()).searchParams.get("session");
    const tabs = await p.getByRole("tab").allInnerTexts();
    check("free console: only its own panel (no hand-in, no teacher)", tabs.join("|") === "Consola libre", tabs.join("|"));
    // type in Cloud Shell (xterm)
    await p.locator(".xterm-rows", { hasText: "student@cloudshell" }).waitFor({ timeout: 15000 });
    await p.locator(".xterm-screen").first().click();
    await p.keyboard.type("gcloud compute instances create juguete-1 --zone=europe-west1-b");
    await p.keyboard.press("Enter");
    await p.locator(".xterm-rows", { hasText: "juguete-1" }).last().waitFor({ timeout: 15000 }).catch(() => {});
    let list = { output: "" };
    for (let i = 0; i < 10 && !list.output.includes("juguete-1"); i++) {
      list = await (await exec(tok, sid, "gcloud compute instances list")).json();
      if (!list.output.includes("juguete-1")) await p.waitForTimeout(500);
    }
    check("free console: a command typed in Cloud Shell changes the project", list.output.includes("juguete-1"), list.output.split("\n")[1] ?? "");
    await p.getByRole("button", { name: "Guardar este mundo" }).click();
    await p.waitForSelector("text=Guardado a las");
    check("free console: the world is saved", true);
    await p.goto(base + "/sandbox", { waitUntil: "networkidle" });
    check("free console: the saved world and the open one are offered", (await p.getByText("Mundo guardado").count()) === 1 && (await p.getByText("Abierta ahora").count()) === 1);
    await p.getByRole("button", { name: "Continuar donde lo dejé" }).click();
    await p.waitForURL(/\/lab\?session=/, { timeout: 30000 });
    const sid2 = new URL(p.url()).searchParams.get("session");
    const again = await (await exec(tok, sid2, "gcloud compute instances list")).json();
    check("free console: continuing brings the saved world back", sid2 !== sid && again.output.includes("juguete-1"));
    const atts = (await (await call(tok, "/api/me/attempts")).json()) ?? [];
    check("free console: nothing counts towards progress", atts.length === 0, `${atts.length} attempts`);
    check("free console: no page errors", errors.length === 0, errors.join("; "));
    await ctx.close();
  },

  // Platform requests (week 10): find a project by its number in the picker,
  // switch to it (Cloud Shell follows) and add a domain to its reCAPTCHA key
  // from the console, keeping the domains it already had.
  async projectPickerRecaptcha(browser) {
    const tok = await account("pick");
    const s = await (await call(tok, "/api/labs/zentra-p02-recaptcha-domain/start", { mode: "assisted" })).json();
    const { ctx, p, errors } = await page(browser, tok);
    await p.goto(`${base}/lab?session=${s.id}&id=zentra-p02-recaptcha-domain`, { waitUntil: "networkidle" });
    await skipTour(p);
    await p.locator(".cc-project").click();
    const pop = p.getByRole("dialog", { name: "Seleccionar un proyecto" });
    const rows = await pop.locator("tbody tr").count();
    check("picker: lists the DEV, AU and PRO projects", rows >= 12, `${rows} projects`);
    await pop.getByLabel("Buscar proyectos").fill("318204657913");
    const found = await pop.locator("tbody tr").allInnerTexts();
    check("picker: searching by number finds the one project", found.length === 1 && found[0].includes("dev-zentra-altas"), found.join(" | "));
    await pop.locator("tbody tr button.cc-link").click();
    await p.locator(".cc-project", { hasText: "dev-zentra-altas" }).waitFor({ timeout: 15000 });
    const cfg = await (await exec(tok, s.id, "gcloud config get-value project")).json();
    check("picker: Cloud Shell follows the chosen project", cfg.output.trim() === "dev-zentra-altas", cfg.output.trim());
    // Menu › reCAPTCHA › the key › Añadir un dominio
    await p.getByRole("button", { name: "Menú de navegación" }).click();
    await p.locator(".cc-menu-item", { hasText: "reCAPTCHA" }).click();
    const row = p.locator(".cc-main tbody tr", { hasText: "6LdZtR8qAAAAAJx3wPm7vK2nYq5cT9bH4sLfE1uA" });
    await row.waitFor({ timeout: 15000 });
    check("recaptcha: the DEV key is listed in the DEV project", (await row.count()) === 1);
    await row.locator("button, a").first().click();
    await p.getByRole("button", { name: "Añadir un dominio" }).click();
    await p.getByRole("textbox", { name: /^Dominio/ }).fill("dev-zentra-docs.s3.amazonaws.com");
    await p.getByRole("button", { name: "Guardar", exact: true }).click();
    let d = { output: "" };
    for (let i = 0; i < 10 && !d.output.includes("dev-zentra-docs"); i++) {
      d = await (await exec(tok, s.id, "gcloud recaptcha keys describe 6LdZtR8qAAAAAJx3wPm7vK2nYq5cT9bH4sLfE1uA --format='value(webSettings.allowedDomains)'")).json();
      if (!d.output.includes("dev-zentra-docs")) await p.waitForTimeout(500);
    }
    check("recaptcha: the domain is added and the old ones kept", ["dev-zentra-docs.s3.amazonaws.com", "altas.dev.zentra.dev", "localhost"].every((x) => d.output.includes(x)), d.output.trim());
    await p.reload({ waitUntil: "networkidle" });
    const shell = p.locator(".xterm-rows", { hasText: "(dev-zentra-altas)$" });
    check("picker: after a reload Cloud Shell still shows the chosen project", await shell.waitFor({ timeout: 15000 }).then(() => true, () => false));
    check("recaptcha: no page errors", errors.length === 0, errors.join("; "));
    await ctx.close();
  },
});

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
