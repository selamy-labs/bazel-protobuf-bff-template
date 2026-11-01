// Sign-in for the production page (spec 012 FR-016): the authorization code flow with a proof key (PKCE,
// RFC 7636) against any issuer that publishes its endpoints by discovery and signs identity tokens, which is
// what an OpenID Connect provider does. The page keeps the identity token as the bearer credential for the
// generated client. The server verifies its signature, issuer, audience and expiry (bff/adapters/identity/oidc);
// the checks here only keep a token that is plainly not for this page out of the session, and say who is
// signed in. No library: the flow is two requests, and the token is opaque to the page.

/** What the page's configuration says: the issuer, and the client this page is registered as. */
export type SignInConfig = { issuer: string; clientId: string };

/** A signed-in identity: the credential the client sends, who it is for, and when it ends (ms since the epoch). */
export type Session = { credential: string; subject: string; expiresAt: number };

/** The browser pieces the flow touches, so a test supplies its own. */
export type Ports = {
  fetch: typeof fetch;
  storage: Pick<Storage, "getItem" | "setItem" | "removeItem">;
  crypto: Pick<Crypto, "getRandomValues" | "subtle">;
  now: () => number;
};

/** Where this tab keeps the session, and the sign-in it started. */
export const sessionKey = "signin.session";
export const pendingKey = "signin.pending";

/** Why a sign-in did not complete: one of a fixed set of reasons, never token material. */
export type Reason = "discovery" | "denied" | "state" | "token" | "claims";

export class SignInError extends Error {
  constructor(readonly reason: Reason) {
    super(`sign-in: ${reason}`);
    this.name = "SignInError";
  }
}

/** What the issuer sent the browser back with, read from the page's query. */
export type Return = { code: string; state: string } | { error: string };

type Pending = {
  verifier: string;
  state: string;
  nonce: string;
  redirectUri: string;
  tokenEndpoint: string;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function base64url(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function fromBase64url(text: string): Uint8Array {
  const padded =
    text.replace(/-/g, "+").replace(/_/g, "/") + "=".repeat((4 - (text.length % 4)) % 4);
  return Uint8Array.from(atob(padded), (character) => character.charCodeAt(0));
}

function random(ports: Ports): string {
  return base64url(ports.crypto.getRandomValues(new Uint8Array(32)));
}

/** The S256 challenge of a verifier (RFC 7636 section 4.2). */
export async function challenge(verifier: string, ports: Ports): Promise<string> {
  const digest = await ports.crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier));
  return base64url(new Uint8Array(digest));
}

/** An endpoint is reached over https, or http on a loopback address, which a local issuer uses. */
function endpoint(value: unknown): value is string {
  if (typeof value !== "string") {
    return false;
  }
  try {
    const url = new URL(value);
    const loopback = /^127\.\d+\.\d+\.\d+$/.test(url.hostname) || url.hostname === "[::1]";
    return url.protocol === "https:" || (url.protocol === "http:" && loopback);
  } catch {
    return false;
  }
}

async function json(response: Response | null): Promise<unknown> {
  if (response === null || !response.ok) {
    return null;
  }
  return response.json().catch(() => null);
}

/** Reads the issuer's endpoints from its discovery document, which must name the same issuer. */
async function discover(
  config: SignInConfig,
  ports: Ports,
): Promise<{ authorizationEndpoint: string; tokenEndpoint: string }> {
  const location = config.issuer.replace(/\/$/, "") + "/.well-known/openid-configuration";
  const document = await json(await ports.fetch(location).catch(() => null));
  if (
    !isRecord(document) ||
    document.issuer !== config.issuer ||
    !endpoint(document.authorization_endpoint) ||
    !endpoint(document.token_endpoint)
  ) {
    throw new SignInError("discovery");
  }
  return {
    authorizationEndpoint: document.authorization_endpoint,
    tokenEndpoint: document.token_endpoint,
  };
}

/** Starts a sign-in: remembers this tab's proof and state, and returns where to send the browser. */
export async function begin(
  config: SignInConfig,
  redirectUri: string,
  ports: Ports,
): Promise<string> {
  const { authorizationEndpoint, tokenEndpoint } = await discover(config, ports);
  const pending: Pending = {
    verifier: random(ports),
    state: random(ports),
    nonce: random(ports),
    redirectUri,
    tokenEndpoint,
  };
  ports.storage.setItem(pendingKey, JSON.stringify(pending));
  const url = new URL(authorizationEndpoint);
  const parameters = {
    response_type: "code",
    client_id: config.clientId,
    redirect_uri: redirectUri,
    scope: "openid",
    state: pending.state,
    nonce: pending.nonce,
    code_challenge: await challenge(pending.verifier, ports),
    code_challenge_method: "S256",
  };
  for (const [name, value] of Object.entries(parameters)) {
    url.searchParams.set(name, value);
  }
  return url.toString();
}

/** The issuer's answer in the page's query, or null when the page was not opened by a return. */
export function returned(search: string): Return | null {
  const parameters = new URLSearchParams(search);
  const error = parameters.get("error");
  if (error !== null) {
    return { error };
  }
  const code = parameters.get("code");
  const state = parameters.get("state");
  return code === null || state === null ? null : { code, state };
}

/** What storage holds, or null when it holds nothing readable. */
function parse(stored: string | null): unknown {
  try {
    return stored === null ? null : JSON.parse(stored);
  } catch {
    return null;
  }
}

/** Takes the sign-in this tab started, if any. A return is answered once, whatever it says. */
function takePending(ports: Ports): Pending | null {
  const parsed = parse(ports.storage.getItem(pendingKey));
  ports.storage.removeItem(pendingKey);
  return isRecord(parsed) && typeof parsed.state === "string" ? (parsed as Pending) : null;
}

function claimsOf(token: string): Record<string, unknown> | null {
  const parts = token.split(".");
  if (parts.length !== 3 || parts[1] === undefined) {
    return null;
  }
  try {
    const parsed: unknown = JSON.parse(new TextDecoder().decode(fromBase64url(parts[1])));
    return isRecord(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

/** A token is kept only when it says it is from this issuer, for this client, for this sign-in. */
function toSession(token: string, config: SignInConfig, nonce: string): Session {
  const claims = claimsOf(token);
  if (claims === null) {
    throw new SignInError("claims");
  }
  const audience = Array.isArray(claims.aud) ? claims.aud : [claims.aud];
  if (
    claims.iss !== config.issuer ||
    !audience.includes(config.clientId) ||
    claims.nonce !== nonce ||
    typeof claims.sub !== "string" ||
    claims.sub === "" ||
    typeof claims.exp !== "number"
  ) {
    throw new SignInError("claims");
  }
  return { credential: "Bearer " + token, subject: claims.sub, expiresAt: claims.exp * 1000 };
}

/** Finishes a sign-in: the state must be this tab's, and the token must be for this client from this issuer. */
export async function complete(config: SignInConfig, back: Return, ports: Ports): Promise<Session> {
  const pending = takePending(ports);
  if ("error" in back) {
    throw new SignInError("denied");
  }
  if (pending === null || back.state !== pending.state) {
    throw new SignInError("state");
  }
  const body = new URLSearchParams({
    grant_type: "authorization_code",
    code: back.code,
    redirect_uri: pending.redirectUri,
    client_id: config.clientId,
    code_verifier: pending.verifier,
  });
  const request = {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body,
  };
  const issued = await json(await ports.fetch(pending.tokenEndpoint, request).catch(() => null));
  if (!isRecord(issued) || typeof issued.id_token !== "string") {
    throw new SignInError("token");
  }
  const session = toSession(issued.id_token, config, pending.nonce);
  ports.storage.setItem(sessionKey, JSON.stringify(session));
  return session;
}

function isSession(value: unknown): value is Session {
  return (
    isRecord(value) &&
    typeof value.credential === "string" &&
    typeof value.subject === "string" &&
    typeof value.expiresAt === "number"
  );
}

/** The session this tab holds, or null when there is none or it has ended. An ended one is forgotten. */
export function current(ports: Ports): Session | null {
  const parsed = parse(ports.storage.getItem(sessionKey));
  if (!isSession(parsed)) {
    return null;
  }
  if (parsed.expiresAt <= ports.now()) {
    ports.storage.removeItem(sessionKey);
    return null;
  }
  return parsed;
}

/** Forgets the session and any sign-in in progress in this tab. The issuer's own session is its own. */
export function signOut(ports: Ports): void {
  ports.storage.removeItem(sessionKey);
  ports.storage.removeItem(pendingKey);
}
