// A valid caller of the generated client. compile_gate.mjs compiles it as written, then with one
// call changed at a time. Each change must fail on that call.
import { createClient, type Transport } from "@connectrpc/connect";

import { NoteService } from "../../template/notes/v1/notes_pb.js";

export async function create(transport: Transport): Promise<string> {
  const client = createClient(NoteService, transport);
  const { note } = await client.createNote({ note: { title: "A title", body: "text" } });
  const found = await client.getNote({ name: note?.name ?? "" });
  return found.note?.title ?? "";
}
