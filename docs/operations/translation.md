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

DeepL translates through its API with a key of a DeepL API plan. The server sends the text of a post with the markers
that stand in for mentions, links, and decoration as XML elements, which DeepL keeps in place, and nothing else of the
post.

1. Get an authentication key from your DeepL account. A key of the free plan ends in `:fx`.
2. Set it in the [deployment configuration](../reference/configuration.md) and restart the server:

   ```sh
   MY_TRANSLATOR_DEEPL_API_KEY=your-key
   ```

   The server calls `https://api-free.deepl.com` for a key of the free plan and `https://api.deepl.com` for any other.
   To go through a proxy, set `MY_TRANSLATOR_DEEPL_URL`. The key is a secret: the server never logs it, puts it in an
   error, or shows it in a setting or an answer of the client API, and it sends it to that address only, following no
   redirects.
3. Enable the provider with the [instance setting](instance-settings.md) `translation_providers`, alone or in your order:

   ```sh
   my instance set translation_providers deepl
   ```

The server asks DeepL which languages it translates when it first translates, and again each day. A post's language
goes to the closest variant DeepL has: English to `en-US`, Portuguese to `pt-BR`, Chinese of Taiwan to `zh-Hant`. A
language that DeepL does not have goes to the next provider, or the reader is told that it cannot be translated.

**Quota.** Each plan has a monthly character quota, and a paid plan can have a cost limit that you set in your DeepL
account. When DeepL reports the quota or its rate limit, the server tries the next provider of `translation_providers`.
With none left, the reader sees that the post could not be translated right now, and the web client uses the browser's
translation when you allow it. The server's own monthly limits (`translation_user_monthly_characters` and
`translation_server_monthly_characters`) apply before any call, so set them below your DeepL quota. DeepL may count the
characters differently from them.

**Data.** Read [DeepL's terms](https://www.deepl.com/en/pro-license) before you enable it:

- On a paid plan, DeepL keeps the text and the translations only as long as it needs to translate them, and for at most 72
  hours when it investigates an error.
- On the free plan, DeepL may keep the text and the translations for good, and its terms forbid sending personal data
  through it. Posts usually contain personal data, so use the free plan only to try the provider out. Never allow it
  `private` or `direct` posts in `translation_provider_visibilities`, and say in your privacy policy that DeepL keeps what
  it receives.

On a paid plan you can allow `private` and `direct` posts, provider by provider:

```sh
my instance set translation_provider_visibilities 'deepl=public|local|unlisted|private'
```

**Attribution.** DeepL's terms require that its name or logo is shown with each translation. The web client shows
"DeepL" with a link to `https://www.deepl.com` next to every translation from it, so you have nothing to add.

## Google Cloud Translation

TODO(#2236)

## Amazon Translate

TODO(#2237)
