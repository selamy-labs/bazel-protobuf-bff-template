/**
 * One request in flight per owner. A newer start cancels the older one, an older one that still
 * settles is ignored, and disposal (the page unmounting) cancels whatever is in flight and ignores
 * everything after it. Nothing here knows React: the page owns one Requests and asks it for each call.
 */

/** One started request: the signal its call must carry, and whether it is still the latest. */
export type Attempt = { readonly signal: AbortSignal; current(): boolean };

/** How a call ended, when its result still matters. */
export type Settled<T> = { ok: true; value: T } | { ok: false; reason: unknown };

export class Requests {
  #controller: AbortController | null = null;
  #disposed = false;

  /** Cancels the request in flight, if any, and starts a new one. After dispose, it is born cancelled. */
  start(): Attempt {
    this.cancel();
    const controller = new AbortController();
    if (this.#disposed) {
      controller.abort();
    } else {
      this.#controller = controller;
    }
    return { signal: controller.signal, current: () => this.#controller === controller };
  }

  /** Cancels the request in flight, if any. Its result, if it still arrives, is ignored. */
  cancel(): void {
    this.#controller?.abort();
    this.#controller = null;
  }

  /** Cancels and refuses every later start: the owner is gone. */
  dispose(): void {
    this.#disposed = true;
    this.cancel();
  }
}

/**
 * Runs one call as the latest request. The result is null when the call is no longer the latest by
 * the time it settles, whether it was superseded, cancelled or disposed: the owner must then do nothing.
 */
export async function latest<T>(
  requests: Requests,
  call: (signal: AbortSignal) => Promise<T>,
): Promise<Settled<T> | null> {
  const attempt = requests.start();
  let settled: Settled<T>;
  try {
    settled = { ok: true, value: await call(attempt.signal) };
  } catch (reason) {
    settled = { ok: false, reason };
  }
  return attempt.current() ? settled : null;
}
