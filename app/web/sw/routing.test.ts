// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, it } from "vitest";
import { classify, type RequestFacts } from "./routing.ts";

const origin = "https://my.example";

function facts(path: string, overrides: Partial<RequestFacts> = {}): RequestFacts {
  return { url: `${origin}${path}`, method: "GET", mode: "cors", destination: "", authorized: false, ...overrides };
}

const navigate = (path: string) => facts(path, { mode: "navigate", destination: "document" });

describe("classify", () => {
  it.each(["/", "/local", "/@alice", "/@alice/01HZX", "/@alice@remote.example", "/settings?tab=profile", "/apple"])(
    "goes to the network first for the SPA navigation %s",
    (path) => {
      expect(classify(navigate(path), origin)).toBe("navigation");
    },
  );

  it.each([
    "/api",
    "/api/v1/timelines/home",
    "/oauth/authorize?client_id=x",
    "/auth/sign-in",
    "/.well-known/webfinger?resource=acct:alice@my.example",
    "/nodeinfo/2.1",
    "/ap/users/alice",
    "/media/x/full.jpg",
    "/uploads/x",
    "/licenses/LICENSE-Unicode.txt",
    "/manifest.webmanifest",
  ])("leaves the navigation to %s to the browser", (path) => {
    expect(classify(navigate(path), origin)).toBe("network");
  });

  it("never answers the client API, with or without a token", () => {
    expect(classify(facts("/api/v1/accounts/verify_credentials", { authorized: true }), origin)).toBe("network");
    expect(classify(facts("/api/v1/instance"), origin)).toBe("network");
  });

  it("caches public images under /media/ unless the request carries credentials", () => {
    expect(classify(facts("/media/a/thumb.webp", { destination: "image", mode: "no-cors" }), origin)).toBe("media");
    expect(classify(facts("/media/a/thumb.webp", { destination: "image", authorized: true }), origin)).toBe("network");
    expect(classify(facts("/media/a/clip.mp4", { destination: "video" }), origin)).toBe("network");
    expect(classify(facts("/mediae/a.png", { destination: "image" }), origin)).toBe("network");
  });

  it("caches the emoji fonts of the build, which the precache leaves out", () => {
    const font = { destination: "font", mode: "cors" };
    expect(classify(facts("/assets/noto-emoji-colrv1-smileys-emotion-B2x9Qa1c.woff2", font), origin)).toBe(
      "emoji-font",
    );
    expect(classify(facts("/assets/noto-emoji-svg-country-flags-Cq0_aZ3d.woff2", font), origin)).toBe("emoji-font");
    expect(classify(facts("/assets/index-B2x9Qa1c.woff2", font), origin)).toBe("network");
    expect(classify(facts("/media/noto-emoji-colrv1-x.woff2", font), origin)).toBe("network");
    expect(classify(facts("/assets/noto-emoji-colrv1-x.woff2", { destination: "" }), origin)).toBe("network");
  });

  it("caches the instance theme's stylesheets, whose URL names their revision", () => {
    expect(classify(facts("/theme/0199f0e0.css", { destination: "style", mode: "no-cors" }), origin)).toBe("theme");
    expect(classify(facts("/theme/0199f0e0.css", { destination: "style", authorized: true }), origin)).toBe("network");
    expect(classify(facts("/theme.js", { destination: "script" }), origin)).toBe("network");
    expect(classify(facts("/theme/0199f0e0.css", { mode: "navigate" }), origin)).toBe("network");
  });

  it("leaves other origins and other methods to the browser", () => {
    expect(classify({ ...navigate("/"), url: "https://cdn.example/" }, origin)).toBe("network");
    expect(
      classify(facts("/media/a.png", { destination: "image", url: "https://cdn.example/media/a.png" }), origin),
    ).toBe("network");
    expect(classify(facts("/", { method: "POST", mode: "navigate" }), origin)).toBe("network");
  });
});
