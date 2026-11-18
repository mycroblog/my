// SPDX-License-Identifier: AGPL-3.0-only

// A download the reader asked for in a session that has closed since saves nothing: the data of a closed session
// settles no download, so that no file of the account reaches the device after a sign-out and no row says anything of
// it. What the screen says of a download in a session open is the app's route test (settings-account-data.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import { dataExportsQuery, downloadDataExport, exportCsv } from "./exports";
import { closeSession } from "./session";
import { fakeData } from "./test/data";

/** What `download` came to within a while: "saved", "failed", or "pending" when it did not settle. */
const outcome = (download: Promise<unknown>): Promise<string> =>
  Promise.race([
    download.then(
      () => "saved",
      () => "failed",
    ),
    new Promise<string>((resolve) =>
      setTimeout(() => {
        resolve("pending");
      }, 100),
    ),
  ]);

/** Closes the session of `data`, with nothing of the browser to clean. */
const closed = (data: Parameters<typeof closeSession>[0]) => closeSession(data, () => Promise.resolve());

/** The app's data on a server whose reader has an archive ready, and the archive's ID. */
async function archived() {
  const on = fakeData({ dataExport: "ready" });
  const [ready] = (await on.data.queryClient.query(dataExportsQuery(on.data))).items;
  if (ready === undefined) throw new Error("test: no archive is listed");
  return { ...on, id: ready.id };
}

describe("a download in a session that closed never settles", () => {
  test("downloadDataExport, the session closed before the call", async () => {
    const { data, id } = await archived();
    await closed(data);

    expect(await outcome(downloadDataExport(data, id))).toBe("pending");
  });

  test("downloadDataExport, the session closed by the time the answer arrives", async () => {
    const { fake, data, id } = await archived();
    const held = fake.script("GET /exports/{id}/archive", { hold: "response" });
    const download = downloadDataExport(data, id);
    await held.served();

    await closed(data);
    held.release();

    expect(await outcome(download)).toBe("pending");
  });

  test("exportCsv, the session closed before the call", async () => {
    const { data } = fakeData();
    await closed(data);

    expect(await outcome(exportCsv(data, "blocks"))).toBe("pending");
  });

  test("exportCsv, the session closed by the time the answer arrives", async () => {
    const { fake, data } = fakeData();
    const held = fake.script("GET /csv_exports/{kind}", { hold: "response" });
    const download = exportCsv(data, "mutes");
    await held.served();

    await closed(data);
    held.release();

    expect(await outcome(download)).toBe("pending");
  });

  test("in a session open, both save", async () => {
    const { data, id } = await archived();

    expect([await outcome(downloadDataExport(data, id)), await outcome(exportCsv(data, "blocks"))]).toEqual([
      "saved",
      "saved",
    ]);
  });
});
