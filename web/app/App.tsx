import { useState, type FormEvent, type ReactElement } from "react";

import { describe, toFailure, type Failure } from "../client/errors.js";
import type { NotesClient } from "../client/transport.js";
import type { Note } from "../../api/template/notes/v1/notes_pb.js";

type Result = { note: Note } | { failure: Failure } | null;

function fieldError(result: Result, path: string): string | undefined {
  if (result === null || !("failure" in result) || result.failure.kind !== "fields") {
    return undefined;
  }
  return result.failure.fields[path];
}

function Outcome({ result }: { result: Result }): ReactElement | null {
  if (result === null) {
    return null;
  }
  if ("note" in result) {
    return (
      <section data-testid="note" aria-live="polite">
        <h2>{result.note.title}</h2>
        <p data-testid="note-name">{result.note.name}</p>
        <p data-testid="note-body">{result.note.body}</p>
      </section>
    );
  }
  const text = describe(result.failure);
  return text === "" ? null : (
    <p role="alert" data-testid="failure">
      {text}
    </p>
  );
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
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<Result>(null);

  async function run(call: () => Promise<{ note?: Note | undefined }>): Promise<void> {
    setBusy(true);
    setResult(null);
    try {
      const { note } = await call();
      setResult(note === undefined ? { failure: { kind: "unknown" } } : { note });
    } catch (reason) {
      setResult({ failure: toFailure(reason) });
    } finally {
      setBusy(false);
    }
  }

  function create(event: FormEvent): void {
    event.preventDefault();
    void run(() => client.createNote({ note: { title, body } }));
  }

  function open(event: FormEvent): void {
    event.preventDefault();
    void run(() => client.getNote({ name }));
  }

  function clearField(path: string): void {
    if (fieldError(result, path) !== undefined) {
      setResult(null);
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
        <FieldMessage id="title-error" text={fieldError(result, "note.title")} />
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
        <FieldMessage id="body-error" text={fieldError(result, "note.body")} />
        <button type="submit" disabled={busy}>
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
        <FieldMessage id="name-error" text={fieldError(result, "name")} />
        <button type="submit" disabled={busy}>
          Open note
        </button>
      </form>
      <Outcome result={result} />
    </main>
  );
}
