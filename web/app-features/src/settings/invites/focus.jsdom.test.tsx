// SPDX-License-Identifier: AGPL-3.0-only

// Where the focus goes once the form that creates an invite gave way to something else (#3207), on jsdom: what takes
// the focus there is no control (the title of the summary that says the invite is created, or that the reader may
// not create invites; the sentence that says the reader holds as many usable invites as allowed), and happy-dom
// gives the focus to any element a script asks it for, focusable or not. The same summary and the same sentence on
// a screen that opens with them are no result of a press, and take no focus. What each view shows is pinned on
// happy-dom (create.dom.test.tsx, invites.dom.test.tsx), where the focus on a field or a button is pinned too.

import { describe, expect, test } from "bun:test";
import { screen } from "@testing-library/react";
import { createDrawn, createForm, invitesRead, refusalOn } from "../../test/invites";
import { atRest, focused, focusOn, section, words } from "../../test/security";
import { requestsOnJsdom } from "../../test/settings";

const cannot = "You cannot create invites";
const atLimit = "You have as many usable invites as allowed (5). Revoke one, or wait until one expires or is used up.";

const create = () => screen.getByRole("button", { name: "Create invite" });

describe("in the form's place, what came of Create invite takes the focus", () => {
  test("the invite that was created: the title that says so, and Tab goes on to Copy link", async () => {
    requestsOnJsdom();
    const { user } = await createForm();

    await user.click(create());

    await focusOn(() => screen.queryAllByRole("heading", { name: "The invite is created" })[0]);
    expect(focused()).toBe("heading: The invite is created");

    await user.tab();

    expect(focused()).toBe("button: Copy link");
  });

  test("a refusal that the account may not create invites: the title of the summary that says why", async () => {
    requestsOnJsdom();
    const problem = await refusalOn({ access: { invites: "not-allowed", invitesRefusal: "restricted" } });
    const { fake, user } = await createForm();
    // From the refusal on, the server answers the list as for an account that may not create invites.
    fake.script("POST /invites", { fail: problem });
    fake.script("GET /invites", {
      change: (body) => ({ ...body, can_create: false, cannot_create_reason: "restricted" as const }),
    });

    await user.click(create());

    await focusOn(() => screen.queryAllByRole("heading", { name: cannot })[0]);
    expect(focused()).toBe(`heading: ${cannot}`);
    expect(section(cannot).contains(document.activeElement)).toBe(true);
  });

  test("a refusal at the limit of usable invites, which the list read again confirms: the sentence that says so, as a status of the section", async () => {
    requestsOnJsdom();
    const problem = await refusalOn({ access: { invites: "at-cap" } });
    const { fake, user } = await createForm();
    // From the refusal on, the server counts as many usable invites as it allows: another tab created the rest.
    fake.script("POST /invites", { fail: problem });
    fake.script("GET /invites", { change: (body) => ({ ...body, usable: body.limits.max_active }) });

    await user.click(create());

    await focusOn(() => screen.queryAllByRole("status").find((status) => words(status) === atLimit));
    expect(focused()).toBe(`status: ${atLimit}`);
    expect(section("New invite").contains(document.activeElement)).toBe(true);
    // The form is gone: nothing is left to send.
    expect(screen.queryAllByRole("button", { name: "Create invite" })).toHaveLength(0);
    expect(screen.queryAllByRole("alert")).toHaveLength(0);
  });
});

// The reader pressed nothing: the app's shell gives the focus to the screen's title as it opens, and the summary or
// the sentence does not take it away.
describe("on a screen that opens with it, what stands in the form's place takes no focus", () => {
  test("the summary of an account that may not create invites, on the form's screen", async () => {
    requestsOnJsdom();
    const on = await createDrawn({ invites: "not-allowed" });
    const title = await screen.findByRole("heading", { name: cannot });
    await atRest(on);

    expect(focused()).toBe("nothing");
    title.focus();
    expect(focused()).toBe("nothing");
  });

  test("the same summary on the list", async () => {
    requestsOnJsdom();
    await invitesRead({ invites: "not-allowed" });
    const title = screen.getByRole("heading", { name: cannot });

    expect(focused()).toBe("nothing");
    title.focus();
    expect(focused()).toBe("nothing");
  });

  test("the sentence of an account at the limit of usable invites, on the form's screen", async () => {
    requestsOnJsdom();
    const on = await createDrawn({ invites: "at-cap" });
    const sentence = await screen.findByText(atLimit);
    await atRest(on);

    expect(focused()).toBe("nothing");
    sentence.focus();
    expect(focused()).toBe("nothing");
  });
});
