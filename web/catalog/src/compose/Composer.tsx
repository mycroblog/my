// SPDX-License-Identifier: AGPL-3.0-only

// The rough post composer (#3046), on the catalog only: one composer for a new post, a reply and a quote, laid out by
// the viewport in compose.css — a screen of its own on a phone, its row of tools held just above the software
// keyboard, and a dialog over the page from `tablet` (40rem). From the top: the head (the title, then close at its
// end), the post replied to or the post quoted (both above the text, as context), the content warning, the text with
// the over part marked, the images with their descriptions or the poll, the form's error, and the row that sends it (its tools with the audience
// and the language, the count, Post). It keeps its own state and fakes the server: Post is busy for a moment, then
// fails when the draft says so, else empties the composer. Close and Escape discard a written post (the page offers
// to undo it); its owner saves it as a draft when the composer is left by accident (a press on the backdrop).

import {
  Avatar,
  Button,
  Byline,
  CaptionLine,
  CheckField,
  Embed,
  FieldError,
  FormAlert,
  Icon,
  IconButton,
  MediaGrid,
  useLocale,
  type IconName,
} from "@my/design-system";
import { useEffect, useId, useState, type ReactNode } from "react";
import { posts, type QuotedPost, type TimelinePost } from "../timeline/fixtures";
import { stringsFor, type ComposeStrings, type Interaction, type Policy, type Visibility } from "./strings";
import "./compose.css";

export type Language = "en" | "ja" | "de" | "es" | "fr";

export type Image = { src: string; alt: string; uploading?: boolean };

export type Draft = {
  kind: "new" | "reply" | "quote";
  text: string;
  /** The content warning; none while it is off. */
  cw?: string | undefined;
  visibility: Visibility;
  /** Who may reply, boost and quote (ADR-0155). */
  policy: Record<Interaction, Policy>;
  language: Language;
  images: Image[];
  poll?: { options: string[]; duration: number; multiple: boolean; hideTotals: boolean } | undefined;
  replyTo?: TimelinePost | undefined;
  /** The quoted post, and whether it is on another server. */
  quote?: { post: QuotedPost; remote: boolean } | undefined;
  phase: "editing" | "submitting" | "failed";
  /** The server asks for a description of every image (FR-105's option). */
  descriptionsRequired?: boolean | undefined;
  /** Post was pressed: a description the server asks for and is missing shows as the field's error. */
  checked?: boolean | undefined;
  /** A popover open at first, for a specimen. */
  pop?: "audience" | "permissions" | "language" | undefined;
  /** The fake server refuses the next post. */
  failNext?: boolean | undefined;
};

/** How the composer was left: a post sent, an empty one closed, or a written one discarded. */
export type Leave = "posted" | "empty" | "discarded";

/** The most characters of the text and the content warning together (FR-101: the server's setting, 500 by default). */
export const maxChars = 500;
/** How many left before the count stops being muted. */
const nearChars = 20;

const visibilityIcon: Record<Visibility, IconName> = {
  public: "public",
  local: "local",
  unlisted: "unlisted",
  private: "followers",
  direct: "direct",
};
const policyIcon: Record<Policy, IconName> = {
  everyone: "public",
  followers: "followers",
  mentioned: "direct",
  nobody: "profile",
};
const visibilities: Visibility[] = ["public", "local", "unlisted", "private", "direct"];
const policies: Policy[] = ["everyone", "followers", "mentioned", "nobody"];
const interactions: Interaction[] = ["reply", "boost", "quote"];
const languages: Language[] = ["en", "ja", "de", "es", "fr"];
// A post that is local, followers-only or direct cannot quote a post from another server (ADR-0069 §4).
const closed: Visibility[] = ["local", "private", "direct"];

const chars = (s: string) => Array.from(s);

// What Add images attaches: the timeline's scenes, in turn.
const scenes = posts.flatMap((post) => post.media ?? []).map((m) => m.src);
const sampleImage = (n: number) => scenes[n % scenes.length] ?? "";

/** The empty draft of a new post. */
export const emptyDraft = (language: Language): Draft => ({
  kind: "new",
  text: "",
  visibility: "public",
  policy: { reply: "everyone", boost: "everyone", quote: "everyone" },
  language,
  images: [],
  phase: "editing",
});

/** Whether a draft holds anything worth keeping. */
export const written = (d: Draft) =>
  d.text.trim() !== "" || d.images.length > 0 || d.poll !== undefined || (d.cw ?? "") !== "";

/**
 * A popover of its trigger, closed by a press outside it or Escape: under the trigger from `tablet`, where the page
 * below scrolls into view, and over the row on a phone, where the keyboard is below; capped in height, it scrolls.
 */
function Pop({
  open,
  onClose,
  trigger,
  label,
  children,
}: {
  open: boolean;
  onClose: () => void;
  trigger: ReactNode;
  label: string;
  children: ReactNode;
}) {
  useEffect(() => {
    if (!open) return;
    const away = (e: Event) => {
      if (!(e.target instanceof Element) || !e.target.closest("[data-cmp-pop]")) onClose();
    };
    const escape = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        onClose();
      }
    };
    document.addEventListener("pointerdown", away);
    document.addEventListener("keydown", escape, true);
    return () => {
      document.removeEventListener("pointerdown", away);
      document.removeEventListener("keydown", escape, true);
    };
  }, [open, onClose]);
  return (
    <span className="cmp-pop-anchor" data-cmp-pop="">
      {trigger}
      {open && (
        <div className="cmp-pop" role="dialog" aria-label={label}>
          {children}
        </div>
      )}
    </span>
  );
}

/** A group of choices in a popover: each a glyph, a label and maybe a line about it; the chosen one marked. */
function Choices<T extends string>({
  title,
  items,
  chosen,
  onChoose,
  note,
}: {
  title: string;
  items: { id: T; icon: IconName; label: string; about?: string | undefined; disabled?: boolean }[];
  chosen: T;
  onChoose: (id: T) => void;
  note?: string | undefined;
}) {
  return (
    <div className="cmp-choices" role="radiogroup" aria-label={title}>
      <p className="cmp-choices-title">{title}</p>
      {items.map((item) => (
        <button
          key={item.id}
          type="button"
          role="radio"
          aria-checked={item.id === chosen}
          disabled={item.disabled}
          className="cmp-choice"
          onClick={() => {
            onChoose(item.id);
          }}
        >
          <span className="cmp-choice-glyph">
            <Icon name={item.icon} place="action" />
          </span>
          <span className="cmp-choice-text">
            <span className="cmp-choice-label">{item.label}</span>
            {item.about !== undefined && <span className="cmp-choice-about">{item.about}</span>}
          </span>
        </button>
      ))}
      {note !== undefined && <p className="cmp-choices-note">{note}</p>}
    </div>
  );
}

/** An item of a popover that opens another panel of it (a submenu): a glyph, a label, what it holds now, and ›. */
function Opens({ icon, label, now, onPress }: { icon: IconName; label: string; now: string; onPress: () => void }) {
  return (
    <button type="button" className="cmp-choice cmp-opens" onClick={onPress}>
      <span className="cmp-choice-glyph">
        <Icon name={icon} place="action" />
      </span>
      <span className="cmp-choice-text">
        <span className="cmp-choice-label">{label}</span>
        <span className="cmp-choice-about">{now}</span>
      </span>
      <span className="cmp-opens-mark" aria-hidden="true">
        ›
      </span>
    </button>
  );
}

/** The text field: a plain area on the composer's surface, the part over the limit marked behind the text. */
function TextArea({
  id,
  value,
  over,
  placeholder,
  label,
  error,
  readOnly,
  onChange,
}: {
  id: string;
  value: string;
  /** How many characters at its end are over the limit. */
  over: number;
  placeholder: string;
  label: string;
  error?: string | undefined;
  readOnly: boolean;
  onChange: (value: string) => void;
}) {
  const all = chars(value);
  const keep = Math.max(0, all.length - over);
  return (
    <div className="cmp-text-field">
      <div className="cmp-text-wrap">
        <div className="cmp-text-mirror" aria-hidden="true">
          {all.slice(0, keep).join("")}
          {over > 0 && <mark>{all.slice(keep).join("")}</mark>}
          {"​"}
        </div>
        <textarea
          id={id}
          className="cmp-text"
          value={value}
          placeholder={placeholder}
          aria-label={label}
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? `${id}-error` : undefined}
          readOnly={readOnly}
          dir="auto"
          onChange={(e) => {
            onChange(e.target.value);
          }}
        />
      </div>
      {error && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </div>
  );
}

/** The post replied to, above the text: its author on one line and the start of its text, muted. */
function ReplyContext({ post, s }: { post: TimelinePost; s: ComposeStrings }) {
  return (
    <div className="cmp-reply" lang={post.lang}>
      <Byline
        compact
        avatar={<Avatar name={post.author.name} src={post.author.avatar} size="inline" decorative />}
        name={post.author.name}
        handle={post.author.handle}
      />
      <p className="cmp-reply-text" dir="auto">
        {post.text}
      </p>
      <span className="cmp-sr">{s.replyingTo(post.author.handle)}</span>
    </div>
  );
}

/** The quoted post, under the text, as the post will carry it: a reduced preview on the raised surface. */
function QuoteContext({ post, s, onRemove }: { post: QuotedPost; s: ComposeStrings; onRemove: () => void }) {
  return (
    <div className="cmp-quote">
      <Embed lang={post.lang}>
        <Byline
          compact
          avatar={<Avatar name={post.author.name} src={post.author.avatar} size="inline" decorative />}
          name={post.author.name}
          handle={post.author.handle}
        />
        <p className="cmp-quote-text" dir="auto">
          {post.text}
        </p>
        {post.media && (
          <MediaGrid items={post.media.map((m, i) => ({ id: `q-${i}`, src: m.src, alt: m.alt }))} size="preview" />
        )}
      </Embed>
      <span className="cmp-quote-remove">
        <IconButton icon="close" label={s.removeQuote} onPress={onRemove} />
      </span>
    </div>
  );
}

/** The images: each a thumbnail beside its description, which is asked for where it is missing. */
function Images({
  images,
  required,
  s,
  onChange,
  onRemove,
}: {
  images: Image[];
  required: boolean;
  s: ComposeStrings;
  onChange: (i: number, alt: string) => void;
  onRemove: (i: number) => void;
}) {
  const id = useId();
  return (
    <ul className="cmp-images">
      {images.map((image, i) => {
        const missing = !image.uploading && image.alt.trim() === "";
        const field = `${id}-${i}`;
        return (
          <li key={i} className="cmp-image">
            <img className="cmp-thumb" src={image.src} alt="" data-uploading={image.uploading || undefined} />
            <div className="cmp-image-field">
              {image.uploading ? (
                <p className="cmp-muted">{s.uploading}</p>
              ) : (
                <>
                  <label className="cmp-label" htmlFor={field}>
                    {s.description}
                  </label>
                  <textarea
                    id={field}
                    className="cmp-field cmp-description"
                    rows={2}
                    value={image.alt}
                    placeholder={s.descriptionHint}
                    aria-invalid={missing && required ? true : undefined}
                    aria-describedby={missing ? `${field}-note` : undefined}
                    dir="auto"
                    onChange={(e) => {
                      onChange(i, e.target.value);
                    }}
                  />
                  {missing &&
                    (required ? (
                      <FieldError id={`${field}-note`}>{s.descriptionRequired}</FieldError>
                    ) : (
                      <span id={`${field}-note`}>
                        <CaptionLine icon="caution">{s.noDescription}</CaptionLine>
                      </span>
                    ))}
                </>
              )}
            </div>
            <span className="cmp-image-remove">
              <IconButton
                icon="close"
                label={s.remove}
                onPress={() => {
                  onRemove(i);
                }}
              />
            </span>
          </li>
        );
      })}
    </ul>
  );
}

/**
 * The poll, a raised card under its caption and its remove: its options on the page's surface, each headed by the
 * mark it will be answered with in the post (a radio's ring, or a checkbox's square when several may be chosen), one
 * more while under four and each past two removable; then how long it runs, a control that reads as one (a field's
 * surface, its glyph and a chevron), and how it counts, two checkboxes. Nothing in it looks pressable that is not.
 */
function Poll({
  poll,
  s,
  onChange,
  onRemove,
}: {
  poll: NonNullable<Draft["poll"]>;
  s: ComposeStrings;
  onChange: (poll: NonNullable<Draft["poll"]>) => void;
  onRemove: () => void;
}) {
  const id = useId();
  const [choosing, setChoosing] = useState(false);
  const options = (
    <div className="cmp-poll-options">
      {poll.options.map((option, i) => (
        <div key={i} className="cmp-poll-option">
          {/* How the option will be answered in the post: a radio's ring, or a checkbox's square when several may be
              chosen. Decorative: nothing to press here. */}
          <span className="cmp-poll-mark" data-multiple={poll.multiple ? "" : undefined} aria-hidden="true" />
          <input
            className="cmp-poll-input"
            value={option}
            placeholder={s.option(i + 1)}
            aria-label={s.option(i + 1)}
            dir="auto"
            onChange={(e) => {
              onChange({ ...poll, options: poll.options.map((o, j) => (j === i ? e.target.value : o)) });
            }}
          />
          {poll.options.length > 2 && (
            <IconButton
              icon="close"
              label={`${s.remove}: ${s.option(i + 1)}`}
              onPress={() => {
                onChange({ ...poll, options: poll.options.filter((_, j) => j !== i) });
              }}
            />
          )}
        </div>
      ))}
      {poll.options.length < 4 && (
        <span className="cmp-poll-add">
          <Button
            kind="quiet"
            size="timeline"
            onPress={() => {
              onChange({ ...poll, options: [...poll.options, ""] });
            }}
          >
            <span className="cmp-glyph-label">
              <Icon name="add" place="action" />
              {s.addOption}
            </span>
          </Button>
        </span>
      )}
    </div>
  );
  const settings = (
    <div className="cmp-poll-settings">
      <Pop
        open={choosing}
        onClose={() => {
          setChoosing(false);
        }}
        label={s.duration}
        trigger={
          <button
            type="button"
            className="cmp-select-button"
            aria-haspopup="dialog"
            aria-expanded={choosing}
            onClick={() => {
              setChoosing((c) => !c);
            }}
          >
            <Icon name="time" place="caption" />
            <span>{s.endsIn(s.durations[poll.duration] ?? "")}</span>
            <Icon name="expand" place="caption" />
          </button>
        }
      >
        <Choices
          title={s.duration}
          items={s.durations.map((d, i) => ({ id: String(i), icon: "time" as const, label: d }))}
          chosen={String(poll.duration)}
          onChoose={(i) => {
            onChange({ ...poll, duration: Number(i) });
            setChoosing(false);
          }}
        />
      </Pop>
      {/* The checkbox keeps its own state; its change reaches the poll, so that the options' marks follow it. */}
      <div
        className="cmp-poll-check"
        onChange={(e) => {
          if (e.target instanceof HTMLInputElement) onChange({ ...poll, multiple: e.target.checked });
        }}
      >
        <CheckField id={`${id}-multiple`} name="multiple" value="1" label={s.multiple} defaultChecked={poll.multiple} />
      </div>
      <CheckField
        id={`${id}-hide`}
        name="hide_totals"
        value="1"
        label={s.hideTotals}
        defaultChecked={poll.hideTotals}
      />
    </div>
  );
  return (
    <fieldset className="cmp-poll">
      <legend className="cmp-sr">{s.pollTitle}</legend>
      <div className="cmp-poll-head">
        <span className="cmp-glyph-label cmp-muted">
          <Icon name="poll" place="caption" />
          {s.pollTitle}
        </span>
        <IconButton icon="close" label={s.pollOff} onPress={onRemove} />
      </div>
      {options}
      {settings}
    </fieldset>
  );
}

/** The composer, from `initial`; `onLeave` hears how it was left, with the draft as it was. */
export function Composer({
  initial,
  onLeave,
}: {
  initial: Draft;
  onLeave?: ((how: Leave, draft: Draft) => void) | undefined;
}) {
  const { locale } = useLocale();
  const s = stringsFor(locale);
  const [d, setD] = useState(initial);
  const [pop, setPop] = useState(initial.pop);
  const textId = useId();
  const titleId = useId();
  const set = (patch: Partial<Draft>) => {
    setD((prev) => ({ ...prev, ...patch }));
  };

  const used = chars(d.text).length + chars(d.cw ?? "").length;
  const left = maxChars - used;
  const over = Math.max(0, -left);
  const busy = d.phase === "submitting";
  const remote = d.quote?.remote === true;

  // Close and Escape mean to leave: a written post is discarded, and the page offers to undo it.
  const close = () => {
    onLeave?.(written(d) ? "discarded" : "empty", d);
  };
  const submit = () => {
    if (over > 0) return;
    const missing = d.images.some((image) => !image.uploading && image.alt.trim() === "");
    if (missing && d.descriptionsRequired) {
      set({ checked: true });
      return;
    }
    set({ phase: "submitting" });
    setTimeout(() => {
      if (d.failNext) set({ phase: "failed" });
      else {
        setD(emptyDraft(d.language));
        onLeave?.("posted", d);
      }
    }, 1500);
  };

  const shut = () => {
    setPop(undefined);
  };
  const toggle = (which: "audience" | "language") => () => {
    setPop((p) => (p === which || (which === "audience" && p === "permissions") ? undefined : which));
  };
  const limited = interactions.some((i) => d.policy[i] !== "everyone");

  const audienceChoices =
    pop === "permissions" ? (
      <>
        <button
          type="button"
          className="cmp-choice cmp-back"
          onClick={() => {
            setPop("audience");
          }}
        >
          <span className="cmp-choice-glyph">
            <Icon name="back" place="action" />
          </span>
          <span className="cmp-choice-label">{s.permissions}</span>
        </button>
        {interactions.map((i) => (
          <Choices
            key={i}
            title={s.interaction[i]}
            items={policies.map((p) => ({ id: p, icon: policyIcon[p], label: s.policy[p] }))}
            chosen={d.policy[i]}
            onChoose={(p) => {
              set({ policy: { ...d.policy, [i]: p } });
            }}
          />
        ))}
      </>
    ) : (
      <>
        <Choices
          title={s.visibilityLabel}
          items={visibilities.map((v) => ({
            id: v,
            icon: visibilityIcon[v],
            label: s.visibility[v],
            about: s.visibilityAbout[v],
            disabled: remote && closed.includes(v),
          }))}
          chosen={d.visibility}
          onChoose={(visibility) => {
            set({ visibility });
          }}
          note={remote ? s.quoteRemote : undefined}
        />
        <div className="cmp-choices cmp-choices-apart" role="group" aria-label={s.interactionsLabel}>
          <p className="cmp-choices-title">{s.interactionsLabel}</p>
          <Opens
            icon="settings"
            label={s.permissionsShort}
            now={limited ? s.limited : s.everyone}
            onPress={() => {
              setPop("permissions");
            }}
          />
        </div>
      </>
    );

  const audienceName = `${s.visibilityLabel}: ${s.visibility[d.visibility]}${limited ? `, ${s.limited}` : ""}`;
  const audience = (
    <Pop
      open={pop === "audience" || pop === "permissions"}
      onClose={shut}
      label={s.visibilityLabel}
      trigger={<IconButton icon={visibilityIcon[d.visibility]} label={audienceName} onPress={toggle("audience")} />}
    >
      {audienceChoices}
    </Pop>
  );
  // The language by its name in its own words (English, 日本語), short as the picker lists it, never a code.
  const language = (
    <Pop
      open={pop === "language"}
      onClose={shut}
      label={s.language}
      trigger={
        <Button
          kind="quiet"
          size="timeline"
          aria-label={`${s.language}: ${s.languages[d.language]}`}
          onPress={toggle("language")}
        >
          <span className="cmp-language" lang={d.language}>
            {s.languages[d.language]}
          </span>
        </Button>
      }
    >
      <Choices
        title={s.language}
        items={languages.map((l) => ({ id: l, icon: "translate", label: s.languages[l] }))}
        chosen={d.language}
        onChoose={(language) => {
          set({ language });
          setPop(undefined);
        }}
      />
    </Pop>
  );

  const title = d.kind === "reply" ? s.title.reply : d.kind === "quote" ? s.title.quote : s.title.new;
  return (
    <div
      className="cmp"
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      onKeyDown={(e) => {
        if (e.key === "Escape" && pop === undefined) close();
      }}
    >
      <div className="cmp-head">
        <h2 id={titleId} className="cmp-title">
          {title}
        </h2>
        <IconButton icon="close" label={s.close} onPress={close} />
      </div>

      <div className="cmp-body">
        {d.replyTo && <ReplyContext post={d.replyTo} s={s} />}

        {d.quote && (
          <QuoteContext
            post={d.quote.post}
            s={s}
            onRemove={() => {
              set({ quote: undefined, kind: "new" });
            }}
          />
        )}

        {d.cw !== undefined && (
          <div className="cmp-cw">
            <span className="cmp-cw-glyph">
              <Icon name="hidden" place="caption" />
            </span>
            <input
              className="cmp-cw-input"
              value={d.cw}
              placeholder={s.cwPlaceholder}
              aria-label={s.cw}
              readOnly={busy}
              dir="auto"
              onChange={(e) => {
                set({ cw: e.target.value });
              }}
            />
          </div>
        )}

        <TextArea
          id={textId}
          value={d.text}
          over={over}
          placeholder={s.text}
          label={s.textLabel}
          error={over > 0 ? s.tooLong(maxChars) : undefined}
          readOnly={busy}
          onChange={(text) => {
            set({ text });
          }}
        />

        {d.images.length > 0 && (
          <Images
            images={d.images}
            required={d.descriptionsRequired === true && d.checked === true}
            s={s}
            onChange={(i, alt) => {
              set({ images: d.images.map((image, j) => (j === i ? { ...image, alt } : image)) });
            }}
            onRemove={(i) => {
              set({ images: d.images.filter((_, j) => j !== i) });
            }}
          />
        )}

        {d.poll && (
          <Poll
            poll={d.poll}
            s={s}
            onChange={(poll) => {
              set({ poll });
            }}
            onRemove={() => {
              set({ poll: undefined });
            }}
          />
        )}
      </div>

      <div className="cmp-end">
        {d.phase === "failed" && <FormAlert>{s.failed}</FormAlert>}
        <div className="cmp-row">
          <div className="cmp-tools">
            <IconButton
              icon="attach"
              label={s.attach}
              isDisabled={d.poll !== undefined || d.images.length >= 4}
              onPress={() => {
                set({ images: [...d.images, { src: sampleImage(d.images.length), alt: "" }] });
              }}
            />
            <IconButton
              icon="poll"
              label={d.poll ? s.pollOff : s.poll}
              pressed={d.poll !== undefined}
              isDisabled={d.images.length > 0}
              onPress={() => {
                set({
                  poll: d.poll ? undefined : { options: ["", ""], duration: 2, multiple: false, hideTotals: false },
                });
              }}
            />
            <IconButton
              icon="hidden"
              label={d.cw === undefined ? s.cwOn : s.cwOff}
              pressed={d.cw !== undefined}
              onPress={() => {
                set({ cw: d.cw === undefined ? "" : undefined });
              }}
            />
            <IconButton icon="emoji" label={s.emoji} />
            {audience}
            {language}
          </div>
          <span
            className="cmp-count"
            data-near={left <= nearChars && left >= 0 ? "" : undefined}
            data-over={left < 0 ? "" : undefined}
            aria-label={s.remaining(left)}
            role="status"
          >
            {left}
          </span>
          <Button kind="primary" size="timeline" busy={busy} onPress={submit}>
            {s.post}
          </Button>
        </div>
      </div>
    </div>
  );
}
