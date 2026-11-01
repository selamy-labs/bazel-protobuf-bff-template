// Every scenario in specs/001 contracts/notes-api.md, in a real browser against the real server.
// Titles are stable: the defect witness names them.
import { test as base, expect } from "@playwright/test";

import { serve } from "../serve/serve.mjs";
import { startStack } from "./stack.mjs";

const test = base.extend({
  // Each worker has its own server, so tests share nothing with other runs.
  stack: [
    async ({}, use) => {
      const stack = await startStack();
      await use(stack);
      await stack.stop();
    },
    { scope: "worker" },
  ],
  notes: async ({ page, stack }, use) => {
    await page.goto(stack.origin);
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

async function openNote(page, name) {
  await page.getByLabel("Name").fill(name);
  await page.getByRole("button", { name: "Open note" }).click();
}

test("creates a note and opens it by name", async ({ notes }) => {
  await identify(notes, "ana");
  await createNote(notes, "  Shopping  ", "milk");
  await expect(notes.getByRole("heading", { name: "Shopping" })).toBeVisible();
  const name = await notes.getByTestId("note-name").textContent();
  expect(name).toMatch(/^notes\/[a-z0-9-]+$/);
  await notes.reload();
  await identify(notes, "ana");
  await openNote(notes, name);
  await expect(notes.getByTestId("note-body")).toHaveText("milk");
});

test("empty title shows on the title field", async ({ notes }) => {
  await createNote(notes, "   ");
  await expect(notes.getByTestId("title-error")).toHaveText("title is required");
  await expect(notes.getByTestId("note")).toHaveCount(0);
});

test("title too long shows on the title field", async ({ notes }) => {
  await createNote(notes, "a".repeat(121));
  await expect(notes.getByTestId("title-error")).toHaveText("title must be at most 120 characters");
});

test("body too long shows on the body field", async ({ notes }) => {
  await createNote(notes, "a", "b".repeat(4001));
  await expect(notes.getByTestId("body-error")).toContainText("4000");
  await expect(notes.getByTestId("title-error")).toHaveCount(0);
});

test("invalid name shows on the name field", async ({ notes }) => {
  await openNote(notes, "tasks/1");
  await expect(notes.getByTestId("name-error")).toBeVisible();
});

test("a field message clears when the input changes", async ({ notes }) => {
  await createNote(notes, " ");
  await expect(notes.getByTestId("title-error")).toBeVisible();
  await notes.getByLabel("Title").fill("fixed");
  await expect(notes.getByTestId("title-error")).toHaveCount(0);
});

test("not found", async ({ notes }) => {
  await openNote(notes, "notes/does-not-exist");
  await expect(notes.getByTestId("failure")).toHaveText("Not found.");
});

test("another principal's note reads as not found", async ({ notes }) => {
  await identify(notes, "ana");
  await createNote(notes, "private");
  await expect(notes.getByTestId("note")).toBeVisible();
  const name = await notes.getByTestId("note-name").textContent();
  await identify(notes, "bob");
  await openNote(notes, name);
  await expect(notes.getByTestId("failure")).toHaveText("Not found.");
  await expect(notes.getByTestId("note")).toHaveCount(0);
});

test("no identity is unauthenticated", async ({ notes }) => {
  await identify(notes, "");
  await createNote(notes, "a");
  await expect(notes.getByTestId("failure")).toHaveText("Authentication is required.");
});

test("storage unavailable on create", async ({ notes }) => {
  await identify(notes, "fault-unavailable");
  await createNote(notes, "a");
  await expect(notes.getByTestId("failure")).toHaveText("The service is unavailable. Try again.");
});

test("deadline exceeded on open", async ({ notes }) => {
  await identify(notes, "fault-hang");
  await openNote(notes, "notes/a");
  await expect(notes.getByTestId("failure")).toHaveText("The request took too long. Try again.", {
    timeout: 5000,
  });
});

for (const subject of ["fault-secret", "fault-panic"]) {
  test(`internal failure shows only a correlation id: ${subject}`, async ({ notes }) => {
    await identify(notes, subject);
    await openNote(notes, "notes/a");
    await expect(notes.getByTestId("failure")).toHaveText(
      /^Something went wrong\. Reference: [0-9a-f]+$/,
    );
    await expect(notes.locator("body")).not.toContainText("planted");
  });
}

test("a stopped server is unavailable and a restart is not assumed", async ({ page }) => {
  const stack = await startStack();
  await page.goto(stack.origin);
  await stack.stopServer();
  await createNote(page, "a");
  await expect(page.getByTestId("failure")).toHaveText("The service is unavailable. Try again.");
  await stack.stop();
});

test("a newer open supersedes a hanging one, whose outcome never shows", async ({ notes }) => {
  await identify(notes, "ana");
  await createNote(notes, "kept", "the newer result");
  const name = await notes.getByTestId("note-name").textContent();
  await identify(notes, "fault-hang");
  await openNote(notes, name);
  await expect(notes.getByTestId("loading")).toBeVisible();
  await identify(notes, "ana");
  await openNote(notes, name);
  await expect(notes.getByTestId("note-body")).toHaveText("the newer result");
  // Past the hanging call's deadline, the page still shows the newer result and no failure.
  await notes.waitForTimeout(2500);
  await expect(notes.getByTestId("note")).toBeVisible();
  await expect(notes.getByTestId("failure")).toHaveCount(0);
});

test("a render error shows the boundary's message, not a blank page", async ({ page, stack }) => {
  await page.goto(`${stack.origin}#planted-render-error`);
  await expect(page.getByTestId("render-error")).toHaveText(
    "The page failed to render. Reload to continue.",
  );
  await expect(page.getByTestId("test-identity")).toBeVisible();
  await expect(page.getByRole("button", { name: "Create note" })).toHaveCount(0);
});

/** Where keyboard focus is, and whether the browser paints it: the focus ring's computed outline. */
const focused = (page) =>
  page.evaluate(() => {
    const element = document.activeElement;
    const style = element === null ? null : getComputedStyle(element);
    return {
      label: element?.labels?.[0]?.textContent?.trim() ?? element?.textContent?.trim() ?? "",
      visible: style !== null && style.outlineStyle !== "none" && style.outlineWidth !== "0px",
    };
  });

test("the keyboard alone creates a note and opens it, with focus visible throughout", async ({
  notes,
}) => {
  await notes.keyboard.press("Tab");
  expect(await focused(notes)).toEqual({
    label: "Test identity: development only, never deployed",
    visible: true,
  });
  await notes.keyboard.press("Tab");
  expect((await focused(notes)).label).toBe("Title");
  await notes.keyboard.type("By keyboard");
  await notes.keyboard.press("Tab");
  expect((await focused(notes)).label).toBe("Body");
  await notes.keyboard.type("no pointer");
  await notes.keyboard.press("Tab");
  expect(await focused(notes)).toEqual({ label: "Create note", visible: true });
  await notes.keyboard.press("Enter");
  await expect(notes.getByRole("heading", { name: "By keyboard" })).toBeVisible();
  // Focus stays on the button through the call: it was never disabled.
  expect((await focused(notes)).label).toBe("Create note");
  const name = await notes.getByTestId("note-name").textContent();
  await notes.keyboard.press("Tab");
  expect((await focused(notes)).label).toBe("Name");
  await notes.keyboard.type(name);
  await notes.keyboard.press("Tab");
  expect(await focused(notes)).toEqual({ label: "Open note", visible: true });
  await notes.keyboard.press("Enter");
  await expect(notes.getByTestId("note-body")).toHaveText("no pointer");
});

test("after a validation error, focus returns to the first invalid field, which names its message", async ({
  notes,
}) => {
  await notes.getByLabel("Body").focus();
  await notes.keyboard.type("a body alone");
  await notes.keyboard.press("Tab");
  await notes.keyboard.press("Enter");
  const title = notes.getByRole("textbox", { name: "Title" });
  await expect(title).toBeFocused();
  await expect(title).toHaveAccessibleDescription("title is required");
  expect((await focused(notes)).visible).toBe(true);
  // Correcting the field clears its message, and the description with it.
  await notes.keyboard.type("Titled");
  await expect(notes.getByTestId("title-error")).toHaveCount(0);
  await expect(title).toHaveAccessibleDescription("");
});

test("every control has a role and a name in the accessibility tree", async ({ notes }) => {
  await expect(notes.locator("body")).toMatchAriaSnapshot(`
    - complementary:
      - textbox /Test identity/
    - main:
      - heading "Notes" [level=1]
      - form "New note":
        - textbox "Title"
        - textbox "Body"
        - button "Create note"
      - form "Open note":
        - textbox "Name"
        - button "Open note"
  `);
  await notes.getByLabel("Name").fill("tasks/1");
  await notes.getByRole("button", { name: "Open note" }).click();
  await expect(notes.getByRole("alert")).toBeVisible();
  await expect(notes.getByLabel("Name")).toHaveAccessibleDescription(/does not match/);
});

test("a second submit does not create a second note", async ({ notes }) => {
  await identify(notes, "ana");
  let creates = 0;
  notes.on("request", (request) => {
    if (request.method() === "POST" && request.url().endsWith("/CreateNote")) {
      creates += 1;
    }
  });
  await notes.getByLabel("Title").fill("once");
  await notes.getByRole("button", { name: "Create note" }).dblclick();
  await expect(notes.getByTestId("note")).toBeVisible();
  expect(creates).toBe(1);
});

test("the browser blocks a disallowed origin", async ({ page, stack }) => {
  const other = await serve(process.env.WEB_DIST, 0);
  other.setBffUrl(stack.bffUrl);
  await page.goto(other.origin);
  await createNote(page, "a");
  await expect(page.getByTestId("failure")).toHaveText("The service is unavailable. Try again.");
  await other.close();
});

test("a cookie gives no access in the browser or outside it", async ({
  notes,
  stack,
  context,
  request,
}) => {
  const credential = "Bearer test-identity.ana";
  await context.addCookies([{ name: "session", value: "cookie-value", url: stack.bffUrl }]);
  const blocked = await notes.evaluate(
    async (url) =>
      fetch(`${url}/template.notes.v1.NoteService/GetNote`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: "notes/a" }),
      }).then(
        () => "allowed",
        (reason) => reason.name,
      ),
    stack.bffUrl,
  );
  expect(blocked).toBe("TypeError");
  const direct = await request.post(`${stack.bffUrl}/template.notes.v1.NoteService/GetNote`, {
    data: { name: "notes/a" },
    headers: { Cookie: `session=${credential}`, "Content-Type": "application/json" },
  });
  expect(direct.status()).toBe(401);
  expect((await direct.json()).code).toBe("unauthenticated");
});

test("the client omits a cookie the browser would send", async ({ notes, stack, context }) => {
  await identify(notes, "ana");
  // Cookies are not scoped by port, so this one matches both the page and the server.
  await context.addCookies([{ name: "session", value: "cookie-value", url: stack.origin }]);

  // Control: a same-origin request in this context does carry it, and the check can see that.
  const [same] = await Promise.all([
    notes.waitForRequest((candidate) => candidate.url() === `${stack.origin}/config.json`),
    notes.evaluate(() => fetch("/config.json")),
  ]);
  expect((await same.allHeaders()).cookie).toBe("session=cookie-value");

  // The real client call to the same host: the POST happened, succeeded, and carried no cookie.
  const [response] = await Promise.all([
    notes.waitForResponse(
      (candidate) =>
        candidate.url().endsWith("/CreateNote") && candidate.request().method() === "POST",
    ),
    createNote(notes, "sent without a cookie"),
  ]);
  expect(response.status()).toBe(200);
  await expect(notes.getByRole("heading", { name: "sent without a cookie" })).toBeVisible();
  const headers = await response.request().allHeaders();
  expect(headers.authorization).toBe("Bearer test-identity.ana");
  expect(headers.cookie).toBeUndefined();
});
