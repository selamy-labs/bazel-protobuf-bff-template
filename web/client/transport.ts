import { createClient, type Client, type Interceptor } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

import { NoteService } from "../../api/template/notes/v1/notes_pb.js";

/** Supplies the Authorization value. An adopter connects this to their sign-in. */
export type CredentialSource = () => string | null;

export type NotesClient = Client<typeof NoteService>;

/** What the composition decides for every call. */
export type ClientOptions = {
  /**
   * The page's patience: every call carries this deadline unless the call sets its own, and ends as
   * a deadline failure when it passes. The server caps any deadline at its own maximum.
   */
  deadlineMs: number;
};

/** The generated client over the Connect protocol, with JSON for debuggability. */
export function createNotesClient(
  baseUrl: string,
  credentials: CredentialSource,
  options: ClientOptions,
): NotesClient {
  const authorize: Interceptor = (next) => (request) => {
    const value = credentials();
    if (value !== null) {
      request.header.set("Authorization", value);
    }
    return next(request);
  };
  return createClient(
    NoteService,
    createConnectTransport({
      baseUrl,
      interceptors: [authorize],
      defaultTimeoutMs: options.deadlineMs,
    }),
  );
}
