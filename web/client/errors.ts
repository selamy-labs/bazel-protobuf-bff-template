import { Code, ConnectError } from "@connectrpc/connect";

import { ViolationsSchema } from "../../api/buf/validate/validate_pb.js";

/** What the page shows for a failed call. Each contract error is one kind. */
export type Failure =
  | { kind: "fields"; fields: Record<string, string>; general: string[] }
  | { kind: "not_found" }
  | { kind: "unauthenticated" }
  | { kind: "unavailable" }
  | { kind: "deadline" }
  | { kind: "internal"; correlationId: string | null }
  | { kind: "unknown" };

const correlationPattern = /correlation id ([0-9a-f]+)/;

/** Fields the page has an input for. A violation on anything else stays visible as a general message. */
const inputs = new Set(["note.title", "note.body", "name"]);

function violations(error: ConnectError): Failure {
  const fields: Record<string, string> = {};
  const general: string[] = [];
  for (const detail of error.findDetails(ViolationsSchema)) {
    for (const violation of detail.violations) {
      const path = (violation.field?.elements ?? []).map((element) => element.fieldName).join(".");
      if (inputs.has(path)) {
        fields[path] = violation.message;
      } else {
        general.push(violation.message);
      }
    }
  }
  if (Object.keys(fields).length === 0 && general.length === 0) {
    general.push(error.rawMessage);
  }
  return { kind: "fields", fields, general };
}

/** A fetch that never reached a server fails with a TypeError, whatever code wraps it. */
function neverReachedServer(reason: unknown, error: ConnectError): boolean {
  return reason instanceof TypeError || error.cause instanceof TypeError;
}

/** Turns anything a client call throws into the failure the page shows. */
export function toFailure(reason: unknown): Failure {
  const error = ConnectError.from(reason);
  if (neverReachedServer(reason, error)) {
    return { kind: "unavailable" };
  }
  switch (error.code) {
    case Code.InvalidArgument:
      return violations(error);
    case Code.NotFound:
      return { kind: "not_found" };
    case Code.Unauthenticated:
      return { kind: "unauthenticated" };
    case Code.Unavailable:
      return { kind: "unavailable" };
    case Code.DeadlineExceeded:
      return { kind: "deadline" };
    case Code.Internal:
      return { kind: "internal", correlationId: correlationPattern.exec(error.rawMessage)?.[1] ?? null };
    default:
      return { kind: "unknown" };
  }
}

/** The text for a failure that is not shown on a field. */
export function describe(failure: Failure): string {
  switch (failure.kind) {
    case "fields":
      return failure.general.join(" ");
    case "not_found":
      return "Not found.";
    case "unauthenticated":
      return "Authentication is required.";
    case "unavailable":
      return "The service is unavailable. Try again.";
    case "deadline":
      return "The request took too long. Try again.";
    case "internal":
      return failure.correlationId === null
        ? "Something went wrong."
        : `Something went wrong. Reference: ${failure.correlationId}`;
    case "unknown":
      return "Something went wrong.";
  }
}
