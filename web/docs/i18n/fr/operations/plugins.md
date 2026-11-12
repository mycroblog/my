<!-- source: plugins 4a60f7abbec9 -->
# Plugins

Un plugin est un programme ou un service distinct que vous exécutez, auquel le serveur confie une tâche précise à travers le réseau.
Le serveur se contente de se connecter à une adresse : il ne démarre, n’arrête ni ne surveille le plugin. Exécutez-le comme sidecar, sous systemd,
ou sur une autre machine.

Rien n’est délégué par défaut. Un serveur sans adresse de plugin n’ouvre aucune connexion et ne démarre aucun processus.
Le build tag [`no_plugins`](../reference/build-tags.md) retire le client de plugins du binaire.

![Où se situent les plugins : le processus my confie six ports à un plugin et pose cinq hooks de décision, le tout par un unique client de plugins qui se connecte aux plugins que vous exécutez ; le bas du schéma montre ce que fait chacun quand un appel échoue. La liste ci-dessous décrit chaque partie.](images/plugins.svg)

Du haut du schéma vers le bas :

- **Le processus my**, avec n’importe quel `role`, effectue deux sortes d’appels vers un plugin.
  - **Les ports qu’il confie** : un plugin accomplit une tâche à la place d’un composant intégré, et chacun n’est actif que lorsqu’un paramètre nomme
    `plugin` : `translator` (le paramètre d’instance `translation_providers = plugin`), `billing` (`billing_sources` inclut
    `plugin`), `mailer` (`mail = plugin`), `media_image`, `media_video` et `media_audio` (`media_image = plugin` et ainsi
    de suite), `key_manager` (`key_manager = plugin`) et `search` (`search = plugin`).
  - **[Hooks de décision](decision-hooks.md)** : un plugin répond à la question d’accepter ou non quelque chose, et il est interrogé sauf si le
    mode du hook `hook_<hook>_mode` est `off` : `inbound.status` pour les publications d’autres serveurs, `inbound.follow` pour les abonnements venant
    d’autres serveurs, `signup.check` pour les inscriptions, `media.check` pour les envois, `local.status` pour les publications des comptes locaux,
    `inbound.actor` pour les profils de comptes d’autres serveurs, et `local.profile` pour les modifications de profil des comptes
    locaux.
- **Le client de plugins** effectue tous les appels. Chaque type a une destination, `plugin_<kind>_address`, et un délai,
  `plugin_<kind>_timeout`, et exécute au plus 8 appels à la fois (2 pour chaque type de média). Il se connecte par un socket Unix, `https://` ou,
  pour [certains types](#plain-tcp) seulement, `http://`.
- **Vos plugins** sont des programmes distincts que vous exécutez, comme sidecar, sous systemd ou sur une autre machine. Le serveur ne fait que
  s’y connecter.
- **Quand un appel échoue ou expire** ([détails](#when-a-call-fails)) :
  - `billing`, `mailer` et les types de média : la tâche qui a fait l’appel s’exécute de nouveau plus tard.
  - `translator` : le serveur interroge le prestataire suivant, et quand il n’en reste aucun, le lecteur voit que la traduction a échoué.
  - `key_manager` échoue en mode fermé : tout ce qui a besoin d’une clé échoue, et le serveur ne démarre pas sans le plugin.
  - `search` : la tâche d’une écriture s’exécute de nouveau plus tard, et une recherche échoue, la personne voit donc que la recherche est indisponible.
  - Un hook de décision fait ce que dit `hook_<hook>_on_failure` : `pass`, `retry` ou, pour les inscriptions, `require_approval`.

<!-- source: kinds 73da92ef7563 -->
## Types

Chaque type a sa propre destination, sous les paramètres `plugin_<kind>_*`. Un point dans le nom d’un type devient `_` dans la
clé : `inbound.status` a donc `plugin_inbound_status_address`.

| Type | Ce qu’il transporte | TCP simple (`http://`) | Protocole par défaut | Délai par défaut |
| --- | --- | --- | --- | --- |
| `translator` | Le texte d’une publication qu’un lecteur a demandé à traduire | **Refusé** | `json` | 10 s |
| `search` | Le texte, l’auteur, la visibilité et la langue des publications `public` et `local`, ainsi que les requêtes des personnes connectées | **Refusé** | `json` | 10 s |
| `billing` | Les coûts de l’instance | Autorisé | `json` | 2 min |
| `mailer` | Des adresses et des secrets comme les liens de confirmation | **Refusé** | `json` | 30 s |
| `media_image` | Les images envoyées, avec leurs métadonnées, qui peuvent contenir un lieu | Autorisé | `proto` | 4 min |
| `media_video` | Les vidéos envoyées, avec leurs métadonnées, qui peuvent contenir un lieu | Autorisé | `proto` | 4 min |
| `media_audio` | Les fichiers audio envoyés, avec leurs métadonnées | Autorisé | `proto` | 4 min |
| `key_manager` | Les clés de chiffrement des données en clair | **Refusé** | `json` | 5 s |
| `inbound.status` | Le texte des publications entrantes | **Refusé** | `json` | 3 s |
| `local.status` | Le texte des publications locales | **Refusé** | `json` | 1 s |
| `local.profile` | Le profil public des comptes locaux, sans les images | Autorisé | `json` | 1 s |
| `inbound.follow` | Les deux parties d’un abonnement | Autorisé | `json` | 3 s |
| `inbound.actor` | Les profils publics des comptes d’autres serveurs | Autorisé | `json` | 3 s |
| `signup.check` | Adresses e-mail et adresses IP | **Refusé** | `json` | 2 s |
| `media.check` | Une petite version traitée du média, jamais l’original | Autorisé | `json` | 10 s |

Les paramètres qui font utiliser une destination au serveur appartiennent à chaque fonctionnalité : les cinq derniers types du tableau sont les
[hooks de décision](decision-hooks.md). Cette page couvre ce qu’ils partagent.

Le serveur envoie au plus la limite fixe d’un type par message et en lit au plus une autre. Un appel au-delà de la limite échoue, et
la tâche qui l’a fait réessaie.

<!-- source: the-destination f397f6e80550 -->
## La destination

Les paramètres n’existent que dans le fichier de configuration et l’environnement
([référence de la configuration](../reference/configuration.md)). Aucun paramètre d’instance, aucune page d’administration ni aucune API ne peut les afficher ni les modifier :
un compte administrateur volé ne peut donc pas rediriger les publications ou les clés. Un changement prend effet au redémarrage.

`plugin_<kind>_address` a l’une de trois formes. Le serveur vérifie la forme au démarrage sans résoudre le nom, et
résout le nom à chaque connexion.

- `unix:///run/my/mailer.sock` : un socket Unix.
- `https://mailer.internal:8443` : TLS sur TCP. Le serveur vérifie toujours le certificat, par rapport à
  `plugin_<kind>_tls_ca_file` ou aux racines du système, et aucun paramètre ne désactive cela.
- `http://billing.internal:5000` : TCP simple.

<!-- source: authentication 2fe4df9369ac -->
### Authentification

Choisissez-en une :

- **Un socket Unix :** les permissions du fichier du socket le protègent.
- **mTLS :** `plugin_<kind>_tls_cert_file` et `plugin_<kind>_tls_key_file`, avec une adresse `https://`.
- **Un jeton bearer :** `plugin_<kind>_token` (ou `MY_PLUGIN_<KIND>_TOKEN_FILE`), envoyé comme `Authorization: Bearer`, uniquement par
  `https://` ou un socket Unix. Un jeton avec une adresse `http://` est une erreur de configuration.

Une adresse `https://` sans jeton ni certificat client démarre avec un avertissement, car le plugin ne peut pas distinguer le serveur de toute autre personne qui l’atteint.
Le serveur lit les fichiers de `plugin_<kind>_tls_ca_file`, `plugin_<kind>_tls_cert_file` et
`plugin_<kind>_tls_key_file` au démarrage, où un fichier manquant ou illisible l’arrête. Ensuite, il les surveille de nouveau
pour détecter un changement au plus toutes les 5 secondes, à l’ouverture d’une nouvelle connexion : un certificat de courte durée ou une CA qu’un
gestionnaire de certificats comme Vault, cert-manager ou SPIFFE renouvelle sur place fonctionne donc sans redémarrage. Les connexions déjà
ouvertes gardent le certificat avec lequel elles ont démarré jusqu’à leur fermeture. Pendant qu’un renouvellement est à moitié écrit, par exemple un nouveau
certificat dont la clé n’est pas encore en place, le serveur garde les fichiers lus en dernier et réessaie à la connexion suivante. Le jeton n’est lu qu’au démarrage.

<!-- source: plain-tcp cbb88078173e -->
### TCP simple

`mailer`, `key_manager`, `translator`, `search`, `inbound.status`, `local.status` et `signup.check` refusent `http://` : le serveur ne démarre pas. Cela vaut aussi pour
le bouclage (`http://127.0.0.1`), car une adresse de bouclage n’empêche pas les autres processus de l’hôte de lire le
trafic. Placez un tel sidecar sur un socket Unix dans un volume partagé.

Les autres types acceptent `http://`, et le serveur journalise au démarrage un avertissement qui nomme le type et l’adresse. Rien
ne chiffre ni n’authentifie ce trafic, et aucun jeton bearer ne peut être utilisé dessus. Garder ce réseau fermé est de votre
responsabilité : ce que liste le tableau ci-dessus y circule en clair.

<!-- source: protocols eb5a03718f1b -->
## Protocoles

`plugin_<kind>_protocol` indique ce que le serveur envoie, et cela doit être ce que le plugin accepte.

| Valeur | Ce que le serveur envoie | Pour |
| --- | --- | --- |
| `json` | Le protocole unaire de Connect avec `application/json`, sur HTTP/1.1 pour les connexions simples | Un plugin construit sur une bibliothèque Connect, ou sur un serveur HTTP et une seule bibliothèque JSON |
| `proto` | Le protocole de Connect avec `application/proto` | Un plugin construit sur une bibliothèque Connect |
| `grpc` | gRPC sur HTTP/2 | Un plugin qui ne parle que gRPC |

Un plugin répond `415` au mauvais protocole. Les types de média transmettent en flux dans les deux sens, ce qui exige HTTP/2 : ils prennent donc `proto`
ou `grpc`, et `json` est une erreur de configuration.

Les définitions du protocole sont les fichiers `.proto` sous `api/plugin/` du dépôt. Il y a un paquet pour chaque
type de port, et ils ne partagent aucun type ; les hooks de décision partagent tous un seul paquet, `my.plugin.hook.v1`.
[Le protocole des plugins](../plugins/overview.md) contient la référence de chaque paquet, générée à partir de ces fichiers.

<!-- source: when-a-call-fails 1caa41ab429e -->
## Quand un appel échoue

Un appel attend son tour parmi les appels de son type qui s’exécutent en même temps, et l’attente compte dans son délai et dans la
métrique [`my.plugin.slot_wait`](running.md). Un appel qui échoue compte dans la métrique [`my.plugin.failures`](running.md) par type et code Connect. Le serveur abandonne le
message qu’un plugin met dans une erreur : les raisons des échecs d’un plugin se trouvent donc dans le journal du plugin lui-même. Le serveur
ne réessaie pas un appel lui-même : c’est la tâche qui l’a fait qui le fait, un plugin doit donc supporter deux fois le même appel.

Un plugin qui est arrêté au démarrage du serveur n’arrête rien, sauf `key_manager` : le serveur ne démarre pas sans lui, et
[ce qui s’arrête sans lui](key-management.md#when-the-plugin-is-unreachable) est une partie de sa configuration à lire en premier.

Un appel a le délai de `plugin_<kind>_timeout`, et un plugin le reçoit comme `Connect-Timeout-Ms` (`grpc-timeout` sur
gRPC). Un plugin peut ignorer cet en-tête : le serveur cesse d’attendre de toute façon. Le délai doit rester inférieur à 30 secondes pour les
types qu’une requête peut attendre (`key_manager`, `translator`, `search`, `signup.check`, `media.check`, `local.status`, `inbound.actor` et `local.profile`), et
inférieur à 5 minutes, la durée maximale d’une tâche, pour les autres.

<!-- source: test-a-plugin 5d8195bb27f5 -->
## Tester un plugin

`my plugin test` exécute le contrat d’un type contre un plugin en marche et rapporte chaque cas. Il ne lit aucune configuration :
tout ce qu’il faut, c’est le binaire `my`.

```sh
my plugin test --kind probe unix:///run/my/plugin.sock
my plugin test --kind probe --protocol grpc --tls-ca-file ca.pem --token-file token https://plugin.internal:8443
```

```text
PASS reachable
PASS echo
FAIL echo_empty: the outcome is OUTCOME_UNSPECIFIED, not OUTCOME_OK
2 passed, 1 failed, 0 skipped
```

Le code de sortie est 1 quand un cas échoue. Les options ont le sens des paramètres : `--protocol`, `--token-file`, `--tls-ca-file`,
`--tls-cert-file`, `--tls-key-file` et `--timeout`. Il applique les mêmes règles que le serveur : `--kind mailer` refuse donc une
adresse `http://`. Le plugin testé peut être un vrai plugin : un cas qui fait plus que l’appel lui-même ne s’exécute donc que si
une option le demande. Un type dont la version n’a pas encore le contrat rapporte `reachable` et un `SKIP`.

`--kind billing` vérifie seulement que le plugin refuse de dépenser quoi que ce soit quand aucune requête facturée n’est autorisée. `--billed` effectue aussi
une vraie récupération du mois en cours et du mois précédent, et vérifie la réponse comme le fait un import ; cela peut coûter de l’argent auprès
des prestataires derrière le plugin. [Importer les coûts d’utilisation du cloud](billing-import.md#a-plugin-billing_sources--plugin) contient le
contrat de ce type.

`--kind mailer` n’effectue aucun appel du RPC sauf si `--mail-to` nomme un destinataire. Avec lui, la commande envoie un vrai message à cette adresse,
depuis `--mail-from` (par défaut : la même adresse), et vérifie que le plugin refuse une requête sans destinataire.
[Envoyer des e-mails](email.md#a-plugin-mail--plugin) contient le contrat de ce type.

`--kind translator` envoie quelques phrases anglaises fixes, en les traduisant vers le japonais (`--translate-from` et `--translate-to`
choisissent une autre paire pour un plugin qui n’en a que certaines). Un service derrière le plugin qui facture au caractère facture aussi ceux-ci,
environ 200 caractères. Il vérifie la forme des réponses, et que le plugin refuse la langue `qaa`, un code que l’ISO 639
réserve à un usage privé. Il ne peut pas juger les mots : lisez vous-même une traduction du plugin.
[Traduire des publications](#translating-posts) contient le contrat de ce type.

`--kind search` effectue des recherches, et n’écrit et ne retire que ses propres documents : quelques-uns avec des ID qu’aucune publication n’a, d’un
auteur qui n’est aucun compte, contenant un mot fabriqué pour l’exécution, qu’il retire à la fin. Il vérifie qu’une requête sans
mot ne trouve rien, qu’une requête dans le langage de requête d’un moteur ou en japonais n’échoue pas, que le plugin refuse un
curseur qu’il n’a pas émis, et que les documents sont trouvés du plus récent au plus ancien, page par page, remplacés, et retirés par publication et par
auteur. `--clear-index` exécute aussi les cas qui retirent tous les documents d’une visibilité et l’index entier : ne l’utilisez que
contre un index de test. [Rechercher des publications](#searching-posts) contient le contrat de ce type.

`--kind media_image`, `--kind media_video` et `--kind media_audio` n’envoient que de petits fichiers de test qu’ils fabriquent eux-mêmes :
une photo, un émoji et un GIF animé, un MP4 avec une piste vidéo et une piste son, ou un MP3. Chacun cache un lieu ou un
titre dans ses métadonnées, et un cas échoue quand cela revient dans une sortie. Ils vérifient chaque sortie comme le fait le serveur.
[Convertir des médias](#converting-media) contient le contrat de ces types.

`--kind key_manager` exécute les vérifications que le serveur exécute à chaque démarrage, et vérifie que le plugin refuse une valeur modifiée et une
KEK qu’il ne détient pas. Il n’envoie que des clés qu’il invente et jette, il est donc sûr contre un vrai magasin de clés, qui peut facturer
les appels. Il vérifie que les réponses ont la bonne forme, pas comment le plugin conserve sa KEK.
[Gérer la clé](key-management.md#a-plugin) contient le contrat de ce type.

`probe` est le plus petit type, et le serveur ne lui délègue rien. Servez-le en premier, pour vérifier le transport, le
protocole et l’authentification avant d’écrire le reste du plugin.

<!-- source: translating-posts c9d31b2b9157 -->
## Traduire des publications

Un plugin du type `translator` permet au serveur de traduire une publication quand un lecteur le demande, via un service de votre choix
autre que les [prestataires intégrés](translation.md) (LibreTranslate, DeepL, Google Cloud Translation et Amazon
Translate). Sans prestataire côté serveur, la seule traduction est celle intégrée au navigateur du lecteur, qui n’existe
que dans Chrome, Edge et Opera sur ordinateur : les lecteurs sur téléphone, dans Firefox ou dans Safari n’ont aucun bouton de traduction tant que vous n’en
activez pas un.

```sh
MY_PLUGIN_TRANSLATOR_ADDRESS=unix:///run/my/translator.sock
MY_PLUGIN_TRANSLATOR_NAME="Our translator"    # what readers see next to a translation
```

1. Définissez `plugin_translator_address` et `plugin_translator_name`, puis redémarrez. Le nom est obligatoire, car les lecteurs jugent une
   traduction fiable selon son origine. L’adresse ne quitte jamais la configuration.
2. [Mettez à jour votre politique de confidentialité](privacy-policy-template.md#the-services-you-choose), et nommez le service derrière le plugin
   comme service de traduction : ce que le plugin transmet est ce qu’il reçoit.
3. Activez-le avec le [paramètre d’instance](instance-settings.md) `translation_providers = plugin`. Un changement prend effet immédiatement.

Avoir une adresse n’active rien : tant que le paramètre ne nomme pas `plugin`, le serveur n’envoie rien et les lecteurs ne voient aucun bouton.

Le plugin est un prestataire comme un autre. [`translation_provider_visibilities`](instance-settings.md) décide de ce qu’il peut
recevoir, les limites mensuelles comptent ce qui lui est envoyé, et une traduction qu’il a faite reste dans le cache du serveur comme les autres.
Avec `translation_reports_to_server` activé, il reçoit aussi ce que les modérateurs demandent à traduire dans un signalement, selon les mêmes visibilités.
Il peut venir avant ou après un autre prestataire dans l’ordre de `translation_providers`.

<!-- source: what-the-plugin-receives 975c7e56a522 -->
### Ce que reçoit le plugin

Le plugin reçoit le texte d’une publication quand un lecteur demande une traduction : son corps, son avertissement de contenu, les options de sondage et
les descriptions d’images, ainsi que la langue du texte et la langue cible. Il ne reçoit ni l’adresse de la publication,
ni son auteur, ni le lecteur, ni les mentions, hashtags, liens, émojis personnalisés ou code : des marqueurs numérotés les remplacent.

Par défaut, ce sont les publications `public`, `local` et `unlisted`. **Les publications `private` et `direct` n’atteignent le plugin que si vous les autorisez**,
avec `translation_provider_visibilities = plugin=public|local|unlisted|private|direct`. Le plugin, et tout
service derrière lui, lit alors un texte que l’auteur destinait à peu de personnes : ne les autorisez donc que si vous contrôlez où va le texte,
et dites-le dans votre politique de confidentialité. Le serveur ne peut pas rappeler un texte qu’il a envoyé.

Le serveur refuse `http://` pour ce type, y compris sur le bouclage, car le texte est celui de publications. Utilisez un socket Unix ou
`https://`.

<!-- source: an-llm-behind-a-plugin 611b6fc77bb1 -->
### Un LLM derrière un plugin

Le serveur n’a aucun prestataire LLM et n’en propose pas, délibérément. N’importe qui peut écrire une publication : le texte à traduire est donc un moyen de
donner des instructions à un LLM (une injection de prompt), et les lecteurs prennent une traduction pour ce que dit la publication. L’assainissement protège
le HTML d’une page : il ne peut pas empêcher une traduction de dire quelque chose que la publication ne dit pas.

Le serveur ne peut pas voir ce qu’un plugin appelle. **Si votre plugin utilise un LLM, ce risque et ses conséquences relèvent de votre propre
responsabilité.**

<!-- source: the-contract ef518f876dd2 -->
### Le contrat

Le RPC est `Translate` de `my.plugin.translator.v1.TranslatorService` :

```text
POST /my.plugin.translator.v1.TranslatorService/Translate
Content-Type: application/json

{"fragments":["Good morning ⟦1⟧, this is ⟦2⟧important⟦/2⟧.","⟦3⟧"],"sourceLanguage":"en","targetLanguage":"ja"}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","translations":["⟦1⟧さん、おはようございます。これは⟦2⟧重要⟦/2⟧です。","⟦3⟧"],"detectedLanguage":"en"}
```

- `sourceLanguage` est une balise BCP 47, ou `und` quand le serveur ne connaît pas la langue. Le plugin la détecte alors et la signale
  dans `detectedLanguage`, ou répond `OUTCOME_UNSUPPORTED_LANGUAGE`. `detectedLanguage` peut être absent quand le plugin n’a
  rien à ajouter, et le serveur ignore une valeur qui n’est pas une balise de langue.
- `translations` a une entrée par fragment, dans l’ordre des fragments, aucune n’étant vide. Un fragment peut n’être composé que de
  marqueurs, et sa traduction est alors ces marqueurs.
- `⟦1⟧` représente un élément ou ouvre une paire que `⟦/1⟧` ferme. Chaque marqueur d’un fragment revient exactement une fois dans sa
  traduction, inchangé. Les éléments peuvent se déplacer selon l’ordre des mots, mais une paire reste ouverte avant de se fermer, et les paires s’imbriquent.
- Une traduction est du texte brut. Le serveur échappe tout balisage qu’elle contient, et assainit le résultat avant de le stocker ou de l’afficher.

| `outcome` | Signification | Ce que fait le serveur |
| --- | --- | --- |
| `OUTCOME_OK` | Les traductions sont dans la réponse | Les vérifie, les stocke et répond au lecteur |
| `OUTCOME_TEMPORARY` | Le service a échoué ou n’a pas répondu | Essaie le prestataire suivant |
| `OUTCOME_PROVIDER_LIMIT` | Le service signale sa propre limite de débit ou son quota | Essaie le prestataire suivant |
| `OUTCOME_UNSUPPORTED_LANGUAGE` | Le service n’a pas la langue source ou cible, ou ne peut pas la détecter | Essaie le prestataire suivant, et le dit au lecteur si aucun ne l’a |

Une réponse qui enfreint ces règles compte comme `OUTCOME_TEMPORARY` et rien n’en est stocké, comme pour un `outcome` manquant ou inconnu
et un appel échoué. Quand aucun prestataire ne répond, le lecteur voit que la traduction a échoué et la publication reste telle quelle, sauf si
`translation_browser` est activé et que le navigateur du lecteur peut traduire la publication : le client web la traduit alors à cet endroit.
Les caractères de chaque appel comptent dans les limites mensuelles, y compris pour un appel qui échoue, car le service peut les avoir reçus.

Le serveur cesse d’attendre après `plugin_translator_timeout`, et après 20 secondes quelle que soit la valeur de ce paramètre. Tous les
prestataires d’une même requête disposent de 45 secondes en tout. Il ne réessaie pas un
appel échoué, et les mêmes fragments peuvent revenir pour un autre lecteur.

<!-- source: searching-posts bd9dfba5a3e7 -->
## Rechercher des publications

Un plugin du type `search` tient l’index de la [recherche en texte intégral des publications](search.md) dans un moteur que le serveur
ne prend pas en charge nativement, comme Groonga ou PGroonga, un index construit avec Sudachi, Typesense, Manticore ou un service de
recherche hébergé. Pour le serveur, c’est un moteur de plus : les mêmes publications entrent dans l’index, les mêmes tâches le tiennent à jour,
et le serveur décide toujours de ce que chaque personne peut voir.

```sh
MY_SEARCH=plugin
MY_PLUGIN_SEARCH_ADDRESS=unix:///run/my/search.sock
```

1. Exécutez le plugin, et vérifiez-le avec [`my plugin test --kind search`](#test-a-plugin).
2. Définissez `search = plugin` et `plugin_search_address`, puis suivez [L’activer](search.md#turning-it-on), comme pour tout
   moteur : l’index se remplit à partir de là, et `my search rebuild` ajoute les publications existantes.
3. Si le plugin envoie les publications à un service d’une autre entreprise, nommez ce service dans vos
   [documents de confidentialité](search.md#your-privacy-documents).

Le serveur n’ouvre aucune connexion au démarrage : un plugin arrêté n’arrête donc rien d’autre que la recherche. Pour passer au plugin ou le quitter,
suivez [Désactiver ou changer le moteur](search.md#switching-off-or-changing-the-engine).

<!-- source: what-the-plugin-receives-1 d2b09f214f3a -->
### Ce que reçoit le plugin

- **Les publications de l’index.** Pour chaque publication que l’index contient, le plugin reçoit son texte (le corps, l’avertissement de contenu
  et les descriptions d’images, en texte brut), son ID, l’ID de compte de son auteur, sa visibilité, sa langue
  et sa date de publication. Il s’agit uniquement de publications `public` et `local`, comme le dit [`search_visibilities`](instance-settings.md),
  d’auteurs qui autorisent la recherche. Les publications `local` sont destinées aux seuls comptes de ce serveur : le plugin, et tout
  service derrière lui, les lit aussi. Les publications `unlisted`, `private` et `direct` ne l’atteignent jamais.
- **Les requêtes.** Quand une personne connectée recherche des publications, le plugin reçoit ce qu’elle a saisi, avec le curseur de la
  page et le nombre de résultats. Il ne reçoit pas l’identité de la personne qui a cherché.
- **Les retraits :** d’une publication, de toutes les publications d’un compte, de toutes les publications d’une visibilité, et de l’index entier.

Le plugin ne renvoie que des ID de publications. Le serveur relit chaque publication dans sa base de données et n’affiche que ce que la personne peut
voir à cet instant : un plugin ne peut donc pas montrer une publication à qui ne peut pas la voir, et un index en retard ne fait que trouver moins de choses. Le
serveur n’affiche en outre jamais les erreurs d’un plugin à qui que ce soit.

Le serveur refuse `http://` pour ce type, y compris sur le bouclage, car il transporte le texte des publications et ce que les gens recherchent.
Utilisez un socket Unix ou `https://`.

<!-- source: the-contract-1 d500365750c4 -->
### Le contrat

Le service est `my.plugin.search.v1.SearchService`, avec un RPC par opération :

| RPC | Ce qu’il fait |
| --- | --- |
| `Replace` | Ajoute le document d’une publication, ou remplace le document ayant le même `statusId` |
| `Remove` | Retire le document d’une publication |
| `RemoveByAuthor` | Retire tous les documents d’un compte |
| `RemoveByVisibility` | Retire tous les documents d’une visibilité, `VISIBILITY_PUBLIC` ou `VISIBILITY_LOCAL` |
| `Clear` | Retire tous les documents ; l’index reprend ensuite des documents |
| `Search` | Renvoie une page des ID des documents qu’une requête trouve, du plus récent au plus ancien |

```text
POST /my.plugin.search.v1.SearchService/Replace
Content-Type: application/json

{"document":{"statusId":"019a5f3e-8b2c-7d41-9a3e-2f6c1b8d4e07","authorId":"0199c2d4-1e5f-7a3b-8c9d-0e1f2a3b4c5d","visibility":"VISIBILITY_PUBLIC","language":"ja","publishedAt":"2026-11-07T05:12:30Z","text":"週末に図書館で全文検索の本を読んだ"}}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK"}
```

```text
POST /my.plugin.search.v1.SearchService/Search
Content-Type: application/json

{"text":"全文検索","limit":1}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","statusIds":["019a5f3e-8b2c-7d41-9a3e-2f6c1b8d4e07"],"nextCursor":"019a5f3e-8b2c-7d41-9a3e-2f6c1b8d4e07"}
```

- **Une requête est du texte brut.** Le plugin la découpe en mots et trouve les documents qui contiennent chaque mot. Il ne lit jamais le
  texte comme le langage de requête de son moteur : un opérateur, un joker, un guillemet ou un nom de champ qu’elle contient n’est que du texte. Une requête
  sans mot ne trouve rien. La façon dont le plugin trouve des mots en japonais et dans les autres langues écrites sans espaces relève de son propre
  choix, et c’est la raison d’en avoir un.
- **Une page va du plus récent au plus ancien.** Les `statusIds` sont en ordre décroissant, chacun au plus une fois, `limit` au plus, et chacun
  un ID de statut tel que le serveur l’a envoyé (36 caractères minuscules). Les ID sont des UUIDv7 : leur ordre en chaînes est donc l’ordre
  chronologique.
- **Le curseur appartient au plugin.** `nextCursor` est vide sur la dernière page, et sinon fait au plus 256 octets d’ASCII
  imprimable sans espace, comme le dernier ID de la page. Le serveur le renvoie tel quel pour la page suivante, qui continue
  après cet ID même si des documents sont arrivés et partis. Une page qui n’est pas la dernière peut contenir moins de `limit` ID. Un curseur
  que le plugin n’a pas émis reçoit `OUTCOME_INVALID_CURSOR`.
- **Une écriture se voit à la recherche suivante**, et chaque appel peut arriver deux fois : retirer ce qui n’est pas là réussit. Un moteur
  qui n’affiche les écritures qu’après un rafraîchissement oblige le plugin à l’attendre.

| `outcome` | Signification | Ce que fait le serveur |
| --- | --- | --- |
| `OUTCOME_OK` | L’appel a fait ce qu’il demande | Applique la politique de visibilité aux ID d’une page |
| `OUTCOME_FAILED` | Le moteur a échoué ou n’a pas répondu | Réessaie une écriture avec sa tâche ; fait échouer une recherche, que la personne peut réessayer |
| `OUTCOME_INVALID_CURSOR` | `Search` uniquement : le curseur n’est pas un curseur émis par le plugin | Indique au client que son curseur est erroné |

Un `outcome` manquant ou inconnu, un appel échoué et une page qui enfreint ces règles comptent comme `OUTCOME_FAILED`, et rien
d’une telle page n’est affiché. Un appel dure au plus `plugin_search_timeout` (10 secondes par défaut), et 10 secondes quelle que soit la valeur de ce
paramètre.

<!-- source: converting-media 36df3dbdb21b -->
## Convertir des médias

Un plugin média accomplit la partie lourde du traitement des envois, afin qu’il puisse tourner sur une autre machine. Il existe trois types,
et vous choisissez chacun séparément :

- `media_image` ré-encode les images en leurs variantes.
- `media_video` remuxe les vidéos : des fichiers avec une piste vidéo, avec ou sans son.
- `media_audio` remuxe les fichiers audio seuls.

Le serveur fait toujours le reste lui-même. Il détecte le format de chaque envoi, distingue les vidéos des fichiers audio, vérifie chaque
envoi par rapport aux limites à son arrivée, et fabrique les aperçus des médias distants. Un type dont le paramètre reste `native` est
traité dans le serveur.

```sh
# Only videos go to a plugin, on a machine of its own; images and audio stay in the server.
MY_MEDIA_VIDEO=plugin
MY_PLUGIN_MEDIA_VIDEO_ADDRESS=https://media.internal:9000
```

Définissez `media_<kind> = plugin` et `plugin_media_<kind>_address` pour chaque type que vous confiez, puis redémarrez. Un plugin peut
prendre plusieurs types : donnez-lui la même adresse pour chacun. Chaque type exécute au plus deux appels à la fois, et chaque appel dure au plus
`plugin_media_<kind>_timeout` (4 minutes par défaut), afin qu’une vidéo lente ne retarde pas les images.

- **Retirer les métadonnées est le travail du plugin.** Les envois parviennent au plugin tels qu’ils ont été envoyés, avec leurs données EXIF et leurs
  balises, qui peuvent contenir le lieu où une photo a été prise. Le serveur vérifie le format et la taille de chaque sortie, mais il ne peut pas
  savoir si toute trace de métadonnées a disparu. `my plugin test --kind media_image` vérifie que le lieu de ses fichiers de test
  ne revient pas ; vérifiez le reste dans le plugin.
- **Le TCP simple envoie les fichiers sans chiffrement.** Les types de média acceptent `http://`, avec un avertissement au démarrage. Les envois, avec leurs
  métadonnées, traversent alors le réseau en clair, et rien n’authentifie aucune des extrémités : quiconque se trouve sur ce réseau peut les lire
  ou répondre à la place du plugin. Utilisez un socket Unix ou `https://` sauf si le réseau n’appartient qu’à vous.
- **Le serveur ne fait pas confiance à la sortie.** Il détecte le format de chaque sortie d’après son contenu, jamais d’après ce que le
  plugin déclare, et la sert avec le type de contenu et l’extension de fichier de ce format. Il décode chaque sortie sous
  les mêmes limites qu’un envoi, et vérifie qu’elle respecte sa taille. Une sortie plus grande que déclaré ou que ces
  limites, d’un format que le plugin n’a pas déclaré, ou d’un format que la variante n’accepte pas, fait échouer l’envoi comme un fichier
  que le serveur ne peut pas traiter.
- **Un échec est un retard.** Quand le plugin est arrêté ou échoue, la tâche réessaie plus tard, comme avec le processeur intégré. Une
  action qui a besoin immédiatement de l’image d’un émoji personnalisé, comme copier un émoji distant ou accepter une demande pour en obtenir un, échoue
  tant que le plugin échoue.

<!-- source: the-contract-2 812c987c276c -->
### Le contrat

Le service est `my.plugin.media.v1.MediaService`, avec les RPC `Process` pour une image et `Remux` pour une vidéo ou un fichier audio.
Les deux transmettent en flux dans les deux sens : le plugin a donc besoin d’une bibliothèque Connect ou gRPC et de HTTP/2 : `proto` ou `grpc`, sur TLS avec ALPN
`h2`, ou sur HTTP/2 en clair (h2c) sur un socket Unix ou en TCP simple.

Un appel est semi-duplex :

1. Le serveur envoie un `head` : les limites, et pour une image son usage, les boîtes de ses trois tailles, et son point
   focal. Le fichier suit dans des messages `chunk` de 1 MiB au plus. Le serveur ferme ensuite son côté.
2. Le plugin répond par un `head` dont l'`outcome` est le résultat. Avec `OUTCOME_OK`, l’en-tête de `Process` contient aussi les
   dimensions de l’image après son orientation EXIF et le blurhash de la petite sortie fixe.
3. Pour chaque sortie, le plugin envoie un `part`, qui nomme la sortie et déclare son format, ses dimensions et sa longueur,
   suivi de messages `chunk` de 1 MiB au plus.

`Process` renvoie les sorties fixes petite, moyenne et grande, et les trois sorties animées d’une image de plus d’une
image. Les sorties petite et moyenne sont en WebP ; la grande sortie fixe est un JPEG ou un PNG (toujours un PNG pour un émoji personnalisé),
et la grande sortie animée un GIF. `Remux` renvoie le fichier dans son propre conteneur sans métadonnées, ainsi que l’image de couverture qu’il
intègre, le cas échéant. Le serveur lit au plus deux fois la limite d’image depuis `Process`, et depuis `Remux` la limite vidéo pour le
fichier et la limite d’image pour sa couverture.

| `outcome` | Signification | Ce que fait le serveur |
| --- | --- | --- |
| `OUTCOME_OK` | Les sorties suivent | Les vérifie et les stocke |
| `OUTCOME_TEMPORARY` | Le plugin a échoué | Réessaie la tâche plus tard |
| `OUTCOME_UNSUPPORTED_FORMAT`, `OUTCOME_UNSUPPORTED_CODEC`, `OUTCOME_TOO_LARGE`, `OUTCOME_DIMENSIONS`, `OUTCOME_CORRUPT`, `OUTCOME_TOO_LONG` | Le fichier n’est pas acceptable | Marque l’envoi comme échoué |

Un `outcome` manquant ou inconnu et un appel échoué comptent comme `OUTCOME_TEMPORARY`.

<!-- source: a-plugin-with-no-library be054eb80e74 -->
## Un plugin sans bibliothèque

Avec `json`, un plugin n’a besoin que d’un serveur HTTP et d’une bibliothèque JSON. Un RPC est un `POST` vers
`/<package>.<Service>/<Method>` avec `Content-Type: application/json`, et le plugin répond `200` avec un objet JSON :

```text
POST /my.plugin.probe.v1.ProbeService/Echo
Content-Type: application/json
Connect-Protocol-Version: 1

{"payload":"AP8=","number":"9007199254740993"}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","payload":"AP8=","number":"9007199254740993"}
```

Le JSON suit la correspondance standard de proto3 :

- Un `int64` est une chaîne décimale, les `bytes` sont en base64, et une énumération est son nom.
- Un champ à sa valeur zéro peut être absent, dans les deux sens. Un plugin doit lire un champ absent comme zéro.
- Le serveur ignore les champs qu’il ne connaît pas.
- Un résultat est l'`outcome` de la réponse. Un `outcome` manquant ou inconnu est un échec, jamais un succès.
- Un plugin qui échoue répond par une erreur HTTP, ou par `{"code":"unavailable"}` de Connect avec le statut correspondant. La tâche réessaie.

`Connect-Timeout-Ms` et `Connect-Protocol-Version` peuvent être ignorés.
