// SPDX-License-Identifier: AGPL-3.0-only

// A keyword filter's editor, the screen under the settings' entry Keyword filters at /settings/filters/new and
// /settings/filters/<id> (FR-209, ADR-0157 §5, §6, §9): a form saved as a whole by its submit bar, Add for a new
// filter and Save for one that is there.
//
// - Its name, which a folded post shows, its count near the limit of 50; what it does (folds a matching post behind a
//   warning, or hides it); and when it ends: never, after one of the durations offered, or at a date and time in the
//   reader's zone; a filter that has an end keeps it (No change) unless another is chosen, and one that ended says so
//   in words under its end, and that a new end turns it on again. While the form is sent, nothing in it can change.
// - Its words, each a word or a phrase in a field named by its number for assistive technology alone, with Whole word
//   and Remove on its line in a wide frame and under it on a phone (Remove at the row's end), in the order given:
//   added from the section's head (the new one takes the focus), removed by the row's own button (the focus goes to
//   the word that takes its place, to the one before it when it was the last, or to what adds one when none is left,
//   as after a field of the profile is removed). How many more the filter and all the reader's filters may take is
//   said under the heading as the room runs short, and adding stops at the limit, which says why.
// - Where it applies, the five places of the server, every one for a new filter, a checkbox each a finger's height.
// - Delete, after a confirmation, for a filter that is there.
//
// What is wrong stands under what it is about, which it describes, and the focus goes there, the error clear of the
// submit bar (`focusRefusedField`). What the form finds before sending (no name, no word, no place, a text too long,
// a time gone by) is marked all at once, each with its sentence, the focus on the first; each mark then goes as its
// value is put right, and the bar's sentence with the last. What the server refuses of a member it names stands in
// the words of the rule the value breaks where the form knows it (the server says only that it is not valid), else
// in the server's (what the filter does, its end, its places, its words as a whole), and the limit of a filter's
// words or of all of them under the words. The words as a whole are their list, which takes the focus, or what adds
// a word while there is none; no word written, with a row to write it in, is that row's field. The submit bar says
// only that nothing was saved.
// Once saved or deleted the screen goes back to the list, and app-data reads the open timelines again, where a
// matching post is then folded. Leaving with changes not saved is the app's to notice (`onUnsaved`): it holds the
// reader back and gives `leaving`, and the screen asks whether to leave.

import { localInput, localInstant } from "@my/app-components";
import {
  filterContexts,
  filterLimitOf,
  filterLimits,
  keywordFilterQuery,
  keywordFiltersQuery,
  useKeywordFilter,
  useKeywordFilters,
  useKeywordFilterWrites,
  type AppData,
  type FilterAction,
  type FilterContext,
  type KeywordFilter,
  type KeywordFilterRequest,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  CheckField,
  ConfirmDialog,
  DateTimeField,
  FeedNotice,
  FieldError,
  FieldNote,
  focusRefusedField,
  FormAlert,
  ModalPresence,
  Row,
  Select,
  SettingRow,
  SettingsEmpty,
  SettingsList,
  SettingsFieldRow,
  SettingsScreen,
  SettingsSection,
  Stack,
  SubmitBar,
  TextField,
  TextLink,
  useLocale,
  useTimeZone,
} from "@my/design-system";
import { useEffect, useId, useRef, useState } from "react";
import { placeRefusal, type FieldMembers } from "../field-errors";
import { stringsIn } from "../locale";
import { LimitCount } from "../profile-identity";
import { ReadingNotice } from "./controls";
import { filtersPath, useFilterDate } from "./filters";

type T = ReturnType<typeof stringsIn>["t"];

/** What the route's loader waits for: the reader's filters, and the one of `id` unless it is a new one. */
export async function loadFilterEdit(data: AppData, id: string | undefined): Promise<void> {
  await Promise.all([
    data.queryClient.query({ ...keywordFiltersQuery(data), staleTime: Infinity }),
    ...(id === undefined ? [] : [data.queryClient.query({ ...keywordFilterQuery(data, id), staleTime: Infinity })]),
  ]).catch(() => undefined);
}

/** The title of the screen: of a new filter (no `id`), or of one that is there. */
export function useFilterEditTitle(id: string | undefined): string {
  return stringsIn(useLocale().locale).t("settings.filters.edit.title", { kind: id === undefined ? "new" : "edit" });
}

export type FilterEditProps = {
  /** The ID of the filter; undefined for a new one. */
  id: string | undefined;
  /** Goes back to the list of filters: the way back, and where the screen goes once the filter is saved or deleted. */
  onBack: () => void;
  /** Hears whether the form holds changes that are not saved, as that comes and goes; false before it leaves. */
  onUnsaved?: ((unsaved: boolean) => void) | undefined;
  /**
   * The reader is leaving with changes not saved, and is held back: the screen asks whether to leave (`onLeave`, the
   * changes lost) or stay (`onStay`).
   */
  leaving?: { onStay: () => void; onLeave: () => void } | undefined;
};

/** The durations a filter may end after, from now. */
const durations = {
  m30: 30 * 60_000,
  h1: 60 * 60_000,
  h6: 6 * 60 * 60_000,
  h12: 12 * 60 * 60_000,
  d1: 24 * 60 * 60_000,
  w1: 7 * 24 * 60 * 60_000,
} as const;
type Duration = keyof typeof durations;

/** When a filter ends: as it does now (`keep`), never, after a duration, or at a date and time the reader chooses. */
type Ends = "keep" | "never" | Duration | "date";

/** A word of the form: what the reader typed, whether it matches whole words only, and a key no other word had. */
type Word = { readonly key: number; readonly keyword: string; readonly whole: boolean };

/** What the form holds. */
type Form = {
  readonly title: string;
  readonly words: readonly Word[];
  readonly contexts: readonly FilterContext[];
  readonly action: FilterAction;
  readonly ends: Ends;
  /** The date and time it ends at, as a `datetime-local` field holds it, for `ends` of `date`. */
  readonly date: string;
};

/** The form of `filter`, or of a new one: one empty word, every place, folding, never ending. */
const formOf = (filter: KeywordFilter | undefined): Form =>
  filter === undefined
    ? {
        title: "",
        words: [{ key: 0, keyword: "", whole: false }],
        contexts: filterContexts,
        action: "warn",
        ends: "never",
        date: "",
      }
    : {
        title: filter.title,
        words: filter.keywords.map((k, key) => ({ key, keyword: k.keyword, whole: k.whole_word })),
        contexts: filter.contexts,
        action: filter.action,
        ends: filter.expires_at === undefined ? "never" : "keep",
        date: "",
      };

/** What of the form is saved, to tell whether it changed: its words written, the places in the server's order. */
const savedOf = (form: Form) =>
  JSON.stringify([
    form.title.trim(),
    written(form.words).map((w) => [w.keyword.trim(), w.whole]),
    filterContexts.filter((c) => form.contexts.includes(c)),
    form.action,
    form.ends,
    form.ends === "date" ? form.date : "",
  ]);

/** The words to save: those with something written, in their order. */
const written = (words: readonly Word[]): readonly Word[] => words.filter((w) => w.keyword.trim() !== "");

/** The characters of a text as the server counts them: grapheme clusters, of the word once normalized. */
const graphemes = (text: string) =>
  [...new Intl.Segmenter(undefined, { granularity: "grapheme" }).segment(text)].length;

/**
 * What something wrong with the form is placed on: its name, what it does, its end (the choice, or the date and time
 * while it ends at one), its words as a whole, the word of a key, and its places.
 */
type Input = "title" | "action" | "ends" | "words" | `word:${number}` | "contexts";

/** Something wrong with the form: what it is about, and the sentence that stands under it. */
type Wrong = { readonly input: Input; readonly text: string };

/** What a member of the request answers to: an input, or the word at an index of the words sent. */
type Named = Exclude<Input, `word:${number}`> | `sent:${number}`;

/** Which input answers to which member of the request (`POST /filters`, `PATCH /filters/{id}`). */
const members: FieldMembers<Named> = {
  title: "title",
  keywords: "words",
  "keywords[]": (index) => `sent:${index}`,
  "keywords[].keyword": (index) => `sent:${index}`,
  "keywords[].whole_word": (index) => `sent:${index}`,
  contexts: "contexts",
  "contexts[]": () => "contexts",
  action: "action",
  expires_at: "ends",
};

const isSent = (named: Named): named is `sent:${number}` => named.startsWith("sent:");

/** The ids of what describes a control, in order, as `aria-describedby` takes them; undefined for none. */
const describedBy = (...ids: (string | false)[]): string | undefined =>
  ids.filter((id) => id !== false).join(" ") || undefined;

/**
 * A save that was not made: what is wrong, each thing under what it is about, in the form's order (`wrong`), and what
 * the submit bar says where that is more than that something is marked (`reason`).
 */
type Refusal = { readonly reason: string | undefined; readonly wrong: readonly Wrong[] };

/**
 * Where the form's last save is; one that was not made, `by` the form's own check before sending, which is made
 * again as the form changes, or by the server.
 */
type Sent =
  { readonly kind: "idle" | "sending" } | ({ readonly kind: "refused"; readonly by: "form" | "server" } & Refusal);

/**
 * A failed save placed on the form, whose words were sent in the order of `keys`: the limits of the words under them,
 * of the filters in the submit bar; a member the server names, under its input, in the words of the rule it breaks
 * where the form knows it (a name, a word: the server says only that the value is not valid), else in the server's;
 * the submit bar then says only that nothing was saved.
 */
function placed(error: unknown, keys: readonly number[], t: T): Refusal {
  const at = (input: Input, text: string): Refusal => ({ reason: undefined, wrong: [{ input, text }] });
  const limit = filterLimitOf(error);
  if (limit === "words")
    return at(
      "words",
      t("settings.filters.words.too_many", { filter: filterLimits.filterWords, max: filterLimits.words }),
    );
  if (limit === "filters") return { reason: t("settings.filters.full", { max: filterLimits.filters }), wrong: [] };
  const refused = placeRefusal(error, members);
  const named = refused.field;
  if (named === undefined) return { reason: refused.reason, wrong: [] };
  if (named.input === "title") return at("title", t("settings.filters.name.rule", { max: filterLimits.title }));
  if (isSent(named.input)) {
    // A word the form did not send is no row's: the rule stands under the words.
    const key = keys[Number(named.input.slice("sent:".length))];
    return at(key === undefined ? "words" : `word:${key}`, t("settings.filters.word.rule", { max: filterLimits.word }));
  }
  return at(named.input, named.text);
}

/**
 * What the form finds wrong before sending, every thing at once, each placed where it is, in the form's order; none
 * when it can be sent. No word written is the first row's to put right, or the words' as a whole while there is no
 * row.
 */
function checked(form: Form, timeZone: string | undefined, t: T): readonly Wrong[] {
  const wrong: Wrong[] = [];
  const title = form.title.trim();
  if (title === "") wrong.push({ input: "title", text: t("settings.filters.name.missing") });
  else if (graphemes(title) > filterLimits.title)
    wrong.push({ input: "title", text: t("settings.filters.name.over", { max: filterLimits.title }) });
  if (form.ends === "date") {
    const end = localInstant(form.date, timeZone);
    if (Number.isNaN(end.getTime())) wrong.push({ input: "ends", text: t("settings.filters.ends.date_missing") });
    else if (end.getTime() <= Date.now()) wrong.push({ input: "ends", text: t("settings.filters.ends.date_past") });
  }
  const words = written(form.words);
  const [row] = form.words;
  if (words.length === 0)
    wrong.push({
      input: row === undefined ? "words" : `word:${row.key}`,
      text: t("settings.filters.words.missing"),
    });
  for (const word of words)
    if (graphemes(word.keyword.trim().normalize("NFKC")) > filterLimits.word)
      wrong.push({ input: `word:${word.key}`, text: t("settings.filters.word.over", { max: filterLimits.word }) });
  if (form.contexts.length === 0) wrong.push({ input: "contexts", text: t("settings.filters.contexts.missing") });
  return wrong;
}

/** The request that saves `form`: a duration from now, a date in the reader's zone, or the end the filter has. */
function requestOf(form: Form, filter: KeywordFilter | undefined, timeZone: string | undefined): KeywordFilterRequest {
  const expires =
    form.ends === "never"
      ? undefined
      : form.ends === "keep"
        ? filter?.expires_at
        : form.ends === "date"
          ? localInstant(form.date, timeZone).toISOString()
          : new Date(Date.now() + durations[form.ends]).toISOString();
  return {
    title: form.title.trim(),
    action: form.action,
    contexts: filterContexts.filter((c) => form.contexts.includes(c)),
    keywords: written(form.words).map((w) => ({ keyword: w.keyword.trim(), whole_word: w.whole })),
    ...(expires === undefined ? {} : { expires_at: expires }),
  };
}

function EditForm({
  id: filterId,
  filter,
  othersWords,
  onBack,
  onUnsaved,
  leaving,
}: FilterEditProps & {
  filter: KeywordFilter | undefined;
  /** How many words the reader's other filters have. */
  othersWords: number;
}) {
  const { locale } = useLocale();
  const { t, lang } = stringsIn(locale);
  const timeZone = useTimeZone();
  const date = useFilterDate();
  const id = useId();
  const writes = useKeywordFilterWrites();
  const [base] = useState(() => formOf(filter));
  const [form, setForm] = useState<Form>(base);
  const [sent, setSent] = useState<Sent>({ kind: "idle" });
  const [deleting, setDeleting] = useState<"asking" | "busy" | undefined>(undefined);
  // The keys of the words the reader adds follow those the form opened with.
  const [opened] = useState(base.words.length);
  const nextKey = useRef(base.words.length);
  // What takes the focus once the form is drawn again: a word after one removed, or what adds one; and what a save
  // that was not made is about, whose error comes into view with it, clear of the submit bar.
  const focusNext = useRef<{ readonly id: string; readonly refused: boolean } | undefined>(undefined);
  useEffect(() => {
    const next = focusNext.current;
    if (next === undefined) return;
    focusNext.current = undefined;
    if (next.refused) focusRefusedField(next.id);
    else document.getElementById(next.id)?.focus();
  });

  const busy = sent.kind === "sending" || deleting === "busy";
  const wrong = sent.kind === "refused" ? sent.wrong : [];
  /** What is wrong with `input`, or undefined. */
  const wrongWith = (input: Input) => wrong.find((w) => w.input === input)?.text;
  const unsaved = savedOf(form) !== savedOf(base);
  useEffect(() => {
    onUnsaved?.(unsaved);
  }, [onUnsaved, unsaved]);
  useEffect(
    () => () => {
      onUnsaved?.(false);
    },
    [onUnsaved],
  );

  const titleId = `${id}-title`;
  const addId = `${id}-add`;
  const limitId = `${id}-limit`;
  const listId = `${id}-words`;
  const actionId = `${id}-action`;
  const endsId = `${id}-ends`;
  const dateId = `${id}-date`;
  const endedId = `${id}-ended`;
  const wordId = (key: number) => `${id}-word-${String(key)}`;
  const contextId = (context: FilterContext) => `${id}-context-${context}`;
  /**
   * The element of an input, which takes the focus when what is wrong is about it (`focusRefusedField`), its error at
   * `<its id>-error`: a text's field, the control of a setting's row (the date and time while the filter ends at
   * one, else the choice of its end), the list of the words or what adds one while there is none, the first place.
   */
  const elementOf = (input: Input): string => {
    if (input === "title") return titleId;
    if (input === "action") return `${actionId}-control`;
    if (input === "ends") return form.ends === "date" ? `${dateId}-control` : `${endsId}-control`;
    if (input === "words") return form.words.length === 0 ? addId : listId;
    if (input === "contexts") return contextId(filterContexts[0] ?? "home");
    return wordId(Number(input.slice("word:".length)));
  };
  const errorId = (input: Input) => `${elementOf(input)}-error`;
  const endedNote =
    filter?.expired === true && filter.expires_at !== undefined
      ? t("settings.filters.ends.ended", { date: date(filter.expires_at) })
      : undefined;

  /**
   * A change of the form. What the server said was about the form as it was, and goes; what the form found itself is
   * looked for again in the form as it now is, so that each mark goes as its value is put right, and the bar's
   * sentence with the last of them.
   */
  const change = (patch: Partial<Form>) => {
    const next = { ...form, ...patch };
    setForm(next);
    if (sent.kind !== "refused") return;
    const left = sent.by === "form" ? checked(next, timeZone, t) : [];
    setSent(left.length === 0 ? { kind: "idle" } : { ...sent, wrong: left });
  };

  /** A save that was not made: what is wrong is drawn, and the first of it takes the focus from the button. */
  const refuse = (by: "form" | "server", refusal: Refusal) => {
    setSent({ kind: "refused", by, ...refusal });
    const [first] = refusal.wrong;
    // What names nothing leaves the focus on the button, by the bar's alert.
    if (first !== undefined) focusNext.current = { id: elementOf(first.input), refused: true };
  };

  const save = () => {
    if (busy) return;
    const found = checked(form, timeZone, t);
    if (found.length > 0) {
      refuse("form", { reason: undefined, wrong: found });
      return;
    }
    const keys = written(form.words).map((w) => w.key);
    setSent({ kind: "sending" });
    writes.save(requestOf(form, filter, timeZone), filterId).then(
      () => {
        onUnsaved?.(false);
        onBack();
      },
      (error: unknown) => {
        refuse("server", placed(error, keys, t));
      },
    );
  };
  const remove = () => {
    if (filterId === undefined || deleting === "busy") return;
    setDeleting("busy");
    writes.remove(filterId).then(
      () => {
        onUnsaved?.(false);
        onBack();
      },
      () => {
        setDeleting(undefined);
        setSent({ kind: "refused", by: "server", reason: t("settings.filters.delete.failed"), wrong: [] });
      },
    );
  };

  // The room left for words: in this filter, and in all the reader's filters.
  const inFilter = filterLimits.filterWords - form.words.length;
  const inAll = filterLimits.words - othersWords - form.words.length;
  const room = Math.min(inFilter, inAll);
  const roomLine =
    room > 10
      ? undefined
      : room > 0
        ? t("settings.filters.words.left", { count: room })
        : inFilter <= 0
          ? t("settings.filters.words.filter_full", { max: filterLimits.filterWords })
          : t("settings.filters.words.all_full", { max: filterLimits.words });
  const addWord = () => {
    if (room <= 0) return;
    const key = nextKey.current;
    nextKey.current += 1;
    change({ words: [...form.words, { key, keyword: "", whole: false }] });
  };
  const wordsHeading = t("settings.filters.words");
  const actionError = wrongWith("action");
  const endsError = wrongWith("ends");
  const wordsError = wrongWith("words");
  const contextsError = wrongWith("contexts");
  // What adds a word is what the words' error is about while there is none.
  const addDescribed = describedBy(
    roomLine !== undefined && limitId,
    wordsError !== undefined && form.words.length === 0 && errorId("words"),
  );
  const add = (
    <Button
      id={addId}
      size="timeline"
      isDisabled={busy || room <= 0}
      onPress={addWord}
      {...(addDescribed === undefined ? {} : { "aria-describedby": addDescribed })}
    >
      {t("settings.filters.words.add")}
    </Button>
  );
  const isNew = filterId === undefined;

  const endsChoices: Ends[] = [
    ...(filter?.expires_at === undefined ? [] : (["keep"] as const)),
    "never",
    ...(Object.keys(durations) as Duration[]),
    "date",
  ];
  const endsLabel = (choice: Ends) =>
    choice === "keep"
      ? t("settings.filters.ends.keep", {
          state: filter?.expired === true ? "ended" : "ends",
          date: filter?.expires_at === undefined ? "" : date(filter.expires_at),
        })
      : t("settings.filters.ends.choice", { choice });

  return (
    <>
      <SettingsScreen
        sub
        title={t("settings.filters.edit.title", { kind: isNew ? "new" : "edit" })}
        back={<BackButton onPress={onBack}>{t("settings.entry.filters")}</BackButton>}
        foot={
          <SubmitBar
            result={
              sent.kind === "refused" ? (
                <FormAlert>
                  {sent.reason ??
                    t(sent.wrong.length === 0 ? "settings.filters.save.failed" : "settings.filters.save.refused")}
                </FormAlert>
              ) : undefined
            }
          >
            <Button kind="primary" busy={sent.kind === "sending"} onPress={save}>
              {t("settings.filters.save", { kind: isNew ? "new" : "edit" })}
            </Button>
          </SubmitBar>
        }
      >
        <SettingsSection heading={t("settings.filters.section.filter")}>
          <TextField
            id={titleId}
            name="title"
            label={t("settings.filters.name")}
            hint={t("settings.filters.name.hint")}
            defaultValue={form.title}
            onChange={(title) => {
              change({ title });
            }}
            readOnly={busy}
            end={<LimitCount left={filterLimits.title - graphemes(form.title.trim())} near={10} />}
            error={wrongWith("title")}
          />
          <SettingRow id={actionId} label={t("settings.filters.action")} form>
            <Select
              name="action"
              options={(["warn", "hide"] as const).map((action) => ({
                value: action,
                label: t("settings.filters.action.choice", { action }),
              }))}
              value={form.action}
              onChange={(value) => {
                change({ action: value === "hide" ? "hide" : "warn" });
              }}
              describedBy={actionError === undefined ? undefined : errorId("action")}
              isDisabled={busy}
            />
          </SettingRow>
          {actionError !== undefined && <FieldError id={errorId("action")}>{actionError}</FieldError>}
          <SettingRow id={endsId} label={t("settings.filters.ends")} form>
            <Select
              name="ends"
              options={endsChoices.map((choice) => ({ value: choice, label: endsLabel(choice) }))}
              value={form.ends}
              onChange={(value) => {
                const ends = endsChoices.find((choice) => choice === value) ?? "never";
                // A date to choose starts a day from now, in the reader's zone.
                change(
                  ends === "date" && form.date === ""
                    ? { ends, date: localInput(new Date(Date.now() + durations.d1), timeZone) }
                    : { ends },
                );
              }}
              describedBy={describedBy(
                endedNote !== undefined && endedId,
                endsError !== undefined && form.ends !== "date" && errorId("ends"),
              )}
              isDisabled={busy}
            />
          </SettingRow>
          {/* An expired filter says in words, under its end, that it acts nowhere and how it comes back. */}
          {endedNote !== undefined && <FieldNote id={endedId}>{endedNote}</FieldNote>}
          {form.ends === "date" && (
            <SettingRow id={dateId} label={t("settings.filters.ends.date")} form>
              <DateTimeField
                id={`${dateId}-control`}
                label={t("settings.filters.ends.date")}
                value={form.date}
                onChange={(value) => {
                  change({ date: value });
                }}
                min={localInput(new Date(), timeZone)}
                invalid={endsError !== undefined}
                describedBy={endsError === undefined ? undefined : errorId("ends")}
                disabled={busy}
              />
            </SettingRow>
          )}
          {endsError !== undefined && <FieldError id={errorId("ends")}>{endsError}</FieldError>}
        </SettingsSection>
        <SettingsSection
          heading={wordsHeading}
          count={form.words.length === 0 ? undefined : new Intl.NumberFormat(lang).format(form.words.length)}
          about={t("settings.filters.words.about")}
          action={form.words.length > 0 ? add : undefined}
        >
          {/* Under the heading, beside what adds a word, however long the list is. */}
          {roomLine !== undefined && (
            <Box as="p" id={limitId} text="caption" tone="muted">
              {roomLine}
            </Box>
          )}
          {form.words.length === 0 ? (
            <SettingsEmpty action={add}>{t("settings.filters.words.empty")}</SettingsEmpty>
          ) : (
            <SettingsList
              label={wordsHeading}
              id={listId}
              describedBy={wordsError === undefined ? undefined : errorId("words")}
            >
              {form.words.map((word, i) => (
                <SettingsFieldRow
                  key={word.key}
                  field={
                    <TextField
                      id={wordId(word.key)}
                      name="keyword"
                      // Its number is its name for assistive technology; the list's place says it to the eye.
                      label={t("settings.filters.word", { n: i + 1 })}
                      labelHidden
                      defaultValue={word.keyword}
                      onChange={(keyword) => {
                        change({ words: form.words.map((w) => (w.key === word.key ? { ...w, keyword } : w)) });
                      }}
                      readOnly={busy}
                      // A word the reader just added is theirs to write: the button that added it may be gone.
                      autoFocus={word.key >= opened}
                      error={wrongWith(`word:${word.key}`)}
                    />
                  }
                >
                  <CheckField
                    id={`${wordId(word.key)}-whole`}
                    name="whole_word"
                    value="1"
                    label={t("settings.filters.word.whole")}
                    checked={word.whole}
                    onChange={(whole) => {
                      change({ words: form.words.map((w) => (w.key === word.key ? { ...w, whole } : w)) });
                    }}
                    touch
                    disabled={busy}
                  />
                  <Button
                    size="timeline"
                    touch
                    isDisabled={busy}
                    aria-label={t("settings.filters.word.remove_named", { n: i + 1 })}
                    onPress={() => {
                      // The button leaves with its word: the focus goes to the word that takes its place, the one
                      // before it, or what adds a first one.
                      const after = form.words[i + 1] ?? form.words[i - 1];
                      focusNext.current = { id: after === undefined ? addId : wordId(after.key), refused: false };
                      change({ words: form.words.filter((w) => w.key !== word.key) });
                    }}
                  >
                    {t("settings.filters.word.remove")}
                  </Button>
                </SettingsFieldRow>
              ))}
            </SettingsList>
          )}
          {wordsError !== undefined && <FieldError id={errorId("words")}>{wordsError}</FieldError>}
        </SettingsSection>
        <SettingsSection heading={t("settings.filters.contexts")}>
          <Stack>
            {filterContexts.map((context) => (
              <CheckField
                key={context}
                id={contextId(context)}
                name="contexts"
                value={context}
                label={t("settings.filters.context", { context })}
                checked={form.contexts.includes(context)}
                onChange={(on) => {
                  change({
                    contexts: on ? [...form.contexts, context] : form.contexts.filter((c) => c !== context),
                  });
                }}
                describedBy={contextsError === undefined ? undefined : errorId("contexts")}
                touch
                disabled={busy}
              />
            ))}
          </Stack>
          {contextsError !== undefined && <FieldError id={errorId("contexts")}>{contextsError}</FieldError>}
        </SettingsSection>
        {!isNew && (
          <SettingsSection heading={t("settings.filters.delete")} about={t("settings.filters.delete.about")}>
            <Row justify="start">
              <Button
                size="timeline"
                isDisabled={busy}
                onPress={() => {
                  setDeleting("asking");
                }}
              >
                {t("settings.filters.delete.open")}
              </Button>
            </Row>
          </SettingsSection>
        )}
      </SettingsScreen>
      <ModalPresence>
        {deleting !== undefined ? (
          <ConfirmDialog
            key="delete"
            title={t("settings.filters.delete.title", { name: base.title })}
            cancel={t("settings.filters.delete.cancel")}
            confirm={t("settings.filters.delete.confirm")}
            busy={deleting === "busy"}
            onCancel={() => {
              if (deleting !== "busy") setDeleting(undefined);
            }}
            onConfirm={remove}
          >
            {t("settings.filters.delete.text")}
          </ConfirmDialog>
        ) : (
          leaving !== undefined && (
            <ConfirmDialog
              key="leave"
              title={t("settings.filters.leave.title")}
              cancel={t("settings.filters.leave.cancel")}
              confirm={t("settings.filters.leave.confirm")}
              onCancel={leaving.onStay}
              onConfirm={leaving.onLeave}
            >
              {t("settings.filters.leave.text")}
            </ConfirmDialog>
          )
        )}
      </ModalPresence>
    </>
  );
}

/** The screen: the form once the filter and the reader's other filters are read, and until then that it loads. */
export function FilterEdit({ id, onBack, onUnsaved, leaving }: FilterEditProps) {
  const { t } = stringsIn(useLocale().locale);
  const all = useKeywordFilters();
  const one = useKeywordFilter(id ?? "", id !== undefined);
  const title = useFilterEditTitle(id);
  const back = <BackButton onPress={onBack}>{t("settings.entry.filters")}</BackButton>;
  const gone = one.error !== null && "status" in one.error && one.error.status === 404;
  if (id !== undefined && gone)
    return (
      <SettingsScreen
        sub
        title={title}
        back={back}
        notice={
          <FeedNotice alert lead action={<TextLink href={filtersPath}>{t("settings.filters.all")}</TextLink>}>
            {t("settings.filters.not_found")}
          </FeedNotice>
        }
      />
    );
  const filter = id === undefined ? undefined : one.filter;
  if (all.status !== "success" || (id !== undefined && filter === undefined))
    return (
      <SettingsScreen
        sub
        title={title}
        back={back}
        notice={
          <ReadingNotice
            failed={all.status === "error" || one.error !== null}
            onRetry={() => {
              all.retry();
              one.retry();
            }}
          />
        }
      />
    );
  const othersWords = all.filters.filter((f) => f.id !== id).reduce((n, f) => n + f.keywords.length, 0);
  return (
    <EditForm
      key={id ?? "new"}
      id={id}
      filter={filter}
      othersWords={othersWords}
      onBack={onBack}
      onUnsaved={onUnsaved}
      leaving={leaving}
    />
  );
}
