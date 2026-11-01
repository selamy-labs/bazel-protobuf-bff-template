/// <reference types="node" />

import assert from "node:assert/strict";
import { test } from "node:test";

import {
  begin,
  challenge,
  complete,
  current,
  pendingKey,
  returned,
  sessionKey,
  signOut,
  SignInError,
  type Ports,
  type Return,
  type Session,
  type SignInConfig,
} from "./oidc.js";

const config: SignInConfig = {
  issuer: "https://issuer.example.invalid",
  clientId: "bff-hosted-example",
};
const redirect = "https://site.example.invalid/bazel-protobuf-bff-template/";
const authorizationEndpoint = "https://issuer.example.invalid/authorize";
const tokenEndpoint = "https://issuer.example.invalid/token";

const encode = (value: unknown): string => Buffer.from(JSON.stringify(value)).toString("base64url");

/** An identity token as the page sees it: three parts, the middle one the claims. The signature is not read. */
function idToken(claims: Record<string, unknown>): string {
  return `${encode({ alg: "RS256", kid: "k" })}.${encode(claims)}.signature`;
}

function claims(pending: { nonce: string }, changes: Record<string, unknown> = {}) {
  return {
    iss: config.issuer,
    aud: config.clientId,
    sub: "ana",
    exp: 2_000_000_000,
    iat: 1_999_999_000,
    nonce: pending.nonce,
    ...changes,
  };
}

type Pending = { verifier: string; state: string; nonce: string; tokenEndpoint: string };

/** A fake of the issuer's three surfaces: discovery, the token endpoint, and what storage holds. */
function fake() {
  const store = new Map<string, string>();
  const tokenRequests: { body: URLSearchParams; contentType: string | null }[] = [];
  const issuer = {
    discovery: {
      issuer: config.issuer,
      authorization_endpoint: authorizationEndpoint,
      token_endpoint: tokenEndpoint,
    } as Record<string, unknown>,
    discoveryResponse: null as Response | null,
    reachable: true,
    issue: (() => null) as (body: URLSearchParams) => Response | null,
    store,
    tokenRequests,
    ports: {
      fetch: (input: URL | RequestInfo, init?: RequestInit): Promise<Response> => {
        const url = String(input);
        if (!issuer.reachable) {
          return Promise.reject(new TypeError("fetch failed"));
        }
        if (url === config.issuer + "/.well-known/openid-configuration") {
          return Promise.resolve(issuer.discoveryResponse ?? Response.json(issuer.discovery));
        }
        if (url === tokenEndpoint) {
          const body = new URLSearchParams(String(init?.body));
          tokenRequests.push({ body, contentType: new Headers(init?.headers).get("Content-Type") });
          const response = issuer.issue(body);
          return response === null
            ? Promise.reject(new TypeError("fetch failed"))
            : Promise.resolve(response);
        }
        return Promise.resolve(new Response("", { status: 404 }));
      },
      storage: {
        getItem: (key: string) => store.get(key) ?? null,
        setItem: (key: string, value: string) => void store.set(key, value),
        removeItem: (key: string) => void store.delete(key),
      },
      crypto: globalThis.crypto,
      now: () => 1_999_999_500_000,
    } satisfies Ports,
    pending: (): Pending => JSON.parse(store.get(pendingKey) ?? "null") as Pending,
  };
  return issuer;
}

test("begin keeps this tab's proof and sends the browser to the issuer with the S256 challenge", async () => {
  const issuer = fake();
  const url = new URL(await begin(config, redirect, issuer.ports));
  const pending = issuer.pending();
  assert.equal(url.origin + url.pathname, authorizationEndpoint);
  assert.deepEqual(Object.fromEntries(url.searchParams), {
    response_type: "code",
    client_id: config.clientId,
    redirect_uri: redirect,
    scope: "openid",
    state: pending.state,
    nonce: pending.nonce,
    code_challenge: await challenge(pending.verifier, issuer.ports),
    code_challenge_method: "S256",
  });
  assert.match(pending.verifier, /^[A-Za-z0-9_-]{43}$/);
  assert.match(pending.state, /^[A-Za-z0-9_-]{43}$/);
  assert.match(pending.nonce, /^[A-Za-z0-9_-]{43}$/);
  assert.equal(pending.tokenEndpoint, tokenEndpoint);
  assert.notEqual(await challenge(pending.verifier, issuer.ports), pending.verifier);
});

test("two sign-ins never share a proof, a state or a nonce", async () => {
  const issuer = fake();
  await begin(config, redirect, issuer.ports);
  const first = issuer.pending();
  await begin(config, redirect, issuer.ports);
  const second = issuer.pending();
  assert.notEqual(first.verifier, second.verifier);
  assert.notEqual(first.state, second.state);
  assert.notEqual(first.nonce, second.nonce);
});

test("a discovery that is not the issuer's is refused, and nothing is started", async () => {
  const cases: Record<string, (issuer: ReturnType<typeof fake>) => void> = {
    "the issuer is unreachable": (issuer) => {
      issuer.reachable = false;
    },
    "the document is refused": (issuer) => {
      issuer.discoveryResponse = new Response("", { status: 503 });
    },
    "the document is not JSON": (issuer) => {
      issuer.discoveryResponse = new Response("<html>", { status: 200 });
    },
    "the document is not an object": (issuer) => {
      issuer.discoveryResponse = Response.json("yes");
    },
    "the document names another issuer": (issuer) => {
      issuer.discovery.issuer = "https://other.example.invalid";
    },
    "the authorization endpoint is plain http": (issuer) => {
      issuer.discovery.authorization_endpoint = "http://issuer.example.invalid/authorize";
    },
    "the token endpoint is not a URL": (issuer) => {
      issuer.discovery.token_endpoint = "token";
    },
    "the token endpoint is missing": (issuer) => {
      delete issuer.discovery.token_endpoint;
    },
  };
  for (const [name, plant] of Object.entries(cases)) {
    const issuer = fake();
    plant(issuer);
    await assert.rejects(begin(config, redirect, issuer.ports), new SignInError("discovery"), name);
    assert.equal(issuer.store.has(pendingKey), false, name);
  }
});

test("an issuer on a loopback address may use http", async () => {
  const issuer = fake();
  issuer.discovery.authorization_endpoint = "http://127.0.0.1:9/authorize";
  issuer.discovery.token_endpoint = "http://[::1]:9/token";
  const url = new URL(await begin(config, redirect, issuer.ports));
  assert.equal(url.origin, "http://127.0.0.1:9");
  assert.equal(issuer.pending().tokenEndpoint, "http://[::1]:9/token");
});

test("returned reads the issuer's answer from the query", () => {
  assert.deepEqual(returned("?code=c1&state=s1"), { code: "c1", state: "s1" });
  assert.deepEqual(returned("?error=access_denied&state=s1"), { error: "access_denied" });
  assert.equal(returned(""), null);
  assert.equal(returned("?code=c1"), null);
  assert.equal(returned("?state=s1"), null);
});

async function started(): Promise<{
  issuer: ReturnType<typeof fake>;
  back: Return;
  pending: Pending;
}> {
  const issuer = fake();
  await begin(config, redirect, issuer.ports);
  const pending = issuer.pending();
  issuer.issue = () => Response.json({ id_token: idToken(claims(pending)) });
  return { issuer, back: { code: "c1", state: pending.state }, pending };
}

test("complete exchanges the code with the proof and keeps the session for this tab", async () => {
  const { issuer, back, pending } = await started();
  const session = await complete(config, back, issuer.ports);
  const token = idToken(claims(pending));
  const expected: Session = {
    credential: "Bearer " + token,
    subject: "ana",
    expiresAt: 2_000_000_000_000,
  };
  assert.deepEqual(session, expected);
  assert.equal(issuer.tokenRequests.length, 1);
  assert.equal(issuer.tokenRequests[0]?.contentType, "application/x-www-form-urlencoded");
  assert.deepEqual(Object.fromEntries(issuer.tokenRequests[0]?.body ?? []), {
    grant_type: "authorization_code",
    code: "c1",
    redirect_uri: redirect,
    client_id: config.clientId,
    code_verifier: pending.verifier,
  });
  assert.deepEqual(JSON.parse(issuer.store.get(sessionKey) ?? "null"), expected);
  assert.equal(issuer.store.has(pendingKey), false);
  assert.deepEqual(current(issuer.ports), expected);
});

test("an audience list that names the client is accepted", async () => {
  const { issuer, back, pending } = await started();
  issuer.issue = () =>
    Response.json({ id_token: idToken(claims(pending, { aud: ["other", config.clientId] })) });
  assert.equal((await complete(config, back, issuer.ports)).subject, "ana");
});

test("every failed completion is a named refusal that keeps no session and carries no token", async () => {
  type Plant = (issuer: ReturnType<typeof fake>, pending: Pending) => Return | undefined;
  const cases: Record<string, [Plant, SignInError["reason"]]> = {
    "the issuer declined": [() => ({ error: "access_denied" }), "denied"],
    "a state from another tab": [() => ({ code: "c1", state: "other" }), "state"],
    "no sign-in started here": [(issuer) => void issuer.store.delete(pendingKey), "state"],
    "the sign-in started here is unreadable": [
      (issuer) => void issuer.store.set(pendingKey, "{not json"),
      "state",
    ],
    "the token endpoint refuses": [
      (issuer) => void (issuer.issue = () => new Response("", { status: 400 })),
      "token",
    ],
    "the token endpoint is unreachable": [(issuer) => void (issuer.issue = () => null), "token"],
    "no identity token is issued": [
      (issuer) => void (issuer.issue = () => Response.json({ access_token: "x" })),
      "token",
    ],
    "the issue is not JSON": [
      (issuer) => void (issuer.issue = () => new Response("nope", { status: 200 })),
      "token",
    ],
    "the issue is not an object": [
      (issuer) => void (issuer.issue = () => Response.json(7)),
      "token",
    ],
    "another issuer's token": [plantClaims({ iss: "https://other.example.invalid" }), "claims"],
    "another client's token": [plantClaims({ aud: "someone-else" }), "claims"],
    "another sign-in's token": [plantClaims({ nonce: "other" }), "claims"],
    "no subject": [plantClaims({ sub: "" }), "claims"],
    "no expiry": [plantClaims({ exp: "soon" }), "claims"],
    "not a token": [
      (issuer) => void (issuer.issue = () => Response.json({ id_token: "abc" })),
      "claims",
    ],
    "claims that are not JSON": [
      (issuer) =>
        void (issuer.issue = () =>
          Response.json({ id_token: `h.${Buffer.from("nope").toString("base64url")}.s` })),
      "claims",
    ],
    "claims that are not an object": [
      (issuer) => void (issuer.issue = () => Response.json({ id_token: `h.${encode(42)}.s` })),
      "claims",
    ],
  };
  function plantClaims(changes: Record<string, unknown>): Plant {
    return (issuer, pending) => {
      issuer.issue = () => Response.json({ id_token: idToken(claims(pending, changes)) });
      return undefined;
    };
  }
  for (const [name, [plant, reason]] of Object.entries(cases)) {
    const { issuer, back, pending } = await started();
    const planted = plant(issuer, pending) ?? back;
    await assert.rejects(
      complete(config, planted, issuer.ports),
      (error: unknown) => {
        assert.ok(error instanceof SignInError, name);
        assert.equal(error.reason, reason, name);
        assert.equal(error.message, `sign-in: ${reason}`, name);
        return true;
      },
      name,
    );
    assert.equal(issuer.store.has(sessionKey), false, name);
    assert.equal(issuer.store.has(pendingKey), false, name);
  }
});

test("current is the session while it lasts, and an ended one is forgotten", () => {
  const issuer = fake();
  assert.equal(current(issuer.ports), null);
  const session: Session = { credential: "Bearer t", subject: "ana", expiresAt: 1_999_999_500_001 };
  issuer.store.set(sessionKey, JSON.stringify(session));
  assert.deepEqual(current(issuer.ports), session);
  issuer.store.set(sessionKey, JSON.stringify({ ...session, expiresAt: 1_999_999_500_000 }));
  assert.equal(current(issuer.ports), null);
  assert.equal(issuer.store.has(sessionKey), false);
  for (const stored of ["{not json", JSON.stringify({ subject: "ana" }), JSON.stringify(null)]) {
    issuer.store.set(sessionKey, stored);
    assert.equal(current(issuer.ports), null, stored);
  }
});

test("signOut forgets the session and any sign-in in progress", async () => {
  const { issuer, back } = await started();
  await complete(config, back, issuer.ports);
  await begin(config, redirect, issuer.ports);
  signOut(issuer.ports);
  assert.equal(issuer.store.size, 0);
  assert.equal(current(issuer.ports), null);
});
