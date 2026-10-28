// SPDX-License-Identifier: AGPL-3.0-only
import {
  BookmarkIcon,
  FederatedIcon,
  HomeIcon,
  LocalIcon,
  NotificationsIcon,
  ProfileIcon,
  SearchIcon,
  SettingsIcon,
} from "@my/ui";
import type { ReactNode } from "react";
import type { NavigationEntryId } from "../hooks/useNavigationSettings";

/** The icon of each item that the user arranges, the same in the navigation and in its settings. */
export const navigationEntryIcons: Record<NavigationEntryId, ReactNode> = {
  home: <HomeIcon />,
  local: <LocalIcon />,
  public: <FederatedIcon />,
  notifications: <NotificationsIcon />,
  search: <SearchIcon />,
  profile: <ProfileIcon />,
  bookmarks: <BookmarkIcon />,
  settings: <SettingsIcon />,
};
