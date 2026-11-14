// SPDX-License-Identifier: AGPL-3.0-only

// The app's shell (ADR-0025 §3, §4, ADR-0110 §11): what goes in each region of the design system's frame. The rail
// and the sidebar hold the mark, the primary action (a new post) under it and the places; the tab bar holds the same
// places on a phone, under the primary action floating over its end; the column beside the main one holds what is
// around the reader. Where the reader is comes from the router; the frame lays the regions out by the viewport.

import wordmarkDark from "../../../brand/Wordmark-DarkBg.svg";
import wordmarkLight from "../../../brand/Wordmark-Primary-Color.svg";
import {
  Frame,
  FrameAction,
  FrameFloat,
  FrameMain,
  FrameMark,
  FrameSide,
  FrameTabBar,
  NavItem,
  NavList,
  useAction,
  type IconName,
} from "@my/design-system";
import { ComposeLayer, PostActions } from "@my/app-features";
import type { ReactNode } from "react";
import type { RuntimeConfig } from "../config";
import { useStrings } from "../locale";
import { Around } from "./around";
import { signInPath } from "../sign-in";
import { useCompose } from "./compose";

/** The places of the navigation, in its order: the same list in the rail, the sidebar and the tab bar. */
export const places = [
  { href: "/", icon: "home", label: "app.home" },
  { href: "/notifications", icon: "notifications", label: "app.notifications" },
  { href: "/search", icon: "search", label: "app.search" },
  { href: "/profile", icon: "profile", label: "app.profile" },
  { href: "/settings", icon: "settings", label: "app.settings" },
] as const satisfies readonly { href: string; icon: IconName; label: string }[];

function Places({ pathname }: { pathname: string }) {
  const t = useStrings();
  return (
    <NavList>
      {places.map((place) => (
        <NavItem
          key={place.href}
          href={place.href}
          icon={place.icon}
          label={t(place.label)}
          current={place.href === pathname}
        />
      ))}
    </NavList>
  );
}

/**
 * The shell around the screen at `pathname`, and the composer over it, which the primary action opens (a screen of its
 * own on a phone, a dialog from `tablet`), as a post's Reply and Quote do on every screen (PostActions); without a
 * session the primary action leads to the sign-in.
 */
export function Shell({
  pathname,
  config,
  signedIn,
  children,
}: {
  pathname: string;
  config: RuntimeConfig;
  signedIn: boolean;
  children: ReactNode;
}) {
  const t = useStrings();
  const { target, open, close } = useCompose();
  const openCompose = () => {
    if (signedIn) open({ kind: "new" });
    else location.assign(signInPath);
  };
  const compose = useAction("compose.open", openCompose, { scope: "screen", label: t("app.compose") });
  const action = <FrameAction icon="compose" label={t("app.compose")} onPress={openCompose} {...compose} />;
  return (
    <Frame>
      <FrameSide label={t("app.navigation")}>
        <FrameMark href="/" label="mycroblog" light={wordmarkLight} dark={wordmarkDark} />
        {action}
        <Places pathname={pathname} />
      </FrameSide>
      <FrameMain>{signedIn ? <PostActions onCompose={open}>{children}</PostActions> : children}</FrameMain>
      <Around config={config} />
      <FrameTabBar label={t("app.navigation")}>
        <Places pathname={pathname} />
      </FrameTabBar>
      <FrameFloat>{action}</FrameFloat>
      {signedIn && <ComposeLayer target={target} onOpen={open} onClose={close} phone="screen" />}
    </Frame>
  );
}
