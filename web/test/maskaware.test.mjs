import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

import { analyze } from "../engine.js";
import { mask } from "../mask.js";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "..", "..");
const rules = JSON.parse(readFileSync(path.join(root, "engine", "rules.json"), "utf8"));

const has = (r, id) => r.hits.some((h) => h.id === id);

describe("mask-aware: [PHONE]/[CODE] behave like raw PII", () => {
  it("personalAcct fires on [PHONE]", () => {
    const r = analyze("GCash: pakisend sa GCash [PHONE] ngayon", "", rules);
    assert.ok(has(r, "personalAcct"), `hits=${r.hits.map((h) => h.id)}`);
  });

  it("sendMoney fires on [PHONE]", () => {
    const r = analyze("Pa-send muna sa GCash [PHONE] emergency", "", rules);
    assert.ok(has(r, "sendMoney"), `hits=${r.hits.map((h) => h.id)}`);
  });

  it("senderMobile fires on [PHONE] sender", () => {
    const r = analyze("GCash: i-verify ang account mo dito", "[PHONE]", rules);
    assert.ok(has(r, "senderMobile"), `hits=${r.hits.map((h) => h.id)}`);
  });

  it("credential fires on [CODE]", () => {
    const r = analyze("paki-send ang [CODE] ngayon din", "", rules);
    assert.ok(has(r, "credential"), `hits=${r.hits.map((h) => h.id)}`);
  });

  it("end-to-end: mask then analyze keeps money-trail rules", () => {
    const raw = "CONGRATULATIONS! Nanalo ka ng premyo. Para ma-claim, magpadala ng processing fee sa GCash 09171234567.";
    const { masked } = mask(raw);
    assert.ok(!masked.includes("09171234567"), `not redacted: ${masked}`);
    assert.ok(masked.includes("[PHONE]"), `missing token: ${masked}`);
    const r = analyze(masked, "", rules);
    assert.ok(has(r, "personalAcct"), `hits=${r.hits.map((h) => h.id)} masked=${masked}`);
    assert.ok(has(r, "sendMoney"), `hits=${r.hits.map((h) => h.id)} masked=${masked}`);
  });
});
