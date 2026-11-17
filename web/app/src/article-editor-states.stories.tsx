// SPDX-License-Identifier: AGPL-3.0-only

// The states of the article editor that follow what the writer writes or presses (#3091), on the fake server as yuki
// writes: a draft whose title, or whose body, is over its limit, which the server is seeded with; and those no address
// leads to, which the editor is opened in (its specimen), the editor alone on a fake server of its own: the preview
// behind Write | Preview (under `desktop`, where one of them shows), the schedule's form, the audience menu (the
// composer's: who can see it, never direct, and who can reply, boost and quote it, with approval) and its submenus,
// and what the server refused of a publication or of an edit.

import { AppDataProvider, createAppData, defaultComposeSettings } from "@my/app-data";
import { ArticleEditor, emptyArticle, type ArticleOpening, type ArticleSpecimen } from "@my/app-features";
import { MenuSheet } from "@my/design-system";
import { createFakeApi, fakeOrigin, scenario, type FakeApi, type FakeSeed } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { screen, screenMeta } from "./app.samples";
import { useNarrowViewport } from "./shell/viewport";

const meta = {
  title: "Article editor states",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/articles/new" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The editor of the reader's draft of an article `key`, on a server that holds it besides the scenario. */
const draftOf = (key: string, title: string, text: string) =>
  screen({
    at: `article-draft:${key}`,
    server: {
      seed: {
        ...scenario,
        drafts: [
          ...scenario.drafts,
          { key, author: "yuki", text, language: "en", article: { title, summary: "", tags: [] } },
        ],
      } satisfies FakeSeed,
    },
  });

/** A title one character over its 200: the field is invalid and says by how much. */
export const TitleOver: Story = {
  parameters: draftOf("yuki-article-title-over", "A starter in a jar. ".repeat(11).slice(0, 201), "Flour and water."),
};

/** A body whose HTML is over the 100,000 bytes (102,000 characters): the limits' line says by how much. */
export const BodyOver: Story = {
  parameters: draftOf("yuki-article-body-over", "A starter in a jar", "flour ".repeat(17_000)),
};

/** What a specimen of the editor opens: a new article, or the reader's draft or article of a key of the scenario. */
type Opens = { kind: "new" } | { kind: "draft" | "edit"; key: string };

/**
 * The editor alone, opened in `specimen`, on a fake server of its own as `viewer` reads it (yuki by default). Its menus
 * and the schedule's form open in a sheet on a phone's viewport, as the app decides, else in a popover.
 */
function Specimen({ opens, viewer, specimen }: { opens: Opens; viewer?: string; specimen: ArticleSpecimen }) {
  const narrow = useNarrowViewport();
  const [{ data, opening }] = useState(() => {
    const fake: FakeApi = createFakeApi(viewer === undefined ? {} : { viewer });
    const data = createAppData(createClient({ server: fakeOrigin, retry: { disabled: true }, fetch: fake.serve }));
    const opening: ArticleOpening = opens.kind === "new" ? opens : { kind: opens.kind, id: fake.idOf(opens.key) };
    return { data, opening };
  });
  return (
    <AppDataProvider data={data}>
      <MenuSheet sheet={narrow}>
        <ArticleEditor opening={opening} onLeave={() => undefined} onOpenDraft={() => undefined} specimen={specimen} />
      </MenuSheet>
    </AppDataProvider>
  );
}

/** A story of the editor opened in `specimen`. */
const opened = (specimen: ArticleSpecimen, opens: Opens = { kind: "new" }, viewer?: string): Story => ({
  render: (_, { id }) => (
    <Specimen key={id} opens={opens} specimen={specimen} {...(viewer === undefined ? {} : { viewer })} />
  ),
});

const draft: Opens = { kind: "draft", key: "yuki-article-trail" };
const published: Opens = { kind: "edit", key: "sam-article" };

/**
 * A draft previewed from Write | Preview, which shows under `desktop` alone: from it, the preview stands beside what
 * is written either way.
 */
export const Preview: Story = opened({ mode: "preview" }, draft);

/**
 * The schedule's form open from the row's tool: a sheet headed by its name and close on a phone, from `tablet` a
 * popover over the tool.
 */
export const ScheduleForm: Story = opened({ opened: "schedule" });

/** The composer's audience menu, open from the row's tool: who can see the article, and who can reply, boost and quote it. */
export const AudienceMenu: Story = opened({ opened: "audience" });

/** The audience menu's visibilities: the article's four, from the widest, and no Direct. */
export const AudienceVisibilities: Story = opened({ opened: "visibility" });

/** Who can reply, set to followers: the audiences, and under them whether others may ask for approval. */
export const AudienceReply: Story = opened({
  opened: "reply",
  initial: {
    ...emptyArticle(defaultComposeSettings, "article-editor-states"),
    policy: { ...defaultComposeSettings.policy, reply: { audience: "followers", approval: false } },
  },
});

/** A publication the server's review refused: its words before Publish, what is written kept. */
export const PublishingError: Story = opened(
  {
    phase: {
      kind: "failed",
      reason: "This server's review did not accept the article. Change it and try again.",
    },
  },
  draft,
);

/**
 * An edit the server refused for the value of the summary, which it named: what is wrong stands under the summary, the
 * problem's title alone before Save.
 */
export const SummaryRefused: Story = opened(
  {
    phase: {
      kind: "failed",
      reason: "This could not be done.",
      field: { input: "summary", text: "This value is not valid. Check what you entered." },
    },
  },
  published,
  "sam",
);
