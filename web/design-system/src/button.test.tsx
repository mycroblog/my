// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { Button } from "./button";
import { Preview } from "./preview";

test("a button is a secondary form button of type button by default", () => {
  const html = renderToStaticMarkup(<Button>Save draft</Button>);
  expect(html).toContain('type="button"');
  expect(html).toContain("bg-surface-raised");
  expect(html).toContain("h-control-form");
  expect(html).not.toContain("aria-busy");
});

test("a busy button keeps its label as its name and hides the spinner from assistive technology", () => {
  const html = renderToStaticMarkup(
    <Button kind="primary" busy>
      Post
    </Button>,
  );
  expect(html).toContain('aria-busy="true"');
  expect(html).toContain('aria-disabled="true"');
  expect(html).toContain('data-pending="true"');
  expect(html).toContain('<span class="text-transparent">Post</span>');
  expect(html).toMatch(/<span class="[^"]*animate-spin[^"]*" aria-hidden="true"><\/span>/);
});

test("a disabled button is disabled", () => {
  const html = renderToStaticMarkup(<Button isDisabled>Cancel</Button>);
  expect(html).toContain('disabled=""');
  expect(html).toContain('data-disabled="true"');
});

test("a preview shows a state at rest", () => {
  for (const [state, attribute] of [
    ["hover", "data-hovered"],
    ["pressed", "data-pressed"],
    ["focus-visible", "data-focus-visible"],
  ] as const) {
    const html = renderToStaticMarkup(
      <Preview state={state}>
        <Button>Save draft</Button>
      </Preview>,
    );
    expect(html).toContain(`${attribute}="true"`);
  }
  expect(renderToStaticMarkup(<Button>Save draft</Button>)).not.toContain("data-hovered=");
});
