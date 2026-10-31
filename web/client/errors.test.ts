/// <reference types="node" />

import assert from "node:assert/strict";
import { test } from "node:test";

import { create, toBinary } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";

import {
  FieldPathSchema,
  ViolationSchema,
  ViolationsSchema,
} from "../../api/buf/validate/validate_pb.js";
import { describe, toFailure } from "./errors.js";

function invalid(path: string[], message: string): ConnectError {
  const violations = create(ViolationsSchema, {
    violations: [
      create(ViolationSchema, {
        field: create(FieldPathSchema, { elements: path.map((fieldName) => ({ fieldName })) }),
        message,
      }),
    ],
  });
  return new ConnectError("the request is invalid", Code.InvalidArgument, undefined, [
    { desc: ViolationsSchema, value: violations },
  ]);
}

test("each contract code is one distinct outcome", () => {
  const cases: [Code, string][] = [
    [Code.NotFound, "not_found"],
    [Code.Unauthenticated, "unauthenticated"],
    [Code.Unavailable, "unavailable"],
    [Code.DeadlineExceeded, "deadline"],
    [Code.Internal, "internal"],
    [Code.PermissionDenied, "unknown"],
  ];
  for (const [code, kind] of cases) {
    assert.equal(toFailure(new ConnectError("x", code)).kind, kind);
  }
  const texts = cases.map(([code]) => describe(toFailure(new ConnectError("x", code))));
  assert.equal(new Set(texts.slice(0, 4)).size, 4);
});

test("a violation lands on its field", () => {
  assert.deepEqual(toFailure(invalid(["note", "title"], "title is required")), {
    kind: "fields",
    fields: { "note.title": "title is required" },
    general: [],
  });
});

test("a violation on a field with no input stays visible", () => {
  const failure = toFailure(invalid(["note", "other"], "other is wrong"));
  assert.deepEqual(failure, { kind: "fields", fields: {}, general: ["other is wrong"] });
  assert.equal(describe(failure), "other is wrong");
});

test("an invalid response without violations still shows something", () => {
  const failure = toFailure(new ConnectError("bad request", Code.InvalidArgument));
  assert.equal(describe(failure), "bad request");
});

test("an internal failure shows only the correlation id", () => {
  const failure = toFailure(new ConnectError("internal error, correlation id 00ff", Code.Internal));
  assert.equal(describe(failure), "Something went wrong. Reference: 00ff");
  const other = toFailure(new ConnectError("planted detail", Code.Internal));
  assert.equal(describe(other), "Something went wrong.");
});

test("a fetch that never reached a server is unavailable", () => {
  assert.equal(toFailure(new TypeError("Failed to fetch")).kind, "unavailable");
  const wrapped = new ConnectError("x", Code.Unknown, undefined, undefined, new TypeError("Failed to fetch"));
  assert.equal(toFailure(wrapped).kind, "unavailable");
});

test("the violations fixture really encodes", () => {
  assert.ok(toBinary(ViolationsSchema, create(ViolationsSchema)).length === 0);
});
