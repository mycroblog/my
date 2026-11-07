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

[Cloud Translation](https://cloud.google.com/translate) has the provider ID `google`. The server calls its
`TranslateText` method with the general translation model, and sends the text as HTML so that the mentions, links, and
other parts that must stay as they are come back in place.

1. Enable the Cloud Translation API (`translate.googleapis.com`) in the project that pays for the translations:
   `gcloud services enable translate.googleapis.com --project PROJECT`.
2. Grant the service account of the server `roles/cloudtranslate.user` on that project:
   `gcloud projects add-iam-policy-binding PROJECT --member serviceAccount:SA_EMAIL --role roles/cloudtranslate.user`.
3. Set `MY_TRANSLATOR_GOOGLE_PROJECT` to the project ID. `MY_TRANSLATOR_GOOGLE_LOCATION` is `global` by default; it
   can name another location of Cloud Translation that serves the general model, such as `us-central1`.
4. Restart the server, and add `google` to the instance setting `translation_providers`.

The server authenticates with Application Default Credentials, like the other Google Cloud services it uses: on
Cloud Run, the service account of the service; elsewhere, the key file that `GOOGLE_APPLICATION_CREDENTIALS` names. No
API key is used. The server makes no call at start, so a missing API or role shows only when a reader asks for a
translation: the server then tries the next provider, and the reader sees that the translation failed when none is left.
A quota or rate limit of the project that Google reports also moves on to the next provider.

Google charges by the character sent, after a monthly free allowance; see
[Cloud Translation pricing](https://cloud.google.com/translate/pricing). The server's own monthly limits count the
characters too, but not exactly as Google bills them, so set a budget alert in Cloud Billing as well.

Google [states](https://cloud.google.com/translate/data-usage) that it uses the text only to provide the translation,
keeps it in memory only briefly, does not use it to train its models, and does not share it. The server calls the
global endpoint of Cloud Translation, where Google does not keep the processing to one region.

Google's [attribution requirements](https://cloud.google.com/translate/attribution) ask that a translation be shown as
machine translated and that the "powered by Google Translate" badge appear next to it, linking to
`https://translate.google.com`. The web client shows the name "Google Translate" with that link next to every
translation by this provider. Google offers the badge images only as a download, so the server gives clients no logo for
this provider; check that what readers see meets Google's requirements for your instance.

## Amazon Translate

Amazon Translate is the provider `aws`. The server calls its `TranslateText` operation with the credentials of the
AWS SDK's default chain, as for the other AWS services: the execution role of the task or instance, the environment
variables `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`, or the shared credentials files. No key goes into the
server's configuration.

1. Give the role or user that the server runs as the IAM permission `translate:TranslateText`. For a post whose
   language is unknown, Amazon Translate detects the language with Amazon Comprehend; AWS's managed policies for
   Amazon Translate grant `comprehend:DetectDominantLanguage` with it, so grant that too.
2. Set `translator_aws_region` (`MY_TRANSLATOR_AWS_REGION`) to the region of Amazon Translate, such as `us-east-1`. Detecting
   the language needs a region where Amazon Comprehend runs too.
3. Restart the server, then enable the provider with `translation_providers = aws`, alone or in your order of preference
   among the others ([instance settings](instance-settings.md)).

Amazon Translate charges by the character sent, with a free tier for the first 12 months
([pricing](https://aws.amazon.com/translate/pricing/)). The server sends only the words of a post: mentions, hashtags,
links, and code stay on the server, and each of them goes as a short placeholder in HTML markup. Keep the monthly limits
of `translation_user_monthly_characters` and `translation_server_monthly_characters` within what you want to pay.

**Amazon may store the text and use it to improve its services, possibly in another region, unless the AWS
Organizations [AI services opt-out policy](https://docs.aws.amazon.com/organizations/latest/userguide/orgs_manage_policies_ai-opt-out.html)
covers the account.** Without the opt-out, Amazon is not only your service provider for that text: say so in your
privacy policy ([privacy policy template](privacy-policy-template.md#the-services-you-choose)), and do not allow
`private` or `direct` for `aws` in `translation_provider_visibilities`. By default the provider receives only `public`,
`local`, and `unlisted` posts.

A server built with the build tag `no_translator_aws` does not have the provider and refuses to start with
`translator_aws_region` set ([build tags](../reference/build-tags.md)).
