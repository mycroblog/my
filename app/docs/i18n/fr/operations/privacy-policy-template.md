<!-- source: privacy-policy-template 9966bbc6844c -->
# Modèle de politique de confidentialité

Une politique de confidentialité à adapter pour votre serveur : elle décrit ce que ce serveur conserve, pourquoi, pendant combien de temps, et qui le reçoit.

**Ceci est un modèle et non un conseil juridique.** Il décrit ce que fait le logiciel avec ses paramètres par défaut. Il ne peut pas savoir comment vous exploitez votre instance, et il ne décide pas des lois qui s’appliquent à vous. Ce qu’une politique doit dire dépend de votre juridiction et de celle de vos utilisateurs (par exemple le RGPD dans l’UE, le UK GDPR, ou la loi japonaise sur la protection des informations personnelles). Faites-le vérifier pour votre juridiction avant de le publier, et considérez les bases légales et la déclaration sur les transferts ci-dessous comme des questions à trancher, non comme des réponses.

Le registre des activités de traitement qui l’accompagne se trouve dans le [Modèle de registre des activités de traitement](processing-record-template.md).

<!-- source: how-to-use-it 9cb843cc1ba2 -->
## Comment l’utiliser

1. Copiez [le modèle](#the-template) dans un fichier, `privacy-policy.md`.
2. Remplacez chaque `{{...}}`. Une valeur entre accolades, comme `{{180 days}}`, est la valeur par défaut du logiciel : confirmez-la par rapport à votre instance ([Les durées et leurs paramètres](#the-periods-and-their-settings)) et retirez les accolades.
3. Une ligne ou une phrase qui commence par `{{IF ...}}` ne s’applique qu’à cette configuration ([Les services que vous choisissez](#the-services-you-choose)). Supprimez-la quand elle ne s’applique pas. Quand elle s’applique, retirez le marqueur.
4. Vérifiez qu’il ne reste rien : `grep -n '{{' privacy-policy.md` n’affiche aucune ligne.
5. Publiez-la avec `my instance set privacy_policy - < privacy-policy.md`, ou sur la page « Politique de confidentialité » de « Conditions et politiques » (`/admin/documents/privacy-policy`) dans l’administration. Les lecteurs la trouvent à `/about/privacy` ([paramètres d’instance](instance-settings.md#terms-and-privacy-policy)).

Une politique de confidentialité écrite ajoute une case d’accord obligatoire à l’écran d’inscription. Cette case montre que quelqu’un a lu le document. Ne partez pas du principe qu’elle vaut consentement au traitement là où votre droit exige un consentement.

Le texte a une seule valeur pour toutes les langues. Si vous avez des lecteurs dans plusieurs langues, écrivez les langues dont vous avez besoin dans ce texte unique.

Revoyez la politique quand vous changez un paramètre ci-dessous ou un service, et après chaque mise à jour dont les notes de version mentionnent des données personnelles.

<!-- source: the-periods-and-their-settings 111d5a4b6af8 -->
## Les durées et leurs paramètres

`my instance list` affiche chaque paramètre d’instance et s’il a sa valeur par défaut. Un changement de durée prend effet à la prochaine exécution de sa tâche horaire. Les valeurs hors de la plage sont refusées.

| Enregistrement | Par défaut | Paramètre d’instance | Plage (jours) | Point de départ |
| --- | --- | --- | --- | --- |
| Adresse IP et User-Agent de connexion | 180 jours | `session_origins_retention_days` | 90-3650 | La connexion |
| Adresse IP d’inscription | 180 jours | `registration_origins_retention_days` | 90-3650 | L’inscription |
| Signalements | 365 jours | `reports_retention_days` | 180-3650 | La dernière résolution |
| Mesures contre des comptes, et contestations | 365 jours | `moderation_actions_retention_days` | 180-3650 | La fin de la mesure |
| Journal d’audit | 730 jours | `audit_log_retention_days` | 365-3650 | L’enregistrement |
| Qui a participé à un salon vocal | 90 jours | `voice_participants_retention_days` | 30-3650 | La fin du salon |
| Les décisions d’un examen externe, si vous en utilisez un | 90 jours | `hook_decisions_retention_days` | 30-3650 | La décision |
| Publications distantes auxquelles rien ici ne se réfère | 90 jours | `remote_status_retention_days` | 1-3650 | La publication de la publication |
| Toute publication distante | Aucune limite (`0`) | `remote_status_max_retention_days` | 0-3650 | La publication de la publication |
| Publications distantes qu’un relais seul a apportées | 7 jours | `relay_status_retention_days` | 1-3650 | La publication de la publication |
| Copies de médias distants | 1 jour, et aucune conservée tant que `remote_media_policy` vaut `none` | `remote_media_cache_ttl_seconds`, `remote_media_policy` | 60-31536000 secondes | La récupération |
| Traductions de publications | 30 jours | `translation_retention_days` | 1-3650 | La traduction. La lire ne prolonge pas la durée, et elle disparaît quand la publication est supprimée |

Ce sont des paramètres du déploiement ([référence de la configuration](../reference/configuration.md)), non des paramètres d’instance :

| Enregistrement | Par défaut | Paramètre |
| --- | --- | --- |
| Envois rattachés à rien | 24 heures | `media_unattached_retention` |
| Anciennes versions d’images, après leur remplacement par une version plus récente | 30 jours | `media_replaced_retention` |
| Tâches définitivement échoués | 30 jours | `dead_job_retention` |
| Enregistrements d’exécutions de tâches qui n’ont pas réussi | 30 jours | `job_attempt_retention` |

Ces durées sont fixées par le logiciel, et aucun paramètre ne les modifie :

| Enregistrement | Durée |
| --- | --- |
| Effacement d’un compte supprimé | 7 jours après la suppression ; jusqu’à 30 jours pour un compte distant quand seul un `410 Gone` de son serveur l’a indiqué et que le serveur ne répond plus |
| Suppression d’un compte désactivé | 30 jours après la désactivation |
| Blocages d’inscription | 30 jours après la levée ou l’expiration ; sans l’une ni l’autre, tant qu’ils sont en vigueur |
| Sessions, jetons et codes d’autorisation | 30 jours après leur expiration |
| Archives d’exports de données, et imports de données | 7 jours |
| Enregistrements des activités reçues d’autres serveurs | 30 jours, et le document lui-même 7 jours |
| Enregistrements des activités livrées à d’autres serveurs | 30 jours ; la livraison d’une publication jusqu’à 30 jours après la suppression de la publication ou de son auteur |
| Vérifications d’identifiants distants | 90 jours, en conservant la plus récente de chaque compte |
| Messages et livraisons de webhooks | 14 jours |
| Reçus de webhooks de paiement | 45 jours |
| Compteurs d’erreurs horaires et avis d’erreurs | 90 jours |

Une [conservation légale](moderation.md#holds) interrompt l’effacement pour un compte : chaque durée ci-dessus peut donc se prolonger pour un compte placé sous conservation. Si vous utilisez des conservations, dites-le, comme le fait le modèle.

<!-- source: records-with-no-automatic-end 90a13d1f648e -->
## Enregistrements sans fin automatique

Le serveur ne les retire que lorsqu’un compte est effacé, ou jamais. Une loi peut vous demander de justifier leur conservation, ou de les retirer par d’autres moyens : décidez donc de ce que dit votre politique pour chacun.

- Les lignes d’un compte actif : adresses e-mail, anciennes versions du profil, anciennes versions des publications, notifications, abonnements, et demande d’inscription d’un utilisateur rejeté. Un compte que personne ne supprime reste.
- Ce qui reste après l’effacement : l’URI et le nom d’utilisateur du compte (afin qu’aucun des deux ne puisse être réutilisé), ses événements de cycle de vie, l’enregistrement qu’une publication a été supprimée, les enregistrements de modération jusqu’à la fin de leur propre durée, les invitations et attributions de rôles, les déplacements de comptes, les salons vocaux sans leurs noms et descriptions.
- Les enregistrements de soutien (souscriptions, changements d’état, paiements, remboursements, rétrofacturations), conservés pour la comptabilité et pour le rapprochement avec le prestataire. Le serveur ne supprime pas un client chez le prestataire de paiement : faites-le dans le tableau de bord du prestataire si vos obligations le permettent.
- Une mesure sans fin (un `ban`, ou un `suspend` sans heure de fin), jusqu’à sa levée. Les signalements ouverts.
- Les comptes distants, jusqu’à ce que leur serveur les supprime.
- Le dernier rapport de chaque groupe d’erreurs, que le rapport suivant de ce groupe remplace. Il est nettoyé par motifs : un fragment de texte d’utilisateur peut donc subsister.
- Les sauvegardes, vos journaux, et les journaux de votre plateforme et de votre proxy. Le serveur ne les gère pas : [la sauvegarde](backup.md#how-often-and-where) décrit les générations à conserver, et une sauvegarde restaurée ramène des données effacées.

<!-- source: the-services-you-choose 81461216c966 -->
## Les services que vous choisissez

Chaque service qui reçoit des données personnelles n’apparaît dans la politique que si vous l’avez activé. Remplissez le tableau de la section 6 à partir de cette liste, et lisez la politique et le contrat du service : ce que le service fait des données après leur arrivée n’est pas quelque chose que ce document peut vous dire.

| Marqueur | Paramètre | Ce que le service reçoit |
| --- | --- | --- |
| Hébergement | `database`, `blob`, `queue`, `key_manager` | Tout ce que contiennent la base de données et le stockage de blobs, sous la forme où le prestataire les stocke. Les charges utiles de tâches dans une file cloud contiennent des ID et des catégories, sans adresses, corps de messages ni secrets. |
| `{{IF cdn}}` | `media_origin` | Les adresses IP des lecteurs et leurs requêtes de médias ([CDN](cdn.md)). |
| Proxy | Votre reverse proxy ou équilibreur de charge, dont le serveur a besoin pour TLS | Chaque requête avec son adresse IP, selon la journalisation que vous configurez. |
| `{{IF mail}}` | `mail` vaut `smtp` ou `ses` | Les adresses des destinataires, les objets et les corps. Les e-mails de notification portent le même texte que la notification ([e-mail](email.md)). |
| `{{IF captcha}}` | `captcha` n’est pas `none` | L’adresse IP, les informations sur le navigateur et l’appareil, les cookies et les interactions avec le widget de toute personne qui ouvre la page de connexion, d’inscription ou de demande de réinitialisation du mot de passe, directement depuis son navigateur. Le serveur n’envoie au service que le jeton et n’ajoute aucune adresse IP ([CAPTCHA](captcha.md#what-to-write-in-the-privacy-policy)). |
| `{{IF login providers}}` | [Fournisseurs de connexion](login-providers.md) | Le fournisseur voit la connexion. Le serveur reçoit l’identifiant que le fournisseur donne à la personne, le conserve, et à l’inscription utilise l’adresse e-mail et le nom d’utilisateur que le fournisseur fournit. |
| `{{IF stripe}}` | `payments = stripe` | L’adresse e-mail qui préremplit Checkout, la formule et l’ID de souscription local. Stripe collecte les données de paiement sur ses propres pages ([Stripe](payments-stripe.md#card-data-and-pci-dss)). |
| `{{IF payjp}}` | `payments = payjp` | L’adresse e-mail et la formule. Le navigateur charge payjp.js sur les pages de paiement du serveur, la carte va dans l’iframe de PAY.JP, et le nom et l’adresse e-mail vont à PAY.JP pour le 3-D Secure ([PAY.JP](payments-payjp.md#card-data-and-pci-dss)). |
| `{{IF error reports}}` | `errors_exporter` | Des rapports nettoyés sans compte, adresse IP, corps ni en-tête. Le nettoyage repose sur des motifs : un fragment de texte d’utilisateur peut donc subsister ([signalement d’erreurs](error-reporting.md#what-scrubbing-does-and-does-not-guarantee)). |
| `{{IF telemetry}}` | `metrics_exporter`, `traces_exporter` | Des métriques et des spans sans comptes ni adresses IP de clients. Les spans des requêtes sortantes nomment les noms d’hôtes des serveurs distants ([exécution](running.md)). |
| `{{IF voice media server}}` | `voice` vaut `livekit` ou `sora` | Les adresses IP des participants, leur audio pendant qu’ils parlent, un ID de session et l’ID du salon. L’ID du compte n’est pas envoyé ([chat vocal](voice.md)). |
| `{{IF turn}}` | `voice_stun_urls`, `voice_turn` | Les serveurs STUN et TURN voient les adresses IP des participants, et un serveur TURN relaie leur audio chiffré. |
| `{{IF webhooks}}` | [Webhooks du serveur](webhooks.md) | Votre récepteur reçoit les corps des événements. Certains portent l’adresse e-mail d’un candidat ou le commentaire d’un signalement. |
| `{{IF follow review}}` | `plugin_inbound_follow_address` est défini et `hook_inbound_follow_mode` n’est pas `off` | Les adresses des deux comptes de chaque abonnement venant d’un autre serveur, envoyées par le serveur à l’arrivée de l’abonnement ([hooks de décision](decision-hooks.md)). La décision enregistrée conserve l’adresse de l’activité et le compte qui l’a envoyée. |
| `{{IF sign-up review}}` | `plugin_signup_check_address` est défini et `hook_signup_check_mode` n’est pas `off` | L’adresse e-mail et l’adresse IP de chaque candidat sans invitation, envoyées par le serveur à son inscription ([hooks de décision](decision-hooks.md#sign-ups)). La décision enregistrée ne contient ni l’une ni l’autre. Nommez le service dans la section 6, sauf si vous exploitez vous-même le plugin. |
| `{{IF post review}}` | `plugin_local_status_address` est défini et `hook_local_status_mode` n’est pas `off` | Le texte de chaque publication `public`, `local` et `unlisted` d’un compte plus jeune que `hook_local_status_account_age_days` (7 jours par défaut ; `0` signifie tous les comptes), tel que saisi et tel qu’affiché, avec son avertissement de contenu, sa langue, s’il est marqué comme sensible, les options de sondage, les descriptions d’images, les hashtags, le nombre de comptes qu’il mentionne, l’adresse qu’il recevrait, les adresses des publications auxquelles il répond et qu’il cite, l’adresse et le nom d’utilisateur de l’auteur, et la date de création du compte, envoyés par le serveur quand la publication est faite et de nouveau à chaque modification ([hooks de décision](decision-hooks.md#local-posts)). Les publications `private` et `direct` ne sont jamais envoyées. La décision enregistrée ne contient aucun texte. Un auteur qui conteste une publication refusée envoie son texte, que le serveur conserve avec la décision. Nommez le service dans la section 6, sauf si vous exploitez vous-même le plugin. |
| `{{IF profile review}}` | `plugin_inbound_actor_address` est défini et `hook_inbound_actor_mode` n’est pas `off` | Les profils publics de comptes d’autres serveurs, chaque fois que l’un change : le nom d’affichage, la bio, les champs du profil, les adresses de l’avatar et de l’image d’en-tête (pas d’images), l’adresse, le serveur et le nom d’utilisateur du compte, et si le compte est nouveau ici ([hooks de décision](decision-hooks.md#profiles-from-other-servers)). Rien des comptes de votre serveur n’est envoyé. La décision enregistrée conserve l’adresse du compte et une empreinte du profil. |
| `{{IF profile review}}` | `plugin_local_profile_address` est défini et `hook_local_profile_mode` n’est pas `off` | Le nom d’affichage, la bio telle que saisie et telle qu’affichée, les champs du profil, si l’avatar ou l’en-tête a changé, l’adresse et le nom d’utilisateur du compte, et la date de création du compte, envoyés par le serveur chaque fois qu’un compte plus jeune que `hook_local_profile_account_age_days` (7 jours par défaut ; `0` signifie tous les comptes) les modifie ([hooks de décision](decision-hooks.md#local-profiles)). Aucune image n’est envoyée. La décision enregistrée n’en contient rien. Nommez le service dans la section 6, sauf si vous exploitez vous-même le plugin. |
| `{{IF external review of posts}}` | `plugin_inbound_status_address` est défini et `hook_inbound_status_mode` n’est pas `off` | Les publications et révisions qu’envoient d’autres serveurs et qui concernent un compte d’ici, y compris les publications réservées aux abonnés (`private`) : le texte dans chacune de ses langues, l’avertissement de contenu, s’il est marqué comme sensible, sa date de publication, son adresse et son URL, l’adresse de la publication à laquelle il répond, les options de sondage, les hashtags, les adresses et descriptions des pièces jointes (pas de médias), son auteur et le serveur de l’auteur, sa visibilité, combien de comptes il nomme et combien d’entre eux sont ici, et si quelqu’un d’ici suit l’auteur ([hooks de décision](decision-hooks.md#what-is-sent-and-what-it-costs)). Les publications directes ne partent que lorsque quelqu’un d’ici suit l’auteur, ou avec `hook_inbound_status_unfollowed_direct` depuis n’importe quel auteur. |
| `{{IF media review}}` | `plugin_media_check_address` est défini et `hook_media_check_mode` n’est pas `off` | La plus petite image traitée de chaque envoi, sans métadonnées, jamais l’original, avec l’usage du fichier ainsi que son type, sa taille et sa durée ([hooks de décision](decision-hooks.md#what-mediacheck-reviews)). La décision enregistrée conserve l’ID du fichier et son auteur d’envoi. Un envoi rejeté n’est pas du tout conservé : si vous devez en préserver ou en signaler un, votre plugin doit le faire. Nommez le service dans la section 6, sauf si vous exploitez vous-même le plugin. |
| `{{IF translation}}` | `translation_providers` n’est pas vide | Le texte d’une publication quand un lecteur demande à la traduire : son corps, son avertissement de contenu, les options de sondage et les descriptions d’images. L’adresse de la publication, son auteur, le lecteur, les mentions et les liens ne sont pas envoyés. Par défaut, un prestataire reçoit les publications `public`, `local` et `unlisted`. Il agit comme votre sous-traitant, comme l’hébergement : les publications `local` lui parviennent donc bien qu’elles ne soient pas fédérées ; conservez la phrase sur `local` dans la section 5. Un prestataire qui garde le texte pour son propre usage n’est pas un sous-traitant en ce sens, et la politique doit le dire. |
| `{{IF translation}}` avec la formule gratuite de DeepL | `translation_providers` inclut `deepl` avec une clé se terminant par `:fx` | La même chose. DeepL peut conserver définitivement le texte et les traductions, et la politique doit le dire ([traduction](translation.md#deepl)). |
| `{{IF translation}}` avec Amazon Translate | `translation_providers` inclut `aws` | La même chose. Sauf si la politique de désinscription des services d’IA d’AWS Organizations couvre votre compte, Amazon peut stocker le texte et l’utiliser pour améliorer ses services, éventuellement dans une autre région, et la politique doit le dire ([traduction](translation.md#amazon-translate)). |
| `{{IF translation to private and direct}}` | `translation_provider_visibilities` autorise `private` ou `direct` pour un prestataire | La même chose, pour ces publications. N’utilisez ce marqueur que si vous l’avez autorisé, prestataire par prestataire. Il ajoute des lignes aux sections 2 et 5, et il élargit votre promesse sur ces publications : annoncez-le donc avant de changer le paramètre. |
| `{{IF translation of reports}}` | `translation_reports_to_server` | Le commentaire d’un signalement et les publications qu’il nomme, quand un modérateur demande à les traduire. Chaque prestataire ne reçoit toujours que les visibilités qui lui sont autorisées, et le commentaire compte comme `private` : si aucun prestataire n’est autorisé pour `private`, les commentaires n’en atteignent aucun. La traduction du commentaire n’est pas stockée. |
| `{{IF browser translation}}` | `translation_browser` | Rien n’atteint ce serveur ni un prestataire. Le navigateur traduit sur l’appareil du lecteur, et ce que fait l’éditeur du navigateur dépend de ce navigateur. |

Les services sont vos choix : ce modèle ne dit donc pas où chacun est établi ni quel contrat le couvre. Confirmez les deux vous-même.

Deux choses atteignent des tiers sans paramètre de votre part :

- **Web Push.** Quand un utilisateur l’active, son navigateur choisit le service de push et donne son adresse au serveur. Le corps du message est chiffré pour ce navigateur : le service de push ne devrait donc pas pouvoir le lire. Il voit en revanche l’adresse d’abonnement, la taille et l’heure de chaque message, et le contact (sujet VAPID) de votre serveur, et il conserve un message jusqu’à 48 heures pendant que le navigateur est hors ligne.
- **Fédération.** Voir la section 5 du modèle. `federation_mode` la désactive ou en limite le sens, et les relais sont à vous d’ajouter ([relais](relays.md)).

<!-- source: the-template 8b6bd2fc983a -->
## Le modèle

```markdown
Last updated: {{DATE}}

This policy describes how {{OPERATOR NAME}} ("we") handles personal data on {{INSTANCE NAME}} ({{https://INSTANCE DOMAIN}}).

# 1. Who is responsible

- Controller: {{OPERATOR NAME, POSTAL ADDRESS}}
- Contact for privacy questions: {{EMAIL OR FORM}}
- {{IF you have a representative in the EU or the UK: name and contact}}
- {{IF you have a data protection officer: name and contact}}

# 2. Whom this covers

This policy covers people with an account here, people who read pages here without an account, and people on other servers whose posts and profiles reach this server.

{{IF translation}} If you are on another server, the text of your posts that reach this server can go to the translation services in section 6 when a reader here asks for a translation. Your server cannot tell you when this happens.

{{IF external review of posts}} If you are on another server, your posts that relate to an account here, followers-only posts included, go with their text to the external review in section 6 before this server accepts them. It can stop a post or mark it, or file a report for our moderators; people decide on any action against an account.

# 3. What we keep, why, and for how long

"Until your account is erased" means: deleting your account hides it at once and erases it 7 days later. A deactivated account stays hidden, can be reactivated for 30 days, and is then deleted the same way.

| Data | Why | How long |
| --- | --- | --- |
| Email addresses | Signing in, confirming the address, resetting the password, and the email you choose to get | Until your account is erased |
| Password hash, two-factor secret (encrypted) and recovery codes (hashed), passkeys, the identifier of an account you linked at a login provider | Signing you in | Until your account is erased |
| Language, time zone, default post settings, notification settings, and other preferences | Running the service the way you asked | Until your account is erased |
| Registration application: the reason you gave, the invite you used, and the decision | Deciding who joins | Until your account is erased |
| IP address at registration | Preventing abuse | {{180 days}} from registration |
| IP address and browser name (User-Agent) at each sign-in | Security and preventing abuse | {{180 days}} from the sign-in |
| Sessions and app access (stored as hashes) | Keeping you signed in | 30 days after they expire. A session lasts 30 days |
| IP address, and for password-reset requests the email address, in rate limit counters | Protecting the service | Removed automatically, within twice the window of the limit |
| Profile: display name, bio, extra fields, avatar, header, aliases, and earlier versions of them | Showing your profile, which is public | Until your account is erased |
| Posts, their earlier versions, and attached media | Publishing what you write | Until you delete them, your automatic deletion policy deletes them, or your account is erased. {{IF you set a maximum for keeping posts}} At most {{N days}} after you publish them, unless your account may keep them longer. Uploads attached to nothing go after {{24 hours}} |
| Favourites, reactions, boosts, bookmarks, pins, poll votes, follows, blocks, mutes, lists | Running the features you use | Until you undo them, or your account is erased |
| Notifications and what you have read | Telling you what happened | Until your account is erased or the post they are about is removed |
| Web Push subscription: your browser's push address and keys | Sending you push notifications | Until you turn it off, the push service ends it, or your account is erased |
| Records that an email bounced or was reported as spam | Not sending mail that is unwanted or cannot arrive | Until your account is erased |
| Data export archives and data imports | Giving you your data, and bringing it in | 7 days |
| Reports: the comment, the posts and rules named, and moderators' notes | Moderation | {{365 days}} from the last resolution, or as long as an action refers to them. Open reports stay |
| Actions against accounts, and appeals | Moderation, and handling repeat abuse | {{365 days}} from the end of the action. An action with no end stays until it is lifted |
| Posts that a report or an action points to | Evidence for moderation | Hidden when deleted, and removed with that record |
| Audit log of staff actions, which can name accounts | Accountability of staff | {{730 days}} |
| Sign-up blocks: an email domain, a hashed email address, or an IP range | Preventing repeat abuse | While in effect, and 30 days after the lift or expiry |
| {{IF voice}} Voice chat rooms: name, description, hosts, and how they ended | Showing and moderating rooms | Kept. The name and description you wrote are emptied when your account is erased |
| {{IF voice}} Who took part in a room: participation, speakers, raised hands, invitations, removals | Moderating rooms | {{90 days}} from the end of the room, or as long as a report about the room. We record neither audio nor text chat |
| {{IF payments}} Supporter plans: the provider's IDs for you and your subscription, amounts, periods, and states. Not your card details | Running your subscription, and accounting | {{PERIOD AND REASON, e.g. as long as tax law requires}}. The link to the provider's customer goes when your account is erased |
| {{IF payments}} Your choice to appear in the list of supporters | Showing the list | Until the subscription ends or your account is erased |
| Copies of activities sent to and received from other servers | Delivering, retrying, and rejecting duplicates | 30 days. The first delivery of a post stays until 30 days after the post or its author is deleted |
| Posts and profiles of people on other servers | Showing them here | Posts: {{90 days}} when nothing here refers to them, {{7 days}} when only a relay brought them, and {{no limit}} otherwise. Profiles: until their server deletes the account. Then erased after 7 days |
| Copies of remote media | Showing it | {{Not kept, or the period you set}} |
| Server logs: request paths, which can contain a username or a post ID. Our server software writes no IP address in them | Running and debugging the service | {{PERIOD}} |
| {{IF proxy logs}} Logs of our proxy or load balancer: IP addresses and requests | Running and protecting the service | {{PERIOD}} |
| Error reports, with personal details removed by pattern | Fixing faults | The latest report of each kind of error stays until a newer one replaces it. Hourly counts: 90 days |
| {{IF webhooks}} Messages to our webhook receivers, which can carry an applicant's email address or a report's comment | Connecting our own tools | 14 days here. {{PERIOD AT THE RECEIVER}} |
| {{IF post review}} The text of a refused post that you send with an appeal, and the decision | Letting staff review the appeal | {{90 days}} from the refusal, and longer while you can still appeal or staff have not decided. Removed when your account is erased |
| {{IF translation}} Translations of posts that readers asked for, which every reader who may see the post can reuse | Showing a translation again without sending the post again | {{30 days}} from the translation, and no longer than the post |
| {{IF translation}} How many characters your requests sent for translation in a month | Keeping translation within its limits | Until the end of the following month, or until your account is erased |
| Backups of all the above | Restoring the service after a failure | {{PERIOD}}. Data you erased stays in a backup until the backup expires |

We can stop erasure for one account when a court order or a legal request requires it. The records of that account then stay until the hold is released.

After erasure we keep the account's address and username so that nobody can reuse them, the record that its posts were deleted, its moderation records until their period ends, invites and roles it took part in, moves it made, and {{IF payments}} the supporter records above.

# 4. Why we are allowed to

{{CHOOSE WITH YOUR ADVISER. A common starting point under GDPR Article 6(1), not a conclusion:}}

- Running your account and the features you use: {{performance of a contract, point (b)}}.
- Security, preventing abuse, moderation, the audit log, logs, and backups: {{legitimate interests, point (f): keeping the service safe and working}}.
- Storing and showing what people on other servers publish: {{legitimate interests, point (f)}}.
- Records the law makes us keep: {{legal obligation, point (c)}}.
- {{IF you rely on consent for anything: what, and how to withdraw it}}.

You need an email address {{OR a login provider}} to create an account here, and without one we cannot provide an account.

# 5. What other people and other servers receive

**This server is part of a federated network.** Other servers run by other people, in other countries, receive your data as part of how the service works, and their own policies apply to it. We cannot make them delete it.

| What | Who receives it |
| --- | --- |
| Your profile, including your avatar, header, and aliases | Anyone, on the web and through other servers |
| `public` and `unlisted` posts | Anyone with the link. The servers of your followers and of the people you mention. {{IF relays}} `public` posts also go to relays we joined, and through them to other servers |
| `local` posts | People signed in to this server, and your followers on other servers. {{IF translation}} When a reader asks for a translation, the text also goes to the translation services in section 6, which work for us. They do not receive the post's address or your handle |
| `private` posts | Your followers and the people you mention, on any server |
| `direct` posts | The people you mention, on any server. They are not end-to-end encrypted: we can technically read them |
| {{IF translation to private and direct}} {{`private` AND/OR `direct`, AS YOU ALLOWED}} posts | {{TRANSLATION SERVICES YOU ALLOWED THEM}}, when a reader who may see the post asks for a translation. They do not receive the post's address, your handle, the reader, or who is mentioned |
| Follows, blocks, boosts, favourites, reactions, poll votes, quotes | The other person's server. Boosts also go to your followers' servers |
| That your account moved, was deactivated, suspended, or deleted | Your followers' servers and, except for a move, other servers we know |
| A report you chose to forward | The reported account's server receives your comment and the posts named. It does not receive your name, IP address, or email address, so do not put those in the comment |
| {{IF translation of reports}} A report's comment and the posts it names | {{TRANSLATION SERVICES YOU ALLOWED `private` POSTS}} receive the comment, and the services in section 6 receive the posts as for a reader, when a moderator asks for a translation. They do not receive who reported, the report, or the posts' addresses |

When you edit or delete a post or delete your account, we tell those servers. Most honour it, but we cannot guarantee it, and copies, including ones made by others, can remain.

Other people here see what you publish. Staff with the right permission can reveal your email address and your registration and sign-in IP addresses, and each reveal is recorded in the audit log. Staff can also see email addresses of pending registrations.

Search engines and AI services: {{DESCRIBE YOUR search_indexing, indexable_default, AND ai_usage SETTINGS. By default we ask them not to index and not to use the content}}. These are requests, and not every crawler follows them. You can change whether your own profile appears in search in your profile settings.

{{IF the full-text search runs on another company's service}} Full-text search: the text of posts in search ({{public / local}} posts of people who chose to appear in search) goes to {{SERVICE}} to make them searchable, and is removed there when the post is deleted or you stop appearing in search. {{SAY HOW LONG ITS BACKUPS KEEP IT}}. {{IF you keep logs that hold search terms}} What you search for can appear in our request logs, which we keep for {{PERIOD}}.

Apps you authorize can read and do what the permissions you granted allow. Their own policies apply. You can revoke them in your settings.

{{IF the list of supporters is public}} If you choose to appear in the list of supporters, it shows your avatar, display name, handle, and plan.

{{IF browser translation}} A reader can also translate a post with the translation built into their browser. That sends nothing to us or to a translation service, and the translation is not stored.

# 6. Service providers

We use these services to run the instance. {{SAY WHETHER EACH ACTS ON YOUR BEHALF AND WHAT CONTRACT APPLIES.}}

| Service | Purpose | Data | Location | Their policy |
| --- | --- | --- | --- | --- |
| {{HOSTING PROVIDER}} | Server, database, and file storage | Everything in section 3 | {{COUNTRY}} | {{LINK}} |
| {{IF cdn}} {{CDN PROVIDER}} | Delivering media | IP address and requests for media | {{COUNTRY}} | {{LINK}} |
| {{IF mail}} {{MAIL PROVIDER}} | Sending email | Your address, and the subject and text of each email | {{COUNTRY}} | {{LINK}} |
| {{IF captcha}} {{CAPTCHA SERVICE}} | Stopping automated sign-ins, sign-ups, and password-reset requests | IP address, browser and device information, cookies, and interactions with the widget. Your browser sends these directly, from the sign-in, sign-up, and password-reset request pages only | {{COUNTRY}} | {{LINK}} |
| {{IF login providers}} {{PROVIDER}} | Signing in with an account there | That you sign in. We receive their identifier for you, and at sign-up your email address and username | {{COUNTRY}} | {{LINK}} |
| {{IF follow review}} {{FOLLOW REVIEW SERVICE}} | Screening follows from other servers for abuse | The addresses of your account and of the account on the other server, when someone there follows you | {{COUNTRY}} | {{LINK}} |
| {{IF sign-up review}} {{SIGN-UP REVIEW SERVICE}} | Screening sign-ups for abuse | The email address and IP address you sign up with, unless you sign up with an invite | {{COUNTRY}} | {{LINK}} |
| {{IF post review}} {{POST REVIEW SERVICE}} | Screening new posts and their edits for abuse | Your `public`, `local`, and `unlisted` posts, each time you post or edit one, while your account is younger than {{7 days}}: their text, content warning, language, whether they are marked sensitive, poll options, image descriptions, hashtags, and how many accounts they mention, their address and the addresses of the posts they reply to or quote, your address and username, and when your account was created. Not your media | {{COUNTRY}} | {{LINK}} |
| {{IF profile review}} {{PROFILE REVIEW SERVICE}} | Screening the profiles of accounts on other servers for abuse | Nothing about your account on this server. The public profile of an account on another server when this server takes in a change of it: its display name, bio, profile fields, the addresses of its avatar and header image, its address and username | {{COUNTRY}} | {{LINK}} |
| {{IF profile review}} {{PROFILE REVIEW SERVICE}} | Screening changes of profiles for abuse | Your display name, bio, and profile fields, and whether you changed your avatar or header, each time you change them while your account is younger than {{7 days}}, with your address and username and when your account was created. Not your images | {{COUNTRY}} | {{LINK}} |
| {{IF media review}} {{MEDIA REVIEW SERVICE}} | Screening uploads for abuse before anyone can see them | A small processed copy of each image you upload, or of the poster or cover of a video or audio file, without its metadata, and what the file is for. Not the original, its description, or your account | {{COUNTRY}} | {{LINK}} |
| {{IF translation}} {{TRANSLATION SERVICE}} | Translating a post when a reader asks | The text of the post: its body, content warning, poll options, and image descriptions. Not its address, its author, the reader, mentions, or links. Text sent to the service cannot be recalled, and deleting the post does not remove it there: {{HOW LONG THE SERVICE KEEPS IT}} | {{COUNTRY}} | {{LINK}} |
| {{IF external review of posts}} {{REVIEW SERVICE}} | Reviewing posts from other servers before they are accepted | The post as its server sent it, followers-only posts included: its text and language, content warning, whether it is marked sensitive, when it was published, its address, the address of the post it replies to, poll options, hashtags, its author, its visibility, attachments' addresses and descriptions, how many accounts it names and how many of them are here, and whether anyone here follows its author. Direct posts {{only when someone here follows their author, or from any author if you turned on `hook_inbound_status_unfollowed_direct`}}. No post of this server's accounts | {{COUNTRY}} | {{LINK}} |
| {{IF stripe}} Stripe | Taking payments for supporter plans | Your email address and plan. You enter payment details on Stripe's pages | {{COUNTRY}} | <https://stripe.com/privacy> |
| {{IF payjp}} PAY.JP | Taking payments for supporter plans | Your email address, cardholder name, and plan. You enter card details in PAY.JP's frame, whose script runs on our payment pages | {{COUNTRY}} | {{LINK}} |
| {{IF error reports}} {{ERROR TRACKER}} | Fixing faults | Error reports with personal details removed by pattern | {{COUNTRY}} | {{LINK}} |
| {{IF telemetry}} {{METRICS OR TRACING SERVICE}} | Watching the health of the service | Measurements, and the names of other servers we contact | {{COUNTRY}} | {{LINK}} |
| {{IF voice media server}} {{VOICE SERVICE}} | Carrying voice chat | Your IP address and audio while you speak in a room, and a session ID. Not your account | {{COUNTRY}} | {{LINK}} |
| {{IF turn}} {{STUN OR TURN SERVICE}} | Connecting voice chat | Your IP address, and encrypted audio when relayed | {{COUNTRY}} | {{LINK}} |

{{IF peer-to-peer voice}} In voice chat rooms, your browser connects directly to other participants, so they can see your IP address.

If you turn on push notifications, your browser's maker or its push service delivers them. The content is encrypted for your browser, but the service sees your browser's push address and when messages are sent.

# 7. Transfers abroad

Delivery to other servers (section 5) sends your data to servers abroad, wherever those servers are. We do not choose or know their countries in advance, and we cannot limit delivery by country.

{{YOUR STATEMENT ON THIS AND ON THE SERVICES OF SECTION 6: which transfer rules you consider apply, and the safeguards you rely on, if any. Settle this with your adviser: whether federated delivery is a restricted transfer is not settled, and this template takes no position.}}

# 8. Cookies and storage in your browser

We use no advertising or analytics cookies. Our pages use these strictly to work:

| Name | Purpose | Lasts |
| --- | --- | --- |
| `__Host-my_session` | Keeps you signed in to the sign-in pages | Up to 30 days |
| `__Host-my_rt` | Keeps the app signed in | Up to 30 days |
| `__Host-my_csrf` | Protects forms against forged requests | Until you close your browser |
| `__Host-my_2fa` | Carries a sign-in to the two-factor step | 5 minutes |
| `__Host-my_external` | Carries a sign-in through a login provider | At most 30 minutes |

The app also stores data in your browser only: display settings, recently used emoji and hashtags, the draft of a post (for the tab), and the newest posts of your home timeline for offline reading, without followers-only and direct posts. The drafts and the timeline are removed when you sign out.

{{IF captcha}} The CAPTCHA service sets its own cookies on the sign-in, sign-up, and password-reset request pages.

{{IF payjp}} PAY.JP's script runs on our payment pages and may set its own cookies. {{CHECK AGAINST PAY.JP'S POLICY.}}

# 9. How we protect it

Passwords are stored as Argon2id hashes. Two-factor secrets and the keys that sign your activities are encrypted, and the encryption key is kept outside the database. Session and access tokens are stored as hashes. Traffic uses HTTPS. {{YOUR MEASURES: encryption at rest and of backups, who has access, and how you handle a breach.}}

# 10. Your rights

{{ADJUST TO THE RIGHTS OF YOUR JURISDICTION. Under the GDPR these are access, rectification, erasure, restriction, objection, and portability, and the right to withdraw consent.}}

- **See and take your data:** your settings offer a data export (your profile, your posts, and your moderation records) and CSV files of follows, blocks, mutes, muted domains, and bookmarks. For everything else, including your email and IP addresses, write to us.
- **Correct it:** edit your profile and account settings, or write to us.
- **Delete it:** delete your account in your settings, or deactivate it first. This is what section 3 and section 5 describe.
- **Move:** you can move your followers to an account on another server.
- **Limit it:** choose your notifications in your settings. Every notification email has an unsubscribe link. You can also object to or ask us to restrict a use of your data by writing to us.
- **Reports about you:** we do not reveal who reported you.

Write to {{CONTACT}}. We answer within {{one month, or the period your law sets}}. You can complain to a data protection authority: {{YOUR AUTHORITY, OR "the authority where you live"}}.

We keep some records even when you ask us to delete them, where we need them for moderation or the law requires them (section 3).

# 11. Children

{{YOUR MINIMUM AGE AND HOW YOU APPLY IT. The server does not ask for a birth date or check age.}}

# 12. Automated decisions

We make no decisions about you by automated means that have legal or similarly significant effects, {{CONFIRM FOR YOUR USE. These are automatic: a sign-up that matches a block, or that an external review answers about, is refused or sent for review, an upload that an external review rejects is refused, {{IF post review}} a post that an external review refuses, which you can appeal to our staff, {{IF profile review}} a change of your profile that an external review refuses, which you can change and save again, the CAPTCHA check, and rate limits. Your staff decide the rest.}}

# 13. Changes

We change this policy when what we do changes, and show the new date at the top. {{HOW YOU TELL PEOPLE, for example an announcement.}} If we close the service, we tell you in advance, with a deadline for taking your data. {{HOW YOU TELL PEOPLE, for example by email and an announcement.}}
```
