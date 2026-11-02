// The hosted journey with the sign-in, locally (spec 012 FR-016, FR-021, T014): the production bundle under the
// example's base path, the production server with its verifier pointed at a local issuer, and the page signing in
// through the issuer's own page. What is proven here is the protocol between the page, the issuer and the server;
// the live site against a real issuer is T017's.
import { readFileSync } from "node:fs";

import { test as base, expect } from "@playwright/test";

import { startProductionStack } from "./stack.mjs";

const { frontend } = JSON.parse(readFileSync(process.env.EXAMPLE_INPUTS, "utf8"));
const basePath = frontend.base_path;
const refused = "Authentication is required.";

const test = base.extend({
  stack: [
    async ({}, use) => {
      const stack = await startProductionStack(basePath);
      await use(stack);
      await stack.stop();
    },
    { scope: "worker" },
  ],
  notes: async ({ page, stack }, use) => {
    stack.configureSignIn({
      issuer: stack.issuer.origin,
      clientId: "bff-hosted-example",
    });
    await page.goto(stack.origin + basePath);
    await expect(page.getByTestId("sign-in")).toBeVisible();
    await use(page);
  },
});

async function createNote(page, title, body = "") {
  await page.getByLabel("Title").fill(title);
  await page.getByLabel("Body").fill(body);
  await page.getByRole("button", { name: "Create note" }).click();
}

// Through the issuer's page and back: the browser leaves the site and returns to the page's own address.
async function signIn(page, stack, subject = "ana") {
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL((url) => url.origin === stack.issuer.origin);
  await page.getByLabel("Continue as").fill(subject);
  await page.getByRole("button", { name: "Continue" }).click();
  await page.waitForURL((url) => url.origin === stack.origin);
  await expect(page.getByTestId("signed-in")).toHaveText(`Signed in as ${subject}`);
}

const rpc = (stack, method) => `${stack.bffUrl}/template.notes.v1.NoteService/${method}`;

test("the production page offers the sign-in and carries no test identity", async ({ notes }) => {
  await expect(notes.getByRole("status")).toHaveText("Not signed in.");
  await expect(notes.getByRole("button", { name: "Sign in" })).toBeVisible();
  await expect(notes.getByTestId("test-identity")).toHaveCount(0);
  await expect(notes.getByText("Sign-in is not configured")).toHaveCount(0);
});

test("before the sign-in, a call is refused by the server", async ({ notes }) => {
  await createNote(notes, "too early");
  await expect(notes.getByRole("alert")).toHaveText(refused);
});

test("the sign-in yields a verified identity, and the calls carry it", async ({ notes, stack }) => {
  await signIn(notes, stack, "ana");
  expect(new URL(notes.url()).pathname).toBe(basePath);
  expect(new URL(notes.url()).search).toBe("");
  const [request] = await Promise.all([
    notes.waitForRequest((r) => r.url().endsWith("/CreateNote")),
    createNote(notes, "verified", "by the issuer's key"),
  ]);
  const authorization = request.headers()["authorization"];
  expect(authorization).toBe(`Bearer ${stack.issuer.issued.at(-1)}`);
  expect((await request.response()).status()).toBe(200);
  await expect(notes.getByTestId("note-body")).toHaveText("by the issuer's key");
  const name = await notes.getByTestId("note-name").textContent();
  await notes.getByLabel("Name").fill(name);
  await notes.getByRole("button", { name: "Open note" }).click();
  await expect(notes.getByRole("heading", { name: "verified" })).toBeVisible();
  // Another verified identity does not see it: the owner is the token's subject.
  await notes.getByRole("button", { name: "Sign out" }).click();
  await signIn(notes, stack, "bob");
  await notes.getByLabel("Name").fill(name);
  await notes.getByRole("button", { name: "Open note" }).click();
  await expect(notes.getByRole("alert")).toHaveText("Not found.");
});

test("a reload keeps the session in this tab, and signing out forgets it", async ({
  notes,
  stack,
}) => {
  await signIn(notes, stack, "ana");
  await notes.reload();
  await expect(notes.getByTestId("signed-in")).toHaveText("Signed in as ana");
  await notes.getByRole("button", { name: "Sign out" }).click();
  await expect(notes.getByRole("status")).toHaveText("Not signed in.");
  const [request] = await Promise.all([
    notes.waitForRequest((r) => r.url().endsWith("/CreateNote")),
    createNote(notes, "after the sign-out"),
  ]);
  expect(request.headers()["authorization"]).toBeUndefined();
  await expect(notes.getByRole("alert")).toHaveText(refused);
  await notes.reload();
  await expect(notes.getByRole("status")).toHaveText("Not signed in.");
});

test("a declined sign-in is reported, and no credential is kept", async ({ notes, stack }) => {
  await notes.getByRole("button", { name: "Sign in" }).click();
  await notes.waitForURL((url) => url.origin === stack.issuer.origin);
  await notes.getByRole("button", { name: "Decline" }).click();
  await notes.waitForURL((url) => url.origin === stack.origin);
  await expect(notes.getByRole("alert")).toHaveText("Sign-in was declined.");
  expect(new URL(notes.url()).search).toBe("");
  await expect(notes.getByRole("button", { name: "Sign in" })).toBeVisible();
  await createNote(notes, "still refused");
  await expect(notes.getByRole("alert").last()).toHaveText(refused);
});

test("a deep link carrying another tab's answer is refused", async ({ page, stack }) => {
  await page.goto(`${stack.origin}${basePath}?code=stolen&state=elsewhere`);
  await expect(page.getByRole("alert")).toHaveText(
    "This sign-in did not start in this tab. Try again.",
  );
  expect(new URL(page.url()).search).toBe("");
  expect(new URL(page.url()).pathname).toBe(basePath);
});

test("an ended session is forgotten, and the page says so", async ({ notes, stack }) => {
  stack.issuer.ttlSeconds = 1;
  try {
    await signIn(notes, stack, "ana");
  } finally {
    stack.issuer.ttlSeconds = 3600;
  }
  await notes.waitForTimeout(1500);
  await createNote(notes, "too late");
  await expect(notes.getByRole("alert")).toHaveText(refused);
  await expect(notes.getByRole("status")).toHaveText("Not signed in.");
});

test("the server refuses the development identity and a token for another audience", async ({
  stack,
  request,
}) => {
  const call = (authorization) =>
    request.post(rpc(stack, "CreateNote"), {
      headers: {
        Authorization: authorization,
        "Content-Type": "application/json",
      },
      data: { note: { title: "refused" } },
    });
  const development = await call("Bearer test-identity.ana");
  expect(development.status()).toBe(401);
  expect((await development.json()).code).toBe("unauthenticated");
  const elsewhere = await call(`Bearer ${stack.issuer.mint({ sub: "ana", aud: "someone-else" })}`);
  expect(elsewhere.status()).toBe(401);
  const unsigned = await call(`Bearer ${stack.issuer.issued.at(-1)}x`);
  expect(unsigned.status()).toBe(401);
  const verified = await call(`Bearer ${stack.issuer.mint({ sub: "ana" })}`);
  expect(verified.status()).toBe(200);
});

test("the backend's log carries no token and no subject", async ({ notes, stack }) => {
  await signIn(notes, stack, "ana");
  await createNote(notes, "logged");
  await expect(notes.getByTestId("note")).toBeVisible();
  const log = stack.log();
  expect(log).not.toContain("Bearer ");
  for (const token of stack.issuer.issued) {
    expect(log).not.toContain(token.split(".")[2]);
  }
  expect(log).not.toMatch(/"subject"|"owner"/);
});

test("removing sign-in configuration prevents reuse of a stored credential", async ({
  notes,
  stack,
}) => {
  await signIn(notes, stack, "ana");
  stack.configureSignIn(undefined);
  await notes.reload();
  await expect(notes.getByText("Sign-in is not configured in this build.")).toBeVisible();
  const [request] = await Promise.all([
    notes.waitForRequest((r) => r.url().endsWith("/CreateNote")),
    createNote(notes, "no old credential"),
  ]);
  expect(request.headers()["authorization"]).toBeUndefined();
  await expect(notes.getByRole("alert")).toHaveText(refused);
});

test("changing the client invalidates an earlier session", async ({ notes, stack }) => {
  await signIn(notes, stack, "ana");
  stack.configureSignIn({
    issuer: stack.issuer.origin,
    clientId: "another-client",
  });
  await notes.reload();
  await expect(notes.getByRole("status")).toHaveText("Not signed in.");
  const [request] = await Promise.all([
    notes.waitForRequest((r) => r.url().endsWith("/CreateNote")),
    createNote(notes, "no reused client credential"),
  ]);
  expect(request.headers()["authorization"]).toBeUndefined();
});

test("a failed callback clears a session acquired earlier", async ({ notes, stack }) => {
  await signIn(notes, stack, "ana");
  await notes.goto(`${stack.origin}${basePath}?error=access_denied`);
  await expect(notes.getByRole("alert")).toHaveText("Sign-in was declined.");
  const [request] = await Promise.all([
    notes.waitForRequest((r) => r.url().endsWith("/CreateNote")),
    createNote(notes, "failed callback"),
  ]);
  expect(request.headers()["authorization"]).toBeUndefined();
});
