// SPDX-License-Identifier: AGPL-3.0-only
// What each screen's L4 tasks need on the demo server (`node e2e/server.ts demo`, the scenario of demo/scenario.ts):
// the address the agent starts at, the person of the scenario it is signed in as (or nobody), the changes to the
// scenario that the tasks need, the tasks that read email in Mailpit, and the tasks the demo server cannot set up,
// with the reason. A screen without an entry starts at `/home` as the user yuki. Add an entry with a screen's L1 file
// when its tasks need more, and keep the starts in line with the "入口" of the L1; screens.test.ts checks that every
// task named here is a row of the L1.

/** What a screen's setup can reach on the demo server. */
export type SetupContext = {
  /** Runs `bin/my` with `args` against the demo server's database and returns its standard output. */
  runMy: (args: readonly string[], input?: string) => string;
  /** Calls the client API (`/api/v1` + `path`) as the person `username` of the scenario; a failure throws. */
  api: (
    username: string,
    method: "GET" | "PUT" | "PATCH" | "POST" | "DELETE",
    path: string,
    body?: unknown,
  ) => Promise<void>;
};

export type ScreenSetup = {
  /** The path the browser opens before the agent starts. */
  start: string;
  /** The person of the scenario the tasks begin signed in as, by username; null begins signed out. */
  persona: string | null;
  /** The paths of the tasks (by their text in the L1) that start elsewhere than `start`. */
  starts?: Readonly<Record<string, string>>;
  /** Changes the scenario as the tasks need; it runs once per run, before the first task of the screen. */
  seed?: (context: SetupContext) => Promise<void>;
  /**
   * The tasks that read an email: the username of the person of the scenario whose mailbox the agent reads in
   * Mailpit's web UI, in a second tab. The harness removes that person's earlier email from Mailpit before the task.
   */
  mail?: Readonly<Record<string, string>>;
  /** The tasks (by their text in the L1) that the demo server cannot set up, and why; they are recorded, not measured. */
  unavailable?: Readonly<Record<string, string>>;
};

/** Why a task that chooses a file is not measured: the browser tools give the agent no file of its own to choose. */
const noFile = "the agent has no file to choose: the browser tools give it none";

/**
 * yuki holds the supporter role, shows its badge, and is on the list of supporters (demo/scenario.ts). The badge
 * tasks need a badge she holds but does not show: the role `translator`, granted without showing it; the task of
 * being listed needs her off the list.
 */
async function seedBadges({ runMy, api }: SetupContext): Promise<void> {
  runMy(["admin", "role", "grant", "yuki", "translator"]);
  await api("yuki", "PUT", "/preferences/supporter_listing", { listed: false });
}

const screenSetups: Readonly<Record<string, ScreenSetup>> = {
  "admin-settings": {
    start: "/admin",
    persona: "hana",
    unavailable: {
      "アイコンを設定する（SVG を選び、プレビューを見て保存する）": noFile,
      ロゴを設定する: noFile,
      アイコンを組み込みのものに戻す:
        "the demo scenario sets no icon of its own, and setting one needs the PNG renditions that the web client draws from a chosen file",
    },
  },
  badges: { start: "/settings", persona: "yuki", seed: seedBadges },
  landing: {
    start: "/",
    persona: null,
    mail: { "パスワードを忘れた利用者が、再設定のメールのリンクから新しいパスワードを設定する": "marco" },
  },
  "settings-menu": { start: "/settings", persona: "yuki" },
  settings: {
    start: "/settings",
    persona: "hana",
    starts: { "ナビゲーションから設定を開き、二要素認証がオンかを確かめる": "/home" },
  },
  "settings-preferences": { start: "/settings", persona: "yuki" },
  "settings-notifications": {
    start: "/settings",
    persona: "yuki",
    unavailable: {
      "この端末でプッシュ通知を受け取る（ブラウザの許可の確認は数えない）":
        "the demo server has no Web Push: it sets no VAPID keys and reaches no push service",
    },
  },
  "settings-profile": {
    start: "/settings",
    persona: "yuki",
    unavailable: { "アイコンの画像を選び直して保存する（ファイルの選択は 1 つと数える）": noFile },
  },
  "settings-auto-delete": { start: "/settings", persona: "yuki" },
  // The staff overviews start from the app, whose navigation has the entries into the mode.
  moderation: { start: "/home", persona: "marco" },
  admin: { start: "/home", persona: "hana" },
  "moderation-reports": {
    start: "/moderation",
    persona: "marco",
    unavailable: {
      違法な内容の通報を開く: "the demo scenario has no report of the legal category",
    },
  },
  "moderation-report": { start: "/moderation/reports", persona: "marco" },
};

/** The setup of `screen`: its entry, or a start at `/home` as yuki. */
export function setupOf(screen: string): ScreenSetup {
  return screenSetups[screen] ?? { start: "/home", persona: "yuki" };
}

/** The screens with an entry, for screens.test.ts. */
export const setupScreens: readonly string[] = Object.keys(screenSetups);
