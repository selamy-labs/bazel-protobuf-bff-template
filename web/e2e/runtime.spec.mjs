// Required actual-container witness for spec009 T018/T019. Source tests cannot replace it.
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { test as base, expect } from "@playwright/test";
import { startIssuer } from "./issuer.mjs";
import { openImageRuntime } from "./image-runtime.mjs";
import { checkReady } from "./runtime-contract.mjs";
import { sameOriginJourney, signIn, createNote } from "./same-origin-journey.mjs";

// Local discard receiver only, so the production exporter can drain on shutdown. This is not
// collector/export acceptance; the existing telemetry smoke owns that separate witness.
async function discardReceiver() {
  const server = createServer((request, response) => {
    request.resume();
    request.once("end", () =>
      response.writeHead(200, { "Content-Type": "application/x-protobuf" }).end(),
    );
  });
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  return {
    origin: `http://127.0.0.1:${server.address().port}`,
    close: () => new Promise((resolve) => server.close(resolve)),
  };
}
const test = base.extend({
  stack: [
    async ({}, use) => {
      const issuer = await startIssuer("actual-image-browser");
      let receiver, runtime, running;
      try {
        receiver = await discardReceiver();
        runtime = await openImageRuntime();
        const start = (options) => runtime.start({ issuer, otlp: receiver.origin, ...options });
        running = await start({});
        checkReady(running.event, running.port);
        await use({ ...running, issuer, start });
      } finally {
        try {
          if (running) await running.close();
        } finally {
          try {
            if (runtime) await runtime.close();
          } finally {
            try {
              if (receiver) await receiver.close();
            } finally {
              await issuer.close();
            }
          }
        }
      }
    },
    { scope: "worker", timeout: 90000 },
  ],
  notes: async ({ page, stack }, use) => {
    await page.goto(stack.origin + "/");
    await expect(page.getByTestId("sign-in")).toBeVisible();
    await use(page);
  },
});
// One positive artifact first, then planted runtime configuration defects and restored controls.
test.describe.configure({ mode: "serial" });
sameOriginJourney(test);
test("missing packaged assets are caught while the same image still serves authenticated RPC", async ({
  stack,
}) => {
  const bad = await stack.start({ missingAssets: true });
  try {
    checkReady(bad.event, bad.port);
    expect((await fetch(bad.origin + "/")).status).toBe(404);
    const reply = await fetch(bad.origin + "/template.notes.v1.NoteService/CreateNote", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer " + stack.issuer.mint({ sub: "ana" }),
      },
      body: JSON.stringify({ note: { title: "assets-only fault" } }),
    });
    expect(reply.status).toBe(200);
    expect((await fetch(stack.origin + "/")).status).toBe(200);
  } finally {
    await bad.close();
  }
});
test("a real wrong-PORT listener is rejected and the declared-port control is restored", async ({
  stack,
}) => {
  const bad = await stack.start({ wrongPort: true });
  try {
    assert.throws(() => checkReady(bad.event, bad.port), /PORT listener mismatch/);
    expect((await fetch(`http://127.0.0.1:${bad.otherPort}/config.json`)).status).toBe(200);
    await expect(fetch(bad.origin + "/", { signal: AbortSignal.timeout(1500) })).rejects.toThrow();
    checkReady(stack.event, stack.port);
    expect((await fetch(stack.origin + "/")).status).toBe(200);
  } finally {
    await bad.close();
  }
});
test("a planted RPC deadline breaks the generated client and the same-artifact positive is restored", async ({
  page,
  stack,
}) => {
  const bad = await stack.start({ brokenRPC: true });
  try {
    checkReady(bad.event, bad.port);
    await page.goto(bad.origin + "/");
    await signIn(page, { ...stack, origin: bad.origin }, "ana");
    const response = page.waitForResponse((r) => r.url().endsWith("/CreateNote"));
    await createNote(page, "broken RPC");
    expect((await response).status()).toBe(504);
    await expect(page.getByRole("alert")).toHaveText("The request took too long. Try again.");
    await expect(page.getByTestId("note-body")).toHaveCount(0);
    await page.goto(stack.origin + "/");
    await signIn(page, stack, "ana");
    await createNote(page, "restored RPC");
    await expect(page.getByTestId("note-body")).toHaveText("from the production bundle");
  } finally {
    await bad.close();
  }
});

test("the actual image refuses a missing verifier with no listener", async ({ stack }) => {
  const bad = await stack.start({ missingVerifier: true });
  try {
    expect(bad.refused).toBe(true);
    await expect(fetch(bad.origin + "/", { signal: AbortSignal.timeout(1500) })).rejects.toThrow();
    expect((await fetch(stack.origin + "/")).status).toBe(200);
  } finally {
    await bad.close();
  }
});
