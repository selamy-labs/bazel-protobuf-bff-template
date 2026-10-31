// The pinned browser starts with only pinned libraries, and runs a script.
import assert from "node:assert/strict";

import { chromium } from "@playwright/test";

import { browserLaunchOptions } from "./stack.mjs";

const browser = await chromium.launch(browserLaunchOptions());
assert.equal(browser.version(), "156.0.8078.4");
const page = await browser.newPage();
await page.setContent('<p id="a">before</p>');
await page.evaluate(() => {
  document.getElementById("a").textContent = "after";
});
assert.equal(await page.textContent("#a"), "after");
await browser.close();
console.log("browser smoke passed");
