<!-- Code generated from api/plugin by "scripts/task.sh api:plugin-generate". DO NOT EDIT. -->

# Search protocol

The services, messages, and enums of the package `my.plugin.search.v1`, which [`search.proto`](../../api/plugin/my/plugin/search/v1/search.proto) defines.

Keeping the full-text index of posts in a search engine of the operator's choice, such as one that the server has no
built-in support for. The plugin holds a copy of the posts the server indexes and answers searches with the IDs of
posts: the text, author, visibility, and language of public and local posts, and the queries of signed-in people. A
plugin must not log them, and the server refuses plain TCP to this kind.

The plugin decides nothing that a person sees. The server reads every post of a result again from its database and
shows only what the searcher may see now: an index that lags behind finds fewer posts, or posts that the server then
drops, and never shows a post that it should not.

The server calls every RPC from background jobs, which retry a failure and may send the same call twice, and Search
from the search of its API too, while a person waits. Each call lasts plugin_search_timeout at most. A write must show
in the next search.

## Services

### `SearchService`

#### `Replace`

```proto
rpc Replace(ReplaceRequest) returns (ReplaceResponse);
```

Request: [`ReplaceRequest`](#replacerequest). Response: [`ReplaceResponse`](#replaceresponse).

Replace adds the document of a post, or replaces the document with the same status_id, whatever it held. The server
sends it when a post is published or edited, when its visibility or its author's choices change, and to rebuild the
index.

#### `Remove`

```proto
rpc Remove(RemoveRequest) returns (RemoveResponse);
```

Request: [`RemoveRequest`](#removerequest). Response: [`RemoveResponse`](#removeresponse).

Remove removes the document of a post. Removing a document that is not there succeeds.

#### `RemoveByAuthor`

```proto
rpc RemoveByAuthor(RemoveByAuthorRequest) returns (RemoveByAuthorResponse);
```

Request: [`RemoveByAuthorRequest`](#removebyauthorrequest). Response: [`RemoveByAuthorResponse`](#removebyauthorresponse).

RemoveByAuthor removes every document of an account, such as one that is suspended, deleted, or that stopped
allowing search. Removing nothing succeeds.

#### `RemoveByVisibility`

```proto
rpc RemoveByVisibility(RemoveByVisibilityRequest) returns (RemoveByVisibilityResponse);
```

Request: [`RemoveByVisibilityRequest`](#removebyvisibilityrequest). Response: [`RemoveByVisibilityResponse`](#removebyvisibilityresponse).

RemoveByVisibility removes every document of a visibility, when the operator stops indexing it. Removing nothing
succeeds.

#### `Clear`

```proto
rpc Clear(ClearRequest) returns (ClearResponse);
```

Request: [`ClearRequest`](#clearrequest). Response: [`ClearResponse`](#clearresponse).

Clear removes every document, before a rebuild of the index and when the operator clears it. Clearing an empty index
succeeds, and the index takes documents again afterwards.

#### `Search`

```proto
rpc Search(SearchRequest) returns (SearchResponse);
```

Request: [`SearchRequest`](#searchrequest). Response: [`SearchResponse`](#searchresponse).

Search returns one page of the IDs of the documents that a query finds, newest first.

## Messages

### `Document`

What the index holds of one post. Nothing reads a document back: the server only searches it.

- `status_id` (`string`, field 1): The ID of the post: a UUIDv7 in 36 lowercase characters. Its order as a string is the order of time, which is the
  order of the results.
- `author_id` (`string`, field 2): The ID of the author's account, a UUIDv7 in 36 lowercase characters, which RemoveByAuthor names.
- `visibility` ([`Visibility`](#visibility), field 3): A name in JSON: VISIBILITY_PUBLIC or VISIBILITY_LOCAL, never another value.
- `language` (`string`, field 4): The BCP 47 tag of the language of the post, such as en or ja, or nothing when the post does not say. A plugin may
  analyze the text by language; one that ignores this field still works.
- `published_at` (`string`, field 5): When the post was published, in RFC 3339 in UTC, such as 2026-01-02T03:04:05Z.
- `text` (`string`, field 6): What a search looks in: the body, the content warning, and the image descriptions of the post, as plain text, with
  no HTML. It may be in any language and may be empty.

### `ReplaceRequest`

- `document` ([`Document`](#document), field 1): The document, whose status_id names the one it replaces.

### `ReplaceResponse`

- `outcome` ([`Outcome`](#outcome), field 1): A name in JSON. Anything but OUTCOME_OK, an empty response included, is a failure.

### `RemoveRequest`

- `status_id` (`string`, field 1): The ID of the post whose document to remove.

### `RemoveResponse`

- `outcome` ([`Outcome`](#outcome), field 1): A name in JSON. Anything but OUTCOME_OK, an empty response included, is a failure.

### `RemoveByAuthorRequest`

- `author_id` (`string`, field 1): The ID of the account whose documents to remove.

### `RemoveByAuthorResponse`

- `outcome` ([`Outcome`](#outcome), field 1): A name in JSON. Anything but OUTCOME_OK, an empty response included, is a failure.

### `RemoveByVisibilityRequest`

- `visibility` ([`Visibility`](#visibility), field 1): A name in JSON: the visibility whose documents to remove, VISIBILITY_PUBLIC or VISIBILITY_LOCAL.

### `RemoveByVisibilityResponse`

- `outcome` ([`Outcome`](#outcome), field 1): A name in JSON. Anything but OUTCOME_OK, an empty response included, is a failure.

### `ClearRequest`

No fields.

### `ClearResponse`

- `outcome` ([`Outcome`](#outcome), field 1): A name in JSON. Anything but OUTCOME_OK, an empty response included, is a failure.

### `SearchRequest`

- `text` (`string`, field 1): What a person typed, plain text of 200 characters at most. The plugin splits it into words and finds the documents
  that hold every word. It never reads the text as the query language of its engine: an operator, a wildcard, a
  quote, or a field name in it is just text. Text with no word, such as only spaces, finds nothing.
- `cursor` (`string`, field 2): The next_cursor of the previous page, as the plugin returned it, or nothing for the first page. A cursor stays valid
  while documents are added and removed: the next page goes on after the last ID of the previous one.
- `limit` (`int32`, field 3): The most IDs of the page, at least 1.

### `SearchResponse`

- `outcome` ([`Outcome`](#outcome), field 1): A name in JSON. Anything but a value below, an empty response included, is a failure.
- `status_ids` (repeated `string`, field 2): With OUTCOME_OK, the status_id of each document found, each at most once, newest first: in descending order as
  strings. They are at most limit, and each is a UUIDv7 in 36 lowercase characters. A page that breaks one of these is
  a failure, and the server shows nothing of it.
- `next_cursor` (`string`, field 3): With OUTCOME_OK, the cursor of the next page, opaque to the server, or nothing when no document follows. It is at
  most 256 bytes of printable ASCII with no space, such as the last ID of the page or base64url. A page that is not
  the last may hold fewer than limit IDs, and the page after a cursor may be empty.

## Enums

### `Visibility`

The visibility of a document. The server indexes only these two: posts of other visibilities never reach the plugin.

- `VISIBILITY_UNSPECIFIED` = 0
- `VISIBILITY_PUBLIC` = 1: A post that anyone, signed in or not, may see.
- `VISIBILITY_LOCAL` = 2: A post that only the accounts of this server may see.

### `Outcome`

The result of a call. Connect errors say only that the call failed in transport, which the server treats like
OUTCOME_FAILED. The server never shows a plugin's message to anyone and drops it from its logs.

- `OUTCOME_UNSPECIFIED` = 0
- `OUTCOME_OK` = 1: The call did what it asks.
- `OUTCOME_FAILED` = 2: The engine failed or did not answer. A write is retried by its job; a search fails, and the person may try again.
- `OUTCOME_INVALID_CURSOR` = 3: Search only: the cursor is not one the plugin issued, which the server tells the client as a mistake of its own
  rather than a failure of the search.
