import { expect } from "@playwright/test";

export async function signIn(page, stack, subject) {
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.waitForURL((url) => url.origin === stack.issuer.origin);
  await page.getByLabel("Continue as").fill(subject);
  await page.getByRole("button", { name: "Continue" }).click();
  await page.waitForURL((url) => url.origin === stack.origin);
  await expect(page.getByTestId("signed-in")).toHaveText(`Signed in as ${subject}`);
}

export async function createNote(page, title) {
  await page.getByLabel("Title").fill(title);
  await page.getByLabel("Body").fill("from the production bundle");
  await page.getByRole("button", { name: "Create note" }).click();
}

export function sameOriginJourney(test) {
  test("the production UI shares the origin and preserves missing-bearer refusal", async ({
    notes,
  }) => {
    await expect(notes.getByTestId("test-identity")).toHaveCount(0);
    const response = notes.waitForResponse((reply) => reply.url().endsWith("/CreateNote"));
    await createNote(notes, "before sign-in");
    expect((await response).status()).toBe(401);
    await expect(notes.getByRole("alert")).toHaveText("Authentication is required.");
  });

  test("the generated client succeeds and reports a typed owner-isolation failure on this origin", async ({
    notes,
    stack,
  }) => {
    await signIn(notes, stack, "ana");
    const [request] = await Promise.all([
      notes.waitForRequest((request) => request.url().endsWith("/CreateNote")),
      createNote(notes, "one origin"),
    ]);
    expect(new URL(request.url()).origin).toBe(stack.origin);
    expect((await request.response()).status()).toBe(200);
    await expect(notes.getByTestId("note-body")).toHaveText("from the production bundle");
    const name = await notes.getByTestId("note-name").textContent();
    await notes.getByRole("button", { name: "Sign out" }).click();
    await signIn(notes, stack, "bob");
    await notes.getByLabel("Name").fill(name);
    await notes.getByRole("button", { name: "Open note" }).click();
    await expect(notes.getByRole("alert")).toHaveText("Not found.");
  });

  test("the real verifier rejects wrong audience, wrong issuer and invalid signature beside the UI", async ({
    notes,
    stack,
  }) => {
    const valid = stack.issuer.mint({ sub: "ana" });
    const parts = valid.split(".");
    parts[2] = (parts[2][0] === "A" ? "B" : "A") + parts[2].slice(1);
    for (const token of [
      stack.issuer.mint({ sub: "ana", aud: "wrong-audience" }),
      stack.issuer.mint({ sub: "ana", iss: "https://wrong-issuer.invalid" }),
      parts.join("."),
    ]) {
      const response = await notes.request.post(
        stack.origin + "/template.notes.v1.NoteService/CreateNote",
        {
          headers: { authorization: `Bearer ${token}` },
          data: { note: { title: "refused" } },
        },
      );
      expect(response.status()).toBe(401);
    }
  });
}
