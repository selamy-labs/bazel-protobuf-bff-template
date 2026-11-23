// Conforming stateful Connect/Protobuf fake for template.notes.v1.NoteService (testonly, #27).
// Speaks the generated Protobuf/Connect JSON and binary contracts, persists state to disk across
// reloads and restarts, and exercises validation, denial, and unavailable failure behavior.
import { mkdir, readFile, rename, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import path from "node:path";

import { create, fromBinary, fromJson, toBinary, toJson } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";

import {
  FieldPathElementSchema,
  FieldPathSchema,
  ViolationSchema,
  ViolationsSchema,
} from "../../api/buf/validate/validate_pb.js";
import {
  CreateNoteRequestSchema,
  CreateNoteResponseSchema,
  GetNoteRequestSchema,
  GetNoteResponseSchema,
  NoteSchema,
} from "../../api/template/notes/v1/notes_pb.js";

const CREATE_PATH = "/template.notes.v1.NoteService/CreateNote";
const GET_PATH = "/template.notes.v1.NoteService/GetNote";
const NOTE_NAME_PATTERN = /^notes\/[a-z0-9-]{1,64}$/;
const TEST_IDENTITY_PREFIX = "Bearer test-identity.";
const ALLOW_HEADERS =
  "Authorization, Content-Type, Connect-Protocol-Version, Connect-Timeout-Ms, X-Grpc-Web";

function isProtoContent(contentType) {
  return (contentType ?? "").toLowerCase().startsWith("application/proto");
}

function corsHeaders(requestOrigin, allowedOrigin) {
  if (!requestOrigin || requestOrigin !== allowedOrigin) {
    return { Vary: "Origin" };
  }
  return {
    "Access-Control-Allow-Origin": allowedOrigin,
    "Access-Control-Allow-Methods": "POST, OPTIONS",
    "Access-Control-Allow-Headers": ALLOW_HEADERS,
    Vary: "Origin",
  };
}

function writeConnectError(response, cors, status, code, message, details = []) {
  const payload = details.length > 0 ? { code, message, details } : { code, message };
  response
    .writeHead(status, { ...cors, "Content-Type": "application/json" })
    .end(JSON.stringify(payload));
}

function encodeViolations(items) {
  const violations = items.map(({ field, id, message }) =>
    create(ViolationSchema, {
      field: create(FieldPathSchema, {
        elements: field
          .split(".")
          .map((fieldName) => create(FieldPathElementSchema, { fieldName })),
      }),
      ruleId: id,
      message,
    }),
  );
  const bytes = toBinary(ViolationsSchema, create(ViolationsSchema, { violations }));
  return [
    {
      type: "buf.validate.Violations",
      value: Buffer.from(bytes).toString("base64"),
    },
  ];
}

function readBody(request) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    request.on("data", (chunk) => chunks.push(chunk));
    request.once("end", () => resolve(Buffer.concat(chunks)));
    request.once("error", reject);
  });
}

function decodeRequest(schema, body, protoMode) {
  if (protoMode) {
    return fromBinary(schema, body);
  }
  const text = body.toString("utf8").trim();
  const parsed = text === "" ? {} : JSON.parse(text);
  return fromJson(schema, parsed, { ignoreUnknownFields: false });
}

function writeSuccess(response, cors, schema, message, protoMode) {
  if (protoMode) {
    const bytes = toBinary(schema, message);
    response.writeHead(200, { ...cors, "Content-Type": "application/proto" }).end(bytes);
    return;
  }
  const body = JSON.stringify(toJson(schema, message));
  response.writeHead(200, { ...cors, "Content-Type": "application/json" }).end(body);
}

function extractSubject(authorization) {
  if (typeof authorization !== "string" || !authorization.startsWith(TEST_IDENTITY_PREFIX)) {
    return null;
  }
  const subject = authorization.slice(TEST_IDENTITY_PREFIX.length).trim();
  return subject === "" ? null : subject;
}

function codePointLength(text) {
  return Array.from(text).length;
}

function validateCreateNote(requestMessage) {
  const note = requestMessage.note;
  if (!note) {
    return [{ field: "note", id: "required", message: "value is required" }];
  }
  const violations = [];
  const trimmed = note.title.trim();
  const titleLength = codePointLength(trimmed);
  if (titleLength === 0) {
    violations.push({
      field: "note.title",
      id: "note.title.required",
      message: "title is required",
    });
  } else if (titleLength > 120) {
    violations.push({
      field: "note.title",
      id: "note.title.max_len",
      message: "title must be at most 120 characters",
    });
  }
  if (codePointLength(note.body) > 4000) {
    violations.push({
      field: "note.body",
      id: "string.max_len",
      message: "value length must be at most 4000 characters",
    });
  }
  return violations;
}

async function loadState(stateFile, initialNotes) {
  if (stateFile) {
    try {
      const raw = await readFile(stateFile, "utf8");
      const parsed = JSON.parse(raw);
      return {
        nextId: Number(parsed.nextId ?? 1),
        notes: new Map(Object.entries(parsed.notes ?? {})),
      };
    } catch (error) {
      if (error.code !== "ENOENT") {
        throw error;
      }
    }
  }
  const notes = new Map();
  let nextId = 1;
  for (const seed of initialNotes) {
    const name = seed.name ?? `notes/seed-${nextId}`;
    nextId += 1;
    notes.set(name, {
      name,
      owner: seed.owner,
      title: seed.title.trim(),
      body: seed.body ?? "",
      createTimeIso: seed.createTimeIso ?? "2026-01-01T00:00:00.000Z",
    });
  }
  return { nextId, notes };
}

async function persistState(stateFile, state) {
  if (!stateFile) {
    return;
  }
  await mkdir(path.dirname(stateFile), { recursive: true });
  const tempPath = `${stateFile}.tmp-${process.pid}-${Date.now()}`;
  const serialized = JSON.stringify(
    { nextId: state.nextId, notes: Object.fromEntries(state.notes.entries()) },
    null,
    2,
  );
  await writeFile(tempPath, `${serialized}\n`, "utf8");
  await rename(tempPath, stateFile);
}

function toNoteMessage(entry) {
  return create(NoteSchema, {
    name: entry.name,
    title: entry.title,
    body: entry.body,
    createTime: timestampFromDate(new Date(entry.createTimeIso)),
  });
}

async function handleCreate({ body, protoMode, subject, state, stateFile, response, cors }) {
  let req;
  try {
    req = decodeRequest(CreateNoteRequestSchema, body, protoMode);
  } catch (error) {
    writeConnectError(response, cors, 400, "invalid_argument", String(error.message));
    return;
  }
  const violations = validateCreateNote(req);
  if (violations.length > 0) {
    writeConnectError(
      response,
      cors,
      400,
      "invalid_argument",
      "validation failed",
      encodeViolations(violations),
    );
    return;
  }
  const name = `notes/note-${state.nextId}`;
  state.nextId += 1;
  const entry = {
    name,
    owner: subject,
    title: req.note.title.trim(),
    body: req.note.body,
    createTimeIso: new Date().toISOString(),
  };
  state.notes.set(name, entry);
  await persistState(stateFile, state);
  const reply = create(CreateNoteResponseSchema, {
    note: toNoteMessage(entry),
  });
  writeSuccess(response, cors, CreateNoteResponseSchema, reply, protoMode);
}

function handleGet({ body, protoMode, subject, state, response, cors }) {
  let req;
  try {
    req = decodeRequest(GetNoteRequestSchema, body, protoMode);
  } catch (error) {
    writeConnectError(response, cors, 400, "invalid_argument", String(error.message));
    return;
  }
  if (!NOTE_NAME_PATTERN.test(req.name)) {
    const details = encodeViolations([
      {
        field: "name",
        id: "string.pattern",
        message: "value does not match regex pattern `^notes/[a-z0-9-]{1,64}$`",
      },
    ]);
    writeConnectError(response, cors, 400, "invalid_argument", "validation failed", details);
    return;
  }
  const entry = state.notes.get(req.name);
  if (!entry || entry.owner !== subject) {
    writeConnectError(response, cors, 404, "not_found", "note not found");
    return;
  }
  const reply = create(GetNoteResponseSchema, { note: toNoteMessage(entry) });
  writeSuccess(response, cors, GetNoteResponseSchema, reply, protoMode);
}

async function routeRpc(request, response, { allowedOrigin, state, stateFile }) {
  const cors = corsHeaders(request.headers.origin, allowedOrigin);
  if (request.method === "OPTIONS") {
    response.writeHead(204, cors).end();
    return;
  }
  const pathname = new URL(request.url, "http://localhost").pathname;
  if (request.method !== "POST" || (pathname !== CREATE_PATH && pathname !== GET_PATH)) {
    writeConnectError(response, cors, 404, "unimplemented", "unknown method");
    return;
  }
  const subject = extractSubject(request.headers.authorization);
  if (!subject) {
    writeConnectError(response, cors, 401, "unauthenticated", "authentication required");
    return;
  }
  if (subject === "fault-unavailable") {
    writeConnectError(response, cors, 503, "unavailable", "storage unavailable");
    return;
  }
  const body = await readBody(request);
  const protoMode = isProtoContent(request.headers["content-type"]);
  if (pathname === CREATE_PATH) {
    await handleCreate({
      body,
      protoMode,
      subject,
      state,
      stateFile,
      response,
      cors,
    });
    return;
  }
  handleGet({ body, protoMode, subject, state, response, cors });
}

export async function startStatefulNotesConnectFake({
  allowedOrigin,
  stateFile = null,
  initialNotes = [],
} = {}) {
  const state = await loadState(stateFile, initialNotes);
  await persistState(stateFile, state);
  const server = createServer((request, response) => {
    void routeRpc(request, response, { allowedOrigin, state, stateFile }).catch((error) => {
      const cors = corsHeaders(request.headers.origin, allowedOrigin);
      writeConnectError(response, cors, 500, "internal", `internal error: ${String(error)}`);
    });
  });
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const url = `http://127.0.0.1:${server.address().port}`;
  return {
    url,
    stateFile,
    snapshot: () => Object.fromEntries(state.notes.entries()),
    close: () =>
      new Promise((resolve) => {
        server.close(resolve);
        server.closeAllConnections();
      }),
  };
}
