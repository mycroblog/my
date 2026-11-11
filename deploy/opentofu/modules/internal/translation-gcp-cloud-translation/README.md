# translation-gcp-cloud-translation

Google Cloud Translation for a server on Cloud Run: the translation provider `google`. It enables the Cloud Translation API in the project that pays for the translations, asks for `roles/cloudtranslate.user` there for the service account of the service, and sets the provider's settings. It follows [the backend contract](../README.md#the-backend-contract), and works only with `app-gcp-cloud-run`. [Translating posts](../../../../../docs/operations/translation.md#google-cloud-translation) describes the provider; the instance setting `translation_providers` turns it on.

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that Google Cloud accepts it. Unverified: that a translation works right after the first apply, while the API and the role propagate.

`mycroblog-gcp` calls it for its section `translation`.

| Input | Default | Meaning |
| --- | --- | --- |
| `project_id` | required | The project that pays for the translations: the API and the role go there |
| `app_project_id` | required | The app's project. A role in it names no resource; one in another project names that project |
| `location` | `global` | `global`, or a location of Cloud Translation that serves the general model, such as `us-central1` |

| Output | Value |
| --- | --- |
| `env` | `MY_TRANSLATOR_GOOGLE_PROJECT`, `MY_TRANSLATOR_GOOGLE_LOCATION` |
| `secret_env` | Empty. The server authenticates with the service account of the service |
| `permissions` | `roles/cloudtranslate.user` on the project, after the API is enabled |

The API stays enabled when the module goes, since something else in the project may use it.
