import { StrictMode, useMemo, useRef, useState, type ReactElement } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./App.js";
import { ErrorBoundary, RenderFault } from "./ErrorBoundary.js";
import { SignInPanel, useSignIn } from "./signin/SignIn.js";
import type { SignInConfig } from "./signin/oidc.js";
import { testCredential, TestIdentity } from "./test-identity.js";
import { createNotesClient } from "../client/transport.js";

/** True only in the testonly build. The bundler removes the test identity code from production. */
declare const __TEST_IDENTITY__: boolean;

/** The page's patience for one call. The server caps any deadline at its own maximum (bff/transport). */
const deadlineMs = 2000;

/** What the site's config.json says: where the server is, and the issuer to sign in with, if any. */
type PageConfig = { bffUrl: string; signIn?: SignInConfig };

function Production({ config }: { config: PageConfig }): ReactElement {
  // The sign-in is the configured issuer's (spec 012 FR-016); with none configured, every call goes without a
  // credential and the server refuses it.
  const signIn = useSignIn(config.signIn);
  const client = useMemo(
    () => createNotesClient(config.bffUrl, signIn.credential, { deadlineMs }),
    [config.bffUrl, signIn.credential],
  );
  return (
    <>
      <SignInPanel signIn={signIn} />
      <ErrorBoundary>
        <App client={client} />
      </ErrorBoundary>
    </>
  );
}

/** The development build throws while rendering when the page is opened with this fragment. */
const plantedRenderError = "#planted-render-error";

function Development({ baseUrl }: { baseUrl: string }): ReactElement {
  const [subject, setSubject] = useState("ana");
  const current = useRef(subject);
  current.current = subject;
  const client = useMemo(
    () => createNotesClient(baseUrl, () => testCredential(current.current), { deadlineMs }),
    [baseUrl],
  );
  return (
    <>
      <TestIdentity subject={subject} onChange={setSubject} />
      <ErrorBoundary>
        <App client={client} />
        {window.location.hash === plantedRenderError ? <RenderFault /> : null}
      </ErrorBoundary>
    </>
  );
}

async function start(): Promise<void> {
  // Relative to the page, so a site under a base path reads its own configuration (spec 012 FR-019).
  const response = await fetch("config.json");
  const config = (await response.json()) as PageConfig;
  const root = document.getElementById("root");
  if (root === null) {
    throw new Error("the page has no root element");
  }
  createRoot(root).render(
    <StrictMode>
      {__TEST_IDENTITY__ ? <Development baseUrl={config.bffUrl} /> : <Production config={config} />}
    </StrictMode>,
  );
}

void start();
