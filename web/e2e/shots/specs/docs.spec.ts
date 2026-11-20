// SPDX-License-Identifier: AGPL-3.0-only

// The documentation site's screenshots (list.ts, shots.ts), one test a shot: what it shows is made (stage.ts), the
// person who looks at it signs in through the API, and the screen is shot in the light and the dark theme at the
// viewport of the configuration, at the time of time.ts, once its fonts and the pictures in view have loaded
// (capture.ts), and written as WebP.

import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { runMyOn } from "../../src/server/demo";
import { person, test } from "../../specs/test";
import { screenshotsDir, shotFile, shotNames, themes } from "../list";
import { shots } from "../shots";
import { Stage } from "../stage";
import { fixTime } from "../time";
import { picturesDrawn, stillShot, toWebP } from "../capture";

let stage: Stage | undefined;

for (const name of shotNames) {
  test(name, async ({ page, browser, baseURL, signedIn }) => {
    if (baseURL === undefined) throw new Error("shots: no demo server");
    const envFile = process.env["E2E_DEMO_ENV"] ?? "";
    stage ??= new Stage(baseURL, (args, input) => runMyOn(envFile, args, input));
    const shot = shots[name];
    const path = await shot.open(stage, browser);
    await stage.settled();
    await fixTime(page, new URL(baseURL).origin);
    await signedIn(page, person(shot.as));
    for (const theme of themes) {
      await page.emulateMedia({ colorScheme: theme });
      await page.goto(path);
      await shot.ready(page);
      await picturesDrawn(page);
      const png = await stillShot(page);
      writeFileSync(join(screenshotsDir, shotFile(name, theme)), await toWebP(page, png));
    }
  });
}
