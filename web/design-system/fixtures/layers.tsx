// SPDX-License-Identifier: AGPL-3.0-only
// The design system imports only the tokens (ADR-0025 §3): every layer above it, by name, by a re-export, by a dynamic
// import or by a relative path, is refused; web/brand, which holds files, is open.

import { color } from "@my/design-tokens";
import { Column } from "@my/app-components"; // error: layers @my/app-components
import { useHomeTimeline } from "@my/app-data"; // error: layers @my/app-data
import { catalogReader } from "../../app-components/src/locale"; // error: layers @my/app-components
import wordmark from "../../brand/Wordmark-Primary-Color.svg";

export { HomeTimeline } from "@my/app-features"; // error: layers @my/app-features
export * from "@my/i18n"; // error: layers @my/i18n

export const app = () => import("@my/app"); // error: layers @my/app

export const used = [color, Column, useHomeTimeline, catalogReader, wordmark];
