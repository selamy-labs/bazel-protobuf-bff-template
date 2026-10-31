import { StrictMode, useMemo, useRef, useState, type ReactElement } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./App.js";
import { testCredential, TestIdentity } from "./test-identity.js";
import { createNotesClient } from "../client/transport.js";

/** True only in the testonly build. The bundler removes the test identity code from production. */
declare const __TEST_IDENTITY__: boolean;

function Production({ baseUrl }: { baseUrl: string }): ReactElement {
  // This repository ships no identity provider. Connect your sign-in here.
  const client = useMemo(() => createNotesClient(baseUrl, () => null), [baseUrl]);
  return (
    <>
      <p role="status">Sign-in is not configured in this build.</p>
      <App client={client} />
    </>
  );
}

function Development({ baseUrl }: { baseUrl: string }): ReactElement {
  const [subject, setSubject] = useState("ana");
  const current = useRef(subject);
  current.current = subject;
  const client = useMemo(() => createNotesClient(baseUrl, () => testCredential(current.current)), [baseUrl]);
  return (
    <>
      <TestIdentity subject={subject} onChange={setSubject} />
      <App client={client} />
    </>
  );
}

async function start(): Promise<void> {
  const response = await fetch("/config.json");
  const { bffUrl } = (await response.json()) as { bffUrl: string };
  const root = document.getElementById("root");
  if (root === null) {
    throw new Error("the page has no root element");
  }
  createRoot(root).render(
    <StrictMode>{__TEST_IDENTITY__ ? <Development baseUrl={bffUrl} /> : <Production baseUrl={bffUrl} />}</StrictMode>,
  );
}

void start();
