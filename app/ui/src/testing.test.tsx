// SPDX-License-Identifier: AGPL-3.0-only
import { render } from "@testing-library/react";
import { describe, expect, test } from "vitest";
import { Button } from "./Button";
import { Checkbox } from "./Checkbox";
import { Switch } from "./Switch";
import { expectNoSwitchInSubmitForm, switchesInSubmitForms, watchSwitchesInSubmitForms } from "./testing";

describe("the check for a switch inside a form with a submit button", () => {
  test("finds a switch in a form saved by a button, by its label", () => {
    render(
      <form>
        <Switch>Post about the room</Switch>
        <Button type="submit">Save</Button>
      </form>,
    );

    expect(switchesInSubmitForms()).toEqual(['"Post about the room"']);
    expect(() => {
      expectNoSwitchInSubmitForm();
    }).toThrow(/use a Checkbox there: "Post about the room"/);
  });

  test("finds a switch whose form's submit button is outside it, named with form=", () => {
    render(
      <>
        <form id="profile">
          <Switch>Show the badge</Switch>
        </form>
        <button type="submit" form="profile">
          Save
        </button>
      </>,
    );

    expect(switchesInSubmitForms()).toEqual(['"Show the badge"']);
  });

  test("accepts a checkbox in such a form, and a switch outside a form or in one without a submit button", () => {
    render(
      <>
        <form>
          <Checkbox>Post about the room</Checkbox>
          <Button type="submit">Save</Button>
        </form>
        <form>
          <Switch>Play a sound</Switch>
          <Button>Test the sound</Button>
        </form>
        <Switch>Show new posts automatically</Switch>
      </>,
    );

    expect(() => {
      expectNoSwitchInSubmitForm();
    }).not.toThrow();
  });

  test("the watch catches a switch that was shown and taken away during the test", async () => {
    const watch = watchSwitchesInSubmitForms();
    const { rerender } = render(<form />);
    rerender(
      <form>
        <Switch>Post about the room</Switch>
        <Button type="submit">Save</Button>
      </form>,
    );
    // The observer reports the change after the current task's microtasks.
    await Promise.resolve();
    rerender(<form />);

    expect(() => {
      watch.stop();
    }).toThrow(/"Post about the room"/);
  });
});
