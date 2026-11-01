/// <reference types="node" />

// Properties of the client's error mapping and transport (spec 011 FR-008, T009, research R5): the invariants
// named before the tests, each over generated inputs rather than the few examples errors.test.ts keeps.
import assert from "node:assert/strict";
import { test } from "node:test";

import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import fc from "fast-check";

import {
  FieldPathSchema,
  ViolationSchema,
  ViolationsSchema,
} from "../../api/buf/validate/validate_pb.js";
import { describe, toFailure, type Failure } from "./errors.js";
import { createNotesClient } from "./transport.js";

const kinds: Failure["kind"][] = [
  "fields",
  "not_found",
  "unauthenticated",
  "unavailable",
  "deadline",
  "cancelled",
  "internal",
  "unknown",
];

const codes = Object.values(Code).filter((value): value is Code => typeof value === "number");

/** The contract: each named code is one kind, and every other code is unknown. */
const contract: Partial<Record<Code, Failure["kind"]>> = {
  [Code.InvalidArgument]: "fields",
  [Code.NotFound]: "not_found",
  [Code.Unauthenticated]: "unauthenticated",
  [Code.Unavailable]: "unavailable",
  [Code.DeadlineExceeded]: "deadline",
  [Code.Canceled]: "cancelled",
  [Code.Internal]: "internal",
};

const runs = { numRuns: 300 };

test("the mapping is total: every code with any message is exactly one kind, and every kind has text", () => {
  fc.assert(
    fc.property(fc.constantFrom(...codes), fc.string(), (code, message) => {
      const failure = toFailure(new ConnectError(message, code));
      assert.ok(kinds.includes(failure.kind));
      assert.equal(failure.kind, contract[code] ?? "unknown");
      assert.equal(typeof describe(failure), "string");
    }),
    runs,
  );
});

test("anything thrown is a failure, and a call that never reached a server is unavailable", () => {
  fc.assert(
    fc.property(fc.anything(), (reason) => {
      assert.ok(kinds.includes(toFailure(reason).kind));
    }),
    runs,
  );
  fc.assert(
    fc.property(fc.string(), (message) => {
      assert.equal(toFailure(new TypeError(message)).kind, "unavailable");
      assert.equal(
        toFailure(
          new ConnectError(message, Code.Internal, undefined, undefined, new TypeError(message)),
        ).kind,
        "unavailable",
      );
    }),
    runs,
  );
});

const hex = fc.stringMatching(/^[0-9a-f]{1,32}$/);
const withoutReference = fc.string().filter((text) => !/correlation id [0-9a-f]/.test(text));

test("an internal error's reference is the correlation id in its message, and nothing else", () => {
  fc.assert(
    fc.property(hex, withoutReference, fc.string(), (id, before, after) => {
      // A space ends the id, so what follows cannot extend it.
      const failure = toFailure(
        new ConnectError(`${before}correlation id ${id} ${after}`, Code.Internal),
      );
      assert.deepEqual(failure, { kind: "internal", correlationId: id });
      assert.equal(describe(failure), `Something went wrong. Reference: ${id}`);
    }),
    runs,
  );
  fc.assert(
    fc.property(withoutReference, (message) => {
      assert.deepEqual(toFailure(new ConnectError(message, Code.Internal)), {
        kind: "internal",
        correlationId: null,
      });
    }),
    runs,
  );
});

/** Fields the page has an input for; the rule errors.ts applies, restated. */
const inputs = new Set(["note.title", "note.body", "name"]);

const path = fc.oneof(
  fc.constantFrom("note.title", "note.body", "name", "note", "owner"),
  fc
    .array(fc.stringMatching(/^[a-z_]{1,8}$/), { minLength: 1, maxLength: 3 })
    .map((p) => p.join(".")),
);

test("violations are partitioned by whether the page has an input for the path, in order", () => {
  fc.assert(
    fc.property(
      fc.array(fc.record({ path, message: fc.string() }), { maxLength: 6 }),
      fc.string(),
      (violations, raw) => {
        const error = new ConnectError(raw, Code.InvalidArgument, undefined, [
          {
            desc: ViolationsSchema,
            value: create(ViolationsSchema, {
              violations: violations.map(({ path, message }) =>
                create(ViolationSchema, {
                  field: create(FieldPathSchema, {
                    elements: path.split(".").map((fieldName) => ({ fieldName })),
                  }),
                  message,
                }),
              ),
            }),
          },
        ]);
        const failure = toFailure(error);
        assert.equal(failure.kind, "fields");
        if (failure.kind !== "fields") {
          return;
        }
        const fields: Record<string, string> = {};
        const general: string[] = [];
        for (const { path, message } of violations) {
          if (inputs.has(path)) {
            fields[path] = message;
          } else {
            general.push(message);
          }
        }
        if (violations.length === 0) {
          general.push(raw);
        }
        assert.deepEqual(failure, { kind: "fields", fields, general });
      },
    ),
    runs,
  );
});

/** A header value the platform accepts: printable, no line breaks. */
const headerValue = fc
  .stringMatching(/^[\x20-\x7e]{1,64}$/)
  .map((value) => value.trim())
  .filter((value) => value !== "");

test("the transport sends exactly the credential it is given, and nothing without one", async () => {
  const original = globalThis.fetch;
  const seen: (string | null)[] = [];
  globalThis.fetch = (_input: URL | RequestInfo, init?: RequestInit): Promise<Response> => {
    seen.push(new Headers(init?.headers).get("Authorization"));
    return Promise.resolve(
      new Response("{}", { status: 200, headers: { "Content-Type": "application/json" } }),
    );
  };
  try {
    await fc.assert(
      fc.asyncProperty(fc.option(headerValue, { nil: null }), async (credential) => {
        seen.length = 0;
        await createNotesClient("http://localhost", () => credential, { deadlineMs: 1000 })
          .getNote({})
          .catch(() => undefined);
        assert.deepEqual(seen, [credential]);
      }),
      { numRuns: 100 },
    );
  } finally {
    globalThis.fetch = original;
  }
});
