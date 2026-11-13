// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { Avatar } from "./avatar";

// React's server renderer puts a preload link for the image before the avatar.
const render = (node: React.ReactElement) => renderToStaticMarkup(node).replace(/^<link [^>]*\/>/, "");

test("the avatar is named by the display name, or hidden when the name sits next to it", () => {
  const named = render(<Avatar name="Alice Chen" src="/a.png" size="post" />);
  expect(named).toMatch(/^<span[^>]*role="img"[^>]*aria-label="Alice Chen"/);
  expect(named).toContain('alt=""');
  const decorative = render(<Avatar name="Alice Chen" src="/a.png" size="post" decorative />);
  expect(decorative).toMatch(/^<span[^>]*aria-hidden="true"/);
  expect(decorative).not.toContain("aria-label");
});

test("without an image, the first letter of the name stands in, one grapheme in any script", () => {
  const initial = (name: string) => renderToStaticMarkup(<Avatar name={name} size="inline" />).replace(/<[^>]*>/g, "");
  expect(initial("ravi")).toBe("R");
  expect(initial("たくや")).toBe("た");
  expect(initial("佐藤 美香")).toBe("佐");
  expect(initial("@ Kenji")).toBe("K");
  expect(renderToStaticMarkup(<Avatar name="Ravi" size="inline" />)).not.toContain("<img");
});

test("the place gives the size", () => {
  for (const size of ["inline", "post", "preview"] as const) {
    expect(renderToStaticMarkup(<Avatar name="Ravi" size={size} />)).toContain(`size-avatar-${size}`);
  }
});
