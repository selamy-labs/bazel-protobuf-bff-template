import { browserLaunchOptions } from "./stack.mjs";

const outputs = process.env.TEST_UNDECLARED_OUTPUTS_DIR ?? "test-results";

export default {
  forbidOnly: true,
  fullyParallel: true,
  outputDir: `${outputs}/playwright`,
  reporter: [
    ["list"],
    ["json", { outputFile: process.env.E2E_REPORT ?? `${outputs}/report.json` }],
  ],
  retries: 0,
  testDir: ".",
  testMatch: "*.spec.mjs",
  timeout: 15000,
  use: {
    browserName: "chromium",
    launchOptions: browserLaunchOptions(),
    screenshot: "only-on-failure",
  },
  workers: 2,
};
