// SPDX-License-Identifier: AGPL-3.0-only
// One domain action (FR-702), the page /moderation/domain-actions/$id below its heading: its state with the lift,
// then what it is (the domain in both of its forms, the kind, whether the public list hides part of the domain) and
// the comments the moderator wrote, as plain text in any language. Lifting takes effect at once and asks nothing
// first; the button goes away, so focus moves to the heading of the state's region and the StatusMessage
// (role="status") there says what was lifted. A failure is a toast, and the button stays.
import { Button, DescriptionItem, DescriptionList, Hint, Link, Stack, StatusMessage, Text } from "@my/ui";
import { useId, useRef, useState } from "react";
import { useT } from "../../../lib/i18n";
import type { DomainAction } from "../api/staff";
import { domainActionKindLabels, domainInText, useDomainAction, useLiftDomainAction } from "../hooks/useDomainActions";
import { useHasPermission } from "../hooks/usePermissions";
import { DomainActionEnd, DomainActionStateText } from "./DomainActions";
import { DateTime, DomainName, RequirePermission, Section, useFocusAfterChange } from "./parts";

/**
 * The text of an action's page heading, which names it by its kind and domain; for a viewer without manage_domains,
 * whose page loads no action, the screen's name.
 */
export function DomainActionTitle({ id }: { id: string }) {
  const t = useT();
  if (!useHasPermission("manage_domains")) return t("client:admin.domain_actions.detail.title");
  return <PermittedTitle id={id} />;
}

function PermittedTitle({ id }: { id: string }) {
  const t = useT();
  const action = useDomainAction(id);
  // A long domain or value wraps anywhere rather than widen the page.
  return (
    <Text wrap="anywhere">
      {t("client:admin.domain_actions.detail.heading", {
        kind: t(domainActionKindLabels[action.kind]),
        domain: domainInText(t, action.domain),
      })}
    </Text>
  );
}

export function DomainActionDetail({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_domains">
      <DomainActionView id={id} />
    </RequirePermission>
  );
}

function DomainActionView({ id }: { id: string }) {
  const t = useT();
  const action = useDomainAction(id);
  return (
    <Stack gap="8">
      <StateSection action={action} />
      <Section surface="card" title={t("client:admin.domain_actions.detail.details")}>
        <DescriptionList>
          <DescriptionItem term={t("client:admin.domain_actions.domain")}>
            <Stack as="span" gap="1" align="start">
              <Text wrap="anywhere">
                <DomainName domain={action.domain} />
              </Text>
              <Link
                href={`/moderation/domain-actions?filter=all&domain=${encodeURIComponent(action.domain)}`}
                variant="standalone"
                size="sm"
              >
                {t("client:admin.domain_actions.detail.of_domain")}
              </Link>
            </Stack>
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.domain_actions.kind")}>
            {t(domainActionKindLabels[action.kind])}
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.domain_actions.obfuscate.term")}>
            {action.obfuscate
              ? t("client:admin.domain_actions.obfuscate.yes")
              : t("client:admin.domain_actions.obfuscate.no")}
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.domain_actions.public_comment")}>
            {action.public_comment === undefined ? (
              t("client:admin.domain_actions.detail.no_text")
            ) : (
              // Written by a moderator, in any language.
              <Text dir="auto" whitespace="preWrap" wrap="anywhere">
                {action.public_comment}
              </Text>
            )}
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.domain_actions.private_note")}>
            {action.private_note === undefined ? (
              t("client:admin.domain_actions.detail.no_text")
            ) : (
              <Text dir="auto" whitespace="preWrap" wrap="anywhere">
                {action.private_note}
              </Text>
            )}
          </DescriptionItem>
        </DescriptionList>
      </Section>
    </Stack>
  );
}

function StateSection({ action }: { action: DomainAction }) {
  const t = useT();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const liftHelpId = useId();
  const [status, setStatus] = useState<string | undefined>(undefined);
  // The lift button goes away with the change of state: focus moves to the region's heading.
  const focusHeading = useFocusAfterChange(headingRef, action.state);
  const lift = useLiftDomainAction((lifted) => {
    focusHeading();
    setStatus(
      t("client:admin.domain_actions.lifted", {
        kind: t(domainActionKindLabels[lifted.kind]),
        domain: domainInText(t, lifted.domain),
      }),
    );
  });
  return (
    <Section surface="card" title={t("client:admin.domain_actions.state")} headingRef={headingRef}>
      <Stack gap="4">
        <StatusMessage>{status}</StatusMessage>
        <DescriptionList columns={3}>
          <DescriptionItem term={t("client:admin.domain_actions.state")}>
            <DomainActionStateText action={action} />
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.domain_actions.issued_at")}>
            <DateTime value={action.issued_at} />
          </DescriptionItem>
          {action.lifted_at === undefined ? (
            <DescriptionItem term={t("client:admin.domain_actions.expires_at")}>
              <DomainActionEnd action={action} />
            </DescriptionItem>
          ) : (
            <DescriptionItem term={t("client:admin.domain_actions.lifted_at")}>
              <DateTime value={action.lifted_at} />
            </DescriptionItem>
          )}
        </DescriptionList>
        {action.state === "active" && (
          <Stack align="start" gap="1">
            <Button
              isPending={lift.pendingIds.includes(action.id)}
              aria-describedby={liftHelpId}
              onPress={() => {
                lift.lift(action);
              }}
            >
              {t("client:admin.domain_actions.lift")}
            </Button>
            <Hint id={liftHelpId}>{t("client:admin.domain_actions.lift.description")}</Hint>
          </Stack>
        )}
      </Stack>
    </Section>
  );
}
