import { useEffect, useRef, useState, type FormEvent, type ReactElement } from "react";

import { describe, toFailure, type Failure } from "../client/errors.js";
import { latest, Requests } from "../client/lifecycle.js";
import type { NotesClient } from "../client/transport.js";
import type { Note } from "../../api/template/notes/v1/notes_pb.js";

/** What the page is doing, or last did. Each state renders differently, and none is implied. */
type View =
  | { state: "idle" }
  | { state: "loading"; operation: "create" | "open" }
  | { state: "note"; note: Note }
  | { state: "empty" }
  | { state: "failure"; failure: Failure };

function fieldError(view: View, path: string): string | undefined {
  if (view.state !== "failure" || view.failure.kind !== "fields") {
    return undefined;
  }
  return view.failure.fields[path];
}

function Outcome({ view }: { view: View }): ReactElement | null {
  switch (view.state) {
    case "idle":
      return null;
    case "loading":
      return (
        <p role="status" data-testid="loading">
          Working…
        </p>
      );
    case "note":
      return (
        <section data-testid="note" aria-live="polite">
          <h2>{view.note.title}</h2>
          <p data-testid="note-name">{view.note.name}</p>
          <p data-testid="note-body">{view.note.body}</p>
        </section>
      );
    case "empty":
      return (
        <p role="status" data-testid="empty">
          The server returned no note.
        </p>
      );
    case "failure": {
      const text = describe(view.failure);
      return text === "" ? null : (
        <p role="alert" data-testid="failure">
          {text}
        </p>
      );
    }
  }
}

function FieldMessage({ id, text }: { id: string; text: string | undefined }): ReactElement | null {
  return text === undefined ? null : (
    <p id={id} role="alert" data-testid={id}>
      {text}
    </p>
  );
}

export function App({ client }: { client: NotesClient }): ReactElement {
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [name, setName] = useState("");
  const [view, setView] = useState<View>({ state: "idle" });
  // One request at a time. Unmounting cancels the one in flight, and nothing is set afterwards.
  const requests = useRef(new Requests());
  useEffect(() => {
    const owned = requests.current;
    return () => owned.dispose();
  }, []);

  async function run(
    operation: "create" | "open",
    call: (signal: AbortSignal) => Promise<{ note?: Note | undefined }>,
  ): Promise<void> {
    setView({ state: "loading", operation });
    const settled = await latest(requests.current, call);
    if (settled === null) {
      return; // superseded, cancelled or unmounted: a newer request owns the view
    }
    if (!settled.ok) {
      setView({ state: "failure", failure: toFailure(settled.reason) });
    } else if (settled.value.note === undefined) {
      setView({ state: "empty" });
    } else {
      setView({ state: "note", note: settled.value.note });
    }
  }

  // A mutation is never duplicated: while anything is in flight, Create waits. A read may be
  // superseded: a newer Open cancels the older one, whose result is then ignored.
  const creating = view.state === "loading" && view.operation === "create";
  const loading = view.state === "loading";

  function create(event: FormEvent): void {
    event.preventDefault();
    if (loading) {
      return;
    }
    void run("create", (signal) => client.createNote({ note: { title, body } }, { signal }));
  }

  function open(event: FormEvent): void {
    event.preventDefault();
    if (creating) {
      return;
    }
    void run("open", (signal) => client.getNote({ name }, { signal }));
  }

  function clearField(path: string): void {
    if (fieldError(view, path) !== undefined) {
      setView({ state: "idle" });
    }
  }

  return (
    <main>
      <h1>Notes</h1>
      <form onSubmit={create} aria-label="New note">
        <label>
          Title
          <input
            name="title"
            value={title}
            aria-describedby="title-error"
            onChange={(event) => {
              setTitle(event.target.value);
              clearField("note.title");
            }}
          />
        </label>
        <FieldMessage id="title-error" text={fieldError(view, "note.title")} />
        <label>
          Body
          <textarea
            name="body"
            value={body}
            aria-describedby="body-error"
            onChange={(event) => {
              setBody(event.target.value);
              clearField("note.body");
            }}
          />
        </label>
        <FieldMessage id="body-error" text={fieldError(view, "note.body")} />
        <button type="submit" disabled={loading}>
          Create note
        </button>
      </form>
      <form onSubmit={open} aria-label="Open note">
        <label>
          Name
          <input
            name="name"
            value={name}
            placeholder="notes/…"
            aria-describedby="name-error"
            onChange={(event) => {
              setName(event.target.value);
              clearField("name");
            }}
          />
        </label>
        <FieldMessage id="name-error" text={fieldError(view, "name")} />
        <button type="submit" disabled={creating}>
          Open note
        </button>
      </form>
      <Outcome view={view} />
    </main>
  );
}
