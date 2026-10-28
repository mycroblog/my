// SPDX-License-Identifier: AGPL-3.0-only
// The account search below its heading (FR-714): a form of the conditions, then the accounts that match, newest
// first. The selects are React Aria Selects (the WAI-ARIA APG select-only combobox) and the fields React Aria
// TextFields, checked by the browser's constraint validation before the search runs. The route keeps the search
// in the address (web-client/README.md S3). A local account can be searched by its email address, its email domain or
// an IP address it used, one of them at a time; the results never show them (the account's page does, when asked).
import {
  AccountItem,
  Box,
  Button,
  Card,
  DetailSection,
  EmptyState,
  Grid,
  RegionError,
  Select,
  SelectItem,
  Spinner,
  Stack,
  TableCell,
  TableColumnHeader,
  TableRow,
  TableRowHeader,
  Text,
  TextField,
} from "@my/ui";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import {
  type AdminAccount,
  adminAccountContactFields,
  adminAccountOrigins,
  type AdminAccountSearch,
  adminAccountSearchLimit,
  adminAccountStates,
} from "../api/accounts";
import {
  adminAccountContactLabels,
  adminAccountOriginLabels,
  adminAccountRegistrationStateLabels,
  adminAccountRestrictionLabels,
  adminAccountStateLabels,
  anyOption,
  isAccountState,
  isContactField,
  isOrigin,
  useAccountSearchForm,
  useAccountSearchResults,
} from "../hooks/useAdminAccounts";
import { accountName, handleOf } from "../hooks/useReports";
import { DateTime, DomainName, RecordTable, RequirePermission } from "./parts";
import { imageSources } from "../../../lib/images";

export type AccountSearchProps = {
  /** The search, from the URL. */
  search: AdminAccountSearch;
  /** Runs another search: the route puts it into the URL. */
  onSearch: (search: AdminAccountSearch) => void;
};

export function AccountSearch(props: AccountSearchProps) {
  return (
    <RequirePermission permission="manage_accounts">
      <AccountSearchView {...props} />
    </RequirePermission>
  );
}

/** Whether a search has any condition. */
function isFiltered(search: AdminAccountSearch): boolean {
  return Object.values(search).some((value) => value !== undefined);
}

function AccountSearchView({ search, onSearch }: AccountSearchProps) {
  const t = useT();
  return (
    <Stack gap="6">
      <Card>
        <DetailSection title={t("client:admin.accounts.search.title")}>
          {/* Another search in the address (back, forward, a link) starts the form from it (web-client/README.md R10). */}
          <SearchForm key={JSON.stringify(search)} search={search} onSearch={onSearch} />
        </DetailSection>
      </Card>
      <DetailSection title={t("client:admin.accounts.results.title")}>
        <Results search={search} />
      </DetailSection>
    </Stack>
  );
}

function SearchForm({ search, onSearch }: AccountSearchProps) {
  const t = useT();
  const form = useAccountSearchForm(search, onSearch);
  const { draft } = form;
  return (
    <Stack as="form" onSubmit={form.submit} container gap="4">
      <Grid at="md" gap="4">
        <TextField
          label={t("client:admin.accounts.search.username")}
          description={t("client:admin.accounts.search.username.description")}
          value={draft.username}
          onChange={(username) => {
            form.change({ username });
          }}
          maxLength={adminAccountSearchLimit}
          autoComplete="off"
          spellCheck="false"
        />
        <TextField
          label={t("client:admin.accounts.search.domain")}
          description={t("client:admin.accounts.search.domain.description")}
          value={draft.domain}
          onChange={(domain) => {
            form.change({ domain });
          }}
          maxLength={adminAccountSearchLimit}
          autoComplete="off"
          spellCheck="false"
        />
        <Select
          label={t("client:admin.accounts.search.origin")}
          value={draft.origin}
          onChange={(key) => {
            form.change({ origin: isOrigin(key) ? key : anyOption });
          }}
        >
          <SelectItem id={anyOption}>{t("client:admin.accounts.search.origin.any")}</SelectItem>
          {adminAccountOrigins.map((origin) => (
            <SelectItem key={origin} id={origin}>
              {t(adminAccountOriginLabels[origin])}
            </SelectItem>
          ))}
        </Select>
        <Select
          label={t("client:admin.accounts.search.state")}
          value={draft.state}
          onChange={(key) => {
            form.change({ state: isAccountState(key) ? key : anyOption });
          }}
        >
          <SelectItem id={anyOption}>{t("client:admin.accounts.search.state.any")}</SelectItem>
          {adminAccountStates.map((state) => (
            <SelectItem key={state} id={state}>
              {t(adminAccountStateLabels[state])}
            </SelectItem>
          ))}
        </Select>
        <Select
          label={t("client:admin.accounts.search.contact")}
          description={t("client:admin.accounts.search.contact.description")}
          value={draft.contact}
          onChange={(key) => {
            form.change({ contact: isContactField(key) ? key : anyOption });
          }}
        >
          <SelectItem id={anyOption}>{t("client:admin.accounts.search.contact.none")}</SelectItem>
          {adminAccountContactFields.map((field) => (
            <SelectItem key={field} id={field}>
              {t(adminAccountContactLabels[field])}
            </SelectItem>
          ))}
        </Select>
        {draft.contact !== anyOption && (
          <TextField
            label={t(adminAccountContactLabels[draft.contact])}
            value={draft.contactValue}
            onChange={(contactValue) => {
              form.change({ contactValue });
            }}
            isRequired
            maxLength={adminAccountSearchLimit}
            {...(draft.contact === "email" && { type: "email" })}
            autoComplete="off"
            spellCheck="false"
            errorMessage={(validation) =>
              validation.validationDetails.valueMissing
                ? t("client:admin.accounts.search.contact.required")
                : t("client:admin.accounts.search.contact.invalid")
            }
          />
        )}
      </Grid>
      <Box>
        <Button type="submit" variant="primary">
          {t("client:admin.accounts.search.submit")}
        </Button>
      </Box>
    </Stack>
  );
}

function Results({ search }: { search: AdminAccountSearch }) {
  const t = useT();
  const results = useAccountSearchResults(search);
  switch (results.status) {
    case "loading":
      return <Spinner size="sm" label={t("client:admin.accounts.results.loading")} />;
    case "error":
      return (
        <RegionError
          heading={t("client:admin.accounts.results.failed")}
          headingLevel={3}
          reason={results.reason}
          retryLabel={t("client:admin.accounts.results.retry")}
          onRetry={results.retry}
          isRetrying={results.isRetrying}
        />
      );
    case "ready": {
      const { list } = results;
      // A page reads a part of the accounts, so it may find none and still go on.
      const empty = list.hasMore ? (
        <EmptyState
          headingLevel={3}
          heading={t("client:admin.accounts.empty.so_far")}
          description={t("client:admin.accounts.empty.so_far.description")}
          action={
            <Button onPress={list.loadMore} isPending={list.isLoadingMore}>
              {t("client:admin.accounts.empty.so_far.more")}
            </Button>
          }
        />
      ) : isFiltered(search) ? (
        <EmptyState
          headingLevel={3}
          heading={t("client:admin.accounts.empty.filtered")}
          description={t("client:admin.accounts.empty.filtered.description")}
        />
      ) : (
        <EmptyState headingLevel={3} heading={t("client:admin.accounts.empty")} />
      );
      return (
        // Another search is another list: it starts at its first page.
        <RecordTable
          key={JSON.stringify(search)}
          list={list}
          caption={t("client:admin.accounts.results.title")}
          getId={(account) => account.id}
          empty={
            <Stack gap="4">
              {empty}
              {results.moreError !== undefined && (
                <RegionError heading={t("client:admin.accounts.results.more_failed")} reason={results.moreError} />
              )}
            </Stack>
          }
          moreError={
            results.moreError !== undefined && (
              <RegionError heading={t("client:admin.accounts.results.more_failed")} reason={results.moreError} />
            )
          }
          head={
            <>
              <TableColumnHeader>{t("client:admin.accounts.column.account")}</TableColumnHeader>
              <TableColumnHeader>{t("client:admin.accounts.state")}</TableColumnHeader>
              <TableColumnHeader>{t("client:admin.accounts.origin")}</TableColumnHeader>
              <TableColumnHeader>{t("client:admin.accounts.created_at")}</TableColumnHeader>
            </>
          }
        >
          {(account) => <AccountRow account={account} />}
        </RecordTable>
      );
    }
  }
}

/** The place of an account: this server, or the domain of its own. */
export function AccountOrigin({ account }: { account: AdminAccount }) {
  const t = useT();
  return account.local ? t("client:admin.accounts.origin.local") : <DomainName domain={account.account.domain} />;
}

function AccountRow({ account }: { account: AdminAccount }) {
  const t = useT();
  const intl = useIntl();
  return (
    <TableRow data-row-id={account.id}>
      <TableRowHeader wrap="anywhere">
        <Stack gap="1">
          <AccountItem
            name={accountName(account.account)}
            handle={account.account.display_name === "" ? undefined : handleOf(account.account)}
            avatar={imageSources(account.account.avatar)}
            avatarSeed={account.account.id}
            href={`/moderation/accounts/${encodeURIComponent(account.id)}`}
          />
          {account.restrictions.length > 0 && (
            <Text size="sm" tone="muted">
              {intl.formatList(
                account.restrictions.map((restriction) => t(adminAccountRestrictionLabels[restriction])),
                { type: "conjunction" },
              )}
            </Text>
          )}
        </Stack>
      </TableRowHeader>
      <TableCell>
        <Text weight="semibold">{t(adminAccountStateLabels[account.state])}</Text>
        {account.registration_state !== undefined && (
          <Text as="p" size="sm" tone="muted">
            {t(adminAccountRegistrationStateLabels[account.registration_state])}
          </Text>
        )}
      </TableCell>
      <TableCell wrap="anywhere">
        <AccountOrigin account={account} />
      </TableCell>
      <TableCell>
        <DateTime value={account.account.created_at} />
      </TableCell>
    </TableRow>
  );
}
