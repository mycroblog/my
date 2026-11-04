// SPDX-License-Identifier: AGPL-3.0-only
// Vite config of the kit's own tooling: Storybook (@storybook/react-vite loads this file) and the
// story tests (vitest.config.ts extends it). Components compile with the same React Compiler as in app/web,
// oxc-transform-react, through @vitejs/plugin-react's `compiler` option, and Tailwind (ADR-0025). The gate of
// web-client/README.md R4 is app/web's build, which compiles these components too.
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react({ compiler: true }), tailwindcss()],
});
