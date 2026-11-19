// SPDX-License-Identifier: AGPL-3.0-only

// The screens under the entries of federation in Moderation (ADR-0058 §1, §2), as their routes render them for a
// signed-in reader: a domain action and the form of a new one, Mastodon's CSV, the allowlist, the block-list
// subscriptions, a new one and one of them, and a remote server. Each is titled by what it shows once read, by its
// entry's name meanwhile, and goes back the way its address says.

import {
  DomainActionScreen,
  domainActionPath,
  domainActionsPath,
  DomainAllowsScreen,
  DomainImportScreen,
  DomainListScreen,
  domainListPath,
  domainListsPath,
  DomainListsScreen,
  NewDomainActionScreen,
  NewDomainListScreen,
  RemoteServerScreen,
  remoteServersPath,
  useDomainActionTitle,
  useDomainListTitle,
  useStaffTitle,
} from "@my/app-features";
import { StaffOpened, useStaffAddress } from "./staff-under";

/** The name of the entry of Moderation at `entry`, the title of a screen under it while it is read. */
const useEntryName = (entry: string): string => useStaffTitle("moderation", entry) ?? "";

/** The domain action of `id`. */
export function DomainActionRoute({ id }: { id: string }) {
  const { go } = useStaffAddress();
  const own = useDomainActionTitle(id);
  const name = useEntryName("domain-actions");
  const title = own ?? name;
  return (
    <StaffOpened entry="domain-actions" title={title}>
      <DomainActionScreen
        key={id}
        id={id}
        onBack={() => {
          go(domainActionsPath);
        }}
      />
    </StaffOpened>
  );
}

/** The form of a new domain action, on the domain the address names. */
export function NewDomainActionRoute() {
  const { search, go } = useStaffAddress();
  const name = useEntryName("domain-actions");
  return (
    <StaffOpened entry="domain-actions" title={name}>
      <NewDomainActionScreen
        domain={search["domain"]}
        onBack={() => {
          go(domainActionsPath);
        }}
        onIssued={(id) => {
          go(domainActionPath(id));
        }}
      />
    </StaffOpened>
  );
}

/** Mastodon's CSV. */
export function DomainImportRoute() {
  const { go } = useStaffAddress();
  return (
    <StaffOpened entry="domain-actions" title={useEntryName("domain-actions")}>
      <DomainImportScreen
        onBack={() => {
          go(domainActionsPath);
        }}
      />
    </StaffOpened>
  );
}

/** The allowlist of federation. */
export function DomainAllowsRoute() {
  const { go } = useStaffAddress();
  return (
    <StaffOpened entry="domain-actions" title={useEntryName("domain-actions")}>
      <DomainAllowsScreen
        onBack={() => {
          go(domainActionsPath);
        }}
      />
    </StaffOpened>
  );
}

/** The block-list subscriptions. */
export function DomainListsRoute() {
  const address = useStaffAddress();
  return (
    <StaffOpened entry="domain-actions" title={useEntryName("domain-actions")}>
      <DomainListsScreen
        address={address}
        onBack={() => {
          address.go(domainActionsPath);
        }}
      />
    </StaffOpened>
  );
}

/** The form of a new subscription. */
export function NewDomainListRoute() {
  const { go } = useStaffAddress();
  return (
    <StaffOpened entry="domain-actions" title={useEntryName("domain-actions")}>
      <NewDomainListScreen
        onBack={() => {
          go(domainListsPath);
        }}
        onSubscribed={(id) => {
          go(domainListPath(id));
        }}
      />
    </StaffOpened>
  );
}

/** The subscription of `id`. */
export function DomainListRoute({ id }: { id: string }) {
  const address = useStaffAddress();
  const own = useDomainListTitle(id);
  const name = useEntryName("domain-actions");
  const title = own ?? name;
  return (
    <StaffOpened entry="domain-actions" title={title}>
      <DomainListScreen
        key={id}
        id={id}
        {...address}
        onBack={() => {
          address.go(domainListsPath);
        }}
        onRemoved={() => {
          address.go(domainListsPath);
        }}
      />
    </StaffOpened>
  );
}

/** The remote server of `domain`. */
export function RemoteServerRoute({ domain }: { domain: string }) {
  const address = useStaffAddress();
  return (
    <StaffOpened entry="instances" title={domain}>
      <RemoteServerScreen
        key={domain}
        domain={domain}
        {...address}
        onBack={() => {
          address.go(remoteServersPath);
        }}
      />
    </StaffOpened>
  );
}
