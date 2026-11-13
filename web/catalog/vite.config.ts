// SPDX-License-Identifier: AGPL-3.0-only

import { devtools } from "@my/devtools/vite";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// The catalog page runs on the dev server only (`scripts/task.sh web:catalog`). Its production build exists for
// web:devtools-absent, which proves that the build leaves @my/devtools out. Tailwind builds the stylesheet of
// @my/design-system without its own optimizer, and the build targets the browsers of light-dark() (ADR-0110 §14):
// older targets would lower it to the system's scheme alone and lose an element's color-scheme.
export default defineConfig({
  plugins: [devtools(), react(), tailwindcss({ optimize: false })],
  build: { cssTarget: ["chrome123", "firefox120", "safari17.5"] },
});
