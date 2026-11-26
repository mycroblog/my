<!-- source: run-voice-chat f44427fbbe76 -->
# Exploiter le chat vocal

Le chat vocal permet aux utilisateurs connectés d’ouvrir un salon vocal, d’y inviter d’autres personnes et de parler. Il est désactivé tant que vous n’avez pas choisi comment l’audio circule.
Ce guide couvre ce choix, les serveurs STUN et TURN ou les nœuds média dont chaque choix a besoin, ainsi que ce qu’il faut savoir sur le déploiement, la confidentialité et la modération.
Chaque paramètre nommé ici est décrit dans la [référence de la configuration](../reference/configuration.md#voice).

<!-- source: choose-how-the-audio-travels 2ef72c636c49 -->
## Choisir comment l’audio circule

`voice` choisit une méthode pour toute l’instance ; chaque salon l’utilise.

| `voice` | Ce qui transporte l’audio | Convient à |
| --- | --- | --- |
| `off` | Rien. L’API du chat vocal répond `404`, le client web n’affiche aucun chat vocal, et les autres réglages `voice_*` ne sont pas vérifiés au démarrage. | La valeur par défaut. |
| `p2p` | Les navigateurs des participants s’envoient l’audio directement. Le serveur ne relaie que les messages qui établissent les connexions. | Les petits salons et l’installation la moins coûteuse : aucun serveur supplémentaire, hormis un serveur STUN ou TURN facultatif. |
| `sfu` | Les nœuds média du serveur lui-même : chaque navigateur envoie son audio au nœud qui détient son salon, lequel transmet l’audio des intervenants aux participants. | Les grands salons, et un contrôle strict de qui peut parler. |

![Les deux méthodes côte à côte. Avec p2p, le serveur relaie les signaux et les navigateurs s’envoient l’audio, ou le font passer par un serveur TURN. Avec sfu, les navigateurs obtiennent leurs autorisations du serveur et envoient les signaux et l’audio au nœud média de leur salon. La liste ci-dessous décrit chaque méthode.](images/voice-methods.svg)

Dans chaque méthode, le serveur (`my serve`) détient les salons, les personnes qui y participent et leurs sessions, et remet à chaque participant une autorisation : ce dont le navigateur a besoin pour se connecter. Ce qui diffère, c’est où vont la signalisation (les messages qui établissent une connexion) et l’audio :

- **`p2p`.** Un navigateur envoie ses signaux (offres, réponses et candidats ICE) au serveur, qui les transmet à l’autre navigateur par le flux d’événements du salon ; un signal attend dans le magasin clé-valeur au plus 30 secondes. L’audio va directement de navigateur à navigateur. L’autorisation liste les serveurs STUN de `voice_stun_urls` et le serveur TURN de `voice_turn` (`coturn` ou `cloudflare`) avec des identifiants de courte durée. Un navigateur apprend son adresse publique par STUN, et le serveur TURN relaie l’audio lorsque les deux navigateurs ne trouvent aucun chemin direct.
- **`sfu`.** Le serveur place chaque salon sur un nœud média (une unité de transmission sélective, ou SFU) : un processus avec `role = all` ou `sfu`. L’autorisation contient l’URL `wss:` de ce nœud (`voice_sfu_public_url`) et un ticket à usage unique. Le navigateur envoie ses signaux par un WebSocket au nœud et son audio au port UDP `voice_sfu_udp_port` du nœud (TCP `voice_sfu_tcp_port` quand UDP est bloqué), et le nœud transmet l’audio des intervenants aux autres participants. Le serveur TURN de `voice_turn` relaie pour les navigateurs qui ne peuvent pas joindre le nœud directement.

Remarques sur le choix :

- **`p2p`** n’a besoin d’aucun port UDP sur le serveur : l’audio ne passe jamais par lui (il passe par le serveur TURN lorsqu’on en utilise un), et la charge du serveur se limite aux messages des salons, aux signaux de présence et aux flux. Il convient à une petite instance, à de petits salons et à un déploiement sur Cloud Run seul.
- **`sfu`** impose sur le serveur qui peut parler : le nœud ne transmet que l’audio des intervenants. Les navigateurs ne joignent que le nœud média : les adresses IP des participants sont donc visibles du nœud, pas des autres participants. Il faut, pour chaque nœud, une machine dotée d’une IP publique et de ports UDP et TCP ouverts ([Nœuds média](#media-nodes)). `voice_stun_urls` n’est pas nécessaire, puisqu’un nœud a une adresse publique, et `voice_ice_transport_policy = relay` est refusé.
- Servez l’instance en HTTPS : les navigateurs n’autorisent l’accès au micro que sur des pages sécurisées.

Les limites bornent le coût des salons. `0` supprime n’importe laquelle d’entre elles sauf la dernière :

| Réglage | Valeur par défaut | Signification |
| --- | --- | --- |
| `voice_max_room_duration` | `4h` | La durée maximale pendant laquelle un salon reste ouvert. Un salon qui l’atteint est terminé. |
| `voice_max_open_rooms` | `10` | Combien de salons peuvent être ouverts en même temps sur ce serveur. Un salon ouvert pour une heure programmée compte à partir de son ouverture, pendant qu’il attend son premier participant. |
| `voice_max_participants` | `10` | Le plus grand salon, de 2 à 500 (50 avec `p2p`). Un hôte peut fixer un maximum plus petit pour son salon. |

Qui peut ouvrir des salons est le paramètre d’instance `voice_room_starters` ([paramètres de l’instance](instance-settings.md)) : par défaut, seuls les détenteurs de la permission `start_voice_rooms` (le rôle intégré `admin` l’a) ; `everyone` permet à tout utilisateur local connecté qui n’est ni suspendu ni limité d’ouvrir des salons.

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

<!-- source: size-and-bandwidth e54157c29322 -->
### Taille et bande passante

Deux navigateurs se connectent l’un à l’autre quand au moins l’un d’eux parle ; deux personnes qui ne font qu’écouter ne se connectent pas.
Une personne qui parle envoie donc une copie distincte de son audio à chaque autre participant. La parole en Opus prend environ 30 à 50 kbit/s avec la surcharge : une personne qui parle dans un salon vocal de 10 envoie donc environ 9 copies, quelques centaines de kbit/s,
et une personne qui parle dans un salon vocal de 50 envoie 49 copies, soit environ 2 Mbit/s, ce qu’un téléphone en données mobiles ou une connexion domestique peut ne pas soutenir. Le nombre de connexions dans un salon vocal de N personnes est au plus N(N-1)/2 : 45 pour 10 et 1 225 pour 50.
C’est pourquoi la valeur par défaut est 10. N’augmentez `voice_max_participants` que si vos utilisateurs disposent de la bande passante montante nécessaire ; utilisez `sfu` pour les grands salons vocaux.

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

<!-- source: what-p2p-cannot-enforce 4f97611bece5 -->
### Ce que le P2P ne peut pas imposer

Seuls certains participants peuvent parler dans un salon vocal : les hôtes et les auditeurs qu’un hôte y autorise. En `p2p`, cela est imposé à deux endroits : le serveur refuse de relayer une offre de connexion qui enverrait de l’audio depuis quelqu’un qui n’a pas le droit de parler, et les autres navigateurs ne lisent pas l’audio d’un participant qui n’est pas intervenant.

Cela arrête les utilisateurs ordinaires et le client web officiel, mais ce n’est pas une défense complète. **Deux participants avec des clients modifiés qui coopèrent peuvent tout de même échanger de l’audio** : un client modifié de quelqu’un qui n’a pas le droit de parler peut l’envoyer, et un client modifié d’un destinataire peut le lire, et le serveur ne peut pas voir à l’intérieur des médias chiffrés.
Si vous devez garantir qui parle, par exemple pour des événements publics modérés, utilisez `sfu`, où le nœud média lui-même ne transmet que l’audio des intervenants.

<!-- source: media-nodes 17f4fc8eb80b -->
## Nœuds média

Avec `voice = sfu`, chaque salon est détenu par un nœud média, dont le processus transmet l’audio du salon. Un salon ne s’étend jamais sur plusieurs nœuds.

<!-- source: run-media-nodes fac01c5bdbf9 -->
### Exploiter des nœuds média

- **`role = all`** exécute un nœud dans le même processus que tout le reste, à `wss://<host>/voice/sfu`. Une seule machine avec les ports UDP et TCP ouverts suffit.
- **`role = sfu`** exécute un nœud à part, à côté des processus `web` et `worker`. Il partage leur base de données, leur `stream` (pas `hub`) et leur `kv` (pas `memory`) : les nœuds s’enregistrent eux-mêmes, ainsi que leurs salons, dans le magasin clé-valeur, et apprennent les changements de leurs salons par le flux. Il n’exécute aucune tâche et ne sert que les contrôles de santé et le WebSocket du nœud. Exécutez-en autant qu’il vous en faut : un processus `web` place chaque nouveau salon sur le nœud qui a le moins de places réservées et qui a de la place pour lui. Un processus `web` n’exécute aucun nœud.

Ce dont chaque nœud a besoin :

| Quoi | Réglage | Remarques |
| --- | --- | --- |
| Une adresse IP publique | `voice_sfu_public_ip` derrière un NAT | Le nœud donne cette adresse aux navigateurs, ou sa propre adresse quand elle n’est pas définie. Une VM cloud ou un conteneur a souvent sa propre adresse privée : définissez-la. |
| Le port UDP, ouvert sur Internet | `voice_sfu_udp_port` (7882) | Toutes les sessions du nœud le partagent. |
| Le port TCP, ouvert sur Internet | `voice_sfu_tcp_port` (7881) | Pour les navigateurs sur des réseaux qui bloquent UDP. |
| Une URL `wss:` avec TLS | `voice_sfu_public_url` (`wss://<host>/voice/sfu`) | Chaque nœud a la sienne, comme `wss://sfu1.example.org/voice/sfu`. Un proxy inverse placé devant termine TLS et transmet le WebSocket, ou le nœud sert lui-même le HTTPS : les réglages `tls` fonctionnent avec `role = sfu`, pour l’hôte de `voice_sfu_public_url` ([Servir le HTTPS depuis le serveur](running.md#serving-https-from-the-server)). |
| Combien de participants il détient | `voice_sfu_node_capacity` (500) | Voir [Capacité](#capacity). |

Transférez les ports UDP et TCP au nœud sans les modifier : le nœud donne aux navigateurs ses propres numéros de port. Un nœud sur un autre hôte que `host` a aussi besoin de `voice_sfu_origins` sur les processus `web`, comme `wss://*.sfu.example.org`, afin que la Content Security Policy du client web autorise la connexion.

<!-- source: docker-compose 35fa31462add -->
### Docker Compose

La couche [`compose.sfu.yaml`](../../deploy/docker/compose.sfu.yaml) exécute le serveur en `role = web`, un `worker` et un nœud média `sfu` sur une seule machine, sur PostgreSQL derrière Caddy ([Fichiers Docker Compose](running.md#docker-compose-files)) :

```sh
# In .env:
# COMPOSE_FILE=compose.yaml:compose.postgres.yaml:compose.caddy.yaml:compose.sfu.yaml
# MY_VOICE_SFU_PUBLIC_IP=203.0.113.10   the public address of this machine
docker compose up -d
```

- Caddy envoie `/voice/sfu` au nœud et toute autre requête au serveur : l’URL du nœud est donc la valeur par défaut `wss://<MY_HOST>/voice/sfu` et `voice_sfu_origins` n’est pas nécessaire.
- Ouvrez UDP 7882 et TCP 7881 dans le pare-feu de la machine, en plus de 80 et 443.
- La couche définit `voice = sfu` et `stream = postgres` pour les trois processus, quoi que dise `.env`.

<!-- source: capacity 2137d8a7bcce -->
### Capacité

Un salon réserve son maximum (le maximum de l’hôte pour le salon, ou `voice_max_participants`) sur son nœud quand il y est placé, quel que soit le nombre de personnes qui l’ont rejoint. Un nœud n’accepte un salon que tant que la somme de ses réservations reste dans `voice_sfu_node_capacity` : un nœud ne détient donc jamais plus de participants que sa capacité. Quand aucun nœud n’a de place pour le maximum d’un salon, l’ouverture du salon ou sa première arrivée répond `503`, et l’utilisateur peut réessayer plus tard.

Définissez `voice_sfu_node_capacity` d’après ce que la machine peut transmettre ([Coût mesuré](#measured-cost)), et ne laissez pas `voice_max_participants` dépasser ce dont vos salons ont besoin : un salon de 500 réserve 500 places même avec 3 personnes dedans.

<!-- source: measured-cost fef7c9ecfe21 -->
### Coût mesuré

Mesuré avec des clients simulés envoyant de l’audio Opus à 50 paquets par seconde, sur un AMD Ryzen 9 7900 (12 cœurs) sous Linux avec WSL2 et Go 1.27, le tout en boucle locale. Le CPU est exprimé en cœurs de cette machine ; la mémoire est le tas du nœud et les piles des goroutines, en plus du reste du processus.

| Quoi | CPU | Mémoire |
| --- | --- | --- |
| Un nœud inactif | non mesurable | environ 0,1 MiB |
| Chaque salon ouvert | non mesurable | environ 24 KiB |
| Chaque participant | environ 0,0002 cœur tant qu’il se tait | environ 200 KiB |
| Chaque flux transmis (un intervenant entendu par un participant) | 20 à 27 µs par paquet, environ 0,001 cœur | environ 24 KiB |
| Un salon de 100 avec 3 intervenants qui émettent du son | environ 0,3 cœur | environ 25 MiB |

Une machine plus rapide ou plus lente fait varier ces chiffres ; les métriques propres à un nœud ([Métriques à surveiller](#metrics-to-watch)) montrent ce qu’il porte.

Le coût de la transmission suit les paquets que le nœud envoie : l’audio de chaque intervenant va à chaque autre participant, si bien qu’un salon de N participants avec K intervenants envoie environ K × (N − 1) flux de 30 à 50 kbit/s. Un salon avec beaucoup d’auditeurs et peu d’intervenants coûte peu ; un salon où tout le monde parle en même temps coûte le plus. Prévoyez de la même façon la bande passante sortante du nœud.

<!-- source: drain-a-node-for-a-deploy 249929addcae -->
### Drainer un nœud avant un déploiement

À la réception de SIGTERM, un nœud se draine avant de s’arrêter :

1. Il n’accepte plus de nouveaux salons.
2. Il place chacun de ses salons sur un autre nœud et dit aux clients du salon de se déplacer. Ils se reconnectent au nouveau nœud avec une nouvelle autorisation, et l’audio s’interrompt environ 1 à 2 secondes.
3. Il attend que ses clients soient partis, pendant au plus 20 des 25 secondes de l’arrêt, puis attend les requêtes restantes et se termine ([Arrêter le serveur](running.md#stop-the-server)).

Donnez au processus un délai de grâce d’au moins 30 secondes avant qu’il ne soit tué (Kubernetes et ECS ont 30 par défaut, Docker 10, ce que `compose.sfu.yaml` relève), et redémarrez les nœuds un par un, afin que les autres puissent prendre leurs salons. Un salon pour lequel aucun autre nœud n’a de place reste sur le nœud en cours de drainage jusqu’à ce qu’il se termine ou que les 20 secondes passent. **Avec un seul nœud**, tous les salons restent : leurs participants perdent l’audio quand le nœud se termine, et se reconnectent une fois le nouveau nœud démarré.

<!-- source: when-a-node-is-lost c577f2240d7e -->
### Quand un nœud est perdu

Quand un nœud s’arrête sans se drainer (un plantage, une machine perdue), les WebSockets de ses clients se ferment et ses baux dans le magasin clé-valeur expirent en moins de 15 secondes. Les clients demandent une nouvelle autorisation, et le processus `web` place de nouveau le salon sur un autre nœud. Un salon ne perd rien avec son nœud : ses participants, ses intervenants et ses coupures de micro sont dans la base de données.

<!-- source: turn-for-clients-without-udp 1c1230dc875c -->
### TURN pour les clients sans UDP

Un navigateur joint un nœud en UDP, ou en TCP quand UDP est bloqué. Sur les réseaux qui ne laissent passer que le port 443 (certains réseaux d’entreprise et publics), il a besoin d’un serveur TURN en TLS sur 443 : définissez `voice_turn` sur `coturn`, avec une URL `turns:` sur le port 443 dans `voice_turn_coturn_urls`, ou sur `cloudflare`, comme pour `p2p` ([coturn](#coturn), [Cloudflare TURN](#cloudflare-turn)). `voice_ice_transport_policy = relay` est refusé avec `sfu` : les participants ne voient de toute façon que l’adresse du nœud.

<!-- source: cloud-run 5cf3ffcbb7a6 -->
### Cloud Run

Cloud Run n’accepte que des requêtes HTTP, donc `role = sfu` ne peut pas y fonctionner. Exécutez les processus `web` et `worker` sur Cloud Run et les nœuds sur des VM dotées d’une IP publique, en partageant avec eux la base de données, le `stream` (`postgres` ou `pubsub`) et le `kv` (`db` ou `valkey`). Donnez à chaque nœud son propre nom d’hôte et son propre `voice_sfu_public_url`, servez TLS sur le nœud (`tls = acme`) ou devant lui, et définissez `voice_sfu_origins` sur le service Cloud Run avec les origines des nœuds. Les modules OpenTofu de [Déployer sur Google Cloud](deploy-google-cloud.md) ne créent pas les nœuds.

<!-- source: metrics-to-watch 3346746d92f1 -->
### Métriques à surveiller

Chaque nœud exporte ses propres métriques `my.voice.sfu.*` ([Métriques](running.md#metrics)).

| Métrique | Ce qu’elle vous dit |
| --- | --- |
| `my.voice.sfu.capacity.reserved` face à `voice_sfu_node_capacity` | À quel point un nœud est plein pour les nouveaux salons. Quand tous les nœuds en sont proches, les nouveaux salons reçoivent `503` : ajoutez un nœud, ou augmentez la capacité si le nœud a du CPU à revendre. |
| `my.voice.sfu.capacity.connected`, `my.voice.sfu.sessions`, `my.voice.sfu.rooms` | Qui est connecté. Très en dessous des réservations signifie que les maximums des salons sont plus grands que nécessaire. |
| `my.voice.sfu.packets` par `my.outcome` | `forwarded` est le travail du nœud. `silent` et `not_allowed` (un participant qui n’a pas le droit de parler ou est coupé) sont abandonnés par conception. `queue_full` signifie que la connexion d’un participant n’a pas pu suivre : un client lent quand ils sont peu nombreux, un nœud à court de CPU ou de bande passante quand cela monte pour tous les clients à la fois. |
| `my.voice.sfu.bytes` | L’audio transmis : la bande passante sortante du nœud. |
| `my.voice.sfu.client.rtt`, `my.voice.sfu.client.packet_loss` | La qualité sur le chemin vers les participants. Élevée pour la plupart des clients, elle désigne le réseau ou l’emplacement du nœud ; pour quelques-uns, leurs propres réseaux. |
| `my.voice.sfu.migrations`, `my.voice.sfu.ice_restarts`, `my.voice.sfu.renegotiations` | Les salons déplacés par un drainage, les clients qui ont changé de réseau et les changements d’intervenants. |
| `my.voice.sfu.ticket_refusals` par `my.voice.sfu.reason` | Un `other_node` qui ne cesse de monter signifie que des clients joignent un autre nœud que celui de leur autorisation : vérifiez que le `voice_sfu_public_url` de chaque nœud mène à ce nœud. |

<!-- source: deployment-notes 63042f72a7c5 -->
## Notes de déploiement

- **Deux flux par utilisateur** : pendant un appel, un participant tient le flux d’événements propre au salon (Server-Sent Events) en plus du flux que l’application ouvre toujours. Dimensionnez les limites de connexions pour deux requêtes de longue durée par utilisateur en appel.
- **Plusieurs processus** : utilisez un adaptateur de flux partagé (`stream` défini sur `postgres`, `valkey`, `sns` ou `pubsub`), comme pour le reste de l’application. Avec `stream = hub` et un `role` autre que `all`, le serveur avertit au démarrage que le chat vocal ne fonctionnera pas d’un processus à l’autre.
- **Cloud Run** : un flux est une requête, et le CPU est alloué tant qu’elle est ouverte. Lorsque le délai d’expiration de la requête la coupe, le client se reconnecte. Réglez le délai aussi long que vous le pouvez : `service.request_timeout_seconds` de [Déployer sur Google Cloud](deploy-google-cloud.md#limits).
- **Les nœuds média ont besoin d’une VM** : un nœud média de `sfu` reçoit l’audio en UDP dans un processus de longue durée, que Cloud Run ne peut pas héberger ([Cloud Run](#cloud-run)).

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
