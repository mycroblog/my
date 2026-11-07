# Translating posts with a translation service

The server can translate a post when a reader asks, through a translation service that you configure: LibreTranslate,
DeepL, Google Cloud Translation, or Amazon Translate. A provider is built in when the server has its keys, and the
[instance setting](instance-settings.md) `translation_providers` enables it.

## LibreTranslate

TODO(#2234)

## DeepL

TODO(#2235)

## Google Cloud Translation

TODO(#2236)

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
