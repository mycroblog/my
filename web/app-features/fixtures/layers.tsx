// SPDX-License-Identifier: AGPL-3.0-only
// A feature imports the shared components, the data and what is under them (ADR-0025 §3), each by its name: the app
// above it is refused, and so is a layer below it reached through a relative path.

import { Column } from "@my/app-components";
import { useHomeTimeline } from "@my/app-data";
import { Button } from "@my/design-system";
import { color } from "@my/design-tokens";
import { AppRoot } from "@my/app"; // error: layers @my/app
import { openSession } from "../../app-data/src/session"; // error: layers @my/app-data
import { keymap } from "../../app/src/keymap"; // error: layers @my/app
import { stringsIn } from "../src/locale";

export const used = [Column, useHomeTimeline, Button, color, AppRoot, openSession, keymap, stringsIn];
