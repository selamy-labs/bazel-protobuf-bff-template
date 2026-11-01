// The ESLint policy (spec 006 FR-017). Inline configuration is off, so a directive in a comment
// cannot change a result, and the suppression ban rejects it as well.
import tseslint from "typescript-eslint";

export default tseslint.config(
  { linterOptions: { noInlineConfig: true, reportUnusedDisableDirectives: "off" } },
  ...tseslint.configs.recommended,
  {
    // The cyclomatic ceiling (spec 011 FR-004, FR-005): the measured maximum over the first-party TypeScript and
    // JavaScript at a287ed7 (51 functions in 21 files; a browser journey at 12, the error mapping at 11), set
    // from that measurement and only ever lowered. The witness in complexity.test.mjs plants one over it.
    rules: { complexity: ["error", 12] },
  },
);
