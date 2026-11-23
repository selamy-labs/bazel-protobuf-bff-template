// Contract conformance tests proving the stateful Connect fake speaks the generated Protobuf/Connect
// client contract: create, read, restart persistence, validation details, denial, and failures (#27).
import assert from "node:assert/strict";
import { mkdtemp, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { toFailure } from "../client/errors.js";
import { createNotesClient } from "../client/transport.js";
import { startStatefulNotesConnectFake } from "./connect-notes-fake.mjs";

const ORIGIN = "http://127.0.0.1:4173";

function clientFor(baseUrl, subject) {
  const credential = () => (subject === null ? null : `Bearer test-identity.${subject}`);
  return createNotesClient(baseUrl, credential, { deadlineMs: 2000 });
}

test("stateful Connect fake persists created notes across server restarts", async (t) => {
  const dir = await mkdtemp(path.join(process.env.TEST_TMPDIR ?? os.tmpdir(), "connect-fake-"));
  t.after(() => rm(dir, { recursive: true, force: true }));
  const stateFile = path.join(dir, "notes-state.json");

  const first = await startStatefulNotesConnectFake({
    allowedOrigin: ORIGIN,
    stateFile,
  });
  let createdName;
  try {
    const ana = clientFor(first.url, "ana");
    const created = await ana.createNote({
      note: { title: "  Sprint notes  ", body: "persisted" },
    });
    assert.equal(created.note?.title, "Sprint notes");
    assert.equal(created.note?.body, "persisted");
    assert.match(created.note?.name ?? "", /^notes\/[a-z0-9-]+$/);
    createdName = created.note.name;
  } finally {
    await first.close();
  }

  const second = await startStatefulNotesConnectFake({
    allowedOrigin: ORIGIN,
    stateFile,
  });
  t.after(() => second.close());
  const ana = clientFor(second.url, "ana");
  const opened = await ana.getNote({ name: createdName });
  assert.equal(opened.note?.title, "Sprint notes");
  assert.equal(opened.note?.body, "persisted");
});

test("stateful Connect fake enforces validation, cross-subject denial, auth, and unavailability", async (t) => {
  const fake = await startStatefulNotesConnectFake({ allowedOrigin: ORIGIN });
  t.after(() => fake.close());

  const ana = clientFor(fake.url, "ana");
  const bob = clientFor(fake.url, "bob");
  const unauth = clientFor(fake.url, null);
  const unavailable = clientFor(fake.url, "fault-unavailable");

  const { note } = await ana.createNote({
    note: { title: "Private", body: "only ana" },
  });

  await assert.rejects(
    () => ana.createNote({ note: { title: "   ", body: "" } }),
    (error) => {
      assert.deepEqual(toFailure(error), {
        kind: "fields",
        fields: { "note.title": "title is required" },
        general: [],
      });
      return true;
    },
  );

  await assert.rejects(
    () => ana.getNote({ name: "tasks/1" }),
    (error) => {
      const failure = toFailure(error);
      assert.equal(failure.kind, "fields");
      assert.match(failure.fields.name, /does not match/);
      return true;
    },
  );

  await assert.rejects(
    () => bob.getNote({ name: note.name }),
    (error) => {
      assert.deepEqual(toFailure(error), { kind: "not_found" });
      return true;
    },
  );

  await assert.rejects(
    () => unauth.createNote({ note: { title: "No token", body: "" } }),
    (error) => {
      assert.deepEqual(toFailure(error), { kind: "unauthenticated" });
      return true;
    },
  );

  await assert.rejects(
    () => unavailable.createNote({ note: { title: "Outage", body: "" } }),
    (error) => {
      assert.deepEqual(toFailure(error), { kind: "unavailable" });
      return true;
    },
  );
});

test("stateful Connect fake counts Unicode code points rather than UTF-16 units for schema bounds", async (t) => {
  const fake = await startStatefulNotesConnectFake({ allowedOrigin: ORIGIN });
  t.after(() => fake.close());

  const ana = clientFor(fake.url, "ana");
  const emoji = "😀";
  assert.equal(emoji.length, 2);

  // 120 supplementary code points (240 UTF-16 code units) and 4000 code points (8000 UTF-16 units) are valid.
  const exactTitle = emoji.repeat(120);
  const exactBody = emoji.repeat(4000);
  const created = await ana.createNote({ note: { title: `  ${exactTitle}  `, body: exactBody } });
  assert.equal(created.note?.title, exactTitle);
  assert.equal(created.note?.body, exactBody);

  // 121 code points exceeds the title bound.
  await assert.rejects(
    () => ana.createNote({ note: { title: emoji.repeat(121), body: "" } }),
    (error) => {
      assert.deepEqual(toFailure(error), {
        kind: "fields",
        fields: { "note.title": "title must be at most 120 characters" },
        general: [],
      });
      return true;
    },
  );

  // 4001 code points exceeds the body bound.
  await assert.rejects(
    () => ana.createNote({ note: { title: "ok", body: emoji.repeat(4001) } }),
    (error) => {
      const failure = toFailure(error);
      assert.equal(failure.kind, "fields");
      assert.match(failure.fields["note.body"] ?? "", /4000/);
      return true;
    },
  );
});
