// SPDX-License-Identifier: AGPL-3.0-only
// The new-post form through the whole app (web-client/README.md T4): the real router and QueryClient, MSW for the
// server. jsdom has no matchMedia, so the layout is the phone's (a button that opens a sheet) unless a
// test stubs it; it has no createImageBitmap / OffscreenCanvas either, so the image tests stub those
// browser APIs and check what the client did with them.
import { Button } from "@my/ui";
import { createMemoryHistory, createRootRouteWithContext, createRouter, Outlet } from "@tanstack/react-router";
import { isInaccessible, screen, waitFor, within } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { delay, http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test, vi } from "vitest";
import type { RouterContext } from "../../lib/query";
import { apiBase, oauthPaths, problem, tokenResponse } from "../../test/msw/http";
import {
  accountFixture,
  instanceFixture,
  preferencesFixture,
  relationshipFixture,
  remoteAccountFixture,
  viewerFixture,
  viewerStatusFixture,
} from "../../test/fixtures";
import { uploadedMediaId } from "./test/handlers";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { ComposeEntry, ComposeProvider, useReply } from ".";

/** Resumes a session with the token the default handlers accept. */
function signedIn() {
  server.use(http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")));
}

type Sent = { body: Record<string, unknown>; idempotencyKey: string | null };

/** Records every POST /statuses; `answers` are used in turn, then the default handler answers. */
function recordPosts(...answers: (() => Response)[]) {
  const sent: Sent[] = [];
  server.use(
    http.post(`${apiBase}/statuses`, async ({ request }) => {
      sent.push({
        body: (await request.clone().json()) as Record<string, unknown>,
        idempotencyKey: request.headers.get("Idempotency-Key"),
      });
      const answer = answers.shift();
      return answer === undefined ? undefined : answer();
    }),
  );
  return sent;
}

/** Finds a visible toast by its text (the announcer's live regions say it too). */
async function findToast(text: string) {
  return within(await screen.findByRole("region", { name: "Messages" })).findByText(text);
}

/** Whether the toast of `message` (its text) carries a celebrated moment (web-client §21.5) on its icon. */
function isCelebrating(message: HTMLElement): boolean {
  // The toast's icon is decorative and has no role.
  return message.closest("li")?.querySelector("[data-celebrating]") != null;
}

/** Opens the sheet from the navigation's button and returns it. */
async function openSheet(user: Awaited<ReturnType<typeof renderApp>>["user"]) {
  await user.click((await screen.findAllByRole("button", { name: "Write a post" }))[0] as HTMLElement);
  return screen.findByRole("dialog", { name: "New post" });
}

beforeEach(() => {
  sessionStorage.clear();
});

describe("the new-post form", () => {
  test("is not offered while signed out", async () => {
    await renderApp({ path: "/local" });

    expect(await screen.findByRole("heading", { level: 1, name: "Local timeline" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Write a post" })).not.toBeInTheDocument();
  });

  test("posts the text with its content warning, visibility and language, with an Idempotency-Key", async () => {
    signedIn();
    const sent = recordPosts();
    const { user } = await renderApp();
    const sheet = await openSheet(user);

    const text = await within(sheet).findByRole("textbox", { name: "Your post" });
    expect(text).toHaveFocus();
    await user.type(text, "Hello, world");
    expect(within(sheet).getByText("488 characters left")).toBeInTheDocument();
    const warning = within(sheet).getByRole("button", { name: "Add a content warning" });
    expect(warning).toHaveAttribute("aria-pressed", "false");
    await user.click(warning);
    expect(warning).toHaveAttribute("aria-pressed", "true");
    await user.type(within(sheet).getByRole("textbox", { name: "Content warning" }), "Spoilers");
    // The warning counts toward the instance's limit.
    expect(within(sheet).getByText("480 characters left")).toBeInTheDocument();

    await user.click(within(sheet).getByRole("button", { name: "Visibility: Public" }));
    const choices = await screen.findByRole("radiogroup", { name: "Who can see this post?" });
    expect(
      within(choices)
        .getAllByRole("radio")
        .map((radio) => radio.getAttribute("value")),
    ).toEqual(["public", "local", "unlisted", "private", "direct"]);
    expect(within(choices).getByRole("radio", { name: "This server" })).toHaveAccessibleDescription(
      "People signed in here, and your followers on other servers. Shown on this server's local timeline.",
    );
    await user.click(within(choices).getByRole("radio", { name: "Followers" }));
    await user.click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() => {
      expect(within(sheet).getByRole("button", { name: "Visibility: Followers" })).toHaveFocus();
    });

    await user.click(within(sheet).getByRole("button", { name: "English Language" }));
    await user.click(await screen.findByRole("option", { name: "Japanese" }));
    expect(within(sheet).getByRole("button", { name: "Japanese Language" })).toBeInTheDocument();

    await user.click(within(sheet).getByRole("button", { name: "Post" }));

    expect(await findToast("Posted.")).toBeInTheDocument();
    expect(sent).toEqual([
      {
        body: { text: "Hello, world", spoiler_text: "Spoilers", visibility: "private", language: "ja" },
        idempotencyKey: expect.stringMatching(/^[\da-f-]{36}$/) as unknown,
      },
    ]);
    // The sheet closes; the next draft keeps the visibility and language.
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    const next = await openSheet(user);
    expect(within(next).getByRole("textbox", { name: "Your post" })).toHaveValue("");
    expect(within(next).getByRole("button", { name: "Visibility: Followers" })).toBeInTheDocument();
  });

  test("celebrates the account's first post in its toast, and only the first (web-client §21.5)", async () => {
    signedIn();
    server.use(
      http.get(`${apiBase}/accounts/verify_credentials`, () =>
        HttpResponse.json({ ...viewerFixture, statuses_count: 0 }),
      ),
    );
    const { user } = await renderApp();

    let sheet = await openSheet(user);
    await user.type(await within(sheet).findByRole("textbox", { name: "Your post" }), "My first post");
    await user.click(within(sheet).getByRole("button", { name: "Post" }));
    const first = await findToast("Posted your first post. Welcome.");
    // The toast's icon carries the moment for a little while.
    expect(isCelebrating(first)).toBe(true);

    sheet = await openSheet(user);
    await user.type(within(sheet).getByRole("textbox", { name: "Your post" }), "And another");
    await user.click(within(sheet).getByRole("button", { name: "Post" }));
    const second = await findToast("Posted.");
    expect(isCelebrating(second)).toBe(false);
  });

  test("puts the visibility first and the other tools after it under the text, each with its current value", async () => {
    signedIn();
    const { user } = await renderApp();
    const sheet = await openSheet(user);
    const text = await within(sheet).findByRole("textbox", { name: "Your post" });

    const images = within(sheet).getByRole("button", { name: "Add images, video, or audio" });
    expect(images).toHaveAccessibleDescription(
      "Up to 4 files: JPEG, PNG, GIF, or WebP images, MP4 or WebM video, or MP3, Ogg, or M4A audio. Location and other metadata are removed.",
    );
    expect(within(sheet).getByRole("button", { name: "Add a content warning" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
    const visibility = within(sheet).getByRole("button", { name: "Visibility: Public" });
    expect(visibility).toHaveTextContent("Public");
    expect(visibility).toHaveAccessibleDescription("Anyone, here and on other servers. Shown on the public timelines.");
    expect(within(sheet).getByRole("button", { name: "English Language" })).toHaveTextContent("English");
    // In the order of reading: the text, then the visibility, which decides most about the post, then the other
    // tools, then the submit button (screens/compose.md).
    const follows = (a: Element, b: Element) => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
    expect(follows(text, visibility)).toBe(true);
    expect(follows(visibility, images)).toBe(true);
    expect(follows(images, within(sheet).getByRole("button", { name: "Post" }))).toBe(true);

    // Keyboard focus shows the tooltip of an icon-only button, and the button keeps its description. React
    // Aria hides what is outside the modal sheet from the accessibility tree, the tooltip's container with it.
    // The emoji picker's button is in the corner of the text, before the tools (ADR-0091 §4).
    expect(text).toHaveFocus();
    await user.tab();
    expect(within(sheet).getByRole("button", { name: "Add emoji" })).toHaveFocus();
    expect(await screen.findByRole("tooltip", { hidden: true })).toHaveTextContent("Add emoji");
    await user.tab();
    expect(visibility).toHaveFocus();
    await user.tab();
    expect(images).toHaveFocus();
    expect(await screen.findByRole("tooltip", { hidden: true })).toHaveTextContent("Add images, video, or audio");
    expect(images).toHaveAccessibleDescription(/^Up to 4 files/);
    await user.tab();
    expect(await screen.findByRole("tooltip", { hidden: true })).toHaveTextContent("Add a poll");
    await user.tab();
    expect(await screen.findByRole("tooltip", { hidden: true })).toHaveTextContent("Add a content warning");
  });

  test("shows the name and current value of the visibility, quote policy and language as tooltips on keyboard focus, and not while the list is open", async () => {
    signedIn();
    const { user } = await renderApp();
    const sheet = await openSheet(user);
    await within(sheet).findByRole("textbox", { name: "Your post" });

    const visibility = within(sheet).getByRole("button", { name: "Visibility: Public" });
    // From the text: the emoji button comes first, then the visibility.
    for (let i = 0; i < 2; i++) await user.tab();
    expect(visibility).toHaveFocus();
    expect(await screen.findByRole("tooltip", { hidden: true })).toHaveTextContent("Visibility: Public");
    // Opening the popover hides it, and the choice updates it.
    await user.click(visibility);
    await waitFor(() => {
      expect(screen.queryByRole("tooltip", { hidden: true })).not.toBeInTheDocument();
    });
    await user.click(await screen.findByRole("radio", { name: /Followers/ }));
    await user.click(screen.getByRole("button", { name: "Done" }));
    // Focus returns to the button; leave and come back to see the new value.
    await waitFor(() => {
      expect(within(sheet).getByRole("button", { name: "Visibility: Followers" })).toHaveFocus();
    });
    await user.tab();
    await user.tab({ shift: true });
    expect(within(sheet).getByRole("button", { name: "Visibility: Followers" })).toHaveFocus();
    expect(await screen.findByRole("tooltip", { hidden: true })).toHaveTextContent("Visibility: Followers");

    // Past images, poll and content warning.
    for (let i = 0; i < 4; i++) await user.tab();
    expect(within(sheet).getByRole("button", { name: /Who can quote/ })).toHaveFocus();
    expect(await screen.findByRole("tooltip", { hidden: true })).toHaveTextContent(/^Quote: /);
    await user.tab();
    expect(within(sheet).getByRole("button", { name: "English Language" })).toHaveFocus();
    expect(await screen.findByRole("tooltip", { hidden: true })).toHaveTextContent("Language: English");
    await user.keyboard("{Enter}");
    expect(await screen.findByRole("listbox")).toBeInTheDocument();
    expect(screen.queryByRole("tooltip", { hidden: true })).not.toBeInTheDocument();
  });

  test("posts a poll, and attaches no files to a post with one (FR-112)", async () => {
    signedIn();
    const sent = recordPosts();
    const { user } = await renderApp();
    const sheet = await openSheet(user);
    await user.type(await within(sheet).findByRole("textbox", { name: "Your post" }), "Which one?");

    const toggle = within(sheet).getByRole("button", { name: "Add a poll" });
    await user.click(toggle);
    expect(toggle).toHaveAttribute("aria-pressed", "true");
    // A post with a poll attaches no files.
    expect(within(sheet).getByRole("button", { name: "Add images, video, or audio" })).toBeDisabled();
    const poll = within(sheet).getByRole("group", { name: "Poll" });
    // Two empty options: the post waits for them, and says so beside the button when it is pressed (G23).
    const post = within(sheet).getByRole("button", { name: "Post" });
    expect(post).toBeEnabled();
    await user.click(post);
    expect(within(sheet).getByText("Fix the poll's options to post.")).toBeInTheDocument();
    await user.type(within(poll).getByRole("textbox", { name: "Option 1" }), "Tea");
    await user.type(within(poll).getByRole("textbox", { name: "Option 2" }), "tea ");
    // The reason goes once it no longer holds.
    expect(within(sheet).queryByText("Fix the poll's options to post.")).not.toBeInTheDocument();
    await user.clear(within(poll).getByRole("textbox", { name: "Option 2" }));
    await user.type(within(poll).getByRole("textbox", { name: "Option 2" }), "Tea ");
    expect(within(poll).getByText("Each option must be different.")).toBeInTheDocument();
    await user.clear(within(poll).getByRole("textbox", { name: "Option 2" }));
    await user.type(within(poll).getByRole("textbox", { name: "Option 2" }), "Coffee");

    // Options up to four; each beyond two can be removed.
    await user.click(within(poll).getByRole("button", { name: "Add an option" }));
    await user.type(within(poll).getByRole("textbox", { name: "Option 3" }), "Water");
    await user.click(within(poll).getByRole("button", { name: "Add an option" }));
    expect(within(poll).queryByRole("button", { name: "Add an option" })).not.toBeInTheDocument();
    await user.click(within(poll).getByRole("button", { name: "Remove option 4" }));
    expect(within(poll).getAllByRole("textbox")).toHaveLength(3);

    await user.click(within(poll).getByRole("button", { name: /Duration/ }));
    await user.click(await screen.findByRole("option", { name: "3 days" }));
    await user.click(within(poll).getByRole("checkbox", { name: "Allow choosing several options" }));
    await user.click(within(poll).getByRole("checkbox", { name: "Hide the votes until the poll ends" }));
    await user.click(post);

    expect(await findToast("Posted.")).toBeInTheDocument();
    expect(sent.map((s) => s.body)).toEqual([
      {
        text: "Which one?",
        visibility: "public",
        language: "en",
        poll: { options: ["Tea", "Coffee", "Water"], expires_in: 3 * 24 * 3600, multiple: true, hide_totals: true },
      },
    ]);
  });

  test("opens the sheet with focus on the text, never on the dialog itself, while the form's data is slow", async () => {
    signedIn();
    server.use(
      http.get(`${apiBase}/instance`, async () => {
        await delay(200);
        return HttpResponse.json(instanceFixture);
      }),
    );
    const { user } = await renderApp();
    const focused: Element[] = [];
    const onFocusIn = (event: FocusEvent) => {
      if (event.target instanceof Element) focused.push(event.target);
    };
    document.addEventListener("focusin", onFocusIn);
    try {
      const sheet = await openSheet(user);

      // A sheet that focused itself on open would be focused again by React Aria 500 ms later if focus were
      // on the body then, taking focus from a popover in the form that is returning it to its trigger.
      expect(within(sheet).getByRole("textbox", { name: "Your post" })).toHaveFocus();
      expect(focused).not.toContain(sheet);
    } finally {
      document.removeEventListener("focusin", onFocusIn);
    }
  });

  test("keeps the draft when posting fails, and sends it again with the same Idempotency-Key", async () => {
    signedIn();
    const sent = recordPosts(() => problem(503, "about:blank", "The server is busy"));
    const { user } = await renderApp();
    const sheet = await openSheet(user);

    await user.type(await within(sheet).findByRole("textbox", { name: "Your post" }), "Do not lose me");
    await user.click(within(sheet).getByRole("button", { name: "Post" }));

    expect(await within(sheet).findByText("Could not send the post: The server is busy")).toBeInTheDocument();
    expect(within(sheet).getByRole("textbox", { name: "Your post" })).toHaveValue("Do not lose me");
    expect(
      screen.getAllByRole("alert").some((alert) => alert.textContent === "Could not send the post: The server is busy"),
    ).toBe(true);

    await user.click(within(sheet).getByRole("button", { name: "Post" }));

    expect(await findToast("Posted.")).toBeInTheDocument();
    expect(sent).toHaveLength(2);
    expect(sent[1]?.idempotencyKey).toBe(sent[0]?.idempotencyKey);
  });

  test("a failed post stays in the outbox when another draft was started meanwhile", async () => {
    signedIn();
    let fail: () => void = () => undefined;
    const failure = new Promise<void>((resolve) => {
      fail = resolve;
    });
    const sent = recordPosts();
    server.use(
      http.post(
        `${apiBase}/statuses`,
        async () => {
          await failure;
          return problem(500, "about:blank", "Broken");
        },
        { once: true },
      ),
    );
    const { user } = await renderApp();
    const sheet = await openSheet(user);

    await user.type(await within(sheet).findByRole("textbox", { name: "Your post" }), "First");
    await user.click(within(sheet).getByRole("button", { name: "Post" }));
    const outbox = await within(sheet).findByRole("list", { name: "Posts being sent" });
    expect(within(outbox).getByText("First")).toBeInTheDocument();
    expect(within(outbox).getByRole("progressbar", { name: "Sending…" })).toBeInTheDocument();
    await user.type(within(sheet).getByRole("textbox", { name: "Your post" }), "Second");
    fail();

    expect(await within(outbox).findByText("Could not send the post: Broken")).toBeInTheDocument();
    expect(within(sheet).getByRole("textbox", { name: "Your post" })).toHaveValue("Second");
    await user.click(within(outbox).getByRole("button", { name: "Send again" }));

    expect(await findToast("Posted.")).toBeInTheDocument();
    expect(sent.map((s) => s.body["text"])).toEqual(["First"]);
  });

  test("keeps the draft across a reload of the page", async () => {
    signedIn();
    const view = await renderApp();
    const sheet = await openSheet(view.user);
    await view.user.type(await within(sheet).findByRole("textbox", { name: "Your post" }), "Half-written");
    view.unmount();

    const { user } = await renderApp();
    const again = await openSheet(user);

    expect(await within(again).findByRole("textbox", { name: "Your post" })).toHaveValue("Half-written");
  });

  test("counts against the instance's limit and refuses text that is too long", async () => {
    signedIn();
    server.use(
      http.get(`${apiBase}/instance`, () =>
        HttpResponse.json({
          ...instanceFixture,
          limits: { max_post_characters: 10, require_media_description: false },
        }),
      ),
    );
    const { user } = await renderApp();
    const sheet = await openSheet(user);

    await user.type(await within(sheet).findByRole("textbox", { name: "Your post" }), "Twelve chars");

    expect(within(sheet).getByText("2 characters too many")).toBeInTheDocument();
    expect(within(sheet).getByRole("textbox", { name: "Your post" })).toHaveAttribute("aria-invalid", "true");
    // The button stays pressable (G23): pressed, the field's own validation stops the post and takes the person
    // back to the text, whose error says by how much.
    const sent = recordPosts();
    await user.click(within(sheet).getByRole("button", { name: "Post" }));
    expect(within(sheet).getByRole("textbox", { name: "Your post" })).toHaveFocus();
    expect(sent).toHaveLength(0);
  });

  test("says why an empty post is not sent, beside the button, and sends it once written (C11)", async () => {
    signedIn();
    const sent = recordPosts();
    const { user } = await renderApp();
    const sheet = await openSheet(user);
    const text = await within(sheet).findByRole("textbox", { name: "Your post" });

    await user.click(within(sheet).getByRole("button", { name: "Post" }));
    expect(within(sheet).getByText("Write something to post.")).toBeInTheDocument();
    expect(text).toHaveFocus();
    expect(sent).toHaveLength(0);

    await user.type(text, "Hello");
    expect(within(sheet).queryByText("Write something to post.")).not.toBeInTheDocument();
    await user.click(within(sheet).getByRole("button", { name: "Post" }));
    expect(await findToast("Posted.")).toBeInTheDocument();
  });

  test("highlights mentions, hashtags, URLs, known shortcodes and the text over the limit on a layer hidden from assistive technology", async () => {
    signedIn();
    server.use(
      http.get(`${apiBase}/instance`, () =>
        HttpResponse.json({
          ...instanceFixture,
          limits: { max_post_characters: 50, require_media_description: false },
        }),
      ),
      http.get(`${apiBase}/custom_emojis`, () =>
        HttpResponse.json({
          items: [
            {
              shortcode: "blobcat",
              url: "https://media.example/blobcat.png",
              static_url: "https://media.example/blobcat.png",
              aliases: [],
            },
          ],
        }),
      ),
    );
    const { user } = await renderApp();
    const sheet = await openSheet(user);
    const textbox = await within(sheet).findByRole("textbox", { name: "Your post" });

    await user.type(textbox, "@alice #tag :blobcat: :nope: https://example.com/ and more text");

    expect(textbox).toHaveAttribute("dir", "auto");
    const marks = () => within(sheet).queryAllByText((_, element) => element?.getAttribute("data-highlight") != null);
    await waitFor(() => {
      expect(marks().map((mark) => [mark.getAttribute("data-highlight"), mark.textContent])).toEqual([
        ["mention", "@alice"],
        ["hashtag", "#tag"],
        ["shortcode", ":blobcat:"],
        ["url", "https://example.com/"],
        ["over", "and more text"],
      ]);
    });
    // The layer is aria-hidden: only the textbox carries the text.
    for (const mark of marks()) expect(isInaccessible(mark)).toBe(true);
  });
});

describe("hints before posting", () => {
  test("a restricted account sees its visibility ceiling and the wider choices disabled, and posts with the ceiling", async () => {
    signedIn();
    server.use(
      http.get(`${apiBase}/accounts/verify_credentials`, () =>
        HttpResponse.json({
          ...viewerFixture,
          posting: { visibility_ceiling: "private", visibilities: ["private", "direct"] },
        }),
      ),
    );
    const sent = recordPosts();
    const { user } = await renderApp();
    const sheet = await openSheet(user);

    // The preference is public; the account can post at most as followers.
    await user.click(await within(sheet).findByRole("button", { name: "Visibility: Followers" }));
    const choices = await screen.findByRole("radiogroup", { name: "Who can see this post?" });
    expect(choices).toHaveAccessibleDescription(
      "Your account can post at most as “Followers” right now. A wider choice is posted as “Followers”.",
    );
    for (const name of ["Public", "This server", "Unlisted"])
      expect(within(choices).getByRole("radio", { name })).toBeDisabled();
    expect(within(choices).getByRole("radio", { name: "Followers" })).toBeChecked();
    expect(within(choices).getByRole("radio", { name: "Mentioned people" })).toBeEnabled();
    await user.click(screen.getByRole("button", { name: "Done" }));

    await user.type(within(sheet).getByRole("textbox", { name: "Your post" }), "Quietly");
    await user.click(within(sheet).getByRole("button", { name: "Post" }));

    expect(await findToast("Posted.")).toBeInTheDocument();
    expect(sent[0]?.body).toMatchObject({ visibility: "private" });
  });

  test("a local post warns about the mentioned accounts of other servers that do not follow the viewer", async () => {
    signedIn();
    const asked: string[][] = [];
    server.use(
      http.get(`${apiBase}/accounts/relationships`, ({ request }) => {
        asked.push(new URL(request.url).searchParams.getAll("id"));
        return undefined;
      }),
    );
    const { user } = await renderApp();
    const sheet = await openSheet(user);
    const warning =
      "@carol@remote.example is on another server and does not follow you, so they will not receive this post. Outside this server, “This server” posts reach only your followers, who see this mention as a plain link.";

    // The local viewer and an unknown handle are not warned about; neither is a public post.
    await user.type(
      await within(sheet).findByRole("textbox", { name: "Your post" }),
      "Hi @Carol@remote.example, @alice@my.example and @nobody@else.example",
    );
    await user.click(within(sheet).getByRole("button", { name: "Visibility: Public" }));
    await user.click(await screen.findByRole("radio", { name: "This server" }));
    await user.click(screen.getByRole("button", { name: "Done" }));

    expect(await within(sheet).findByText(warning)).toBeInTheDocument();
    expect(asked).toEqual([[remoteAccountFixture.id]]);

    await user.click(within(sheet).getByRole("button", { name: "Visibility: This server" }));
    await user.click(await screen.findByRole("radio", { name: "Unlisted" }));
    await user.click(screen.getByRole("button", { name: "Done" }));
    expect(within(sheet).queryByText(warning)).not.toBeInTheDocument();
  });

  test("a local post warns about a mentioned account that the server resolves from its server", async () => {
    signedIn();
    // Dave is not known yet: only resolving (WebFinger, as when the post is made) finds him.
    const dave = accountFixture(4, "Dave", "new.example");
    const resolved: (string | null)[] = [];
    server.use(
      http.get(`${apiBase}/accounts/lookup`, ({ request }) => {
        const query = new URL(request.url).searchParams;
        resolved.push(query.get("resolve"));
        if (query.get("acct") !== dave.acct) return undefined;
        return query.get("resolve") === "true" ? HttpResponse.json(dave) : problem(404, "about:blank", "Not Found");
      }),
    );
    const { user } = await renderApp();
    const sheet = await openSheet(user);

    await user.click(await within(sheet).findByRole("button", { name: "Visibility: Public" }));
    await user.click(await screen.findByRole("radio", { name: "This server" }));
    await user.click(screen.getByRole("button", { name: "Done" }));
    await user.type(
      within(sheet).getByRole("textbox", { name: "Your post" }),
      "Hi @carol@remote.example and @dave@new.example",
    );

    expect(
      await within(sheet).findByText(
        "@carol@remote.example and @dave@new.example are on other servers and do not follow you, so they will not receive this post. Outside this server, “This server” posts reach only your followers, who see these mentions as plain links.",
      ),
    ).toBeInTheDocument();
    expect(resolved.length).toBeGreaterThan(0);
    expect(resolved.every((r) => r === "true")).toBe(true);
  });

  test("a local post does not warn about a mentioned follower on another server", async () => {
    signedIn();
    let asked = false;
    server.use(
      http.get(`${apiBase}/accounts/relationships`, () => {
        asked = true;
        return HttpResponse.json([{ ...relationshipFixture(remoteAccountFixture.id), followed_by: true }]);
      }),
    );
    const { user } = await renderApp();
    const sheet = await openSheet(user);

    await user.click(await within(sheet).findByRole("button", { name: "Visibility: Public" }));
    await user.click(await screen.findByRole("radio", { name: "This server" }));
    await user.click(screen.getByRole("button", { name: "Done" }));
    await user.type(within(sheet).getByRole("textbox", { name: "Your post" }), "Hi @carol@remote.example");

    await waitFor(() => {
      expect(asked).toBe(true);
    });
    expect(within(sheet).queryByText(/does not follow you/)).not.toBeInTheDocument();
  });
});

/** Stubs the browser's image decoding and encoding, and records what the client asked of them. */
async function stubImageApis() {
  const drawn: { width: number; height: number }[] = [];
  // Node.js's fetch (the Request that the API client builds) cannot read a body made of jsdom's FormData
  // and Blob, so the upload uses Node.js's own: Blob and File from node:buffer (the web tsconfig has no
  // Node.js types, hence the untyped import), and the FormData class of Node.js's fetch, from a Response.
  const nodeBuffer = "node:buffer";
  const { Blob: NodeBlob, File: NodeFile } = (await import(/* @vite-ignore */ nodeBuffer)) as {
    Blob: typeof Blob;
    File: typeof File;
  };
  const NodeFormData = (
    await new Response("", { headers: { "Content-Type": "application/x-www-form-urlencoded" } }).formData()
  ).constructor as typeof FormData;
  vi.stubGlobal("FormData", NodeFormData);
  vi.stubGlobal("Blob", NodeBlob);
  vi.stubGlobal("File", NodeFile);
  vi.stubGlobal(
    "createImageBitmap",
    vi.fn(() => Promise.resolve({ width: 6000, height: 4000, close: vi.fn() })),
  );
  vi.stubGlobal(
    "OffscreenCanvas",
    class {
      getContext() {
        return {
          drawImage: (_image: unknown, _x: number, _y: number, width: number, height: number) => {
            drawn.push({ width, height });
          },
        };
      }
      convertToBlob({ type }: { type: string }) {
        return Promise.resolve(new Blob(["re-encoded"], { type }));
      }
    },
  );
  // jsdom has no object URLs.
  Object.defineProperty(URL, "createObjectURL", { configurable: true, value: () => "blob:preview" });
  Object.defineProperty(URL, "revokeObjectURL", { configurable: true, value: () => undefined });
  return drawn;
}

/** Records the files of POST /media. */
function recordUploads() {
  const files: File[] = [];
  server.use(
    http.post(`${apiBase}/media`, async ({ request }) => {
      const file = (await request.formData()).get("file");
      if (file instanceof File) files.push(file);
      return undefined;
    }),
  );
  return files;
}

/** The hidden file input of the "Add images, video, or audio" button (React Aria's FileTrigger). */
function fileInput(): HTMLInputElement {
  // eslint-disable-next-line testing-library/no-node-access -- the file input is hidden behind its button, as the browser's picker is.
  const input = document.querySelector<HTMLInputElement>('input[type="file"]');
  if (input === null) throw new Error("no file input");
  return input;
}

describe("images", () => {
  test("are scaled down and re-encoded before upload, always have a description field, and post with it", async () => {
    signedIn();
    const drawn = await stubImageApis();
    const uploads = recordUploads();
    const sent = recordPosts();
    const { user } = await renderApp();
    const sheet = await openSheet(user);
    await user.type(await within(sheet).findByRole("textbox", { name: "Your post" }), "A photo");

    await user.upload(fileInput(), new File(["exif and pixels"], "IMG_0001.jpeg", { type: "image/jpeg" }));

    const description = await within(sheet).findByRole("textbox", { name: "Description of attachment 1" });
    expect(description).toHaveAccessibleDescription(/Describe it for people who cannot see or hear it\./);
    expect(within(sheet).getByRole("button", { name: "Add images, video, or audio" })).toHaveAccessibleDescription(
      /^1 file attached Up to 4 files/,
    );
    // Pressed while the file uploads, the button says to wait (G23); it is never disabled by the input.
    await user.click(within(sheet).getByRole("button", { name: "Post" }));
    expect(
      within(sheet).getByText("The attachments are still uploading. Post once they are ready."),
    ).toBeInTheDocument();
    // 6000 × 4000 is more than 3840 × 2160 pixels of area: scaled to fit, with the aspect ratio kept.
    await waitFor(() => {
      expect(uploads).toHaveLength(1);
    });
    expect(drawn).toEqual([{ width: 3527, height: 2351 }]);
    expect(uploads[0]?.name).toBe("IMG_0001.jpg");
    expect(uploads[0]?.type).toBe("image/jpeg");
    expect(await uploads[0]?.text()).toBe("re-encoded");

    await user.type(description, "A cat on a sofa");
    await user.click(within(sheet).getByRole("checkbox", { name: "Hide the images until readers show them" }));
    // The upload was processing; the poll finds it ready.
    const post = within(sheet).getByRole("button", { name: "Post" });
    await waitFor(() => {
      expect(within(sheet).queryByText(/^(Uploading|Processing)…$/)).not.toBeInTheDocument();
    });
    await user.click(post);

    expect(await findToast("Posted.")).toBeInTheDocument();
    expect(sent[0]?.body).toEqual({
      text: "A photo",
      visibility: "public",
      language: "en",
      media: [{ id: uploadedMediaId, description: "A cat on a sofa" }],
      sensitive: true,
    });
  });

  test("ask before posting without descriptions when the preferences require them", async () => {
    signedIn();
    await stubImageApis();
    server.use(
      http.get(`${apiBase}/preferences`, () =>
        HttpResponse.json({ ...preferencesFixture, require_media_description: true }),
      ),
    );
    const sent = recordPosts();
    const { user } = await renderApp();
    const sheet = await openSheet(user);
    await user.type(await within(sheet).findByRole("textbox", { name: "Your post" }), "Look");
    await user.upload(fileInput(), new File(["png"], "chart.png", { type: "image/png" }));
    const description = await within(sheet).findByRole("textbox", { name: "Description of attachment 1" });
    expect(description).toHaveAccessibleDescription(/Your settings ask for a description of every attachment\./);
    const post = within(sheet).getByRole("button", { name: "Post" });
    await waitFor(() => {
      expect(within(sheet).queryByText(/^(Uploading|Processing)…$/)).not.toBeInTheDocument();
    });

    await user.click(post);
    const question = await screen.findByRole("alertdialog", { name: "Post without image descriptions?" });
    expect(within(question).getByText(/1 image has no description\./)).toBeInTheDocument();
    await user.click(within(question).getByRole("button", { name: "Add descriptions" }));
    await waitFor(() => {
      expect(description).toHaveFocus();
    });
    expect(sent).toHaveLength(0);

    await user.click(post);
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Post anyway" }));

    expect(await findToast("Posted.")).toBeInTheDocument();
    expect(sent[0]?.body["media"]).toEqual([{ id: uploadedMediaId }]);
  });

  test("does not post images without descriptions when the server requires them", async () => {
    signedIn();
    await stubImageApis();
    server.use(
      http.get(`${apiBase}/instance`, () =>
        HttpResponse.json({
          ...instanceFixture,
          limits: { ...instanceFixture.limits, require_media_description: true },
        }),
      ),
      // The server's requirement wins over the preference's question.
      http.get(`${apiBase}/preferences`, () =>
        HttpResponse.json({ ...preferencesFixture, require_media_description: true }),
      ),
    );
    const sent = recordPosts();
    const { user } = await renderApp();
    const sheet = await openSheet(user);
    await user.type(await within(sheet).findByRole("textbox", { name: "Your post" }), "Look");
    await user.upload(fileInput(), new File(["png"], "chart.png", { type: "image/png" }));
    const description = await within(sheet).findByRole("textbox", { name: "Description of attachment 1" });
    expect(description).toBeRequired();
    expect(description).toHaveAccessibleDescription(/This server requires a description of every attachment\./);
    const post = within(sheet).getByRole("button", { name: "Post" });
    await waitFor(() => {
      expect(within(sheet).queryByText(/^(Uploading|Processing)…$/)).not.toBeInTheDocument();
    });

    await user.type(description, "   ");
    await user.click(post);
    await waitFor(() => {
      expect(description).toHaveFocus();
    });
    expect(description).toBeInvalid();
    expect(description).toHaveAccessibleDescription(/Add a description of this attachment\./);
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(sent).toHaveLength(0);

    await user.type(description, "A bar chart");
    await user.click(post);
    expect(await findToast("Posted.")).toBeInTheDocument();
    expect(sent[0]?.body["media"]).toEqual([{ id: uploadedMediaId, description: "A bar chart" }]);
  });
});

/** Records the file and the thumbnail (poster) of every POST /media. */
function recordMediaParts() {
  const parts: { file: File; thumbnail: File | null }[] = [];
  server.use(
    http.post(`${apiBase}/media`, async ({ request }) => {
      const form = await request.formData();
      const file = form.get("file");
      const thumbnail = form.get("thumbnail");
      if (file instanceof File) parts.push({ file, thumbnail: thumbnail instanceof File ? thumbnail : null });
      return undefined;
    }),
  );
  return parts;
}

/**
 * jsdom loads no video: the poster's <video> fires `loadeddata` when given a source and `seeked` when moved, and has
 * the size and length of a 4K video of 12 seconds.
 */
function stubVideoElement() {
  vi.spyOn(HTMLMediaElement.prototype, "src", "set").mockImplementation(function (this: HTMLMediaElement) {
    queueMicrotask(() => this.dispatchEvent(new Event("loadeddata")));
  });
  vi.spyOn(HTMLMediaElement.prototype, "currentTime", "set").mockImplementation(function (this: HTMLMediaElement) {
    queueMicrotask(() => this.dispatchEvent(new Event("seeked")));
  });
  vi.spyOn(HTMLMediaElement.prototype, "duration", "get").mockReturnValue(12);
  vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => undefined);
  vi.spyOn(HTMLVideoElement.prototype, "videoWidth", "get").mockReturnValue(3840);
  vi.spyOn(HTMLVideoElement.prototype, "videoHeight", "get").mockReturnValue(2160);
}

describe("videos and audio files", () => {
  test("upload as they are, a video with a poster drawn from it, an audio file without one", async () => {
    signedIn();
    const drawn = await stubImageApis();
    stubVideoElement();
    const parts = recordMediaParts();
    const { user } = await renderApp();
    const sheet = await openSheet(user);

    await user.upload(fileInput(), [
      new File(["moov and mdat"], "IMG_0002.MOV", { type: "video/quicktime" }),
      new File(["ID3 and frames"], "song.mp3", { type: "audio/mpeg" }),
    ]);

    expect(await within(sheet).findByRole("textbox", { name: "Description of attachment 1" })).toBeInTheDocument();
    expect(within(sheet).getByRole("textbox", { name: "Description of attachment 2" })).toBeInTheDocument();
    await waitFor(() => {
      expect(parts).toHaveLength(2);
    });
    const [video, audio] = [
      parts.find((p) => p.file.name === "IMG_0002.MOV"),
      parts.find((p) => p.file.name === "song.mp3"),
    ];
    // The files go as they are: the server removes their metadata without re-encoding them.
    expect(video?.file.type).toBe("video/quicktime");
    expect(await video?.file.text()).toBe("moov and mdat");
    expect(await audio?.file.text()).toBe("ID3 and frames");
    expect(audio?.thumbnail).toBeNull();
    // The poster is a frame of the video, within the medium size of 1920 × 1080.
    expect(video?.thumbnail?.type).toBe("image/jpeg");
    expect(drawn).toEqual([{ width: 1920, height: 1080 }]);
  });

  test("of other types are not attached", async () => {
    signedIn();
    const parts = recordMediaParts();
    const { user } = await renderApp();
    const sheet = await openSheet(user);
    // The picker offers only the accepted types; a file dropped past it is still checked.
    await userEvent
      .setup({ applyAccept: false })
      .upload(fileInput(), new File(["mkv"], "movie.mkv", { type: "video/x-matroska" }));

    expect(
      await screen.findByText(
        "movie.mkv is not a JPEG, PNG, GIF, or WebP image, an MP4 or WebM video, or an MP3, Ogg, or M4A audio file.",
      ),
    ).toBeInTheDocument();
    expect(within(sheet).queryByRole("list", { name: "Attached media" })).not.toBeInTheDocument();
    expect(parts).toHaveLength(0);
  });
});

/** A post by someone else, in the `local` visibility, that mentions the viewer and a third account. */
const localPost = viewerStatusFixture({
  id: "0192f0c4-7a8e-7000-8000-0000000000c1",
  account: {
    ...viewerStatusFixture().account,
    id: "0192f0c4-7a8e-7000-8000-000000000002",
    username: "bob",
    acct: "bob",
    display_name: "Bob",
  },
  content: "<p>Anyone around?</p>",
  visibility: "local",
  mentions: [
    { id: "0192f0c4-7a8e-7000-8000-000000000001", username: "alice", acct: "alice", url: "https://my.example/@alice" },
    {
      id: "0192f0c4-7a8e-7000-8000-000000000003",
      username: "carol",
      acct: "carol@remote.example",
      url: "https://remote.example/@carol",
    },
  ],
});

/** A stand-in for a timeline: the compose provider, a reply button as other features use it, and the form. */
function ReplyPage() {
  const reply = useReply();
  return (
    <>
      <h1>Thread</h1>
      {reply !== null && (
        <Button
          onPress={() => {
            reply(localPost);
          }}
        >
          Reply to Bob
        </Button>
      )}
    </>
  );
}

function createReplyRouter({
  history,
  ...context
}: RouterContext & { history: ReturnType<typeof createMemoryHistory> }) {
  const rootRoute = createRootRouteWithContext<RouterContext>()({
    component: () => (
      <ComposeProvider>
        <ReplyPage />
        <ComposeEntry />
        <Outlet />
      </ComposeProvider>
    ),
  });
  return createRouter({ routeTree: rootRoute, context, history });
}

describe("replies", () => {
  test("name the post, start with its mentions, default to its narrower visibility and post in reply to it", async () => {
    signedIn();
    const sent = recordPosts();
    const { user } = await renderApp({ createRouter: createReplyRouter });

    await user.click(await screen.findByRole("button", { name: "Reply to Bob" }));

    const sheet = await screen.findByRole("dialog", { name: "New post" });
    expect(within(sheet).getByText("Replying to Bob")).toBeInTheDocument();
    expect(within(sheet).getByText("Anyone around?")).toBeInTheDocument();
    const text = within(sheet).getByRole("textbox", { name: "Your post" });
    expect(text).toHaveValue("@bob @carol@remote.example ");
    // A reply to a `local` post is `local` by default (ADR-0017), narrower than the preference (public).
    expect(within(sheet).getByRole("button", { name: "Visibility: This server" })).toBeInTheDocument();

    await user.type(text, "Here!");
    await user.click(within(sheet).getByRole("button", { name: "Reply" }));

    expect(await findToast("Posted.")).toBeInTheDocument();
    expect(sent[0]?.body).toEqual({
      text: "@bob @carol@remote.example Here!",
      visibility: "local",
      language: "en",
      in_reply_to_id: localPost.id,
    });
  });

  test("can be turned into a normal post", async () => {
    signedIn();
    const sent = recordPosts();
    const { user } = await renderApp({ createRouter: createReplyRouter });
    await user.click(await screen.findByRole("button", { name: "Reply to Bob" }));
    const sheet = await screen.findByRole("dialog", { name: "New post" });

    await user.click(within(sheet).getByRole("button", { name: "Cancel reply" }));

    expect(within(sheet).queryByText("Replying to Bob")).not.toBeInTheDocument();
    expect(within(sheet).getByRole("button", { name: "Visibility: Public" })).toBeInTheDocument();
    await user.click(within(sheet).getByRole("button", { name: "Post" }));
    expect(await findToast("Posted.")).toBeInTheDocument();
    expect(sent[0]?.body).not.toHaveProperty("in_reply_to_id");
  });
});

describe("the new-post buttons (web-client/16-look-states-and-wording.md G4)", () => {
  /** Matches the given media queries only, as a viewport of that width would. */
  function stubViewport(...matching: string[]) {
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: matching.includes(query),
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }));
  }

  /** The rail's button, in the navigation (the stylesheet shows it from md only). */
  const railButton = () =>
    within(screen.getByRole("navigation", { name: "Main" })).getByRole("button", { name: "Write a post" });

  test.each(["/home", "/local", "/tags/hello", "/@bob"])("floats over %s, a feed, below md", async (path) => {
    signedIn();
    await renderApp({ path });

    expect(await screen.findAllByRole("button", { name: "Write a post" })).toHaveLength(2);
    expect(railButton()).toBeInTheDocument();
    expect(screen.getByRole("main")).toHaveAttribute("data-compose-fab");
  });

  test.each([
    ["/notifications", "Notifications"],
    ["/search", "Search"],
    ["/settings", "Settings"],
    ["/@bob/followers", "Followers"],
  ])("does not float over %s, which has no feed, but the rail has the button", async (path, heading) => {
    signedIn();
    await renderApp({ path });

    expect(await screen.findByRole("heading", { level: 1, name: new RegExp(heading) })).toBeInTheDocument();
    expect(await screen.findAllByRole("button", { name: "Write a post" })).toHaveLength(1);
    expect(railButton()).toBeInTheDocument();
    expect(screen.getByRole("main")).not.toHaveAttribute("data-compose-fab");
  });

  test("is not offered while signed out", async () => {
    await renderApp({ path: "/local" });

    expect(await screen.findByRole("navigation", { name: "Main" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Write a post" })).not.toBeInTheDocument();
  });

  test("below md the floating button is only an icon, named by its label and with the label as its tooltip", async () => {
    signedIn();
    const { user } = await renderApp({ path: "/home" });
    const button = (await screen.findAllByRole("button", { name: "Write a post" })).at(-1) as HTMLElement;
    expect(button).toHaveTextContent("");

    // The button is the page's last focusable element: Shift+Tab from the start reaches it. On keyboard focus,
    // React Aria shows the tooltip at once.
    await user.tab({ shift: true });
    expect(button).toHaveFocus();

    expect(await screen.findByRole("tooltip")).toHaveTextContent("Write a post");
  });

  test("from md nothing floats and the rail's button opens the form in a dialog, never inline, and posts from it", async () => {
    signedIn();
    stubViewport("(min-width: 48rem)");
    const sent = recordPosts();
    const { user } = await renderApp({ path: "/home" });
    expect(await screen.findAllByRole("button", { name: "Write a post" })).toHaveLength(1);
    expect(screen.getByRole("main")).not.toHaveAttribute("data-compose-fab");
    expect(screen.queryByRole("textbox", { name: "Your post" })).not.toBeInTheDocument();

    await user.click(railButton());
    const sheet = await screen.findByRole("dialog", { name: "New post" });
    await user.type(within(sheet).getByRole("textbox", { name: "Your post" }), "From the rail");
    await user.click(within(sheet).getByRole("button", { name: "Post" }));

    expect(await findToast("Posted.")).toBeInTheDocument();
    expect(sent[0]?.body["text"]).toBe("From the rail");
    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "New post" })).not.toBeInTheDocument();
    });
  });

  test("the floating button gets focus back when the sheet it opened with the keyboard closes", async () => {
    signedIn();
    const { user } = await renderApp({ path: "/home" });
    const button = (await screen.findAllByRole("button", { name: "Write a post" })).at(-1) as HTMLElement;
    button.focus();

    await user.keyboard("{Enter}");
    const sheet = await screen.findByRole("dialog", { name: "New post" });
    expect(within(sheet).getByRole("textbox", { name: "Your post" })).toHaveFocus();
    await user.keyboard("{Escape}");

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "New post" })).not.toBeInTheDocument();
    });
    // React Aria's FocusScope restores focus a frame after the dialog unmounts.
    await waitFor(() => {
      expect(button).toHaveFocus();
    });
  });
});
