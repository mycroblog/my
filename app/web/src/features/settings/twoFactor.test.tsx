// SPDX-License-Identifier: AGPL-3.0-only
// The two-factor authentication page through the whole app (web-client/README.md T4): enrolling an authenticator app
// from its QR code, the recovery codes shown once, replacing them, and turning it off, with MSW playing the server.
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { recoveryCodesFixture, totpSetupFixture } from "./test/handlers";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

const invalidCode = () =>
  problem(422, "/api/v1/problems/invalid-two-factor-code", "The code is incorrect or was already used.");

/** The server's two-factor authentication of the viewer, which the operations change, with the codes they received. */
function twoFactorServer(initial: Schemas["TwoFactor"]) {
  let state = initial;
  const codes: string[] = [];
  const on: Schemas["TwoFactor"] = { enabled: true, enrolled_at: "2026-10-20T00:00:00Z", recovery_codes_remaining: 2 };
  server.use(
    http.get(`${apiBase}/two_factor`, () => HttpResponse.json(state)),
    http.post(`${apiBase}/two_factor/totp`, async ({ request }) => {
      const body = (await request.json()) as Schemas["EnableTotpRequest"];
      codes.push(body.code);
      if (body.code !== "123456") return invalidCode();
      state = on;
      return HttpResponse.json(recoveryCodesFixture);
    }),
    http.post(`${apiBase}/two_factor/recovery_codes`, async ({ request }) => {
      const body = (await request.json()) as Schemas["TwoFactorCodeRequest"];
      codes.push(body.code);
      if (body.code !== "123456") return invalidCode();
      return HttpResponse.json({ recovery_codes: ["2222-3333-4444-5555"] });
    }),
    http.post(`${apiBase}/two_factor/totp/disable`, async ({ request }) => {
      const body = (await request.json()) as Schemas["TwoFactorCodeRequest"];
      codes.push(body.code);
      if (body.code !== "abcd-efgh-ijkl-mnop") return invalidCode();
      state = { enabled: false };
      return HttpResponse.json(state);
    }),
  );
  return { codes };
}

describe("two-factor authentication", () => {
  test("is in the settings menu", async () => {
    await renderApp({ path: "/settings" });
    const link = await screen.findByRole("link", { name: "Two-factor authentication" });
    expect(link).toHaveAttribute("href", "/settings/two-factor");
  });

  test("enrolls an authenticator app from its QR code: a wrong code is the field's error, the right one shows the recovery codes once", async () => {
    const api = twoFactorServer({ enabled: false });
    const { user } = await renderApp({ path: "/settings/two-factor" });

    expect(await screen.findByRole("heading", { level: 1, name: "Two-factor authentication" })).toBeInTheDocument();
    expect(screen.getByText("Off. Signing in asks only for your password.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Set up" }));

    const qr = await screen.findByRole("img", { name: "QR code for your authenticator app" });
    expect(qr).toHaveAttribute(
      "viewBox",
      `0 0 ${String(totpSetupFixture.qr_code.size)} ${String(totpSetupFixture.qr_code.size)}`,
    );
    expect(screen.getByText("JBSW Y3DP EHPK 3PXP JBSW Y3DP EHPK 3PXP")).toHaveAttribute("dir", "ltr");

    const code = screen.getByRole("textbox", { name: "Authentication code" });
    await user.type(code, "000000");
    await user.click(screen.getByRole("button", { name: "Turn on" }));
    expect(await screen.findByText("The code is incorrect or was already used.")).toBeInTheDocument();
    expect(code).toHaveAttribute("aria-invalid", "true");

    await user.clear(code);
    await user.type(code, " 123456 ");
    await user.click(screen.getByRole("button", { name: "Turn on" }));

    expect(await screen.findByText("Two-factor authentication is on.")).toBeInTheDocument();
    const saved = screen.getByRole("region", { name: "Save your recovery codes" });
    expect(
      within(saved)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(recoveryCodesFixture.recovery_codes);
    expect(screen.getByText("On")).toBeInTheDocument();
    expect(screen.queryByRole("img", { name: "QR code for your authenticator app" })).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Turn off two-factor authentication" })).toBeInTheDocument();
    expect(api.codes).toEqual(["000000", "123456"]);
  });

  test("an expired setup gives way to a new one", async () => {
    twoFactorServer({ enabled: false });
    server.use(
      http.post(`${apiBase}/two_factor/totp`, () => problem(422, "/api/v1/problems/totp-setup-expired", "Expired")),
    );
    const { user } = await renderApp({ path: "/settings/two-factor" });

    await user.click(await screen.findByRole("button", { name: "Set up" }));
    await user.type(await screen.findByRole("textbox", { name: "Authentication code" }), "123456");
    await user.click(screen.getByRole("button", { name: "Turn on" }));

    expect(await screen.findByText("The setup took too long. Start again.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Set up" })).toBeInTheDocument();
    expect(screen.queryByRole("img", { name: "QR code for your authenticator app" })).not.toBeInTheDocument();
  });

  test("replaces the recovery codes and turns it off with a recovery code", async () => {
    const api = twoFactorServer({ enabled: true, enrolled_at: "2026-10-01T00:00:00Z", recovery_codes_remaining: 8 });
    const { user } = await renderApp({ path: "/settings/two-factor" });

    expect(await screen.findByText("On")).toBeInTheDocument();
    expect(screen.getByText("8")).toBeInTheDocument();

    const recovery = screen.getByRole("region", { name: "Recovery codes" });
    await user.type(within(recovery).getByRole("textbox", { name: "Authentication code" }), "123456");
    await user.click(within(recovery).getByRole("button", { name: "Replace recovery codes" }));
    expect(await screen.findByText("New recovery codes are ready. The old ones no longer work.")).toBeInTheDocument();
    expect(
      within(screen.getByRole("region", { name: "Save your recovery codes" })).getByText("2222-3333-4444-5555"),
    ).toBeInTheDocument();

    const off = screen.getByRole("region", { name: "Turn off two-factor authentication" });
    await user.type(within(off).getByRole("textbox", { name: "Authentication code" }), "wrong");
    await user.click(within(off).getByRole("button", { name: "Turn off" }));
    expect(await within(off).findByText("The code is incorrect or was already used.")).toBeInTheDocument();

    await user.clear(within(off).getByRole("textbox", { name: "Authentication code" }));
    await user.type(within(off).getByRole("textbox", { name: "Authentication code" }), "abcd-efgh-ijkl-mnop");
    await user.click(within(off).getByRole("button", { name: "Turn off" }));

    expect(await screen.findByText("Off. Signing in asks only for your password.")).toBeInTheDocument();
    expect(screen.getByText("Two-factor authentication is off.")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Save your recovery codes" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Set up" })).toBeInTheDocument();
    expect(api.codes).toEqual(["123456", "wrong", "abcd-efgh-ijkl-mnop"]);
  });

  test("too many wrong codes say so with the server's reason", async () => {
    twoFactorServer({ enabled: true, enrolled_at: "2026-10-01T00:00:00Z", recovery_codes_remaining: 8 });
    server.use(
      http.post(`${apiBase}/two_factor/totp/disable`, () =>
        HttpResponse.json(
          { type: "about:blank", title: "Too Many Requests", status: 429, detail: "Try again in 60 seconds." },
          { status: 429, headers: { "Content-Type": "application/problem+json", "Retry-After": "60" } },
        ),
      ),
    );
    const { user } = await renderApp({ path: "/settings/two-factor" });

    const off = await screen.findByRole("region", { name: "Turn off two-factor authentication" });
    await user.type(within(off).getByRole("textbox", { name: "Authentication code" }), "123456");
    await user.click(within(off).getByRole("button", { name: "Turn off" }));
    expect(await within(off).findByRole("alert")).toHaveTextContent(
      "Too many incorrect codes: Try again in 60 seconds.",
    );
  });
});
