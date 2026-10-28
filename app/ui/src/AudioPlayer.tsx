// SPDX-License-Identifier: AGPL-3.0-only
import { type ComponentPropsWithRef, useEffect, useRef, useState } from "react";
import { Button, Slider, SliderThumb, SliderTrack, useLocale } from "react-aria-components";
import { tv } from "tailwind-variants";
import { formatClock } from "./clock";
import { PauseIcon, PlayIcon } from "./Icons";

const audioPlayer = tv({
  slots: {
    base: "flex items-center gap-3 rounded-lg bg-surface-sunken p-4",
    play: [
      "grid size-11 shrink-0 cursor-default place-items-center rounded-full bg-accent-fill text-accent-fill-fg",
      "data-focus-visible:focus-ring data-hovered:bg-accent-fill-hover motion-safe:transition-colors [&>svg]:size-5",
    ],
    core: "min-w-0 flex-1",
    slider: "w-full",
    track: "relative flex h-9.5 w-full items-center gap-1",
    // The bars are a fixed pattern that only decorates; the slider over them carries the position.
    bar: "flex-1 rounded-full bg-wave motion-safe:transition-opacity",
    // The playback position: a thin bar over the waveform, which the keyboard moves (the arrow keys, Home and End).
    thumb: "top-0 h-full w-1 rounded-full bg-accent-fill data-focus-visible:focus-ring data-disabled:hidden",
    details: "flex justify-between gap-2 text-xs text-fg",
    name: "min-w-0 truncate",
    time: "shrink-0 text-fg-muted tabular-nums",
  },
});

/** The height pattern of the waveform's bars, in the brand kit's order; it is not an analysis of the sound. */
const barHeights = ["h-3", "h-3", "h-8", "h-6", "h-4.5", "h-8", "h-3", "h-6", "h-3", "h-8", "h-4.5", "h-6"];
const bars = Array.from({ length: 24 }, (_, i) => ({
  id: `bar-${String(i)}`,
  height: barHeights[(i * 5 + (i % 3)) % barHeights.length] ?? "h-3",
}));

export type AudioPlayerLabels = {
  /** The accessible name of the play button ("Play"). */
  play: string;
  /** The accessible name of the play button while the audio plays ("Pause"). */
  pause: string;
  /** The accessible name of the seek slider ("Position in the audio"). */
  seek: string;
};

export type AudioPlayerProps = Omit<ComponentPropsWithRef<"div">, "children"> & {
  /** The audio file to play. */
  src: string;
  /** The name shown over the time, and the name of the whole player (a file name, or the description). */
  name: string;
  /** The language of `name`, when known. */
  lang?: string | undefined;
  labels: AudioPlayerLabels;
};

function useMediaTimes(audioRef: React.RefObject<HTMLAudioElement | null>) {
  const [times, setTimes] = useState({ current: 0, duration: 0, playing: false });
  useEffect(() => {
    const element = audioRef.current;
    if (element === null) return;
    const read = () => {
      setTimes({
        current: element.currentTime,
        duration: Number.isFinite(element.duration) ? element.duration : 0,
        playing: !element.paused && !element.ended,
      });
    };
    const events = [
      "loadedmetadata",
      "durationchange",
      "timeupdate",
      "seeked",
      "play",
      "playing",
      "pause",
      "ended",
    ] as const;
    for (const event of events) element.addEventListener(event, read);
    read();
    return () => {
      for (const event of events) element.removeEventListener(event, read);
    };
  }, [audioRef]);
  return times;
}

/**
 * The player of an audio file: a play / pause button, a decorative waveform with a seek slider over it, the name, and
 * the elapsed and total time. It wraps an `<audio>` without the browser's controls and never starts by itself
 * (ADR-0054 §6). The slider (React Aria) moves with the arrow keys, Home / End and PageUp / PageDown; it is disabled
 * until the file's length is known.
 */
export function AudioPlayer({ src, name, lang, labels, className, ...props }: AudioPlayerProps) {
  const audioRef = useRef<HTMLAudioElement>(null);
  const { current, duration, playing } = useMediaTimes(audioRef);
  const { locale } = useLocale();
  const slots = audioPlayer();
  const toggle = () => {
    const element = audioRef.current;
    if (element === null) return;
    if (element.paused || element.ended) {
      // A rejected play() (the file failed to load, or the user agent refused) leaves the button as it was.
      void element.play().catch(() => undefined);
    } else {
      element.pause();
    }
  };
  const clock = (seconds: number) => formatClock(locale, seconds);
  const times = `${clock(current)} / ${clock(duration)}`;
  const fraction = duration > 0 ? current / duration : 0;
  return (
    <div role="group" aria-label={name} {...props} className={slots.base({ className })}>
      <audio ref={audioRef} src={src} preload="metadata" />
      <Button aria-label={playing ? labels.pause : labels.play} onPress={toggle} className={slots.play()}>
        {playing ? <PauseIcon /> : <PlayIcon />}
      </Button>
      <div className={slots.core()}>
        <Slider
          aria-label={labels.seek}
          value={Math.min(current, duration)}
          minValue={0}
          maxValue={duration > 0 ? duration : 1}
          step={1}
          isDisabled={duration <= 0}
          formatOptions={{ style: "unit", unit: "second", unitDisplay: "long" }}
          onChange={(value) => {
            if (audioRef.current !== null) audioRef.current.currentTime = value;
          }}
          className={slots.slider()}
        >
          <SliderTrack className={slots.track()}>
            {bars.map(({ id, height }, index) => (
              <i
                key={id}
                aria-hidden="true"
                className={`${slots.bar()} ${height} ${current > 0 && (index + 1) / bars.length > fraction ? "opacity-40" : ""}`}
              />
            ))}
            <SliderThumb className={slots.thumb()} />
          </SliderTrack>
        </Slider>
        <div className={slots.details()}>
          <span lang={lang} dir="auto" className={slots.name()}>
            {name}
          </span>
          <span className={slots.time()}>{times}</span>
        </div>
      </div>
    </div>
  );
}
