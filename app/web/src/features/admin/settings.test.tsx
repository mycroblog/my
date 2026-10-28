// SPDX-License-Identifier: AGPL-3.0-only
// The server settings screen through the whole app (web-client/README.md T4): an administrator with manage_settings
// names the server once, in any language, the app then calls the server by that name instead of its domain, and
// a name that the server would refuse is caught in the form; the descriptions are saved one by one, and the
// policies (registration, language, timelines, federation, domain actions) together; with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { instanceFixture, mediaFixture, type Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { nodeUploadFile } from "../../test/multipart";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { instanceSettingsFixture } from "./test/handlers";

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

type Text = Schemas["AdminInstanceText"];

/** The server with voice chat on (GET /instance) and no room open. */
function voiceOn() {
  server.use(
    http.get(`${apiBase}/instance`, () =>
      HttpResponse.json({ ...instanceFixture, voice_enabled: true, voice_room_starters: "permitted" }),
    ),
    http.get(`${apiBase}/voice/rooms`, () => HttpResponse.json({ items: [] })),
  );
}

/**
 * Plays the instance name: GET /instance names the instance by it, or its domain without one, and each PUT changes
 * it; records the changes.
 */
function namesServer(initial: string | undefined) {
  let name = initial;
  const changes: string[] = [];
  server.use(
    http.get(`${apiBase}/instance`, () =>
      HttpResponse.json({ ...instanceFixture, name: name ?? instanceFixture.domain }),
    ),
    http.get(`${apiBase}/admin/instance_texts`, () =>
      HttpResponse.json({
        items:
          name === undefined ? [] : [{ key: "name", body: name, revised_at: "2026-10-16T00:00:00Z" } satisfies Text],
      }),
    ),
    http.put(`${apiBase}/admin/instance_texts/name`, async ({ request }) => {
      const { body } = (await request.json()) as Schemas["SetInstanceTextRequest"];
      changes.push(body);
      name = body === "" ? undefined : body;
      return HttpResponse.json({ key: "name", body, revised_at: "2026-10-17T00:00:00Z" } satisfies Text);
    }),
  );
  return { changes };
}

describe("the server's name", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_settings"]));
  });

  test("shows the name in one field, and the domain that stands in without one", async () => {
    namesServer("my example");
    await renderApp({ path: "/admin/settings" });

    expect(await screen.findByRole("heading", { level: 1, name: "Server settings" })).toBeInTheDocument();
    const section = screen.getByRole("region", { name: "Name" });
    expect(within(section).getByText(/in place of the domain my\.example/)).toBeInTheDocument();
    expect(within(section).getAllByRole("textbox")).toHaveLength(1);
    expect(within(section).getByRole("textbox", { name: "Name" })).toHaveValue("my example");
  });

  test("saves a new name, after which the app calls the server by it", async () => {
    const names = namesServer("my example");
    const { user } = await renderApp({ path: "/admin/settings" });
    const field = await screen.findByRole("textbox", { name: "Name" });
    await waitFor(() => {
      expect(document.title).toBe("Server settings – my example");
    });

    await user.clear(field);
    await user.type(field, "例のソーシャル");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Saved the name.")).toBeInTheDocument();
    expect(names.changes).toEqual(["例のソーシャル"]);
    // The tab title and the sidebar take the new name without a reload, whatever the app's language.
    await waitFor(() => {
      expect(document.title).toBe("Server settings – 例のソーシャル");
    });
    expect(screen.getAllByText("例のソーシャル").length).toBeGreaterThan(0);
    expect(screen.queryByText("my example")).not.toBeInTheDocument();

    // Saving again without a change sends nothing.
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText("The name is already saved.")).toBeInTheDocument();
    expect(names.changes).toHaveLength(1);
  });

  test("clearing the name names the server by its domain", async () => {
    namesServer("Example Social");
    const { user } = await renderApp({ path: "/admin/settings" });
    await user.clear(await screen.findByRole("textbox", { name: "Name" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Saved the name.")).toBeInTheDocument();
    await waitFor(() => {
      expect(document.title).toBe("Server settings – my.example");
    });
  });

  test("keeps a name the server would refuse in the form, with what to change", async () => {
    const names = namesServer(undefined);
    const { user } = await renderApp({ path: "/admin/settings" });
    const english = await screen.findByRole("textbox", { name: "Name" });

    await user.type(english, "   ");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(english).toHaveAccessibleDescription(/Enter a name with letters, or leave the field empty\./);
    expect(english).toBeInvalid();

    await user.clear(english);
    await user.click(english);
    await user.paste("a".repeat(101));
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(english).toHaveAccessibleDescription(/Use at most 100 characters\./);
    expect(names.changes).toEqual([]);
  });

  test("shows why a save failed and keeps the names", async () => {
    namesServer(undefined);
    const refusal: Schemas["Problem"] = {
      type: "about:blank",
      title: "Unprocessable",
      status: 422,
      detail: "body is invalid",
    };
    server.use(
      http.put(`${apiBase}/admin/instance_texts/name`, () =>
        HttpResponse.json(refusal, { status: 422, headers: { "Content-Type": "application/problem+json" } }),
      ),
    );
    const { user } = await renderApp({ path: "/admin/settings" });
    const english = await screen.findByRole("textbox", { name: "Name" });
    await user.type(english, "Example Social");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Could not save the name: body is invalid")).toHaveAttribute("role", "alert");
    expect(english).toHaveValue("Example Social");
  });

  test("is in the administration menu", async () => {
    namesServer(undefined);
    const { user } = await renderApp({ path: "/admin" });
    await user.click(await within(await screen.findByRole("main")).findByRole("link", { name: "Server settings" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Server settings" })).toBeInTheDocument();
  });
});

test("a viewer without manage_settings sees why instead of the names, and nothing is read", async () => {
  server.use(signedInHandler, operatorHandler(["manage_reports"]));
  let reads = 0;
  server.use(
    http.get(`${apiBase}/admin/instance_texts`, () => {
      reads++;
      return HttpResponse.json({ items: [] });
    }),
  );
  await renderApp({ path: "/admin/settings" });

  expect(await screen.findByRole("heading", { level: 1, name: "Server settings" })).toBeInTheDocument();
  expect(await screen.findByText(/manage_settings/)).toBeInTheDocument();
  expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  expect(reads).toBe(0);
});

describe("the server's descriptions", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_settings"]));
  });

  test("saves only the descriptions that changed", async () => {
    const changes: { key: string; body: string }[] = [];
    server.use(
      http.get(`${apiBase}/admin/instance_texts`, () =>
        HttpResponse.json({
          items: [
            { key: "short_description", body: "A small server.", revised_at: "2026-10-16T00:00:00Z" },
          ] satisfies Text[],
        }),
      ),
      http.put(`${apiBase}/admin/instance_texts/:key`, async ({ request, params }) => {
        const { body } = (await request.json()) as Schemas["SetInstanceTextRequest"];
        const key = String(params["key"]) as Schemas["InstanceTextKey"];
        changes.push({ key, body });
        return HttpResponse.json({ key, body, revised_at: "2026-10-17T00:00:00Z" } satisfies Text);
      }),
    );
    const { user } = await renderApp({ path: "/admin/settings" });
    const section = await screen.findByRole("region", { name: "Descriptions" });
    expect(within(section).getByRole("textbox", { name: "Short description" })).toHaveValue("A small server.");

    await user.type(within(section).getByRole("textbox", { name: "Description" }), "小さなサーバーです。");
    await user.click(within(section).getByRole("button", { name: "Save the descriptions" }));

    expect(
      await within(section).findByText("Saved the descriptions. The audit log records the change."),
    ).toBeInTheDocument();
    expect(changes).toEqual([{ key: "description", body: "小さなサーバーです。" }]);
  });
});

describe("the server's policies", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_settings"]));
  });

  /** Plays PATCH /admin/settings and counts the reads of GET /instance; records the changes. */
  function settingsServer() {
    const patches: Schemas["UpdateInstanceSettingsRequest"][] = [];
    let instanceReads = 0;
    server.use(
      http.get(`${apiBase}/instance`, () => {
        instanceReads++;
        return HttpResponse.json(instanceFixture);
      }),
      http.patch(`${apiBase}/admin/settings`, async ({ request }) => {
        const body = (await request.json()) as Schemas["UpdateInstanceSettingsRequest"];
        patches.push(body);
        return HttpResponse.json(instanceSettingsFixture(body.values));
      }),
    );
    return { patches, instanceReads: () => instanceReads };
  }

  test("shows the saved settings and sends the changed ones together", async () => {
    const settings = settingsServer();
    const { user } = await renderApp({ path: "/admin/settings" });
    const registration = await screen.findByRole("region", { name: "Registration" });
    expect(within(registration).getByRole("radio", { name: "With approval" })).toBeChecked();
    const timelines = screen.getByRole("region", { name: "Timelines" });
    const local = within(timelines).getByRole("radiogroup", { name: "Local timeline" });
    expect(within(local).getByRole("radio", { name: "Everyone" })).toBeChecked();
    expect(within(timelines).getByRole("radiogroup", { name: "Federated timeline" })).toBeInTheDocument();

    await user.click(within(registration).getByRole("radio", { name: "With an invite" }));
    await user.click(within(local).getByRole("radio", { name: "Signed-in users" }));
    await user.click(
      within(screen.getByRole("region", { name: "Federation" })).getByRole("checkbox", {
        name: "Require signed requests (authorized fetch)",
      }),
    );
    const domainActions = screen.getByRole("radiogroup", { name: "Who may read the list" });
    await user.click(within(domainActions).getByRole("radio", { name: "Signed-in users" }));
    const reads = settings.instanceReads();
    await user.click(screen.getByRole("button", { name: "Save the settings" }));

    expect(
      await screen.findByText("Saved the settings. They take effect at once, and the audit log records each change."),
    ).toBeInTheDocument();
    expect(settings.patches).toEqual([
      {
        values: {
          registration_mode: "invite",
          local_timeline_access: "logged_in",
          authorized_fetch: "true",
          domain_actions_visibility: "users",
        },
      },
    ]);
    // The navigation's timelines and the registration mode come from GET /instance, read again before the status.
    expect(settings.instanceReads()).toBeGreaterThan(reads);
    expect(within(registration).getByRole("radio", { name: "With an invite" })).toBeChecked();

    // Saving again without a change sends nothing.
    await user.click(screen.getByRole("button", { name: "Save the settings" }));
    expect(await screen.findByText("The settings are already saved.")).toBeInTheDocument();
    expect(settings.patches).toHaveLength(1);
  });

  test("switches the three routes by which apps register, all on by default", async () => {
    const settings = settingsServer();
    const { user } = await renderApp({ path: "/admin/settings" });
    const apps = await screen.findByRole("region", { name: "Apps" });
    for (const label of ["Users register apps", "Client ID Metadata Documents", "Dynamic client registration"]) {
      expect(within(apps).getByRole("checkbox", { name: label })).toBeChecked();
    }

    await user.click(within(apps).getByRole("checkbox", { name: "Users register apps" }));
    await user.click(within(apps).getByRole("checkbox", { name: "Dynamic client registration" }));
    await user.click(screen.getByRole("button", { name: "Save the settings" }));

    expect(
      await screen.findByText("Saved the settings. They take effect at once, and the audit log records each change."),
    ).toBeInTheDocument();
    expect(settings.patches).toEqual([
      { values: { user_app_registration: "false", dynamic_client_registration: "false" } },
    ]);
    expect(within(apps).getByRole("checkbox", { name: "Users register apps" })).not.toBeChecked();
    expect(within(apps).getByRole("checkbox", { name: "Client ID Metadata Documents" })).toBeChecked();
  });

  test("publishes the running costs, saying that the breakdown reveals the providers", async () => {
    const settings = settingsServer();
    const { user } = await renderApp({ path: "/admin/settings" });
    const costs = await screen.findByRole("region", { name: "Running costs" });
    const choice = within(costs).getByRole("radiogroup", { name: "What to publish" });
    expect(within(choice).getByRole("radio", { name: "Nothing" })).toBeChecked();
    const breakdown = within(choice).getByRole("radio", { name: "Totals and breakdown" });
    expect(breakdown).toHaveAccessibleDescription(
      /This reveals which providers, such as cloud services, run this server\./u,
    );

    await user.click(breakdown);
    await user.click(screen.getByRole("button", { name: "Save the settings" }));

    expect(
      await screen.findByText("Saved the settings. They take effect at once, and the audit log records each change."),
    ).toBeInTheDocument();
    expect(settings.patches).toEqual([{ values: { finance_visibility: "breakdown" } }]);
    expect(breakdown).toBeChecked();
  });

  test("publishes the income apart from the costs, saying that the members of a small plan may be guessed", async () => {
    const settings = settingsServer();
    const { user } = await renderApp({ path: "/admin/settings" });
    const income = await screen.findByRole("region", { name: "Income" });
    const choice = within(income).getByRole("radiogroup", { name: "What to publish" });
    expect(within(choice).getByRole("radio", { name: "Nothing" })).toBeChecked();
    const plans = within(choice).getByRole("radio", { name: "Totals and members of each plan" });
    expect(plans).toHaveAccessibleDescription(/On a plan with few members, people may guess who they are\./u);

    await user.click(plans);
    await user.click(screen.getByRole("button", { name: "Save the settings" }));

    expect(
      await screen.findByText("Saved the settings. They take effect at once, and the audit log records each change."),
    ).toBeInTheDocument();
    expect(settings.patches).toEqual([{ values: { income_visibility: "plans" } }]);
  });

  test("publishes the supporter list to everyone or to signed-in users, to nobody by default", async () => {
    const settings = settingsServer();
    const { user } = await renderApp({ path: "/admin/settings" });
    const list = await screen.findByRole("region", { name: "Supporter list" });
    const choice = within(list).getByRole("radiogroup", { name: "Who may read the supporter list" });
    expect(within(choice).getByRole("radio", { name: "Nobody" })).toBeChecked();
    const signedIn = within(choice).getByRole("radio", { name: "Signed-in users" });
    expect(signedIn).toHaveAccessibleDescription(/Only people signed in to this server/u);

    await user.click(signedIn);
    await user.click(screen.getByRole("button", { name: "Save the settings" }));

    expect(
      await screen.findByText("Saved the settings. They take effect at once, and the audit log records each change."),
    ).toBeInTheDocument();
    expect(settings.patches).toEqual([{ values: { supporter_list_visibility: "signed_in" } }]);
  });

  test("hides the counts or the operators on the landing page, both shown by default", async () => {
    const settings = settingsServer();
    const { user } = await renderApp({ path: "/admin/settings" });
    const landing = await screen.findByRole("region", { name: "Landing page" });
    const counts = within(landing).getByRole("checkbox", { name: "Show the numbers of the server" });
    const operators = within(landing).getByRole("checkbox", { name: "Show who runs the server" });
    expect(counts).toBeChecked();
    expect(operators).toBeChecked();
    expect(operators).toHaveAccessibleDescription(/never listed/u);

    await user.click(operators);
    await user.click(screen.getByRole("button", { name: "Save the settings" }));

    expect(
      await screen.findByText("Saved the settings. They take effect at once, and the audit log records each change."),
    ).toBeInTheDocument();
    expect(settings.patches).toEqual([{ values: { public_operators: "false" } }]);
  });

  test("chooses what the holders of view_finance see, nothing of the items by default", async () => {
    const settings = settingsServer();
    const { user } = await renderApp({ path: "/admin/settings" });
    const costs = await screen.findByRole("region", { name: "Running costs" });
    const names = within(costs).getByRole("checkbox", { name: "Show the names of the items" });
    const memos = within(costs).getByRole("checkbox", { name: "Show the memos of the items" });
    const income = within(costs).getByRole("radiogroup", { name: "Income" });
    expect(names).not.toBeChecked();
    expect(memos).not.toBeChecked();
    expect(memos).toHaveAccessibleDescription(/meant for the operators only/u);
    expect(within(income).getByRole("radio", { name: "Totals only" })).toBeChecked();

    await user.click(memos);
    await user.click(within(income).getByRole("radio", { name: "Totals and members of each plan" }));
    await user.click(screen.getByRole("button", { name: "Save the settings" }));

    expect(
      await screen.findByText("Saved the settings. They take effect at once, and the audit log records each change."),
    ).toBeInTheDocument();
    expect(settings.patches).toEqual([
      { values: { finance_viewer_item_memos: "true", finance_viewer_income: "plans" } },
    ]);
    expect(names).not.toBeChecked();
    expect(memos).toBeChecked();
  });

  test("requires a description of every image", async () => {
    const settings = settingsServer();
    const { user } = await renderApp({ path: "/admin/settings" });
    const posts = await screen.findByRole("region", { name: "Posts" });
    const required = within(posts).getByRole("checkbox", { name: "Require a description of every image" });
    expect(required).not.toBeChecked();
    expect(required).toHaveAccessibleDescription(
      /Posts and edits with an image without a description \(alt text\) are refused/,
    );

    await user.click(required);
    await user.click(screen.getByRole("button", { name: "Save the settings" }));

    expect(
      await screen.findByText("Saved the settings. They take effect at once, and the audit log records each change."),
    ).toBeInTheDocument();
    expect(settings.patches).toEqual([{ values: { require_media_description: "true" } }]);
    expect(required).toBeChecked();
  });

  test("sends what the server states to search engines and AI, and its security contact", async () => {
    const settings = settingsServer();
    const { user } = await renderApp({ path: "/admin/settings" });
    const crawlers = await screen.findByRole("region", { name: "Search engines and AI" });
    // Unset, the server denies both.
    expect(within(crawlers).getByRole("radio", { name: "Deny indexing" })).toBeChecked();
    expect(within(crawlers).getByRole("radio", { name: "Deny training and use" })).toBeChecked();

    await user.click(within(crawlers).getByRole("radio", { name: "Allow indexing" }));
    await user.click(within(crawlers).getByRole("checkbox", { name: "New accounts allow search indexing" }));
    await user.click(within(crawlers).getByRole("radio", { name: "Deny training" }));
    await user.type(within(crawlers).getByRole("textbox", { name: "Security contact" }), "mailto:security@example.com");
    await user.click(screen.getByRole("button", { name: "Save the settings" }));

    expect(await screen.findByText(/^Saved the settings\./)).toBeInTheDocument();
    expect(settings.patches).toEqual([
      {
        values: {
          search_indexing: "allow",
          indexable_default: "true",
          ai_usage: "deny_training",
          security_contact: "mailto:security@example.com",
        },
      },
    ]);
  });

  test("sends the default language picked from the app's languages", async () => {
    const settings = settingsServer();
    const { user } = await renderApp({ path: "/admin/settings" });
    const language = await screen.findByRole("region", { name: "Language" });
    await user.click(within(language).getByRole("button", { name: /Default language/ }));
    await user.click(await screen.findByRole("option", { name: "Japanese" }));
    await user.click(screen.getByRole("button", { name: "Save the settings" }));

    expect(await screen.findByText(/^Saved the settings\./)).toBeInTheDocument();
    expect(settings.patches).toEqual([{ values: { default_locale: "ja" } }]);
  });

  test("offers the four federation directions, both ways by default", async () => {
    settingsServer();
    await renderApp({ path: "/admin/settings" });
    const direction = await screen.findByRole("radiogroup", { name: "Federation direction" });
    expect(
      within(direction)
        .getAllByRole("radio")
        .map((radio) => radio.getAttribute("value")),
    ).toEqual(["both", "inbound", "outbound", "off"]);
    expect(within(direction).getByRole("radio", { name: "Both ways" })).toBeChecked();
  });

  test("asks before narrowing the federation direction, saying what stops, and saves once confirmed", async () => {
    const settings = settingsServer();
    const { user } = await renderApp({ path: "/admin/settings" });
    const direction = await screen.findByRole("radiogroup", { name: "Federation direction" });
    await user.click(within(direction).getByRole("radio", { name: "Receive only" }));
    await user.click(screen.getByRole("button", { name: "Save the settings" }));

    const dialog = await screen.findByRole("alertdialog", {
      name: "Change the federation direction to “Receive only”?",
    });
    const items = within(dialog)
      .getAllByRole("listitem")
      .map((item) => item.textContent);
    expect(items).toEqual([
      expect.stringContaining("no longer reach other servers. Followers there stay but receive nothing"),
      expect.stringContaining("nothing is sent to end the follows"),
    ]);
    // The least destructive choice has the focus (APG, alert dialog pattern); cancelling saves nothing.
    expect(within(dialog).getByRole("button", { name: "Cancel" })).toHaveFocus();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(settings.patches).toEqual([]);
    expect(within(direction).getByRole("radio", { name: "Receive only" })).toBeChecked();

    await user.click(screen.getByRole("button", { name: "Save the settings" }));
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Change and save" }));
    expect(
      await screen.findByText("Saved the settings. They take effect at once, and the audit log records each change."),
    ).toBeInTheDocument();
    expect(settings.patches).toEqual([{ values: { federation_mode: "inbound" } }]);
  });

  test("turning federation off says that deletions meanwhile do not reach other servers", async () => {
    settingsServer();
    const { user } = await renderApp({ path: "/admin/settings" });
    const direction = await screen.findByRole("radiogroup", { name: "Federation direction" });
    await user.click(within(direction).getByRole("radio", { name: "Off" }));
    await user.click(screen.getByRole("button", { name: "Save the settings" }));

    const dialog = await screen.findByRole("alertdialog", { name: "Change the federation direction to “Off”?" });
    expect(within(dialog).getByText(/Changing from “Both ways” to “Off”/)).toBeInTheDocument();
    expect(within(dialog).getAllByRole("listitem")).toHaveLength(4);
    expect(within(dialog).getByText(/Posts and reactions from other servers no longer come in/)).toBeInTheDocument();
    expect(
      within(dialog).getByText(/deleted while federation is off are not deleted on other servers/),
    ).toBeInTheDocument();
  });

  test("widening the federation direction saves without asking", async () => {
    const settings = settingsServer();
    server.use(
      http.get(`${apiBase}/admin/settings`, () =>
        HttpResponse.json(instanceSettingsFixture({ federation_mode: "inbound" })),
      ),
    );
    const { user } = await renderApp({ path: "/admin/settings" });
    const direction = await screen.findByRole("radiogroup", { name: "Federation direction" });
    expect(within(direction).getByRole("radio", { name: "Receive only" })).toBeChecked();
    await user.click(within(direction).getByRole("radio", { name: "Both ways" }));
    await user.click(screen.getByRole("button", { name: "Save the settings" }));

    expect(
      await screen.findByText("Saved the settings. They take effect at once, and the audit log records each change."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(settings.patches).toEqual([{ values: { federation_mode: "both" } }]);
  });

  test("shows why a save failed, keeps the choices and changes nothing", async () => {
    const refusal: Schemas["Problem"] = {
      type: "about:blank",
      title: "Unprocessable",
      status: 422,
      detail: "values.registration_mode is invalid",
    };
    server.use(
      http.patch(`${apiBase}/admin/settings`, () =>
        HttpResponse.json(refusal, { status: 422, headers: { "Content-Type": "application/problem+json" } }),
      ),
    );
    const { user } = await renderApp({ path: "/admin/settings" });
    const registration = await screen.findByRole("region", { name: "Registration" });
    await user.click(within(registration).getByRole("radio", { name: "Nobody" }));
    await user.click(screen.getByRole("button", { name: "Save the settings" }));

    expect(
      await screen.findByText("Could not save the settings; nothing was changed: values.registration_mode is invalid"),
    ).toHaveAttribute("role", "alert");
    expect(within(registration).getByRole("radio", { name: "Nobody" })).toBeChecked();
  });

  test("has no voice chat section while the server does not offer voice chat", async () => {
    settingsServer();
    await renderApp({ path: "/admin/settings" });
    expect(await screen.findByRole("region", { name: "Registration" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Voice chat" })).not.toBeInTheDocument();
  });

  test("chooses who starts voice chat rooms, the visitors' summary, and the days the records are kept, checking the days", async () => {
    const settings = settingsServer();
    voiceOn();
    const { user } = await renderApp({ path: "/admin/settings" });
    const voice = await screen.findByRole("region", { name: "Voice chat" });
    const starters = within(voice).getByRole("radiogroup", { name: "Who can start rooms" });
    expect(within(starters).getByRole("radio", { name: "Only people with the permission" })).toBeChecked();
    const summary = within(voice).getByRole("checkbox", { name: "Show a summary of rooms to visitors" });
    expect(summary).not.toBeChecked();
    expect(summary).toHaveAccessibleDescription(/They never see who is in it\./);
    const days = within(voice).getByRole("textbox", { name: "Days to keep the participants' records" });
    expect(days).toHaveValue("90");
    expect(days).toHaveAccessibleDescription(/30 to 3650 days/);

    await user.click(within(starters).getByRole("radio", { name: "Everyone on this server" }));
    await user.click(summary);
    await user.clear(days);
    await user.type(days, "20");
    await user.click(screen.getByRole("button", { name: "Save the settings" }));
    // Too few days stay in the form with what to change, and nothing is sent.
    expect(days).toHaveAccessibleDescription(/Enter a whole number of days from 30 to 3650\./);
    expect(settings.patches).toHaveLength(0);

    await user.clear(days);
    await user.type(days, " 45 ");
    await user.click(screen.getByRole("button", { name: "Save the settings" }));
    expect(
      await screen.findByText("Saved the settings. They take effect at once, and the audit log records each change."),
    ).toBeInTheDocument();
    expect(settings.patches).toEqual([
      { values: { voice_room_starters: "everyone", voice_public_summary: "true", retention_voice_participants: "45" } },
    ]);
    expect(days).toHaveValue("45");
  });

  test("chooses who creates invites and the caps of the members' invites, checking the numbers", async () => {
    const settings = settingsServer();
    const { user } = await renderApp({ path: "/admin/settings" });
    const invites = await screen.findByRole("region", { name: "Member invites" });
    const creators = within(invites).getByRole("radiogroup", { name: "Who can create invites" });
    expect(within(creators).getByRole("radio", { name: "Only people with the permission" })).toBeChecked();
    const everyone = within(creators).getByRole("radio", { name: "Everyone on this server" });
    // Inviting skips the review of the approval mode, which the choice says.
    expect(everyone).toHaveAccessibleDescription(/the people they invite skip the review/);
    const active = within(invites).getByRole("textbox", { name: "Usable invites per person" });
    expect(active).toHaveValue("5");
    expect(active).toHaveAccessibleDescription(/0 to 100\. 0 lets nobody create invites\./);
    const uses = within(invites).getByRole("textbox", { name: "Sign-ups per invite" });
    expect(uses).toHaveValue("10");
    expect(uses).toHaveAccessibleDescription(/1 to 1000\./);
    const expiry = within(invites).getByRole("button", { name: /Longest time an invite works/ });
    expect(expiry).toHaveTextContent("30 days");

    await user.click(everyone);
    await user.clear(uses);
    await user.type(uses, "2000");
    await user.click(screen.getByRole("button", { name: "Save the settings" }));
    // A number out of bounds stays in the form with what to change, and nothing is sent.
    expect(uses).toHaveAccessibleDescription(/Enter a whole number from 1 to 1000\./);
    expect(settings.patches).toHaveLength(0);

    await user.clear(uses);
    await user.type(uses, "3");
    await user.clear(active);
    await user.type(active, "0");
    await user.click(expiry);
    await user.click(await screen.findByRole("option", { name: "7 days" }));
    await user.click(screen.getByRole("button", { name: "Save the settings" }));
    expect(
      await screen.findByText("Saved the settings. They take effect at once, and the audit log records each change."),
    ).toBeInTheDocument();
    expect(settings.patches).toEqual([
      {
        values: {
          invite_creators: "everyone",
          member_invite_max_active: "0",
          member_invite_max_uses: "3",
          member_invite_max_expires_in: String(7 * 86_400),
        },
      },
    ]);
  });
});

describe("the server's contact and thumbnail", () => {
  const uploadedId = "0199f0e0-0000-7000-8000-00000000c0a1";

  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_settings"]));
  });

  /** Plays the settings and the alt texts; records the changes and the purposes of the uploads. */
  function contactServer(stored: Readonly<Partial<Record<Schemas["InstanceSettingKey"], string>>>) {
    let current = { ...stored };
    const patches: Schemas["UpdateInstanceSettingsRequest"][] = [];
    const alts: string[] = [];
    const purposes: string[] = [];
    server.use(
      http.get(`${apiBase}/instance`, () =>
        HttpResponse.json({
          ...instanceFixture,
          ...(current.thumbnail
            ? {
                thumbnail: {
                  url: `https://my.example/media/${current.thumbnail}/r/large.webp`,
                  width: 1600,
                  height: 900,
                },
              }
            : {}),
        }),
      ),
      http.get(`${apiBase}/admin/settings`, () => HttpResponse.json(instanceSettingsFixture(current))),
      http.patch(`${apiBase}/admin/settings`, async ({ request }) => {
        const body = (await request.json()) as Schemas["UpdateInstanceSettingsRequest"];
        patches.push(body);
        current = { ...current, ...body.values };
        return HttpResponse.json(instanceSettingsFixture(current));
      }),
      http.put(`${apiBase}/admin/instance_texts/thumbnail_alt`, async ({ request }) => {
        const { body } = (await request.json()) as Schemas["SetInstanceTextRequest"];
        alts.push(body);
        return HttpResponse.json({ key: "thumbnail_alt", body, revised_at: "2026-10-17T00:00:00Z" } satisfies Text);
      }),
      http.post(`${apiBase}/media`, async ({ request }) => {
        purposes.push(/name="purpose"\r\n\r\n([a-z]+)/u.exec(await request.text())?.[1] ?? "");
        return HttpResponse.json(mediaFixture(uploadedId), { status: 202 });
      }),
    );
    return { patches, alts, purposes };
  }

  test("saves the contact account, the email address and a new thumbnail with its description", async () => {
    const contact = contactServer({ contact_email: "old@my.example" });
    const { user } = await renderApp({ path: "/admin/settings" });
    const section = await screen.findByRole("region", { name: "Contact and thumbnail" });
    const email = within(section).getByRole("textbox", { name: "Contact email address" });
    expect(email).toHaveValue("old@my.example");

    await user.type(within(section).getByRole("textbox", { name: "Contact account" }), "@Admin");
    await user.clear(email);
    await user.type(email, "admin@my.example");
    // eslint-disable-next-line testing-library/no-node-access -- FileTrigger's input is hidden and has no role.
    const input = section.querySelector<HTMLInputElement>('input[type="file"]');
    if (input === null) throw new Error("no file input");
    await user.upload(input, nodeUploadFile("png", "beach.png", "image/png"));
    expect(await within(section).findByRole("button", { name: "Remove the image" })).toBeInTheDocument();
    await user.type(within(section).getByRole("textbox", { name: "Description of the image" }), "A beach at dawn");
    await user.click(within(section).getByRole("button", { name: "Save the contact and thumbnail" }));

    expect(
      await within(section).findByText("Saved the contact and thumbnail. The audit log records the change."),
    ).toBeInTheDocument();
    expect(contact.purposes).toEqual(["attachment"]);
    // The @ that people type before a username is not part of it.
    expect(contact.patches).toEqual([
      { values: { contact_account: "Admin", contact_email: "admin@my.example", thumbnail: uploadedId } },
    ]);
    expect(contact.alts).toEqual(["A beach at dawn"]);

    // Saving again without a change sends nothing.
    await user.click(within(section).getByRole("button", { name: "Save the contact and thumbnail" }));
    expect(await within(section).findByText("The contact and thumbnail are already saved.")).toBeInTheDocument();
    expect(contact.patches).toHaveLength(1);
  });

  test("removes the thumbnail", async () => {
    const contact = contactServer({ thumbnail: uploadedId });
    const { user } = await renderApp({ path: "/admin/settings" });
    const section = await screen.findByRole("region", { name: "Contact and thumbnail" });
    await user.click(await within(section).findByRole("button", { name: "Remove the image" }));
    await user.click(within(section).getByRole("button", { name: "Save the contact and thumbnail" }));

    expect(
      await within(section).findByText("Saved the contact and thumbnail. The audit log records the change."),
    ).toBeInTheDocument();
    expect(contact.patches).toEqual([{ values: { thumbnail: "" } }]);
  });
});
