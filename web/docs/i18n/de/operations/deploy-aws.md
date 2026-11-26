<!-- source: deploying-on-aws d7e8643cd93d -->
# Auf AWS bereitstellen

So betreibst du `my` auf AWS mit dem OpenTofu-Modul [`mycroblog-aws`](../../deploy/opentofu/README.md#aws): ECS auf Fargate hinter
einem Application Load Balancer, die Jobs in der Datenbank, S3, SNS und E-Mail über Amazon SES. Nichts skaliert auf null: Rechne im Leerlauf mit
etwa 42 $ im Monat vor der Datenbank und lies [Kosten](#costs).

**Nichts davon wurde je auf AWS angewendet.** Die Prüfungen planen es mit gemockten Providern und ohne Zugangsdaten, was zeigt, dass
die Konfiguration stimmig ist, nicht, dass AWS sie akzeptiert oder der Server startet. Behandle eine erste Bereitstellung als Test.

Was jede OpenTofu-Bereitstellung gemeinsam hat, darunter die Eingaben des Moduls, seine Secrets, die Konsole und Upgrades, steht in
[Mit OpenTofu bereitstellen](deploy-opentofu.md). Diese Seite ergänzt, was AWS braucht.

<!-- source: what-it-creates 1ca642b05ab0 -->
## Was es anlegt

Ein Block von `mycroblog-aws` legt all das an. Jede Zeile ist ein Abschnitt des Moduls, und ihr Standardwert ist die empfohlene
Einrichtung; [die Referenz des Moduls](../../deploy/opentofu/README.md#sections-of-mycroblog-aws) listet jedes Feld auf.

| Teil | Abschnitt | Standard | Andere Arten |
| --- | --- | --- | --- |
| Server | `service`, `jobs` | Ein ECS-Cluster und -Service auf Fargate mit `role = all`, Task-Definitionen für die Migration und, solange du sie brauchst, die Konsole sowie eine Log-Gruppe `/ecs/<name>` | |
| Netzwerk | `network` | Ein VPC über zwei oder mehr Zonen, mit je einem öffentlichen und einem privaten Subnetz | Ein eigenes VPC: [Netzwerke](#networks) |
| Traffic | `ingress` | Ein Application Load Balancer, immer vorhanden, mit deinem Zertifikat | CloudFront oder Cloudflare davor: [Wie der Traffic den Dienst erreicht](#how-traffic-reaches-the-service) |
| Jobs | | Keine: Die Tasks bleiben oben und nehmen die Jobs aus der Datenbank | |
| Datenbank | `database` | Ein Secret für die URL eines PostgreSQL, das du betreibst, etwa Neon | Amazon RDS: [Die Datenbank wählen](#2-choose-the-database) |
| Medien | `media` | Ein privater S3-Bucket | Cloudflare R2 oder jeder S3-kompatible Speicher |
| Caches und Rate Limits | `kv` | Keine: Die Datenbank hält sie | ElastiCache Serverless für Valkey oder ein Valkey, das du betreibst |
| Echtzeit-Aktualisierungen | `stream` | Ein SNS-Topic. Der Server legt seine eigenen SQS-Queues an und entfernt sie | Die Datenbank oder das Valkey von `kv` |
| E-Mail | `mail` | Die SES-Identität der Domain von `mail.from`: [E-Mail über Amazon SES](#email-through-amazon-ses) | Jeder SMTP-Dienst |
| Secrets | | Secrets Manager: ein erzeugter VAPID-Schlüssel und die Secrets, die nur du füllen kannst, leer | |
| KEK | `kek` | Ein erzeugtes Secret in Secrets Manager | Ein Schlüssel in AWS KMS, der es nie verlässt |
| Metriken, Traces, Fehler | `observability` | Metriken und Fehlerberichte in CloudWatch, Traces an X-Ray über einen Collector in jedem Task: [Traces zu X-Ray](#traces-to-x-ray) | Deine eigenen Exporter, über `settings` |
| Alarme | `monitoring` | Ein CloudWatch-Synthetics-Canary und Alarme: [Monitoring und Alarme](#monitoring-and-alerts) | Keine, mit `null` |

`translation = {}` fügt Amazon Translate hinzu, `search = { domain_name = "..." }` eine Domain von Amazon OpenSearch Service, die du
angelegt hast, und `billing = {}` den Import der Nutzungskosten aus Cost Explorer: [Beiträge übersetzen](translation.md#amazon-translate),
[Suchmaschinen](search-engines.md) und [Cloud-Nutzungskosten importieren](billing-import.md).

<!-- source: before-you-start 9d5614a650e1 -->
## Bevor du anfängst

- Was [die erste Bereitstellung](deploy-opentofu.md#first-deployment) braucht, mit `aws` als CLI der Plattform.
- Ein AWS-Konto, nur für diese Instanz verwendet, und Zugangsdaten dafür, die all das Obige anlegen dürfen, etwa
  `AdministratorAccess`.
- Eine Region. SES sendet aus ihr, und das Modul nimmt sie vom Provider `aws`.
- Empfohlen: eine öffentliche Hosted Zone von `host` in Route 53, in der das Modul dann den Eintrag von `host` anlegt, auch an einem
  Zone Apex.

<!-- source: first-deployment 26b562a3db03 -->
## Erste Bereitstellung

Mache die ersten drei Schritte hier und folge dann [der ersten Bereitstellung](deploy-opentofu.md#first-deployment) mit den Hinweisen des
vierten.

<!-- source: 1-request-the-certificates 3b26d03ba06c -->
### 1. Die Zertifikate anfordern

Der Load Balancer braucht ein validiertes Zertifikat für `host` in AWS Certificate Manager, in der Region. Das Modul nimmt es als
ARN, da die Validierung innerhalb des Apply das Apply anhalten würde, bis du Einträge hinzufügst, die nur das Apply ausgibt.

```sh
aws acm request-certificate --region ap-northeast-1 --domain-name social.example.org --validation-method DNS
aws acm describe-certificate --region ap-northeast-1 --certificate-arn "$CERTIFICATE_ARN" \
  --query 'Certificate.DomainValidationOptions[].ResourceRecord'
# Add that CNAME record at your DNS provider, then:
aws acm wait certificate-validated --region ap-northeast-1 --certificate-arn "$CERTIFICATE_ARN"
```

Fordere mit `ingress.kind = cloudfront` ein zweites in `us-east-1` an, der einzigen Region, aus der CloudFront eines nimmt; in `us-east-1`
bedient ein Zertifikat beide. Mit `ingress.media_host` muss das Zertifikat auch diesen Host abdecken.

<!-- source: 2-choose-the-database d9cc7b8b2210 -->
### 2. Die Datenbank wählen

`database.kind` wählt sie. In beiden Fällen fragen die Tasks die Datenbank etwa einmal pro Sekunde nach Jobs ab, sie ruht also nie.

**`external`, der Standard und die empfohlene Art,** liest die URL eines PostgreSQL, das du betreibst, etwa [Neon](https://neon.com),
aus einem Secret, das du füllst. Das Modul legt die Datenbank nie an, sodass ihr Passwort aus dem OpenTofu-State herausbleibt. Lege bei Neon
ein Projekt in einer Region nahe deiner an und kopiere seine Verbindungs-Strings:

- Der **direkte** (der Host ohne `-pooler`) ist Pflicht, im Secret `<name>-database-url`. Die Migration und die
  Konsole verwenden immer ihn: `my schema apply` hält eine Sperre auf Sitzungsebene, die ein Pooler im Transaktionsmodus nicht hält.
- Der **gepoolte** (der Host mit `-pooler`) ist optional, für den Dienst: Setze `database = { kind = "external", pooled = true }`,
  und das Modul legt das Secret `<name>-database-url-pooled` an. Siehe
  [Über einen Connection Pooler](database.md#through-a-connection-pooler).

Rechne mit Neons Launch-Tarif: Da die Compute nie anhält, sind die Compute-Stunden des Free-Tarifs innerhalb des Monats aufgebraucht.

**`rds`** legt Amazon RDS für PostgreSQL in den privaten Subnetzen an, mit einem Passwort, das das Modul erzeugt und selbst in
`<name>-database-url` schreibt, sodass nichts zu füllen ist:

```hcl
  database = { kind = "rds" }
```

Standardmäßig ist es `db.t4g.micro` mit 20 GB (`instance_class`, `allocated_storage_gb`), behält 7 Tage automatische Backups und
hat Löschschutz. Es hat keine öffentliche Adresse, sodass nur die Tasks es erreichen: Ein `pg_dump` muss aus dem VPC heraus laufen, und
vor einem Upgrade tritt ein Snapshot an seine Stelle,
`aws rds create-db-snapshot --db-instance-identifier my --db-snapshot-identifier my-before-upgrade`. Es hat keine gepoolte URL.

<!-- source: 3-write-the-block db167b7d9566 -->
### 3. Den Block schreiben

`examples/aws-ecs-neon` ist ein Weg zum Start: der Provider `aws` und ein Block mit den Standardwerten, dessen Variablen
`terraform.tfvars.example` auflistet. Womit du auch beginnst, der Block braucht diese:

```hcl
provider "aws" {
  region = "ap-northeast-1"
}

module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-aws?ref=v0.1.0&depth=1"

  host       = "social.example.org"
  network    = { egress = "public-ip", availability_zones = ["ap-northeast-1a", "ap-northeast-1c"] }
  ingress    = { certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/00000000-0000-0000-0000-000000000000" }
  media      = { bucket_name = "social-example-org-media" }
  mail       = { from = "noreply@social.example.org" }
  monitoring = { notification_emails = ["ops@social.example.org"] }
}
```

- `network.egress` hat keinen Standardwert, da beide Wege nach Stunden abgerechnet werden: Lies [Netzwerke](#networks).
- `ingress.route53_zone_id` nennt die Hosted Zone von `host`, wenn du eine hast.
- `media.bucket_name` muss in ganz AWS eindeutig sein.
- `monitoring` nennt, wen die Alarme benachrichtigen, oder ist `null`.

Füge den Abschnitt der Datenbank hinzu, die du gewählt hast, und jeden anderen, den du änderst.

<!-- source: 4-deploy 891c5c7cef83 -->
### 4. Bereitstellen

Folge [der ersten Bereitstellung](deploy-opentofu.md#first-deployment). Auf AWS, wo die Befehle dieser Seite den Standard-`name`
`my` verwenden:

- **Secrets**: Sie liegen in Secrets Manager, benannt `<name>-...`. `tofu output kek` hat den Befehl, der den KEK kopiert, und
  `aws secretsmanager get-secret-value --secret-id my-vapid-key --query SecretString --output text` kopiert den VAPID-Schlüssel. Der
  KEK erreicht den Server als `MY_KEK`, da Fargate keine Datei einhängt. Ein Task liest seine Secrets beim Start: Nachdem du
  eines geändert hast, führe `aws ecs update-service --cluster my --service my --force-new-deployment` aus, und dasselbe für jeden Worker,
  `my-<key>`.
- **Apply**: Zuerst kommt die Task-Definition der Migration mit allem, wovon sie abhängt, und die Dienste erst, nachdem die Migration
  erfolgreich war. Die Prüfung des Load Balancers fragt `/readyz`, was mit `503` antwortet, bis das Schema existiert.
- **DNS**: `tofu output dns_records` listet den Eintrag von `host` auf, es sei denn, Route 53 oder Cloudflare legt ihn an, und die drei DKIM-
  Einträge von SES. An einem Zone Apex ohne Route 53 braucht der Eintrag von `host`, was dein DNS-Anbieter `ALIAS` oder
  `ANAME` nennt, wie `tofu output dns_note` sagt.
- **E-Mail**: SES sendet nur an Adressen, die du verifiziert hast, bis AWS den Produktionszugriff gewährt, sodass sich noch niemand sonst
  registrieren kann: Lies [E-Mail über Amazon SES](#email-through-amazon-ses).
- **Alarme**: Jede Adresse von `monitoring.notification_emails` erhält von SNS eine Abo-E-Mail, die sie bestätigen muss.
- **Der erste Administrator**: Der Link bleibt in CloudWatch Logs, in `/ecs/<name>`, wo ihn jeder verwenden kann, der die Logs des Kontos
  lesen kann.

<!-- source: the-console-and-upgrades-on-aws 0e4bc0bdeff2 -->
## Die Konsole und Upgrades auf AWS

[Die Konsole](deploy-opentofu.md#the-console) und [Upgrades](deploy-opentofu.md#upgrading) führen einmalige Fargate-Tasks von dort aus, wo
du anwendest, über `aws` aus und lesen ihre Ausgabe aus CloudWatch Logs. Ein unterbrochenes Apply lässt den Task weiterlaufen, und das nächste
Apply wartet, bis er endet. Bei einem Upgrade bekommt jede Task-Definition eine neue Revision mit dem Image, die nichts ausführt, bis ein
Dienst sie übernimmt; jeder Dienst wartet dann, bis seine neuen Tasks gestartet sind, und beim Standarddienst, bis sie die Prüfung des Load Balancers
bestehen, bevor die alten gehen, und der Deployment Circuit Breaker setzt eine Revision, deren Tasks nicht starten, auf die
vorherige zurück.

Die Zugangsdaten dort, wo du anwendest, oder die des CI-Jobs, der anwendet, brauchen neben dem, was das Apply selbst braucht:

- für die Migration: `ecs:RunTask`, `ecs:DescribeTasks`, `ecs:ListTasks`, `logs:FilterLogEvents` und `iam:PassRole` auf den
  beiden Rollen der Tasks;
- für die Konsole außerdem: `ecs:DescribeServices`, `ecs:DescribeTaskDefinition`, `ecs:ListTagsForResource` und
  `ecs:RegisterTaskDefinition`, da die Konsole zuerst das Image des laufenden Dienstes nimmt.

Um [zuerst das SQL zu prüfen](deploy-opentofu.md#checking-the-sql-first), schalte die Konsole ein, solange der Block noch das
aktuelle Release nennt, erhöhe dann `ref`, wende allein die Task-Definition der Migration an und führe damit `my upgrade -dry-run` aus:

```sh
tofu apply -var console_enabled=true
# Name the new release in main.tf, then:
tofu init
tofu apply -var console_enabled=true -target='module.mycroblog.module.app.aws_ecs_task_definition.task["migrate"]'
tofu output -json console | jq -c '.task_definition = "my-migrate" | .log_prefix = "migrate" | del(.service)' |
  "$SCRIPTS/console.sh" upgrade -dry-run
```

<!-- source: monitoring-and-alerts 9b712c82cae2 -->
## Monitoring und Alarme

`monitoring` fügt einen CloudWatch-Synthetics-Canary für `/readyz`, Metriken in CloudWatch und Alarme mit den Namen aus
[dem Runbook](alerts.md#aws-ecs) hinzu, die die Adressen von `notification_emails` oder die SNS-Topics von
`notification_topic_arns` benachrichtigen. Die Eingabe hat keinen Standardwert: Nenne, wer benachrichtigt wird, oder setze `monitoring = null`, um all das wegzulassen.
Jeder Lauf des Canary wird abgerechnet, und `uptime_period_minutes`, standardmäßig 15, bestimmt, wie oft er läuft: Lies [Kosten](#costs).

<!-- source: choosing-the-backends b83753b1f860 -->
## Die Backends wählen

Jeder Abschnitt hat standardmäßig die empfohlene Art; [ändere die `kind` eines Abschnitts](deploy-opentofu.md#changing-a-backend) für
eine andere. Die Datenbank steht [oben](#2-choose-the-database).

| Abschnitt | Empfohlen | Andere Arten und wann |
| --- | --- | --- |
| `media` | `s3`, ein Bucket in der Region, mit ausgeschaltetem `versioning` | `r2` in `cloudflare.account_id` oder `s3-compatible` unter `endpoint`, jeweils mit einem Zugriffsschlüssel in Secrets, die du füllst. Um einen Bucket zu teilen, nimmt `existing_bucket = true` deinen, und `prefix` (`tenant-a/`) beginnt jeden Objektnamen; mit `s3` erreicht die Rolle der Tasks dann nur die Objekte darunter |
| `kv` | `db`, das nichts extra kostet | `elasticache`, das auch im Leerlauf abgerechnet wird und in den privaten Subnetzen liegt, oder `external`, eine Valkey-URL in einem Secret, das du füllst |
| `stream` | `sns`, das jeden Task erreicht | `postgres`, über die direkte Verbindung der Datenbank, oder `valkey`, das Valkey von `kv` |
| `mail` | `ses`, ohne Passwort | `smtp`, mit `smtp.host`, `smtp.port` (587) und `smtp.username`, dessen Passwort in ein Secret kommt, das du füllst |
| `kek` | `secrets-manager`, den du einmal kopierst | `kms`, ein Schlüssel, der AWS KMS nie verlässt. Auf einer Instanz mit Daten ist [ihn zu ändern](deploy-opentofu.md#the-kek) ein Umzug |
| `observability` | CloudWatch und X-Ray | `traces = "none"` oder `"settings"` für jedes Signal, das du anderswohin sendest, etwa an Sentry |

`architecture = "arm64"` führt die Tasks auf Graviton aus, was weniger kostet; das veröffentlichte Image hat beide.

<!-- source: email-through-amazon-ses 0d8bc9f93296 -->
## E-Mail über Amazon SES

`mail.kind = "ses"`, der Standard, sendet mit der SES-API in der Region der Bereitstellung, von der Domain von `mail.from`. Das
Modul legt die Identität der Domain mit Easy DKIM an und erlaubt den Tasks `ses:SendEmail` darauf, es gibt also kein Passwort, und
`mail.configuration_set` legt ein Configuration Set an und wendet es auf jede Nachricht an, oder nennt mit
`existing_configuration_set = true` eines, das sich Instanzen teilen; jede Instanz handelt dann nur bei den Benachrichtigungen zu den
Mails, die sie gesendet hat. Bevor SES an deine Nutzer sendet, füge die
Einträge hinzu, die `tofu output dns_records` auflistet, und verlasse die Sandbox:
[E-Mails über Amazon SES versenden](email-ses.md) hat die Schritte und sagt, wie der Server von Bounces und Beschwerden erfährt.

<!-- source: traces-to-x-ray e5ad1c35fdd5 -->
## Traces zu X-Ray

`observability.traces = "x-ray"` ist der Standard, weil der Collector ohne Zusatzkosten zur empfohlenen Einrichtung passt. Es ist der
AWS-Distro-for-OpenTelemetry-Collector, der in jedem Task neben dem Server läuft, nur auf dem Loopback des Tasks lauscht und
mit der Rolle der Tasks (`xray:PutTraceSegments`) an X-Ray schreibt. Er nimmt 32 CPU-Einheiten und 128 MiB aus jedem Task, den AWS
allein nach der Größe des Tasks abrechnet, sodass der Task des Standarddienstes mit 256 CPU-Einheiten und 1.024 MiB dasselbe kostet und dem
Server 224 CPU-Einheiten und 896 MiB gibt. Das standardmäßige `traces_sample_ratio`, 1 %, zeichnet bei 100.000
Anfragen pro Tag etwa 30.000 Traces im Monat auf, unter den 100.000 Traces im Monat, die X-Ray kostenlos aufzeichnet.

Was er kostet, ist das Image des Collectors, das bei jedem Start eines Tasks gezogen wird, und der Speicher des Servers. Setze `traces = "none"`, wenn
du die Traces nicht liest oder für einen Task, der zu klein ist, um 128 MiB zu entbehren: Der Plan lehnt eine Größe ab, die dem Server weniger als
128 CPU-Einheiten oder 512 MiB lässt, in `service`, `jobs` oder einem Worker. Der Collector ist nicht essenziell: Hält er an, macht der Server
ohne Traces weiter. Dass X-Ray seine Spans annimmt, ist nicht verifiziert.

<!-- source: networks 36dc33a6cbef -->
## Netzwerke

ECS braucht immer ein VPC. `network.kind = "vpc"`, die empfohlene Art, legt eines in `cidr_block` (`10.0.0.0/16`) über
`availability_zones` an, mindestens zwei, die der Load Balancer und RDS brauchen: ein öffentliches und ein privates Subnetz in jeder Zone und ein
Internet Gateway. Es ist nur IPv4, sodass der Server keinen reinen IPv6-Peer erreicht. `network.kind = "existing"` nimmt stattdessen ein VPC von dir:
`vpc_id`, `vpc_cidr`, `public_subnet_ids`, `public_subnet_cidrs` und `private_subnet_ids`.

Ein Task braucht einen Weg ins Internet, für das Image, die Datenbank, die Föderation und die AWS-APIs, und `network.egress` wählt ihn.
Eingehender Traffic erreicht einen Task in beiden Fällen nur über den Load Balancer:

| | `public-ip` (empfohlen) | `nat` |
| --- | --- | --- |
| Subnetze der Tasks | Öffentlich, mit einer öffentlichen IPv4-Adresse an jedem Task | Privat |
| Jede Stunde | 0,005 $ pro Adresse eines Tasks | Ein NAT Gateway, etwa 0,045 $, und seine Adresse |
| Jedes GB | Nichts für die Adresse | Etwa 0,045 $ durch das Gateway, Medien und Föderation eingeschlossen |
| Mit `existing` | Nichts mehr | Deine privaten Subnetze müssen das Internet bereits erreichen |

Nutze `public-ip`, es sei denn, eine Richtlinie von dir verbietet öffentliche Subnetze: Es kostet weit weniger und ist nicht offener. `vpc` legt ein NAT
Gateway an, in der ersten Zone: Fällt diese Zone aus, verlieren die privaten Subnetze das Internet, bis sie zurückkehrt. RDS und
ElastiCache liegen bei beiden in den privaten Subnetzen und brauchen keinen Weg nach draußen.

<!-- source: how-traffic-reaches-the-service d009efcf3ee2 -->
## Wie der Traffic den Dienst erreicht

Der Load Balancer ist immer da, mit dem Zertifikat von `ingress.certificate_arn`, und die Tasks lassen nur ihn zu.
`ingress.kind` entscheidet, was davor steht:

| | `load-balancer` (empfohlen) | `cloudfront` | `cloudflare` |
| --- | --- | --- | --- |
| `host` zeigt auf | Den Load Balancer | Eine CloudFront-Distribution | Cloudflare, das sich mit dem Load Balancer verbindet |
| Gecacht | Nichts | `/media/`, nach dem `Cache-Control` des Servers | Nichts auf `host`; `media_host` fügt einen gecachten Host hinzu |
| Einträge | Route 53 legt sie mit `route53_zone_id` an, oder `dns_records` listet sie auf | Route 53 mit `route53_zone_id` oder `dns_records` | Cloudflare legt ihn an |
| Braucht | | `cloudfront_certificate_arn`, in `us-east-1`. `cloudfront_price_class` wählt die Edge-Standorte | `cloudflare = { zone_id = "..." }` und das Token in `CLOUDFLARE_API_TOKEN` |
| Schließt die Origin durch | Die Security Group der Tasks | Die Security Group des Load Balancers lässt nur CloudFront zu, und sein Listener lehnt eine Anfrage ohne einen Header ab, den CloudFront sendet | Einen Header, den die Regel von Cloudflare hinzufügt und der Server prüft |

Hinter CloudFront oder Cloudflare landet der Wert des Headers im OpenTofu-State, der privat bleiben muss. Ein Eintrag des
Load Balancers von vor CloudFront muss zuerst weg: Ein bestehender Eintrag desselben Namens lässt das Apply fehlschlagen. Die
Adressen von CloudFront kommen beim Planen von AWS, wende also ab und zu an, sonst teilt sich ein Edge an einer neuen Adresse die Rate Limits seiner Betrachter.

`ingress.media_host` fügt einen gecachten Medien-Host auf Cloudflare hinzu, neben `load-balancer` oder `cloudflare`, mit
`cloudflare.zone_id`. Eine gelöschte Datei bleibt gecacht, bis du sie purgst: Lies [Medien über ein CDN ausliefern](cdn.md). Schalte mit Cloudflare
vor irgendetwas den Bot Fight Mode in seinem Dashboard aus, da eine Challenge auf `/ap/`, `/.well-known/` oder `/api/` die
Föderation bricht, und das Modul ihn nicht verwaltet. Die Cloudflare-Arten liefen nur auf gemockten Providern.

<!-- source: limits 42ed50837851 -->
## Limits

- **Health**: Die einzige Prüfung des Load Balancers fragt `/readyz`, was auch fehlschlägt, solange die Datenbank ausgefallen ist: Nach 2,5 Minuten
  Fehlschlägen ersetzt ECS den Task und tut das weiter, bis die Datenbank zurück ist. Es gibt keine Container-Prüfung, sodass ein Worker,
  der keinen Load Balancer hat, bei einem fehlerhaften Image nur in seinen Logs scheitert.
- **Uploads**: Der Load Balancer setzt kein Limit für den Body einer Anfrage, daher entscheiden die Einstellungen `media_max_*` des Servers.
- **Streams**: Ein Stream sendet alle 30 Sekunden einen Heartbeat, unter dem Idle-Timeout des Load Balancers von 60. Er bleibt offen, bis
  sein Task geht, und der Client verbindet sich dann erneut.
- **Client-Adressen**: Die Rate Limits stützen sich darauf, was der Server vertraut, was das Modul aus dem Ingress setzt. Prüfe die Client-
  Adressen in den ersten Request-Logs: Eine Adresse aus den Subnetzen des Load Balancers oder von CloudFront heißt, dass es falsch ist.
- **Die Platte** geht mit dem Task, daher lehnt der Plan SQLite und Medien im Dateisystem ab.
- **Secrets** unter einem vom Kunden verwalteten KMS-Schlüssel brauchen auch `kms:Decrypt`, das das Modul nicht gewährt.

<!-- source: costs 19e0e43bc201 -->
## Kosten

Nichts skaliert auf ECS auf null: Der Task, der Load Balancer und seine öffentlichen Adressen werden den ganzen Monat abgerechnet. Diese Zahlen stammen aus
der Preisliste von AWS für Tokio, von 2026-10 und 2026-11, sofern eine Zeile nichts anderes sagt, und wurden nicht mit einer Rechnung abgeglichen.

| Teil | Pro Monat | Wahl |
| --- | --- | --- |
| Ein Task mit 0,25 vCPU und 1 GB | 13,26 $ auf x86, 10,61 $ auf arm64 | `service`, `architecture`. Jeder Worker fügt seine eigenen Tasks hinzu |
| Load Balancer | 17,74 $, plus 0 $ bis 5,84 $ für jede den ganzen Monat genutzte Kapazitätseinheit | Immer vorhanden |
| Die öffentlichen Adressen des Load Balancers | 7,30 $, für zwei Zonen | Eine pro Zone |
| Der Weg nach draußen | 3,65 $ pro Task mit `public-ip`; etwa 36,50 $ und 0,045 $ pro GB mit `nat`, zum Tarif von Ohio | `network.egress` |
| Datenbank | Etwa 19 $ im Launch-Tarif von Neon bei 0,25 CU, da die Compute nie anhält; etwa 21 $ mit `rds`, `db.t4g.micro` und 20 GB | `database` |
| Caches und Rate Limits | 0 $ mit `db`; mit `elasticache` etwa 6,13 $ oder laut einem anderen Dokument von AWS etwa 61 $, in N. Virginia | `kv` |
| Der Canary | Etwa 3,34 $ bei alle 15 Minuten; etwa 51,72 $ bei jede Minute | `monitoring.uptime_period_minutes` |
| Traces | 0 $ bis 100.000 Traces im Monat | `observability.traces` |

**Im Leerlauf etwa 42 $ mit `public-ip` und etwa 75 $ mit `nat`**, plus Kapazitätseinheiten, vor Datenbank, Speicher, Traffic
und Canary. Nicht erfasst und deshalb in keiner Zeile: CloudFront, Secrets Manager, CloudWatch Logs, S3, SNS und SQS, Daten aus AWS hinaus und jedes
Free Tier. Einmalige Tasks kosten ihre Größe für die Minuten, die sie laufen, und ein Upgrade führt ein paar Minuten lang einen zweiten Service-Task aus.
