<!-- source: deploying-on-cloud-run 7728d9a37c56 -->
# Auf Cloud Run bereitstellen

So betreibst du `my` auf Google Cloud mit dem OpenTofu-Modul [`mycroblog-gcp`](../../deploy/opentofu/README.md): Cloud Run, Cloud Tasks, Cloud Scheduler, Cloud Storage und Pub/Sub, mit einer PostgreSQL-Datenbank auf Neon, hinter einem globalen externen Application Load Balancer. Der Dienst betreibt keine Instanz, solange ihn niemand nutzt, abgesehen von einem Tick, der den Server jede Minute weckt. Ein Server kostet Geld: Rechne mit etwa 37 $ im Monat, vor Speicher, Traffic und offenen Streams, und lies [Kosten](#costs), was Sorgfalt senken kann.

Das Modul braucht OpenTofu 1.11 oder neuer. Terraform wird nicht unterstützt.

<!-- source: what-it-creates cae1e55a1722 -->
## Was erstellt wird

Ein Block von `mycroblog-gcp` erstellt all das. Jede Zeile ist ein Abschnitt des Moduls, und sein Standard ist das, was dieser Leitfaden bereitstellt; [die README des Moduls](../../deploy/opentofu/README.md#sections) listet die anderen Arten auf.

| Teil | Abschnitt | Ressourcen |
| --- | --- | --- |
| Server | `service`, `jobs` | Ein Cloud-Run-Dienst mit `role = all`, ein Migrations-Job, ein Konsolen-Job, solange du ihn brauchst, ein Dienstkonto und öffentlicher Zugriff |
| Traffic | `ingress` | Ein globaler externer Application Load Balancer mit Zertifikat. Ein Domain-Mapping oder Cloudflare sind die anderen Arten, und ein Medien-Host auf Cloudflare kann neben jeder stehen |
| Jobs | `queue` | Eine Cloud-Tasks-Queue, ein Cloud-Scheduler-Job und ein Dienstkonto, das den Server aufruft |
| Medien | `media` | Ein privater Bucket |
| Echtzeit-Aktualisierungen | `stream` | Ein Topic und eine nutzerdefinierte Rolle. Der Server legt seine eigenen Subscriptions an und entfernt sie wieder |
| Datenbank | `database` | Ein Secret für ihre URL: Du legst die Datenbank von Hand an |
| E-Mail | `mail` | Ein Secret für das SMTP-Passwort: ein beliebiger SMTP-Dienst |
| Secrets | | Ein generierter VAPID-Schlüssel und die Secrets, die nur du befüllen kannst, leer |
| KEK | `kek` | Ein generiertes Secret, das als Datei eingebunden wird. Cloud KMS ist die andere Art |
| Metriken, Traces, Fehler | `observability` | Die APIs von Cloud Monitoring, Cloud Trace und Error Reporting, die lesen, was Cloud Run ihnen sendet |

Secret-Werte gelangen nie in den OpenTofu-Plan oder -State: Die Werte, die von anderswo kommen, fügst du selbst hinzu, und das Modul generiert den KEK und den VAPID-Schlüssel über Write-only-Attribute.

<!-- source: before-you-start 10846ce6f3b1 -->
## Bevor du anfängst

- `tofu` 1.11 oder neuer, `gcloud` und `jq`, dort, wo du anwendest: Ein Apply, der die Version ändert, führt die Migration von dort aus über `gcloud` aus.
- Ein Google-Cloud-Projekt mit Abrechnung, das nur für diese Instanz genutzt wird, und ein Konto darin mit der Rolle Owner.
- Eine Domain für [`host`](../reference/configuration.md#host). Sie lässt sich später nicht mehr ändern: Lies [Einstellungen, die sich später nicht mehr ändern lassen](production-checklist.md#settings-you-cannot-change-later).
- Eine Version von `my` aus den [Releases](running.md#get-the-image). Es gibt kein `latest`-Tag.

<!-- source: first-deployment 996fcc6eb35e -->
## Erste Bereitstellung

<!-- source: 1-enable-the-apis c7f2ef4a9ee9 -->
### 1. Die APIs aktivieren

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com compute.googleapis.com \
  monitoring.googleapis.com
```

`compute.googleapis.com` ist für den Load Balancer.

<!-- source: 2-create-the-database-on-neon ca5f51e8979e -->
### 2. Die Datenbank auf Neon anlegen

Rechne mit dem Launch-Tarif: Mit dem Standard-Tick stoppt die Compute-Instanz nie. Der Free-Tarif funktioniert ebenfalls, mit den Abstrichen unter [Kosten](#costs).

Lege in der Neon-Konsole ein Projekt in einer Region nahe deiner Cloud-Run-Region an und kopiere seine Verbindungsstrings. Das Modul legt es nie an, damit sein Passwort aus dem OpenTofu-State herausbleibt. Ein Projekt hat zwei Verbindungsstrings, und die Bereitstellung verwendet jeden dort, wo er hingehört:

- Der **direkte** (der Host ohne `-pooler`) ist erforderlich. Der Migrations-Job und die Konsole verwenden ihn immer: `my schema apply` hält eine Sperre auf Sitzungsebene, die ein Pooler im Transaktionsmodus nicht aufrechterhält.
- Der **gepoolte** (der Host mit `-pooler`) ist optional, für den Dienst. Der Pooler von Neon verarbeitet, was der Server braucht: siehe [Über einen Connection-Pooler](database.md#through-a-connection-pooler). Setze `database = { pooled = true }`, und das Modul legt dafür ein zweites Secret an.

Ohne gepoolten String verwendet der Dienst ebenfalls den direkten, bis zum Verbindungslimit der Datenbank, das `service.max_instances` begrenzt. Um den Pool von seinem Standard von 4 pro Instanz anzuheben, für die 80 Anfragen, die eine Cloud-Run-Instanz gleichzeitig annimmt, füge `pool_max_conns` zu dem String hinzu, den der Dienst verwendet, zum Beispiel `postgres://...-pooler...neon.tech/neondb?sslmode=require&pool_max_conns=8`. Siehe [Auf eine Datenbankverbindung warten](running.md#waiting-for-a-database-connection).

<!-- source: 3-copy-the-example-and-set-the-variables 1b99f37cb3a7 -->
### 3. Das Beispiel kopieren und die Variablen setzen

Beginne mit einer Kopie in einem leeren Verzeichnis, mit der Provider-Lock-Datei, aus dem Release, das du bereitstellst:

```sh
VERSION=v0.1.0
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r my-source/deploy/opentofu/examples/gcp-cloud-run-neon/. my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

`main.tf` ist der `google`-Provider und ein Block, `module "mycroblog"`. **Setze `ref` auf dieses Release.** Der Block nennt ein Release mit `?ref=`, was die Kopie eines stabilen Releases bereits richtig hat. Das Server-Image ist dasselbe Release, `ref` allein nennt also, was läuft. Bei einem Prerelease setzt du beides selbst: `sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf` und `image = { version = "0.1.0-rc.4" }` im Block, die Version ohne ihr `v`, da das Modul eines Prereleases nicht von sich aus sein eigenes Image verwendet. Das Modul gibt es erst ab dem ersten Release, das es enthält, ein früherer Tag hat also kein Beispiel zum Kopieren. Behalte dieses Verzeichnis: Es enthält deinen State und deine Einstellungen.

Fülle `terraform.tfvars` aus. `mail_from`, `mail_smtp_host` und `mail_smtp_username` konfigurieren die [E-Mail](email.md); der Port ist 587, da Google Cloud den ausgehenden Port 25 blockiert. Um etwas anderes zu ändern, füge den Abschnitt zum Block in `main.tf` hinzu, etwa `ingress = { kind = "domain-mapping" }`: Jeder Abschnitt und was er ablehnt steht in [der README des Moduls](../../deploy/opentofu/README.md#sections).

`settings` nimmt jede andere Einstellung aus der [Konfigurationsreferenz](../reference/configuration.md) über ihren Schlüssel, und `secrets` die Schlüssel geheimer Einstellungen, von denen jede ein leeres Secret zum Befüllen bekommt. Ein Schlüssel, den ein Abschnitt oder das Modul setzt, lässt den Plan fehlschlagen, ebenso ein Schlüssel, den das Modul auf Cloud Run nicht annimmt, etwa `listen`; der Fehler sagt, warum. Entscheide dich jetzt für [`account_domain`](../reference/configuration.md#account_domain), als `settings = { account_domain = "example.org" }`, wenn Handles eine andere Domain verwenden.

**Der State** bleibt standardmäßig in diesem Verzeichnis. Er enthält keine Secret-Werte, ist aber die einzige Aufzeichnung dessen, was angelegt wurde: Bewahre ihn in einem Bucket mit einem `backend "gcs"`-Block auf, wenn mehr als eine Person das hier ausführt.

<!-- source: 4-create-the-secrets-and-add-their-values 51cddae33ec9 -->
### 4. Die Secrets anlegen und ihre Werte hinzufügen

Das legt die Secrets an und generiert den KEK und den VAPID-Schlüssel:

```sh
tofu init
tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secret_manager
```

`tofu init` holt außerdem die Skripte dieses Releases: `migrate.sh`, das ein Apply ausführt, und `console.sh`, das ein späterer Schritt verwendet:

```sh
SCRIPTS=.terraform/modules/mycroblog/deploy/opentofu
```

**Kopiere den KEK jetzt** und bewahre die Kopie getrennt von der Datenbank auf, zum Beispiel in einem Passwortmanager. Ohne ihn lässt sich ein [Backup](backup.md) nicht wiederherstellen, und die Schlüssel der Konten gehen verloren. `tofu output kek` hat den Befehl. Tu dasselbe mit dem VAPID-Schlüssel, dessen Verlust jedes Web-Push-Abonnement beendet:

```sh
gcloud secrets versions access latest --secret=my-kek --project="$PROJECT"
gcloud secrets versions access latest --secret=my-vapid-key --project="$PROJECT"
```

Füge dann die Werte hinzu, die von anderswo kommen. `tofu output secrets_to_fill` listet jedes Secret auf, das das Modul leer angelegt hat, mit dem, was hineingehört, und dem Befehl, der es hinzufügt. Eine Instanz, deren Secret sich nicht lesen lässt, startet nicht, also füge sie vor dem Dienst hinzu:

```sh
printf '%s' 'postgres://...' | gcloud secrets versions add my-database-url --project="$PROJECT" --data-file=- # the direct string
printf '%s' '<the SMTP password>' | gcloud secrets versions add my-smtp-password --project="$PROJECT" --data-file=-
```

Der KEK erreicht den Server als eingebundene Datei, benannt durch `MY_KEK_FILE`. Die anderen sind Umgebungsvariablen, die beim Start einer Instanz aufgelöst werden, sodass eine neue Version bei der nächsten Instanz wirksam wird: Stelle eine neue Revision bereit, um sicherzugehen.

**OpenTofu generiert den KEK einmal und nie wieder.** Das Secret und seine Version haben `prevent_destroy`, und die Versionsnummer, die einen neuen Wert schreiben würde, ist im Modul fest vorgegeben: Ein neuer zufälliger KEK macht alles unlesbar, was der alte verschlüsselt hat. Um ihn zu rotieren, folge [Den KEK rotieren](key-management.md#rotate-the-kek) und füge den neuen Wert mit `gcloud secrets versions add` hinzu. OpenTofu verfolgt nicht, was du dort hinzufügst.

Um den KEK von Anfang an in Cloud KMS zu halten, setze vor diesem Schritt `kek = { kind = "kms" }`, nimm `module.mycroblog.module.kek_kms` anstelle von `kek_secret_manager` als Ziel und aktiviere `cloudkms.googleapis.com`.

<!-- source: 5-apply 2076866c0f3b -->
### 5. Anwenden

```sh
tofu apply
```

Das legt alles andere an, in einer Reihenfolge, die der Startup-Probe des Dienstes braucht: Der Probe wartet auf `/health/ready`, das mit `503` antwortet, bis das Schema existiert. Zuerst kommt der Migrations-Job, mit dem, wovon er abhängt, wozu die Adresse des Load Balancers gehört, da die Einstellungen des Servers sie nennen. Dann führt das Apply `my upgrade` als Ausführung des Jobs aus, von deinem Rechner über `gcloud`, und wartet darauf: `my config check`, `my db upgrade -check`, `my schema apply` und `my db upgrade`, was beim ersten anhält, der fehlschlägt. Seine Ausgabe erscheint in der des Apply. Der erste Befehl, der die Datenbank verwendet, trägt `host` und `account_domain` darin ein. Erst dann legt das Apply die Dienste an.

Schlägt die Migration fehl, hält das Apply vor den Diensten an. Behebe die Ursache, oft ein Secret ohne Wert, und führe `tofu apply` erneut aus: Es führt die Migration erneut aus.

<!-- source: 6-point-the-domain-at-the-load-balancer c81cc51be2af -->
### 6. Die Domain auf den Load Balancer zeigen lassen

`tofu output dns_records` listet einen `A`-Record auf, von `host` auf die Adresse des Load Balancers. Füge ihn bei deinem DNS-Anbieter hinzu. Hat dieser Anbieter einen eigenen Proxy, lass ihn für diesen Record ausgeschaltet.

Das Zertifikat wird von Google verwaltet und wird erst aktiv, wenn DNS auf die Adresse zeigt, was eine Weile dauern kann. Bis dahin scheitert `https://{host}` mit einem TLS-Fehler, sodass Föderation und Browser den Server noch nicht erreichen können. Nichts anderes wartet darauf: Cloud Tasks und Cloud Scheduler rufen die Standard-`run.app`-URL des Dienstes auf, `https://<service>-<project number>.<region>.run.app`. **Deaktiviere die Standard-URL nicht**, sonst bleiben die Jobs stehen. Diese URL weist Anfragen aus dem Internet ab, und Cloud Run zählt diese beiden nur aus demselben Projekt und auf dieser URL als intern, nicht auf einer eigenen Domain.

Was du sonst noch wählen kannst, steht unter [Wie der Traffic den Dienst erreicht](#how-traffic-reaches-the-service).

<!-- source: 7-create-the-first-administrator 19137d6d01f0 -->
### 7. Den ersten Administrator anlegen

Cloud-Run-Jobs haben kein Terminal und keine Standardeingabe, daher gibt `my admin create` einen Link aus, über den das Passwort gesetzt wird, statt eines zu lesen. Schalte die [Konsole](deploy-opentofu.md#the-console) ein und führe aus:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

Öffne den Link sofort und setze das Passwort. Er funktioniert einmal, eine Stunde lang. **Bis dahin bleibt er in Cloud Logging**, wo ihn jeder verwenden kann, der die Logs des Projekts lesen darf. Läuft er ab, führe `my admin account password-reset-link @alice` über die Konsole aus. Schalte dann die Zwei-Faktor-Authentifizierung ein.

<!-- source: 8-go-through-the-checklist e7fe33d10e80 -->
### 8. Die Checkliste durchgehen

Gehe die [Produktions-Checkliste](production-checklist.md) durch, beginnend mit der [E-Mail](production-checklist.md#email): SPF, DKIM und DMARC auf der Domain von `mail_from`.

<!-- source: the-console 52df6f8c43ac -->
## Die Konsole

Die Konsole führt einen `my`-Befehl mit dem Image, den Einstellungen und den Secrets des Dienstes aus. Sie existiert nur, solange `console_enabled` eingeschaltet ist, schalte sie also aus, wenn du fertig bist:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` wartet auf den Befehl, gibt seine Ausgabe und seinen Exit-Code aus und beendet sich mit diesem Code. Es braucht `roles/logging.viewer`, weil die Ausgabe eines Jobs in Cloud Logging landet. Es verwendet das Image, das der Dienst ausführt: Nach einem Upgrade, das bei der Migration anhielt, ist das das vorherige Release, zu dem die Datenbank noch passt.

Der Befehl hat keine Standardeingabe und kein Terminal, daher lässt sich ein Befehl, der ein Passwort oder eine Datei von dort liest, so nicht ausführen. Was er ausgibt, bleibt in den Logs, führe also keinen Befehl aus, dessen Ausgabe du nicht dort hinschreiben würdest.

<!-- source: upgrading fe1bed77d3af -->
## Upgrade

Lies zuerst [Upgrade](database.md#upgrading) und die Release Notes und gehe jeweils eine Minor-Version weiter. Ein Upgrade ist ein Apply des neuen Releases: Der Plan zeigt jeden Dienst und jeden Job, der sein Image übernimmt, und das Apply führt die Migration aus, bevor einer von ihnen es ausliefert.

```sh
# 1. Back up the database while nothing writes: see "The maintenance mode" below, and Backup and restore.
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Name the new release in main.tf, as ?ref=v0.2.0, and fetch it.
tofu init

# 3. Read the plan, then apply it.
tofu plan
tofu apply
```

Das Image ist das Release, das `ref` nennt, die Änderung von `ref` ist also das ganze Upgrade. Lies den Plan, bevor du ihn anwendest: Das Modul eines neuen Releases kann auch Ressourcen ändern. Das Apply tut Folgendes und hält beim ersten Schritt an, der fehlschlägt:

1. Gibt dem Migrations- und dem Konsolen-Job das neue Image, mit dem Rest des Plans, von dem die Dienste nicht abhängen.
2. Führt `my upgrade` als Ausführung des Migrations-Jobs aus, von dort, wo du anwendest, und wartet darauf: `my config check`, `my db upgrade -check`, `my schema apply` und `my db upgrade`. Seine Ausgabe erscheint in der des Apply.
3. Stellt die neue Revision jedes Workers bereit, dann die des Dienstes. Eine Revision nimmt Traffic erst an, wenn ihr Startup-Probe auf `/health/ready` besteht.

**Dort, wo du anwendest**, braucht es `gcloud` und `jq` sowie Zugangsdaten, die den Job ausführen und seine Logs lesen dürfen (`roles/run.developer` und `roles/logging.viewer`, die Owner einschließt). Dasselbe gilt für einen CI-Job, der anwendet.

**Wenn ein Schritt fehlschlägt**, bleiben die Dienste auf ihren Revisionen, und die Release Notes oder die Ausgabe der Migration sagen, warum. Behebe die Ursache und führe `tofu apply` erneut aus, mit derselben Version: Es führt die Migration erneut aus, und die abgeschlossenen Schritte finden nichts mehr zu tun. Ein Apply, das unterbrochen wurde oder seine Verbindung verlor, lässt die Ausführung auf Cloud Run weiterlaufen; das nächste Apply wartet, bis sie endet, bevor es seine eigene startet. Um nach einer Schemaänderung zurückzugehen, stelle das Backup wieder her: siehe [Zurückgehen](database.md#rolling-back). Ein Apply mit einer früheren Version hält bei der Migration an, da dieses Release ein neueres Schema ablehnt, und ändert keinen Dienst.

Um einen Release Candidate auszuprobieren oder das Image eines früheren Releases unter einem neueren Modul zu behalten, setze die Version selbst; es ist der Image-Tag, ohne das `v` des Releases:

```hcl
  image = { version = "0.2.0-rc.1" }
```

<!-- source: checking-the-sql-first 9ff10080c36d -->
### Das SQL zuerst prüfen

Der Plan zeigt das SQL der Migration nicht. Um es vor dem Apply zu lesen, gib allein dem Migrations-Job das neue Image und führe darin `my upgrade -dry-run` aus, das nichts ändert:

```sh
tofu apply -target='module.mycroblog.module.app.google_cloud_run_v2_job.one_off["migrate"]'
gcloud run jobs execute my-migrate --project="$PROJECT" --region=asia-northeast1 --args=upgrade,-dry-run --wait
gcloud logging read 'resource.type="cloud_run_job" AND resource.labels.job_name="my-migrate"' --project="$PROJECT" \
  --freshness=1h --format='value(textPayload)' --order=asc
```

<!-- source: destructive-changes 5b3fdea90a55 -->
### Zerstörerische Änderungen

Wenn die Release Notes `-allow-destructive` verlangen, nenne diese Version im Block:

```hcl
  upgrade = { allow_destructive_for = "0.3.0" }
```

`my upgrade` bekommt `-allow-destructive` nur beim Apply dieser Version, sodass ein stehengebliebener Wert kein späteres Release Daten löschen lässt.

<!-- source: pinning-the-image-by-digest b31962ecc908 -->
### Das Image an einen Digest heften

Die [Produktions-Checkliste](production-checklist.md#operations) empfiehlt einen Digest, aus den Release Notes:

```hcl
  image = { digest = "sha256:..." }
```

Ein Digest nennt genau ein Image, was auch immer der Tag daneben sagt, ändere ihn also in derselben Bearbeitung wie `ref`: Ein Digest, der vom vorigen Release übrig bleibt, behält das Image dieses Releases unter der neuen Version.

<!-- source: the-maintenance-mode 518e9aa176c0 -->
### Der Wartungsmodus

Der [Wartungsmodus](maintenance.md) braucht drei Applies, da sich die Dienste vor dem Upgrade und danach ändern. Füge ihn zu den Einstellungen des Blocks hinzu und wende an, sichere die Datenbank, erhöhe `ref` und wende an, entferne ihn dann und wende an:

```hcl
  settings = { maintenance = "true" }
```

Ohne ihn bedient die alte Revision während der Migration weiter, was [nicht garantiert funktioniert](database.md#the-server-during-an-upgrade). Einstellungen, die von Hand mit `gcloud` geändert wurden, halten nicht: Das nächste Apply stellt wieder her, was der Block sagt.

<!-- source: monitoring-and-alerts 930234cd0dde -->
## Monitoring und Alarme

Das Modul fügt eine Uptime-Prüfung von `/health/live`, logbasierte Metriken und Alarmrichtlinien mit den Namen aus [dem Runbook](alerts.md) hinzu, die die Adressen von `monitoring.notification_emails` (`alert_emails` im Beispiel) oder die Kanäle von `notification_channels` benachrichtigen. Die Eingabe hat keinen Standard: Nenne, wer benachrichtigt wird, oder setze `monitoring = null`, um alles wegzulassen. Es wurde noch nie angewendet, probiere es also einmal aus, indem du eine Prüfung fehlschlagen lässt. Jeder Lauf der Prüfung weckt eine Instanz auf, was zu [dem Zielkonflikt beim Tick](#costs) hinzukommt: `uptime_period`, standardmäßig 15 Minuten, ist, wie oft.

<!-- source: changing-a-backend c20598caf230 -->
## Ein Backend wechseln

Ändere die `kind` seines Abschnitts. Jede Art setzt auf dem Server, was sie braucht, und der Plan sagt, was sich ändert. Eine Datenbank zu wechseln heißt auch, ihre Daten zu verschieben, da eine neue leer ist: Mache ein [Backup](backup.md) der alten und stelle es in der neuen wieder her, während nichts schreibt. Zum Beispiel Cloud SQL anstelle von Neon:

```hcl
  database = { kind = "cloud-sql" }
```

Für einen Speicher außerhalb von Google Cloud setze `media.kind` auf `s3`, für jeden S3-kompatiblen Dienst mit seinem `endpoint`, oder auf `r2`, mit `cloudflare.account_id`. Beide lesen einen Zugriffsschlüssel aus den Secrets, die `secrets_to_fill` auflistet.

<!-- source: when-you-need-a-network 0a4359c002ed -->
## Wenn du ein Netzwerk brauchst

Dieser Leitfaden braucht keines: Neon, Cloud Tasks, Cloud Storage und Pub/Sub werden alle über das Internet mit der eigenen Identität des Dienstes erreicht, sodass Cloud Run an nichts angebunden wird und es nichts zu bezahlen oder zu pflegen gibt.

Du brauchst eines, um Memorystore in die Bereitstellung aufzunehmen, da es nur private Adressen hat, oder um Cloud SQL eine private Adresse anstelle seiner öffentlichen zu geben. `network = { kind = "vpc" }` legt eines an, und `network = { kind = "existing", ... }` bindet ein VPC an, das du hast und das du durch `network`, `subnetwork`, `private_services_range` und `memorystore_connection_policy` beschreibst:

```hcl
  network = { kind = "vpc" }
  kv      = { kind = "memorystore" }
```

Nur private Bereiche laufen durch das Netzwerk, der Dienst erreicht das Internet also wie zuvor, ohne Cloud NAT. `nat = true` schickt stattdessen alles durch Cloud NAT, für eine statische Quelladresse. Aktiviere zuerst die APIs Service Networking und Network Connectivity sowie die Compute Engine API. Ein Netzwerk verlängert den Kaltstart einer Instanz, und Memorystore wird abgerechnet, während der Server im Leerlauf ist.

<!-- source: splitting-jobs-into-workers 831929244c6d -->
## Jobs auf Worker aufteilen

Wenn eine Art von Job den Rest aufhält, gib ihr einen eigenen Dienst. Entscheide anhand der Metriken, was du aufteilst, wie [Worker nach Job-Typ aufteilen](running.md#splitting-workers-by-job-type) es sagt. Ein Wert, `workers`, geht an den Dienst, die Queue und das Monitoring, und jede Queue, URL, Route und Einstellung ergibt sich aus seinen Schlüsseln, sodass du keine davon schreibst:

```hcl
  workers = {
    delivery = {
      job_types     = ["federation.fanout", "federation.deliver"]
      max_instances = 20
      concurrency   = 16
    }
  }
```

Der Plan lehnt eine Aufteilung der Typen ab, die zusammenbleiben müssen. Er braucht außerdem einen Broker, der jeden Prozess erreicht, was `stream.kind = pubsub` ist.

Die zwei gängigen Aufteilungen dieser Seite sehen so aus. Damit Zustellungen den Rest nicht verzögern, nimm den Wert oben. Damit E-Mails und Benachrichtigungen schnell bleiben:

```hcl
  workers = {
    notify = {
      job_types = [
        "identity.verification_email", "identity.password_reset_email", "identity.password_notice_email",
        "identity.closure_email", "identity.deactivation_email",
        "notification.email", "notification.moderator_email", "notification.supporter_email",
        "notification.push", "notification.post_subscribers",
        "instance.notify_announcement", "voice.notify_started",
      ]
      max_instances = 10
    }
  }
```

Lass die Typen von Einstellungen weg, die du nicht gesetzt hast, wie diese Seite sagt: Ein Typ, den der Server nicht registriert hat, verhindert den Start, und der Plan kann nicht sehen, welche das sind. Mit `workers` hilft der Standarddienst bei diesen Typen nicht, wenn er frei ist, da ihre Tasks in ihre eigene Queue gehen.

Jeder Worker nimmt `job_types` sowie `min_instances`, `max_instances`, `cpu`, `memory_mib`, `concurrency`, `max_concurrent_dispatches` und `max_dispatches_per_second`, jeweils die des Dienstes oder der Queue, wo er sie nicht nennt. Der Dienst eines Workers nimmt keinen Traffic von außen an. Ein Upgrade verschiebt jeden nach der Migration und vor dem Standarddienst, und ohne dass etwas es ihm sagt: Nur ein Schritt, der fehlschlägt, hält es an. Jeder Worker kann bringen, was der Dienst kostet, und die Verbindungen seiner Instanzen zählen gegen das Limit der Datenbank.

<!-- source: how-traffic-reaches-the-service 156fc6f76743 -->
## Wie der Traffic den Dienst erreicht

`ingress.kind` entscheidet das. Der Standard ist der Load Balancer, der für den Produktivbetrieb gedacht ist. Das Domain-Mapping dient dazu, eine Bereitstellung günstig auszuprobieren: Setze `ingress = { kind = "domain-mapping" }`.

| | `load-balancer` (der Standard) | `domain-mapping` |
| --- | --- | --- |
| Monatliche Gebühr | Etwa 18 $ für die Weiterleitungsregeln, plus eine Gebühr pro GB. Prüfe die [Preisseite](https://cloud.google.com/vpc/network-pricing): Diese Zahl ließ sich dort nicht bestätigen | Keine |
| Status | Der von Google Cloud empfohlene Weg | Eine Preview-Funktion, die Google Cloud wegen der Latenz nicht für den Produktivbetrieb empfiehlt |
| Regionen | Alle | `asia-northeast1`, `us-central1` und einige weitere |
| DNS | Ein `A`-Record, aus `tofu output dns_records` | Die Records, die dieselbe Ausgabe auflistet |
| Zertifikat | Von Google verwaltet, aktiv, sobald DNS auf die Adresse zeigt | Von Google verwaltet, bis zu 24 Stunden |
| Domain-Inhaberschaft | Nichts außer DNS | Du und jedes andere Konto, das anwendet, müsst vor dem Apply ein [verifizierter Inhaber](https://cloud.google.com/run/docs/mapping-custom-domains) sein |
| `run.app`-URL | Aus dem Internet abgewiesen, sodass niemand den Load Balancer umgeht | Für jeden erreichbar: Ein Domain-Mapping kann keinen Header hinzufügen, der Origin bleibt also offen |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` und die Adresse des Load Balancers | `169.254.0.0/16` |

Cloud Tasks und Cloud Scheduler kommen über keines von beiden: Sie rufen die Standard-`run.app`-URL direkt auf.

<!-- source: cloudflare e55e367a0064 -->
### Cloudflare

`ingress = { kind = "cloudflare" }` stellt Cloudflare vor `host`, wobei der Cache für alles umgangen wird, und `ingress.media_host` fügt neben jeder Art einen gecachten Medien-Host hinzu. Beide brauchen `cloudflare = { zone_id = "..." }`, und das Token kommt in `CLOUDFLARE_API_TOKEN`. Sie sind nur auf gemockten Providern gelaufen.

- **Dein Tarif entscheidet den Origin.** Cloud Run routet nach `Host`, und Cloudflare bietet die Überschreibung des Host-Headers nur im Enterprise-Tarif an. Bei jedem anderen Tarif muss der Origin deinen Host selbst annehmen, daher legt das Modul ein Domain-Mapping jedes Hosts an, mit den Abstrichen der Tabelle oben, und Cloudflare verbindet sich damit. Ob sein Zertifikat hinter dem Proxy ausgestellt und erneuert wird, ist ungeprüft, beginne also mit `cloudflare.proxied = false`. Im Enterprise-Tarif verbindet `cloudflare.host_header_override = true` stattdessen mit der `run.app`-URL, ohne Domain-Mapping.
- **Ein Header schließt den Origin.** Das Modul fügt eine Transformationsregel für Anfrage-Header hinzu, und der Server weist eine Anfrage ohne den Header mit `403` ab, abgesehen von Health-Checks und den eigenen `POST`s der Jobs, die ihre Tokens prüfen. So umgeht niemand Cloudflare über die `run.app`-URL, und ein eigener Cloudflare Worker eines Aufrufers kann nicht die Client-Adresse wählen, die die Ratenbegrenzungen sehen.
- **Der Wert landet im State.** Das Modul generiert ihn und schreibt ihn in das Secret `my-origin-secret`, aber der Cloudflare-Provider hat keine Write-only-Form für den Header-Wert einer Regel, halte den State also privat. [Der CDN-Leitfaden](cdn.md#rotate-the-value) erklärt, wie die Rotation funktioniert.
- **Nichts fügt den Header hinzu, solange ein Record nicht geproxyt ist**, sodass der Server alles außer seinen Health-Checks abweist: Schalte bei `proxied = false` für das Zertifikat eines Domain-Mappings den Proxy ein, sobald das Zertifikat aktiv ist.
- **Schalte Bot Fight Mode aus** im Dashboard. Er gilt für die ganze Zone, und keine Regel kann ihn überspringen, und eine Challenge auf `/ap/`, `/.well-known/` oder `/api/` bricht die Föderation, da andere Server keine Browser sind. Jobs laufen nicht über Cloudflare. Das Modul fügt keine Challenge hinzu und schaltet Browser Integrity Check aus, verwaltet aber Bot Fight Mode nicht. Schalte aus demselben Grund „I'm Under Attack“ nicht ein.
- **Email Address Obfuscation kann eingeschaltet bleiben.** Es schreibt E-Mail-Adressen im HTML in `[email protected]` um und fügt ein Skript hinzu, das sie wiederherstellt, was die Content-Security-Policy des Servers ablehnt. Die eigenen Seiten des Servers, etwa die, die sagt, wohin die Bestätigungs-E-Mail ging, markieren ihren Inhalt so, dass er die Adressen in Ruhe lässt.
- **Schalte Always Use HTTPS ein** im Dashboard, wenn du `http://` umleiten willst. Es gilt für die ganze Zone, daher überlässt das Modul es dir.
- **Timeouts**: Cloudflare antwortet mit `524`, wenn der Origin 125 Sekunden lang nichts sendet. Jobs sind nicht betroffen, da sie die `run.app`-URL direkt aufrufen. Streams senden alle 30 Sekunden einen Heartbeat, unter dem Leerlauflimit von 900 Sekunden. Ob es Streams auf einem gewöhnlichen geproxyten Host puffert, ist ungeprüft.
- **Uploads**: 100 MB bei Free und Pro, über den 32 MiB von Cloud Run, die das Limit bleiben.
- **Video**: Die Bedingungen von Cloudflare schränken das Ausliefern von Video und einen unverhältnismäßig hohen Anteil großer Dateien bei Free, Pro und Business ein, und es kann den Inhalt umleiten oder andere Maßnahmen ergreifen. Entscheide das, bevor du Video auf dem Medien-Host cachst.

**Eine gelöschte Datei bleibt im Cache.** Der Medien-Host folgt dem `Cache-Control` des Servers, sodass eine gelöschte lokale Datei bis zu einem Jahr sichtbar bleiben kann und eine entfernte für `remote_media_cache_ttl_seconds`. Der Server leert sie nicht. Wenn etwas verschwinden muss, leere sie von Hand, unter **Caching > Configuration > Custom Purge** im Dashboard, oder:

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

Eine Proxy-URL trägt ihr `sig` in der Query-Zeichenfolge, die zu dem gehört, was gecacht wird, gib also die ganze URL an.

<!-- source: changing-the-kek-backend b1e8cbb9ccfb -->
## Das KEK-Backend wechseln

`kek.kind` zu ändern ist eine Zeile in `main.tf`, aber bei einer Instanz mit Daten ist es nicht umsonst. Die Datenbank enthält Secrets, die mit dem aktuellen Schlüssel verpackt sind, sodass eine Instanz, die ihn verliert, sie nicht mehr lesen kann, und die Schlüssel jedes Kontos gehören dazu.

Folge [Zwischen dem lokalen KEK und einem KMS wechseln](key-management.md#move-between-the-local-kek-and-a-kms). Vom Secret zu Cloud KMS behalte den aktuellen KEK als `kek_previous`, während die Secrets umziehen: Füge `secrets = ["kek_previous"]` hinzu und befülle das Secret `my-kek-previous` mit dem aktuellen KEK. Führe `my kek rewrap` über [die Konsole](deploy-opentofu.md#the-console) aus und entferne ihn dann, wenn `my kek status` keine `local:`-Schlüssel mehr zählt. Behalte den KEK, solange du Datenbank-Backups von vor dem Rewrap behältst.

Auf einer neuen Instanz ohne Daten muss nichts umziehen.

<!-- source: limits 3ff113a57279 -->
## Limits

- **Uploads**: Cloud Run weist eine HTTP/1-Anfrage über 32 MiB ab, und der Server spricht HTTP/1.1. Das Modul setzt daher `MY_MEDIA_MAX_AV_BYTES` auf 30 MiB, gegenüber dem Standard von 100 MiB, damit ein größerer Upload die eigene Antwort des Servers bekommt.
- **Streams**: Das Request-Timeout, standardmäßig 15 Minuten, beendet einen offenen Stream, und der Client verbindet sich erneut.
- **Client-Adressen**: Die Ratenbegrenzungen stützen sich auf `MY_TRUSTED_PROXIES`, das das Modul aus dem Ingress setzt. Kein Dokument, das wir gelesen haben, sagt, was Cloud Run bei einer direkten Anfrage zu `X-Forwarded-For` hinzufügt oder welche Peer-Adresse ein Container sieht, und der Container-Vertrag sagt zu beidem nichts, daher ist `169.254.0.0/16` eine Annahme. Die dokumentierte Form des externen Load Balancers ist `[<supplied>,]<client-ip>,<lb-ip>`. Prüfe die Adressen in den ersten Request-Logs: Eine Client-IP, die eine `169.254`-Adresse oder eine Google-Adresse ist, bedeutet, dass die Einstellung für dein Projekt falsch ist.
- **Öffentlicher Zugriff**: Der Dienst gewährt `roles/run.invoker` an `allUsers`, was eine Organisationsrichtlinie zur domänenbeschränkten Freigabe abweist. Ein solches Projekt braucht eine Ausnahme.

<!-- source: costs b8be3997d723 -->
## Kosten

Ein Server kostet Geld. Diese Zahlen sind ein Anhaltspunkt dafür, was dich erwartet und wo Sorgfalt sie senkt, keine Grenzen, an die sich die Bereitstellung hält. Es sind ungefähre Listenpreise für eine Region, von 2026-10 und 2026-11. Prüfe sie auf den Preisseiten von [Cloud Run](https://cloud.google.com/run/pricing) und [Neon](https://neon.com/pricing). Compute ist pro Rechnungskonto bis zu 180.000 vCPU-Sekunden und 360.000 GiB-Sekunden im Monat kostenlos.

| Teil | Leerlauf | Was dazukommt |
| --- | --- | --- |
| Load Balancer | Etwa 18 $ im Monat, auf der Preisseite nicht bestätigt | Eine Gebühr pro GB; siehe [Wie der Traffic den Dienst erreicht](#how-traffic-reaches-the-service) |
| Neon, im Launch-Tarif | Etwa 19 $ im Monat, mit dem Standard-Tick | Speicher zu 0,35 $ pro GB-Monat. `database.kind = cloud-sql` hat stattdessen feste Kosten von etwa 9,40 $ im Monat, für eine kleinere Maschine |
| Cloud Run | 0 $: min instances ist 0, und die CPU wird nur während Anfragen abgerechnet | Jeder offene Stream hält eine Instanz in Rechnung; siehe unten |
| Cloud Tasks | 0 $ | 1 Million Operationen frei, danach 0,40 $ pro Million. Ein Job braucht etwa 2 |
| Cloud Scheduler | 0 $: 3 Jobs frei pro Rechnungskonto | 0,10 $ pro Job darüber |
| Cloud Storage | Etwa 0,02 $ pro GiB im Monat | 5 GiB sind nur in `us-east1`, `us-west1` und `us-central1` frei. Medien gehen über Cloud Run hinaus |
| Pub/Sub | 0 $: 10 GiB frei | Nur solange Streams offen sind |
| Secret Manager | Cent-Beträge | Einige freie Versionen, danach ein paar Cent pro Version |

**Der Tick hält Neon wach.** Jeder Tick liest und schreibt die Datenbank, und Neon stoppt eine Compute-Instanz erst nach 5 Minuten ohne Aktivität, daher stoppt sie mit dem Standard `* * * * *` nie. Das sind 730 Stunden im Monat oder 182,5 CU-Stunden bei 0,25 CU, was die 19 $ oben ergibt. Der Tick bleibt bei jeder Minute, damit periodische Jobs pünktlich laufen und ein verlorenes Weck-Signal bald wieder aufgeholt wird.

**Ein offener Stream hält eine Instanz in Rechnung.** Bei Abrechnung nach Anfragen wird eine Instanz abgerechnet, solange sie irgendeine Anfrage bearbeitet, und die Echtzeit-Aktualisierungen sind eine lange Anfrage. Ein Client, der seinen Stream den ganzen Monat offen hält, kostet eine Instanz rund um die Uhr: etwa 60 $ im Monat bei 1 vCPU und 512 MiB, nach dem Freikontingent. Mehr Streams teilen sich die Instanz, bis `concurrency`. Wenn Streams die meiste Zeit offen bleiben, vergleiche mit der Abrechnung nach Instanzen oder mit einer kleinen VM und [Docker Compose](deploy-docker-compose.md).

<!-- source: where-care-reduces-it 28bfc2e5ffcc -->
### Wo Sorgfalt die Kosten senkt

- **Das Domain-Mapping** anstelle des Load Balancers nimmt die 18 $ weg, mit [den Abstrichen oben](#how-traffic-reaches-the-service). Es eignet sich zum Ausprobieren einer Bereitstellung.
- **Der Free-Tarif von Neon** nimmt die 19 $ weg, und eine kleine Instanz kann darauf laufen, mit diesen Abstrichen:
  - Er erlaubt 100 CU-Stunden im Monat, die der Standard-Tick nach etwa 17 Tagen verbraucht. Bleibe darunter mit einem längeren Zeitplan, etwa `queue = { tick_schedule = "*/10 * * * *" }`: höchstens die halbe Zeit wach, etwa 91 CU-Stunden, bevor irgendetwas anderes sie weckt. Periodische Jobs laufen dann bis zu 10 Minuten zu spät, und ein verlorenes Weck-Signal wird später aufgeholt.
  - Ein föderierter Server empfängt rund um die Uhr Zustellungen, die auch die Datenbank wecken, sodass eine ausgelastete Instanz ihn unabhängig vom Zeitplan überschreitet.
  - Er fasst 1 GB pro Projekt und einen 6-Stunden-Verlauf zum Wiederherstellen, und seine Compute-Instanz stoppt immer nach 5 Minuten Leerlauf, sodass die erste Anfrage danach auf sie wartet.
