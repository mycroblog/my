// SPDX-License-Identifier: AGPL-3.0-only

import { devtools } from "@my/devtools/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// The catalog page runs on the dev server only (`scripts/task.sh web:catalog`). Its production build exists for
// web:devtools-absent, which proves that the build leaves @my/devtools out.
export default defineConfig({
  plugins: [devtools(), react()],
});
