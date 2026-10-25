// SPDX-License-Identifier: AGPL-3.0-only
// The server's maintenance mode as the app shell handles it (ADR-0084 §3, web-client/09-app-shell.md §9 "メンテナンス"):
// the client API and the OAuth endpoints answer every request with the 503 of the maintenance problem
// (MaintenanceError), the API client and the session report it here, and the shell shows the maintenance in place of
// the app. Nothing discards a token or signs out for it. The shell tries the API again after Retry-After, and the first
// response that is not an error ends the maintenance: the page loads again, back where the user was, with the session
// resumed from its cookie. A Service Worker that serves a cached shell during the maintenance ends up here the same way.
import { useSyncExternalStore } from "react";
import type { Api } from "./api/client";
import type { MaintenanceError } from "./errors";

export type MaintenanceOptions = {
  /** Loads the page again; `location.reload()` by default (tests record it, as jsdom cannot). */
  reload?: (() => void) | undefined;
};

/** Whether the server is under maintenance, as the last maintenance problem said: a store outside React. */
export class Maintenance {
  #current: MaintenanceError | null = null;
  #reloading = false;
  readonly #reload: () => void;
  readonly #listeners = new Set<() => void>();

  constructor({ reload }: MaintenanceOptions = {}) {
    this.#reload =
      reload ??
      (() => {
        window.location.reload();
      });
  }

  /** The last maintenance problem while the server is under maintenance, or null. */
  get current(): MaintenanceError | null {
    return this.#current;
  }

  /** Calls `listener` whenever `current` changes (the subscribe of useSyncExternalStore). */
  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  /** A request got the maintenance problem: the server is under maintenance, with what it says now. */
  report(error: MaintenanceError): void {
    if (this.#reloading) return;
    this.#current = error;
    for (const listener of this.#listeners) listener();
  }

  /**
   * A response that is not an error arrived. If the app showed the maintenance, the maintenance has ended: the page
   * loads again, so that the session, the page and its data start over as on any visit.
   */
  answered(): void {
    if (this.#current === null || this.#reloading) return;
    this.#reloading = true;
    this.#reload();
  }
}

/** The last maintenance problem while the server is under maintenance, or null; the component renders again when it changes. */
export function useMaintenance(maintenance: Maintenance): MaintenanceError | null {
  const current = () => maintenance.current;
  return useSyncExternalStore(maintenance.subscribe, current, current);
}

/**
 * Asks the server whether the maintenance goes on, with a request that needs no session (GET /instance). The API
 * client reports the answer to the Maintenance: a new maintenance problem, or the end of the maintenance. Returns the
 * error of the request (a MaintenanceError while the maintenance goes on, a NetworkError without an answer), or null
 * when the server answered.
 */
export async function checkMaintenance(api: Api): Promise<unknown> {
  try {
    await api.fetch.GET("/instance");
    return null;
  } catch (error) {
    return error;
  }
}
