// SPDX-License-Identifier: AGPL-3.0-only
// The badges of the user's roles on the profile settings page through the whole app (web-client/README.md T4, ADR-0071):
// MSW plays the server's choices, and the user's switches change them.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, signedInHandler, withViewer } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

const staffId = "0192f0c4-7a8e-7000-8000-0000000000b1";
const supporterId = "0192f0c4-7a8e-7000-8000-0000000000b2";

/** The server's badge choices of the viewer, which PUT changes, with the requests it received. */
function badgeServer(initial: Schemas["BadgeChoice"][], refuse = false) {
  let items = initial;
  const changes: { roleId: string; displayed: boolean }[] = [];
  server.use(
    http.get(`${apiBase}/preferences/badges`, ({ request }) => withViewer(request, () => HttpResponse.json({ items }))),
    http.put(`${apiBase}/preferences/badges/:roleId`, async ({ request, params }) => {
      const roleId = String(params["roleId"]);
      const { displayed } = (await request.json()) as Schemas["UpdateBadgeChoiceRequest"];
      changes.push({ roleId, displayed });
      if (refuse) return problem(404, "about:blank", "Not Found");
      items = items.map((c) => (c.role_id === roleId ? { ...c, displayed } : c));
      const saved = items.find((c) => c.role_id === roleId);
      return saved === undefined ? problem(404, "about:blank", "Not Found") : HttpResponse.json(saved);
    }),
  );
  return { changes };
}

describe("the badges of the profile", () => {
  test("are not offered to a viewer without a role that has a badge", async () => {
    await renderApp({ path: "/settings/profile" });

    expect(await screen.findByRole("heading", { level: 1, name: "Profile" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Badges" })).not.toBeInTheDocument();
  });

  test("are hidden until switched on, each switch named by its badge", async () => {
    const { changes } = badgeServer([
      { role_id: staffId, label: "Staff", color: "purple", displayed: true },
      { role_id: supporterId, label: "サポーター", color: "green", displayed: false },
    ]);
    const { user } = await renderApp({ path: "/settings/profile" });

    const section = await screen.findByRole("region", { name: "Badges" });
    const staff = within(section).getByRole("switch", { name: "Staff" });
    const supporter = within(section).getByRole("switch", { name: "サポーター" });
    expect(staff).toBeChecked();
    expect(supporter).not.toBeChecked();
    // The badge's name is the staff's text, drawn as user content (web-client/README.md Y10).
    expect(within(section).getByText("サポーター")).toHaveAttribute("dir", "auto");

    await user.click(supporter);
    await waitFor(() => {
      expect(changes).toEqual([{ roleId: supporterId, displayed: true }]);
    });
    expect(supporter).toBeChecked();
    expect(await screen.findByText("⁨サポーター⁩ is shown on your profile.")).toBeInTheDocument();

    await user.click(staff);
    await waitFor(() => {
      expect(changes).toEqual([
        { roleId: supporterId, displayed: true },
        { roleId: staffId, displayed: false },
      ]);
    });
    expect(staff).not.toBeChecked();
  });

  test("a change the server refuses is reported under its switch, which goes back", async () => {
    badgeServer([{ role_id: supporterId, label: "Supporter", color: "blue", displayed: false }], true);
    const { user } = await renderApp({ path: "/settings/profile" });

    const supporter = within(await screen.findByRole("region", { name: "Badges" })).getByRole("switch", {
      name: "Supporter",
    });
    await user.click(supporter);

    expect(await screen.findByText(/^Could not change the badge:/)).toBeInTheDocument();
    expect(supporter).not.toBeChecked();
  });
});
