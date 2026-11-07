# Translating posts with a translation service

The server can translate a post when a reader asks, through a translation service that you configure: LibreTranslate,
DeepL, Google Cloud Translation, or Amazon Translate. A provider is built in when the server has its keys, and the
[instance setting](instance-settings.md) `translation_providers` enables it.

## LibreTranslate

TODO(#2234)

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
