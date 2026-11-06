// SPDX-License-Identifier: AGPL-3.0-only
// The custom emoji screen through the whole app (web-client/README.md T4, ADR-0051): an administrator with
// manage_custom_emojis registers an emoji with an image on its form's page, changes and deletes it on its own page,
// and disables and copies another server's; with MSW playing the API.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { mediaFixture, type Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { nodeUploadFile } from "../../test/multipart";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { sectionHeadings } from "../../test/sections";
import { adminCustomEmojiFixture } from "./test/handlers";

const uploadedId = "0199f0e0-0000-7000-8000-00000000e0a1";

/** Answers the image upload at once as processed, and records the purpose it was sent for. */
function acceptUploads(): string[] {
  const purposes: string[] = [];
  server.use(
    http.post(`${apiBase}/media`, async ({ request }) => {
      const body = await request.text();
      purposes.push(/name="purpose"\r\n\r\n([a-z]+)/.exec(body)?.[1] ?? "");
      return HttpResponse.json(mediaFixture(uploadedId), { status: 202 });
    }),
  );
  return purposes;
}

/** The hidden file input of a FileTrigger, which user-event uploads through as the file picker would. */
function fileInput(region: HTMLElement): HTMLInputElement {
  // eslint-disable-next-line testing-library/no-node-access -- FileTrigger's input is hidden and has no role.
  const input = region.querySelector<HTMLInputElement>('input[type="file"]');
  if (input === null) throw new Error("no file input");
  return input;
}

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

describe("the custom emoji screen", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_custom_emojis"]));
  });

  test("tells a viewer without the permission why it is empty", async () => {
    server.use(operatorHandler(["manage_settings"]));
    await renderApp({ path: "/admin/custom-emojis" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
  });

  test("lists the emoji with the pages of the screen, and links each row to its page", async () => {
    server.use(
      http.get(`${apiBase}/admin/custom_emojis`, () =>
        HttpResponse.json({ items: [adminCustomEmojiFixture("e1", "blobcat")] }),
      ),
    );
    await renderApp({ path: "/admin/custom-emojis" });
    // docs/architecture/web-client/screens/admin-custom-emojis.md §3: one region, its one primary action a link.
    const list = await screen.findByRole("region", { name: "Registered custom emoji" });
    expect(sectionHeadings()).toEqual(["Registered custom emoji"]);
    const pages = screen.getByRole("navigation", { name: "Custom emoji pages" });
    expect(within(pages).getByRole("link", { name: "Emoji" })).toHaveAttribute("aria-current", "page");
    expect(within(pages).getByRole("link", { name: "Requests" })).toHaveAttribute(
      "href",
      "/admin/custom-emojis/requests",
    );
    expect(within(list).getByRole("link", { name: "Register a custom emoji" })).toHaveAttribute(
      "href",
      "/admin/custom-emojis/new",
    );
    expect(within(list).getByRole("link", { name: ":blobcat:" })).toHaveAttribute("href", "/admin/custom-emojis/e1");
  });

  test("registers an emoji with an uploaded image, its category and other names, and opens its page", async () => {
    const purposes = acceptUploads();
    let sent: Schemas["CreateCustomEmojiRequest"] | undefined;
    const created = () => adminCustomEmojiFixture("e1", "blobcat", { category: "Blobs", aliases: ["cat", "kitty"] });
    server.use(
      http.post(`${apiBase}/admin/custom_emojis`, async ({ request }) => {
        sent = (await request.json()) as Schemas["CreateCustomEmojiRequest"];
        return HttpResponse.json(created());
      }),
      http.get(`${apiBase}/admin/custom_emojis/e1`, () => HttpResponse.json(created())),
    );
    const { user } = await renderApp({ path: "/admin/custom-emojis/new" });
    expect(await screen.findByRole("heading", { level: 1, name: "Register a custom emoji" })).toBeInTheDocument();

    await user.type(screen.getByRole("textbox", { name: "Shortcode" }), "blobcat");
    await user.click(screen.getByRole("button", { name: "Register" }));
    expect(screen.getByText("Choose an image.")).toBeInTheDocument();

    const main = screen.getByRole("main");
    await user.upload(fileInput(main), nodeUploadFile("png", "blobcat.png", "image/png"));
    expect(screen.getByText("Chosen: blobcat.png")).toBeInTheDocument();
    expect(screen.queryByText("Choose an image.")).not.toBeInTheDocument();
    await user.type(screen.getByRole("textbox", { name: "Category" }), "Blobs");
    await user.type(screen.getByRole("textbox", { name: "Other names" }), "cat, kitty");
    await user.click(screen.getByRole("button", { name: "Register" }));

    // The new emoji's page opens; its heading shows what was done.
    expect(await screen.findByRole("heading", { level: 1, name: "Custom emoji details" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "About this emoji" })).toHaveTextContent(":blobcat:");
    expect(purposes).toEqual(["emoji"]);
    expect(sent).toEqual({
      shortcode: "blobcat",
      media_id: uploadedId,
      category: "Blobs",
      aliases: ["cat", "kitty"],
      visible_in_picker: true,
    });
  });

  test("shows why the server refuses an emoji, and keeps the form", async () => {
    acceptUploads();
    server.use(
      http.post(`${apiBase}/admin/custom_emojis`, () =>
        HttpResponse.json(
          {
            type: "/api/v1/problems/invalid-custom-emoji",
            title: "The custom emoji cannot be saved.",
            status: 422,
            detail: "A custom emoji of this server already has this shortcode.",
            reason: "shortcode_taken",
          },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: "/admin/custom-emojis/new" });
    const shortcode = await screen.findByRole("textbox", { name: "Shortcode" });
    await user.upload(fileInput(screen.getByRole("main")), nodeUploadFile("png", "blobcat.png", "image/png"));
    await user.type(shortcode, "blobcat");
    await user.click(screen.getByRole("button", { name: "Register" }));

    expect(
      await screen.findByText(
        "Could not register the custom emoji: A custom emoji of this server already has this shortcode.",
      ),
    ).toBeInTheDocument();
    expect(shortcode).toHaveValue("blobcat");
  });

  test("saves the settings of an emoji on its page, which shows the answer", async () => {
    let change: Schemas["UpdateCustomEmojiRequest"] | undefined;
    server.use(
      http.get(`${apiBase}/admin/custom_emojis/e1`, () => HttpResponse.json(adminCustomEmojiFixture("e1", "blobcat"))),
      http.patch(`${apiBase}/admin/custom_emojis/:id`, async ({ request }) => {
        change = (await request.json()) as Schemas["UpdateCustomEmojiRequest"];
        return HttpResponse.json(adminCustomEmojiFixture("e1", "blobcat", { ...change, category: "Blobs" }));
      }),
    );
    const { user } = await renderApp({ path: "/admin/custom-emojis/e1" });
    const settings = await screen.findByRole("region", { name: "Settings" });
    expect(sectionHeadings()).toEqual([
      "About this emoji",
      "Settings",
      "Image",
      "Enable or disable",
      "Delete this emoji",
    ]);
    await user.type(within(settings).getByRole("textbox", { name: "Category" }), "Blobs");
    await user.click(within(settings).getByRole("checkbox", { name: "Show in the picker" }));
    await user.click(within(settings).getByRole("button", { name: "Save" }));

    expect(await within(settings).findByRole("status")).toHaveTextContent("is saved.");
    expect(change).toEqual({ category: "Blobs", aliases: [], visible_in_picker: false });
    expect(within(screen.getByRole("region", { name: "About this emoji" })).getByText("Hidden")).toBeInTheDocument();
  });

  test("deletes an emoji after asking, and goes back to the list", async () => {
    let deleted = "";
    server.use(
      http.get(`${apiBase}/admin/custom_emojis/e1`, () => HttpResponse.json(adminCustomEmojiFixture("e1", "blobcat"))),
      http.get(`${apiBase}/admin/custom_emojis`, () =>
        HttpResponse.json({ items: deleted === "" ? [adminCustomEmojiFixture("e1", "blobcat")] : [] }),
      ),
      http.delete(`${apiBase}/admin/custom_emojis/:id`, ({ params }) => {
        deleted = String(params["id"]);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = await renderApp({ path: "/admin/custom-emojis/e1" });
    await user.click(await screen.findByRole("button", { name: "Delete ⁦:blobcat:⁩" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Delete ⁦:blobcat:⁩?" });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(await screen.findByRole("region", { name: "Registered custom emoji" })).toBeInTheDocument();
    expect(deleted).toBe("e1");
    expect(await screen.findByText("⁦:blobcat:⁩ is deleted.")).toBeInTheDocument();
  });

  test("shows the emoji of other servers from the toolbar, where the reason to copy stands for adding", async () => {
    server.use(http.get(`${apiBase}/admin/custom_emojis`, () => HttpResponse.json({ items: [] })));
    const { user } = await renderApp({ path: "/admin/custom-emojis" });
    await user.click(await screen.findByRole("button", { name: /^This server/ }));
    await user.click(await screen.findByRole("option", { name: "Other servers" }));

    expect(
      await screen.findByRole("heading", { level: 3, name: "No custom emoji of other servers" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Register a custom emoji" })).not.toBeInTheDocument();
    expect(screen.getByText(/Copy one to use it on this server\./)).toBeInTheDocument();
  });

  test("disables an emoji of another server from its row", async () => {
    const remote = adminCustomEmojiFixture("e3", "partyparrot", {
      local: false,
      domain: "remote.example",
      uri: "https://remote.example/emojis/1",
    });
    server.use(
      http.get(`${apiBase}/admin/custom_emojis`, ({ request }) =>
        HttpResponse.json({ items: new URL(request.url).searchParams.get("local") === "false" ? [remote] : [] }),
      ),
      http.post(`${apiBase}/admin/custom_emojis/:id/disable`, () =>
        HttpResponse.json({ ...remote, state: "disabled" }),
      ),
    );
    const { user } = await renderApp({ path: "/admin/custom-emojis?origin=remote" });
    await user.click(await screen.findByRole("button", { name: "Disable ⁦:partyparrot:@remote.example⁩" }));

    expect(await screen.findByRole("button", { name: "Enable ⁦:partyparrot:@remote.example⁩" })).toHaveFocus();
    expect(screen.getByText("Disabled: shown as text")).toBeInTheDocument();
  });

  test("disables and enables an emoji of this server on its page with one press", async () => {
    const emoji = adminCustomEmojiFixture("e1", "blobcat");
    server.use(
      http.get(`${apiBase}/admin/custom_emojis/e1`, () => HttpResponse.json(emoji)),
      http.post(`${apiBase}/admin/custom_emojis/:id/disable`, () => HttpResponse.json({ ...emoji, state: "disabled" })),
      http.post(`${apiBase}/admin/custom_emojis/:id/enable`, () => HttpResponse.json(emoji)),
    );
    const { user } = await renderApp({ path: "/admin/custom-emojis/e1" });
    const state = await screen.findByRole("region", { name: "Enable or disable" });
    await user.click(within(state).getByRole("button", { name: "Disable ⁦:blobcat:⁩" }));
    expect(await within(state).findByRole("status")).toHaveTextContent("⁦:blobcat:⁩ is disabled.");

    await user.click(await within(state).findByRole("button", { name: "Enable ⁦:blobcat:⁩" }));

    expect(await within(state).findByRole("button", { name: "Disable ⁦:blobcat:⁩" })).toBeInTheDocument();
  });

  test("copies an emoji of another server under a new shortcode, keeps a refusal, and opens the copy", async () => {
    const remote = adminCustomEmojiFixture("e3", "party-parrot", {
      local: false,
      domain: "remote.example",
      uri: "https://remote.example/emojis/1",
    });
    const sent: { id: string; body: Schemas["CopyCustomEmojiRequest"] }[] = [];
    server.use(
      http.get(`${apiBase}/admin/custom_emojis/e3`, () => HttpResponse.json(remote)),
      http.get(`${apiBase}/admin/custom_emojis/e4`, () =>
        HttpResponse.json(adminCustomEmojiFixture("e4", "party_parrot")),
      ),
      http.post(`${apiBase}/admin/custom_emojis/:id/copy`, async ({ params, request }) => {
        const body = (await request.json()) as Schemas["CopyCustomEmojiRequest"];
        sent.push({ id: String(params["id"]), body });
        if (sent.length === 1) {
          return HttpResponse.json(
            {
              type: "/api/v1/problems/invalid-custom-emoji",
              title: "The custom emoji cannot be saved.",
              status: 422,
              detail: "A custom emoji of this server already has this shortcode.",
              reason: "shortcode_taken",
            },
            { status: 422, headers: { "Content-Type": "application/problem+json" } },
          );
        }
        return HttpResponse.json(adminCustomEmojiFixture("e4", body.shortcode ?? remote.shortcode));
      }),
    );
    const { user } = await renderApp({ path: "/admin/custom-emojis/e3" });
    const copy = await screen.findByRole("region", { name: "Copy to this server" });
    expect(sectionHeadings()).toEqual(["About this emoji", "Copy to this server", "Enable or disable"]);
    // The remote shortcode breaks the rules of a local one, so a new one is asked for.
    const shortcode = within(copy).getByRole("textbox", { name: "Shortcode" });
    expect(shortcode).toHaveValue("");
    await user.type(shortcode, "parrot");
    await user.click(within(copy).getByRole("button", { name: "Copy" }));
    expect(await within(copy).findByRole("alert")).toHaveTextContent(
      "A custom emoji of this server already has this shortcode.",
    );

    await user.clear(shortcode);
    await user.type(shortcode, "party_parrot");
    await user.click(within(copy).getByRole("button", { name: "Copy" }));
    expect(await screen.findByRole("region", { name: "Settings" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "About this emoji" })).toHaveTextContent(":party_parrot:");
    expect(sent).toEqual([
      { id: "e3", body: { shortcode: "parrot" } },
      { id: "e3", body: { shortcode: "party_parrot" } },
    ]);
  });

  test("shows a failure to list the emoji as the screen's error", async () => {
    server.use(
      http.get(`${apiBase}/admin/custom_emojis`, () =>
        problem(403, "/api/v1/problems/missing-permission", "Forbidden"),
      ),
    );
    await renderApp({ path: "/admin/custom-emojis" });

    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
  });
});
