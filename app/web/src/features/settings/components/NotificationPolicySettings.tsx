// SPDX-License-Identifier: AGPL-3.0-only
import {
  AutosaveControl,
  DescriptionItem,
  DescriptionList,
  InlineError,
  Radio,
  RadioGroup,
  Section,
  Stack,
  Text,
} from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  type NotificationPolicyAction,
  notificationPolicyActions,
  type NotificationPolicyCondition,
} from "../api/notificationPolicy";
import { useSaveLabels } from "../hooks/saveState";
import { useNotificationPolicy } from "../hooks/useNotificationPolicy";

const actionLabels: Readonly<Record<NotificationPolicyAction, ClientMessageKey>> = {
  accept: "client:notification_policy.action.accept",
  filter: "client:notification_policy.action.filter",
  drop: "client:notification_policy.action.drop",
};

const actionDescriptions: Readonly<Record<NotificationPolicyAction, ClientMessageKey>> = {
  accept: "client:notification_policy.action.accept.description",
  filter: "client:notification_policy.action.filter.description",
  drop: "client:notification_policy.action.drop.description",
};

const conditionLabels: Readonly<Record<NotificationPolicyCondition, ClientMessageKey>> = {
  not_following: "client:notification_policy.condition.not_following",
  not_follower: "client:notification_policy.condition.not_follower",
  new_account: "client:notification_policy.condition.new_account",
  unsolicited_private_mention: "client:notification_policy.condition.unsolicited_private_mention",
  limited_account: "client:notification_policy.condition.limited_account",
};

const conditionDescriptions: Readonly<Record<NotificationPolicyCondition, ClientMessageKey>> = {
  not_following: "client:notification_policy.condition.not_following.description",
  not_follower: "client:notification_policy.condition.not_follower.description",
  new_account: "client:notification_policy.condition.new_account.description",
  unsolicited_private_mention: "client:notification_policy.condition.unsolicited_private_mention.description",
  limited_account: "client:notification_policy.condition.limited_account.description",
};

function isAction(value: string): value is NotificationPolicyAction {
  return notificationPolicyActions.some((action) => action === value);
}

/**
 * The notification policy (ADR-0158 §1 to §3, web-client/screens/settings-notifications.md): what each of the three
 * actions does, then one radio group per condition (the APG radio group pattern: Tab moves into and out of a group,
 * the arrow keys choose), each saving its action as soon as it changes, with its save mark at its end.
 */
export function NotificationPolicySettings() {
  const t = useT();
  const labels = useSaveLabels();
  const policy = useNotificationPolicy();
  return (
    <Section title={t("client:notification_policy.title")} description={t("client:notification_policy.description")}>
      <Stack gap="6">
        <DescriptionList>
          {notificationPolicyActions.map((action) => (
            <DescriptionItem key={action} term={t(actionLabels[action])}>
              <Text as="span" size="sm" tone="fg">
                {t(actionDescriptions[action])}
              </Text>
            </DescriptionItem>
          ))}
        </DescriptionList>
        {policy.rows.map((row) => (
          <AutosaveControl key={row.condition} state={row.saveState} labels={labels}>
            <RadioGroup
              label={t(conditionLabels[row.condition])}
              description={t(conditionDescriptions[row.condition])}
              orientation="horizontal"
              value={row.action}
              onChange={(value) => {
                if (isAction(value)) row.setAction(value);
              }}
            >
              {notificationPolicyActions.map((action) => (
                <Radio key={action} value={action}>
                  {t(actionLabels[action])}
                </Radio>
              ))}
            </RadioGroup>
          </AutosaveControl>
        ))}
        {policy.error !== undefined && <InlineError alert>{policy.error}</InlineError>}
      </Stack>
    </Section>
  );
}
