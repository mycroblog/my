// SPDX-License-Identifier: MIT

/** The header that makes a request safe to send again (draft-ietf-httpapi-idempotency-key-header). */
export const idempotencyKeyHeader = "Idempotency-Key";

/** The 409 of a request whose key's first request is still being carried out. */
export const problemTypeIdempotencyKeyInUse = "/api/v1/problems/idempotency-key-in-use";

/**
 * The writes whose spec takes no Idempotency-Key, since their answer carries a secret or a payment provider's URL that
 * the server does not keep to send again, or each call must reach the audit log, as the method and the path of the spec (with its `{param}`s); every other write takes one. When
 * the caller sends no key to a write that takes one, the client adds a new one, so that the request can be retried.
 * A test keeps the list equal to the writes of schema.gen.ts that take no such header.
 */
export const writesWithoutIdempotencyKey: readonly { readonly method: string; readonly path: string }[] = [
  { method: "POST", path: "/two_factor/totp/setup" },
  { method: "POST", path: "/two_factor/totp" },
  { method: "POST", path: "/two_factor/recovery_codes" },
  { method: "POST", path: "/passkeys/registration" },
  { method: "POST", path: "/passkeys/confirmation" },
  { method: "POST", path: "/linked_identities" },
  { method: "POST", path: "/linked_identities/confirmation" },
  { method: "POST", path: "/oauth_apps" },
  { method: "POST", path: "/oauth_apps/{id}/rotate_secret" },
  { method: "POST", path: "/oauth_apps/{id}/tokens" },
  { method: "POST", path: "/voice/rooms" },
  { method: "POST", path: "/voice/rooms/{id}/participation" },
  { method: "POST", path: "/voice/rooms/{id}/grant" },
  { method: "POST", path: "/admin/webhooks" },
  { method: "POST", path: "/admin/webhooks/{id}/rotate_secret" },
  { method: "POST", path: "/admin/accounts/{id}/password_reset_link" },
  { method: "POST", path: "/admin/accounts/{id}/reveal" },
  { method: "POST", path: "/admin/instances/{domain}/deliveries/{id}/reveal" },
  { method: "POST", path: "/supporter/checkout" },
  { method: "POST", path: "/supporter/tips" },
  { method: "POST", path: "/supporter/portal" },
];

/**
 * The writes that take an Idempotency-Key but to which the client adds none, as the method and the path of the spec:
 * each is sent often, and a lost one is overtaken by the next (a voice room's signals, heartbeat, and microphone
 * state) or not worth a retry (the reports of the client's errors and measurements), so that a key would only fill the
 * server's store. A key the caller sends is kept, and makes the request retried as any other.
 */
export const writesSentWithoutIdempotencyKey: readonly { readonly method: string; readonly path: string }[] = [
  { method: "POST", path: "/voice/rooms/{id}/signals" },
  { method: "POST", path: "/voice/rooms/{id}/heartbeat" },
  { method: "POST", path: "/voice/rooms/{id}/mic" },
  { method: "POST", path: "/client_errors" },
  { method: "POST", path: "/client_measurements" },
];

/** Whether the client adds an Idempotency-Key to a request to the spec path `schemaPath` that has none. */
export function addsIdempotencyKey(method: string, schemaPath: string): boolean {
  const m = method.toUpperCase();
  return (
    acceptsIdempotencyKey(m, schemaPath) &&
    !writesSentWithoutIdempotencyKey.some((op) => op.method === m && op.path === schemaPath)
  );
}

/**
 * The operations whose spec takes an Idempotency-Key, as the method and the path of the spec (with its `{param}`s):
 * every write but writesWithoutIdempotencyKey. A test keeps the list equal to the operations of schema.gen.ts that
 * take the header.
 *
 * @deprecated Every write but a few takes the key: use writesWithoutIdempotencyKey, which lists those few.
 */
export const idempotencyKeyOperations: readonly { readonly method: string; readonly path: string }[] = [
  { method: "DELETE", path: "/custom_emojis/mine/{id}" },
  { method: "PATCH", path: "/custom_emojis/mine/{id}" },
  { method: "POST", path: "/custom_emoji_requests" },
  { method: "POST", path: "/custom_emoji_requests/{id}/withdraw" },
  { method: "PATCH", path: "/accounts/update_credentials" },
  { method: "POST", path: "/accounts/aliases" },
  { method: "DELETE", path: "/accounts/aliases" },
  { method: "PATCH", path: "/preferences" },
  { method: "PUT", path: "/preferences/auto_delete" },
  { method: "POST", path: "/preferences/auto_delete/preview" },
  { method: "PUT", path: "/preferences/badges/{role_id}" },
  { method: "PATCH", path: "/preferences/notifications" },
  { method: "PATCH", path: "/preferences/notification_policy" },
  { method: "PUT", path: "/preferences/event_reminders" },
  { method: "PUT", path: "/preferences/supporter_listing" },
  { method: "POST", path: "/accounts/deactivation" },
  { method: "POST", path: "/accounts/delete" },
  { method: "POST", path: "/accounts/move" },
  { method: "DELETE", path: "/accounts/move" },
  { method: "POST", path: "/accounts/password" },
  { method: "POST", path: "/accounts/password/remove" },
  { method: "POST", path: "/accounts/email/verification" },
  { method: "POST", path: "/exports" },
  { method: "POST", path: "/imports" },
  { method: "POST", path: "/imports/preview" },
  { method: "POST", path: "/two_factor/totp/disable" },
  { method: "POST", path: "/passkeys" },
  { method: "DELETE", path: "/passkeys/{id}" },
  { method: "DELETE", path: "/linked_identities/{id}" },
  { method: "POST", path: "/sessions/revoke_others" },
  { method: "DELETE", path: "/sessions/{id}" },
  { method: "DELETE", path: "/authorized_apps/{id}" },
  { method: "DELETE", path: "/oauth_apps/{id}" },
  { method: "PATCH", path: "/oauth_apps/{id}" },
  { method: "POST", path: "/accounts/{id}/block" },
  { method: "DELETE", path: "/accounts/{id}/block" },
  { method: "POST", path: "/accounts/{id}/follow" },
  { method: "DELETE", path: "/accounts/{id}/follow" },
  { method: "POST", path: "/accounts/{id}/mute" },
  { method: "DELETE", path: "/accounts/{id}/mute" },
  { method: "PUT", path: "/accounts/{id}/note" },
  { method: "DELETE", path: "/accounts/{id}/note" },
  { method: "DELETE", path: "/accounts/{id}/follower" },
  { method: "POST", path: "/accounts/{id}/subscribe" },
  { method: "DELETE", path: "/accounts/{id}/subscribe" },
  { method: "POST", path: "/follow_requests/{id}/accept" },
  { method: "POST", path: "/follow_requests/{id}/reject" },
  { method: "POST", path: "/interaction_requests/{id}/accept" },
  { method: "POST", path: "/interaction_requests/{id}/reject" },
  { method: "POST", path: "/domain_mutes" },
  { method: "DELETE", path: "/domain_mutes/{domain}" },
  { method: "POST", path: "/lists" },
  { method: "DELETE", path: "/lists/{id}" },
  { method: "PATCH", path: "/lists/{id}" },
  { method: "POST", path: "/lists/{id}/accounts/{account_id}" },
  { method: "DELETE", path: "/lists/{id}/accounts/{account_id}" },
  { method: "POST", path: "/media" },
  { method: "PATCH", path: "/media/{id}" },
  { method: "POST", path: "/moderation/actions/{id}/appeal" },
  { method: "POST", path: "/hook_decisions/{id}/appeal" },
  { method: "PUT", path: "/notifications/marker" },
  { method: "POST", path: "/notifications/{id}/dismiss" },
  { method: "POST", path: "/notifications/requests/{id}/accept" },
  { method: "POST", path: "/notifications/requests/{id}/dismiss" },
  { method: "PUT", path: "/push/subscription" },
  { method: "DELETE", path: "/push/subscription" },
  { method: "POST", path: "/statuses" },
  { method: "POST", path: "/articles" },
  { method: "POST", path: "/articles/preview" },
  { method: "PUT", path: "/articles/{id}" },
  { method: "POST", path: "/drafts" },
  { method: "PUT", path: "/drafts/{id}" },
  { method: "DELETE", path: "/drafts/{id}" },
  { method: "PUT", path: "/drafts/{id}/schedule" },
  { method: "DELETE", path: "/drafts/{id}/schedule" },
  { method: "POST", path: "/drafts/{id}/publish" },
  { method: "PUT", path: "/statuses/{id}" },
  { method: "DELETE", path: "/statuses/{id}" },
  { method: "POST", path: "/statuses/{id}/bookmark" },
  { method: "DELETE", path: "/statuses/{id}/bookmark" },
  { method: "POST", path: "/statuses/{id}/mute" },
  { method: "DELETE", path: "/statuses/{id}/mute" },
  { method: "POST", path: "/statuses/{id}/boost" },
  { method: "DELETE", path: "/statuses/{id}/boost" },
  { method: "POST", path: "/statuses/{id}/favourite" },
  { method: "DELETE", path: "/statuses/{id}/favourite" },
  { method: "POST", path: "/statuses/{id}/pin" },
  { method: "DELETE", path: "/statuses/{id}/pin" },
  { method: "POST", path: "/statuses/{id}/poll/votes" },
  { method: "PUT", path: "/statuses/{id}/reaction" },
  { method: "DELETE", path: "/statuses/{id}/reaction" },
  { method: "POST", path: "/statuses/{id}/translation" },
  { method: "PUT", path: "/statuses/{id}/interaction_policy" },
  { method: "DELETE", path: "/statuses/{id}/quotes/{quote_id}/approval" },
  { method: "POST", path: "/tags/{hashtag}/follow" },
  { method: "DELETE", path: "/tags/{hashtag}/follow" },
  { method: "POST", path: "/filters" },
  { method: "DELETE", path: "/filters/{id}" },
  { method: "PATCH", path: "/filters/{id}" },
  { method: "PUT", path: "/featured_tags" },
  { method: "POST", path: "/conversations/{id}/read" },
  { method: "POST", path: "/conversations/{id}/hide" },
  { method: "POST", path: "/reports" },
  { method: "POST", path: "/announcements/{id}/read" },
  { method: "POST", path: "/supporter/subscription/sync" },
  { method: "POST", path: "/supporter/subscription/change" },
  { method: "POST", path: "/supporter/subscription/cancel" },
  { method: "POST", path: "/supporter/subscription/resume" },
  { method: "POST", path: "/supporter/subscription/cancel_scheduled_change" },
  { method: "DELETE", path: "/voice/rooms/{id}" },
  { method: "PATCH", path: "/voice/rooms/{id}" },
  { method: "DELETE", path: "/voice/rooms/{id}/participation" },
  { method: "POST", path: "/voice/rooms/{id}/heartbeat" },
  { method: "POST", path: "/voice/rooms/{id}/host" },
  { method: "POST", path: "/voice/rooms/{id}/speaker" },
  { method: "DELETE", path: "/voice/rooms/{id}/speaker" },
  { method: "POST", path: "/voice/rooms/{id}/hand" },
  { method: "DELETE", path: "/voice/rooms/{id}/hand" },
  { method: "DELETE", path: "/voice/rooms/{id}/participants/{account_id}" },
  { method: "POST", path: "/voice/rooms/{id}/participants/{account_id}/speaking_permission" },
  { method: "DELETE", path: "/voice/rooms/{id}/participants/{account_id}/speaking_permission" },
  { method: "DELETE", path: "/voice/rooms/{id}/participants/{account_id}/hand" },
  { method: "POST", path: "/voice/rooms/{id}/participants/{account_id}/mute" },
  { method: "POST", path: "/voice/rooms/{id}/participants/{account_id}/cohost" },
  { method: "DELETE", path: "/voice/rooms/{id}/participants/{account_id}/cohost" },
  { method: "POST", path: "/voice/rooms/{id}/invitations" },
  { method: "DELETE", path: "/voice/rooms/{id}/invitations/{account_id}" },
  { method: "POST", path: "/voice/rooms/{id}/signals" },
  { method: "POST", path: "/voice/rooms/{id}/mic" },
  { method: "POST", path: "/voice/rooms/{id}/reactions" },
  { method: "POST", path: "/events" },
  { method: "PATCH", path: "/events/{id}" },
  { method: "POST", path: "/events/{id}/cancel" },
  { method: "PUT", path: "/events/{id}/rsvp" },
  { method: "PUT", path: "/events/{id}/attendees/{account_id}/approval" },
  { method: "DELETE", path: "/events/{id}/attendees/{account_id}/approval" },
  { method: "POST", path: "/events/{id}/invitations" },
  { method: "DELETE", path: "/events/{id}/invitations/{account_id}" },
  { method: "PUT", path: "/events/{id}/coorganizers/{account_id}" },
  { method: "DELETE", path: "/events/{id}/coorganizers/{account_id}" },
  { method: "POST", path: "/events/{id}/candidates" },
  { method: "DELETE", path: "/events/{id}/candidates/{candidate_id}" },
  { method: "PUT", path: "/events/{id}/candidates/{candidate_id}/vote" },
  { method: "POST", path: "/events/{id}/candidates/{candidate_id}/confirm" },
  { method: "POST", path: "/client_errors" },
  { method: "POST", path: "/client_measurements" },
  { method: "POST", path: "/admin/jobs/{id}/retry" },
  { method: "POST", path: "/admin/instances/{domain}/mark_available" },
  { method: "POST", path: "/admin/billing_imports/{source}/runs" },
  { method: "POST", path: "/admin/cost_items" },
  { method: "DELETE", path: "/admin/cost_items/{id}" },
  { method: "PATCH", path: "/admin/cost_items/{id}" },
  { method: "POST", path: "/admin/cost_items/{id}/price_revisions" },
  { method: "PUT", path: "/admin/cost_items/{id}/end" },
  { method: "POST", path: "/admin/domain_actions" },
  { method: "POST", path: "/admin/hook_decisions/{id}/override" },
  { method: "POST", path: "/admin/hook_appeals/{id}/uphold" },
  { method: "POST", path: "/admin/hook_appeals/{id}/overturn" },
  { method: "POST", path: "/admin/error_groups/{id}/resolve" },
  { method: "POST", path: "/admin/error_groups/{id}/mute" },
  { method: "POST", path: "/admin/error_groups/{id}/unmute" },
  { method: "POST", path: "/admin/domain_actions/{id}/lift" },
  { method: "POST", path: "/admin/reports/{id}/assign_to_self" },
  { method: "POST", path: "/admin/reports/{id}/unassign" },
  { method: "POST", path: "/admin/reports/{id}/resolve" },
  { method: "POST", path: "/admin/reports/{id}/notes" },
  { method: "POST", path: "/admin/reports/{id}/reopen" },
  { method: "POST", path: "/admin/reports/{id}/forward" },
  { method: "POST", path: "/admin/reports/{id}/comment/translation" },
  { method: "POST", path: "/admin/reports/{id}/statuses/{status_id}/translation" },
  { method: "POST", path: "/admin/voice_rooms/{id}/end" },
  { method: "POST", path: "/admin/events/{id}/cancel" },
  { method: "POST", path: "/admin/accounts/{id}/actions" },
  { method: "POST", path: "/admin/account_actions/{id}/lift" },
  { method: "POST", path: "/admin/rules" },
  { method: "PUT", path: "/admin/rules/{id}" },
  { method: "POST", path: "/admin/rules/{id}/move" },
  { method: "POST", path: "/admin/rules/{id}/retire" },
  { method: "POST", path: "/admin/announcements" },
  { method: "PUT", path: "/admin/announcements/{id}" },
  { method: "DELETE", path: "/admin/announcements/{id}" },
  { method: "POST", path: "/admin/announcements/{id}/publish" },
  { method: "POST", path: "/admin/announcements/{id}/withdraw" },
  { method: "PUT", path: "/admin/theme/draft" },
  { method: "POST", path: "/admin/theme/publish" },
  { method: "POST", path: "/admin/theme/restore" },
  { method: "POST", path: "/admin/instance/brand/sanitize" },
  { method: "PUT", path: "/admin/instance/brand/{kind}" },
  { method: "DELETE", path: "/admin/instance/brand/{kind}" },
  { method: "POST", path: "/admin/custom_emojis" },
  { method: "DELETE", path: "/admin/custom_emojis/{id}" },
  { method: "PATCH", path: "/admin/custom_emojis/{id}" },
  { method: "POST", path: "/admin/custom_emojis/{id}/enable" },
  { method: "POST", path: "/admin/custom_emojis/{id}/disable" },
  { method: "PUT", path: "/admin/custom_emojis/{id}/image" },
  { method: "POST", path: "/admin/custom_emojis/{id}/copy" },
  { method: "POST", path: "/admin/custom_emoji_requests/{id}/approve" },
  { method: "POST", path: "/admin/custom_emoji_requests/{id}/reject" },
  { method: "PUT", path: "/admin/instance_texts/{key}" },
  { method: "POST", path: "/admin/accounts/{id}/two_factor/disable" },
  { method: "POST", path: "/admin/accounts/{id}/email_suppression/lift" },
  { method: "POST", path: "/admin/roles" },
  { method: "DELETE", path: "/admin/roles/{role}" },
  { method: "PATCH", path: "/admin/roles/{role}" },
  { method: "PUT", path: "/admin/accounts/{id}/roles/{role}" },
  { method: "DELETE", path: "/admin/accounts/{id}/roles/{role}" },
  { method: "POST", path: "/admin/appeals/{id}/overturn" },
  { method: "POST", path: "/admin/appeals/{id}/uphold" },
  { method: "POST", path: "/admin/registrations/{id}/approve" },
  { method: "POST", path: "/admin/registrations/{id}/reject" },
  { method: "POST", path: "/admin/signup_blocks" },
  { method: "POST", path: "/admin/signup_blocks/{id}/lift" },
  { method: "POST", path: "/admin/signup_blocks/import" },
  { method: "POST", path: "/admin/domain_actions/import/preview" },
  { method: "POST", path: "/admin/domain_actions/import" },
  { method: "POST", path: "/admin/domain_list_subscriptions" },
  { method: "DELETE", path: "/admin/domain_list_subscriptions/{id}" },
  { method: "PATCH", path: "/admin/domain_list_subscriptions/{id}" },
  { method: "POST", path: "/admin/domain_list_subscriptions/{id}/fetch" },
  { method: "POST", path: "/admin/domain_list_subscriptions/{id}/drafts/accept" },
  { method: "POST", path: "/admin/domain_list_subscriptions/{id}/drafts/reject" },
  { method: "PATCH", path: "/admin/settings" },
  { method: "POST", path: "/admin/domain_allows" },
  { method: "DELETE", path: "/admin/domain_allows/{domain}" },
  { method: "POST", path: "/invites" },
  { method: "POST", path: "/invites/{id}/revoke" },
  { method: "POST", path: "/admin/invites" },
  { method: "POST", path: "/admin/invites/{id}/revoke" },
  { method: "PUT", path: "/admin/closure/announcement" },
  { method: "DELETE", path: "/admin/closure/announcement" },
  { method: "POST", path: "/admin/closure/close" },
  { method: "POST", path: "/admin/oauth_clients/{id}/disable" },
  { method: "POST", path: "/admin/oauth_clients/{id}/enable" },
  { method: "POST", path: "/admin/oauth_client_host_denials" },
  { method: "DELETE", path: "/admin/oauth_client_host_denials/{host}" },
  { method: "POST", path: "/admin/login_providers" },
  { method: "DELETE", path: "/admin/login_providers/{id}" },
  { method: "PATCH", path: "/admin/login_providers/{id}" },
  { method: "POST", path: "/admin/login_providers/{id}/enable" },
  { method: "POST", path: "/admin/login_providers/{id}/disable" },
  { method: "POST", path: "/admin/login_providers/{id}/rotate_secret" },
  { method: "DELETE", path: "/admin/webhooks/{id}" },
  { method: "PATCH", path: "/admin/webhooks/{id}" },
  { method: "POST", path: "/admin/webhooks/{id}/enable" },
  { method: "POST", path: "/admin/webhooks/{id}/disable" },
  { method: "POST", path: "/admin/webhooks/{id}/test" },
  { method: "POST", path: "/admin/webhooks/{id}/deliveries/{delivery_id}/redeliver" },
  { method: "PUT", path: "/admin/link_card_domains/{domain}" },
  { method: "DELETE", path: "/admin/link_card_domains/{domain}" },
  { method: "POST", path: "/admin/relays" },
  { method: "DELETE", path: "/admin/relays/{id}" },
  { method: "POST", path: "/admin/relays/{id}/follow" },
  { method: "DELETE", path: "/admin/relays/{id}/follow" },
  { method: "POST", path: "/admin/statuses/{id}/refetch" },
  { method: "POST", path: "/admin/accounts/{id}/refetch_statuses" },
  { method: "POST", path: "/admin/instances/{domain}/refetch_statuses" },
  { method: "POST", path: "/admin/payment_reconciliation/runs" },
  { method: "PUT", path: "/admin/supporter_program" },
  { method: "PUT", path: "/admin/supporter_tips" },
  { method: "POST", path: "/admin/supporter_plans" },
  { method: "PATCH", path: "/admin/supporter_plans/{id}" },
  { method: "POST", path: "/admin/supporter_plans/{id}/retire" },
  { method: "POST", path: "/admin/supporter_subscriptions/{id}/end" },
];

const writes: ReadonlySet<string> = new Set(["POST", "PUT", "PATCH", "DELETE"]);

/** Whether method is one of a write: POST, PUT, PATCH, or DELETE. */
export function isWrite(method: string): boolean {
  return writes.has(method.toUpperCase());
}

/** Whether the request to the spec path `schemaPath` is a write that takes an Idempotency-Key. */
export function acceptsIdempotencyKey(method: string, schemaPath: string): boolean {
  const m = method.toUpperCase();
  return isWrite(m) && !writesWithoutIdempotencyKey.some((op) => op.method === m && op.path === schemaPath);
}

/**
 * Whether a request to `path`, the path of its URL under the client API's base path, is a write that takes an
 * Idempotency-Key; a `{param}` of a spec path matches one non-empty segment.
 */
export function acceptsIdempotencyKeyAt(method: string, path: string): boolean {
  const m = method.toUpperCase();
  if (!isWrite(m) || !path.startsWith("/")) return false;
  const segments = path.split("/");
  return !writesWithoutIdempotencyKey.some((op) => {
    const pattern = op.path.split("/");
    return (
      op.method === m &&
      pattern.length === segments.length &&
      pattern.every((p, i) => (p.startsWith("{") && p.endsWith("}") ? segments[i] !== "" : p === segments[i]))
    );
  });
}
