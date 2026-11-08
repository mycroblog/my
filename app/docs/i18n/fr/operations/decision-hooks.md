<!-- source: decision-hooks 9945608ea473 -->
# Hooks de décision

Un hook de décision demande à l’un de vos [plugins](plugins.md) s’il faut accepter quelque chose avant que le serveur ne l’accepte. Le plugin
répond par un verdict tiré d’une liste fixe. Il ne réécrit jamais rien et n’entreprend aucune action sur un compte : tout au plus peut-il
arrêter une publication, un abonnement, une inscription, un téléversement ou une modification de profil, ou déposer un signalement pour vos modérateurs. Ce sont des personnes qui décident de ce qu’il advient d’un compte.

Rien n’est examiné par défaut. Un serveur sans adresse de hook n’effectue aucun appel et n’y consacre aucun temps. Cette version conserve
les enregistrements, les modes et les écrans d’administration décrits ici. Parmi les chemins d’entrée, les publications d’autres serveurs interrogent
`inbound.status`, les abonnements d’autres serveurs interrogent `inbound.follow`, les inscriptions interrogent `signup.check`, les téléversements interrogent
`media.check`, les publications de comptes locaux interrogent `local.status`, les profils de comptes d’autres serveurs interrogent `inbound.actor`, et les
modifications de profil de comptes locaux interrogent `local.profile` ; les autres hooks ne sont pas encore appelés.

![Le chemin d’un hook de décision : une publication distante, un abonnement distant, une inscription, un téléversement, une publication locale ou le profil modifié d’un acteur distant passe par le mode du hook jusqu’à votre plugin, dont le verdict ou l’échec aboutit à une décision enregistrée. La liste ci-dessous décrit chaque étape.](images/decision-hooks.svg)

Du haut du schéma vers le bas :

1. **Ce qui est examiné, et qui attend.** Une publication d’un autre serveur (`inbound.status`), un abonnement d’un autre serveur
   (`inbound.follow`) et un téléversement (`media.check`) sont examinés dans une tâche. Une inscription (`signup.check`) est examinée pendant que
   le candidat attend, une publication d’un compte local (`local.status`) pendant que son auteur attend, et une modification du profil d’un compte
   local (`local.profile`) pendant que son titulaire attend. Un profil d’un autre serveur (`inbound.actor`) est
   examiné partout où le serveur récupère le compte : dans une tâche, et aussi pendant qu’une requête attend, comme celle d’un serveur
   dont le compte est nouveau ici.
2. **Le mode**, `hook_<hook>_mode` : `off` n’effectue aucun appel, `observe` enregistre le verdict et n’agit sur rien, et `enforce`
   agit en conséquence ([Modes](#modes)).
3. **Votre plugin** reçoit `POST /my.plugin.hook.v1.HookService/Check`. Le serveur attend jusqu’à `plugin_<hook>_timeout`, effectue
   au plus 8 appels simultanés et, après 5 échecs consécutifs, ignore le plugin pendant 30 secondes (`breaker_open`).
4. **Un verdict** : `allow` accepte le sujet, `flag` l’accepte et dépose un signalement, `mark_sensitive` l’accepte en le marquant comme
   sensible, `require_approval` retient une inscription, et `drop` et `reject` l’arrêtent et ne conservent rien (le `reject` d’une inscription uniquement avec
   [`hook_signup_check_reject`](#sign-ups)). **Un échec** est un `timeout`, une `error`, une réponse `invalid` ou `breaker_open`, et une réponse que le hook n’accepte pas
   en compte comme un. Lorsque le hook applique, `hook_<hook>_on_failure` décide : `pass` accepte le sujet sans examen, `retry`
   n’enregistre aucune décision et relance la tâche, et `require_approval` retient une inscription
   ([Lorsque le plugin échoue](#when-the-plugin-fails)).
5. **Une décision par hook et par sujet** est enregistrée, et une tâche relancée la lit au lieu d’appeler le plugin
   ([Les décisions](#the-decisions)).

<!-- source: the-hooks f1005c829ed5 -->
## Les hooks

| Hook | Ce qu’il examine | Ce qu’il peut répondre | Lorsque le plugin échoue |
| --- | --- | --- | --- |
| `inbound.status` | Publications d’autres serveurs | `allow`, `drop`, `flag`, `mark_sensitive` | `pass` (par défaut) ou `retry` |
| `inbound.follow` | Abonnements d’autres serveurs | `allow`, `drop`, `flag` | `pass` (par défaut) ou `retry` |
| `signup.check` | Inscriptions, après le CAPTCHA et la liste de blocage | `allow`, `require_approval`, `reject` | `pass` (par défaut) ou `require_approval` |
| `media.check` | Médias téléversés, avant que quiconque puisse les utiliser | `allow`, `mark_sensitive`, `flag`, `reject` | `pass` (par défaut) ou `retry` |
| `local.status` | Publications de comptes locaux, et leurs modifications | `allow`, `flag`, `reject` | `pass` |
| `inbound.actor` | Profils de comptes d’autres serveurs, lorsqu’ils changent | `allow`, `drop`, `flag` | `pass` (par défaut) ou `retry` |
| `local.profile` | Modifications de profil de comptes locaux | `allow`, `flag`, `reject` | `pass` |

Définissez `plugin_<hook>_address` pour donner un plugin à un hook, le point dans le nom devenant `_`
(`plugin_inbound_status_address`). [Plugins](plugins.md) décrit la destination, son authentification et la manière de tester un
plugin avec `my plugin test --kind inbound.status <address>`. L’appel est `POST /my.plugin.hook.v1.HookService/Check`.

Ce que fait chaque réponse :

- `drop` arrête le sujet et n’en stocke rien. Sur `inbound.actor`, il conserve le profil stocké auparavant
  ([ci-dessous](#profiles-from-other-servers)). Un `drop` erroné masque aussi silencieusement une publication à ses abonnés ; lisez donc
  en `observe` ce qu’un hook abandonnerait avant d’appliquer.
- `flag` le laisse passer et dépose un signalement par décision. Sur `inbound.follow`, le signalement concerne le compte qui a envoyé
  l’abonnement, ne nomme aucune publication et est déposé une fois l’abonnement enregistré ; son auteur, sa catégorie et son commentaire sont ceux
  du [signalement d’une publication](#what-inboundstatus-reviews). Sur `inbound.actor`, le signalement concerne le compte dont c’est le profil,
  de la même manière.
- `mark_sensitive` le laisse passer et affiche ses pièces jointes comme sensibles jusqu’à ce qu’un modérateur annule la décision.
- `require_approval` retient l’inscription pour approbation.
- `reject` le refuse, avec un texte fixe qui ne montre rien de la réponse du plugin. Sur `signup.check`, cela dépend aussi de
  [le paramètre ci-dessous](#sign-ups). Sur `media.check`, rien du téléversement n’est conservé, original compris
  ([ci-dessous](#what-mediacheck-reviews)). Sur `local.status`, son auteur peut [contester](#local-posts). Sur `local.profile`, le
  profil reste tel qu’il était et il n’y a pas de contestation ([ci-dessous](#local-profiles)).

Une réponse peut porter un motif codé (`spam`, `harassment`, `sexual`, `violence`, `illegal`, `malware`, `other`) et une note d’au plus
200 caractères pour vos modérateurs. Une réponse que le hook n’accepte pas compte comme un échec.

<!-- source: what-inboundstatus-reviews 1dcf4ccd16f9 -->
## Ce qu’examine `inbound.status`

Le serveur interroge sur chaque `Create` d’une publication et chaque `Update` d’une publication qu’il accepterait, avant de rien stocker. Chaque
révision est un sujet à part entière avec sa propre décision.

- Une publication n’est interrogée que si elle est liée à un compte local : un compte local suit son auteur, elle nomme un compte
  local, ou elle appartient à un fil comportant une publication locale. Le fait qu’un compte local suive l’auteur ne restreint pas davantage :
  le plugin reçoit `any_local_follower` et peut le prendre en compte. Une publication amenée uniquement par un relais n’est jamais interrogée, pas plus qu’un
  `Update` d’une publication stockée qui n’est plus liée à un compte local : sa révision est stockée sans question.
- Une publication directe d’un auteur qu’aucun compte local ne suit n’est pas envoyée, sauf si vous activez
  `hook_inbound_status_unfollowed_direct`. Les publications de vos comptes locaux ne sont jamais envoyées, quel que soit le paramètre.
- Les votes de sondage, les suppressions, les annulations, les mentions J’aime, les réactions et les boosts ne sont jamais interrogés ; arrêter un plugin ne peut donc pas empêcher
  une suppression d’arriver. Une récupération de fil ou la récupération d’un boost peut tout de même amener la même publication d’une autre manière.

Ce que fait chaque réponse à une publication :

- `drop` sur un `Create` ne stocke rien et ne crée ni notification ni entrée de fil, et une nouvelle remise est ignorée comme
  doublon. `drop` sur un `Update` ne crée aucune révision et conserve la précédente. Une publication abandonnée n’est pas stockée ; un
  `Update` de celle-ci publié au cours de la dernière journée est donc pris pour une nouvelle publication : le plugin est interrogé de nouveau sur cette révision, comme sur
  toute autre, et sa réponse au `Create` précédent ne se reporte pas.
- `flag` dépose un signalement par décision, avec votre acteur de serveur comme auteur, la catégorie du motif (`spam` et
  `malware` comme Spam, `illegal` comme Légal, `harassment`, `sexual` et `violence` comme Violation, et tout le reste comme Autre), et
  la note du plugin comme commentaire. Le signalement indique qu’un examen l’a déposé. Il n’est pas transmis au serveur de l’auteur
  à moins qu’un modérateur ne le [transmette](moderation.md#reports), et une transmission n’envoie jamais la note : seulement le texte que le modérateur
  écrit alors. Le résoudre ne notifie personne. Chaque décision dépose son propre
  signalement ; une vague de spam peut donc remplir la file.
- `mark_sensitive` marque la publication, pas la révision : la marque reste sur les révisions ultérieures, quelle que soit leur réponse, jusqu’à ce qu’un
  modérateur annule la décision. Elle affiche les pièces jointes comme sensibles et laisse le texte tel quel ; une publication sans pièce jointe
  n’a donc pas d’aspect différent, et les autres serveurs ne la voient pas.

<!-- source: what-mediacheck-reviews f93110fe5a83 -->
## Ce qu’examine `media.check`

Le serveur interroge sur chaque fichier qu’un compte local téléverse, quel que soit son usage : une pièce jointe, un avatar, un en-tête ou un émoji
personnalisé. Il interroge une fois, lorsque le fichier est traité pour la première fois et avant qu’il soit prêt, de sorte que personne ne peut l’utiliser pendant que le plugin
décide. Réutiliser plus tard le même fichier pour autre chose lit la décision déjà prise. Les médias distants ne sont jamais interrogés.

Il envoie la plus petite image que le serveur a produite à partir du fichier, sans ses métadonnées, et jamais l’original : pour une vidéo ou un
fichier audio, la petite image de son affiche ou de sa pochette intégrée, ou aucune image s’il n’y en a pas. L’accompagnent l’usage du fichier,
son type (image, vidéo ou audio), sa taille, sa durée de lecture et le fait qu’il bouge ou non. Le texte alternatif et
tout ce qui concerne le compte ou une publication ne sont pas envoyés. Le message `MediaSubject` du protocole `HookService` contient les champs.

Ce que fait chaque réponse à un téléversement :

- `reject` fait échouer le traitement, comme pour un fichier que le serveur ne peut pas lire : la personne qui téléverse voit que le fichier n’a pas pu être
  traité, et une publication ne peut pas le joindre. **Le serveur ne conserve rien d’un fichier rejeté.** Il supprime l’original une fois le
  traitement terminé, quelle que soit la réponse, et un fichier rejeté n’obtient aucune version traitée. Si vous devez conserver de tels
  fichiers, ou les signaler à quelqu’un, votre plugin doit le faire au moment où il répond : le serveur ne peut pas rendre le fichier.
  Annuler un `reject` ne modifie que son enregistrement.
- `mark_sensitive` marque la publication qui joint le fichier, dans la transaction qui crée la publication ou la modification qui ajoute
  le fichier, et la marque reste jusqu’à ce qu’un modérateur annule la décision, comme pour `inbound.status`. Un avatar, un en-tête ou un
  émoji n’a pas de publication ; la décision est donc seulement enregistrée.
- `flag` dépose un signalement par décision, avec votre acteur de serveur comme auteur. Pour une pièce jointe, il est déposé avec la
  publication qui joint le fichier et désigne cette publication ; un fichier qui n’est jamais publié n’en dépose aucun. Pour un avatar, un en-tête ou
  un émoji, il est déposé lorsque le fichier est traité, au sujet du compte qui l’a téléversé.

La tâche de traitement attend le plugin jusqu’à `plugin_media_check_timeout` (10 secondes par défaut). Ajouter un émoji personnalisé
d’un autre serveur ou approuver une demande d’émoji traite l’image pendant que le modérateur attend ; cette attente est donc la sienne.

<!-- source: sign-ups be7ddf1149a9 -->
## Inscriptions

`signup.check` s’exécute sur les inscriptions depuis la page d’inscription et via un fournisseur de connexion, après le CAPTCHA et la
[liste de blocage des inscriptions](moderation.md), de sorte qu’une demande que l’un des deux refuse n’atteint jamais le plugin. Une inscription avec un
code d’invitation ne l’appelle jamais. Il envoie l’adresse e-mail saisie par le candidat et l’adresse IP du client ; le compte
n’existe pas encore, la clé du sujet est donc l’ID que recevrait le compte, et un candidat qui réessaie est un nouveau sujet.

Les réponses agissent comme celles de la liste de blocage lorsque le hook applique :

- `require_approval` laisse l’inscription en attente sous **Modération** > **Inscriptions**, comme le fait un blocage de même
  gravité, dans tous les modes d’inscription. Approuvez-la ou rejetez-la là : annuler la décision ne modifie pas l’inscription.
- `reject` ne fait rien tant que vous n’activez pas `hook_signup_check_reject` (**Paramètres du serveur** > **Fédération** > **Examen externe**, désactivé par
  défaut). Tant qu’il est désactivé, un `reject` est enregistré en `observe`, puisqu’il n’arrête rien, et l’inscription se poursuit. Tant qu’il
  est activé, le candidat voit le même texte fixe que pour un blocage (« Ce serveur ne peut pas accepter cette inscription. »). Aucun compte n’est
  écrit ; annuler une telle décision ne modifie donc que son enregistrement, et le candidat s’inscrit de nouveau.

Les candidats ne voient jamais le motif ni la note d’une décision.

La requête du candidat attend le plugin jusqu’à `plugin_signup_check_timeout` (2 secondes par défaut ; gardez-le court, car
une personne attend). L’attente ne fait pas partie d’une transaction de base de données. Une inscription qui échoue ensuite, par exemple pour un nom d’utilisateur
déjà pris, conserve sa décision jusqu’à ce que la conservation la supprime.

<!-- source: profiles-from-other-servers e324990a50b1 -->
## Profils d’autres serveurs

`inbound.actor` examine le profil d’un compte d’un autre serveur à l’endroit où le serveur l’accueille : la première fois qu’il récupère
le compte, lorsque le compte envoie un `Update` de lui-même, et lorsque le serveur le récupère de nouveau, comme il le fait pour les comptes
que suivent vos utilisateurs une fois par jour. Il n’interroge que si le profil diffère de celui stocké ; récupérer de nouveau un compte avec
le même profil n’effectue donc aucun appel. Le profil qu’il examine est :

- le nom affiché, en texte brut,
- la biographie et les champs de profil, tels que le serveur les stocke (HTML assaini),
- les adresses de l’avatar et de l’image d’en-tête sur l’autre serveur. Le serveur n’envoie aucune image.

Le plugin reçoit aussi l’adresse du compte, son serveur, son nom d’utilisateur et le fait que le compte soit nouveau pour votre serveur. La
clé d’une décision est l’adresse du compte, `#`, et une empreinte du profil ; chaque profil distinct obtient donc une décision, et
le même profil envoyé de nouveau la lit.

Ce que font les verdicts :

- `drop` conserve le profil stocké auparavant : le nom affiché, la biographie, les champs, l’avatar, l’en-tête et leurs émojis
  personnalisés restent tels qu’ils étaient. Le serveur stocke tout de même le reste de ce qu’il a reçu : les clés du compte, sa boîte de réception et ses autres
  adresses, son nom d’utilisateur, et s’il est verrouillé, un bot, découvrable et indexable. Sans les clés et les adresses, le
  serveur ne pourrait ni vérifier les activités du compte ni lui remettre quoi que ce soit, et les paramètres verrouillé et indexable indiquent comment
  suivre le compte et si son titulaire autorise l’indexation. Tant que le profil reste abandonné, le récupérer de nouveau lit la
  même décision et conserve celui qui est stocké.
- Un compte récupéré pour la première fois est toujours stocké, car les abonnements et les mentions en ont besoin ; son `drop` est donc pris pour un
  `flag` : le profil est stocké et un signalement est déposé. La décision affiche toujours `drop`.
- `flag` stocke le profil et dépose un signalement sur le compte par décision.

Lorsque le plugin échoue, `pass` stocke le profil sans examen, et le serveur ne l’interroge plus à son sujet tant qu’il ne change pas.
Avec `retry`, le serveur ne stocke pas un compte dont le profil a changé pendant que le plugin échoue : un `Update` attend dans sa
tâche, ce qui retient les autres activités de ce compte comme pour [`inbound.status`](#when-the-plugin-fails), et une récupération
effectuée pendant qu’une requête attend échoue ; un serveur dont le compte est nouveau ici voit donc sa remise refusée jusqu’au retour du plugin
et jusqu’à ce qu’il la renvoie.

Annuler un `drop` de `inbound.actor` récupère de nouveau le compte auprès de son serveur, qui stocke le profil qu’il sert maintenant
sans appeler le plugin lorsque c’est celui qui avait été abandonné, et interroge à son sujet lorsqu’il a changé depuis.

<!-- source: local-posts 37ba27537191 -->
## Publications locales

`local.status` examine une publication d’un compte de votre serveur avant que le serveur ne la crée, et chaque modification de celle-ci avant que le
serveur ne la stocke. L’auteur attend le plugin ;
l’attente est donc courte : `plugin_local_status_timeout` (1 seconde par défaut). Un échec, un délai dépassé ou un disjoncteur ouvert laisse toujours passer la
publication (`pass` est son seul comportement en cas d’échec), et l’attente ne fait pas partie d’une transaction de base de données.

Ce qu’il envoie et quand :

- Uniquement les publications des comptes plus jeunes que `hook_local_status_account_age_days` (**Paramètres du serveur** > **Fédération** >
  **Examen externe**, 7 jours par défaut ; `0` envoie les publications de tous les comptes). Les nouveaux comptes sont la source de la plupart des spams, et les comptes
  établis ne sont pas envoyés.
- Uniquement les publications `public`, `local` et `unlisted`. Les publications `private` et `direct` ne sont jamais envoyées.
- Le texte tel que l’auteur l’a saisi et tel que le serveur l’affiche, l’avertissement de contenu, la langue, le fait qu’elle soit marquée
  comme sensible, les descriptions d’images, les options du sondage, les hashtags, le nombre de comptes qu’elle mentionne, l’adresse que la publication
  recevrait, la publication à laquelle elle répond ou qu’elle cite (par adresse uniquement), l’adresse et le nom d’utilisateur de l’auteur, et la date de
  création du compte. Aucun média n’est envoyé.
- La clé du sujet est l’ID que recevrait la publication. Renvoyer une publication avec le même `Idempotency-Key` renvoie la publication
  qui a été créée, sans appeler le plugin ; une nouvelle tentative est un nouveau sujet.
- Une modification est envoyée de la même manière, avec la publication telle que modifiée, aux mêmes comptes : ceux qui sont dans
  `hook_local_status_account_age_days`. Sa clé est l’ID de la publication, `#`, et l’ID de la nouvelle révision ; chaque modification est donc un
  sujet à part entière.
- Le démarrage d’un salon vocal publie sa première publication sans ce hook.

Lorsqu’il applique, `reject` refuse la publication et rien n’est stocké. L’auteur voit un texte fixe (« Cette publication n’a pas été publiée. »)
et un moyen de contester ; jamais le motif ni la note. `flag` laisse passer la publication et dépose un signalement à son sujet, contre son
auteur.

Sur une modification, `reject` refuse cette seule modification : la publication reste telle qu’elle était, et l’auteur voit un texte fixe (« Cette modification n’a pas été
enregistrée. »). Une modification ne peut pas être contestée, puisque la publication est toujours là et que vos modérateurs peuvent agir dessus comme d’habitude. `flag` stocke
la modification et dépose un signalement de la publication, et `allow` la laisse passer.

<!-- source: appeals 79991f552151 -->
### Contestations

L’auteur d’une publication refusée peut la contester dans `appeal_window_days` (le même délai que pour les mesures de modération), une fois par
décision. La contestation porte le texte de la publication, puisque le serveur n’en a rien stocké, et un commentaire facultatif pour vos
modérateurs. Aucun plugin n’est appelé.

Vos modérateurs (`manage_reports`) sont avertis de chaque nouvelle contestation (**Contestations de publications refusées** dans leurs paramètres de
notification, qu’ils peuvent aussi recevoir par e-mail ; la notification ouvre la contestation), et examinent les contestations sous **Modération** >
**Examens automatisés** > **Contestations de publications refusées**. Chaque contestation s’ouvre sur sa propre page, avec le texte envoyé par l’auteur :

- **Rejeter la contestation** laisse la décision telle quelle, et est inscrit dans le journal d’audit sous `hook_appeal_upheld`.
- **Accepter la contestation** annule la décision, ce qui est inscrit dans le journal d’audit sous `hook_decision_overridden`, et cesse d’envoyer les
  publications du compte à `local.status`, dans une seule transaction. La publication refusée n’est pas rétablie : l’auteur la publie de nouveau.

Dans les deux cas, l’auteur est averti du résultat, parmi les avis des modérateurs, et le formulaire de publication l’affiche sous
le refus. La notification et le formulaire indiquent seulement si la décision est maintenue ou a été annulée, jamais le motif ni la
note. Un client officiel lit l’état d’une contestation avec `GET /api/v1/hook_decisions/{id}/appeal`.

Un compte qu’une contestation acceptée a retiré du hook en reste exclu jusqu’à la suppression du compte. S’il est repris plus tard,
`local.status` ne l’examine plus.

<!-- source: local-profiles dbafa1545e2a -->
## Profils locaux

`local.profile` examine une modification du profil d’un compte de votre serveur avant que le serveur ne la stocke. Le titulaire attend
le plugin ; l’attente est donc courte : `plugin_local_profile_timeout` (1 seconde par défaut). Un échec, un délai dépassé ou un
disjoncteur ouvert laisse toujours passer la modification (`pass` est son seul comportement en cas d’échec), et l’attente ne fait pas partie d’une transaction
de base de données.

Ce qu’il envoie et quand :

- Uniquement les modifications des comptes plus jeunes que `hook_local_profile_account_age_days` (**Paramètres du serveur** > **Fédération** >
  **Examen externe**, 7 jours par défaut ; `0` envoie celles de tous les comptes).
- Uniquement une modification de ce que voit le hook : le nom affiché, la biographie, les champs de profil, l’avatar ou l’en-tête. Une modification
  qui ne fait que verrouiller le compte ou changer ses paramètres de découverte n’est pas envoyée.
- Le nom affiché, la biographie telle que saisie et telle que le serveur l’affiche, le nom et la valeur de chaque champ de profil (tels que saisis et
  tels qu’affichés), le fait que la modification définisse, remplace ou supprime l’avatar ou l’en-tête, l’adresse et le nom d’utilisateur du compte,
  et la date de création du compte. Aucune image n’est envoyée. Le profil est public ; le hook peut donc utiliser du TCP en clair.
- La clé du sujet est l’ID du compte, `#`, et l’ID de la révision de profil que la modification stockerait ; chaque modification
  est donc un sujet à part entière.

Lorsqu’il applique, `reject` refuse cette seule modification : le profil reste tel qu’il était, et le titulaire voit un texte fixe (« Cette
modification de votre profil n’a pas été enregistrée. »), jamais le motif ni la note. Il n’y a pas de contestation : le titulaire peut modifier le profil
et l’enregistrer de nouveau. `flag` enregistre la modification et dépose un signalement contre le compte, qui ne nomme aucune publication ; son auteur,
sa catégorie et son commentaire sont ceux du [signalement d’une publication](#what-inboundstatus-reviews).

<!-- source: modes 6133f824d202 -->
## Modes

Chaque hook a un mode, un [paramètre du serveur](instance-settings.md) (`hook_inbound_status_mode`, etc.) que vous modifiez sous
**Paramètres du serveur** > **Fédération** > **Examen externe**. Les modifications sont inscrites dans le journal d’audit sous `setting_changed`.

| Mode | Ce qu’il fait |
| --- | --- |
| `off` | N’appelle rien. |
| `observe` | Enregistre le verdict et n’agit sur rien. C’est la valeur par défaut de chaque hook. |
| `enforce` | Agit selon le verdict. |

Un hook ne passe de `off` à `enforce` qu’en passant par `observe`.

<!-- source: when-the-plugin-fails 7c987d71eb78 -->
## Lorsque le plugin échoue

Un délai dépassé, une erreur et une réponse invalide sont enregistrés comme des résultats à part entière (`timeout`, `error`, `invalid`), jamais comme des
verdicts. Après 5 appels échoués consécutifs, le serveur ignore le plugin pendant 30 secondes, puis tente un appel ; une panne
n’impose donc aucune attente à chaque sujet ; un appel ignoré est enregistré comme `breaker_open`. Chaque processus du serveur compte de son côté.

Ce qui arrive alors au sujet dépend de `hook_<hook>_on_failure` :

- `pass` le laisse passer sans examen. Du spam peut entrer pendant une panne, mais rien d’autre ne s’arrête. C’est la valeur par défaut.
- `retry` fait échouer la tâche, qui est relancée et appelle de nouveau le plugin, et n’enregistre aucune décision. Les tâches d’un même acteur s’exécutent
  dans l’ordre ; tout ce qui vient ensuite de cet acteur attend donc dans l’intervalle, suppressions comprises, pendant environ 4 heures et 15 minutes au plus.
  Après sa dernière exécution, la tâche devient une [tâche morte](running.md#the-job-queue-and-failed-dead-jobs), ce qui libère les autres.
- `require_approval` retient l’inscription pour un modérateur.

Pour `media.check`, `pass` rend le fichier prêt sans examen. `retry` garde le téléversement en cours de traitement jusqu’à ce que le plugin
réponde ou que la tâche épuise ses exécutions ; le fichier n’est jamais prêt dans l’intervalle, et une tâche morte le laisse non traité, ce que le
nettoyage des téléversements non rattachés supprime ensuite. Aucune autre tâche ne l’attend.

Chaque processus effectue au plus 8 appels simultanés à un plugin, et un appel qui attend son tour au-delà de l’échéance du hook compte comme un
délai dépassé. Les attentes apparaissent dans `my.plugin.slot_wait` ([Exploitation](running.md)). Gardez `plugin_inbound_status_timeout` court : tant qu’un appel
attend, les tâches suivantes du même acteur attendent avec lui.

En `observe`, un échec ne retient jamais rien. Les modérateurs reçoivent une notification une fois par jour et par hook lorsque ses appels échouent de façon répétée.
Les métriques sont `my.hook.decisions`, `my.hook.failures` et `my.hook.duration` ([Exploitation](running.md)).

<!-- source: the-decisions 4d41f7512468 -->
## Les décisions

Le serveur enregistre une décision par hook et par sujet ; une tâche relancée la lit donc et n’appelle pas de nouveau le plugin.
Une décision contient le hook, la clé du sujet, le mode, le résultat, le motif, la note et la durée de l’appel. Elle
ne contient ni texte de publication, ni adresse e-mail, ni adresse IP.

**Modération** > **Examens automatisés** (`manage_reports`) affiche les comptes et les attentes des 7 derniers jours, ainsi que ce que les hooks
ont arrêté, auraient arrêté en `observe` ou n’ont pas pu vérifier. Chaque décision s’ouvre sur sa propre page avec son motif, sa note et son
attente. Annuler une décision, depuis la liste ou sa page, est inscrit dans le journal d’audit sous `hook_decision_overridden`. Un `reject` de `local.status` n’est pas annulé là : son auteur [le conteste](#appeals).

Ce que fait l’annulation dépend de la réponse. Un `mark_sensitive` perd sa marque, et une marque issue d’une mesure de modération sur la même
publication reste. Un `flag` conserve son signalement, que vous résolvez comme d’habitude. Un `drop` de `inbound.status` traite de nouveau la publication reçue
sans interroger le plugin, aussi longtemps que le serveur conserve l’activité reçue (7 jours) ; ensuite, l’écran
l’indique et l’annulation ne fait qu’enregistrer l’annulation.

Annuler un `drop` de `inbound.follow` rejoue l’abonnement, sans appeler le plugin, aussi longtemps que le serveur conserve
l’activité reçue (7 jours). Ensuite, l’abonnement ne peut plus revenir et l’annulation ne fait qu’être consignée ; l’
autre serveur doit envoyer un nouvel abonnement.

Un abonnement n’interroge le hook que s’il serait enregistré comme un nouvel abonnement ou une nouvelle demande d’abonnement. Retirer un abonnement (`Undo`) n’interroge jamais,
pas plus qu’un abonnement que le serveur refuse de toute façon : celui que le mode de fédération refuse, celui d’un compte déplacé ou
désactivé, celui de l’acteur du serveur, ou celui entre des comptes qui se bloquent mutuellement, qui reçoit son rejet comme
d’habitude. Un abonnement renvoyé pour un abonnement ou une demande qui existe déjà n’interroge pas non plus : le serveur y répond comme d’habitude,
et l’accepte de nouveau lorsque l’abonnement est déjà accepté. Un abonnement abandonné ne reçoit aucune réponse, de sorte que l’autre serveur continue de
l’afficher comme demandé. Renvoyer la même activité est ignoré comme doublon.

Un effet automatisé est inscrit dans le journal d’audit, avec le système comme acteur, uniquement lorsqu’il laisse un état : `mark_sensitive`, un
`flag` déposé et `require_approval` (`hook_decision_applied`). `drop` et `reject` ne stockent rien et restent donc dans les
décisions seules.

La tâche `hook.purge_decisions` supprime les décisions après `hook_decisions_retention_days` jours (90 par défaut, au moins 30). Elle conserve
celles sur lesquelles repose une marque sensible d’une publication existante, celles d’un compte sous
[conservation](moderation.md#record-retention-and-holds), et chaque `reject` d’une nouvelle publication par `local.status` que son auteur peut
encore contester ou dont la contestation attend d’être examinée. Un `reject` d’une modification ne peut pas être contesté ; il disparaît donc une fois la période de conservation
écoulée. Une contestation disparaît avec sa décision. Supprimer un compte supprime ses décisions, ses contestations et
son exemption de `local.status`.

<!-- source: what-you-tell-your-users 2ab3b8f14096 -->
## Ce que vous dites à vos utilisateurs

Tant qu’un hook a un plugin et n’est pas `off`, votre page d’informations (`/about`) indique qu’un examen externe est utilisé, ce qu’il
couvre et ce qui lui est envoyé. Le serveur l’écrit à partir de vos paramètres, y compris l’ancienneté de compte de
`hook_local_status_account_age_days` et `hook_local_profile_account_age_days` (ou le fait que ceux de tous les comptes partent, à
`0`) et si
`hook_inbound_status_unfollowed_direct` envoie les publications directes d’auteurs que personne ici ne suit.

<!-- source: what-is-sent-and-what-it-costs fd966d9abd0d -->
## Ce qui est envoyé, et ce que cela coûte

Les appels, leur coût et les données envoyées croissent avec le volume que le serveur reçoit, sur `inbound.status` surtout : chaque publication dans le périmètre
est un appel, et chaque révision un autre. Examinez ce que vous envoyez avant d’activer un hook :

- `inbound.status` et `local.status` envoient le texte des publications et refusent le TCP en clair, y compris en bouclage. `inbound.status` envoie
  ce que le serveur distant a envoyé : le texte dans chacune de ses langues, l’avertissement de contenu, le fait qu’il soit marqué comme sensible, la date de
  publication, l’adresse et l’URL de la publication, l’adresse de la publication à laquelle elle répond, les adresses et descriptions des
  pièces jointes (pas de média), les options du sondage, les hashtags, l’auteur et le serveur de l’auteur, la visibilité, et le nombre de comptes
  que la publication nomme et combien d’entre eux sont locaux. Il envoie aussi si un compte local suit l’auteur
  (`any_local_follower`), ce qui est une donnée sur vos utilisateurs. Les publications réservées aux abonnés (`private`) sont envoyées comme les publiques, et les
  publications directes comme [ci-dessus](#what-inboundstatus-reviews). Il n’envoie rien de la publication à laquelle elle répond hormis son adresse.
  `local.status` envoie les champs listés sous [Publications locales](#local-posts). Le message `InboundStatus` du protocole `HookService` contient les champs.
- `signup.check` envoie l’adresse e-mail et l’adresse IP de chaque candidat, et refuse le TCP en clair.
- `inbound.follow` envoie les deux parties d’un abonnement.
- `inbound.actor` envoie le profil public d’un compte d’un autre serveur, tel que listé sous
  [Profils d’autres serveurs](#profiles-from-other-servers), et peut utiliser du TCP en clair. Il est appelé chaque fois qu’un tel profil
  change ; son volume suit donc le nombre de comptes que votre serveur connaît.
- `media.check` envoie la plus petite image traitée de chaque téléversement, jamais l’original, avec l’usage du fichier, son
  type, sa taille et sa durée. Il peut utiliser du TCP en clair, car il n’envoie aucun texte : une image traitée peut tout de même montrer une personne.
- `local.profile` envoie le profil public listé sous [Profils locaux](#local-profiles), et aucune image. Il peut utiliser du TCP
  en clair.
