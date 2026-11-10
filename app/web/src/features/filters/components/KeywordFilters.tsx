// SPDX-License-Identifier: AGPL-3.0-only
// The keyword filters page below its heading (web-client/screens/settings-filters.md, ADR-0157): the list-and-add part
// (`ListSection`) with the user's filters, oldest first, its count over the most filters, and the button that opens the
// form that adds one (a note in its place once the user has the most). Each row has the filter's name, what it does,
// whether it expired, where it applies, its words and its end, with "Edit" (the form in the row's place) and "Delete"
// (after an alert dialog: its words are lost). What adding did shows above the rows; saving and deleting are announced,
// and focus goes back to the row's button, or to the next row (web-client/16-look-states-and-wording.md G24).
import {
  Box,
  Button,
  Checkbox,
  Cluster,
  ConfirmDialog,
  ExternalLink,
  Fieldset,
  FormActionBar,
  Heading,
  Hint,
  ListSection,
  Radio,
  RadioGroup,
  Select,
  SelectItem,
  Stack,
  Tag,
  Text,
  TextField,
} from "@my/ui";
import { useEffect, useId, useRef, useState } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { filterContexts, filterInputLimit, type KeywordFilter, maxFilters } from "../api/filters";
import {
  type DeleteFilter,
  type Expiry,
  expiryChoices,
  type FilterForm,
  type FilterSave,
  isolate,
  useContextLabels,
  useCreateFilter,
  useDeleteFilter,
  useFilterForm,
  useFilterLimits,
  useKeywordFilterList,
  useUpdateFilter,
  useWordMatchingManual,
} from "../hooks/useKeywordFilters";

/** How many words a row shows before it counts the rest. */
const shownWords = 5;

export function KeywordFilters() {
  const t = useT();
  const limits = useFilterLimits();
  const filters = useKeywordFilterList();
  const create = useCreateFilter();
  const remove = useDeleteFilter();
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    filters.map((filter) => filter.id),
  );
  const [confirming, setConfirming] = useState<KeywordFilter | null>(null);
  return (
    <>
      <ListSection
        title={t("client:filters.list.title")}
        description={t("client:filters.description")}
        items={filters}
        itemKey={(filter) => filter.id}
        renderItem={(filter) => (
          <FilterRow
            filter={filter}
            remove={remove}
            onDelete={() => {
              setConfirming(filter);
            }}
          />
        )}
        listRef={listRef}
        countLabel={t("client:filters.count", { count: filters.length, max: limits.filters })}
        limit={maxFilters}
        empty={{ heading: t("client:filters.empty"), description: t("client:filters.empty.description") }}
        add={
          create.isFull
            ? { unavailable: t("client:filters.limit", { max: limits.filters }) }
            : {
                label: t("client:filters.add"),
                panel: (close) => (
                  <FilterFormFields
                    filter={undefined}
                    saver={create}
                    submitLabel={t("client:filters.form.add")}
                    close={close}
                  />
                ),
              }
        }
        result={create.status === undefined ? undefined : { tone: "success", message: create.status }}
      />
      <ConfirmDialog
        isOpen={confirming !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) setConfirming(null);
        }}
        title={t("client:filters.delete.title", { title: isolate(confirming?.title ?? "") })}
        description={t("client:filters.delete.description")}
        confirmLabel={t("client:filters.delete")}
        cancelLabel={t("client:filters.delete.cancel")}
        tone="danger"
        isPending={confirming !== null && remove.pendingId === confirming.id}
        onConfirm={(close) => {
          if (confirming === null) return;
          remove.run(confirming, (deleted) => {
            if (deleted) removed(confirming.id);
            close();
          });
        }}
      />
    </>
  );
}

type FilterFormFieldsProps = {
  /** The filter to change, or undefined for a new one. */
  filter: KeywordFilter | undefined;
  saver: FilterSave;
  submitLabel: string;
  /** Closes the form; called once the filter is saved too. */
  close: () => void;
};

/**
 * The fields of a filter and the form's action bar: the panel that adds a filter, and the row that changes one. The
 * fields follow the browser's constraint validation (React Aria's native validation): an empty or too long name or
 * word is reported under its field when the form is sent.
 */
function FilterFormFields({ filter, saver, submitLabel, close }: FilterFormFieldsProps) {
  const t = useT();
  const limits = useFilterLimits();
  const form = useFilterForm(filter, saver);
  return (
    <Stack
      as="form"
      onSubmit={(event) => {
        form.submit(event, close);
      }}
      gap="4"
    >
      <Box width="sm">
        <TextField
          label={t("client:filters.form.title")}
          description={t("client:filters.form.title.description", { max: limits.title })}
          value={form.title}
          onChange={form.setTitle}
          isRequired
          maxLength={filterInputLimit}
          autoComplete="off"
          validate={form.titleProblem}
        />
      </Box>
      <FilterWords form={form} />
      <FilterContexts form={form} />
      <RadioGroup
        label={t("client:filters.form.action")}
        value={form.action}
        onChange={(value) => {
          if (value === "warn" || value === "hide") form.setAction(value);
        }}
      >
        <Radio value="warn" description={t("client:filters.form.action.warn.description")}>
          {t("client:filters.action.warn")}
        </Radio>
        <Radio value="hide" description={t("client:filters.form.action.hide.description")}>
          {t("client:filters.action.hide")}
        </Radio>
      </RadioGroup>
      <FilterExpiry form={form} />
      <FormActionBar result={form.error === undefined ? undefined : { tone: "danger", message: form.error }}>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {submitLabel}
        </Button>
        <Button onPress={close}>{t("client:filters.form.cancel")}</Button>
      </FormActionBar>
    </Stack>
  );
}

/**
 * The words of the form, each with its "whole word" checkbox and its remove button, the button that adds one, and the
 * one line about whole words in languages without spaces, which links to the user manual (ADR-0157 §5).
 */
function FilterWords({ form }: { form: FilterForm }) {
  const t = useT();
  const limits = useFilterLimits();
  const manual = useWordMatchingManual();
  return (
    <Fieldset legend={t("client:filters.form.words")}>
      {form.words.map((word, index) => {
        const number = String(index + 1);
        return (
          <Stack key={word.key} gap="2" rowAt="md">
            <Box width="sm">
              <TextField
                label={t("client:filters.form.word", { number })}
                value={word.keyword}
                onChange={(keyword) => {
                  form.changeWord(word.key, { keyword });
                }}
                isRequired
                maxLength={filterInputLimit}
                autoComplete="off"
                autoCorrect="off"
                spellCheck="false"
                validate={form.wordProblem}
              />
            </Box>
            <Cluster gap="3" align="center">
              <Checkbox
                isSelected={word.wholeWord}
                onChange={(wholeWord) => {
                  form.changeWord(word.key, { wholeWord });
                }}
              >
                {t("client:filters.form.whole_word")}
              </Checkbox>
              {form.words.length > 1 && (
                <Button
                  size="sm"
                  variant="ghost"
                  onPress={() => {
                    form.removeWord(word.key);
                  }}
                >
                  {t("client:filters.form.word.remove", { number })}
                </Button>
              )}
            </Cluster>
          </Stack>
        );
      })}
      <Cluster gap="3" align="center">
        <Button size="sm" onPress={form.addWord} isDisabled={!form.canAddWord}>
          {t("client:filters.form.word.add")}
        </Button>
        {!form.canAddWord && <Hint>{t("client:filters.form.words.limit", { max: limits.words })}</Hint>}
      </Cluster>
      <Stack gap="1">
        <Hint>{t("client:filters.form.whole_word.note")}</Hint>
        <div>
          <ExternalLink href={manual} size="sm">
            {t("client:filters.form.whole_word.help")}
          </ExternalLink>
        </div>
      </Stack>
    </Fieldset>
  );
}

/** The places the filter applies, one checkbox each; at least one is needed. */
function FilterContexts({ form }: { form: FilterForm }) {
  const t = useT();
  const labels = useContextLabels();
  return (
    <Fieldset legend={t("client:filters.form.contexts")}>
      {filterContexts.map((context) => (
        <Checkbox
          key={context}
          isSelected={form.contexts.has(context)}
          onChange={(on) => {
            form.toggleContext(context, on);
          }}
        >
          {labels[context]}
        </Checkbox>
      ))}
    </Fieldset>
  );
}

/** The key of an end in the select. */
function expiryKey(expiry: Expiry): string {
  return typeof expiry === "number" ? String(expiry) : expiry;
}

/** When the filter ends: as it is (for a filter with an end), never, or after one of the durations from now. */
function FilterExpiry({ form }: { form: FilterForm }) {
  const t = useT();
  const duration = (seconds: number): string => {
    if (seconds % 86_400 === 0) return t("client:filters.form.duration.days", { count: seconds / 86_400 });
    if (seconds % 3600 === 0) return t("client:filters.form.duration.hours", { count: seconds / 3600 });
    return t("client:filters.form.duration.minutes", { count: seconds / 60 });
  };
  const keep =
    form.currentEnd === null
      ? []
      : [
          {
            id: "keep",
            expiry: "keep" as const,
            name: form.currentEnd.expired
              ? t("client:filters.form.expiry.keep_expired")
              : t("client:filters.form.expiry.keep", { date: form.currentEnd.date }),
          },
        ];
  const items: { id: string; expiry: Expiry; name: string }[] = [
    ...keep,
    { id: "never", expiry: "never", name: t("client:filters.form.expiry.never") },
    ...expiryChoices.map((seconds) => ({ id: String(seconds), expiry: seconds, name: duration(seconds) })),
  ];
  return (
    <Box width="sm">
      <Select
        label={t("client:filters.form.expiry")}
        value={expiryKey(form.expiry)}
        onChange={(key) => {
          const item = items.find((option) => option.id === key);
          if (item !== undefined) form.setExpiry(item.expiry);
        }}
        items={items}
      >
        {(item) => (
          <SelectItem id={item.id} textValue={item.name}>
            {item.name}
          </SelectItem>
        )}
      </Select>
    </Box>
  );
}

type FilterRowProps = { filter: KeywordFilter; remove: DeleteFilter; onDelete: () => void };

/** A filter in the list; the form that changes it takes the row's place while it is open. */
function FilterRow({ filter, remove, onDelete }: FilterRowProps) {
  const t = useT();
  const intl = useIntl();
  const labels = useContextLabels();
  // `returned` once the form closed, which gives the Edit button focus back.
  const [editing, setEditing] = useState<"closed" | "open" | "returned">("closed");
  const editButtonId = useId();
  useEffect(() => {
    // Focus is moved in the DOM, outside React, once the row shows its buttons again.
    if (editing === "returned") document.getElementById(editButtonId)?.focus();
  }, [editing, editButtonId]);
  if (editing === "open") {
    return (
      <EditFilter
        filter={filter}
        close={() => {
          setEditing("returned");
        }}
      />
    );
  }
  const title = isolate(filter.title);
  const words = filter.keywords.map((keyword) => keyword.keyword);
  const listed = intl.formatList(words.slice(0, shownWords).map(isolate), { type: "unit" });
  return (
    <Stack gap="3" rowAt="md">
      <Stack gap="1" shrink grow>
        <Cluster gap="2" align="center">
          <Text as="span" dir="auto" weight="semibold" wrap="anywhere">
            {filter.title}
          </Text>
          <Tag variant="outlined">
            {filter.action === "warn" ? t("client:filters.action.warn") : t("client:filters.action.hide")}
          </Tag>
          {filter.expired && <Tag variant="outlined">{t("client:filters.expired")}</Tag>}
        </Cluster>
        <Hint as="span">
          {words.length > shownWords
            ? t("client:filters.words.more", {
                count: words.length,
                words: listed,
                rest: intl.formatNumber(words.length - shownWords),
              })
            : t("client:filters.words", { count: words.length, words: listed })}
        </Hint>
        <Hint as="span">
          {t("client:filters.contexts", {
            contexts: intl.formatList(
              filter.contexts.map((context) => labels[context]),
              { type: "conjunction" },
            ),
          })}
        </Hint>
        {filter.expires_at !== undefined && !filter.expired && (
          <Hint as="span">
            {t("client:filters.until", {
              date: intl.formatDate(filter.expires_at, { dateStyle: "medium", timeStyle: "short" }),
            })}
          </Hint>
        )}
      </Stack>
      <Cluster gap="2">
        <Button
          id={editButtonId}
          size="sm"
          aria-label={t("client:filters.row_action", { action: t("client:filters.edit"), title })}
          onPress={() => {
            setEditing("open");
          }}
        >
          {t("client:filters.edit")}
        </Button>
        <Button
          size="sm"
          variant="danger"
          isPending={remove.pendingId === filter.id}
          aria-label={t("client:filters.row_action", { action: t("client:filters.delete"), title })}
          onPress={onDelete}
        >
          {t("client:filters.delete")}
        </Button>
      </Cluster>
    </Stack>
  );
}

/** The form that changes a filter, in the row's place: its title (`h3`), the fields with the filter's values, and its bar. */
function EditFilter({ filter, close }: { filter: KeywordFilter; close: () => void }) {
  const t = useT();
  const saver = useUpdateFilter(filter);
  const headingId = useId();
  const ref = useRef<HTMLElement>(null);
  useEffect(() => {
    // Focus goes into the form's first field once it has rendered (a DOM node outside React's state).
    ref.current?.querySelector<HTMLElement>("input")?.focus();
  }, []);
  return (
    <Box as="section" ref={ref} aria-labelledby={headingId} border="lg" background="sunken" padding="4">
      <Stack gap="4">
        <Heading level={3} id={headingId}>
          {t("client:filters.edit.title", { title: isolate(filter.title) })}
        </Heading>
        <FilterFormFields filter={filter} saver={saver} submitLabel={t("client:filters.form.save")} close={close} />
      </Stack>
    </Box>
  );
}
