// SPDX-License-Identifier: AGPL-3.0-only
// The account search (FR-305): a search landmark (WAI-ARIA APG, landmark regions) with one labelled text
// field and its submit button. Enter in the field or the button sends it; the found account's profile
// opens, and a failure is the field's error message, linked to it and announced.
import { Button, Cluster, Link, Stack, TextField } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { signInPath } from "../../../lib/paths";
import { maxSearchLength } from "../api/lookup";
import { type AccountSearchOptions, useAccountSearch } from "../hooks/useAccountSearch";

export type AccountSearchProps = AccountSearchOptions & {
  /** The id of the page heading, which names the search landmark. */
  headingId: string;
};

export function AccountSearch({ headingId, ...options }: AccountSearchProps) {
  const t = useT();
  const search = useAccountSearch(options);
  return (
    <Stack gap="3">
      <Stack as="form" gap="3" role="search" aria-labelledby={headingId} onSubmit={search.submit} autoCapitalize="none">
        <TextField
          type="search"
          label={t("client:search.label")}
          description={
            search.signedOut
              ? t("client:search.description_signed_out")
              : search.knownOnly
                ? t("client:search.description_known_only")
                : t("client:search.description")
          }
          value={search.value}
          onChange={search.onChange}
          isRequired
          maxLength={maxSearchLength}
          // A handle or a URL: no corrections and no suggestions from other forms (the form turns off capitals).
          autoComplete="off"
          autoCorrect="off"
          spellCheck="false"
          enterKeyHint="search"
          // Only a failed search sets isInvalid: React Aria ignores the native constraint once isInvalid is given at all.
          {...(search.error === null ? {} : { isInvalid: true })}
          errorMessage={search.error ?? t("client:search.required")}
          shrink
        />
        <Cluster justify="end">
          {/* A pending button keeps its focus and tells screen readers it is busy (React Aria Button, isPending). */}
          <Button type="submit" variant="primary" isPending={search.isPending}>
            {t("client:search.submit")}
          </Button>
        </Cluster>
      </Stack>
      {search.signedOut && (
        <p>
          <Link href={signInPath("/search")} variant="standalone">
            {t("client:search.sign_in")}
          </Link>
        </p>
      )}
    </Stack>
  );
}
