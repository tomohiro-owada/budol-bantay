import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

import { analyze } from "../engine.js";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "..", "..");
const rules = JSON.parse(readFileSync(path.join(root, "engine", "rules.json"), "utf8"));
const corpus = JSON.parse(readFileSync(path.join(root, "engine", "testdata", "corpus.json"), "utf8"));
const golden = JSON.parse(readFileSync(path.join(root, "engine", "testdata", "golden.json"), "utf8"));
const byId = new Map(golden.map((g) => [g.id, g]));

describe("parity: web/engine.js matches Go engine (golden.json) and corpus levels", () => {
  assert.ok(corpus.length >= 22, `corpus must have >=22 items, got ${corpus.length}`);
  assert.equal(golden.length, corpus.length, "golden.json out of date: re-run go test ./engine/ -run TestGolden -update");

  for (const item of corpus) {
    it(`${item.id}`, () => {
      const r = analyze(item.text, item.sender, rules);
      // same level as corpus.json
      assert.equal(r.level, item.level, `level mismatch (score=${r.score} hits=${r.hits.map((h) => h.id)})`);
      // same score and hit ids as Go engine (golden.json)
      const g = byId.get(item.id);
      assert.ok(g, `golden.json missing ${item.id}`);
      assert.equal(r.score, g.score, `score drift vs Go (hits=${r.hits.map((h) => h.id)})`);
      const got = [...r.hits.map((h) => h.id)].sort();
      const want = [...g.hits].sort();
      assert.deepEqual(got, want, `hit ids drift vs Go`);
      assert.equal(r.level, g.level, `level drift vs Go`);
    });
  }
});
