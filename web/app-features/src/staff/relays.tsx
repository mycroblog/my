// SPDX-License-Identifier: AGPL-3.0-only

// The relays the instance joins (FR-724, ADR-0006), as a holder of `manage_settings` keeps them: at /admin/relays,
// each relay a door to its page, where its membership stands and whether it brings posts in, with Add a relay; at
// /admin/relays/new, a relay registered by its URL and protocol, joined at once (its Follow sent) and taking in what it
// relays unless the reader says it only sends; at /admin/relays/{id}, what is known of it and what can be done now:
// leave it while it is joined or waits, join it again or delete it once it was left or rejected the server. Leaving and
// deleting ask first. What the server refuses of the URL stands under it, in the words of its reason.

import {
  loadAdminRelays,
  refusalDetail,
  relayProtocols,
  relayRefusalOf,
  useAdminRelay,
  useAdminRelays,
  useAdminRelayWrites,
  type AdminRelay,
  type AppData,
  type RelayProtocol,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  ConfirmDialog,
  focusRefusedField,
  FormAlert,
  LinkButton,
  ModalPresence,
  Select,
  SettingRow,
  SettingsEmpty,
  SettingsFacts,
  SettingsList,
  SettingsListRow,
  SettingsResult,
  SettingsScreen,
  SettingsSection,
  SubmitBar,
  TextField,
} from "@my/design-system";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { useMoment, useStaffStrings } from "./parts";
import { LeaveDialog, OnOffRow, ScreenNotice, type AdminSubProps } from "./settings/parts";

/** The address of the relays. */
const adminRelaysPath = "/admin/relays";
/** The address of a new relay's form. */
const newRelayPath = `${adminRelaysPath}/new`;
/** The address of the relay of `id`. */
const relayPath = (id: string): string => `${adminRelaysPath}/${id}`;

/** Reads the relays, for the routes' loaders. */
export const loadRelays = (data: AppData): Promise<void> => loadAdminRelays(data);

/** The host of a relay's URL, as a row names it; the URL itself where it has none. */
const hostOf = (url: string): string => {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
};

/** Whether a relay is joined or waits to be: what the server sends to it. */
const joined = (relay: AdminRelay) => relay.state === "pending" || relay.state === "accepted";

/** The relays, each a door to its page, and Add a relay. */
export function AdminRelaysScreen(): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminRelays();
  const title = t("staff.entry.relays");
  if (read.relays === undefined)
    return (
      <SettingsScreen
        sub
        icon="federated"
        title={title}
        notice={
          <ScreenNotice
            status={read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{ loading: t("staff_settings.relays.loading"), failed: t("staff_settings.relays.failed") }}
          />
        }
      />
    );
  return (
    <SettingsScreen sub icon="federated" title={title}>
      <SettingsSection
        heading={t("staff_settings.relays.list")}
        count={String(read.relays.length)}
        about={t("staff_settings.relays.about")}
        action={
          <LinkButton size="timeline" href={newRelayPath}>
            {t("staff_settings.relays.add")}
          </LinkButton>
        }
      >
        {read.relays.length === 0 ? (
          <SettingsEmpty>{t("staff_settings.relays.empty")}</SettingsEmpty>
        ) : (
          <SettingsList label={t("staff_settings.relays.list")}>
            {read.relays.map((relay) => (
              <SettingsListRow
                key={relay.id}
                title={hostOf(relay.url)}
                inactive={!joined(relay) || relay.domain_suspended}
                state={t("staff_settings.relays.state", {
                  state: relay.domain_suspended ? "suspended" : relay.state,
                })}
                facts={[
                  t("staff_settings.relays.receive", { receive: relay.receive ? "yes" : "no" }),
                  t("staff_settings.relays.last_day", {
                    taken: relay.taken_last_day,
                    dropped: relay.dropped_last_day,
                  }),
                ]}
                href={relayPath(relay.id)}
              />
            ))}
          </SettingsList>
        )}
      </SettingsSection>
    </SettingsScreen>
  );
}

/** The way back to the relays. */
function BackToRelays({ go }: { go: (href: string) => void }) {
  const { t } = useStaffStrings();
  return (
    <BackButton
      onPress={() => {
        go(adminRelaysPath);
      }}
    >
      {t("staff.entry.relays")}
    </BackButton>
  );
}

/** A relay to register: its URL, how it is joined, and whether what it relays is taken in. */
export function NewRelayScreen({ go, onUnsaved, leaving }: AdminSubProps): ReactNode {
  const { t } = useStaffStrings();
  const id = useId();
  const writes = useAdminRelayWrites();
  const [url, setUrl] = useState("");
  const [protocol, setProtocol] = useState<RelayProtocol | "auto">("auto");
  const [receive, setReceive] = useState(true);
  const [sent, setSent] = useState<
    { kind: "idle" | "sending" } | { kind: "refused"; field?: string | undefined; reason?: string | undefined }
  >({ kind: "idle" });
  const busy = sent.kind === "sending";
  const unsaved = url !== "" || protocol !== "auto" || !receive;
  const focusNext = useRef(false);
  useEffect(() => {
    if (!focusNext.current) return;
    focusNext.current = false;
    focusRefusedField(`${id}-url`);
  });
  useEffect(() => {
    onUnsaved?.(unsaved && !busy);
  }, [onUnsaved, unsaved, busy]);
  useEffect(
    () => () => {
      onUnsaved?.(false);
    },
    [onUnsaved],
  );
  const clear = () => {
    setSent((was) => (was.kind === "refused" ? { kind: "idle" } : was));
  };
  const save = () => {
    if (busy) return;
    if (!url.trim().startsWith("https://")) {
      setSent({ kind: "refused", field: t("staff_settings.relays.refused", { reason: "url" }) });
      focusNext.current = true;
      return;
    }
    setSent({ kind: "sending" });
    writes.create({ url: url.trim(), receive, ...(protocol === "auto" ? {} : { protocol }) }).then(
      (relay) => {
        onUnsaved?.(false);
        go(relayPath(relay.id));
      },
      (error: unknown) => {
        const reason = relayRefusalOf(error);
        if (reason !== undefined) {
          setSent({ kind: "refused", field: t("staff_settings.relays.refused", { reason }) });
          focusNext.current = true;
        } else setSent({ kind: "refused", reason: refusalDetail(error) });
      },
    );
  };
  return (
    <>
      <SettingsScreen
        sub
        title={t("staff_settings.relays.new")}
        back={<BackToRelays go={go} />}
        foot={
          <SubmitBar
            result={
              sent.kind === "refused" ? (
                <FormAlert>
                  {sent.reason ??
                    t(sent.field === undefined ? "staff_settings.save.failed" : "staff_settings.save.refused")}
                </FormAlert>
              ) : undefined
            }
          >
            <Button kind="primary" busy={busy} onPress={save}>
              {t("staff_settings.relays.join")}
            </Button>
          </SubmitBar>
        }
      >
        <SettingsSection heading={t("staff_settings.relays.section")} about={t("staff_settings.relays.new.about")}>
          <TextField
            id={`${id}-url`}
            name="url"
            label={t("staff_settings.relays.url")}
            row
            hint={t("staff_settings.relays.url.about")}
            autoCapitalize="none"
            inputMode="text"
            maxLength={2048}
            error={sent.kind === "refused" ? sent.field : undefined}
            onChange={(value) => {
              setUrl(value);
              clear();
            }}
          />
          <SettingRow
            id={`${id}-protocol`}
            label={t("staff_settings.relays.protocol")}
            about={t("staff_settings.relays.protocol.about")}
            form
          >
            <Select
              name="protocol"
              options={(["auto", ...relayProtocols] as const).map((p) => ({
                value: p,
                label: t("staff_settings.relays.protocol.choice", { protocol: p }),
              }))}
              value={protocol}
              onChange={(value) => {
                setProtocol(value === "mastodon" || value === "litepub" ? value : "auto");
              }}
              isDisabled={busy}
            />
          </SettingRow>
          <OnOffRow
            id={`${id}-receive`}
            label={t("staff_settings.relays.receive.check")}
            on={receive}
            onChange={setReceive}
            disabled={busy}
          />
        </SettingsSection>
      </SettingsScreen>
      <ModalPresence>{leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />}</ModalPresence>
    </>
  );
}

/** What a relay's page asks before. */
type Ask = "leave" | "delete";

/** A relay: what is known of it, and what can be done now. */
function Relay({ relay, go }: { relay: AdminRelay; go: (href: string) => void }) {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const writes = useAdminRelayWrites();
  const [asking, setAsking] = useState<Ask | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const [said, setSaid] = useState<string | undefined>(undefined);
  const act = (write: () => Promise<unknown>, after?: () => void) => {
    setBusy(true);
    setSaid(undefined);
    write().then(
      () => {
        setBusy(false);
        setAsking(undefined);
        after?.();
      },
      (error: unknown) => {
        setBusy(false);
        setAsking(undefined);
        const reason = relayRefusalOf(error);
        setSaid(
          reason === undefined
            ? (refusalDetail(error) ?? t("staff_settings.act_failed"))
            : t("staff_settings.relays.refused", { reason }),
        );
      },
    );
  };
  const state = relay.domain_suspended ? "suspended" : relay.state;
  return (
    <>
      <SettingsScreen sub title={hostOf(relay.url)} back={<BackToRelays go={go} />}>
        <SettingsSection heading={t("staff_settings.relays.section")}>
          <SettingsFacts
            label={t("staff_settings.relays.section")}
            items={[
              {
                text: t("staff_settings.relays.state", { state }),
                about: [t("staff_settings.relays.since", { date: moment(relay.state_changed_at) })],
              },
              { text: t("staff_settings.relays.receive", { receive: relay.receive ? "yes" : "no" }) },
              {
                text: t("staff_settings.relays.last_day", {
                  taken: relay.taken_last_day,
                  dropped: relay.dropped_last_day,
                }),
              },
              {
                text: t("staff_settings.relays.protocol.choice", { protocol: relay.protocol }),
                about: [relay.url, ...(relay.inbox_url === relay.url ? [] : [relay.inbox_url])],
              },
            ]}
          />
        </SettingsSection>
        {joined(relay) ? (
          <SettingsSection heading={t("staff_settings.relays.leave")} about={t("staff_settings.relays.leave.about")}>
            <Box>
              <Button
                size="timeline"
                touch
                isDisabled={busy}
                onPress={() => {
                  setAsking("leave");
                }}
              >
                {t("staff_settings.relays.leave.press")}
              </Button>
            </Box>
            <SettingsResult>{said}</SettingsResult>
          </SettingsSection>
        ) : (
          <>
            <SettingsSection
              heading={t("staff_settings.relays.rejoin")}
              about={t("staff_settings.relays.rejoin.about")}
            >
              <Box>
                <Button
                  size="timeline"
                  touch
                  isDisabled={busy || relay.domain_suspended}
                  onPress={() => {
                    act(() => writes.join(relay.id));
                  }}
                >
                  {t("staff_settings.relays.rejoin.press")}
                </Button>
              </Box>
              <SettingsResult>{said}</SettingsResult>
            </SettingsSection>
            <SettingsSection
              heading={t("staff_settings.relays.delete")}
              about={t("staff_settings.relays.delete.about")}
            >
              <Box>
                <Button
                  size="timeline"
                  touch
                  isDisabled={busy}
                  onPress={() => {
                    setAsking("delete");
                  }}
                >
                  {t("staff_settings.relays.delete.press")}
                </Button>
              </Box>
            </SettingsSection>
          </>
        )}
      </SettingsScreen>
      <ModalPresence>
        {asking === "leave" ? (
          <ConfirmDialog
            key="leave"
            title={t("staff_settings.relays.leave.title")}
            cancel={t("staff_settings.ask.cancel")}
            confirm={t("staff_settings.relays.leave.press")}
            busy={busy}
            onCancel={() => {
              setAsking(undefined);
            }}
            onConfirm={() => {
              act(() => writes.leave(relay.id));
            }}
          >
            {t("staff_settings.relays.leave.text", { host: hostOf(relay.url) })}
          </ConfirmDialog>
        ) : (
          asking === "delete" && (
            <ConfirmDialog
              key="delete"
              title={t("staff_settings.relays.delete.title")}
              cancel={t("staff_settings.ask.cancel")}
              confirm={t("staff_settings.relays.delete.press")}
              busy={busy}
              onCancel={() => {
                setAsking(undefined);
              }}
              onConfirm={() => {
                act(
                  () => writes.remove(relay.id),
                  () => {
                    go(adminRelaysPath);
                  },
                );
              }}
            >
              {t("staff_settings.relays.delete.text", { host: hostOf(relay.url) })}
            </ConfirmDialog>
          )
        )}
      </ModalPresence>
    </>
  );
}

/** The page of the relay of `id`, or that it loads, failed or is none. */
export function RelayScreen({ id, go }: AdminSubProps & { id: string }): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminRelay(id);
  if (read.relay === undefined)
    return (
      <SettingsScreen
        sub
        title={t("staff.entry.relays")}
        back={<BackToRelays go={go} />}
        notice={
          <ScreenNotice
            status={read.missing ? "missing" : read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{
              loading: t("staff_settings.relays.loading"),
              failed: t("staff_settings.relays.failed"),
              missing: t("staff_settings.relays.missing"),
            }}
          />
        }
      />
    );
  return <Relay relay={read.relay} go={go} />;
}

/** The title of the page of the relay of `id`: its host, once read. */
export function useRelayTitle(id: string): string | undefined {
  const relay = useAdminRelay(id).relay;
  return relay === undefined ? undefined : hostOf(relay.url);
}
