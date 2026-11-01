// The hosted shape of the page, locally (spec 012 FR-019, T014): the page served under the example's base path
// with the backend on another origin, as a Pages project site and a Cloud Run service would be. The identity
// here is the development one; the same shape with the sign-in and the production server is signin.spec.mjs, and
// the hosted journey itself is T017's.
import { readFileSync } from "node:fs";

import { test as base, expect } from "@playwright/test";

import { serve } from "../serve/serve.mjs";
import { startStack } from "./stack.mjs";

// The base path the hosted example declares; the page must work under it with no root-absolute request.
const { frontend } = JSON.parse(readFileSync(process.env.EXAMPLE_INPUTS, "utf8"));
const basePath = frontend.base_path;

const test = base.extend({
  stack: [
    async ({}, use) => {
      const stack = await startStack(basePath);
      await use(stack);
      await stack.stop();
    },
    { scope: "worker" },
  ],
  notes: async ({ page, stack }, use) => {
    await page.goto(stack.origin + basePath);
    await expect(page.getByTestId("test-identity")).toBeVisible();
    await use(page);
  },
});

const identify = (page, subject) => page.getByLabel("Test identity").fill(subject);

async function createNote(page, title, body = "") {
  await page.getByLabel("Title").fill(title);
  await page.getByLabel("Body").fill(body);
  await page.getByRole("button", { name: "Create note" }).click();
}

test("the page loads under the base path and reads its configuration relative to it", async ({
  page,
  stack,
}) => {
  const requests = [];
  page.on("request", (request) => requests.push(new URL(request.url()).pathname));
  await page.goto(stack.origin + basePath);
  await expect(page.getByRole("heading", { name: "Notes" })).toBeVisible();
  expect(requests).toContain(`${basePath}config.json`);
  expect(requests).toContain(`${basePath}main.js`);
  expect(requests.filter((p) => !p.startsWith(basePath))).toEqual([]);
  // Nothing is served at the root: the base path is the site.
  const root = await page.request.get(`${stack.origin}/config.json`);
  expect(root.status()).toBe(404);
});

test("the preflight passes for the page's origin and the call reaches the backend", async ({
  notes,
  stack,
}) => {
  // The browser's own preflight is not visible to the page, but a cross-origin POST with the client's headers
  // succeeds only if it passed; the preflight contract itself is checked with an explicit request.
  await identify(notes, "ana");
  const [call] = await Promise.all([
    notes.waitForResponse(
      (r) => r.request().method() === "POST" && r.url().endsWith("/CreateNote"),
    ),
    createNote(notes, "across origins", "from the base path"),
  ]);
  expect(call.status()).toBe(200);
  expect(new URL(call.url()).origin).toBe(stack.bffUrl);
  expect((await call.allHeaders())["access-control-allow-origin"]).toBe(stack.origin);
  await expect(notes.getByTestId("note-body")).toHaveText("from the base path");
  const preflight = await notes.request.fetch(
    `${stack.bffUrl}/template.notes.v1.NoteService/CreateNote`,
    {
      method: "OPTIONS",
      headers: {
        Origin: stack.origin,
        "Access-Control-Request-Method": "POST",
        "Access-Control-Request-Headers":
          "authorization,content-type,connect-protocol-version,connect-timeout-ms",
      },
    },
  );
  expect(preflight.status()).toBe(204);
  expect(preflight.headers()["access-control-allow-origin"]).toBe(stack.origin);
  expect(preflight.headers()["access-control-allow-headers"]).toContain("Connect-Timeout-Ms");
  const foreign = await notes.request.fetch(
    `${stack.bffUrl}/template.notes.v1.NoteService/CreateNote`,
    {
      method: "OPTIONS",
      headers: { Origin: "https://elsewhere.example", "Access-Control-Request-Method": "POST" },
    },
  );
  expect(foreign.headers()["access-control-allow-origin"]).toBeUndefined();
});

test("a foreign origin under the same base path is blocked", async ({ page, stack }) => {
  const other = await serve(process.env.WEB_DIST, 0, basePath);
  other.setBffUrl(stack.bffUrl);
  await page.goto(other.origin + basePath);
  await createNote(page, "a");
  await expect(page.getByTestId("failure")).toHaveText("The service is unavailable. Try again.");
  await other.close();
});

test("errors and status are announced", async ({ notes }) => {
  await createNote(notes, "   ");
  await expect(notes.getByRole("alert")).toHaveText("title is required");
  await identify(notes, "fault-hang");
  await createNote(notes, "slow");
  await expect(notes.getByRole("status").filter({ hasText: "Working" })).toBeVisible();
  await expect(notes.getByRole("alert")).toHaveText("The request took too long. Try again.", {
    timeout: 5000,
  });
});

test("a reload under the base path keeps working", async ({ notes, stack }) => {
  await identify(notes, "ana");
  await createNote(notes, "before the reload");
  await expect(notes.getByRole("heading", { name: "before the reload" })).toBeVisible();
  await notes.reload();
  expect(new URL(notes.url()).pathname).toBe(basePath);
  await expect(notes.getByTestId("test-identity")).toBeVisible();
  await identify(notes, "ana");
  await createNote(notes, "after the reload");
  await expect(notes.getByRole("heading", { name: "after the reload" })).toBeVisible();
  expect(stack.origin).not.toBe(stack.bffUrl);
});

test("the backend's log carries no credential and no subject", async ({ notes, stack }) => {
  await identify(notes, "fault-secret");
  await createNote(notes, "a");
  await expect(notes.getByTestId("failure")).toHaveText(
    /^Something went wrong\. Reference: [0-9a-f]+$/,
  );
  const log = stack.log();
  expect(log).toContain("internal error");
  expect(log).not.toContain("Bearer ");
  expect(log).not.toContain("test-identity.");
  expect(log).not.toMatch(/"subject"|"owner"/);
});
