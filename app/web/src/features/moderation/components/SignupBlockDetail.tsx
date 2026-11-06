// SPDX-License-Identifier: AGPL-3.0-only
// One sign-up block (FR-719), the page /moderation/signup-blocks/$id below its heading: its state with the lift,
// then what it is (what it matches, the kind, the effect, whether a ban added it) and the comment the moderator
// wrote, as plain text in any language. Lifting takes effect at once and asks nothing first; the button goes away,
// so focus moves to the heading of the state's region and the StatusMessage (role="status") there says what was
// lifted. A failure is a toast, and the button stays.
import { Button, DescriptionItem, DescriptionList, Hint, Stack, StatusMessage, Text } from "@my/ui";
import { useId, useRef, useState } from "react";
import { useT } from "../../../lib/i18n";
import type { SignupBlock } from "../api/signupBlocks";
import { useHasPermission } from "../hooks/usePermissions";
import {
  signupBlockKindLabels,
  signupBlockSeverityLabels,
  signupBlockValueInText,
  useLiftSignupBlock,
  useSignupBlock,
} from "../hooks/useSignupBlocks";
import { DateTime, RequirePermission, Section, useFocusAfterChange } from "./parts";
import { SignupBlockEnd, SignupBlockStateText, SignupBlockValue } from "./SignupBlocks";

/**
 * The text of a block's page heading, which names it by its kind and value; for a viewer without manage_accounts,
 * whose page loads no block, the screen's name.
 */
export function SignupBlockTitle({ id }: { id: string }) {
  const t = useT();
  if (!useHasPermission("manage_accounts")) return t("client:admin.signup_blocks.detail.title");
  return <PermittedTitle id={id} />;
}

function PermittedTitle({ id }: { id: string }) {
  const t = useT();
  const block = useSignupBlock(id);
  // A long domain or value wraps anywhere rather than widen the page.
  return (
    <Text wrap="anywhere">
      {t("client:admin.signup_blocks.detail.heading", {
        kind: t(signupBlockKindLabels[block.kind]),
        value: signupBlockValueInText(t, block),
      })}
    </Text>
  );
}

export function SignupBlockDetail({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_accounts">
      <SignupBlockView id={id} />
    </RequirePermission>
  );
}

function SignupBlockView({ id }: { id: string }) {
  const t = useT();
  const block = useSignupBlock(id);
  return (
    <Stack gap="8">
      <StateSection block={block} />
      <Section surface="card" title={t("client:admin.signup_blocks.detail.details")}>
        <DescriptionList>
          <DescriptionItem term={t("client:admin.signup_blocks.column.value")}>
            <Text wrap="anywhere">
              <SignupBlockValue block={block} />
            </Text>
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.signup_blocks.kind")}>
            {t(signupBlockKindLabels[block.kind])}
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.signup_blocks.severity")}>
            {t(signupBlockSeverityLabels[block.severity])}
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.signup_blocks.source")}>
            {block.moderation_action_id === undefined
              ? t("client:admin.signup_blocks.source.manual")
              : t("client:admin.signup_blocks.source.ban")}
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.signup_blocks.comment")}>
            {block.comment === undefined ? (
              t("client:admin.signup_blocks.detail.no_comment")
            ) : (
              // Written by a moderator, in any language.
              <Text dir="auto" whitespace="preWrap" wrap="anywhere">
                {block.comment}
              </Text>
            )}
          </DescriptionItem>
        </DescriptionList>
      </Section>
    </Stack>
  );
}

function StateSection({ block }: { block: SignupBlock }) {
  const t = useT();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const liftHelpId = useId();
  const [status, setStatus] = useState<string | undefined>(undefined);
  // The lift button goes away with the change of state: focus moves to the region's heading.
  const focusHeading = useFocusAfterChange(headingRef, block.state);
  const lift = useLiftSignupBlock((lifted) => {
    focusHeading();
    setStatus(
      t("client:admin.signup_blocks.lifted", {
        kind: t(signupBlockKindLabels[lifted.kind]),
        value: signupBlockValueInText(t, lifted),
      }),
    );
  });
  return (
    <Section surface="card" title={t("client:admin.signup_blocks.state")} headingRef={headingRef}>
      <Stack gap="4">
        <StatusMessage>{status}</StatusMessage>
        <DescriptionList columns={3}>
          <DescriptionItem term={t("client:admin.signup_blocks.state")}>
            <SignupBlockStateText block={block} />
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.signup_blocks.created_at")}>
            <DateTime value={block.created_at} />
          </DescriptionItem>
          {block.lifted_at === undefined ? (
            <DescriptionItem term={t("client:admin.signup_blocks.expires_at")}>
              <SignupBlockEnd block={block} />
            </DescriptionItem>
          ) : (
            <DescriptionItem term={t("client:admin.signup_blocks.lifted_at")}>
              <DateTime value={block.lifted_at} />
            </DescriptionItem>
          )}
        </DescriptionList>
        {block.state === "active" && (
          <Stack align="start" gap="1">
            <Button
              isPending={lift.pendingIds.includes(block.id)}
              aria-describedby={liftHelpId}
              onPress={() => {
                lift.lift(block);
              }}
            >
              {t("client:admin.signup_blocks.lift")}
            </Button>
            <Hint id={liftHelpId}>{t("client:admin.signup_blocks.lift.description")}</Hint>
          </Stack>
        )}
      </Stack>
    </Section>
  );
}
