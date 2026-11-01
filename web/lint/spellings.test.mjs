// Re-witnesses the suppression ban's ESLint spellings against the pinned ESLint (spec 006 FR-017).
// A spelling earns its place in the ban only if it really suppresses: each fixture must report
// fewer problems with its directive than the same code without it. Prose must suppress nothing.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { ESLint } from "eslint";
import tseslint from "typescript-eslint";

const fixtures = process.env.SPELLINGS.split(" ");
const rules = { "no-console": "error", "no-debugger": "error", "no-undef": "error" };
const eslint = new ESLint({
  overrideConfigFile: true,
  overrideConfig: [{ files: ["**/*.ts"], languageOptions: { parser: tseslint.parser }, rules }],
});
const problems = async (text) =>
  (await eslint.lintText(text, { filePath: "fixture.ts" }))[0].messages.filter((m) => m.ruleId);
const withoutComments = (text) => text.replace(/\/\*[\s\S]*?\*\/|\/\/.*/g, "");

const byName = new Map(fixtures.map((file) => [path.basename(file), file]));
const names = [...byName.keys()];
const read = (name) => readFileSync(byName.get(name), "utf8");

test("the fixture set is present", () => {
  assert.ok(names.filter((n) => n.includes("_bad")).length >= 9, names.join());
});

// ESLint 10 dropped this directive. The ban keeps it: a dead comment that older tools still honor
// has no place here. This test fails if a later ESLint honors it again.
const dead = new Set(["S002_bad_env.ts"]);

for (const name of names.filter((n) => n.includes("_bad"))) {
  const claim = dead.has(name) ? "has no effect" : "really suppresses";
  test(`${name} ${claim} under the pinned ESLint`, async () => {
    const text = read(name);
    const control = await problems(withoutComments(text));
    assert.ok(control.length > 0, "the control reports nothing, so it proves nothing");
    const found = (await problems(text)).length;
    if (dead.has(name)) assert.equal(found, control.length);
    else assert.ok(found < control.length);
  });
}

for (const name of names.filter((n) => n.includes("_good"))) {
  test(`${name} suppresses nothing`, async () => {
    const text = read(name);
    assert.equal((await problems(text)).length, (await problems(withoutComments(text))).length);
  });
}
