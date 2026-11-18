// SPDX-License-Identifier: AGPL-3.0-only

// Aliases, the screen under the settings' entry Account and data at /settings/account/aliases (FR-009, ADR-0089
// §2): the accounts the reader says this account is also known as, which lets each of them move its followers here.
// Everything on it acts at once.
//
// - The aliases, oldest first, as the server lists them, under how many of the most an account has there are: each
//   its handle (its address where the server knows no handle) and the day it was added, with Remove. A row taken out
//   keeps its place until the reader leaves the screen, its button gone and its result said in its place, which takes
//   the focus (the Relationships entry's removals); a removal that failed says so in the section's status.
// - Adding one by its address, a handle or the address of its profile: the server looks the account up, which can
//   take a moment. What came of it is said in the section's status, which takes the focus, and the field is empty
//   again. An address the server finds no account at is said under the field, which takes the focus; so is an empty
//   one. With the most aliases an account has, the field gives way to a sentence that says so.
//
// An alias alone changes nothing, which the screen says once: the move is started from the other account.

import {
  accountAliasesQuery,
  aliasLimit,
  aliasRefusalOf,
  refusalDetail,
  useAccountAliasList,
  useAccountAliasWrites,
  type AccountAlias,
  type AppData,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  focusRefusedField,
  Form,
  Row,
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsScreen,
  SettingsSection,
  Stack,
  TextField,
  useLocale,
} from "@my/design-system";
import { useEffect, useId, useRef, useState } from "react";
import { stringsIn } from "../locale";
import { useActionDate } from "./account";
import { ReadingNotice } from "./controls";
import { useRemovals } from "./relationships";

type T = ReturnType<typeof stringsIn>["t"];

/** What the route's loader waits for: the reader's aliases, from the cache when it has them. */
export async function loadAccountAliases(data: AppData): Promise<void> {
  await data.queryClient.query({ ...accountAliasesQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The title of the screen. */
export function useAccountAliasesTitle(): string {
  return stringsIn(useLocale().locale).t("settings.account.aliases");
}

/** An alias as a sentence names it: its handle with its `@`, or its address where the server knows no handle. */
const aliasName = (alias: AccountAlias): string => (alias.acct === undefined ? alias.uri : `@${alias.acct}`);

/** The list of the aliases, each with Remove. */
function Aliases({ aliases, t }: { aliases: readonly AccountAlias[]; t: T }) {
  const id = useId();
  const day = useActionDate(false);
  const writes = useAccountAliasWrites();
  const { busy, kept, remove, status } = useRemovals<AccountAlias>(id);
  // The aliases the server lists, then each the reader took out put back where it stood.
  const shown = [...aliases];
  const taken = [...kept].sort((a, b) => a[1].at - b[1].at);
  for (const entry of taken) {
    const [key, { item, at }] = entry;
    if (!shown.some((alias) => alias.uri === key)) shown.splice(Math.min(at, shown.length), 0, item);
  }
  const heading = t("settings.account.aliases.count", { count: aliases.length, max: aliasLimit });
  return (
    <SettingsSection heading={heading} about={t("settings.account.aliases.about")}>
      <Stack>
        {shown.length === 0 ? (
          <SettingsEmpty>{t("settings.account.aliases.empty")}</SettingsEmpty>
        ) : (
          <SettingsList label={heading}>
            {shown.map((alias, at) => {
              const name = aliasName(alias);
              // An alias added again after it was removed is listed by the server, and is no longer a removed one.
              const listed = aliases.some((a) => a.uri === alias.uri);
              const result = listed ? undefined : kept.get(alias.uri)?.result;
              return (
                <SettingsListRow
                  key={alias.uri}
                  title={name}
                  // A removed alias says only that it was removed, where its fact was.
                  facts={
                    result === undefined ? [t("settings.account.aliases.added", { date: day(alias.created_at) })] : []
                  }
                  result={result}
                  end={
                    <Button
                      size="timeline"
                      touch
                      busy={busy.has(alias.uri)}
                      onPress={() => {
                        remove(alias.uri, alias, at, {
                          write: () => writes.remove(alias.uri),
                          shown: t("settings.account.aliases.removed"),
                          done: t("settings.account.aliases.removed.spoken", { name }),
                          failed: t("settings.account.aliases.remove.failed", { name }),
                        });
                      }}
                    >
                      {t("settings.account.aliases.remove")}
                    </Button>
                  }
                />
              );
            })}
          </SettingsList>
        )}
        {status}
      </Stack>
    </SettingsSection>
  );
}

/** Adding an alias by its address; with the most an account has, that there is no room for one. */
function AddAlias({ full, t }: { full: boolean; t: T }) {
  const id = useId();
  const writes = useAccountAliasWrites();
  const [acct, setAcct] = useState("");
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const [said, setSaid] = useState<{ readonly text: string; readonly n: number } | undefined>(undefined);
  // The field is drawn anew, empty, once an alias is added.
  const [round, setRound] = useState(0);
  const count = useRef(0);
  // What takes the focus once the section is drawn again: its status, or the field that was refused.
  const focusNext = useRef<"status" | "field" | undefined>(undefined);
  const fieldId = `${id}-acct`;
  const statusId = `${id}-status`;
  useEffect(() => {
    if (focusNext.current === "field") focusRefusedField(fieldId);
    else if (focusNext.current === "status") document.getElementById(statusId)?.focus();
    focusNext.current = undefined;
  });
  const say = (text: string, focus: boolean) => {
    count.current += 1;
    setSaid({ text, n: count.current });
    if (focus) focusNext.current = "status";
  };
  const refuse = (text: string) => {
    setError(text);
    focusNext.current = "field";
  };
  const add = () => {
    if (adding) return;
    const address = acct.trim();
    if (address === "") {
      refuse(t("settings.account.aliases.add.missing"));
      return;
    }
    setAdding(true);
    writes
      .add(address)
      .then(
        () => {
          setAcct("");
          setRound((n) => n + 1);
          say(t("settings.account.aliases.add.done"), true);
        },
        (failure: unknown) => {
          const reason = aliasRefusalOf(failure);
          if (reason === "invalid") refuse(t("settings.account.aliases.add.invalid"));
          else if (reason === "full") say(t("settings.account.aliases.full", { max: aliasLimit }), false);
          else say(refusalDetail(failure) ?? t("settings.account.aliases.add.failed"), false);
        },
      )
      .finally(() => {
        setAdding(false);
      });
  };
  const status = (
    <Box as="p" id={statusId} role="status" tabIndex={-1} breaks="phrase">
      {said !== undefined && <span key={said.n}>{said.text}</span>}
    </Box>
  );
  return (
    <SettingsSection heading={t("settings.account.aliases.add")}>
      {/* The status keeps its place from the start, with no room of its own until it says something. */}
      <Stack gap={said === undefined ? undefined : "screen-row-gap"}>
        {status}
        {full ? (
          <Box as="p" tone="muted" breaks="phrase">
            {t("settings.account.aliases.full", { max: aliasLimit })}
          </Box>
        ) : (
          <Form
            action=""
            onSubmit={(e) => {
              e.preventDefault();
              add();
            }}
          >
            <TextField
              key={round}
              id={fieldId}
              name="acct"
              label={t("settings.account.aliases.add.label")}
              hint={t("settings.account.aliases.add.hint")}
              autoCapitalize="none"
              autoComplete="off"
              onChange={(value) => {
                setAcct(value);
                setError(undefined);
              }}
              readOnly={adding}
              error={error}
            />
            <Row justify="start">
              <Button type="submit" kind="primary" size="timeline" touch busy={adding}>
                {t("settings.account.aliases.add.submit")}
              </Button>
            </Row>
          </Form>
        )}
      </Stack>
    </SettingsSection>
  );
}

/** The screen: the aliases and adding one, once they are read; `onBack` goes back to the entry. */
export function AccountAliasesScreen({ onBack }: { onBack: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  const { aliases, status, retry } = useAccountAliasList();
  const title = t("settings.account.aliases");
  const back = <BackButton onPress={onBack}>{t("settings.entry.account")}</BackButton>;
  if (status !== "success")
    return (
      <SettingsScreen
        sub
        title={title}
        back={back}
        notice={
          <ReadingNotice
            words={{ loading: t("settings.account.aliases.loading"), failed: t("settings.account.aliases.failed") }}
            failed={status === "error"}
            onRetry={retry}
          />
        }
      />
    );
  return (
    <SettingsScreen sub title={title} back={back}>
      <Aliases aliases={aliases} t={t} />
      <AddAlias full={aliases.length >= aliasLimit} t={t} />
    </SettingsScreen>
  );
}
