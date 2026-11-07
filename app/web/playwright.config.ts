// SPDX-License-Identifier: AGPL-3.0-only
// Playwright E2E for the web client (ADR-0015, ADR-0110). Every spec runs in the primary project (en, 1440 px), the
// only one of a run without E2E_MATRIX=full; with E2E_MATRIX=full (the Nightly) the specs whose behaviour changes with
// the width or the locale also run in the projects of those (see `widthSpecs` and `localeSpecs`). The client's tests run against `vite preview` of
// the built app (`scripts/task.sh web:e2e` builds it first), or against E2E_BASE_URL when it is set. The tests of the
// server-rendered pages (e2e/auth-pages.spec.ts) and of the main flow (e2e/main-flow.spec.ts) run against a real `my serve` on SQLite that
// e2e/server.ts starts from bin/my with the pseudo-locales enabled, or against E2E_SERVER_URL when it is
// set (a server started with `my serve -pseudo-locales`). The call between two browsers (e2e/voice-p2p.spec.ts) runs
// once, in Chromium with its fake microphone, against a second server with voice chat on that e2e/server.ts starts. Both servers listen on free ports picked per
// run (E2E_PREVIEW_PORT and E2E_SERVER_PORT override them; e2e/server-origin.ts), and a run never reuses a
// server it did not start, so runs in several worktrees on one machine stay apart.
import { defineConfig, devices, type Project } from "@playwright/test";
import { primaryProject } from "./e2e/projects.ts";
import { previewOrigin, previewPort, serverPort, voiceServerPort } from "./e2e/server-origin.ts";

/**
 * The projects (ADR-0015 §1). Every spec runs in `primaryProject` (en, 1440 px): most specs are flows, whose result
 * the width and the locale do not change, and the screen stories (ADR-0079 §13) check how each screen looks in every
 * locale and at 320 px. With E2E_MATRIX=full (the Nightly), the specs below also run where their behaviour changes:
 */
const primary = { locale: "en", width: 1440 } as const;

/**
 * The specs whose behaviour changes with the width (the breakpoints md 768, lg 1024, and xl 1280, touch, the phones'
 * tab bar): in en at the widths on either side of each breakpoint. 320 px is where horizontal scrolling appears first
 * (web-client/README.md D3).
 */
const widthSpecs = [
  "admin-entry",
  "announcements",
  "bookmarks",
  "emoji-picker",
  "federated",
  "lists",
  "profile-save",
  "reactions",
  "server-settings",
  "settings",
  "shell",
  "status",
] as const;
const widths = [320, 768, 1024] as const;

/**
 * The specs whose behaviour changes with the locale: the shell's direction and navigation, the pages the server
 * renders in the instance's locale, the choice of the display language, and the main flow end to end. They run in
 * Japanese, in the pseudo-locale en-XA (long and bracketed text, at the narrowest width), and right to left in ar-XB
 * at both ends of the widths (ADR-0009, ADR-0015).
 */
const localeSpecs = ["auth-pages", "main-flow", "settings", "shell"] as const;
const localeProjects = [
  { locale: "ja", width: 1440 },
  { locale: "en-XA", width: 320 },
  { locale: "ar-XB", width: 320 },
  { locale: "ar-XB", width: 1440 },
] as const;

const fullMatrix = process.env["E2E_MATRIX"] === "full";
const ci = process.env["CI"] !== undefined;
const port = previewPort;
const externalBaseURL = process.env["E2E_BASE_URL"];
const externalServer = process.env["E2E_SERVER_URL"] !== undefined;

/** The spec of the emoji web font, which runs once per browser engine instead of in the width × locale projects. */
const emojiFontSpec = /emoji-font\.spec\.ts$/;

/** The spec of a voice chat call between two browsers, which runs once in Chromium instead of in the matrix. */
const voiceP2PSpec = /voice-p2p\.spec\.ts$/;

/** The specs of the demo server (e2e/demo-*.spec.ts), which run once in Chromium with E2E_DEMO=1. */
const demoSpec = /demo-[^/]*\.spec\.ts$/;

/**
 * Whether the run starts the demo server (`node e2e/server.ts demo`, ADR-0015) and its specs: it needs the test build
 * (`GO_TAGS=e2e_fakes scripts/task.sh go:bin`) and Mailpit (`scripts/task.sh docker:mailpit`), so only E2E_DEMO=1 asks
 * for it: the `demo-e2e` job of the Nightly workflow, and a run by hand.
 */
const demo = process.env["E2E_DEMO"] === "1" && !externalServer;

/** The specs of `names` as the testMatch of a project. */
const specs = (names: readonly string[]): RegExp[] => names.map((name) => new RegExp(`/${name}\\.spec\\.ts$`));

/** A project of the app in `locale` at `width`, named `<locale>-<width>`, which runs the specs of `testMatch`. */
function appProject(locale: string, width: number, testMatch?: RegExp[]): Project {
  // Widths below the tablet breakpoint emulate a phone: touch, a coarse pointer and the mobile viewport.
  const phone = width < 768;
  return {
    name: `${locale}-${String(width)}`,
    ...(testMatch === undefined ? {} : { testMatch }),
    testIgnore: [emojiFontSpec, voiceP2PSpec, demoSpec],
    use: {
      ...devices["Desktop Chrome"],
      viewport: { width, height: phone ? 800 : 900 },
      isMobile: phone,
      hasTouch: phone,
      locale,
    },
  };
}

const primaryApp = appProject(primary.locale, primary.width);
if (primaryApp.name !== primaryProject)
  throw new Error(`the primary project is ${primaryApp.name}, not ${primaryProject}`);

const matrix: Project[] = [
  primaryApp,
  ...(fullMatrix
    ? [
        ...widths.map((width) => appProject("en", width, specs(widthSpecs))),
        ...localeProjects.map(({ locale, width }) => appProject(locale, width, specs(localeSpecs))),
      ]
    : []),
];

// The Noto Color Emoji web font (ADR-0091 §10) picks its format by the browser's tech() support: COLRv1 in Chromium and
// Firefox, OpenType-SVG in WebKit, so its spec runs in all three engines (the pinned Playwright image has them).
const emojiFontProjects: Project[] = (["Desktop Chrome", "Desktop Firefox", "Desktop Safari"] as const).map(
  (device) => ({
    name: `emoji-font-${devices[device].defaultBrowserType}`,
    testMatch: emojiFontSpec,
    use: { ...devices[device], viewport: { width: 1440, height: 900 }, locale: "en" },
  }),
);

// Chromium's fake microphone (a tone) and its acceptance of the permission prompt let two browser contexts speak to
// each other without a device or a person (Chromium command-line switches); audio plays without a user gesture. The
// project needs the voice chat server of e2e/server.ts, so it is left out when the specs test a server started elsewhere.
const voiceP2PProject: Project = {
  name: "voice-p2p-chromium",
  testMatch: voiceP2PSpec,
  use: {
    ...devices["Desktop Chrome"],
    viewport: { width: 1440, height: 900 },
    locale: "en",
    permissions: ["microphone"],
    launchOptions: {
      args: [
        "--use-fake-device-for-media-stream",
        "--use-fake-ui-for-media-stream",
        "--autoplay-policy=no-user-gesture-required",
      ],
    },
  },
};

// The demo server needs the test build and Mailpit, so its project runs only with E2E_DEMO=1. Its specs share the one
// demo server, and the census of e2e/demo-smoke.spec.ts reads all of it, so they run one at a time.
const demoProject: Project = {
  name: "demo-chromium",
  testMatch: demoSpec,
  workers: 1,
  use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 }, locale: "en" },
};

const projects: Project[] = [
  ...matrix,
  ...emojiFontProjects,
  ...(externalServer ? [] : [voiceP2PProject]),
  ...(demo ? [demoProject] : []),
];

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: ci,
  retries: ci ? 1 : 0,
  ...(ci ? { workers: 2 } : {}),
  reporter: ci ? [["github"], ["html", { open: "never" }]] : [["list"]],
  use: {
    baseURL: externalBaseURL ?? previewOrigin,
    trace: "retain-on-failure",
    // A Service Worker answers navigations before page.route() sees them, which would bypass the locale
    // fixture (e2e/fixtures.ts). Only e2e/pwa.spec.ts, which tests the worker, allows it.
    serviceWorkers: "block",
    // Without motion nothing fades or changes its colours over a transition (the app gates motion on
    // `motion-safe:`), so axe's color-contrast never measures a dialog or a button halfway, whatever the load of the
    // machine. The specs that test the animated path ask for `reducedMotion: "no-preference"` themselves.
    reducedMotion: "reduce",
  },
  projects,
  webServer: [
    ...(externalBaseURL === undefined
      ? [
          {
            command: `bun run preview --host 127.0.0.1 --port ${String(port)} --strictPort`,
            url: `http://127.0.0.1:${String(port)}`,
            // A server already on the port belongs to someone else (another worktree's build, say), so the
            // run fails rather than test it; E2E_BASE_URL is the way to test a server this run does not start.
            reuseExistingServer: false,
          },
        ]
      : []),
    ...(externalServer
      ? []
      : [
          {
            // A fresh database every run: the specs depend on its seed and on rate limits not yet used up.
            command: "node e2e/server.ts",
            url: `http://127.0.0.1:${String(serverPort)}/readyz`,
            reuseExistingServer: false,
            gracefulShutdown: { signal: "SIGTERM", timeout: 5000 },
          } as const,
          {
            // The server of e2e/voice-p2p.spec.ts, with voice chat on (P2P) and a database of its own.
            command: "node e2e/server.ts voice",
            url: `http://127.0.0.1:${String(voiceServerPort)}/readyz`,
            reuseExistingServer: false,
            gracefulShutdown: { signal: "SIGTERM", timeout: 5000 },
          } as const,
        ]),
    ...(demo
      ? [
          {
            // The demo server with the demo scenario, ready once the seed has applied it: Playwright takes the first
            // of `url` and `wait`, so the server is waited for by its line alone.
            command: "node e2e/server.ts demo",
            wait: { stdout: /demo: ready/ },
            timeout: 180_000,
            reuseExistingServer: false,
            gracefulShutdown: { signal: "SIGTERM", timeout: 5000 },
          } as const,
        ]
      : []),
  ],
});
