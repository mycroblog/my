// SPDX-License-Identifier: AGPL-3.0-only

// The reader's data in the settings' entry Account and data (FR-008, FR-009, FR-118, FR-121, ADR-0089 §2, ADR-0151
// §7, §8): the entry's doors and the four screens under it, on the fake server as yuki reads them.
//
// - The entry: its doors as the fake starts (a finished import, a maximum of the server that binds the saved policy,
//   one alias); with something going on behind each (an archive being prepared, an import in progress); and with
//   nothing behind any (no action, no archive, no import, deletion off, no alias).
// - Export: no archive yet; one being prepared; one ready; one being prepared over an earlier one that is ready; one
//   whose time has passed; one that could not be prepared; while the requests load, and when they could not be read;
//   and reached by the address the closure's emails carry.
//   Request an archive and Download act at once (the fake takes a request, and gives an empty archive and the follows
//   as a CSV file); a row says what came of its own Download (its shot is taken by a script that presses).
// - Import: the form, empty, over the fake's finished import of which three entries failed; an import in progress,
//   where another is refused; one that finished whole; one that failed whole; none; while the imports load, and when
//   they could not be read. The preview follows a file the reader chooses (its shot is taken by a script that gives
//   the field a file): what would be added, what the reader has already, the lines that cannot be read, and, last and
//   apart, what a replacement removes.
// - Delete old posts automatically: off; on; under a maximum of the server that the saved policy is within; bound by
//   the maximum, which starts to apply in two weeks; while it loads, and when it could not be read. Each counts what
//   the form as it stands would delete, right under when it deletes, and says the count beside Save. Save asks
//   before a save that deletes more than is deleted now (its shot is taken by a script that turns the deletion on and
//   presses Save).
// - Aliases: one; none; five, the most an account has, where adding gives way to a sentence; while they load, and
//   when they could not be read. Adding an address the fake finds no account at (one with no domain) is refused under
//   the field (its shot is taken by a script that types and presses).

import type { FakeOptions } from "@my/e2e/fake";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const entry = "/settings/account";
const exportAt = "/settings/account/export";
const importAt = "/settings/account/import";
const autoDeleteAt = "/settings/account/auto-delete";
const aliasesAt = "/settings/account/aliases";

type Settings = NonNullable<FakeOptions["settings"]>;
type Import = NonNullable<Settings["imports"]>[number];
type Policy = NonNullable<Settings["autoDelete"]>;

/** An import of follows that merges 120 entries. */
const follows: Import = {
  kind: "following",
  mode: "merge",
  state: "finished",
  created_at: "-2d",
  total: 120,
  processed: 120,
  failed: 0,
};

/** An import of blocked accounts in progress, a third of the way. */
const running: Import = {
  kind: "blocks",
  mode: "overwrite",
  state: "in_progress",
  created_at: "-0d",
  total: 300,
  processed: 96,
  failed: 2,
};

/** A policy that deletes posts older than three months and keeps the pinned and the bookmarked. */
const policy: Policy = {
  enabled: true,
  min_age_days: 90,
  keep_pinned: true,
  keep_bookmarked: true,
  keep_favourited: false,
  keep_media: false,
  keep_polls: false,
  keep_articles: true,
  keep_visibilities: ["direct"],
  keep_min_boosts: 0,
  keep_min_favourites: 25,
};

/** A maximum of half a year that lets pinned posts alone be kept, the three most recently pinned. */
const strict: NonNullable<Settings["autoDeleteLimit"]> = {
  max_age_days: 180,
  allow_pinned: true,
  kept_pins: 3,
  allow_bookmarked: false,
  allow_favourited: false,
  allow_media: false,
  allow_polls: false,
  allow_articles: false,
  allow_visibilities: false,
  allow_thresholds: false,
};

const meta = {
  title: "Settings account data",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: entry }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/**
 * /settings/account as the fake starts: under the account's status, the doors to Export, Import and Delete old posts
 * automatically, which says from when the server's maximum applies, and to Aliases, which says how many there are.
 */
export const Entry: Story = {};
/** The entry while an archive is being prepared and an import is in progress: each door says so. */
export const EntryBusy: Story = {
  parameters: screen({ at: entry, server: { dataExport: "building", settings: { imports: [running, follows] } } }),
};
/** The entry with nothing behind its doors: each says what it leads to, and the deletion that it is off. */
export const EntryQuiet: Story = {
  parameters: screen({
    at: entry,
    server: {
      settings: {
        moderation: [],
        imports: [],
        aliases: [],
        autoDelete: { ...policy, enabled: false },
        autoDeleteLimit: undefined,
      },
    },
  }),
};

/** /settings/account/export with no archive yet: Request an archive in the list's place, over the CSV files. */
export const Export: Story = { parameters: screen({ at: exportAt }) };
/** An archive being prepared: its row says so and that another can be requested once it is done; no request button. */
export const ExportPreparing: Story = { parameters: screen({ at: exportAt, server: { dataExport: "building" } }) };
/**
 * An archive ready: until when it can be downloaded, its size, and Download, the screen's primary action; Request an
 * archive at the head, neutral beside it.
 */
export const ExportReady: Story = { parameters: screen({ at: exportAt, server: { dataExport: "ready" } }) };
/** An archive being prepared over an earlier one that can still be downloaded: both rows. */
export const ExportRebuilding: Story = { parameters: screen({ at: exportAt, server: { dataExport: "rebuilding" } }) };
/** An archive past the day the server keeps it until: it says when it expired, and to request a new one. */
export const ExportExpired: Story = { parameters: screen({ at: exportAt, server: { dataExport: "expired" } }) };
/** An archive the server could not prepare: it says so, and to request a new one. */
export const ExportFailed: Story = { parameters: screen({ at: exportAt, server: { dataExport: "failed" } }) };
/** Export while the requests are on their way: the section says that it loads, over the CSV files. */
export const ExportLoading: Story = { parameters: screen({ at: exportAt, state: "loading" }) };
/** Export when the requests could not be read: the section says so, with the way to try again. */
export const ExportReadFailed: Story = { parameters: screen({ at: exportAt, state: "error" }) };
/** /settings/import-export, the address the closure's emails carry: it leads to Export, with an archive ready. */
export const ExportFromClosureEmail: Story = {
  parameters: screen({ at: "/settings/import-export", server: { dataExport: "ready" } }),
};

/**
 * /settings/account/import: the form, empty, the preview waiting for a file, and the fake's import of follows that
 * finished with three entries failed.
 */
export const Import: Story = { parameters: screen({ at: importAt }) };
/** An import in progress over a finished one: how far it is; Import is refused meanwhile, and the submit bar says so. */
export const ImportRunning: Story = {
  parameters: screen({ at: importAt, server: { settings: { imports: [running, follows] } } }),
};
/** An import that finished whole. */
export const ImportDone: Story = { parameters: screen({ at: importAt, server: { settings: { imports: [follows] } } }) };
/** An import none of whose entries could be carried out, over one that had nothing to do. */
export const ImportFailed: Story = {
  parameters: screen({
    at: importAt,
    server: {
      settings: {
        imports: [
          { ...follows, kind: "bookmarks", total: 14, processed: 14, failed: 14 },
          { ...follows, kind: "domain_mutes", created_at: "-5d", total: 0, processed: 0 },
        ],
      },
    },
  }),
};
/** No import in the last days: the list says so. */
export const ImportNone: Story = { parameters: screen({ at: importAt, server: { settings: { imports: [] } } }) };
/** Import while the imports are on their way: the form is there, and the list says that it loads. */
export const ImportLoading: Story = { parameters: screen({ at: importAt, state: "loading" }) };
/** Import when the imports could not be read: the list says so, with the way to try again. */
export const ImportReadFailed: Story = { parameters: screen({ at: importAt, state: "error" }) };

/** /settings/account/auto-delete with the deletion off, on a server without a maximum: nothing would be deleted. */
export const AutoDeleteOff: Story = {
  parameters: screen({
    at: autoDeleteAt,
    server: { settings: { autoDelete: { ...policy, enabled: false }, autoDeleteLimit: undefined } },
  }),
};
/**
 * The deletion on, on a server without a maximum: the count of what would go and the oldest of it, beside Save too,
 * and every exception can be chosen.
 */
export const AutoDeleteOn: Story = {
  parameters: screen({ at: autoDeleteAt, server: { settings: { autoDelete: policy, autoDeleteLimit: undefined } } }),
};
/**
 * Under a maximum of half a year that the saved policy is within: the deletion cannot be turned off, the age goes up
 * to the maximum, and only the three most recently pinned posts can be kept. The other kinds are named in words
 * under that checkbox, and the audiences and the thresholds each say in one sentence that the server does not allow
 * them: nothing the server forbids is drawn as a control.
 */
export const AutoDeleteLimited: Story = {
  parameters: screen({
    at: autoDeleteAt,
    server: {
      settings: {
        autoDelete: {
          ...policy,
          keep_bookmarked: false,
          keep_articles: false,
          keep_visibilities: [],
          keep_min_favourites: 0,
        },
        autoDeleteLimit: strict,
      },
    },
  }),
};
/**
 * Bound by the fake's maximum of a year, which does not let posts with media be kept: the summary says the maximum
 * and from when it applies, the count is of what it then deletes, and saving, which applies it at once, asks first.
 */
export const AutoDeleteBound: Story = { parameters: screen({ at: autoDeleteAt }) };
/** Delete old posts automatically while the policy is on its way. */
export const AutoDeleteLoading: Story = { parameters: screen({ at: autoDeleteAt, state: "loading" }) };
/** Delete old posts automatically when the policy could not be read. */
export const AutoDeleteFailed: Story = { parameters: screen({ at: autoDeleteAt, state: "error" }) };

/** /settings/account/aliases with one alias: its handle, when it was added, Remove, and the field that adds one. */
export const Aliases: Story = { parameters: screen({ at: aliasesAt }) };
/** No alias: the list says what to do, over the field that adds one. */
export const AliasesNone: Story = { parameters: screen({ at: aliasesAt, server: { settings: { aliases: [] } } }) };
/** Five aliases, the most an account has: adding gives way to a sentence that says so. */
export const AliasesFull: Story = {
  parameters: screen({
    at: aliasesAt,
    server: {
      settings: {
        aliases: [
          "yuki@old.example",
          "yuki@photos.example",
          "yuki_archive@social.example",
          "snow@mastodon.example",
          "y@a-rather-long-name-of-a-server.example",
        ],
      },
    },
  }),
};
/** Aliases while they are on their way. */
export const AliasesLoading: Story = { parameters: screen({ at: aliasesAt, state: "loading" }) };
/** Aliases when they could not be read. */
export const AliasesFailed: Story = { parameters: screen({ at: aliasesAt, state: "error" }) };
