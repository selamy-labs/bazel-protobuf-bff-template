import { useCallback, useEffect, useRef, useState, type ReactElement } from "react";

import {
  begin,
  complete,
  current,
  returned,
  signOut as forget,
  SignInError,
  type Ports,
  type Reason,
  type Session,
  type SignInConfig,
} from "./oidc.js";

/** What the sign-in is doing, or last did. Each state renders differently, and none is implied. */
export type SignInState =
  | { status: "unconfigured" }
  | { status: "signed-out" }
  | { status: "completing" }
  | { status: "signed-in"; session: Session }
  | { status: "failed"; reason: Reason };

const messages: Record<Reason, string> = {
  discovery: "The sign-in service could not be reached. Try again.",
  denied: "Sign-in was declined.",
  state: "This sign-in did not start in this tab. Try again.",
  token: "The sign-in service issued no credential. Try again.",
  claims: "The credential issued is not for this site.",
};

/** The real browser. */
function browserPorts(): Ports {
  return {
    fetch: (input, init) => fetch(input, init),
    storage: sessionStorage,
    crypto,
    now: Date.now,
  };
}

/** The page's own address without query or fragment: where the issuer sends the browser back. */
function redirectUri(): string {
  return location.origin + location.pathname;
}

function initial(config: SignInConfig | undefined, ports: Ports): SignInState {
  if (config === undefined) {
    return { status: "unconfigured" };
  }
  if (returned(location.search) !== null) {
    return { status: "completing" };
  }
  const session = current(ports);
  return session === null ? { status: "signed-out" } : { status: "signed-in", session };
}

function failed(error: unknown): SignInState {
  return { status: "failed", reason: error instanceof SignInError ? error.reason : "token" };
}

export type SignIn = {
  state: SignInState;
  /** The credential for the generated client: the session's while it lasts, else none. */
  credential: () => string | null;
  signIn: () => void;
  signOut: () => void;
};

export function useSignIn(config: SignInConfig | undefined): SignIn {
  const [ports] = useState(browserPorts);
  const [state, setState] = useState<SignInState>(() => initial(config, ports));
  // A return is answered once: the effect runs once even where React mounts twice to check effects.
  const answered = useRef(false);
  useEffect(() => {
    const back = returned(location.search);
    if (config === undefined || back === null || answered.current) {
      return;
    }
    answered.current = true;
    // The query carried the issuer's answer; the address shown is the page's own again.
    history.replaceState(null, "", redirectUri());
    complete(config, back, ports)
      .then((session) => setState({ status: "signed-in", session }))
      .catch((error: unknown) => setState(failed(error)));
  }, [config, ports]);
  // The session may have ended since it was shown: a call then goes without a credential, and the panel says so.
  const credential = useCallback(() => {
    const session = current(ports);
    if (session === null) {
      setState((shown) => (shown.status === "signed-in" ? { status: "signed-out" } : shown));
    }
    return session?.credential ?? null;
  }, [ports]);
  const signIn = useCallback(() => {
    if (config === undefined) {
      return;
    }
    begin(config, redirectUri(), ports)
      .then((url) => location.assign(url))
      .catch((error: unknown) => setState(failed(error)));
  }, [config, ports]);
  const signOut = useCallback(() => {
    forget(ports);
    setState({ status: "signed-out" });
  }, [ports]);
  return { state, credential, signIn, signOut };
}

export function SignInPanel({ signIn }: { signIn: SignIn }): ReactElement {
  const { state } = signIn;
  const button = (
    <button type="button" onClick={signIn.signIn}>
      Sign in
    </button>
  );
  switch (state.status) {
    case "unconfigured":
      return <p role="status">Sign-in is not configured in this build.</p>;
    case "signed-out":
      return (
        <aside data-testid="sign-in">
          <p role="status">Not signed in.</p>
          {button}
        </aside>
      );
    case "completing":
      return (
        <aside data-testid="sign-in">
          <p role="status">Signing in…</p>
        </aside>
      );
    case "signed-in":
      return (
        <aside data-testid="sign-in">
          <p role="status" data-testid="signed-in">
            Signed in as {state.session.subject}
          </p>
          <button type="button" onClick={signIn.signOut}>
            Sign out
          </button>
        </aside>
      );
    case "failed":
      return (
        <aside data-testid="sign-in">
          <p role="alert">{messages[state.reason]}</p>
          {button}
        </aside>
      );
  }
}
