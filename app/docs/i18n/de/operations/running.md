<!-- source: running-the-server-start-stop-and-health-checks 5164430b3203 -->
# Den Server betreiben: Start, Stopp und Health-Checks

So startest und stoppst du den Server, beziehst sein Image und beobachtest ihn im Betrieb: Metriken, Traces, die Job-Warteschlange und Health-Checks.

<!-- source: start-the-server 0d5f8898a2f5 -->
## Den Server starten

```sh
my config check   # report every configuration problem at once
my config check -instance   # also warn about instance settings that are not set (opens the database)
my serve          # start according to the configured role (-config <file> reads a TOML file)
my version        # print the version of the binary
```

`my serve` validiert die Konfiguration, baut die Adapter für Datenbank, Job-Warteschlange und Schlüsselverwaltung auf und nimmt HTTP auf `listen` entgegen.
Dort spricht er reines HTTP/1.1 ohne TLS: Setze einen Reverse-Proxy oder Load Balancer davor, der HTTPS (und für die Clients HTTP/2 oder HTTP/3) terminiert und per HTTP/1.1 weiterleitet.
Die Echtzeit-Updates (`GET /api/v1/streaming`, die Streams der lokalen und föderierten Timelines und die Streams der Voice-Chat-Räume) sind langlebige Antworten, die alle 30 Sekunden (bei einem Voice-Raum alle 15) einen Heartbeat-Kommentar senden und im Leerlauf offen bleiben.
Der Proxy darf diese Antworten nicht puffern (nginx: Der Server sendet `X-Accel-Buffering: no`), und sein Idle- oder Read-Timeout muss länger sein als das Heartbeat-Intervall.
Er wendet das Schema nicht an: Wende es vor dem Start mit `my schema apply` an (siehe [Datenbank](database.md); Upgrades führen es als Teil von [Upgrade](database.md#upgrading) aus).
Im Docker-Compose-Setup (`deploy/docker/compose.yaml`; siehe [Mit Docker Compose bereitstellen](deploy-docker-compose.md)) führt ein einmaliger Dienst `schema` `my schema apply` aus, bevor der Server startet.
Ist das Schema nicht angewendet, startet der Prozess trotzdem, und die Readiness liefert 503, und er liefert nichts anderes aus, bis das Schema da ist.

Der erste `my serve` oder `my`-Befehl, der die Datenbank verwendet, verzeichnet darin `host` und `account_domain`. Weicht die Konfiguration von den verzeichneten Werten ab, protokolliert `my serve` `start failed` mit beiden Wertepaaren und endet mit Exit-Code 1, und die Befehle geben denselben Fehler aus. Die Ausnahmen sind `my schema apply` und `my db backup`, die nicht prüfen, und der Wartungsmodus, der die Datenbank nicht liest. `my config check -instance` und `my db upgrade -check` prüfen, verzeichnen aber nichts.
Ein Prozess, der die Datenbank beim Start nicht lesen konnte, ist nicht bereit, bis er es kann, und führt bis dahin keine Jobs aus und beantwortet jede Anfrage außer den drei Health-Checks unten (`/healthz`, `/readyz`, `/api/v1/instance/health`) mit `503` und `Retry-After`. Das tut auch ein Server, der vor `my schema apply` gestartet wurde. Findet er dann andere Werte, hält er mit demselben Fehler an.

Im Profil `production` prüft `my config check` außerdem, ob die gültigen Einstellungen zu einer Produktionsserver passen: Fehler, etwa ein `host`, den andere Server nicht erreichen können, oder ein KEK, der nicht zufällig ist, lassen ihn mit 1 enden, Warnungen, etwa `mail = none`, nicht.
`my serve` protokolliert beides beim Start als Warnungen, startet aber trotzdem, führe also `my config check` als Schritt der Bereitstellung aus.
Die [Produktions-Checkliste](production-checklist.md) listet die Prüfungen auf und was du vor dem Livegang von Hand verifizieren solltest.

| `role` | HTTP | Job-Worker |
| --- | --- | --- |
| `all` (Standard) | Alle Routen | Läuft |
| `web` | Alle Routen | Läuft nicht |
| `worker` | Nur Health-Checks (für Plattform-Probes) | Läuft |

Logs gehen als JSON Lines auf die Standardfehlerausgabe. Jede Zeile hat `level` (`DEBUG`, `INFO`, `WARN`, `ERROR`) und `severity`, dieselbe Stufe in der Form, die Cloud Logging liest (`DEBUG`, `INFO`, `WARNING`, `ERROR` und `CRITICAL` für einen Fehler, der den Prozess anhält).
Logs, die bei der Bearbeitung einer Anfrage geschrieben werden, tragen eine `request_id`, und dieselbe ID wird im Antwort-Header `X-Request-Id` zurückgegeben.
Besteht das `X-Request-Id` der Anfrage aus höchstens 128 Zeichen aus Buchstaben, Ziffern und `-_.:`, wird es unverändert verwendet, als die ID, die ein Load Balancer vergeben hat.
Logs innerhalb eines Traces (Anfragen und Jobs) tragen außerdem `trace_id` und `span_id` (siehe [Traces](#traces)).

Jede HTTP-Anfrage und jede Job-Ausführung schreibt am Ende eine Zeile. Die Zeile hat dieselben Namen und Werte wie die Labels der [Metriken](#metrics) unten,
sodass du auf Cloud Run mit `metrics_exporter = none` aus diesen Zeilen logbasierte Metriken von Cloud Logging (Zähler und Verteilungen) bauen kannst.

| Zeile (`msg`) | Felder |
| --- | --- |
| `http request` (`INFO`; Health-Checks verwenden `DEBUG`) | `http.request.method`, `http.route`, `my.surface` und `http.response.status_code` (eine Zahl) haben dieselben Werte wie die Labels von `http.server.request.duration`. Außerdem `duration_seconds` (eine Sekundenzahl), `url.path` (der Pfad ohne Query-String), `http.response.body.size` und `request_id`. `http.route` und `my.surface` fehlen bei Anfragen, die keine Route erreicht haben (Health-Checks, vor dem Routing abgelehnte Anfragen), und die Metrik zählt sie nicht. Eine Anfrage, deren Client ging, bevor der Server antworten konnte, hat den Status `499` statt eines 5xx, es sei denn, ein Handler hat einen Panic ausgelöst; der Client hat nichts erhalten, sie zählt also nicht als Serverfehler |
| `job` (`INFO` bei Erfolg und bei `postponed`, `WARN` bei `retry`, `ERROR` bei `dead`) | `my.job.type` und `my.outcome` haben dieselben Werte wie die Labels von `my.job.duration`. Außerdem `duration_seconds` (`my.job.duration`), `wait_seconds` (`my.job.wait`), `job_id`, `attempt` und bei Fehlschlag `error` |
| `error report` (`ERROR`; `CRITICAL` bei einem Fehlschlag, der den Prozess anhält) | Eine Zeile pro gemeldetem Fehler ([Fehlerberichte](#error-reports)) |

<!-- source: error-reports 3d780744461c -->
### Fehlerberichte

Die Anleitung [Fehlerberichte](error-reporting.md) behandelt, wo du diese Zeilen auf jeder Plattform liest, den Bildschirm „Errors“, Benachrichtigungen und das Senden von Berichten an einen Error-Tracker.

Der Server meldet die Fehler, die die Aufmerksamkeit des Betreibers brauchen, als je eine `error report`-Zeile:

- ein Panic in einem HTTP-Handler, einem Job oder einer Hintergrundaufgabe;
- ein Fehler hinter einer 5xx-Antwort der Client-API, der Admin-API, der Anmeldeseiten, von ActivityPub, OAuth oder Medien;
- ein Job, der endgültig fehlschlägt (`dead`): Ihm gingen die Wiederholungen aus, oder er weigerte sich, erneut zu laufen. Zustellungen, die aufgegeben wurden, weil der andere Server ständig fehlschlug, werden nicht gemeldet;
- ein Fehlschlag beim Start: eine ungültige Konfiguration oder eine Datenbank oder ein Adapter, der sich nicht einrichten lässt.

4xx-Antworten, Job-Wiederholungen, der Wartungsmodus, fehlgeschlagene Readiness-Prüfungen, Anfragen, deren Client gegangen ist, und durch andere Server verursachte Fehler werden nicht gemeldet; die Metriken und die übrigen Logzeilen decken sie ab.

Wenn der Client einer Anfrage geht, werden die anderen Fehlerzeilen des Servers zu dieser Anfrage auf `INFO` statt `ERROR` geschrieben, da die meisten nur besagen, dass die Anfrage abgebrochen wurde.

Die Zeile hat die Form, die Google Cloud Error Reporting liest: Auf Cloud Run erscheinen die Berichte ohne Einrichtung als Fehler gruppiert in Error Reporting. Auf anderen Plattformen ist sie eine einfache JSON-Zeile:

| Feld | Wert |
| --- | --- |
| `@type`, `serviceContext` | Der Ereignistyp von Error Reporting und `{"service": "my", "version": "<version>"}` |
| `my.error.fingerprint` | 32 Hexadezimalziffern, die für jedes Auftreten desselben Fehlers gleich sind: gebildet aus der Quelle, dem Ort, dem Fehlertyp und den Funktionen des Servers im Stack, ohne Zeilennummern |
| `my.error.source`, `my.error.kind` | `server` (oder `web`) und `panic` oder `error` |
| `my.error.where` | Wo es geschah: `<surface> <method> <route>` bei einer Anfrage (etwa `client_api GET /api/v1/statuses/{id}`), der Job-Typ bei einem Job, der Aufgabenname bei einer Hintergrundaufgabe, `startup` bei einem Fehlschlag beim Start |
| `my.role`, `deployment.environment.name` | Die `role` des Prozesses und `errors_environment` |
| `request_id`, `job_id`, `attempt`, `trace_id`, `span_id` | Die IDs, um die Anfrage- oder Job-Zeilen desselben Fehlers zu finden, wenn er sie hat |
| `message` | Zuletzt in der Zeile: `<type>: <message>`, ein Zeilenumbruch und der Go-Stack |

Bevor ein Bericht den Prozess verlässt, werden in der Meldung URLs, E-Mail-Adressen, `@user@host`-Handles, IP-Adressen, UUIDs und ULIDs sowie Zahlen ab 10 Stellen durch `<url>`, `<email>`, `<acct>`, `<ip>`, `<id>` und `<n>` ersetzt, und sie wird auf 1 KiB gekürzt.
Der Stack behält seine Dateipositionen, aber nicht die Argumentwerte seiner Funktionszeilen (sie erscheinen als `(...)`), und wird auf 16 KiB gekürzt. Kein Bericht trägt einen Request-Body, Header, eine Query, einen Cookie, ein Konto, einen Nutzernamen, eine IP-Adresse oder einen User-Agent.
Keine andere Zeile trägt einen Stack, daher findet Error Reporting die Stacks nur in diesen Zeilen.

Ein Feldname ist ein einzelner Schlüssel, der `.` enthält; er ist nicht verschachtelt. Setze ihn in Cloud-Logging-Filtern und -Extraktionen in Anführungszeichen (zum Beispiel `jsonPayload."http.route"` und `EXTRACT(jsonPayload.duration_seconds)` für einen Verteilungswert).
Felder, die als Labels verwendet werden, nehmen nur Werte aus einer endlichen Menge an; `url.path`, `request_id` und `job_id` sind nie Labels.

Mit `metrics_exporter = emf` sind diese beiden Zeilen selbst die EMF-Zeilen. Die Zeile `http request` trägt den Wert von `http.server.request.duration`, und die Zeile `job` trägt die Werte von `my.job.duration` und `my.job.wait` (Schlüssel nach den Metriken benannt),
dazu die Direktive `_aws`, `service.name`, `service.version` und `my.role`. Diese drei Metriken werden in keiner anderen Zeile geschrieben. Felder, die du in `metrics_emf_dimensions` wählst, werden als Strings geschrieben, wie EMF es verlangt (zum Beispiel ist die Dimension `http.response.status_code` `"200"`).
Logs gehen auf die Standardfehlerausgabe und die übrigen EMF-Zeilen auf die Standardausgabe. Der Log-Treiber `awslogs` von ECS sendet beide an CloudWatch Logs, sodass Metriken aus beiden extrahiert werden.

<!-- source: docker-compose-files df45a902f45b -->
## Docker-Compose-Dateien

`deploy/docker/` enthält, was einen Server bereitstellt. `compose.yaml` läuft allein, und die anderen Dateien werden darübergelegt.

| Datei | Fügt hinzu | Nutze sie, wenn |
| --- | --- | --- |
| `compose.yaml` | Den Server und seine einmaligen Datenbankdienste, auf SQLite und Mediendateien im Volume `data` | Immer |
| `compose.postgres.yaml` | PostgreSQL in einem Container, im Volume `postgres` | Du PostgreSQL auf diesem Rechner willst. Für einen PostgreSQL-Server anderswo lass sie weg und setze `MY_DATABASE` und `MY_DATABASE_URL` in `.env` |
| `compose.s3.yaml` | Medien in einem S3-kompatiblen Bucket; mit `COMPOSE_PROFILES=garage` auch ein [Garage](https://garagehq.deuxfleurs.fr/)-Speicher mit einem Knoten im Volume `garage` | Medien wachsen über eine Platte hinaus oder du willst sie in einem Bucket. Ohne das Profil ist der Speicher der externe der `MY_BLOB_S3_*`-Einstellungen |
| `compose.caddy.yaml` | Caddy, mit TLS-Zertifikaten auf den Ports 80 und 443 | `MY_HOST` eine öffentliche Domain ist und nichts anderes auf diesem Rechner TLS terminiert |
| `compose.build.yaml` | Baut das Image aus einem Checkout | Du deinen eigenen Build betreibst |

Wähle die Datenbank und den Medienspeicher vor dem ersten Start: Die Daten ziehen nicht von selbst um, wenn sich die Dateien ändern
([Sicherung](backup.md) zeigt, was umziehen muss).

**Schichte Dateien** mit `-f`-Optionen in dieser Reihenfolge oder nenne sie einmal in `.env`, damit jeder spätere `docker compose`-Befehl
(`exec`, `logs`, `pull` und die in den anderen Anleitungen) dieselben Dateien verwendet:

```sh
cd deploy/docker
cp example.env .env
# In .env: COMPOSE_FILE=compose.yaml:compose.postgres.yaml:compose.caddy.yaml
```

Eine Datei namens `compose.override.yaml` im Verzeichnis wird nur gelesen, wenn weder `-f` noch `COMPOSE_FILE` Dateien nennt. Mit
ihnen führe sie ebenfalls auf.

**Einstellungen.** Kopiere `example.env` nach `.env`. Compose liest `.env`, um die Variablen seiner Dateien zu füllen, und reicht die gesamte
Datei an die Container des Servers weiter, sodass jede Einstellung der [Konfigurationsreferenz](../reference/configuration.md) funktioniert,
indem sie dort gesetzt wird. Die Vorlage listet sie alle auf, mit je einem Satz dazu.

- Der Key-Encryption-Key bleibt außerhalb von `.env`, in der Datei `secrets/kek`, und die Container lesen ihn als Docker-Secret
  `/run/secrets/my_kek`, sodass er nie zur Umgebungsvariable wird. Eine Zeile `MY_KEK` in `.env` hält den Server an, der
  `MY_KEK` und `MY_KEK_FILE` zusammen ablehnt ([den Schlüssel verwalten](key-management.md)).
- Der Server läuft als Nutzer `65532`, und Compose bindet die Datei mit den Berechtigungen ein, die sie hat, daher muss die Datei
  für andere lesbar sein. Bewahre sie in einem Verzeichnis auf, das nur du betreten kannst, wie es die Befehle in `example.env` tun.
- Die anderen Secrets erreichen die Container als Umgebungsvariablen. Um eines stattdessen als Datei zu übergeben, binde es mit einem Secret
  in einer eigenen `compose.override.yaml` ein und setze sein `<NAME>_FILE`, wie `compose.yaml` es für den Schlüssel tut.
- `.env` enthält Secrets: `chmod 600 .env`, und bewahre eine Kopie bei deinen Sicherungen auf.
- `MY_LISTEN` bleibt `:8080`, da die Dateien sich mit diesem Port verbinden.

**TLS mit Caddy.** Setze `MY_HOST` auf eine Domain, deren DNS auf den Rechner zeigt, mit aus dem Internet erreichbaren Ports 80 und 443,
und entferne `MY_WEB_ORIGIN` aus `.env`. Caddy holt und erneuert das Zertifikat bei der ersten Anfrage selbst.
Der Server vertraut dem `X-Forwarded-For` der Standard-Adresspools von Docker; setze `MY_TRUSTED_PROXIES`, wenn deine abweichen.

**Das mitgelieferte Garage.** `compose.s3.yaml` mit `COMPOSE_PROFILES=garage` braucht die Werte, die `example.env` an seinem Anfang auflistet.
Der einmalige Dienst `garage-init` bereitet den Speicher bei jedem `up` vor. Er bleibt innerhalb des Compose-Netzwerks, da der Server
die Medien selbst ausliefert. Ein Knoten ist eine Platte: Sichere das Volume `garage` oder den Bucket ([Sicherung](backup.md#media-in-a-bucket)).

<!-- source: get-the-image 238d00ea9826 -->
## Das Image beziehen

Das Server-Image ist `ghcr.io/mycroblog/my`, öffentlich, für `linux/amd64` und `linux/arm64`, mit jedem Adapter.

| Tag | Zeigt auf | Nutze es für |
| --- | --- | --- |
| `0.1.0` | Genau diese Version, für immer | Eine Version benennen |
| `0.1` | Den neuesten Patch von `v0.1` | Docker Compose, das bei jedem `up` das Schema anwendet |
| `0.1.0-rc.1` | Dieses Prerelease | Ein Release ausprobieren, bevor es erscheint |

Es gibt kein `latest` und kein `0`: Beide würden eine Minor-Version überschreiten, und jedes Upgrade auf eine neue Minor-Version muss [über die vorherige](database.md#skipping-releases) gehen. Nenne in jedem Befehl eine Version; `docker pull ghcr.io/mycroblog/my` allein scheitert mit `manifest unknown`.
Ein Patch-Release kann ebenfalls `my schema apply` und `my db upgrade` erfordern, folge also außerhalb von Docker Compose für jede Version [Upgrade](database.md#upgrading). Fixiere in der Produktion den Digest, den die Release Notes nennen:

```text
ghcr.io/mycroblog/my:0.1.0@sha256:...
```

Wo du es betreibst:

- **Docker Compose**: `deploy/docker/compose.yaml` betreibt das Image aus `MY_VERSION` in `.env` ([Docker-Compose-Dateien](#docker-compose-files)). Um es stattdessen aus einem Checkout zu bauen, füge die Überschreibung `compose.build.yaml` hinzu (`docker compose -f compose.yaml -f compose.build.yaml up -d --build`) und setze `MY_VERSION` auf einen eigenen Namen, da er dein lokales Image benennt und das ist, was `my version` ausgibt.
- **Kubernetes**: Beziehe es unverändert von `ghcr.io`.
- **Cloud Run**: Nutze das `ghcr.io`-Image direkt. Eine Bereitstellung löst den Tag in einen Digest auf.
- **AWS (ECS, EKS)**: Kopiere das Image nach Amazon ECR oder nutze einen ECR Pull-Through-Cache, der ghcr.io-Zugangsdaten braucht.
- **Ein kleineres Image**: `docker build --build-arg VERSION=<name> --build-arg GO_TAGS=... -t my:<name> .` in einem Checkout baut eines mit weniger Adaptern ([Build-Tags](../reference/build-tags.md)). Es ist dein eigener Build und wird daher von den Prüfungen unten nicht abgedeckt.

<!-- source: verify-an-image b696c257662d -->
### Ein Image verifizieren

Jedes veröffentlichte Image hat eine über Sigstore signierte Provenance, die den Digest an den Workflow und den Commit bindet, der es gebaut hat, sowie eine SBOM. Mit der [GitHub CLI](https://cli.github.com/), und bei `ghcr.io` angemeldet, falls der Befehl danach fragt:

```sh
gh attestation verify oci://ghcr.io/mycroblog/my@sha256:... -R mycroblog/my \
  --signer-workflow mycroblog/my/.github/workflows/release.yml
docker buildx imagetools inspect ghcr.io/mycroblog/my@sha256:... --format '{{ json .SBOM }}'
```

Der erste Befehl scheitert, es sei denn, das Image wurde vom Release-Workflow dieses Repositorys gebaut. Er gibt den Commit aus, der der Commit des Tags der Version ist; das Label `org.opencontainers.image.revision` des Images nennt denselben.

<!-- source: publish-the-source-code-agpl-30 2f3b73ce9dfc -->
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

<!-- source: metrics 2dea3a7f8f83 -->
## Metriken

Die Einstellung `metrics_exporter` (Standard `none`) exportiert Dienstmetriken. Die zugehörigen Einstellungen stehen in der [Konfigurationsreferenz](../reference/configuration.md).

| `metrics_exporter` | Pfad | Geeignet für |
| --- | --- | --- |
| `none` | Es wird nichts exportiert. Die Instrumentierung kostet fast nichts | Cloud Run (nutze stattdessen die Metriken der Plattform und logbasierte Metriken) |
| `prometheus` | `GET /metrics` auf `metrics_listen` (Standard `127.0.0.1:9464`). Es ist ein vom öffentlichen `listen` getrennter Listener, ohne Authentifizierung, und läuft für jede `role` | Selbst gehostet (abgefragt von einem Prometheus-kompatiblen Server oder KEDA) |
| `otlp` | Wird per OTLP/HTTP an einen OpenTelemetry Collector gesendet. Das Ziel stammt aus den Standard-OTel-Umgebungsvariablen wie `OTEL_EXPORTER_OTLP_ENDPOINT` | Setups, in denen du einen Collector betreiben kannst |
| `emf` | JSON-Zeilen im CloudWatch Embedded Metric Format. Anfrage- und Job-Metriken fahren auf den abschließenden Logzeilen oben mit; die übrigen werden auf die Standardausgabe geschrieben, eine Zeile pro Label-Satz, wenn eine Anfrage oder ein Job endet | AWS ECS |

Die Metriken sind unten aufgeführt. In Prometheus wird `.` in einem Namen zu `_`, und ein Einheitensuffix (`_seconds` und `_total` für einen Zähler) wird angehängt.

| Metrik | Art | Labels |
| --- | --- | --- |
| `http.server.request.duration` | Histogramm, Sekunden | `http.request.method`, `http.route` (der Pfad des Routenmusters; `other`, wenn keines passt), `http.response.status_code`, `my.surface` (`client_api` / `admin` / `activitypub` / `oauth` / `media` / `web` / `payments` / `voice` / `queue`) |
| `http.server.active_requests` | UpDownCounter | `my.surface` |
| `my.inbox.activities` | Zähler | `activity.type` (ein bekannter Activity-Typ oder `other`), `my.outcome` (`accepted` / `duplicate` / `rejected` / `ignored` / `error`), `my.inbox.rejection` (nur mit `rejected`: `signature` für eine Signatur oder einen Integritätsnachweis, der fehlt, ungültig ist oder nicht verifiziert, oder dessen Schlüssel nicht abgerufen werden konnte; `suspended` für eine gesperrte Domain; `federation_off`; `rate_limited`; `invalid` für eine Aktivität ohne gültige `id`, `type`, `actor` oder `object`) |
| `my.relay.activities` | Zähler | `my.outcome` (`admitted` / `over_limit` / `restricted` / `not_receiving`: ob eine von einem Relay getragene Aktivität aufgenommen wurde und warum sie verworfen wurde; siehe [Föderations-Relays](relays.md)) |
| `my.move.activities` | Zähler | `my.outcome` (`recorded` / `duplicate` / `not_own_actor` / `same_account` / `target_unavailable` / `alias_missing` / `target_moved` / `suspended` / `too_soon`: ob ein empfangenes `Move` verzeichnet wurde und warum es verworfen wurde) |
| `my.delivery.attempts` | Zähler | `my.outcome` (`success` / `retry` / `gone` / `failed`; `gone` umfasst ein 410 und Zustellungen, die übersprungen wurden, weil der Server nicht verfügbar oder gesperrt ist) |
| `my.delivery.duration` | Histogramm, Sekunden | `my.outcome` |
| `my.plugin.failures` | Zähler | `my.plugin.kind` (eine Art von [Plugin](plugins.md)), `my.plugin.code` (der Connect-Code des Fehlschlags) |
| `my.plugin.slot_wait` | Histogramm, Sekunden | `my.plugin.kind` (wie lange ein Aufruf auf seinen Platz unter den gleichzeitig laufenden Aufrufen dieser Art wartete) |
| `my.hook.decisions` | Zähler | `hook`, `mode`, `outcome` (die Entscheidungen, die ein [Decision Hook](decision-hooks.md) verzeichnet hat) |
| `my.hook.failures` | Zähler | `hook`, `outcome` (`timeout`, `error`, `invalid` oder `breaker_open`) |
| `my.hook.duration` | Histogramm | `hook` (Sekunden pro Aufruf des Plugins) |
| `my.job.duration` | Histogramm, Sekunden | `my.job.type` (ein registrierter Job-Typ oder `other`), `my.outcome` (`success` / `retry` / `dead` / `postponed`: `postponed` ist ein Lauf, der keine Arbeit getan hat und keinen der Versuche des Jobs verbraucht, etwa ein Abruf einer Link-Karte, der auf seinen Host wartet) |
| `my.job.wait` | Histogramm, Sekunden | `my.job.type` |
| `my.link_card.host_pauses` | Zähler | `http.response.status_code` (`429` / `503`). Einer pro Pause der Link-Karten-Abrufe von einem Host; siehe [Link-Vorschauen](link-previews.md#limits-per-host) |
| `my.translation.requests` | Zähler | `my.outcome` (`author` / `cache` / `provider` / `no_provider` / `limit` / `unsupported` / `failed`). Einer pro Anfrage, einen Beitrag zu übersetzen, den der Betrachter sehen darf, oder den Beitrag oder Kommentar einer Meldung, den ein Moderator anfordert: beantwortet aus den eigenen Worten des Autors, aus einer aufbewahrten Übersetzung oder durch einen Anbieter, oder abgelehnt, weil kein Anbieter den Beitrag erhalten darf, ein Monatslimit erreicht ist, kein Anbieter die Sprache übersetzt oder kein Anbieter eine brauchbare Übersetzung geliefert hat. |
| `my.translation.characters` | Zähler | `my.provider` (die ID eines Übersetzungsanbieters). Die Zeichen von Beiträgen und Meldungskommentaren, die an den Anbieter gesendet wurden, ohne Markierungen, einschließlich fehlgeschlagener Aufrufe. Das ist es, was die [Monatslimits](instance-settings.md#admin-screens-and-the-client-api) zählen, und es kann von dem abweichen, was der Anbieter abrechnet |
| `my.timeline.delivery.lag` | Histogramm, Sekunden | Keine (von der Erstellung eines lokalen Beitrags bis zu seinem Schreiben in die Home-Timelinen und der Benachrichtigung an Streams) |
| `my.stream.connections` | UpDownCounter | `my.stream.kind` (`user`, `timeline` für die Streams der lokalen und föderierten Timelines, `voice` für die Streams der Voice-Chat-Räume) |
| `my.accounts.registered` | Zähler | Keine |
| `my.statuses.created` | Zähler | `my.visibility` |
| `my.billing.imports` | Zähler | `my.billing.source` (`aws` / `gcp` / `plugin`), `my.outcome` (`success` / `failed`), `error.type` (nur mit `failed`: die Art des Fehlschlags, wie in [Cloud-Nutzungskosten importieren](billing-import.md)). Jeder Versuch zählt, einschließlich der Wiederholungen und der Fehlschläge, die auf den nächsten täglichen Import warten |
| `my.voice.turn.credentials` | Zähler | `my.voice.turn` (`coturn` / `cloudflare`), `my.outcome` (`issued` / `reused` / `failed`). Eines pro TURN-Zugangsdatum, das ein Voice-Chat-Teilnehmer beim Betreten eines Raums oder beim Erneuern seiner Verbindung angefordert hat: `reused` heißt, früher für dieselbe Verbindung ausgestellte Cloudflare-Zugangsdaten wurden erneut zurückgegeben, und `failed` heißt, Cloudflare war nicht erreichbar oder lehnte ab, sodass der Teilnehmer einen Fehler erhielt und sich nicht verbinden konnte |
| `my.errors.exported` | Zähler | `my.outcome` (`success` / `dropped` / `failed`: einer pro Fehlerbericht für den Dienst von `errors_exporter`. `dropped` heißt, der Bericht wurde nicht gesendet, weil bereits 1.000 Berichte warteten oder der Dienst den Server zum Drosseln aufforderte, und der Server wartet so lange, wie der Dienst es verlangt, bevor er wieder sendet; `failed` heißt, der Dienst war nicht erreichbar oder lehnte den Bericht ab. Ein Bericht wird einmal gesendet und nie wiederholt, und die Berichte, die 2 Sekunden nach Beginn eines Shutdowns noch warten, gehen verloren) |
| `my.queue.ready` | Gauge | `my.job.type` (die Zahl der ausführbaren Jobs in der Datenbank-Warteschlange) |
| `my.queue.oldest_ready_age` | Gauge, Sekunden | `my.job.type` (wie lange der älteste ausführbare Job jeder Art in der Datenbank-Warteschlange gewartet hat; eine Art ohne ausführbaren Job hat keinen Wert) |
| `my.queue.dead` | Gauge | Keine |
| `my.instance.users`, `my.instance.statuses` | Gauge | Keine (die Zahlen lokaler Nutzer und Beiträge, wie in NodeInfo) |
| `my.media.stored_bytes` | Gauge, Bytes | `my.media.origin` (`local` / `remote_cache`) |
| `my.db.size` | Gauge, Bytes | Keine (die Größe der Datenbank: `pg_database_size` bei PostgreSQL; die Datenbankdatei ohne ihr Write-Ahead-Log bei SQLite) |
| `my.billing.import.failing` | Gauge | `my.billing.source` (1, wenn der neueste Import der Quelle fehlschlug, sonst 0) |
| `my.federation.instances` | Gauge | `my.federation.instance.state` (`available`: Server, an die Zustellungen weiter gehen, die aber an mindestens einem Tag seit ihrer letzten Antwort fehlschlugen; `unavailable`: Server, deren Zustellungen pausiert sind) |
| `my.metrics.snapshot.age` | Gauge, Sekunden | `my.metrics.snapshot.part` (`queue` / `totals`) |
| `my.errors.reported` | Zähler | `my.error.source` (`server` / `web`), `my.error.kind` (`panic` / `error`). Einer pro [Fehlerbericht](#error-reports) |

Health-Checks (`/healthz`, `/readyz`) werden nicht gemessen. Kein Label enthält eine entfernte Domain, ein Konto, eine URL oder eine Client-IP.
`my.queue.*`, `my.instance.*`, `my.media.stored_bytes`, `my.db.size`, `my.federation.instances` und `my.billing.import.failing` sind Werte für die ganzen Server: Jeder Prozess meldet denselben Wert aus einem Snapshot, der im Key-Value-Speicher liegt.

<!-- source: system-metrics 8b5d40a4eb98 -->
### Systemmetriken

Metriken zum Prozess, zur Go-Runtime und zu Verbindungen zu Ressourcen werden über denselben `metrics_exporter` exportiert.
CPU, Speicher, Zahl der Instanzen und Ingress-Anfragen des Containers bleiben den Metriken der Plattform überlassen; diese hier decken die Sättigung innerhalb des Prozesses ab, die die Plattform nicht sehen kann.
Nutze sie nicht, um Skalierung auszulösen; nutze sie für Alarme und zur Abschätzung der Server-Obergrenze.

| Metrik | Art | Labels |
| --- | --- | --- |
| `go.memory.used`, `go.memory.limit`, `go.memory.gc.goal` | Gauge, Bytes | Nur `go.memory.used` hat `go.memory.type` (`stack` / `other`). `go.memory.limit` erscheint nur, wenn `GOMEMLIMIT` gesetzt ist |
| `go.memory.allocated`, `go.memory.allocations` | Zähler | Keine |
| `go.goroutine.count`, `go.processor.limit`, `go.config.gogc` | Gauge | Keine |
| `go.schedule.duration`, `go.memory.gc.pause.duration` | Histogramm, Sekunden | Keine (das Warten von „lauffähig“ bis „läuft“ und GC-Pausen) |
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
| `http.client.request.duration` | Histogramm, Sekunden | `my.http.purpose` (`json` / `html` / `image` / `video` / `human_check` / `web_push` / `webhook` / `client_metadata` / `login_provider` / `robots`), `http.request.method`, `my.http.status_class` (`2xx` und so weiter), `error.type` (`forbidden_destination` ist eine Ablehnung durch den SSRF-Schutz) |
| `http.client.open_connections` | UpDownCounter | Keine |
| `my.readiness.failures` | Zähler | Keine |
| `my.http.origin_refused` | Zähler | Keine. Einer pro Anfrage, die abgelehnt wurde, weil sie nicht über das CDN kam ([`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn)). Probes der Adresse der Origin erhöhen ihn langsam; ein Sprung, während der Verkehr ausbleibt, heißt, das CDN sendet den Header nicht mehr. Der Wartungsmodus misst nichts, nur sein Access-Log zeigt sie |

- Go-Runtime-Metriken (`go.*`) werden nur von `prometheus` und `otlp` exportiert. Mit `emf` schreibt ein periodischer Snapshot-Job die Gauges und Observable Counters von Prozess und Verbindungspool.
- Pool-Wartezeiten und CPU-Zeit sind Observable Counters: Der Pool und der Kernel führen Summen seit dem Start, und sie werden zum Zeitpunkt der Erfassung gelesen. In Prometheus sind sie `_total`-Zähler (etwa `process_cpu_time_seconds_total`), in OTLP monotone Summen, und ihr Wachstum siehst du mit PromQL `rate()`.
  `emf` hat keine Summenzähler, es schreibt daher den Zuwachs seit dem vorherigen Schreiben (addiere sie mit der CloudWatch-Statistik Sum).
- Das `claim` der Warteschlange wird nur gemessen, wenn es einen Job nimmt oder fehlschlägt. Das Polling eines wartenden Workers verzeichnet nichts.
- `db.client.connection.pending_requests` wird nicht exportiert: Die Statistiken von pgx und `database/sql` melden nicht, wie viele warten, daher stehen Anzahl und Dauer der Wartezeiten stellvertretend dafür.
- Alarmiere auf das Verhältnis der Summe von `go.memory.used` zu `go.memory.limit` und auf das Wachstum von `db.client.connection.wait_time` (das Pool-Limit wird erreicht).

<!-- source: memory-limit-gomemlimit 8ea44722eefd -->
### Speicherlimit (`GOMEMLIMIT`)

`GOMEMLIMIT` ist eine Umgebungsvariable der Go-Runtime, die der Betreiber setzt; die Anwendung wählt sie nicht.
Ohne sie läuft Go mit seinem Standard (kein Limit), und der Heap sammelt nicht eifrig, wenn er sich dem Speicherlimit des Containers nähert.
Setze sie bei Setups mit einem Container-Speicherlimit (Cloud Run, ECS, Kubernetes) auf etwa 90 % dieses Limits (zum Beispiel `GOMEMLIMIT=460MiB` für einen Container mit 512 MiB).
Setze `GOMAXPROCS` nicht: Der Standard von Go folgt dem CPU-Limit des Containers.

<!-- source: traces e274793f5885 -->
## Traces

Die Einstellung `traces_exporter` (Standard `none`) exportiert verteilte Traces. Die zugehörigen Einstellungen stehen in der [Konfigurationsreferenz](../reference/configuration.md).

| Span | Name | Attribute |
| --- | --- | --- |
| HTTP-Anfrage (Server) | `<method> <http.route>` (`<method>` bei einer Anfrage, die keine Route erreicht hat) | `http.request.method`, `http.route`, `http.response.status_code`, `my.surface`. Ein 5xx ist ein Fehlschlag |
| Job-Ausführung (Consumer) | Der Job-Typ (`my.job.type`) | `my.job.type`, `my.job.attempt`, `my.outcome`. Jede Wiederholung ist ein eigener Span |
| Transaktion | `my.db.transaction` | `db.system.name`. Jede Wiederholung ist ein eigener Span |
| Query (Client) | Der sqlc-Query-Name (etwa `InsertJob`; sonst das erste Wort des SQL, etwa `BEGIN`) | `db.system.name`, `db.operation.name` |
| Zustellung (`my.delivery`) | `my.delivery` | `activity.type`, `my.outcome` |
| Ausgehendes HTTP (Client) | `<method>` | `http.request.method`, `server.address` (nur der Ziel-Hostname), `http.response.status_code`. Ein 4xx oder 5xx ist ein Fehlschlag |

Ein fehlgeschlagener Span hat `error.type` (ein endlicher Name wie `timeout` oder ein Statuscode). Attribute enthalten nie Konto-, Beitrags- oder Job-IDs, URLs, Pfade oder Queries, Client-IPs, User-Agent-Werte, SQL-Text oder Fehlerstrings.
`server.address` überlässt die Hostnamen entfernter Server dem Empfänger, du entscheidest also, wie lange der Empfänger sie aufbewahrt und wer sie einsehen kann. Health-Checks und die durch `origin_secret` abgelehnten Anfragen erzeugen keinen Span.

- Eine Job-Ausführung ist kein Kind des Spans, der sie in die Warteschlange gestellt hat (meist eine HTTP-Anfrage). Sie ist die **Wurzel eines neuen Traces** mit einem Link auf den einreihenden Span. Folge in einem Viewer dem Link aus dem Trace der Anfrage.
- Ein eingehendes `traceparent` wird nur dann zum Parent, wenn die verbindende Gegenstelle in `trusted_proxies` steht und, mit [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn), die Anfrage das Geheimnis trägt. Andernfalls wird es verworfen, und ein neuer Trace beginnt. `tracestate` wird immer verworfen. Ausgehendes HTTP (Föderation, WebFinger, Medien, Web Push und so weiter) trägt kein `traceparent`.
- Sampling: Eine Anfrage wird aufgezeichnet, wenn ihr Trusted-Proxy-Parent aufgezeichnet wurde oder wenn das Link-Ziel eines Jobs aufgezeichnet wurde; sonst wird sie im Verhältnis `traces_sample_ratio` aufgezeichnet (Standard 0,01). Ein Senken des Verhältnisses lässt die Jobs, die eine aufgezeichnete Anfrage fortsetzen, nie ohne ihren Trace.

| Ziel | Einstellungen | Pfad |
| --- | --- | --- |
| Standard, kleines Self-Hosting | `traces_exporter = none` | Es werden keine Spans exportiert. Folge `request_id` und `job_id` in den Logs. Liefert ein Trusted Proxy ein `traceparent`, werden dessen `trace_id` und `span_id` zu den Logs hinzugefügt |
| Selbst gehostet (Jaeger, Tempo, ein OpenTelemetry Collector und so weiter) | `traces_exporter = otlp`, `traces_flush = batch` | Per OTLP/HTTP an den Empfänger unter `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` gesendet (oder `OTEL_EXPORTER_OTLP_ENDPOINT`, Standard `http://localhost:4318`). Authentifizierungs-Header gehören in `OTEL_EXPORTER_OTLP_HEADERS`. Um Fehler und langsame Traces zuverlässig zu behalten, nutze das Tail Sampling des Collectors |
| Google Cloud (Cloud Run) | `none` oder `otlp` mit `unit`, um Traces zu aktivieren | Siehe unten |
| AWS (ECS) | `none` oder `otlp` mit `batch`, um Traces zu aktivieren | Siehe unten |

<!-- source: cloud-run 7f1a21bbfbef -->
### Cloud Run

Auch mit `none` teilen sich der Anfrage-Trace, den Cloud Run automatisch erstellt, und die `trace_id` in den Logs dieselbe ID (wenn du der Verbindung vor Cloud Run mit `trusted_proxies` vertraust).
Ob Cloud Run das `traceparent` öffentlicher Anfragen immer überschreibt, ist noch nicht verifiziert.
Die Logs haben nicht die Form des Trace-Felds, das Cloud Logging erkennt (`logging.googleapis.com/trace`).

Um Anwendungs-Spans an Cloud Trace zu senden, betreibe einen **OpenTelemetry Collector als Sidecar** im selben Dienst. Der Server hält keine Google-Zugangsdaten; der Sidecar sendet mit Application Default Credentials an den OTLP-Endpunkt von Cloud Trace.

- Server (der Ingress-Container): `MY_TRACES_EXPORTER=otlp`, `MY_TRACES_FLUSH=unit`, `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`
- Sidecar: ein OpenTelemetry Collector, der mit einem `otlp`-Receiver über HTTP auf `localhost:4318` empfängt und mit dem `googlecloud`-Exporter oder dem `otlphttp`-Exporter sendet (`https://telemetry.googleapis.com`, mit der Erweiterung `googleclientauth`). Gib dem Service-Konto die Berechtigung, in Cloud Trace zu schreiben.
- Am Ende jeder aufgezeichneten Anfrage oder jedes Jobs sendet `unit` die bisherigen Spans an den Sidecar, bevor es zurückkehrt. Damit der Sidecar sie senden kann, während keine CPU zugeteilt ist, nutze für ein Setup mit aktivierten Traces serverbasierte Abrechnung.
- Die Kosten sind die Ressourcen des Sidecars und das Span-Volumen (Preise von Cloud Trace). Wähle das Verhältnis mit `traces_sample_ratio`.

<!-- source: aws-ecs 52fa09412f20 -->
### AWS ECS

Um an X-Ray zu senden, betreibe den **AWS Distro for OpenTelemetry (ADOT) Collector** als Sidecar-Container im Task. Der Server hat keine SigV4-Signierung; der Collector signiert und sendet an den OTLP-Endpunkt von X-Ray (`https://xray.<region>.amazonaws.com/v1/traces`).

- Server: `MY_TRACES_EXPORTER=otlp`, `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`. Empfange in der Konfiguration des Collectors mit einem `otlp`-Receiver auf `localhost:4318` und sende mit einem `otlphttp`-Exporter, der die Erweiterung `sigv4auth` nutzt, an X-Ray.
- Gib der Task-Rolle die Berechtigung, in X-Ray zu schreiben.
- Die Kosten sind der Anteil des Sidecars an den Ressourcen des Tasks und die Zahl der Traces, die X-Ray aufzeichnet (die ersten 100.000 pro Monat sind kostenlos).

<!-- source: stop-the-server 37d4c0cc3d97 -->
## Den Server stoppen

Bei SIGTERM oder SIGINT fährt der Server geordnet herunter.

1. Er nimmt keine neuen Verbindungen und keine Jobs mehr an.
2. Er wartet bis zu 25 Sekunden, bis laufende Anfragen und laufende Jobs fertig sind.
3. Nach 25 Sekunden bricht er den Kontext der übrigen Anfragen und Jobs ab und schließt Verbindungen.
   Danach gibt er einem abgebrochenen Job bis zu 3 weitere Sekunden, sein Ergebnis zu verzeichnen, und endet mit Exit-Code 1 (insgesamt höchstens 28 Sekunden, innerhalb der standardmäßigen Kulanzfrist von 30 Sekunden bei Kubernetes und ECS).
   Ein abgebrochener Job wird wiederholt. Ein Job, der innerhalb dieser 3 Sekunden nicht stoppt oder mittendrin beendet wird, wird aufgegeben und von einem Worker wieder aufgenommen, wenn sein Lease abläuft, er kann also zweimal laufen.

Ein Panic in einer der Hintergrundaufgaben des Servers löst dasselbe Herunterfahren aus, damit der Server nie in einem kaputten Zustand weiterläuft.
Die Logzeile `serve failed` nennt die Aufgabe, und der Prozess endet mit Exit-Code 1, damit die Plattform oder der Prozessmanager ihn neu startet.

Starte für Arbeiten an der Datenbank, dem Key-Value-Speicher oder der Warteschlange statt eines Stopps im Wartungsmodus neu (`MY_MAINTENANCE=true`). Siehe [Wartung](maintenance.md).

<!-- source: the-job-queue-and-failed-dead-jobs 55023b12a1aa -->
## Die Job-Warteschlange und fehlgeschlagene (tote) Jobs

[Jobs](../reference/jobs.md) listet jeden Job-Typ auf: was er tut, was ihn startet, wie er wiederholt und was ein erneutes Ausführen bewirkt.

Die Admin-Seite für Jobs (`/admin/jobs`, Berechtigung `manage_jobs`) hat drei Seiten:

- **Zustand der Warteschlange** (`/admin/jobs`): die Zahl pro Zustand und pro Typ und Zustand, wie lange jeder Typ einen Job warten lässt und wie lange der älteste wartende Job gewartet hat.
- **Jobs** (`/admin/jobs/list`): eine Tabelle der Jobs in einem Zustand (ausstehend, wartend, laufend, wartet auf Wiederholung, fehlgeschlagen), eine Seite nach der anderen, mit Typ, Zustand, dem Zeitpunkt, zu dem er geplant ist oder fehlschlug, und der Zahl der Versuche. Fehlgeschlagene Jobs haben **Erneut ausführen**.
- **Ein Job** (`/admin/jobs/<id>`, vom Typ in der Tabelle aus): jede Tatsache des Jobs, sein letzter Fehler, Schlüssel und Payload, der Verlauf erfolgloser Versuche (Ergebnis, Fehler, Worker sowie Start- und Endzeit jedes Versuchs) und **Erneut ausführen** für einen fehlgeschlagenen Job.

Die Admin-API besteht aus `GET /api/v1/admin/queue`, `GET /api/v1/admin/jobs?state=&type=`, `GET /api/v1/admin/jobs/{id}`, `GET /api/v1/admin/jobs/{id}/attempts` und `POST /api/v1/admin/jobs/{id}/retry`.
Ein abgeschlossener Job ist nicht mehr in der Warteschlange, seine Seite wird also nicht gefunden.

- Wartende, laufende und auf Wiederholung wartende Jobs lassen sich nur für die Warteschlangen auflisten, deren Jobs der Server hält: `queue = db`, `queue = cloudtasks` (die Jobs sind auch in der Datenbank) und `queue = inline`. Fehlgeschlagene Jobs und der Versuchsverlauf liegen für jede Warteschlange in der Anwendungsdatenbank und lassen sich daher immer auflisten.
- Der Versuchsverlauf behält nur erfolglose Versuche (Wiederholungen und den Versuch, der schließlich fehlschlug). Verfolge einen erfolgreichen Lauf über die strukturierte Logzeile `job` (die `job_id` trägt).
  Der Verlauf wird von der stündlichen Bereinigung entfernt, sobald seit dem Ende des Versuchs `job_attempt_retention` (Standard 30 Tage; siehe die [Einstellung](../reference/configuration.md#job_attempt_retention)) vergangen ist.
  Der Verlauf enthält keine Payload und kein Handler-Log, und eine Fehlermeldung wird auf ihre ersten 2.000 Bytes gekürzt. Job-Handler halten E-Mail-Adressen, IP-Adressen, Tokens und Beitragstexte aus Fehlermeldungen heraus.
  Für eine auf E-Mail-Job-Typen beschränkte Ansicht siehe [E-Mails senden](email.md).

Ein Job, bei dem der Handler entschied, dass er endgültig fehlschlug, und ein Job, dessen Wiederholungen aufgebraucht sind, wird zu einem toten Job in der Datenbank-Warteschlange (`queue = db` und `queue = cloudtasks`).
`my job` listet, wiederholt und löscht sie. Der Zustand „Fehlgeschlagen“ der Jobs-Seite und die Admin-API können sie ebenfalls auflisten und wiederholen.

```sh
my job list                  # newest failures first: ID, type, run count, time of failure, last error (-limit sets the count, default 50)
my job retry <id> [<id>...]  # reset the attempt count and put the job back in the queue to run immediately
my job delete <id> [<id>...] # remove without running
my job types                 # print every job type of this release
```

- Wartet bereits ein Job mit demselben Idempotenzschlüssel, tritt dieser Job an die Stelle des wiederholten. Job-Handler sind idempotent, es ist also sicher, denselben Job beliebig oft zurückzustellen.
- Eine unbekannte ID wird ausgegeben und der Befehl endet mit Exit-Code 1; die anderen IDs werden trotzdem verarbeitet. Exit-Code 2 bedeutet einen Aufruffehler.
- Ein toter Job, den niemand wiederholt, wird von der stündlichen Bereinigung entfernt, sobald seit dem Fehlschlag `dead_job_retention` (Standard 30 Tage; siehe die [Einstellung](../reference/configuration.md#dead_job_retention)) vergangen ist.
- Tote Jobs von `queue = inline` liegen im Speicher des Serverprozesses, `my job` kann sie also nicht bearbeiten (Exit-Code 1).

<!-- source: splitting-workers-by-job-type 89ced36a3c80 -->
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
| `voice.apply_media_event`, `voice.apply` | Die Ereignisse eines Voice-Raums |
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

<!-- source: running-several-jobs-at-once 91e5181261b2 -->
### Mehrere Jobs gleichzeitig ausführen

[`worker_job_concurrency`](../reference/configuration.md#worker_job_concurrency) legt fest, wie viele Jobs ein Prozess gleichzeitig ausführt, von 1 bis 128. Es funktioniert mit `queue = db`; bei Cloud Tasks entscheidet die Plattform. Der Standard ist ein Job gleichzeitig, und eine Einstellung von 1 verhält sich genauso.

Erhöhe es für Jobs, die ihre Zeit mit Warten auf andere Server verbringen, etwa `federation.deliver`. Diese brauchen pro Job wenig Speicher und wenige Datenbankverbindungen, ein Prozess leistet also die Arbeit vieler: In einem lokalen Test, in dem ein Job nur 200 ms auf eine Antwort wartete, entsprach ein Prozess mit 32 Slots 32 Prozessen mit je einem Slot und brauchte nur einen kleinen Teil des Speichers. Diese Jobs taten nichts anderes, miss also deine eigenen Server. Kombiniere es mit `worker_job_types` und gib jeder Art von Job einen Prozess mit eigener Zahl.

Beachte diese Grenzen:

- **Datenbankverbindungen**: Ein Job braucht eine Verbindung, um übernommen zu werden, für seine eigene Arbeit und um sein Ergebnis zu verzeichnen. Mit `database = postgres` muss die Einstellung unter dem Pool-Maximum liegen, sonst startet der Server nicht. Das Maximum ist `pool_max_conns` in `database_url`, standardmäßig das Größere aus 4 und der CPU-Zahl, eine vCPU erlaubt also 3, bis du es erhöhst. Prüfe, dass deine Datenbank oder dein Connection-Pooler die Verbindungen all deiner Prozesse akzeptiert.
- **SQLite**: Es schreibt eine Transaktion nach der anderen. Über 32 protokolliert der Server eine Warnung, und in einem lokalen Test lief SQLite bei 128 langsamer als bei 32.
- **CPU-intensive Jobs**: Slots teilen sich die CPU und den Speicher eines Prozesses. `media.process` wartet auf seinen Platz bei einem Limit von standardmäßig der Hälfte der CPUs, und das Warten zählt gegen das 5-Minuten-Lease des Jobs, sodass ein Prozess voller `media.process` über das Lease hinauslaufen und von vorn beginnen kann. Gib solchen Jobs einen Prozess mit wenigen Slots.
- **Ein Server nach dem anderen**: Ein Prozess macht höchstens 8 Verbindungen zu einem Host, daher stellt ein Prozess an einen einzelnen Server nicht schneller zu, als 8 Verbindungen erlauben, wie viele Slots er auch hat. Wo die meisten Zustellungen an wenige große Server gehen, helfen mehr Prozesse, wo mehr Slots es nicht tun.
- **Ein festhängender Server hält Slots**: Slots, die auf einen langsamen Server warten, sind für andere Jobs nicht frei, halte Zustellungen also getrennt von E-Mail und Benachrichtigungen.
- **Deployments verbrauchen Versuche**: Wenn der Server stoppt, werden Jobs, die nach `ShutdownTimeout` (standardmäßig 25 Sekunden) noch laufen, abgebrochen, und jeder abgebrochene Job verbraucht einen Versuch. Je mehr Slots langsame Jobs ausführen, desto mehr Versuche verbraucht jedes Deployment. Ein abgebrochener Job, dessen Ergebnis nicht rechtzeitig verzeichnet wird, wartet, bis sein Lease abläuft, bevor er erneut läuft.
- **Ein Absturz nimmt jeden Slot mit**: Geht dem Prozess der Speicher aus, warten alle seine Jobs, bis ihre Leases ablaufen.

Um zu entscheiden, ob du es erhöhst, lies `my.job.wait` pro `my.job.type`: Wenn Jobs eines Typs lange warten, während `my.job.duration` meist aus Warten auf andere Server besteht und CPU und Speicher niedrig bleiben, sollten mehr Slots helfen. Bleiben die Wartezeiten nach dem Erhöhen hoch, sieh dir die Datenbankverbindungen und das Host-Limit oben an, bevor du es weiter erhöhst.

Solange kein Job fällig ist, fragt ein Slot ab, und die anderen warten, ein Prozess im Leerlauf kostet die Datenbank also so viel wie ein Prozess mit einem Job.

<!-- source: waiting-for-a-database-connection f481d577ea58 -->
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
  - Auf Cloud Run nimmt ein Dienst standardmäßig 80 gleichzeitige Anfragen pro Server-Instanz an. Bei einem Pool von 4 staut sich hier ein Schwall langsamer Anfragen, erhöhe also `pool_max_conns`, senke die Nebenläufigkeit des Dienstes oder beides.
- **Häufige `503` oder `db.client.connection.timeouts`** bedeuten, dass der Pool zu klein ist oder Queries langsamer wurden; ein längeres Timeout verbirgt das nur. Folge [`MyDBPoolSaturated`](alerts.md#mydbpoolsaturated).

<!-- source: cloud-tasks ef3f7d7cc896 -->
### Cloud Tasks

[Auf Cloud Run bereitstellen](deploy-cloud-run.md) richtet alles Folgende mit OpenTofu-Modulen ein.

Mit `queue = cloudtasks` bleiben die Jobs in der Datenbank (`database = postgres`), und Google Cloud Tasks weckt den Server nur für jeden von ihnen auf:

- Für jeden Job, den er speichert, wiederholt oder zurückstellt, erstellt der Server nach dem Commit der Transaktion einen Task in `queue_cloudtasks_queue`. Der Task ruft
  `POST {queue_cloudtasks_url}/_internal/jobs` zu dem Zeitpunkt auf, zu dem der Job fällig ist, mit der ID des Jobs als Body und einem OIDC-ID-Token von
  `queue_cloudtasks_service_account`. Ein Job, der mehr als 30 Tage im Voraus fällig ist, wird bei 30 Tagen signalisiert und erneut, wenn dieser Task eintrifft.
- Der Prozess, der den Task erhält, führt den Job aus und antwortet mit `204`, sobald sein Ergebnis verzeichnet ist, auch wenn der Job schon lief oder nicht fällig ist.
  Ein Job, der fehlschlägt, wird vom Server zu seinem nächsten Versuchszeitpunkt wiederholt, nicht von Cloud Tasks. Cloud Tasks wiederholt einen Task nur, wenn die
  Antwort `5xx` ist (die Datenbank war nicht erreichbar) oder `401` (ein Token, das der Server abgelehnt hat), sein `retryConfig` braucht also keine Feinabstimmung.
- Kein Prozess fragt die Tabelle ab, und kein Prozess führt die periodischen Jobs von selbst aus. Erstelle einen Cloud-Scheduler-Job, der
  `POST {queue_cloudtasks_url}/_internal/tick` jede Minute aufruft (`* * * * *`), mit einem OIDC-Token desselben Service-Kontos (und der Audience von
  `queue_cloudtasks_audience`, wenn gesetzt). Jeder Tick stellt die periodischen Jobs, deren Periode gekommen ist, einmal für die ganzen Server in die Warteschlange (die Perioden werden im KV beansprucht,
  daher muss `kv` `db` oder `valkey` sein) und signalisiert die Jobs erneut, die seit über 2 Minuten fällig sind, ohne zu laufen, was ein verlorenes Signal behebt.
- `/_internal/jobs` und `/_internal/tick` werden von den Prozessen mit `role = all` oder `worker` bedient und akzeptieren nur ein von Google für
  `queue_cloudtasks_service_account` signiertes Token mit verifizierter E-Mail, dessen Audience `queue_cloudtasks_audience` ist oder, standardmäßig, die aufgerufene URL. Jede andere Anfrage ist ein `401`.
- Eine Zustellung führt einen Job aus, höchstens für das 5-Minuten-Lease. Halte `queue_cloudtasks_dispatch_deadline` (Standard 10 Minuten) und das Request-Timeout
  des Cloud-Run-Dienstes länger als das.
- Das eigene Service-Konto des Servers braucht `roles/cloudtasks.enqueuer` auf der Queue und `roles/iam.serviceAccountUser` auf `queue_cloudtasks_service_account`.
  Es authentifiziert sich mit Application Default Credentials.

<!-- source: routing-job-types-to-their-own-queues 292b3f6071a8 -->
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
Wo jede Rolle eigene Einstellungen hat, wie bei Cloud-Run-Diensten, gib allen denselben Wert:

```sh
routes='federation.*=projects/p/locations/l/queues/my-federation@https://federation-worker.example.com'
gcloud run services update my-web --update-env-vars "^|^MY_QUEUE_CLOUDTASKS_ROUTES=$routes"
gcloud run services update my-worker --update-env-vars "^|^MY_QUEUE_CLOUDTASKS_ROUTES=$routes"
gcloud run services update my-federation-worker --update-env-vars "^|^MY_QUEUE_CLOUDTASKS_ROUTES=$routes"
```

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

<!-- source: health-checks 06a8834ade0d -->
## Health-Checks

| Pfad | Art | Wann er 200 liefert |
| --- | --- | --- |
| `GET /healthz` | Liveness | Der Prozess kann HTTP beantworten. Er schaut nicht auf Abhängigkeiten wie die Datenbank (damit eine langsame Datenbank den Prozess nicht neu startet) |
| `GET /readyz` | Readiness | Die Datenbank ist erreichbar und das Schema ist angewendet |
| `GET /api/v1/instance/health` | Uptime-Monitoring, Statusseiten und Clients (bedient von der Rolle `web`) | Wie die Readiness. Das Ergebnis wird 60 Sekunden im Prozess zwischengespeichert |

- Ein 200 von `/healthz` oder `/readyz` hat den Body `{"status":"ok"}`.
- `/readyz` prüft die Datenbank bei jeder Anfrage, und gleichzeitige Anfragen teilen sich eine Prüfung.
- `/api/v1/instance/health` liefert bei Gesundheit 200 und `{"status":"ok","checked_at":"<RFC 3339>"}` und bei einem Fehlschlag 503 und `{"status":"unavailable","checked_at":...}` (`Cache-Control: no-store`).
  Jeder Prozess berührt die Datenbank höchstens einmal pro Minute, egal wie oft er überwacht oder angefragt wird, und er schaut weder auf das Access-Token noch auf das Rate Limit. Richte Uptime-Monitore auf diesen Endpunkt, nicht auf die Probe (`/readyz`).
  Zur Einrichtung pro Dienst, zum Überwachungsintervall und zu den Kosten und dazu, wie Wartung behandelt wird, siehe die [Anleitung zum Uptime-Monitoring](monitoring.md).
- Alle drei loggen im Access-Log auf Debug-Stufe und erzeugen keinen Trace-Span.
- Ist die Readiness nicht erfüllt, ist die Antwort 503 mit einem RFC-9457-Body `application/problem+json`.
  Sein `title` wird gemäß `Accept-Language` übersetzt und enthält kein internes Detail. Die Ursache (ein Verbindungsfehler oder die Namen fehlender Tabellen) geht ins Log.
- Eine Readiness-Prüfung wird nach 3 Sekunden abgebrochen.
- Die Readiness verlangt außerdem, dass die Datenbank den konfigurierten `host` und `account_domain` enthält oder dass der Prozess sie verzeichnen konnte.
- **Schema angewendet** heißt, jede Tabelle, die die Schema-Dateien der Datenbank (`schema/<NN>-<module>/<dialect>.sql`) mit `CREATE TABLE` deklarieren, existiert
  (SQLite wird über `sqlite_schema` geprüft, PostgreSQL über das `current_schema()` der Verbindung). `my schema apply` wendet das ganze Schema an, eine fehlende Tabelle bedeutet also, dass das Schema nicht angewendet oder älter als die Binärdatei ist.
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

<!-- source: web-client f35b79dd2cc3 -->
## Webclient

Ein Prozess, dessen `role` `web` oder `all` ist, liefert den in die Binärdatei eingebetteten Webclient aus.

| Pfad | Inhalt | `Cache-Control` |
| --- | --- | --- |
| Jedes `GET`, das keine andere Route trifft (`/`, `/@alice`, `/@alice/status/{id}`, `/settings` und so weiter) | Die HTML-Hülle der SPA. Die Servereinstellungen werden in `<script type="application/json" id="config">` übergeben | `public, max-age=60, stale-while-revalidate=300`, ETag |
| `GET /assets/*` | Build-Dateien mit Hash-Namen | `public, max-age=31536000, immutable`, ETag |
| `GET /manifest.webmanifest` | Das aus den Servereinstellungen gebaute Web-App-Manifest | `public, max-age=60, stale-while-revalidate=300`, ETag |
| `GET /sw.js`, `/icon.svg` und andere Dateien direkt unter dem Build | Unverändert ausgeliefert | `no-cache`, ETag |
| `GET /licenses/*` | Die Lizenzen dessen, was der Build bündelt (npm-Paket-Hinweise sowie `LICENSE-Unicode.txt` und `OFL.txt` für gebündelte Nicht-npm-Daten). Ausgeliefert als `text/plain; charset=utf-8`, was auch immer die Endung ist | `no-cache`, ETag |
| `GET /robots.txt`, `/.well-known/tdmrep.json`, `/.well-known/security.txt` | Aus den Servereinstellungen gebaut (siehe [Suchmaschinen und KI-Crawler](crawlers.md)) | `public, max-age=3600`, ETag |

- Pfade unter `/api`, `/oauth`, `/auth`, `/.well-known`, `/nodeinfo`, `/ap`, `/media`, `/uploads`, `/theme` (das Stylesheet des Server-Themes) und `/licenses` sowie `/robots.txt` liefern nie die Hülle; trifft keine andere Route, liefern sie ein 404-Problem.
- Die Hülle wird nach der ActivityPub-Content-Negotiation ausgeliefert. Wer einen Hüllenpfad mit einem ActivityPub-`Accept` anfragt, erhält 404 (Beitragsseiten leiten weiter, wie unten beschrieben).
  Die Hülle trägt immer `Vary: Accept`. Wenn du ein CDN davorsetzt, nimm die Art des `Accept` (ActivityPub oder HTML) in den Cache-Schlüssel auf oder nimm die Hülle, `/ap/*` und `/@*` vom Caching aus.
  Liefere nur `/assets/*` vom CDN aus.
- Eine Binärdatei, die ohne den Webclient gebaut wurde (ein einfaches `go build`, ohne zuvor den Client zu bauen), protokolliert beim Start eine Warnung und liefert eine Hülle ohne Skripte aus.

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
