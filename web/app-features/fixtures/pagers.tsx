// SPDX-License-Identifier: AGPL-3.0-only
// Every paged list has the design system's one pager (#3279): a button of a screen's own whose words page or grow a
// list is refused; the design system's Pager and ShowMore, a button that pages nothing, and a thread's fold stay open.

import { Button, LinkButton, Pager, ShowMore, TextButton } from "@my/design-system";

declare const t: (id: string, values?: Record<string, unknown>) => string;
declare const strings: { t: typeof t };
const none = () => undefined;

export const own = [
  <Button onPress={none}>{t("settings.invites.more")}</Button>, // error: pager <Button>
  <TextButton onPress={none}>{strings.t("events.list.show_more")}</TextButton>, // error: pager <TextButton>
  <LinkButton href="/x?cursor=1">{t("staff.reports.next")}</LinkButton>, // error: pager <LinkButton>
  <Button onPress={none}>{t("finance.totals.earlier", { granularity: "month" })}</Button>, // error: pager <Button>
];

export const allowed = [
  <Pager next={{ onPress: none }} position={1} />,
  <ShowMore more onPress={none} />,
  <Button onPress={none}>{t("settings.invites.create")}</Button>,
  <TextButton onPress={none}>{t("thread.earlier", { count: 2 })}</TextButton>,
];
