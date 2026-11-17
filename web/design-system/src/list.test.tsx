// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { I18nProvider } from "react-aria-components";
import { renderToStaticMarkup } from "react-dom/server";
import { Badge, ProfileHead } from "./profile";
import { ThreadReply } from "./thread";

test("a reply indents by its depth, up to three steps, and only where its column is wide enough", () => {
  const indent = (depth: number) => renderToStaticMarkup(<ThreadReply depth={depth}>x</ThreadReply>);
  expect(indent(1)).not.toContain("@reply:ms-");
  expect(indent(2)).toContain("@reply:ms-screen-reply-step");
  expect(indent(4)).toContain("@reply:ms-[calc(3*var(--screen-reply-step))]");
  expect(indent(9)).toBe(indent(4));
});

test("a profile's head is named by its name, the avatar's row before the names, in the reader's direction", () => {
  const html = renderToStaticMarkup(
    <I18nProvider locale="ar-XB">
      <ProfileHead
        nameId="n"
        avatar={<span>A</span>}
        actions={<button>Follow</button>}
        name="Alice"
        handle="@alice@example.social"
        badges={<Badge>Follows you</Badge>}
      />
    </I18nProvider>,
  );
  expect(html).toMatch(/<section [^>]*aria-labelledby="n" dir="rtl"/);
  expect(html.indexOf(">Follow<")).toBeLessThan(html.indexOf(">Alice<"));
  expect(html).toContain('<bdi dir="ltr">@alice@example.social</bdi>');
  expect(html).toContain('<bdi dir="auto"');
});
