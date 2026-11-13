// SPDX-License-Identifier: AGPL-3.0-only

// The design system of the web client (ADR-0025 §2–§6): its components, on React Aria Components, styled with
// Tailwind from the tokens of @my/design-tokens, and its icons. An app loads @my/design-system/styles.css once.

export { Avatar, type AvatarProps, type AvatarSize } from "./avatar";
export { Button, markedSurface, type ButtonKind, type ButtonProps, type ButtonSize } from "./button";
export { IconButton, type IconButtonProps } from "./icon-button";
export { Icon, icons, type IconName, type IconPlace } from "./icons";
export { RelativeTime, type RelativeTimeProps } from "./time";
