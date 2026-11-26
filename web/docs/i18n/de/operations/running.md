<!-- source: running-the-server-start-stop-and-health-checks 5164430b3203 -->
# Den Server betreiben: Start, Stopp und Health-Checks

So startest und stoppst du den Server, beziehst sein Image und beobachtest ihn im Betrieb: Metriken, Traces, die Job-Warteschlange und Health-Checks.

<!-- source: start-the-server c19e1ea32f07 -->
## Den Server starten

```sh
my config check   # report every configuration problem at once
my config check -instance   # also warn about instance settings that are not set (opens the database)
my serve          # start according to the configured role (-config <file> reads a TOML file)
my version        # print the version of the binary
```

`my serve` validiert die Konfiguration, baut die Adapter für Datenbank, Job-Queue und Schlüsselverwaltung auf und nimmt HTTP auf `listen` an.
Mit `tls = off`, dem Standard, spricht es dort reines HTTP/1.1: Setze einen Reverse-Proxy oder Load Balancer davor, der HTTPS (und HTTP/2 oder HTTP/3 für die Clients) beendet und per HTTP/1.1 weiterleitet.
Um HTTPS ohne Proxy auszuliefern, siehe [HTTPS vom Server aus ausliefern](#serving-https-from-the-server).
Die Echtzeit-Aktualisierungen (`GET /api/v1/streaming`, die Streams der lokalen und der föderierten Timeline und die Streams der Sprachchat-Räume) sind langlebige Antworten, die alle 30 Sekunden (in einem Sprachraum alle 15) einen Heartbeat-Kommentar senden und im Leerlauf offen bleiben.
Der Proxy darf diese Antworten nicht puffern (nginx: Der Server sendet `X-Accel-Buffering: no`), und sein Idle- oder Read-Timeout muss länger sein als das Heartbeat-Intervall.
Er wendet das Schema nicht an und führt die Daten-Upgrade-Schritte nicht aus: Führe vor dem Start `my schema apply` und dann `my db upgrade` aus (siehe [Datenbank](database.md); Upgrades führen beides als Teil von [Upgraden](database.md#upgrading) aus). Bis `my db upgrade` jeden Schritt des Releases aufgezeichnet hat, antwortet `/readyz` mit `503`.
Im Docker-Compose-Setup (`deploy/docker/compose.yaml`; siehe [Bereitstellung mit Docker Compose](deploy-docker-compose.md)) führen die einmaligen Dienste `schema` und `upgrade` `my schema apply` und `my db upgrade` aus, bevor der Server startet.
Ist das Schema nicht angewendet, startet der Prozess trotzdem und die Bereitschaftsprüfung liefert 503, und er liefert nichts anderes aus, bis das Schema da ist.

Das erste `my serve` oder `my`-Kommando, das die Datenbank verwendet, trägt `host` und `account_domain` darin ein. Weicht die Konfiguration von den eingetragenen Werten ab, protokolliert `my serve` `start failed` mit beiden Wertepaaren und beendet sich mit Exit-Code 1, und die Kommandos geben denselben Fehler aus. Ausgenommen sind `my schema apply` und `my db backup`, die nicht prüfen, und der Wartungsmodus, der die Datenbank nicht liest. `my config check -instance` und `my db upgrade -check` prüfen, tragen aber nichts ein.
Ein Prozess, der die Datenbank beim Start nicht lesen konnte, ist nicht bereit, bis er es kann, und führt bis dahin keine Jobs aus und antwortet auf jede Anfrage außer den Health Checks (`/healthz`, `/readyz`, `/health/live`, `/health/ready`, `/api/v1/instance/health`) mit `503` und `Retry-After`. Das tut auch ein Server, der vor `my schema apply` gestartet wurde. Findet er dann andere Werte, hält er mit demselben Fehler an.

Im Profil `production` prüft `my config check` außerdem, ob die gültigen Einstellungen zu einer Produktionsinstanz passen: Fehler, etwa ein `host`, den andere Server nicht erreichen können, oder ein KEK, der nicht zufällig ist, lassen es mit 1 enden, Warnungen, etwa `mail = none`, nicht.
`my serve` protokolliert beides beim Start als Warnungen, startet aber trotzdem, führe `my config check` daher als Schritt der Bereitstellung aus.
Die [Produktions-Checkliste](production-checklist.md) listet die Prüfungen auf und was du vor dem Livegang von Hand überprüfen solltest.

| `role` | HTTP | Job-Worker |
| --- | --- | --- |
| `all` (Standard) | Alle Routen | Läuft |
| `web` | Alle Routen | Läuft nicht |
| `worker` | Nur Health Checks (für Plattform-Probes) | Läuft |
| `sfu` | Health Checks und der WebSocket des Medienknotens von `voice = sfu`, der das Audio auf eigenen UDP- und TCP-Ports entgegennimmt ([Sprachchat](voice.md#run-media-nodes)) | Läuft nicht |

Logs gehen als JSON Lines auf die Standardfehlerausgabe. Jede Zeile hat `level` (`DEBUG`, `INFO`, `WARN`, `ERROR`) und `severity`, dieselbe Stufe in der Form, die Cloud Logging liest (`DEBUG`, `INFO`, `WARNING`, `ERROR` und `CRITICAL` für einen Fehler, der den Prozess beendet).
Logs, die beim Bearbeiten einer Anfrage geschrieben werden, tragen eine `request_id`, und dieselbe ID wird im Antwort-Header `X-Request-Id` zurückgegeben.
Hat die `X-Request-Id` der Anfrage höchstens 128 Zeichen aus Buchstaben, Ziffern und `-_.:`, wird sie unverändert verwendet, als die von einem Load Balancer vergebene ID.
Logs innerhalb eines Traces (Anfragen und Jobs) tragen außerdem `trace_id` und `span_id` (siehe [Traces](#traces)).
Mit `logs_exporter = gcp`, für Cloud Run, tragen die Zeilen außerdem die Felder von Cloud Logging, die eine Zeile mit ihrem Trace in Cloud Trace verknüpfen, und jede Zugriffszeile ihre Anfrage als `httpRequest`; die übrigen Felder bleiben, wie sie sind.

Mit `logs_exporter = otlp` sendet der Server die Zeilen als OpenTelemetry-Log-Records über OTLP/HTTP an den Empfänger unter `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` oder `OTEL_EXPORTER_OTLP_ENDPOINT` (Standard `http://localhost:4318`), mit `OTEL_EXPORTER_OTLP_HEADERS`.
Jeder Record trägt die Felder seiner Zeile als Attribute und den Trace und Span seiner Zeile, sodass ein OpenTelemetry Collector, an den jeder Host sendet, die Logs aller Hosts an einem Ort sammelt, neben ihren Traces (`traces_exporter = otlp`).
Die Zeilen, die geschrieben werden, bevor der Server gestartet ist, und die Records, die der Empfänger nicht angenommen hat, gehen weiterhin auf die Standardfehlerausgabe, gefolgt von einer Zeile `otlp logs export failed`.
Ein Schwall von mehr als 2.048 Zeilen, während der Empfänger langsam ist, verwirft die Zeilen, die nicht in die Sendewarteschlange passen.
Eine minimale Pipeline des Collectors für die Logs:

```yaml
receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:4318
exporters:
  otlphttp:
    endpoint: http://loki:3100/otlp # any receiver of OTLP logs, such as Loki
service:
  pipelines:
    logs:
      receivers: [otlp]
      exporters: [otlphttp]
```

Jede HTTP-Anfrage und jede Job-Ausführung schreibt am Ende eine Zeile. Die Zeile hat dieselben Namen und Werte wie die Labels der [Metriken](#metrics) unten,
sodass eine Plattform mit `metrics_exporter = none` aus diesen Zeilen logbasierte Metriken (Zähler und Verteilungen) bauen kann.

| Zeile (`msg`) | Felder |
| --- | --- |
| `http request` (`INFO`; Health Checks verwenden `DEBUG`) | `http.request.method`, `http.route`, `my.surface` und `http.response.status_code` (eine Zahl) haben dieselben Werte wie die Labels von `http.server.request.duration`. Dazu `duration_seconds` (eine Zahl in Sekunden), `url.path` (der Pfad ohne Query-String), `http.response.body.size` und `request_id`. `http.route` und `my.surface` fehlen bei Anfragen, die keine Route erreicht haben (Health Checks, vor dem Routing abgelehnte Anfragen), und die Metrik zählt sie nicht. Eine Anfrage, deren Client ging, bevor der Server antworten konnte, hat den Status `499` statt eines 5xx, außer wenn ein Handler in Panik geriet; der Client hat nichts erhalten, daher zählt sie nicht als Serverfehler |
| `job` (`INFO` bei Erfolg und bei `postponed`, `WARN` bei `retry`, `ERROR` bei `dead`) | `my.job.type` und `my.outcome` haben dieselben Werte wie die Labels von `my.job.duration`. Dazu `duration_seconds` (`my.job.duration`), `wait_seconds` (`my.job.wait`), `job_id`, `attempt` und bei einem Fehler `error` |
| `error report` (`ERROR`; `CRITICAL` bei einem Fehler, der den Prozess beendet) | Eine Zeile pro gemeldetem Fehler ([Fehlerberichte](#error-reports)) |

Mit `metrics_exporter = none` tragen die Zeilen `http request` und `job` außerdem die [Systemmetriken](#system-metrics) der Adapter und des Datenbank-Pools.
Wenn die Anfrage oder der Job Operationen von Queue, Key-Value-Store, Stream oder Blob-Store ausgeführt hat: `my.adapter.operations.total`, `my.adapter.errors.total` und
`my.adapter.errors`, die Fehler nach `my.adapter` (etwa `{"kv.valkey": 2}`). `db.client.connection.usage`, die genutzten Verbindungen im Verhältnis zum
Maximum des Pools, wenn er eines hat. Wenn nicht null: `my.db.client.connection.waits`, `db.client.connection.wait_time` (Sekunden) und `db.client.connection.timeouts`: die Zunahme
seit der vorherigen Zeile des Prozesses, sodass ihre Summe über die Zeilen die Gesamtzahl des Prozesses ist.

<!-- source: error-reports b70c69ddef24 -->
### Fehlerberichte

Die Anleitung [Fehlerberichte](error-reporting.md) behandelt, wo du diese Zeilen auf jeder Plattform liest, den Fehler-Bildschirm, Benachrichtigungen und das Senden von Berichten an einen Error-Tracker.

Der Server meldet die Fehler, die die Aufmerksamkeit des Betreibers brauchen, als je eine `error report`-Zeile:

- eine Panic in einem HTTP-Handler, einem Job oder einer Hintergrundaufgabe;
- ein Fehler hinter einer 5xx-Antwort der Client-API, der Admin-API, der Anmeldeseiten, von ActivityPub, OAuth oder Medien;
- ein Job, der endgültig fehlschlägt (`dead`): Ihm gingen die Wiederholungen aus, oder er weigerte sich, erneut zu laufen. Zustellungen, die aufgaben, weil der andere Server weiter fehlschlug, werden nicht gemeldet;
- ein Fehler beim Start: eine ungültige Konfiguration oder eine Datenbank oder ein Adapter, der sich nicht einrichten lässt.

4xx-Antworten, Job-Wiederholungen, der Wartungsmodus, fehlgeschlagene Readiness-Prüfungen, Anfragen, deren Client wegging, und Fehler, die andere Server verursachen, werden nicht gemeldet; die Metriken und die anderen Logzeilen decken sie ab.

Geht der Client einer Anfrage weg, werden die anderen Fehlerzeilen des Servers für diese Anfrage mit `INFO` statt `ERROR` geschrieben, da die meisten von ihnen nur sagen, dass die Anfrage abgebrochen wurde.

Die Zeile hat die Form, die Google Cloud Error Reporting liest: Auf Cloud Run erscheinen die Berichte ohne jede Einrichtung als Fehler gruppiert in Error Reporting. Auf anderen Plattformen ist sie eine einfache JSON-Zeile:

| Feld | Wert |
| --- | --- |
| `@type`, `serviceContext` | Der Ereignistyp von Error Reporting und `{"service": "my", "version": "<version>"}` |
| `my.error.fingerprint` | 32 Hexadezimalziffern, die bei jedem Auftreten desselben Fehlers gleich sind: gebildet aus der Quelle, der Stelle, dem Fehlertyp und den Funktionen des Servers im Stack, ohne Zeilennummern |
| `my.error.source`, `my.error.kind` | `server` (oder `web`) sowie `panic` oder `error` |
| `my.error.where` | Wo es geschah: `<surface> <method> <route>` bei einer Anfrage (etwa `client_api GET /api/v1/statuses/{id}`), der Job-Typ bei einem Job, der Aufgabenname bei einer Hintergrundaufgabe, `startup` bei einem Fehler beim Start |
| `my.role`, `deployment.environment.name` | Die `role` des Prozesses und `errors_environment` |
| `request_id`, `job_id`, `attempt`, `trace_id`, `span_id` | Die IDs, um die Anfrage- oder Job-Zeilen desselben Fehlers zu finden, wenn er sie hat |
| `context.httpRequest` | Bei einem Fehler einer Anfrage: `method`, `url` (die Route, etwa `/api/v1/statuses/{id}`, nie der Pfad, den der Client angefragt hat) und `responseStatusCode`, der beantwortete Status (500) |
| `message` | Zuletzt in der Zeile: `<type>: <message>`, ein Zeilenumbruch und der Go-Stack |

Bevor ein Bericht den Prozess verlässt, werden in der Meldung URLs, E-Mail-Adressen, `@user@host`-Handles, IP-Adressen, UUIDs und ULIDs sowie Zahlen mit 10 oder mehr Ziffern durch `<url>`, `<email>`, `<acct>`, `<ip>`, `<id>` und `<n>` ersetzt, und sie wird auf 1 KiB gekürzt.
Der Stack behält seine Dateipositionen, aber nicht die Argumentwerte seiner Funktionszeilen (sie werden als `(...)` gezeigt), und wird auf 16 KiB gekürzt. Kein Bericht trägt einen Request-Body, Header, Query, Cookie, ein Konto, einen Benutzernamen, eine IP-Adresse oder einen User-Agent.
Keine andere Zeile trägt einen Stack, daher findet Error Reporting die Stacks nur dieser Zeilen.

Ein Feldname ist ein einzelner Schlüssel, der `.` enthält; er ist nicht verschachtelt. Setze ihn in Cloud-Logging-Filtern und -Extraktionen in Anführungszeichen (zum Beispiel `jsonPayload."http.route"` und `EXTRACT(jsonPayload.duration_seconds)` für den Wert einer Verteilung).
Felder, die als Labels verwendet werden, nehmen nur Werte aus einer endlichen Menge an; `url.path`, `request_id` und `job_id` sind nie Labels.

Mit `metrics_exporter = emf` sind diese beiden Zeilen selbst die EMF-Zeilen. Die Zeile `http request` trägt den Wert von `http.server.request.duration`, und die Zeile `job` trägt die Werte von `my.job.duration` und `my.job.wait` (Schlüssel nach den Metriken benannt),
dazu die Direktive `_aws`, `service.name`, `service.version`, `my.role` und `service.namespace` (der `host`). Diese drei Metriken werden in keiner anderen Zeile geschrieben. Felder, die du in `metrics_emf_dimensions` wählst, werden als Strings geschrieben, wie EMF es verlangt (zum Beispiel ist die Dimension `http.response.status_code` `"200"`).
Logs gehen auf die Standardfehlerausgabe und die anderen EMF-Zeilen auf die Standardausgabe. Der Log-Treiber `awslogs` von ECS sendet beide an CloudWatch Logs, sodass Metriken aus beiden extrahiert werden.

<!-- source: docker-compose-files eab514676a87 -->
## Docker-Compose-Dateien

`deploy/docker/` enthält, was eine Instanz bereitstellt. `compose.yaml` läuft allein, und die anderen Dateien werden darübergelegt.

| Datei | Fügt hinzu | Verwende sie, wenn |
| --- | --- | --- |
| `compose.yaml` | Den Server und seine einmaligen Datenbankdienste, auf SQLite und Mediendateien im Volume `data` | Immer |
| `compose.postgres.yaml` | PostgreSQL in einem Container, im Volume `postgres` | Du PostgreSQL auf dieser Maschine willst. Für einen PostgreSQL-Server anderswo lass sie weg und setze `MY_DATABASE` und `MY_DATABASE_URL` in `.env` |
| `compose.s3.yaml` | Medien in einem S3-kompatiblen Bucket; mit `COMPOSE_PROFILES=garage` zusätzlich ein [Garage](https://garagehq.deuxfleurs.fr/)-Speicher mit einem Knoten im Volume `garage` | Die Medien eine Platte übersteigen oder du sie in einem Bucket willst. Ohne das Profil ist der Speicher der externe der Einstellungen `MY_BLOB_S3_*` |
| `compose.caddy.yaml` | Caddy, mit TLS-Zertifikaten auf den Ports 80 und 443 | `MY_HOST` eine öffentliche Domain ist und auf dieser Maschine nichts anderes TLS beendet |
| `compose.sfu.yaml` | Sprachchat mit einem Medienknoten: Der Server teilt sich in `role = web`, `worker` und `sfu`, und Caddy leitet den WebSocket des Knotens an den Knoten. Braucht `compose.postgres.yaml` und `compose.caddy.yaml` | Du `voice = sfu` willst ([Sprachchat](voice.md#docker-compose)) |
| `compose.build.yaml` | Baut das Image aus einem Checkout | Du deinen eigenen Build verwendest |

Wähle die Datenbank und den Medienspeicher vor dem ersten Start: Die Daten ziehen nicht von selbst um, wenn sich die Dateien ändern
([Sichern](backup.md) zeigt, was umziehen muss).

**Lege Dateien übereinander** mit `-f`-Optionen in dieser Reihenfolge, oder nenne sie einmal in `.env`, damit jedes spätere `docker compose`-Kommando
(`exec`, `logs`, `pull` und die in den anderen Anleitungen) dieselben Dateien verwendet:

```sh
cd deploy/docker
cp example.env .env
# In .env: COMPOSE_FILE=compose.yaml:compose.postgres.yaml:compose.caddy.yaml
```

Eine Datei namens `compose.override.yaml` im Verzeichnis wird nur gelesen, wenn weder `-f` noch `COMPOSE_FILE` Dateien nennt. Führe sie
dann ebenfalls auf.

**Einstellungen.** Kopiere `example.env` nach `.env`. Compose liest `.env`, um die Variablen seiner Dateien zu füllen, und gibt die gesamte
Datei an die Container des Servers weiter, sodass jede Einstellung der [Konfigurationsreferenz](../reference/configuration.md) funktioniert,
indem sie dort gesetzt wird. Die Vorlage listet sie alle auf, mit je einem Satz dazu.

- Der Schlüsselverschlüsselungsschlüssel bleibt aus `.env` heraus, in der Datei `secrets/kek`, und die Container lesen ihn als Docker-Secret
  `/run/secrets/my_kek`, sodass er nie zu einer Umgebungsvariable wird. Eine Zeile `MY_KEK` in `.env` hält den Server an, der
  `MY_KEK` und `MY_KEK_FILE` zusammen ablehnt ([den Schlüssel verwalten](key-management.md)).
- Der Server läuft als Benutzer `65532`, und Compose bindet die Datei mit den Rechten ein, die sie hat, daher muss die Datei
  für andere lesbar sein. Lege sie in einem Verzeichnis ab, das nur du betreten kannst, wie es die Kommandos in `example.env` tun.
- Die anderen Secrets erreichen die Container als Umgebungsvariablen. Um eines stattdessen als Datei zu übergeben, binde es mit einem Secret
  in einer eigenen `compose.override.yaml` ein und setze sein `<NAME>_FILE`, wie es `compose.yaml` für den Schlüssel tut.
- `.env` enthält Secrets: `chmod 600 .env`, und bewahre eine Kopie bei deinen Backups auf.
- `MY_LISTEN` bleibt `:8080`, da die Dateien sich mit diesem Port verbinden, es sei denn, der Server liefert HTTPS selbst aus
  ([HTTPS vom Server aus ausliefern](#serving-https-from-the-server)).

**TLS mit Caddy.** Setze `MY_HOST` auf eine Domain, deren DNS auf die Maschine zeigt, mit aus dem Internet erreichbaren Ports 80 und 443,
und entferne `MY_WEB_ORIGIN` aus `.env`. Caddy besorgt und erneuert das Zertifikat bei der ersten Anfrage selbst.
Der Server vertraut dem `X-Forwarded-For` der Standard-Adresspools von Docker; setze `MY_TRUSTED_PROXIES`, wenn deine abweichen.

**Das mitgelieferte Garage.** `compose.s3.yaml` mit `COMPOSE_PROFILES=garage` braucht die Werte, die `example.env` am Anfang auflistet.
Der einmalige Dienst `garage-init` bereitet den Speicher bei jedem `up` vor. Er bleibt innerhalb des Compose-Netzwerks, da der Server
die Medien selbst ausliefert. Ein Knoten ist eine Platte: Sichere das Volume `garage` oder den Bucket ([Sichern](backup.md#media-in-a-bucket)).

<!-- source: serving-https-from-the-server 7b583c946805 -->
## HTTPS vom Server aus ausliefern

`my` kann HTTPS selbst ausliefern, sodass eine kleine Maschine den ganzen Server als eine Binärdatei mit einer Domain und den Ports 80 und 443
ohne Reverse-Proxy betreibt. Mit `tls = acme` besorgt es seine Zertifikate bei einer ACME-Zertifizierungsstelle wie Let's
Encrypt. Mit `tls = files` liefert es ein Zertifikat und einen Schlüssel aus, die du bereitstellst und mit einem anderen Werkzeug, etwa
certbot, aktuell hältst.

```sh
MY_TLS=acme
MY_TLS_ACME_AGREE_TERMS=true            # you agree to the terms of service of the certificate authority
# MY_TLS_ACME_EMAIL=ops@social.example  optional contact of the account
# MY_LISTEN=:443          the default with tls other than off
# MY_TLS_HTTP_LISTEN=:80  the default; empty opens no port 80
```

```sh
MY_TLS=files
MY_TLS_CERT_FILE=/etc/letsencrypt/live/social.example/fullchain.pem
MY_TLS_KEY_FILE=/etc/letsencrypt/live/social.example/privkey.pem
```

- **Ports.** Der Server liefert HTTPS auf `listen` aus (standardmäßig `:443`, sobald `tls` nicht `off` ist). Auf `tls_http_listen` (standardmäßig `:80`)
  leitet er `GET`- und `HEAD`-Anfragen mit `301` an denselben Pfad auf `https` weiter und beantwortet jede andere
  Methode mit `400`, damit kein Anfrage-Body im Klartext unterwegs ist.
- **Hosts.** Der Server bedient `host` und den Host von `web_origin`, wenn der abweicht. Ein Handshake für jeden anderen Namen
  schlägt ohne Zertifikat fehl und veranlasst den Server nie, eines zu bestellen.
- **Verbindungen.** TLS 1.2 oder neuer mit den Standard-Cipher-Suites von Go, HTTP/2 und HTTP/1.1, höchstens 64 KiB Anfrage-Header
  und höchstens 100 gleichzeitige Streams auf einer HTTP/2-Verbindung. Die Timeouts sind die von reinem HTTP. Der Server
  fügt weiterhin jeder Antwort `Strict-Transport-Security` hinzu ([Sicherheits-Header](#security-headers)).
- **Rollen.** Nur die Rollen `web`, `all` und `sfu` hören mit TLS zu. Ein Medienknoten (`role = sfu`) bedient den Host von
  `voice_sfu_public_url` statt `host`. Ein `worker` liefert seine Health Checks in reinem HTTP auf `listen` (standardmäßig `:8080`)
  aus und öffnet keinen Port 80.
- **Client-Adressen.** Ohne Proxy davor ist die Gegenstelle der Verbindung der Client: Lass `trusted_proxies` leer.
  Setze es nur, wenn sich ein CDN davor über `https` mit dem Server verbindet.
- **Wartungsmodus.** `my serve -maintenance` liest weder `tls` noch die Zertifikate und antwortet in reinem HTTP auf
  `listen` (`:8080`, wenn du nichts setzt), richte `MY_LISTEN` und deine Firewall daher auf diesen Port aus, solange er läuft, oder behalte einen
  Reverse-Proxy für die Wartung.

<!-- source: certificates-from-an-acme-certificate-authority 52037a75d641 -->
### Zertifikate von einer ACME-Zertifizierungsstelle

Mit `tls = acme` bestellt der Server für jeden seiner Hosts, der keines hat, ein Zertifikat bei der Zertifizierungsstelle
unter `tls_acme_directory` (standardmäßig Let's Encrypt).

- **Bevor du anfängst.** `host` muss ein DNS-Name sein, der auf den Server zeigt, und die Ports 443 und 80 müssen aus
  dem Internet erreichbar sein: Die Zertifizierungsstelle prüft, ob der Server den Namen kontrolliert, indem sie sich mit ihm verbindet. Er antwortet auf
  `listen` (die TLS-ALPN-01-Challenge) oder auf `tls_http_listen` (die HTTP-01-Challenge), sodass ein leeres `tls_http_listen`
  mit Port 443 allein trotzdem funktioniert. Wildcard-Zertifikate und die Validierung über DNS-Einträge sind nicht verfügbar.
- **Erster Start.** Der Server beginnt zuzuhören, ohne auf ein Zertifikat zu warten, und bestellt sie dann. Bis ein Host sein
  Zertifikat hat, schlägt ein Handshake dafür fehl; bei Let's Encrypt kommen die ersten Zertifikate meist innerhalb einer Minute. Das Log
  sagt für jeden Host `obtained a TLS certificate` und `the TLS certificate could not be obtained` mit dem Grund, wenn eine
  Bestellung fehlschlägt.
- **Erneuerungen.** Der Server erneuert jedes Zertifikat zu dem Zeitpunkt, den die Zertifizierungsstelle dafür vorschlägt (ACME Renewal
  Information): Er wählt einmal einen zufälligen Zeitpunkt innerhalb des vorgeschlagenen Fensters, behält ihn, solange das Fenster bleibt, und fragt erneut,
  wie das `Retry-After` der Stelle sagt, mindestens alle 12 Stunden, damit eine Stelle, die Zertifikate
  früher ersetzen muss, das Fenster verschieben kann. Bietet die Stelle kein solches Fenster, erneuert der Server nach zwei Dritteln der
  Lebensdauer des Zertifikats oder nach der Hälfte bei einem Zertifikat, das weniger als 10 Tage gültig ist. Eine Erneuerung teilt der
  Stelle mit, welches Zertifikat sie ersetzt, was sie außerhalb der Rate Limits von Let's Encrypt hält. Das aktuelle Zertifikat
  bleibt in Betrieb, bis seine Erneuerung kommt, und eine fehlgeschlagene Erneuerung wartet wie eine erste Bestellung.
- **Fehlgeschlagene Bestellungen.** Nach einer fehlgeschlagenen Bestellung wartet der Server vor der nächsten für denselben Host: 1 Minute nach dem
  ersten Fehler, 10 Minuten nach dem zweiten, 100 Minuten nach dem dritten, dann einen Tag, damit er innerhalb der
  Grenzen der Zertifizierungsstelle bleibt. Die Wartezeit wird in der Datenbank gehalten und übersteht Neustarts. Behebe die Ursache (DNS, eine Firewall,
  ein Port), und die nächste Bestellung geht hinaus, wenn die Wartezeit endet.
- **Wo sie aufbewahrt werden.** Das Konto bei der Zertifizierungsstelle, die Zertifikate und die Aufzeichnung jeder Bestellung
  werden in der Datenbank aufbewahrt; die privaten Schlüssel werden wie die anderen Secrets mit dem Schlüsselverschlüsselungsschlüssel versiegelt
  ([Schlüsselverwaltung](key-management.md)). Ein Backup der Datenbank enthält sie, und ein neu gestarteter Server liefert die
  gespeicherten Zertifikate sofort aus, ohne zu bestellen. Wird der Schlüsselverschlüsselungsschlüssel geändert, werden sie neu verpackt; es werden keine neuen
  Zertifikate bestellt.
- **Mehrere Prozesse.** Prozesse, die die Datenbank und den `kv`-Store teilen, koordinieren sich: Einer bestellt das Zertifikat eines
  Hosts, während die anderen warten und es dann laden, und jeder Prozess, mit dem sich die Zertifizierungsstelle verbindet, beantwortet ihre
  Prüfung. Verwende `kv = db` oder `kv = valkey`; `kv = memory` wird abgelehnt, es sei denn, ein Prozess führt alles aus (`role = all`).
  Der Prozess, der ein Zertifikat erhält, sagt es den anderen über den [`stream`](../reference/configuration.md#stream)-Broker, und sie liefern es innerhalb von
  Sekunden aus; jeder `web`-Prozess mit `tls = acme` hört daher weiter am Broker zu. Mit `stream = hub` oder wenn eine
  Mitteilung verloren geht, liest dennoch jeder Prozess alle 5 Minuten die aktuellen Zertifikate aus der Datenbank, und jede
  Minute, sobald sein Zertifikat seinen Erneuerungszeitpunkt erreicht hat, sodass eine Erneuerung sie lange vor dem Ablauf des alten Zertifikats erreicht.
- **Profile.** `tls_acme_profile` wählt ein Profil, das die Zertifizierungsstelle anbietet, etwa `shortlived` bei Let's
  Encrypt. Leer nimmt ihren Standard. Nach einem Profilwechsel folgt die nächste Erneuerung ihm, und das Zertifikat, das sie bringt,
  wird ausgeliefert, auch wenn es vor dem abläuft, das es ersetzt.

Um es auszuprobieren, ohne die Grenzen der Produktionsumgebung von Let's Encrypt zu verbrauchen, beginne mit deren Staging-Umgebung,
die Zertifikate ausstellt, denen Browser nicht vertrauen, unter viel höheren Grenzen:

```sh
MY_TLS_ACME_DIRECTORY=https://acme-staging-v02.api.letsencrypt.org/directory
```

Sobald das Log sagt, dass die Zertifikate erhalten wurden, und `curl -kv https://social.example/` ein Zertifikat von
`(STAGING)` zeigt, entferne die Einstellung und starte neu. Der Server registriert ein neues Konto bei der Produktionsumgebung und
bestellt dort seine Zertifikate; die Staging-Zertifikate bleiben in der Datenbank und werden nicht mehr ausgeliefert.

<!-- source: certificates-from-files 35f16ea23b33 -->
### Zertifikate aus Dateien

Mit `tls = files` enthält `tls_cert_file` zuerst das Zertifikat des Servers, dann seine Zwischenzertifikate (das `fullchain.pem` von
certbot); `tls_key_file` enthält seinen Schlüssel. Das Zertifikat muss `host` abdecken, und es bedient auch den Host von
`web_origin`, wenn der abweicht und das Zertifikat ihn abdeckt.

Der Server sieht jede Minute auf die Änderungszeiten beider Dateien und lädt sie neu, wenn sich eine ändert, ohne
Neustart. Ein Paar, das noch nicht zusammenpasst (ein neues Zertifikat vor seinem neuen Schlüssel), lässt das vorherige Zertifikat
bis zur nächsten Prüfung in Betrieb, und das Log sagt, warum.

<!-- source: commands-and-metrics de97a3993e6b -->
### Kommandos und Metriken

`my tls` zeigt und steuert die Zertifikate mit der Konfiguration des Servers, von jeder Maschine aus, die die
Datenbank erreicht (die [Admin-CLI](admin-cli.md#tls-certificates)):

```sh
my tls status                                          # each host: certificate, renewal, latest orders
my tls renew -host social.example                      # order now, without waiting
my tls revoke -host social.example -reason key_compromise
```

- **`status`** zeigt für jeden Host das aktuelle Zertifikat (Seriennummer, Aussteller und Ablauf), wann es erneuert wird (das Fenster
  der Zertifizierungsstelle oder der aus seiner Lebensdauer abgeleitete Zeitpunkt), wie viele Bestellungen in Folge keines erbracht haben, und
  die letzten Bestellungen mit ihren Fehlern. Mit `tls = files` zeigt es das Zertifikat von `tls_cert_file`.
- **`renew`** bestellt jetzt ein Zertifikat und überspringt den Erneuerungszeitpunkt und die Wartezeit nach fehlgeschlagenen Bestellungen: Führe es aus, sobald du
  die Ursache fehlgeschlagener Bestellungen behoben hast. Der laufende Server beantwortet die Prüfung der Zertifizierungsstelle, er muss also
  laufen, und seine Prozesse liefern das neue Zertifikat innerhalb von Sekunden aus, mit `stream = hub` innerhalb von 5 Minuten.
- **`revoke`** bestellt zuerst ein Zertifikat, das das aktuelle ersetzt, bittet dann die Zertifizierungsstelle, das aktuelle zu widerrufen,
  und zeichnet es auf; jeder Prozess hört innerhalb von Sekunden auf, das widerrufene Zertifikat auszuliefern, mit `stream = hub` innerhalb von 5
  Minuten. Verwende `key_compromise`, wenn der private Schlüssel anderen bekannt sein könnte, sonst `superseded` oder
  `operator`. Lässt sich das Ersatzzertifikat nicht bestellen, geht der Widerruf trotzdem durch: Die Prozesse liefern das vorherige Zertifikat des
  Hosts aus, wenn es nicht widerrufen ist, oder nichts, bis der Server ein neues erhält.
- `renew` und `revoke` brauchen `tls = acme` und ein anderes `kv` als `memory`, da die Prozesse, mit denen sich die Zertifizierungsstelle
  verbindet, die Antworten auf ihre Prüfungen aus dem `kv`-Store lesen. Mit `tls = files` werden sie abgelehnt: Erneuere
  oder widerrufe das Zertifikat mit dem Werkzeug, das die Dateien schreibt.

Ist [`metrics_exporter`](#metrics) gesetzt, alarmiere bei `my.tls.certificate.expiry` unter 7 Tagen und bei
`my.tls.order.consecutive_failures` von 3 oder mehr ([Alarme](alerts.md#mytlscertificateexpiring)): Eine fehlschlagende Erneuerung zeigt sich
Wochen vor dem Ablauf des Zertifikats.

<!-- source: checks-and-running-it 983028d27675 -->
### Prüfungen und Betrieb

`my config check` meldet mit `tls = acme` Bedingungen, denen nicht zugestimmt wurde, einen `host`, der eine IP-Adresse, `localhost`
oder ein Name ohne Punkt ist, ein `tls_acme_directory`, das keine `https`-URL ist, und `kv = memory` mit einer anderen `role` als
`all` als Fehler. Mit `tls = files` meldet es ein Zertifikat oder einen Schlüssel, der nicht gelesen werden kann, einen Schlüssel, der nicht zum
Zertifikat passt, und ein Zertifikat, das `host` nicht abdeckt, abgelaufen oder noch nicht gültig ist, als Fehler und warnt, wenn
weniger als 14 Tage bleiben. Es warnt, wenn `trusted_proxies` mit einem anderen `tls` als `off` gesetzt ist. Mit `tls = off` warnt es,
wenn `listen` Port 80 oder 443 auf einer anderen Adresse als Loopback ist, da anscheinend nichts vor dem
Server TLS beendet; der Server protokolliert dieselbe Warnung einmal pro Start, wenn ein solcher Server mit leerem `trusted_proxies` eine einfache
HTTP-Anfrage von einer anderen Adresse als Loopback erhält.

**systemd.** Ports unter 1024 brauchen für einen anderen Benutzer als root eine Capability. Gib sie dem Dienst und sonst nichts:

```ini
[Service]
User=my
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
```

Mit `tls = files` muss der Benutzer außerdem die Zertifikatsdateien lesen können. Bei certbot sind dessen Verzeichnisse `live` und `archive`
nur für root lesbar: Gib der Gruppe des Dienstes Zugriff (`chgrp` und `chmod g+rx` auf den Verzeichnissen, `g+r` auf
den Schlüsseln), oder kopiere die Dateien in einem Deploy-Hook in ein Verzeichnis, das der Dienst liest.

**Docker.** Docker 20.10 und neuer lassen einen Container Ports unter 1024 ohne Capability öffnen, daher funktioniert das Image
unverändert. Das Image setzt `MY_LISTEN=:8080`, setze also `MY_LISTEN=:443` in `.env`, veröffentliche beide Ports in einer
`compose.override.yaml` (`ports: ["80:80", "443:443"]`). Binde mit `tls = files` die Zertifikatsdateien schreibgeschützt dort ein,
wohin `MY_TLS_CERT_FILE` und `MY_TLS_KEY_FILE` zeigen; der Server läuft als Benutzer `65532`, der sie lesen können muss.
Mit `tls = acme` gibt es nichts einzubinden: Die Zertifikate liegen in der Datenbank.
Lass `compose.caddy.yaml` weg, das dieselben Ports belegen würde. Das gilt nicht mit `network_mode: host`, wo
der Container das Netzwerk der Maschine teilt und dieselbe Capability braucht wie ein Dienst außerhalb eines Containers.

**Im Vergleich zu Caddy.** Wer HTTPS von `my` ausliefert, verzichtet auf HTTP/3, die Kompression von Antworten (der Server sendet die Dateien des Web-
Clients komprimiert und komprimiert sonst nichts), ein eigenes Access-Log (die `http request`-Logzeilen des Servers bleiben) und per
DNS validierte Zertifikate, einschließlich Wildcard-Zertifikaten. Behalte `compose.caddy.yaml` oder einen anderen Proxy, wenn du etwas davon brauchst.

<!-- source: get-the-image 123f0112b538 -->
## Das Image beziehen

Das Server-Image ist `ghcr.io/mycroblog/my`, öffentlich, für `linux/amd64` und `linux/arm64`, mit jedem Adapter.

| Tag | Zeigt auf | Verwende es für |
| --- | --- | --- |
| `0.1.0` | Diese Version, für immer | Eine Version benennen |
| `0.1` | Den neuesten Patch von `v0.1` | Docker Compose, das bei jedem `up` das Schema anwendet |
| `0.1.0-rc.1` | Dieses Prerelease | Ein Release ausprobieren, bevor es erscheint |
| `nightly` | Den neuesten Commit von `main`, der die nächtlichen Tests bestanden hat, jede Nacht verschoben, in der er sich ändert | Änderungen vor einem Prerelease ausprobieren, auf einer Instanz, die du wegwerfen kannst |

Es gibt kein `latest` und kein `0`: Beide würden eine Minor-Version überschreiten, und jedes Upgrade auf eine neue Minor-Version muss [über die vorige](database.md#skipping-releases) laufen. Nenne in jedem Befehl eine Version; `docker pull ghcr.io/mycroblog/my` allein schlägt mit `manifest unknown` fehl.
Auch ein Patch-Release kann `my schema apply` und `my db upgrade` brauchen, folge also außerhalb von Docker Compose für jede Version [Upgrade](database.md#upgrading). Hefte in der Produktion den Digest an, den die Notizen des Releases nennen:

```text
ghcr.io/mycroblog/my:0.1.0@sha256:...
```

**`nightly` hat keinen Upgrade-Pfad.** Upgrades werden nur zwischen veröffentlichten Versionen versprochen: Von einem `nightly`-Image
zum nächsten oder von `nightly` zu einem Release können sich das Schema und die Datenschritte von `my db upgrade` auf eine Weise ändern, die kein
Release durchläuft, rechne also damit, seine Datenbank neu anzulegen. Betreibe es nie in der Produktion. `my version` gibt aus, welcher Commit es ist, wie in
`v0.1.0-rc.5.0.20261109182600.g7a789720a1b2`: eine Version nach dem neuesten Release (`v0.1.0-rc.5`), die Zeit des Commits in UTC und die
ersten 12 Zeichen des Commits nach `g`. Um einen Build zu behalten, hefte seinen Digest an (`ghcr.io/mycroblog/my:nightly@sha256:...`): Ein älterer Build verliert
das Tag, bleibt aber über seinen Digest abrufbar.

Wo du es betreibst:

- **Docker Compose**: `deploy/docker/compose.yaml` betreibt das Image von `MY_VERSION` in `.env` ([Docker-Compose-Dateien](#docker-compose-files)). Um es stattdessen aus einem Checkout zu bauen, füge die Überschreibung `compose.build.yaml` hinzu (`docker compose -f compose.yaml -f compose.build.yaml up -d --build`) und setze `MY_VERSION` auf einen eigenen Namen, da er dein lokales Image benennt und das ist, was `my version` ausgibt.
- **Google Cloud und AWS**: Die OpenTofu-Module betreiben das Image unverändert von `ghcr.io`, das Release, das ihr `ref` nennt ([Mit OpenTofu bereitstellen](deploy-opentofu.md#the-root-modules)).
- **Kubernetes**: Beziehe es unverändert von `ghcr.io`.

<!-- source: verify-an-image 76b9a1293edb -->
### Ein Image verifizieren

Jedes veröffentlichte Image hat eine über Sigstore signierte Provenienz, die den Digest an den Workflow und den Commit bindet, der es gebaut hat, sowie eine SBOM. Mit der [GitHub CLI](https://cli.github.com/), und bei `ghcr.io` angemeldet, wenn der Befehl danach fragt:

```sh
gh attestation verify oci://ghcr.io/mycroblog/my@sha256:... -R mycroblog/my \
  --signer-workflow mycroblog/my/.github/workflows/release.yml
docker buildx imagetools inspect ghcr.io/mycroblog/my@sha256:... --format '{{ json .SBOM }}'
```

Der erste Befehl schlägt fehl, es sei denn, das Image wurde vom Release-Workflow dieses Repositorys gebaut. Verifiziere ein `nightly`-Image stattdessen mit
`--signer-workflow mycroblog/my/.github/workflows/nightly.yml`. Er gibt den Commit aus, der der Commit des Tags der Version ist; das Label `org.opencontainers.image.revision` des Images nennt denselben.

<!-- source: get-the-binary 2608f7bdd54d -->
## Die Binärdatei beziehen

Um `my` ohne Container zu betreiben, hängt jedes Release die Binärdatei des Images an, für `linux/amd64` und `linux/arm64`,
als `my_<version>_linux_<arch>.tar.gz`. Es ist eine einzelne statische Binärdatei mit dem Web-Client darin; das Archiv enthält außerdem
`LICENSE`, die Lizenzhinweise in `licenses/` und eine README. Für andere Betriebssysteme gibt es keinen Build.

```sh
curl -fsSLO https://github.com/mycroblog/my/releases/download/v0.1.0/my_v0.1.0_linux_amd64.tar.gz
tar xzf my_v0.1.0_linux_amd64.tar.gz my
./my version
```

Konfiguriere sie, wie das Image konfiguriert wird ([Konfigurationsreferenz](../reference/configuration.md)), und folge für jede Version
[Upgrade](database.md#upgrading): Nichts wendet das Schema für dich an.

<!-- source: verify-a-binary 03daabe0c940 -->
### Eine Binärdatei verifizieren

Jedes Archiv hat eine über Sigstore signierte Provenienz, wie ein Image, und das Release listet die SHA-256-Summen der Archive in `SHA256SUMS`:

```sh
gh attestation verify my_v0.1.0_linux_amd64.tar.gz -R mycroblog/my \
  --signer-workflow mycroblog/my/.github/workflows/release.yml
sha256sum --ignore-missing -c SHA256SUMS
```

<!-- source: publish-the-source-code-agpl-30 2e87c44ffb2f -->
## Den Quellcode veröffentlichen (AGPL-3.0)

`my` steht unter der AGPL-3.0-only. Abschnitt 13 der AGPL-3.0 verlangt von jedem, der den Server betreibt und Nutzern erlaubt, über ein Netzwerk mit ihm zu interagieren,
diesen Nutzern einen Weg anzubieten, den zugehörigen Quellcode des laufenden Programms zu erhalten. Das gilt für alle, die mit dem Server interagieren, nicht nur für Personen mit einem Konto.

Gib mit der Einstellung `source_url` (Umgebungsvariable `MY_SOURCE_URL`; siehe die [Konfigurationsreferenz](../reference/configuration.md#source_url)) an, wo der Quellcode veröffentlicht ist.

| Wert | Bedeutung |
| --- | --- |
| Nicht gesetzt | Der Standard, `https://github.com/mycroblog/my`. Nutze ihn, wenn du `my` unverändert betreibst: Das `GET /instance` eines Releases nennt dann den Quellcode dieser Version, `https://github.com/mycroblog/my/tree/v0.1.0`, und NodeInfo das Repository |
| Eine `https:`-URL | Ein Ort, an dem der Quellcode gelesen werden kann (das Repository eines Forks, der Tag oder das Archiv einer bestimmten Version und so weiter) |
| Ein `mailto:`-URI | Eine E-Mail-Adresse, bei der der Quellcode angefordert werden kann (zum Beispiel `mailto:source@example.org`; `?subject=...` ist erlaubt) |

Jeder andere Wert (`http:`, `git@...`, ein relativer Pfad, eine URL mit Nutzerangaben oder einem `#`, ein `mailto:` ohne Adresse und so weiter) lässt `my config check` fehlschlagen, und der Server startet nicht.

Der Wert erscheint an diesen Stellen:

- `source_url` im `GET /instance` der Client-API
- Der Abschnitt „Quellcode“ der Informationsseite des Servers im Webclient (`/about`) und ein Link auf der Startseite (`/`)
- `metadata.sourceCode` in NodeInfo (`/nodeinfo/2.1`). Eine `https:`-URL wird auch als `software.repository` veröffentlicht
  (das NodeInfo-2.1-Schema erlaubt in `software.repository` nur eine `http(s)`-URL, ein `mailto:`-Wert erscheint also nur in `metadata.sourceCode`)

Das Image trägt die Lizenz von `my` unter `/usr/share/doc/my/LICENSE` und die Lizenzhinweise des Drittanbieter-Codes und der Daten in der Binärdatei unter `/usr/share/doc/my/licenses/`:
`go-modules.md` für die Go-Module, `npm-packages.md` und `npm-packages-sw.md` für die npm-Pakete des Web-Clients sowie die Lizenzen seiner mitgelieferten Daten und Schriften.
Wenn du das Image oder die Binärdatei weitergibst, gib diese Dateien mit.

<!-- source: when-you-fork-or-modify-my be83256fe6a3 -->
### Wenn du `my` forkst oder veränderst

Betreibst du ein verändertes `my` (du wendest Patches an, baust einen Fork oder tauschst Abhängigkeiten aus), zeigt die Standard-URL nicht mehr auf den Quellcode dessen, was läuft.
Ändere `source_url` auf den Ort, an dem der Quellcode der laufenden Version samt deinen Änderungen zu finden ist. Wer auf den Commit, Tag oder das Archiv zeigt, das zur laufenden Version passt, statt auf den neuesten Stand eines Repositorys, lässt Nutzer verlässlich denselben Quellcode erhalten.
Betreibst du ein unverändertes offizielles Release, behalte den Standard.

<!-- source: when-you-use-mailto 11155a8b71ed -->
### Wenn du `mailto:` verwendest

- Sei bereit, den zugehörigen Quellcode (einschließlich der Skripte, die zum Bauen nötig sind) umgehend auszuhändigen, wenn jemand danach fragt. Nur eine Adresse zu nennen, ohne zur Lieferung bereit zu sein, erfüllt die Pflicht nicht.
- Verwende eine Adresse, die auch dann noch funktioniert, wenn du den Server nicht mehr betreibst oder die zuständige Person wechselt. Eine gemeinsame Betriebsadresse ist besser als ein persönliches Postfach.
- Stelle sicher, dass der Spamfilter Anfragen nicht verwirft.
- Verwende eine Adresse, die du gern öffentlich machst: Jeder kann sie auf der Informationsseite, der Startseite und in NodeInfo sehen.

<!-- source: metrics d456616d0cb0 -->
## Metriken

Die Einstellung `metrics_exporter` (Standard `none`) exportiert Dienstmetriken. Ihre zugehörigen Einstellungen stehen in der [Konfigurationsreferenz](../reference/configuration.md).

| `metrics_exporter` | Pfad | Geeignet für |
| --- | --- | --- |
| `none` | Es wird nichts exportiert. Die Instrumentierung kostet fast nichts. Die Werte der ganzen Instanz gehen weiterhin alle 15 Minuten in eine `metrics snapshot`-Logzeile, die der Adapter und des Pools in die Anfrage- und Job-Zeilen | Google Cloud, wo die Metriken der Plattform und logbasierte Metriken an ihre Stelle treten ([Auf Google Cloud bereitstellen](deploy-google-cloud.md#monitoring-and-alerts)) |
| `prometheus` | `GET /metrics` auf `metrics_listen` (Standard `127.0.0.1:9464`). Es ist ein vom öffentlichen `listen` getrennter Listener ohne Authentifizierung und läuft für jede `role` | Selbst gehostet (von einem Prometheus-kompatiblen Server oder KEDA abgerufen) |
| `otlp` | Wird per OTLP/HTTP an einen OpenTelemetry Collector gesendet. Das Ziel stammt aus den üblichen OTel-Umgebungsvariablen wie `OTEL_EXPORTER_OTLP_ENDPOINT` | Setups, in denen du einen Collector betreiben kannst |
| `emf` | JSON-Zeilen im CloudWatch Embedded Metric Format. Anfrage- und Job-Metriken reisen auf den abschließenden Logzeilen oben; die anderen werden auf die Standardausgabe geschrieben, eine Zeile pro Label-Satz, wenn eine Anfrage oder ein Job endet | AWS ([Auf AWS bereitstellen](deploy-aws.md#monitoring-and-alerts)) |

Die Metriken sind unten aufgelistet. In Prometheus wird `.` in einem Namen zu `_`, und ein Einheiten-Suffix (`_seconds` und `_total` für einen Zähler) wird angehängt.

| Metrik | Art | Labels |
| --- | --- | --- |
| `http.server.request.duration` | Histogramm, Sekunden | `http.request.method`, `http.route` (der Pfad des Routenmusters; `other`, wenn keines passt), `http.response.status_code`, `my.surface` (`client_api` / `admin` / `activitypub` / `oauth` / `media` / `web` / `payments` / `voice` / `queue`) |
| `http.server.active_requests` | UpDownCounter | `my.surface` |
| `my.inbox.activities` | Zähler | `activity.type` (ein bekannter Activity-Typ oder `other`), `my.outcome` (`accepted` / `duplicate` / `rejected` / `ignored` / `error`), `my.inbox.rejection` (nur bei `rejected`: `signature` für eine Signatur oder einen Integritätsnachweis, der fehlt, ungültig ist oder sich nicht verifizieren lässt oder dessen Schlüssel nicht abgerufen werden konnte; `suspended` für eine gesperrte Domain; `federation_off`; `rate_limited`; `invalid` für eine Aktivität ohne gültige `id`, `type`, `actor` oder `object`) |
| `my.relay.activities` | Zähler | `my.outcome` (`admitted` / `over_limit` / `restricted` / `not_receiving`: ob eine von einem Relay getragene Aktivität übernommen wurde und warum sie verworfen wurde; siehe [Föderations-Relays](relays.md)) |
| `my.move.activities` | Zähler | `my.outcome` (`recorded` / `duplicate` / `not_own_actor` / `same_account` / `target_unavailable` / `alias_missing` / `target_moved` / `suspended` / `too_soon`: ob ein empfangenes `Move` aufgezeichnet wurde und warum es verworfen wurde) |
| `my.delivery.attempts` | Zähler | `my.outcome` (`success` / `retry` / `gone` / `failed`; `gone` umfasst ein 410 und Zustellungen, die übersprungen werden, weil der Server nicht verfügbar oder gesperrt ist) |
| `my.delivery.duration` | Histogramm, Sekunden | `my.outcome` |
| `my.ux.duration` | Histogramm, Sekunden | `my.ux.metric` (`lcp` / `inp` / `long_frame` / `transition` / `press` / `call_rtt` / `call_jitter`), `my.ux.name` (die Route, Interaktionsart, Aktion oder der Anruf-Transport; [UX-Messungen](error-reporting.md#ux-measurements)) |
| `my.ux.call.packet_loss` | Histogramm, Prozent | `my.ux.name` (der Anruf-Transport) |
| `my.plugin.failures` | Zähler | `my.plugin.kind` (eine Art von [Plugin](plugins.md)), `my.plugin.code` (der Connect-Code des Fehlers) |
| `my.plugin.slot_wait` | Histogramm, Sekunden | `my.plugin.kind` (wie lange ein Aufruf auf seinen Zug unter den gleichzeitig laufenden Aufrufen dieser Art wartete) |
| `my.hook.decisions` | Zähler | `hook`, `mode`, `outcome` (die Entscheidungen, die ein [Entscheidungs-Hook](decision-hooks.md) aufgezeichnet hat) |
| `my.hook.failures` | Zähler | `hook`, `outcome` (`timeout`, `error`, `invalid` oder `breaker_open`) |
| `my.hook.duration` | Histogramm | `hook` (Sekunden pro Aufruf des Plugins) |
| `my.job.duration` | Histogramm, Sekunden | `my.job.type` (ein registrierter Job-Typ oder `other`), `my.outcome` (`success` / `retry` / `dead` / `postponed`: `postponed` ist ein Lauf, der nichts getan hat und keinen der Versuche des Jobs verbraucht, etwa ein Abruf einer Link-Karte, der auf seinen Host wartet) |
| `my.job.wait` | Histogramm, Sekunden | `my.job.type` |
| `my.link_card.host_pauses` | Zähler | `http.response.status_code` (`429` / `503`). Eine pro Pause der Link-Karten-Abrufe bei einem Host; siehe [Link-Vorschauen](link-previews.md#limits-per-host) |
| `my.projection.drifts` | Zähler | `my.projection` (der Name einer Projektion). Eine pro geplanter Prüfung, die die Projektion abgewichen fand; siehe [Projektionen prüfen und neu aufbauen](database.md#checking-and-rebuilding-projections) |
| `my.translation.requests` | Zähler | `my.outcome` (`author` / `cache` / `provider` / `no_provider` / `limit` / `unsupported` / `failed`). Eine pro Anfrage, einen Beitrag zu übersetzen, den die betrachtende Person sehen darf, oder den Beitrag oder Kommentar einer Meldung, den ein Moderator anfordert: beantwortet aus den eigenen Worten der Autorin oder des Autors, aus einer aufbewahrten Übersetzung oder von einem Anbieter, oder abgelehnt, weil kein Anbieter den Beitrag erhalten darf, ein Monatslimit erreicht ist, kein Anbieter die Sprache übersetzt oder kein Anbieter eine brauchbare Übersetzung geliefert hat. |
| `my.translation.lock_waits` | Zähler | Keine. Eine pro Anfrage, die auf eine andere Anfrage wartete, die denselben Beitrag in dieselbe Sprache übersetzt, statt einen Anbieter zu fragen |
| `my.translation.characters` | Zähler | `my.provider` (die ID eines Übersetzungsanbieters). Die Zeichen von Beiträgen und Meldungskommentaren, die an den Anbieter gesendet werden, ohne Auszeichnungen, einschließlich fehlschlagender Aufrufe. Das zählen die [Monatslimits](instance-settings.md#admin-screens-and-the-client-api), und es kann von dem abweichen, was der Anbieter abrechnet |
| `my.timeline.delivery.lag` | Histogramm, Sekunden | Keine (von der Erstellung eines lokalen Beitrags bis zu seinem Schreiben in die Startseiten-Timelines und der Benachrichtigung der Streams) |
| `my.stream.connections` | UpDownCounter | `my.stream.kind` (`user`, `timeline` für die Streams der lokalen und der föderierten Timeline, `voice` für die Streams der Sprachchat-Räume) |
| `my.stream.signals` | Zähler | `my.stream.kind` (`user` / `timeline`). Eines pro Signal, das ein offener Stream verarbeitet hat, ob er dem Client etwas gesendet hat oder nicht |
| `my.accounts.registered` | Zähler | Keine |
| `my.statuses.created` | Zähler | `my.visibility` |
| `my.billing.imports` | Zähler | `my.billing.source` (`aws` / `gcp` / `plugin`), `my.outcome` (`success` / `failed`), `error.type` (nur bei `failed`: die Art des Fehlers, wie in [Cloud-Nutzungskosten importieren](billing-import.md)). Jeder Versuch zählt, auch die Wiederholungen und die Fehler, die auf den nächsten täglichen Import warten |
| `my.supporter.tip_checkouts` | Zähler | `my.outcome` (`started` / `rate_limited` / `paid` / `expired`). Eines pro Start eines Trinkgelds, das die Seite des Anbieters erhalten hat, pro Start, den die Limits abgelehnt haben, und pro Trinkgeld, das bezahlt wurde oder unbezahlt endete; siehe [Trinkgelder](supporters.md#tips) für die Alarme, die es bedient |
| `my.voice.turn.credentials` | Zähler | `my.voice.turn` (`coturn` / `cloudflare`), `my.outcome` (`issued` / `reused` / `failed`). Eines pro TURN-Zugangsdatum, das ein Teilnehmer eines Sprachchats beim Betreten eines Raums oder beim Erneuern seiner Verbindung angefordert hat: `reused` heißt, früher für dieselbe Verbindung ausgestellte Cloudflare-Zugangsdaten wurden erneut zurückgegeben, und `failed` heißt, Cloudflare war nicht erreichbar oder hat abgelehnt, sodass der Teilnehmer einen Fehler erhielt und sich nicht verbinden konnte |
| `my.voice.sfu.rooms`, `my.voice.sfu.sessions` | Gauge | Keine (pro Medienknoten von `voice = sfu`: die Räume mit Verbindungen auf dem Knoten und die mit ihm verbundenen Sitzungen) |
| `my.voice.sfu.capacity.reserved`, `my.voice.sfu.capacity.connected` | Gauge | Keine (pro Medienknoten: die Summe der Maxima der auf ihm abgelegten Räume und die mit ihm verbundenen Sitzungen, beide im Vergleich zu `voice_sfu_node_capacity`) |
| `my.voice.sfu.packets` | Zähler | `my.outcome` (`forwarded`, eines pro Teilnehmer, an den ein Audiopaket gesendet wurde; oder warum ein Paket verworfen wurde: `silent`, `not_allowed` für einen Teilnehmer, der nicht sprechen darf oder stummgeschaltet ist, `queue_full` für einen Teilnehmer, dessen Verbindung nicht mithalten konnte) |
| `my.voice.sfu.bytes` | Zähler, Bytes | Keine (das Audio, das der Medienknoten weitergeleitet hat) |
| `my.voice.sfu.renegotiations`, `my.voice.sfu.ice_restarts`, `my.voice.sfu.migrations` | Zähler | Keine (neue Offers nach der ersten, weil sich die Sprecher geändert haben; ICE-Neustarts, die Clients nach einem Netzwerkwechsel angefordert haben; Sitzungen, die ein sich beendender Knoten an einen anderen Knoten geschickt hat) |
| `my.voice.sfu.ticket_refusals` | Zähler | `my.voice.sfu.reason` (`invalid` für ein abgelaufenes, benutztes oder unbekanntes Ticket, `other_node` für ein Ticket eines anderen Knotens) |
| `my.voice.sfu.client.rtt` | Histogramm, Sekunden | Keine (die Round-Trip-Zeit zu den Teilnehmern, aus ihren RTCP-Berichten) |
| `my.voice.sfu.client.packet_loss` | Histogramm, Prozent | Keine (der Verlust auf dem Weg zu den Teilnehmern, aus ihren RTCP-Berichten) |
| `my.voice.sfu.offer.wait` | Histogramm, Sekunden | Keine (die Zeit vom Beitritt eines Teilnehmers bis zum ersten Offer des Medienknotens) |
| `my.errors.exported` | Zähler | `my.outcome` (`success` / `dropped` / `failed`: eines pro Fehlerbericht für den Dienst von `errors_exporter`. `dropped` heißt, der Bericht wurde nicht gesendet, weil bereits 1.000 Berichte warteten oder der Dienst den Server gebeten hat, langsamer zu werden, und der Server wartet, so lange der Dienst es verlangt, bevor er wieder sendet; `failed` heißt, der Dienst war nicht erreichbar oder hat den Bericht abgelehnt. Ein Bericht wird einmal gesendet und nie wiederholt, und die Berichte, die 2 Sekunden nach Beginn eines Herunterfahrens noch warten, gehen verloren) |
| `my.queue.ready` | Gauge | `my.job.type` (die Zahl der ausführbaren Jobs in der Datenbank-Queue) |
| `my.queue.oldest_ready_age` | Gauge, Sekunden | `my.job.type` (wie lange der älteste ausführbare Job jedes Typs in der Datenbank-Queue gewartet hat; ein Typ ohne ausführbaren Job hat keinen Wert) |
| `my.queue.dead` | Gauge | Keine |
| `my.instance.users`, `my.instance.statuses` | Gauge | Keine (die Zahlen lokaler Benutzer und Beiträge, wie in NodeInfo) |
| `my.media.stored_bytes` | Gauge, Bytes | `my.media.origin` (`local` / `remote_cache`) |
| `my.db.size` | Gauge, Bytes | Keine (die Größe der Datenbank: die Relationen des aktuellen Schemas bei PostgreSQL; die Datenbankdatei ohne ihr Write-Ahead-Log bei SQLite) |
| `my.billing.import.failing` | Gauge | `my.billing.source` (1, wenn der neueste Import der Quelle fehlschlug, sonst 0) |
| `my.federation.instances` | Gauge | `my.federation.instance.state` (`available`: Server, an die weiter zugestellt wird, die aber seit ihrer letzten Antwort an mindestens einem Tag fehlschlugen; `unavailable`: Server, deren Zustellungen pausiert sind) |
| `my.metrics.snapshot.age` | Gauge, Sekunden | `my.metrics.snapshot.part` (`queue` / `totals`) |
| `my.errors.reported` | Zähler | `my.error.source` (`server` / `web`), `my.error.kind` (`panic` / `error`). Eines pro [Fehlerbericht](#error-reports) |
| `my.tls.certificate.expiry` | Gauge, Sekunden | `host` (die Zeit bis zum Ablauf des Zertifikats, das dieser Prozess für einen seiner Hosts ausliefert, mit `tls = acme` oder `tls = files`; jeder Prozess meldet sein eigenes, sodass ein Prozess mit einem alten Zertifikat auffällt. Siehe [Kommandos und Metriken](#commands-and-metrics)) |
| `my.tls.order.attempts` | Zähler | `host`, `result` (`issued` / `failed`). Eines pro Bestellung eines Zertifikats bei der ACME-Zertifizierungsstelle, durch den Prozess, der sie aufgegeben hat, `my tls renew` und `my tls revoke` eingeschlossen |
| `my.tls.order.consecutive_failures` | Gauge | `host` (die Bestellungen seit dem aktuellen Zertifikat des Hosts, die keines erbracht haben: die Zahl, die die Wartezeit vor der nächsten Bestellung bestimmt) |
| `my.tls.rounds` | Zähler | Keine. Eines pro Runde des Zertifikatsmanagers über seine Hosts mit `tls = acme`; eine Zahl, die aufhört zu wachsen, heißt, der Manager ist stehen geblieben |

Health Checks (`/healthz`, `/readyz`, `/health/live`, `/health/ready`) werden nicht gemessen. Kein Label enthält eine entfernte Domain, ein Konto, eine URL oder eine Client-IP.
`my.queue.*`, `my.instance.*`, `my.media.stored_bytes`, `my.db.size`, `my.federation.instances` und `my.billing.import.failing` sind Werte der ganzen Instanz: Jeder Prozess meldet denselben Wert aus einem Snapshot, der im Key-Value-Store aufbewahrt wird.

<!-- source: system-metrics 973069859f11 -->
### Systemmetriken

Metriken über den Prozess, die Go-Laufzeit und Verbindungen zu Ressourcen werden über denselben `metrics_exporter` exportiert.
CPU, Speicher, Instanzzahl und eingehende Anfragen des Containers überlassen wir den Metriken der Plattform; diese hier decken die Sättigung innerhalb des Prozesses ab, die die Plattform nicht sehen kann.
Verwende sie nicht, um Skalierung auszulösen; verwende sie für Alarme und zur Abschätzung der Instanzobergrenze.

| Metrik | Art | Labels |
| --- | --- | --- |
| `go.memory.used`, `go.memory.limit`, `go.memory.gc.goal` | Gauge, Bytes | Nur `go.memory.used` hat `go.memory.type` (`stack` / `other`). `go.memory.limit` erscheint nur, wenn `GOMEMLIMIT` gesetzt ist |
| `go.memory.allocated`, `go.memory.allocations` | Zähler | Keine |
| `go.goroutine.count`, `go.processor.limit`, `go.config.gogc` | Gauge | Keine |
| `go.schedule.duration`, `go.memory.gc.pause.duration` | Histogramm, Sekunden | Keine (das Warten vom Ausführbar-Werden bis zum Laufen und GC-Pausen) |
| `go.memory.gc.cycles` | Zähler | Keine |
| `process.cpu.time` | Observable Counter, Sekunden (Summe seit dem Start) | `cpu.mode` (`user` / `system`) |
| `process.memory.usage` | Gauge, Bytes | Keine (residenter Speicher, RSS) |
| `process.unix.file_descriptor.count`, `my.process.file_descriptor.limit` | Gauge | Keine (offene Dateideskriptoren und das Limit) |
| `process.uptime` | Gauge, Sekunden | Keine |
| `db.client.connection.count` | Gauge | `db.client.connection.pool.name` (`postgres` / `sqlite`), `db.client.connection.state` (`idle` / `used`) |
| `db.client.connection.max` | Gauge | `db.client.connection.pool.name` |
| `db.client.connection.wait_time`, `my.db.client.connection.waits`, `db.client.connection.timeouts` | Observable Counter (Summe seit dem Start; Sekunden oder Anzahl) | `db.client.connection.pool.name`. `timeouts` gibt es nur bei PostgreSQL |
| `my.adapter.operation.duration` | Histogramm, Sekunden | `my.adapter` (`queue.db` / `kv.db` / `kv.memory` / `kv.valkey` / `stream.<stream>` / `blob.fs`), `my.operation`, `my.outcome` (`success` / `error`) |
| `my.adapter.connections` | UpDownCounter | `my.adapter` (`valkey`-Verbindungen und `stream.<stream>`-Abonnenten) |
| `my.adapter.reconnects` | Zähler | `my.adapter` |
| `http.client.request.duration` | Histogramm, Sekunden | `my.http.purpose` (`json` / `html` / `image` / `video` / `human_check` / `web_push` / `webhook` / `client_metadata` / `login_provider` / `robots` / `domain_list`), `http.request.method`, `my.http.status_class` (`2xx` und so weiter), `error.type` (`forbidden_destination` ist eine Ablehnung durch den SSRF-Schutz) |
| `http.client.open_connections` | UpDownCounter | Keine |
| `my.readiness.failures` | Zähler | Keine |
| `my.readiness.shared` | Zähler | Keine. Eine pro Bereitschaftsanfrage, die das Ergebnis einer bereits laufenden Prüfung übernommen hat, statt eine zu starten |
| `my.http.origin_refused` | Zähler | Keine. Eine pro Anfrage, die abgelehnt wurde, weil sie nicht über das CDN kam ([`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn)). Proben der Adresse des Origins lassen sie langsam steigen; ein Sprung, während der Verkehr aufhört, heißt, das CDN sendet den Header nicht mehr. Der Wartungsmodus misst nichts, sodass nur sein Access-Log sie zeigt |

- Go-Laufzeitmetriken (`go.*`) werden nur von `prometheus` und `otlp` exportiert. Mit `emf` schreibt ein periodischer Snapshot-Job die Gauges und Observable Counter von Prozess und Verbindungspool.
- Pool-Wartezeiten und CPU-Zeit sind Observable Counter: Der Pool und der Kernel halten Summen seit dem Start, die zum Erfassungszeitpunkt gelesen werden. In Prometheus sind es `_total`-Zähler (etwa `process_cpu_time_seconds_total`), in OTLP monotone Summen, und ihr Wachstum siehst du mit PromQL `rate()`.
  `emf` hat keine Summenzähler, daher schreibt es den Zuwachs seit dem vorherigen Schreiben (addiere sie mit der CloudWatch-Statistik Sum).
- Das `claim` der Queue wird nur gemessen, wenn es einen Job nimmt oder fehlschlägt. Das Polling eines wartenden Workers zeichnet nichts auf.
- `db.client.connection.pending_requests` wird nicht exportiert: Die Statistiken von pgx und `database/sql` melden nicht, wie viele warten, daher vertreten die Zahl und die Dauer der Wartezeiten sie.
- Alarmiere auf das Verhältnis der Summe von `go.memory.used` zu `go.memory.limit` und auf das Wachstum von `db.client.connection.wait_time` (das Pool-Limit wird erreicht).

<!-- source: memory-limit-gomemlimit 66320a6c7b76 -->
### Speicherlimit (`GOMEMLIMIT`)

`GOMEMLIMIT` ist eine Umgebungsvariable der Go-Runtime, die der Betreiber setzt; die Anwendung wählt sie nicht.
Ohne sie läuft Go mit seinem Standard (kein Limit), und der Heap sammelt nicht eifrig, wenn er sich dem Speicherlimit des Containers nähert.
Setze sie bei einem Speicherlimit des Containers auf etwa 90 % dieses Limits (zum Beispiel `GOMEMLIMIT=460MiB` für einen Container mit 512 MiB). Die OpenTofu-Module setzen sie so für jeden Dienst, aus seiner Größe.
Setze `GOMAXPROCS` nicht: Der Standard von Go folgt dem CPU-Limit des Containers.

<!-- source: traces 15eb90e8584a -->
## Traces

Die Einstellung `traces_exporter` (Standard `none`) exportiert verteilte Traces. Die zugehörigen Einstellungen stehen in der [Konfigurationsreferenz](../reference/configuration.md).

| Span | Name | Attribute |
| --- | --- | --- |
| HTTP-Anfrage (Server) | `<method> <http.route>` (`<method>` bei einer Anfrage, die keine Route erreichte) | `http.request.method`, `http.route`, `http.response.status_code`, `my.surface`. Ein 5xx ist ein Fehler |
| Job-Ausführung (Consumer) | Der Job-Typ (`my.job.type`) | `my.job.type`, `my.job.attempt`, `my.outcome`. Jede Wiederholung ist ein eigener Span |
| Transaktion | `my.db.transaction` | `db.system.name`. Jede Wiederholung ist ein eigener Span |
| Abfrage (Client) | Der sqlc-Abfragename (etwa `InsertJobs`; sonst das erste Wort des SQL, etwa `BEGIN`) | `db.system.name`, `db.operation.name` |
| Zustellung (`my.delivery`) | `my.delivery` | `activity.type`, `my.outcome` |
| Ausgehendes HTTP (Client) | `<method>` | `http.request.method`, `server.address` (nur der Hostname des Ziels), `http.response.status_code`. Ein 4xx oder 5xx ist ein Fehler |

Ein fehlgeschlagener Span hat `error.type` (ein endlicher Name wie `timeout` oder ein Statuscode). Attribute enthalten nie Konto-, Beitrags- oder Job-IDs, URLs, Pfade oder Queries, Client-IPs, User-Agent-Werte, SQL-Text oder Fehlerstrings.
`server.address` überlässt die Hostnamen entfernter Server dem Empfänger, sodass du entscheidest, wie lange der Empfänger sie behält und wer sie sehen kann. Health-Checks und die von `origin_secret` abgelehnten Anfragen erzeugen keinen Span.

- Eine Job-Ausführung ist kein Kind des Spans, der sie eingereiht hat (meist eine HTTP-Anfrage). Sie ist die **Wurzel eines neuen Traces** mit einem Link zum einreihenden Span. Folge in einem Viewer dem Link vom Trace der Anfrage.
- Ein eingehendes `traceparent` wird nur zum Parent, wenn der verbindende Peer in `trusted_proxies` steht und, bei [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn), die Anfrage das Geheimnis trägt. Andernfalls wird es verworfen und ein neuer Trace beginnt. `tracestate` wird immer verworfen. Ausgehendes HTTP (Föderation, WebFinger, Medien, Web Push und so weiter) trägt kein `traceparent`.
- Sampling: Eine Anfrage wird aufgezeichnet, wenn ihr Parent über einen vertrauenswürdigen Proxy aufgezeichnet wurde oder wenn das Link-Ziel eines Jobs aufgezeichnet wurde; sonst wird sie im Verhältnis `traces_sample_ratio` (Standard 0,01) aufgezeichnet. Das Senken des Verhältnisses lässt nie Jobs, die eine aufgezeichnete Anfrage fortsetzen, ohne ihren Trace zurück.

| Ziel | Einstellungen | Pfad |
| --- | --- | --- |
| Standard, kleines Self-Hosting | `traces_exporter = none` | Es werden keine Spans exportiert. Folge `request_id` und `job_id` in den Logs. Liefert ein vertrauenswürdiger Proxy ein `traceparent`, werden dessen `trace_id` und `span_id` zu den Logs hinzugefügt |
| Self-Hosting (Jaeger, Tempo, ein OpenTelemetry Collector und so weiter) | `traces_exporter = otlp`, `traces_flush = batch` | Über OTLP/HTTP an den Empfänger unter `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` (oder `OTEL_EXPORTER_OTLP_ENDPOINT`, Standard `http://localhost:4318`) gesendet. Authentifizierungs-Header gehören in `OTEL_EXPORTER_OTLP_HEADERS`. Um Fehler und langsame Traces zuverlässig zu behalten, nutze das Tail-Sampling des Collectors |
| Google Cloud | `traces_exporter = none` | Cloud Trace zeigt den Trace, den Cloud Run von jeder Anfrage aufzeichnet: [Auf Google Cloud bereitstellen](deploy-google-cloud.md#traces) |
| AWS | `traces_exporter = otlp`, an einen Collector in jedem Task | X-Ray: [Traces zu X-Ray](deploy-aws.md#traces-to-x-ray) |

<!-- source: profiles d19dd2bf22e2 -->
## Profile

Die [Systemmetriken](#system-metrics) sagen, wie viel Speicher und CPU ein Prozess nutzt; ein Profil der Go-Laufzeit sagt, was sie verbraucht.
`pprof_listen` nennt die Adresse eines Listeners, der die Profile unter `/debug/pprof/` ausliefert, für jede `role`. Es ist standardmäßig leer, was keinen Listener öffnet.

```sh
MY_PPROF_LISTEN=127.0.0.1:6060 my serve

go tool pprof -sample_index=inuse_space http://127.0.0.1:6060/debug/pprof/heap   # what holds the heap now
go tool pprof -sample_index=alloc_space http://127.0.0.1:6060/debug/pprof/heap   # what has allocated since the start
go tool pprof 'http://127.0.0.1:6060/debug/pprof/profile?seconds=30'             # where the CPU goes, for 30 seconds
curl 'http://127.0.0.1:6060/debug/pprof/goroutine?debug=1'                       # every goroutine, by where it waits
go tool pprof 'http://127.0.0.1:6060/debug/pprof/mutex?seconds=30'               # where goroutines wait on locks, for 30 seconds
curl 'http://127.0.0.1:6060/debug/pprof/heap?debug=1' | grep '^# '               # the runtime's memory statistics
```

Der Listener hat keine Authentifizierung, ein Profil zeigt das Innere des Prozesses, und eines zu erstellen kostet CPU: Halte ihn auf Loopback und erreiche ihn über einen Tunnel (`ssh -L 6060:127.0.0.1:6060 <host>`, `kubectl port-forward`), und setze ihn zurück, wenn du fertig bist.
Das Heap-Profil deckt nur den Go-Heap ab. Der residente Speicher des Prozesses enthält auch die gelesenen Seiten der Binärdatei, die Stacks und die eigenen Strukturen der Laufzeit; die Speicherstatistiken geben diese an.
Die Block- und Mutex-Profile sampeln nur, während eine Anfrage mit `seconds` sie erstellt, und sind ohne sie leer.

<!-- source: stop-the-server 72d71a40815a -->
## Den Server stoppen

Bei SIGTERM oder SIGINT fährt der Server geordnet herunter.

1. Er nimmt keine neuen Verbindungen und keine Jobs mehr an.
2. Er wartet bis zu 25 Sekunden, bis laufende Anfragen und laufende Jobs fertig sind.
3. Nach 25 Sekunden bricht er den Kontext der übrigen Anfragen und Jobs ab und schließt Verbindungen.
   Danach gibt er einem abgebrochenen Job bis zu 3 weitere Sekunden, sein Ergebnis zu verzeichnen, und endet mit Exit-Code 1 (insgesamt höchstens 28 Sekunden, innerhalb der standardmäßigen Kulanzfrist von 30 Sekunden bei Kubernetes und ECS).
   Ein abgebrochener Job wird wiederholt. Ein Job, der innerhalb dieser 3 Sekunden nicht stoppt oder mittendrin beendet wird, wird aufgegeben und von einem Worker wieder aufgenommen, wenn sein Lease abläuft, er kann also zweimal laufen.

Ein Prozess, der einen Medienknoten von `voice = sfu` betreibt (`role = sfu` oder `all`), leert ihn zuerst: Er nimmt keine Räume mehr an, verlegt
seine Räume auf andere Knoten und weist deren Clients an, sich dort neu zu verbinden, und wartet bis zu 20 der 25 Sekunden darauf, dass sie
gehen, bevor er auf die Anfragen wartet. Gib ihm dieselbe Kulanzfrist von mindestens 30 Sekunden (der Standard von Docker ist 10:
setze `stop_grace_period` oder `docker stop -t 30`). Siehe [Sprachchat](voice.md#drain-a-node-for-a-deploy).

Ein Panic in einer der Hintergrundaufgaben des Servers löst dasselbe Herunterfahren aus, damit der Server nie in einem kaputten Zustand weiterläuft.
Die Logzeile `serve failed` nennt die Aufgabe, und der Prozess endet mit Exit-Code 1, damit die Plattform oder der Prozessmanager ihn neu startet.

Starte für Arbeiten an der Datenbank, dem Key-Value-Speicher oder der Warteschlange statt eines Stopps im Wartungsmodus neu (`MY_MAINTENANCE=true`). Siehe [Wartung](maintenance.md).

<!-- source: the-job-queue-and-failed-dead-jobs 1fd5aaccbff5 -->
## Die Job-Queue und fehlgeschlagene (tote) Jobs

[Jobs](../reference/jobs.md) listet jeden Job-Typ auf: was er tut, was ihn startet, wie er wiederholt wird und was eine erneute Ausführung bewirkt.

Die Admin-Seite für Jobs (Administration → Jobs, `/admin/jobs`, Berechtigung `manage_jobs`) hat drei Seiten:

- **Queue-Zustand** (`/admin/jobs`): zuerst, wie viele Jobs fehlgeschlagen sind, mit einem Link dorthin; die Zahl pro Zustand (in der Queue, laufend, wartet auf Wiederholung, fehlgeschlagen), jeweils mit Link zu den Jobs in diesem Zustand; wie lange der älteste fällige Job gewartet hat; und eine Tabelle der Zahlen pro Typ und Zustand mit der Zeit, die jeder Typ einen Job warten lässt.
- **Jobliste** (`/admin/jobs/list`): die Jobs, die nicht abgeschlossen sind (in der Queue, laufend, wartet auf Wiederholung), oder die eines Zustands und eines Typs, eine Seite nach der anderen, mit dem Typ, der Zahl der Versuche, der Zeit, zu der er fällig ist oder fehlschlug, dem Zustand und der ersten Zeile des letzten Fehlers.
- **Ein Job** (`/admin/jobs/<id>`, vom Typ in der Liste aus): **Erneut ausführen** für einen fehlgeschlagenen Job, was vorher nachfragt, da ein Lauf wiederholen kann, was der Job vor dem Fehlschlag getan hat; jede Angabe des Jobs, seine ID und Schlüssel jeweils mit einer Kopieren-Schaltfläche; sein letzter Fehler und seine Payload, jeweils mit einer Kopieren-Schaltfläche; eine Tabelle der erfolglosen Versuche, neueste zuerst (Ergebnis, Endzeit, Worker und der Fehler); und ein Link zu den Jobs seines Typs.

Die Admin-API ist `GET /api/v1/admin/queue`, `GET /api/v1/admin/jobs?state=&type=`, `GET /api/v1/admin/jobs/{id}`, `GET /api/v1/admin/jobs/{id}/attempts` und `POST /api/v1/admin/jobs/{id}/retry`.
Ein abgeschlossener Job ist nicht mehr in der Queue, daher wird seine Seite nicht gefunden.

- Wartende, laufende und auf Wiederholung wartende Jobs lassen sich nur für die Queues auflisten, deren Jobs der Server hält: `queue = db`, `queue = cloudtasks` (die Jobs liegen auch in der Datenbank) und `queue = inline`. Fehlgeschlagene Jobs und der Versuchsverlauf liegen für jede Queue in der Anwendungsdatenbank, sodass sie sich immer auflisten lassen.
- Der Versuchsverlauf hält nur erfolglose Versuche (Wiederholungen und den Versuch, der schließlich fehlschlug). Verfolge einen erfolgreichen Lauf über die strukturierte `job`-Logzeile (die `job_id` trägt).
  Der Verlauf wird von der stündlichen Bereinigung entfernt, sobald seit dem Ende des Versuchs `job_attempt_retention` (Standard 30 Tage; siehe die [Einstellung](../reference/configuration.md#job_attempt_retention)) verstrichen ist.
  Der Verlauf enthält weder Payload noch Handler-Log, und eine Fehlermeldung wird auf ihre ersten 2.000 Bytes gekürzt. Job-Handler halten E-Mail-Adressen, IP-Adressen, Tokens und Beitragstexte aus Fehlermeldungen heraus.
  Für eine auf E-Mail-Job-Typen beschränkte Ansicht siehe [E-Mails versenden](email.md).

Ein Job, bei dem der Handler entschieden hat, dass er endgültig fehlschlug, und ein Job, der seine Wiederholungen aufgebraucht hat, wird in der Datenbank-Queue (`queue = db` und `queue = cloudtasks`) zu einem toten Job.
`my job` listet, wiederholt und löscht sie. Der Zustand „Fehlgeschlagen“ der Jobseite und die Admin-API können sie ebenfalls auflisten und wiederholen.

```sh
my job list                  # newest failures first: ID, type, run count, time of failure, last error (-limit sets the count, default 50)
my job retry <id> [<id>...]  # reset the attempt count and put the job back in the queue to run immediately
my job delete <id> [<id>...] # remove without running
my job types                 # print every job type of this release
```

- Wartet bereits ein Job mit demselben Idempotenzschlüssel, vertritt dieser Job den wiederholten. Job-Handler sind idempotent, daher ist es sicher, denselben Job beliebig oft zurückzulegen.
- Eine unbekannte ID wird ausgegeben, und das Kommando endet mit Exit-Code 1; die anderen IDs werden trotzdem verarbeitet. Exit-Code 2 bedeutet einen Aufruffehler.
- Ein toter Job, den niemand wiederholt, wird von der stündlichen Bereinigung entfernt, sobald seit dem Fehlschlag `dead_job_retention` (Standard 30 Tage; siehe die [Einstellung](../reference/configuration.md#dead_job_retention)) verstrichen ist.
- Tote Jobs von `queue = inline` liegen im Speicher des Serverprozesses, daher kann `my job` sie nicht bearbeiten (Exit-Code 1).

<!-- source: splitting-workers-by-job-type e3b4c4374dfb -->
### Worker nach Job-Typ aufteilen

Standardmäßig führt jeder Worker jeden Job-Typ aus, in der Reihenfolge, in der die Jobs fällig wurden, einen Job gleichzeitig pro Prozess (siehe [Mehrere Jobs gleichzeitig ausführen](#running-several-jobs-at-once)). Ein Schwall von Zustellungen verzögert dann E-Mail und Benachrichtigungen, und jeder Worker, den du hinzufügst, teilt seine Zeit auf alle auf. Mit `queue = db` kannst du einigen Typen eigene Worker geben:

- [`worker_job_types`](../reference/configuration.md#worker_job_types): Dieser Prozess führt nur diese Typen aus.
- [`worker_job_types_except`](../reference/configuration.md#worker_job_types_except): Dieser Prozess führt jeden Typ außer diesen aus.

Jede ist eine kommagetrennte Liste von [Job-Typen](../reference/jobs.md) (`my job types` gibt sie aus; `my config check` lehnt einen Namen ab, der keinen trifft), geschrieben als Name (`federation.deliver`) oder Präfix (`federation.*`). Setze sie pro Prozess, zum Beispiel mit `MY_WORKER_JOB_TYPES`. Du kannst nicht beide in einem Prozess setzen. Ein Name oder Präfix, der keinen Job-Typ trifft, hält den Start an, was auch einen Typ erfasst, den ein Release umbenannt hat, oder einen Typ, den es nur mit einer Einstellung gibt, die du nicht gesetzt hast. Sie funktionieren nicht mit `role = web` oder mit `queue = cloudtasks` oder `inline`, wo der Start ebenfalls fehlschlägt: Dort nennt ein Signal den auszuführenden Job, ein Prozess kann seine Jobs also nicht wählen.

Beim Start protokolliert ein Prozess mit einer der Einstellungen die Typen, die er überspringt. Er kann die Einstellungen der anderen Prozesse nicht sehen, **stelle also sicher, dass irgendein Prozess jeden Typ ausführt**, sonst warten diese Jobs ewig. Periodische Bereinigung und E-Mail sind ebenfalls Job-Typen. Jeder Prozess startet weiterhin die periodischen Jobs; welcher Prozess sie ausführt, hängt von diesen Einstellungen ab.

Entscheide anhand der Metriken, was du aufteilst, nicht anhand der Typnamen. `my.job.wait` (Prometheus: `my_job_wait_seconds`) zeigt, wie lange Jobs nach Typ gewartet haben, bevor sie starteten:

```promql
histogram_quantile(0.95, sum by (my_job_type, le) (rate(my_job_wait_seconds_bucket[15m])))
```

`sum by (my_job_type) (rate(my_job_duration_seconds_sum[1h]))` zeigt die Worker-Zeit, die jeder Typ verbraucht, und damit etwa auch, wie viele Prozesse er braucht. Ein Typ, der den Großteil der Zeit verbraucht und die anderen warten lässt, ist der, den du verschieben solltest.

Drei übliche Aufteilungen, mit einer Prozesszahl deiner Wahl:

| Aufteilung | Prozesse |
| --- | --- |
| Zustellungen daran hindern, den Rest zu verzögern | `worker_job_types = federation.fanout,federation.deliver` für die Zustellungen und `worker_job_types_except = federation.fanout,federation.deliver` für alles andere. Füge der ersten Gruppe bei einem Schwall Prozesse hinzu. |
| Ein Such-Backend daran hindern, den Rest zu verzögern | `worker_job_types = search.*` für die Such-Jobs und `worker_job_types_except = search.*` für alles andere ([Die Suche daran hindern, andere Jobs zu verzögern](search.md#keeping-search-from-delaying-other-jobs)). |
| E-Mail und Benachrichtigungen schnell halten | `worker_job_types = identity.verification_email,identity.password_reset_email,identity.password_notice_email,identity.closure_email,identity.deactivation_email,notification.email,notification.moderator_email,notification.supporter_email,notification.push,notification.post_subscribers,instance.notify_announcement,voice.notify_started` für diese und ein Worker ohne Einstellung für alles, der auch bei diesen Typen hilft, wenn er frei ist. Lass die Typen von Einstellungen weg, die du nicht gesetzt hast, sonst hält der Start an: `notification.push` braucht `vapid_private_key`, und `notification.email`, `notification.moderator_email` und `notification.supporter_email` brauchen `mail`. Siehe [Jobs, die von Einstellungen abhängen](../reference/jobs.md#jobs-that-depend-on-settings). |

Ein Präfix wie `identity.*` ist das Code-Modul, dem die Typen gehören, keine Art von Arbeit. `identity.*` erfasst auch Datenexporte, Importe und Ablauf-Jobs und verfehlt `instance.notify_announcement` und `voice.notify_started`, daher nennt eine Aufteilung für eine Art von Arbeit jeden Typ.

Halte diese zusammen, auf Workern mit derselben Einstellung. Jobs mit einem Reihenfolgeschlüssel laufen der Reihe nach, was auch immer ihr Typ ist. Wenn also nichts den früheren Job ausführt, wartet auch der spätere, und wenn sich die Gruppe des früheren Jobs staut, wartet der spätere auf sie:

| Typen | Was sie in Reihenfolge halten |
| --- | --- |
| `federation.fanout`, `federation.deliver` | Die Zustellungen eines Likes und seines Undo |
| `supporters.sync`, `supporters.command` | Die Änderungen eines Abos |

Beim Upgrade:

- Ein Prozess mit `worker_job_types` führt die Typen nicht aus, die eine neue Version hinzufügt. Prüfe die Release Notes und die übersprungenen Typen im Log. Ein Prozess mit `worker_job_types_except` führt sie aus, was ihn zum sichereren Ort für alles andere macht.
- Ein umbenannter Typ hält den Start an, lies also zuerst die Release Notes.

Um eine Aufteilung auszurollen, starte zuerst die neuen Worker und füge dann die Einstellung den alten hinzu. Um sie zurückzunehmen, entferne die Einstellung bei einer Gruppe, bevor du die andere stoppst. Jobs bleiben in der Warteschlange, nichts geht also verloren, und ein Typ, den eine Weile niemand ausführt, wartet nur.

**Einen Typ erkennen, den nichts ausführt.** Die Jobs eines solchen Typs warten, daher wächst `my.queue.oldest_ready_age` für diesen Typ, und die Alarme `MyQueueBacklog` und `MyQueueStalled` schlagen für den Typ an, der am längsten wartet. Um zu sehen, welcher es ist, oder um pro Typ zu alarmieren:

```promql
max by (my_job_type) (my_queue_oldest_ready_age_seconds) > 1800
```

Die Admin-Seite für Jobs zeigt dieselbe Wartezeit pro Typ. Ein Worker mit einer der Einstellungen lässt auch einen Job in Ruhe, dessen Typ gar keinen Handler hat (er zählt in der Metrik unter `other`); gibt es also keinen Worker ohne Einstellung, wartet ein solcher Job, statt zu einem toten Job zu werden.

<!-- source: running-several-jobs-at-once 2a1137a12e8f -->
### Mehrere Jobs gleichzeitig ausführen

[`worker_job_concurrency`](../reference/configuration.md#worker_job_concurrency) legt fest, wie viele Jobs ein Prozess gleichzeitig ausführt, von 1 bis 128. Es funktioniert mit `queue = db`; bei Cloud Tasks entscheidet die Plattform. Der Standard ist ein Job zugleich, und eine Einstellung von 1 verhält sich genauso.

Erhöhe es für Jobs, die ihre Zeit mit dem Warten auf andere Server verbringen, etwa `federation.deliver`. Diese brauchen wenig Speicher und wenige Datenbankverbindungen pro Job, sodass ein Prozess die Arbeit vieler erledigt: In einem lokalen Test, in dem ein Job nur 200 ms auf eine Antwort wartete, leistete ein Prozess mit 32 Slots so viel wie 32 Prozesse mit je einem Slot, bei einem kleinen Teil des Speichers. Diese Jobs taten nichts anderes, miss also deine eigene Instanz. Kombiniere es mit `worker_job_types` und gib jeder Art von Job einen Prozess mit seiner eigenen Zahl.

Behalte diese Grenzen im Blick:

- **Datenbankverbindungen**: Ein Job braucht eine Verbindung, um übernommen zu werden, für seine eigene Arbeit und um sein Ergebnis aufzuzeichnen. Die Einstellung muss unter dem Pool-Maximum liegen, sonst startet der Server nicht. Das Maximum ist `pool_max_conns` in `database_url`, standardmäßig das Größere aus 4 und der CPU-Anzahl, sodass eine vCPU 3 erlaubt, bis du es erhöhst. Prüfe, dass deine Datenbank oder dein Connection Pooler die Verbindungen all deiner Prozesse annimmt.
- **SQLite**: Es schreibt eine Transaktion zugleich, und in einem lokalen Test lief es bei 128 langsamer als bei 32.
- **CPU-intensive Jobs**: Slots teilen sich CPU und Speicher eines Prozesses. `media.process` wartet standardmäßig mit einem Limit von der Hälfte der CPUs auf seinen Zug, und das Warten zählt gegen die 5-Minuten-Lease des Jobs, sodass ein Prozess voller `media.process` über die Lease hinauslaufen und von vorn beginnen kann. Gib solchen Jobs einen Prozess mit wenigen Slots.
- **Ein Server zugleich**: Ein Prozess baut höchstens 8 Verbindungen zu einem einzelnen Host auf, sodass ein Prozess an einen einzelnen Server nicht schneller zustellt, als 8 Verbindungen erlauben, egal wie viele Slots er hat. Gehen die meisten Zustellungen an wenige große Server, helfen mehr Prozesse, wo mehr Slots es nicht tun.
- **Ein hängender Server hält Slots**: Slots, die auf einen langsamen Server warten, sind für andere Jobs nicht frei, halte Zustellungen also getrennt von E-Mails und Benachrichtigungen.
- **Deployments verbrauchen Versuche**: Wenn der Server anhält, werden Jobs, die nach `ShutdownTimeout` (standardmäßig 25 Sekunden) noch laufen, abgebrochen, und jeder abgebrochene Job verbraucht einen Versuch. Je mehr Slots langsame Jobs ausführen, desto mehr Versuche verbraucht jedes Deployment. Ein abgebrochener Job, dessen Ergebnis nicht rechtzeitig aufgezeichnet wird, wartet auf den Ablauf seiner Lease, bevor er wieder läuft.
- **Ein Absturz nimmt jeden Slot mit**: Geht dem Prozess der Speicher aus, warten alle seine Jobs auf den Ablauf ihrer Leases.

Um zu entscheiden, ob du es erhöhen solltest, lies `my.job.wait` pro `my.job.type`: Warten Jobs eines Typs lange, während `my.job.duration` überwiegend aus Warten auf andere Server besteht und CPU und Speicher niedrig bleiben, sollten mehr Slots helfen. Bleiben die Wartezeiten nach dem Erhöhen hoch, sieh dir die Datenbankverbindungen und das Host-Limit oben an, bevor du es erneut erhöhst.

Solange kein Job fällig ist, pollt ein Slot und die anderen warten, sodass ein untätiger Prozess die Datenbank so viel kostet wie ein Prozess mit einem Job.

<!-- source: waiting-for-a-database-connection 795c0fd7f769 -->
### Auf eine Datenbankverbindung warten

Mit `database = postgres` wartet jede Anweisung und Transaktion zuerst auf eine freie Verbindung des Pools des Prozesses.
[`database_acquire_timeout`](../reference/configuration.md#database_acquire_timeout) (Standard `60s`) begrenzt dieses Warten und nichts sonst: Eine lange Query oder
Transaktion, die ihre Verbindung bereits hat, läuft weiter, so lange sie auch dauert. `0` wartet ohne Limit. SQLite hat kein Limit an Verbindungen, auf die gewartet werden könnte, daher schlägt das Setzen dort beim Start fehl.

Wenn das Warten abläuft:

- Eine Anfrage wird mit `503 Service Unavailable` und `Retry-After: 5` beantwortet.
- Ein Job läuft 30 Sekunden später erneut und verbraucht keinen Versuch, eine ausgelastete Datenbank macht also nie einen Job zu einem toten Job.
- Die Metrik `db.client.connection.timeouts` zählt es, und der Alarm [`MyDBPoolWaiting`](alerts.md#mydbpoolwaiting) schlägt an.

Der Standard ist das Schreib-Timeout des Servers von 60 Sekunden: Eine Anfrage, die länger gewartet hat, könnte ihre Antwort ohnehin nicht mehr senden, der Standard lässt also keine Anfrage scheitern, die noch hätte gelingen können.
Wähle einen Wert danach, wie dein Server läuft:

- **Senke ihn**, um schnell Last abzuwerfen: Clients wiederholen nach `Retry-After`, statt ihre Anfragen offen zu halten. Halte ihn länger als deine langsamsten normalen Transaktionen, sonst beginnt ein kurzer Schwall mit `503` zu antworten.
- **Dimensioniere zuerst den Pool.** Ein Prozess braucht etwa so viele Verbindungen, wie er Arbeit gleichzeitig ausführt. `pool_max_conns` in `database_url` ist standardmäßig das Größere aus 4 und der CPU-Zahl, und alle Prozesse zusammen müssen unter dem Verbindungslimit der Datenbank bleiben.
  - Mit [`worker_job_concurrency`](#running-several-jobs-at-once) warten auch Slots auf Verbindungen, und die Einstellung muss unter dem Pool-Maximum bleiben.
  - Auf Google Cloud nimmt eine Instanz viele Anfragen gleichzeitig an: [Ein externes PostgreSQL](deploy-google-cloud.md#an-external-postgresql) dimensioniert seinen Pool.
- **Häufige `503` oder `db.client.connection.timeouts`** bedeuten, dass der Pool zu klein ist oder Queries langsamer wurden; ein längeres Timeout verbirgt das nur. Folge [`MyDBPoolSaturated`](alerts.md#mydbpoolsaturated).

<!-- source: cloud-tasks f14d9721ba24 -->
### Cloud Tasks

[Auf Google Cloud bereitstellen](deploy-google-cloud.md) richtet alles Folgende mit OpenTofu-Modulen ein.

Mit `queue = cloudtasks` bleiben die Jobs in der Datenbank (`database = postgres`), und Google Cloud Tasks weckt den Server nur für jeden von ihnen auf:

- Für jeden Job, den er speichert, wiederholt oder zurücklegt, erstellt der Server eine Task in `queue_cloudtasks_queue`, sobald die Transaktion committet. Die Task ruft
  `POST {queue_cloudtasks_url}/_internal/jobs` zu dem Zeitpunkt auf, an dem der Job fällig ist, mit der ID des Jobs als Body und einem OIDC-ID-Token von
  `queue_cloudtasks_service_account`. Ein Job, der mehr als 30 Tage im Voraus fällig ist, wird bei 30 Tagen signalisiert und erneut, wenn diese Task eintrifft.
- Der Prozess, der die Task empfängt, führt den Job aus und antwortet mit `204`, sobald sein Ergebnis erfasst ist, auch wenn der Job bereits lief oder nicht fällig ist.
  Ein Job, der fehlschlägt, wird vom Server zu seinem nächsten Versuchszeitpunkt wiederholt, nicht von Cloud Tasks. Cloud Tasks wiederholt eine Task nur, wenn die
  Antwort `5xx` ist (die Datenbank war nicht erreichbar) oder `401` (ein Token, das der Server ablehnte), daher braucht sein `retryConfig` keine Feinabstimmung.
- Kein Prozess fragt die Tabelle ab, und kein Prozess führt die periodischen Jobs von selbst aus. Ein Cloud-Scheduler-Job ruft
  `POST {queue_cloudtasks_url}/_internal/tick` auf, standardmäßig jede Minute (`* * * * *`), mit einem OIDC-Token desselben Dienstkontos (und mit der Audience von
  `queue_cloudtasks_audience`, wenn gesetzt). Jeder Tick reiht die periodischen Jobs ein, deren Zeitraum gekommen ist, einmal für die ganze Instanz (die Zeiträume werden im KV beansprucht,
  daher muss `kv` `db` oder `valkey` sein), und signalisiert erneut die Jobs, die seit über 2 Minuten fällig sind, ohne gelaufen zu sein, was ein verlorenes Signal behebt.
- `/_internal/jobs` und `/_internal/tick` werden von den Prozessen mit `role = all` oder `worker` bedient und akzeptieren nur ein von Google signiertes Token für
  `queue_cloudtasks_service_account` mit einer verifizierten E-Mail, dessen einzige Audience `queue_cloudtasks_audience` oder, standardmäßig, die aufgerufene URL ist. Jede andere Anfrage ist ein `401`.
- Eine Zustellung führt einen Job aus, höchstens für die 5-Minuten-Lease. Halte `queue_cloudtasks_dispatch_deadline` (Standard 10 Minuten) und das Anfrage-Timeout des
  Cloud-Run-Dienstes länger als das.
- Das eigene Dienstkonto des Servers braucht `roles/cloudtasks.enqueuer` auf der Queue und `roles/iam.serviceAccountUser` auf `queue_cloudtasks_service_account`.
  Es authentifiziert sich mit Application Default Credentials.

<!-- source: how-late-periodic-jobs-run 775fb52d1df8 -->
### Wie spät periodische Jobs laufen

Ein periodischer Job läuft bis zu einen Zeitraum seines Triggers nach seiner Zeit, plus die Zeit, die die Plattform zum Starten braucht. Das
bestimmt, wie spät die Erinnerungen und Beginn-Benachrichtigungen von Veranstaltungen ankommen und ihre Sprachchat-Räume öffnen, da `gathering.queue_due` sie startet: Eine Erinnerung
kommt bis zum Intervall der Tabelle zu spät. Die Werte sind das, was der Scheduler-Code und der Trigger jeder Plattform ergeben; sie wurden
nicht auf einem laufenden Server gemessen.

| Plattform | Was die periodischen Jobs startet | Erwartete Verzögerung einer Erinnerung oder eines Beginns einer Veranstaltung |
| --- | --- | --- |
| Self-Hosting und [Docker Compose](deploy-docker-compose.md) (`queue = db`) | Der Scheduler in jedem Prozess mit `role = all` oder `worker` prüft einmal pro Minute, ab dem Start des Prozesses, und reiht jeden Job einmal pro Zeitraum für den ganzen Server ein | Bis zu 1 Minute, plus etwa eine Sekunde, bis ein Worker den Job aufnimmt |
| [AWS](deploy-aws.md) (`queue = db`) | Derselbe Scheduler, in jedem Task | Bis zu 1 Minute, plus etwa eine Sekunde |
| [Google Cloud](deploy-google-cloud.md) (`queue = cloudtasks`) | Cloud Scheduler ruft `/_internal/tick` nach `queue.tick_schedule` auf, standardmäßig jede Minute; Cloud Tasks weckt dann den Server für jeden Job | Bis zu 1 Minute, plus der Start einer Instanz, wenn keine läuft (ein Kaltstart, meist einige Sekunden). Bei einem längeren `tick_schedule` bis zu diesem Intervall ([Wo Sorgfalt es verringert](deploy-google-cloud.md#where-care-reduces-it)) |

Eine Erinnerung, die ankäme, nachdem ihre Veranstaltung begonnen hat, wird nicht gesendet, ebenso wenig eine Beginn-Benachrichtigung, nachdem die Veranstaltung endete.
Bei einem Tick, der länger als 5 Minuten ist, können daher die kürzesten Erinnerungen verloren gehen.

<!-- source: routing-job-types-to-their-own-queues f4098db6215c -->
### Job-Typen auf eigene Queues leiten

Mit `queue = cloudtasks` kann ein Worker die Jobs, die er ausführt, nicht wählen: `worker_job_types` funktioniert nur mit `queue = db`, weil die Plattform entscheidet, wer ein Signal erhält.
Um einigen Job-Typen eigene Kapazität zu geben, sende ihre Signale an eine eigene Queue und lege die Kapazität dieser Queue auf der Plattform fest.
Nichts ändert sich, solange du keine Route setzt.

[`queue_cloudtasks_routes`](../reference/configuration.md#queue_cloudtasks_routes): `federation.*=projects/p/locations/l/queues/my-federation@https://federation-worker.example.com,notification.email=projects/p/locations/l/queues/my-mail`.
Eine Route ohne `@` und eine URL ruft `queue_cloudtasks_url` auf.

Die linke Seite ist ein Job-Typ aus der [Job-Referenz](../reference/jobs.md) oder ein Präfix. Ein Name gewinnt gegen ein Präfix und ein längeres Präfix gegen ein kürzeres.
Typen, die keine Route treffen, auch die, die ein späteres Release hinzufügt, nutzen die Standard-Queue (`queue_cloudtasks_queue`) und laufen daher weiter.
`my config check` prüft die Form der Routen; eine linke Seite, die keinen Job-Typ trifft, hält `my serve` beim Start an.

**Gib jedem Prozess dieselben Routen, `role = web` eingeschlossen.** Ein Signal sendet der Prozess, der den Job speichert: ein Web-Prozess für das meiste, was Nutzer tun, etwa den
ersten Job eines Beitrags, und ein Worker für die Jobs, die ein Job speichert, Wiederholungen, den nächsten Job eines Reihenfolgeschlüssels und den Durchlauf verlorener Signale.
Auf Google Cloud gibt `workers` jedem Worker seine eigene Queue und seinen eigenen Dienst und jedem Dienst dieselben Routen und Berechtigungen:
[Jobs auf Worker aufteilen](deploy-opentofu.md#splitting-jobs-into-workers).

Ein Prozess mit anderen Routen löst keinen Fehler aus und verliert keinen Job: Seine Signale gehen an eine andere Queue, und die Jobs laufen dort, außerhalb der Kapazität, die du festgelegt hast.
`my.queue.oldest_ready_age` zeigt das nicht, weil diese Jobs nicht warten. Um die Prozesse zu vergleichen, protokolliert jeder beim Start eine Zeile mit seinen `routes` und deren `routes_hash`:
Der Hash muss überall gleich sein. Um zu sehen, wohin Signale gehen, lies die eigenen Metriken der Plattform zu jeder Queue.

**Berechtigungen.** Die Zugangsdaten jedes Prozesses müssen an jede Queue der Routen senden dürfen: `roles/cloudtasks.enqueuer`.
Ein fehlgeschlagenes Senden wird protokolliert und nicht auf einer anderen Queue versucht, damit ein Typ, den du begrenzt hast, nicht auf die anderen überläuft. Diese Jobs starten spät, sobald der Durchlauf ihre Signale nach 2 Minuten erneut sendet.

**Kapazität.** Jeder Empfänger führt jeden Job aus, du wählst also, wie weit du sie trennst:

| Plattform | Ein Empfänger | Getrennte Empfänger |
| --- | --- | --- |
| Google Cloud | Ein Dienst. Jede Queue hat ihre eigenen `maxConcurrentDispatches` und `maxDispatchesPerSecond`; das Server-Limit des Dienstes wird geteilt | Ein Dienst pro Queue, benannt durch die URL in der Route, jeder mit eigenem Server-Limit und eigener Nebenläufigkeit. Jeder setzt `queue_cloudtasks_url` auf seine eigene URL |

Eine Queue braucht dieselbe Einrichtung wie die Standard-Queue: die Rolle auf der Queue.
Nur die Standard-Queue braucht den Tick.

**Was eine Queue hinzufügt.** Jede Queue ist eine Sache mehr, die du erstellen, beobachten und bezahlen musst, und wie viel das ausmacht, hängt von der Größe deines Servers ab.
Teile auf, was du in `my.job.wait` hast warten sehen, nicht alles.

**Der Durchlauf.** Jede Minute sucht der Durchlauf nach Jobs, die seit über 2 Minuten fällig sind, ohne zu laufen, bis zu 500 pro Route und 500 für die Standard-Queue, und signalisiert sie erneut.
Eine Queue, die nicht mithalten kann, erhält also bis zu 500 doppelte Signale pro Minute. Sie lassen keinen Job zweimal laufen, sind aber Nachrichten, für die du bezahlst, und sie hören auf, sobald sich der Rückstau auflöst.

**Reihenfolgeschlüssel** gelten weiter: Die Datenbank entscheidet, was laufen darf. Jobs eines Schlüssels können auf verschiedenen Queues liegen, aber eine festgefahrene Queue verzögert auch die späteren Jobs ihrer Schlüssel.

**Eine Queue hinzufügen, verschieben und entfernen.**

1. Erstelle die Queue und ihren Empfänger und gib jedem Prozess die Berechtigung, an sie zu senden.
2. Ändere die Routen in jedem Prozess. Solange sie sich unterscheiden, gehen Signale an beide Queues, und die Jobs laufen an beiden Orten.
3. Um eine Queue zu entfernen, nimm sie zuerst überall aus den Routen und warte, bis sie leer ist, bevor du sie löschst. Ein Signal in einer gelöschten Queue geht verloren, nicht sein Job: Der Durchlauf signalisiert den Job erneut, sobald er seit 2 Minuten fällig ist.
   Ein Cloud-Tasks-Task kann ein Signal für einen Job bis zu 30 Tage im Voraus halten, ein Job, der später als die Wartezeit fällig ist, wird also einfach vom Durchlauf signalisiert, wenn seine Zeit kommt.

<!-- source: health-checks e268d13fe7f2 -->
## Health-Checks

| Pfad | Art | Wann er 200 zurückgibt |
| --- | --- | --- |
| `GET /healthz`, `GET /health/live` | Liveness | Der Prozess kann HTTP beantworten. Er schaut nicht auf Abhängigkeiten wie die Datenbank (damit eine langsame Datenbank den Prozess nicht neu startet) |
| `GET /readyz`, `GET /health/ready` | Readiness | Die Datenbank ist erreichbar und das Schema ist angewendet |
| `GET /api/v1/instance/health` | Uptime-Monitoring, Statusseiten und Clients (von der `web`-Rolle bedient) | Dasselbe wie die Readiness. Das Ergebnis wird 60 Sekunden im Prozess zwischengespeichert |

- Cloud Run reserviert einige Pfade, die auf `z` enden, darunter `/healthz`, und beantwortet sie selbst mit einem `404`; rufe dahinter `/health/live` und `/health/ready` auf, die dasselbe beantworten.
- Ein 200 von jedem von ihnen hat den Body `{"status":"ok"}`.
- `/readyz` prüft die Datenbank bei jeder Anfrage, und gleichzeitige Anfragen teilen sich eine Prüfung.
- `/api/v1/instance/health` gibt bei Gesundheit 200 und `{"status":"ok","checked_at":"<RFC 3339>"}` zurück und bei einem Fehler 503 und `{"status":"unavailable","checked_at":...}` (`Cache-Control: no-store`).
  Jeder Prozess berührt die Datenbank höchstens einmal pro Minute, wie oft er auch überwacht oder angefragt wird, und er schaut weder auf das Zugriffstoken noch auf das Ratenlimit. Richte Uptime-Monitore auf diesen Endpunkt, nicht auf den Probe (`/readyz` oder `/health/ready`).
  Zu Einrichtung pro Dienst, Monitoring-Intervall und Kosten und dazu, wie Wartung behandelt wird, siehe die [Anleitung zum Uptime-Monitoring](monitoring.md).
- Alle drei loggen im Access-Log auf Debug-Stufe und erzeugen keinen Trace-Span.
- Ist die Readiness nicht erfüllt, ist die Antwort 503 mit einem RFC-9457-Body `application/problem+json`.
  Sein `title` wird gemäß `Accept-Language` übersetzt und enthält keine internen Details. Die Ursache (ein Verbindungsfehler oder die Namen fehlender Tabellen) geht ins Log.
- Eine Readiness-Prüfung wird nach 3 Sekunden abgebrochen.
- Die Readiness verlangt außerdem, dass die Datenbank den konfigurierten `host` und die `account_domain` enthält oder dass der Prozess sie erfassen konnte.
- **Schema angewendet** bedeutet, dass jede Tabelle, die die Schemadateien der Datenbank (`schema/<NN>-<module>/<dialect>.sql`) mit `CREATE TABLE` deklarieren, existiert
  (SQLite wird über `sqlite_schema` geprüft, PostgreSQL über das `current_schema()` der Verbindung). `my schema apply` wendet das ganze Schema an, daher bedeutet eine fehlende Tabelle, dass das Schema nicht angewendet oder älter als das Binary ist.
  Unterschiede bei Spalten und Indizes werden nicht geprüft; `my schema apply -dry-run` gibt aus, was sich ändern würde.

<!-- source: requests-that-bypass-the-cdn 2cca4127af07 -->
## Anfragen, die das CDN umgehen

Liegt der Server hinter einem CDN und bleibt seine eigene Adresse öffentlich, setze `origin_secret` und `origin_secret_header`, damit er ablehnt, was nicht über das CDN kam. Die Health-Checks bleiben erreichbar. Siehe [Anfragen ablehnen, die nicht über das CDN kamen](cdn.md#refuse-requests-that-did-not-come-through-the-cdn).

<!-- source: security-headers 8c5e82848149 -->
## Sicherheits-Header

Jede Antwort trägt `Strict-Transport-Security`, `X-Content-Type-Options`, `Referrer-Policy`, `Permissions-Policy`, `Cross-Origin-Opener-Policy`, `Cross-Origin-Resource-Policy: same-site` und `X-Frame-Options`
und, bei Antworten, die keine Dokumente sind, `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'; base-uri 'none'`.
Ein Handler überschreibt diese bei Antworten, die andere Werte brauchen, etwa ActivityPub-JSON und öffentliche Medien.
`Permissions-Policy` verweigert Kamera, Geolokalisierung, Zahlung und USB. Sie verweigert auch das Mikrofon, außer solange der Voice-Chat an ist (`voice` ist nicht `off`):
dann erlaubt sie das Mikrofon nur für Seiten der eigenen Origin des Servers (`microphone=(self)`), damit Browser in einem Anruf sprechen können.
Das HTML des Webclients erhält eine Dokument-CSP, die keine Nonce verwendet (`script-src 'self'` und so weiter).
Sie verlangt Trusted Types (`require-trusted-types-for 'script'`) und beschränkt die Policies, die erstellt werden können, auf die zwei des Clients (`trusted-types my-server-sanitized-html my-service-worker`).

<!-- source: web-client 0a0b62acc010 -->
## Web-Client

Ein Prozess, dessen `role` `web` oder `all` ist, liefert den in die Binärdatei eingebetteten Web-Client aus.

| Pfad | Inhalt | `Cache-Control` |
| --- | --- | --- |
| Jedes `GET`, das keine andere Route trifft (`/`, `/@alice`, `/@alice/status/{id}`, `/settings` und so weiter) | Die HTML-Hülle der SPA. Die Instanzeinstellungen werden in `<script type="application/json" id="config">` übergeben | `public, max-age=60, stale-while-revalidate=300`, ETag |
| `GET /assets/*` | Build-Dateien mit gehashten Namen | `public, max-age=31536000, immutable`, ETag |
| `GET /manifest.webmanifest` | Das Web-App-Manifest, aus den Instanzeinstellungen gebaut | `public, max-age=60, stale-while-revalidate=300`, ETag |
| `GET /sw.js`, `/icon.svg` und andere Dateien direkt unter dem Build | Unverändert ausgeliefert | `no-cache`, ETag |
| `GET /licenses/*` | Die Lizenzen dessen, was der Build bündelt (npm-Pakethinweise sowie `LICENSE-Unicode.txt` und `OFL.txt` für gebündelte Nicht-npm-Daten). Als `text/plain; charset=utf-8` ausgeliefert, egal welche Endung | `no-cache`, ETag |
| `GET /robots.txt`, `/.well-known/tdmrep.json`, `/.well-known/security.txt` | Aus den Instanzeinstellungen gebaut (siehe [Suchmaschinen und KI-Crawler](crawlers.md)) | `public, max-age=3600`, ETag |

- Pfade unter `/api`, `/oauth`, `/auth`, `/.well-known`, `/nodeinfo`, `/ap`, `/media`, `/uploads`, `/theme` (das Stylesheet des Server-Designs) und `/licenses` sowie `/robots.txt` liefern nie die Hülle; trifft keine andere Route, liefern sie ein 404-Problem.
- Die Hülle wird nach der ActivityPub-Content-Negotiation ausgeliefert. Wer einen Hüllenpfad mit einem ActivityPub-`Accept` anfordert, erhält 404 (Beitragsseiten leiten weiter, wie unten beschrieben).
  Die Hülle trägt immer `Vary: Accept`. Setzt du ein CDN davor, nimm die Art des `Accept` (ActivityPub oder HTML) in den Cache-Key auf, oder schließe die Hülle, `/ap/*` und `/@*` vom Caching aus.
  Liefere vom CDN nur `/assets/*` aus.
- Die Textdateien des Builds (Skripte, Stylesheets, SVG-Bilder, Lizenzhinweise) sind nur komprimiert eingebettet. Eine Anfrage, die Brotli akzeptiert, erhält sie mit `Content-Encoding: br`, eine, die gzip akzeptiert, mit `Content-Encoding: gzip`, und jede andere Anfrage erhält sie dekomprimiert, immer mit `Vary: Accept-Encoding` und einem ETag pro Kodierung.
  Ein Proxy davor braucht dafür keine eigene Kompression, und ein CDN muss `Vary: Accept-Encoding` (oder die Kodierung) in seinem Cache-Key für `/assets/*` behalten.
- Die Hülle und die Anmeldebildschirme tragen `Link`-Header, die die Stylesheets und das Einstiegsskript nennen, die die Seite zuerst lädt, woraus ein CDN wie Cloudflare (seine Funktion Early Hints) eigene Early Hints machen kann.
  Mit [`web_early_hints`](../reference/configuration.md#web_early_hints) = `true` sendet der Server sie auch zuerst in einer `103 Early Hints`-Antwort, damit der Browser die Dateien abruft, während der Server die Seite vorbereitet. Es ist standardmäßig aus, da ein Proxy, der ein 103 nicht weiterreichen kann, die Anfrage scheitern lassen kann:
  - Ein externer Application Load Balancer von Google Cloud antwortet 502 oder 503 (`backend_interim_response_not_supported`): Lass es dahinter aus, wie es die Cloud-Run-Bereitstellung tut.
  - nginx reicht es ab 1.29.0 weiter, mit gesetztem `early_hints`.
  - Das `reverse_proxy` von Caddy reicht es weiter; eine `encode`-Direktive verwirft es in Releases vor 2.8.
  - Browser verwenden ein 103 nur für eine Seite, zu der sie navigieren, und Chrome und Edge nur über HTTP/2 und HTTP/3.
- Eine Binärdatei, die ohne den Web-Client gebaut wurde (ein einfaches `go build`, ohne zuerst den Client zu bauen), protokolliert beim Start eine Warnung und liefert eine Hülle ohne Skripte aus.

<!-- source: federation-endpoints 7aaee8c9da42 -->
## Föderations-Endpunkte

Ein Prozess, dessen `role` `web` oder `all` ist, beantwortet diese Endpunkte, die andere Fediverse-Server nutzen, um deine Konten zu finden und anzuzeigen.

| Pfad | Inhalt |
| --- | --- |
| `GET /.well-known/webfinger` | WebFinger (RFC 7033). `resource` ist `acct:alice@{account_domain}` (`acct:alice@{host}` funktioniert ebenfalls), eine Actor-`id` oder die URL `/@alice`. `subject` ist immer das Handle auf der Seite der `account_domain` |
| `GET /.well-known/host-meta`, `/.well-known/host-meta.json` | Die WebFinger-URI-Vorlage (XRD / JRD) |
| `GET /.well-known/nodeinfo`, `GET /nodeinfo/2.1` | NodeInfo 2.1. `metadata.sourceCode` ist die Einstellung `source_url`, und `software.repository` wird bei einer `https:`-URL ebenfalls gesetzt (siehe [Den Quellcode veröffentlichen](#publish-the-source-code-agpl-30)) |
| `GET /ap/actors/{uuid}` | Der Actor (`publicKey` ist der RSA-`#main-key`). Eine Anfrage, die HTML bevorzugt, erhält ein 303 auf `/@{username}` |
| `GET /ap/actors/{uuid}/{outbox,followers,following,featured,featured-tags}` | Collections |
| `POST /ap/actors/{uuid}/inbox`, `POST /ap/inbox` | Die Inbox jedes lokalen Actors und die Shared Inbox. Anfragen müssen HTTP-signiert sein |
| `GET /@{username}` | Ein 302 auf die `id` des Actors bei einem ActivityPub-`Accept`; sonst das HTML des Webclients (siehe unten) |
| `GET /@{username}/status/{uuid}` | Bei einem ActivityPub-`Accept` ein 302 auf die `id` (`/ap/objects/{uuid}`) des Beitrags, wenn ihn ein abgemeldeter Besucher sehen kann, und 404, wenn nicht. Sonst das HTML des Webclients |

- Das Handle des Server-Actors ist `{account_domain}@{account_domain}` (dieselbe Form wie bei Mastodon).
- Ein Konto, das nicht existiert oder nicht gezeigt werden kann (gesperrt, deaktiviert, gelöscht oder entfernt), erhält immer dasselbe 404 (`Cache-Control: no-store`).
- Anonyme Antworten tragen `Cache-Control: public, max-age=60`. Antworten, die nach `Accept` variieren, tragen `Vary: Accept`.
  Nimm bei einem CDN, das `Vary: Accept` nicht verarbeiten kann, `/ap/*` und `/@*` vom Caching aus.

<!-- source: separate-handle-and-delivery-domains b1c23d994d0e -->
### Getrennte Handle- und Zustell-Domains

Unterscheidet sich `account_domain` von `host`, werden Anfragen an `/.well-known/webfinger`, `/.well-known/host-meta` und `/.well-known/nodeinfo` auf der Seite der `account_domain` mit einem 301 auf `host` umgeleitet (mit `Access-Control-Allow-Origin: *`).
Leitest du Anfragen für `account_domain` an diesen Server, liefert der Server diese Umleitung selbst.
Antwortet stattdessen ein anderer Server oder Reverse-Proxy, leite auf `https://{host}` um und behalte Pfad und Query-String bei.
