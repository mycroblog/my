// SPDX-License-Identifier: AGPL-3.0-only

// The screens under the entries of accounts and sanctions in Moderation (ADR-0058 §2), as their routes render them for
// a signed-in reader: an account's three pages and an action of it, an appeal, a decision of the automated reviews and
// the appeals of refused posts, a sign-up block and its two forms, and the form of a new invite. Each is titled by
// what it shows once read, by its entry's name meanwhile, and goes back the way its address says.

import { reportPath } from "@my/app-components";
import {
  AccountActionScreen,
  accountPagePath,
  AccountScreen,
  AppealScreen,
  HookAppealScreen,
  HookAppealsScreen,
  hookAppealsPath,
  HookDecisionScreen,
  ImportSignupBlocksScreen,
  NewInviteScreen,
  NewSignupBlockScreen,
  SignupBlockScreen,
  signupBlockPath,
  signupBlocksPath,
  staffEntryPath,
  staffHref,
  useAccountActionTitle,
  useAppealTitle,
  useHookAppealTitle,
  useHookDecisionTitle,
  useModeratedAccountTitle,
  useSignupBlockTitle,
  useStaffTitle,
  useUnderTitle,
  type AccountPage,
} from "@my/app-features";
import { StaffOpened, useStaffAddress } from "./staff-under";

/** The name of the entry of Moderation at `entry`, the title of a screen under it while it is read. */
const useEntryName = (entry: string): string => useStaffTitle("moderation", entry) ?? "";

/** An account's page of `page`. */
export function AccountRoute({ id, page }: { id: string; page: AccountPage }) {
  const address = useStaffAddress();
  const own = useModeratedAccountTitle(id);
  const fallback = useEntryName("accounts");
  const title = own ?? fallback;
  return (
    <StaffOpened entry="accounts" title={title}>
      <AccountScreen
        key={id}
        id={id}
        page={page}
        report={address.search["report"]}
        search={address.search}
        onSearch={address.onSearch}
        onPage={(next) => {
          const report = address.search["report"];
          address.go(staffHref(accountPagePath(id, next), report === undefined ? {} : { report }));
        }}
        onBack={() => {
          const report = address.search["report"];
          address.go(report === undefined ? staffEntryPath("moderation", "accounts") : reportPath(report));
        }}
      />
    </StaffOpened>
  );
}

/** An action on an account. */
export function AccountActionRoute({ account, id }: { account: string; id: string }) {
  const { go } = useStaffAddress();
  const own = useAccountActionTitle(account, id);
  const fallback = useEntryName("accounts");
  const title = own ?? fallback;
  return (
    <StaffOpened entry="accounts" title={title}>
      <AccountActionScreen
        key={id}
        account={account}
        id={id}
        onBack={() => {
          go(accountPagePath(account, "actions"));
        }}
      />
    </StaffOpened>
  );
}

/** An appeal, whose way back keeps the queue's query it was opened from. */
export function AppealRoute({ id }: { id: string }) {
  const { search, go } = useStaffAddress();
  const own = useAppealTitle(id);
  const fallback = useEntryName("appeals");
  const title = own ?? fallback;
  return (
    <StaffOpened entry="appeals" title={title}>
      <AppealScreen
        key={id}
        id={id}
        onBack={() => {
          go(staffHref(staffEntryPath("moderation", "appeals"), search));
        }}
      />
    </StaffOpened>
  );
}

/** A decision of the automated reviews. */
export function HookDecisionRoute({ id }: { id: string }) {
  const { go } = useStaffAddress();
  const own = useHookDecisionTitle(id);
  const fallback = useEntryName("hook-decisions");
  const title = own ?? fallback;
  return (
    <StaffOpened entry="hook-decisions" title={title}>
      <HookDecisionScreen
        key={id}
        id={id}
        onBack={() => {
          go(staffEntryPath("moderation", "hook-decisions"));
        }}
      />
    </StaffOpened>
  );
}

/** The appeals of refused posts. */
export function HookAppealsRoute() {
  const { search, onSearch, go } = useStaffAddress();
  const title = useUnderTitle("hook-appeals");
  return (
    <StaffOpened entry="hook-decisions" title={title}>
      <HookAppealsScreen
        search={search}
        onSearch={onSearch}
        onBack={() => {
          go(staffEntryPath("moderation", "hook-decisions"));
        }}
      />
    </StaffOpened>
  );
}

/** An appeal of a refused post, whose way back keeps the list's query it was opened from. */
export function HookAppealRoute({ id }: { id: string }) {
  const { search, go } = useStaffAddress();
  const own = useHookAppealTitle(id);
  const fallback = useEntryName("hook-decisions");
  const title = own ?? fallback;
  return (
    <StaffOpened entry="hook-decisions" title={title}>
      <HookAppealScreen
        key={id}
        id={id}
        onBack={() => {
          go(staffHref(hookAppealsPath, search));
        }}
      />
    </StaffOpened>
  );
}

/** A sign-up block. */
export function SignupBlockRoute({ id }: { id: string }) {
  const { go } = useStaffAddress();
  const own = useSignupBlockTitle(id);
  const fallback = useEntryName("signup-blocks");
  const title = own ?? fallback;
  return (
    <StaffOpened entry="signup-blocks" title={title}>
      <SignupBlockScreen
        key={id}
        id={id}
        onBack={() => {
          go(signupBlocksPath);
        }}
      />
    </StaffOpened>
  );
}

/** The form of a new sign-up block (`new`), or of blocks from a list (`import`). */
export function SignupBlockFormRoute({ form }: { form: "new" | "import" }) {
  const { go } = useStaffAddress();
  const title = useUnderTitle(form === "new" ? "new-block" : "import-blocks");
  const back = () => {
    go(signupBlocksPath);
  };
  return (
    <StaffOpened entry="signup-blocks" title={title}>
      {form === "new" ? (
        <NewSignupBlockScreen
          onBack={back}
          onAdded={(id) => {
            go(signupBlockPath(id));
          }}
        />
      ) : (
        <ImportSignupBlocksScreen onBack={back} />
      )}
    </StaffOpened>
  );
}

/** The form of a new invite. */
export function NewInviteRoute() {
  const { go } = useStaffAddress();
  const title = useUnderTitle("new-invite");
  return (
    <StaffOpened entry="invites" title={title}>
      <NewInviteScreen
        onBack={() => {
          go(staffEntryPath("moderation", "invites"));
        }}
      />
    </StaffOpened>
  );
}
