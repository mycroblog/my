// SPDX-License-Identifier: AGPL-3.0-only
// The organizers' regions of an event's page (ADR-0161 §4, §9): the invitations, where an organizer finds local
// users by name and invites them (an invited user sees the event whatever its visibility) or revokes an invitation,
// and, for the organizer alone, the co-organizers (up to three), who change the event, invite, and approve.
import { AccountItem, Button, Hint, InlineError, RegionError, Section, Spinner, Stack, TextField } from "@my/ui";
import type { ReactNode } from "react";
import { handleOf } from "../../../lib/handle";
import { useT } from "../../../lib/i18n";
import { imageSources } from "../../../lib/images";
import { profilePath } from "../../../lib/paths";
import { type Event, type EventAccount, eventLimits } from "../api/events";
import { accountName } from "../hooks/names";
import { type AccountSearch, useCoorganizers, useEventInvitations } from "../hooks/useEventOrganizing";

/** One account as a row, with its action at the end. */
function AccountRow({ account, action }: { account: EventAccount; action?: ReactNode }) {
  return (
    <AccountItem
      name={accountName(account)}
      handle={handleOf(account.acct)}
      avatar={imageSources(account.avatar)}
      avatarSeed={account.id}
      href={profilePath(account.acct)}
      trailing={action}
    />
  );
}

type SearchResultsProps = {
  search: AccountSearch;
  label: string;
  /** The action of a found account, or undefined for one that needs none (invited already). */
  action: (account: EventAccount) => ReactNode;
};

/** The local accounts that a search found, each with its action. */
function SearchResults({ search, label, action }: SearchResultsProps) {
  const t = useT();
  switch (search.status) {
    case "idle":
      return null;
    case "loading":
      return <Spinner label={t("client:events.search.loading")} />;
    case "error":
      return <InlineError size="inherit">{t("client:events.search.failed", { reason: search.reason })}</InlineError>;
    case "done":
      if (search.accounts.length === 0) return <Hint>{t("client:events.search.empty")}</Hint>;
      return (
        <Stack as="ul" gap="3" aria-label={label}>
          {search.accounts.map((account) => (
            <li key={account.id}>
              <AccountRow account={account} action={action(account)} />
            </li>
          ))}
        </Stack>
      );
  }
}

/** The invitations of the event, for its organizers. */
export function EventInvitations({ event }: { event: Event }) {
  const t = useT();
  const invitations = useEventInvitations(event);
  const { list } = invitations;
  return (
    <Section title={t("client:events.invitations.title")} description={t("client:events.invitations.description")}>
      <Stack gap="4">
        <TextField
          type="search"
          label={t("client:events.invitations.search")}
          value={invitations.search}
          onChange={invitations.changeSearch}
          autoComplete="off"
        />
        <SearchResults
          search={invitations.found}
          label={t("client:events.invitations.results")}
          action={(account) =>
            invitations.isInvited(account) ? (
              <Hint>{t("client:events.invitations.already")}</Hint>
            ) : (
              <Button
                size="sm"
                isPending={invitations.invitingId === account.id}
                onPress={() => {
                  invitations.invite(account);
                }}
                aria-label={t("client:events.invitations.invite_of", { name: accountName(account) })}
              >
                {t("client:events.invitations.invite")}
              </Button>
            )
          }
        />
        {invitations.inviteError !== undefined && (
          <InlineError alert size="inherit">
            {t("client:events.invitations.failed", { reason: invitations.inviteError })}
          </InlineError>
        )}
        {list.status === "loading" && <Spinner label={t("client:events.loading")} />}
        {list.status === "error" && (
          <RegionError
            heading={t("client:events.invitations.list_failed")}
            reason={list.reason}
            retryLabel={t("client:events.retry")}
            onRetry={list.retry}
            isRetrying={list.isRetrying}
          />
        )}
        {list.status === "ready" &&
          (list.invitations.length === 0 ? (
            <Hint>{t("client:events.invitations.empty")}</Hint>
          ) : (
            <Stack gap="3">
              <Stack as="ul" gap="3" aria-label={t("client:events.invitations.list")}>
                {list.invitations.map((invitation) => (
                  <li key={invitation.id}>
                    <AccountRow
                      account={invitation.account}
                      action={
                        <Button
                          size="sm"
                          variant="subtle"
                          isPending={invitations.revokingId === invitation.account.id}
                          onPress={() => {
                            invitations.revoke(invitation.account);
                          }}
                          aria-label={t("client:events.invitations.revoke_of", {
                            name: accountName(invitation.account),
                          })}
                        >
                          {t("client:events.invitations.revoke")}
                        </Button>
                      }
                    />
                  </li>
                ))}
              </Stack>
              {list.hasMore && (
                <div>
                  <Button onPress={list.loadMore} isPending={list.isLoadingMore}>
                    {t("client:events.invitations.more")}
                  </Button>
                </div>
              )}
            </Stack>
          ))}
      </Stack>
    </Section>
  );
}

/** The co-organizers of the event, for its organizer: naming them by a search, and removing them. */
export function EventCoorganizers({ event }: { event: Event }) {
  const t = useT();
  const coorganizers = useCoorganizers(event);
  const full = event.coorganizers.length >= eventLimits.coorganizers;
  return (
    <Section
      title={t("client:events.coorganizers.title")}
      description={t("client:events.coorganizers.description", { max: String(eventLimits.coorganizers) })}
    >
      <Stack gap="4">
        {event.coorganizers.length > 0 && (
          <Stack as="ul" gap="3" aria-label={t("client:events.coorganizers.title")}>
            {event.coorganizers.map((account) => (
              <li key={account.id}>
                <AccountRow
                  account={account}
                  action={
                    <Button
                      size="sm"
                      variant="subtle"
                      isPending={coorganizers.pendingId === account.id}
                      onPress={() => {
                        coorganizers.remove(account);
                      }}
                      aria-label={t("client:events.coorganizers.remove_of", { name: accountName(account) })}
                    >
                      {t("client:events.coorganizers.remove")}
                    </Button>
                  }
                />
              </li>
            ))}
          </Stack>
        )}
        {full ? (
          <Hint>{t("client:events.coorganizers.full", { max: String(eventLimits.coorganizers) })}</Hint>
        ) : (
          <>
            <TextField
              type="search"
              label={t("client:events.coorganizers.search")}
              value={coorganizers.search}
              onChange={coorganizers.changeSearch}
              autoComplete="off"
            />
            <SearchResults
              search={coorganizers.found}
              label={t("client:events.coorganizers.results")}
              action={(account) =>
                coorganizers.isOrganizer(account) ? (
                  <Hint>{t("client:events.coorganizers.already")}</Hint>
                ) : (
                  <Button
                    size="sm"
                    isPending={coorganizers.pendingId === account.id}
                    onPress={() => {
                      coorganizers.assign(account);
                    }}
                    aria-label={t("client:events.coorganizers.assign_of", { name: accountName(account) })}
                  >
                    {t("client:events.coorganizers.assign")}
                  </Button>
                )
              }
            />
          </>
        )}
        {coorganizers.error !== undefined && (
          <InlineError alert size="inherit">
            {t("client:events.coorganizers.failed", { reason: coorganizers.error })}
          </InlineError>
        )}
      </Stack>
    </Section>
  );
}
