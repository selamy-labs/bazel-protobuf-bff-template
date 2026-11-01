// The ESLint policy (spec 006 FR-017). Inline configuration is off, so a directive in a comment
// cannot change a result, and the suppression ban rejects it as well.
import tseslint from "typescript-eslint";

export default tseslint.config(
  { linterOptions: { noInlineConfig: true, reportUnusedDisableDirectives: "off" } },
  ...tseslint.configs.recommended,
);
