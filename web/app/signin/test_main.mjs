// This package's tests, through the shared runner (web/testing).
import { runTests } from "../../testing/node_tests.mjs";

const here = new URL(".", import.meta.url).pathname;
runTests([`${here}oidc.test.js`], "**/web/app/signin/*.js");
