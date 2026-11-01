// The TypeScript complexity budget's planted control (spec 011 FR-002, FR-005): the pinned ESLint with the
// repository's own configuration refuses a function one over the ceiling and accepts one at it, and the ceiling
// is the measured maximum, which only falls.
import assert from "node:assert/strict";
import { test } from "node:test";
import { ESLint } from "eslint";

import config from "./eslint.config.mjs";

// The measured maximum cyclomatic complexity over the first-party TypeScript and JavaScript at a287ed7: 51
// functions in 21 files, the largest a browser journey at 12 and the error mapping at 11.
const ceiling = 12;

const eslint = new ESLint({ overrideConfigFile: true, overrideConfig: config });
const complexityReports = async (text) =>
  (await eslint.lintText(text, { filePath: "planted.ts" }))[0].messages.filter(
    (m) => m.ruleId === "complexity",
  );

// A function with n independent paths: n - 1 `if` statements in a row.
const withComplexity = (n) =>
  `export function planted(x: number): number {\n${Array.from(
    { length: n - 1 },
    (_, i) => `  if (x === ${i}) {\n    x += ${i};\n  }\n`,
  ).join("")}  return x;\n}\n`;

test("the configuration carries the measured ceiling", () => {
  const rule = config
    .flatMap((c) => Object.entries(c.rules ?? {}))
    .find(([id]) => id === "complexity");
  assert.deepEqual(rule?.[1], ["error", ceiling]);
});

test("a function one over the ceiling is refused, naming its complexity", async () => {
  const reports = await complexityReports(withComplexity(ceiling + 1));
  assert.equal(reports.length, 1);
  assert.match(
    reports[0].message,
    new RegExp(`complexity of ${ceiling + 1}\\. Maximum allowed is ${ceiling}`),
  );
});

test("a function at the ceiling passes, and a trivial one reports nothing", async () => {
  assert.equal((await complexityReports(withComplexity(ceiling))).length, 0);
  assert.equal((await complexityReports(withComplexity(1))).length, 0);
});
