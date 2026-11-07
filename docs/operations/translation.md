# Translating posts with a translation service

The server can translate a post when a reader asks, through a translation service that you configure: LibreTranslate,
DeepL, Google Cloud Translation, or Amazon Translate. A provider is built in when the server has its keys, and the
[instance setting](instance-settings.md) `translation_providers` enables it.

## LibreTranslate

LibreTranslate is open-source machine translation that you can run yourself, or use as a hosted service at
[libretranslate.com](https://libretranslate.com). Point the server at it with two settings of the
[configuration](../reference/configuration.md):

```sh
MY_TRANSLATOR_LIBRETRANSLATE_URL=http://libretranslate:5000
# Only when the server asks for a key, as libretranslate.com does:
MY_TRANSLATOR_LIBRETRANSLATE_API_KEY=your-key
```

- `translator_libretranslate_url` is the base URL of the LibreTranslate server, with `http:` or `https:` and an
  optional path. A server on a private address works, such as a container next to this one or `http://10.0.0.5:5000`.
  The server does not follow redirects from it, so the key goes nowhere else.
- `translator_libretranslate_api_key` is a secret, read only from the environment
  (`MY_TRANSLATOR_LIBRETRANSLATE_API_KEY` or `MY_TRANSLATOR_LIBRETRANSLATE_API_KEY_FILE`). Leave it unset when the
  server needs no key; a self-hosted one usually needs none. The key is never shown to clients or written to
  the logs.

Then add `libretranslate` to the instance setting `translation_providers`, in the order you want it tried, such as
`translation_providers = libretranslate` alone, or followed by another provider's ID to fall back to it.

The server asks LibreTranslate for its list of languages before the first translation and again every hour, and does
not send a post in a language pair that it does not list. When a post has no language, LibreTranslate detects it.
Install the language models that your readers need on a server you run.

**Translation quality varies by language.** LibreTranslate's models are good for some pairs and rough for others,
and a translation from a language with little training data can miss the meaning. Try the languages that your
community uses before you enable it, and keep a commercial provider after it in `translation_providers` if you need
one.

**Where the text goes.** A server that you run keeps the text of posts on your own machines, and no one else
receives it. Then you may allow it `private` and `direct` posts with `translation_provider_visibilities`, such as
`libretranslate=public|local|unlisted|private|direct`, and the
[privacy policy](privacy-policy-template.md#the-services-you-choose) lists no outside service for translation. A
hosted LibreTranslate, libretranslate.com or someone else's, receives the text like any other provider: read its terms, keep the default
visibilities, and name it in the policy.

## DeepL

TODO(#2235)

## Google Cloud Translation

TODO(#2236)

## Amazon Translate

TODO(#2237)
