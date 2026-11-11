// SPDX-License-Identifier: AGPL-3.0-only
// The remote servers screen below its heading (federation.md §7.3), in the admin mode's table form: a toolbar
// with the state filter, kept in the URL by the route (web-client/README.md S3), and one page of the servers of the
// selected filter at a time, previous / next, each with its state, since when, and its failures, linking to
// the server's page with the reasons. Above them, a form opens the page of any server by its domain, a handle on
// it or a URL on it: a server whose deliveries never failed is not in the list.
import {
  Box,
  Button,
  Card,
  EmptyState,
  Link,
  ListToolbar,
  Section,
  Select,
  SelectItem,
  Stack,
  TextField,
} from "@my/ui";
import { type SubmitEvent, useState } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { type InstanceFilter, instanceFilters, type InstanceHealth } from "../api/staff";
import { instanceDomainOf, instancePath, useInstances } from "../hooks/useInstances";
import {
  Day,
  DateTime,
  DomainName,
  InstanceStateLabel,
  type RecordColumns,
  RecordTable,
  RequirePermission,
} from "./parts";

const filterLabels: Readonly<Record<InstanceFilter, ClientMessageKey>> = {
  all: "client:admin.instances.filter.all",
  unavailable: "client:admin.instances.filter.unavailable",
  available: "client:admin.instances.filter.available",
};

function isFilter(key: unknown): key is InstanceFilter {
  return instanceFilters.some((filter) => filter === key);
}

/** The longest text the form takes: a URL on the server. */
const openInputLimit = 2048;

export type InstanceListProps = {
  /** The selected filter, from the URL. */
  filter: InstanceFilter;
  /** Selects another filter: the route puts it into the URL. */
  onFilterChange: (filter: InstanceFilter) => void;
  /** Opens the page of the server with this domain (IDNA A-labels in lower case). */
  onOpen: (domain: string) => void;
};

export function InstanceList({ onOpen, ...props }: InstanceListProps) {
  return (
    <RequirePermission permission="manage_domains">
      <Stack gap="6">
        <Card>
          <OpenInstance onOpen={onOpen} />
        </Card>
        <InstanceTabs {...props} />
      </Stack>
    </RequirePermission>
  );
}

/** The form that opens the page of any server, listed or not. */
function OpenInstance({ onOpen }: Pick<InstanceListProps, "onOpen">) {
  const t = useT();
  const [input, setInput] = useState("");
  const submit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    const domain = instanceDomainOf(input);
    if (domain !== undefined) onOpen(domain);
  };
  return (
    <Section
      surface="card"
      title={t("client:admin.instances.open.title")}
      description={t("client:admin.instances.open.description")}
    >
      <Stack
        as="form"
        role="search"
        aria-label={t("client:admin.instances.open.title")}
        onSubmit={submit}
        container
        gap="3"
      >
        <TextField
          label={t("client:admin.instances.open.domain")}
          description={t("client:admin.instances.open.domain.description")}
          value={input}
          onChange={setInput}
          isRequired
          maxLength={openInputLimit}
          validate={(value) =>
            value.trim() === "" || instanceDomainOf(value) !== undefined
              ? null
              : t("client:admin.instances.open.domain.invalid")
          }
          errorMessage={(validation) =>
            validation.validationDetails.valueMissing
              ? t("client:admin.instances.open.domain.required")
              : validation.validationErrors.join(" ")
          }
          autoComplete="off"
          spellCheck="false"
          dir="ltr"
        />
        <Box>
          <Button type="submit">{t("client:admin.instances.open.submit")}</Button>
        </Box>
      </Stack>
    </Section>
  );
}

function InstanceTabs({ filter, onFilterChange }: Omit<InstanceListProps, "onOpen">) {
  const t = useT();
  return (
    <Stack gap="4">
      <ListToolbar label={t("client:admin.list.filters")}>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.instances.filter.label")}
          value={filter}
          onChange={(key) => {
            if (isFilter(key)) onFilterChange(key);
          }}
        >
          {instanceFilters.map((item) => (
            <SelectItem key={item} id={item}>
              {t(filterLabels[item])}
            </SelectItem>
          ))}
        </Select>
      </ListToolbar>
      {/* Another filter is another list: it starts at its first page. */}
      <Instances key={filter} filter={filter} />
    </Stack>
  );
}

function Instances({ filter }: { filter: InstanceFilter }) {
  const t = useT();
  const list = useInstances(filter);
  const columns: RecordColumns<InstanceHealth> = [
    {
      id: "server",
      header: t("client:admin.instances.column.server"),
      wrap: "anywhere",
      cell: (health) => (
        <Link href={instancePath(health.domain)} variant="standalone" wrap="anywhere">
          <DomainName domain={health.domain} />
        </Link>
      ),
    },
    {
      id: "state",
      header: t("client:admin.instance.state"),
      cell: (health) => <InstanceStateLabel state={health.state} />,
    },
    {
      id: "since",
      header: t("client:admin.instance.since"),
      cell: (health) =>
        health.state_changed_at === undefined ? (
          t("client:admin.instance.none")
        ) : (
          <DateTime value={health.state_changed_at} />
        ),
    },
    {
      id: "failure_days",
      header: t("client:admin.instance.failure_days"),
      numeric: true,
      cell: (health) => t("client:admin.instance.failure_days.value", { count: health.failure_days }),
    },
    {
      id: "last_failure",
      header: t("client:admin.instance.last_failure"),
      cell: (health) =>
        health.last_failure_day === undefined ? (
          t("client:admin.instance.none")
        ) : (
          <Day value={health.last_failure_day} />
        ),
    },
  ];
  return (
    <RecordTable
      list={list}
      caption={t(filterLabels[filter])}
      columns={columns}
      getId={(health) => health.domain}
      headingLevel={2}
      empty={
        <EmptyState
          headingLevel={2}
          heading={t("client:admin.instances.empty")}
          description={t("client:admin.instances.empty.description")}
        />
      }
    />
  );
}
