// SPDX-License-Identifier: AGPL-3.0-only

// The kinds of change of screen: a button per kind runs it on a stage between two sample screens, a timeline and a
// post opened from it, whose avatar is a shared element. Reduced motion stills them all; a right-to-left locale mirrors
// push and pop. Whether the browser runs view transitions comes from the host's loader, which asks @my/platform.

import { rootPx, text, transition } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { flushSync } from "react-dom";
import { Avatar } from "./avatar";
import { Button } from "./button";
import { runTransition, Shared, transitionKinds, type TransitionKind } from "./motion";
import { TransitionStage } from "./preview";

const rem = (px: number) => `${px / rootPx}rem`;

const caption = {
  color: "var(--color-muted-text)",
  fontSize: `${text.caption.size}rem`,
  lineHeight: text.caption.lineHeight,
} as const;

const people = ["Aiko Tanaka", "Ben Ortiz", "Chidi Okafor"] as const;

const about: Record<TransitionKind, string> = {
  push: `deeper, to what the reader opened · ${transition.duration.push} ms`,
  pop: `back out of it · ${transition.duration.pop} ms`,
  modal: `raised over the screen · ${transition.duration.modal} ms`,
  replace: `a sibling in its place · ${transition.duration.replace} ms`,
  none: "no motion",
};

function Timeline() {
  return (
    <div style={{ display: "grid", gap: rem(12) }}>
      <strong>Timeline</strong>
      {people.map((name, i) => (
        <div key={name} style={{ alignItems: "center", display: "flex", gap: rem(10) }}>
          {i === 0 ? (
            <Shared name="motion-sample">
              <Avatar name={name} size="post" decorative />
            </Shared>
          ) : (
            <Avatar name={name} size="post" decorative />
          )}
          <span>
            {name}
            <br />
            <span style={caption}>A post of {name.split(" ")[0]}</span>
          </span>
        </div>
      ))}
    </div>
  );
}

function Post() {
  const [name] = people;
  return (
    <div style={{ display: "grid", gap: rem(12), justifyItems: "start" }}>
      <strong>Post</strong>
      <Shared name="motion-sample">
        <Avatar name={name} size="preview" decorative />
      </Shared>
      <span>{name}</span>
      <span>A post of {name.split(" ")[0]}, opened.</span>
    </div>
  );
}

function Motion({ viewTransitions }: { viewTransitions: boolean }) {
  const [opened, setOpened] = useState(false);
  const run = (kind: TransitionKind) => {
    void runTransition(
      kind,
      () => {
        flushSync(() => {
          setOpened((o) => !o);
        });
      },
      viewTransitions,
    );
  };
  return (
    <div style={{ display: "flex", flexWrap: "wrap", gap: rem(24) }}>
      <div style={{ display: "grid", gap: rem(8), alignContent: "start" }}>
        {transitionKinds.map((kind) => (
          <div key={kind} style={{ alignItems: "center", display: "flex", gap: rem(12) }}>
            <Button
              kind="secondary"
              size="form"
              onPress={() => {
                run(kind);
              }}
            >
              {kind}
            </Button>
            <span style={caption}>{about[kind]}</span>
          </div>
        ))}
        <span style={caption}>Each runs from the screen shown to the other one.</span>
      </div>
      <div
        style={{
          border: "1px solid #ccc",
          borderRadius: 8,
          boxSizing: "border-box",
          height: rem(280),
          maxWidth: "100%",
          padding: rem(16),
          width: rem(300),
        }}
      >
        <TransitionStage>{opened ? <Post /> : <Timeline />}</TransitionStage>
      </div>
    </div>
  );
}

const meta = { title: "Motion" } satisfies Meta;
export default meta;

export const ChangeOfScreen: StoryObj = {
  render: (_, { loaded }) => <Motion viewTransitions={loaded["viewTransitions"] === true} />,
};
