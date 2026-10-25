// SPDX-License-Identifier: MIT

/** The header that makes a request safe to send again (draft-ietf-httpapi-idempotency-key-header). */
export const idempotencyKeyHeader = "Idempotency-Key";

/**
 * The operations whose spec takes an Idempotency-Key, as the method and the path of the spec (with its `{param}`s).
 * When the caller sends none, the client adds a new one, so that the request can be retried. A test keeps the list
 * equal to the operations of schema.gen.ts that take the header.
 */
export const idempotencyKeyOperations: readonly { readonly method: string; readonly path: string }[] = [{ method: "POST", path: "/statuses" }];

/** Whether the request to the spec path `schemaPath` is one of idempotencyKeyOperations. */
export function acceptsIdempotencyKey(method: string, schemaPath: string): boolean {
  const m = method.toUpperCase();
  return idempotencyKeyOperations.some((op) => op.method === m && op.path === schemaPath);
}
