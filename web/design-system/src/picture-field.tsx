// SPDX-License-Identifier: AGPL-3.0-only

// A picture the reader picks from their device (a profile's avatar, its header): a field of a form whose control is
// the picture as it will show, in its shape, and what the reader may do with it. `round` is an avatar at its
// preview's size, the initial of `name` standing in for none; `banner` is 3:1 across the field, the surface standing
// in for none. Its one button opens the system's picker for one image, and says so in the caller's words (choose one,
// or replace the one shown); remove stands after it while there is a picture to remove. The picture itself is
// decoration: what the field holds is told by its label and by the buttons it offers.
//
// For assistive technology the field is a group named by its label and described by its hint and its error, so that
// the same two buttons of two pictures are told apart; the error stands right under what it is about.

import { useRef } from "react";
import { tv } from "tailwind-variants/lite";
import { Avatar } from "./avatar";
import { Button } from "./button";
import { describedBy } from "./field";
import { FieldError } from "./form-page";
import { Box, Row, Stack } from "./layout";

const picture = tv({
  slots: {
    label: "font-label",
    hint: "text-caption text-muted-text [word-break:auto-phrase]",
    // The banner's box: the surface a field has, the page's on a raised section, under the picture.
    banner: "rounded-overlay bg-surface-raised [[data-surface-raised]_&]:bg-surface",
    image: "object-cover",
  },
});

export type PictureFieldProps = {
  id: string;
  label: string;
  /** How the picture shows where it is used: an avatar's circle, or a header's 3:1. */
  shape: "round" | "banner";
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
}: PictureFieldProps) {
  const s = picture();
  const input = useRef<HTMLInputElement>(null);
  const actions = (
    <Row wrap align="center" gap="screen-tabs-gap">
      <Button
        size="timeline"
        isDisabled={isDisabled}
        onPress={() => {
          input.current?.click();
        }}
      >
        {choose}
      </Button>
      {remove !== undefined && (
        <Button kind="quiet" size="timeline" isDisabled={isDisabled} onPress={remove.onPress}>
          {remove.label}
        </Button>
      )}
      <input
        ref={input}
        type="file"
        accept="image/*"
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
      ) : (
        <>
          <Box ratio={3} clip className={s.banner()}>
            {src !== undefined && <Box full="both" className={s.image()} as={<img src={src} alt="" />} />}
          </Box>
          {actions}
        </>
      )}
      {error !== undefined && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </Stack>
  );
}
