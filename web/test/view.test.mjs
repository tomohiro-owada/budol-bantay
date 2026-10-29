import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { isSafeBand, guidance, SAFE_ADVICE, SAFE_HEAD, REPORT_COMPACT } from "../view.js";

const SCAM_WORDS = /i-report|cicc|huwag i-click|do not click|report|通報|クリック/i;

test("only the lowest band is safe", () => {
  assert.equal(isSafeBand("none"), true);
  for (const b of ["lo", "mid", "hi"]) assert.equal(isSafeBand(b), false);
});

test("safe band gets neutral guidance in every language, never scam advice", () => {
  for (const lang of ["tl", "en", "ja"]) {
    const g = guidance({ band: "none", lang, aiActions: ["Huwag i-click ang link"], rulesAdvice: ["I-report: CICC Hotline 1326"] });
    assert.equal(g.safe, true);
    assert.equal(g.head, SAFE_HEAD[lang]);
    assert.deepEqual(g.items, SAFE_ADVICE[lang]);
    assert.ok(g.items.length >= 3);
    for (const it of g.items) assert.ok(!SCAM_WORDS.test(it), `scam wording in safe advice (${lang}): ${it}`);
  }
  assert.match(SAFE_HEAD.tl, /Mukhang ligtas, pero mag-ingat pa rin/);
});

test("risky bands use AI steps when present, else rules advice", () => {
  const withAI = guidance({ band: "hi", lang: "tl", aiActions: ["a", "b"], rulesAdvice: ["r"] });
  assert.equal(withAI.safe, false);
  assert.deepEqual(withAI.items, ["a", "b"]);
  const noAI = guidance({ band: "mid", lang: "en", aiActions: [], rulesAdvice: ["r"] });
  assert.deepEqual(noAI.items, ["r"]);
});

test("compact report line still names CICC 1326 and the ACG link slot", () => {
  for (const lang of ["tl", "en", "ja"]) {
    assert.match(REPORT_COMPACT[lang], /CICC 1326/);
    assert.match(REPORT_COMPACT[lang], /\{acg\}/);
  }
});

test("app.js: one merged AI card with <details>, disclaimer + report line outside it", () => {
  const src = readFileSync(new URL("../app.js", import.meta.url), "utf8");
  const a = src.indexOf("function renderAIDone");
  const body = src.slice(a, src.indexOf("async function runAI"));
  assert.ok(body.includes('$("#result").innerHTML = ""'), "rules card must be dropped when AI result is shown");
  const dIdx = body.indexOf('<details class="more">');
  assert.ok(dIdx > 0, "details section missing");
  assert.ok(body.indexOf('class="resultdisc"') > 0 && body.indexOf('class="resultdisc"') < dIdx, "disclaimer must be visible (before <details>)");
  assert.ok(body.indexOf('class="reportline"') > 0 && body.indexOf('class="reportline"') < dIdx, "report line must be visible (before <details>)");
  assert.ok(body.indexOf("sentTitle") > dIdx && body.indexOf("extraRules") > dIdx, "rule hits + sent text belong inside <details>");
  assert.equal((body.match(/id="dl"/g) || []).length, 1, "single share button");
});
