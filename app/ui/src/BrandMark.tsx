// SPDX-License-Identifier: AGPL-3.0-only
import type { ComponentPropsWithRef } from "react";
import { tv, type VariantProps } from "tailwind-variants";
import type { BrandLogo } from "./BrandImage";
import { Wordmark } from "./Wordmark";

const brandMark = tv({
  slots: {
    base: "flex min-w-0 items-center gap-3",
    name: "min-w-0 truncate font-semibold text-fg",
  },
  variants: {
    size: {
      sm: { name: "text-base" },
      md: { name: "text-lg" },
      lg: { name: "text-2xl" },
    },
  },
  defaultVariants: { size: "md" },
});

export type BrandMarkProps = Omit<ComponentPropsWithRef<"div">, "children"> &
  VariantProps<typeof brandMark> & {
    /** The accessible name of the wordmark; the product name ("my"). */
    wordmarkName: string;
    /** The server's name, shown as text next to the wordmark, or the name of its own logo. */
    instanceName: string;
    /** The server's own logo, which includes its name: it replaces both the wordmark and the name's text. */
    logo?: BrandLogo | undefined;
  };

/**
 * The wordmark next to the name of the server, or the server's own logo alone, an image named by the server's name.
 * The name is user content: `dir="auto"`, and it truncates when long.
 */
export function BrandMark({ wordmarkName, instanceName, size, logo, className, ...props }: BrandMarkProps) {
  const { base, name } = brandMark({ size });
  if (logo !== undefined) {
    return (
      <div {...props} className={base({ className })}>
        <Wordmark name={instanceName} decorative={false} size={size} logo={logo} />
      </div>
    );
  }
  return (
    <div {...props} className={base({ className })}>
      <Wordmark name={wordmarkName} decorative={false} size={size} />
      <span dir="auto" title={instanceName} className={name()}>
        {instanceName}
      </span>
    </div>
  );
}
