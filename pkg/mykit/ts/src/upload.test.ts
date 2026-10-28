// SPDX-License-Identifier: MIT
import { describe, expect, test } from "bun:test";
import { createClient } from "./client.js";
import { fakeFetch, json, problem, server } from "./test/fake.js";
import { uploadMedia } from "./upload.js";

describe("uploadMedia", () => {
  test("sends the file, the thumbnail, and the fields as multipart/form-data", async () => {
    const f = fakeFetch(() => json(202, { id: "m1", state: "processing" }));
    const api = createClient({ server, fetch: f.fetch });
    const media = await uploadMedia(api, {
      file: new File(["video"], "clip.mp4", { type: "video/mp4" }),
      thumbnail: new Blob(["png"], { type: "image/png" }),
      description: "a clip",
      purpose: "attachment",
    });
    expect(media.id).toBe("m1");
    const [sent] = f.requests;
    expect(sent?.request.url).toBe(`${server}/api/v1/media`);
    expect(sent?.request.headers.get("Content-Type")).toStartWith("multipart/form-data; boundary=");
    const form = await new Request(sent?.request.url ?? "", {
      method: "POST",
      body: sent?.body ?? "",
      headers: sent?.request.headers ?? {},
    }).formData();
    const file = form.get("file") as File;
    expect(file.name).toBe("clip.mp4");
    expect(file.type).toBe("video/mp4");
    expect(await file.text()).toBe("video");
    expect((form.get("thumbnail") as File).name).toBe("thumbnail");
    expect(form.get("description")).toBe("a clip");
    expect(form.get("purpose")).toBe("attachment");
  });

  test("is sent again after a 401 with the same body", async () => {
    const f = fakeFetch((r) =>
      r.headers.get("Authorization") === "Bearer t2"
        ? json(202, { id: "m1" })
        : problem(401, "/api/v1/problems/invalid-token"),
    );
    const api = createClient({
      server,
      fetch: f.fetch,
      tokenSource: { getAccessToken: () => Promise.resolve("t1"), refresh: () => Promise.resolve("t2") },
    });
    await uploadMedia(api, { file: new Blob(["data"]), fileName: "a.bin" });
    expect(f.requests).toHaveLength(2);
    expect(f.requests[1]?.body).toBe(f.requests[0]?.body ?? "");
    expect(f.requests[1]?.body).toContain('filename="a.bin"');
  });
});
