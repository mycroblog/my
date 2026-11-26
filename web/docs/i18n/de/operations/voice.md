<!-- source: run-voice-chat f44427fbbe76 -->
# Sprachchat betreiben

Mit dem Sprachchat können angemeldete Nutzer einen Raum öffnen, andere einladen und sprechen. Er ist ausgeschaltet, bis du wählst, wie das Audio übertragen wird.
Dieser Leitfaden behandelt diese Wahl, die STUN- und TURN-Server oder die Medienknoten, die jede Wahl braucht, und was du zu Deployment, Datenschutz und Moderation wissen solltest.
Jede hier genannte Einstellung ist in der [Konfigurationsreferenz](../reference/configuration.md#voice) beschrieben.

<!-- source: choose-how-the-audio-travels 2ef72c636c49 -->
## Wählen, wie das Audio übertragen wird

`voice` wählt eine Methode für die ganze Instanz; jeder Raum nutzt sie.

| `voice` | Was das Audio trägt | Passt für |
| --- | --- | --- |
| `off` | Nichts. Die Sprachchat-API antwortet `404`, der Webclient zeigt keinen Sprachchat, und die anderen Einstellungen `voice_*` werden beim Start nicht geprüft. | Der Standard. |
| `p2p` | Die Browser der Teilnehmer senden das Audio direkt aneinander. Der Server leitet nur die Nachrichten weiter, die die Verbindungen aufbauen. | Kleine Räume und das günstigste Setup: kein zusätzlicher Server außer einem optionalen STUN- oder TURN-Server. |
| `sfu` | Die eigenen Medienknoten des Servers: Jeder Browser sendet sein Audio an den Knoten, der seinen Raum hält, und dieser leitet das Audio der Sprecher an die Teilnehmer weiter. | Größere Räume und strikte Kontrolle darüber, wer sprechen darf. |

![Die beiden Methoden nebeneinander. Bei p2p leitet der Server die Signale weiter, und die Browser senden das Audio aneinander oder über einen TURN-Server. Bei sfu erhalten die Browser ihre Grants vom Server und senden Signale und Audio an den Medienknoten ihres Raums. Die Liste unten beschreibt jede Methode.](images/voice-methods.svg)

Bei jeder Methode hält der Server (`my serve`) die Räume, wer an ihnen teilnimmt, und ihre Sitzungen und übergibt jedem
Teilnehmer ein Grant: das, was der Browser zum Verbinden braucht. Der Unterschied ist, wohin das Signaling
(die Nachrichten, die eine Verbindung aufbauen) und das Audio gehen:

- **`p2p`.** Ein Browser sendet seine Signale (Offers, Answers und ICE-Kandidaten) an den Server, der sie über den Ereignisstrom des Raums an den
  anderen Browser weitergibt; ein Signal wartet höchstens 30 Sekunden im Key-Value-Speicher. Das Audio
  geht direkt von Browser zu Browser. Das Grant listet die STUN-Server aus `voice_stun_urls` und den TURN-Server aus
  `voice_turn` (`coturn` oder `cloudflare`) mit kurzlebigen Zugangsdaten. Ein Browser erfährt seine öffentliche Adresse über STUN,
  und der TURN-Server leitet das Audio weiter, wenn die beiden Browser keinen direkten Weg finden.
- **`sfu`.** Der Server legt jeden Raum auf einem Medienknoten ab (einer Selective Forwarding Unit, kurz SFU): einem Prozess mit
  `role = all` oder `sfu`. Das Grant enthält die `wss:`-URL dieses Knotens (`voice_sfu_public_url`) und ein Einmal-Ticket.
  Der Browser sendet seine Signale über einen WebSocket an den Knoten und sein Audio an den UDP-Port `voice_sfu_udp_port` des Knotens
  (TCP `voice_sfu_tcp_port`, wenn UDP blockiert ist), und der Knoten leitet das Audio der Sprecher an die übrigen
  Teilnehmer weiter. Der TURN-Server aus `voice_turn` leitet für die Browser weiter, die den Knoten nicht direkt erreichen.

Hinweise zur Wahl:

- **`p2p`** braucht keinen UDP-Port auf dem Server: Das Audio läuft nie über ihn (es läuft über den TURN-Server, wenn einer verwendet wird), und die Last des Servers sind nur die Raumnachrichten, Heartbeats und Streams. Es passt zu einer kleinen Instanz, kleinen Räumen und einem Deployment allein auf Cloud Run.
- **`sfu`** setzt auf dem Server durch, wer sprechen darf: Der Knoten leitet nur das Audio der Sprecher weiter. Die Browser erreichen nur den Medienknoten, sodass die IP-Adressen der Teilnehmer für den Knoten sichtbar sind, nicht für die anderen Teilnehmer. Es braucht für jeden Knoten eine Maschine mit öffentlicher IP und offenen UDP- und TCP-Ports ([Medienknoten](#media-nodes)). `voice_stun_urls` wird nicht gebraucht, da ein Knoten eine öffentliche Adresse hat, und `voice_ice_transport_policy = relay` wird abgelehnt.
- Liefere die Instanz über HTTPS aus: Browser erlauben Mikrofonzugriff nur auf sicheren Seiten.

Die Limits halten die Kosten der Räume begrenzt. `0` hebt jedes davon auf, außer dem letzten:

| Einstellung | Standard | Bedeutung |
| --- | --- | --- |
| `voice_max_room_duration` | `4h` | Die längste Zeit, die ein Raum offen bleibt. Ein Raum, der sie erreicht, wird beendet. |
| `voice_max_open_rooms` | `10` | Wie viele Räume auf diesem Server gleichzeitig offen sein dürfen. Ein Raum, der für eine geplante Zeit geöffnet wird, zählt ab seiner Öffnung, während er auf seinen ersten Teilnehmer wartet. |
| `voice_max_participants` | `10` | Der größte Raum, von 2 bis 500 (50 bei `p2p`). Ein Host kann für seinen Raum ein kleineres Maximum festlegen. |

Wer Räume öffnen darf, ist die Instanzeinstellung `voice_room_starters` ([Instanzeinstellungen](instance-settings.md)): standardmäßig nur die Inhaber der Berechtigung `start_voice_rooms` (die eingebaute Rolle `admin` hat sie); `everyone` lässt jeden angemeldeten lokalen Benutzer, der nicht gesperrt oder eingeschränkt ist, Räume öffnen.

<!-- source: peer-to-peer-rooms 2199ca9a427c -->
## Peer-to-Peer-Räume

<!-- source: try-it-with-docker-compose ac3681681768 -->
### Mit Docker Compose ausprobieren

Um den Sprachchat auf deiner Maschine mit dem Docker-Compose-Setup ([`deploy/docker/compose.yaml`](../../deploy/docker/compose.yaml)) auszuprobieren,
setze `MY_VOICE=p2p` in `.env` und erstelle den Server neu.

```sh
cd deploy/docker
docker compose up -d
```

- Öffne den Server unter `http://localhost:8080` (dem `MY_WEB_ORIGIN` aus `example.env`). Browser lassen eine Seite das Mikrofon nur auf
  `localhost` oder über HTTPS nutzen; mit einer Adresse wie `http://192.168.1.10:8080` kann man also nicht sprechen.
- Browser auf derselben Maschine (zwei Browserprofile oder ein normales und ein privates Fenster) verbinden sich ohne STUN- oder TURN-Server
  miteinander. Um Browser auf anderen Maschinen zu erreichen, stelle den Server hinter HTTPS und setze `MY_VOICE_STUN_URLS` in `.env`.
- Standardmäßig darf nur die Rolle `admin` Räume öffnen. Melde dich als der Administrator an, den du angelegt hast, oder setze `voice_room_starters` auf `everyone`
  auf der Seite „Sprachchat“ der Servereinstellungen, damit jeder Nutzer Räume öffnen darf.

<!-- source: size-and-bandwidth e54157c29322 -->
### Größe und Bandbreite

Zwei Browser verbinden sich miteinander, wenn mindestens einer von ihnen spricht; zwei Personen, die nur zuhören, verbinden sich nicht.
Ein Sprecher sendet daher an jeden anderen Teilnehmer eine eigene Kopie seines Audios. Sprache in Opus braucht mit Overhead etwa 30 bis 50 kbit/s, ein Sprecher in einem Raum mit 10 lädt also etwa 9 Kopien hoch, einige hundert kbit/s,
und ein Sprecher in einem Raum mit 50 lädt 49 Kopien hoch, rund 2 Mbit/s, was ein Handy mit mobilen Daten oder ein Heimanschluss möglicherweise nicht schafft. Die Zahl der Verbindungen in einem Raum mit N Personen ist höchstens N(N-1)/2: 45 bei 10 und 1.225 bei 50.
Deshalb ist der Standard 10. Erhöhe `voice_max_participants` nur, wenn deine Nutzer die Upload-Bandbreite haben; nutze `sfu` für große Räume.

<!-- source: stun-and-turn 51e427976a77 -->
### STUN und TURN

Ein Browser hinter NAT braucht einen STUN-Server, um seine öffentliche Adresse zu erfahren, und einen TURN-Server, der das Audio weiterleitet, wenn es keinen direkten Weg gibt (strikte NATs, Firmen-Firewalls, manche Mobilfunknetze).

- `voice_stun_urls`: kommagetrennte URLs mit `stun:` oder `stuns:`. Jeder öffentliche STUN-Server funktioniert, und der coturn-Server unten beantwortet ebenfalls STUN.
- `voice_turn`: `none`, `coturn` oder `cloudflare`.

Mit keinem von beiden warnt der Server beim Start: Nur Browser im selben Netzwerk verbinden sich. Ein öffentlicher Server braucht STUN und TURN für die Nutzer, die sich nicht direkt verbinden können.

Der Server gibt jedem Teilnehmer kurzlebige TURN-Zugangsdaten, nie ein gemeinsames Passwort. `voice_turn_credential_ttl` (Standard `6h`) legt fest, wie lange sie gelten. Der Web-Client fordert zehn Minuten vor ihrem Ablauf neue Zugangsdaten an und startet ICE neu, damit ein Anruf nicht abbricht, wenn sie ablaufen.

<!-- source: coturn d665e942c91b -->
#### coturn

Setze `voice_turn = coturn`, `voice_turn_coturn_urls` und `voice_turn_coturn_secret`. Der Server berechnet die Zugangsdaten selbst (der Benutzername ist die Ablaufzeit und die Sitzungs-ID, das Passwort ist ein HMAC-SHA1 davon mit dem Secret), stellt also keine Anfrage an den TURN-Server.
Konfiguriere coturn passend dazu in `turnserver.conf`:

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

- `denied-peer-ip` ist wichtig. Ohne es kann jeder mit Zugangsdaten den TURN-Server nutzen, um Hosts in dessen internem Netzwerk zu erreichen (Cloud-Metadatendienste, Datenbanken, andere VMs). Sperre jeden privaten, Loopback- und Link-Local-Bereich; erlaube eine interne Adresse (`allowed-peer-ip`) nur, wenn du wirklich dorthin weiterleiten musst.
- Öffne UDP und TCP 3478, TCP 5349 für `turns:` und den Relay-Portbereich von coturn (`min-port` / `max-port`, standardmäßig 49152-65535).
- Führe in `voice_turn_coturn_urls` auch eine `turns:`-URL oder eine mit `?transport=tcp` auf, damit sich Nutzer in Netzwerken, die UDP blockieren, trotzdem verbinden können.
- coturn prüft das Ablaufdatum von Zugangsdaten nur, wenn eine Allocation zum ersten Mal angelegt wird. Eine bestehende Allocation funktioniert darüber hinaus weiter, und die neuen Zugangsdaten gelten für die nächste Allocation, die der Web-Client durch einen Neustart von ICE anlegt.

<!-- source: cloudflare-turn be5846573be8 -->
#### Cloudflare TURN

Setze `voice_turn = cloudflare`, `voice_turn_cloudflare_key_id` und `voice_turn_cloudflare_api_token` aus einem TURN-Schlüssel, den du im Dashboard von Cloudflare angelegt hast. Der Server fragt Cloudflare nach den Zugangsdaten jedes Teilnehmers, wenn dieser einen Raum öffnet oder betritt (schlägt das fehl, antwortet die Anfrage mit `503` und öffnet oder betritt nichts, sodass sie erneut versucht werden kann), und widerruft sie, wenn der Teilnehmer geht oder entfernt wird.

- Zugangsdaten gelten höchstens 48 Stunden, daher darf `voice_turn_credential_ttl` `48h` nicht überschreiten (sonst weigert sich der Server zu starten).
- **Cloudflare trennt ein Relay kurz nach dem Ablauf seiner Zugangsdaten.** Der Web-Client erneuert sie vorher; halte `voice_turn_credential_ttl` also deutlich über zehn Minuten.
- Preise, Stand 2026-10-26: die ersten 1.000 GB pro Monat sind kostenlos, danach 0,05 $ pro GB, das an Clients gesendet wird. Prüfe die aktuellen Preise von Cloudflare, bevor du dich darauf verlässt.

<!-- source: hiding-ip-addresses de67ffa9e7b7 -->
### IP-Adressen verbergen

Mit `voice_ice_transport_policy = all` (dem Standard) verbinden sich Browser direkt, wenn sie können, sodass die Teilnehmer eines Raums die IP-Adressen der anderen erfahren können.

Mit `relay` nutzen Browser nur den TURN-Server, sodass ein Teilnehmer die Adresse des TURN-Servers sieht und nicht die der anderen Teilnehmer. Das Audio bleibt zwischen den Browsern verschlüsselt. Was `relay` nicht verbirgt: Wer den TURN-Server betreibt (du oder Cloudflare), sieht die Adresse jedes Teilnehmers, und dieser Server sieht die Adresse jedes, der seine API aufruft.
`relay` verlangt, dass `voice_turn` `coturn` oder `cloudflare` ist (sonst weigert sich der Server zu starten), und leitet jeden Anruf über den TURN-Server, was bei coturn deine Bandbreite und bei Cloudflare deine Transferrechnung ist.

<!-- source: what-p2p-cannot-enforce 4f97611bece5 -->
### Was P2P nicht durchsetzen kann

In einem Raum dürfen nur manche Teilnehmer sprechen: die Hosts und die Zuhörer, die ein Host zulässt. Bei `p2p` wird das an zwei Stellen durchgesetzt: Der Server weigert sich, ein Verbindungsangebot weiterzuleiten, das Audio von jemandem senden würde, der nicht sprechen darf, und die anderen Browser spielen kein Audio eines Teilnehmers ab, der kein Sprecher ist.

Das hält gewöhnliche Nutzer und den offiziellen Web-Client auf, ist aber keine vollständige Verteidigung. **Zwei Teilnehmer mit veränderten Clients, die zusammenarbeiten, können trotzdem Audio austauschen**: Ein veränderter Client von jemandem, der nicht sprechen darf, kann es senden, und ein veränderter Client eines Empfängers kann es abspielen, und der Server kann nicht in die verschlüsselten Medien hineinsehen.
Wenn du garantieren musst, wer spricht, zum Beispiel bei moderierten öffentlichen Veranstaltungen, nutze `sfu`, wo der Medienknoten selbst nur das Audio der Sprecher weiterleitet.

<!-- source: media-nodes 17f4fc8eb80b -->
## Medienknoten

Mit `voice = sfu` wird jeder Raum von einem Medienknoten gehalten, dessen Prozess das Audio des Raums weiterleitet. Ein Raum
erstreckt sich nie über mehrere Knoten.

<!-- source: run-media-nodes fac01c5bdbf9 -->
### Medienknoten betreiben

- **`role = all`** betreibt einen Knoten im selben Prozess wie alles andere, unter `wss://<host>/voice/sfu`. Eine Maschine mit
  offenen UDP- und TCP-Ports genügt.
- **`role = sfu`** betreibt einen eigenen Knoten neben den Prozessen `web` und `worker`. Er teilt deren Datenbank, deren
  `stream` (nicht `hub`) und deren `kv` (nicht `memory`): Die Knoten tragen sich und ihre Räume im Key-Value-Speicher ein und
  erfahren über den Stream von Änderungen an ihren Räumen. Er führt keine Jobs aus und bedient nur die Health-Checks und
  den WebSocket des Knotens. Betreibe so viele, wie du brauchst: Ein `web`-Prozess legt jeden neuen Raum auf dem Knoten mit den
  wenigsten reservierten Plätzen ab, der noch Platz dafür hat. Ein `web`-Prozess betreibt keinen Knoten.

Was jeder Knoten braucht:

| Was | Einstellung | Hinweise |
| --- | --- | --- |
| Eine öffentliche IP-Adresse | `voice_sfu_public_ip` hinter NAT | Der Knoten gibt den Browsern diese Adresse, oder seine eigene Adresse, wenn sie nicht gesetzt ist. Eine Cloud-VM oder ein Container hat meist eine eigene private Adresse: Setze sie. |
| Der UDP-Port, zum Internet offen | `voice_sfu_udp_port` (7882) | Alle Sitzungen des Knotens teilen ihn. |
| Der TCP-Port, zum Internet offen | `voice_sfu_tcp_port` (7881) | Für Browser in Netzwerken, die UDP blockieren. |
| Eine `wss:`-URL mit TLS | `voice_sfu_public_url` (`wss://<host>/voice/sfu`) | Jeder Knoten hat eine eigene, etwa `wss://sfu1.example.org/voice/sfu`. Ein vorgeschalteter Reverse-Proxy beendet TLS und leitet den WebSocket weiter, oder der Knoten liefert HTTPS selbst aus: Die `tls`-Einstellungen gelten bei `role = sfu` für den Host von `voice_sfu_public_url` ([HTTPS vom Server ausliefern](running.md#serving-https-from-the-server)). |
| Wie viele Teilnehmer er hält | `voice_sfu_node_capacity` (500) | Siehe [Kapazität](#capacity). |

Leite den UDP- und den TCP-Port unverändert an den Knoten weiter: Der Knoten gibt den Browsern seine eigenen Portnummern. Ein Knoten auf
einem anderen Host als `host` braucht außerdem `voice_sfu_origins` auf den `web`-Prozessen, etwa `wss://*.sfu.example.org`, damit
die Content Security Policy des Web-Clients die Verbindung erlaubt.

<!-- source: docker-compose 35fa31462add -->
### Docker Compose

Die Ebene [`compose.sfu.yaml`](../../deploy/docker/compose.sfu.yaml) betreibt den Server als `role = web`, einen `worker` und
einen Medienknoten `sfu` auf einer Maschine, mit PostgreSQL hinter Caddy ([Docker-Compose-Dateien](running.md#docker-compose-files)):

```sh
# In .env:
# COMPOSE_FILE=compose.yaml:compose.postgres.yaml:compose.caddy.yaml:compose.sfu.yaml
# MY_VOICE_SFU_PUBLIC_IP=203.0.113.10   the public address of this machine
docker compose up -d
```

- Caddy leitet `/voice/sfu` an den Knoten und jede andere Anfrage an den Server, sodass die URL des Knotens der Standard
  `wss://<MY_HOST>/voice/sfu` ist und `voice_sfu_origins` nicht nötig ist.
- Öffne UDP 7882 und TCP 7881 in der Firewall der Maschine, zusätzlich zu 80 und 443.
- Die Ebene setzt `voice = sfu` und `stream = postgres` für die drei Prozesse, was auch immer `.env` sagt.

<!-- source: capacity 2137d8a7bcce -->
### Kapazität

Ein Raum reserviert sein Maximum (das Maximum des Hosts für den Raum oder `voice_max_participants`) auf seinem Knoten, wenn er
abgelegt wird, egal wie viele beigetreten sind. Ein Knoten nimmt einen Raum nur an, solange die Summe seiner Reservierungen innerhalb von
`voice_sfu_node_capacity` bleibt, sodass ein Knoten nie mehr Teilnehmer hält als seine Kapazität. Hat kein Knoten Platz für das
Maximum eines Raums, antwortet das Öffnen des Raums oder der erste Beitritt mit `503`, und der Nutzer kann es später erneut versuchen.

Setze `voice_sfu_node_capacity` danach, was die Maschine weiterleiten kann ([Gemessene Kosten](#measured-cost)), und halte
`voice_max_participants` nicht größer, als deine Räume brauchen: Ein Raum mit 500 reserviert 500 Plätze, auch wenn nur 3 Personen darin sind.

<!-- source: measured-cost 2d6571f6e923 -->
### Gemessene Kosten

Gemessen mit simulierten Clients, die Opus-Audio mit 50 Paketen pro Sekunde senden, auf einem AMD Ryzen 9 7900 (12 Kerne) mit
Linux unter WSL2 und Go 1.27, alles über Loopback. Die CPU ist in Kernen dieser Maschine angegeben; der Speicher ist der Heap des Knotens und
die Goroutine-Stacks zusätzlich zum Rest des Prozesses.

| Was | CPU | Speicher |
| --- | --- | --- |
| Ein untätiger Knoten | nicht messbar | etwa 0,1 MiB |
| Jeder offene Raum | nicht messbar | unter etwa 25 KiB |
| Jeder Teilnehmer | etwa 0,0002 Kerne, solange er schweigt | etwa 180 KiB |
| Jeder weitergeleitete Stream (ein Sprecher, der von einem Teilnehmer gehört wird) | 18 bis 27 µs pro Paket, etwa 0,001 Kerne | unter etwa 25 KiB |
| Ein Raum mit 100 und 3 Sprechern, die Geräusche machen | etwa 0,3 Kerne | etwa 23 MiB |

Eine schnellere oder langsamere Maschine skaliert diese Werte; die eigenen Metriken eines Knotens ([Zu beobachtende Metriken](#metrics-to-watch)) zeigen,
was er trägt.

Die Kosten der Weiterleitung folgen den Paketen, die der Knoten sendet: Das Audio jedes Sprechers geht an jeden anderen
Teilnehmer, sodass ein Raum mit N Teilnehmern und K Sprechern etwa K × (N − 1) Streams mit 30 bis 50 kbit/s sendet. Ein Raum mit
vielen Zuhörern und wenigen Sprechern ist günstig; ein Raum, in dem alle gleichzeitig sprechen, kostet am meisten. Plane die
ausgehende Bandbreite des Knotens auf dieselbe Weise.

<!-- source: drain-a-node-for-a-deploy 249929addcae -->
### Einen Knoten für ein Deployment leeren

Bei SIGTERM leert sich ein Knoten, bevor er stoppt:

1. Er nimmt keine neuen Räume mehr an.
2. Er legt jeden seiner Räume auf einem anderen Knoten ab und weist die Clients des Raums an, umzuziehen. Sie verbinden sich mit einem neuen Grant
   erneut mit dem neuen Knoten, und das Audio setzt für etwa 1 bis 2 Sekunden aus.
3. Er wartet, bis seine Clients gegangen sind, höchstens 20 der 25 Sekunden des Herunterfahrens, wartet dann auf die
   übrigen Anfragen und beendet sich ([Den Server stoppen](running.md#stop-the-server)).

Gib dem Prozess eine Gnadenfrist von mindestens 30 Sekunden, bevor er beendet wird (Kubernetes und ECS haben standardmäßig 30, Docker
10, was `compose.sfu.yaml` erhöht), und starte die Knoten nacheinander neu, damit die anderen ihre Räume übernehmen können.
Ein Raum, für den kein anderer Knoten Platz hat, bleibt auf dem sich leerenden Knoten, bis er endet oder die 20 Sekunden vergehen. **Bei einem
einzelnen Knoten** bleibt jeder Raum: Seine Teilnehmer verlieren das Audio, wenn sich der Knoten beendet, und verbinden sich erneut, sobald der neue Knoten
gestartet ist.

<!-- source: when-a-node-is-lost c577f2240d7e -->
### Wenn ein Knoten verloren geht

Stoppt ein Knoten ohne Leerung (ein Absturz, eine verlorene Maschine), schließen sich die WebSockets seiner Clients, und seine Leases im
Key-Value-Speicher laufen innerhalb von 15 Sekunden ab. Die Clients fordern ein neues Grant an, und der `web`-Prozess legt den Raum erneut
auf einem anderen Knoten ab. Ein Raum verliert mit seinem Knoten nichts: Seine Teilnehmer, Sprecher und Stummschaltungen liegen in der Datenbank.

<!-- source: turn-for-clients-without-udp 1c1230dc875c -->
### TURN für Clients ohne UDP

Ein Browser erreicht einen Knoten über UDP oder über TCP, wenn UDP blockiert ist. In Netzwerken, die nur Port 443 durchlassen (manche
Firmen- und öffentliche Netzwerke), braucht er einen TURN-Server über TLS auf 443: Setze `voice_turn` auf `coturn`, mit einer `turns:`-URL
auf Port 443 in `voice_turn_coturn_urls`, oder auf `cloudflare`, wie bei `p2p` ([coturn](#coturn),
[Cloudflare TURN](#cloudflare-turn)). `voice_ice_transport_policy = relay` wird bei `sfu` abgelehnt: Die Teilnehmer
sehen ohnehin nur die Adresse des Knotens.

<!-- source: cloud-run 5cf3ffcbb7a6 -->
### Cloud Run

Cloud Run akzeptiert nur HTTP-Anfragen, daher kann `role = sfu` dort nicht laufen. Betreibe die Prozesse `web` und `worker` auf Cloud
Run und die Knoten auf VMs mit öffentlicher IP, die Datenbank, den `stream` (`postgres` oder `pubsub`) und den `kv`
(`db` oder `valkey`) mit ihnen teilend. Gib jedem Knoten einen eigenen Hostnamen und `voice_sfu_public_url`, liefere TLS auf dem Knoten
(`tls = acme`) oder davor aus, und setze `voice_sfu_origins` auf dem Cloud-Run-Dienst auf die Origins der Knoten. Die
OpenTofu-Module von [Auf Google Cloud bereitstellen](deploy-google-cloud.md) legen die Knoten nicht an.

<!-- source: metrics-to-watch a8b32402b50d -->
### Zu beobachtende Metriken

Jeder Knoten exportiert seine eigenen Metriken `my.voice.sfu.*` ([Metriken](running.md#metrics)).

| Metrik | Was sie dir sagt |
| --- | --- |
| `my.voice.sfu.capacity.reserved` gegenüber `voice_sfu_node_capacity` | Wie voll ein Knoten für neue Räume ist. Sind alle Knoten nahe daran, erhalten neue Räume `503`: Füge einen Knoten hinzu oder erhöhe die Kapazität, wenn der Knoten CPU übrig hat. |
| `my.voice.sfu.capacity.connected`, `my.voice.sfu.sessions`, `my.voice.sfu.rooms` | Wer verbunden ist. Weit unter den Reservierungen heißt, dass die Maxima der Räume größer sind als nötig. |
| `my.voice.sfu.packets` nach `my.outcome` | `forwarded` ist die Arbeit des Knotens. `silent` und `not_allowed` (ein Teilnehmer, der nicht sprechen darf oder stummgeschaltet ist) werden absichtlich verworfen. `queue_full` heißt, dass die Verbindung eines Teilnehmers nicht mithalten konnte: ein langsamer Client, wenn es wenige sind, ein Knoten mit zu wenig CPU oder Bandbreite, wenn es für alle Clients gleichzeitig steigt. |
| `my.voice.sfu.bytes` | Das weitergeleitete Audio: die ausgehende Bandbreite des Knotens. |
| `my.voice.sfu.client.rtt`, `my.voice.sfu.client.packet_loss` | Die Qualität auf dem Weg zu den Teilnehmern. Hoch bei den meisten Clients deutet auf das Netzwerk oder den Standort des Knotens; bei wenigen auf deren eigene Netzwerke. |
| `my.voice.sfu.offer.wait` | Wie lange ein beitretender Teilnehmer auf das erste Offer des Knotens wartet: Millisekunden bei einem Knoten mit CPU übrig; Sekunden heißen, dass Beitritte auf einem ausgelasteten Knoten warten. |
| `my.voice.sfu.migrations`, `my.voice.sfu.ice_restarts`, `my.voice.sfu.renegotiations` | Durch eine Leerung verschobene Räume, Clients, die das Netzwerk gewechselt haben, und Wechsel der Sprecher. |
| `my.voice.sfu.ticket_refusals` nach `my.voice.sfu.reason` | Ein `other_node`, das weiter steigt, heißt, dass Clients einen anderen Knoten erreichen als den ihres Grants: Prüfe, dass die `voice_sfu_public_url` jedes Knotens zu diesem Knoten führt. |

<!-- source: deployment-notes 63042f72a7c5 -->
## Hinweise zum Deployment

- **Zwei Streams pro Nutzer**: Während eines Anrufs hält ein Teilnehmer zusätzlich zu dem Stream, den die App immer öffnet, den eigenen Ereignisstrom des Raums (Server-Sent Events). Dimensioniere die Verbindungslimits für zwei langlebige Anfragen pro Nutzer in einem Anruf.
- **Mehrere Prozesse**: Nutze einen gemeinsamen Stream-Adapter (`stream` auf `postgres`, `valkey`, `sns` oder `pubsub` gesetzt), wie beim Rest der App. Bei `stream = hub` und einer `role` außer `all` warnt der Server beim Start, dass der Sprachchat nicht über Prozesse hinweg funktioniert.
- **Cloud Run**: Ein Stream ist eine Anfrage, und CPU wird zugewiesen, solange sie offen ist. Wenn das Anfrage-Timeout sie abschneidet, verbindet sich der Client neu. Setze das Timeout so lang wie möglich: `service.request_timeout_seconds` unter [Auf Google Cloud bereitstellen](deploy-google-cloud.md#limits).
- **Medienknoten brauchen eine VM**: Ein Medienknoten von `sfu` nimmt Audio über UDP in einem dauerhaft laufenden Prozess entgegen, den Cloud Run nicht hosten kann ([Cloud Run](#cloud-run)).

<!-- source: ending-rooms-and-records b4d3d196ecd7 -->
## Räume beenden und Aufzeichnungen

- Ein Moderator mit `manage_reports` kann einen offenen Raum für alle beenden, auch wenn der Sprachchat ausgeschaltet ist ([Moderation](moderation.md#voice-chat-rooms)).
- Wer an einem Raum teilgenommen hat (Teilnahmen, Sprecher und Zuhörer, gehobene Hände, Einladungen und Entfernungen), wird `voice_participants_retention_days` Tage nach dem Ende des Raums aufbewahrt (Standard 90, mindestens 30) oder so lange, wie eine Meldung auf den Raum zeigt. Der Name des Raums, seine Beschreibung, die Hosts und wie er geendet hat, werden aufbewahrt. Das Audio, der Mikrofonzustand und Reaktionen werden nie aufgezeichnet.
- `voice_public_summary` steuert, ob Besucher, die einem `local`-Raum nicht beitreten können, seinen Namen und ob er offen ist sehen, auf seiner Seite und in seiner Link-Vorschau ([Servereinstellungen](instance-settings.md)).

<!-- source: notes-for-users f44150f15915 -->
## Hinweise für Nutzer

- Auf einem iPhone oder iPad kann das Senden von Audio aufhören, während die App im Hintergrund ist, weil iOS eine Webseite anhalten kann, die nicht im Vordergrund ist. Lass die App während eines Anrufs auf dem Bildschirm geöffnet.
- Der Browser fragt beim ersten Mal nach der Erlaubnis, das Mikrofon zu nutzen. Wenn du sie verweigert hast, erlaube es wieder in den Website-Einstellungen des Browsers.
- In einem Peer-to-Peer-Raum können andere Teilnehmer möglicherweise deine IP-Adresse herausfinden, es sei denn, der Server leitet Anrufe über ein Relay. Frag den Betreiber, wenn dir das wichtig ist.
