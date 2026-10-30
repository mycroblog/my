// SPDX-License-Identifier: AGPL-3.0-only
// The theme screen through the whole app (web-client/README.md T4, ADR-0059): an administrator with manage_settings
// overrides design tokens and adds a custom CSS, sees the contrast of the color pairs as they type, and gets every
// reason the server refuses a theme; with MSW playing the API.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

describe("the server's theme", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_settings"]));
  });

  test("shows the saved overrides, and the defaults of the other tokens", async () => {
    server.use(
      http.get(`${apiBase}/admin/theme`, () =>
        HttpResponse.json({
          revision: "r1",
          revised_at: "2026-10-20T00:00:00Z",
          stylesheet_url: "https://my.example/theme/r1.css",
          tokens: [
            { token: "color-accent", light: "#1447e6", dark: "#92c5fe" },
            { token: "radius-md", value: "2px" },
          ],
          custom_css: ".brand { color: red }",
        } satisfies Schemas["AdminTheme"]),
      ),
    );
    await renderApp({ path: "/admin/theme" });

    expect(await screen.findByRole("heading", { level: 1, name: "Theme" })).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "color-accent Light" })).toHaveValue("#1447e6");
    expect(screen.getByRole("textbox", { name: "color-surface Light" })).toHaveValue("");
    expect(screen.getByRole("textbox", { name: "color-surface Light" })).toHaveAccessibleDescription(
      /Default: #fcf9f6/,
    );
    expect(screen.getByRole("textbox", { name: "radius-md" })).toHaveValue("2px");
    expect(screen.getByRole("textbox", { name: "Custom CSS" })).toHaveValue(".brand { color: red }");
    expect(
      within(screen.getByRole("region", { name: "Contrast" })).getByText(
        /Every text and control color has enough contrast/,
      ),
    ).toBeInTheDocument();
  });

  test("shows the pairs that lose contrast as a color is typed, and saves the overrides", async () => {
    let saved: Schemas["SetThemeRequest"] | undefined;
    server.use(
      http.put(`${apiBase}/admin/theme`, async ({ request }) => {
        saved = (await request.json()) as Schemas["SetThemeRequest"];
        return HttpResponse.json({
          ...saved,
          revision: "r2",
          revised_at: "2026-10-20T00:00:00Z",
        } satisfies Schemas["AdminTheme"]);
      }),
    );
    const { user } = await renderApp({ path: "/admin/theme" });
    await user.type(await screen.findByRole("textbox", { name: "color-fg-muted Light" }), "#cccccc");

    const contrast = screen.getByRole("region", { name: "Contrast" });
    expect(
      within(contrast).getByText(
        /color-fg-muted on color-surface has a contrast of [\d.]+:1 in the light theme; it needs 4.5:1/,
      ),
    ).toBeInTheDocument();

    await user.clear(screen.getByRole("textbox", { name: "color-fg-muted Light" }));
    await user.type(screen.getByRole("textbox", { name: "color-fg-muted Light" }), "#333333");
    await user.type(screen.getByRole("textbox", { name: "spacing" }), "0.3rem");
    await user.click(screen.getByRole("button", { name: "Save theme" }));

    expect(await screen.findByText(/Theme saved/)).toBeInTheDocument();
    expect(saved).toEqual({
      tokens: [
        // The dark theme keeps its default.
        { token: "color-fg-muted", light: "#333333", dark: "#b7a7a5" },
        { token: "spacing", value: "0.3rem" },
      ],
      custom_css: "",
    });
  });

  test("lists every reason the server refuses a theme", async () => {
    server.use(
      http.put(`${apiBase}/admin/theme`, () =>
        HttpResponse.json(
          {
            type: "/api/v1/problems/invalid-theme",
            title: "The theme cannot be saved.",
            status: 422,
            theme_errors: [
              { code: "out_of_range", token: "radius-md", min: 0, max: 2 },
              { code: "css_too_long", max: 65536 },
            ],
          } satisfies Schemas["ThemeProblem"],
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: "/admin/theme" });
    await user.type(await screen.findByRole("textbox", { name: "radius-md" }), "40px");
    await user.click(screen.getByRole("button", { name: "Save theme" }));

    await screen.findByText("The theme was not saved:");
    // React Aria's live announcer is an alert too.
    const alert = screen
      .getAllByRole("alert")
      .find((element) => within(element).queryByText("The theme was not saved:") !== null);
    if (alert === undefined) throw new Error("the reasons are not in an alert");
    expect(within(alert).getByText("radius-md must be from 0 to 2.")).toBeInTheDocument();
    expect(within(alert).getByText("The custom CSS can be at most 65536 bytes.")).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "radius-md" })).toHaveValue("40px");
  });
});
