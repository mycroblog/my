// SPDX-License-Identifier: AGPL-3.0-only
// The server's own icon and logos through the whole app (web-client/README.md T4, ADR-0116): an administrator with
// manage_settings chooses an SVG, which the server checks; the previews and what the server removed show; saving sends
// the sanitized SVG with every PNG rendition, and clearing brings the built-in image back. A file the server refuses
// says why under its button. jsdom draws nothing, so the renditions are stand-in PNGs; their layout is tested in
// hooks/brandRenditions.test.ts. With MSW playing the API.
/// <reference types="node" />
import { File as NodeFile } from "node:buffer";
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test, vi } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { nodeUploadFile } from "../../test/multipart";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import type { DrawnRendition } from "./hooks/brandRenditions";
import { adminBrandImageFixture, brandSanitizedFixture } from "./test/handlers";

vi.mock("./hooks/brandRenditions", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./hooks/brandRenditions")>();
  return {
    ...actual,
    // A stand-in PNG of each size the server asked for, drawn on the background given.
    drawRenditions: vi.fn(
      async (
        _svg: string,
        _viewBox: unknown,
        renditions: readonly Schemas["BrandRenditionSize"][],
        background: string,
      ): Promise<DrawnRendition[]> =>
        renditions.map(({ rendition, width, height }) => ({
          rendition,
          width,
          height,
          png: new NodeFile([`png ${rendition} on ${background}`], `${rendition}.png`, {
            type: "image/png",
          }) as unknown as Blob,
          url: `blob:test/${rendition}`,
        })),
    ),
  };
});

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
  server.use(signedInHandler, operatorHandler(["manage_settings"]));
  // jsdom has no blob: URLs, and its File does not go into Node.js's FormData (src/test/multipart.ts).
  let next = 0;
  URL.createObjectURL = () => `blob:test/${next++}`;
  URL.revokeObjectURL = () => {};
  vi.stubGlobal("File", NodeFile);
});

/**
 * The parts of a multipart body by name, as text. Node.js's parser of FormData refuses the files of a body that jsdom's
 * globals took part in, so the tests read the text (as settings.test.tsx does).
 */
function multipartParts(body: string): Record<string, string> {
  const parts: Record<string, string> = {};
  for (const match of body.matchAll(/name="([^"]+)"[^\r]*\r\n(?:[^\r]+\r\n)*\r\n([\s\S]*?)\r\n--/gu)) {
    const [, name, value] = match;
    if (name !== undefined && value !== undefined) parts[name] = value;
  }
  return parts;
}

/** Records the brand requests: the kinds checked, the parts of each PUT, and the kinds cleared. */
function brandServer(initial: Schemas["AdminBrands"] = {}) {
  let brands = initial;
  const checked: string[] = [];
  const saved: { kind: string; parts: Record<string, string> }[] = [];
  const cleared: string[] = [];
  server.use(
    http.get(`${apiBase}/admin/instance/brand`, () => HttpResponse.json(brands)),
    http.post(`${apiBase}/admin/instance/brand/sanitize`, async ({ request }) => {
      const kind = multipartParts(await request.text())["kind"] as Schemas["BrandKind"];
      checked.push(kind);
      return HttpResponse.json(
        brandSanitizedFixture(kind, {
          dropped: [
            { kind: "element", name: "script" },
            { kind: "attribute", name: "onclick" },
          ],
        }),
      );
    }),
    http.put(`${apiBase}/admin/instance/brand/:kind`, async ({ request, params }) => {
      const kind = params["kind"] as Schemas["BrandKind"];
      saved.push({ kind, parts: multipartParts(await request.text()) });
      const image = adminBrandImageFixture(kind, "0199f0e0-0000-7000-8000-00000000b002");
      brands = { ...brands, [kind]: image };
      return HttpResponse.json(image);
    }),
    http.delete(`${apiBase}/admin/instance/brand/:kind`, ({ params }) => {
      const kind = params["kind"] as Schemas["BrandKind"];
      cleared.push(kind);
      const { [kind]: _, ...rest } = brands;
      brands = rest;
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return { checked, saved, cleared };
}

/** The file input of a form (FileTrigger's input is hidden and has no role). */
function fileInput(form: HTMLElement): HTMLInputElement {
  // eslint-disable-next-line testing-library/no-node-access -- FileTrigger's input is hidden and has no role.
  const input = form.querySelector<HTMLInputElement>('input[type="file"]');
  if (input === null) throw new Error("no file input");
  return input;
}

const svg = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><script>alert(1)</script></svg>';

describe("the server's icon and logo", () => {
  test("checks an icon, previews it with what was removed, saves it with every rendition, and clears it", async () => {
    const brand = brandServer();
    const { user } = await renderApp({ path: "/admin/settings" });
    const section = await screen.findByRole("region", { name: "Icon and logo" });
    const form = within(section).getByRole("form", { name: "Icon" });
    expect(within(form).getByText("The built-in one of my is in use.")).toBeInTheDocument();
    expect(within(form).queryByRole("button", { name: "Use the built-in icon" })).not.toBeInTheDocument();

    await user.upload(fileInput(form), nodeUploadFile(svg, "icon.svg", "image/svg+xml"));

    // The previews of where the icon shows, and what the server removed.
    expect(await within(form).findByText("Home screen")).toBeInTheDocument();
    expect(within(form).getByText("Browser tab")).toBeInTheDocument();
    expect(within(form).getByText("App icon")).toBeInTheDocument();
    expect(within(form).getByText("Removed from the file (2):")).toBeInTheDocument();
    expect(within(form).getByText("<script>")).toBeInTheDocument();
    expect(within(form).getByText("onclick")).toBeInTheDocument();
    expect(brand.checked).toEqual(["icon"]);
    const background = within(form).getByRole("textbox", { name: "Background color" });
    // jsdom has no canvas to read the theme's surface color with: the stand-in is white.
    expect(background).toHaveValue("#ffffff");

    await user.clear(background);
    await user.type(background, "#102030");
    await user.click(within(form).getByRole("button", { name: "Save the icon" }));

    expect(await within(form).findByText("Saved the icon. The audit log records the change.")).toBeInTheDocument();
    expect(brand.saved).toHaveLength(1);
    const [put] = brand.saved;
    expect(put?.kind).toBe("icon");
    expect(Object.keys(put?.parts ?? {}).toSorted()).toEqual([
      "apple_touch_180",
      "background",
      "favicon_32",
      "favicon_48",
      "maskable_512",
      "pwa_192",
      "pwa_512",
      "svg",
    ]);
    expect(put?.parts["svg"]).toBe(brandSanitizedFixture("icon").svg);
    // The opaque renditions were drawn again on the new background, and the server gets that background.
    expect(put?.parts["background"]).toBe("#102030");
    expect(put?.parts["apple_touch_180"]).toBe("png apple-touch-180 on #102030");
    // The saved icon is shown, and can be cleared.
    expect(within(form).queryByText("Home screen")).not.toBeInTheDocument();
    expect(within(form).getByText("Now in use")).toBeInTheDocument();

    await user.click(within(form).getByRole("button", { name: "Use the built-in icon" }));
    expect(
      await within(form).findByText("The built-in icon is in use again. The audit log records the change."),
    ).toBeInTheDocument();
    expect(brand.cleared).toEqual(["icon"]);
    expect(within(form).getByText("The built-in one of my is in use.")).toBeInTheDocument();
  });

  test("saves a logo with its mail and link preview images, and no background", async () => {
    const brand = brandServer();
    const { user } = await renderApp({ path: "/admin/settings" });
    const form = await screen.findByRole("form", { name: "Logo" });

    await user.upload(fileInput(form), nodeUploadFile(svg, "logo.svg", "image/svg+xml"));
    expect(await within(form).findByText("Link preview")).toBeInTheDocument();
    expect(within(form).getByText("Email")).toBeInTheDocument();
    expect(within(form).queryByRole("textbox", { name: "Background color" })).not.toBeInTheDocument();
    await user.click(within(form).getByRole("button", { name: "Save the logo" }));

    expect(await within(form).findByText("Saved the logo. The audit log records the change.")).toBeInTheDocument();
    expect(Object.keys(brand.saved[0]?.parts ?? {}).toSorted()).toEqual(["logo_mail", "preview_1200x630", "svg"]);
  });

  test("says why the server refused a file, under the button that chose it", async () => {
    server.use(
      http.post(`${apiBase}/admin/instance/brand/sanitize`, () =>
        HttpResponse.json(
          {
            type: "/api/v1/problems/invalid-brand",
            title: "The image cannot be saved.",
            status: 422,
            brand_error: { code: "text", rendition: "svg" },
          } satisfies Schemas["BrandProblem"],
          { status: 422, headers: { "Content-Type": "application/problem+json" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: "/admin/settings" });
    const form = await screen.findByRole("form", { name: "Logo" });

    await user.upload(fileInput(form), nodeUploadFile(svg, "logo.svg", "image/svg+xml"));

    const error = await within(form).findByText(/Convert the text to outlines/);
    expect(error).toBeInTheDocument();
    expect(within(form).getByRole("button", { name: "Choose an SVG file" })).toHaveAccessibleDescription(
      /Logo.*Convert the text to outlines/,
    );
    expect(within(form).queryByText("Link preview")).not.toBeInTheDocument();
  });

  test("asks for a file when the save button is pressed without one", async () => {
    const brand = brandServer();
    const { user } = await renderApp({ path: "/admin/settings" });
    const form = await screen.findByRole("form", { name: "Logo for dark backgrounds" });

    await user.click(within(form).getByRole("button", { name: "Save the logo for dark backgrounds" }));

    expect(await within(form).findByText("Choose an SVG file to save.")).toBeInTheDocument();
    await waitFor(() => {
      expect(brand.saved).toHaveLength(0);
    });
  });

  test("shows the saved images, each with the button that clears it", async () => {
    brandServer({
      logo: adminBrandImageFixture("logo", "0199f0e0-0000-7000-8000-00000000b003"),
    });
    await renderApp({ path: "/admin/settings" });
    const logo = await screen.findByRole("form", { name: "Logo" });
    expect(within(logo).getByText("Now in use")).toBeInTheDocument();
    expect(within(logo).getByRole("button", { name: "Use the built-in logo" })).toBeInTheDocument();
    const icon = screen.getByRole("form", { name: "Icon" });
    expect(within(icon).queryByRole("button", { name: "Use the built-in icon" })).not.toBeInTheDocument();
  });
});
