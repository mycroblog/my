<!-- source: sending-email 941ecea58cd9 -->
# Envoi d’e-mails

Quels e-mails le serveur envoie, comment configurer l’envoi, et comment gérer les échecs et les désabonnements.
Les paramètres sont listés dans la [référence de configuration](../reference/configuration.md#mail).
Les parcours utilisateur de confirmation de l’adresse e-mail et de réinitialisation du mot de passe sont décrits dans [inscription et confirmation de l’e-mail](registration.md).

<!-- source: what-is-sent-and-how-to-configure-it 3f387cd1afcc -->
## Ce qui est envoyé, et comment le configurer

Le serveur n’envoie jamais d’e-mail au sein d’une requête. Il met en file une tâche pour chaque usage dans la même transaction que le fait qui
déclenche l’e-mail, et un worker de tâches l’envoie. Un échec d’envoi ne se voit pas dans la réponse à une inscription ou à une réinitialisation :
l’utilisateur reçoit le message « nous avons envoyé un e-mail de confirmation » et le redemande s’il n’arrive pas.

| Classe | E-mail | Type de tâche | L’utilisateur peut-il se désabonner ? |
| --- | --- | --- | --- |
| Transactionnel (`transactional`) | Confirmation de l’adresse e-mail | `identity.verification_email` | Non |
| Transactionnel | Réinitialisation du mot de passe | `identity.password_reset_email` | Non |
| Transactionnel | Avis de changement de mot de passe ou de suppression | `identity.password_notice_email` | Non |
| Transactionnel | Avis de désactivation du compte, avec la date de sa suppression | `identity.deactivation_email` | Non |
| Transactionnel | Fermeture du service (l’annonce, un changement de date, une annulation, 7 jours et 1 jour avant la date de fermeture, la fermeture, et 7 jours avant la date limite d’export) | `identity.closure_email` | Non |
| Notification (`notification`) | Avis aux modérateurs (activé par défaut, par type d’avis) | `notification.moderator_email` | Oui |
| Notification (`notification`) | Une notification restée non lue pendant 15 minutes : mentions, abonnements, boosts et autres notifications d’autres utilisateurs, et avis des modérateurs (désactivé par défaut, par type) | `notification.email` | Oui |

Les e-mails de notification destinés aux souscripteurs (`notification.supporter_email`) passent par la même file de tâches.

<!-- source: which-notifications-are-emailed 76c2b871762c -->
### Quelles notifications sont envoyées par e-mail

Chaque utilisateur choisit, par type de notification, si elle arrive dans l’application et par e-mail, sur la page **Notifications** des
paramètres (`/settings/notifications` dans le client web). La colonne E-mail n’y apparaît que lorsque le serveur envoie
des e-mails. Un type désactivé dans l’application n’est pas créé du tout : il n’est donc ni envoyé par e-mail ni poussé.

| Type | E-mail par défaut | Quand il est envoyé |
| --- | --- | --- |
| Notifications d’autres utilisateurs (mentions, nouveaux abonnés, demandes d’abonnement, demandes d’abonnement acceptées, boosts, favoris, réactions d’émoji, citations, sondages terminés, migration de comptes que vous suivez, chats vocaux) | Désactivé | 15 minutes après la notification, seulement si elle est toujours non lue (ni marquée comme lue, ni écartée) et toujours visible, et si l’utilisateur reçoit toujours ce type par e-mail. Un e-mail par notification |
| Avis des modérateurs (une mesure de modération sur le compte, un signalement résolu, une contestation tranchée, des abonnements perdus à cause d’une suspension, une demande d’émoji personnalisé tranchée) | Désactivé | Comme ci-dessus |
| Avis aux modérateurs (nouveaux signalements, inscriptions en attente d’approbation, contestations, webhooks désactivés, demandes d’émoji personnalisé, migration de comptes limités, passage de la date de fermeture, erreurs nouvelles et récurrentes), aux personnes dont le rôle a la permission | Activé | Aussitôt, sans attendre qu’il soit lu |
| Fermeture du service, et avis des formules de soutien | Toujours | Aussitôt ; ils ne peuvent pas être désactivés |

Les e-mails de notification ne vont qu’à l’adresse principale de l’utilisateur, une fois confirmée, et jamais à une adresse dont les e-mails sont
arrêtés (voir plus bas). L’objet de l’e-mail d’une notification d’un autre utilisateur est le même texte que son message Web Push,
et l’e-mail renvoie vers la publication, le profil ou les notifications.

Les e-mails de fermeture du service vont à chaque utilisateur dont l’adresse principale est confirmée. En manquer un peut faire perdre des données : ils sont donc envoyés
comme e-mails transactionnels, ne comportent aucun lien de désabonnement et ne sont pas arrêtés par une plainte (un hard bounce, lui, les arrête).
L’annonce, un changement de date, une annulation et la fermeture mettent chacun en file `identity.closure_email_fanout` dans la même
transaction que leur enregistrement. La tâche horaire `identity.queue_closure_reminders` décide quand envoyer les rappels de 7 jours et
de 1 jour avant la date de fermeture et de 7 jours avant la date limite d’export. Un rappel dont l’heure était déjà passée lorsque la fermeture
a été annoncée n’est pas envoyé, car l’e-mail d’annonce indique les dates.
La tâche de diffusion enregistre une ligne de `service_closure_emails` par utilisateur et met en file `identity.closure_email` uniquement pour les utilisateurs
qu’elle a pu enregistrer : quel que soit le nombre d’exécutions de la tâche, chaque utilisateur reçoit chaque e-mail une seule fois. Avant l’envoi, la tâche vérifie que l’e-mail est
toujours valable (qu’il ne concerne ni une annonce annulée ni une date qui a changé depuis).

Le paramètre `mail` choisit la façon d’envoyer les e-mails.

| `mail` | Comportement |
| --- | --- |
| `none` (par défaut) | N’envoie rien et écrit `mail not sent (mail = none)` dans le journal du serveur avec la catégorie du message. Avec `profile = development`, la ligne contient aussi les adresses, l’objet et le corps, liens compris. Pour les configurations qui n’utilisent pas l’e-mail, et pour le développement |
| `smtp` | Envoie par soumission SMTP. Exige STARTTLS et vérifie le certificat avant de s’authentifier et d’envoyer |
| `ses` | Envoie avec l’API Amazon SES v2 (`SendEmail` avec contenu brut). S’authentifie par la chaîne d’identifiants par défaut du SDK AWS (un rôle IAM, par exemple) et ne détient aucun mot de passe SMTP |
| `plugin` | Confie chaque message à un [plugin](#a-plugin-mail--plugin) que vous exploitez, pour un service d’e-mail qui n’offre qu’une API HTTP |

Les paramètres de `smtp` sont `mail_from`, `mail_smtp_host`, `mail_smtp_port` (587 par défaut), `mail_smtp_username` et `mail_smtp_password`. Les
paramètres de `ses` sont `mail_from`, `mail_ses_region` et `mail_ses_configuration_set`. Les paramètres de `plugin` sont `mail_from`
et `plugin_mailer_address` avec le reste de [sa destination](plugins.md#the-destination).
`mail_from` est une adresse simple telle que `noreply@social.example`. `From` devient `"<server name>" <mail_from>`, et le domaine
de `mail_from` est aussi le domaine de `Message-ID` et celui auquel s’appliquent SPF, DKIM et DMARC (voir plus bas).
`Reply-To` est le paramètre d’instance `contact_email` lorsqu’il est défini ([paramètres de l’instance](instance-settings.md)).
Chaque e-mail porte `Auto-Submitted: auto-generated`.
`smtp` et `ses` construisent le même message. SMTPUTF8 n’est pas utilisé : les adresses sont donc envoyées en ASCII et les domaines internationalisés sont
convertis en Punycode. Une adresse dont la partie locale (avant le `@`) n’est pas en ASCII ne peut pas recevoir d’e-mail, et sa tâche échoue.

<!-- source: examples-by-provider 7ab129a02c43 -->
## Exemples par fournisseur

Transmettez les secrets par variable d’environnement ou par fichier (`MY_MAIL_SMTP_PASSWORD` ou `MY_MAIL_SMTP_PASSWORD_FILE`). Ne les mettez pas dans le
fichier de configuration.

<!-- source: amazon-ses-api 1cef4371e61c -->
### Amazon SES (API)

Si vous êtes sur AWS, `mail = ses` utilise l’API SES v2. Les identifiants proviennent de la chaîne par défaut du SDK AWS (le rôle de tâche d’
ECS, les variables d’environnement, le fichier de configuration partagé) : il n’y a donc aucun secret à configurer.

1. Dans SES, vérifiez le domaine de `mail_from` comme identité et activez Easy DKIM (voir plus bas).
2. Demandez l’accès à la production pour quitter le sandbox. Dans le sandbox, vous ne pouvez envoyer qu’à des adresses vérifiées.
3. Autorisez `ses:SendEmail` pour le rôle d’exécution sur l’identité de `mail_from` (et sur le jeu de configuration, si vous en utilisez un).

```sh
MY_MAIL=ses
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SES_REGION=ap-northeast-1          # the region where you verified the identity; AWS_REGION when empty
MY_MAIL_SES_CONFIGURATION_SET=my-events    # optional; a configuration set with event destinations
```

Lorsque `mail_ses_configuration_set` est défini, chaque e-mail utilise ce jeu de configuration. SES remplace le `Message-ID` et le `Date` de
l’e-mail par ses propres valeurs.
SES ne rejette jamais un destinataire de manière synchrone à l’envoi. Il accepte un envoi vers une adresse de la liste de suppression du compte, puis
ne le distribue pas, et le bounce arrive plus tard sous forme de notification (SNS).
Pour enregistrer les bounces et les plaintes, configurez les notifications SES ci-dessous pour qu’elles arrivent par SNS.

<!-- source: a-plugin-mail--plugin 65b46feaf1cf -->
### Un plugin (`mail = plugin`)

Pour un service d’e-mail qui n’offre qu’une API HTTP, exécutez un [plugin](plugins.md) de type `mailer` et laissez-le appeler le service.

```sh
MY_MAIL=plugin
MY_MAIL_FROM=noreply@social.example
MY_PLUGIN_MAILER_ADDRESS=unix:///run/my/mailer.sock
```

`plugin_mailer_address` est obligatoire avec cette valeur. Un message peut contenir un secret, comme un lien de vérification : ce type
refuse donc `http://`, y compris en loopback : utilisez un socket Unix ou `https://`. Le serveur démarre sans appeler le plugin, même s’il
est arrêté, et l’e-mail attend dans sa tâche.

Avec `plugin_mailer_protocol = json`, la valeur par défaut, un plugin n’a besoin que d’un serveur HTTP et d’une bibliothèque JSON
([Un plugin sans bibliothèque](plugins.md#a-plugin-with-no-library)). `Send` dure au plus 30 secondes, ou `plugin_mailer_timeout`,
sous 5 minutes. Une requête fait au plus 2 Mio et une réponse au plus 64 Kio.

```text
POST /my.plugin.mailer.v1.MailerService/Send
Content-Type: application/json
Connect-Protocol-Version: 1

{
  "category": "CATEGORY_NOTIFICATION",
  "to": "alice@example.com",
  "envelopeFrom": "noreply@social.example",
  "fromName": "My Server",
  "replyTo": "admin@social.example",
  "subject": "You have a new follower",
  "text": "Bob followed you.\n",
  "html": "<p>Bob followed you.</p>",
  "headers": {
    "List-Unsubscribe": "<https://social.example/unsubscribe?token=abc>",
    "List-Unsubscribe-Post": "List-Unsubscribe=One-Click"
  }
}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_RECIPIENT_REJECTED","enhancedStatus":"5.1.1"}
```

Le serveur vérifie un message avant d’appeler le plugin : aucun en-tête ne peut donc être injecté par l’un de ces champs. La requête
contient les parties du message, et le plugin construit ce dont son service a besoin :

- **Le plugin ajoute trois en-têtes :** un `Message-ID` unique par message, sur le domaine de `envelopeFrom` ; un `Date` ; et
  `Auto-Submitted: auto-generated`, qui évite que des réponses automatiques bouclent. Un service qui écrit lui-même son `Message-ID` et son
  `Date` couvre ces deux-là.
- `From` est composé de `fromName` et `envelopeFrom`. `fromName`, `replyTo`, `html` et `headers` peuvent être absents. `headers` ne contient que les
  deux en-têtes `List-Unsubscribe`.
- Les adresses sont en UTF-8. Un plugin dont le service exige de l’ASCII convertit le domaine en Punycode, et répond `OUTCOME_INVALID` pour une
  partie locale qu’il ne peut pas envoyer.
- Le même message peut arriver deux fois, puisqu’une tâche qui échoue est réessayée. N’écrivez pas un message dans un journal.

`outcome` indique ce qui est arrivé au message :

| `outcome` | Signification | Le serveur |
| --- | --- | --- |
| `OUTCOME_SENT` | Le service a pris le message | Termine la tâche |
| `OUTCOME_INVALID` | Il ne peut pas être envoyé tel quel | Fait échouer la tâche aussitôt |
| `OUTCOME_RECIPIENT_REJECTED` | L’adresse n’existe pas | Fait échouer la tâche aussitôt, et [supprime l’adresse](#suppression-of-addresses-that-cannot-receive-email) |
| `OUTCOME_REJECTED` | Le service l’a refusé définitivement, pour une autre raison | Fait échouer la tâche aussitôt |
| `OUTCOME_TEMPORARY` | Le service l’a refusé pour l’instant, ou n’a pas répondu | Réessaie la tâche |
| Toute autre valeur, ou aucune | | Réessaie la tâche |

Une erreur HTTP, ou le `{"code":"unavailable"}` de Connect avec le statut correspondant, est aussi un échec temporaire.
`enhancedStatus` est facultatif : le code de statut étendu d’un refus, tel que `5.1.1`. Le serveur conserve un code de classe 5 pour un
rejet et de classe 4 pour `OUTCOME_TEMPORARY`, et ignore tout le reste. Il ne conserve aucun texte venant d’un plugin : n’y mettez donc aucune raison :
une raison a sa place dans le journal propre du plugin.

Le serveur n’apprend un bounce que par cette réponse. Il n’a aucun point d’accès pour les bounces et plaintes ultérieurs du service
d’un plugin : un bounce que le service signale ensuite ne supprime donc pas l’adresse.

Testez le plugin avec le binaire `my` :

```sh
my plugin test --kind mailer unix:///run/my/mailer.sock
my plugin test --kind mailer --mail-to you@example.com --mail-from noreply@social.example unix:///run/my/mailer.sock
```

La première commande vérifie seulement que le plugin répond, et n’envoie rien. La seconde envoie un vrai message à `--mail-to`, et envoie au
plugin une requête sans destinataire, qu’il doit refuser. Le serveur ne peut pas voir le message une fois que le plugin l’a : regardez donc
celui qui arrive : ses en-têtes doivent contenir `Message-ID`, `Date` et `Auto-Submitted: auto-generated`.
Voir [Tester un plugin](plugins.md#test-a-plugin).

<!-- source: receive-ses-notifications-sns e21b650ab5fe -->
### Recevoir les notifications SES (SNS)

Les bounces et plaintes SES vont vers un topic SNS, et un abonnement HTTPS à ce topic les remet au
`POST /mail/events/ses` du serveur.
Le point d’accès ne s’ouvre que lorsque `mail_ses_sns_topic_arn` est défini ; sinon il répond `404`.

1. Créez un topic SNS standard dans la même région que l’identité SES.
2. Définissez l’attribut de topic `SignatureVersion` sur `2` (SHA256withRSA). Une signature avec la valeur par défaut `1` n’est pas acceptée et le
   serveur répond `400`.
3. Dans la politique d’accès du topic, autorisez `ses.amazonaws.com` à effectuer `SNS:Publish` depuis votre propre compte (la console SES l’ajoute
   lorsqu’elle crée le topic).
4. Dirigez les notifications SES vers le topic, de l’une ou l’autre façon (le serveur lit à la fois `notificationType` et `eventType` dans le corps).
   - Notifications d’identité : utilisez ce topic pour les notifications de retour « Bounce » et « Complaint » de l’identité `mail_from`.
   - Destination d’événements d’un jeu de configuration : ajoutez ce topic comme destination du jeu de configuration de `mail_ses_configuration_set`,
     pour les types d’événements `BOUNCE` et `COMPLAINT`.
5. Définissez `MY_MAIL_SES_SNS_TOPIC_ARN` sur l’ARN du topic, vérifiez avec `my config check` et redémarrez.
6. Créez un abonnement au topic avec le protocole `https` et le point d’accès `https://<server host>/mail/events/ses`.
   **N’activez pas la remise de message brut** (un corps sans signature est rejeté).

```sh
aws sns create-topic --region ap-northeast-1 --name my-ses-feedback
aws sns set-topic-attributes --region ap-northeast-1 \
  --topic-arn arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback \
  --attribute-name SignatureVersion --attribute-value 2
aws ses set-identity-notification-topic --region ap-northeast-1 --identity social.example \
  --notification-type Bounce --sns-topic arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback
aws ses set-identity-notification-topic --region ap-northeast-1 --identity social.example \
  --notification-type Complaint --sns-topic arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback

MY_MAIL_SES_SNS_TOPIC_ARN=arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback   # restart, then create the subscription below

aws sns subscribe --region ap-northeast-1 \
  --topic-arn arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback \
  --protocol https --notification-endpoint https://social.example/mail/events/ses
```

Lorsque le serveur reçoit la confirmation d’abonnement (`SubscriptionConfirmation`), il envoie un `GET` à son `SubscribeURL` pour
confirmer l’abonnement et consigne `SNS subscription of the SES notifications confirmed`.
Vérifiez avec `aws sns list-subscriptions-by-topic` que l’abonnement n’est plus en `PendingConfirmation`.

Avant d’utiliser un message, le point d’accès vérifie tout ce qui suit. Si l’un des points échoue, il répond `400` et n’enregistre rien.

- `TopicArn` est égal à `mail_ses_sns_topic_arn`.
- `SignatureVersion` vaut `2` et la signature est valide.
- L’URL du certificat de la signature (`SigningCertURL`) est en `https` et son hôte est `sns.<region>.amazonaws.com` pour la région
  du topic. Le `SubscribeURL` d’une confirmation d’abonnement doit avoir le même hôte.

Le certificat est récupéré avec le client HTTP sortant qui protège contre le SSRF et mis en cache dans le magasin KV pendant 24 heures. Le
corps est lu jusqu’à 1 Mio, et un corps plus gros reçoit `413`.
Lorsque le certificat ne peut pas être récupéré, que la confirmation d’abonnement échoue ou que l’enregistrement ne peut pas être écrit, le point d’accès
répond `503` et laisse SNS réessayer. Il répond aussi `503` pendant le mode maintenance.

Voici les notifications qu’il enregistre. Il compare l’adresse du destinataire à l’adresse principale (sans tenir compte de la casse, et un
domaine Punycode correspond aussi sous sa forme Unicode) ; si rien ne correspond, il ne fait rien.

| Notification | Enregistrée comme |
| --- | --- |
| Un bounce dont le `bounceType` est `Permanent` (n’importe quel sous-type tel que `General`, `NoEmail`, `Suppressed` ou `OnAccountSuppressionList`) | `hard_bounce` |
| Une plainte (sauf celles dont le `complaintFeedbackType` est `not-spam`) | `complaint` |
| Un bounce `Transient` ou `Undetermined`, et d’autres notifications telles que la distribution ou l’envoi | Non enregistrée (répond `200`) |

Le `feedbackId` de SES est la clé unique d’un enregistrement : quel que soit le nombre de fois où SNS envoie la même notification, il n’y a qu’un seul enregistrement.

<!-- source: amazon-ses-smtp-interface f46a7a562966 -->
### Amazon SES (interface SMTP)

Lorsque vous ne pouvez pas fournir d’identifiants IAM, par exemple si vous êtes hors d’AWS, utilisez l’interface SMTP de SES avec `mail = smtp`. Dans cette
configuration, seul le rejet du destinataire à l’envoi est enregistré comme suppression
([suppression des adresses qui ne peuvent pas recevoir d’e-mails](#suppression-of-addresses-that-cannot-receive-email)).

1. Dans SES, vérifiez le domaine de `mail_from` comme identité et activez Easy DKIM (voir plus bas).
2. Demandez l’accès à la production pour quitter le sandbox. Dans le sandbox, vous ne pouvez envoyer qu’à des adresses vérifiées.
3. Créez des identifiants SMTP dans « SMTP settings » de SES (ils diffèrent des clés d’accès IAM).

```sh
MY_MAIL=smtp
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SMTP_HOST=email-smtp.ap-northeast-1.amazonaws.com   # the endpoint of your SES region
MY_MAIL_SMTP_PORT=587                                       # STARTTLS
MY_MAIL_SMTP_USERNAME=<user name of the SMTP credentials>
MY_MAIL_SMTP_PASSWORD_FILE=/run/secrets/mail_smtp_password
```

SES remplace `Message-ID` et `Date` par ses propres valeurs.

<!-- source: a-general-smtp-submission-server 62921988a793 -->
### Un serveur de soumission SMTP générique

Utilisez-le pour votre propre serveur SMTP tel que Postfix, ou pour le serveur de soumission d’un fournisseur d’envoi d’e-mails.

```sh
MY_MAIL=smtp
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SMTP_HOST=smtp.provider.example     # a STARTTLS server whose certificate is valid for this name
MY_MAIL_SMTP_PORT=587
MY_MAIL_SMTP_USERNAME=<user name>           # empty for a server without authentication
MY_MAIL_SMTP_PASSWORD_FILE=/run/secrets/mail_smtp_password
```

Le serveur n’envoie rien à un serveur qui n’offre pas STARTTLS, ou dont le certificat ne peut pas être vérifié pour le nom de
`mail_smtp_host`. Les identifiants ne sont envoyés qu’après STARTTLS.
Après avoir modifié les paramètres, vérifiez avec `my config check` et redémarrez.

<!-- source: spf-dkim-and-dmarc d6b36a4f4d74 -->
## SPF, DKIM et DMARC

Gmail et Yahoo exigent SPF, DKIM, DMARC et le désabonnement en un clic de la part des expéditeurs qui envoient beaucoup d’e-mails par jour. Même pour un petit
volume, un e-mail sans ces éléments a plus de chances d’être classé comme spam.
Définissez-les tous dans le DNS du domaine de `mail_from` (par exemple `social.example`), avec les valeurs que votre fournisseur vous donne.

- **SPF** : placez un enregistrement TXT sur le domaine qui autorise le fournisseur par lequel vous envoyez (par exemple, si vous utilisez un domaine MAIL FROM
  personnalisé avec SES, l’`include:` que le fournisseur indique, sur ce domaine).
- **DKIM** : placez dans le DNS la clé publique émise par le fournisseur et activez la signature côté envoi. Pour SES, ce sont les trois
  enregistrements CNAME d’Easy DKIM. Le serveur lui-même ne signe pas : c’est le fournisseur qui signe.
- **DMARC** : un enregistrement TXT à `_dmarc.social.example`. **Commencez par `p=none`**, vérifiez avec les rapports agrégés (`rua=`) que SPF
  et DKIM s’alignent (correspondent au domaine From), puis durcissez vers `quarantine` et `reject`.

```text
_dmarc.social.example.  TXT  "v=DMARC1; p=none; rua=mailto:dmarc@social.example"
```

Pour aligner SPF sur SES, définissez un domaine MAIL FROM personnalisé (le MAIL FROM par défaut est `amazonses.com`, qui ne correspond pas au domaine
From). Si DKIM s’aligne, DMARC réussit.

<!-- source: look-at-send-failures ab9c4f356d3b -->
## Examiner les échecs d’envoi

Examinez les échecs d’envoi sur la page des tâches (`/admin/jobs/list`, permission `manage_jobs` ; voir
[la file de tâches et les tâches en échec](running.md#the-job-queue-and-failed-dead-jobs)).
Filtrez par type de tâche pour ne voir que les tâches d’e-mail (dans l’API d’administration, `GET /api/v1/admin/jobs?type=identity.verification_email`).
Les types figurent dans le tableau ci-dessus.

- **Les échecs temporaires** (erreurs de connexion, délais d’attente dépassés, réponses 4xx) sont réessayés. Un e-mail transactionnel est tenté 6 fois (un
  backoff exponentiel à partir de 30 secondes, environ 15 minutes au total) ; un e-mail de notification suit la politique par défaut (10 fois, environ
  4 heures). Lorsque les tentatives sont épuisées, la tâche devient une tâche en échec (dead job). Un `mail_smtp_host` erroné, une
  authentification échouée et un réseau bloqué apparaissent ici. Après avoir corrigé la cause, relancez-la avec `my job retry <id>` ou depuis l’écran
  des tâches.
- **Les rejets définitifs** (un 5xx pour le destinataire ou le contenu) ne sont pas réessayés et échouent aussitôt.
- Avec `mail = ses`, le `MessageRejected` de SES (le contenu a été rejeté) est un rejet définitif. `TooManyRequestsException`
  (la limite de débit d’envoi), `SendingPausedException`, `MailFromDomainNotVerifiedException`, `AccountSuspendedException` et
  les requêtes qui n’atteignent pas SES sont des échecs temporaires, et une nouvelle tentative réussit une fois que vous avez corrigé le compte ou les permissions.
  Seul le code d’erreur SES est conservé dans le texte de l’erreur.
- Avec `mail = plugin`, c’est l’[`outcome`](#a-plugin-mail--plugin) de la réponse qui décide. Un plugin injoignable, ou qui
  répond par une erreur, est un échec temporaire, et il est compté dans [`my.plugin.failures`](running.md). Seuls l’outcome et le code de statut
  étendu sont conservés dans le texte de l’erreur : la raison d’un échec se trouve donc dans le journal du plugin.
- Un e-mail transactionnel plus ancien qu’une limite de temps depuis sa demande (15 minutes pour une réinitialisation, 1 heure pour une confirmation) est
  terminé sans être envoyé. Cela évite que d’anciens liens n’arrivent en retard ; ce n’est pas un échec.
- Lorsque la tâche s’exécute et qu’il n’y a aucune raison d’envoyer (l’adresse est supprimée, elle a été confirmée entre-temps, ou l’utilisateur
  a perdu la permission ou le paramètre de la notification), elle se termine sans envoyer.

La charge utile d’une tâche ne contient aucune adresse e-mail, aucun corps ni aucun jeton, et le texte d’une erreur non plus. Seuls les ID, le type de tâche et le code de
réponse SMTP subsistent.
Un envoi réussi peut être suivi dans la ligne de journal structurée `job` de chaque exécution (`my.job.type`).

<!-- source: suppression-of-addresses-that-cannot-receive-email 18b4750f1531 -->
## Suppression des adresses qui ne peuvent pas recevoir d’e-mails

Lorsque le serveur SMTP indique que l’adresse du destinataire n’existe pas (un 5xx à `RCPT TO` avec un statut étendu `5.1.x`), le
serveur enregistre l’adresse comme hard bounce et cesse d’y envoyer.
Avec `mail = plugin`, `OUTCOME_RECIPIENT_REJECTED` enregistre le même hard bounce, et rien d’autre ne le fait.
Avec `mail = smtp`, seul ce rejet à l’envoi est enregistré. Les bounces asynchrones qui reviennent après que le serveur SMTP a
accepté l’e-mail, et les boucles de retour de plaintes, ne sont pas reçus.
Avec `mail = ses`, SES ne rejette jamais un destinataire de manière synchrone : rien n’est donc enregistré à l’envoi ; les bounces permanents et les
plaintes sont enregistrés à partir des notifications SES
([Recevoir les notifications SES (SNS)](#receive-ses-notifications-sns)).

| Type | E-mails arrêtés |
| --- | --- |
| `hard_bounce` | Tout sauf l’e-mail de confirmation. L’e-mail de réinitialisation du mot de passe est arrêté lui aussi |
| `complaint` (signalé comme spam) | Les e-mails de notification et d’annonce. Les e-mails transactionnels sont toujours envoyés |

- Les modérateurs voient le type et l’origine d’une suppression sur la page Gestion du compte dans les écrans d’administration (`email_suppression` dans
  `GET /api/v1/admin/accounts/{id}`).
- Il y a deux façons de la **lever**. Lorsque l’utilisateur demande un e-mail de confirmation et confirme de nouveau l’adresse, la suppression
  pour hard bounce est levée (l’e-mail de confirmation est envoyé même si l’adresse est supprimée). L’utilisateur le demande par l’API
  cliente (`POST /api/v1/accounts/email/verification`, 3 par heure au plus) ou sur la page d’état
  de connexion. Lorsque vous avez vérifié qu’il est
  possible d’envoyer, par exemple parce que la boîte aux lettres a été réparée, un modérateur la lève avec
  `POST /api/v1/admin/accounts/{id}/email_suppression/lift` (permission `manage_accounts`). Cela enregistre
  `email_suppression_lifted` dans le journal d’audit ([modération](moderation.md)).
- Pour un utilisateur dont l’e-mail de réinitialisation du mot de passe est arrêté, un administrateur émet un lien de réinitialisation et le lui transmet
  (`my admin account password-reset-link @alice`).
- Sur l’écran des paramètres, les utilisateurs voient que les e-mails n’atteignent pas leur adresse principale, et depuis quand.

<!-- source: unsubscribing 2806dbccc409 -->
## Désabonnement

Les e-mails de notification comportent un lien de désabonnement et les en-têtes `List-Unsubscribe` et `List-Unsubscribe-Post:
List-Unsubscribe=One-Click` (RFC 8058). Le bouton « se désabonner » des clients de messagerie les utilise.

- Les classes de désabonnement correspondent une à une aux lignes des paramètres de notification dont l’utilisateur peut choisir l’e-mail, comme
  `mention` ou `admin_report` (une par type d’avis aux modérateurs, et `moderation` pour les avis des modérateurs).
- Le lien est `https://{host}/mail/unsubscribe?token=...` et ne nécessite aucune connexion. Un jeton est émis par e-mail, seul son hachage est
  conservé dans la base de données, et il expire après 90 jours.
  - `GET` n’affiche qu’un écran de confirmation et ne change rien (pour que les scanners qui préchargent les liens n’arrêtent pas l’e-mail).
    Le bouton envoie le même `POST`.
  - `POST` (un clic) désactive l’e-mail de cette ligne des paramètres de notification, et rien d’autre. Jusqu’à l’expiration du jeton,
    le renvoyer donne le même résultat.
- **On ne peut pas se désabonner des e-mails transactionnels.** La confirmation, la réinitialisation et l’avis de changement de mot de passe empêcheraient
  des avis touchant à la sécurité d’arriver. Ces e-mails ne comportent pas non plus ces en-têtes.
- Les utilisateurs peuvent modifier le même paramètre sur la page **Notifications** des paramètres.

<!-- source: the-html-version 2ca008b1a48a -->
## La version HTML

Chaque e-mail est `multipart/alternative` avec le même contenu en `text/plain` et `text/html` (tous deux en UTF-8). Avec `mail = none`
dans le profil `development`, seule la version texte est écrite dans le journal.

- Il y a une mise en page externe unique, avec des paragraphes, des boutons et une signature (nom du serveur, URL, contact, la raison de l’e-mail,
  et le lien de désabonnement). `<html lang dir>` provient de la langue du destinataire, et les langues comme `ar` sont disposées de droite à
  gauche.
- Il ne charge ni polices ni feuilles de style externes. Les couleurs sont les design tokens par défaut, et le clair et le sombre suivent
  `color-scheme` et `prefers-color-scheme`. Les surcharges de thème de l’administrateur n’affectent pas l’e-mail.
- Il ne charge aucune image, sauf le logo propre à l’instance lorsqu’il y en a un ([logo et icône](instance-settings.md#logo-and-icon)) :
  la version HTML commence alors par le logo (`https://<host>/brand/...`, le logo sombre à la place lorsque le client de messagerie suit
  un jeu de couleurs sombre), la même URL pour tous les destinataires, de sorte qu’elle ne révèle pas qui a ouvert l’e-mail. Les clients qui ne chargent aucune
  image externe affichent à la place le nom du serveur (le texte alternatif de l’image). Sans logo, ou lorsque l’URL du serveur n’est pas en `https`,
  l’e-mail n’a aucune image.
- Seules les URL en `https` deviennent des liens. Lorsqu’une URL est en `http`, comme dans une configuration de développement où `web_origin` vaut `http://{host}`, l’URL
  est écrite en texte au lieu d’un bouton ou d’un lien de désabonnement, et l’URL du serveur dans la signature est omise.
- La mise en page fait partie du binaire : aucun JavaScript ne s’exécute donc dans le serveur à l’exécution.
