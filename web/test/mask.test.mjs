import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

import { mask } from "../mask.js";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "..", "..");
const fixtures = JSON.parse(readFileSync(path.join(root, "engine", "testdata", "pii.json"), "utf8"));

describe("pii: web/mask.js passes engine/testdata/pii.json", () => {
  assert.ok(fixtures.length > 0, "pii.json empty");
  fixtures.forEach((item, i) => {
    it(`pii[${i}] ${JSON.stringify(item.input).slice(0, 60)}`, () => {
      const { masked } = mask(item.input);
      for (const s of item.mustNotContain || []) {
        if (s) assert.ok(!masked.includes(s), `output still contains ${JSON.stringify(s)} -> ${JSON.stringify(masked)}`);
      }
      for (const s of item.mustContain || []) {
        assert.ok(masked.includes(s), `output missing ${JSON.stringify(s)} -> ${JSON.stringify(masked)}`);
      }
    });
  });
});
