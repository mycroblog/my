// SPDX-License-Identifier: AGPL-3.0-only

// The screenshots of the documentation site (web/docs/src/assets/screenshots/, which its Screenshot component shows):
// each name is a screen of the web client that `scripts/task.sh web:docs-screenshots` shoots on the demo server, in the
// light and the dark theme, as <name>-<theme>.webp. list.test.ts fails when the directory holds an image this list
// does not make, so that no screenshot is made by hand.

import { join } from "node:path";

/** The screenshots, by the name of their files. */
export const shotNames = ["home", "post", "reports", "supporters", "voice-room"] as const;

export type ShotName = (typeof shotNames)[number];

export const themes = ["light", "dark"] as const;

export type Theme = (typeof themes)[number];

/** The directory of the documentation site's screenshots. */
export const screenshotsDir = join(import.meta.dirname, "../../docs/src/assets/screenshots");

/** The file of the shot `name` in `theme`. */
export const shotFile = (name: ShotName, theme: Theme) => `${name}-${theme}.webp`;

/** Every file the shots write, in their order. */
export const shotFiles = (): string[] => shotNames.flatMap((name) => themes.map((theme) => shotFile(name, theme)));
