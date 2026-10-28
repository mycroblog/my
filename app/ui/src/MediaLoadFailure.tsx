// SPDX-License-Identifier: AGPL-3.0-only
// The loading and the failure of a post's media (ADR-0110 P2): the texts that the grid, the viewer and the audio player
// share, and the mark that a picture or a file could not load, with the button that tries again. The mark sits over
// the media's place, so the failure shows where the media were to be, not only as a blur that never clears.
import { Button } from "react-aria-components";
import { tv } from "tailwind-variants";
import { MediaFailedIcon } from "./Icons";

/** The texts of the loading and the failure of media, which the app translates. */
export type MediaLoadLabels = {
  /** The name of the progress while media load ("Loading media"). */
  loading: string;
  /** What failed ("Couldn't load this media."). */
  failed: string;
  /** The button that loads the media again ("Try again"). */
  retry: string;
};

const mediaLoadFailure = tv({
  slots: {
    base: "absolute inset-0 z-10 flex flex-col items-center justify-center gap-2 bg-surface-sunken p-2 text-center text-sm text-fg",
    icon: "text-fg-muted [&>svg]:size-6",
    retry: [
      "min-h-9 cursor-default rounded-full bg-surface-raised px-3 font-extrabold shadow-sm pointer-coarse:min-h-11",
      "data-focus-visible:focus-ring data-hovered:bg-soft motion-safe:transition-colors",
    ],
  },
});

export type MediaLoadFailureProps = {
  labels: MediaLoadLabels;
  /** Loads the media again. */
  onRetry: () => void;
  className?: string;
};

/** The mark of media that could not load, over their place, with "Try again". */
export function MediaLoadFailure({ labels, onRetry, className }: MediaLoadFailureProps) {
  const slots = mediaLoadFailure();
  return (
    <div role="group" aria-label={labels.failed} className={slots.base({ className })} data-media-failed="">
      <span aria-hidden="true" className={slots.icon()}>
        <MediaFailedIcon />
      </span>
      <p>{labels.failed}</p>
      <Button onPress={onRetry} className={slots.retry()}>
        {labels.retry}
      </Button>
    </div>
  );
}
