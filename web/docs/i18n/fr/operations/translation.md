<!-- source: translating-posts-with-a-translation-service bf15671669b4 -->
# Traduire les publications avec un service de traduction

Le serveur peut traduire une publication lorsqu’un lecteur le demande, par l’intermédiaire d’un service de traduction que vous configurez : LibreTranslate,
DeepL, Google Cloud Translation ou Amazon Translate. Un fournisseur est intégré lorsque le serveur dispose de ses clés, et le
[paramètre du serveur](instance-settings.md) `translation_providers` l’active.

<!-- source: libretranslate 0d5e448bb450 -->
## LibreTranslate

LibreTranslate est un service de traduction automatique open source que vous pouvez héberger vous-même, ou utiliser en service hébergé sur
[libretranslate.com](https://libretranslate.com). Indiquez-le au serveur avec deux paramètres de la
[configuration](../reference/configuration.md) :

```sh
MY_TRANSLATOR_LIBRETRANSLATE_URL=http://libretranslate:5000
# Only when the server asks for a key, as libretranslate.com does:
MY_TRANSLATOR_LIBRETRANSLATE_API_KEY=your-key
```

- `translator_libretranslate_url` est l’URL de base du serveur LibreTranslate, avec `http:` ou `https:` et un
  chemin facultatif. Un serveur à une adresse privée fonctionne, comme un conteneur placé à côté de celui-ci ou `http://10.0.0.5:5000`.
  Le serveur ne suit pas les redirections qui en proviennent, de sorte que la clé n’est envoyée nulle part ailleurs.
- `translator_libretranslate_api_key` est un secret, lu uniquement depuis l’environnement
  (`MY_TRANSLATOR_LIBRETRANSLATE_API_KEY` ou `MY_TRANSLATOR_LIBRETRANSLATE_API_KEY_FILE`). Laissez-le non défini lorsque le
  serveur n’exige aucune clé ; un serveur auto-hébergé n’en exige généralement pas. La clé n’est jamais montrée aux clients ni écrite dans
  les journaux.

Ajoutez ensuite `libretranslate` au paramètre du serveur `translation_providers`, dans l’ordre dans lequel vous voulez qu’il soit essayé, par exemple
`translation_providers = libretranslate` seul, ou suivi de l’ID d’un autre fournisseur pour s’y rabattre.

Le serveur demande à LibreTranslate sa liste de langues avant la première traduction, puis toutes les heures, et
n’envoie pas une publication dans une paire de langues qu’il ne liste pas. Lorsqu’une publication n’a pas de langue, LibreTranslate la détecte.
Installez les modèles de langue dont vos lecteurs ont besoin sur un serveur que vous exploitez.

**La qualité de la traduction varie selon la langue.** Les modèles de LibreTranslate sont bons pour certaines paires et approximatifs pour d’autres,
et une traduction depuis une langue disposant de peu de données d’entraînement peut manquer le sens. Essayez les langues que votre
communauté utilise avant de l’activer, et gardez un fournisseur commercial après lui dans `translation_providers` si vous en avez
besoin.

**Où va le texte.** Un serveur que vous exploitez garde le texte des publications sur vos propres machines, et personne d’autre
ne le reçoit. Vous pouvez alors lui autoriser les publications `private` et `direct` avec `translation_provider_visibilities`, par exemple
`libretranslate=public|local|unlisted|private|direct`, et la
[politique de confidentialité](privacy-policy-template.md#the-services-you-choose) ne liste aucun service extérieur pour la traduction. Un
LibreTranslate hébergé, libretranslate.com ou celui de quelqu’un d’autre, reçoit le texte comme n’importe quel autre fournisseur : lisez ses conditions, gardez les visibilités
par défaut et nommez-le dans la politique.

<!-- source: deepl 80449fb800f7 -->
## DeepL

DeepL traduit via son API avec une clé d’une formule d’API DeepL. Le serveur envoie le texte d’une publication avec les marqueurs
qui remplacent les mentions, les liens et la mise en forme sous forme d’éléments XML, que DeepL laisse en place, et rien d’autre de la
publication.

1. Obtenez une clé d’authentification depuis votre compte DeepL. Une clé de la formule gratuite se termine par `:fx`.
2. Définissez-la dans la [configuration du déploiement](../reference/configuration.md) et redémarrez le serveur :

   ```sh
   MY_TRANSLATOR_DEEPL_API_KEY=your-key
   ```

   Le serveur appelle `https://api-free.deepl.com` pour une clé de la formule gratuite et `https://api.deepl.com` pour toute autre.
   Pour passer par un proxy, définissez `MY_TRANSLATOR_DEEPL_URL`. La clé est un secret : le serveur ne l’écrit jamais dans les journaux, ne la met pas dans une
   erreur, ne la montre pas dans un paramètre ni dans une réponse de l’API client, et ne l’envoie qu’à cette adresse, sans suivre
   de redirection.
3. Activez le fournisseur avec le [paramètre du serveur](instance-settings.md) `translation_providers`, seul ou dans votre ordre :

   ```sh
   my instance set translation_providers deepl
   ```

Le serveur demande à DeepL quelles langues il traduit lors de la première traduction, puis chaque jour. La langue d’une publication
est envoyée à la variante la plus proche dont dispose DeepL : l’anglais vers `en-US`, le portugais vers `pt-BR`, le chinois de Taïwan vers `zh-Hant`. Une
langue que DeepL ne possède pas est envoyée au fournisseur suivant, ou le lecteur est informé qu’elle ne peut pas être traduite.

**Quota.** Chaque formule a un quota mensuel de caractères, et une formule payante peut avoir une limite de coût que vous définissez dans votre compte
DeepL. Lorsque DeepL signale le quota ou sa limite de débit, le serveur essaie le fournisseur suivant de `translation_providers`.
S’il n’en reste aucun, le lecteur voit que la publication n’a pas pu être traduite pour le moment, et le client web utilise la traduction
du navigateur si vous l’autorisez. Les limites mensuelles propres au serveur (`translation_user_monthly_characters` et
`translation_server_monthly_characters`) s’appliquent avant tout appel : définissez-les donc en dessous de votre quota DeepL. DeepL peut compter les
caractères différemment.

**Données.** Lisez [les conditions de DeepL](https://www.deepl.com/en/pro-license) avant de l’activer :

- Sur une formule payante, DeepL ne conserve le texte et les traductions que le temps nécessaire pour les traduire, et au plus 72
  heures lorsqu’il examine une erreur.
- Sur la formule gratuite, DeepL peut conserver définitivement le texte et les traductions, et ses conditions interdisent d’envoyer des données personnelles
  par ce biais. Les publications contiennent généralement des données personnelles ; n’utilisez donc la formule gratuite que pour essayer le fournisseur. Ne lui autorisez jamais
  les publications `private` ou `direct` dans `translation_provider_visibilities`, et indiquez dans votre politique de confidentialité que DeepL conserve ce
  qu’il reçoit.

Sur une formule payante, vous pouvez autoriser les publications `private` et `direct`, fournisseur par fournisseur :

```sh
my instance set translation_provider_visibilities 'deepl=public|local|unlisted|private'
```

**Attribution.** Les conditions de DeepL exigent que son nom ou son logo soit affiché avec chaque traduction. Le client web affiche
« DeepL » avec un lien vers `https://www.deepl.com` à côté de chaque traduction qui en provient ; vous n’avez donc rien à ajouter.

<!-- source: google-cloud-translation ab42392ab00c -->
## Google Cloud Translation

[Cloud Translation](https://cloud.google.com/translate) a l’ID de fournisseur `google`. Le serveur appelle sa
méthode `TranslateText` avec le modèle de traduction général, et envoie le texte en HTML afin que les mentions, les liens et
les autres éléments qui doivent rester tels quels reviennent à leur place.

1. Activez l’API Cloud Translation (`translate.googleapis.com`) dans le projet qui paie les traductions :
   `gcloud services enable translate.googleapis.com --project PROJECT`.
2. Accordez au compte de service du serveur `roles/cloudtranslate.user` sur ce projet :
   `gcloud projects add-iam-policy-binding PROJECT --member serviceAccount:SA_EMAIL --role roles/cloudtranslate.user`.
3. Définissez `MY_TRANSLATOR_GOOGLE_PROJECT` sur l’ID du projet. `MY_TRANSLATOR_GOOGLE_LOCATION` vaut `global` par défaut ; il
   peut nommer un autre emplacement de Cloud Translation qui sert le modèle général, comme `us-central1`.
4. Redémarrez le serveur et ajoutez `google` au paramètre d’instance `translation_providers`.

Sur Google Cloud avec le module OpenTofu [`mycroblog-gcp`](../../deploy/opentofu/README.md#sections), la section
`translation` réalise les trois premières étapes, et l’application redémarre le service :

```hcl
  translation = {}   # or { project_id = "other-project", location = "us-central1" }
```

Elle active l’API dans le projet du déploiement, ou dans `project_id`, y accorde au compte de service du service
`roles/cloudtranslate.user`, et définit `translator_google_project` et `translator_google_location`. Ajoutez ensuite `google` à
`translation_providers`.

Le serveur s’authentifie avec les Application Default Credentials, comme les autres services Google Cloud qu’il utilise : sur
Cloud Run, le compte de service du service ; ailleurs, le fichier de clé que nomme `GOOGLE_APPLICATION_CREDENTIALS`. Aucune
clé d’API n’est utilisée. Le serveur n’effectue aucun appel au démarrage ; une API ou un rôle manquant ne se révèle donc que lorsqu’un lecteur demande une
traduction : le serveur essaie alors le fournisseur suivant, et le lecteur voit que la traduction a échoué lorsqu’il n’en reste aucun.
Un quota ou une limite de débit du projet que Google signale passe aussi au fournisseur suivant.

Google facture au caractère envoyé, après un quota gratuit mensuel ; voir
[tarifs de Cloud Translation](https://cloud.google.com/translate/pricing). Les propres limites mensuelles du serveur comptent aussi les
caractères, mais pas exactement comme Google les facture ; définissez donc aussi une alerte de budget dans Cloud Billing.

Google [indique](https://cloud.google.com/translate/data-usage) qu’il n’utilise le texte que pour fournir la traduction,
ne le conserve que brièvement en mémoire, ne l’utilise pas pour entraîner ses modèles et ne le partage pas. Le serveur appelle le point de terminaison
global de Cloud Translation, où Google ne limite pas le traitement à une seule région.

Les [exigences d’attribution](https://cloud.google.com/translate/attribution) de Google demandent qu’une traduction soit présentée comme
traduite automatiquement et que le badge « powered by Google Translate » apparaisse à côté, avec un lien vers
`https://translate.google.com`. Le client web affiche le nom « Google Translate » avec ce lien à côté de chaque
traduction faite par ce fournisseur. Google ne propose les images du badge qu’en téléchargement ; le serveur ne donne donc aux clients aucun logo pour
ce fournisseur ; vérifiez que ce que voient les lecteurs répond aux exigences de Google pour votre instance.

<!-- source: amazon-translate b7aa8e79d2d7 -->
## Amazon Translate

Amazon Translate est le fournisseur `aws`. Le serveur appelle son opération `TranslateText` avec les identifiants de la chaîne par défaut du
SDK AWS, comme pour les autres services AWS : le rôle d’exécution de la tâche ou de l’instance, les variables
d’environnement `AWS_ACCESS_KEY_ID` et `AWS_SECRET_ACCESS_KEY`, ou les fichiers d’identifiants partagés. Aucune clé ne va dans la
configuration du serveur.

1. Donnez au rôle ou à l’utilisateur sous lequel le serveur s’exécute la permission IAM `translate:TranslateText`. Pour une publication dont la
   langue est inconnue, Amazon Translate détecte la langue avec Amazon Comprehend ; les politiques gérées d’AWS pour
   Amazon Translate accordent `comprehend:DetectDominantLanguage` avec elle ; accordez donc aussi celle-ci.
2. Définissez `translator_aws_region` (`MY_TRANSLATOR_AWS_REGION`) sur la région d’Amazon Translate, comme `us-east-1`. La détection de
   la langue exige une région où Amazon Comprehend fonctionne aussi.
3. Redémarrez le serveur, puis activez le fournisseur avec `translation_providers = aws`, seul ou dans votre ordre de préférence
   parmi les autres ([paramètres de l’instance](instance-settings.md)).

Sur AWS avec le module OpenTofu [`mycroblog-aws`](../../deploy/opentofu/README.md#sections-of-mycroblog-aws), la section
`translation` réalise les deux premières étapes : `translation = {}` accorde au rôle des tâches les deux actions et définit
`translator_aws_region` sur la région du déploiement, ou sur `region` lorsque vous en donnez une. Activez ensuite le fournisseur.

Amazon Translate facture au caractère envoyé, avec un niveau gratuit pendant les 12 premiers mois
([tarifs](https://aws.amazon.com/translate/pricing/)). Le serveur n’envoie que les mots d’une publication : les mentions, les hashtags,
les liens et le code restent sur le serveur, et chacun part sous forme d’un court espace réservé dans un balisage HTML. Maintenez les limites mensuelles
de `translation_user_monthly_characters` et `translation_server_monthly_characters` dans ce que vous voulez payer.

**Amazon peut stocker le texte et l’utiliser pour améliorer ses services, éventuellement dans une autre région, sauf si la
[politique de désactivation des services d’IA](https://docs.aws.amazon.com/organizations/latest/userguide/orgs_manage_policies_ai-opt-out.html) d’AWS
Organizations couvre le compte.** Sans cette désactivation, Amazon n’est pas seulement votre prestataire pour ce texte : dites-le dans votre
politique de confidentialité ([modèle de politique de confidentialité](privacy-policy-template.md#the-services-you-choose)), et n’autorisez pas
`private` ou `direct` pour `aws` dans `translation_provider_visibilities`. Par défaut, le fournisseur ne reçoit que les publications `public`,
`local` et `unlisted`.
