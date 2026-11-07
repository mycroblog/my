<!-- Code generated from api/plugin by "scripts/task.sh api:plugin-generate". DO NOT EDIT. -->

# Translator protocol

The services, messages, and enums of the package `my.plugin.translator.v1`, which [`translator.proto`](../../api/plugin/my/plugin/translator/v1/translator.proto) defines.

Translating a post with a service of the operator's choice. The request holds the text of a post, which may be a
private one if the operator allows it, so a plugin must not log it, and the server refuses plain TCP to this kind.

## Services

### `TranslatorService`

#### `Translate`

```proto
rpc Translate(TranslateRequest) returns (TranslateResponse);
```

Request: [`TranslateRequest`](#translaterequest). Response: [`TranslateResponse`](#translateresponse).

Translate translates the fragments of one post when a reader asks. The server does not retry a failure itself, and the
same fragments may come again for another reader or after a failure. It lasts plugin_translator_timeout at most, and
the server waits 20 seconds at most whatever that setting says.

## Messages

### `TranslateRequest`

- `fragments` (repeated `string`, field 1): The text to translate: one or more fragments, none of them empty. It holds no address of the post, author, reader,
  handle, or URL: markers stand in for mentions, hashtags, links, custom emoji, and code.

  A marker is a number between U+27E6 and U+27E7. ⟦1⟧ stands for one item, or opens a pair that ⟦/1⟧ closes, around
  words that keep their decoration. A fragment may be nothing but markers.
- `source_language` (`string`, field 2): The BCP 47 tag of the language of the fragments, or und when the server does not know it: the plugin detects it, or
  answers OUTCOME_UNSUPPORTED_LANGUAGE if it cannot.
- `target_language` (`string`, field 3): The BCP 47 tag of the language to translate into, never und.

### `TranslateResponse`

- `outcome` ([`Outcome`](#outcome), field 1): A name in JSON. Anything but a value below, an empty response included, is a temporary failure.
- `translations` (repeated `string`, field 2): With OUTCOME_OK, one translation per fragment, in the order of the fragments. Each one holds every marker of its
  fragment exactly once, unchanged; the items may move, but a pair stays open before it closes, and pairs nest. A
  translation holds plain text: the server escapes any markup in it. An answer that breaks one of these is a temporary
  failure, and the server stores nothing of it.
- `detected_language` (`string`, field 3): With OUTCOME_OK, the BCP 47 tag of the language of the source: what the plugin detected, or nothing to mean
  source_language. The server ignores anything that is not a language tag: it takes no text of the service.

## Enums

### `Outcome`

The result of a call. Connect errors say only that the call failed in transport, which the server treats like
OUTCOME_TEMPORARY. On every failure the server tries its next provider, if it has one.

- `OUTCOME_UNSPECIFIED` = 0
- `OUTCOME_OK` = 1: The translations are in the response.
- `OUTCOME_TEMPORARY` = 2: The service failed or did not answer. Asking again may work.
- `OUTCOME_PROVIDER_LIMIT` = 3: The service reports its own rate limit or quota.
- `OUTCOME_UNSUPPORTED_LANGUAGE` = 4: The service does not translate the source or the target language, or cannot detect the language of the fragments.
