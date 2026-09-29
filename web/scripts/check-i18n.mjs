// check-i18n extracts every Spanish literal passed to t("…"), tr("…"),
// k("…") or translate(lang, "…") and fails when lib/en.ts has no English
// entry for it. `--missing` prints the missing keys as JSON.
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

const root = new URL("..", import.meta.url).pathname;
const files = [];
const walk = (d) => {
  for (const f of readdirSync(d)) {
    const p = join(d, f);
    if (statSync(p).isDirectory()) {
      if (!["node_modules", "out", ".next", "e2e"].includes(f)) walk(p);
    } else if (/\.(tsx?)$/.test(f) && !p.endsWith("lib/en.ts")) files.push(p);
  }
};
for (const d of ["app", "components", "lib"]) walk(join(root, d));

const lit = String.raw`"((?:[^"\\]|\\.)*)"`;
const re = new RegExp(String.raw`(?:\b(?:t|tr|k)\(\s*|translate\([^,]+?,\s*)` + lit, "gs");
const keys = new Map();
for (const f of files) {
  const src = readFileSync(f, "utf8");
  for (const m of src.matchAll(re)) {
    const key = JSON.parse(`"${m[1]}"`);
    if (!keys.has(key)) keys.set(key, f.slice(root.length));
  }
}

const enSrc = readFileSync(join(root, "lib/en.ts"), "utf8");
const have = new Set();
for (const m of enSrc.matchAll(new RegExp(String.raw`^\s*` + lit + String.raw`\s*:`, "gm"))) have.add(JSON.parse(`"${m[1]}"`));

const missing = [...keys.keys()].filter((k) => !have.has(k));
const unused = [...have].filter((k) => !keys.has(k));
if (process.argv.includes("--missing")) {
  console.log(JSON.stringify(missing, null, 1));
  process.exit(0);
}
for (const k of unused) console.warn(`warning: unused English entry: ${JSON.stringify(k)}`);
if (missing.length) {
  for (const k of missing) console.error(`missing English translation (${keys.get(k)}): ${JSON.stringify(k)}`);
  process.exit(1);
}
console.log(`i18n: ${keys.size} strings, all translated`);
