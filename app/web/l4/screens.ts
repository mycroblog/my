// SPDX-License-Identifier: AGPL-3.0-only
// What each screen's L4 tasks need beyond the web E2E fixtures of e2e/server.ts (the admin alice, signed in): the
// address the agent starts at, the data the tasks act on, and the tasks the fixtures cannot set up, with the reason.
// A screen without an entry starts at `/` with nothing more. Add an entry with a screen's L1 file when its tasks need
// more, and keep the starts in line with the "入口" of the L1.
import { randomUUID } from "node:crypto";
import { DatabaseSync } from "node:sqlite";

/** What a screen's setup can reach: `bin/my` and the database of the running server. */
type SetupContext = {
  /** Runs `bin/my` with `args` against the server's database and returns its standard output. */
  runMy: (args: readonly string[], input?: string) => string;
  /** The SQLite database of the server. */
  database: string;
};

export type ScreenSetup = {
  /** The path the browser opens before the agent starts. */
  start: string;
  /** Writes the data the tasks need; it runs once per run, before the first task of the screen. */
  seed?: (context: SetupContext) => void;
  /** The tasks (by their text in the L1) that the fixtures cannot set up, and why; they are recorded, not measured. */
  unavailable?: Readonly<Record<string, string>>;
};

/**
 * Gives alice two roles with badges: `admin`, which she holds and shows on her profile, and a role `l4-helper`
 * granted to her, which she does not show yet. The badge tables have no command of `bin/my`, so the rows are written
 * directly (data-model/04-tables/01-identity.md §4.1).
 */
function seedBadges({ runMy, database }: SetupContext): void {
  const db = new DatabaseSync(database);
  const now = Date.now();
  const helperRoleID = randomUUID();
  db.prepare("INSERT INTO roles (id, key, position, created_at) VALUES (?, 'l4-helper', 50, ?)").run(helperRoleID, now);
  db.close();
  runMy(["admin", "role", "grant", "alice", "l4-helper"]);
  const seed = new DatabaseSync(database);
  const adminRole = seed.prepare("SELECT id FROM roles WHERE key = 'admin'").get() as { id: string } | undefined;
  const alice = seed.prepare("SELECT user_id FROM local_accounts WHERE username_normalized = 'alice'").get() as
    { user_id: string } | undefined;
  if (adminRole === undefined || alice === undefined) {
    seed.close();
    throw new Error("seedBadges: the admin role or alice is missing from the E2E fixtures");
  }
  const badge = seed.prepare("INSERT INTO role_badges (role_id, label, color, updated_at) VALUES (?, ?, ?, ?)");
  badge.run(adminRole.id, "Admin", "red", now);
  badge.run(helperRoleID, "Helper", "teal", now);
  seed
    .prepare("INSERT INTO user_role_badge_displays (user_id, role_id, created_at) VALUES (?, ?, ?)")
    .run(alice.user_id, adminRole.id, now);
  seed.close();
}

const screenSetups: Readonly<Record<string, ScreenSetup>> = {
  badges: {
    start: "/settings",
    seed: seedBadges,
    unavailable: {
      "サポーターの一覧に載る（加入者）":
        "the fixtures have no supporter plan subscription, which needs a payment provider",
    },
  },
  "settings-menu": { start: "/settings" },
  // The staff overviews start from the app, whose navigation has the entries into the mode.
  moderation: { start: "/home" },
  admin: { start: "/home" },
  "moderation-reports": {
    start: "/moderation",
    unavailable: {
      いちばん長く待っている未解決の通報を開く:
        "the E2E fixtures have no report, and bin/my has no command that files one",
      違法な内容の通報を開く: "the E2E fixtures have no report, and bin/my has no command that files one",
      解決済みの通報を見る: "the E2E fixtures have no report, and bin/my has no command that files one",
    },
  },
  "moderation-report": {
    start: "/moderation/reports",
    unavailable: {
      担当になる: "the E2E fixtures have no report, and bin/my has no command that files one",
      "警告して解決する（処分の種類を選び、処分を出す）":
        "the E2E fixtures have no report, and bin/my has no command that files one",
      "処分せずに解決する（対応不要）": "the E2E fixtures have no report, and bin/my has no command that files one",
      ほかの画面で処分済みとして解決する: "the E2E fixtures have no report, and bin/my has no command that files one",
      メモを残す: "the E2E fixtures have no report, and bin/my has no command that files one",
      解決済みの通報を再開する: "the E2E fixtures have no report, and bin/my has no command that files one",
    },
  },
};

/** The setup of `screen`: its entry, or a start at `/` with nothing more. */
export function setupOf(screen: string): ScreenSetup {
  return screenSetups[screen] ?? { start: "/" };
}
