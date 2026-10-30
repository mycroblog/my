// SPDX-License-Identifier: AGPL-3.0-only
// A list's timeline (/lists/{id}; web-client/screens/list-timeline.md, ADR-0122 §4) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { statusPage } from "../../test/fixtures";
import { apiBase, problem, withViewer } from "../../test/msw/http";
import { pictureHandlers, variedTimeline } from "../../test/postStories";
import {
  answering,
  answeringWithErrorPage,
  failingOn,
  inLocale,
  loadingOn,
  narrow,
  renderScreen,
  screenLoader,
  settled,
} from "../../test/stories";
import { friendsList, listFixture } from "./test/handlers";

const meta = {
  title: "Screens/Lists/ListTimeline",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: `/lists/${friendsList.id}`,
      signedIn: true,
      handlers: [
        ...pictureHandlers,
        http.get(`${apiBase}/timelines/list/:id`, ({ request }) =>
          withViewer(request, () => HttpResponse.json(statusPage(variedTimeline))),
        ),
      ],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The members' posts of every kind. */
export const Normal: Story = {};

/** Members who have not posted yet. */
export const NoPosts: Story = answering(
  http.get(`${apiBase}/timelines/list/:id`, () => HttpResponse.json(statusPage([]))),
);

/** A list without members: the empty state leads to managing them. */
export const NoMembers: Story = answering(
  http.get(`${apiBase}/lists/:id`, () => HttpResponse.json(listFixture(friendsList.id, "Friends"))),
  http.get(`${apiBase}/timelines/list/:id`, () => HttpResponse.json(statusPage([]))),
);

/** Another user's list, or one that is gone. */
export const NotFound: Story = answeringWithErrorPage(
  http.get(`${apiBase}/lists/:id`, () => problem(404, "/api/v1/problems/not-found", "Not Found")),
);

/** The first page has not arrived yet. */
export const Loading: Story = loadingOn("get", "/timelines/list/:id");

/** The server failed to answer the first page. */
export const ServerError: Story = failingOn("get", "/timelines/list/:id");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
