// SPDX-License-Identifier: AGPL-3.0-only

// The layout parts (ADR-0171 §3), each holding more than fits: long names, words with no place to break, more
// children than a row takes. A story per part or per parts that work together, in a page with the side margin of the
// app's column, so that a reader judges at any width of the toolbar, in either direction, where a part wraps,
// truncates or runs over; the layout tests open the same stories (web/e2e/catalog). The frame of the app has its own
// stories (Shell). The words are samples in English; what people wrote is in a `ContentBody`, as the app shows it.

import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { ArticlePane } from "./article-editor";
import { Avatar } from "./avatar";
import { Button, TextButton, TextLink } from "./button";
import { Chip, ChipRow } from "./chip";
import { AttachmentList, AttachmentRow, ChoiceSelect, PollSettings } from "./composer";
import { ContentBody, ContentLink, UrlPart } from "./content";
import { ConfirmDialog, PanelDialog } from "./dialog";
import { Disclosure } from "./disclosure";
import { DraftCount, DraftList, DraftRow, DraftSection, DraftSections } from "./drafts";
import { ActionRow, Byline, CaptionLine, CaptionName, Entry, Stamp } from "./entry";
import { Feed, FeedItem, FeedNotice } from "./feed";
import {
  CheckField,
  Form,
  FormActions,
  FormChoice,
  FormHead,
  FormLink,
  FormLinks,
  FormList,
  FormPage,
  FormPageFooter,
  FormSection,
  FormText,
  TextField,
} from "./form-page";
import { Version, VersionList } from "./history";
import { IconButton } from "./icon-button";
import { Stack } from "./layout";
import {
  AccountRow,
  BackButton,
  ButtonPair,
  OpenRow,
  RowItem,
  RowList,
  ScreenHead,
  ScreenStack,
  ScreenTitle,
} from "./list";
import { FormPopup, MenuButton, MenuGroup, MenuItem } from "./menu";
import { ago, images } from "./samples";
import { ThreadReply } from "./thread";

/** A name with no space to break at, longer than a phone's line. */
const longName = "Bartholomew-Maximilian-Featherstonehaugh-Cholmondeley-Warner";
/** A word no line of a phone holds. */
const longWord = "Donaudampfschifffahrtselektrizitätenhauptbetriebswerkbauunterbeamtengesellschaft";
const longHandle = "@bartholomew_maximilian_featherstonehaugh@a-very-long-subdomain.of.an.instance.example";
const sentence =
  "A wide valley at first light, three ridges one behind the other, the nearest dark with pines and the farthest a pale blue line.";
const nothing = () => undefined;

/**
 * The side margin of the app's column around a story, which an entry's and a row's surface reach into; a story of a
 * whole page (`parameters.page`) has none.
 */
const column: Decorator = (Story, { parameters }) =>
  parameters["page"] === true ? (
    <Story />
  ) : (
    <div style={{ padding: "1rem" }}>
      <Story />
    </div>
  );

const meta = { title: "Layout parts", parameters: { layout: "fullscreen" }, decorators: [column] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

const reactions = ["🍞", "🌊", "🌲", "☕", "📷", "🎉", "🙏", "🔥", "🌙", "🍵", "🚲", "🎈", "🧭", "🪴"];
/** A second name as long, of who boosted. */
const boosterName = "Wilhelmina-Alexandrina-Marjoribanks-Featherstonehaugh-Cholmondeley";

/** More chips than a row holds: they wrap onto further rows. */
export const ChipRowWrapping: Story = {
  render: () => (
    <ChipRow label="Reactions">
      {reactions.map((glyph, i) => (
        <Chip key={glyph} label={`${glyph} ${String(i + 1)}`} glyph={glyph} count={i + 1} pressed={i === 2} />
      ))}
    </ChipRow>
  ),
};

/**
 * An entry's parts one under another, and its row of actions with More at its end; `counts` gives three of the actions
 * counts in the hundreds of thousands.
 */
export const EntryParts: StoryObj<Meta<{ counts: "small" | "large" }>> = {
  args: { counts: "small" },
  argTypes: {
    counts: { control: "select", options: ["small", "large"], description: "How many replied, boosted and favourited" },
  },
  render: ({ counts }) => (
    <Entry labelledBy="layout-entry-name">
      <CaptionLine icon="boost" oneLine>
        <CaptionName>{boosterName}</CaptionName> boosted
      </CaptionLine>
      <Byline
        avatar={<Avatar name={longName} src={images.dusk} size="post" decorative />}
        name={longName}
        nameId="layout-entry-name"
        handle={longHandle}
        href="/@bartholomew"
        end={<Stamp glyph={{ icon: "public", label: "Public" }} date={ago(3_600_000)} />}
      />
      <Stack gap="entry">
        <p className="m-0">First part: {sentence}</p>
        <p className="m-0">Second part: {sentence}</p>
        <Stack gap="entry-line">
          <p className="m-0">First line of the third part.</p>
          <p className="m-0">Second line of the third part.</p>
        </Stack>
      </Stack>
      <ActionRow end={<IconButton icon="more" label="More" />}>
        <IconButton icon="reply" label="Reply" count={counts === "large" ? 123_456 : 12} />
        <IconButton icon="boost" label="Boost" count={counts === "large" ? 234_567 : 3} />
        <IconButton icon="favourite" label="Favourite" count={counts === "large" ? 345_678 : 45} />
        <IconButton icon="bookmark" label="Bookmark" />
      </ActionRow>
    </Entry>
  ),
};

/** Replies one to four and nine deep under a post: a reply indents by its depth, up to three steps, where its column has the room. */
export const ThreadReplies: Story = {
  render: () => (
    <div>
      <p className="m-0">The post the replies are under.</p>
      {[1, 2, 3, 4, 9].map((depth) => (
        <ThreadReply key={depth} depth={depth}>
          <p className="m-0">A reply at depth {depth}.</p>
        </ThreadReply>
      ))}
    </div>
  ),
};

/** A screen's head over its rows, of accounts with long names and of what opens elsewhere, and a pair of buttons. */
export const ScreenRows: Story = {
  render: () => (
    <ScreenStack>
      <ScreenHead back={<BackButton onPress={nothing}>Held-back notifications</BackButton>}>
        <AccountRow
          heading
          avatar={<Avatar name={longName} src={images.dusk} size="post" decorative />}
          name={longName}
          handle={longHandle}
          href="/@bartholomew"
          end={
            <ButtonPair>
              <Button size="timeline">Dismiss</Button>
              <Button size="timeline" kind="primary">
                Accept
              </Button>
            </ButtonPair>
          }
        />
      </ScreenHead>
      <RowList label="People">
        <RowItem>
          <OpenRow
            href="/follow-requests"
            icon="followers"
            title="Follow requests"
            hint={`Three people wait for an answer. ${sentence}`}
          />
        </RowItem>
        <RowItem>
          <AccountRow
            avatar={<Avatar name="Layla" size="post" decorative />}
            name={`Layla ${longWord}`}
            handle={longHandle}
            href="/@layla"
            note={sentence}
            end={<Button size="timeline">Follow</Button>}
          />
        </RowItem>
        <RowItem>
          <AccountRow
            avatar={<Avatar name="Sam" size="post" decorative />}
            name="Sam"
            handle="@sam@my.example"
            href="/@sam"
            end={<Button size="timeline">Follow</Button>}
          />
        </RowItem>
      </RowList>
      <ButtonPair>
        <Button>Not now</Button>
        <Button kind="primary">Accept every request</Button>
      </ButtonPair>
    </ScreenStack>
  ),
};

/** The head of a screen of one thing whose name runs long: it breaks before its capitals, and its action ends its row. */
export const LongScreenTitle: Story = {
  render: () => (
    <ScreenTitle
      back={<BackButton onPress={nothing}>Back</BackButton>}
      glyph="hashtag"
      count="1,234 posts"
      actions={<Button size="timeline">Follow</Button>}
    >
      #TheLongestHashtagSomebodyEverWroteAboutSlowMorningsByTheSea2026
    </ScreenTitle>
  ),
};

/* oxlint-disable restrict/token-colors -- a sample picture is drawn in its own colours */
const banner = `data:image/svg+xml,${encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 160 90" width="640" height="360"><rect width="160" height="90" fill="#7fb3d5"/><path d="M0 60 L40 42 L80 56 L120 38 L160 52 V90 H0Z" fill="#3f6f86"/></svg>',
)}`;
const wordmark = `data:image/svg+xml,${encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 32" width="120" height="32"><text x="0" y="24" font-family="sans-serif" font-size="24" fill="#767676">mycroblog</text></svg>',
)}`;
/* oxlint-enable restrict/token-colors */

type FormPageArgs = {
  /** The page belongs to a server, which it introduces. */
  server: boolean;
  /** The server has a banner. */
  banner: boolean;
  /** The server's front page. */
  front: boolean;
  /** What is longer than a line of a phone: nothing, the app's name (one word), or the first action's label. */
  long: "none" | "word" | "label";
};

function FormPageSpecimen({ server, banner: withBanner, front, long }: FormPageArgs) {
  const app = long === "word" ? longWord : "Tide Tables";
  return (
    <FormPage
      server={
        server
          ? {
              name: "A server by the sea",
              banner: withBanner ? banner : undefined,
              description: `A small server for people who like the sea, photography and slow mornings. ${sentence}`,
            }
          : undefined
      }
      wordmark={{ light: wordmark, dark: wordmark }}
      front={front ? { more: <FormLink href="/about">About this server</FormLink> } : undefined}
      footer={
        <FormPageFooter
          label="This server"
          links={[
            { href: "/about", label: "About" },
            { href: "/terms", label: "Terms of service" },
            { href: "/privacy", label: "Privacy policy" },
            { href: "https://status.my.example", label: "Service status" },
          ]}
        />
      }
    >
      <FormHead title={`Allow ${app} to use your account?`}>
        <FormText>{sentence}</FormText>
      </FormHead>
      <Form action="/layout-parts">
        <TextField id="layout-email" name="email" label="Email address" type="email" hint={sentence} />
        <TextField id="layout-about" name="about" label="About you" multiline />
        <CheckField id="layout-agree" name="agree" value="yes" label={`I agree to the rules. ${sentence}`} />
        <FormActions>
          <Button kind="primary" type="submit">
            {long === "label" ? "Request an export of your data and of everything you wrote here" : "Request an export"}
          </Button>
          <Button>Download your follows as CSV</Button>
        </FormActions>
      </Form>
      <FormSection id="layout-permissions" heading="This app will be able to">
        <FormList
          items={["Read your posts", `Write posts as you. ${sentence}`, app, "Read who you follow"].map((item) => (
            <span key={item}>{item}</span>
          ))}
        />
      </FormSection>
      <FormChoice>
        <Button>Deny</Button>
        <Button kind="primary">Allow</Button>
      </FormChoice>
      <FormLinks>
        <FormLink href="/login">Sign in with another account</FormLink>
        <FormLink href="/forgot">Forgot your password?</FormLink>
        <FormLink href="/help">{`Help with ${app}`}</FormLink>
      </FormLinks>
    </FormPage>
  );
}

/**
 * A page of its own, a screen longer than a phone: of a server with a banner by default; `banner`, `server` and
 * `front` (the Controls) take the banner away, leave the column alone, or make it the server's front page, and `long`
 * names the app by a word no line holds, or gives the first action a label longer than a phone's line.
 */
export const FormPageScreen: StoryObj<Meta<FormPageArgs>> = {
  args: { server: true, banner: true, front: false, long: "none" },
  parameters: { page: true },
  argTypes: {
    long: {
      control: "select",
      options: ["none", "word", "label"],
      description: "What is longer than a line of a phone: the app's name (one word), or the first action's label",
    },
    server: { control: "boolean", description: "The page belongs to a server, which it introduces" },
    banner: { control: "boolean", description: "The server has a banner" },
    front: { control: "boolean", description: "The server's front page" },
  },
  render: (args) => <FormPageSpecimen {...args} />,
};

/** The sections of the drafts' list, their rows holding long text. */
export const Drafts: Story = {
  render: () => (
    <DraftSections>
      <DraftSection
        title={
          <>
            Scheduled <DraftCount count={2} />
          </>
        }
        note="Times are in Asia/Tokyo (GMT+9)"
      >
        <DraftList label="Scheduled">
          {["first", "second"].map((which) => (
            <DraftRow
              key={which}
              lead={{ kind: "scheduled", text: `The ${which} goes out on Monday, November 16 at 9:00 AM` }}
              text={`The ${which} scheduled post. ${sentence} ${longWord}`}
              blank="No text"
              warning={which === "first" ? `Spoilers: ${longWord}` : undefined}
              marks={[
                { icon: "attach", label: "2 images", count: 2 },
                { icon: "poll", label: "A poll" },
              ]}
              saved="Saved 2h ago"
              openLabel={`Open the ${which} scheduled post`}
              onOpen={nothing}
              menu={
                <MenuButton icon="more" label={`More for the ${which} scheduled post`}>
                  <MenuItem id="delete" icon="delete">
                    Delete…
                  </MenuItem>
                </MenuButton>
              }
            />
          ))}
        </DraftList>
      </DraftSection>
      <DraftSection
        title={
          <>
            Drafts <DraftCount count={1} />
          </>
        }
      >
        <DraftList label="Drafts">
          <DraftRow
            text=""
            blank="No text"
            marks={[]}
            saved="Saved 3d ago"
            openLabel="Open the draft"
            onOpen={nothing}
            menu={
              <MenuButton icon="more" label="More for the draft">
                <MenuItem id="delete" icon="delete">
                  Delete…
                </MenuItem>
              </MenuButton>
            }
          />
        </DraftList>
      </DraftSection>
    </DraftSections>
  ),
};

/** The versions of a post, newest first, each holding long text. */
export const Versions: Story = {
  render: () => (
    <VersionList label="Versions">
      {["Current", "Edit", "Original"].map((label, i) => (
        <Version key={label} label={label} time={<time dateTime="2026-11-16T09:00">November 16, 2026, 9:0{i} AM</time>}>
          <ContentBody lang="en">
            <p>
              {label}: {sentence} {longWord}
            </p>
          </ContentBody>
        </Version>
      ))}
    </VersionList>
  ),
};

function ComposerSpecimen() {
  const [descriptions, setDescriptions] = useState(["", `A grazed knee. ${sentence} ${longWord}`]);
  const [duration, setDuration] = useState<"day" | "week">("day");
  const [multiple, setMultiple] = useState(false);
  const durations = [
    { id: "day", icon: "schedule", label: "1 day" },
    { id: "week", icon: "schedule", label: "7 days, which is as long as a poll of this server may run" },
  ] as const;
  return (
    <div>
      <AttachmentList>
        {descriptions.map((description, i) => (
          <AttachmentRow
            key={i}
            src={i === 0 ? images.dusk : images.snow}
            state={i === 0 ? "failed" : "ready"}
            status={
              i === 0 ? "The upload failed: the file is larger than the server takes, which is 16 MB for an image." : ""
            }
            retry={i === 0 ? { label: "Try again", onPress: nothing } : undefined}
            description={description}
            onDescription={(value) => {
              setDescriptions(descriptions.map((d, at) => (at === i ? value : d)));
            }}
            descriptionLabel={`Description of image ${String(i + 1)}`}
            placeholder="Describe it for people who cannot see it"
            missing={i === 0 ? { text: "Add a description so that everyone can read it", required: false } : undefined}
            removeLabel={`Remove image ${String(i + 1)}`}
            onRemove={nothing}
          />
        ))}
      </AttachmentList>
      <PollSettings>
        <ChoiceSelect
          label="Duration"
          icon="schedule"
          value={durations.find((d) => d.id === duration)?.label ?? ""}
          items={durations}
          chosen={duration}
          onChoose={setDuration}
        />
        <CheckField
          id="layout-poll-multiple"
          name="multiple"
          value="yes"
          label="Allow several choices, as many as the voter likes"
          checked={multiple}
          onChange={setMultiple}
        />
      </PollSettings>
    </div>
  );
}

/** The composer's rows of images, with long descriptions and a failure, and a poll's settings. */
export const ComposerParts: Story = { render: () => <ComposerSpecimen /> };

/** The panes of the article editor: the ones shown at every width, from `desktop` alone, under it alone, and never. */
export const ArticlePanes: Story = {
  render: () => (
    <div>
      {(["always", "desktop", "narrow", "never"] as const).map((shown) => (
        <ArticlePane key={shown} shown={shown}>
          <p className="m-0">A pane shown {shown}.</p>
        </ArticlePane>
      ))}
    </div>
  ),
};

type FeedArgs = { newItems: boolean; tight: boolean };

const feedEntries = Array.from({ length: 24 }, (_, i) => `feed-entry-${String(i + 1)}`);

/** A feed longer than the view, with the new entries' button over its top (`newItems`). */
export const LongFeed: StoryObj<Meta<FeedArgs>> = {
  args: { newItems: true, tight: false },
  argTypes: {
    newItems: { control: "boolean", description: "New entries wait: the button floats over the feed's top" },
    tight: { control: "boolean", description: "Its entries closer together, as one exchange" },
  },
  render: ({ newItems, tight }) => (
    <Feed
      label="Home"
      count={feedEntries.length}
      more={false}
      busy={false}
      tight={tight}
      newItems={newItems ? { label: "12 new posts", onPress: nothing } : undefined}
      end={<FeedNotice action={<TextButton>Try again</TextButton>}>No more posts could be loaded</FeedNotice>}
    >
      {feedEntries.map((id, i) => (
        <FeedItem key={id} id={id} position={i + 1}>
          <Entry labelledBy={id}>
            <Byline
              avatar={<Avatar name="Sam" size="post" decorative />}
              name={`Entry ${String(i + 1)}`}
              nameId={id}
              handle="@sam@my.example"
            />
            <p className="m-0">{sentence}</p>
          </Entry>
        </FeedItem>
      ))}
    </Feed>
  ),
};

/** A text people wrote: words and addresses with no place to break, a mention, a shortened URL, a quote and lists. */
export const WrittenText: Story = {
  render: () => (
    <ContentBody lang="en">
      <p>
        A word no line holds: {longWord}
        {longWord}
      </p>
      <p>
        {sentence} Then a mention at the end of the line:{" "}
        <ContentLink href="/@bartholomew" kind="mention" translate="no" inApp>
          @bartholomew_maximilian
        </ContentLink>{" "}
        and a hashtag{" "}
        <ContentLink href="/tags/slowmornings" kind="hashtag" inApp>
          #SlowMorningsByTheSea
        </ContentLink>
      </p>
      <p>
        A shortened address:{" "}
        <ContentLink href="https://example.com/a/very/long/path/to/a/page/that/goes/on?and=on" kind="link">
          <UrlPart part="hidden">https://</UrlPart>
          <UrlPart part="cut">example.com/a/very/long/path/to</UrlPart>
          <UrlPart part="hidden">/a/page/that/goes/on?and=on</UrlPart>
        </ContentLink>
      </p>
      <blockquote>
        <p>A quoted line. {sentence}</p>
      </blockquote>
      <ul>
        <li>A first item of a list. {sentence}</li>
        <li>A second item.</li>
      </ul>
      <ol>
        <li>A first step. {sentence}</li>
        <li>A second step.</li>
      </ol>
      <pre>
        <code>const aLineOfCodeThatIsLongerThanAnyPhoneIsWide = "and a string after it";</code>
      </pre>
    </ContentBody>
  ),
};

/** A folded part that holds a link, between two buttons: what is folded is not shown and takes no focus. */
export const FoldedDisclosure: Story = {
  render: () => (
    <div style={{ display: "grid", gap: "1rem", justifyItems: "start" }}>
      <Button>Before the fold</Button>
      <Disclosure summary="Spoilers for the season finale" icon="hidden">
        <p className="m-0">
          The ending, with <TextLink href="/finale">a link in the folded part</TextLink>.
        </p>
      </Disclosure>
      <Button>After the fold</Button>
    </div>
  ),
};

function Popups({ long, children }: { long: boolean; children?: ReactNode }) {
  const [form, setForm] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const [panel, setPanel] = useState(false);
  const items = (
    <MenuGroup>
      <MenuItem id="link" icon="link">
        Copy link
      </MenuItem>
      <MenuItem id="share" icon="share">
        Share
      </MenuItem>
      <MenuItem id="long" icon="bookmark" about={sentence}>
        {long ? `Bookmark ${longWord}` : "Bookmark"}
      </MenuItem>
    </MenuGroup>
  );
  return (
    <div style={{ display: "grid", gap: "2rem" }}>
      <div style={{ display: "flex", justifyContent: "space-between" }}>
        <MenuButton icon="boost" label="Menu at the start" align="start">
          {items}
        </MenuButton>
        <MenuButton icon="more" label="Menu at the end" align="end">
          {items}
        </MenuButton>
      </div>
      <div style={{ display: "flex", gap: "1rem", flexWrap: "wrap" }}>
        <FormPopup open={form} onOpenChange={setForm} label="Schedule" trigger={<Button>Open the form</Button>}>
          <div style={{ padding: "1rem", display: "grid", gap: "1rem" }}>
            <TextField id="layout-popup-when" name="when" label="Date and time" />
            <Button
              kind="primary"
              onPress={() => {
                setForm(false);
              }}
            >
              Set time
            </Button>
          </div>
        </FormPopup>
        <Button
          onPress={() => {
            setConfirm(true);
          }}
        >
          Open the question
        </Button>
        <Button
          onPress={() => {
            setPanel(true);
          }}
        >
          Open the panel
        </Button>
      </div>
      {children}
      {confirm && (
        <ConfirmDialog
          title={long ? `Delete ${longWord}?` : "Delete this list?"}
          cancel="Cancel"
          confirm="Delete"
          onCancel={() => {
            setConfirm(false);
          }}
          onConfirm={() => {
            setConfirm(false);
          }}
        >
          {sentence}
        </ConfirmDialog>
      )}
      {panel && (
        <PanelDialog
          title="Edit history"
          closeLabel="Close"
          onClose={() => {
            setPanel(false);
          }}
        >
          {Array.from({ length: 30 }, (_, i) => (
            <p key={i}>
              Line {i + 1} of the panel. {sentence}
            </p>
          ))}
        </PanelDialog>
      )}
    </div>
  );
}

/**
 * What opens over a page, each from its button: a menu from a button at the row's start and one at its end, a small
 * form, a question and a panel to read; over a page longer than the view. `long` names what the menu and the question
 * are about by a word no line holds.
 */
export const Overlays: StoryObj<Meta<{ long: boolean }>> = {
  args: { long: false },
  argTypes: { long: { control: "boolean", description: "A word no line holds in the menu and in the question" } },
  render: ({ long }) => (
    <Popups long={long}>
      {Array.from({ length: 12 }, (_, i) => (
        <p key={i} className="m-0">
          Line {i + 1} of the page under the overlays. {sentence}
        </p>
      ))}
    </Popups>
  ),
};
