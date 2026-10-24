// SPDX-License-Identifier: AGPL-3.0-only
// The completion of mentions, hashtags and emoji in the text fields (ADR-0092 §1–§3, §5) through the whole app
// (web-client/README.md T4): the real router and QueryClient, MSW for the server, and the Unicode emoji data of
// features/emoji/unicode. The keyboard and ARIA details of the list are @my/ui TextAreaAutocomplete's own tests.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { frequentEmojisKey, recentTagsKey, skinToneKey } from "../../lib/storage";
import { apiBase, oauthPaths, problem, tokenResponse } from "../../test/msw/http";
import { accountFixture, instanceFixture, otherAccountFixture } from "../../test/fixtures";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { accountSearchHandler, tagSearchHandler } from "./test/handlers";

/** GET /custom_emojis answering `emojis`, by shortcode with their aliases, images at media.example. */
function customEmojisHandler(emojis: Record<string, string[]>) {
  return http.get(`${apiBase}/custom_emojis`, () =>
    HttpResponse.json({
      items: Object.entries(emojis).map(([shortcode, aliases]) => ({
        shortcode,
        url: `https://media.example/emoji/${shortcode}.png`,
        static_url: `https://media.example/emoji/${shortcode}.png`,
        aliases,
      })),
    }),
  );
}

/** Resumes a session with the token the default handlers accept. */
function signedIn() {
  server.use(http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")));
}

/** Opens the new-post sheet and returns it with its text area. */
async function openSheet(user: Awaited<ReturnType<typeof renderApp>>["user"]) {
  await user.click((await screen.findAllByRole("button", { name: "Write a post" }))[0] as HTMLElement);
  const sheet = await screen.findByRole("dialog", { name: "New post" });
  const text = await within(sheet).findByRole("textbox", { name: "Your post" });
  return { sheet, text: text as HTMLTextAreaElement };
}

/** Records the `resolve` of every GET /accounts/lookup, answering `account` for its handle. */
function recordLookups(account: ReturnType<typeof accountFixture>) {
  const lookups: { acct: string | null; resolve: string | null }[] = [];
  server.use(
    http.get(`${apiBase}/accounts/lookup`, ({ request }) => {
      const query = new URL(request.url).searchParams;
      lookups.push({ acct: query.get("acct"), resolve: query.get("resolve") });
      return query.get("acct") === account.acct ? HttpResponse.json(account) : problem(404, "about:blank", "Not Found");
    }),
  );
  return lookups;
}

const dave = accountFixture(4, "Dave", "new.example");

beforeEach(() => {
  sessionStorage.clear();
  signedIn();
});

describe("the completion after @", () => {
  test("offers the accounts the server finds and puts the handle and a space in place of the word", async () => {
    const queries: string[] = [];
    server.use(accountSearchHandler([otherAccountFixture, dave], (q) => queries.push(q)));
    const { user } = await renderApp();
    const { text } = await openSheet(user);

    await user.type(text, "Hi @bo");
    const listbox = await screen.findByRole("listbox", { name: "Suggestions" });
    expect(queries).toEqual(["bo"]);
    expect(text).toHaveAttribute("aria-controls", listbox.id);
    const options = within(listbox).getAllByRole("option");
    // A local account by its username, an account of another server with its domain.
    expect(options).toHaveLength(2);
    expect(options[0]).toHaveAccessibleName("Bob @bob");
    expect(options[1]).toHaveAccessibleName("Dave @dave@new.example");
    expect(screen.getByText("2 suggestions")).toBeInTheDocument();

    await user.keyboard("{ArrowDown}{Enter}");
    expect(text).toHaveValue("Hi @dave@new.example ");
    await waitFor(() => {
      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    });

    // Tab accepts too; the local account goes in without the domain.
    await user.type(text, "and @bo");
    await screen.findByRole("listbox", { name: "Suggestions" });
    await user.keyboard("{Tab}");
    expect(text).toHaveValue("Hi @dave@new.example and @bob ");
    expect(text).toHaveFocus();
  });

  test("looks a full handle up, resolving it, only when the search has no exact match", async () => {
    const lookups = recordLookups(dave);
    const eve = accountFixture(5, "Eve", "other.example");
    server.use(
      http.get(`${apiBase}/accounts/search`, ({ request }) =>
        HttpResponse.json(new URL(request.url).searchParams.get("q")?.startsWith("eve") === true ? [eve, otherAccountFixture] : [otherAccountFixture]),
      ),
    );
    const { user } = await renderApp();
    const { text } = await openSheet(user);

    // The search knows the handle: nothing is looked up.
    await user.type(text, "@eve@other.example");
    expect(await screen.findByRole("option", { name: "Eve @eve@other.example" })).toBeInTheDocument();
    expect(lookups).toEqual([]);
    await user.keyboard("{Escape}");

    // The search does not know it: it is looked up and resolved, and comes first.
    await user.type(text, " @dave@new.example");
    await screen.findByRole("option", { name: "Dave @dave@new.example" });
    expect(within(screen.getByRole("listbox")).getAllByRole("option")[0]).toHaveAccessibleName("Dave @dave@new.example");
    expect(lookups).toEqual([{ acct: "dave@new.example", resolve: "true" }]);
  });

  test("does not ask to resolve while the instance takes nothing in from other servers", async () => {
    const lookups = recordLookups(dave);
    server.use(http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, federation: { mode: "outbound" } })));
    const { user } = await renderApp();
    const { text } = await openSheet(user);

    await user.type(text, "@dave@new.example");
    await screen.findByRole("option", { name: /Dave/u });
    expect(lookups).toEqual([{ acct: "dave@new.example", resolve: null }]);
  });

  test("closes without a message when the search fails", async () => {
    let asked = false;
    server.use(
      http.get(`${apiBase}/accounts/search`, () => {
        asked = true;
        return problem(429, "about:blank", "Too Many Requests");
      }),
    );
    const { user } = await renderApp();
    const { text } = await openSheet(user);

    await user.type(text, "@bo");
    await waitFor(() => {
      expect(asked).toBe(true);
    });
    // The list never opens, and nothing says the request failed.
    await new Promise((resolve) => setTimeout(resolve, 400));
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(screen.queryByText("No suggestions")).not.toBeInTheDocument();
    expect(text).not.toHaveAttribute("aria-controls");
  });
});

describe("the completion after #", () => {
  test("offers the hashtags with how many posts carry them, those used lately on this browser first", async () => {
    recentTagsKey.write(["Caturday", "golang"]);
    server.use(
      tagSearchHandler([
        { name: "cats", statuses_count: 12 },
        { name: "caturday", statuses_count: 1 },
      ]),
    );
    const { user } = await renderApp();
    const { text } = await openSheet(user);

    await user.type(text, "#ca");
    const listbox = await screen.findByRole("listbox", { name: "Suggestions" });
    const options = within(listbox).getAllByRole("option");
    expect(options).toHaveLength(2);
    expect(options[0]).toHaveAccessibleName("#Caturday 1 post");
    expect(options[1]).toHaveAccessibleName("#cats 12 posts");
    await user.keyboard("{Enter}");
    expect(text).toHaveValue("#Caturday ");
  });

  test("remembers the hashtags of a post when it is sent, the latest first", async () => {
    recentTagsKey.write(["golang"]);
    const { user } = await renderApp();
    const { sheet, text } = await openSheet(user);

    // Ending with a space, so that no hashtag is being typed.
    await user.type(text, "Hello #Caturday and #cats ");
    await user.click(within(sheet).getByRole("button", { name: "Post" }));
    await waitFor(() => {
      expect(recentTagsKey.read()).toEqual(["Caturday", "cats", "golang"]);
    });
  });
});

describe("the completion after :", () => {
  test("offers custom and Unicode emoji from two characters, those used often first, in the skin tone chosen", async () => {
    server.use(
      customEmojisHandler({ blobcat: ["kitty"], blobfox: [] }),
      http.get(`${apiBase}/accounts/search`, () => {
        throw new Error("the emoji are not asked for");
      }),
    );
    frequentEmojisKey.write([{ emoji: ":blobfox:", score: 1, at: Date.now() }]);
    skinToneKey.write("medium");
    const { user } = await renderApp();
    const { text } = await openSheet(user);

    await user.type(text, ":b");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    await user.type(text, "lob");
    const listbox = await screen.findByRole("listbox", { name: "Suggestions" });
    const options = within(listbox).getAllByRole("option");
    expect(options).toHaveLength(2);
    expect(options[0]).toHaveAccessibleName(":blobfox:");
    expect(options[1]).toHaveAccessibleName(":blobcat:");
    await user.keyboard("{ArrowDown}{Enter}");
    expect(text).toHaveValue(":blobcat: ");

    // By an alias.
    await user.type(text, ":kit");
    expect(await screen.findByRole("option", { name: ":blobcat:" })).toBeInTheDocument();
    await user.keyboard("{Escape}");

    // A Unicode emoji by its CLDR name, in the chosen skin tone.
    await user.type(text, " :waving");
    await user.click(await screen.findByRole("option", { name: "waving hand" }));
    expect(text).toHaveValue(":blobcat: :kit 👋🏽 ");
  }, 30_000);

  test("is the only completion of the content warning", async () => {
    const queries: string[] = [];
    server.use(accountSearchHandler([otherAccountFixture], (q) => queries.push(q)), customEmojisHandler({ blobcat: [] }));
    const { user } = await renderApp();
    const { sheet } = await openSheet(user);

    await user.click(within(sheet).getByRole("button", { name: "Add a content warning" }));
    const warning = within(sheet).getByRole("textbox", { name: "Content warning" });
    await user.type(warning, "@bo #ca");
    await new Promise((resolve) => setTimeout(resolve, 400));
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(queries).toEqual([]);

    await user.type(warning, " :blob");
    await screen.findByRole("option", { name: ":blobcat:" });
    await user.keyboard("{Enter}");
    expect(warning).toHaveValue("@bo #ca :blobcat: ");
  });
});

describe("the completion in the profile", () => {
  test("completes emoji only in the display name, and mentions in the bio", async () => {
    server.use(accountSearchHandler([otherAccountFixture]), customEmojisHandler({ blobcat: [] }));
    const { user } = await renderApp({ path: "/settings/profile" });
    const displayName = await screen.findByRole("textbox", { name: "Display name" });

    await user.clear(displayName);
    await user.type(displayName, "@bo");
    await new Promise((resolve) => setTimeout(resolve, 400));
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    await user.type(displayName, " :blob");
    await screen.findByRole("option", { name: ":blobcat:" });
    await user.keyboard("{Enter}");
    expect(displayName).toHaveValue("@bo :blobcat: ");

    const note = screen.getByRole("textbox", { name: "Bio" });
    await user.clear(note);
    await user.type(note, "Friend of @bo");
    await user.click(await screen.findByRole("option", { name: /Bob/u }));
    expect(note).toHaveValue("Friend of @bob ");
  });
});
