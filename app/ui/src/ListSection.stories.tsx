// SPDX-License-Identifier: AGPL-3.0-only
// ListSection stories: a list with its count and the one place to add to it. The play functions check that the add
// button is in one place at a time, that the panel opens in its place with focus in its first field, and that adding
// shows the result above the rows and gives focus back to the button.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { type SyntheticEvent, useState } from "react";
import { expect, waitFor } from "storybook/test";
import { Button } from "./Button";
import { Cluster } from "./Cluster";
import { FormActionBar } from "./FormActionBar";
import { ListSection, type ListSectionProps } from "./ListSection";
import { Stack } from "./Stack";
import { Hint, Text } from "./Text";
import { TextField } from "./TextField";
import type { ActionResultValue } from "./ActionResult";

type Domain = { domain: string; since: string };

function Row({ item }: { item: Domain }) {
  return (
    <Stack gap="3" rowAt="md">
      <Stack as="p" shrink grow>
        <Text dir="ltr" weight="semibold">
          {item.domain}
        </Text>
        <Hint as="span">Muted since {item.since}</Hint>
      </Stack>
      <Cluster gap="2">
        <Button size="sm" aria-label={`Unmute ${item.domain}`}>
          Unmute
        </Button>
      </Cluster>
    </Stack>
  );
}

const twoDomains: Domain[] = [
  { domain: "example.com", since: "Oct 1, 2026" },
  { domain: "spam.example", since: "Oct 20, 2026" },
];

/** A form in the panel: the domain, then Mute and Cancel in its action bar. */
function MuteForm({ onMute, close }: { onMute: (domain: string) => void; close: () => void }) {
  const [domain, setDomain] = useState("");
  return (
    <Stack
      as="form"
      gap="4"
      onSubmit={(event: SyntheticEvent<HTMLFormElement>) => {
        event.preventDefault();
        if (domain.trim() === "") return;
        onMute(domain.trim());
        close();
      }}
    >
      <TextField label="Domain" value={domain} onChange={setDomain} isRequired />
      <FormActionBar>
        <Button type="submit" variant="primary">
          Mute
        </Button>
        <Button onPress={close}>Cancel</Button>
      </FormActionBar>
    </Stack>
  );
}

function Demo({ initial, ...props }: { initial: Domain[] } & Partial<ListSectionProps<Domain>>) {
  const [items, setItems] = useState(initial);
  const [result, setResult] = useState<ActionResultValue | undefined>(undefined);
  return (
    <ListSection
      title="Muted domains"
      description="Posts and notifications from a muted domain stay out of your timelines."
      items={items}
      itemKey={(item) => item.domain}
      renderItem={(item) => <Row item={item} />}
      countLabel={`${String(items.length)} muted domains`}
      empty={{ heading: "No muted domains", description: "Mute a domain to keep its posts out of your timelines." }}
      add={{
        label: "Mute a domain",
        panel: (close) => (
          <MuteForm
            close={close}
            onMute={(domain) => {
              setItems((current) => [...current, { domain, since: "Oct 29, 2026" }]);
              setResult({ tone: "success", message: `${domain} is muted.` });
            }}
          />
        ),
      }}
      result={result}
      {...props}
    />
  );
}

const meta = { title: "Layout/ListSection" } satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Two items: the count beside the title and the add button at the end of the heading line. */
export const Default: Story = {
  render: () => <Demo initial={twoDomains} />,
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("region", { name: "Muted domains" })).toBeInTheDocument();
    await expect(canvas.getByText("2 muted domains")).toBeInTheDocument();
    await expect(canvas.getAllByRole("button", { name: "Mute a domain" })).toHaveLength(1);
    await expect(canvas.getAllByRole("listitem")).toHaveLength(2);
  },
};

/** Adding: the panel opens in place of the button; the new row and the result show once it closes. */
export const Adding: Story = {
  render: () => <Demo initial={twoDomains} />,
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Mute a domain" }));
    const field = canvas.getByRole("textbox", { name: /Domain/ });
    await waitFor(() => expect(field).toHaveFocus());
    await expect(canvas.queryByRole("button", { name: "Mute a domain" })).not.toBeInTheDocument();
    await userEvent.type(field, "noisy.example");
    await userEvent.click(canvas.getByRole("button", { name: "Mute" }));
    await waitFor(() => expect(canvas.getByRole("status")).toHaveTextContent("noisy.example is muted."));
    await waitFor(() => expect(canvas.getByRole("button", { name: "Mute a domain" })).toHaveFocus());
    await expect(canvas.getAllByRole("listitem")).toHaveLength(3);
  },
};

/** The add panel open. */
export const PanelOpen: Story = {
  render: () => (
    <Demo
      initial={twoDomains}
      add={{
        label: "Mute a domain",
        panel: (close) => <MuteForm close={close} onMute={() => undefined} />,
        defaultOpen: true,
      }}
    />
  ),
};

/** No items: the empty state leads to adding with the same button, which is nowhere else. */
export const Empty: Story = {
  render: () => <Demo initial={[]} />,
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("heading", { level: 3, name: "No muted domains" })).toBeInTheDocument();
    await expect(canvas.getAllByRole("button", { name: "Mute a domain" })).toHaveLength(1);
  },
};

/** A list loaded in pages: the count is a lower bound until the last page. */
export const MorePages: Story = {
  render: () => (
    <Demo initial={twoDomains} hasMore more={{ label: "Show more", onPress: () => undefined, isPending: false }} />
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getByText("2+")).toBeInTheDocument();
    await expect(canvas.getByRole("button", { name: "Show more" })).toBeInTheDocument();
  },
};

/** Nothing can be added now: the reason stands where the button would. */
export const Unavailable: Story = {
  render: () => <Demo initial={twoDomains} add={{ unavailable: "You have the most aliases you can have." }} />,
  play: async ({ canvas }) => {
    await expect(canvas.queryByRole("button", { name: "Mute a domain" })).not.toBeInTheDocument();
    await expect(canvas.getByText("You have the most aliases you can have.")).toBeInTheDocument();
  },
};

/** A failed removal, above the rows it concerns. */
export const Failure: Story = {
  render: () => <Demo initial={twoDomains} result={{ tone: "danger", message: "Couldn't unmute spam.example." }} />,
};

/** A search that narrows the list, under the heading line and above the rows, inside the region. */
export const WithToolbar: Story = {
  render: () => (
    <Demo
      initial={twoDomains}
      toolbar={
        <Cluster as="form" gap="2" align="end" role="search" aria-label="Search the muted domains">
          <TextField type="search" label="Domain" />
          <Button type="submit">Search</Button>
        </Cluster>
      }
    />
  ),
  play: async ({ canvas }) => {
    const region = canvas.getByRole("region", { name: "Muted domains" });
    await expect(region).toContainElement(canvas.getByRole("search", { name: "Search the muted domains" }));
  },
};
