# translation-aws-translate

Amazon Translate for a server on ECS: the translation provider `aws`. It asks for the IAM actions the provider calls, and sets its region. It creates nothing and declares no provider, and follows [the backend contract](../README.md#the-backend-contract). [Translating posts](../../../../../docs/operations/translation.md#amazon-translate) describes the provider; the instance setting `translation_providers` turns it on.

**This module has never been applied.** Its test checks its outputs alone. Unverified: that Amazon Translate accepts the calls of the tasks' role with these two actions.

`mycroblog-aws` calls it for its section `translation`.

| Input | Default | Meaning |
| --- | --- | --- |
| `region` | required | The region of Amazon Translate. Detecting the language of a post needs Amazon Comprehend in it too |

| Output | Value |
| --- | --- |
| `env` | `MY_TRANSLATOR_AWS_REGION` |
| `secret_env` | Empty. The server authenticates with the tasks' role |
| `permissions` | `translate:TranslateText`, and `comprehend:DetectDominantLanguage`, which Amazon Translate calls for a post whose language is unknown. Neither takes a resource |
