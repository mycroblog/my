<!-- source: record-of-processing-activities-template 75f7037cf953 -->
# Modèle de registre des activités de traitement

Le registre des activités de traitement que l’article 30, paragraphe 1, du RGPD demande à un responsable du traitement de tenir, rempli dans la mesure où le logiciel en décide.

**Ceci est un modèle et non un conseil juridique.** Il décrit ce que fait le logiciel avec ses paramètres par défaut, et il ne décide pas si le RGPD, ou ce registre, s’applique à vous. Faites-le vérifier pour votre juridiction.

L’article 30, paragraphe 5, exempte une organisation de moins de 250 salariés, sauf si son traitement est susceptible de comporter un risque pour les droits et libertés des personnes, n’est pas occasionnel, ou porte sur des catégories particulières de données ou sur des données pénales. Exploiter une instance revient à tenir des comptes en continu, ne présumez donc pas de l’exemption sans la vérifier. Tenir le registre vous aide aussi à répondre aux demandes et à rédiger la [politique de confidentialité](privacy-policy-template.md).

<!-- source: how-to-use-it 32e30db3b6f3 -->
## Mode d’emploi

1. Copiez tout ce qui suit [Registre](#record) dans vos propres dossiers. C’est un document interne : ne le publiez pas avec les textes de l’instance, et remettez-le à une autorité de contrôle sur demande.
2. Remplacez chaque `{{...}}`. Une valeur entre les accolades, comme `{{180 days}}`, est la valeur par défaut du logiciel. Confirmez-la pour votre instance. Le [modèle de politique de confidentialité](privacy-policy-template.md#the-periods-and-their-settings) nomme le paramètre de chaque durée.
3. Supprimez une activité, ou une ligne qui commence par `{{IF ...}}`, qui ne s’applique pas à votre configuration. [Les services que vous choisissez](privacy-policy-template.md#the-services-you-choose) indique quel paramètre suit chaque service.
4. Vérifiez qu’il ne reste rien : `grep -n '{{' <your file>` n’affiche aucune ligne.
5. Réexaminez-le lorsque vous changez un paramètre ou un service, et datez chaque révision.

L’article 30 ne demande pas la base juridique, et le registre comporte une ligne pour elle parce que vous en avez besoin pour la politique. Considérez les entrées de cette ligne comme des questions à trancher.

Si vous exploitez une instance pour le compte de quelqu’un d’autre, vous pouvez être un sous-traitant pour cette personne. L’article 30, paragraphe 2, demande alors un registre différent, que ce modèle ne couvre pas.

<!-- source: record ed52538cf508 -->
## Registre

<!-- source: controller-article-301a 9720484d93ff -->
### Responsable du traitement (article 30, paragraphe 1, point a)

| | |
| --- | --- |
| Responsable du traitement | {{NAME, POSTAL ADDRESS, EMAIL}} |
| Responsable conjoint | {{NAME AND CONTACT, OR "none"}} |
| Représentant dans l’Union | {{NAME AND CONTACT, OR "not applicable"}} |
| Délégué à la protection des données | {{NAME AND CONTACT, OR "none"}} |
| Instance | {{INSTANCE NAME AND URL}} |
| Logiciel | my, version {{VERSION}} |
| Dernière révision | {{DATE, BY WHOM}} |

<!-- source: recipients-used-in-this-record 8f1748f022f5 -->
### Destinataires utilisés dans ce registre

Ces noms sont utilisés dans les activités ci-dessous. Listez chacun de ceux que vous utilisez.

| Destinataire | Nom, contact et contrat | Pays |
| --- | --- | --- |
| Hébergement (serveur, base de données, stockage de blobs, file, gestion des clés) | {{...}} | {{...}} |
| Proxy inverse ou répartiteur de charge | {{...}} | {{...}} |
| {{IF cdn}} CDN | {{...}} | {{...}} |
| {{IF mail}} Fournisseur d’e-mail | {{...}} | {{...}} |
| {{IF captcha}} Service de CAPTCHA | {{...}} | {{...}} |
| {{IF login providers}} Fournisseurs de connexion | {{...}} | {{...}} |
| {{IF payments}} Fournisseur de paiement (Stripe ou PAY.JP) | {{...}} | {{...}} |
| {{IF error reports}} Outil de suivi des erreurs | {{...}} | {{...}} |
| {{IF telemetry}} Service de métriques et de traces | {{...}} | {{...}} |
| {{IF voice}} Serveur multimédia vocal, STUN et TURN | {{...}} | {{...}} |
| {{IF webhooks}} Destinataires de webhooks | {{...}} | {{...}} |
| {{IF follow review}} Service d’examen des abonnements | {{...}} | {{...}} |
| {{IF sign-up review}} Service d’examen des inscriptions | {{...}} | {{...}} |
| {{IF post review}} Service d’examen des publications | {{...}} | {{...}} |
| {{IF profile review}} Service d’examen des profils | {{...}} | {{...}} |
| {{IF media review}} Service d’examen des médias | {{...}} | {{...}} |
| {{IF translation}} Fournisseurs de traduction | {{...}} | {{...}} |
| {{IF external review of posts}} Examen externe des publications d’autres serveurs | {{...}} | {{...}} |
| Stockage des sauvegardes | {{...}} | {{...}} |
| Serveurs fédérés | Autres serveurs du réseau, exploités par d’autres responsables du traitement, qui changent avec le temps. `my` tient la liste des serveurs connus | N’importe quel pays |
| {{IF relays}} Relais | {{EACH RELAY YOU JOINED}} | {{...}} |
| Services push | Choisis par le navigateur de chaque utilisateur | Dépend du navigateur |
| Applications | Autorisées par chaque utilisateur | Dépend de l’application |

<!-- source: transfers-article-301e 82e1298af5f2 -->
### Transferts (article 30, paragraphe 1, point e)

- La livraison fédérée envoie des données personnelles à des serveurs à l’étranger, dans des pays qui ne sont pas connus à l’avance et ne peuvent pas être limités. {{YOUR ASSESSMENT: whether this is a transfer under GDPR Chapter V, and on what basis. This template leaves it open.}}
- Transferts vers les services ci-dessus : {{FOR EACH, THE THIRD COUNTRY, AND THE ADEQUACY DECISION OR THE SAFEGUARD}}.

<!-- source: 1-accounts-and-signing-in 865bacb0785b -->
### 1. Comptes et connexion

| | |
| --- | --- |
| Finalités | Créer des comptes, se connecter, récupérer l’accès, trancher les inscriptions |
| Personnes concernées | Les personnes ayant un compte, les personnes qui en demandent un |
| Données personnelles | Adresses e-mail, empreinte du mot de passe, secret à deux facteurs et codes de récupération hachés, clés d’accès, identifiant chez un fournisseur de connexion, préférences (langue, fuseau horaire, valeurs par défaut de publication), raison de l’inscription, invitation et décision, rôles, jetons de session et d’application hachés |
| Destinataires | Hébergement, fournisseur d’e-mail, fournisseurs de connexion, équipe disposant de `manage_accounts` |
| Effacement | Effacement du compte : 7 jours après la suppression. Un compte désactivé est supprimé après 30 jours. Sessions, jetons et codes : 30 jours après l’expiration. Les invitations et attributions de rôles, l’URI du compte et le nom d’utilisateur restent. Un compte que personne ne supprime reste |
| Base juridique | {{e.g. contract}} |

<!-- source: 2-protecting-sign-in-and-registration 66d632cbf187 -->
### 2. Protection de la connexion et de l’inscription

| | |
| --- | --- |
| Finalités | Prévenir les abus, retrouver les comptes d’une même personne pendant la modération, limiter les débits, arrêter les robots |
| Personnes concernées | Les personnes ayant un compte, les demandeurs, les visiteurs des pages de connexion |
| Données personnelles | Adresse IP d’inscription, adresse IP et User-Agent de connexion, adresse IP (et, pour les demandes de réinitialisation du mot de passe, l’adresse e-mail) dans les compteurs de limitation de débit, blocages d’inscription (domaine de l’e-mail, adresse hachée, plage d’IP). Avec un CAPTCHA : adresse IP, informations sur le navigateur et l’appareil, cookies. Avec un examen des inscriptions : l’adresse e-mail et l’adresse IP de chaque demandeur sans invitation |
| Destinataires | Hébergement, équipe disposant de `manage_accounts` (chaque affichage est dans le journal d’audit), service de CAPTCHA, service d’examen des inscriptions |
| Effacement | Adresses IP d’inscription `{{180 days}}`, origines de connexion `{{180 days}}`, compteurs dans un délai de deux fois la fenêtre de la limite, blocages d’inscription 30 jours après la levée ou l’expiration. Suspendu par une conservation obligatoire |
| Base juridique | {{e.g. legitimate interests}} |

<!-- source: 3-publishing-and-social-features 5e271f0279f9 -->
### 3. Publication et fonctionnalités sociales

| | |
| --- | --- |
| Finalités | Publier des profils et des publications, et les interactions que les personnes demandent |
| Personnes concernées | Les personnes ayant un compte, les personnes qu’elles mentionnent |
| Données personnelles | Profil et ses versions, publications et leurs versions, médias, favoris, réactions, boosts, marque-pages, épingles, votes de sondage, abonnements, blocages, mises en sourdine, listes. Les publications peuvent contenir toute donnée qu’une personne écrit, y compris des catégories particulières. `{{IF post review}}` Le texte d’une publication refusée que son auteur envoie avec une contestation |
| Destinataires | Hébergement, CDN, service d’examen des médias (une petite copie traitée de chaque téléversement), autres utilisateurs, le web public pour le contenu public, serveurs fédérés (activité 4), moteurs de recherche et robots d’IA selon `search_indexing` et `ai_usage`, `{{the operator of the full-text search engine, if another company runs it}}`, applications que la personne a autorisées, `{{IF post review}}` le service d’examen des publications pour les publications `public`, `local` et `unlisted` des nouveaux comptes et leurs modifications (leur texte tel que saisi et tel que rendu, avertissement de contenu, langue, indicateur de contenu sensible, options de sondage, descriptions d’images, hashtags et nombre de mentions, leur adresse et les adresses des publications auxquelles elles répondent ou qu’elles citent, l’adresse et le nom d’utilisateur de l’auteur, et la date de création du compte ; aucun média), `{{IF profile review}}` le service d’examen des profils pour les modifications de profil des nouveaux comptes (le nom d’affichage, la biographie telle que saisie et telle que rendue, les champs du profil, si l’avatar ou l’en-tête a changé, l’adresse et le nom d’utilisateur du compte, et la date de création du compte ; aucune image) |
| Effacement | Lorsque la personne la supprime, par sa politique de suppression automatique, ou lors de l’effacement du compte. `{{IF auto_delete_max_age_days}}` Au plus `{{auto_delete_max_age_days}}` jours après la publication, pour les comptes sans `keep_posts_longer`. Téléversements rattachés à rien `{{24 hours}}`. Anciennes versions d’images après qu’une version plus récente les remplace `{{30 days}}`. L’enregistrement de l’URI d’une publication supprimée reste. `{{IF post review}}` Un contestation disparaît avec sa décision (`hook_decisions_retention_days`, conservé tant qu’il peut faire l’objet d’une contestation ou attend un examen) ou lors de l’effacement du compte |
| Base juridique | {{e.g. contract}} |

<!-- source: 4-federation 795a56004c37 -->
### 4. Fédération

| | |
| --- | --- |
| Finalités | Livrer l’activité locale à d’autres serveurs, et recevoir la leur |
| Personnes concernées | Les personnes ayant un compte, les personnes sur d’autres serveurs |
| Données personnelles | Envoyées : document d’acteur, publications, abonnements, blocages, boosts, mentions « j’aime », réactions, votes de sondage, citations, déplacements, commentaires de signalements transmis (sans la personne qui signale), changements de cycle de vie. Conservées : copies des activités envoyées et reçues, tentatives de livraison |
| Destinataires | Serveurs fédérés, relais, service d’examen des abonnements, service d’examen des profils |
| Effacement | Ici : reçues 30 jours et leurs documents 7 jours. Envoyées 30 jours, et la livraison d’une publication jusqu’à 30 jours après la suppression de la publication ou de son auteur. Là-bas : hors de votre contrôle. `Delete` est envoyé, et les autres serveurs sont censés le suivre mais n’y sont pas contraints |
| Base juridique | {{e.g. contract, legitimate interests}} |

<!-- source: 5-storing-data-of-people-on-other-servers 3708a84995bb -->
### 5. Stockage des données de personnes sur d’autres serveurs

| | |
| --- | --- |
| Finalités | Afficher les profils et publications distants aux utilisateurs locaux |
| Personnes concernées | Les personnes sur d’autres serveurs, qui n’ont généralement aucun lien avec vous |
| Données personnelles | Documents d’acteur, versions de profil, identifiants, publications, références de médias, copies de médias, interactions |
| Destinataires | Hébergement, CDN, utilisateurs locaux, pages publiques, `{{IF external review of posts}}` l’examen externe des publications d’autres serveurs pour les publications et révisions liées à un compte local, y compris les publications réservées aux abonnés (`private`), et les publications directes uniquement lorsqu’un compte local suit l’auteur, sauf si `hook_inbound_status_unfollowed_direct` est activé (leur texte et leurs langues, avertissement de contenu, indicateur de contenu sensible, heure de publication, adresse et URL, l’adresse de la publication à laquelle elles répondent, options de sondage, hashtags, adresses et descriptions des pièces jointes, l’auteur et son serveur, la visibilité, le nombre de comptes nommés et de comptes locaux parmi eux, et si un compte local suit l’auteur) |
| Effacement | Publications `{{90 days}}` lorsque rien ici n’y fait référence, `{{7 days}}` lorsque seul un relais les a apportées, et `{{no limit}}` pour toute autre publication. Copies de médias `{{not kept}}`. Vérifications d’identifiants 90 jours. Tout ce qui concerne un compte 7 jours après sa suppression par son serveur, ou jusqu’à 30 jours lorsque seul un `410 Gone` l’a indiqué et que le serveur ne répond plus. Son URI et son domaine restent |
| Base juridique | {{e.g. legitimate interests}} |

<!-- source: 6-notifications ad34beea6ab2 -->
### 6. Notifications

| | |
| --- | --- |
| Finalités | Informer les personnes de l’activité et de leur compte, par e-mail et Web Push |
| Personnes concernées | Les personnes ayant un compte, les personnes nommées dans une notification |
| Données personnelles | Notifications et marqueurs de lecture, paramètres de notification, adresse e-mail, texte de chaque e-mail, enregistrements de rebonds et de plaintes, adresse et clés Web Push, abonnements aux publications |
| Destinataires | Hébergement, fournisseur d’e-mail, services push (le corps du message est chiffré, et ils le conservent jusqu’à 48 heures) |
| Effacement | Lors de l’effacement du compte, ou avec la publication concernée. Web Push : aussi lorsque la personne ou le service push y met fin. Le fournisseur d’e-mail : {{ITS PERIOD}} |
| Base juridique | {{e.g. contract, legitimate interests}} |

<!-- source: 7-moderation-and-audit d72a42b6312e -->
### 7. Modération et audit

| | |
| --- | --- |
| Finalités | Traiter les signalements, les mesures et les contestations, prouver ce que l’équipe a fait, conservations pour demandes légales |
| Personnes concernées | Les personnes signalées, les personnes qui signalent, l’équipe, les personnes nommées dans un commentaire ou une note |
| Données personnelles | Signalements (commentaire, publications, règles, notes, attributions, résolutions), mesures (motif, note, publications), contestations, journal d’audit (acteur, action, nom de la cible), conservations et leur motif, publications gardées comme preuves. Peut inclure des données pénales ou des catégories particulières si un signalement ou une publication en contient |
| Destinataires | Hébergement, équipe disposant de la permission correspondante, la personne concernée à travers ses propres enregistrements de modération (jamais l’identité de la personne qui signale), le serveur du compte signalé pour un signalement transmis |
| Effacement | Signalements `{{365 days}}` après la dernière résolution, ou avec une mesure qui y fait référence. Mesures et contestations `{{365 days}}` après la fin de la mesure. Journal d’audit `{{730 days}}`. Les signalements ouverts et les actions sans fin restent. Une conservation levée disparaît après la durée applicable aux actions |
| Base juridique | {{e.g. legitimate interests, legal obligation}} |

<!-- source: 8-requests-from-people 762c1e8bc31c -->
### 8. Demandes des personnes

| | |
| --- | --- |
| Finalités | Répondre aux demandes d’accès et de portabilité, et aux imports |
| Personnes concernées | Les personnes ayant un compte |
| Données personnelles | Archives d’export (acteur, publications, propres enregistrements de modération), fichiers CSV, imports et leurs résultats, la correspondance d’une demande |
| Destinataires | Hébergement, la personne |
| Effacement | Archives et imports 7 jours. Correspondance : {{PERIOD}} |
| Base juridique | {{e.g. legal obligation, contract}} |

L’export propre à l’utilisateur ne contient ni adresses e-mail, ni adresses IP, ni notifications, ni préférences. Pour une réponse complète, utilisez `my admin account export-moderation` pour les enregistrements de modération et [l’affichage](moderation.md#account-management) pour les adresses e-mail et IP, et lisez le reste dans la base de données. L’export de modération omet l’identité des personnes qui signalent : vérifiez qu’un commentaire n’en révèle pas une avant de le remettre.

<!-- source: 9-voice-chat e0bf1be42da8 -->
### 9. Chat vocal

Supprimez cette activité lorsque `voice = off`.

| | |
| --- | --- |
| Finalités | Exploiter des salons de chat vocal, et les modérer |
| Personnes concernées | Les hôtes et les participants |
| Données personnelles | Nom du salon, description, hôtes, et manière dont le salon s’est terminé ; qui a participé, parlé, levé la main, été invité ou été retiré. Les adresses IP et l’audio transitent par le serveur multimédia vocal ou le serveur TURN, ou parviennent directement aux autres participants avec `voice = p2p`. Le serveur n’enregistre aucun audio |
| Destinataires | Hébergement, serveur multimédia vocal, STUN et TURN, participants |
| Effacement | Participants `{{90 days}}` après la fin du salon, ou aussi longtemps qu’un signalement le concernant. Les salons restent, avec le nom et la description vidés lors de l’effacement de l’auteur. Chez les services : {{THEIR PERIOD}} |
| Base juridique | {{e.g. contract, legitimate interests}} |

<!-- source: 10-supporter-payments 99d64d001780 -->
### 10. Paiements de soutien

Supprimez cette activité lorsque `payments = none`.

| | |
| --- | --- |
| Finalités | Encaisser les souscriptions, attribuer des rôles, comptabilité |
| Personnes concernées | Les souscripteurs |
| Données personnelles | Identifiants d’objets chez le fournisseur, montants, périodes, états, remboursements, rétrofacturations, le choix d’être listé. Adresse e-mail envoyée au fournisseur. Aucun numéro de carte, quatre derniers chiffres ou date d’expiration |
| Destinataires | Hébergement, fournisseur de paiement, personnes qui peuvent voir la liste des personnes soutenant le serveur, équipe disposant de la permission correspondante |
| Effacement | Accusés de réception de webhooks 45 jours. Le lien client et l’inscription sur la liste lors de l’effacement du compte. Les enregistrements de souscriptions et de paiements n’ont pas de fin automatique : {{PERIOD AND THE LAW THAT REQUIRES IT}}. Chez le fournisseur : {{ITS PERIOD, AND WHETHER YOU DELETE THE CUSTOMER}} |
| Base juridique | {{e.g. contract, legal obligation}} |

<!-- source: 11-running-the-service 9c89577f9cee -->
### 11. Exploitation du service

| | |
| --- | --- |
| Finalités | Trouver les pannes, restaurer après une défaillance, connecter vos propres outils |
| Personnes concernées | Toutes les personnes ci-dessus |
| Données personnelles | Journaux (chemins pouvant nommer un nom d’utilisateur ou une publication, texte d’erreur), journaux du proxy avec adresses IP, rapports d’erreurs nettoyés par motif, enregistrements de tâches contenant des identifiants, corps de webhooks, sauvegardes de tout |
| Destinataires | Hébergement, proxy inverse, outil de suivi des erreurs, service de métriques et de traces, destinataires de webhooks, stockage des sauvegardes |
| Effacement | Tâches échouées et enregistrements d’exécution `{{30 days}}`. Messages de webhooks 14 jours. Compteurs horaires d’erreurs 90 jours. Journaux de l’application et du proxy : {{PERIOD}}. Le dernier rapport de chaque groupe d’erreurs : remplacé par le suivant, sans fin automatique. Sauvegardes : {{PERIOD}}. Chez l’outil de suivi des erreurs et les destinataires : {{THEIR PERIOD}} |
| Base juridique | {{e.g. legitimate interests}} |

<!-- source: 12-translating-posts 17a9b7a81356 -->
### 12. Traduction des publications

Supprimez cette activité lorsque `translation_providers` est vide.

| | |
| --- | --- |
| Finalités | Traduire une publication lorsqu’un lecteur le demande, {{IF translation of reports}} traduire un signalement lorsqu’un modérateur le demande, et maintenir la traduction dans ses limites |
| Personnes concernées | Les auteurs de publications, ici et sur d’autres serveurs, et les personnes que le texte nomme. Les lecteurs, pour le nombre de caractères. {{IF translation of reports}} Les personnes qui signalent, pour le texte de leurs commentaires |
| Données personnelles | Tout ce que contient le texte d’une publication : son corps, son avertissement de contenu, ses options de sondage et ses descriptions d’images. L’adresse de la publication, son auteur, le lecteur, les mentions et les liens sont remplacés ou omis avant l’envoi. Conservés : les traductions, et les caractères par compte et par mois |
| Destinataires | Fournisseurs de traduction, en tant que vos sous-traitants : publications `public`, `local` et `unlisted` par défaut. {{IF translation to private and direct}} Aussi les publications `private` et `direct`, pour {{THE PROVIDERS YOU ALLOWED}}. {{IF translation of reports}} Les commentaires des signalements, pour les fournisseurs autorisés pour les publications `private`, et les publications que les signalements nomment, lorsqu’un modérateur le demande |
| Effacement | Traductions `{{30 days}}` après la traduction, et avec la publication lorsqu’elle est supprimée. Caractères par compte jusqu’à la fin du mois suivant, ou lors de l’effacement du compte. Chez les fournisseurs : ce qui a été envoyé ne peut pas être rappelé. {{THEIR PERIODS}} |
| Base juridique | {{e.g. legitimate interests}} |

<!-- source: security-measures-article-301g 756bb9e562e3 -->
### Mesures de sécurité (article 30, paragraphe 1, point g)

Ce que fait le logiciel :

- Les mots de passe sont stockés sous forme d’empreintes Argon2id. Les jetons de session, d’accès et à usage unique ainsi que les codes de récupération sont stockés sous forme d’empreintes.
- Les clés de signature privées, les secrets à deux facteurs et les secrets des fournisseurs et des webhooks sont chiffrés avec AES-256-GCM sous une clé de chiffrement de clés conservée hors de la base de données ([gestion des clés](key-management.md)).
- Les adresses e-mail et IP ne sont affichées qu’à l’équipe disposant de `manage_accounts`, par une action explicite que le journal d’audit consigne.
- Les demandes de connexion, d’inscription et de réinitialisation du mot de passe sont soumises à une limitation de débit. Les permissions de l’équipe passent par des rôles.
- Les requêtes sortantes que des serveurs distants peuvent influencer passent par une protection contre l’accès aux adresses internes.

Ce que vous faites :

- TLS au niveau du proxy inverse : {{CONFIGURATION}}
- Chiffrement au repos de la base de données, du stockage de blobs et des sauvegardes : {{MEASURES}}
- Qui peut accéder à l’hôte, à la base de données et à la KEK, et comment l’accès est revu : {{MEASURES}}
- Comptes de l’équipe, rôles et authentification à deux facteurs : {{POLICY}}
- Mises à jour du serveur et de son hôte : {{POLICY}}
- Gestion d’une violation de données personnelles : {{PROCEDURE AND CONTACT}}
