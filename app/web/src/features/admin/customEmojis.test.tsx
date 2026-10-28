// SPDX-License-Identifier: AGPL-3.0-only
// The custom emoji screen through the whole app (web-client/README.md T4, ADR-0051): an administrator with
// manage_custom_emojis registers an emoji with an image, changes, replaces and deletes it, and disables another
// server's; with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
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

  test("registers an emoji with an uploaded image, its category and other names", async () => {
    const purposes = acceptUploads();
    let sent: Schemas["CreateCustomEmojiRequest"] | undefined;
    server.use(
      http.post(`${apiBase}/admin/custom_emojis`, async ({ request }) => {
        sent = (await request.json()) as Schemas["CreateCustomEmojiRequest"];
        return HttpResponse.json(
          adminCustomEmojiFixture("e1", sent.shortcode, { category: "Blobs", aliases: ["cat"] }),
        );
      }),
    );
    const { user } = await renderApp({ path: "/admin/custom-emojis" });
    // docs/architecture/web-client/screens/admin-custom-emojis.md §3: the list first, adding is its one primary action.
    const list = await screen.findByRole("region", { name: "Registered custom emoji" });
    expect(sectionHeadings()).toEqual(["Registered custom emoji"]);
    await user.click(within(list).getByRole("button", { name: "Register a custom emoji" }));
    const form = within(list).getByRole("region", { name: "Register a custom emoji" });
    expect(within(form).getByRole("button", { name: "Choose an image" })).toHaveFocus();

    await user.type(within(form).getByRole("textbox", { name: "Shortcode" }), "blobcat");
    await user.click(within(form).getByRole("button", { name: "Register" }));
    expect(within(form).getByText("Choose an image.")).toBeInTheDocument();

    await user.upload(fileInput(form), nodeUploadFile("png", "blobcat.png", "image/png"));
    expect(within(form).getByText("Chosen: blobcat.png")).toBeInTheDocument();
    expect(within(form).queryByText("Choose an image.")).not.toBeInTheDocument();
    await user.type(within(form).getByRole("textbox", { name: "Category" }), "Blobs");
    await user.type(within(form).getByRole("textbox", { name: "Other names" }), "cat, kitty");
    await user.click(within(form).getByRole("button", { name: "Register" }));

    // The panel closes; the result shows above the list and focus is back on the button.
    expect(await within(list).findByRole("status")).toHaveTextContent(/is registered\./);
    expect(within(list).queryByRole("region", { name: "Register a custom emoji" })).not.toBeInTheDocument();
    expect(purposes).toEqual(["emoji"]);
    expect(sent).toEqual({
      shortcode: "blobcat",
      media_id: uploadedId,
      category: "Blobs",
      aliases: ["cat", "kitty"],
      visible_in_picker: true,
    });
    await user.click(within(list).getByRole("button", { name: "Register a custom emoji" }));
    expect(within(list).getByRole("textbox", { name: "Shortcode" })).toHaveValue("");
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
    const { user } = await renderApp({ path: "/admin/custom-emojis" });
    await user.click(await screen.findByRole("button", { name: "Register a custom emoji" }));
    const form = screen.getByRole("region", { name: "Register a custom emoji" });
    await user.upload(fileInput(form), nodeUploadFile("png", "blobcat.png", "image/png"));
    await user.type(within(form).getByRole("textbox", { name: "Shortcode" }), "blobcat");
    await user.click(within(form).getByRole("button", { name: "Register" }));

    expect(
      await within(form).findByText(
        "Could not register the custom emoji: A custom emoji of this server already has this shortcode.",
      ),
    ).toBeInTheDocument();
    expect(within(form).getByRole("textbox", { name: "Shortcode" })).toHaveValue("blobcat");
  });

  test("changes the settings of an emoji in a dialog", async () => {
    let change: Schemas["UpdateCustomEmojiRequest"] | undefined;
    server.use(
      http.get(`${apiBase}/admin/custom_emojis`, () =>
        HttpResponse.json({ items: [adminCustomEmojiFixture("e1", "blobcat")] }),
      ),
      http.patch(`${apiBase}/admin/custom_emojis/:id`, async ({ request }) => {
        change = (await request.json()) as Schemas["UpdateCustomEmojiRequest"];
        return HttpResponse.json(adminCustomEmojiFixture("e1", "blobcat", { ...change, category: "Blobs" }));
      }),
    );
    const { user } = await renderApp({ path: "/admin/custom-emojis" });
    await user.click(await screen.findByRole("button", { name: "Edit ⁦:blobcat:⁩" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit ⁦:blobcat:⁩" });
    await user.type(within(dialog).getByRole("textbox", { name: "Category" }), "Blobs");
    await user.click(within(dialog).getByRole("checkbox", { name: "Show in the picker" }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    expect(change).toEqual({ category: "Blobs", aliases: [], visible_in_picker: false, enabled: true });
    const list = screen.getByRole("region", { name: "Registered custom emoji" });
    expect(await within(list).findByText("Hidden")).toBeInTheDocument();
    expect(within(list).getByRole("status")).toHaveTextContent("is saved.");
  });

  test("deletes an emoji after asking, and moves focus on", async () => {
    let deleted = "";
    server.use(
      http.get(`${apiBase}/admin/custom_emojis`, () =>
        HttpResponse.json({
          items: [adminCustomEmojiFixture("e1", "blobcat"), adminCustomEmojiFixture("e2", "blobfox")],
        }),
      ),
      http.delete(`${apiBase}/admin/custom_emojis/:id`, ({ params }) => {
        deleted = String(params["id"]);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = await renderApp({ path: "/admin/custom-emojis" });
    await user.click(await screen.findByRole("button", { name: "Delete ⁦:blobcat:⁩" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Delete ⁦:blobcat:⁩?" });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(screen.queryByRole("heading", { level: 3, name: ":blobcat:" })).not.toBeInTheDocument();
    });
    expect(deleted).toBe("e1");
    expect(screen.getByRole("heading", { level: 3, name: ":blobfox:" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Edit ⁦:blobfox:⁩" })).toHaveFocus();
  });

  test("shows the emoji of other servers from the toolbar, where the reason to copy stands for adding", async () => {
    server.use(http.get(`${apiBase}/admin/custom_emojis`, () => HttpResponse.json({ items: [] })));
    const { user } = await renderApp({ path: "/admin/custom-emojis" });
    await user.click(await screen.findByRole("button", { name: /^This server/ }));
    await user.click(await screen.findByRole("option", { name: "Other servers" }));

    expect(
      await screen.findByRole("heading", { level: 3, name: "No custom emoji of other servers" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Register a custom emoji" })).not.toBeInTheDocument();
    expect(screen.getByText(/Copy one to use it on this server\./)).toBeInTheDocument();
  });

  test("disables an emoji of another server", async () => {
    const remote = adminCustomEmojiFixture("e3", "partyparrot", {
      local: false,
      domain: "remote.example",
      uri: "https://remote.example/emojis/1",
    });
    server.use(
      http.get(`${apiBase}/admin/custom_emojis`, ({ request }) =>
        HttpResponse.json({ items: new URL(request.url).searchParams.get("local") === "false" ? [remote] : [] }),
      ),
      http.patch(`${apiBase}/admin/custom_emojis/:id`, () => HttpResponse.json({ ...remote, enabled: false })),
    );
    const { user } = await renderApp({ path: "/admin/custom-emojis?origin=remote" });
    await user.click(await screen.findByRole("button", { name: "Disable ⁦:partyparrot:@remote.example⁩" }));

    expect(await screen.findByRole("button", { name: "Enable ⁦:partyparrot:@remote.example⁩" })).toBeInTheDocument();
    expect(screen.getByText("Disabled: shown as text")).toBeInTheDocument();
  });

  test("copies an emoji of another server under a new shortcode, and keeps a refusal in the dialog", async () => {
    const remote = adminCustomEmojiFixture("e3", "party-parrot", {
      local: false,
      domain: "remote.example",
      uri: "https://remote.example/emojis/1",
    });
    const sent: { id: string; body: Schemas["CopyCustomEmojiRequest"] }[] = [];
    server.use(
      http.get(`${apiBase}/admin/custom_emojis`, ({ request }) =>
        HttpResponse.json({ items: new URL(request.url).searchParams.get("local") === "false" ? [remote] : [] }),
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
    const { user } = await renderApp({ path: "/admin/custom-emojis?origin=remote" });
    await user.click(
      await screen.findByRole("button", { name: "Copy ⁦:party-parrot:@remote.example⁩ to this server" }),
    );
    const dialog = await screen.findByRole("dialog", { name: "Copy ⁦:party-parrot:@remote.example⁩ to this server" });
    // The remote shortcode breaks the rules of a local one, so a new one is asked for.
    const shortcode = within(dialog).getByRole("textbox", { name: "Shortcode" });
    expect(shortcode).toHaveValue("");
    await user.type(shortcode, "parrot");
    await user.click(within(dialog).getByRole("button", { name: "Copy" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "A custom emoji of this server already has this shortcode.",
    );

    await user.clear(shortcode);
    await user.type(shortcode, "party_parrot");
    await user.click(within(dialog).getByRole("button", { name: "Copy" }));
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    expect(sent).toEqual([
      { id: "e3", body: { shortcode: "parrot" } },
      { id: "e3", body: { shortcode: "party_parrot" } },
    ]);
    const list = screen.getByRole("region", { name: "Registered custom emoji" });
    expect(within(list).getByRole("status")).toHaveTextContent("is copied as ⁦:party_parrot:⁩.");
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
