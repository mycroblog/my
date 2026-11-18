// SPDX-License-Identifier: AGPL-3.0-only

// A keyword filter's editor, the screen under the settings' entry Keyword filters at /settings/filters/new and
// /settings/filters/<id> (FR-209, ADR-0157 §5, §6, §9): a form saved as a whole by its submit bar, Add for a new
// filter and Save for one that is there.
//
// - Its name, which a folded post shows, its count near the limit of 50.
// - Its words, each a word or a phrase with whether it matches whole words only, in the order given: added from the
//   section's head (the new one takes the focus), removed by the row's own button (the focus goes to the word that
//   takes its place, or to what adds one). How many more the filter and all the reader's filters may take is said as
//   the room runs short, and adding stops at the limit, which says why.
// - Where it applies, the five places of the server, every one for a new filter.
// - What it does (folds a matching post behind a warning, or hides it) and when it ends: never, after one of the
//   durations offered, or at a date and time in the reader's zone; a filter that has an end keeps it unless another is
//   chosen, which brings an expired one back.
// - Delete, after a confirmation, for a filter that is there.
//
// What is wrong stands under the input it is about, and the focus goes there: what the form finds before sending (no
// name, no word, no place, a text too long, a time gone by), and what the server refuses of a member it names, or of
// the words when they pass the limit of a filter or of all of them; the submit bar says only that nothing was saved.
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
  FormAlert,
  ModalPresence,
  Row,
  Select,
  SettingRow,
  SettingsEmpty,
  SettingsList,
  SettingsListItem,
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
import { fieldError, placeRefusal, type FieldMembers, type Refused } from "../field-errors";
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

/** The inputs of the form, which what is wrong is placed on. */
type Input = "title" | "words" | "contexts" | "action" | "ends" | `word:${number}`;

/** Which input answers to which member of the request (`POST /filters`, `PATCH /filters/{id}`). */
const members: FieldMembers<Input> = {
  title: "title",
  keywords: "words",
  "keywords[]": (index) => `word:${index}`,
  "keywords[].keyword": (index) => `word:${index}`,
  "keywords[].whole_word": (index) => `word:${index}`,
  contexts: "contexts",
  "contexts[]": () => "contexts",
  action: "action",
  expires_at: "ends",
};

/** Where the form's last save is; one refused, or stopped before sending, with the keys of the words it held. */
type Sent =
  | { readonly kind: "idle" | "sending" }
  | { readonly kind: "refused"; readonly refused: Refused<Input>; readonly keys: readonly number[] };

/** A failed save placed on the form: the limits of the words under them, of the filters in the submit bar. */
function placed(error: unknown, t: T): Refused<Input> {
  const limit = filterLimitOf(error);
  if (limit === "words")
    return {
      reason: undefined,
      field: {
        input: "words",
        text: t("settings.filters.words.too_many", { filter: filterLimits.filterWords, max: filterLimits.words }),
      },
    };
  if (limit === "filters") return { reason: t("settings.filters.full", { max: filterLimits.filters }) };
  return placeRefusal(error, members);
}

/** What the form finds wrong before sending, the first thing, placed where it is; undefined when it can be sent. */
function checked(form: Form, timeZone: string | undefined, t: T): Refused<Input> | undefined {
  const at = (input: Input, text: string): Refused<Input> => ({ reason: undefined, field: { input, text } });
  const word = (index: number): Input => `word:${index}`;
  const title = form.title.trim();
  if (title === "") return at("title", t("settings.filters.name.missing"));
  if (graphemes(title) > filterLimits.title)
    return at("title", t("settings.filters.name.over", { max: filterLimits.title }));
  const words = written(form.words);
  if (words.length === 0) return at("words", t("settings.filters.words.missing"));
  const long = words.findIndex((w) => graphemes(w.keyword.trim().normalize("NFKC")) > filterLimits.word);
  if (long >= 0) return at(word(long), t("settings.filters.word.over", { max: filterLimits.word }));
  if (form.contexts.length === 0) return at("contexts", t("settings.filters.contexts.missing"));
  if (form.ends === "date") {
    const end = localInstant(form.date, timeZone);
    if (Number.isNaN(end.getTime())) return at("ends", t("settings.filters.ends.date_missing"));
    if (end.getTime() <= Date.now()) return at("ends", t("settings.filters.ends.date_past"));
  }
  return undefined;
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
  // What takes the focus once the form is drawn again: a word after one removed, what adds one, or what was refused.
  const focusNext = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (focusNext.current === undefined) return;
    document.getElementById(focusNext.current)?.focus();
    focusNext.current = undefined;
  });

  const busy = sent.kind === "sending" || deleting === "busy";
  const refused = sent.kind === "refused" ? sent.refused : undefined;
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
  const contextsErrorId = `${id}-contexts-error`;
  const endsErrorId = `${id}-ends-error`;
  const actionId = `${id}-action`;
  const endsId = `${id}-ends`;
  const dateId = `${id}-date`;
  const wordId = (key: number) => `${id}-word-${String(key)}`;
  const contextId = (context: FilterContext) => `${id}-context-${context}`;

  /** A change of the form: what the last save said was about the form as it was. */
  const change = (next: Partial<Form>) => {
    setForm((now) => ({ ...now, ...next }));
    setSent((was) => (was.kind === "refused" ? { kind: "idle" } : was));
  };

  /** The element of an input that was refused, which takes the focus. */
  const elementOf = (input: Input, keys: readonly number[]): string => {
    if (input === "title") return titleId;
    if (input === "contexts") return contextId(form.contexts[0] ?? "home");
    if (input === "action") return `${actionId}-control`;
    if (input === "ends") return `${endsId}-control`;
    if (input === "words") {
      const first = form.words[0];
      return first === undefined ? addId : wordId(first.key);
    }
    const key = keys[Number(input.slice("word:".length))];
    return key === undefined ? addId : wordId(key);
  };
  const refuse = (r: Refused<Input>, keys: readonly number[]) => {
    setSent({ kind: "refused", refused: r, keys });
    if (r.field !== undefined) focusNext.current = elementOf(r.field.input, keys);
  };

  const sentKeys = written(form.words).map((w) => w.key);
  const save = () => {
    if (busy) return;
    const wrong = checked(form, timeZone, t);
    if (wrong !== undefined) {
      refuse(wrong, sentKeys);
      return;
    }
    setSent({ kind: "sending" });
    writes.save(requestOf(form, filter, timeZone), filterId).then(
      () => {
        onUnsaved?.(false);
        onBack();
      },
      (error: unknown) => {
        refuse(placed(error, t), sentKeys);
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
        setSent({ kind: "refused", refused: { reason: t("settings.filters.delete.failed") }, keys: sentKeys });
      },
    );
  };

  /** What was refused of the word of `key`, placed by its place among the words sent. */
  const wordError = (key: number) => {
    const at = sent.kind === "refused" ? sent.keys.indexOf(key) : -1;
    return at < 0 ? undefined : fieldError<Input>(refused, `word:${at}`);
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
  const add = (
    <Button
      id={addId}
      size="timeline"
      isDisabled={busy || room <= 0}
      onPress={addWord}
      {...(roomLine === undefined ? {} : { "aria-describedby": limitId })}
    >
      {t("settings.filters.words.add")}
    </Button>
  );
  const wordsHeading = t("settings.filters.words");
  const wordsError = fieldError(refused, "words");
  const contextsError = fieldError(refused, "contexts");
  const endsError = fieldError(refused, "ends");
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
                <FormAlert>{sent.refused.reason ?? t("settings.filters.save.failed")}</FormAlert>
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
            error={fieldError(refused, "title")}
          />
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
            <SettingsList label={wordsHeading}>
              {form.words.map((word, i) => (
                <SettingsListItem key={word.key}>
                  <Row wrap align="end" gap="screen-row-gap">
                    <Box grow="names">
                      <TextField
                        id={wordId(word.key)}
                        name="keyword"
                        label={t("settings.filters.word", { n: i + 1 })}
                        defaultValue={word.keyword}
                        onChange={(keyword) => {
                          change({ words: form.words.map((w) => (w.key === word.key ? { ...w, keyword } : w)) });
                        }}
                        readOnly={busy}
                        // A word the reader just added is theirs to write: the button that added it may be gone.
                        autoFocus={word.key >= opened}
                        error={wordError(word.key)}
                      />
                    </Box>
                    <Row align="center" gap="screen-row-gap">
                      <CheckField
                        id={`${wordId(word.key)}-whole`}
                        name="whole_word"
                        value="1"
                        label={t("settings.filters.word.whole")}
                        checked={word.whole}
                        onChange={(whole) => {
                          change({ words: form.words.map((w) => (w.key === word.key ? { ...w, whole } : w)) });
                        }}
                      />
                      <Button
                        size="timeline"
                        isDisabled={busy}
                        aria-label={t("settings.filters.word.remove_named", { n: i + 1 })}
                        onPress={() => {
                          // The button leaves with its word: the focus goes to the word that takes its place, the one
                          // before it, or what adds a first one.
                          const after = form.words[i + 1] ?? form.words[i - 1];
                          focusNext.current = after === undefined ? addId : wordId(after.key);
                          change({ words: form.words.filter((w) => w.key !== word.key) });
                        }}
                      >
                        {t("settings.filters.word.remove")}
                      </Button>
                    </Row>
                  </Row>
                </SettingsListItem>
              ))}
            </SettingsList>
          )}
          {wordsError !== undefined && <FieldError>{wordsError}</FieldError>}
        </SettingsSection>
        <SettingsSection heading={t("settings.filters.contexts")}>
          <Stack gap="screen-row-gap">
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
                describedBy={contextsError === undefined ? undefined : contextsErrorId}
              />
            ))}
          </Stack>
          {contextsError !== undefined && <FieldError id={contextsErrorId}>{contextsError}</FieldError>}
        </SettingsSection>
        <SettingsSection heading={t("settings.filters.section.does")}>
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
              isDisabled={busy}
            />
          </SettingRow>
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
              describedBy={endsError === undefined || form.ends === "date" ? undefined : endsErrorId}
              isDisabled={busy}
            />
          </SettingRow>
          {form.ends === "date" && (
            <SettingRow id={dateId} label={t("settings.filters.ends.date")} form>
              <DateTimeField
                label={t("settings.filters.ends.date")}
                value={form.date}
                onChange={(value) => {
                  change({ date: value });
                }}
                min={localInput(new Date(), timeZone)}
                invalid={endsError !== undefined}
                describedBy={endsError === undefined ? undefined : endsErrorId}
              />
            </SettingRow>
          )}
          {endsError !== undefined && <FieldError id={endsErrorId}>{endsError}</FieldError>}
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
