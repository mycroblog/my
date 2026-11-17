// SPDX-License-Identifier: AGPL-3.0-only

// The Stream panel of the Storybook host: what the stream would bring a screen, as buttons beside the canvas, which
// shows the app alone. A screen's story names what it offers in `parameters.stream` (web/app/src/app.samples.tsx):
// the home timeline's new posts and reconnections, or a new notification. A press sends the event the story's app
// hears over Storybook's channel; its name is app.samples.tsx's too.

import { createElement, type CSSProperties } from "react";
import { addons, types, useChannel, useParameter } from "storybook/manager-api";

const panelId = "my/stream/panel";
const streamEvent = "my/stream:event";

type Stream = "home" | "notifications";
type Press = {
  label: string;
  event: { kind: "posts"; count: number } | { kind: "reconnect" } | { kind: "notification" };
};

const presses: Record<Stream, Press[]> = {
  home: [
    { label: "New post", event: { kind: "posts", count: 1 } },
    { label: "3 new posts", event: { kind: "posts", count: 3 } },
    { label: "Reconnect", event: { kind: "reconnect" } },
  ],
  notifications: [{ label: "Receive a notification", event: { kind: "notification" } }],
};

const panel: CSSProperties = { display: "flex", flexWrap: "wrap", gap: 8, padding: 12, fontSize: 13 };

// Written with createElement, not JSX: Storybook builds the manager with the classic JSX transform.
function StreamPanel() {
  const stream = useParameter<Stream | undefined>("stream", undefined);
  const emit = useChannel({});
  if (stream === undefined) return createElement("div", { style: panel }, "This story has no stream.");
  return createElement(
    "div",
    { style: panel },
    presses[stream].map(({ label, event }) =>
      createElement(
        "button",
        {
          key: label,
          type: "button",
          onClick: () => {
            emit(streamEvent, event);
          },
        },
        label,
      ),
    ),
  );
}

addons.register("my/stream", () => {
  addons.add(panelId, {
    type: types.PANEL,
    title: "Stream",
    render: ({ active }) => (active ? createElement(StreamPanel) : null),
  });
});
