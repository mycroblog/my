// SPDX-License-Identifier: AGPL-3.0-only
// The shared components import the design system and the tokens (ADR-0025 §3): app-data beside them and the layers
// above them are refused, and so is a package outside the layers, the fake server of the stories too outside a story.

import { Button } from "@my/design-system";
import { color } from "@my/design-tokens";
import { useHomeTimeline } from "@my/app-data"; // error: layers @my/app-data
import { HomeTimeline } from "@my/app-features"; // error: layers @my/app-features
import { keymap } from "@my/storybook/.storybook/preview"; // error: layers @my/storybook
import { serverHtml } from "@my/e2e/fake"; // error: layers @my/e2e

export const used = [Button, color, useHomeTimeline, HomeTimeline, keymap, serverHtml];
