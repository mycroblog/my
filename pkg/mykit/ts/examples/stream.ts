// SPDX-License-Identifier: MIT
import { runStream, staticToken } from "@mycroblog/mykit";

// Read the event stream until the signal aborts. It connects again whenever the stream ends. The server replays
// nothing and the library sends no Last-Event-ID, so read the timelines and the notifications again whenever
// onOpen says the stream was reopened.
export async function watch(signal: AbortSignal): Promise<void> {
  await runStream({
    server: "https://example.com",
    tokenSource: staticToken("access token"),
    signal,
    onOpen: (reconnected) => {
      if (reconnected) console.log("reconnected: read the home timeline and the notifications again");
    },
    onEvent: (event) => {
      switch (event.type) {
        case "update":
          console.log("new post", event.status.id);
          break;
        case "notification":
          console.log("notification", event.notification.type);
          break;
        case "announcement":
        case "announcement_removed":
          console.log("the announcements changed");
          break;
        case "unknown":
          break; // a newer server sent an event this library does not know
      }
    },
  });
}
