// SPDX-License-Identifier: AGPL-3.0-only
// FormActionBar stories: the result beside the submit button. The play function checks that the regions are in the
// page, empty, before the submit, and that the result shows beside the button right after it.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, waitFor } from "storybook/test";
import { Button } from "./Button";
import { FormActionBar, type FormActionResult } from "./FormActionBar";

const meta = {
  title: "Forms/FormActionBar",
  component: FormActionBar,
  args: { children: <Button variant="primary">Save</Button> },
} satisfies Meta<typeof FormActionBar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Success: Story = { args: { result: { tone: "success", message: "Your password is changed." } } };

export const Failure: Story = {
  args: {
    result: { tone: "danger", message: "Couldn't change the password: the current password is not right." },
  },
};

function Demo() {
  const [result, setResult] = useState<FormActionResult | undefined>(undefined);
  const [pending, setPending] = useState(false);
  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(event) => {
        event.preventDefault();
        setPending(true);
        setTimeout(() => {
          setPending(false);
          setResult({ tone: "success", message: "Your profile is saved." });
        }, 200);
      }}
    >
      <FormActionBar result={result}>
        <Button variant="primary" type="submit" isPending={pending}>
          Save
        </Button>
      </FormActionBar>
    </form>
  );
}

/** The result arrives in the empty region beside the button. */
export const AfterASubmit: Story = {
  render: () => <Demo />,
  play: async ({ canvas, userEvent }) => {
    const status = canvas.getByRole("status");
    await expect(status).toBeEmptyDOMElement();
    await userEvent.click(canvas.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(status).toHaveTextContent("Your profile is saved."));
  },
};
