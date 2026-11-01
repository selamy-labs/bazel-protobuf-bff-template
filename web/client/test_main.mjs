// This package's tests, through the shared runner (web/testing).
import { runTests } from "../testing/node_tests.mjs";

const here = new URL(".", import.meta.url).pathname;
runTests(
  [
    `${here}errors.property.test.js`,
    `${here}errors.test.js`,
    `${here}lifecycle.test.js`,
    `${here}transport.test.js`,
  ],
  "**/web/client/*.js",
);
