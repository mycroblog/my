// SPDX-License-Identifier: AGPL-3.0-only

// The rough of the post detail (#3047), at /app/post: the focused post in its thread (FR-106), as the API gives it
// (StatusContext: the ancestors oldest first, then the replies depth first in posting order). The thread's first post,
// then a fold of the ancestors between it and the last two, then those two; the focused post, larger, with its date,
// its visibility and its counts inside it, under its body and before its actions (the post's `details`), in place of
// the stamp at the end of its author's line; then its replies, each indented by how deep it replies (up to three steps
// from the tablet, none on a phone, where the reply's own context line says to whom), a branch the server cut short
// ending in a fold, and the replies beyond what it gave in a fold at the end. `?state=lone` shows a post with no
// thread and no replies. All of it is one feed, so that the keys of the next and the previous entry run through it.

import { Feed, FeedItem, FeedNotice, Icon, TextButton } from "@my/design-system";
import type { CSSProperties, ReactNode } from "react";
import { SamplePost } from "../timeline/SamplePost";
import type { TimelinePost } from "../timeline/fixtures";
import { lone, thread } from "./samples";
import { catalogHref, Moves, ScreenPreview, stateOf, useLang } from "./ScreenPreview";
import { GroupHeading } from "./parts";
import type { Strings } from "./strings";

const states = ["thread", "lone"] as const;

function Fold({ label }: { label: string }) {
  return (
    <p className="sc-fold">
      <TextButton>{label}</TextButton>
    </p>
  );
}

/** The focused post's own lines: when it was posted, to whom, and how many boosted, quoted and favourited it. */
function Details({ post, s, lang }: { post: TimelinePost; s: Strings; lang: "en" | "ja" }) {
  const when = new Date(Date.now() - post.ago);
  const date = new Intl.DateTimeFormat(lang, { dateStyle: "medium", timeStyle: "short" }).format(when);
  const n = (v: number) => new Intl.NumberFormat(lang).format(v);
  const counts = [
    [s.boosts, post.counts.boosts],
    [s.quotes, post.id === thread.focused.id ? thread.quotes : 0],
    [s.favourites, post.counts.favourites],
  ] as const;
  const shown = counts.filter(([, v]) => v > 0);
  const visibility = post.visibility;
  return (
    <div className="sc-details">
      <p>
        <time dateTime={when.toISOString()}>{date}</time>
        <span aria-hidden="true"> · </span>
        <span className="sc-glyph-text">
          <Icon name={visibility} place="caption" />
          {s.visibility[visibility]}
        </span>
      </p>
      {shown.length > 0 && (
        <p className="sc-counts">
          {shown.map(([label, v], i) => (
            <span key={i}>
              {i > 0 && <span aria-hidden="true"> · </span>}
              <a href="#who">{label(n(v))}</a>
            </span>
          ))}
        </p>
      )}
    </div>
  );
}

function Thread({ lang, s }: { lang: "en" | "ja"; s: Strings }) {
  const ancestors = [thread.root, ...thread.parents];
  let position = 0;
  const item = (post: TimelinePost, details?: ReactNode) => {
    position += 1;
    return (
      <FeedItem id={post.id} position={position}>
        <SamplePost post={post} lang={lang} details={details} stamp={details === undefined} />
      </FeedItem>
    );
  };
  return (
    <Feed label={s.post} count={ancestors.length + 1 + thread.replies.length} more busy={false}>
      <Moves key="moves" />
      <div key="root">{item(thread.root)}</div>
      <Fold key="fold" label={s.earlier(thread.folded)} />
      {thread.parents.map((p) => (
        <div key={p.id}>{item(p)}</div>
      ))}
      <div key="focus" className="sc-focus">
        {item(thread.focused, <Details post={thread.focused} s={s} lang={lang} />)}
      </div>
      <GroupHeading key="replies">{s.replies}</GroupHeading>
      {thread.replies.map((r) => (
        <div key={r.post.id} className="sc-reply" style={{ "--sc-depth": r.depth - 1 } as CSSProperties}>
          {item(r.post)}
          {r.more !== undefined && (
            <div className="sc-reply-more" style={{ "--sc-depth": r.depth } as CSSProperties}>
              <Fold label={s.moreReplies(r.more)} />
            </div>
          )}
        </div>
      ))}
      <Fold key="later" label={s.moreReplies(thread.later)} />
    </Feed>
  );
}

function Lone({ lang, s }: { lang: "en" | "ja"; s: Strings }) {
  return (
    <Feed label={s.post} count={1} more={false} busy={false} end={<FeedNotice>{s.noReplies}</FeedNotice>}>
      <Moves />
      <div className="sc-focus">
        <FeedItem id={lone.id} position={1}>
          <SamplePost post={lone} lang={lang} details={<Details post={lone} s={s} lang={lang} />} stamp={false} />
        </FeedItem>
      </div>
    </Feed>
  );
}

export function PostDetail() {
  const { lang, s } = useLang();
  const state = stateOf(states);
  return (
    <ScreenPreview
      place="/post"
      title={s.post}
      states={[
        { id: "thread", name: "Thread" },
        { id: "lone", name: "Alone, no replies" },
      ]}
      current={state}
    >
      <p className="sc-back">
        <a href={catalogHref("/")}>
          <Icon name="back" place="caption" />
          {s.back}
        </a>
      </p>
      {state === "thread" ? <Thread lang={lang} s={s} /> : <Lone lang={lang} s={s} />}
    </ScreenPreview>
  );
}
