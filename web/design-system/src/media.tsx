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
import { useId, useRef, useState, type ComponentProps, type ReactNode } from "react";
import { Button as AriaButton } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Blurhash } from "./blurhash";
import { Button } from "./button";
import { IconButton } from "./icon-button";
import { Icon } from "./icons";
import { Box, Pin, Row, Stack } from "./layout";
import { stringsIn, useReader } from "./locale";
import { usePreviewState } from "./preview";

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
  /** The size of the file itself in px (an image's original), when it is known: the viewer's box before it loads. */
  width?: number | undefined;
  height?: number | undefined;
  /** The picture's blurhash, drawn while it is covered. */
  blurhash?: string | undefined;
};

// How a picture answers the pointer, never moving: the press tokens' shade over it, at the share a filled control mixes
// in hovered and pressed; and the ring inside it on keyboard focus.
const pictureShade = [
  "data-hovered:bg-[color-mix(in_srgb,var(--press-shade)_var(--press-hover),transparent)]",
  "data-pressed:bg-[color-mix(in_srgb,var(--press-shade)_var(--press-pressed),transparent)]",
  "data-focus-visible:shadow-focus-inset",
].join(" ");

const grid = tv({
  slots: {
    cell: "bg-surface-raised",
    picture: "object-cover",
    press: ["cursor-zoom-in rounded-control border-0 bg-transparent p-0 outline-hidden", pictureShade],
    marks: "pointer-events-none",
    mark: "rounded-control bg-media-badge px-media-mark text-caption font-label text-on-media",
    play: "pointer-events-none size-media-play rounded-full bg-media-badge text-on-media",
    corner: "rounded-control bg-media-badge text-on-media",
    tile: "h-media-preview-height w-media-preview-width rounded-control bg-surface-raised",
    tilePicture: "object-cover",
  },
});

// The pictures' box, a layout part of its own: one box 16:9 of its container's width up to a height, whose frame is
// the container it measures, laid out by how many pictures there are (two columns from two, two rows from three, the
// first of three taking the whole first column).
const pictures = tv({
  slots: {
    frame: "@container",
    box: [
      "grid gap-media overflow-hidden rounded-control",
      "h-[min(calc(100cqw*var(--media-ratio)),var(--media-max-height))]",
    ],
  },
  variants: {
    columns: { 1: { box: "grid-cols-[minmax(0,1fr)]" }, 2: { box: "grid-cols-[minmax(0,1fr)_minmax(0,1fr)]" } },
    rows: { 1: { box: "grid-rows-[minmax(0,1fr)]" }, 2: { box: "grid-rows-[minmax(0,1fr)_minmax(0,1fr)]" } },
    tall: { true: { box: "[&>:first-child]:row-span-2" } },
  },
});

function PictureGrid({ count, children }: { count: number; children: ReactNode }) {
  const s = pictures({ columns: count === 1 ? 1 : 2, rows: count > 2 ? 2 : 1, tall: count === 3 });
  return (
    <div className={s.frame()}>
      <div className={s.box()}>{children}</div>
    </div>
  );
}

const line = tv({
  slots: {
    // A caption's type in the muted color, the glyph and the toggle level with the label's first line.
    root: "font-sans text-caption text-muted-text",
    glyph: "h-control-timeline",
    // Centred on the toggle's height, as a content warning's summary is (disclosure.tsx).
    text: "py-media-line",
  },
});

// The covered pictures' own button: a press anywhere on them shows them, as the line does; out of the tab order and
// hidden from a screen reader, since the line is the one a keyboard reaches and a screen reader hears. It goes as it is
// pressed, so it never takes the focus, which stays where it was.
const veil = `cursor-pointer rounded-control border-0 bg-transparent p-0 outline-hidden ${pictureShade}`;

const audio = tv({
  slots: {
    root: "rounded-control bg-surface-raised py-disclosure-padding pe-embed ps-disclosure-padding",
    name: "text-body",
    meta: "text-caption text-muted-text tabular-nums",
    track: "h-media-track rounded-full bg-surface-raised-pressed",
    done: "bg-text",
  },
});

const file = tv({
  base: [
    "rounded-control bg-surface-raised px-embed py-disclosure-padding",
    "text-body text-text no-underline outline-hidden data-focus-visible:shadow-focus hover:bg-surface-raised-hover",
    "active:translate-y-press active:bg-surface-raised-pressed",
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

/** A press layer's button, hovered or pressed as a story's preview asks. */
function usePreviewed() {
  const preview = usePreviewState();
  return (props: ComponentProps<"button">, { isHovered, isPressed }: { isHovered: boolean; isPressed: boolean }) => (
    <button
      {...props}
      data-hovered={isHovered || preview === "hover" || undefined}
      data-pressed={isPressed || preview === "pressed" || undefined}
    />
  );
}

/** One picture of the grid: what it draws, what pressing it does, and its marks. */
function Cell({
  item,
  covered,
  t,
  onOpen,
}: {
  item: MediaItem;
  covered: boolean;
  t: T;
  onOpen: (() => void) | undefined;
}) {
  const kind = kindOf(item);
  const s = grid();
  const previewed = usePreviewed();
  const [playing, setPlaying] = useState(false);
  const still = useReducedMotion() ?? false;
  const named =
    item.alt === "" ? t("media.open", { kind }) : t("media.open_described", { kind, description: item.alt });
  // Covered, nothing of the picture loads (neither the image nor a video's poster): its blurhash stands in for it.
  if (covered)
    return (
      <Box anchor clip shrink="block" className={s.cell()}>
        <Box
          as={<Blurhash hash={item.blurhash} />}
          display="block"
          full="both"
          shrink="block"
          className={s.picture()}
        />
      </Box>
    );
  const whole = kind === "video" && onOpen !== undefined && (
    <Pin as="span" end="media-badge" top="media-badge" className={s.corner()}>
      <MediaIconButton icon="whole" label={t("media.whole")} onPress={onOpen} />
    </Pin>
  );
  // Playing, a video keeps its description as its name, and still opens whole from its corner.
  if (kind === "video" && playing)
    return (
      <Box anchor clip shrink="block" className={s.cell()}>
        <Box
          as={
            <video
              ref={takeFocus}
              tabIndex={0}
              src={item.href}
              poster={item.src}
              aria-label={item.alt === "" ? undefined : item.alt}
              controls
              autoPlay
              playsInline
            />
          }
          display="block"
          full="both"
          shrink="block"
          className={s.picture()}
        />
        {whole}
      </Box>
    );
  const picture =
    kind === "gifv" ? (
      <Box
        as={
          <video
            src={item.href}
            poster={item.src}
            aria-label={item.alt === "" ? undefined : item.alt}
            muted
            loop
            playsInline
            autoPlay={!still}
          />
        }
        display="block"
        full="both"
        shrink="block"
        className={s.picture()}
      />
    ) : (
      <Box
        as={<img src={item.src} alt={item.alt} />}
        display="block"
        full="both"
        shrink="block"
        className={s.picture()}
      />
    );
  return (
    <Box anchor clip shrink="block" className={s.cell()}>
      {picture}
      {kind === "video" ? (
        <Pin
          as={
            <AriaButton
              aria-label={item.alt === "" ? t("media.play") : t("media.play_described", { description: item.alt })}
              onPress={() => {
                setPlaying(true);
              }}
              render={previewed}
            />
          }
          cover
          className={s.press()}
        />
      ) : (
        onOpen !== undefined && (
          <Pin as={<AriaButton aria-label={named} onPress={onOpen} render={previewed} />} cover className={s.press()} />
        )
      )}
      {kind === "video" && (
        <Row as={<Pin as="span" center aria-hidden="true" />} align="center" justify="center" className={s.play()}>
          <Icon name="play" place="action" />
        </Row>
      )}
      <Row
        as={<Pin as="span" insetInline="media-badge" bottom="media-badge" aria-hidden="true" />}
        align="end"
        gap="media"
        className={s.marks()}
      >
        {item.alt !== "" && <span className={s.mark()}>{t("media.alt")}</span>}
        {kind === "gifv" && <span className={s.mark()}>{t("media.gif")}</span>}
        {kind === "video" && item.duration !== undefined && (
          <Box as="span" push="end" className={`${s.mark()} tabular-nums`}>
            {playTime(item.duration)}
          </Box>
        )}
      </Row>
      {whole}
    </Box>
  );
}

/** An icon button over a picture: white on the badge's backing, whatever the scheme. */
function MediaIconButton({ icon, label, onPress }: { icon: "whole" | "hidden"; label: string; onPress: () => void }) {
  return (
    <Row as="span" className="[color-scheme:dark] [&>button]:text-on-media">
      <IconButton icon={icon} label={label} onPress={onPress} />
    </Row>
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
  return (
    <PictureGrid count={n}>
      {items.map((m, i) => (
        <Cell
          key={m.id}
          item={m}
          covered={covered}
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
    </PictureGrid>
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
    <Row align="center" gap="entry-caption" className={s.root()} dir={direction}>
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
      <Stack grow gap="hair">
        <Box as="span" lines={1} className={s.name()}>
          {item.alt === "" ? t("media.audio") : item.alt}
        </Box>
        <Row as="span" align="center" gap="entry-caption" className={s.meta()}>
          <Icon name="audio" place="caption" />
          <Box as="span" anchor clip grow className={s.track()} aria-hidden="true">
            <Box
              as={<Pin as="span" insetBlock="0" start="0" />}
              share={length > 0 ? at / length : 0}
              className={s.done()}
            />
          </Box>
          <span aria-hidden="true">
            {playTime(at)} / {playTime(length)}
          </span>
          <span className="sr-only">{t("media.audio_time", { at: playTime(at), length: playTime(length) })}</span>
        </Row>
      </Stack>
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
    </Row>
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
    <Row
      as={<a dir={direction} href={item.href ?? item.src} target="_blank" rel="noopener noreferrer" />}
      align="center"
      gap="entry-caption"
      className={file()}
    >
      <Row as="span" rigid className="text-muted-text">
        <Icon name="file" place="action" />
      </Row>
      <Stack as="span" grow>
        <Box as="span" lines={1}>
          {title}
        </Box>
        {host !== "" && (
          <Box as="bdi" dir="ltr" lines={1} className="text-caption text-muted-text">
            {host}
          </Box>
        )}
      </Stack>
      <Row as="span" rigid className="text-muted-text">
        <Icon name="open" place="caption" />
      </Row>
      <span className="sr-only"> {t("media.new_tab")}</span>
    </Row>
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
    <Row full align="start" gap="entry-caption" className={s.root()} dir={direction}>
      <Row as="span" rigid align="center" className={s.glyph()}>
        <Icon name="hidden" place="caption" />
      </Row>
      <Box as="span" grow id={labelId} className={s.text()}>
        {label}
      </Box>
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
    </Row>
  );
}

export type MediaGridProps = {
  items: readonly MediaItem[];
  /** At the full width (a post's), or as small previews (a quoted post's). */
  size?: "full" | "preview";
  /** Covered until the reader shows it. */
  sensitive?: boolean | undefined;
  /** Sensitive media is shown at first; covered by default. */
  defaultShown?: boolean | undefined;
  /** Opens the viewer at the picture of this index among the pictures (images, GIFs and videos). */
  onOpen?: ((index: number) => void) | undefined;
  /** The reader's locale, for the marks and the cover; the I18nProvider's by default. */
  locale?: string | undefined;
};

export function MediaGrid({
  items,
  size = "full",
  sensitive = false,
  defaultShown = false,
  onOpen,
  locale,
}: MediaGridProps) {
  const { lang, direction } = useReader(locale);
  const { t } = stringsIn(lang);
  const [shown, setShown] = useState(defaultShown);
  const covers = useId();
  const previewed = usePreviewed();
  if (items.length === 0) return null;
  const pictures = items.filter(isPicture);
  const sounds = items.filter((m) => kindOf(m) === "audio");
  const files = items.filter((m) => kindOf(m) === "unknown");
  const covered = sensitive && !shown;
  if (size === "preview") {
    // A preview never shows sensitive media: its blurhash stands in for it, and nothing of it loads.
    const s = grid();
    return (
      <Row wrap gap="media">
        {items.map((m) => (
          <Box as="span" key={m.id} anchor clip rigid className={s.tile()}>
            {isPicture(m) ? (
              sensitive ? (
                <Box as={<Blurhash hash={m.blurhash} />} display="block" full="both" className={s.tilePicture()} />
              ) : (
                <Box as={<img src={m.src} alt={m.alt} />} display="block" full="both" className={s.tilePicture()} />
              )
            ) : (
              <Row as="span" full="both" align="center" justify="center" className="text-muted-text">
                <Icon name={kindOf(m) === "audio" ? "audio" : "file"} place="action" />
              </Row>
            )}
          </Box>
        ))}
      </Row>
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
  if (!sensitive)
    return (
      <Stack gap="media" dir={direction}>
        {pictureBox(false)}
        {rest}
      </Stack>
    );
  // Sensitive: the line above, the pictures blurred and the sounds and files folded until shown.
  const label = sensitiveLabel(items, t);
  return (
    <Stack gap="media" dir={direction}>
      <SensitiveLine
        label={label}
        shown={shown}
        controls={covers}
        locale={locale}
        onToggle={() => {
          setShown(!shown);
        }}
      />
      <Stack id={covers} gap="media">
        {pictures.length > 0 && (
          <Box anchor>
            {pictureBox(covered)}
            {covered && (
              <div className="contents" aria-hidden="true">
                <Pin
                  as={
                    <AriaButton
                      aria-label={`${label} · ${t("media.show")}`}
                      excludeFromTabOrder
                      preventFocusOnPress
                      onPress={() => {
                        setShown(true);
                      }}
                      render={previewed}
                    />
                  }
                  cover
                  className={veil}
                />
              </div>
            )}
          </Box>
        )}
        {!covered && rest}
      </Stack>
    </Stack>
  );
}
