// SPDX-License-Identifier: AGPL-3.0-only
import { ServerNameLink as ServerNameLinkView } from "@my/ui";
import { useLocation } from "@tanstack/react-router";
import { useT } from "../../lib/i18n";
import { useInstanceName } from "../../lib/instanceName";

export type ServerNameLinkProps = {
  /** `lg` where the name heads a column (the sidebar, the third column); `base` in the menu's sheet. */
  size?: "base" | "lg";
};

/**
 * The server's name as a link to the root address: the landing page for people who are not signed in, the home
 * timeline for signed-in users (routes/index.tsx), the same link for both. It is named by the name it shows and
 * described as the server's top page; on the landing page itself it has aria-current="page". The name is the
 * administrator's text, in any language (web-client/README.md Y10), so the paragraph around it takes its direction.
 */
export function ServerNameLink({ size }: ServerNameLinkProps) {
  const t = useT();
  const instanceName = useInstanceName();
  const atRoot = useLocation({ select: (location) => location.pathname === "/" });
  return (
    <ServerNameLinkView
      name={instanceName}
      description={t("client:shell.nav.landing")}
      current={atRoot}
      {...(size !== undefined && { size })}
    />
  );
}
