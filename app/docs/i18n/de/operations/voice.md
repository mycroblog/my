<!-- source: run-voice-chat 54808e78eb0d -->
# Sprachchat betreiben

Mit dem Sprachchat können angemeldete Nutzer einen Raum öffnen, andere einladen und sprechen. Er ist ausgeschaltet, bis du wählst, wie das Audio übertragen wird.
Dieser Leitfaden behandelt diese Wahl, die STUN- und TURN-Server oder den Medienserver, den jede Wahl braucht, und was du zu Deployment, Datenschutz und Moderation wissen solltest.
Jede hier genannte Einstellung ist in der [Konfigurationsreferenz](../reference/configuration.md#voice) beschrieben.

<!-- source: choose-how-the-audio-travels 9d60bd0655e9 -->
## Wählen, wie das Audio übertragen wird

`voice` wählt eine Methode für den ganzen Server; jeder Raum nutzt sie.

| `voice` | Was das Audio trägt | Eignet sich für |
| --- | --- | --- |
| `off` | Nichts. Die Sprachchat-API antwortet mit `404`, der Web-Client zeigt keinen Sprachchat, und die anderen `voice_*`-Einstellungen werden beim Start nicht geprüft. | Der Standard. |
| `p2p` | Die Browser der Teilnehmer senden Audio direkt aneinander. Der Server leitet nur die Nachrichten weiter, die die Verbindungen aufbauen. | Kleine Räume und das günstigste Setup: kein zusätzlicher Server außer einem optionalen STUN- oder TURN-Server. |
| `livekit` | Ein LiveKit-Server, selbst gehostet oder LiveKit Cloud. | Größere Räume und strenge Kontrolle darüber, wer sprechen darf. |
| `sora` | Ein Sora-Server, selbst gehostet oder Sora Cloud. | Dasselbe, wenn du Sora ohnehin betreibst. |

![Die drei Methoden nebeneinander. Bei p2p leitet der Server die Signale weiter, und die Browser senden Audio aneinander oder über einen TURN-Server. Bei livekit und sora senden die Browser Signale und Audio an den Medienserver, und der Server spricht über dessen API und Webhooks mit diesem Medienserver. Die Liste unten beschreibt jede Methode.](images/voice-methods.svg)

Bei jeder Methode hält der Server (`my serve`) die Räume, wer an ihnen teilnimmt und ihre Sitzungen, und gibt jedem
Teilnehmer ein Grant: was der Browser zum Verbinden braucht. Er trägt nie das Audio. Der Unterschied liegt darin, wohin
das Signaling (die Nachrichten, die eine Verbindung aufbauen) und das Audio gehen:

- **`p2p`.** Ein Browser sendet seine Signale (Offers, Answers und ICE-Kandidaten) an den Server, der sie über den
  Ereignisstrom des Raums an den anderen Browser weitergibt; ein Signal wartet höchstens 30 Sekunden im Key-Value-Store.
  Das Audio geht direkt von Browser zu Browser. Der Grant listet die STUN-Server aus `voice_stun_urls` und den
  TURN-Server aus `voice_turn` (`coturn` oder `cloudflare`) mit kurzlebigen Zugangsdaten. Ein Browser erfährt seine
  öffentliche Adresse über STUN, und der TURN-Server leitet das Audio weiter, wenn die beiden Browser keinen direkten
  Weg finden.
- **`livekit`.** Der Grant enthält `voice_livekit_url` und ein Zugriffstoken, das 60 Sekunden gilt. Der Browser sendet
  seine Signale und sein Audio an den LiveKit-Server, der seine eigenen STUN- und TURN-Dienste mitbringt und das Audio an
  die anderen Teilnehmer weiterleitet (eine Selective Forwarding Unit, kurz SFU).
  Der Server legt die LiveKit-Räume an und ändert über die API von LiveKit, wer sprechen darf; LiveKit meldet Beitritte
  und Austritte per Webhooks an den Server.
- **`sora`.** Der Grant enthält `voice_sora_signaling_urls` und einen signierten Beitrittswert. Der Browser sendet seine
  Signale und sein Audio an den Sora-Server, der seine eigenen STUN- und TURN-Dienste mitbringt. Sora fragt vor jeder
  Verbindung den Auth-Webhook des Servers, der Server steuert die Räume über die API von Sora, und Sora meldet
  Verbindungen per Webhooks.

Hinweise zur Wahl:

- Bei `p2p` läuft das Audio nie über diesen Server (es läuft über den TURN-Server, wenn einer genutzt wird). Die Last des Servers sind nur die Raumnachrichten, Heartbeats und Streams.
- Ein kleiner Server ist mit `p2p` oder mit dem kostenlosen Build-Tarif von LiveKit Cloud am besten bedient (siehe [LiveKit Cloud](#livekit-cloud)).
- Bei `livekit` oder `sora` werden die STUN- und TURN-Einstellungen und die ICE-Richtlinie unten nicht genutzt (der Server warnt beim Start, wenn du sie setzt), weil der Medienserver seine eigenen Server mitbringt. Die IP-Adressen der Teilnehmer sind dann nur für den Medienserver sichtbar, nicht für die anderen Teilnehmer.
- Die Build-Tags `no_voice_livekit` und `no_voice_sora` lassen die beiden Medienserver-Methoden aus einer Binärdatei weg ([Build-Tags](../reference/build-tags.md)).
- Liefere den Server über HTTPS aus: Browser erlauben den Mikrofonzugriff nur auf sicheren Seiten.

Die Limits halten die Kosten eines bezahlten Medienservers begrenzt. `0` entfernt jedes davon außer dem letzten:

| Einstellung | Standard | Bedeutung |
| --- | --- | --- |
| `voice_max_room_duration` | `4h` | Wie lange ein Raum höchstens offen bleibt. Ein Raum, der sie erreicht, wird beendet. |
| `voice_max_open_rooms` | `10` | Wie viele Räume auf diesem Server gleichzeitig offen sein dürfen. |
| `voice_sfu_max_participants` | `10` | Die Raumgröße für `livekit` und `sora`. |
| `voice_p2p_max_participants` | `10` | Die Raumgröße für `p2p`, von 2 bis 50. |

Wer Räume öffnen darf, bestimmt die Servereinstellung `voice_room_starters` ([Servereinstellungen](instance-settings.md)): Standardmäßig nur Inhaber der Berechtigung `start_voice_rooms` (die eingebaute Rolle `admin` hat sie); `everyone` erlaubt jedem angemeldeten lokalen Nutzer, der nicht gesperrt oder eingeschränkt ist, Räume zu öffnen.

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

<!-- source: size-and-bandwidth 9872bf829e26 -->
### Größe und Bandbreite

Zwei Browser verbinden sich miteinander, wenn mindestens einer von ihnen spricht; zwei Personen, die nur zuhören, verbinden sich nicht.
Ein Sprecher sendet daher an jeden anderen Teilnehmer eine eigene Kopie seines Audios. Sprache in Opus braucht mit Overhead etwa 30 bis 50 kbit/s, ein Sprecher in einem Raum mit 10 lädt also etwa 9 Kopien hoch, einige hundert kbit/s,
und ein Sprecher in einem Raum mit 50 lädt 49 Kopien hoch, rund 2 Mbit/s, was ein Handy mit mobilen Daten oder ein Heimanschluss möglicherweise nicht schafft. Die Zahl der Verbindungen in einem Raum mit N Personen ist höchstens N(N-1)/2: 45 bei 10 und 1.225 bei 50.
Deshalb ist der Standard 10. Erhöhe `voice_p2p_max_participants` nur, wenn deine Nutzer die Upload-Bandbreite haben; nutze `livekit` oder `sora` für große Räume.

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

<!-- source: what-p2p-cannot-enforce 31e06affcd4f -->
### Was P2P nicht durchsetzen kann

In einem Raum dürfen nur manche Teilnehmer sprechen: die Hosts und die Zuhörer, die ein Host zulässt. Bei `p2p` wird das an zwei Stellen durchgesetzt: Der Server weigert sich, ein Verbindungsangebot weiterzuleiten, das Audio von jemandem senden würde, der nicht sprechen darf, und die anderen Browser spielen kein Audio eines Teilnehmers ab, der kein Sprecher ist.

Das hält gewöhnliche Nutzer und den offiziellen Web-Client auf, ist aber keine vollständige Verteidigung. **Zwei Teilnehmer mit veränderten Clients, die zusammenarbeiten, können trotzdem Audio austauschen**: Ein veränderter Client von jemandem, der nicht sprechen darf, kann es senden, und ein veränderter Client eines Empfängers kann es abspielen, und der Server kann nicht in die verschlüsselten Medien hineinsehen.
Wenn du garantieren musst, wer spricht, zum Beispiel bei moderierten öffentlichen Veranstaltungen, nutze `livekit` oder `sora`, wo der Medienserver selbst das Audio von jedem ohne Erlaubnis verwirft.

<!-- source: deployment-notes b0f03da306b7 -->
## Hinweise zum Deployment

- **Zwei Streams pro Nutzer**: Während eines Anrufs hält ein Teilnehmer zusätzlich zu dem Stream, den die App immer öffnet, den eigenen Ereignisstrom des Raums (Server-Sent Events). Dimensioniere die Verbindungslimits für zwei langlebige Anfragen pro Nutzer in einem Anruf.
- **Mehrere Prozesse**: Nutze einen gemeinsamen Stream-Adapter (`stream` auf `postgres`, `valkey`, `sns` oder `pubsub` gesetzt), wie beim Rest der App. Bei `stream = hub` und einer `role` außer `all` warnt der Server beim Start, dass der Sprachchat nicht über Prozesse hinweg funktioniert.
- **Cloud Run**: Ein Stream ist eine Anfrage, und CPU wird zugewiesen, solange sie offen ist. Wenn das Anfrage-Timeout sie abschneidet, verbindet sich der Client neu. Setze das Timeout so lang wie möglich.
- **Medienserver brauchen eine VM**: LiveKit und Sora nehmen Audio über UDP in einem dauerhaft laufenden Prozess entgegen. Cloud Run akzeptiert nur HTTP-Anfragen und kann sie daher nicht hosten. Betreibe sie (und coturn) auf einer VM oder nutze LiveKit Cloud, Sora Cloud oder Cloudflare TURN.

<!-- source: livekit b16c1d192419 -->
## LiveKit

Setze `voice = livekit` und:

| Einstellung | Wert |
| --- | --- |
| `voice_livekit_url` | Die `wss:`-URL, mit der sich Browser verbinden, zum Beispiel `wss://livekit.example.org`. |
| `voice_livekit_api_url` | Die `https:`-URL, die dieser Server aufruft, wenn sie sich von der obigen unterscheidet. Standardmäßig ist es derselbe Host mit `https:`. |
| `voice_livekit_api_key`, `voice_livekit_api_secret` | Ein Schlüsselpaar des LiveKit-Servers. Das Secret signiert die Zugriffstoken der Teilnehmer und verifiziert die Webhooks. |

Der Server legt jeden Raum bei LiveKit an, benennt ihn mit der Raum-ID und gibt jedem Teilnehmer ein Zugriffstoken, das 60 Sekunden gilt. Das Token eines Zuhörers kann nichts veröffentlichen; das eines Sprechers kann nur ein Mikrofon veröffentlichen.

<!-- source: self-hosted-livekit 2b118136f4dc -->
### Selbst gehostetes LiveKit

In `livekit.yaml`:

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

- Der LiveKit-Server muss die Webhook-URL über HTTPS erreichen. Der Server verifiziert die Signatur jedes Webhooks mit dem API-Secret.
- Webhooks beschleunigen die Dinge nur. Jede Minute vergleicht der Server die Teilnehmer jedes offenen Raums mit dem, was LiveKit meldet, und entfernt jeden, der nicht dort sein sollte; ein Webhook, der verloren ging, während der Server kalt war, wird so nachgeholt.
- Um es auf deiner Maschine auszuprobieren, startet `livekit-server --dev` mit einem festen Entwicklungs-Schlüsselpaar.
- **Ein entfernter Teilnehmer kann sich kurz wieder anschließen.** Selbst gehostetes LiveKit kann ein Token nicht widerrufen: Es schickt verbundenen Clients laufend frische Tokens, jedes bis zu etwa 10 Minuten gültig. Jemand, der entfernt wird, kann sich daher mit seinem neuesten Token bis zu etwa 10 Minuten wieder anschließen, bis der Server es bemerkt (der Join-Webhook oder die Prüfung im Minutentakt) und ihn trennt, und er kann in der Zwischenzeit einige Sekunden bis eine Minute lang zu hören sein. Nutze LiveKit Cloud, wenn das Entfernen sofort wirken muss: Es widerruft das Token sofort.

<!-- source: livekit-cloud 5bd9f833be11 -->
### LiveKit Cloud

Verwende die WebSocket-URL, den API-Key und das Secret des Projekts und füge `https://<your domain>/voice/webhooks/livekit` in den Einstellungen des Projekts als Webhook-Endpunkt hinzu.
Der kostenlose Build-Tarif bietet 5.000 Minuten und 50 GB pro Monat sowie 100 gleichzeitige Verbindungen, Stand 2026-10-26; die aktuellen Tarife findest du auf der Preisseite von LiveKit.

<!-- source: sora 5e9209dd3baa -->
## Sora

**Der Sora-Adapter wird nur gegen ein simuliertes Sora getestet, das der Dokumentation von Sora folgt, nicht gegen einen echten Sora-Server.** Probiere ihn auf einem Staging-Server mit einem echten Sora aus, bevor du dich darauf verlässt.

Setze `voice = sora`, `voice_sora_signaling_urls` (kommagetrennte `wss:`-URLs, mit denen sich Browser verbinden), `voice_sora_join_secret` (mindestens 32 Bytes, zum Beispiel aus `openssl rand -base64 32`; es signiert die Werte, mit denen Teilnehmer beitreten) und `voice_sora_mode` (`self_hosted` oder `cloud`).
Der Server muss über HTTPS ausgeliefert werden, weil Sora `https://<your domain>` zurückruft.

<!-- source: webhooks 11b2c486a476 -->
### Webhooks

Sora ruft zwei Endpunkte dieses Servers auf:

| Endpunkt | Zweck |
| --- | --- |
| `/voice/webhooks/sora/auth` | Der Auth-Webhook. Sora ruft ihn vor jeder Verbindung auf. Der Server erlaubt sie nur für einen Teilnehmer, der im Raum ist, nicht entfernt wurde, sprechen darf, wenn er sich als Sprecher verbindet, und innerhalb des Größenlimits liegt, und er weist Sora an, nur Audio zu akzeptieren. |
| `/voice/webhooks/sora/events` | Die Ereignis- und Sitzungs-Webhooks (Verbindungen angelegt und beendet, Sitzungen beendet). |

**Der Auth-Webhook muss rechtzeitig antworten.** Sora wartet 5 Sekunden bei Selbst-Hosting und 10 Sekunden bei Sora Cloud. Der Server antwortet aus seinen eigenen Daten, ohne etwas anderes aufzurufen, aber ein Kaltstart des Servers (Cloud Run skaliert von null hoch) kann das überschreiten, und die Verbindung wird dann abgelehnt. Halte den Server warm, solange der Sprachchat genutzt wird.

Setze beim Selbst-Hosting `default_duplicate_client_id = evict` in `sora.conf`. Ein Zuhörer, der zum Sprecher wird, verbindet sich unter derselben Identität neu (Sora kann die Rolle einer Verbindung nicht ändern), und `evict` lässt Sora die alte Verbindung verwerfen, wenn die neue eintrifft.
Beim Standard `allow` trennt der Server die alte Verbindung selbst, sobald er von der neuen erfährt, sodass sich beide kurz überlappen.

<!-- source: self-hosted-sora dd3244368b73 -->
### Selbst gehostetes Sora

| Einstellung | Wert |
| --- | --- |
| `voice_sora_mode` | `self_hosted` |
| `voice_sora_api_url` | Die `https:`-URL eines Reverse-Proxys vor der HTTP-API von Sora. |
| `voice_sora_api_header_name`, `voice_sora_api_header_value` | Der Header, mit dem sich der Proxy bei diesem Server authentifiziert, zum Beispiel `Authorization` und `Basic ...`. |
| `voice_sora_webhook_username`, `voice_sora_webhook_password` | Die Basic-Zugangsdaten der Webhooks. Erforderlich. |

**Die HTTP-API von Sora selbst hat keine Authentifizierung** und soll nur auf der Loopback-Adresse lauschen. Mache sie nie öffentlich zugänglich. Stelle einen Reverse-Proxy davor, der Zugangsdaten verlangt, und gib diese Zugangsdaten über die beiden Header-Einstellungen an diesen Server.
Konfiguriere die Webhook-URLs in Sora mit denselben Basic-Zugangsdaten. Der Server akzeptiert nur eine API-URL mit `https:` (`http:` nur für localhost) und startet nicht ohne die Webhook-Zugangsdaten. mTLS wird nicht unterstützt, weil Cloud Run und der Load Balancer vor ECS TLS vor dem Server beenden.

<!-- source: sora-cloud ac4935c7f7a7 -->
### Sora Cloud

Setze `voice_sora_mode = cloud`, `voice_sora_cloud_project_id` und `voice_sora_cloud_api_key`. Der Server signiert die Zugriffstoken mit dem Schlüssel, ruft die API damit auf und verifiziert damit die Webhook-Signaturen; Basic-Zugangsdaten sind nicht nötig.

<!-- source: behavior-to-expect 03fd5e1655fa -->
### Zu erwartendes Verhalten

- Ein Zuhörer, der sprechen darf, verbindet sich automatisch als Sprecher neu; es gibt also eine kurze Lücke, während die neue Verbindung startet.
- Ein Sprecher, dem das Recht entzogen wird, wird sofort durch einen Forwarding-Filter stummgeschaltet und dann als Zuhörer neu verbunden.

<!-- source: licensing 534e304817e3 -->
### Lizenzierung

Sora ist ein kommerzielles Produkt von Shiguredo. Stand 2026-10-26: Sora Cloud hat eine Grundgebühr von 5.000 ¥ pro Monat und keine kostenlose Phase; Selbst-Hosting braucht eine Paketlizenz, im Voraus bezahlt für eine Zahl gleichzeitiger Verbindungen und einen Zeitraum. Auf der Produktseite steht, dass es Unternehmen, Organisationen oder Einzelpersonen außerhalb Japans nicht angeboten wird; prüfe also, ob du es kaufen darfst, bevor du darum herum planst. LiveKit ist bei Selbst-Hosting Open Source.

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
