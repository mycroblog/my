// SPDX-License-Identifier: AGPL-3.0-only

// A picture the reader picks from their device (a profile's avatar, its header): a field of a form whose control is
// the picture as it will show, in its shape, and what the reader may do with it. `round` is an avatar at its
// preview's size, the initial of `name` standing in for none; `banner` is 3:1 across the field, a picture's glyph on
// a tone of the raised surface standing in for none, so that the empty place does not read as a field to write in.
// `square` (an icon, at an avatar preview's size) and `wordmark` (a logo, 3:1 across the field) show the whole
// picture, never cut, on the same tone, or on the dark behind pictures where it is drawn for dark backgrounds (`dark`).
// Its one button opens the system's picker for one image, and says so in the caller's words (choose one,
// or replace the one shown); remove stands after it while there is a picture to remove. Both are drawn a timeline
// control's height, and their press targets reach the touch height on a touch screen. The picture itself is
// decoration: what the field holds is told by its label and by the buttons it offers.
//
// For assistive technology the field is a group named by its label and described by its hint and its error, so that
// the same two buttons of two pictures are told apart; the error stands right under what it is about.

import { color } from "@my/design-tokens";
import { useRef } from "react";
import { tv } from "tailwind-variants/lite";
import { Avatar } from "./avatar";
import { Button } from "./button";
import { describedBy } from "./field";
import { FieldError } from "./form-page";
import { Icon } from "./icons";
import { Box, Row, Stack } from "./layout";

const picture = tv({
  slots: {
    label: "font-label",
    hint: "text-caption text-muted-text [word-break:auto-phrase]",
    // The banner's box, under the picture. Without one it is a place for a picture, not a field to write in: a tone
    // of the raised surface, a step off the section it stands on and never the page's, which the fields there have.
    banner: "rounded-overlay bg-surface-raised [[data-surface-raised]_&]:bg-surface-raised-hover",
    image: "object-cover",
    // A picture shown whole in its place (an icon, a logo), never cut.
    whole: "object-contain",
    // An icon's place, at an avatar preview's size, on the banner's tone.
    square: "size-avatar-preview rounded-overlay bg-surface-raised [[data-surface-raised]_&]:bg-surface-raised-hover",
    // The place of a picture drawn for dark backgrounds: the dark behind pictures, in either scheme.
    dark: "rounded-overlay bg-(--viewer-scrim)",
    darkSquare: "size-avatar-preview rounded-overlay bg-(--viewer-scrim)",
    // The glyph of a picture that is not there yet, in the middle of its place.
    none: "text-muted-text",
  },
});

export type PictureFieldProps = {
  /**
   * The id of the button that opens the picker, the field's control: what holds the field puts the focus on it when
   * the picture was refused, its error (`${id}-error`) coming into view with it (`focusRefusedField`).
   */
  id: string;
  label: string;
  /** How the picture shows where it is used: an avatar's circle, a header's 3:1, an icon's square, a logo's line. */
  shape: "round" | "banner" | "square" | "wordmark";
  /** `square` and `wordmark`: the picture is drawn for dark backgrounds, and shows on one. */
  dark?: boolean | undefined;
  /** What the picker offers, as an `<input type="file">` takes it: images by default. */
  accept?: string | undefined;
  /** The picture as it shows now: the one picked, else the one kept; left out for none. */
  src?: string | undefined;
  /** `round`: whose picture it is, for the initial that stands in for none. */
  name?: string | undefined;
  hint?: string | undefined;
  /** What is wrong with the picture, right under the field. */
  error?: string | undefined;
  /** The words of the button that opens the picker: choosing one, or replacing the one shown. */
  choose: string;
  /** The image the reader picked. */
  onFile: (file: File) => void;
  /** Removes the picture shown, under these words; left out while there is none to remove. */
  remove?: { label: string; onPress: () => void } | undefined;
  /** It cannot be changed now (the form is being sent). */
  isDisabled?: boolean | undefined;
};

/**
 * The theme's light surface, as `#rrggbb`: what an opaque picture drawn from a mark stands on where nothing else is
 * chosen (a home screen's icon, a link preview of a logo).
 */
export const lightSurfaceColor: string = color.surface.light;

export function PictureField({
  id,
  label,
  shape,
  src,
  name = "",
  hint,
  error,
  choose,
  onFile,
  remove,
  isDisabled = false,
  dark = false,
  accept = "image/*",
}: PictureFieldProps) {
  const s = picture();
  const input = useRef<HTMLInputElement>(null);
  const actions = (
    <Row wrap align="center" gap="screen-tabs-gap">
      <Button
        id={id}
        size="timeline"
        touch
        isDisabled={isDisabled}
        onPress={() => {
          input.current?.click();
        }}
      >
        {choose}
      </Button>
      {remove !== undefined && (
        <Button kind="quiet" size="timeline" touch isDisabled={isDisabled} onPress={remove.onPress}>
          {remove.label}
        </Button>
      )}
      <input
        ref={input}
        type="file"
        accept={accept}
        hidden
        onChange={(e) => {
          const file = e.target.files?.[0];
          // The same file picked again is a change again.
          e.target.value = "";
          if (file !== undefined) onFile(file);
        }}
      />
    </Row>
  );
  return (
    <Stack
      gap="form-page-field-gap"
      role="group"
      aria-labelledby={`${id}-label`}
      aria-describedby={describedBy(hint !== undefined && `${id}-hint`, error !== undefined && `${id}-error`)}
    >
      <span id={`${id}-label`} className={s.label()}>
        {label}
      </span>
      {hint !== undefined && (
        <p id={`${id}-hint`} className={s.hint()}>
          {hint}
        </p>
      )}
      {shape === "round" ? (
        <Row wrap align="center" gap="entry-avatar">
          <Avatar name={name} src={src} size="preview" decorative />
          {actions}
        </Row>
      ) : shape === "square" ? (
        <Row wrap align="center" gap="entry-avatar">
          <Box rigid clip className={dark ? s.darkSquare() : s.square()}>
            {src === undefined ? (
              <Row full="both" align="center" justify="center" className={s.none()}>
                <Icon name="picture" place="navigation" />
              </Row>
            ) : (
              <Box full="both" className={s.whole()} as={<img src={src} alt="" />} />
            )}
          </Box>
          {actions}
        </Row>
      ) : (
        <>
          <Box ratio={3} clip className={shape === "wordmark" && dark ? s.dark() : s.banner()}>
            {src === undefined ? (
              <Row full="both" align="center" justify="center" className={s.none()}>
                <Icon name="picture" place="navigation" />
              </Row>
            ) : (
              <Box full="both" className={shape === "wordmark" ? s.whole() : s.image()} as={<img src={src} alt="" />} />
            )}
          </Box>
          {actions}
        </>
      )}
      {error !== undefined && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </Stack>
  );
}
