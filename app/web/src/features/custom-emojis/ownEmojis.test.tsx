// SPDX-License-Identifier: AGPL-3.0-only
// The user's page of their custom emoji through the whole app (web-client/README.md T4, ADR-0070 §1–§3, ADR-0124): a user
// whose roles have create_custom_emojis finds it in the settings, sees what they used of their monthly limit and when it
// resets, requests an emoji with an
// image, is told why a request is refused, withdraws a pending request, changes and deletes an emoji they made, and
// requests a new image of it; a user without the permission is told why; with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { mediaFixture, type Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { nodeUploadFile } from "../../test/multipart";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { customEmojiRequestFixture, ownCustomEmojiFixture } from "./test/handlers";

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

/** GET /custom_emojis/mine answering `items`, `used`, `pending`, and `limit` (absent for none), resetting in November. */
function mineHandler(items: Schemas["OwnCustomEmoji"][], used: number, limit?: number, pending = 0) {
  return http.get(`${apiBase}/custom_emojis/mine`, () =>
    HttpResponse.json({
      items,
      used,
      pending,
      resets_at: "2026-11-01T00:00:00Z",
      ...(limit === undefined ? {} : { limit }),
    } satisfies Schemas["OwnCustomEmojiPage"]),
  );
}

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
  server.use(signedInHandler);
});

describe("the user's custom emoji page", () => {
  test("is in the settings only for a holder of create_custom_emojis, and tells others why it is empty", async () => {
    server.use(operatorHandler([]));
    const { unmount } = await renderApp({ path: "/settings" });
    expect(await screen.findByRole("link", { name: "Import and export" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Your custom emoji" })).not.toBeInTheDocument();
    unmount();

    await renderApp({ path: "/settings/custom-emojis" });
    expect(await screen.findByRole("heading", { level: 2, name: "You cannot use this screen" })).toBeInTheDocument();
  });

  test("is linked from the settings for a holder of create_custom_emojis", async () => {
    server.use(operatorHandler(["create_custom_emojis"]));
    await renderApp({ path: "/settings" });

    expect(await screen.findByRole("link", { name: "Your custom emoji" })).toHaveAttribute(
      "href",
      "/settings/custom-emojis",
    );
  });

  test("says what the user used of their monthly limit and when it resets, or that there is none", async () => {
    server.use(
      operatorHandler(["create_custom_emojis"]),
      mineHandler([ownCustomEmojiFixture("e1", "blobcat")], 2, 5, 1),
    );
    const { unmount } = await renderApp({ path: "/settings/custom-emojis" });
    const meter = await screen.findByRole("meter", { name: "Created this month" });
    expect(meter).toHaveAttribute("aria-valuetext", "2 of 5, 1 of them pending");
    const resetsAt = new Intl.DateTimeFormat("en", { dateStyle: "long", timeStyle: "short" }).format(
      new Date("2026-11-01T00:00:00Z"),
    );
    expect(screen.getByText(`Resets on ${resetsAt}`)).toBeInTheDocument();
    unmount();

    server.use(mineHandler([], 0));
    await renderApp({ path: "/settings/custom-emojis" });
    expect(
      await screen.findByText("You requested 0 custom emoji this month. Your roles set no limit."),
    ).toBeInTheDocument();
  });

  test("requests an emoji with an uploaded image", async () => {
    server.use(operatorHandler(["create_custom_emojis"]));
    const purposes = acceptUploads();
    let sent: Schemas["NewCustomEmojiRequest"] | undefined;
    server.use(
      http.post(`${apiBase}/custom_emoji_requests`, async ({ request }) => {
        sent = (await request.json()) as Schemas["NewCustomEmojiRequest"];
        return HttpResponse.json(customEmojiRequestFixture("r1", sent.shortcode ?? ""));
      }),
    );
    const { user } = await renderApp({ path: "/settings/custom-emojis" });
    const form = await screen.findByRole("region", { name: "Request a custom emoji" });

    await user.type(within(form).getByRole("textbox", { name: "Shortcode" }), "blobcat");
    await user.click(within(form).getByRole("button", { name: "Request" }));
    expect(within(form).getByRole("alert")).toHaveTextContent("Choose an image.");

    await user.upload(fileInput(form), nodeUploadFile("png", "blobcat.png", "image/png"));
    await user.click(within(form).getByRole("button", { name: "Request" }));

    expect(await within(form).findByText("⁦:blobcat:⁩ is requested. An administrator reviews it.")).toBeInTheDocument();
    expect(purposes).toEqual(["emoji"]);
    expect(sent).toEqual({ kind: "create", shortcode: "blobcat", media_id: uploadedId });
    expect(within(form).getByRole("textbox", { name: "Shortcode" })).toHaveValue("");
  });

  test("shows why the server refuses a request, such as a reached limit, and keeps the form", async () => {
    server.use(operatorHandler(["create_custom_emojis"]));
    acceptUploads();
    server.use(
      http.post(`${apiBase}/custom_emoji_requests`, () =>
        HttpResponse.json(
          {
            type: "/api/v1/problems/invalid-custom-emoji",
            title: "The custom emoji cannot be saved.",
            status: 422,
            detail: "You have already requested as many custom emoji this month as your monthly limit.",
            reason: "limit_reached",
          },
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: "/settings/custom-emojis" });
    const form = await screen.findByRole("region", { name: "Request a custom emoji" });
    await user.upload(fileInput(form), nodeUploadFile("png", "blobcat.png", "image/png"));
    await user.type(within(form).getByRole("textbox", { name: "Shortcode" }), "blobcat");
    await user.click(within(form).getByRole("button", { name: "Request" }));

    expect(await within(form).findByRole("alert")).toHaveTextContent(
      "Could not send the request: You have already requested as many custom emoji this month as your monthly limit.",
    );
    expect(within(form).getByRole("textbox", { name: "Shortcode" })).toHaveValue("blobcat");
  });

  test("lists the requests with the reason of a rejection, and withdraws a pending one", async () => {
    let withdrawn = "";
    server.use(
      operatorHandler(["create_custom_emojis"]),
      http.get(`${apiBase}/custom_emoji_requests`, () =>
        HttpResponse.json({
          items: [
            customEmojiRequestFixture(
              "r2",
              "blobfox",
              withdrawn === "" ? {} : { state: "withdrawn", decided_at: "2026-10-22T00:00:00Z" },
            ),
            customEmojiRequestFixture("r1", "blobcat", {
              state: "rejected",
              reason: "Too close to :blob:.",
              decided_at: "2026-10-21T12:00:00Z",
            }),
          ],
        }),
      ),
      http.post(`${apiBase}/custom_emoji_requests/:id/withdraw`, ({ params }) => {
        withdrawn = String(params["id"]);
        return HttpResponse.json(
          customEmojiRequestFixture(withdrawn, "blobfox", { state: "withdrawn", decided_at: "2026-10-22T00:00:00Z" }),
        );
      }),
    );
    const { user } = await renderApp({ path: "/settings/custom-emojis" });
    const region = await screen.findByRole("region", { name: "Your requests" });
    expect(within(region).getByText("Too close to :blob:.")).toBeInTheDocument();
    expect(within(region).getByText("Rejected")).toBeInTheDocument();
    expect(within(region).getAllByRole("button", { name: /Withdraw/ })).toHaveLength(1);

    await user.click(within(region).getByRole("button", { name: "Withdraw the request for ⁦:blobfox:⁩" }));
    expect(await within(region).findByText("The request for ⁦:blobfox:⁩ is withdrawn.")).toBeInTheDocument();
    expect(withdrawn).toBe("r2");
    expect(within(region).getByText("Withdrawn")).toBeInTheDocument();
    expect(within(region).queryByRole("button", { name: /Withdraw/ })).not.toBeInTheDocument();
  });

  test("changes the settings of an emoji the user made in a dialog", async () => {
    let change: Schemas["UpdateOwnCustomEmojiRequest"] | undefined;
    server.use(
      operatorHandler(["create_custom_emojis"]),
      mineHandler([ownCustomEmojiFixture("e1", "blobcat")], 1),
      http.patch(`${apiBase}/custom_emojis/mine/:id`, async ({ request }) => {
        change = (await request.json()) as Schemas["UpdateOwnCustomEmojiRequest"];
        return HttpResponse.json(ownCustomEmojiFixture("e1", "blobcat", { ...change, category: "Blobs" }));
      }),
    );
    const { user } = await renderApp({ path: "/settings/custom-emojis" });
    await user.click(await screen.findByRole("button", { name: "Edit ⁦:blobcat:⁩" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit ⁦:blobcat:⁩" });
    await user.type(within(dialog).getByRole("textbox", { name: "Category" }), "Blobs");
    await user.type(within(dialog).getByRole("textbox", { name: "Other names" }), "cat");
    await user.click(within(dialog).getByRole("checkbox", { name: "Show in the picker" }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    expect(change).toEqual({ category: "Blobs", aliases: ["cat"], visible_in_picker: false });
    const list = screen.getByRole("region", { name: "Custom emoji you made" });
    expect(await within(list).findByText("Hidden")).toBeInTheDocument();
    expect(within(list).getByText("⁦:blobcat:⁩ is saved.")).toBeInTheDocument();
  });

  test("deletes an emoji the user made after asking", async () => {
    let deleted = "";
    server.use(
      operatorHandler(["create_custom_emojis"]),
      http.get(`${apiBase}/custom_emojis/mine`, () =>
        HttpResponse.json({
          items:
            deleted === ""
              ? [ownCustomEmojiFixture("e1", "blobcat"), ownCustomEmojiFixture("e2", "blobfox")]
              : [ownCustomEmojiFixture("e2", "blobfox")],
          // Deleting an emoji frees nothing of the month's limit.
          used: 2,
          pending: 0,
          resets_at: "2026-11-01T00:00:00Z",
        }),
      ),
      http.delete(`${apiBase}/custom_emojis/mine/:id`, ({ params }) => {
        deleted = String(params["id"]);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = await renderApp({ path: "/settings/custom-emojis" });
    await user.click(await screen.findByRole("button", { name: "Delete ⁦:blobcat:⁩" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Delete ⁦:blobcat:⁩?" });
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(screen.queryByRole("heading", { level: 3, name: ":blobcat:" })).not.toBeInTheDocument();
    });
    expect(deleted).toBe("e1");
    expect(screen.getByRole("heading", { level: 3, name: ":blobfox:" })).toBeInTheDocument();
    expect(
      await screen.findByText("You requested 2 custom emoji this month. Your roles set no limit."),
    ).toBeInTheDocument();
  });

  test("requests a new image of an emoji the user made", async () => {
    acceptUploads();
    let sent: Schemas["NewCustomEmojiRequest"] | undefined;
    server.use(
      operatorHandler(["create_custom_emojis"]),
      mineHandler([ownCustomEmojiFixture("e1", "blobcat")], 1),
      http.post(`${apiBase}/custom_emoji_requests`, async ({ request }) => {
        sent = (await request.json()) as Schemas["NewCustomEmojiRequest"];
        return HttpResponse.json(customEmojiRequestFixture("r3", "blobcat", { kind: "image", emoji_id: "e1" }));
      }),
    );
    const { user } = await renderApp({ path: "/settings/custom-emojis" });
    const list = await screen.findByRole("region", { name: "Custom emoji you made" });
    await user.upload(fileInput(list), nodeUploadFile("png", "blobcat2.png", "image/png"));

    expect(await within(list).findByText(/A new image of ⁦:blobcat:⁩ is requested\./)).toBeInTheDocument();
    expect(sent).toEqual({ kind: "image", emoji_id: "e1", media_id: uploadedId });
  });
});
