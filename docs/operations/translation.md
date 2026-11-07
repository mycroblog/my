# Translating posts with a translation service

The server can translate a post when a reader asks, through a translation service that you configure: LibreTranslate,
DeepL, Google Cloud Translation, or Amazon Translate. A provider is built in when the server has its keys, and the
[instance setting](instance-settings.md) `translation_providers` enables it.

## LibreTranslate

TODO(#2234)

## DeepL

TODO(#2235)

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

TODO(#2237)
