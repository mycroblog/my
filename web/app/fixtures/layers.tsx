// SPDX-License-Identifier: AGPL-3.0-only
// The app imports every layer below it and @my/i18n (ADR-0025 §3); a development package is no layer of it.

import { HomeTimeline } from "@my/app-features";
import { Column } from "@my/app-components";
import { openSession } from "@my/app-data";
import { Frame } from "@my/design-system";
import { shell } from "@my/design-tokens";
import { compile } from "@my/i18n/src/compile";
import { devtools } from "@my/devtools/vite"; // error: layers @my/devtools
import wordmark from "../../brand/Wordmark-DarkBg.svg";

export const used = [HomeTimeline, Column, openSession, Frame, shell, compile, devtools, wordmark];
