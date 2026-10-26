// SPDX-License-Identifier: AGPL-3.0-only
// The page of a report of a voice chat room through the whole app (web-client/README.md T4, ADR-0100 §11, §13): a
// moderator with manage_reports reads the room's revisions, participations and bans in place of the posts, and ends
// the open room with an optional reason, after a confirmation, with MSW playing the admin API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { accountFixture, otherAccountFixture, type Schemas, viewerFixture } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { reportFixture } from "./test/handlers";

const roomId = "0192f0c4-7a8e-7000-8000-0000000000f1";
const carol = accountFixture(7, "Carol", "my.example");

const openingRevision: Schemas["AdminVoiceRoomRevision"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000a0001",
  name: "Friday night radio",
  description: "",
  hashtags: [],
  visibility: "local",
  speakers: "manual",
  revised_by_account_id: otherAccountFixture.id,
  revised_at: "2026-10-26T12:00:00Z",
};
const reportedRevision: Schemas["AdminVoiceRoomRevision"] = {
  ...openingRevision,
  id: "0192f0c4-7a8e-7000-8000-0000000a0002",
  name: "Buy my stuff",
  description: "Discounts every hour.",
  hashtags: ["Deals"],
  speakers: "everyone",
  revised_at: "2026-10-26T12:30:00Z",
};

/** Bob's open room as the report's page shows it: two revisions, Carol removed after joining, Alice still in. */
function roomFixture(overrides: Partial<Schemas["AdminReportVoiceRoom"]> = {}): Schemas["AdminReportVoiceRoom"] {
  return {
    id: roomId,
    reported_revision_id: reportedRevision.id,
    opened_at: "2026-10-26T12:00:00Z",
    revisions: [openingRevision, reportedRevision],
    participations: [
      { account_id: otherAccountFixture.id, joined_at: "2026-10-26T12:00:00Z" },
      { account_id: carol.id, joined_at: "2026-10-26T12:10:00Z", left_at: "2026-10-26T12:20:00Z", leave_reason: "removed" },
      { account_id: viewerFixture.id, joined_at: "2026-10-26T12:40:00Z" },
    ],
    bans: [{ account_id: carol.id, banned_by_account_id: otherAccountFixture.id, banned_at: "2026-10-26T12:20:00Z" }],
    ...overrides,
  };
}

const reportId = "0192f0c4-7a8e-7000-8000-0000000c0009";

/** Plays the report of the room and ending it; records the bodies of POST /admin/voice_rooms/{id}/end. */
function roomReportServer(room: Schemas["AdminReportVoiceRoom"]) {
  let current = room;
  const ends: Schemas["AdminVoiceRoomEnd"][] = [];
  server.use(
    http.get(`${apiBase}/admin/reports/:id`, () =>
      HttpResponse.json(reportFixture({ id: reportId, status_ids: [], statuses: [], voice_room_id: roomId, voice_room: current, comment: "Selling things." })),
    ),
    http.get(`${apiBase}/accounts/:id`, ({ params }) => (params["id"] === carol.id ? HttpResponse.json(carol) : undefined)),
    http.post(`${apiBase}/admin/voice_rooms/:id/end`, async ({ request }) => {
      const body = (await request.json()) as Schemas["AdminVoiceRoomEnd"];
      ends.push(body);
      current = {
        ...current,
        ended_at: "2026-10-26T13:00:00Z",
        end_reason: "moderator",
        ended_by_account_id: viewerFixture.id,
        ...(body.reason === undefined ? {} : { end_note: body.reason }),
        participations: current.participations.map((participation) =>
          participation.left_at === undefined ? { ...participation, left_at: "2026-10-26T13:00:00Z", leave_reason: "room_ended" } : participation,
        ),
      };
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return { ends };
}

beforeEach(() => {
  server.use(signedInHandler, operatorHandler(["manage_reports"]));
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

describe("the report of a voice chat room", () => {
  test("shows the room's revisions, participants and bans in place of the posts", async () => {
    roomReportServer(roomFixture());
    await renderApp({ path: `/moderation/reports/${reportId}` });
    const section = await screen.findByRole("region", { name: "Voice chat room" });
    expect(screen.queryByRole("region", { name: "Posts" })).not.toBeInTheDocument();
    expect(within(section).getByText("Open")).toBeInTheDocument();

    // Oldest first, the one in effect when the room was reported marked.
    const [opening, reported, ...rest] = within(within(section).getByRole("region", { name: "Revisions of the settings" })).getAllByRole("listitem");
    expect(rest).toHaveLength(0);
    expect(opening).toHaveTextContent("Friday night radio");
    expect(opening).not.toHaveTextContent("When reported");
    expect(reported).toHaveTextContent(/When reported.*Buy my stuff.*#Deals.*Everyone who joins.*Discounts every hour\./u);

    const participants = within(section).getByRole("region", { name: "Participants" });
    expect(await within(participants).findAllByText("Carol")).not.toHaveLength(0);
    expect(within(participants).getByText("Removed by a host")).toBeInTheDocument();
    expect(within(participants).getAllByText("Still in the room")).toHaveLength(2);
    const bans = within(section).getByRole("region", { name: "Removed from the room" });
    expect(await within(bans).findByText("Carol")).toBeInTheDocument();
  });

  test("ends the open room with a reason after its confirmation, then shows it ended", async () => {
    const api = roomReportServer(roomFixture());
    const { user } = await renderApp({ path: `/moderation/reports/${reportId}` });
    const section = await screen.findByRole("region", { name: "Voice chat room" });
    await user.click(within(section).getByRole("button", { name: "End room" }));
    const dialog = await screen.findByRole("alertdialog", { name: "End this room?" });
    const reason = within(dialog).getByRole("textbox", { name: "Reason (optional)" });
    await waitFor(() => {
      expect(reason).toHaveFocus();
    });
    await user.type(reason, "  Advertising in a room  ");
    await user.click(within(dialog).getByRole("button", { name: "End room" }));

    expect(await within(section).findByText("Ended the room.")).toBeInTheDocument();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(api.ends).toEqual([{ reason: "Advertising in a room" }]);
    expect(within(section).getByText("A moderator ended it")).toBeInTheDocument();
    expect(within(section).getByText("Advertising in a room")).toBeInTheDocument();
    expect(within(section).queryByRole("button", { name: "End room" })).not.toBeInTheDocument();
    // The pressed button went away, so focus is on the section's heading.
    await waitFor(() => {
      expect(within(section).getByRole("heading", { level: 2, name: "Voice chat room" })).toHaveFocus();
    });
    expect(within(within(section).getByRole("region", { name: "Participants" })).getAllByText("The room ended")).toHaveLength(2);
  });

  test("ends the room without a reason", async () => {
    const api = roomReportServer(roomFixture());
    const { user } = await renderApp({ path: `/moderation/reports/${reportId}` });
    const section = await screen.findByRole("region", { name: "Voice chat room" });
    await user.click(within(section).getByRole("button", { name: "End room" }));
    await user.click(within(await screen.findByRole("alertdialog", { name: "End this room?" })).getByRole("button", { name: "End room" }));
    expect(await within(section).findByText("Ended the room.")).toBeInTheDocument();
    expect(api.ends).toEqual([{}]);
  });

  test("keeps the confirmation and the reason when ending fails", async () => {
    roomReportServer(roomFixture());
    server.use(http.post(`${apiBase}/admin/voice_rooms/:id/end`, () => problem(404, "about:blank", "Not Found")));
    const { user } = await renderApp({ path: `/moderation/reports/${reportId}` });
    const section = await screen.findByRole("region", { name: "Voice chat room" });
    await user.click(within(section).getByRole("button", { name: "End room" }));
    const dialog = await screen.findByRole("alertdialog", { name: "End this room?" });
    await user.type(within(dialog).getByRole("textbox", { name: "Reason (optional)" }), "Spam");
    await user.click(within(dialog).getByRole("button", { name: "End room" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent(/^Couldn't end the room: /u);
    expect(within(dialog).getByRole("textbox", { name: "Reason (optional)" })).toHaveValue("Spam");
  });

  test("offers no ending for a room that ended, and names who ended it and why", async () => {
    roomReportServer(roomFixture({ ended_at: "2026-10-26T13:00:00Z", end_reason: "host", ended_by_account_id: otherAccountFixture.id }));
    await renderApp({ path: `/moderation/reports/${reportId}` });
    const section = await screen.findByRole("region", { name: "Voice chat room" });
    expect(within(section).getByText("Ended")).toBeInTheDocument();
    expect(within(section).getByText("The host ended it")).toBeInTheDocument();
    expect(within(section).queryByRole("button", { name: "End room" })).not.toBeInTheDocument();
  });
});
