// SPDX-License-Identifier: AGPL-3.0-only
// The first-run part of the home timeline (web-client/screens/home-first-run.md): the checklist of three first steps
// with how many are done, and the accounts suggested to follow (FR-308). Both stand above the timeline, whose own
// empty state stays as it is. A step done while the page is open celebrates (21-experience-principles.md §21.5).
import {
  AccountRowSkeleton,
  Box,
  Button,
  CheckIcon,
  Cluster,
  Link,
  LocalIcon,
  PlusIcon,
  ProfileIcon,
  ProgressMeter,
  RegionError,
  SearchIcon,
  Section,
  Stack,
  StarIcon,
  StatusBadge,
  Text,
} from "@my/ui";
import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useOpenCompose } from "../../compose";
import { AccountFollowItem } from "../../profile";
import { type AccountSuggestion, suggestionsQueryOptions } from "../api/suggestions";
import { type FirstRunStep, firstRunSteps, useFirstRun } from "../hooks/useFirstRun";

/** The id of the suggestions' heading, which the checklist's follow step links to. */
const suggestionsHeadingId = "suggested-accounts";

export type FirstRunProps = {
  /** Moves the focus once the checklist is closed and gone: to the page's heading. */
  onDismissed: () => void;
};

export function FirstRun({ onDismissed }: FirstRunProps) {
  const run = useFirstRun();
  if (run.status === "hidden") return null;
  return (
    <Stack gap="6">
      {run.showChecklist && (
        <Checklist
          run={run}
          hasSuggestions={run.showSuggestions}
          onDismiss={() => {
            run.dismiss();
            onDismissed();
          }}
        />
      )}
      {run.showSuggestions && <Suggestions />}
    </Stack>
  );
}

const stepText = {
  profile: "client:onboarding.step.profile",
  follow: "client:onboarding.step.follow",
  post: "client:onboarding.step.post",
} as const satisfies Record<FirstRunStep, ClientMessageKey>;

const stepIcon = {
  profile: <ProfileIcon />,
  follow: <SearchIcon />,
  post: <PlusIcon />,
} as const satisfies Record<FirstRunStep, ReactNode>;

type ChecklistProps = {
  run: Extract<ReturnType<typeof useFirstRun>, { status: "shown" }>;
  hasSuggestions: boolean;
  onDismiss: () => void;
};

function Checklist({ run, hasSuggestions, onDismiss }: ChecklistProps) {
  const t = useT();
  const openCompose = useOpenCompose();
  const doneCount = firstRunSteps.filter((step) => run.done[step]).length;
  const action = (step: FirstRunStep): ReactNode => {
    switch (step) {
      case "profile":
        return (
          <Link href="/settings/profile" variant="standalone">
            {t("client:onboarding.action.profile")}
          </Link>
        );
      case "follow":
        return hasSuggestions ? (
          <Link href={`#${suggestionsHeadingId}`} variant="standalone">
            {t("client:onboarding.action.follow")}
          </Link>
        ) : (
          <Link href="/search" variant="standalone">
            {t("client:onboarding.action.search")}
          </Link>
        );
      case "post":
        return openCompose === null ? null : (
          <Button size="sm" onPress={openCompose}>
            {t("client:onboarding.action.post")}
          </Button>
        );
    }
  };
  return (
    <Section title={t("client:onboarding.checklist")} surface="card">
      <Stack gap="4">
        <ProgressMeter
          kind="progress"
          label={t("client:onboarding.progress")}
          value={doneCount}
          maxValue={firstRunSteps.length}
          valueText={t("client:onboarding.progress.value", {
            done: String(doneCount),
            total: String(firstRunSteps.length),
          })}
        />
        <Stack as="ol" gap="3">
          {firstRunSteps.map((step) => {
            const done = run.done[step];
            const celebration = run.celebrations[step];
            return (
              <Box as="li" key={step}>
                <Cluster gap="3" align="center" justify="between">
                  <Cluster gap="2" align="center">
                    <StatusBadge
                      icon={done ? <CheckIcon /> : stepIcon[step]}
                      label={done ? t("client:onboarding.done") : t("client:onboarding.todo")}
                      tone={done ? "success" : "neutral"}
                      celebrating={celebration.isCelebrating}
                    />
                    <Text>{t(stepText[step])}</Text>
                  </Cluster>
                  {!done && action(step)}
                </Cluster>
              </Box>
            );
          })}
        </Stack>
        <Cluster justify="end">
          <Button size="sm" variant="ghost" onPress={onDismiss}>
            {t("client:onboarding.dismiss")}
          </Button>
        </Cluster>
      </Stack>
    </Section>
  );
}

function Suggestions() {
  const t = useT();
  const api = useApi();
  const query = useQuery(suggestionsQueryOptions(api));
  let content: ReactNode;
  if (query.isPending) {
    content = (
      <Stack gap="2">
        <AccountRowSkeleton />
        <AccountRowSkeleton />
        <AccountRowSkeleton />
      </Stack>
    );
  } else if (query.isError) {
    content = (
      <RegionError
        heading={t("client:onboarding.suggestions.failed")}
        reason={errorMessage(query.error)}
        retryLabel={t("client:onboarding.suggestions.retry")}
        onRetry={() => {
          void query.refetch();
        }}
        isRetrying={query.isFetching}
      />
    );
  } else {
    content =
      query.data.length === 0 ? null : (
        <Stack as="ul" divided border="y">
          {query.data.map((suggestion) => (
            <Box as="li" key={suggestion.account.id}>
              <AccountFollowItem account={suggestion.account} badge={<ReasonBadge suggestion={suggestion} />} />
            </Box>
          ))}
        </Stack>
      );
  }
  return (
    <Section title={t("client:onboarding.suggestions")} headingId={suggestionsHeadingId} surface="rows">
      {content}
      <p>
        <Link href="/search" variant="standalone">
          {t("client:onboarding.suggestions.more")}
        </Link>
      </p>
    </Section>
  );
}

/** Why an account is suggested: the server features it, or it posted lately. */
function ReasonBadge({ suggestion }: { suggestion: AccountSuggestion }) {
  const t = useT();
  return suggestion.source === "featured" ? (
    <StatusBadge icon={<StarIcon />} label={t("client:onboarding.reason.featured")} tone="accent" />
  ) : (
    <StatusBadge icon={<LocalIcon />} label={t("client:onboarding.reason.recently_active")} />
  );
}
