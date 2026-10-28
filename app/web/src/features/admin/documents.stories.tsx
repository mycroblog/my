// SPDX-License-Identifier: AGPL-3.0-only
// The terms and policies (/admin/documents; web-client/screens/admin-documents.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { instanceFixture, type Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { instanceTextFixture } from "./test/handlers";
import { adminAnswering, adminFailingOn, adminLoadingOn, adminScreen, withoutPermission } from "./test/stories";

/** A long document in Markdown: `sections` numbered sections of a few sentences each. */
function longDocument(title: string, sections: number): string {
  const body = Array.from(
    { length: sections },
    (_, index) =>
      `## ${String(index + 1)}. Section ${String(index + 1)}\n\nWhen you use this server you agree to keep to these terms. ` +
      "We may change them; we announce every change at least thirty days before it takes effect, and you may leave " +
      "the server and take your posts with you at any time.\n\n- Be kind.\n- Describe your images.\n- Mark sensitive media.",
  );
  return [`# ${title}`, ...body].join("\n\n");
}

/** The server's documents: `texts`, and the pages that publish them. */
function documents(texts: Schemas["AdminInstanceText"][]) {
  return [
    http.get(`${apiBase}/admin/instance_texts`, ({ request }) =>
      withViewer(request, () => HttpResponse.json({ items: texts })),
    ),
    http.get(`${apiBase}/instance`, () =>
      HttpResponse.json({
        ...instanceFixture,
        terms_url: "https://my.example/about/terms",
        privacy_policy_url: "https://my.example/about/privacy",
      }),
    ),
  ];
}

const meta = {
  title: "Screens/Admin/Documents",
  ...adminScreen(
    "/admin/documents",
    documents([
      instanceTextFixture("terms", longDocument("Terms of service", 12)),
      instanceTextFixture("privacy_policy", longDocument("Privacy policy", 6)),
    ]),
  ),
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Long terms of service and a privacy policy, each with when it was saved and its page. */
export const Normal: Story = {};

/** A new server, which has published neither document. */
export const Empty: Story = adminAnswering();

/** A moderator without manage_settings. */
export const MissingPermission: Story = withoutPermission;

/** The documents have not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/instance_texts");

/** The server failed to answer the documents. */
export const ServerError: Story = adminFailingOn("get", "/admin/instance_texts");

/** In Japanese, with the documents in Japanese. */
export const Japanese: Story = {
  ...inLocale("ja"),
  ...adminAnswering(
    ...documents([
      instanceTextFixture(
        "terms",
        "# 利用規約\n\n## 1. はじめに\n\nこのサーバーを利用することで、本規約に同意したものとみなします。",
      ),
      instanceTextFixture("privacy_policy", "# プライバシーポリシー\n\n収集する情報と、その利用目的を説明します。"),
    ]),
  ),
};

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
