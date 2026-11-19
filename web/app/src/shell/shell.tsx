// SPDX-License-Identifier: AGPL-3.0-only

// The app's shell (ADR-0025 §3, §4, ADR-0110 §11): what goes in each region of the design system's frame. The places
// are one list in one order (places.ts), by the timelines the instance offers and the reader's rights, with the unread
// notifications, the unread direct messages and what awaits the moderators on their places. The rail and the sidebar
// hold the mark (the instance's logo when it has one), the primary action under it (a new post, or signing in without
// a session; New article under it for a reader who may write articles, who on a phone finds it first in the menu's
// sheet and in the floating new post's menu, by a long press or a right click), the whole list, Menu with what is
// around the reader (announcements, voice rooms, events, funding) and the server's pages, and the bar of the call the
// reader is in at their end; the tab bar on a phone holds its first part and Menu, which holds the rest too, counting
// the unread direct messages and marked while something waits inside, with the call's bar above it. On a phone a new post
// floats over the tab bar on the timelines and the profiles alone, and signing in, with its label, on every screen. The
// server's notices stand at the top of the main column, the update's toast and that of a setting that was not saved in
// the dock; the column beside the main one
// holds what is around the reader, and beside a screen of the settings their menu (settings.tsx), the settings staying
// the marked place. Where the reader is comes from the router; the frame lays the regions out by the
// viewport, and every menu and the emoji picker of a post and of the composer open as a sheet on a phone's
// (viewport.ts).

import wordmarkDark from "../../../brand/Wordmark-DarkBg.svg";
import wordmarkLight from "../../../brand/Wordmark-Primary-Color.svg";
import {
  useArticleAuthoring,
  useInstance,
  useModerationCount,
  useNotificationSounds,
  useRights,
  useUnreadConversations,
  useUnreadNotifications,
  type Instance,
} from "@my/app-data";
import {
  ComposeLayer,
  leaveCall,
  muteCall,
  PostActions,
  resumeCallPlayback,
  UnsavedSettings,
  useCall,
  type Call,
} from "@my/app-features";
import {
  CallBar,
  Frame,
  FrameAction,
  FrameActions,
  FrameDock,
  FrameMain,
  FrameMark,
  FrameSide,
  MenuGroup,
  MenuItem,
  MenuSheet,
  NavItem,
  NavList,
  NavMenu,
  useAction,
  useLocale,
} from "@my/design-system";
import { playSound, showAppBadge } from "@my/platform";
import { useRouter } from "@tanstack/react-router";
import { useEffect, type ReactNode } from "react";
import { useStrings } from "../locale";
import { signInPath } from "../sign-in";
import { editArticlePath, newArticlePath } from "./article";
import { Around, ServerFooter } from "./around";
import { useCompose } from "./compose";
import { ServerBanners, UpdateToast } from "./notices";
import { aroundPlacesFor, composesAt, isPlaceAt, listsPlace, placesFor, type Place } from "./places";
import { SettingsAside, settingsEntryAt } from "./settings";
import { useNarrowViewport } from "./viewport";

/** The article editor, first in the menu's sheet on a phone, where New post floats alone. */
const articlePlace: Place = { href: "/articles/new", icon: "article", label: "app.compose.article" };

/**
 * The app's badge, where the platform has one (an installed app): the reader's unread notifications, cleared at none
 * and on leaving. It draws nothing.
 */
function AppBadge() {
  const count = useUnreadNotifications();
  useEffect(() => {
    if (count === undefined) return;
    showAppBadge(count);
    return () => {
      showAppBadge(0);
    };
  }, [count]);
  return null;
}

/** Plays the app's sound; the platform says whether the page may. */
const sound = () => {
  void playSound();
};

/**
 * The notification sound (ADR-0086 §12): played when the stream brings a notification of a kind the reader lets sound
 * on this device, once for a burst. It draws nothing.
 */
function NotificationSound() {
  useNotificationSounds(sound);
  return null;
}

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

/** A place whose badge counts the reader's unread conversations of direct messages. */
function DirectPlace({ place, current }: { place: Place; current: boolean }) {
  const t = useStrings();
  const count = useUnreadConversations();
  return (
    <NavItem
      href={place.href}
      icon={place.icon}
      label={t(place.label)}
      current={current}
      count={count}
      countLabel={count === undefined ? undefined : t("app.direct_messages_count", { count })}
    />
  );
}

/**
 * Menu of the tab bar, which counts the reader's unread conversations of direct messages, a place inside it, and is
 * marked while something waits for them as a moderator; what it counts and what waits are said after its name.
 */
function TabMenu({ signedIn, footer, children }: { signedIn: boolean; footer: ReactNode; children: ReactNode }) {
  const t = useStrings();
  const { locale } = useLocale();
  const waiting = useModerationCount(signedIn) ?? 0;
  const unread = useUnreadConversations(signedIn) ?? 0;
  const said = [
    ...(unread > 0 ? [t("app.menu_direct", { count: unread })] : []),
    ...(waiting > 0 ? [t("app.menu_marked", { count: waiting })] : []),
  ];
  return (
    <NavMenu
      label={t("app.menu")}
      marked={waiting > 0}
      count={unread > 0 ? unread : undefined}
      markLabel={said.length === 0 ? undefined : new Intl.ListFormat(locale, { type: "conjunction" }).format(said)}
      footer={footer}
    >
      {children}
    </NavMenu>
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
    const current = isPlaceAt(place, pathname);
    if (place.count === "notifications") return <NotificationsPlace key={place.href} place={place} current={current} />;
    if (place.count === "direct") return <DirectPlace key={place.href} place={place} current={current} />;
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

/**
 * The bar of the call the reader is in: the room and who hosts it (or that it connects, or that its audio waits for a
 * gesture), the microphone, and leaving. While the browser blocks the call's audio, the reader's next tap or key
 * anywhere on the page lets it play.
 */
function CallSlot({ call }: { call: Call }) {
  const t = useStrings();
  const blocked = call.playbackBlocked === true;
  useEffect(() => {
    if (!blocked) return undefined;
    const resume = () => {
      resumeCallPlayback();
    };
    document.addEventListener("pointerdown", resume, true);
    document.addEventListener("keydown", resume, true);
    return () => {
      document.removeEventListener("pointerdown", resume, true);
      document.removeEventListener("keydown", resume, true);
    };
  }, [blocked]);
  const detail = blocked
    ? t("app.call_playback_blocked")
    : call.phase === "connecting"
      ? t("app.call_connecting")
      : call.phase === "reconnecting"
        ? t("app.call_reconnecting")
        : t("app.room_caption", { host: call.host, count: call.participants });
  return (
    <CallBar
      href={`/voice/${call.roomId}`}
      room={call.room}
      detail={detail}
      muted={call.muted}
      micLabel={t(call.muted ? "app.call_unmute" : "app.call_mute")}
      onMic={() => {
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
  const narrow = useNarrowViewport();
  const router = useRouter();
  const writesArticles = useArticleAuthoring(signedIn) === true;
  const openCompose = () => {
    open({ kind: "new" });
  };
  const go = (href: string) => {
    void router.navigate({ href });
  };
  const signIn = () => {
    location.assign(signInPath);
  };
  const compose = useAction("compose.open", signedIn ? openCompose : signIn, {
    scope: "screen",
    label: t("app.compose"),
  });
  // What the primary action writes, in its menu: a post, and an article, for a reader the server lets write them.
  const composeMenu = writesArticles
    ? {
        label: t("app.compose.menu"),
        children: (
          <MenuGroup>
            <MenuItem id="post" icon="compose" onAction={openCompose}>
              {t("app.compose")}
            </MenuItem>
            <MenuItem
              id="article"
              icon="article"
              onAction={() => {
                go(newArticlePath());
              }}
            >
              {t("app.compose.article")}
            </MenuItem>
          </MenuGroup>
        ),
      }
    : undefined;
  const newPost = (menu: typeof composeMenu) => (
    <FrameAction icon="compose" label={t("app.compose")} onPress={openCompose} menu={menu} {...compose} />
  );
  // In the rail and the sidebar, New article stands under New post, for a reader the server lets write articles.
  const primary = !signedIn ? (
    <FrameAction icon="signIn" label={t("app.sign_in_action")} onPress={signIn} />
  ) : writesArticles ? (
    <FrameActions>
      {newPost(undefined)}
      <FrameAction
        icon="article"
        label={t("app.compose.article")}
        secondary
        onPress={() => {
          go(newArticlePath());
        }}
      />
    </FrameActions>
  ) : (
    newPost(undefined)
  );
  const { tabs, side, own, rights: staff } = placesFor(signedIn, instance, rights);
  const around = aroundPlacesFor(signedIn, instance, signInPath);
  const callBar = call === null ? undefined : <CallSlot call={call} />;
  const footer = <ServerFooter instance={instance} />;
  return (
    <MenuSheet sheet={narrow}>
      {signedIn && <AppBadge />}
      {signedIn && <NotificationSound />}
      <Frame>
        <FrameSide label={t("app.navigation")}>
          <Mark instance={instance} serverName={serverName} />
          {primary}
          <PlaceList items={side} pathname={pathname} />
          <PlaceList items={staff} pathname={pathname} />
          {/* The rail and the sidebar hold the reader's and the staff's places themselves; their menu holds the rest. */}
          <NavList>
            <NavMenu label={t("app.menu")} footer={footer}>
              <PlaceList items={signedIn ? [listsPlace] : []} pathname={pathname} />
              <PlaceList items={around} pathname={pathname} />
            </NavMenu>
          </NavList>
          {callBar}
        </FrameSide>
        <FrameMain>
          <ServerBanners instance={instance} />
          {/* A reader without an account copies, shares and opens posts too; the rest leads them to sign in. */}
          <PostActions
            onCompose={signedIn ? open : undefined}
            onEditArticle={(id) => {
              go(editArticlePath(id));
            }}
            sheet={narrow}
          >
            {children}
          </PostActions>
        </FrameMain>
        {signedIn && settingsEntryAt(pathname) !== undefined ? (
          <SettingsAside pathname={pathname} instance={instance} />
        ) : (
          <Around signedIn={signedIn} pathname={pathname} instance={instance} />
        )}
        <FrameDock
          label={t("app.navigation")}
          tabs={
            <NavList>
              <Places items={tabs} pathname={pathname} />
              <TabMenu signedIn={signedIn} footer={footer}>
                <PlaceList items={writesArticles ? [articlePlace] : []} pathname={pathname} />
                <PlaceList items={signedIn ? [...own, listsPlace] : own} pathname={pathname} />
                <PlaceList items={staff} pathname={pathname} />
                <PlaceList items={around} pathname={pathname} />
              </TabMenu>
            </NavList>
          }
          call={callBar}
          action={
            signedIn ? (
              composesAt(pathname) ? (
                // On a phone, New post floats alone; its menu (New post, New article) and the menu's sheet hold New article.
                newPost(composeMenu)
              ) : undefined
            ) : (
              <FrameAction icon="signIn" label={t("app.sign_in_action")} labelled onPress={signIn} />
            )
          }
          toasts={
            <>
              {signedIn && <UnsavedSettings onOpen={go} />}
              <UpdateToast inCall={call !== null} />
            </>
          }
        />
        {signedIn && (
          <ComposeLayer
            target={target}
            onOpen={open}
            onClose={close}
            onOpenArticle={(id) => {
              go(newArticlePath(id));
            }}
            phone="screen"
            pickerSheet={narrow}
          />
        )}
      </Frame>
    </MenuSheet>
  );
}
