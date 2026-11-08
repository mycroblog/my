<!-- source: alerts-and-runbook 2c201450ab32 -->
# Alarme und Runbook

Worüber dich die eigenen Metriken des Servers alarmieren sollten, wie dringend jeder Alarm ist und was du prüfen und tun solltest, wenn einer auslöst. Den Server von außen zu überwachen (Uptime-Monitore und eine Statusseite), steht in [Den Server von außen überwachen](monitoring.md);
richte auch das ein, denn von einem Server, der ausgefallen ist, kommt keine Metrik an.

<!-- source: getting-the-metrics 58d8055fe44d -->
## Die Metriken abrufen

Der Server exportiert seine Metriken, wenn `metrics_exporter` gesetzt ist ([Konfigurationsreferenz](../reference/configuration.md#metrics_exporter));
der [Betriebsleitfaden](running.md) listet jede Metrik und ihre Labels auf. Die folgenden Alarme sind für Prometheus geschrieben, und die Namen sind
die, die Prometheus sieht: Punkte werden zu Unterstrichen, und die Suffixe für Einheit und Zähler werden angehängt
(`my.queue.oldest_ready_age` wird zu `my_queue_oldest_ready_age_seconds`, `my.delivery.attempts` zu `my_delivery_attempts_total`).

| Ziel | Einstellung | Alarmierung |
| --- | --- | --- |
| Selbst betrieben (Docker Compose, Kubernetes, eine VM) | `metrics_exporter = prometheus` | Prometheus ruft `GET /metrics` auf `metrics_listen` ab, Alertmanager leitet die Alarme weiter. Die folgenden Regeln funktionieren unverändert |
| Jedes Ziel mit einem OpenTelemetry Collector | `metrics_exporter = otlp` | Der Collector leitet an das Backend deiner Wahl weiter. Ein Prometheus-kompatibles Backend, das die Suffixe für Einheit und Typ anhängt (wie es der OTLP-Empfänger von Prometheus standardmäßig tut), sieht dieselben Namen |
| Google Cloud (Cloud Run) | `metrics_exporter = none` | Alerting-Richtlinien von Cloud Monitoring auf einer Uptime-Prüfung, den Plattformmetriken und logbasierten Metriken (siehe [Cloud Run](#google-cloud-cloud-run)) |
| AWS (ECS) | `metrics_exporter = emf` | CloudWatch-Alarme auf den Metriken, die CloudWatch Logs extrahiert (siehe [AWS ECS](#aws-ecs)) |

<!-- source: prometheus-and-alertmanager 8e8dcaa334e9 -->
### Prometheus und Alertmanager

Die Regeln stehen in [`deploy/monitoring/prometheus/alerts.yml`](../../deploy/monitoring/prometheus/alerts.yml). Kopiere die Datei neben deine
`prometheus.yml` und lade sie:

```yaml
rule_files:
  - alerts.yml

scrape_configs:
  - job_name: my          # the rules expect this job name
    static_configs:
      - targets: ["my:9464"]
```

- `metrics_listen` ist standardmäßig `127.0.0.1:9464`, was nur ein Prometheus auf demselben Host erreicht. Setze in Docker Compose oder Kubernetes
  `MY_METRICS_LISTEN=0.0.0.0:9464` und halte den Port im privaten Netzwerk: Der Listener hat keine Authentifizierung.
- Rufe jeden Prozess ab (jeder Prozess mit `role = web` und `role = worker` hat seinen eigenen Listener). Die Gauges für Queue, Medien, Datenbankgröße,
  entfernte Server, Billing-Import und Snapshot tragen in jedem Prozess denselben instanzweiten Wert, daher nehmen die Regeln das Maximum.
- Jede Regel hat ein Label `severity`: **`page`** braucht sofort jemanden (Nutzer sind betroffen oder bald betroffen), **`ticket`** kann bis zu den Arbeitszeiten warten.
  Leite beide in Alertmanager an unterschiedliche Empfänger weiter. Die `runbook_url` jeder Regel verweist auf ihren Abschnitt weiter unten.
- Die Schwellenwerte sind Ausgangspunkte für einen kleinen Server. Beobachte die Werte eine Woche lang und passe Schwellenwerte und `for`-Dauern an deinen Traffic an.
  `MyRemoteMediaCacheOverLimit`, `MyMediaStorageLarge` und `MyDatabaseStorageLarge` enthalten Größen, die du auf deine eigenen Limits setzen musst.
- Im Wartungsmodus liefert der Server keine Metriken und antwortet mit `503`, daher lösen `MyTargetDown` und die HTTP-Alarme aus.
  Schalte die Alarme in Alertmanager für das Zeitfenster stumm, bevor du beginnst ([Wartung](maintenance.md#steps)).

Prüfe die Datei, nachdem du die Regeln geändert hast, und führe ihre Unit-Tests (`alerts.test.yml`) mit `promtool` aus, das mit Prometheus kommt:

```sh
cd deploy/monitoring/prometheus
promtool check rules alerts.yml
promtool test rules alerts.test.yml
```

<!-- source: google-cloud-cloud-run 72c348585d24 -->
### Google Cloud (Cloud Run)

Baue die Alarme mit `metrics_exporter = none` aus dem, was Cloud Run und Cloud Logging bereits haben:

- **HTTP-Fehler und Latenz**: die Anzahl der Cloud-Run-Anfragen nach Antwortcode-Klasse und die Latenzmetrik des Dienstes.
- **Fehler**: Die Zeilen `error report` gehen an Error Reporting, das sie gruppiert und dich über neue und wiederkehrende Fehler benachrichtigen kann ([Fehlerberichte](error-reporting.md#google-cloud-cloud-run)).
- **Jobs und Anfragen**: Der Server schreibt pro HTTP-Anfrage (`msg` = `http request`) und pro Job-Lauf (`msg` = `job`) eine Logzeile mit denselben
  Labels wie die Metriken. Erzeuge daraus logbasierte Metriken, zum Beispiel einen Zähler von `jsonPayload.msg="job" AND jsonPayload."my.outcome"="dead"`
  für fehlgeschlagene Jobs oder eine Verteilung von `jsonPayload.duration_seconds`, gefiltert auf `jsonPayload."my.surface"="client_api"`, für die Latenz.
- **Queue-Rückstau**: Mit `queue = cloudtasks` liegen die Jobs in der Datenbank, daher zeigen der Job-Bildschirm und die Logzeilen der Job-Läufe den Rückstau;
  die Metrik für die Queue-Tiefe von Cloud Tasks zählt die Signale, die auf Zustellung warten.
- **Datenbank**: die Metriken zu Festplattenauslastung und Verbindungen von Cloud SQL oder die Nutzungsalarme deines Datenbankanbieters.
- **Kosten**: ein Cloud-Billing-Budget mit Alarmschwellen ([Kosten und Budget](#cost-and-budget)).

Das Modul [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) erzeugt diese als Richtlinien mit den Namen aus der Tabelle unten und listet jeden Alarm auf, den es anders abdeckt oder nicht ausdrücken kann. Es wurde noch nie angewendet. Eine Metrik, die nur Traffic erzeugt, hat bei einem Dienst, der auf null skaliert, Lücken, daher löst ein Alarm auf ihr Ausbleiben fälschlich aus; seine Uptime-Prüfung ist es, die einen toten, untätigen Server bemerkt.

<!-- source: aws-ecs 609e5bf632ef -->
### AWS ECS

Mit `metrics_exporter = emf` macht CloudWatch Logs aus den Zeilen des Servers Metriken im Namespace `metrics_emf_namespace`, mit den Namen mit Punkten
(`my.job.duration`, `my.queue.oldest_ready_age`). Die Metriken zur Dauer von Anfragen und Jobs stammen aus den Logzeilen der Anfragen und Jobs, und die
Dimensionen sind die Labels, die du in `metrics_emf_dimensions` auflistest; nimm `my.outcome` auf, um bei fehlgeschlagenen Jobs zu alarmieren, und `http.response.status_code`,
um bei 5xx zu alarmieren. Die instanzweiten Gauges (Queue, Medien, Datenbankgröße, entfernte Server, Billing-Importe, Snapshot) schreibt der periodische Snapshot-Job. Die Go-Runtime-Metriken (`go.*`)
werden nicht in EMF geschrieben; nutze stattdessen die CPU- und Speichermetriken des ECS-Dienstes.

- **HTTP-Fehler**: die Metriken für 5xx und Anfragenzahl des Application Load Balancers.
- **Queue-Rückstau**: `my.queue.oldest_ready_age`, die der Snapshot-Job schreibt.
- **Datenbank**: die Speicher- und Verbindungsmetriken von RDS oder Aurora oder die Nutzungsalarme deines Datenbankanbieters.
- **E-Mail** (`mail = ses`): die SES-Reputationsmetriken (Bounce- und Beschwerderaten) in CloudWatch.
- **Kosten**: ein AWS-Budgets-Budget mit Alarmen ([Kosten und Budget](#cost-and-budget)).

Das Modul [`monitoring-aws`](../../deploy/opentofu/modules/monitoring-aws/README.md) erzeugt diese als Alarme mit den Namen aus der Tabelle unten, mit derselben Liste. Es wurde noch nie angewendet.

<!-- source: the-alerts d76842ca02b8 -->
## Die Alarme

Schweregrad: **page** = sofort handeln, **ticket** = während der Arbeitszeit. „–“ in der Regelspalte bedeutet, dass es dafür noch keine Metrik gibt; prüfe es wie beschrieben.

| Bereich | Alarm | Signal | Schwellenwert | Schweregrad | Wahrscheinliche Ursachen |
| --- | --- | --- | --- | --- | --- |
| Queue | [`MyQueueBacklog`](#myqueuebacklog) | `my_queue_oldest_ready_age_seconds` | über 5 Min. für 10 Min. | ticket | Zu wenige Worker für einen Ansturm (ein beliebter Beitrag, ein Relay), langsame Jobs, ein Worker, der angehalten hat |
| Queue | [`MyQueueStalled`](#myqueuestalled) | `my_queue_oldest_ready_age_seconds` | über 30 Min. für 5 Min. | page | Kein Worker läuft (abgestürzt, nicht ausgerollt, nur `role = web`), kein Worker führt einen Job-Typ aus, Worker hängen an der Datenbank |
| Queue | [`MyDeadJobsIncreasing`](#mydeadjobsincreasing) | `delta(my_queue_dead[1h])` | über 10 pro Stunde für 15 Min. | ticket | Ein Fehler in einem Job, ein falsch konfigurierter Adapter (Mail, Blob, Billing), ein entfernter Dienst, der ablehnt |
| Queue | [`MyMetricsSnapshotStale`](#mymetricssnapshotstale) | `my_metrics_snapshot_age_seconds` | Queue-Teil über 5 Min., Summen-Teil über 1 Std., für 15 Min. | ticket | Der KV-Speicher ist nicht erreichbar, die Datenbankabfragen des Snapshots schlagen fehl oder laufen in ein Timeout |
| Föderation | [`MyDeliveryFailuresHigh`](#mydeliveryfailureshigh) | `my_delivery_attempts_total` mit `my_outcome` `retry` oder `failed` | über 50 % der Versuche für 30 Min. | ticket | Viele Follower auf Servern, die ausgefallen sind, ein großer Server ist ausgefallen, Probleme im ausgehenden Netzwerk |
| Föderation | [`MyDeliveryAllFailing`](#mydeliveryallfailing) | dasselbe | über 95 % der Versuche für 15 Min. | page | Ausgehendes Netzwerk oder DNS kaputt, Egress blockiert, die Schlüssel oder `host` des Servers falsch |
| Föderation | [`MyRemoteServersPaused`](#myremoteserverspaused) | `my_federation_instances{my_federation_instance_state="unavailable"}` | 5 mehr als einen Tag zuvor | ticket | Server, die abgeschaltet wurden oder diesen blockieren, ein Netzwerkproblem auf dieser Seite, das tagelang andauerte |
| Föderation | [`MyInboxRejectionsHigh`](#myinboxrejectionshigh) | `my_inbox_activities_total{my_outcome="rejected"}` | über 25 % der POSTs für 30 Min. | ticket | Signaturprüfung schlägt fehl (Zeitabweichung, Schlüsselabrufe schlagen fehl), ein Spammer oder eine gesperrte Domain versucht es erneut, Ratenbegrenzungen |
| Föderation | [`MyInboxSignatureFailures`](#myinboxsignaturefailures) | `my_inbox_activities_total{my_inbox_rejection="signature"}` | über 10 % der POSTs für 30 Min. | ticket | Die Uhr des Servers geht falsch, der Abruf der Schlüssel der Absender schlägt fehl, ein Server hat seine Schlüssel gewechselt |
| Föderation | [`MyInboxErrors`](#myinboxerrors) | `my_inbox_activities_total{my_outcome="error"}` | über 5 % der POSTs für 15 Min. | ticket | Die Datenbank fällt aus, ein Fehler in der Inbox |
| Fehler | [`MyErrorReportsIncreasing`](#myerrorreportsincreasing) | `my_errors_reported_total` | über 10 in 15 Min. und das Dreifache des Durchschnitts des Vortags, für 15 Min. | ticket | Ein neuer Fehler aus einem Release, eine Abhängigkeit, die auf neue Weise ausfällt, eine fehlerhafte Konfiguration |
| HTTP | [`MyHTTPErrorsHigh`](#myhttperrorshigh) | 5xx von `http_server_request_duration_seconds_count` | über 5 % für 10 Min. | page | Datenbank oder KV ausgefallen, ein fehlerhaftes Release, der Wartungsmodus |
| HTTP | [`MyHTTPErrorsElevated`](#myhttperrorselevated) | dasselbe | über 1 % für 30 Min. | ticket | Eine fehlerhafte Route, ein Adapter, der hin und wieder ausfällt |
| HTTP | [`MyAPILatencyHigh`](#myapilatencyhigh) | p95 der Client-API (Streaming ausgenommen) | über 300 ms für 30 Min. | ticket | Datenbank ausgelastet oder weit entfernt, eine langsame Abfrage, zu wenig CPU, Kaltstarts |
| Ressourcen | [`MyTargetDown`](#mytargetdown) | `up{job="my"}` | 0 für 5 Min. | page | Der Prozess ist ausgefallen, startet in einer Schleife neu oder ist im Wartungsmodus; der Abrufweg ist kaputt |
| Ressourcen | [`MyNotReady`](#mynotready) | `my_readiness_failures_total` | irgendein Anstieg für 5 Min. | page | Die Datenbank ist nicht erreichbar, oder das Schema ist nicht angewendet |
| Ressourcen | [`MyProcessRestarting`](#myprocessrestarting) | `resets(process_uptime_seconds[1h])` | 3 Neustarts in einer Stunde | ticket | Wegen Speicher beendet (OOM), eine Panic, eine fehlschlagende Liveness-Probe |
| Ressourcen | [`MyMemoryNearLimit`](#mymemorynearlimit) | `go_memory_used_bytes` / `go_memory_limit_bytes` | über 90 % für 15 Min. | ticket | `GOMEMLIMIT` zu niedrig für die Last, viele offene Streams, ein Leck |
| Ressourcen | [`MyDBPoolSaturated`](#mydbpoolsaturated) | `db_client_connection_count{db_client_connection_state="used"}` / `db_client_connection_max` | über 90 % für 15 Min. | ticket | Pool zu klein für den Traffic, langsame Abfragen halten Verbindungen fest |
| Ressourcen | [`MyDBPoolWaiting`](#mydbpoolwaiting) | `db_client_connection_wait_time_seconds_total`, `db_client_connection_timeouts_total` | Wartezeit über 0,5 s pro Sekunde oder irgendein Timeout, für 10 Min. | ticket | Wie oben; das eigene Verbindungslimit der Datenbank ist erreicht |
| Ressourcen | [`MyDatabaseStorageLarge`](#mydatabasestoragelarge) | `my_db_size_bytes` | über dem Limit deines Tarifs abzüglich eines Puffers (Beispiel 8 GiB) für 1 Std. | ticket | Wachstum von Beiträgen und zwischengespeicherten entfernten Inhalten; Zeit, den Tarif zu erhöhen oder die Aufbewahrung zu verkürzen |
| Ressourcen | [`MyFileDescriptorsNearLimit`](#myfiledescriptorsnearlimit) | `process_unix_file_descriptor_count` / `my_process_file_descriptor_limit` | über 80 % für 15 Min. | ticket | Viele offene Streams und Verbindungen, ein niedriges `ulimit -n` |
| Ressourcen | [`MyAdapterErrors`](#myadaptererrors) | `my_adapter_operation_duration_seconds_count{my_outcome="error"}` | über 5 % der Operationen eines Adapters für 15 Min. | ticket | Valkey, das Blob-Verzeichnis oder das Stream-Backend fällt aus |
| Ressourcen | [`MyRemoteMediaCacheOverLimit`](#myremotemediacacheoverlimit) | `my_media_stored_bytes{my_media_origin="remote_cache"}` | über dem 1,2-Fachen von `remote_media_cache_max_bytes` für 3 Std. | ticket | Die stündliche Bereinigung läuft nicht oder schlägt fehl |
| Ressourcen | [`MyMediaStorageLarge`](#mymediastoragelarge) | `my_media_stored_bytes` | über deiner geplanten Größe (Beispiel 50 GiB) für 1 Std. | ticket | Wachstum der Uploads; Zeit, den Tarif oder das Budget zu erhöhen |
| E-Mail | [`MyEmailFailing`](#myemailfailing) | `my_job_duration_seconds_count` der E-Mail-Jobs mit `my_outcome="dead"` | irgendeiner in einer Stunde | ticket | Falsche SMTP- oder SES-Einstellungen, ein gesperrtes Konto oder aufgebrauchtes Kontingent, dauerhaft abgelehnte Adressen |
| E-Mail | [`MyEmailRetrying`](#myemailretrying) | dasselbe, `my_outcome="retry"` | über 50 % der Läufe für 15 Min. | ticket | Der Mailserver ist nicht erreichbar oder lehnt die Anmeldung ab, ein Limit der Senderate |
| Kosten | [`MyBillingImportFailing`](#mybillingimportfailing) | `my_billing_import_failing` | 1 für 6 Std. | ticket | Fehlende Berechtigungen, der Billing-Export ist nicht eingerichtet, das Anfragelimit ist erreicht, der Anbieter ist nicht verfügbar |
| Kosten | Budget überschritten | – | – | – | Traffic, Speicher oder eine Fehlkonfiguration kosten mehr als geplant ([unten](#cost-and-budget)) |

<!-- source: where-to-look 9ef1d37c4f05 -->
## Wo du nachsehen kannst

Diese Werkzeuge kehren in den meisten der folgenden Runbook-Einträge wieder.

- **Der Job-Bildschirm** (Administration → Jobs, `/admin/jobs`, braucht `manage_jobs`): die Anzahl nach Zustand (in der Queue, laufend, Wiederholung, fehlgeschlagen) und Typ sowie
  der am längsten wartende Job; seine Seite **Jobs** (`/admin/jobs/list`) listet die Jobs eines Zustands auf, und die Seite jedes Jobs zeigt seinen letzten Fehler und die Versuche, die nicht erfolgreich waren. Ausstehende Jobs werden nur für die Queues aufgelistet, die in der Datenbank liegen
  (`queue = db` und `cloudtasks`); fehlgeschlagene Jobs und Versuche werden für jede Queue aufgelistet.
- **Der Befehl `my job`** auf einem Host, der die Konfiguration des Servers hat (`queue = db` und `cloudtasks`):
  `my job list` listet die fehlgeschlagenen Jobs auf, die neuesten zuerst; `my job retry <id>...` stellt sie zurück in die Queue; `my job delete <id>...` verwirft sie.
  Jobs können gefahrlos erneut laufen.
- **Der Bildschirm für entfernte Server** (Moderation → Entfernte Server, `/moderation/instances`, braucht `manage_domains`): die Server, die Zustellungen nicht erreichen konnten,
  und jene, deren Zustellungen pausiert sind, mit Grund und Zeitpunkt. „Zustellungen fortsetzen“ markiert einen Server wieder als verfügbar.
- **Die Logs**: JSON-Zeilen auf der Standardfehlerausgabe. Jede HTTP-Anfrage endet mit einer Zeile, deren `msg` `http request` ist, jeder Job-Lauf mit einer Zeile, deren `msg` `job` ist
  (`INFO` bei Erfolg, `WARN` bei einer Wiederholung, `ERROR` bei einem Job, der endgültig fehlgeschlagen ist, mit `error`, `job_id` und `my.job.type`).
  Zeilen innerhalb einer Anfrage tragen deren `request_id`, die der Client als `X-Request-Id` sieht.
  - Docker Compose: `docker compose logs my | jq -c 'select(.msg == "job" and ."my.outcome" == "dead")'`
  - Cloud Logging: `jsonPayload.msg="job" AND jsonPayload."my.outcome"="dead"`
  - CloudWatch Logs Insights: ``filter msg = "job" and `my.outcome` = "dead"``
- **Fehlerberichte**: die Zeilen `error report` und der Fehler-Bildschirm (Administration → Fehler, `/admin/errors`, braucht `manage_errors`) ([Fehlerberichte](error-reporting.md)).
- **Traces**, wenn `traces_exporter = otlp`: Die Logzeilen tragen `trace_id`; öffne sie in deinem Trace-Backend, um zu sehen, wo eine langsame Anfrage ihre Zeit verbracht hat.
- **Der Wartungsmodus** ([Wartung](maintenance.md)): beantwortet jede Anfrage mit einer Wartungsseite und berührt keinen Adapter. Nutze ihn, während du die Datenbank
  oder den KV-Speicher reparierst.

<!-- source: runbook e60e811fcff0 -->
## Runbook

<!-- source: myqueuebacklog e625ad2bb49f -->
### MyQueueBacklog

Jobs (Zustellungen, Benachrichtigungen, E-Mails, Timeline-Fan-out) warten mehr als 5 Minuten auf einen Worker.

Prüfen:

- Den Job-Bildschirm: welche Typen in der Queue liegen und ob viele wiederholt werden. Ein Ansturm eines Typs (`federation.deliver` nach einem Beitrag mit vielen Followern,
  Relay-Aktivitäten) klärt sich von selbst; prüfe, ob das Alter sinkt.
- Dass die Worker-Prozesse laufen (`role = all` oder `role = worker`) und ihre Logs `job`-Zeilen zeigen.
- `MyDBPoolSaturated` oder `MyAPILatencyHigh` zur gleichen Zeit: Die Worker warten auf die Datenbank.

Beheben:

- Starte weitere Worker-Prozesse (`role = worker`) oder gib den vorhandenen mehr CPU.
- Wenn ein Job-Typ langsam ist, finde ihn in den `job`-Logzeilen (`duration_seconds`) und suche nach Fehlern dieses Typs.

<!-- source: myqueuestalled c92f00f06f95 -->
### MyQueueStalled

Der am längsten bereite Job wartet seit über 30 Minuten: Nichts oder fast nichts wird verarbeitet.

Prüfen:

- Dass mindestens ein Prozess mit `role = all` oder `role = worker` läuft und nicht in einer Absturzschleife steckt (`MyProcessRestarting`).
- Die Logs des Workers auf Fehler, die sich bei jedem Job wiederholen (Datenbank, KV, eine Panic).
- `MyNotReady`: Ein Worker, der die Datenbank nicht erreicht, kann keine Jobs annehmen.
- `max by (my_job_type) (my_queue_oldest_ready_age_seconds)`: Wenn ein Typ wartet und die anderen nicht, prüfe, ob ein Worker ihn ausführt ([Worker nach Job-Typ aufteilen](running.md#splitting-workers-by-job-type)).

Beheben:

- Starte die Worker oder starte sie neu. Jobs, die ein angehaltener Worker hielt, gehen zurück in die Queue, wenn ihre Lease abläuft; sie können gefahrlos erneut laufen.
- Wenn die Datenbank die Ursache ist, folge [MyNotReady](#mynotready).

<!-- source: mydeadjobsincreasing b8e928621ac7 -->
### MyDeadJobsIncreasing

Mehr als 10 Jobs pro Stunde schlagen endgültig fehl: Ihre Wiederholungen sind aufgebraucht, oder der Job hat sich geweigert, erneut zu laufen.

Prüfen:

- Den Job-Bildschirm, gefiltert auf „Fehlgeschlagen“: die Typen und den letzten Fehler. Die Versuche eines Jobs zeigen jeden Fehler auf dem Weg.
- Dieselben Fehler in den `job`-Logzeilen mit `my.outcome` = `dead`.

Beheben:

- Ein Konfigurationsfehler (Mail, Blob, Billing-Import, ein Webhook-Ziel): Behebe ihn und führe die Jobs dann vom Job-Bildschirm aus erneut aus („Erneut ausführen“) oder mit `my job retry <id>...`.
- Zustellungen an Server, die nicht mehr existieren, schlagen nach etwa 4 Tagen Wiederholungen endgültig fehl; das ist erwartet und erfordert nichts.
- Fehlgeschlagene Jobs, die niemand wiederholt, werden nach `dead_job_retention` entfernt (standardmäßig 30 Tage). Lösche die, die du nicht wiederholen wirst, mit `my job delete`.
- Ein Fehler, der wie ein Bug aussieht: Melde ihn mit dem Job-Typ und dem Fehler (er enthält keine personenbezogenen Daten).

<!-- source: mymetricssnapshotstale c74fe15e6934 -->
### MyMetricsSnapshotStale

Die Gauges für Queue und Speicher stammen aus einem Snapshot, den die Prozesse über den KV-Speicher teilen. Er ist alt, daher
sehen die Alarme für Queue und Speicher alte Werte und bleiben womöglich still.

Prüfen:

- Die Logs auf Fehler zum Metrik-Snapshot oder zum KV-Speicher.
- `MyAdapterErrors` für `kv.valkey` oder `kv.db` und den Zustand der Datenbank.

Beheben:

- Repariere den KV-Speicher oder die Datenbankverbindung. Der Snapshot wird bei der nächsten Erfassung neu berechnet; der periodische Snapshot-Job läuft außerdem alle 15 Minuten.

<!-- source: mydeliveryfailureshigh 709985dfbf2b -->
### MyDeliveryFailuresHigh

Mehr als die Hälfte der Zustellversuche an andere Server schlägt fehl. Manche Fehlschläge sind normal: Server verschwinden, und der Server versucht es etwa 4 Tage lang weiter.

Prüfen:

- Den Bildschirm für entfernte Server: ob sich die Fehlschläge auf wenige Server konzentrieren (sie sind ausgefallen; nichts zu tun) oder über viele verteilen (dann ist dieser Server verdächtig).
- `http_client_request_duration_seconds_count` nach `error_type`: Timeouts, DNS-Fehler oder `forbidden_destination` (der SSRF-Schutz hat das Ziel abgelehnt).

Beheben:

- Fehlschläge bei wenigen Servern: nichts. Nach Fehlschlägen an 7 verschiedenen Tagen werden die Zustellungen an diesen Server pausiert, und der Server wird täglich geprüft, um sie fortzusetzen.
- Fehlschläge überall: Folge [MyDeliveryAllFailing](#mydeliveryallfailing).

<!-- source: mydeliveryallfailing c81fcf407cb9 -->
### MyDeliveryAllFailing

Fast jede Zustellung schlägt fehl. Das liegt fast immer auf der Seite dieses Servers.

Prüfen:

- Vom Host oder Container des Servers aus, ob ausgehendes HTTPS funktioniert (DNS, Egress-Firewall, NAT, ein Proxy).
- Die `job`-Logzeilen von `federation.deliver` auf den Fehler: TLS-Fehler, Timeouts oder `401` von vielen Servern (sie können die Signatur dieses Servers nicht prüfen).
- Nach einer Änderung von Domain oder Schlüsseln: `host` muss gleich bleiben, sobald der Server föderiert hat, und der KEK muss der sein, mit dem die Schlüssel versiegelt wurden
  ([KEK-Verwaltung](key-management.md)).

Beheben:

- Repariere das ausgehende Netzwerk. Fehlgeschlagene Versuche werden mit Backoff wiederholt (1 Minute, 5 Minuten, 30 Minuten, 2 Stunden und so weiter), daher holen die Zustellungen von selbst auf.
- Wenn die Wiederholungen der Zustellungen aufgebraucht sind, führe die fehlgeschlagenen `federation.deliver`-Jobs vom Job-Bildschirm aus erneut aus.
- Wenn entfernte Server während des Ausfalls pausiert wurden, werden sie bei der nächsten erfolgreichen Zustellung, signierten Anfrage oder der täglichen Prüfung fortgesetzt; du kannst auch
  „Zustellungen fortsetzen“ im Bildschirm für entfernte Server drücken.

<!-- source: myremoteserverspaused 17c3ec51fb53 -->
### MyRemoteServersPaused

Zustellungen an 5 oder mehr Server wurden am letzten Tag pausiert. Ein Server wird pausiert, nachdem Zustellungen an ihn an 7 verschiedenen Tagen fehlgeschlagen sind, daher sind ein paar Pausen pro Woche
normal: Server werden abgeschaltet. Viele auf einmal bedeuten, dass so viele Server gleichzeitig tagelang ausgefallen sind. `my_federation_instances` mit
`my_federation_instance_state` = `available` zählt die Server, an die noch zugestellt wird, die aber an irgendeinem Tag ausgefallen sind; ein Sprung dort kommt vor den Pausen.

Die Metrik hat kein Label pro Server (Labels tragen nie eine entfernte Domain). Prüfen:

- Den Bildschirm für entfernte Server: Der Tab „Pausiert“ listet die pausierten Server auf, mit Zeitpunkt und Grund. Ob sie etwas gemeinsam haben (einen Hosting-Anbieter, eine Software)
  oder breit gestreut sind.
- `MyDeliveryFailuresHigh` und `MyDeliveryAllFailing` über die letzte Woche: Ein Netzwerkproblem auf dieser Seite, das tagelang andauerte, pausiert viele Server.

Beheben:

- Server, die verschwunden sind: nichts. Pausierte Server werden täglich geprüft und setzen sich von selbst fort, wenn sie antworten.
- Nach einem Problem auf dieser Seite: Behebe es und drücke dann „Zustellungen fortsetzen“ für die Server, die dir wichtig sind, oder warte auf die tägliche Prüfung.
- Ein Server, dessen Betreiber meldet, dass er diesen blockiert: Auf dieser Seite hilft nichts. Um die Föderation mit einem Server gezielt zu beenden, nutze eine Domain-Aktion
  (Moderation → Domain-Aktionen).

<!-- source: myinboxrejectionshigh ee5a244ccf12 -->
### MyInboxRejectionsHigh

Mehr als ein Viertel der Aktivitäten, die andere Server an die Inboxen POSTen, wird abgelehnt.

Prüfen:

- `my_inbox_activities_total{my_outcome="rejected"}` nach `my_inbox_rejection`: `signature` (folge [MyInboxSignatureFailures](#myinboxsignaturefailures)),
  `suspended` (eine gesperrte Domain, die es weiter versucht), `federation_off` (die Föderation ist ausgeschaltet), `rate_limited` (eine Client-IP oder ein Server über der
  Ratenbegrenzung) oder `invalid` (Aktivitäten ohne gültiges `id`, `type`, `actor` oder `object`).
- Die `http request`-Logzeilen der Inbox-Routen (`my.surface` = `activitypub`) tragen dieselbe Aufteilung nach `http.response.status_code`
  (`401`, `403`, `429`, `400`) und die Adresse des Clients, wenn du wissen musst, woher die Anfragen kommen.

Beheben:

- Ablehnungen einer gesperrten Domain oder bei ausgeschalteter Föderation erfordern nichts: Sie werden wie vorgesehen abgelehnt.
- Ratenbegrenzte Anfragen eines Servers: ein Spammer oder ein Ansturm; sperre die Domain, wenn es anhält.
- Ungültige Aktivitäten eines Servers: Die Software dieses Servers sendet etwas, das dieser Server nicht liest; melde es mit der Software und Version des Servers.

<!-- source: myinboxsignaturefailures 85aa65f3247c -->
### MyInboxSignatureFailures

Mehr als 10 % der Aktivitäten, die andere Server an die Inboxen POSTen, werden abgelehnt, weil ihre Signatur oder ihr Integritätsnachweis fehlt oder sich nicht prüfen lässt
oder weil der Schlüssel des Absenders nicht abgerufen werden konnte.

Prüfen:

- Die Uhr des Servers: Signaturen werden außerhalb eines Zeitfensters abgelehnt. Prüfe NTP auf dem Host oder der Container-Runtime.
- Den Abruf der Schlüssel der Absender: `MyDeliveryAllFailing` und `http_client_request_duration_seconds_count{my_http_purpose="json"}` nach `error_type`
  (das ausgehende Netzwerk, DNS, der SSRF-Schutz).
- Ob die Ablehnungen von vielen Servern oder von einem kommen (die `http request`-Zeilen mit Status `401`): Ein Server, der seine Schlüssel gewechselt hat oder auf eine
  Weise signiert, die dieser Server nicht akzeptiert, betrifft nur diesen Server.

Beheben:

- Korrigiere die Uhr oder das ausgehende Netzwerk. Andere Server wiederholen ihre Zustellungen, daher geht nichts verloren, wenn es innerhalb etwa eines Tages behoben wird.
- Ein einzelner Server, der fehlschlägt: Melde es dem Betreiber dieses Servers oder diesem Projekt mit der Software und Version des Servers.

<!-- source: myinboxerrors e3cf874c925e -->
### MyInboxErrors

Die Inbox antwortet bei mehr als 5 % der Aktivitäten mit `500`. Die Absender wiederholen es, aber die Aktivitäten kommen verspätet an.

**Prüfe** die `http request`- und Fehler-Logzeilen der Inbox-Routen auf den Fehler und den Zustand der Datenbank (`MyNotReady`, `MyDBPoolWaiting`).

**Behebe** die Datenbank oder melde den Fehler als Bug. Nichts muss erneut abgespielt werden: Die Absender wiederholen es.

<!-- source: myerrorreportsincreasing deed4980cc8c -->
### MyErrorReportsIncreasing

Der Server meldet Fehler (Panics, die Fehler hinter 5xx, fehlgeschlagene Jobs) mit mehr als dem Dreifachen seiner üblichen Rate.

Prüfen:

- Die `error report`-Zeilen der letzten Stunde, gruppiert nach `my.error.fingerprint`: die häufigsten, ihr `my.error.where` und die Kopfzeile von `message`.
  In Cloud Run zeigt Google Cloud Error Reporting dieselben Gruppen.
- Ob ein Release, eine Konfigurationsänderung oder eine ausfallende Abhängigkeit (`MyNotReady`, `MyAdapterErrors`) zur gleichen Zeit begann.
- Die Anfrage- oder Job-Zeilen mit derselben `request_id` oder `job_id`.

Beheben:

- Eine ausfallende Abhängigkeit oder ein Konfigurationsfehler: Behebe sie; fehlgeschlagene Jobs können dann vom Job-Bildschirm aus erneut ausgeführt werden.
- Ein fehlerhaftes Release: Gehe auf das vorherige Image zurück.
- Ein Bug: Melde ihn mit der Kopfzeile und dem Stack der Zeile. Beide sind von personenbezogenen Daten bereinigt.

<!-- source: myhttperrorshigh dd55e9ba0dfd -->
### MyHTTPErrorsHigh

Mehr als 5 % aller Anfragen werden mit 5xx beantwortet. Nutzer sehen Fehler.

Prüfen:

- Ob du im Wartungsmodus bist (erwartetes `503`; schalte den Alarm beim nächsten Mal stumm).
- `MyNotReady` (die Datenbank), `MyAdapterErrors` (KV, Blob, Stream) und ein aktuelles Release.
- Die `http request`-Zeilen mit `http.response.status_code` ≥ 500 nach `http.route` und die Fehlerzeilen mit derselben `request_id`.

Beheben:

- Repariere die ausfallende Abhängigkeit. Wenn die Datenbank Arbeit braucht, schalte in den Wartungsmodus, damit Nutzer eine verständliche Seite statt Fehlern sehen.
- Ein fehlerhaftes Release: Gehe auf das vorherige Image zurück.
- Informiere die Nutzer ([Nutzer über einen Vorfall informieren](#telling-users-about-an-incident)), wenn es länger dauert.

<!-- source: myhttperrorselevated 34d53d12ea4d -->
### MyHTTPErrorsElevated

Mehr als 1 % der Anfragen werden über eine halbe Stunde mit 5xx beantwortet. Meist eine Route oder ein Adapter.

**Prüfe** die `http request`-Zeilen mit einem 5xx nach `http.route` und die Fehlerzeilen dieser Anfragen. **Behebe** die Abhängigkeit oder melde den Fehler als Bug.

<!-- source: myapilatencyhigh 81b66aee9099 -->
### MyAPILatencyHigh

Das 95. Perzentil der Client-API liegt über 300 ms (das Ziel für eine Seite der Start-Timeline mit 20 Beiträgen, warm). Der Streaming-Endpunkt ist ausgenommen.

Prüfen:

- `http_server_request_duration_seconds` nach `http_route`: eine langsame Route oder alle.
- Die Datenbank: `MyDBPoolWaiting`, die CPU und das Slow-Query-Log der Datenbank, die Netzwerkdistanz zwischen Server und Datenbank.
- Den Prozess: CPU-Drosselung, `MyMemoryNearLimit` (ein GC unter Speicherdruck), `go_schedule_duration_seconds` (Goroutinen, die auf eine CPU warten).
- Traces langsamer Anfragen, falls aktiviert.

Beheben:

- Gib der Datenbank oder dem Server mehr Ressourcen oder platziere sie näher beieinander.
- Kaltstarts nach dem Skalieren auf null erhöhen das Perzentil bei einem ruhigen Server; das ist erwartet.

<!-- source: mytargetdown e0308f141452 -->
### MyTargetDown

Prometheus kann einen Prozess nicht abrufen.

Prüfen:

- Dass der Prozess läuft: `docker compose ps`, `kubectl get pods` oder die Konsole der Plattform. Seine letzten Logzeilen sagen, warum er angehalten hat.
- Ob er im Wartungsmodus ist (es werden keine Metriken geliefert; erwartet).
- Dass `metrics_exporter = prometheus` gilt und `metrics_listen` von Prometheus aus erreichbar ist.

Beheben:

- Starte den Prozess erneut. Wenn er nicht startet, sagt das Log, warum: ein Konfigurationsfehler (`my config check` zeigt ihn), die Datenbank nicht erreichbar, ein nicht angewendetes Schema.

<!-- source: mynotready 1092418fbba2 -->
### MyNotReady

Die Readiness-Prüfung schlägt fehl: Die Datenbank ist nicht erreichbar, oder ihre Tabellen passen nicht zu dieser Version. Der Load Balancer schickt keinen Traffic mehr an den Prozess,
und der externe Health-Endpunkt meldet `unavailable`.

Prüfen:

- Die Logzeilen der Readiness-Prüfung: ein Verbindungsfehler (Netzwerk, Zugangsdaten, die Datenbank ist ausgefallen oder hat keine Verbindungen mehr frei) oder die fehlenden Tabellen.
- Die Statusseite und die Konsole des Datenbankanbieters: Ist sie erreichbar, ist der Speicher voll oder ist Wartung?
- Nach einem Upgrade: Wurde das Schema angewendet, bevor die neue Version gestartet ist ([Die Datenbank initialisieren](database.md))?

Beheben:

- Datenbank ausgefallen oder nicht erreichbar: Stelle sie wieder her oder führe ein Failover durch. Während du arbeitest, zeigt der Wartungsmodus den Nutzern eine verständliche Seite.
- Datenbankspeicher voll: Erhöhe den Tarif oder schaffe Platz ([MyDatabaseStorageLarge](#mydatabasestoragelarge)).
- Schema nicht angewendet: Führe `my schema apply` derselben Version aus, danach werden die Prozesse von selbst bereit.
- Daten verloren oder beschädigt: Stelle aus deinem Backup wieder her und behalte den KEK, mit dem die Schlüssel des Backups versiegelt wurden ([KEK-Verwaltung](key-management.md)).

<!-- source: myprocessrestarting d96b041c2ff0 -->
### MyProcessRestarting

Ein Prozess wurde in einer Stunde 3-mal neu gestartet.

Prüfen:

- Den Grund der Plattform für den Neustart: `OOMKilled` (Speicher), eine fehlgeschlagene Liveness-Probe oder der Exit-Code.
- Die letzten Logzeilen vor jedem Neustart auf eine Panic oder einen fatalen Konfigurationsfehler.

Beheben:

- Speicher erschöpft: Setze `GOMEMLIMIT` auf etwa 90 % des Speicherlimits des Containers oder gib dem Container mehr Speicher ([MyMemoryNearLimit](#mymemorynearlimit)).
- Eine Panic: Melde sie mit den Logzeilen drumherum.
- Das Log sagt, dass die Datenbank zu einem anderen `host` oder `account_domain` gehört: Setze die aufgezeichneten Werte in der Konfiguration oder richte `database_url` auf die richtige Datenbank. Diese Werte lassen sich nicht ändern ([Einstellungen, die du später nicht mehr ändern kannst](production-checklist.md#settings-you-cannot-change-later)).

<!-- source: mymemorynearlimit 438190b311c9 -->
### MyMemoryNearLimit

Die Go-Runtime nutzt mehr als 90 % von `GOMEMLIMIT`. Nahe am Limit läuft der Garbage Collector viel häufiger, was CPU und Latenz kostet,
und darüber hinaus kann die Plattform den Prozess beenden. Dieser Alarm braucht `GOMEMLIMIT`; ohne es beobachte stattdessen die Speichermetriken des Containers.

**Prüfe** `http_server_active_requests` und die offenen Streams (`my_stream_connections`) und ob der Speicher weiter wächst, obwohl die Last gleich bleibt (ein Leck).

**Behebe** es, indem du den Speicher des Containers und `GOMEMLIMIT` mit ihm erhöhst oder mehr Prozesse startest. Melde ein stetiges Wachstum, das nicht der Last folgt.

<!-- source: mydbpoolsaturated a737e35a86f5 -->
### MyDBPoolSaturated

Mehr als 90 % der Datenbankverbindungen eines Prozesses sind in Benutzung. Anfragen warten bald auf eine Verbindung (`MyDBPoolWaiting`).

**Prüfe**, ob der Traffic gewachsen ist oder Abfragen langsamer wurden (das Slow-Query-Log der Datenbank, `my_adapter_operation_duration_seconds`).

Beheben:

- Erhöhe die Pool-Größe in `database_url` (bei PostgreSQL `pool_max_conns`) und halte die Summe aller Prozesse unter dem Verbindungslimit der Datenbank.
- Setze einen Connection-Pooler vor die Datenbank, wenn viele Prozesse sie teilen.

<!-- source: mydbpoolwaiting eacb1d75cd54 -->
### MyDBPoolWaiting

Arbeit wartet auf eine freie Datenbankverbindung oder hat das Warten nach [`database_acquire_timeout`](../reference/configuration.md#database_acquire_timeout) aufgegeben (Timeouts gibt es nur bei PostgreSQL): Anfragen antworten mit `503`, und Jobs laufen später erneut. Folge [MyDBPoolSaturated](#mydbpoolsaturated);
prüfe außerdem, dass die Datenbank selbst nicht an ihrem Verbindungslimit ist.

<!-- source: mydatabasestoragelarge edf767115918 -->
### MyDatabaseStorageLarge

Die Datenbank beansprucht mehr, als du eingeplant hast. `my_db_size_bytes` ist die Größe der PostgreSQL-Datenbank (`pg_database_size`, was die Speicherlimits der Anbieter zählen)
oder der SQLite-Datei (ihr Write-Ahead-Log ausgenommen). Das ist Wachstum, kein Ausfall. Setze den Schwellenwert in der Regel einen Puffer unter das Speicherlimit
deines Datenbanktarifs oder des Volumes, das die SQLite-Datei hält, das auch Platz für das Write-Ahead-Log und für Backups behalten muss.

Beheben:

- Erhöhe den Tarif oder das Volume.
- Entfernte Beiträge werden nach der Servereinstellung `remote_status_retention_days` entfernt; ein niedrigerer Wert verlangsamt das Wachstum.
- Gelöschte Zeilen geben Platz frei, den beide Datenbanken wiederverwenden, aber die Größe sinkt selten: PostgreSQL behält den Platz seiner Tabellen, und SQLite behält die freien
  Seiten in der Datei bis zu einem `VACUUM` (mit `sqlite3` ausführen, während der Server angehalten ist).

<!-- source: myfiledescriptorsnearlimit 8a3112c11968 -->
### MyFileDescriptorsNearLimit

Der Prozess hat mehr als 80 % seines Limits an Dateideskriptoren offen. Darüber schlagen neue Verbindungen fehl.

**Behebe** es, indem du das Limit erhöhst (`ulimit -n`, das `nofile` der Container-Runtime) oder mehr Prozesse startest. Prüfe `my_stream_connections` und
`http_client_open_connections`, was sie festhält.

<!-- source: myadaptererrors fcf1d91a0f01 -->
### MyAdapterErrors

Mehr als 5 % der Operationen eines Adapters schlagen fehl: `kv.valkey` (Valkey), `kv.db` (die KV-Tabelle), `blob.fs` (das Medienverzeichnis),
`queue.db` (die Queue-Tabelle) oder ein `stream.<backend>` (die Benachrichtigungen zwischen Prozessen).

**Prüfe** die Fehlerzeilen dieses Adapters und den Dienst dahinter: Valkey läuft und ist erreichbar, das Medien-Volume ist eingehängt, beschreibbar und nicht voll.

**Behebe** den Dienst. Fehlgeschlagene Job-Operationen werden wiederholt; fehlgeschlagene Stream-Benachrichtigungen verzögern nur Live-Aktualisierungen.

<!-- source: myremotemediacacheoverlimit dea5d39a7027 -->
### MyRemoteMediaCacheOverLimit

Die Kopien entfernter Medien liegen dauerhaft über `remote_media_cache_max_bytes`. Die stündliche Bereinigung entfernt normalerweise abgelaufene und am längsten nicht benutzte Kopien.
Setze den Schwellenwert in der Regel auf deinen Wert von `remote_media_cache_max_bytes`.

**Prüfe** den Job-Bildschirm und die Logs auf Fehlschläge der Bereinigungs-Jobs und `MyAdapterErrors` auf den Blob-Speicher.

**Behebe** die Ursache der Fehlschläge oder senke die Cache-Größe oder die Richtlinie des Servers für entfernte Medien.

<!-- source: mymediastoragelarge ff5e5906636c -->
### MyMediaStorageLarge

Die Mediendateien beanspruchen mehr, als du eingeplant hast. Das ist Wachstum, kein Ausfall. Setze den Schwellenwert in der Regel auf das, was dein Speichertarif oder Budget erlaubt.

**Behebe** es, indem du den Tarif oder das Budget erhöhst oder die Upload-Limits (`media_max_image_bytes`, `media_max_av_bytes`) senkst.

<!-- source: myemailfailing f7a0c64c5c60 -->
### MyEmailFailing

E-Mail-Jobs sind in der letzten Stunde endgültig fehlgeschlagen: Verifizierung, Zurücksetzen des Passworts, Passwort-Hinweise, Benachrichtigungen oder die E-Mails zur Schließung.
Nutzer, die auf eine Verifizierungs- oder Zurücksetzungs-E-Mail warten, kommen nicht weiter.

**Prüfe** den Job-Bildschirm, gefiltert auf die E-Mail-Job-Typen (sie enden auf `_email`), und die letzten Fehler. [E-Mails versenden](email.md#look-at-send-failures)
listet die Fehler auf und was sie bedeuten.

Beheben:

- Falscher SMTP-Host, Port oder falsche Zugangsdaten oder ein Problem mit SES-Konto oder -Berechtigung: Behebe es und führe die fehlgeschlagenen Jobs dann erneut aus.
  Verifizierungs- und Zurücksetzungs-E-Mails, die älter sind als die Lebensdauer ihres Links, werden bei einer Wiederholung ohne Versand abgeschlossen; bitte diese Nutzer also, eine neue anzufordern.
- Dauerhaft abgelehnte Adressen werden unterdrückt; nichts zu tun.

<!-- source: myemailretrying 40d3ed1a76f5 -->
### MyEmailRetrying

Die meisten E-Mail-Versände schlagen fehl und warten auf eine Wiederholung: Der Mailserver ist nicht erreichbar, lehnt die Anmeldung ab oder begrenzt die Rate. Wiederholungen laufen etwa 15 Minuten lang bei
den transaktionalen E-Mails und etwa 4 Stunden bei Benachrichtigungen, bevor sie endgültig fehlschlagen. Folge [MyEmailFailing](#myemailfailing), bevor das passiert.

<!-- source: cost-and-budget 5a63bdd2754d -->
### Kosten und Budget

Der Server alarmiert nicht bei Kosten. Lege beim Anbieter, der die Kosten als Erster sieht, ein Budget mit Alarmen an:

- AWS: ein Kostenbudget von AWS Budgets mit Alarmen bei zum Beispiel 80 % und 100 % des monatlichen Betrags, gefiltert nach dem Cost-Allocation-Tag dieses Servers.
- Google Cloud: ein Cloud-Billing-Budget für das Projekt mit Alarmschwellen bei 50 %, 90 % und 100 %. Die Benachrichtigungen desselben Budgets können den
  Kostenimport speisen ([Nutzungskosten der Cloud importieren](billing-import.md)).

Die importierten Kosten und die Schätzung für den laufenden Monat erscheinen unter Administration → Kosten → Billing-Importe. Fehlgeschlagene Importe werden dort festgehalten und vom nächsten täglichen Import wiederholt;
[`MyBillingImportFailing`](#mybillingimportfailing) sagt dir Bescheid, bevor die Zahlen des Monats veralten.

<!-- source: mybillingimportfailing da997f400ac4 -->
### MyBillingImportFailing

Der neueste Import der Nutzungskosten einer Billing-Quelle ist fehlgeschlagen, und seit 6 Stunden war kein späterer Import erfolgreich. Importe laufen einmal am Tag; Drosselung und
Nichtverfügbarkeit werden innerhalb von Stunden wiederholt, die anderen Fehler warten auf den Import des nächsten Tages, daher zeigt der Kostenbildschirm bis dahin ältere Zahlen.

Prüfen:

- Administration → Kosten → Billing-Importe: den letzten Import der Quelle und seinen Fehler.
- `my_billing_imports_total{my_outcome="failed"}` nach `error_type` oder die `WARN`-Logzeile, deren `msg` `billing import failed` ist, mit `source` und
  `error_kind`. [Nutzungskosten der Cloud importieren](billing-import.md) listet die Arten auf und wie man jede behebt.

Beheben:

- Behebe die Ursache (Berechtigungen, der Billing-Export, das Anfragelimit) und drücke dann „Jetzt importieren“ unter Administration → Kosten → Billing-Importe, statt auf den nächsten Tag zu warten.

Ohne Prometheus (Cloud Run mit `metrics_exporter = none`) lege stattdessen einen logbasierten Alarm auf die Zeile `billing import failed` an.

<!-- source: telling-users-about-an-incident e8cb0ca13fe8 -->
## Nutzer über einen Vorfall informieren

1. **Bestätige** den Alarm, damit andere in Bereitschaft wissen, dass sich jemand darum kümmert.
2. **Veröffentliche auf der Statusseite** (`status_page_url`, siehe [Den Server von außen überwachen](monitoring.md#a-status-page)), sobald Nutzer betroffen sind:
   was kaputt ist, seit wann und wann du wieder etwas postest. Die Statusseite bleibt erreichbar, wenn dieser Server es nicht ist, und der Web-Client verlinkt auf sie.
3. **Veröffentliche eine Ankündigung**, wenn Nutzer den Server noch erreichen (eingeschränkt, nicht ausgefallen): Administration → Ankündigungen, den Text schreiben und veröffentlichen.
   Ankündigungen erscheinen über der Start-Timeline aller. Nutze stattdessen den Wartungsmodus mit einer `maintenance_message`, wenn der Server anhalten muss.
4. **Aktualisiere** beides zu dem Zeitpunkt, den du versprochen hast, auch wenn sich nichts geändert hat.
5. **Schließe ab**: Markiere den Vorfall auf der Statusseite als behoben, ziehe die Ankündigung zurück (oder veröffentliche eine kurze Folgemeldung) und schreibe auf, was passiert ist
   und was du geändert hast, solange es frisch ist.
