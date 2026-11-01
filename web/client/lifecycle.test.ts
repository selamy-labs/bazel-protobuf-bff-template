/// <reference types="node" />

import assert from "node:assert/strict";
import { test } from "node:test";

import { latest, Requests } from "./lifecycle.js";
import { toFailure } from "./errors.js";
import { createNotesClient } from "./transport.js";

/** A promise settled by hand, so that the test decides the order in which calls finish. */
function deferred<T>(): {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (reason: unknown) => void;
} {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((ok, fail) => {
    resolve = ok;
    reject = fail;
  });
  return { promise, resolve, reject };
}

test("slow A, fast B: B is applied, A is cancelled and then ignored", async () => {
  const requests = new Requests();
  const a = deferred<string>();
  const b = deferred<string>();
  const signals: AbortSignal[] = [];
  const started = (value: Promise<string>) => (signal: AbortSignal) => {
    signals.push(signal);
    return value;
  };
  const resultA = latest(requests, started(a.promise));
  const resultB = latest(requests, started(b.promise));
  assert.equal(signals[0]?.aborted, true, "starting B cancels A");
  assert.equal(signals[1]?.aborted, false);
  b.resolve("B");
  assert.deepEqual(await resultB, { ok: true, value: "B" });
  a.resolve("A");
  assert.equal(await resultA, null, "A settled after B and is ignored");
});

test("a rejection of the latest call is reported, one of a superseded call is not", async () => {
  const requests = new Requests();
  const first = deferred<string>();
  const second = deferred<string>();
  const resultFirst = latest(requests, () => first.promise);
  const resultSecond = latest(requests, () => second.promise);
  first.reject(new Error("too late"));
  assert.equal(await resultFirst, null);
  second.reject(new Error("the real failure"));
  const settled = await resultSecond;
  assert.ok(settled !== null && !settled.ok);
  assert.equal((settled.reason as Error).message, "the real failure");
});

test("dispose cancels the request in flight and refuses every later start", async () => {
  const requests = new Requests();
  const pending = deferred<string>();
  let signal: AbortSignal | undefined;
  const result = latest(requests, (s) => {
    signal = s;
    return pending.promise;
  });
  requests.dispose();
  assert.equal(signal?.aborted, true);
  pending.resolve("after unmount");
  assert.equal(await result, null);
  const late = requests.start();
  assert.equal(late.signal.aborted, true, "born cancelled");
  assert.equal(late.current(), false);
  assert.equal(await latest(requests, () => Promise.resolve("never shown")), null);
});

test("cancel with nothing in flight is harmless, and a new start is current", () => {
  const requests = new Requests();
  requests.cancel();
  const attempt = requests.start();
  assert.equal(attempt.current(), true);
  assert.equal(attempt.signal.aborted, false);
  requests.cancel();
  assert.equal(attempt.current(), false);
  assert.equal(attempt.signal.aborted, true);
});

/** A fetch that never answers: it settles only when the request's signal aborts, with the signal's reason. */
function hanging(): { restore: () => void } {
  const original = globalThis.fetch;
  globalThis.fetch = (_input: URL | RequestInfo, init?: RequestInit): Promise<Response> =>
    new Promise((_resolve, reject) => {
      const signal = init?.signal;
      if (signal === null || signal === undefined) {
        reject(new Error("the transport sent no signal"));
        return;
      }
      signal.addEventListener("abort", () => reject(signal.reason as unknown), { once: true });
    });
  return {
    restore: () => {
      globalThis.fetch = original;
    },
  };
}

test("through the real transport, the deadline ends a hanging call as a deadline failure", async () => {
  const { restore } = hanging();
  try {
    const client = createNotesClient("http://localhost", () => null, { deadlineMs: 20 });
    const reason = await client.getNote({ name: "notes/a" }).then(
      () => assert.fail("the call must not succeed"),
      (error: unknown) => error,
    );
    assert.equal(toFailure(reason).kind, "deadline");
  } finally {
    restore();
  }
});

test("through the real transport, cancelling a hanging call is a cancelled failure", async () => {
  const { restore } = hanging();
  try {
    const client = createNotesClient("http://localhost", () => null, { deadlineMs: 10_000 });
    const requests = new Requests();
    const result = latest(requests, (signal) => client.getNote({ name: "notes/a" }, { signal }));
    const controller = new AbortController();
    const direct = client.getNote({ name: "notes/a" }, { signal: controller.signal }).then(
      () => assert.fail("the call must not succeed"),
      (error: unknown) => toFailure(error).kind,
    );
    requests.cancel();
    controller.abort();
    assert.equal(await result, null, "a cancelled latest call is ignored by its owner");
    assert.equal(await direct, "cancelled", "the same cancellation, seen directly, is named");
  } finally {
    restore();
  }
});
