# Paging

Collections of the client API use keyset pagination. A page is an object with its `items` and, when there may be more, a `next_cursor`.

```json
{ "items": [ { "id": "..." } ], "next_cursor": "..." }
```

- Pass the `next_cursor` of a page as the `cursor` query parameter of the next request, with the same other parameters. Omit `cursor` for the first page.
- A cursor is opaque: pass it back unchanged and never parse or build one. It is at most 256 characters.
- `limit` sets the number of items of a page. Where an operation takes the shared `limit` parameter, the default is 20 and the most is 40; the operation in the OpenAPI document states its own bounds.
- A page can hold fewer items than `limit`, even none, while `next_cursor` is present: the server leaves out what the user may not see and the posts that the user's keyword filters with the action `hide` match. Keep following `next_cursor`.
- When `next_cursor` is absent, there is no next page. A timeline has no end in practice, because new posts keep arriving: stop when you have enough.
- IDs are UUIDv7 strings. Use the cursor, not an ID, to move through a collection.

```sh
curl -H "Authorization: Bearer $TOKEN" "https://my.example/api/v1/timelines/home?limit=40"
curl -H "Authorization: Bearer $TOKEN" "https://my.example/api/v1/timelines/home?limit=40&cursor=$NEXT_CURSOR"
```

The [client libraries](libraries.md) follow the cursor for you: `mykit.All` in Go and `all` in TypeScript fetch a page only when your loop needs it.
