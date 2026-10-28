// SPDX-License-Identifier: AGPL-3.0-only
// A post and the thread around it (/@{acct}/status/{id}; FR-113) in the states of ADR-0112 §1: replies, a quote,
// a poll, more pictures than a post shows, a content warning, and a link card.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect } from "storybook/test";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem } from "../../test/msw/http";
import {
  abdulrahman,
  bob,
  cardPost,
  carol,
  dmitri,
  haruka,
  longPost,
  manyPicturesPost,
  maximiliana,
  picture,
  pictureHandlers,
  pollPost,
  post,
  postId,
  quotePost,
  viewer,
  viewerPost,
  warnedPost,
} from "../../test/postStories";
import {
  answeringWithErrorPage,
  failingOn,
  inLocale,
  loadingOn,
  narrow,
  renderScreen,
  screenLoader,
  settled,
} from "../../test/stories";

type Status = Schemas["Status"];

/** The question that the post answers. */
const question = post(60, maximiliana, {
  content: "<p>Does anyone have a sourdough guide for someone who has never baked bread before?</p>",
  replies_count: 1,
  favourites_count: 4,
});

/** The post of the page: Bob's guide, in reply to the question. */
const focused: Status = {
  ...longPost,
  in_reply_to_id: question.id,
  in_reply_to_account_id: maximiliana.id,
  created_at: "2026-10-28T12:30:00Z",
};

/** The replies to the guide, one of them answered in turn, oldest first. */
const replies: Status[] = [
  post(61, haruka, {
    content: "<p>ありがとうございます！今週末さっそく試してみます。</p>",
    language: "ja",
    in_reply_to_id: focused.id,
    created_at: "2026-10-28T12:40:00Z",
    favourites_count: 2,
  }),
  post(62, abdulrahman, {
    content: "<p>شكراً على المشاركة، هل يمكن استخدام دقيق القمح الكامل؟</p>",
    language: "ar",
    in_reply_to_id: focused.id,
    created_at: "2026-10-28T12:50:00Z",
    replies_count: 1,
  }),
  post(63, bob, {
    content: "<p>Yes, but start with a quarter of whole wheat and add more once the starter is strong.</p>",
    in_reply_to_id: postId(62),
    created_at: "2026-10-28T13:00:00Z",
    favourites_count: 1,
  }),
  post(64, dmitri, {
    content: "<p>My first loaf, following the guide step by step. Not perfect, but I am proud of it.</p>",
    in_reply_to_id: focused.id,
    created_at: "2026-10-28T13:10:00Z",
    media_attachments: [picture(20, "A round loaf with a dark crust on a cooling rack")],
    favourites_count: 17,
  }),
  post(65, viewer, {
    content: "<p>Bookmarked. The section on shaping is the clearest I have read.</p>",
    in_reply_to_id: focused.id,
    created_at: "2026-10-28T13:20:00Z",
  }),
];

/** GET /statuses/{id} answering each of `statuses` by its ID, and 404 for any other. */
function statuses(...items: Status[]) {
  return http.get(`${apiBase}/statuses/:id`, ({ params }) => {
    const found = items.find((status) => status.id === params["id"]);
    return found === undefined ? problem(404, "about:blank", "Not Found") : HttpResponse.json(found);
  });
}

/** GET /statuses/{id}/context answering `context`. */
function thread(context: Schemas["StatusContext"]) {
  return http.get(`${apiBase}/statuses/:id/context`, () => HttpResponse.json(context));
}

/** The page of `status`, alone in its thread. */
function alone(status: Status) {
  return {
    parameters: {
      screen: {
        path: `/@${status.account.acct}/status/${status.id}`,
        handlers: [...pictureHandlers, statuses(status), thread({ ancestors: [], descendants: [] })],
      },
    },
  };
}

const meta = {
  title: "Screens/Timeline/Post",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: `/@${focused.account.acct}/status/${focused.id}`,
      signedIn: true,
      handlers: [
        ...pictureHandlers,
        statuses(question, focused, ...replies),
        thread({ ancestors: [question], descendants: replies }),
      ],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A reply in a thread: the question above it, and replies in several languages below, one answered in turn. */
export const Normal: Story = {};

/** To someone who is not signed in. */
export const SignedOut: Story = { parameters: { screen: { signedIn: false } } };

/** A post nobody replied to. */
export const NoReplies: Story = alone({ ...longPost, replies_count: 0 });

/** A post with a poll. */
export const Poll: Story = alone(pollPost);

/** A post that quotes another. */
export const Quote: Story = alone(quotePost);

/** A quote whose quoted post is gone. */
export const QuoteDeleted: Story = alone({ ...quotePost, quote: { state: "deleted" } });

/** A post of another server with seven pictures, of which the server kept four. */
export const ManyPictures: Story = alone(manyPicturesPost);

/** A post behind a content warning, opened. */
export const ContentWarning: Story = {
  ...alone(warnedPost),
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    const toggle = await canvas.findByRole("button", { name: "Show content", expanded: false });
    await userEvent.click(toggle);
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
  },
};

/** A post with a link card. */
export const LinkCard: Story = alone(cardPost);

/** The viewer's own post, with its pictures. */
export const OwnPost: Story = alone(viewerPost);

/** A post of another server. */
export const RemotePost: Story = alone(
  post(66, carol, { content: "<p>Greetings from another server!</p>", boosts_count: 3 }),
);

/** The post does not exist, or the viewer may not see it. */
export const NotFound: Story = answeringWithErrorPage(
  http.get(`${apiBase}/statuses/:id`, () => problem(404, "about:blank", "Not Found")),
);

/** The post has not arrived yet. */
export const Loading: Story = loadingOn("get", `/statuses/${focused.id}`);

/** The thread around the post has not arrived yet. */
export const ThreadLoading: Story = loadingOn("get", `/statuses/${focused.id}/context`);

/** The server failed to answer the post. */
export const ServerError: Story = failingOn("get", `/statuses/${focused.id}`);

/** The server failed to answer the thread around the post. */
export const ThreadError: Story = failingOn("get", `/statuses/${focused.id}/context`);

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;

/** At 320 CSS px, a post of seven pictures. */
export const NarrowManyPictures: Story = { ...narrow, ...alone(manyPicturesPost) };
