// The ESLint policy (spec 006 FR-017). Inline configuration is off, so a directive in a comment
// cannot change a result, and the suppression ban rejects it as well.
import { readFileSync } from "node:fs";

import tseslint from "typescript-eslint";

// The cyclomatic ceiling (spec 011 FR-004, FR-005) is the complexity budgets' `typescript` section, one policy
// input beside the Go ceilings, so the base's comparator judges its direction (`complexity-from-base`): it only
// ever falls. Bazel names the file; outside Bazel it is the repository's own.
const budgets =
  process.env.COMPLEXITY_BUDGETS ??
  new URL("../../quality/complexity_budgets.json", import.meta.url);
export const ceiling = JSON.parse(readFileSync(budgets, "utf8")).typescript.ceilings.cyclomatic;

export default tseslint.config(
  { linterOptions: { noInlineConfig: true, reportUnusedDisableDirectives: "off" } },
  ...tseslint.configs.recommended,
  { rules: { complexity: ["error", ceiling] } },
);
