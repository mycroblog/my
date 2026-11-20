// SPDX-License-Identifier: AGPL-3.0-only

// The screens under the entries of the operations in the administration (ADR-0058 §2), as their routes render them: the
// jobs of a state and a type and a job, under Jobs; an error group, under Errors, whose way back keeps the list's
// query; the UX measurements, whose window the address keeps and whose form holds the reader back while it has changes
// not saved; and a rule of the link card crawler and a new one, under Link card domains. Each is titled by what it shows
// once read, by its entry's name meanwhile; without a session each asks the reader to sign in.

import {
  ErrorGroupScreen,
  JobListScreen,
  jobQueuePath,
  JobScreen,
  jobsListPath,
  LinkCardRuleScreen,
  NewLinkCardRuleScreen,
  staffHref,
  useErrorGroupTitle,
  useJobListTitle,
  useJobTitle,
  useLinkCardRuleTitle,
  UXMeasurementsScreen,
} from "@my/app-features";
import { AdminSubHeading, AdminSubPage } from "./staff";
import { useStaffAddress } from "./staff-under";

/** The jobs of the state and the type of the address's query. */
export function JobListRoute({ signedIn }: { signedIn: boolean }) {
  const address = useStaffAddress();
  const title = useJobListTitle();
  return (
    <AdminSubPage
      entry="jobs"
      signedIn={signedIn}
      returnTo={jobsListPath}
      heading={<AdminSubHeading entry="jobs" title={title} />}
    >
      {({ go }) => (
        <JobListScreen
          search={address.search}
          onSearch={address.onSearch}
          onBack={() => {
            go(jobQueuePath);
          }}
        />
      )}
    </AdminSubPage>
  );
}

/** The heading of a job's screen: its type, once read. */
function JobHeading({ id }: { id: string }) {
  return <AdminSubHeading entry="jobs" title={useJobTitle(id)} />;
}

/** The job of `id`. */
export function JobRoute({ id, signedIn }: { id: string; signedIn: boolean }) {
  return (
    <AdminSubPage
      key={id}
      entry="jobs"
      signedIn={signedIn}
      returnTo={`/admin/jobs/${encodeURIComponent(id)}`}
      heading={<JobHeading id={id} />}
    >
      {({ go }) => (
        <JobScreen
          id={id}
          onBack={() => {
            go(jobsListPath);
          }}
        />
      )}
    </AdminSubPage>
  );
}

/** The heading of an error group's screen: its type, once read. */
function ErrorGroupHeading({ id }: { id: string }) {
  return <AdminSubHeading entry="errors" title={useErrorGroupTitle(id)} />;
}

/** The error group of `id`, its way back to the list at the query it came from. */
export function ErrorGroupRoute({ id, signedIn }: { id: string; signedIn: boolean }) {
  const address = useStaffAddress();
  return (
    <AdminSubPage
      key={id}
      entry="errors"
      signedIn={signedIn}
      returnTo={`/admin/errors/${encodeURIComponent(id)}`}
      heading={<ErrorGroupHeading id={id} />}
    >
      {({ go }) => (
        <ErrorGroupScreen
          id={id}
          search={address.search}
          onBack={() => {
            go(staffHref("/admin/errors", address.search));
          }}
        />
      )}
    </AdminSubPage>
  );
}

/** The UX measurements over the window of the address's query, and their form. */
export function UXMeasurementsRoute({ signedIn }: { signedIn: boolean }) {
  const address = useStaffAddress();
  return (
    <AdminSubPage entry="ux-measurements" signedIn={signedIn} returnTo="/admin/ux-measurements">
      {(sub) => <UXMeasurementsScreen {...sub} search={address.search} onSearch={address.onSearch} />}
    </AdminSubPage>
  );
}

/** A new rule of the link card crawler. */
export function NewLinkCardRuleRoute({ signedIn }: { signedIn: boolean }) {
  return (
    <AdminSubPage entry="link-card-domains" signedIn={signedIn} returnTo="/admin/link-card-domains/new">
      {(sub) => <NewLinkCardRuleScreen {...sub} />}
    </AdminSubPage>
  );
}

/** The heading of a rule's screen: its domain, once read. */
function LinkCardRuleHeading({ domain }: { domain: string }) {
  return <AdminSubHeading entry="link-card-domains" title={useLinkCardRuleTitle(domain)} />;
}

/** The rule of the link card crawler of `domain`. */
export function LinkCardRuleRoute({ domain, signedIn }: { domain: string; signedIn: boolean }) {
  return (
    <AdminSubPage
      key={domain}
      entry="link-card-domains"
      signedIn={signedIn}
      returnTo={`/admin/link-card-domains/${encodeURIComponent(domain)}`}
      heading={<LinkCardRuleHeading domain={domain} />}
    >
      {(sub) => <LinkCardRuleScreen domain={domain} {...sub} />}
    </AdminSubPage>
  );
}
