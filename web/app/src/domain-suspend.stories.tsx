// SPDX-License-Identifier: AGPL-3.0-only

// The confirmation of a domain's suspend (FR-702, FR-717) of @my/app-features, open as the form of a new action opens
// it after its press, on the fake server of the moderators' screens (domains.ts of @my/e2e): what the suspend stops,
// the follows it ends as the server counts them (photos.example, whose photographer follows the scenario's reader), the
// domain to type before Suspend; and the same while the server counts, and when it could not count. Typing the domain
// and confirming are the route tests' and the E2E's.

import { AppDataProvider, createAppData } from "@my/app-data";
import { SuspendConfirm } from "@my/app-features";
import { createFakeApi, fakeOrigin } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

/** How the server answers the count: as the fake counts, never, or with a failure. */
type Count = "counted" | "counting" | "failed";

/** The fake server of the moderators' screens, read by the scenario's moderator, its count answered as `count` says. */
function createSuspendApp(count: Count) {
  const fake = createFakeApi({ viewer: "marco" });
  const serve = (request: Request): Promise<Response> => {
    if (new URL(request.url).pathname.endsWith("/admin/domain_actions/impact")) {
      if (count === "counting") return new Promise(() => undefined);
      if (count === "failed")
        return Promise.resolve(
          new Response(JSON.stringify({ type: "about:blank", title: "Internal Server Error", status: 500 }), {
            status: 500,
            headers: { "Content-Type": "application/problem+json" },
          }),
        );
    }
    return fake.serve(request);
  };
  return createAppData(createClient({ server: fakeOrigin, retry: { disabled: true }, fetch: serve }));
}

/** The confirmation in the flow of the page, its count answered as `count` says. */
function Specimen({ count }: { count: Count }) {
  const [data] = useState(() => createSuspendApp(count));
  return (
    <AppDataProvider data={data}>
      <SuspendConfirm
        domain="photos.example"
        busy={false}
        layer={false}
        onCancel={() => undefined}
        onConfirm={() => undefined}
      />
    </AppDataProvider>
  );
}

const countOf = (parameters: StoryContext["parameters"]): Count => (parameters as { count?: Count }).count ?? "counted";

const meta = {
  title: "Moderation/Domains/Suspend confirmation",
  parameters: { layout: "centered" },
  render: (_, { id, parameters }) => <Specimen key={id} count={countOf(parameters)} />,
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The follows the suspend ends, counted: one, of one person here; Suspend waits for the domain typed. */
export const Counted: Story = { parameters: { count: "counted" } };
/** While the server counts the follows. */
export const Counting: Story = { parameters: { count: "counting" } };
/** The server could not count them: every follow ends, uncounted. */
export const CountFailed: Story = { parameters: { count: "failed" } };
