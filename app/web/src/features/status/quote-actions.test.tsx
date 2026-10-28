// SPDX-License-Identifier: AGPL-3.0-only
// Quoting posts through the whole app (web-client/README.md T4, FR-114): the quote button and the form, who may quote the
// viewer's post, the posts that quote it, the revocation of an approval, and taking a quote away in an edit. The real
// router and QueryClient, MSW for the client API.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { apiBase, oauthPaths, tokenResponse } from "../../test/msw/http";
import { type Schemas, statusFixture, viewerStatusFixture } from "../../test/fixtures";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import type { Status } from "./api/status";

const bobsId = "0192f0c4-7a8e-7000-8000-0000000000f1";
const ownId = "0192f0c4-7a8e-7000-8000-0000000000f2";
const bobs: Status = { ...statusFixture(bobsId), content: "<p>Bob's thought</p>", quotable: true };
const own: Status = {
  ...viewerStatusFixture({ id: ownId, content: "<p>My post</p>" }),
  quotable: true,
  quote_policy: "public",
};

function signedIn() {
  server.use(http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")));
}

/** Answers GET /statuses/{id} with `statuses` by ID. */
function serve(...statuses: Status[]) {
  server.use(
    http.get(`${apiBase}/statuses/:id`, ({ params }) => {
      const found = statuses.find((s) => s.id === params["id"]);
      return HttpResponse.json(found ?? statusFixture(String(params["id"])));
    }),
  );
}

/** Finds a visible toast by its text (the announcer's live regions say it too). */
async function findToast(text: string) {
  return within(await screen.findByRole("region", { name: "Messages" })).findByText(text);
}

beforeEach(() => {
  // Only Date is faked: timers, MSW and user-event run as usual.
  vi.useFakeTimers({ toFake: ["Date"], now: new Date("2026-10-16T12:00:00Z") });
  sessionStorage.clear();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("quoting a post", () => {
  test("opens the form with the post, and posts the quote with the chosen policy", async () => {
    signedIn();
    serve(bobs);
    const sent: Schemas["CreateStatusRequest"][] = [];
    server.use(
      http.post(`${apiBase}/statuses`, async ({ request }) => {
        const body = (await request.json()) as Schemas["CreateStatusRequest"];
        sent.push(body);
        return HttpResponse.json(viewerStatusFixture({ content: `<p>${body.text}</p>` }));
      }),
    );
    const { user } = await renderApp({ path: `/@bob/status/${bobsId}` });

    await user.click(await screen.findByRole("button", { name: "Quote" }));
    const sheet = await screen.findByRole("dialog", { name: "New post" });
    expect(within(sheet).getByText("Quoting Bob")).toBeInTheDocument();
    expect(within(sheet).getByText("Bob's thought")).toBeInTheDocument();

    await user.click(within(sheet).getByRole("button", { name: "Anyone Who can quote" }));
    await user.click(await screen.findByRole("option", { name: "Followers" }));
    await user.type(within(sheet).getByRole("textbox", { name: "Your post" }), "Look at this");
    await user.click(within(sheet).getByRole("button", { name: "Post" }));

    await findToast("Posted.");
    expect(sent).toEqual([
      expect.objectContaining({ text: "Look at this", quoted_status_id: bobsId, quote_policy: "followers" }),
    ]);
  });

  test("can be dropped from the draft", async () => {
    signedIn();
    serve(bobs);
    const { user } = await renderApp({ path: `/@bob/status/${bobsId}` });

    await user.click(await screen.findByRole("button", { name: "Quote" }));
    const sheet = await screen.findByRole("dialog", { name: "New post" });
    await user.click(within(sheet).getByRole("button", { name: "Don't quote" }));
    expect(within(sheet).queryByText("Quoting Bob")).not.toBeInTheDocument();
  });

  test("is not offered for a post the viewer may not quote", async () => {
    signedIn();
    serve({ ...bobs, quotable: false });
    await renderApp({ path: `/@bob/status/${bobsId}` });
    expect(await screen.findByRole("button", { name: "Quote" })).toBeDisabled();
  });
});

describe("the viewer's own post", () => {
  test("changes who may quote it from its menu", async () => {
    signedIn();
    serve(own);
    const sent: Schemas["SetQuotePolicyRequest"][] = [];
    server.use(
      http.put(`${apiBase}/statuses/:id/quote_policy`, async ({ request }) => {
        const body = (await request.json()) as Schemas["SetQuotePolicyRequest"];
        sent.push(body);
        return HttpResponse.json({ ...own, quote_policy: body.quote_policy });
      }),
    );
    const { user } = await renderApp({ path: `/@alice/status/${ownId}` });

    await user.click(await screen.findByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Change who can quote" }));
    const dialog = await screen.findByRole("dialog", { name: "Who can quote this post" });
    expect(within(dialog).getByRole("radio", { name: /^Anyone/ })).toBeChecked();
    await user.click(within(dialog).getByRole("radio", { name: /^Only you/ }));

    await findToast("Changed who can quote the post");
    expect(sent).toEqual([{ quote_policy: "nobody" }]);
    expect(within(dialog).getByRole("radio", { name: /^Only you/ })).toBeChecked();
  });

  test("lists the posts that quote it", async () => {
    signedIn();
    serve(own);
    const quoting: Status = {
      ...statusFixture("0192f0c4-7a8e-7000-8000-0000000000f3"),
      content: "<p>Quoting you</p>",
      quote: { state: "accepted", quoted_status: own },
    };
    server.use(
      http.get(`${apiBase}/statuses/:id/quotes`, () =>
        HttpResponse.json({ items: [quoting] } satisfies Schemas["StatusPage"]),
      ),
    );
    const { user } = await renderApp({ path: `/@alice/status/${ownId}` });

    await user.click(await screen.findByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Posts that quote this" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Quotes of your post" })).toBeInTheDocument();
    expect(await screen.findByText("Quoting you")).toBeInTheDocument();
  });

  test("takes its quote away in an edit", async () => {
    signedIn();
    const quoting: Status = { ...own, quote: { state: "accepted", quoted_status: bobs } };
    serve(quoting);
    const sent: Schemas["EditStatusRequest"][] = [];
    server.use(
      http.put(`${apiBase}/statuses/:id`, async ({ request }) => {
        const body = (await request.json()) as Schemas["EditStatusRequest"];
        sent.push(body);
        const { quote: _, ...rest } = quoting;
        return HttpResponse.json({ ...rest, edited_at: "2026-10-16T12:00:00Z" });
      }),
    );
    const { user } = await renderApp({ path: `/@alice/status/${ownId}` });

    await user.click(await screen.findByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Edit" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit post" });
    await user.click(await within(dialog).findByRole("checkbox", { name: /^Remove the quoted post/ }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await findToast("Post edited");
    expect(sent).toEqual([expect.objectContaining({ remove_quote: true })]);
  });
});

describe("a quote of the viewer's post", () => {
  test("takes the viewer's post out of the quote from the quoting post's menu", async () => {
    signedIn();
    const quoting: Status = { ...bobs, quote: { state: "accepted", quoted_status: own } };
    serve(quoting);
    const revoked: string[] = [];
    server.use(
      http.post(`${apiBase}/statuses/:id/quotes/:quoteId/revoke`, ({ params }) => {
        revoked.push(`${String(params["id"])}:${String(params["quoteId"])}`);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = await renderApp({ path: `/@bob/status/${bobsId}` });

    expect(await screen.findByRole("group", { name: "Quoted post" })).toHaveTextContent("My post");
    await user.click(await screen.findByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Remove my post from this quote" }));

    await findToast("Your post was removed from the quote");
    expect(revoked).toEqual([`${ownId}:${bobsId}`]);
    expect(await screen.findByRole("group", { name: "Quoted post" })).toHaveTextContent(
      "The author of the quoted post withdrew it from this quote.",
    );
  });

  test("offers no revocation for a quote of someone else's post", async () => {
    signedIn();
    serve({
      ...bobs,
      quote: { state: "accepted", quoted_status: statusFixture("0192f0c4-7a8e-7000-8000-0000000000f4") },
    });
    const { user } = await renderApp({ path: `/@bob/status/${bobsId}` });

    await user.click(await screen.findByRole("button", { name: "More actions" }));
    expect(await screen.findByRole("menuitem", { name: "Copy link" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Remove my post from this quote" })).not.toBeInTheDocument();
  });
});
