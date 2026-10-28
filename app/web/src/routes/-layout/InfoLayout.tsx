// SPDX-License-Identifier: AGPL-3.0-only
import { InfoPage } from "@my/ui";
import type { ReactNode } from "react";
import { useT } from "../../lib/i18n";
import { useSignedOut } from "./Navigation";
import { useMediaQuery } from "./useMediaQuery";

/** The width that the column (42.5rem) and the contents in its margin (12.5rem and the gaps) need when centered. */
const besideQuery = "(min-width: 72rem)";

type InfoLayoutProps = {
  /** The page's `PageHeading`. */
  heading: ReactNode;
  /** The page: sections each headed by an `h2` with an id, which make up the table of contents. */
  children: ReactNode;
};

/**
 * The reading column of an information page (about, costs, supporters). Signed out, the page stands alone under the
 * top bar and its table of contents fits in the margin; signed in, the rail and the third column leave no margin, so
 * the contents fold above the column; so they do on a narrow screen.
 */
export function InfoLayout({ heading, children }: InfoLayoutProps) {
  const t = useT();
  const signedOut = useSignedOut();
  const roomy = useMediaQuery(besideQuery);
  return (
    <InfoPage
      heading={heading}
      contentsLabel={t("client:info.contents")}
      placement={signedOut && roomy ? "beside" : "inline"}
    >
      {children}
    </InfoPage>
  );
}
