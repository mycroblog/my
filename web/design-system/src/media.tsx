// SPDX-License-Identifier: AGPL-3.0-only

// The media of an entry (a post's attachments), with no border and the corners of a control. The pictures (images,
// GIFs and videos) share one box 16:9 of its width up to a height at the full width, so that the next entry shows on
// the same screen at PC: one alone, two side by side, three or four in a grid of two by two. A sound is a row of its
// own under them, a play button, its description and its length; a file of a kind the client cannot show is a link
// to it. As a preview (in a quoted post), each picture is a small tile in a row, so that what is embedded never
// outweighs what carries it, and nothing in it plays or opens.
//
// A picture is a button: an image or a GIF opens the viewer at it (`onOpen`), a video plays where it is, with the
// browser's controls, and opens whole from its corner. A GIF plays muted and looped, and stands still, on its first
// frame, for a reader who prefers reduced motion. Marks sit in a picture's corners on a dark backing, in white at any
// scheme: ALT where it has a description, GIF, a video's length. Each picture's alternative text is its description;
// one without one is named by its kind.
//
// Sensitive media (`sensitive`) stays covered until the reader shows it, apart from a content warning, which folds
// the text: one muted line above it names what it is by kind and count ("4 sensitive images") and ends in a button,
// Show, then Hide, in the same place and words in both states; the pictures under it are their blurhash, nothing of them loaded
// until shown (the reader's privacy and bandwidth), and a press on them shows them too; sounds and files are folded out
// of the layout. A link card's sensitive image takes the same line.

import { useReducedMotion } from "motion/react";
import { useId, useRef, useState } from "react";
import { Button as AriaButton } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Blurhash } from "./blurhash";
import { Button } from "./button";
import { IconButton } from "./icon-button";
import { Icon } from "./icons";
import { stringsIn, useReader } from "./locale";

export type MediaKind = "image" | "gifv" | "video" | "audio" | "unknown";

export type MediaItem = {
  /** Where the picture is served: a preview's own rendition when there is one, a video's or a sound's poster. */
  src: string;
  /** The alternative text; empty for none. */
  alt: string;
  /** What tells the item apart from the others (its id). */
  id: string;
  /** The kind; an image by default. */
  kind?: MediaKind | undefined;
  /** The file itself: a video's or a sound's, an image's largest rendition (the viewer's), an unknown file's link. */
  href?: string | undefined;
  /** A video's or a sound's length in seconds. */
  duration?: number | undefined;
  /** The picture's blurhash, drawn while it is covered. */
  blurhash?: string | undefined;
};

const grid = tv({
  slots: {
    root: "grid grid-cols-[minmax(0,1fr)] gap-media",
    frame: "@container",
    box: [
      "grid gap-media overflow-hidden rounded-control",
      "h-[min(calc(100cqw*var(--media-ratio)),var(--media-max-height))]",
    ],
    cell: "relative min-h-0 overflow-hidden bg-surface-raised",
    picture: "block size-full min-h-0 object-cover",
    press: [
      "absolute inset-0 cursor-zoom-in rounded-control border-0 bg-transparent p-0 outline-hidden",
      "data-focus-visible:shadow-focus-inset",
    ],
    marks: "pointer-events-none absolute inset-x-media-badge bottom-media-badge flex items-end gap-media",
    mark: "rounded-control bg-media-badge px-[0.375em] text-caption font-label text-on-media",
    end: "ms-auto",
    play: [
      "pointer-events-none absolute inset-0 m-auto flex size-media-play items-center justify-center rounded-full",
      "bg-media-badge text-on-media",
    ],
    corner: "absolute end-media-badge top-media-badge rounded-control bg-media-badge text-on-media",
    row: "flex flex-wrap gap-media",
    tile: "relative h-media-preview-height w-media-preview-width overflow-hidden rounded-control bg-surface-raised",
    tilePicture: "block size-full object-cover",
  },
  variants: {
    columns: { 1: { box: "grid-cols-[minmax(0,1fr)]" }, 2: { box: "grid-cols-[minmax(0,1fr)_minmax(0,1fr)]" } },
    rows: { 1: { box: "grid-rows-[minmax(0,1fr)]" }, 2: { box: "grid-rows-[minmax(0,1fr)_minmax(0,1fr)]" } },
    // Three in a grid of two by two: the first takes the whole first column.
    tall: { true: { cell: "first:row-span-2" } },
  },
});

const line = tv({
  slots: {
    // A caption's type in the muted color, the glyph and the toggle level with the label's first line.
    root: "flex w-full items-start gap-entry-caption font-sans text-caption text-muted-text",
    glyph: "inline-flex h-control-timeline shrink-0 items-center",
    // Centred on the toggle's height, as a content warning's summary is (disclosure.tsx).
    text: "min-w-0 flex-1 py-[calc((var(--control-timeline-height)-1lh)/2)]",
  },
});

// The covered pictures' own button: a press anywhere on them shows them, as the line does; out of the tab order and
// hidden from a screen reader, since the line is the one a keyboard reaches and a screen reader hears.
const veil = [
  "absolute inset-0 cursor-pointer rounded-control border-0 bg-transparent p-0 outline-hidden",
  "data-focus-visible:shadow-focus-inset",
].join(" ");

const audio = tv({
  slots: {
    root: "flex items-center gap-entry-caption rounded-control bg-surface-raised py-disclosure-padding pe-embed ps-disclosure-padding",
    text: "grid min-w-0 flex-1 gap-[0.125rem]",
    name: "truncate text-body",
    meta: "flex items-center gap-entry-caption text-caption text-muted-text tabular-nums",
    track: "relative h-[0.1875rem] flex-1 overflow-hidden rounded-full bg-surface-raised-pressed",
    done: "absolute inset-y-0 start-0 bg-text",
  },
});

const file = tv({
  base: [
    "flex items-center gap-entry-caption rounded-control bg-surface-raised px-embed py-disclosure-padding",
    "text-body text-text no-underline outline-hidden data-focus-visible:shadow-focus hover:bg-surface-raised-hover",
  ],
});

/** A length in seconds as the time a player shows: 0:07, 3:12, 1:02:03. */
export function playTime(seconds: number): string {
  const s = Math.max(0, Math.round(seconds));
  const two = (n: number) => String(n).padStart(2, "0");
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  return h > 0 ? `${h}:${two(m)}:${two(s % 60)}` : `${m}:${two(s % 60)}`;
}

const kindOf = (m: MediaItem): MediaKind => m.kind ?? "image";
const isPicture = (m: MediaItem) => ["image", "gifv", "video"].includes(kindOf(m));

type T = ReturnType<typeof stringsIn>["t"];

/** Gives the focus to a video's player, a tab stop, as it takes the place of the play button, which had it. */
const takeFocus = (player: HTMLVideoElement | null) => {
  player?.focus({ preventScroll: true });
};

/** One picture of the grid: what it draws, what pressing it does, and its marks. */
function Cell({
  item,
  covered,
  tall,
  t,
  onOpen,
}: {
  item: MediaItem;
  covered: boolean;
  tall: boolean;
  t: T;
  onOpen: (() => void) | undefined;
}) {
  const kind = kindOf(item);
  const s = grid({ tall });
  const [playing, setPlaying] = useState(false);
  const still = useReducedMotion() ?? false;
  const named =
    item.alt === "" ? t("media.open", { kind }) : t("media.open_described", { kind, description: item.alt });
  // Covered, nothing of the picture loads (neither the image nor a video's poster): its blurhash stands in for it.
  if (covered)
    return (
      <div className={s.cell()}>
        <Blurhash hash={item.blurhash} className={s.picture()} />
      </div>
    );
  const whole = kind === "video" && onOpen !== undefined && (
    <span className={s.corner()}>
      <MediaIconButton icon="whole" label={t("media.whole")} onPress={onOpen} />
    </span>
  );
  // Playing, a video keeps its description as its name, and still opens whole from its corner.
  if (kind === "video" && playing)
    return (
      <div className={s.cell()}>
        <video
          ref={takeFocus}
          tabIndex={0}
          className={s.picture()}
          src={item.href}
          poster={item.src}
          aria-label={item.alt === "" ? undefined : item.alt}
          controls
          autoPlay
          playsInline
        />
        {whole}
      </div>
    );
  const picture =
    kind === "gifv" ? (
      <video
        className={s.picture()}
        src={item.href}
        poster={item.src}
        aria-label={item.alt === "" ? undefined : item.alt}
        muted
        loop
        playsInline
        autoPlay={!still}
      />
    ) : (
      <img className={s.picture()} src={item.src} alt={item.alt} />
    );
  return (
    <div className={s.cell()}>
      {picture}
      {kind === "video" ? (
        <AriaButton
          className={s.press()}
          aria-label={item.alt === "" ? t("media.play") : t("media.play_described", { description: item.alt })}
          onPress={() => {
            setPlaying(true);
          }}
        />
      ) : (
        onOpen !== undefined && <AriaButton className={s.press()} aria-label={named} onPress={onOpen} />
      )}
      {kind === "video" && (
        <span className={s.play()} aria-hidden="true">
          <Icon name="play" place="action" />
        </span>
      )}
      <span className={s.marks()} aria-hidden="true">
        {item.alt !== "" && <span className={s.mark()}>{t("media.alt")}</span>}
        {kind === "gifv" && <span className={s.mark()}>{t("media.gif")}</span>}
        {kind === "video" && item.duration !== undefined && (
          <span className={`${s.mark()} ${s.end()} tabular-nums`}>{playTime(item.duration)}</span>
        )}
      </span>
      {whole}
    </div>
  );
}

/** An icon button over a picture: white on the badge's backing, whatever the scheme. */
function MediaIconButton({ icon, label, onPress }: { icon: "whole" | "hidden"; label: string; onPress: () => void }) {
  return (
    <span className="flex [color-scheme:dark] [&>button]:text-on-media">
      <IconButton icon={icon} label={label} onPress={onPress} />
    </span>
  );
}

/** The pictures' box at the full width, laid out by how many there are. */
function Pictures({
  items,
  covered,
  t,
  onOpen,
}: {
  items: readonly MediaItem[];
  covered: boolean;
  t: T;
  onOpen: ((index: number) => void) | undefined;
}) {
  const n = items.length;
  const s = grid({ columns: n === 1 ? 1 : 2, rows: n > 2 ? 2 : 1 });
  return (
    <div className={s.frame()}>
      <div className={s.box()}>
        {items.map((m, i) => (
          <Cell
            key={m.id}
            item={m}
            covered={covered}
            tall={n === 3}
            t={t}
            onOpen={
              onOpen === undefined
                ? undefined
                : () => {
                    onOpen(i);
                  }
            }
          />
        ))}
      </div>
    </div>
  );
}

/** A sound: a play button, its description (or its kind) over its progress and its length. */
export function AudioRow({ item, locale }: { item: MediaItem; locale?: string | undefined }) {
  const { lang, direction } = useReader(locale);
  const { t } = stringsIn(lang);
  const s = audio();
  const ref = useRef<HTMLAudioElement>(null);
  const [playing, setPlaying] = useState(false);
  const [at, setAt] = useState(0);
  const length = item.duration ?? 0;
  return (
    <div className={s.root()} dir={direction}>
      {/* One button whose name says what it does now: Play, or Pause while it plays; no toggle state besides. */}
      <IconButton
        icon={playing ? "pause" : "play"}
        label={t(playing ? "media.audio_pause" : "media.audio_play")}
        onPress={() => {
          const el = ref.current;
          if (el === null) return;
          if (el.paused) void el.play();
          else el.pause();
        }}
      />
      <div className={s.text()}>
        <span className={s.name()}>{item.alt === "" ? t("media.audio") : item.alt}</span>
        <span className={s.meta()}>
          <Icon name="audio" place="caption" />
          <span className={s.track()} aria-hidden="true">
            <span className={s.done()} style={{ width: length > 0 ? `${(at / length) * 100}%` : "0%" }} />
          </span>
          <span aria-hidden="true">
            {playTime(at)} / {playTime(length)}
          </span>
          <span className="sr-only">{t("media.audio_time", { at: playTime(at), length: playTime(length) })}</span>
        </span>
      </div>
      <audio
        ref={ref}
        src={item.href}
        preload="none"
        onPlay={() => {
          setPlaying(true);
        }}
        onPause={() => {
          setPlaying(false);
        }}
        onTimeUpdate={(e) => {
          setAt(e.currentTarget.currentTime);
        }}
      />
    </div>
  );
}

/** The name of the file at `url`: its path's last segment, decoded; empty when it has none. */
function fileName(url: URL): string {
  const last = url.pathname.split("/").findLast((segment) => segment !== "") ?? "";
  try {
    return decodeURIComponent(last);
  } catch {
    return last;
  }
}

/** A file of a kind the client cannot show: a link to it, by its description or else its file's name, with its host. */
export function FileLink({ item, locale }: { item: MediaItem; locale?: string | undefined }) {
  const { lang, direction } = useReader(locale);
  const { t } = stringsIn(lang);
  let host = "";
  let name = "";
  try {
    const url = new URL(item.href ?? item.src);
    host = url.host;
    name = fileName(url);
  } catch {
    host = "";
  }
  const title = [item.alt, name].find((text) => text !== "") ?? t("media.file");
  return (
    <a className={file()} dir={direction} href={item.href ?? item.src} target="_blank" rel="noopener noreferrer">
      <span className="inline-flex text-muted-text">
        <Icon name="file" place="action" />
      </span>
      <span className="grid min-w-0 flex-1">
        <span className="truncate">{title}</span>
        {host !== "" && (
          <bdi dir="ltr" className="truncate text-caption text-muted-text">
            {host}
          </bdi>
        )}
      </span>
      <span className="inline-flex text-muted-text">
        <Icon name="open" place="caption" />
      </span>
      <span className="sr-only"> {t("media.new_tab")}</span>
    </a>
  );
}

/**
 * What sensitive media is, by its kind and count: "4 sensitive images", "Sensitive video", "3 sensitive attachments"
 * for several kinds. A link card's image is one image.
 */
export function sensitiveLabel(items: readonly Pick<MediaItem, "kind">[], t: T): string {
  const kinds = new Set(items.map((m) => m.kind ?? "image"));
  const count = items.length;
  if (kinds.size !== 1) return t("media.sensitive_mixed", { count });
  const [kind = "image"] = kinds;
  const ids = {
    image: "media.sensitive_image",
    gifv: "media.sensitive_gif",
    video: "media.sensitive_video",
    audio: "media.sensitive_audio",
    unknown: "media.sensitive_file",
  } as const;
  return t(ids[kind], { count });
}

export type SensitiveLineProps = {
  /** What is covered (`sensitiveLabel`). */
  label: string;
  shown: boolean;
  onToggle: () => void;
  /** The id of what it covers, which it controls. */
  controls?: string | undefined;
  /** The reader's locale, for Show and Hide; the I18nProvider's by default. */
  locale?: string | undefined;
};

/**
 * The line above sensitive media: the cover's glyph, what it covers, then Show, or Hide once shown, in the same place
 * and words in both states: the quiet button a content warning's Show is (disclosure.tsx), described by what it covers.
 */
export function SensitiveLine({ label, shown, onToggle, controls, locale }: SensitiveLineProps) {
  const { lang, direction } = useReader(locale);
  const { t } = stringsIn(lang);
  const labelId = useId();
  const s = line();
  return (
    <div className={s.root()} dir={direction}>
      <span className={s.glyph()}>
        <Icon name="hidden" place="caption" />
      </span>
      <span id={labelId} className={s.text()}>
        {label}
      </span>
      <Button
        kind="quiet"
        size="timeline"
        aria-expanded={shown}
        aria-describedby={labelId}
        {...(controls === undefined ? {} : { "aria-controls": controls })}
        onPress={onToggle}
      >
        <span lang={lang}>{t(shown ? "media.hide" : "media.show")}</span>
      </Button>
    </div>
  );
}

export type MediaGridProps = {
  items: readonly MediaItem[];
  /** At the full width (a post's), or as small previews (a quoted post's). */
  size?: "full" | "preview";
  /** Covered until the reader shows it. */
  sensitive?: boolean | undefined;
  /** Opens the viewer at the picture of this index among the pictures (images, GIFs and videos). */
  onOpen?: ((index: number) => void) | undefined;
  /** The reader's locale, for the marks and the cover; the I18nProvider's by default. */
  locale?: string | undefined;
};

export function MediaGrid({ items, size = "full", sensitive = false, onOpen, locale }: MediaGridProps) {
  const { lang, direction } = useReader(locale);
  const { t } = stringsIn(lang);
  const [shown, setShown] = useState(false);
  const covers = useId();
  if (items.length === 0) return null;
  const pictures = items.filter(isPicture);
  const sounds = items.filter((m) => kindOf(m) === "audio");
  const files = items.filter((m) => kindOf(m) === "unknown");
  const covered = sensitive && !shown;
  if (size === "preview") {
    // A preview never shows sensitive media: its blurhash stands in for it, and nothing of it loads.
    const s = grid();
    return (
      <div className={s.row()}>
        {items.map((m) => (
          <span key={m.id} className={s.tile()}>
            {isPicture(m) ? (
              sensitive ? (
                <Blurhash hash={m.blurhash} className={s.tilePicture()} />
              ) : (
                <img className={s.tilePicture()} src={m.src} alt={m.alt} />
              )
            ) : (
              <span className="flex size-full items-center justify-center text-muted-text">
                <Icon name={kindOf(m) === "audio" ? "audio" : "file"} place="action" />
              </span>
            )}
          </span>
        ))}
      </div>
    );
  }
  const pictureBox = (hidden: boolean) =>
    pictures.length > 0 && <Pictures items={pictures} covered={hidden} t={t} onOpen={onOpen} />;
  const rest = (
    <>
      {sounds.map((m) => (
        <AudioRow key={m.id} item={m} locale={locale} />
      ))}
      {files.map((m) => (
        <FileLink key={m.id} item={m} locale={locale} />
      ))}
    </>
  );
  const s = grid();
  if (!sensitive)
    return (
      <div className={s.root()} dir={direction}>
        {pictureBox(false)}
        {rest}
      </div>
    );
  // Sensitive: the line above, the pictures blurred and the sounds and files folded until shown.
  const label = sensitiveLabel(items, t);
  return (
    <div className={s.root()} dir={direction}>
      <SensitiveLine
        label={label}
        shown={shown}
        controls={covers}
        locale={locale}
        onToggle={() => {
          setShown(!shown);
        }}
      />
      <div id={covers} className={s.root()}>
        {pictures.length > 0 && (
          <div className="relative">
            {pictureBox(covered)}
            {covered && (
              <div className="contents" aria-hidden="true">
                <AriaButton
                  className={veil}
                  aria-label={`${label} · ${t("media.show")}`}
                  excludeFromTabOrder
                  onPress={() => {
                    setShown(true);
                  }}
                />
              </div>
            )}
          </div>
        )}
        {!covered && rest}
      </div>
    </div>
  );
}
