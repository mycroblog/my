<!-- source: run-voice-chat 54808e78eb0d -->
# Exploiter le chat vocal

Le chat vocal permet aux utilisateurs connectés d’ouvrir un salon vocal, d’y inviter d’autres personnes et de parler. Il est désactivé tant que vous n’avez pas choisi comment l’audio circule.
Ce guide couvre ce choix, les serveurs STUN et TURN ou le serveur média dont chaque choix a besoin, ainsi que ce qu’il faut savoir sur le déploiement, la confidentialité et la modération.
Chaque paramètre nommé ici est décrit dans la [référence de la configuration](../reference/configuration.md#voice).

<!-- source: choose-how-the-audio-travels 9d60bd0655e9 -->
## Choisir comment l’audio circule

`voice` choisit une méthode pour toute l’instance ; chaque salon vocal l’utilise.

| `voice` | Ce qui transporte l’audio | Convient pour |
| --- | --- | --- |
| `off` | Rien. L’API du chat vocal répond `404`, le client web n’affiche aucun chat vocal, et les autres paramètres `voice_*` ne sont pas vérifiés au démarrage. | La valeur par défaut. |
| `p2p` | Les navigateurs des participants s’envoient l’audio directement. Le serveur ne relaie que les messages qui établissent les connexions. | Les petits salons vocaux et la configuration la moins chère : aucun serveur supplémentaire, hormis un serveur STUN ou TURN facultatif. |
| `livekit` | Un serveur LiveKit, auto-hébergé ou LiveKit Cloud. | Les salons vocaux plus grands, et un contrôle strict de qui peut parler. |
| `sora` | Un serveur Sora, auto-hébergé ou Sora Cloud. | Idem, si vous exploitez déjà Sora. |

![Les trois méthodes côte à côte. Avec p2p, le serveur relaie les signaux et les navigateurs s’envoient l’audio entre eux, ou via un serveur TURN. Avec livekit et sora, les navigateurs envoient les signaux et l’audio au serveur média, et le serveur communique avec ce serveur média par son API et ses webhooks. La liste ci-dessous décrit chaque méthode.](images/voice-methods.svg)

Dans toutes les méthodes, le serveur (`my serve`) détient les salons vocaux, les personnes qui y participent et leurs sessions, et remet à chaque
participant une autorisation : ce dont le navigateur a besoin pour se connecter. Il ne transporte jamais l’audio. Ce qui diffère, c’est l’endroit où passent la signalisation
(les messages qui établissent une connexion) et l’audio :

- **`p2p`.** Un navigateur envoie ses signaux (offres, réponses et candidats ICE) au serveur, qui les transmet à
  l’autre navigateur par le flux d’événements du salon vocal ; un signal attend dans le magasin clé-valeur pendant 30 secondes au plus. L’audio
  va directement d’un navigateur à l’autre. L’autorisation liste les serveurs STUN de `voice_stun_urls` et le serveur TURN de
  `voice_turn` (`coturn` ou `cloudflare`) avec des identifiants de courte durée. Un navigateur apprend son adresse publique grâce à STUN,
  et le serveur TURN relaie l’audio quand les deux navigateurs ne trouvent aucun chemin direct.
- **`livekit`.** L’autorisation contient `voice_livekit_url` et un jeton d’accès de 60 secondes. Le navigateur envoie ses signaux et son
  audio au serveur LiveKit, qui apporte ses propres STUN et TURN et transmet l’audio aux autres participants (une unité de transfert sélectif,
  ou SFU).
  Le serveur crée les salons vocaux LiveKit et change qui peut parler via l’API de LiveKit, et LiveKit signale les arrivées et
  les départs au serveur par des webhooks.
- **`sora`.** L’autorisation contient `voice_sora_signaling_urls` et une valeur d’adhésion signée. Le navigateur envoie ses signaux et son
  audio au serveur Sora, qui apporte ses propres STUN et TURN. Sora interroge le webhook d’authentification du serveur avant chaque
  connexion, le serveur contrôle les salons vocaux via l’API de Sora, et Sora signale les connexions par des webhooks.

Remarques sur le choix :

- Avec `p2p`, l’audio ne passe jamais par ce serveur (il passe par le serveur TURN quand on en utilise un). La charge du serveur se limite aux messages des salons vocaux, aux heartbeats et aux flux.
- Une petite instance est mieux servie par `p2p`, ou par la formule gratuite Build de LiveKit Cloud (voir [LiveKit Cloud](#livekit-cloud)).
- Avec `livekit` ou `sora`, les paramètres STUN et TURN et la politique ICE ci-dessous ne sont pas utilisés (le serveur avertit au démarrage si vous les définissez), car le serveur média apporte ses propres serveurs. Les adresses IP des participants ne sont alors visibles que du serveur média, pas des autres participants.
- Les build tags `no_voice_livekit` et `no_voice_sora` excluent d’un binaire les deux méthodes à serveur média ([build tags](../reference/build-tags.md)).
- Servez l’instance en HTTPS : les navigateurs n’autorisent l’accès au microphone que sur les pages sécurisées.

Les limites bornent le coût d’un serveur média payant. `0` supprime n’importe laquelle d’entre elles, sauf la dernière :

| Paramètre | Par défaut | Signification |
| --- | --- | --- |
| `voice_max_room_duration` | `4h` | La durée maximale pendant laquelle un salon vocal reste ouvert. Un salon vocal qui l’atteint est terminé. |
| `voice_max_open_rooms` | `10` | Combien de salons vocaux peuvent être ouverts en même temps sur ce serveur. |
| `voice_sfu_max_participants` | `10` | La taille du salon vocal pour `livekit` et `sora`. |
| `voice_p2p_max_participants` | `10` | La taille du salon vocal pour `p2p`, de 2 à 50. |

Qui peut ouvrir des salons vocaux relève du paramètre d’instance `voice_room_starters` ([paramètres d’instance](instance-settings.md)) : par défaut, seuls les titulaires de la permission `start_voice_rooms` (le rôle intégré `admin` l’a) ; `everyone` permet à tout utilisateur local connecté, qui n’est ni suspendu ni limité, d’ouvrir des salons vocaux.

<!-- source: peer-to-peer-rooms 2199ca9a427c -->
## Salons pair à pair

<!-- source: try-it-with-docker-compose ac3681681768 -->
### Essayer avec Docker Compose

Pour essayer le chat vocal sur votre machine avec la configuration Docker Compose ([`deploy/docker/compose.yaml`](../../deploy/docker/compose.yaml)),
définissez `MY_VOICE=p2p` dans `.env` et recréez le serveur.

```sh
cd deploy/docker
docker compose up -d
```

- Ouvrez l’instance sur `http://localhost:8080` (le `MY_WEB_ORIGIN` de `example.env`). Les navigateurs ne laissent une page utiliser le microphone que sur
  `localhost` ou en HTTPS : une adresse comme `http://192.168.1.10:8080` ne permet donc pas de parler.
- Les navigateurs d’une même machine (deux profils de navigateur, ou une fenêtre normale et une fenêtre privée) se connectent entre eux sans serveur STUN ni TURN.
  Pour joindre des navigateurs d’autres machines, placez l’instance derrière HTTPS et définissez `MY_VOICE_STUN_URLS` dans `.env`.
- Par défaut, seul le rôle `admin` peut ouvrir des salons vocaux. Connectez-vous avec l’administrateur que vous avez créé, ou définissez `voice_room_starters` sur `everyone`
  dans la page « Chat vocal » des paramètres du serveur pour que tous les utilisateurs puissent ouvrir des salons vocaux.

<!-- source: size-and-bandwidth 9872bf829e26 -->
### Taille et bande passante

Deux navigateurs se connectent l’un à l’autre quand au moins l’un d’eux parle ; deux personnes qui ne font qu’écouter ne se connectent pas.
Une personne qui parle envoie donc une copie distincte de son audio à chaque autre participant. La parole en Opus prend environ 30 à 50 kbit/s avec la surcharge : une personne qui parle dans un salon vocal de 10 envoie donc environ 9 copies, quelques centaines de kbit/s,
et une personne qui parle dans un salon vocal de 50 envoie 49 copies, soit environ 2 Mbit/s, ce qu’un téléphone en données mobiles ou une connexion domestique peut ne pas soutenir. Le nombre de connexions dans un salon vocal de N personnes est au plus N(N-1)/2 : 45 pour 10 et 1 225 pour 50.
C’est pourquoi la valeur par défaut est 10. N’augmentez `voice_p2p_max_participants` que si vos utilisateurs disposent de la bande passante montante nécessaire ; utilisez `livekit` ou `sora` pour les grands salons vocaux.

<!-- source: stun-and-turn 51e427976a77 -->
### STUN et TURN

Un navigateur derrière un NAT a besoin d’un serveur STUN pour apprendre son adresse publique, et d’un serveur TURN pour relayer l’audio quand il n’existe aucun chemin direct (NAT stricts, pare-feu d’entreprise, certains réseaux mobiles).

- `voice_stun_urls` : des URL `stun:` ou `stuns:` séparées par des virgules. Tout serveur STUN public convient, et le serveur coturn ci-dessous répond aussi à STUN.
- `voice_turn` : `none`, `coturn` ou `cloudflare`.

Avec ni l’un ni l’autre, le serveur avertit au démarrage : seuls les navigateurs d’un même réseau se connectent. Une instance publique veut STUN, et TURN pour les utilisateurs qui ne peuvent pas se connecter directement.

Le serveur remet à chaque participant des identifiants TURN de courte durée, jamais un mot de passe partagé. `voice_turn_credential_ttl` (par défaut `6h`) fixe leur durée. Le client web demande de nouveaux identifiants dix minutes avant leur expiration et relance ICE : un appel ne s’arrête donc pas quand ils expirent.

<!-- source: coturn d665e942c91b -->
#### coturn

Définissez `voice_turn = coturn`, `voice_turn_coturn_urls` et `voice_turn_coturn_secret`. Le serveur calcule lui-même les identifiants (le nom d’utilisateur est l’heure d’expiration et l’ID de session, le mot de passe en est un HMAC-SHA1 avec le secret) : il n’adresse donc aucune requête au serveur TURN.
Configurez coturn en conséquence, dans `turnserver.conf` :

```conf
# Time-limited credentials signed with a shared secret.
use-auth-secret
static-auth-secret=<the same value as voice_turn_coturn_secret>
realm=turn.example.org

listening-port=3478
tls-listening-port=5349
# If the host is behind NAT (most cloud VMs), tell coturn its public address.
external-ip=203.0.113.10

# Do not let the relay reach your internal networks.
no-multicast-peers
denied-peer-ip=0.0.0.0-0.255.255.255
denied-peer-ip=10.0.0.0-10.255.255.255
denied-peer-ip=100.64.0.0-100.127.255.255
denied-peer-ip=127.0.0.0-127.255.255.255
denied-peer-ip=169.254.0.0-169.254.255.255
denied-peer-ip=172.16.0.0-172.31.255.255
denied-peer-ip=192.168.0.0-192.168.255.255
```

- `denied-peer-ip` est important. Sans lui, toute personne détenant un identifiant peut utiliser le serveur TURN pour atteindre des hôtes de son réseau interne (services de métadonnées du cloud, bases de données, autres VM). Refusez toutes les plages privées, de bouclage et de lien local ; n’autorisez une adresse interne (`allowed-peer-ip`) que si vous avez vraiment besoin de relayer vers elle.
- Ouvrez UDP et TCP 3478, TCP 5349 pour `turns:`, ainsi que la plage de ports de relais de coturn (`min-port` / `max-port`, par défaut 49152-65535).
- Listez aussi dans `voice_turn_coturn_urls` une URL `turns:` ou une URL avec `?transport=tcp`, afin que les utilisateurs de réseaux qui bloquent UDP puissent tout de même se connecter.
- coturn ne vérifie l’expiration d’un identifiant qu’à la première création d’une allocation. Une allocation qui existe déjà continue de fonctionner au-delà, et les nouveaux identifiants prennent effet pour l’allocation suivante, que le client web crée en relançant ICE.

<!-- source: cloudflare-turn be5846573be8 -->
#### Cloudflare TURN

Définissez `voice_turn = cloudflare`, `voice_turn_cloudflare_key_id` et `voice_turn_cloudflare_api_token`, à partir d’une clé TURN créée dans le tableau de bord de Cloudflare. Le serveur demande à Cloudflare les identifiants de chaque participant quand il ouvre ou rejoint un salon vocal (si cela échoue, la requête répond `503` et n’ouvre ni ne rejoint rien, de sorte qu’on peut réessayer) et les révoque quand le participant part ou est retiré.

- Les identifiants durent au plus 48 heures : `voice_turn_credential_ttl` ne peut donc pas dépasser `48h` (sinon le serveur refuse de démarrer).
- **Cloudflare coupe un relais peu après l’expiration de ses identifiants.** Le client web les renouvelle avant, gardez donc `voice_turn_credential_ttl` bien au-dessus de dix minutes.
- Tarifs, tels que vérifiés le 2026-10-26 : les premiers 1 000 Go par mois sont gratuits, puis 0,05 $ par Go envoyé aux clients. Vérifiez les tarifs actuels de Cloudflare avant de vous y fier.

<!-- source: hiding-ip-addresses de67ffa9e7b7 -->
### Masquer les adresses IP

Avec `voice_ice_transport_policy = all` (la valeur par défaut), les navigateurs se connectent directement quand ils le peuvent : les participants d’un salon vocal peuvent donc apprendre mutuellement leurs adresses IP.

Avec `relay`, les navigateurs n’utilisent que le serveur TURN : un participant voit l’adresse du serveur TURN et non celles des autres participants. L’audio reste chiffré entre les navigateurs. Ce que `relay` ne masque pas : la personne qui exploite le serveur TURN (vous, ou Cloudflare) voit l’adresse de chaque participant, et ce serveur voit l’adresse de quiconque appelle son API.
`relay` exige que `voice_turn` soit `coturn` ou `cloudflare` (sinon le serveur refuse de démarrer), et fait passer chaque appel par le serveur TURN, ce qui représente votre bande passante avec coturn et votre facture de transfert avec Cloudflare.

<!-- source: what-p2p-cannot-enforce 31e06affcd4f -->
### Ce que le P2P ne peut pas imposer

Seuls certains participants peuvent parler dans un salon vocal : les hôtes et les auditeurs qu’un hôte y autorise. En `p2p`, cela est imposé à deux endroits : le serveur refuse de relayer une offre de connexion qui enverrait de l’audio depuis quelqu’un qui n’a pas le droit de parler, et les autres navigateurs ne lisent pas l’audio d’un participant qui n’est pas intervenant.

Cela arrête les utilisateurs ordinaires et le client web officiel, mais ce n’est pas une défense complète. **Deux participants avec des clients modifiés qui coopèrent peuvent tout de même échanger de l’audio** : un client modifié de quelqu’un qui n’a pas le droit de parler peut l’envoyer, et un client modifié d’un destinataire peut le lire, et le serveur ne peut pas voir à l’intérieur des médias chiffrés.
Si vous devez garantir qui parle, par exemple pour des événements publics modérés, utilisez `livekit` ou `sora`, où le serveur média lui-même abandonne l’audio de quiconque n’en a pas la permission.

<!-- source: deployment-notes b0f03da306b7 -->
## Notes de déploiement

- **Deux flux par utilisateur** : pendant un appel, un participant tient le flux d’événements propre au salon vocal (Server-Sent Events) en plus du flux que l’application ouvre toujours. Dimensionnez les limites de connexions pour deux requêtes de longue durée par utilisateur en appel.
- **Plusieurs processus** : utilisez un adaptateur de flux partagé (`stream` défini sur `postgres`, `valkey`, `sns` ou `pubsub`), comme pour le reste de l’application. Avec `stream = hub` et un `role` autre que `all`, le serveur avertit au démarrage que le chat vocal ne fonctionnera pas entre processus.
- **Cloud Run** : un flux est une requête, et le CPU est alloué tant qu’elle est ouverte. Quand le délai d’expiration de la requête la coupe, le client se reconnecte. Réglez le délai aussi long que possible.
- **Les serveurs média ont besoin d’une VM** : LiveKit et Sora reçoivent l’audio en UDP dans un processus de longue durée. Cloud Run n’accepte que des requêtes HTTP et ne peut donc pas les héberger. Exécutez-les (ainsi que coturn) sur une VM, ou utilisez LiveKit Cloud, Sora Cloud ou Cloudflare TURN.

<!-- source: livekit b16c1d192419 -->
## LiveKit

Définissez `voice = livekit` et :

| Paramètre | Valeur |
| --- | --- |
| `voice_livekit_url` | L’URL `wss:` à laquelle les navigateurs se connectent, par exemple `wss://livekit.example.org`. |
| `voice_livekit_api_url` | L’URL `https:` que ce serveur appelle, quand elle diffère de la précédente. Par défaut, c’est le même hôte avec `https:`. |
| `voice_livekit_api_key`, `voice_livekit_api_secret` | Une paire de clés du serveur LiveKit. Le secret signe les jetons d’accès des participants et vérifie les webhooks. |

Le serveur crée chaque salon vocal sur LiveKit, le nomme avec l’ID du salon vocal, et donne à chaque participant un jeton d’accès de 60 secondes. Le jeton d’un auditeur ne peut pas publier ; celui d’un intervenant peut publier un microphone uniquement.

<!-- source: self-hosted-livekit 2b118136f4dc -->
### LiveKit auto-hébergé

Dans `livekit.yaml` :

```yaml
keys:
  <api key>: <api secret>
room:
  # Rooms are created only by this server.
  auto_create: false
  # Audio only.
  enabled_codecs:
    - mime: audio/opus
    - mime: audio/red
webhook:
  api_key: <api key>
  urls:
    - https://<your domain>/voice/webhooks/livekit
```

- Le serveur LiveKit doit atteindre l’URL du webhook en HTTPS. Le serveur vérifie la signature de chaque webhook avec le secret d’API.
- Les webhooks ne font qu’accélérer les choses. Chaque minute, le serveur compare les participants de chaque salon vocal ouvert avec ce que LiveKit signale et retire toute personne qui ne devrait pas s’y trouver : un webhook perdu pendant que l’instance était froide est donc rattrapé.
- Pour l’essayer sur votre machine, `livekit-server --dev` démarre avec une paire de clés de développement fixe.
- **Un participant retiré peut se reconnecter pendant un court moment.** LiveKit auto-hébergé ne peut pas révoquer un jeton : il continue d’envoyer de nouveaux jetons aux clients connectés, chacun valable jusqu’à environ 10 minutes. Une personne retirée peut donc se reconnecter avec son dernier jeton pendant environ 10 minutes au plus, jusqu’à ce que le serveur s’en aperçoive (le webhook d’arrivée ou la vérification à la minute) et la coupe, et elle peut être entendue pendant quelques secondes à une minute entre-temps. Utilisez LiveKit Cloud si le retrait doit être immédiat : il révoque le jeton aussitôt.

<!-- source: livekit-cloud 5bd9f833be11 -->
### LiveKit Cloud

Utilisez l’URL WebSocket, la clé d’API et le secret du projet, et ajoutez `https://<your domain>/voice/webhooks/livekit` comme point de terminaison de webhook dans les paramètres du projet.
La formule gratuite Build offre 5 000 minutes et 50 Go par mois et 100 connexions simultanées, telles que vérifiées le 2026-10-26 ; consultez la page de tarifs de LiveKit pour les formules actuelles.

<!-- source: sora 5e9209dd3baa -->
## Sora

**L’adaptateur Sora n’est testé que contre un Sora simulé qui suit la documentation de Sora, pas contre un vrai serveur Sora.** Essayez-le sur une instance de préproduction avec un vrai Sora avant d’en dépendre.

Définissez `voice = sora`, `voice_sora_signaling_urls` (des URL `wss:` séparées par des virgules auxquelles les navigateurs se connectent), `voice_sora_join_secret` (au moins 32 octets, par exemple issus de `openssl rand -base64 32` ; il signe les valeurs avec lesquelles les participants rejoignent) et `voice_sora_mode` (`self_hosted` ou `cloud`).
L’instance doit être servie en HTTPS, car Sora rappelle `https://<your domain>`.

<!-- source: webhooks 11b2c486a476 -->
### Webhooks

Sora appelle deux points de terminaison de ce serveur :

| Point de terminaison | Rôle |
| --- | --- |
| `/voice/webhooks/sora/auth` | Le webhook d’authentification. Sora l’appelle avant chaque connexion. Le serveur ne l’autorise que pour un participant qui est dans le salon vocal, non retiré, autorisé à parler s’il se connecte comme intervenant, et dans la limite de taille, et il indique à Sora de n’accepter que l’audio. |
| `/voice/webhooks/sora/events` | Les webhooks d’événements et de sessions (connexions créées et détruites, sessions terminées). |

**Le webhook d’authentification doit répondre à temps.** Sora attend 5 secondes en auto-hébergé et 10 secondes sur Sora Cloud. Le serveur répond à partir de ses propres données sans rien appeler d’autre, mais un démarrage à froid de l’instance (Cloud Run qui monte depuis zéro) peut dépasser ce délai, et la connexion est alors refusée. Gardez une instance chaude tant que le chat vocal est utilisé.

En auto-hébergement, définissez `default_duplicate_client_id = evict` dans `sora.conf`. Un auditeur qui devient intervenant se reconnecte sous la même identité (Sora ne peut pas changer le rôle d’une connexion), et `evict` fait que Sora abandonne l’ancienne connexion à l’arrivée de la nouvelle.
Avec la valeur par défaut `allow`, le serveur coupe lui-même l’ancienne connexion dès qu’il apprend l’existence de la nouvelle : les deux coexistent donc un instant.

<!-- source: self-hosted-sora dd3244368b73 -->
### Sora auto-hébergé

| Paramètre | Valeur |
| --- | --- |
| `voice_sora_mode` | `self_hosted` |
| `voice_sora_api_url` | L’URL `https:` d’un reverse proxy devant l’API HTTP de Sora. |
| `voice_sora_api_header_name`, `voice_sora_api_header_value` | L’en-tête avec lequel le proxy authentifie ce serveur, par exemple `Authorization` et `Basic ...`. |
| `voice_sora_webhook_username`, `voice_sora_webhook_password` | Les identifiants Basic des webhooks. Obligatoires. |

**L’API HTTP propre de Sora n’a aucune authentification** et est prévue pour n’écouter que sur l’adresse de bouclage. Ne l’exposez jamais. Placez devant elle un reverse proxy qui exige un identifiant, et donnez cet identifiant à ce serveur via les deux paramètres d’en-tête.
Configurez les URL des webhooks dans Sora avec les mêmes identifiants Basic. Le serveur n’accepte qu’une URL d’API en `https:` (`http:` uniquement pour localhost) et ne démarre pas sans les identifiants des webhooks. mTLS n’est pas pris en charge, car Cloud Run et l’équilibreur de charge devant ECS terminent TLS avant le serveur.

<!-- source: sora-cloud ac4935c7f7a7 -->
### Sora Cloud

Définissez `voice_sora_mode = cloud`, `voice_sora_cloud_project_id` et `voice_sora_cloud_api_key`. Le serveur signe les jetons d’accès avec la clé, appelle l’API avec elle et vérifie avec elle les signatures des webhooks ; aucun identifiant Basic n’est nécessaire.

<!-- source: behavior-to-expect 03fd5e1655fa -->
### Comportement à prévoir

- Un auditeur autorisé à parler se reconnecte automatiquement comme intervenant : il y a donc un court intervalle pendant que la nouvelle connexion démarre.
- Un intervenant dont le droit est retiré est réduit au silence aussitôt par un filtre de transfert, puis reconnecté comme auditeur.

<!-- source: licensing 534e304817e3 -->
### Licence

Sora est un produit commercial de Shiguredo. Tel que vérifié le 2026-10-26 : Sora Cloud a des frais de base de 5 000 ¥ par mois et aucune période gratuite ; l’auto-hébergement exige une licence par forfait, payée à l’avance pour un nombre de connexions simultanées et une période. La page du produit indique qu’il n’est pas proposé aux entreprises, organisations ou particuliers hors du Japon : vérifiez donc que vous pouvez l’acheter avant de bâtir vos plans dessus. LiveKit est open source quand il est auto-hébergé.

<!-- source: ending-rooms-and-records b4d3d196ecd7 -->
## Terminer les salons vocaux et les enregistrements

- Un modérateur qui détient `manage_reports` peut terminer un salon vocal ouvert pour tout le monde, même quand le chat vocal est désactivé ([modération](moderation.md#voice-chat-rooms)).
- Les personnes qui ont participé à un salon vocal (participations, intervenants et auditeurs, mains levées, invitations et retraits) sont conservées pendant `voice_participants_retention_days` jours après la fin du salon vocal (90 par défaut, 30 au minimum), ou aussi longtemps qu’un signalement pointe vers le salon vocal. Le nom du salon vocal, sa description, ses hôtes et la manière dont il s’est terminé sont conservés. L’audio, l’état du microphone et les réactions ne sont jamais enregistrés.
- `voice_public_summary` contrôle si les visiteurs qui ne peuvent pas rejoindre un salon vocal `local` voient son nom et s’il est ouvert, sur sa page et dans son aperçu de lien ([paramètres d’instance](instance-settings.md)).

<!-- source: notes-for-users f44150f15915 -->
## Notes pour les utilisateurs

- Sur un iPhone ou un iPad, l’envoi de l’audio peut s’arrêter quand l’application est en arrière-plan, car iOS peut suspendre une page web qui n’est pas au premier plan. Gardez l’application ouverte à l’écran pendant un appel.
- Le navigateur demande l’autorisation d’utiliser le microphone la première fois. Si vous l’avez refusée, autorisez-la de nouveau dans les paramètres du site du navigateur.
- Dans un salon vocal pair à pair, les autres participants peuvent être en mesure de découvrir votre adresse IP, sauf si l’instance fait passer les appels par un relais. Demandez à l’opérateur si cela compte pour vous.
