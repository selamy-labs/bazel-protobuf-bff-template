// Runs a package's compiled tests in one process through Node's own runner, and reports coverage when Bazel asks
// for it. Lines are of the compiled JavaScript: a source map would need the `.ts` files in runfiles (spec 007 R7).
// rules_js's report keeps only `.ts` entries, and V8 reports the compiled `.js`. Each package's entry point names
// its files and the glob of what it measures.
import { createWriteStream } from "node:fs";
import { run } from "node:test";
import { lcov, spec } from "node:test/reporters";

export function runTests(files, coverageIncludeGlob) {
  const output = process.env.COVERAGE_OUTPUT_FILE;
  const stream = run({
    files,
    isolation: "none",
    coverage: Boolean(output),
    coverageIncludeGlobs: [coverageIncludeGlob],
    coverageExcludeGlobs: ["**/*.test.js"],
  });
  stream.on("test:fail", () => {
    process.exitCode = 1;
  });
  stream.compose(spec).pipe(process.stdout);
  if (output) {
    stream.compose(lcov).pipe(createWriteStream(output));
  }
}
