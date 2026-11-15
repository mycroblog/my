// SPDX-License-Identifier: AGPL-3.0-only

// The app's shell (ADR-0025 §3, §4, ADR-0110 §11): what goes in each region of the design system's frame. The places
// are one list in one order (places.ts), by the timelines the instance offers and the reader's rights, with the unread
// notifications and what awaits the moderators on their places. The rail and the sidebar hold the mark (the instance's
// logo when it has one), the primary action under it (a new post, or signing in without a session), the whole list,
// Menu with what is around the reader (announcements, voice rooms, events, funding) and the server's pages, and the
// bar of the call the reader is in at their end; the tab bar on a phone holds its first part and Menu, which holds the
// rest too, marked while something waits inside, with the call's bar above it. On a phone a new post
// floats over the tab bar on the timelines and the profiles alone, and signing in, with its label, on every screen. The
// server's notices stand at the top of the main column, the update's toast in the dock; the column beside the main one
// holds what is around the reader. Where the reader is comes from the router; the frame lays the regions out by the
// viewport.

import wordmarkDark from "../../../brand/Wordmark-DarkBg.svg";
import wordmarkLight from "../../../brand/Wordmark-Primary-Color.svg";
import { useInstance, useModerationCount, useRights, useUnreadNotifications, type Instance } from "@my/app-data";
import { ComposeLayer, leaveCall, muteCall, PostActions, useCall, type Call } from "@my/app-features";
import {
  CallBar,
  Frame,
  FrameAction,
  FrameDock,
  FrameMain,
  FrameMark,
  FrameSide,
  NavItem,
  NavList,
  NavMenu,
  useAction,
} from "@my/design-system";
import type { ReactNode } from "react";
import { useStrings } from "../locale";
import { signInPath } from "../sign-in";
import { Around, ServerFooter } from "./around";
import { useCompose } from "./compose";
import { ServerBanners, UpdateToast } from "./notices";
import { aroundPlacesFor, composesAt, placesFor, type Place } from "./places";

/** A place whose badge counts the reader's unread notifications. */
function NotificationsPlace({ place, current }: { place: Place; current: boolean }) {
  const t = useStrings();
  const count = useUnreadNotifications();
  return (
    <NavItem
      href={place.href}
      icon={place.icon}
      label={t(place.label)}
      current={current}
      count={count}
      countLabel={count === undefined ? undefined : t("app.notifications_count", { count })}
    />
  );
}

/** A place whose badge counts what awaits the reader as a moderator. */
function ModerationPlace({ place, current }: { place: Place; current: boolean }) {
  const t = useStrings();
  const count = useModerationCount();
  return (
    <NavItem
      href={place.href}
      icon={place.icon}
      label={t(place.label)}
      current={current}
      count={count}
      countLabel={count === undefined ? undefined : t("app.moderation_count", { count })}
    />
  );
}

/** The places of a list, the one at `pathname` current. */
function Places({ items, pathname }: { items: readonly Place[]; pathname: string }) {
  const t = useStrings();
  return items.map((place) => {
    const current = place.href === pathname;
    if (place.count === "notifications") return <NotificationsPlace key={place.href} place={place} current={current} />;
    if (place.count === "moderation") return <ModerationPlace key={place.href} place={place} current={current} />;
    return <NavItem key={place.href} href={place.href} icon={place.icon} label={t(place.label)} current={current} />;
  });
}

/** A list of places, or nothing when it has none. */
function PlaceList({ items, pathname }: { items: readonly Place[]; pathname: string }) {
  if (items.length === 0) return null;
  return (
    <NavList>
      <Places items={items} pathname={pathname} />
    </NavList>
  );
}

/** The bar of the call the reader is in: the room and who hosts it, the microphone, and leaving. */
function CallSlot({ call }: { call: Call }) {
  const t = useStrings();
  return (
    <CallBar
      href={`/voice/${call.roomId}`}
      room={call.room}
      detail={t("app.room_caption", { host: call.host, count: call.participants })}
      muted={call.muted}
      muteLabel={t("app.call_mute")}
      onMute={() => {
        muteCall(!call.muted);
      }}
      leaveLabel={t("app.call_leave")}
      onLeave={leaveCall}
    />
  );
}

/** The mark of the rail: the instance's own logo, else the built-in wordmark, named by the instance. */
function Mark({ instance, serverName }: { instance: Instance | undefined; serverName: string }) {
  return (
    <FrameMark
      href="/"
      label={instance?.name ?? serverName}
      light={wordmarkLight}
      dark={wordmarkDark}
      logo={instance?.logo?.svg_url}
    />
  );
}

/**
 * The shell around the screen at `pathname`, and the composer over it, which the primary action opens (a screen of its
 * own on a phone, a dialog from `tablet`), as a post's Reply and Quote do on every screen (PostActions); without a
 * session the primary action leads to the sign-in. `serverName` names the server until the instance answered.
 */
export function Shell({
  pathname,
  signedIn,
  serverName,
  children,
}: {
  pathname: string;
  signedIn: boolean;
  serverName: string;
  children: ReactNode;
}) {
  const t = useStrings();
  const instance = useInstance();
  const rights = useRights(signedIn);
  const call = useCall();
  const { target, open, close } = useCompose();
  const openCompose = () => {
    open({ kind: "new" });
  };
  const signIn = () => {
    location.assign(signInPath);
  };
  const compose = useAction("compose.open", signedIn ? openCompose : signIn, {
    scope: "screen",
    label: t("app.compose"),
  });
  const primary = signedIn ? (
    <FrameAction icon="compose" label={t("app.compose")} onPress={openCompose} {...compose} />
  ) : (
    <FrameAction icon="signIn" label={t("app.sign_in_action")} onPress={signIn} />
  );
  const { tabs, own, rights: staff } = placesFor(signedIn, instance, rights);
  const around = aroundPlacesFor(signedIn, instance, signInPath);
  const waiting = useModerationCount(signedIn) ?? 0;
  const callBar = call === null ? undefined : <CallSlot call={call} />;
  const footer = <ServerFooter instance={instance} />;
  return (
    <Frame>
      <FrameSide label={t("app.navigation")}>
        <Mark instance={instance} serverName={serverName} />
        {primary}
        <PlaceList items={[...tabs, ...own]} pathname={pathname} />
        <PlaceList items={staff} pathname={pathname} />
        {/* The rail and the sidebar hold the reader's and the staff's places themselves; their menu holds the rest. */}
        <NavList>
          <NavMenu label={t("app.menu")} closeLabel={t("app.menu_close")} footer={footer}>
            <PlaceList items={around} pathname={pathname} />
          </NavMenu>
        </NavList>
        {callBar}
      </FrameSide>
      <FrameMain>
        <ServerBanners instance={instance} />
        {signedIn ? <PostActions onCompose={open}>{children}</PostActions> : children}
      </FrameMain>
      <Around signedIn={signedIn} pathname={pathname} instance={instance} />
      <FrameDock
        label={t("app.navigation")}
        tabs={
          <NavList>
            <Places items={tabs} pathname={pathname} />
            <NavMenu
              label={t("app.menu")}
              closeLabel={t("app.menu_close")}
              marked={waiting > 0}
              markLabel={waiting > 0 ? t("app.menu_marked", { count: waiting }) : undefined}
              footer={footer}
            >
              <PlaceList items={own} pathname={pathname} />
              <PlaceList items={staff} pathname={pathname} />
              <PlaceList items={around} pathname={pathname} />
            </NavMenu>
          </NavList>
        }
        call={callBar}
        action={
          signedIn ? (
            composesAt(pathname) ? (
              primary
            ) : undefined
          ) : (
            <FrameAction icon="signIn" label={t("app.sign_in_action")} labelled onPress={signIn} />
          )
        }
        toasts={<UpdateToast inCall={call !== null} />}
      />
      {signedIn && <ComposeLayer target={target} onOpen={open} onClose={close} phone="screen" />}
    </Frame>
  );
}
