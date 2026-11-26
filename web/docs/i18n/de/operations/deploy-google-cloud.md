<!-- source: deploying-on-google-cloud 03b87618467f -->
# Auf Google Cloud bereitstellen

So betreibst du `my` auf Google Cloud mit dem OpenTofu-Modul `mycroblog-gcp`: ein Cloud-Run-Dienst und ein Backend für jede Rolle,
das du über die `kind` ihres Abschnitts wählst. Jeder Abschnitt hat standardmäßig die Empfehlung des Moduls, die keine Instanz laufen lässt,
solange niemand den Server nutzt, abgesehen von einem Tick, der sie jede Minute weckt. Ein Server kostet Geld: Die empfohlene Einrichtung mit einer
Datenbank bei Neon kommt vor Speicher und Traffic auf etwa 37 $ im Monat, und [Kosten](#costs) nennt die Zahl jeder Wahl.

Was jede OpenTofu-Bereitstellung gemeinsam hat, darunter der Block und seine Abschnitte, die Secrets, die Konsole und Upgrades, steht in
[Mit OpenTofu bereitstellen](deploy-opentofu.md). Diese Seite ergänzt, was Google Cloud braucht.

<!-- source: what-it-creates 7b13c8bbc968 -->
## Was es anlegt

Ein Block von `mycroblog-gcp` legt Folgendes an. Die erste Art jeder Zeile ist die empfohlene und der Standard;
[die Referenz der Module](../../deploy/opentofu/README.md#sections) listet jedes Feld auf.

| Rolle | Abschnitt | Empfohlen | Andere Arten |
| --- | --- | --- | --- |
| Server | `service`, `jobs` | Ein Cloud-Run-Dienst mit `role = all`, ein Migrationsjob, solange du ihn brauchst ein Konsolen-Job, ein Dienstkonto und öffentlicher Zugriff | |
| Traffic | `ingress` | `load-balancer`: ein globaler externer Application Load Balancer mit einem von Google verwalteten Zertifikat | `domain-mapping`, `cloudflare` und ein gecachter Medien-Host auf Cloudflare neben jeder |
| Datenbank | `database` | `external`: ein Secret für die URL eines PostgreSQL, das du betreibst, etwa Neon | `cloud-sql`: eine Instanz, ihre Datenbank, ein Benutzer und ein Passwort |
| Jobs | `queue` | `cloud-tasks`: eine Cloud-Tasks-Queue, der Cloud-Scheduler-Job des Ticks und ein Dienstkonto, das den Server aufruft | `db`: die Jobs in der Datenbank |
| Medien | `media` | `gcs`: ein privater Cloud-Storage-Bucket | `r2`, `s3` |
| Caches und Rate Limits | `kv` | `db`: in der Datenbank, ohne dass etwas anzulegen ist | `memorystore`, `external` (Valkey) |
| Echtzeit-Aktualisierungen | `stream` | `pubsub`: ein Topic und eine benutzerdefinierte Rolle, mit der der Server seine eigenen Subscriptions verwaltet | `postgres`, `valkey` |
| E-Mail | `mail` | Jeder SMTP-Dienst, mit einem Secret für sein Passwort | |
| KEK | `kek` | `secret-manager`: ein erzeugtes Secret, als Datei eingehängt | `kms`: ein Schlüssel in Cloud KMS |
| Netzwerk | `network` | `none`: nichts anzuhängen | `vpc`, `existing` |
| Metriken, Traces, Fehler | `observability` | Cloud Monitoring, Cloud Trace und Error Reporting, die lesen, was Cloud Run ihnen sendet. Das Modul aktiviert ihre APIs | `settings`, etwa Sentry |
| Alarme | `monitoring` | Eine Uptime-Prüfung, logbasierte Metriken und Alarmrichtlinien: [Monitoring und Alarme](#monitoring-and-alerts) | `null`, keine |

Das Modul erzeugt außerdem den VAPID-Schlüssel und legt die Secrets, die nur du füllen kannst, leer an.

`examples/gcp-cloud-run-neon` ist ein Weg zum Start: der Provider und ein Block mit jeder empfohlenen Art, was mit
jeder externen Datenbank funktioniert, Neon oder nicht. Füge seinem Block die Abschnitte hinzu, die du änderst.

<!-- source: before-you-start 9869afda9a07 -->
## Bevor du anfängst

- Was [die erste Bereitstellung](deploy-opentofu.md#first-deployment) braucht, mit `gcloud` als CLI der Plattform und
  `gcloud auth application-default login` für den Provider.
- Ein Google-Cloud-Projekt mit Abrechnung, nur für diese Instanz verwendet, und ein Konto darauf mit der Rolle Owner. Die benutzerdefinierte
  Rolle von `stream = pubsub` liegt auf dem Projekt und würde daher die Subscriptions von allem anderen dort erreichen.

<!-- source: first-deployment 894b6af2cee9 -->
## Erste Bereitstellung

Mache die ersten beiden Schritte hier und folge dann [der ersten Bereitstellung](deploy-opentofu.md#first-deployment) mit den Hinweisen des
dritten.

<!-- source: 1-enable-the-apis fdd2dc400c20 -->
### 1. Die APIs aktivieren

Jede Bereitstellung braucht diese:

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com
```

Füge die API jeder Wahl hinzu, die eine braucht:

| Wahl | API |
| --- | --- |
| `ingress = load-balancer` | `compute.googleapis.com` |
| `database = cloud-sql` | `sqladmin.googleapis.com` |
| `kek = kms` | `cloudkms.googleapis.com` |
| `network = vpc` oder `existing` | `compute.googleapis.com`, `servicenetworking.googleapis.com`, `networkconnectivity.googleapis.com` |
| `kv = memorystore` | `memorystore.googleapis.com`, `serviceconsumermanagement.googleapis.com` und die des Netzwerks |

`translation` aktiviert seine eigene API, ebenso `observability`. [Cloud-Nutzungskosten importieren](billing-import.md) listet auf, was
`billing` braucht.

<!-- source: 2-prepare-the-database b54e3b01002b -->
### 2. Die Datenbank vorbereiten

- **`external`**: Lege die Datenbank an und halte ihre direkte Verbindungs-URL und, wenn sie eine hat, ihre gepoolte URL für
  `secrets_to_fill` bereit: [Die Datenbank](#the-database) sagt, welche wohin gehört. Das Modul legt sie nie an, sodass ihr Passwort aus
  dem OpenTofu-State herausbleibt.
- **`cloud-sql`**: Nichts zu tun. Das Modul legt die Instanz an und schreibt ihre URL mit einem erzeugten Passwort in das Secret.

<!-- source: 3-deploy 4470affd40e4 -->
### 3. Bereitstellen

Folge [der ersten Bereitstellung](deploy-opentofu.md#first-deployment). Auf Google Cloud:

- **Variablen**: `mail_from`, `mail_smtp_host` und `mail_smtp_username` des Beispiels konfigurieren [E-Mail](email.md). Der Port
  ist 587, da Google Cloud den ausgehenden Port 25 sperrt.
- **Secrets**: Der KEK ist `my-kek` und der VAPID-Schlüssel `my-vapid-key`, jeweils beginnend mit `name`, und
  `gcloud secrets versions access latest --secret=<id> --project="$PROJECT"` kopiert sie. Der KEK erreicht den Server als
  eingehängte Datei, benannt durch `MY_KEK_FILE`, und die anderen Secrets als Umgebungsvariablen.
- **Apply**: Zuerst kommt der Migrationsjob mit allem, wovon er abhängt, wozu der Ingress gehört, da die Einstellungen des Servers
  ihn nennen. Der Startup-Probe des Dienstes wartet auf `/health/ready`, was mit `503` antwortet, bis das Schema existiert.
- **DNS**: Füge die Einträge von `tofu output dns_records` hinzu, wie [Wie der Traffic den Dienst erreicht](#how-traffic-reaches-the-service)
  es für jede Art sagt. Bis das Zertifikat aktiv ist, schlägt `https://{host}` mit einem TLS-Fehler fehl, sodass Föderation und Browser
  den Server noch nicht erreichen. Nichts anderes wartet darauf: Cloud Tasks und Cloud Scheduler rufen die Standard-URL des Dienstes auf,
  `https://<name>-<project number>.<region>.run.app`. **Deaktiviere die Standard-URL nicht**, sonst bleiben die Jobs stehen. Cloud Run zählt
  diese beiden Aufrufer nur dann als intern, wenn sie aus demselben Projekt kommen und diese URL verwenden, wodurch ein Load Balancer sie
  für das Internet schließen kann.
- **Der erste Administrator**: Der Link bleibt in Cloud Logging, wo ihn jeder verwenden kann, der die Logs des Projekts lesen kann.

<!-- source: the-console-and-upgrades a40a1df56865 -->
## Die Konsole und Upgrades

[Die Konsole](deploy-opentofu.md#the-console) und [Upgrades](deploy-opentofu.md#upgrading) führen Ausführungen der Cloud-Run-
Jobs `<name>-console` und `<name>-migrate` von dort aus, wo du anwendest, über `gcloud` aus. Sie brauchen Zugangsdaten, die die
Jobs ausführen und ihre Logs lesen dürfen, `roles/run.developer` und `roles/logging.viewer`, die Owner einschließt. Ein unterbrochenes Apply
lässt die Ausführung auf Cloud Run weiterlaufen, und das nächste Apply wartet, bis sie endet.

Um [zuerst das SQL zu prüfen](deploy-opentofu.md#checking-the-sql-first):

```sh
tofu apply -target='module.mycroblog.module.app.google_cloud_run_v2_job.one_off["migrate"]'
gcloud run jobs execute my-migrate --project="$PROJECT" --region="$REGION" --args=upgrade,-dry-run --wait
gcloud logging read 'resource.type="cloud_run_job" AND resource.labels.job_name="my-migrate"' --project="$PROJECT" \
  --freshness=1h --format='value(textPayload)' --order=asc
```

<!-- source: the-database 0303fdd046ef -->
## Die Datenbank

Das Modul empfiehlt `external`, das kein Netzwerk braucht und auf Google Cloud nichts abrechnet: Eine Datenbank, die im Leerlauf anhält,
etwa Neon, hält einen ruhigen Server günstig. `cloud-sql` hält alles in einem Projekt, zu festen Kosten. Daten zwischen ihnen zu verschieben ist
[ein Wechsel des Backends](deploy-opentofu.md#changing-a-backend).

<!-- source: an-external-postgresql d6604bfb9e2f -->
### Ein externes PostgreSQL

`database = { kind = "external" }`, der Standard, nimmt jedes PostgreSQL, das Cloud Run erreichen kann: Neon, ein anderes gehostetes
PostgreSQL oder eines, das du betreibst, über das Internet oder auf einer privaten Adresse über [ein Netzwerk](#networks). Das Modul legt das
Secret `my-database-url` an, für die **direkte** URL, die der Migrationsjob und die Konsole immer verwenden: `my schema apply` hält
eine Sperre auf Sitzungsebene, die ein Pooler im Transaktionsmodus nicht hält.

`database = { kind = "external", pooled = true }` fügt das Secret `my-database-url-pooled` hinzu, für eine **gepoolte** URL, die der
Dienst dann verwendet. Ein Pooler passt zum Server, wie [Über einen Connection Pooler](database.md#through-a-connection-pooler)
sagt. Ohne einen verwendet der Dienst ebenfalls die direkte URL, daher muss `service.max_instances` mal der Pool jeder Instanz
unter dem Verbindungslimit der Datenbank bleiben. Der Pool hat standardmäßig 4 Verbindungen pro Instanz, für die 80 Anfragen, die eine Instanz
gleichzeitig annimmt: Erhöhe ihn mit `pool_max_conns` in der URL, die der Dienst verwendet, etwa `...?sslmode=require&pool_max_conns=8`
([Auf eine Datenbankverbindung warten](running.md#waiting-for-a-database-connection)).

**Bei Neon** lege ein Projekt in einer Region nahe deiner Cloud-Run-Region an. Sein direkter Verbindungs-String hat den einfachen Host und sein
gepoolter den Host mit `-pooler`, der Neons Pooler ist und zum Server passt: Setze `pooled = true`. Rechne mit dem Launch-Tarif,
da der Standard-Tick die Compute wach hält; der Free-Tarif funktioniert mit [den Abstrichen unter Kosten](#where-care-reduces-it).

<!-- source: cloud-sql dc9461df8cc2 -->
### Cloud SQL

`database = { kind = "cloud-sql" }` legt eine PostgreSQL-18-Instanz an, standardmäßig `db-f1-micro`, mit täglichen Backups und
Point-in-Time-Recovery, 7 Tage aufbewahrt, ihre Datenbank und ihren Benutzer und ein erzeugtes Passwort, das weder den Plan noch den
State erreicht. `tier`, `disk_size_gb`, `retained_backups` und `deletion_protection` ändern sie. Sie wurde noch nie angewendet: Behandle die
erste Bereitstellung als ihren Test.

- **Ohne Netzwerk** erreicht Cloud Run sie über den Socket des Cloud-SQL-Connectors, und die öffentliche Adresse der Instanz lässt
  nichts anderes zu.
- **Mit einem Netzwerk** hat sie nur eine private Adresse, und ein `pg_dump` muss aus dem Netzwerk heraus laufen.
- **Sie hat keinen Pooler**: `pooled` wird abgelehnt, und `service.max_instances` und `pool_max_conns` begrenzen die Verbindungen. Halte den
  Pool bei `db-f1-micro`, das 0,6 GB Speicher hat, klein.
- **Sie skaliert nie auf null**, wird also unabhängig vom Traffic abgerechnet, und Googles SLA deckt keine Shared-Core-Stufe ab.

<!-- source: the-other-backends 91ef8e2dea11 -->
## Die anderen Backends

Jedes hat seine Empfehlung als Standard, und [ein Wechsel der `kind`](deploy-opentofu.md#changing-a-backend) wechselt zu einem anderen.

- **Jobs**: `cloud-tasks` schickt jeden Job an den Dienst, der auf null skalieren kann. `queue.tick_schedule`, standardmäßig jede Minute,
  weckt den Server und die Datenbank für die periodischen Jobs. `db` fragt die Datenbank von einem Prozess ab, der oben bleibt, braucht also
  `service.min_instances` von 1 oder mehr und `service.cpu_always_allocated = true`.
- **Medien**: `gcs` legt einen privaten Bucket, `media.bucket_name`, in der Region an. `r2` legt einen R2-Bucket in
  `cloudflare.account_id` an, und `s3` verwendet jeden S3-kompatiblen Speicher an seinem `endpoint`; beide lesen einen Zugriffsschlüssel aus den Secrets,
  die `secrets_to_fill` auflistet. Um einen Bucket mit anderen Instanzen zu teilen, nimmt `existing_bucket = true` deinen, und `prefix`
  (`tenant-a/`) beginnt jeden Objektnamen; mit `gcs` erreicht das Dienstkonto dann nur die Objekte darunter, was einen
  einheitlichen Zugriff auf Bucket-Ebene braucht.
- **Caches und Rate Limits**: `db` hält sie in der Datenbank. `memorystore` legt Memorystore für Valkey an, das ein
  Netzwerk braucht und kein Passwort hat: Alles im Netzwerk kann es erreichen. `external` liest die URL eines Valkey, das du betreibst, aus einem Secret.
- **Echtzeit-Aktualisierungen**: `pubsub` erreicht jede Instanz. `postgres` nutzt `LISTEN` und `NOTIFY` über die direkte Verbindung der Datenbank,
  und `valkey` das Valkey von `kv`.
- **Der KEK**: `secret-manager` erzeugt ihn einmal, damit du ihn kopierst. `kms` hält ihn in Cloud KMS, das er nie verlässt, sodass ein
  geleakter Datenbank-Dump und eine geleakte Konfiguration zusammen noch kein Secret preisgeben. Ihn auf einer Instanz mit Daten zu ändern ist
  [ein Umzug](deploy-opentofu.md#the-kek).

<!-- source: networks 9d28311e11a6 -->
## Netzwerke

Die empfohlene Einrichtung braucht keines: Eine externe Datenbank, Cloud Tasks, Cloud Storage und Pub/Sub werden alle über das
Internet mit der eigenen Identität des Dienstes erreicht, sodass Cloud Run an nichts angehängt wird.

Du brauchst eines für Memorystore, das nur private Adressen hat, um Cloud SQL eine private Adresse zu geben oder um eine Datenbank von dir
darauf zu erreichen. `network = { kind = "vpc" }` legt ein VPC, seine Subnetze und den Zugriff auf private Dienste an, und
`network = { kind = "existing", ... }` hängt an ein VPC an, das du hast und das du mit `network`, `subnetwork`,
`private_services_range` und `memorystore_connection_policy` beschreibst:

```hcl
  network = { kind = "vpc" }
  kv      = { kind = "memorystore" }
```

Der Dienst und die Jobs hängen sich über Direct VPC Egress an, und nur private Bereiche gehen durch das Netzwerk, sodass das Internet
seinen eigenen Weg behält, ohne Cloud NAT. Mit `vpc` schickt `nat = true` stattdessen alles durch Cloud NAT, für eine statische
Quelladresse. Ein Netzwerk kann den Start einer Instanz um eine Minute oder mehr verzögern, was ein Server, der auf null skaliert, oft
erlebt. Das VPC, das das Modul anlegt, wurde noch nie angewendet.

<!-- source: how-traffic-reaches-the-service 094ddaf201c8 -->
## Wie der Traffic den Dienst erreicht

`ingress.kind` entscheidet das. Das Modul empfiehlt den Load Balancer, für den Produktivbetrieb. Das Domain-Mapping eignet sich, eine
Bereitstellung günstig auszuprobieren.

| | `load-balancer` | `domain-mapping` |
| --- | --- | --- |
| Monatliche Gebühr | Etwa 18 $ für die Weiterleitungsregeln, plus eine Gebühr pro GB. Prüfe die [Preisseite](https://cloud.google.com/vpc/network-pricing): Diese Zahl ließ sich dort nicht bestätigen | Keine |
| Status | Der von Google Cloud empfohlene Weg | Eine Preview-Funktion, die Google Cloud wegen der Latenz nicht für den Produktivbetrieb empfiehlt |
| Regionen | Alle | `asia-northeast1`, `us-central1` und einige weitere |
| DNS | Ein `A`-Eintrag von `host` auf seine Adresse. Lass jeden Proxy deines DNS-Anbieters dafür aus | Die Einträge, die `dns_records` auflistet |
| Zertifikat | Von Google verwaltet, aktiv, sobald DNS auf die Adresse zeigt, was eine Weile dauern kann | Von Google verwaltet, bis zu 24 Stunden |
| Domain-Inhaberschaft | Nichts außer DNS | Wer anwendet, muss zuerst ein [verifizierter Inhaber](https://cloud.google.com/run/docs/mapping-custom-domains) der Domain sein |
| `run.app`-URL | Aus dem Internet abgewiesen, sodass niemand den Load Balancer umgeht | Für jeden erreichbar: Ein Domain-Mapping kann keinen Header hinzufügen, die Origin bleibt also offen |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` und die Adresse des Load Balancers | `169.254.0.0/16` |

Cloud Tasks und Cloud Scheduler kommen über keines von beiden: Sie rufen die Standard-`run.app`-URL direkt auf.

<!-- source: cloudflare 35dc1e54d2bb -->
### Cloudflare

`ingress = { kind = "cloudflare" }` stellt Cloudflare vor `host`, wobei der Cache für alles umgangen wird, und
`ingress.media_host` fügt neben jeder Art einen gecachten Medien-Host hinzu. Beide brauchen `cloudflare = { zone_id = "..." }`, der Provider
liest sein Token aus `CLOUDFLARE_API_TOKEN`, und das Modul legt die Einträge an, sodass `dns_records` leer ist. Sie liefen nur
auf gemockten Providern.

- **Dein Tarif entscheidet die Origin.** Cloud Run routet nach `Host`, und Cloudflare überschreibt den Host-Header nur im Enterprise-
  Tarif. Bei jedem anderen Tarif legt das Modul ein Domain-Mapping jedes Hosts an, mit den Abstrichen der Tabelle oben, und
  Cloudflare verbindet sich damit. Ob sein Zertifikat hinter dem Proxy ausgestellt und erneuert wird, ist ungeprüft, beginne also mit
  `cloudflare.proxied = false`. Im Enterprise-Tarif verbindet `cloudflare.host_header_override = true` stattdessen mit der `run.app`-
  URL, ohne Domain-Mapping.
- **Ein Header schließt die Origin.** Eine Transformationsregel für Anfrage-Header fügt einen Wert hinzu, den das Modul erzeugt, und der Server
  weist eine Anfrage ohne ihn mit `403` ab, abgesehen von Health Checks und den eigenen `POST`s der Jobs, die ihre Tokens prüfen.
  So umgeht niemand Cloudflare über die `run.app`-URL, und niemand kann die Client-Adresse wählen, die die Rate Limits sehen.
- **Nichts fügt den Header hinzu, solange ein Eintrag nicht geproxyt ist**, sodass der Server dann alles außer seinen Health Checks abweist:
  Schalte bei `proxied = false` für das Zertifikat eines Domain-Mappings den Proxy ein, sobald das Zertifikat aktiv ist.
- **Der Wert landet im State**, wie [Secrets](deploy-opentofu.md#secrets) sagt, und im Secret `my-origin-secret`.
  [Der CDN-Leitfaden](cdn.md#rotate-the-value) erklärt, wie man ihn rotiert.
- **Schalte Bot Fight Mode aus** im Dashboard und schalte „I'm Under Attack“ nicht ein: Eine Challenge auf `/ap/`, `/.well-known/`
  oder `/api/` bricht die Föderation, da andere Server keine Browser sind. Beide gelten für die ganze Zone, die keine Regel ausnehmen kann.
  Das Modul fügt keine Challenge hinzu und schaltet Browser Integrity Check aus.
- **Email Address Obfuscation kann eingeschaltet bleiben.** Die eigenen Seiten des Servers markieren den Inhalt, dessen Adressen er in Ruhe lassen muss.
- **Schalte Always Use HTTPS ein** im Dashboard, wenn du `http://` umleiten willst. Es gilt für die ganze Zone, daher überlässt das Modul
  es dir.
- **Timeouts**: Cloudflare antwortet mit `524`, wenn die Origin 125 Sekunden lang nichts sendet. Jobs sind nicht betroffen, da sie nicht
  über Cloudflare kommen. Streams senden alle 30 Sekunden einen Heartbeat, unter dem Leerlauflimit von 900 Sekunden. Ob Cloudflare
  Streams auf einem geproxyten Host puffert, ist ungeprüft.
- **Video**: Die Bedingungen von Cloudflare schränken das Ausliefern von Video und einen unverhältnismäßig hohen Anteil großer Dateien bei Free, Pro und
  Business ein, und es kann gegen den Inhalt vorgehen. Entscheide das, bevor du Video auf dem Medien-Host cachst.

**Eine gelöschte Datei bleibt im Cache.** Der Medien-Host folgt dem `Cache-Control` des Servers, sodass eine gelöschte lokale Datei bis zu einem Jahr sichtbar bleiben kann und eine entfernte für `remote_media_cache_ttl_seconds`. Der Server leert sie nicht. Wenn etwas verschwinden muss, leere sie von Hand, unter **Caching > Configuration > Custom Purge** im Dashboard, oder:

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

Eine Proxy-URL trägt ihr `sig` in der Query-Zeichenfolge, die zu dem gehört, was gecacht wird, gib also die ganze URL an.

<!-- source: monitoring-and-alerts eb28e653de36 -->
## Monitoring und Alarme

`monitoring` fügt eine Uptime-Prüfung von `/health/live`, logbasierte Metriken und Alarmrichtlinien mit den Namen aus
[dem Runbook](alerts.md) hinzu, die die Adressen von `notification_emails` (`alert_emails` im Beispiel) oder die Kanäle
von `notification_channels` benachrichtigen. Der Abschnitt hat keinen Standardwert: Nenne, wer benachrichtigt wird, oder setze `monitoring = null`, um alles
wegzulassen. Er wurde noch nie angewendet, probiere ihn also einmal aus, indem du eine Prüfung fehlschlagen lässt. Jeder Lauf der Prüfung weckt eine Instanz:
`uptime_period`, standardmäßig 15 Minuten, bestimmt, wie oft.

<!-- source: traces b1f84a2f93b9 -->
### Traces

Cloud Trace zeigt den Trace, den Cloud Run von jeder Anfrage aufzeichnet, und die Logzeilen des Servers tragen seine `trace_id`, da das
Modul dem Ingress vertraut (`MY_TRUSTED_PROXIES`). Das Modul setzt `logs_exporter = gcp`, sodass jede Zeile auch die Trace-Felder
füllt, die Cloud Logging mit Cloud Trace verknüpft (`logging.googleapis.com/trace`), und jede Zugriffszeile als Anfrage erscheint.
`observability.logs = "settings"` überlässt `logs_exporter` den `settings`. Ungeprüft: dass Cloud Logging eine Zeile über die
reine Trace-ID verknüpft, die sie schreibt, und dass Cloud Run das `traceparent` einer öffentlichen Anfrage immer ersetzt.
Die eigenen Spans des Servers bräuchten einen OpenTelemetry Collector daneben, den das Modul nicht betreibt.

<!-- source: limits 3d4721415f18 -->
## Limits

- **Uploads**: Cloud Run weist eine HTTP/1-Anfrage über 32 MiB ab, und der Server spricht HTTP/1.1, daher setzt das Modul
  `MY_MEDIA_MAX_AV_BYTES` auf 30 MiB, gegenüber dem Standard von 100 MiB, damit ein größerer Upload die eigene Antwort des Servers bekommt.
  Das eigene Limit von Cloudflare, 100 MB bei Free und Pro, liegt darüber.
- **Streams**: `service.request_timeout_seconds`, standardmäßig 15 Minuten, beendet einen offenen Stream, und der Client verbindet sich erneut.
- **Datenbankverbindungen**: Google begrenzt eine Cloud-Run-Instanz auf 100 Verbindungen zu Cloud SQL.
- **Client-Adressen**: Die Rate Limits stützen sich auf `MY_TRUSTED_PROXIES`, das das Modul aus dem Ingress setzt. Kein Dokument, das wir
  gelesen haben, sagt, was Cloud Run bei einer direkten Anfrage zu `X-Forwarded-For` hinzufügt oder welche Peer-Adresse ein Container sieht, daher ist
  `169.254.0.0/16` eine Annahme. Die dokumentierte Form des Load Balancers ist `[<supplied>,]<client-ip>,<lb-ip>`. Prüfe die
  Adressen in den ersten Request-Logs: Eine Client-Adresse in `169.254` oder von Google bedeutet, dass die Einstellung für dein Projekt falsch ist.
- **Öffentlicher Zugriff**: Der Dienst gewährt `roles/run.invoker` an `allUsers`, was eine Organisationsrichtlinie zur domänenbeschränkten
  Freigabe abweist. Ein solches Projekt braucht eine Ausnahme.
- **Namen, die belegt bleiben**: Cloud SQL und Cloud Tasks weisen den Namen einer gelöschten Instanz oder Queue etwa eine Woche lang ab, und eine
  gelöschte benutzerdefinierte Rolle behält ihre ID eine Weile, sodass eine Bereitstellung, die nach einem `destroy` erneut erfolgt, einen anderen `name` braucht.

<!-- source: costs ea658d24d79d -->
## Kosten

Diese Zahlen sind ein Anhaltspunkt dafür, was dich erwartet und wo Sorgfalt sie senkt, keine Grenzen, an die sich die Bereitstellung hält. Es
sind ungefähre Listenpreise für eine Region, von 2026-10 und 2026-11: Prüfe sie auf den Preisseiten von
[Cloud Run](https://cloud.google.com/run/pricing), [Cloud SQL](https://cloud.google.com/sql/pricing) und
[Neon](https://neon.com/pricing). Die Compute von Cloud Run ist pro Rechnungskonto bis zu 180.000 vCPU-Sekunden und 360.000
GiB-Sekunden im Monat kostenlos.

Was jede Bereitstellung zahlt, im Leerlauf etwa 0 $:

| Teil | Leerlauf | Was dazukommt |
| --- | --- | --- |
| Cloud Run | 0 $: Keine Instanz läuft, und die CPU wird nur während Anfragen abgerechnet | Jeder offene Stream hält eine Instanz in Rechnung; siehe unten |
| Cloud Tasks | 0 $ | 1 Million Operationen frei, danach 0,40 $ pro Million. Ein Job braucht etwa 2 Operationen (ihn einzureihen und zuzustellen) |
| Cloud Scheduler | 0 $: Der Tick ist ein Job, und 3 sind pro Rechnungskonto frei | 0,10 $ pro Job darüber |
| Pub/Sub | 0 $: 10 GiB frei | Nur solange Streams offen sind |
| Secret Manager | Cent-Beträge | Einige freie Versionen, danach ein paar Cent pro Version |
| Cloud Storage | Etwa 0,02 $ pro GiB im Monat | 5 GiB sind nur in `us-east1`, `us-west1` und `us-central1` frei. Medien gehen über Cloud Run hinaus |
| Monitoring | 0 $ innerhalb der Freikontingente | |

Was jede Wahl dazubringt:

| Abschnitt | Wahl | Pro Monat |
| --- | --- | --- |
| `ingress` | `load-balancer` | Etwa 18 $, auf der Preisseite nicht bestätigt, plus eine Gebühr pro GB |
| | `domain-mapping` | 0 $ |
| | `cloudflare` | Dein Cloudflare-Tarif und die Origin: 0 $ mit einem Domain-Mapping |
| `database` | `external` im Launch-Tarif von Neon | Etwa 19 $ mit dem Standard-Tick, plus 0,35 $ pro GB-Monat |
| | `external` im Free-Tarif von Neon | 0 $, innerhalb seiner Limits: [unten](#where-care-reduces-it) |
| | `external` anderswo | Was diese Datenbank kostet |
| | `cloud-sql` | Etwa 9,40 $ für `db-f1-micro` und 10 GiB in `us-central1`, plus die Backups, unabhängig vom Traffic |
| `queue` | `db` | Eine rund um die Uhr abgerechnete Instanz, mit immer zugeteilter CPU |
| `kv` | `memorystore` | Etwa 23 $ für den kleinsten Knoten, unabhängig vom Traffic |
| `kek` | `kms` | Cent-Beträge, für den Schlüssel und seine Operationen |
| `network` | `vpc` mit `nat = true` | Cloud NAT, nach Stunde und nach GB |

Die empfohlene Einrichtung im Launch-Tarif von Neon, der Load Balancer und Neon, kommt auf etwa 37 $ im Monat; mit Cloud SQL auf
etwa 27 $.

**Der Tick hält Neon wach.** Jeder Tick liest und schreibt die Datenbank, und Neon hält eine Compute erst nach 5 Minuten ohne
Aktivität an, daher stoppt sie mit dem Standard `* * * * *` nie. Das sind 730 Stunden im Monat oder 182,5 CU-Stunden bei 0,25 CU,
was die 19 $ oben ergibt. Der Tick bleibt bei jeder Minute, damit periodische Jobs pünktlich laufen und ein verlorenes Weck-Signal bald
wieder aufgeholt wird. Cloud SQL rechnet unabhängig vom Tick gleich ab.

**Ein offener Stream hält eine Instanz in Rechnung.** Bei Abrechnung nach Anfragen wird eine Instanz abgerechnet, solange sie irgendeine Anfrage bearbeitet,
und die Echtzeit-Aktualisierungen sind eine lange Anfrage. Ein Client, der seinen Stream den ganzen Monat offen hält, kostet eine Instanz rund um die
Uhr: etwa 60 $ im Monat bei 1 vCPU und 512 MiB, nach dem Freikontingent. Mehr Streams teilen sich die Instanz, bis
`service.concurrency`. Wenn Streams die meiste Zeit offen bleiben, vergleiche mit der Abrechnung nach Instanzen
(`service.cpu_always_allocated`) oder mit einer kleinen VM und [Docker Compose](deploy-docker-compose.md).

<!-- source: where-care-reduces-it 427be9d46d32 -->
### Wo Sorgfalt die Kosten senkt

- **Das Domain-Mapping** anstelle des Load Balancers nimmt die 18 $ weg, mit
  [seinen Abstrichen](#how-traffic-reaches-the-service). Es eignet sich zum Ausprobieren einer Bereitstellung.
- **Der Free-Tarif von Neon** nimmt die 19 $ weg, und eine kleine Instanz kann darauf laufen, mit diesen Abstrichen:
  - Er erlaubt 100 CU-Stunden im Monat, die der Standard-Tick nach etwa 17 Tagen verbraucht. Bleibe darunter mit einem längeren
    Zeitplan, etwa `queue = { tick_schedule = "*/10 * * * *" }`: höchstens die halbe Zeit wach, etwa 91 CU-Stunden, bevor
    irgendetwas anderes sie weckt. Periodische Jobs laufen dann bis zu 10 Minuten zu spät, und ein verlorenes Weck-Signal wird später aufgeholt.
  - Ein föderierter Server empfängt rund um die Uhr Zustellungen, die auch die Datenbank wecken, sodass eine ausgelastete Instanz ihn
    unabhängig vom Zeitplan überschreitet.
  - Er fasst 1 GB pro Projekt und einen 6-Stunden-Verlauf zum Wiederherstellen, und seine Compute stoppt immer nach 5 Minuten Leerlauf, sodass
    die erste Anfrage danach auf sie wartet.
