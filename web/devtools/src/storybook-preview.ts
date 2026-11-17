// SPDX-License-Identifier: AGPL-3.0-only

// The preview side of the Storybook addon (storybook.ts): the overlay over the story, and a decorator that tells it
// the story shown, with its args and the toolbar's globals, whenever Storybook renders one, so that a note names the
// story that reproduces it.

import type { Decorator } from "@storybook/react-vite";
import "./overlay";
import { showStory } from "./story";

/** The globals of the host's toolbar, without those of Storybook's own tools (measure, outline, backgrounds). */
const toolbar = ["theme", "locale", "viewport"];

const story: Decorator = (Story, { id, title, name, args, globals }) => {
  const shown = Object.fromEntries(Object.entries(globals).filter(([key]) => toolbar.includes(key)));
  showStory(window, { id, title, name, args, globals: shown });
  return Story();
};

export const decorators = [story];
