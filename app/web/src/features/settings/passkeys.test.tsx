// SPDX-License-Identifier: AGPL-3.0-only
// The passkeys page through the whole app (web-client/README.md T4): adding a passkey (the password, then the browser's
// prompt, which test/webauthn plays), what a wrong password or a dismissed prompt says, a browser without passkeys,
// and removing one after its dialog, with MSW playing the server.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test } from "vitest";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { sectionHeadings } from "../../test/sections";
import { installPasskeys, removePasskeys } from "../../test/webauthn";

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

afterEach(() => {
  removePasskeys();
});

const phone: Schemas["Passkey"] = {
  id: "019a0000-0000-7000-8000-0000000000a1",
  name: "Phone",
  created_at: "2026-10-01T09:00:00Z",
};

/** The server's passkeys of the viewer, which the operations change, with the requests they received. */
function passkeyServer(initial: Schemas["Passkey"][]) {
  let items = initial;
  const registrations: Schemas["PasskeyRegistrationRequest"][] = [];
  const created: Schemas["CreatePasskeyRequest"][] = [];
  server.use(
    http.get(`${apiBase}/passkeys`, () => HttpResponse.json({ items })),
    http.post(`${apiBase}/passkeys/registration`, async ({ request }) => {
      const body = (await request.json()) as Schemas["PasskeyRegistrationRequest"];
      registrations.push(body);
      if (body.passkey === undefined && body.password !== "correct horse") {
        return problem(422, "/api/v1/problems/wrong-password", "The password is wrong.");
      }
      return HttpResponse.json({
        options: {
          rp: { name: "social.example", id: "social.example" },
          user: { name: "alice@social.example", displayName: "alice@social.example", id: "dTE" },
          challenge: "AQID",
          pubKeyCredParams: [{ type: "public-key", alg: -7 }],
          authenticatorSelection: { residentKey: "required", userVerification: "required" },
          attestation: "none",
        },
        registration_token: "reg-1",
        expires_at: "2026-10-20T00:05:00Z",
      });
    }),
    http.post(`${apiBase}/passkeys`, async ({ request }) => {
      const body = (await request.json()) as Schemas["CreatePasskeyRequest"];
      created.push(body);
      const added = {
        id: "019a0000-0000-7000-8000-0000000000a2",
        name: registrations.at(-1)?.name ?? "",
        created_at: "2026-10-20T00:00:00Z",
      };
      items = [...items, added];
      return HttpResponse.json(added, { status: 201 });
    }),
    http.delete(`${apiBase}/passkeys/:id`, ({ params }) => {
      items = items.filter((p) => p.id !== params["id"]);
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return { registrations, created };
}

describe("passkeys", () => {
  test("are in the settings menu", async () => {
    await renderApp({ path: "/settings" });
    const link = await screen.findByRole("link", { name: "Passkeys" });
    expect(link).toHaveAttribute("href", "/settings/passkeys");
  });

  test("adds a passkey after the password: a wrong one is the field's error, the right one opens the browser's prompt", async () => {
    const credentials = installPasskeys();
    const api = passkeyServer([phone]);
    const { user } = await renderApp({ path: "/settings/passkeys" });

    expect(await screen.findByRole("heading", { level: 1, name: "Passkeys" })).toBeInTheDocument();
    // docs/architecture/web-client/screens/settings-passkeys.md §3
    expect(sectionHeadings()).toEqual(["Your passkeys"]);
    const list = screen.getByRole("region", { name: "Your passkeys" });
    expect(within(list).getByText("Phone")).toHaveAttribute("dir", "auto");
    expect(within(list).getByText("1 passkey")).toBeInTheDocument();

    await user.click(within(list).getByRole("button", { name: "Add a passkey" }));
    const form = within(list).getByRole("region", { name: "Add a passkey" });
    await user.type(within(form).getByRole("textbox", { name: "Name" }), " Laptop ");
    const password = within(form).getByLabelText(/^Current password/);
    await user.type(password, "wrong");
    await user.click(within(form).getByRole("button", { name: "Add passkey" }));
    expect(await within(form).findByText("The password is wrong.")).toBeInTheDocument();
    expect(password).toHaveAttribute("aria-invalid", "true");
    expect(credentials.create).not.toHaveBeenCalled();

    await user.clear(password);
    await user.type(password, "correct horse");
    await user.click(within(form).getByRole("button", { name: "Add passkey" }));

    // The panel closes; the result shows above the list, where the new row is.
    expect(await within(list).findByRole("status")).toHaveTextContent("The passkey “Laptop” was added.");
    expect(await within(list).findByText("Laptop")).toBeInTheDocument();
    expect(within(list).queryByRole("region", { name: "Add a passkey" })).not.toBeInTheDocument();
    expect(within(list).getByRole("button", { name: "Add a passkey" })).toHaveFocus();
    expect(api.registrations).toEqual([
      { name: "Laptop", password: "wrong" },
      { name: "Laptop", password: "correct horse" },
    ]);
    const options = credentials.create.mock.calls[0]?.[0].publicKey;
    expect(options?.rp.id).toBe("social.example");
    expect(options?.authenticatorSelection?.residentKey).toBe("required");
    expect(api.created).toEqual([
      {
        registration_token: "reg-1",
        credential: {
          id: "AQID",
          rawId: "AQID",
          type: "public-key",
          authenticatorAttachment: "platform",
          clientExtensionResults: {},
          response: { clientDataJSON: "AQID", attestationObject: "BAUG", transports: ["internal"] },
        },
      },
    ]);
  });

  test("says why when the prompt is dismissed or the server refuses the passkey", async () => {
    const credentials = installPasskeys();
    credentials.create.mockRejectedValueOnce(new DOMException("dismissed", "NotAllowedError"));
    passkeyServer([]);
    const { user } = await renderApp({ path: "/settings/passkeys" });

    expect(await screen.findByRole("heading", { level: 3, name: "No passkeys yet" })).toBeInTheDocument();
    // The empty list leads to the form.
    await user.click(screen.getByRole("button", { name: "Add a passkey" }));
    const form = screen.getByRole("region", { name: "Add a passkey" });
    await user.type(within(form).getByRole("textbox", { name: "Name" }), "Laptop");
    await user.type(within(form).getByLabelText(/^Current password/), "correct horse");
    await user.click(within(form).getByRole("button", { name: "Add passkey" }));
    expect(await within(form).findByRole("alert")).toHaveTextContent(
      "No passkey was added: the browser's prompt was closed or timed out.",
    );

    server.use(
      http.post(`${apiBase}/passkeys`, () =>
        problem(422, "/api/v1/problems/invalid-passkey", "The passkey could not be verified."),
      ),
    );
    await user.click(within(form).getByRole("button", { name: "Add passkey" }));
    expect(await within(form).findByRole("alert")).toHaveTextContent(
      "The passkey could not be verified. Try again with another device",
    );
  });

  test("tells a browser without passkeys that it cannot add one", async () => {
    passkeyServer([phone]);
    await renderApp({ path: "/settings/passkeys" });
    expect(
      await screen.findByText(
        "This browser cannot create passkeys. Open this page in a browser that supports them to add one.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add a passkey" })).not.toBeInTheDocument();
    expect(screen.getByText("Phone")).toBeInTheDocument();
  });

  test("removes a passkey after its dialog", async () => {
    installPasskeys();
    passkeyServer([phone]);
    const { user } = await renderApp({ path: "/settings/passkeys" });

    await user.click(await screen.findByRole("button", { name: "Remove: Phone" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Remove the passkey “Phone”?" });
    await user.click(within(dialog).getByRole("button", { name: "Remove" }));

    expect(await screen.findByRole("heading", { level: 3, name: "No passkeys yet" })).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
  });

  test("adds a passkey with another passkey for a user without a password", async () => {
    const credentials = installPasskeys();
    const api = passkeyServer([phone]);
    server.use(http.get(`${apiBase}/accounts/password`, () => HttpResponse.json({ has_password: false })));
    const { user } = await renderApp({ path: "/settings/passkeys" });

    await user.click(await screen.findByRole("button", { name: "Add a passkey" }));
    const form = screen.getByRole("region", { name: "Add a passkey" });
    expect(
      within(form).getByText(
        /^Name the passkey so that you can tell it apart. Your browser asks for one of your passkeys/,
      ),
    ).toBeInTheDocument();
    expect(within(form).queryByLabelText(/^Current password/)).not.toBeInTheDocument();
    await user.type(within(form).getByRole("textbox", { name: "Name" }), "Laptop");
    await user.click(within(form).getByRole("button", { name: "Add passkey" }));

    expect(await screen.findByText("The passkey “Laptop” was added.")).toBeInTheDocument();
    expect(credentials.get).toHaveBeenCalledTimes(1);
    expect(credentials.create).toHaveBeenCalledTimes(1);
    expect(api.registrations[0]?.password).toBeUndefined();
    expect(api.registrations[0]?.passkey?.confirmation_token).toBe("conf-1");
  });
});
