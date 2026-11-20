// SPDX-License-Identifier: AGPL-3.0-only
// A feature measures nothing itself (ADR-0167 §2): the measurement module of each layer is refused here, whichever of
// its parts is named and however it is imported, and the entries of the same layers stay open.

import { Button } from "@my/design-system";
import { measureWith, timeTransition } from "@my/design-system/measure"; // error: measure @my/design-system/measure
import { watchVitals } from "@my/platform/measure"; // error: measure @my/platform/measure
import { reportQuality } from "@my/rtc/measure"; // error: measure @my/rtc/measure
import { measuredSession } from "@my/app-data/measure"; // error: measure @my/app-data/measure
import type { InteractionSample } from "@my/design-system/measure"; // error: measure @my/design-system/measure

export const used = [Button, measureWith, timeTransition, watchVitals, reportQuality, measuredSession];
export type Sample = InteractionSample;

export async function later() {
  return import("@my/platform/measure"); // error: measure @my/platform/measure
}
