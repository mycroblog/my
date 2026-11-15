// SPDX-License-Identifier: AGPL-3.0-only

// The composer's parts fetched as chunks of their own (its view, its editor): React.lazy keeps a failed fetch for good,
// so a lazy part here is made anew when it is tried again; the boundary around it shows, in its place, that it could
// not be loaded, and its Try again draws it anew, which fetches it again.

import { Component, Fragment, lazy, type ComponentType, type ReactNode } from "react";

/** A component fetched as it is first drawn, which a failed fetch leaves until it is tried again. */
export type RetryingLazy<P> = {
  /** The component: the same one until its fetch failed and it was tried again. */
  readonly get: () => ComponentType<P>;
  /** After a failed fetch, the next `get` gives a component that fetches again. */
  readonly retry: () => void;
};

/**
 * A component fetched by `load`. A failed one keeps failing until `retry` (React draws it again before its boundary
 * shows the failure, and a new one then would fetch, and fail, without end).
 */
export function retryingLazy<P extends object>(load: () => Promise<ComponentType<P>>): RetryingLazy<P> {
  let current: ComponentType<P> | undefined;
  let failed = false;
  return {
    get: () =>
      (current ??= lazy(() =>
        load().then(
          (component) => ({ default: component }),
          (error: unknown) => {
            failed = true;
            throw error;
          },
        ),
      )),
    retry: () => {
      if (!failed) return;
      failed = false;
      current = undefined;
    },
  };
}

/**
 * Its children, or, when they fail to be drawn (a chunk that could not be fetched), `fallback` with what draws them
 * anew, after `onRetry`; `onFailed` hears the failure, for a caller that says so elsewhere.
 */
export class ChunkBoundary extends Component<
  {
    fallback: (retry: () => void) => ReactNode;
    onRetry: () => void;
    onFailed?: (() => void) | undefined;
    children: ReactNode;
  },
  { failed: boolean; attempt: number }
> {
  override state = { failed: false, attempt: 0 };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  override componentDidCatch() {
    this.props.onFailed?.();
  }

  override render() {
    if (this.state.failed)
      return this.props.fallback(() => {
        this.props.onRetry();
        this.setState((s) => ({ failed: false, attempt: s.attempt + 1 }));
      });
    // Each attempt mounts its children anew, which choose their component again.
    return <Fragment key={this.state.attempt}>{this.props.children}</Fragment>;
  }
}
