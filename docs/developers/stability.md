# Stability of the API

What an app can rely on as the API changes. The operations themselves are in the OpenAPI document of the instance.

From the v0 release, `/api/v1` keeps these promises:

- **Additions are not breaking**: new fields, operations, scopes, and enum values can appear at any time. Ignore what you do not know.
- **Breaking changes are announced first.** Removing or renaming something, changing a type, making a field required, or changing the meaning of a scope is allowed in v0,
  but the operation is first marked `deprecated: true` in the OpenAPI document and its responses carry a `Deprecation` header
  ([RFC 9745](https://www.rfc-editor.org/rfc/rfc9745), a date such as `@1688169599`) and a `Sunset` header ([RFC 8594](https://www.rfc-editor.org/rfc/rfc8594)).
  It stays for at least one v0.x minor release. The release notes have an "API changes" section. Log these headers in your client.
- **Security fixes** can change behaviour without notice.
- Version 1.0 makes no breaking change to `/api/v1`; one would go to `/api/v2`.

The OpenAPI document of the running server is at `GET /api/v1/openapi.yaml`, with the scopes of each operation; generate your client from it.
