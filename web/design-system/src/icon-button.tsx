// SPDX-License-Identifier: AGPL-3.0-only

// The icon button of a post's actions (reply, boost, favourite, more): a quiet button holding an icon and an optional
// count. Its tile is a timeline control's height and hugs the glyph and the count; on a touch screen its hit area
// reaches further up and down. Zero shows no count. The count reads compact in the reader's locale ("1.2K",
// "1.3万"), from Intl.NumberFormat, so it needs no strings of its own. The name is the label and the count as shown,
// so that what the reader sees is in it (WCAG 2.5.3); when the compact count rounds, the exact one is its description.
// The visible count is hidden from assistive technology, which reads it in the name. A toggle (boost, favourite) is on by shape and the text color, with
// no hue. Inside a surface marked with `markedSurface`, its hover steps a tone down so that it still shows.

import { ButtonBase, type ButtonProps } from "./button";
import { Icon, type IconName } from "./icons";
import { useReader } from "./locale";

export type IconButtonProps = Omit<ButtonProps, "kind" | "size" | "busy" | "children" | "aria-label"> & {
  /** The icon, by its meaning. */
  icon: IconName;
  /** The action's name; the accessible name is it and the count as shown. */
  label: string;
  /** How many have done it (replies, boosts, favourites); zero or none shows no count. */
  count?: number | undefined;
  /** A toggle's state: on or off; none for an action that is no toggle. */
  pressed?: boolean | undefined;
  /** The reader's locale, for the count; the I18nProvider's by default. */
  locale?: string | undefined;
};

/** The count as the button shows it, compact in `locale`: "1.2K", "1.3万". */
export const compactCount = (n: number, locale: string) =>
  new Intl.NumberFormat(locale, { notation: "compact", maximumFractionDigits: 1 }).format(n);

export function IconButton({ icon, label, count, pressed, locale, ...rest }: IconButtonProps) {
  const { lang, direction } = useReader(locale);
  const shows = count !== undefined && count > 0;
  const shown = shows ? compactCount(count, lang) : "";
  const exact = shows ? new Intl.NumberFormat(lang).format(count) : "";
  return (
    <ButtonBase
      {...rest}
      dir={direction}
      kind="icon"
      size="icon"
      pressed={pressed}
      aria-label={shows ? `${label}, ${shown}` : label}
      aria-description={shown === exact ? undefined : exact}
    >
      <Icon name={icon} place="action" on={pressed ?? false} />
      {shows && (
        <span aria-hidden="true" lang={lang} className="tabular-nums">
          {shown}
        </span>
      )}
    </ButtonBase>
  );
}
