// SPDX-License-Identifier: AGPL-3.0-only
// A shared component measures nothing itself (ADR-0167 §2): the design system's measurement module is refused here,
// while the design system's entry stays open.

import { Button } from "@my/design-system";
import { timeTransition } from "@my/design-system/measure"; // error: measure @my/design-system/measure

export const used = [Button, timeTransition];
