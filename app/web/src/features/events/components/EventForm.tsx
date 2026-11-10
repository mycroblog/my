// SPDX-License-Identifier: AGPL-3.0-only
// The editor of an event (`/events/new`, `/events/{id}/edit`; ADR-0161 §4–§6, web-client/screens/event-editor.md):
// what it is (title, description), when (start and end as the wall clock of the event's time zone, and the zone),
// where (a place to meet in person, a link of another service, and who sees them), who sees it, and how many may go.
// Everything is saved together by the submit; the page of the event opens after it. Each group is a fieldset, so that
// the places of other kinds (the voice chat room) join as groups of their own.
import {
  ButtonLink,
  Checkbox,
  Fieldset,
  FormActionBar,
  Hint,
  InlineError,
  Link,
  Radio,
  RadioGroup,
  RegionError,
  Select,
  SelectItem,
  Stack,
  Text,
  TextArea,
  TextField,
  Button,
} from "@my/ui";
import type { ReactNode } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useTimeZoneOptions } from "../../settings";
import {
  type Event,
  eventLimits,
  eventPath,
  type EventPlaceDisclosure,
  eventPlaceDisclosures,
  type EventVisibility,
  eventVisibilities,
} from "../api/events";
import { type EventField, useEventForm } from "../hooks/useEventForm";
import { zoneCity } from "../hooks/eventTime";
import { useEventPage } from "../hooks/useEventPage";
import { EventPageSkeleton } from "./EventSkeletons";

/** The label of each visibility. */
const visibilityLabels: Readonly<Record<EventVisibility, ClientMessageKey>> = {
  public: "client:events.visibility.public",
  followers: "client:events.visibility.followers",
  following: "client:events.visibility.following",
  mutuals: "client:events.visibility.mutuals",
  invited: "client:events.visibility.invited",
};

const disclosureLabels: Readonly<Record<EventPlaceDisclosure, ClientMessageKey>> = {
  approved: "client:events.form.disclosure.approved",
  viewers: "client:events.form.disclosure.viewers",
};

export function EventForm({ event }: { event?: Event }) {
  const t = useT();
  const form = useEventForm(event);
  const { draft, errors } = form;
  const zones = useTimeZoneOptions(draft.timeZone);
  const fieldError = (field: EventField) =>
    errors[field] === undefined ? {} : { isInvalid: true, errorMessage: errors[field] };
  const formatRemaining = (remaining: number) =>
    remaining < 0
      ? t("client:events.form.characters_over", { count: -remaining })
      : t("client:events.form.characters_left", { count: remaining });
  return (
    <Stack as="form" gap="6" onSubmit={form.submit} noValidate>
      <Fieldset legend={t("client:events.form.about")}>
        <TextField
          label={t("client:events.form.title")}
          name="title"
          value={draft.title}
          onChange={(title) => {
            form.change({ title });
          }}
          dir="auto"
          isRequired
          autoComplete="off"
          {...fieldError("title")}
        />
        <TextArea
          label={t("client:events.form.description")}
          description={t("client:events.form.description.description")}
          name="description"
          value={draft.description}
          onChange={(description) => {
            form.change({ description });
          }}
          rows={5}
          characterLimit={eventLimits.description}
          formatRemaining={formatRemaining}
          {...fieldError("description")}
        />
      </Fieldset>
      <Fieldset legend={t("client:events.form.when")}>
        {form.unscheduled && <Text as="p">{t("client:events.form.unscheduled")}</Text>}
        <Select
          label={t("client:events.form.time_zone")}
          description={t("client:events.form.time_zone.description")}
          value={draft.timeZone}
          onChange={(key) => {
            if (typeof key === "string") form.change({ timeZone: key });
          }}
          items={zones.map((zone) => ({ ...zone, description: zone.id }))}
        >
          {(item) => (
            <SelectItem id={item.id} textValue={item.typeahead} description={item.description}>
              {item.label}
            </SelectItem>
          )}
        </Select>
        <TextField
          type="datetime-local"
          label={t("client:events.form.starts_at")}
          description={t("client:events.form.zone_note", { zone: zoneCity(draft.timeZone) })}
          name="starts_at"
          value={draft.startsAt}
          onChange={(startsAt) => {
            form.change({ startsAt });
          }}
          isRequired={!form.unscheduled}
          {...fieldError("starts_at")}
        />
        <TextField
          type="datetime-local"
          label={t("client:events.form.ends_at")}
          description={t("client:events.form.zone_note", { zone: zoneCity(draft.timeZone) })}
          name="ends_at"
          value={draft.endsAt}
          onChange={(endsAt) => {
            form.change({ endsAt });
          }}
          isRequired={!form.unscheduled}
          {...fieldError("ends_at")}
        />
      </Fieldset>
      <Fieldset legend={t("client:events.form.places")}>
        <Hint>{t("client:events.form.places.description")}</Hint>
        <Checkbox
          isSelected={draft.physical.enabled}
          onChange={(enabled) => {
            form.changePhysical({ enabled });
          }}
        >
          {t("client:events.form.physical")}
        </Checkbox>
        {draft.physical.enabled && (
          <Stack gap="4">
            <TextField
              label={t("client:events.form.physical.name")}
              name="physical_name"
              value={draft.physical.name}
              onChange={(name) => {
                form.changePhysical({ name });
              }}
              dir="auto"
              isRequired
              autoComplete="off"
              {...fieldError("physical_name")}
            />
            <TextArea
              label={t("client:events.form.physical.address")}
              name="physical_address"
              value={draft.physical.address}
              onChange={(address) => {
                form.changePhysical({ address });
              }}
              rows={3}
              characterLimit={eventLimits.address}
              formatRemaining={formatRemaining}
              {...fieldError("physical_address")}
            />
            <TextField
              type="url"
              label={t("client:events.form.physical.map")}
              description={t("client:events.form.url.description")}
              name="physical_url"
              value={draft.physical.url}
              onChange={(url) => {
                form.changePhysical({ url });
              }}
              dir="ltr"
              autoComplete="off"
              {...fieldError("physical_url")}
            />
          </Stack>
        )}
        <Checkbox
          isSelected={draft.link.enabled}
          onChange={(enabled) => {
            form.changeLink({ enabled });
          }}
        >
          {t("client:events.form.link")}
        </Checkbox>
        {draft.link.enabled && (
          <Stack gap="4">
            <TextField
              label={t("client:events.form.link.name")}
              name="link_name"
              value={draft.link.name}
              onChange={(name) => {
                form.changeLink({ name });
              }}
              dir="auto"
              isRequired
              autoComplete="off"
              {...fieldError("link_name")}
            />
            <TextField
              type="url"
              label={t("client:events.form.link.url")}
              description={t("client:events.form.url.description")}
              name="link_url"
              value={draft.link.url}
              onChange={(url) => {
                form.changeLink({ url });
              }}
              dir="ltr"
              isRequired
              autoComplete="off"
              {...fieldError("link_url")}
            />
          </Stack>
        )}
        {event?.places.some((place) => place.kind === "voice") === true && (
          <Hint>{t("client:events.form.voice_kept")}</Hint>
        )}
        <RadioGroup
          label={t("client:events.form.disclosure")}
          value={draft.placeDisclosure}
          onChange={(value) => {
            const placeDisclosure = eventPlaceDisclosures.find((d) => d === value);
            if (placeDisclosure !== undefined) form.change({ placeDisclosure });
          }}
        >
          {eventPlaceDisclosures.map((disclosure) => (
            <Radio key={disclosure} value={disclosure}>
              {t(disclosureLabels[disclosure])}
            </Radio>
          ))}
        </RadioGroup>
      </Fieldset>
      <Fieldset legend={t("client:events.form.audience")}>
        <RadioGroup
          label={t("client:events.form.visibility")}
          value={draft.visibility}
          isDisabled={!form.canChangeVisibility}
          {...(!form.canChangeVisibility && { description: t("client:events.form.visibility.organizer_only") })}
          onChange={(value) => {
            const visibility = eventVisibilities.find((v) => v === value);
            if (visibility !== undefined) form.change({ visibility });
          }}
        >
          {eventVisibilities.map((visibility) => (
            <Radio key={visibility} value={visibility}>
              {t(visibilityLabels[visibility])}
            </Radio>
          ))}
        </RadioGroup>
        <TextField
          inputMode="numeric"
          label={t("client:events.form.capacity")}
          description={t("client:events.form.capacity.description")}
          name="capacity"
          value={draft.capacity}
          onChange={(capacity) => {
            form.change({ capacity });
          }}
          autoComplete="off"
          {...fieldError("capacity")}
        />
      </Fieldset>
      {form.error !== undefined && (
        <InlineError size="inherit" alert>
          {t(form.mode === "new" ? "client:events.form.create_failed" : "client:events.form.save_failed", {
            reason: form.error,
          })}
        </InlineError>
      )}
      <FormActionBar>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t(form.mode === "new" ? "client:events.form.submit_new" : "client:events.form.submit_edit")}
        </Button>
        <ButtonLink href={event === undefined ? "/events" : eventPath(event.id)} variant="ghost">
          {t("client:events.form.cancel")}
        </ButtonLink>
      </FormActionBar>
    </Stack>
  );
}

export type EventEditViewProps = {
  id: string;
  /** Renders the page heading (the route's level-one heading) around its text. */
  renderHeading: (text: string) => ReactNode;
};

/** The editor of an existing event, once it loads; an event that the user does not see is not found. */
export function EventEditView({ id, renderHeading }: EventEditViewProps) {
  const t = useT();
  const page = useEventPage(id);
  switch (page.status) {
    case "loading":
    case "signed-out":
      return (
        <>
          {renderHeading(t("client:events.form.edit_title"))}
          <EventPageSkeleton />
        </>
      );
    case "error":
      return (
        <>
          {renderHeading(t("client:events.form.edit_title"))}
          <RegionError
            heading={t("client:events.page.failed")}
            reason={page.reason}
            retryLabel={t("client:events.retry")}
            onRetry={page.retry}
            isRetrying={page.isRetrying}
          />
        </>
      );
    case "not-found":
      return (
        <>
          {renderHeading(t("client:events.page.not_found"))}
          <Text as="p">{t("client:events.page.not_found.body")}</Text>
        </>
      );
    case "ready":
      // A guest, and anyone once the event is cancelled, has nothing to change: the way back is the event's page.
      if (page.role === "guest" || page.event.state === "cancelled") {
        return (
          <>
            {renderHeading(t("client:events.form.edit_title"))}
            <Text as="p">{t("client:events.form.not_editable")}</Text>
            <Text as="p">
              <Link href={eventPath(id)} variant="standalone">
                {t("client:events.form.back_to_event")}
              </Link>
            </Text>
          </>
        );
      }
      return (
        <>
          {renderHeading(t("client:events.form.edit_title"))}
          <EventForm event={page.event} />
        </>
      );
  }
}
