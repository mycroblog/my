// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { backdrop, backdropFromTablet } from "./backdrop";
import { AttachmentRow, CharCount, ChoiceGroup, ComposerFrame, ComposerText, PollOption, UndoNotice } from "./composer";

const none = () => undefined;

test("the frame's head is the title, then close at its end; at its end the alert, then the tools, the count and the action", () => {
  const html = renderToStaticMarkup(
    <ComposerFrame
      title="New post"
      closeLabel="Close"
      onClose={none}
      onDismiss={none}
      layer={false}
      alert={<p>alert</p>}
      tools={<span>tools</span>}
      count={<span>count</span>}
      action={<button type="button">Post</button>}
    >
      <p>body</p>
    </ComposerFrame>,
  );
  expect(html).toMatch(/^<div role="dialog" aria-labelledby="[^"]+">/);
  expect(html.indexOf("New post")).toBeLessThan(html.indexOf('aria-label="Close"'));
  const order = ["body", "alert", "tools", "count", "Post"].map((s) => html.indexOf(s));
  expect(order).toEqual([...order].sort((a, b) => a - b));
  // On a phone, as tall as the viewport less the software keyboard.
  expect(html).toContain("h-[calc(100dvh-var(--keyboard-inset,env(keyboard-inset-height,0px)))]");
});

test("a modal's backdrop dims and blurs the page, and only dims with reduced transparency", () => {
  for (const b of [backdrop, backdropFromTablet]) {
    expect(b).toContain("bg-scrim");
    expect(b).toContain("backdrop-blur-[var(--dialog-blur)]");
    expect(b).toContain("[@media(prefers-reduced-transparency:reduce)]:backdrop-blur-none");
  }
});

test("the text marks the characters over the limit behind it, and says what is wrong under it", () => {
  const html = renderToStaticMarkup(
    <ComposerText value="abcdef" onChange={none} label="Text" placeholder="…" over={2} error="Too long" />,
  );
  expect(html).toMatch(/aria-hidden="true">abcd<mark class="[^"]*">ef<\/mark>/);
  expect(html).toContain('aria-invalid="true"');
  expect(html).toContain("Too long");
});

test("the count is muted until near the limit, strong past it, and read as words", () => {
  expect(renderToStaticMarkup(<CharCount left={100} near={20} label="100 left" />)).not.toContain("text-text");
  expect(renderToStaticMarkup(<CharCount left={5} near={20} label="5 left" />)).toContain("text-text");
  const over = renderToStaticMarkup(<CharCount left={-3} near={20} label="3 over" />);
  expect(over).toContain("font-label");
  expect(over).toContain('<span aria-hidden="true">-3</span><span class="sr-only">3 over</span>');
});

test("an image asks for its description, and a failed upload offers its retry", () => {
  const row = (state: "ready" | "failed", required = false) =>
    renderToStaticMarkup(
      <AttachmentRow
        src="a.png"
        state={state}
        status="The upload did not finish."
        retry={{ label: "Try again", onPress: none }}
        description=""
        onDescription={none}
        descriptionLabel="Description"
        placeholder="What it shows"
        missing={{ text: "No description yet", required }}
        removeLabel="Remove"
        onRemove={none}
      />,
    );
  expect(row("ready")).toContain(">Description</label>");
  expect(row("ready")).not.toContain('aria-invalid="true"');
  expect(row("ready", true)).toContain('aria-invalid="true"');
  expect(row("failed")).toContain('role="status"');
  expect(row("failed")).toContain("Try again");
  expect(row("failed")).not.toContain("<textarea");
});

test("a poll's option is headed by a radio's ring, or a checkbox's square with several choices", () => {
  const option = (multiple: boolean) =>
    renderToStaticMarkup(<PollOption value="" onChange={none} label="Option 1" multiple={multiple} maxLength={50} />);
  expect(option(false)).toContain("rounded-full");
  expect(option(true)).toContain("rounded-[3px]");
});

test("a group of choices marks the chosen one, and a choice that may not be made is disabled", () => {
  const html = renderToStaticMarkup(
    <ChoiceGroup
      title="Who can see it"
      items={[
        { id: "public", icon: "public", label: "Public" },
        { id: "local", icon: "local", label: "Local", disabled: true },
      ]}
      chosen="public"
      onChoose={none}
    />,
  );
  expect(html).toContain('role="radiogroup" aria-label="Who can see it"');
  expect(html).toMatch(/role="radio" aria-checked="true"[^>]*>.*Public/);
  expect(html).toMatch(/role="radio" aria-checked="false" disabled=""/);
});

test("the notice of a discarded post is a status with its action", () => {
  const html = renderToStaticMarkup(
    <UndoNotice action="Undo" onAction={none} inline>
      Post discarded
    </UndoNotice>,
  );
  expect(html).toMatch(/^<div class="[^"]*" role="status" dir="ltr"><span>Post discarded<\/span><button/);
  expect(html).toContain(">Undo</button>");
});
