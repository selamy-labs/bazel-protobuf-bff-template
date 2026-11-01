import { Component, type ErrorInfo, type ReactElement, type ReactNode } from "react";

/**
 * A render error inside the boundary shows one message instead of a blank page. React only offers
 * this through a class component. The error itself stays in the console: the page shows no detail.
 */
export class ErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  override state = { failed: false };

  static getDerivedStateFromError(): { failed: boolean } {
    return { failed: true };
  }

  override componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error("the page failed to render", error, info.componentStack);
  }

  override render(): ReactNode {
    if (this.state.failed) {
      return (
        <p role="alert" data-testid="render-error">
          The page failed to render. Reload to continue.
        </p>
      );
    }
    return this.props.children;
  }
}

/** Throws while rendering. Only the development build can reach it, through a planted URL fragment. */
export function RenderFault(): ReactElement {
  throw new Error("planted render error");
}
