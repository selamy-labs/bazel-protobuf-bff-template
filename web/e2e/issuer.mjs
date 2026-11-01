// A local OpenID Connect issuer for the journey with the sign-in (spec 012 FR-016, T014): discovery, an
// authorization page that asks who to continue as, the code exchange with the proof key checked, identity
// tokens signed with a key generated here and published as a key set. It stands for the issuer's protocol
// surface, not for any provider; the server verifies its tokens exactly as it would a provider's.
import { createHash, generateKeyPairSync, randomBytes, sign, constants } from "node:crypto";
import { createServer } from "node:http";

const b64 = (value) => Buffer.from(value).toString("base64url");

const html = (body) =>
  `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Local issuer</title></head><body>${body}</body></html>`;

function readForm(request) {
  return new Promise((resolve) => {
    let text = "";
    request.on("data", (chunk) => {
      text += chunk;
    });
    request.on("end", () => resolve(new URLSearchParams(text)));
  });
}

// clientId is the one client registered here; tokens carry it as their audience.
export async function startIssuer(clientId) {
  const { privateKey, publicKey } = generateKeyPairSync("rsa", { modulusLength: 2048 });
  const jwk = { ...publicKey.export({ format: "jwk" }), kid: "local", use: "sig", alg: "RS256" };
  const requests = new Map(); // an authorization request awaiting the person's decision, by id
  const codes = new Map(); // a code awaiting its exchange, by value
  const issuer = {
    origin: "",
    clientId,
    ttlSeconds: 3600,
    issued: [], // every token issued, for a test to read
    // A token minted outside the flow, for a test that sends one the server must refuse.
    mint: (claims) => mint(claims),
  };

  function mint(claims) {
    const now = Math.floor(Date.now() / 1000);
    const payload = {
      iss: issuer.origin,
      aud: clientId,
      iat: now,
      exp: now + issuer.ttlSeconds,
      ...claims,
    };
    const signingInput = `${b64(JSON.stringify({ alg: "RS256", typ: "JWT", kid: "local" }))}.${b64(JSON.stringify(payload))}`;
    const signature = sign("sha256", Buffer.from(signingInput), {
      key: privateKey,
      padding: constants.RSA_PKCS1_PADDING,
    });
    const token = `${signingInput}.${signature.toString("base64url")}`;
    issuer.issued.push(token);
    return token;
  }

  function json(response, status, body) {
    response
      .writeHead(status, {
        "Content-Type": "application/json",
        "Access-Control-Allow-Origin": "*",
      })
      .end(JSON.stringify(body));
  }

  function authorize(url, response) {
    const p = url.searchParams;
    const wanted = {
      response_type: "code",
      client_id: clientId,
      scope: "openid",
      code_challenge_method: "S256",
    };
    for (const [name, value] of Object.entries(wanted)) {
      if (p.get(name) !== value) {
        response.writeHead(400).end(`${name} must be ${value}`);
        return;
      }
    }
    for (const name of ["redirect_uri", "state", "nonce", "code_challenge"]) {
      if (!p.get(name)) {
        response.writeHead(400).end(`${name} is required`);
        return;
      }
    }
    const id = randomBytes(16).toString("hex");
    requests.set(id, Object.fromEntries(p));
    response
      .writeHead(200, { "Content-Type": "text/html" })
      .end(
        html(
          `<h1>Local issuer</h1><form method="post" action="/authorize"><input type="hidden" name="request" value="${id}">` +
            `<label>Continue as <input name="subject" value="ana"></label>` +
            `<button name="decision" value="continue">Continue</button>` +
            `<button name="decision" value="decline">Decline</button></form>`,
        ),
      );
  }

  async function decide(request, response) {
    const form = await readForm(request);
    const pending = requests.get(form.get("request"));
    requests.delete(form.get("request"));
    if (pending === undefined) {
      response.writeHead(400).end("no such request");
      return;
    }
    const back = new URL(pending.redirect_uri);
    back.searchParams.set("state", pending.state);
    if (form.get("decision") !== "continue" || !form.get("subject")) {
      back.searchParams.set("error", "access_denied");
    } else {
      const code = randomBytes(16).toString("hex");
      codes.set(code, { ...pending, subject: form.get("subject") });
      back.searchParams.set("code", code);
    }
    response.writeHead(302, { Location: back.toString() }).end();
  }

  async function token(request, response) {
    const form = await readForm(request);
    const granted = codes.get(form.get("code"));
    codes.delete(form.get("code")); // a code is exchanged once
    const challenge =
      form.get("code_verifier") === null
        ? null
        : createHash("sha256").update(form.get("code_verifier")).digest("base64url");
    if (
      granted === undefined ||
      form.get("grant_type") !== "authorization_code" ||
      form.get("client_id") !== clientId ||
      form.get("redirect_uri") !== granted.redirect_uri ||
      challenge !== granted.code_challenge
    ) {
      json(response, 400, { error: "invalid_grant" });
      return;
    }
    json(response, 200, {
      id_token: mint({ sub: granted.subject, nonce: granted.nonce }),
      token_type: "Bearer",
      expires_in: issuer.ttlSeconds,
    });
  }

  const server = createServer(async (request, response) => {
    const url = new URL(request.url, "http://localhost");
    const route = `${request.method} ${url.pathname}`;
    switch (route) {
      case "GET /.well-known/openid-configuration":
        json(response, 200, {
          issuer: issuer.origin,
          authorization_endpoint: `${issuer.origin}/authorize`,
          token_endpoint: `${issuer.origin}/token`,
          jwks_uri: `${issuer.origin}/jwks`,
          response_types_supported: ["code"],
          code_challenge_methods_supported: ["S256"],
          id_token_signing_alg_values_supported: ["RS256"],
        });
        return;
      case "GET /jwks":
        json(response, 200, { keys: [jwk] });
        return;
      case "GET /authorize":
        authorize(url, response);
        return;
      case "POST /authorize":
        await decide(request, response);
        return;
      case "POST /token":
        await token(request, response);
        return;
      default:
        response.writeHead(404).end();
    }
  });
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  issuer.origin = `http://127.0.0.1:${server.address().port}`;
  issuer.jwksUrl = `${issuer.origin}/jwks`;
  issuer.close = () =>
    new Promise((resolve) => {
      server.close(resolve);
      server.closeAllConnections();
    });
  return issuer;
}
