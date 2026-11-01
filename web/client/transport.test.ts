/// <reference types="node" />

import assert from "node:assert/strict";
import { test } from "node:test";

import { createNotesClient } from "./transport.js";

function capturing(): { headers: Headers[]; restore: () => void } {
  const original = globalThis.fetch;
  const headers: Headers[] = [];
  globalThis.fetch = (_input: URL | RequestInfo, init?: RequestInit): Promise<Response> => {
    headers.push(new Headers(init?.headers));
    return Promise.resolve(
      new Response("{}", { status: 200, headers: { "Content-Type": "application/json" } }),
    );
  };
  return {
    headers,
    restore: () => {
      globalThis.fetch = original;
    },
  };
}

test("a credential becomes the Authorization header", async () => {
  const { headers, restore } = capturing();
  try {
    await createNotesClient("http://localhost", () => "Bearer token")
      .getNote({})
      .catch(() => undefined);
    assert.equal(headers.length, 1);
    assert.equal(headers[0]?.get("Authorization"), "Bearer token");
  } finally {
    restore();
  }
});

test("with no credential, no Authorization header is sent", async () => {
  const { headers, restore } = capturing();
  try {
    await createNotesClient("http://localhost", () => null)
      .getNote({})
      .catch(() => undefined);
    assert.equal(headers.length, 1);
    assert.equal(headers[0]?.has("Authorization"), false);
  } finally {
    restore();
  }
});
