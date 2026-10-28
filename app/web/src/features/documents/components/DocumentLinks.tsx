// SPDX-License-Identifier: AGPL-3.0-only
// The links to the server's terms of service and privacy policy. Their pages are routes of the app (information
// pages, readable without signing in), so a link opens them in the same tab (web-client/README.md Y12).
import { Box, Cluster, Section } from "@my/ui";
import { useId } from "react";
import { useT } from "../../../lib/i18n";
import { useDocumentLinks } from "../hooks/useDocumentLinks";
import { DocumentLink } from "./DocumentLink";

type DocumentLinksProps = {
  /** The id of a heading that names the links; without it they are named "Terms and policies". */
  labelledBy?: string;
  /** Where the links are: among other content, or under the items of the navigation's menu sheet, set apart by a line. */
  placement?: "inline" | "sheet";
};

/** The links to the published documents, in a row that wraps; nothing when the server publishes none. */
export function DocumentLinks({ labelledBy, placement = "inline" }: DocumentLinksProps) {
  const t = useT();
  const links = useDocumentLinks();
  if (links.length === 0) return null;
  return (
    <Box
      as="nav"
      {...(labelledBy === undefined
        ? { "aria-label": t("client:documents.label") }
        : { "aria-labelledby": labelledBy })}
      {...(placement === "sheet" ? ({ marginTop: "3", border: "top", paddingX: "3", paddingTop: "3" } as const) : {})}
    >
      <Cluster as="ul" gap="4" rowGap="1" textSize="sm">
        {links.map((link) => (
          <li key={link.href}>
            <DocumentLink href={link.href}>{link.label}</DocumentLink>
          </li>
        ))}
      </Cluster>
    </Box>
  );
}

/** The section of the links to the published documents (settings, the information page, the landing page); nothing when the server publishes none. */
export function DocumentsSection() {
  const t = useT();
  const headingId = useId();
  const links = useDocumentLinks();
  if (links.length === 0) return null;
  return (
    <Section title={t("client:documents.label")} headingId={headingId}>
      <DocumentLinks labelledBy={headingId} />
    </Section>
  );
}
