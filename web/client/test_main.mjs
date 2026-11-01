// Runs this package's tests in one process, and reports coverage when Bazel asks for it. Lines are of
// the compiled JavaScript. A source map would need the `.ts` files in runfiles (spec 007 R7).
import { createWriteStream } from "node:fs";
import { run } from "node:test";
import { lcov, spec } from "node:test/reporters";

const here = new URL(".", import.meta.url).pathname;
const output = process.env.COVERAGE_OUTPUT_FILE;
const stream = run({
  files: [
    `${here}errors.property.test.js`,
    `${here}errors.test.js`,
    `${here}lifecycle.test.js`,
    `${here}transport.test.js`,
  ],
  isolation: "none",
  coverage: Boolean(output),
  coverageIncludeGlobs: ["**/web/client/*.js"],
  coverageExcludeGlobs: ["**/*.test.js"],
});
stream.on("test:fail", () => {
  process.exitCode = 1;
});
stream.compose(spec).pipe(process.stdout);
if (output) {
  stream.compose(lcov).pipe(createWriteStream(output));
}
