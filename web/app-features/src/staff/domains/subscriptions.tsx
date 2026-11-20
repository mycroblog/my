// SPDX-License-Identifier: AGPL-3.0-only

// The subscriptions to the domain block lists others publish (ADR-0159 §4–§6): their list at
// /moderation/domain-actions/subscriptions, newest first, each its title, its URL, its mode and its last fetch, the
// failures in a row said where there are any; a new subscription (…/new): its title, the URL and the format of the
// list, whether its differences wait for a moderator or apply at each fetch, and what its rows issue here; and a
// subscription's page (…/{id}): what it is, Fetch now, the drafts that wait, accepted or rejected as picked (a suspend
// among those accepted asks first), the fetches, newest first, the settings, and Stop subscribing, which asks whether to
// lift the actions it issued.

import {
  domainListFormats,
  domainListModes,
  domainListSilenceAs,
  domainListSuspendAs,
  domainListTitleLimit,
  domainListUrlLimit,
  loadDomainListSubscriptions,
  refusedMember,
  useDomainListDrafts,
  useDomainListFetches,
  useDomainListSubscription,
  useDomainListSubscriptions,
  useDomainListWrites,
  type DomainListFetch,
  type DomainListSubscription,
  type DomainListSubscriptionChange,
  type AppData,
} from "@my/app-data";
import {
  Pager,
  BackButton,
  Button,
  CheckField,
  ConfirmDialog,
  FactList,
  FeedNotice,
  FormAlert,
  LinkButton,
  LogEntry,
  LogList,
  ModalPresence,
  RecordTable,
  Row,
  ScreenStack,
  ScreenTitle,
  Stack,
  StateMark,
  TextField,
  TextLink,
  type Fact,
  type IconName,
  type RecordColumn,
  type RecordRow,
  useAddedFocus,
} from "@my/design-system";
import { useId, useState, type ReactNode } from "react";
import type { StaffAddress, StaffSearch } from "../entries";
import { ListMore, ReadNotice, useMoment, useStaffStrings, type T, useSaid } from "../parts";
import {
  cursorOf,
  domainKindWords,
  domainListPath,
  domainListsPath,
  LabelledSelect,
  newDomainListPath,
  useAddressPages,
  refusalWords,
  Section,
  tied,
} from "./parts";

/** The last fetch of a subscription, in words: what came of it. */
function fetchWords(t: T, fetch: DomainListFetch): string {
  if (fetch.outcome === "failed")
    return t("staff_domains.fetch.failed", {
      why: fetch.error_code ?? "other",
      status: fetch.http_status ?? 0,
    });
  return tied(t("staff_domains.fetch.outcome", { outcome: fetch.outcome, count: fetch.entry_count }));
}

/** The glyph of a fetch, by what came of it. */
const fetchGlyph = (fetch: DomainListFetch): IconName =>
  fetch.outcome === "failed" ? "error" : fetch.outcome === "applied" ? "update" : "read";

type Column = "mode" | "fetched";

/** Reads the page of subscriptions an address's query asks for, for the route's loader. */
export const loadDomainListsScreen = (data: AppData, search: StaffSearch): Promise<void> =>
  loadDomainListSubscriptions(data, cursorOf(search));

/** The list of subscriptions: Add subscription, the page, and the pages. */
export function DomainListsScreen({ address, onBack }: { address: StaffAddress; onBack: () => void }): ReactNode {
  const pages = useAddressPages(address);
  const { t } = useStaffStrings();
  const moment = useMoment();
  const page = useDomainListSubscriptions(pages.cursor);
  const head = (
    <ScreenTitle
      back={<BackButton onPress={onBack}>{t("staff_domains.actions.back")}</BackButton>}
      actions={
        <LinkButton href={newDomainListPath} size="timeline">
          {t("staff_domains.lists.add")}
        </LinkButton>
      }
    >
      {t("staff_domains.lists")}
    </ScreenTitle>
  );
  if (page.forbidden) return <FeedNotice lead>{t("staff_domains.forbidden")}</FeedNotice>;
  if (page.status !== "success")
    return (
      <ScreenStack>
        {head}
        <ReadNotice
          status={page.status}
          loading={t("staff_domains.lists.loading")}
          failed={t("staff_domains.lists.failed")}
          retry={page.refetch}
        />
      </ScreenStack>
    );
  const columns: RecordColumn<Column>[] = [
    { key: "mode", label: t("staff_domains.lists.column.mode"), kind: "fit", bare: true },
    { key: "fetched", label: t("staff_domains.lists.column.fetched"), beside: true },
  ];
  const rows = page.items.map((s): RecordRow<Column> => ({
    id: s.id,
    head: s.title,
    caption: s.url,
    href: domainListPath(s.id),
    cells: {
      mode: t("staff_domains.list.mode", { mode: s.mode }),
      fetched:
        s.last_fetch === undefined ? (
          t("staff_domains.lists.never")
        ) : s.consecutive_failures > 0 ? (
          <StateMark state="failed">
            {tied(t("staff_domains.lists.failing", { count: s.consecutive_failures }))}
          </StateMark>
        ) : (
          moment(s.last_fetch.fetched_at)
        ),
    },
  }));
  return (
    <ScreenStack busy={page.stale}>
      {head}
      <RecordTable
        label={t("staff_domains.lists")}
        head={{ label: t("staff_domains.lists.column.title") }}
        columns={columns}
        rows={rows}
        busy={page.stale}
        empty={<FeedNotice>{t("staff_domains.lists.empty")}</FeedNotice>}
        even
      />
      <Pager {...pages.pager(page.next)} />
    </ScreenStack>
  );
}

/** The settings of a subscription, as its forms hold them. */
type Settings = Required<Omit<DomainListSubscriptionChange, "title">>;

const defaults: Settings = {
  mode: "draft",
  suspend_as: "suspend",
  silence_as: "limit",
  apply_reject_media: true,
  apply_reject_reports: true,
  public_comment_from_list: false,
};

/** What a subscription's rows issue here, and whether its differences wait for a moderator: the fields of both forms. */
function SettingsFields({
  settings,
  onChange,
  isDisabled,
}: {
  settings: Settings;
  onChange: (settings: Settings) => void;
  isDisabled: boolean;
}) {
  const { t } = useStaffStrings();
  const id = useId();
  const set = (change: Partial<Settings>) => {
    onChange({ ...settings, ...change });
  };
  return (
    <>
      <LabelledSelect
        label={t("staff_domains.list.form.mode")}
        options={domainListModes.map((m) => ({ value: m, label: t("staff_domains.list.mode.choice", { mode: m }) }))}
        value={settings.mode}
        onChange={(mode) => {
          set({ mode: domainListModes.find((m) => m === mode) ?? "draft" });
        }}
        isDisabled={isDisabled}
      />
      <LabelledSelect
        label={t("staff_domains.list.form.suspend_as")}
        options={domainListSuspendAs.map((k) => ({ value: k, label: t("staff_domains.list.issue_as", { kind: k }) }))}
        value={settings.suspend_as}
        onChange={(kind) => {
          set({ suspend_as: domainListSuspendAs.find((k) => k === kind) ?? "suspend" });
        }}
        isDisabled={isDisabled}
      />
      <LabelledSelect
        label={t("staff_domains.list.form.silence_as")}
        options={domainListSilenceAs.map((k) => ({ value: k, label: t("staff_domains.list.issue_as", { kind: k }) }))}
        value={settings.silence_as}
        onChange={(kind) => {
          set({ silence_as: domainListSilenceAs.find((k) => k === kind) ?? "limit" });
        }}
        isDisabled={isDisabled}
      />
      <Stack>
        <CheckField
          id={`${id}-media`}
          name="apply_reject_media"
          value="true"
          label={t("staff_domains.list.form.reject_media")}
          checked={settings.apply_reject_media}
          onChange={(on) => {
            set({ apply_reject_media: on });
          }}
          disabled={isDisabled}
          touch
        />
        <CheckField
          id={`${id}-reports`}
          name="apply_reject_reports"
          value="true"
          label={t("staff_domains.list.form.reject_reports")}
          checked={settings.apply_reject_reports}
          onChange={(on) => {
            set({ apply_reject_reports: on });
          }}
          disabled={isDisabled}
          touch
        />
        <CheckField
          id={`${id}-comment`}
          name="public_comment_from_list"
          value="true"
          label={t("staff_domains.list.form.comment")}
          checked={settings.public_comment_from_list}
          onChange={(on) => {
            set({ public_comment_from_list: on });
          }}
          disabled={isDisabled}
          touch
        />
      </Stack>
    </>
  );
}

/** The member of a refusal's 422, put under its field, else the form's alert. */
function useRefusal() {
  const { t } = useStaffStrings();
  const [faults, setFaults] = useState<Partial<Record<string, string>>>({});
  const [alert, setAlert] = useState<string | undefined>(undefined);
  const refuse = (e: unknown, ids: Readonly<Record<string, string>>) => {
    const member = refusedMember(e);
    const field = member === undefined ? undefined : ids[member.member];
    if (member !== undefined && field !== undefined) {
      setFaults({ [member.member]: member.said });
      requestAnimationFrame(() => document.getElementById(field)?.focus());
    } else setAlert(member?.said ?? refusalWords(t, e));
  };
  const clear = () => {
    setFaults({});
    setAlert(undefined);
  };
  return { faults, setFaults, alert, refuse, clear };
}

/** A new subscription: its form, which opens the subscription's page once subscribed. */
export function NewDomainListScreen({
  onBack,
  onSubscribed,
}: {
  onBack: () => void;
  onSubscribed: (id: string) => void;
}) {
  const { t } = useStaffStrings();
  const { subscribe } = useDomainListWrites();
  const id = useId();
  const ids = { title: `${id}-title`, url: `${id}-url` };
  const [title, setTitle] = useState("");
  const [url, setUrl] = useState("");
  const [format, setFormat] = useState<(typeof domainListFormats)[number]>("mastodon_csv");
  const [settings, setSettings] = useState<Settings>(defaults);
  const [busy, setBusy] = useState(false);
  const { faults, setFaults, alert, refuse, clear } = useRefusal();
  const submit = () => {
    if (busy) return;
    clear();
    const blank = {
      ...(title.trim() === "" ? { title: t("staff_domains.list.form.title.blank") } : {}),
      ...(url.trim() === "" ? { url: t("staff_domains.list.form.url.blank") } : {}),
    };
    const first = Object.keys(blank)[0];
    if (first !== undefined) {
      setFaults(blank);
      document.getElementById(first === "title" ? ids.title : ids.url)?.focus();
      return;
    }
    setBusy(true);
    subscribe({ title: title.trim(), url: url.trim(), format, ...settings }).then(
      (subscription) => {
        setBusy(false);
        onSubscribed(subscription.id);
      },
      (e: unknown) => {
        setBusy(false);
        refuse(e, ids);
      },
    );
  };
  return (
    <ScreenStack>
      <ScreenTitle back={<BackButton onPress={onBack}>{t("staff_domains.lists")}</BackButton>}>
        {t("staff_domains.lists.add")}
      </ScreenTitle>
      <Stack
        gap="screen-field-gap"
        as={
          <form
            onSubmit={(e) => {
              e.preventDefault();
              submit();
            }}
          />
        }
      >
        <TextField
          id={ids.title}
          name="title"
          label={t("staff_domains.list.form.title")}
          maxLength={domainListTitleLimit}
          error={faults["title"]}
          onChange={setTitle}
        />
        <TextField
          id={ids.url}
          name="url"
          label={t("staff_domains.list.form.url")}
          hint={t("staff_domains.list.form.url.hint")}
          autoCapitalize="none"
          maxLength={domainListUrlLimit}
          error={faults["url"]}
          onChange={setUrl}
        />
        <LabelledSelect
          label={t("staff_domains.list.form.format")}
          options={domainListFormats.map((f) => ({ value: f, label: t("staff_domains.list.format", { format: f }) }))}
          value={format}
          onChange={(value) => {
            setFormat(domainListFormats.find((f) => f === value) ?? "mastodon_csv");
          }}
          isDisabled={busy}
        />
        <SettingsFields settings={settings} onChange={setSettings} isDisabled={busy} />
        {alert !== undefined && <FormAlert regular>{alert}</FormAlert>}
        <Row>
          <Button type="submit" kind="primary" busy={busy}>
            {t("staff_domains.list.form.subscribe")}
          </Button>
        </Row>
      </Stack>
    </ScreenStack>
  );
}

/** The title of a subscription's page, its title; undefined until it is read. */
export function useDomainListTitle(id: string): string | undefined {
  return useDomainListSubscription(id).value?.title;
}

export type DomainListScreenProps = {
  id: string;
  onBack: () => void;
  /** Goes back to the list once the subscription is stopped. */
  onRemoved: () => void;
};

/** The subscription of `id` on its page; or why it shows none. */
export function DomainListScreen({ id, onBack, onRemoved }: DomainListScreenProps): ReactNode {
  const { t } = useStaffStrings();
  const read = useDomainListSubscription(id);
  if (read.missing)
    return (
      <FeedNotice alert lead action={<TextLink href={domainListsPath}>{t("staff_domains.list.all")}</TextLink>}>
        {t("staff_domains.list.missing")}
      </FeedNotice>
    );
  if (read.forbidden) return <FeedNotice lead>{t("staff_domains.forbidden")}</FeedNotice>;
  if (read.value === undefined)
    return (
      <ReadNotice
        status={read.status === "error" ? "error" : "pending"}
        loading={t("staff_domains.list.loading")}
        failed={t("staff_domains.list.failed")}
        retry={read.refetch}
      />
    );
  const subscription = read.value;
  return (
    <ScreenStack>
      <ScreenTitle back={<BackButton onPress={onBack}>{t("staff_domains.lists")}</BackButton>}>
        {subscription.title}
      </ScreenTitle>
      <ListHead subscription={subscription} />
      <Drafts id={id} />
      <Fetches id={id} />
      <ListSettings key={subscription.updated_at} subscription={subscription} />
      <StopSubscribing subscription={subscription} onRemoved={onRemoved} />
    </ScreenStack>
  );
}

/** What a subscription is, and Fetch now. */
function ListHead({ subscription: s }: { subscription: DomainListSubscription }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const { fetchNow } = useDomainListWrites();
  const { say, status } = useSaid();
  const [busy, setBusy] = useState(false);
  const items: Fact[] = [
    { term: t("staff_domains.list.fact.url"), value: s.url, code: true },
    { term: t("staff_domains.list.fact.format"), value: t("staff_domains.list.format", { format: s.format }) },
    { term: t("staff_domains.list.fact.mode"), value: t("staff_domains.list.mode", { mode: s.mode }) },
    {
      term: t("staff_domains.list.fact.last_fetch"),
      value: s.last_fetch === undefined ? t("staff_domains.lists.never") : moment(s.last_fetch.fetched_at),
    },
    { term: t("staff_domains.list.fact.created"), value: moment(s.created_at) },
  ];
  return (
    <Stack gap="entry-room">
      {/* That its fetches fail is said once, here, as the list of subscriptions marks it. */}
      {s.consecutive_failures > 0 && (
        <Row>
          <StateMark state="failed">
            {tied(t("staff_domains.lists.failing", { count: s.consecutive_failures }))}
          </StateMark>
        </Row>
      )}
      <Row>
        <Button
          size="timeline"
          busy={busy}
          onPress={() => {
            setBusy(true);
            fetchNow(s.id).then(
              () => {
                setBusy(false);
                say(t("staff_domains.list.fetching"));
              },
              (e: unknown) => {
                setBusy(false);
                say(refusalWords(t, e), true);
              },
            );
          }}
        >
          {t("staff_domains.list.fetch")}
        </Button>
      </Row>
      {status}
      <FactList label={t("staff_domains.list.facts")} items={items} />
    </Stack>
  );
}

/** The drafts that wait, the lifts first, which the reader accepts or rejects as picked. */
function Drafts({ id }: { id: string }) {
  const { t } = useStaffStrings();
  const read = useDomainListDrafts(id);
  const { accept, reject } = useDomainListWrites();
  const { say, status } = useSaid();
  const [picked, setPicked] = useState<ReadonlySet<string>>(new Set());
  const [busy, setBusy] = useState<"accept" | "reject" | undefined>(undefined);
  const [asking, setAsking] = useState(false);
  const drafts = read.value ?? [];
  const chosen = drafts.filter((d) => picked.has(d.key));
  const suspends = chosen.filter((d) => d.change === "issue" && d.kind === "suspend").length;
  const decide = (how: "accept" | "reject") => {
    setBusy(how);
    (how === "accept" ? accept : reject)(id, [...picked]).then(
      (result) => {
        setBusy(undefined);
        setAsking(false);
        setPicked(new Set());
        say(
          tied(
            t(how === "accept" ? "staff_domains.drafts.accepted" : "staff_domains.drafts.rejected", {
              count: result.done.length,
              missing: result.missing.length,
            }),
          ),
        );
      },
      (e: unknown) => {
        setBusy(undefined);
        setAsking(false);
        say(refusalWords(t, e), true);
      },
    );
  };
  const rows = drafts.map((d): RecordRow<"change" | "held"> => ({
    id: d.key,
    head: d.domain,
    caption: d.public_comment,
    cells: {
      change: t("staff_domains.draft.change", { change: d.change, kind: domainKindWords(t, d.kind) }),
      held: t("staff_domains.draft.held", { held: d.held }),
    },
  }));
  return (
    <Section title={t("staff_domains.drafts")}>
      <Stack gap="entry-line">
        {read.value === undefined ? (
          <ReadNotice
            status={read.status === "error" ? "error" : "pending"}
            loading={t("staff_domains.drafts.loading")}
            failed={t("staff_domains.drafts.failed")}
            retry={read.refetch}
          />
        ) : (
          <RecordTable
            label={t("staff_domains.drafts")}
            head={{ label: t("staff_domains.drafts.column.domain") }}
            columns={[
              { key: "change", label: t("staff_domains.drafts.column.change") },
              { key: "held", label: t("staff_domains.drafts.column.held"), bare: true },
            ]}
            rows={rows}
            selection={{
              selected: picked,
              onChange: setPicked,
              actions: [
                {
                  id: "accept",
                  label: t("staff_domains.drafts.accept"),
                  busy: busy === "accept" && !asking,
                  onPress: () => {
                    if (busy !== undefined) return;
                    if (suspends > 0) setAsking(true);
                    else decide("accept");
                  },
                },
                {
                  id: "reject",
                  label: t("staff_domains.drafts.reject"),
                  busy: busy === "reject",
                  onPress: () => {
                    if (busy === undefined) decide("reject");
                  },
                },
              ],
            }}
            empty={<FeedNotice start>{t("staff_domains.drafts.empty")}</FeedNotice>}
          />
        )}
        {status}
      </Stack>
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            title={tied(t("staff_domains.drafts.confirm.title", { count: chosen.length }))}
            cancel={t("staff_domains.cancel")}
            confirm={t("staff_domains.drafts.accept")}
            busy={busy === "accept"}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={() => {
              decide("accept");
            }}
          >
            {tied(t("staff_domains.drafts.confirm.about", { count: suspends }))}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </Section>
  );
}

/** The fetches of the list, newest first, grown under Show more. */
function Fetches({ id }: { id: string }) {
  const { t } = useStaffStrings();
  const list = useDomainListFetches(id);
  const added = useAddedFocus(list.items.length);
  return (
    <Section title={t("staff_domains.fetches")}>
      {list.items.length === 0 && list.status !== "success" ? (
        <ReadNotice
          status={list.status}
          loading={t("staff_domains.fetches.loading")}
          failed={t("staff_domains.fetches.failed")}
          retry={list.refetch}
        />
      ) : list.items.length === 0 ? (
        <FeedNotice start>{t("staff_domains.fetches.empty")}</FeedNotice>
      ) : (
        <Stack gap="entry-room">
          <LogList label={t("staff_domains.fetches")}>
            {list.items.map((f, i) => (
              <LogEntry key={f.id} itemRef={added.item(i)} icon={fetchGlyph(f)} date={f.fetched_at}>
                {fetchWords(t, f)}
              </LogEntry>
            ))}
          </LogList>
          <ListMore list={list} added={added} />
        </Stack>
      )}
    </Section>
  );
}

/** The settings of a subscription, saved as a whole; the URL and the format do not change. */
function ListSettings({ subscription: s }: { subscription: DomainListSubscription }) {
  const { t } = useStaffStrings();
  const { change } = useDomainListWrites();
  const id = useId();
  const ids = { title: `${id}-title` };
  const [title, setTitle] = useState(s.title);
  const [settings, setSettings] = useState<Settings>({
    mode: s.mode,
    suspend_as: s.suspend_as,
    silence_as: s.silence_as,
    apply_reject_media: s.apply_reject_media,
    apply_reject_reports: s.apply_reject_reports,
    public_comment_from_list: s.public_comment_from_list,
  });
  const [busy, setBusy] = useState(false);
  const { say, status } = useSaid();
  const { faults, setFaults, alert, refuse, clear } = useRefusal();
  const submit = () => {
    if (busy) return;
    clear();
    if (title.trim() === "") {
      setFaults({ title: t("staff_domains.list.form.title.blank") });
      document.getElementById(ids.title)?.focus();
      return;
    }
    setBusy(true);
    change(s.id, { title: title.trim(), ...settings }).then(
      () => {
        setBusy(false);
        say(t("staff_domains.list.saved"));
      },
      (e: unknown) => {
        setBusy(false);
        refuse(e, ids);
      },
    );
  };
  return (
    <Section title={t("staff_domains.list.settings")}>
      <Stack
        gap="screen-field-gap"
        as={
          <form
            onSubmit={(e) => {
              e.preventDefault();
              submit();
            }}
          />
        }
      >
        <TextField
          id={ids.title}
          name="title"
          label={t("staff_domains.list.form.title")}
          defaultValue={s.title}
          maxLength={domainListTitleLimit}
          error={faults["title"]}
          onChange={setTitle}
        />
        <SettingsFields settings={settings} onChange={setSettings} isDisabled={busy} />
        {alert !== undefined && <FormAlert regular>{alert}</FormAlert>}
        <Row>
          <Button type="submit" busy={busy}>
            {t("staff_domains.list.save")}
          </Button>
        </Row>
        {status}
      </Stack>
    </Section>
  );
}

/** Stop subscribing, last and apart, which asks whether to lift the actions the subscription issued. */
function StopSubscribing({ subscription, onRemoved }: { subscription: DomainListSubscription; onRemoved: () => void }) {
  const { t } = useStaffStrings();
  const { remove } = useDomainListWrites();
  const id = useId();
  const [asking, setAsking] = useState(false);
  const [lift, setLift] = useState(false);
  const [busy, setBusy] = useState(false);
  const [alert, setAlert] = useState<string | undefined>(undefined);
  return (
    <Section title={t("staff_domains.list.stop")}>
      <Stack gap="screen-field-gap">
        {alert !== undefined && <FormAlert regular>{alert}</FormAlert>}
        <Row>
          <Button
            size="timeline"
            onPress={() => {
              setAlert(undefined);
              setLift(false);
              setAsking(true);
            }}
          >
            {t("staff_domains.list.stop.ask")}
          </Button>
        </Row>
      </Stack>
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            title={t("staff_domains.list.stop.title", { title: subscription.title })}
            cancel={t("staff_domains.cancel")}
            confirm={t("staff_domains.list.stop.confirm")}
            busy={busy}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={() => {
              setBusy(true);
              remove(subscription.id, lift).then(
                () => {
                  setBusy(false);
                  setAsking(false);
                  onRemoved();
                },
                (e: unknown) => {
                  setBusy(false);
                  setAsking(false);
                  setAlert(refusalWords(t, e));
                },
              );
            }}
          >
            <Stack gap="entry-line">
              <span>{t("staff_domains.list.stop.about")}</span>
              <CheckField
                id={`${id}-lift`}
                name="lift_actions"
                value="true"
                label={t("staff_domains.list.stop.lift")}
                checked={lift}
                onChange={setLift}
                touch
              />
            </Stack>
          </ConfirmDialog>
        )}
      </ModalPresence>
    </Section>
  );
}
