// SPDX-License-Identifier: AGPL-3.0-only

// Custom emoji, the entry of the settings at /settings/custom-emojis of a reader who may ask for custom emoji (FR-915,
// ADR-0070 §2–§4): what they used of their monthly limit and when the count starts again, with Request an emoji
// while they may ask for one more (the sentence why not in its place once they may not); their requests, newest
// first, each with where it stands (a rejection's reason too) and Withdraw while it waits; and the emoji they made,
// each a door to its screen. Under it: a new request, /settings/custom-emojis/new (a shortcode and an image), and one
// of their emoji, /settings/custom-emojis/{id}: its image, its place in the picker as a form saved as a whole, and
// Delete last, which asks first and gives no place of the month back.

import {
  aliasesOf,
  emojiLimits,
  emojiRefusal,
  loadOwnEmojis,
  mayRequest,
  useOwnEmoji,
  useOwnEmojis,
  type OwnMore,
  useOwnEmojiWrites,
  validShortcode,
  type AppData,
  type EmojiQuota,
  type OwnCustomEmoji,
  type OwnCustomEmojiRequest,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  CustomEmoji,
  FileField,
  focusRefusedField,
  FormAlert,
  LinkButton,
  ModalPresence,
  Row,
  SaveState,
  Select,
  SettingRow,
  SettingsEmpty,
  SettingsFacts,
  SettingsList,
  SettingsListRow,
  MoreRecords,
  SettingsNote,
  StateMark,
  type RecordState,
  SettingsResult,
  SettingsScreen,
  SettingsSection,
  SubmitBar,
  TextField,
  useLocale,
} from "@my/design-system";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { stringsIn } from "../locale";
import type { SettingsEntryProps, SettingsSubProps } from "./entries";
import { Asking, ReadingNotice, useMoment } from "./parts";

type T = ReturnType<typeof stringsIn>["t"];

/** The addresses of the entry, of a new request, and of the reader's emoji of `id`. */
const entryPath = "/settings/custom-emojis";
const newRequestPath = `${entryPath}/new`;
const ownEmojiPath = (id: string): string => `${entryPath}/${id}`;

/** A shortcode as a text writes it, between colons. */
const coded = (shortcode: string): string => `:${shortcode}:`;

/** What the loader of a screen of the entry waits for: the reader's emoji and requests. */
export async function loadCustomEmojiSettings(data: AppData): Promise<void> {
  await loadOwnEmojis(data);
}

/** The strings of the settings in the reader's locale. */
function useT(): T {
  return stringsIn(useLocale().locale).t;
}

/** What the reader used of their monthly limit, and when the count starts again, as sentences. */
function quotaFacts(t: T, quota: EmojiQuota, day: (iso: string) => string): { text: string }[] {
  return [
    {
      text:
        quota.limit === undefined
          ? t("emoji_own.quota.unlimited", { used: quota.used })
          : t("emoji_own.quota.used", { used: quota.used, limit: quota.limit }),
    },
    ...(quota.pending === 0 ? [] : [{ text: t("emoji_own.quota.pending", { count: quota.pending }) }]),
    ...(quota.limit === undefined ? [] : [{ text: t("emoji_own.quota.resets", { date: day(quota.resetsAt) }) }]),
  ];
}

/** A request's state in words. */
/** How a request's state looks, as the administrators' list of requests marks it: what waits, what was decided. */
const requestLook: Record<OwnCustomEmojiRequest["state"], RecordState> = {
  pending: "pending",
  approved: "resolved",
  rejected: "resolved",
  withdrawn: "lifted",
  lapsed: "expired",
};

const stateOf = (t: T, r: OwnCustomEmojiRequest): string => t("emoji_own.request.state", { state: r.state });

/** What came of a press in a section, each saying a new element, so that the same words are read again. */
type Said = { readonly text: string; readonly n: number };

/** The reader's requests, with Withdraw while one waits, and what came of a withdrawal in the section's status. */
function Requests({ requests, more }: { requests: readonly OwnCustomEmojiRequest[]; more: OwnMore }) {
  const t = useT();
  const moment = useMoment(false);
  const writes = useOwnEmojiWrites();
  const [busy, setBusy] = useState<string | undefined>(undefined);
  const [said, setSaid] = useState<Said | undefined>(undefined);
  const saidRef = useRef<HTMLParagraphElement>(null);
  const focusSaid = useRef(false);
  useEffect(() => {
    if (!focusSaid.current || said === undefined) return;
    focusSaid.current = false;
    saidRef.current?.focus();
  }, [said]);
  const say = (text: string) => {
    focusSaid.current = true;
    setSaid((was) => ({ text, n: (was?.n ?? 0) + 1 }));
  };
  const withdraw = (r: OwnCustomEmojiRequest) => {
    setBusy(r.id);
    writes.withdraw(r.id).then(
      () => {
        setBusy(undefined);
        say(t("emoji_own.request.withdrawn", { shortcode: coded(r.shortcode) }));
      },
      (error: unknown) => {
        setBusy(undefined);
        say(
          t(emojiRefusal(error) === "not_pending" ? "emoji_own.request.decided" : "emoji_own.request.withdraw_failed"),
        );
      },
    );
  };
  const heading = t("emoji_own.requests");
  return (
    <SettingsSection heading={heading}>
      {requests.length === 0 ? (
        <SettingsEmpty>{t("emoji_own.requests.empty")}</SettingsEmpty>
      ) : (
        <SettingsList label={heading}>
          {requests.map((r) => (
            <SettingsListRow
              key={r.id}
              title={coded(r.shortcode)}
              inactive={r.state !== "pending" && r.state !== "approved"}
              mark={<StateMark state={requestLook[r.state]}>{stateOf(t, r)}</StateMark>}
              facts={[
                t("emoji_own.request.asked", { date: moment(r.created_at) }),
                ...(r.reason === undefined ? [] : [t("emoji_own.request.reason", { reason: r.reason })]),
              ]}
              end={
                r.state === "pending" ? (
                  <Button
                    size="timeline"
                    touch
                    busy={busy === r.id}
                    isDisabled={busy !== undefined}
                    onPress={() => {
                      withdraw(r);
                    }}
                  >
                    {t("emoji_own.request.withdraw")}
                  </Button>
                ) : undefined
              }
            />
          ))}
        </SettingsList>
      )}
      <SettingsResult ref={saidRef}>
        {said === undefined ? undefined : <span key={said.n}>{said.text}</span>}
      </SettingsResult>
      <More more={more} />
    </SettingsSection>
  );
}

/** What a row of the reader's emoji says of one: its category and aliases, and that the picker leaves it out. */
function emojiFacts(t: T, e: OwnCustomEmoji): string[] {
  return [
    ...(e.category === undefined ? [] : [t("emoji_own.emoji.category", { category: e.category })]),
    ...(e.aliases.length === 0 ? [] : [t("emoji_own.emoji.aliases", { aliases: e.aliases.map(coded).join(" ") })]),
    ...(e.visible_in_picker ? [] : [t("emoji_own.emoji.hidden")]),
  ];
}

/** The entry: the month's limit and Request an emoji, the reader's requests, and their emoji. */
export function CustomEmojiSettings({ back }: SettingsEntryProps): ReactNode {
  const t = useT();
  const day = useMoment(false);
  const read = useOwnEmojis();
  const title = t("settings.entry.custom_emojis");
  if (read.quota === undefined || read.requests === undefined || read.emojis === undefined)
    return (
      <SettingsScreen
        icon="emojiCustom"
        title={title}
        back={back}
        notice={
          <ReadingNotice
            status={read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{ loading: t("emoji_own.loading"), failed: t("emoji_own.failed") }}
          />
        }
      />
    );
  const { quota, requests, emojis } = read;
  return (
    <SettingsScreen icon="emojiCustom" title={title} back={back}>
      <SettingsSection heading={t("emoji_own.month")}>
        <SettingsFacts label={t("emoji_own.month")} items={quotaFacts(t, quota, day)} />
        {mayRequest(quota) ? (
          <Row justify="start">
            <LinkButton kind="primary" size="timeline" href={newRequestPath}>
              {t("emoji_own.request.new")}
            </LinkButton>
          </Row>
        ) : (
          <SettingsNote instead>{t("emoji_own.quota.reached", { date: day(quota.resetsAt) })}</SettingsNote>
        )}
      </SettingsSection>
      <Requests requests={requests} more={read.moreRequests} />
      <SettingsSection heading={t("emoji_own.emojis")}>
        {emojis.length === 0 ? (
          <SettingsEmpty>{t("emoji_own.emojis.empty")}</SettingsEmpty>
        ) : (
          <SettingsList label={t("emoji_own.emojis")}>
            {emojis.map((e) => (
              <SettingsListRow
                key={e.id}
                title={coded(e.shortcode)}
                inactive={e.state === "disabled"}
                facts={emojiFacts(t, e)}
                state={e.state === "disabled" ? t("emoji_own.emoji.disabled") : undefined}
                href={ownEmojiPath(e.id)}
              />
            ))}
          </SettingsList>
        )}
        <More more={read.moreEmojis} />
      </SettingsSection>
    </SettingsScreen>
  );
}

/** Show more under a list of the reader's that has a next page; the words that it failed, with the same button. */
function More({ more }: { more: OwnMore }) {
  const t = useT();
  if (!more.hasNext) return null;
  return (
    <>
      {more.failed && <SettingsNote>{t("emoji_own.more.failed")}</SettingsNote>}
      <MoreRecords busy={more.fetching} onPress={more.next} />
    </>
  );
}

/** What the server's refusal of a request or a change is about, and what to say there; else what to say beside the button. */
function refusalOf(t: T, error: unknown): { field?: "shortcode" | "image" | "category" | "aliases"; why: string } {
  switch (emojiRefusal(error)) {
    case "invalid_shortcode":
      return { field: "shortcode", why: t("emoji_own.shortcode.rule") };
    case "shortcode_taken":
      return { field: "shortcode", why: t("emoji_own.shortcode.taken") };
    case "unsupported_format":
      return { field: "image", why: t("emoji_own.image.format") };
    case "too_large":
      return { field: "image", why: t("emoji_own.image.too_large") };
    case "too_big":
      return { field: "image", why: t("emoji_own.image.too_big") };
    case "media_not_found":
    case "media_not_ready":
      return { field: "image", why: t("emoji_own.image.failed") };
    case "invalid_alias":
      return { field: "aliases", why: t("emoji_own.aliases.rule", { max: emojiLimits.aliases }) };
    case "invalid_category":
      return { field: "category", why: t("emoji_own.category.rule", { max: emojiLimits.category }) };
    case "limit_reached":
      return { why: t("emoji_own.request.limit") };
    default:
  }
  if (error instanceof Error && error.name === "MediaFailedError")
    return { field: "image", why: t("emoji_own.image.failed") };
  return { why: t("emoji_own.save.failed") };
}

/** Where a form's last send is. */
type Sent =
  | { readonly kind: "idle" | "sending" | "saved" }
  | { readonly kind: "refused"; readonly field?: string | undefined; readonly why: string };

/** The state of a form's send, and the focus on the field a refusal names. */
function useSent(id: string) {
  const [sent, setSent] = useState<Sent>({ kind: "idle" });
  const focusNext = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (focusNext.current === undefined) return;
    focusRefusedField(focusNext.current);
    focusNext.current = undefined;
  });
  return {
    sent,
    setSent,
    refuse: (field: string | undefined, why: string) => {
      setSent({ kind: "refused", field, why });
      if (field !== undefined) focusNext.current = `${id}-${field}`;
    },
    errorOf: (field: string) => (sent.kind === "refused" && sent.field === field ? sent.why : undefined),
  };
}

/** The submit bar of a form of the entry. */
function Bar({ sent, label, onPress }: { sent: Sent; label: string; onPress: () => void }) {
  const t = useT();
  return (
    <SubmitBar
      result={
        sent.kind === "saved" ? (
          <SaveState state="saved" />
        ) : sent.kind === "refused" ? (
          <FormAlert>{sent.field === undefined ? sent.why : t("emoji_own.save.refused")}</FormAlert>
        ) : undefined
      }
    >
      <Button kind="primary" busy={sent.kind === "sending"} onPress={onPress}>
        {label}
      </Button>
    </SubmitBar>
  );
}

/** The title of a new request's screen. */
export function useEmojiRequestTitle(): string {
  return useT()("emoji_own.request.new");
}

/** A new request: a shortcode and an image; at the month's limit, why not, and no form. */
export function EmojiRequestScreen({ onBack }: SettingsSubProps): ReactNode {
  const t = useT();
  const day = useMoment(false);
  const read = useOwnEmojis();
  const writes = useOwnEmojiWrites();
  const id = useId();
  const f = useSent(id);
  const [shortcode, setShortcode] = useState("");
  const [file, setFile] = useState<File | undefined>(undefined);
  const title = t("emoji_own.request.new");
  const back = <BackButton onPress={onBack}>{t("settings.entry.custom_emojis")}</BackButton>;
  if (read.quota === undefined)
    return (
      <SettingsScreen
        sub
        title={title}
        back={back}
        notice={
          <ReadingNotice
            status={read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{ loading: t("emoji_own.loading"), failed: t("emoji_own.failed") }}
          />
        }
      />
    );
  if (!mayRequest(read.quota))
    return (
      <SettingsScreen sub title={title} back={back}>
        <SettingsSection heading={t("emoji_own.month")}>
          <SettingsNote instead>{t("emoji_own.quota.reached", { date: day(read.quota.resetsAt) })}</SettingsNote>
        </SettingsSection>
      </SettingsScreen>
    );
  const send = () => {
    if (f.sent.kind === "sending") return;
    if (!validShortcode(shortcode)) {
      f.refuse("shortcode", t("emoji_own.shortcode.rule"));
      return;
    }
    if (file === undefined) {
      f.refuse("image", t("emoji_own.image.missing"));
      return;
    }
    f.setSent({ kind: "sending" });
    writes.request(shortcode, file).then(onBack, (error: unknown) => {
      const r = refusalOf(t, error);
      f.refuse(r.field, r.why);
    });
  };
  const busy = f.sent.kind === "sending";
  return (
    <SettingsScreen
      sub
      title={title}
      back={back}
      foot={<Bar sent={f.sent} label={t("emoji_own.request.send")} onPress={send} />}
    >
      <SettingsSection heading={t("emoji_own.request.emoji")}>
        <TextField
          id={`${id}-shortcode`}
          name="shortcode"
          label={t("emoji_own.shortcode")}
          hint={t("emoji_own.shortcode.rule")}
          row
          defaultValue=""
          maxLength={30}
          autoCapitalize="none"
          error={f.errorOf("shortcode")}
          readOnly={busy}
          onChange={(value) => {
            setShortcode(value.replace(/:/g, ""));
            f.setSent({ kind: "idle" });
          }}
        />
        <FileField
          id={`${id}-image`}
          label={t("emoji_own.image")}
          hint={t("emoji_own.image.hint")}
          error={f.errorOf("image")}
          choose={t(file === undefined ? "emoji_own.image.choose" : "emoji_own.image.another")}
          file={file?.name}
          none={t("emoji_own.image.none")}
          accept="image/png,image/gif,image/webp"
          row
          isDisabled={busy}
          onFile={(picked) => {
            setFile(picked);
            f.setSent({ kind: "idle" });
          }}
        />
      </SettingsSection>
    </SettingsScreen>
  );
}

/** The shortcode of the reader's emoji of `id`, the title of its screen; the entry's name until it is read. */
export function useOwnEmojiTitle(id: string): string {
  const t = useT();
  const emoji = useOwnEmoji(id).emoji;
  return emoji === undefined ? t("settings.entry.custom_emojis") : coded(emoji.shortcode);
}

/** One of the reader's emoji: its image, its place in the picker, and Delete last. */
function OwnEmojiForm({ emoji, onBack }: { emoji: OwnCustomEmoji; onBack: () => void }) {
  const t = useT();
  const writes = useOwnEmojiWrites();
  const id = useId();
  const f = useSent(id);
  const [category, setCategory] = useState(emoji.category ?? "");
  const [aliases, setAliases] = useState(emoji.aliases.join(" "));
  const [visible, setVisible] = useState(emoji.visible_in_picker);
  const [asking, setAsking] = useState(false);
  const busy = f.sent.kind === "sending";
  const save = () => {
    if (busy) return;
    if (Array.from(category.trim()).length > emojiLimits.category) {
      f.refuse("category", t("emoji_own.category.rule", { max: emojiLimits.category }));
      return;
    }
    const names = aliasesOf(aliases);
    if (names.length > emojiLimits.aliases || names.some((a) => !validShortcode(a))) {
      f.refuse("aliases", t("emoji_own.aliases.rule", { max: emojiLimits.aliases }));
      return;
    }
    f.setSent({ kind: "sending" });
    writes.change(emoji.id, { category: category.trim(), aliases: names, visibleInPicker: visible }).then(
      () => {
        f.setSent({ kind: "saved" });
      },
      (error: unknown) => {
        const r = refusalOf(t, error);
        f.refuse(r.field, r.why);
      },
    );
  };
  const changed = () => {
    if (f.sent.kind !== "sending") f.setSent({ kind: "idle" });
  };
  return (
    <>
      <SettingsScreen
        sub
        title={coded(emoji.shortcode)}
        back={<BackButton onPress={onBack}>{t("settings.entry.custom_emojis")}</BackButton>}
        foot={
          <>
            <Bar sent={f.sent} label={t("emoji_own.save")} onPress={save} />
            {/* What cannot be undone, last and apart, in the look the settings give such an act (an invite's revocation). */}
            <SettingsSection heading={t("emoji_own.delete")}>
              <SettingsList label={t("emoji_own.delete")}>
                <SettingsListRow
                  title={coded(emoji.shortcode)}
                  caption={t("emoji_own.delete.about")}
                  end={
                    <Button
                      size="timeline"
                      touch
                      isDisabled={busy}
                      onPress={() => {
                        setAsking(true);
                      }}
                    >
                      {t("emoji_own.delete.press")}
                    </Button>
                  }
                />
              </SettingsList>
            </SettingsSection>
          </>
        }
      >
        <SettingsSection heading={t("emoji_own.emoji")}>
          <Row align="center" gap="screen-row-gap">
            <Box text="title">
              {emoji.url === undefined ? (
                t("emoji_own.no_image")
              ) : (
                <CustomEmoji shortcode={emoji.shortcode} url={emoji.url} staticUrl={emoji.static_url ?? emoji.url} />
              )}
            </Box>
          </Row>
          {emoji.state === "disabled" && <SettingsNote>{t("emoji_own.emoji.disabled.about")}</SettingsNote>}
        </SettingsSection>
        <SettingsSection heading={t("emoji_own.picker")}>
          <TextField
            id={`${id}-category`}
            name="category"
            label={t("emoji_own.category")}
            row
            defaultValue={category}
            maxLength={emojiLimits.category}
            error={f.errorOf("category")}
            readOnly={busy}
            onChange={(value) => {
              setCategory(value);
              changed();
            }}
          />
          <TextField
            id={`${id}-aliases`}
            name="aliases"
            label={t("emoji_own.aliases")}
            hint={t("emoji_own.aliases.hint")}
            row
            defaultValue={aliases}
            autoCapitalize="none"
            error={f.errorOf("aliases")}
            readOnly={busy}
            onChange={(value) => {
              setAliases(value);
              changed();
            }}
          />
          <SettingRow id={`${id}-visible`} label={t("emoji_own.visible")} form>
            <Select
              name="visible"
              options={["true", "false"].map((v) => ({ value: v, label: t("emoji_own.visible.value", { value: v }) }))}
              value={visible ? "true" : "false"}
              isDisabled={busy}
              onChange={(v) => {
                setVisible(v === "true");
                changed();
              }}
            />
          </SettingRow>
        </SettingsSection>
      </SettingsScreen>
      <ModalPresence>
        {asking && (
          <Asking
            key="delete"
            title={t("emoji_own.delete.title", { shortcode: coded(emoji.shortcode) })}
            confirm={t("emoji_own.delete.press")}
            cancel={t("emoji_own.delete.cancel")}
            run={() => writes.remove(emoji.id).then(onBack)}
            onClose={() => {
              setAsking(false);
            }}
          >
            {t("emoji_own.delete.text")}
          </Asking>
        )}
      </ModalPresence>
    </>
  );
}

/** One of the reader's emoji, or that it loads, failed, or the address names none of theirs. */
export function OwnEmojiScreen({ id, onBack }: SettingsSubProps & { id: string }): ReactNode {
  const t = useT();
  const read = useOwnEmoji(id);
  if (read.emoji === undefined)
    return (
      <SettingsScreen
        sub
        title={t("settings.entry.custom_emojis")}
        back={<BackButton onPress={onBack}>{t("settings.entry.custom_emojis")}</BackButton>}
        notice={
          <ReadingNotice
            status={read.missing ? "missing" : read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{ loading: t("emoji_own.loading"), failed: t("emoji_own.failed") }}
            missing={{ said: t("emoji_own.missing"), all: t("emoji_own.all"), href: entryPath }}
          />
        }
      />
    );
  return <OwnEmojiForm key={read.emoji.id} emoji={read.emoji} onBack={onBack} />;
}
