// SPDX-License-Identifier: AGPL-3.0-only
// The "Add to list" dialog that the profile's menu opens for an account the user follows (web-client/screens/profile.md
// §4, ADR-0122): the user's lists, each a checkbox that is checked while the account is in it. Checking or unchecking
// saves at once and keeps focus on the checkbox; the result is announced ("Added to Friends"), and a failure shows next
// to the checkbox, which goes back. A full list cannot be checked and says why. Without lists, the dialog leads to the
// lists page; it has only the close button, and "Manage lists" leads there too.
import { Checkbox, Dialog, Hint, Link, RegionError, Spinner, Stack } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { listsPath } from "../../../lib/paths";
import { type Account, maxLists } from "../api/lists";
import { accountName, isolate, useLimits } from "../hooks/names";
import { type ListChoice, useAddToList } from "../hooks/useAddToList";

export type AddToListDialogProps = {
  account: Account;
  isOpen: boolean;
  onOpenChange: (isOpen: boolean) => void;
};

export function AddToListDialog({ account, isOpen, onOpenChange }: AddToListDialogProps) {
  const t = useT();
  return (
    <Dialog
      title={t("client:lists.add_to_list.title", { name: isolate(accountName(account)) })}
      closeLabel={t("client:lists.add_to_list.close")}
      size="sm"
      isOpen={isOpen}
      onOpenChange={onOpenChange}
    >
      {isOpen && <AddToListChoices account={account} />}
    </Dialog>
  );
}

function AddToListChoices({ account }: { account: Account }) {
  const t = useT();
  const limits = useLimits();
  const state = useAddToList(account);
  const manage = (
    <Link href={listsPath} variant="standalone">
      {t("client:lists.add_to_list.manage")}
    </Link>
  );
  switch (state.status) {
    case "loading":
      return (
        <div aria-busy="true">
          <Spinner label={t("client:lists.add_to_list.loading")} />
        </div>
      );
    case "error":
      return (
        <RegionError
          heading={t("client:lists.add_to_list.load_failed")}
          reason={state.message}
          retryLabel={t("client:lists.retry")}
          onRetry={state.retry}
        />
      );
    case "done":
      if (state.choices.length === 0)
        return (
          <Stack gap="3" align="start">
            <p>{t("client:lists.add_to_list.none")}</p>
            {manage}
          </Stack>
        );
      return (
        <Stack gap="4" align="start">
          <Stack as="ul" gap="3" aria-label={t("client:lists.add_to_list.lists")}>
            {state.choices.map((choice) => (
              <li key={choice.list.id}>
                <ListCheckbox choice={choice} toggle={state.toggle} />
              </li>
            ))}
          </Stack>
          {state.choices.length >= maxLists && <Hint>{t("client:lists.limit", { max: limits.lists })}</Hint>}
          {manage}
        </Stack>
      );
  }
}

type ListCheckboxProps = { choice: ListChoice; toggle: (list: ListChoice["list"], checked: boolean) => void };

/** One list: its name as the checkbox's label, and why it cannot be checked or why the last change failed. */
function ListCheckbox({ choice, toggle }: ListCheckboxProps) {
  const t = useT();
  const limits = useLimits();
  let description: string | undefined;
  if (choice.isPending) description = t("client:lists.add_to_list.saving");
  else if (choice.isFull) description = t("client:lists.members.limit", { max: limits.members });
  return (
    <Checkbox
      isSelected={choice.checked}
      // Read-only while saving, so that the checkbox keeps focus (a disabled one would lose it).
      isReadOnly={choice.isPending}
      isDisabled={choice.isFull}
      isInvalid={choice.error !== undefined}
      {...(description === undefined ? {} : { description })}
      errorMessage={choice.error}
      onChange={(checked) => {
        toggle(choice.list, checked);
      }}
    >
      <span dir="auto">{choice.list.name}</span>
    </Checkbox>
  );
}
