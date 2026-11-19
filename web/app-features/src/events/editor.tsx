// SPDX-License-Identifier: AGPL-3.0-only

// The editor of an event (ADR-0161 §3–§7, §10; FR-940–942, FR-948, FR-952), a form saved as a whole:
//
// - Creating one, at /events/new, for a reader who may create events (FR-940): its title, its description, its date
//   (a fixed start and end, or the candidate dates of a poll in their place, chosen as it is created), its time zone
//   (the reader's by default), its places (a voice chat room with who speaks in it without asking, offered to a reader
//   who may host rooms; a place in person with its name, its address and a map's URL; a place online with its name and
//   URL), who sees the places in person and online, who sees the event, and its capacity. Create sends it, and the
//   event's page takes the form's place. Another reader is told they may not.
// - Changing one, at /events/{id}/edit, for its organizers: the same form with the event's settings and no choice of
//   how its date is set (a poll's dates are changed from its table, on the event's page); who sees the event is the
//   organizer's alone to change, and a co-organizer is told so. Save sends what changed, and goes back to the event.
//
// The dates are read in the event's own time zone, chosen right under them; a date left as it was keeps the moment the
// server holds (a wall time of a night whose clock goes back names two), and is not sent. What the form finds wrong before
// sending, by the server's own counts and limits, stands under its field, every one at once, the focus on the first;
// what the server says of a field stands there too, and what names none (not allowed, too many events this hour, voice
// chat off, a failure) before the button, in the frame's foot with it.

import { localInput, localInstant, nameOf } from "@my/app-components";
import {
  eventFaultField,
  eventLimits,
  eventPlaceDisclosures,
  eventRefusal,
  eventScheduleFault,
  eventVisibilities,
  isEventPlaceUrl,
  latestEventStart,
  useCreateEvent,
  useEvent,
  useEventWrites,
  useMayCreateEvents,
  useMayOpenVoiceRooms,
  type EventCreate,
  type EventPlace,
  type EventPlaceDisclosure,
  type EventUpdate,
  type EventVisibility,
  type ServerEvent,
  type VoiceRoomSpeakers,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  CheckField,
  ChoiceList,
  FeedNotice,
  FieldError,
  FieldLabel,
  focusRefusedField,
  FormAlert,
  GroupHeading,
  NestedFields,
  NumberField,
  Row,
  ScreenStack,
  ScreenTitle,
  Select,
  Stack,
  SubmitBar,
  TextButton,
  TextField,
  timeZoneName,
  useLocale,
  useTimeZone,
} from "@my/design-system";
import { useEffect, useEffectEvent, useId, useState, type ReactNode } from "react";
import { timeZoneOptions, withTimeZone } from "../time-zones";
import { DateField, Retry, useEventStrings, type T } from "./parts";

/** A start and an end as the form's fields hold them, in the event's time zone (2026-11-16T09:00). */
type Span = { readonly start: string; readonly end: string };

/** A candidate date of a new poll, with the key that keeps its fields as others are added and removed. */
type Candidate = Span & { readonly key: number };

/** What the form holds. */
type Draft = {
  readonly title: string;
  readonly description: string;
  /** How the date is set: fixed, or polled from candidates (a new event's choice). */
  readonly dating: "fixed" | "poll";
  readonly fixed: Span;
  readonly candidates: readonly Candidate[];
  readonly timeZone: string;
  readonly voice: boolean;
  readonly speakers: VoiceRoomSpeakers;
  readonly physical: boolean;
  readonly placeName: string;
  readonly address: string;
  readonly mapUrl: string;
  readonly online: boolean;
  readonly onlineName: string;
  readonly onlineUrl: string;
  readonly disclosure: EventPlaceDisclosure;
  readonly visibility: EventVisibility;
  readonly capacity: number;
  /** The moments the server holds of a fixed date (an event that is there): what a date left as it was sends. */
  readonly held?: { readonly startsAt: string; readonly endsAt: string } | undefined;
};

const speakerChoices: readonly VoiceRoomSpeakers[] = ["everyone", "followers", "following", "mutuals", "manual"];

const hour = 60 * 60_000;

/** A span of two hours that starts at the next whole hour a day from `now`, in `timeZone`. */
function spanFrom(now: number, timeZone: string, inDays = 1): Span {
  const start = Math.ceil((now + inDays * 24 * hour) / hour) * hour;
  return { start: localInput(new Date(start), timeZone), end: localInput(new Date(start + 2 * hour), timeZone) };
}

/** `span` a day later, read in `timeZone`: the next date a poll is offered after it. */
function dayAfter(span: Span, timeZone: string): Span {
  const later = (value: string) => localInput(new Date(localInstant(value, timeZone).getTime() + 24 * hour), timeZone);
  return { start: later(span.start), end: later(span.end) };
}

/** Which of the fixed date's fields still name the moment the server holds, read in the draft's zone. */
function heldParts(d: Draft): { start: boolean; end: boolean } {
  if (d.held === undefined) return { start: false, end: false };
  return {
    start: d.fixed.start === localInput(new Date(d.held.startsAt), d.timeZone),
    end: d.fixed.end === localInput(new Date(d.held.endsAt), d.timeZone),
  };
}

/** Whether the fixed date of `d` is another than the server holds: a new event's always is. */
function dateChanged(d: Draft): boolean {
  const held = heldParts(d);
  return d.held === undefined || !held.start || !held.end;
}

/** The draft of a new event: the server's defaults, the reader's zone, and a date tomorrow. */
function newDraft(timeZone: string): Draft {
  const now = Date.now();
  const span = spanFrom(now, timeZone);
  return {
    title: "",
    description: "",
    dating: "fixed",
    fixed: span,
    candidates: [
      { ...span, key: 0 },
      { ...spanFrom(now, timeZone, 2), key: 1 },
    ],
    timeZone,
    voice: false,
    speakers: "manual",
    physical: false,
    placeName: "",
    address: "",
    mapUrl: "",
    online: false,
    onlineName: "",
    onlineUrl: "",
    disclosure: "approved",
    visibility: "public",
    capacity: 0,
  };
}

/** The draft of the event as it is. */
function draftOf(event: ServerEvent): Draft {
  const voice = event.places.find((p) => p.kind === "voice");
  const physical = event.places.find((p) => p.kind === "physical");
  const online = event.places.find((p) => p.kind === "url");
  const zone = event.time_zone;
  const fixed = {
    start: localInput(new Date(event.starts_at), zone),
    end: localInput(new Date(event.ends_at), zone),
  };
  return {
    title: event.title,
    description: event.description,
    dating: event.state === "unscheduled" ? "poll" : "fixed",
    fixed,
    candidates: [],
    timeZone: zone,
    voice: voice !== undefined,
    speakers: voice?.speakers ?? "manual",
    physical: physical !== undefined,
    placeName: physical?.name ?? "",
    address: physical?.address ?? "",
    mapUrl: physical?.url ?? "",
    online: online !== undefined,
    onlineName: online?.name ?? "",
    onlineUrl: online?.url ?? "",
    disclosure: event.place_disclosure,
    visibility: event.visibility,
    capacity: event.capacity,
    ...(event.state === "unscheduled" ? {} : { held: { startsAt: event.starts_at, endsAt: event.ends_at } }),
  };
}

/** The fields of the form, in its order, each a field a fault can be under. */
const fieldOrder = [
  "title",
  "description",
  "starts_at",
  "ends_at",
  "candidates",
  "time_zone",
  "place_name",
  "address",
  "map_url",
  "online_name",
  "online_url",
  "places",
  "place_disclosure",
  "visibility",
  "capacity",
] as const;

type FieldKey = (typeof fieldOrder)[number];

/** What the form says is wrong: under each field, under each candidate's start or end, and before the button. */
type Faults = {
  readonly fields: Partial<Record<FieldKey, string>>;
  readonly candidates: ReadonlyMap<number, { readonly start?: string; readonly end?: string }>;
  readonly form: string | undefined;
};

const noFaults: Faults = { fields: {}, candidates: new Map(), form: undefined };

/** How many characters a reader sees in `text` (its graphemes), as the server counts a title. */
const graphemes = (text: string): number =>
  [...new Intl.Segmenter(undefined, { granularity: "grapheme" }).segment(text)].length;

/** How many code points `text` holds, as the server counts a text and bounds a title. */
const codePoints = (text: string): number => Array.from(text).length;

/** Whether `text` is a line the server takes for a name: something in it, no line break, at most `max` characters. */
const isLine = (text: string, max: number) => text.trim() !== "" && !/[\n\r]/u.test(text) && codePoints(text) <= max;

/** What the server refuses of the start and end of `span` in `timeZone` now, by the field: none for a date it takes. */
function spanFault(span: Span, timeZone: string, t: T): { start?: string; end?: string } {
  const fault = eventScheduleFault(localInstant(span.start, timeZone), localInstant(span.end, timeZone), new Date());
  if (fault === "starts_at") return { start: t("events.form.start_rule") };
  if (fault === "ends_at") return { end: t("events.form.end_rule") };
  return {};
}

/**
 * What the form finds wrong before sending, by the server's own limits: a title with nothing in it, more than 100
 * characters or a line break; a description longer than 3,000; a date out of the limits (the date of a poll's dates
 * each, and the same date twice); a place without a name or with a URL that is not an http or https one; `dated`,
 * whether the date is checked at all (a change that leaves it as it was).
 */
function checked(draft: Draft, dated: boolean, t: T): Faults {
  const fields: Partial<Record<FieldKey, string>> = {};
  const candidates = new Map<number, { start?: string; end?: string }>();
  const title = draft.title.trim();
  if (title === "") fields.title = t("events.form.title_empty");
  else if (!isLine(title, eventLimits.titleCodePoints) || graphemes(title) > eventLimits.title)
    fields.title = t("events.form.title_rule");
  if (codePoints(draft.description) > eventLimits.description) fields.description = t("events.form.description_rule");
  if (dated && draft.dating === "fixed") {
    const fault = spanFault(draft.fixed, draft.timeZone, t);
    if (fault.start !== undefined) fields.starts_at = fault.start;
    if (fault.end !== undefined) fields.ends_at = fault.end;
  }
  if (dated && draft.dating === "poll") {
    const seen = new Set<string>();
    for (const c of draft.candidates) {
      const fault = spanFault(c, draft.timeZone, t);
      const key = `${c.start}/${c.end}`;
      if (fault.start === undefined && fault.end === undefined && seen.has(key))
        candidates.set(c.key, { start: t("events.form.candidate_twice") });
      else if (fault.start !== undefined || fault.end !== undefined) candidates.set(c.key, fault);
      seen.add(key);
    }
  }
  if (draft.physical) {
    if (!isLine(draft.placeName, eventLimits.placeName)) fields.place_name = t("events.form.place_name_rule");
    if (codePoints(draft.address) > eventLimits.address) fields.address = t("events.form.address_rule");
    if (draft.mapUrl.trim() !== "" && !isEventPlaceUrl(draft.mapUrl.trim())) fields.map_url = t("events.form.url_rule");
  }
  if (draft.online) {
    if (!isLine(draft.onlineName, eventLimits.placeName)) fields.online_name = t("events.form.place_name_rule");
    if (!isEventPlaceUrl(draft.onlineUrl.trim())) fields.online_url = t("events.form.url_rule");
  }
  const found = Object.keys(fields).length > 0 || candidates.size > 0;
  return { fields, candidates, form: found ? t("events.form.not_sent") : undefined };
}

/** The places the draft holds, as the API takes them. */
function placesOf(draft: Draft, voiceOffered: boolean): EventPlace[] {
  const places: EventPlace[] = [];
  if (draft.voice && voiceOffered) places.push({ kind: "voice", speakers: draft.speakers });
  if (draft.physical) {
    const url = draft.mapUrl.trim();
    places.push({
      kind: "physical",
      name: draft.placeName.trim(),
      address: draft.address,
      ...(url === "" ? {} : { url }),
    });
  }
  if (draft.online) places.push({ kind: "url", name: draft.onlineName.trim(), url: draft.onlineUrl.trim() });
  return places;
}

/** The moment a field's value names in `timeZone`, as the API takes it. */
const isoIn = (value: string, timeZone: string) => localInstant(value, timeZone).toISOString();

/** The rule a field's value follows, said under it when the server refused it without words of its own. */
const fieldRules: Readonly<Record<string, FieldKey>> = {
  title: "title",
  description: "description",
  starts_at: "starts_at",
  ends_at: "ends_at",
  candidates: "candidates",
  time_zone: "time_zone",
  place_disclosure: "place_disclosure",
  visibility: "visibility",
  capacity: "capacity",
  places: "places",
};

const ruleWords: Readonly<Record<FieldKey, Parameters<T>[0]>> = {
  title: "events.form.title_rule",
  description: "events.form.description_rule",
  starts_at: "events.form.start_rule",
  ends_at: "events.form.end_rule",
  candidates: "events.form.candidates_rule",
  time_zone: "events.form.time_zone_rule",
  place_name: "events.form.place_name_rule",
  address: "events.form.address_rule",
  map_url: "events.form.url_rule",
  online_name: "events.form.place_name_rule",
  online_url: "events.form.url_rule",
  places: "events.form.places_rule",
  place_disclosure: "events.form.places_rule",
  visibility: "events.form.visibility_rule",
  capacity: "events.form.capacity_rule",
};

/**
 * What the server's refusal of a send says, by field and before the button: a field it names (a fixed date's start
 * of a poll's date refused is said under the dates), else why it refused the whole (`events.form.refused`).
 */
function refusalFaults(error: unknown, t: T, polled: boolean): Faults {
  const member = eventFaultField(error);
  const key = member === null ? undefined : fieldRules[member];
  if (key !== undefined) {
    const at: FieldKey = polled && (key === "starts_at" || key === "ends_at") ? "candidates" : key;
    return { fields: { [at]: t(ruleWords[at]) }, candidates: new Map(), form: t("events.form.not_sent") };
  }
  const refusal = eventRefusal(error);
  const kind =
    refusal === "not-eligible" ||
    refusal === "voice-unavailable" ||
    refusal === "rate-limited" ||
    refusal === "not-organizer" ||
    refusal === "organizer-only" ||
    refusal === "closed" ||
    refusal === "not-found" ||
    refusal === "scheduled"
      ? refusal.replaceAll("-", "_")
      : "other";
  return { fields: {}, candidates: new Map(), form: t("events.form.refused", { kind }) };
}

/** A choice of one among several, under its heading. */
function Choice<K extends string>({
  heading,
  keys,
  words,
  value,
  onChange,
  error,
  id,
  nested = false,
}: {
  heading: string;
  keys: readonly K[];
  words: (key: K) => string;
  value: K;
  onChange: (key: K) => void;
  error?: string | undefined;
  /** What takes the focus when the server refuses the choice, its error at `<id>-error`. */
  id?: string | undefined;
  /** A choice of what is checked above it (who speaks in the room), under a field's label, not a group's heading. */
  nested?: boolean | undefined;
}) {
  return (
    <Stack gap={nested ? "form-page-field-gap" : "entry-line"}>
      {nested ? (
        <Box as="p" weight="label">
          {heading}
        </Box>
      ) : (
        <GroupHeading>{heading}</GroupHeading>
      )}
      <ChoiceList
        id={id}
        describedBy={id === undefined || error === undefined ? undefined : `${id}-error`}
        label={heading}
        multiple={false}
        items={keys.map((key) => ({ key, label: words(key) }))}
        chosen={[keys.indexOf(value)]}
        onChange={([i]) => {
          const key = keys[i ?? -1];
          if (key !== undefined) onChange(key);
        }}
      />
      {error !== undefined && <FieldError id={id === undefined ? undefined : `${id}-error`}>{error}</FieldError>}
    </Stack>
  );
}

/** The editor's form, for a new event (`creating`) or one that is there. */
function EventForm({
  initial,
  creating,
  organizer,
  organizerName,
  voiceOffered,
  busy,
  refused,
  onEdit,
  submit,
  onSend,
  t,
}: {
  initial: Draft;
  creating: boolean;
  /** The reader is the organizer: they choose who sees the event. */
  organizer: boolean;
  organizerName: string;
  /** The voice chat room may be a place: the reader may host rooms, or the event has one already. */
  voiceOffered: boolean;
  busy: boolean;
  /** What the server refused of the last send, until the reader changes something (`onEdit`). */
  refused: Faults;
  onEdit: () => void;
  submit: string;
  onSend: (draft: Draft) => void;
  t: T;
}) {
  const id = useId();
  const { locale } = useLocale();
  const [draft, setDraft] = useState(initial);
  const [faults, setFaults] = useState<Faults>(noFaults);
  const [tries, setTries] = useState(0);
  const [nextKey, setNextKey] = useState(initial.candidates.length);
  const shown: Faults = faults.form !== undefined ? faults : refused;
  const dated = creating || draft.dating === "fixed";
  // A change that leaves the date as it was sends none, and checks none: the start of an event that started is past.
  const changedDate = (d: Draft) => creating || dateChanged(d);
  const change = (next: Partial<Draft>) => {
    let changed = { ...draft, ...next };
    // Another zone keeps the moments of a date left as it was: its fields say them in the zone chosen.
    if (next.timeZone !== undefined && changed.held !== undefined) {
      const held = heldParts(draft);
      changed = {
        ...changed,
        fixed: {
          start: held.start ? localInput(new Date(changed.held.startsAt), next.timeZone) : changed.fixed.start,
          end: held.end ? localInput(new Date(changed.held.endsAt), next.timeZone) : changed.fixed.end,
        },
      };
    }
    setDraft(changed);
    onEdit();
    // The form's own check goes again with each change, so a mark goes as its value is put right.
    if (faults.form !== undefined) setFaults(checked(changed, dated && changedDate(changed), t));
  };
  const fieldId = (key: FieldKey) => `${id}-${key}`;
  const candidateId = (key: number, part: "start" | "end") => `${id}-candidate-${String(key)}-${part}`;
  const firstFault = (): string | undefined => {
    for (const key of fieldOrder) {
      if (key === "candidates")
        for (const c of draft.candidates) {
          const f = shown.candidates.get(c.key);
          if (f?.start !== undefined) return candidateId(c.key, "start");
          if (f?.end !== undefined) return candidateId(c.key, "end");
        }
      if (shown.fields[key] !== undefined) return fieldId(key);
    }
    return undefined;
  };
  // Once a press found something wrong, or the server's answer to it did, never as the reader types.
  const focusFault = useEffectEvent(() => {
    const first = firstFault();
    if (first !== undefined) focusRefusedField(first);
  });
  useEffect(() => {
    focusFault();
  }, [tries, refused]);
  const now = new Date();
  const zone = draft.timeZone;
  const min = localInput(now, zone);
  const max = localInput(latestEventStart(now), zone);
  const zoneId = fieldId("time_zone");
  const zones = withTimeZone(timeZoneOptions(locale, t), zone);
  const hasShownPlace = draft.physical || draft.online;
  return (
    <form
      noValidate
      onSubmit={(e) => {
        e.preventDefault();
        if (busy) return;
        const found = checked(draft, dated && changedDate(draft), t);
        setFaults(found);
        setTries(tries + 1);
        if (found.form === undefined) onSend(draft);
      }}
    >
      <Stack gap="form-page-gap">
        <TextField
          id={fieldId("title")}
          name="title"
          label={t("events.form.title")}
          defaultValue={initial.title}
          required
          error={shown.fields.title}
          onChange={(title) => {
            change({ title });
          }}
        />
        <TextField
          id={fieldId("description")}
          name="description"
          label={t("events.form.description")}
          defaultValue={initial.description}
          multiline
          rows={4}
          hint={shown.fields.description === undefined ? t("events.form.description_hint") : undefined}
          error={shown.fields.description}
          onChange={(description) => {
            change({ description });
          }}
        />

        <Stack gap="entry-line">
          <GroupHeading>{t("events.form.when")}</GroupHeading>
          {creating && (
            <ChoiceList
              label={t("events.form.when")}
              multiple={false}
              items={[
                { key: "fixed", label: t("events.form.dating", { dating: "fixed" }) },
                { key: "poll", label: t("events.form.dating", { dating: "poll" }) },
              ]}
              chosen={[draft.dating === "fixed" ? 0 : 1]}
              onChange={([i]) => {
                change({ dating: i === 1 ? "poll" : "fixed" });
              }}
            />
          )}
          {draft.dating === "fixed" ? (
            <Row wrap gap="entry-room">
              <DateField
                id={fieldId("starts_at")}
                label={t("events.form.starts")}
                value={draft.fixed.start}
                onChange={(start) => {
                  change({ fixed: { ...draft.fixed, start } });
                }}
                min={min}
                max={max}
                error={shown.fields.starts_at}
                zoneId={zoneId}
              />
              <DateField
                id={fieldId("ends_at")}
                label={t("events.form.ends")}
                value={draft.fixed.end}
                onChange={(end) => {
                  change({ fixed: { ...draft.fixed, end } });
                }}
                min={draft.fixed.start || min}
                error={shown.fields.ends_at}
                zoneId={zoneId}
              />
            </Row>
          ) : creating ? (
            <Stack
              gap="entry-room"
              id={fieldId("candidates")}
              tabIndex={-1}
              role="group"
              aria-label={t("events.form.when")}
              aria-describedby={shown.fields.candidates === undefined ? undefined : `${fieldId("candidates")}-error`}
            >
              <Box as="p" text="body">
                {t("events.form.candidates_note", { count: eventLimits.candidates })}
              </Box>
              {draft.candidates.map((c, i) => {
                const fault = shown.candidates.get(c.key);
                const set = (part: Partial<Span>) => {
                  change({ candidates: draft.candidates.map((x) => (x.key === c.key ? { ...x, ...part } : x)) });
                };
                return (
                  <Stack
                    key={c.key}
                    gap="entry-line"
                    role="group"
                    aria-label={t("events.form.candidate", { n: i + 1 })}
                  >
                    <Row wrap align="baseline" justify="between" gap="entry-caption">
                      <GroupHeading>{t("events.form.candidate", { n: i + 1 })}</GroupHeading>
                      {draft.candidates.length > 1 && (
                        <TextButton
                          onPress={() => {
                            change({ candidates: draft.candidates.filter((x) => x.key !== c.key) });
                          }}
                        >
                          {t("events.form.candidate_remove", { n: i + 1 })}
                        </TextButton>
                      )}
                    </Row>
                    <Row wrap gap="entry-room">
                      <DateField
                        id={candidateId(c.key, "start")}
                        label={t("events.form.starts")}
                        value={c.start}
                        onChange={(start) => {
                          set({ start });
                        }}
                        min={min}
                        max={max}
                        error={fault?.start}
                        zoneId={zoneId}
                      />
                      <DateField
                        id={candidateId(c.key, "end")}
                        label={t("events.form.ends")}
                        value={c.end}
                        onChange={(end) => {
                          set({ end });
                        }}
                        min={c.start || min}
                        error={fault?.end}
                        zoneId={zoneId}
                      />
                    </Row>
                  </Stack>
                );
              })}
              {draft.candidates.length < eventLimits.candidates && (
                <Box>
                  <Button
                    size="target"
                    onPress={() => {
                      const last = draft.candidates.at(-1);
                      // A day after the last, never the same date twice.
                      const span = last === undefined ? spanFrom(Date.now(), zone) : dayAfter(last, zone);
                      setNextKey(nextKey + 1);
                      change({ candidates: [...draft.candidates, { ...span, key: nextKey }] });
                    }}
                  >
                    {t("events.form.candidate_add")}
                  </Button>
                </Box>
              )}
              {shown.fields.candidates !== undefined && (
                <FieldError id={`${fieldId("candidates")}-error`}>{shown.fields.candidates}</FieldError>
              )}
            </Stack>
          ) : (
            <Box as="p" text="body">
              {t("events.form.poll_elsewhere")}
            </Box>
          )}
          <Stack gap="form-page-field-gap">
            <FieldLabel htmlFor={fieldId("time_zone")}>{t("events.form.time_zone")}</FieldLabel>
            <Select
              id={fieldId("time_zone")}
              label={t("events.form.time_zone")}
              options={zones}
              value={zone}
              describedBy={shown.fields.time_zone === undefined ? undefined : `${fieldId("time_zone")}-error`}
              onChange={(timeZone) => {
                change({ timeZone });
              }}
            />
            {shown.fields.time_zone !== undefined && (
              <FieldError id={`${fieldId("time_zone")}-error`}>{shown.fields.time_zone}</FieldError>
            )}
          </Stack>
        </Stack>

        <Stack
          gap="entry-line"
          id={fieldId("places")}
          tabIndex={-1}
          role="group"
          aria-label={t("events.form.places")}
          aria-describedby={shown.fields.places === undefined ? undefined : `${fieldId("places")}-error`}
        >
          <GroupHeading>{t("events.form.places")}</GroupHeading>
          {voiceOffered && (
            <Stack gap="entry-line">
              <CheckField
                id={`${id}-voice`}
                name="voice"
                value="yes"
                touch
                label={t("events.form.voice")}
                checked={draft.voice}
                onChange={(voice) => {
                  change({ voice });
                }}
              />
              {draft.voice && (
                <NestedFields>
                  <Choice
                    nested
                    heading={t("events.form.speakers")}
                    keys={speakerChoices}
                    words={(speakers) => t("events.form.speakers_choice", { speakers })}
                    value={draft.speakers}
                    onChange={(speakers) => {
                      change({ speakers });
                    }}
                  />
                </NestedFields>
              )}
            </Stack>
          )}
          <Stack gap="entry-line">
            <CheckField
              id={`${id}-physical`}
              name="physical"
              value="yes"
              touch
              label={t("events.form.physical")}
              checked={draft.physical}
              onChange={(physical) => {
                change({ physical });
              }}
            />
            {draft.physical && (
              <NestedFields>
                <TextField
                  id={fieldId("place_name")}
                  name="place_name"
                  label={t("events.form.place_name")}
                  defaultValue={draft.placeName}
                  error={shown.fields.place_name}
                  onChange={(placeName) => {
                    change({ placeName });
                  }}
                />
                <TextField
                  id={fieldId("address")}
                  name="address"
                  label={t("events.form.address")}
                  defaultValue={draft.address}
                  multiline
                  rows={2}
                  error={shown.fields.address}
                  onChange={(address) => {
                    change({ address });
                  }}
                />
                <TextField
                  id={fieldId("map_url")}
                  name="map_url"
                  label={t("events.form.map_url")}
                  hint={shown.fields.map_url === undefined ? t("events.form.map_url_hint") : undefined}
                  defaultValue={draft.mapUrl}
                  inputMode="text"
                  autoCapitalize="none"
                  error={shown.fields.map_url}
                  onChange={(mapUrl) => {
                    change({ mapUrl });
                  }}
                />
              </NestedFields>
            )}
          </Stack>
          <Stack gap="entry-line">
            <CheckField
              id={`${id}-online`}
              name="online"
              value="yes"
              touch
              label={t("events.form.online")}
              checked={draft.online}
              onChange={(online) => {
                change({ online });
              }}
            />
            {draft.online && (
              <NestedFields>
                <TextField
                  id={fieldId("online_name")}
                  name="online_name"
                  label={t("events.form.place_name")}
                  defaultValue={draft.onlineName}
                  error={shown.fields.online_name}
                  onChange={(onlineName) => {
                    change({ onlineName });
                  }}
                />
                <TextField
                  id={fieldId("online_url")}
                  name="online_url"
                  label={t("events.form.online_url")}
                  hint={shown.fields.online_url === undefined ? t("events.form.online_url_hint") : undefined}
                  defaultValue={draft.onlineUrl}
                  autoCapitalize="none"
                  error={shown.fields.online_url}
                  onChange={(onlineUrl) => {
                    change({ onlineUrl });
                  }}
                />
              </NestedFields>
            )}
          </Stack>
          {shown.fields.places !== undefined && (
            <FieldError id={`${fieldId("places")}-error`}>{shown.fields.places}</FieldError>
          )}
        </Stack>

        {hasShownPlace && (
          <Choice
            id={fieldId("place_disclosure")}
            heading={t("events.form.disclosure")}
            keys={eventPlaceDisclosures}
            words={(disclosure) => t("events.form.disclosure_choice", { disclosure })}
            value={draft.disclosure}
            onChange={(disclosure) => {
              change({ disclosure });
            }}
            error={shown.fields.place_disclosure}
          />
        )}

        {organizer ? (
          <Choice
            id={fieldId("visibility")}
            heading={t("events.form.visibility")}
            keys={eventVisibilities}
            words={(visibility) => t("events.form.visibility_choice", { visibility })}
            value={draft.visibility}
            onChange={(visibility) => {
              change({ visibility });
            }}
            error={shown.fields.visibility}
          />
        ) : (
          <Stack gap="entry-line">
            <GroupHeading>{t("events.form.visibility")}</GroupHeading>
            <Box as="p" text="body" dir="auto">
              {t("events.form.visibility_organizer", {
                visibility: draft.visibility,
                organizer: organizerName,
              })}
            </Box>
          </Stack>
        )}

        <Stack gap="form-page-field-gap">
          <Box as="p" id={`${fieldId("capacity")}-label`} weight="label">
            {t("events.form.capacity")}
          </Box>
          <NumberField
            id={fieldId("capacity")}
            labelledBy={`${fieldId("capacity")}-label`}
            describedBy={
              shown.fields.capacity === undefined ? `${fieldId("capacity")}-hint` : `${fieldId("capacity")}-error`
            }
            value={draft.capacity}
            min={0}
            max={eventLimits.capacity}
            unit={t("events.form.capacity_unit", { count: draft.capacity })}
            onChange={(capacity) => {
              change({ capacity: capacity ?? 0 });
            }}
          />
          {shown.fields.capacity === undefined ? (
            <Box as="p" id={`${fieldId("capacity")}-hint`} text="caption" tone="muted">
              {t("events.form.capacity_hint", { max: eventLimits.capacity })}
            </Box>
          ) : (
            <FieldError id={`${fieldId("capacity")}-error`}>{shown.fields.capacity}</FieldError>
          )}
        </Stack>

        {/* The form's one action, held in the frame's foot where the thumb is, what was refused right before it. */}
        <SubmitBar result={shown.form === undefined ? undefined : <FormAlert regular>{shown.form}</FormAlert>}>
          <Button kind="primary" size="form" type="submit" busy={busy}>
            {submit}
          </Button>
        </SubmitBar>
      </Stack>
    </form>
  );
}

/** A new event from the draft. */
function createOf(draft: Draft, voiceOffered: boolean): EventCreate {
  const zone = draft.timeZone;
  const date =
    draft.dating === "fixed"
      ? { starts_at: isoIn(draft.fixed.start, zone), ends_at: isoIn(draft.fixed.end, zone) }
      : {
          candidates: draft.candidates.map((c) => ({
            starts_at: isoIn(c.start, zone),
            ends_at: isoIn(c.end, zone),
          })),
        };
  const places = placesOf(draft, voiceOffered);
  return {
    title: draft.title.trim(),
    description: draft.description,
    visibility: draft.visibility,
    time_zone: zone,
    place_disclosure: draft.disclosure,
    capacity: draft.capacity,
    ...(places.length === 0 ? {} : { places }),
    ...date,
  };
}

/** The change of `event` the draft makes: every setting, the date once it changed, the visibility its organizer's. */
function updateOf(event: ServerEvent, draft: Draft, voiceOffered: boolean): EventUpdate {
  const zone = draft.timeZone;
  const dated = draft.dating === "fixed" && dateChanged(draft);
  // A field left as it was sends the moment the server holds, which its wall time may name ambiguously.
  const held = heldParts(draft);
  const dates = {
    starts_at: held.start && draft.held !== undefined ? draft.held.startsAt : isoIn(draft.fixed.start, zone),
    ends_at: held.end && draft.held !== undefined ? draft.held.endsAt : isoIn(draft.fixed.end, zone),
  };
  return {
    title: draft.title.trim(),
    description: draft.description,
    time_zone: zone,
    place_disclosure: draft.disclosure,
    capacity: draft.capacity,
    places: placesOf(draft, voiceOffered),
    ...(event.viewer.organizer && draft.visibility !== event.visibility ? { visibility: draft.visibility } : {}),
    ...(dated ? dates : {}),
  };
}

/**
 * The form that creates an event, at /events/new, for a reader who may create events; another is told they may not.
 * Create sends it, and `onCreated` goes to its page; `onBack` to the events.
 */
export function EventCreateScreen({ onCreated, onBack }: { onCreated: (id: string) => void; onBack: () => void }) {
  const t = useEventStrings();
  const { may, failed, retry } = useMayCreateEvents();
  const voiceOffered = useMayOpenVoiceRooms();
  const create = useCreateEvent();
  const zone = timeZoneName(useTimeZone());
  const [busy, setBusy] = useState(false);
  const [refused, setRefused] = useState<Faults>(noFaults);
  const back = <BackButton onPress={onBack}>{t("events.back")}</BackButton>;
  const head = (
    <ScreenTitle back={back} glyph="events">
      {t("events.create")}
    </ScreenTitle>
  );
  // Whether the reader may is read from the instance and their account: the form waits for both.
  if (may === undefined)
    return (
      <ScreenStack busy={!failed}>
        {head}
        {failed ? (
          <FeedNotice alert action={<Retry onPress={retry} t={t} />}>
            {t("events.create.failed")}
          </FeedNotice>
        ) : (
          <FeedNotice>{t("events.create.loading")}</FeedNotice>
        )}
      </ScreenStack>
    );
  return (
    <ScreenStack>
      {head}
      {may ? (
        <EventForm
          initial={newDraft(zone)}
          creating
          organizer
          organizerName=""
          voiceOffered={voiceOffered}
          busy={busy}
          refused={refused}
          onEdit={() => {
            setRefused(noFaults);
          }}
          submit={t("events.create.submit")}
          onSend={(draft) => {
            setBusy(true);
            create(createOf(draft, voiceOffered)).then(
              (event) => {
                onCreated(event.id);
              },
              (error: unknown) => {
                setBusy(false);
                setRefused(refusalFaults(error, t, draft.dating === "poll"));
              },
            );
          }}
          t={t}
        />
      ) : (
        <FeedNotice lead>{t("events.create.not_allowed")}</FeedNotice>
      )}
    </ScreenStack>
  );
}

/** The form of the event that is there, once it is read: its writes, and whether the reader may host rooms. */
function EditForm({ event, onSaved, t }: { event: ServerEvent; onSaved: () => void; t: T }) {
  const writes = useEventWrites(event.id);
  const mayHost = useMayOpenVoiceRooms();
  // The server checks the organizer's right to host rooms: a co-organizer keeps a voice place the event has, no more.
  const voiceOffered = event.places.some((p) => p.kind === "voice") || (event.viewer.organizer && mayHost);
  const [initial] = useState(() => draftOf(event));
  const [busy, setBusy] = useState(false);
  const [refused, setRefused] = useState<Faults>(noFaults);
  return (
    <EventForm
      initial={initial}
      creating={false}
      organizer={event.viewer.organizer}
      organizerName={nameOf(event.organizer)}
      voiceOffered={voiceOffered}
      busy={busy}
      refused={refused}
      onEdit={() => {
        setRefused(noFaults);
      }}
      submit={t("events.edit.submit")}
      onSend={(draft) => {
        setBusy(true);
        writes.update(updateOf(event, draft, voiceOffered)).then(onSaved, (error: unknown) => {
          setBusy(false);
          setRefused(refusalFaults(error, t, false));
        });
      }}
      t={t}
    />
  );
}

/**
 * The form that changes the event of `id`, at /events/{id}/edit, for its organizers; Save sends it and goes back to
 * the event (`onBack`, also the way back). Another reader is told that only its organizers change it, and an event
 * that was cancelled says that it is no longer changed.
 */
export function EventEditScreen({ id, onBack }: { id: string; onBack: () => void }) {
  const t = useEventStrings();
  const { event, error, gone, refetch } = useEvent(id);
  const back = <BackButton onPress={onBack}>{t("events.back_to_event")}</BackButton>;
  const head = (
    <ScreenTitle back={back} glyph="events">
      {t("events.edit")}
    </ScreenTitle>
  );
  const notice = (words: ReactNode) => (
    <ScreenStack>
      {head}
      {words}
    </ScreenStack>
  );
  if (gone) return notice(<FeedNotice lead>{t("events.gone")}</FeedNotice>);
  if (event === undefined)
    return error === null ? (
      <ScreenStack busy>
        {head}
        <FeedNotice>{t("events.event_loading")}</FeedNotice>
      </ScreenStack>
    ) : (
      notice(
        <FeedNotice alert action={<Retry onPress={refetch} t={t} />}>
          {t("events.event_failed")}
        </FeedNotice>,
      )
    );
  if (!(event.viewer.organizer || event.viewer.coorganizer))
    return notice(<FeedNotice lead>{t("events.organizers_only", { organizer: nameOf(event.organizer) })}</FeedNotice>);
  if (event.state === "cancelled") return notice(<FeedNotice lead>{t("events.edit.cancelled")}</FeedNotice>);
  return (
    <ScreenStack>
      {head}
      <EditForm key={event.id} event={event} onSaved={onBack} t={t} />
    </ScreenStack>
  );
}
