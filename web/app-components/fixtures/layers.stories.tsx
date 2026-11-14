// SPDX-License-Identifier: AGPL-3.0-only
// A story, and a sample module beside it (`*.samples.ts`), may also import the fake server of @my/e2e, which exists for
// the stories, and no other part of @my/e2e or layer above its own.

import { Button } from "@my/design-system";
import { serverHtml } from "@my/e2e/fake";
import { scenario } from "@my/e2e"; // error: layers @my/e2e
import { HomeTimeline } from "@my/app-features"; // error: layers @my/app-features

export const used = [Button, serverHtml, scenario, HomeTimeline];
