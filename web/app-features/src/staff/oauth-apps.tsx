// SPDX-License-Identifier: AGPL-3.0-only

// The OAuth apps (ADR-0090 §1) as a holder of `manage_oauth_apps` keeps them: at /admin/oauth-apps, every app that is
// not deleted, newest first, a page at a time, kept to one way of registration by the address's query; each its name,
// how it was registered, its client ID and who registered it, where it stands, and Disable or Enable (the server's own
// apps neither). Disabling revokes every token of the app at once, so it asks first, and its users authorize it again
// once it is enabled. A door leads to /admin/oauth-apps/hosts: the hosts whose client metadata documents are denied,
// a host denied (which revokes the tokens of its apps at once, asked first), and a denial lifted.

import { sentenceHandleOf } from "@my/app-components";
import {
  loadAdminOAuthClients,
  loadOAuthHostDenials,
  oAuthClientRegistrations,
  refusalDetail,
  refusedField,
  useAdminOAuthClients,
  useAdminOAuthWrites,
  useOAuthHostDenials,
  type AdminOAuthClient,
  type AppData,
  type OAuthClientRegistration,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  ConfirmDialog,
  focusRefusedField,
  ModalPresence,
  RecordPages,
  Select,
  SettingRow,
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsResult,
  SettingsScreen,
  SettingsSection,
  TextField,
} from "@my/design-system";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import type { StaffAddress, StaffSearch } from "./entries";
import { useMoment, useStaffStrings } from "./parts";
import { ScreenNotice, type AdminSubProps } from "./settings/parts";

/** The address of the apps. */
const adminOAuthAppsPath = "/admin/oauth-apps";
/** The address of the denied hosts. */
const oAuthHostsPath = `${adminOAuthAppsPath}/hosts`;

/** The way of registration the address's query keeps the list to; every way where it names none. */
const registrationOf = (search: StaffSearch): OAuthClientRegistration | undefined =>
  oAuthClientRegistrations.find((r) => r === search["registration"]);

/** The cursors of the pages before the one shown, the first page's as empty: the way back by Previous. */
const trailOf = (search: StaffSearch): string[] =>
  search["trail"] === undefined ? [] : search["trail"].split(",").map((c) => decodeURIComponent(c));

/** Reads the page of the apps the address asks for, for the loader. */
export const loadOAuthApps = (data: AppData, search: StaffSearch): Promise<void> =>
  loadAdminOAuthClients(data, { registration: registrationOf(search), cursor: search["cursor"] });

/** Reads the first page of the denied hosts, for the route's loader. */
export const loadOAuthHosts = (data: AppData): Promise<void> => loadOAuthHostDenials(data);

/** An app's row: what it is, where it stands, and Disable or Enable. */
function AppRow({
  app,
  said,
  busy,
  onAct,
}: {
  app: AdminOAuthClient;
  said: string | undefined;
  busy: boolean;
  onAct: (app: AdminOAuthClient) => void;
}) {
  const { t } = useStaffStrings();
  const owner =
    app.owner !== undefined
      ? t("staff_settings.apps.owner", { handle: sentenceHandleOf(app.owner) })
      : app.owner_deleted
        ? t("staff_settings.apps.owner_gone")
        : undefined;
  return (
    <SettingsListRow
      title={app.name}
      inactive={app.state !== "enabled"}
      standing={t("staff_settings.apps.state", { state: app.state })}
      facts={[
        t("staff_settings.apps.registration", { registration: app.registration }),
        app.client_id,
        ...(owner === undefined ? [] : [owner]),
      ]}
      said={said === undefined ? null : { text: said, n: 1 }}
      end={
        app.registration === "first_party" ? undefined : (
          <Button
            size="timeline"
            touch
            isDisabled={busy}
            onPress={() => {
              onAct(app);
            }}
          >
            {t("staff_settings.apps.act", { state: app.state })}
          </Button>
        )
      }
    />
  );
}

/** The apps, a page at a time, kept to one way of registration; and the door to the denied hosts. */
export function AdminOAuthAppsScreen({ search, onSearch }: StaffAddress): ReactNode {
  const { t } = useStaffStrings();
  const id = useId();
  const registration = registrationOf(search);
  const cursor = search["cursor"];
  const trail = trailOf(search);
  const page = useAdminOAuthClients({ registration, cursor });
  const writes = useAdminOAuthWrites();
  const [asking, setAsking] = useState<AdminOAuthClient | undefined>(undefined);
  const [busy, setBusy] = useState<string | undefined>(undefined);
  const [said, setSaid] = useState<{ id: string; text: string } | undefined>(undefined);
  const title = t("staff.entry.oauth_apps");
  const go = (next: { registration?: string | undefined; cursor?: string | undefined; trail?: string[] }) => {
    onSearch({
      ...(next.registration === undefined ? {} : { registration: next.registration }),
      ...(next.cursor === undefined ? {} : { cursor: next.cursor }),
      ...(next.trail === undefined || next.trail.length === 0
        ? {}
        : { trail: next.trail.map((c) => encodeURIComponent(c)).join(",") }),
    });
  };
  const act = (app: AdminOAuthClient) => {
    setBusy(app.id);
    setSaid(undefined);
    const write = app.state === "enabled" ? writes.disable(app.id) : writes.enable(app.id);
    write.then(
      (now) => {
        setBusy(undefined);
        setAsking(undefined);
        setSaid({ id: app.id, text: t("staff_settings.apps.done", { state: now.state }) });
      },
      (error: unknown) => {
        setBusy(undefined);
        setAsking(undefined);
        setSaid({ id: app.id, text: refusalDetail(error) ?? t("staff_settings.act_failed") });
      },
    );
  };
  const filter = (
    <SettingsSection heading={t("staff_settings.apps.filter")}>
      <SettingRow id={`${id}-registration`} label={t("staff_settings.apps.filter.registration")} save={null}>
        <Select
          name="registration"
          options={(["any", ...oAuthClientRegistrations] as const).map((r) => ({
            value: r,
            label: t("staff_settings.apps.registration", { registration: r }),
          }))}
          value={registration ?? "any"}
          onChange={(value) => {
            go({ registration: value === "any" ? undefined : value });
          }}
        />
      </SettingRow>
    </SettingsSection>
  );
  const hosts = (
    <SettingsSection heading={t("staff_settings.hosts")} about={t("staff_settings.hosts.about")}>
      <SettingsList label={t("staff_settings.hosts")}>
        <SettingsListRow title={t("staff_settings.hosts.door")} href={oAuthHostsPath} />
      </SettingsList>
    </SettingsSection>
  );
  if (page.status !== "success")
    return (
      <SettingsScreen
        icon="apps"
        title={title}
        notice={
          <ScreenNotice
            status={page.status === "error" ? "failed" : "loading"}
            retry={page.refetch}
            words={{ loading: t("staff_settings.apps.loading"), failed: t("staff_settings.apps.failed") }}
          />
        }
      />
    );
  const previous = trail.at(-1);
  return (
    <>
      <SettingsScreen icon="apps" title={title}>
        {filter}
        <SettingsSection heading={t("staff_settings.apps.list")}>
          {page.items.length === 0 ? (
            <SettingsEmpty>{t("staff_settings.apps.empty")}</SettingsEmpty>
          ) : (
            <SettingsList label={t("staff_settings.apps.list")}>
              {page.items.map((app) => (
                <AppRow
                  key={app.id}
                  app={app}
                  said={said?.id === app.id ? said.text : undefined}
                  busy={busy !== undefined}
                  onAct={(a) => {
                    if (a.state === "enabled") setAsking(a);
                    else act(a);
                  }}
                />
              ))}
            </SettingsList>
          )}
          <RecordPages
            previous={
              previous === undefined
                ? undefined
                : {
                    onPress: () => {
                      go({ registration, cursor: previous === "" ? undefined : previous, trail: trail.slice(0, -1) });
                    },
                  }
            }
            next={
              page.next === undefined
                ? undefined
                : {
                    onPress: () => {
                      go({ registration, cursor: page.next, trail: [...trail, cursor ?? ""] });
                    },
                  }
            }
          />
        </SettingsSection>
        {hosts}
      </SettingsScreen>
      <ModalPresence>
        {asking !== undefined && (
          <ConfirmDialog
            key="disable"
            title={t("staff_settings.apps.disable.title")}
            cancel={t("staff_settings.ask.cancel")}
            confirm={t("staff_settings.apps.act", { state: "enabled" })}
            busy={busy !== undefined}
            onCancel={() => {
              setAsking(undefined);
            }}
            onConfirm={() => {
              act(asking);
            }}
          >
            {t("staff_settings.apps.disable.text", { name: asking.name })}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </>
  );
}

/** The denied hosts: a host to deny, and each denial with Lift. */
export function OAuthHostsScreen({ go }: AdminSubProps): ReactNode {
  const { t } = useStaffStrings();
  const id = useId();
  const moment = useMoment();
  const [trail, setTrail] = useState<string[]>([]);
  const cursor = trail.at(-1);
  const page = useOAuthHostDenials(cursor === "" ? undefined : cursor);
  const writes = useAdminOAuthWrites();
  const [host, setHost] = useState("");
  const [round, setRound] = useState(0);
  const [asking, setAsking] = useState(false);
  const [state, setState] = useState<
    { kind: "idle" | "sending" } | { kind: "refused"; field?: string | undefined; reason?: string | undefined }
  >({ kind: "idle" });
  const [said, setSaid] = useState<{ host: string; text: string } | undefined>(undefined);
  const focusNext = useRef(false);
  useEffect(() => {
    if (!focusNext.current) return;
    focusNext.current = false;
    focusRefusedField(`${id}-host`);
  });
  const deny = () => {
    setState({ kind: "sending" });
    writes.deny(host.trim()).then(
      (denial) => {
        setAsking(false);
        setState({ kind: "idle" });
        setHost("");
        setRound((r) => r + 1);
        setSaid({ host: denial.host, text: t("staff_settings.hosts.denied", { host: denial.host }) });
      },
      (error: unknown) => {
        setAsking(false);
        if (refusedField(error) !== undefined) {
          setState({ kind: "refused", field: t("staff_settings.hosts.rule") });
          focusNext.current = true;
        } else setState({ kind: "refused", reason: refusalDetail(error) ?? t("staff_settings.act_failed") });
      },
    );
  };
  const press = () => {
    if (host.trim() === "" || host.includes("/") || host.includes(":")) {
      setState({ kind: "refused", field: t("staff_settings.hosts.rule") });
      focusNext.current = true;
      return;
    }
    setAsking(true);
  };
  const lift = (h: string) => {
    writes.allow(h).then(
      () => {
        setSaid({ host: h, text: t("staff_settings.hosts.lifted", { host: h }) });
      },
      (error: unknown) => {
        setSaid({ host: h, text: refusalDetail(error) ?? t("staff_settings.act_failed") });
      },
    );
  };
  const back = (
    <BackButton
      onPress={() => {
        go(adminOAuthAppsPath);
      }}
    >
      {t("staff.entry.oauth_apps")}
    </BackButton>
  );
  return (
    <>
      <SettingsScreen sub title={t("staff_settings.hosts")} back={back}>
        <SettingsSection heading={t("staff_settings.hosts.deny")} about={t("staff_settings.hosts.deny.about")}>
          <TextField
            key={round}
            id={`${id}-host`}
            name="host"
            label={t("staff_settings.hosts.host")}
            autoCapitalize="none"
            maxLength={253}
            error={state.kind === "refused" ? state.field : undefined}
            onChange={(value) => {
              setHost(value);
              setState((was) => (was.kind === "refused" ? { kind: "idle" } : was));
            }}
          />
          <Box>
            <Button size="timeline" touch busy={state.kind === "sending"} onPress={press}>
              {t("staff_settings.hosts.deny.press")}
            </Button>
          </Box>
          <SettingsResult>{state.kind === "refused" ? state.reason : undefined}</SettingsResult>
        </SettingsSection>
        <SettingsSection heading={t("staff_settings.hosts.list")}>
          {page.status !== "success" ? (
            <ScreenNotice
              status={page.status === "error" ? "failed" : "loading"}
              retry={page.refetch}
              words={{ loading: t("staff_settings.hosts.loading"), failed: t("staff_settings.hosts.read_failed") }}
            />
          ) : page.items.length === 0 ? (
            <SettingsEmpty>{t("staff_settings.hosts.empty")}</SettingsEmpty>
          ) : (
            <SettingsList label={t("staff_settings.hosts.list")}>
              {page.items.map((d) => (
                <SettingsListRow
                  key={d.id}
                  title={d.host}
                  facts={[t("staff_settings.hosts.since", { date: moment(d.denied_at) })]}
                  said={said?.host === d.host ? { text: said.text, n: 1 } : null}
                  end={
                    <Button
                      size="timeline"
                      touch
                      onPress={() => {
                        lift(d.host);
                      }}
                    >
                      {t("staff_settings.hosts.lift")}
                    </Button>
                  }
                />
              ))}
            </SettingsList>
          )}
          {page.status === "success" && (
            <RecordPages
              previous={
                trail.length === 0
                  ? undefined
                  : {
                      onPress: () => {
                        setTrail((now) => now.slice(0, -1));
                      },
                    }
              }
              next={
                page.next === undefined
                  ? undefined
                  : {
                      onPress: () => {
                        const next = page.next;
                        if (next !== undefined) setTrail((now) => [...now, next]);
                      },
                    }
              }
            />
          )}
          {said !== undefined && !page.items.some((d) => d.host === said.host) && (
            <SettingsResult>{said.text}</SettingsResult>
          )}
        </SettingsSection>
      </SettingsScreen>
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            key="deny"
            title={t("staff_settings.hosts.deny.title")}
            cancel={t("staff_settings.ask.cancel")}
            confirm={t("staff_settings.hosts.deny.press")}
            busy={state.kind === "sending"}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={deny}
          >
            {t("staff_settings.hosts.deny.text", { host: host.trim().toLowerCase() })}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </>
  );
}
