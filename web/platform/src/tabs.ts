// SPDX-License-Identifier: AGPL-3.0-only

// The tabs of one browser as a group (ADR-0028 §18): Web Locks choose one tab to lead at a time, and a
// BroadcastChannel carries messages to the others. Both are in the browser floor of ADR-0025 §1 (Web Locks since
// Chrome 69, Firefox 96, Safari 15.4; BroadcastChannel since Chrome 54, Firefox 38, Safari 15.4), but Web Locks need a
// secure context: a page served over plain HTTP from another host than localhost has none, and its tabs act alone.

/** The tabs of the browser that joined a group of one name. */
export type TabGroup = {
  /**
   * Waits until this tab leads the group, then calls lead and leads until the promise it returns settles. lead learns
   * whether another tab led before it in this tab's life. It resolves once lead settles, or once signal aborts while
   * this tab still waits, in which case lead is never called.
   */
  lead(signal: AbortSignal, lead: (tookOver: boolean) => Promise<void>): Promise<void>;
  /** Sends a structured-cloneable message to the other tabs of the group; this tab does not receive it. */
  post(message: unknown): void;
  /** Receives the messages of the other tabs until the function it returns is called. */
  listen(listener: (message: unknown) => void): () => void;
  /** Leaves the group: no message is sent or received any more. */
  close(): void;
};

/** Joins the group `name` of the browser's tabs, or answers null when the browser has no Web Locks or BroadcastChannel. */
export function joinTabGroup(name: string): TabGroup | null {
  const locks = typeof navigator === "undefined" ? undefined : (navigator.locks as LockManager | undefined);
  if (locks === undefined || typeof BroadcastChannel === "undefined") return null;
  const channel = new BroadcastChannel(name);
  return {
    lead: async (signal, lead) => {
      // A lock that is free goes to this tab at once; one that another tab holds is waited for.
      const led = await locks.request(name, { ifAvailable: true }, async (lock): Promise<boolean> => {
        if (lock === null) return false;
        await lead(false);
        return true;
      });
      // signal may abort while the lock is awaited, which narrowing cannot see.
      const aborted = () => signal.aborted;
      if (led || aborted()) return;
      try {
        await locks.request(name, { signal }, () => lead(true));
      } catch (error) {
        if (!aborted()) throw error;
      }
    },
    post: (message) => {
      channel.postMessage(message);
    },
    listen: (listener) => {
      const receive = (event: MessageEvent) => {
        listener(event.data);
      };
      channel.addEventListener("message", receive);
      return () => {
        channel.removeEventListener("message", receive);
      };
    },
    close: () => {
      channel.close();
    },
  };
}
