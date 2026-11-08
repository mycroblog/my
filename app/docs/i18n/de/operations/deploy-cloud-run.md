<!-- source: deploying-on-cloud-run 9d2acbfd443c -->
# Auf Cloud Run bereitstellen

So betreibst du `my` auf Google Cloud mit den OpenTofu-Modulen in [`deploy/opentofu`](../../deploy/opentofu/README.md): Cloud Run, Cloud Tasks, Cloud Scheduler, Cloud Storage und Pub/Sub, mit einer PostgreSQL-Datenbank auf Neon, hinter einem globalen externen Application Load Balancer. Der Dienst führt keine Instanz aus, solange ihn niemand nutzt, bis auf einen Tick, der den Server jede Minute weckt. Ein Server kostet Geld: Rechne mit etwa 37 $ im Monat vor Speicher, Traffic und offenen Streams, und lies [Kosten](#costs), was sich mit Sorgfalt senken lässt.

Die Module brauchen OpenTofu 1.11 oder neuer. Terraform wird nicht unterstützt.

<!-- source: what-it-creates c95687df197f -->
## Was erstellt wird

| Teil | Modul | Ressourcen |
| --- | --- | --- |
| Server | `app-gcp-cloud-run` | Ein Cloud-Run-Dienst mit `role = all`, ein Migrations-Job, ein Konsolen-Job, solange du ihn brauchst, ein Dienstkonto und öffentlicher Zugriff |
| Traffic | `ingress-gcp-load-balancer`, oder `ingress-gcp-domain-mapping` oder `ingress-cloudflare-app`, und optional `ingress-cloudflare-media` | Ein globaler externer Application Load Balancer mit Zertifikat, oder ein Domain-Mapping, oder Cloudflare, und ein Medien-Host |
| Jobs | `queue-gcp-cloud-tasks` | Eine Cloud-Tasks-Queue, ein Cloud-Scheduler-Job und ein Dienstkonto, das den Server aufruft |
| Medien | `blob-gcp-gcs` | Ein privater Bucket |
| Echtzeit-Aktualisierungen | `stream-gcp-pubsub` | Ein Topic und eine nutzerdefinierte Rolle. Der Server legt seine eigenen Subscriptions an und entfernt sie wieder |
| Datenbank | `db-external` | Nichts: Du legst die Datenbank von Hand an |
| E-Mail | `mail-smtp` | Nichts: ein beliebiger SMTP-Dienst |
| Secrets | `secrets-gcp-secret-manager` | Secrets für die Datenbank-URL und das SMTP-Passwort, die du befüllst, und ein generierter VAPID-Schlüssel |
| KEK | `kek-gcp-secret-manager` oder `kek-gcp-kms` | Ein generiertes Secret, das als Datei eingebunden wird, oder ein Cloud-KMS-Schlüssel |

Secret-Werte gelangen nie in den OpenTofu-Plan oder -State: Die Werte, die von anderswo kommen, fügst du selbst hinzu, und die Bereitstellung generiert den KEK und den VAPID-Schlüssel über Write-only-Attribute.

<!-- source: before-you-start 4c624a6b7316 -->
## Bevor du anfängst

- `tofu` 1.11 oder neuer, `gcloud` und `jq`.
- Ein Google-Cloud-Projekt mit Abrechnung, das nur für diesen Server genutzt wird, und ein Konto darin mit der Rolle Owner.
- Eine Domain für [`host`](../reference/configuration.md#host). Sie lässt sich später nicht mehr ändern: Lies [Einstellungen, die sich später nicht mehr ändern lassen](production-checklist.md#settings-you-cannot-change-later).
- Eine Version von `my` aus den [Releases](running.md#get-the-image). Es gibt kein `latest`-Tag.

<!-- source: first-deployment 996fcc6eb35e -->
## Erste Bereitstellung

<!-- source: 1-enable-the-apis 2d9c831fb77d -->
### 1. Die APIs aktivieren

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com compute.googleapis.com
```

`compute.googleapis.com` ist für den Load Balancer.

<!-- source: 2-create-the-database-on-neon 139308cfc452 -->
### 2. Die Datenbank auf Neon anlegen

Rechne mit dem Launch-Tarif: Mit dem Standard-Tick stoppt die Compute-Instanz nie. Der Free-Tarif funktioniert ebenfalls, mit den Abstrichen unter [Kosten](#costs).

Es gibt zwei Wege, und das Beispiel nutzt den ersten:

- **Von Hand, mit `db-external`.** Lege in der Neon-Konsole ein Projekt in einer Region nahe deiner Cloud-Run-Region an und kopiere seine Verbindungsstrings. Das Passwort bleibt aus dem OpenTofu-State heraus.
- **Mit [`db-neon`](../../deploy/opentofu/modules/db-neon/README.md)**, das das Projekt anlegt und die URLs für dich in die Secrets schreibt. **Das Datenbankpasswort landet dann im State**, und der Provider wird nicht mehr gepflegt: Seine README sagt, was das bedeutet, und ein Kommentar am `db`-Block in `main.tf` zeigt den Austausch.

Ein Projekt hat zwei Verbindungsstrings, und die Bereitstellung verwendet jeden dort, wo er hingehört:

- Der **direkte** (der Host ohne `-pooler`) ist erforderlich. Der Migrations-Job und die Konsole verwenden ihn immer: `my schema apply` hält eine Lock auf Sitzungsebene, die ein Pooler im Transaktionsmodus nicht aufrechterhält.
- Der **gepoolte** (der Host mit `-pooler`) ist optional, für den Dienst. Der Pooler von Neon verarbeitet, was der Server braucht: siehe [Über einen Connection-Pooler](database.md#through-a-connection-pooler). Gib ihn an `pooled_url_secret_id` des `db`-Moduls weiter, in einem eigenen Secret, das du zu `referenced` des `secrets`-Moduls hinzufügst.

Ohne gepoolten String verwendet der Dienst ebenfalls den direkten, bis zum Verbindungslimit der Datenbank, das `max_instances` des App-Moduls begrenzt. Um den Pool von seinem Standard von 4 pro Server anzuheben, für die 80 Anfragen, die eine Cloud-Run-Server gleichzeitig annimmt, füge `pool_max_conns` zu dem String hinzu, den der Dienst verwendet, zum Beispiel `postgres://...-pooler...neon.tech/neondb?sslmode=require&pool_max_conns=8`. Siehe [Warten auf eine Datenbankverbindung](running.md#waiting-for-a-database-connection).

<!-- source: 3-copy-the-example-and-set-the-variables 3abe6b730543 -->
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

**Setze `ref` auf dieses Release.** Jeder Modulblock in `main.tf` nennt ein Release mit `?ref=`, was die Kopie eines stabilen Releases bereits richtig hat. Bei einem Prerelease setzt du es selbst: `sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf`. Module gibt es erst ab dem ersten Release, das sie enthält, daher hat ein früheres Tag kein Beispiel zum Kopieren. Behalte dasselbe Release in `ref` und in `image`, und behalte dieses Verzeichnis: Es enthält deinen State und deine Einstellungen.

Fülle `terraform.tfvars` aus. `mail_from`, `mail_smtp_host`, `mail_smtp_port` (standardmäßig 587; Google Cloud blockiert den ausgehenden Port 25) und `mail_smtp_username` konfigurieren die [E-Mail](email.md). `env` und `secret_env` nehmen jede andere Einstellung aus der [Konfigurationsreferenz](../reference/configuration.md) entgegen, per Wert und per Secret-ID. Eine Einstellung, die an zwei Stellen genannt wird, lässt den Plan scheitern.

Um keine E-Mails zu versenden, entferne das `mail`-Modul und sein Secret aus `main.tf`, und `mail` bleibt `none`. Lies zuerst, [was das für Registrierungen bedeutet](production-checklist.md#email).

Entscheide dich jetzt für [`account_domain`](../reference/configuration.md#account_domain), als `MY_ACCOUNT_DOMAIN` in `env`, wenn Handles eine andere Domain verwenden.

**Der State** bleibt standardmäßig in diesem Verzeichnis. Er enthält keine Secret-Werte, ist aber die einzige Aufzeichnung dessen, was angelegt wurde: Bewahre ihn in einem Bucket mit einem `backend "gcs"`-Block auf, wenn mehr als eine Person das hier ausführt.

<!-- source: 4-create-the-secrets-and-add-their-values f94fd484e28f -->
### 4. Die Secrets anlegen und ihre Werte hinzufügen

Das legt die Secrets an und generiert den KEK und den VAPID-Schlüssel:

```sh
tofu init
tofu apply -target=module.secrets -target=module.kek
```

`tofu init` holt außerdem `upgrade.sh` und `console.sh` in diesem Release, die die nächsten Schritte verwenden:

```sh
SCRIPTS=.terraform/modules/app/deploy/opentofu
```

**Kopiere den KEK jetzt** und bewahre die Kopie getrennt von der Datenbank auf, zum Beispiel in einem Passwortmanager. Ohne ihn lässt sich ein [Backup](backup.md) nicht wiederherstellen, und die Schlüssel der Konten gehen verloren. Tu dasselbe mit dem VAPID-Schlüssel, dessen Verlust jedes Web-Push-Abo beendet:

```sh
gcloud secrets versions access latest --secret=my-kek --project="$PROJECT"
gcloud secrets versions access latest --secret=my-vapid-key --project="$PROJECT"
```

Füge dann die Werte hinzu, die von anderswo kommen. Eine Instanz, deren Secret sich nicht lesen lässt, startet nicht, also füge sie vor der App hinzu:

```sh
printf '%s' 'postgres://...' | gcloud secrets versions add my-database-url --project="$PROJECT" --data-file=- # the direct string
printf '%s' '<the SMTP password>' | gcloud secrets versions add my-smtp-password --project="$PROJECT" --data-file=-
```

Der KEK erreicht den Server als eingebundene Datei, benannt durch `MY_KEK_FILE`. Die anderen sind Umgebungsvariablen, die beim Start einer Instanz aufgelöst werden, sodass eine neue Version bei der nächsten Instanz wirksam wird: Stelle eine neue Revision bereit, um sicherzugehen.

**OpenTofu generiert den KEK einmal und nie wieder.** Das Secret und seine Version haben `prevent_destroy`, und die Versionsnummer, die einen neuen Wert schreiben würde, ist im Modul fest vorgegeben: Ändere sie nicht, denn ein neuer zufälliger KEK macht alles unlesbar, was der alte verschlüsselt hat. Zum Rotieren folge [Den KEK rotieren](key-management.md#rotate-the-kek) und füge den neuen Wert mit `gcloud secrets versions add` hinzu. OpenTofu verfolgt nicht, was du dort hinzufügst.

Um den KEK von Anfang an in Cloud KMS zu halten, ersetze den `kek`-Block in `main.tf`, wie sein Kommentar es zeigt, und aktiviere `cloudkms.googleapis.com`. Um einen eigenen KEK oder VAPID-Schlüssel mitzubringen, setze `create = false` am `kek`-Modul oder entferne den Schlüssel aus `generate_p256_keys`, vor diesem Schritt. Ein anderswo angelegtes Secret braucht nur seine ID: Gib sie an `secret_env` des App-Moduls oder an das Modul weiter, das es verwendet.

<!-- source: 5-create-what-the-migration-needs eab1bb33867c -->
### 5. Anlegen, was die Migration braucht

Der Startup-Probe des Dienstes wartet auf `/readyz`, das `503` antwortet, bis das Schema existiert, daher muss das Schema zuerst kommen. Lege den Migrations-Job und alles an, wovon er abhängt, einschließlich der Adresse des Load Balancers, da die Einstellungen des Servers sie nennen:

```sh
tofu apply -target='module.app.google_cloud_run_v2_job.one_off["migrate"]'
```

<!-- source: 6-apply-the-schema 2946ca18d263 -->
### 6. Das Schema anwenden

```sh
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.1.0 --migrate-only
```

Es führt `my config check`, `my db upgrade -check`, `my schema apply` und `my db upgrade` aus, jeweils als Ausführung des Jobs, und hält beim ersten an, der scheitert. Der erste Befehl, der die Datenbank verwendet, trägt `host` und `account_domain` darin ein.

<!-- source: 7-create-the-rest 368abac4aa08 -->
### 7. Den Rest anlegen

```sh
tofu apply
```

<!-- source: 8-point-the-domain-at-the-load-balancer 7e54edc163b2 -->
### 8. Die Domain auf den Load Balancer zeigen lassen

`tofu output dns_records` listet einen `A`-Record auf, von `host` auf die Adresse des Load Balancers. Füge ihn bei deinem DNS-Anbieter hinzu. Hat dieser Anbieter einen eigenen Proxy, lass ihn für diesen Record ausgeschaltet.

Das Zertifikat wird von Google verwaltet und wird erst aktiv, wenn DNS auf die Adresse zeigt, was eine Weile dauern kann. Bis dahin scheitert `https://{host}` mit einem TLS-Fehler, sodass Föderation und Browser den Server noch nicht erreichen können. Nichts anderes wartet darauf: Cloud Tasks und Cloud Scheduler rufen die Standard-`run.app`-URL des Dienstes auf, `https://<service>-<project number>.<region>.run.app`. **Deaktiviere die Standard-URL nicht**, sonst bleiben die Jobs stehen. Diese URL weist Anfragen aus dem Internet ab, und Cloud Run zählt diese beiden nur aus demselben Projekt und auf dieser URL als intern, nicht auf einer eigenen Domain.

Was du sonst noch wählen kannst, steht unter [Wie der Traffic den Dienst erreicht](#how-traffic-reaches-the-service).

<!-- source: 9-create-the-first-administrator c5b03212fb36 -->
### 9. Den ersten Administrator anlegen

Cloud-Run-Jobs haben kein Terminal und keine Standardeingabe, daher gibt `my admin create` einen Link aus, über den das Passwort gesetzt wird, statt eines zu lesen. Schalte die [Konsole](#the-console) ein und führe aus:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

Öffne den Link sofort und setze das Passwort. Er funktioniert einmal, eine Stunde lang. **Bis dahin bleibt er in Cloud Logging**, wo ihn jeder verwenden kann, der die Logs des Projekts lesen darf. Läuft er ab, führe `my admin account password-reset-link @alice` über die Konsole aus. Schalte dann die Zwei-Faktor-Authentifizierung ein.

<!-- source: 10-go-through-the-checklist 90add86e4288 -->
### 10. Die Checkliste durchgehen

Gehe die [Produktions-Checkliste](production-checklist.md) durch, beginnend mit der [E-Mail](production-checklist.md#email): SPF, DKIM und DMARC auf der Domain von `mail_from`.

<!-- source: the-console 5aebf704c6d7 -->
## Die Konsole

Die Konsole führt einen `my`-Befehl mit dem Image, den Einstellungen und den Secrets des Dienstes aus. Sie existiert nur, solange `console_enabled` eingeschaltet ist, schalte sie also aus, wenn du fertig bist:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` wartet auf den Befehl, gibt seine Ausgabe und seinen Exit-Code aus und beendet sich mit diesem Code. Es braucht `roles/logging.viewer`, weil die Ausgabe eines Jobs in Cloud Logging landet. Es verwendet das Image, das der Dienst ausführt, was auch immer `image` sagt.

Der Befehl hat keine Standardeingabe und kein Terminal, daher lässt sich ein Befehl, der ein Passwort oder eine Datei von dort liest, so nicht ausführen. Was er ausgibt, bleibt in den Logs, führe also keinen Befehl aus, dessen Ausgabe du nicht dort hinschreiben würdest.

<!-- source: upgrading 1a1e204c5173 -->
## Aktualisieren

Lies zuerst [Aktualisieren](database.md#upgrading) und die Release Notes, und gehe jeweils eine Minor-Version weiter.

```sh
# 1. Back up the database while nothing writes. Use pg_dump or a Neon branch (see Backup and restore).
gcloud run services update my --project="$PROJECT" --region=asia-northeast1 --update-env-vars=MY_MAINTENANCE=true
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Read the SQL it would run. This changes nothing but the image of the migration job.
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.2.0 --plan

# 3. Upgrade. These scripts are of the release your modules name, which is enough: they only run gcloud and my.
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.2.0 --maintenance
```

Das Skript tut Folgendes und hält beim ersten Schritt an, der scheitert:

1. Schaltet den Dienst mit `--maintenance` in den [Wartungsmodus](maintenance.md).
2. Gibt dem Migrations-Job das neue Image.
3. Führt `my config check`, `my db upgrade -check`, `my schema apply` und `my db upgrade` aus, jeweils im Anschluss an den vorherigen.
4. Stellt die neue Revision bereit, ohne den Wartungsmodus. Sie nimmt Traffic erst an, wenn ihr Startup-Probe auf `/readyz` besteht.

Ohne `--maintenance` bedient die alte Revision während der Schritte 2 und 3 weiter Anfragen, was [nicht garantiert funktioniert](database.md#the-server-during-an-upgrade). Füge `--allow-destructive` hinzu, wenn die Release Notes es verlangen. Pinne einen Digest mit `--image ghcr.io/mycroblog/my:0.2.0@sha256:...`, was die [Produktions-Checkliste](production-checklist.md#operations) empfiehlt.

Setze dann `image` in `terraform.tfvars` auf denselben Wert und `ref` in `main.tf` auf dasselbe Release, gefolgt von `tofu init` und `tofu plan`. Lies diesen Plan, bevor du ihn anwendest: Die Module eines neuen Releases können Ressourcen ändern. `tofu apply` ändert nie das Image eines Dienstes oder Jobs, der existiert, ein vergessenes `image` schadet also nicht, aber der Wert ist das, womit ein neuer Job oder ein neu angelegter Dienst starten würde.

**Wenn ein Schritt scheitert**, bleibt der Dienst auf seiner Revision, im Wartungsmodus, wenn du ihn verlangt hast. Behebe die Ursache und führe das Skript erneut aus: Die Schritte lassen sich gefahrlos wiederholen. So verlässt du den Wartungsmodus ohne Aktualisierung, wenn `my schema apply` nicht gelaufen ist:

```sh
gcloud run services update my --project="$PROJECT" --region=asia-northeast1 --remove-env-vars=MY_MAINTENANCE
```

Um nach einer Schemaänderung zurückzugehen, stelle das Backup wieder her: siehe [Zurückrollen](database.md#rolling-back).

<!-- source: monitoring-and-alerts 67bc34d23fdf -->
## Überwachung und Alarme

Das Modul [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) fügt einen Uptime-Check von `/readyz`, Alert-Policies mit den Namen [des Runbooks](alerts.md) und E-Mail-Benachrichtigungen hinzu. Es wurde noch nie angewendet, probiere es also einmal aus, indem du einen Check scheitern lässt. Aktiviere zuerst `monitoring.googleapis.com` und lies in seiner README, was der Check kostet: Jeder Lauf weckt eine Instanz und über `/readyz` die Datenbank, was zu [den Abwägungen des Ticks](#costs) hinzukommt.

<!-- source: changing-a-backend ef4660f60191 -->
## Ein Backend wechseln

Ersetze den Block des Moduls in `main.tf`. Das App-Modul nimmt, was der Block zurückgibt, daher ändert sich sonst nichts. Eine Datenbank zu wechseln heißt auch, ihre Daten zu verschieben, da eine neue leer ist: [`db-gcp-cloud-sql`](../../deploy/opentofu/modules/db-gcp-cloud-sql/README.md#coming-from-db-external) hat die Schritte. Zum Beispiel eine eigene Datenbank:

```hcl
module "db" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/db-external?ref=v0.1.0&depth=1"

  url_secret_id = "another-database-url"
}
```

Für einen Speicher außerhalb von Google Cloud ersetze den `blob`-Block durch [`blob-s3`](../../deploy/opentofu/modules/blob-s3/README.md), für jeden S3-kompatiblen Dienst, oder [`blob-cloudflare-r2`](../../deploy/opentofu/modules/blob-cloudflare-r2/README.md).

<!-- source: when-you-need-a-network fb392704a1e7 -->
## Wenn du ein Netzwerk brauchst

Dieser Leitfaden braucht keines: Neon, Cloud Tasks, Cloud Storage und Pub/Sub werden alle über das Internet mit der eigenen Identität des Dienstes erreicht, sodass Cloud Run an nichts angebunden wird und es nichts zu bezahlen oder zu pflegen gibt.

Du brauchst eines, um Memorystore in die Bereitstellung aufzunehmen, da es nur private Adressen hat, oder um Cloud SQL eine private Adresse anstelle seiner öffentlichen zu geben. Füge [`network-gcp-vpc`](../../deploy/opentofu/modules/network-gcp-vpc/README.md) hinzu oder beschreibe ein vorhandenes VPC als Objekt [derselben Form](../../deploy/opentofu/README.md#the-network-on-google-cloud), und gib dasselbe an die App und an jedes Modul weiter, das es entgegennimmt:

```hcl
module "network" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/network-gcp-vpc?ref=v0.1.0&depth=1"

  project_id = var.project_id
  region     = var.region
}

module "kv" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/kv-gcp-memorystore?ref=v0.1.0&depth=1"

  project_id = var.project_id
  region     = var.region
  network    = module.network
}
```

Gib der App ebenfalls `network = module.network` und füge `module.kv` zu ihren `backends` hinzu. Nur private Bereiche laufen durch das Netzwerk, daher erreicht der Dienst das Internet wie zuvor, ohne Cloud NAT. Ein Backend auf privaten Adressen mit einer App in einem anderen oder gar keinem Netzwerk lässt den Plan scheitern. Aktiviere zuerst die Service-Networking- und die Network-Connectivity-API sowie die Compute-Engine-API. Ein Netzwerk verlängert den Kaltstart einer Instanz: Lies [was das private Netzwerk hinzufügt](../../deploy/opentofu/modules/kv-gcp-memorystore/README.md#what-the-private-network-adds).

<!-- source: splitting-jobs-into-workers ed661aede0a3 -->
## Jobs auf Worker aufteilen

Wenn eine Art von Job den Rest aufhält, gib ihr einen eigenen Dienst. Entscheide anhand der Metriken, was du aufteilst, wie [Worker nach Job-Typ aufteilen](running.md#splitting-workers-by-job-type) es beschreibt. Ein Wert geht an die Queue und an die App, und jede Queue, URL, Route und Einstellung ergibt sich aus seinen Schlüsseln, sodass du nichts davon schreibst:

```hcl
locals {
  workers = {
    delivery = {
      job_types     = ["federation.fanout", "federation.deliver"]
      max_instances = 20
      concurrency   = 16
    }
  }
}
```

Setze `workers = local.workers` am `queue`-Block und am `app`-Block. Der Plan weist eine Aufteilung der Typen ab, die zusammenbleiben müssen, ebenso eine Queue und eine App mit unterschiedlichen Werten. Er braucht außerdem einen Broker, der jeden Prozess erreicht, was `stream-gcp-pubsub` ist. Derselbe Wert geht an `monitoring-gcp`, falls du es verwendest.

Die beiden gängigen Aufteilungen dieser Seite sehen so aus. Damit Auslieferungen den Rest nicht verzögern, nimm den Wert oben. Damit E-Mails und Benachrichtigungen schnell bleiben:

```hcl
locals {
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
}
```

Lass die Typen von Einstellungen weg, die du nicht gesetzt hast, wie diese Seite es sagt: Ein Typ, den der Server nicht registriert hat, verhindert den Start, und der Plan kann nicht erkennen, welche das sind. Mit `workers` hilft der Standarddienst bei diesen Typen nicht, wenn er frei ist, da ihre Tasks in ihre eigene Queue gehen.

Jedes Feld und was ein Worker von der App hat, steht in [dem Vertrag](../../deploy/opentofu/README.md#workers). Der Dienst eines Workers nimmt keinen Traffic von außen an. `upgrade.sh` verschiebt jeden nach der Migration und vor dem Standarddienst, und ohne dass etwas es ihm sagt: Nur ein Schritt, der scheitert, hält es an. Jeder Worker kann bringen, was der Dienst kostet, und die Verbindungen seiner Serveren zählen gegen das Limit der Datenbank.

<!-- source: how-traffic-reaches-the-service d1e6d0ed195b -->
## Wie der Traffic den Dienst erreicht

Das entscheidet der `ingress`-Block in `main.tf`. Das Beispiel nutzt den Load Balancer, der für die Produktion gedacht ist. Das Domain-Mapping steht daneben in einem Kommentar, um eine Bereitstellung günstig auszuprobieren: Zum Austauschen änderst du diesen Block und `service_ingress` des `app`-Blocks, und der Plan sagt, wenn die beiden nicht übereinstimmen.

| | `ingress-gcp-load-balancer` (das Beispiel) | `ingress-gcp-domain-mapping` |
| --- | --- | --- |
| Monatliche Gebühr | Etwa 18 $ für die Forwarding-Regeln, plus eine Gebühr pro GB. Prüfe die [Preisseite](https://cloud.google.com/vpc/network-pricing): Diese Zahl ließ sich dort nicht bestätigen | Keine |
| Status | Von Google Cloud empfohlener Weg | Ein Preview-Feature, das Google Cloud wegen der Latenz nicht für die Produktion empfiehlt |
| Regionen | Alle | `asia-northeast1`, `us-central1` und einige weitere |
| DNS | Ein `A`-Record, aus `tofu output dns_records` | Die Records, die dieselbe Ausgabe auflistet |
| Zertifikat | Von Google verwaltet, aktiv, sobald DNS auf die Adresse zeigt | Von Google verwaltet, bis zu 24 Stunden |
| Domain-Inhaberschaft | Nichts über DNS hinaus | Du und jedes andere Konto, das sie anwendet, müsst vor dem Anwenden ein [verifizierter Inhaber](https://cloud.google.com/run/docs/mapping-custom-domains) sein |
| `run.app`-URL | Aus dem Internet abgewiesen, sodass niemand den Load Balancer umgeht | Für jeden erreichbar: Ein Domain-Mapping kann keinen Header hinzufügen, daher bleibt der Origin offen |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` und die Adresse des Load Balancers | `169.254.0.0/16` |
| `service_ingress` des `app`-Blocks | `internal-and-load-balancer`. Eine Abweichung vom `ingress`-Block lässt den Plan scheitern | `all` |

Cloud Tasks und Cloud Scheduler kommen über keinen von beiden: Sie rufen die Standard-`run.app`-URL direkt auf.

Um die eigene URL des Dienstes oder etwas anderes davor zu verwenden, etwa ein CDN, entferne den `ingress`-Block und seine Zeile in `backends` und lass `service_ingress` bei dem, was du brauchst: Es ist immer deine ausdrückliche Wahl, und `all` lässt die `run.app`-URL für jeden offen. Setze `MY_TRUSTED_PROXIES` in `env` auf das, was sich mit dem Server verbindet. Was auch immer du davorsetzt, muss weiterhin unter `https://{host}` antworten.

<!-- source: cloudflare 95b7a9baaff1 -->
### Cloudflare

`ingress-cloudflare-app` setzt Cloudflare vor `host`, wobei der Cache für alles umgangen wird, und `ingress-cloudflare-media` fügt neben jedem Ingress einen zwischengespeicherten Medien-Host hinzu. Lies zuerst die README jedes Moduls ([App](../../deploy/opentofu/modules/ingress-cloudflare-app/README.md), [Medien](../../deploy/opentofu/modules/ingress-cloudflare-media/README.md)). Der Token kommt in `CLOUDFLARE_API_TOKEN`. Diese Module sind nur auf gemockten Providern gelaufen.

- **Dein Tarif bestimmt den Origin.** Cloud Run routet nach `Host`, und Cloudflare bietet das Überschreiben des Host-Headers nur im Enterprise-Tarif an. In jedem anderen Tarif lässt ein `*.run.app`-Origin den Plan scheitern, daher muss der Origin deinen Host selbst akzeptieren: Behalte ein `ingress-gcp-domain-mapping` dieses Hosts, außerhalb von `backends`, und setze `origin_host = "ghs.googlehosted.com"`. Jeder Host braucht ein eigenes Mapping. Ob sein Zertifikat hinter dem Proxy ausgestellt und erneuert wird, ist ungeprüft, beginne also mit `proxied = false`.
- **Ein Header schließt den Origin.** `ingress-cloudflare-app` fügt eine Transform-Regel für einen Request-Header hinzu, und der Server weist eine Anfrage ohne den Header mit `403` ab, abgesehen von Health-Checks und den eigenen `POST`s der Jobs, die ihre Token prüfen. So geht niemand über die `run.app`-URL an Cloudflare vorbei, und ein eigener Cloudflare Worker eines Aufrufers kann die Client-Adresse, die die Ratenbegrenzungen sehen, nicht wählen.
- **Der Wert landet im State.** Der Cloudflare-Provider hat keine Write-only-Form für den Header-Wert einer Regel. Generiere einen mit `openssl rand -hex 32`, füge ihn mit `gcloud secrets versions add` zu einem Secret hinzu, gib denselben Wert mit `TF_VAR_origin_secret` mit und halte den State privat. [Der CDN-Leitfaden](cdn.md#rotate-the-value) beschreibt, wie du ihn rotierst.
- **Nichts fügt den Header hinzu, solange ein Record nicht geproxyt ist**, daher weist der Server alles außer seinen Health-Checks ab: Setze bei `proxied = false` für das Zertifikat eines Domain-Mappings das Secret danach auf.
- **Schalte Bot Fight Mode aus** im Dashboard. Er gilt für die ganze Zone, keine Regel kann ihn überspringen, und eine Challenge auf `/ap/`, `/.well-known/` oder `/api/` bricht die Föderation, weil andere Server keine Browser sind. Jobs laufen nicht über Cloudflare. Die Module fügen keine Challenge hinzu und schalten Browser Integrity Check aus, verwalten Bot Fight Mode aber nicht. Schalte aus demselben Grund „I'm Under Attack“ nicht ein.
- **Schalte Always Use HTTPS ein** im Dashboard, wenn `http://` umgeleitet werden soll. Es gilt für die ganze Zone, daher überlassen die Module es dir.
- **Timeouts**: Cloudflare antwortet mit `524`, wenn der Origin 125 Sekunden lang nichts sendet. Jobs sind nicht betroffen, da sie die `run.app`-URL direkt aufrufen. Streams senden alle 30 Sekunden einen Heartbeat, unter dem Leerlauflimit von 900 Sekunden. Ob es Streams auf einem gewöhnlichen geproxyten Host puffert, ist ungeprüft.
- **Uploads**: 100 MB in Free und Pro, über den 32 MiB von Cloud Run, die das Limit bleiben.
- **Video**: Die Bedingungen von Cloudflare schränken das Ausliefern von Video und eines unverhältnismäßig großen Anteils großer Dateien in Free, Pro und Business ein, und es kann die Inhalte umleiten oder andere Maßnahmen ergreifen. Entscheide das, bevor du Video auf dem Medien-Host zwischenspeicherst.

**Eine gelöschte Datei bleibt im Cache.** Der Medien-Host folgt dem `Cache-Control` des Servers, daher kann eine gelöschte lokale Datei bis zu einem Jahr sichtbar bleiben und eine entfernte für `remote_media_cache_ttl_seconds`. Der Server leert sie nicht. Wenn etwas verschwinden muss, leere sie von Hand, unter **Caching > Configuration > Custom Purge** im Dashboard, oder:

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

Eine Proxy-URL trägt ihr `sig` im Query-String, der Teil dessen ist, was zwischengespeichert wird, gib also die ganze URL an.

<!-- source: changing-the-kek-backend c5e918ea5c81 -->
## Das KEK-Backend wechseln

Den `kek`-Block zu ersetzen ist ein Block in `main.tf`, aber bei einem Server mit Daten ist es nicht umsonst. Die Datenbank enthält Secrets, die mit dem aktuellen Schlüssel umhüllt sind, sodass ein Server, der ihn verliert, sie nicht mehr lesen kann, und die Schlüssel jedes Kontos gehören dazu.

Folge [Zwischen dem lokalen KEK und einem KMS wechseln](key-management.md#move-between-the-local-kek-and-a-kms). Vom Secret zu Cloud KMS behältst du den aktuellen KEK als `kek_previous`, während die Secrets umziehen, indem du sein Secret über das App-Modul weitergibst, mit `secret_files = { MY_KEK_PREVIOUS_FILE = "my-kek" }`; führe `my kek rewrap` über [die Konsole](#the-console) aus und entferne ihn dann, wenn `my kek status` keine `local:`-Schlüssel mehr zählt. Behalte den KEK so lange, wie du Datenbank-Backups von vor dem Rewrap aufbewahrst.

Auf einer neuen Server ohne Daten muss nichts umziehen.

<!-- source: limits 5bd276dd3a89 -->
## Limits

- **Uploads**: Cloud Run weist eine HTTP/1-Anfrage über 32 MiB ab, und der Server spricht HTTP/1.1. Das Beispiel setzt daher `MY_MEDIA_MAX_AV_BYTES` auf 30 MiB, gegenüber dem Standard von 100 MiB, damit ein größerer Upload die eigene Antwort des Servers bekommt.
- **Streams**: Das Request-Timeout, standardmäßig 15 Minuten, beendet einen offenen Stream, und der Client verbindet sich erneut.
- **Client-Adressen**: Die Ratenbegrenzungen stützen sich auf `MY_TRUSTED_PROXIES`, das das App-Modul aus dem setzt, was das `ingress`-Modul zurückgibt. Kein Dokument, das wir gelesen haben, sagt, was Cloud Run bei einer direkten Anfrage zu `X-Forwarded-For` hinzufügt oder welche Peer-Adresse ein Container sieht, und der Container-Vertrag sagt zu beidem nichts, daher ist `169.254.0.0/16` eine Annahme. Die dokumentierte Form des externen Load Balancers ist `[<supplied>,]<client-ip>,<lb-ip>`. Prüfe die Adressen in den ersten Request-Logs: Eine Client-IP, die eine `169.254`-Adresse oder eine Google-Adresse ist, bedeutet, dass die Einstellung für dein Projekt falsch ist.
- **Öffentlicher Zugriff**: Der Dienst gewährt `roles/run.invoker` an `allUsers`, was eine Organisationsrichtlinie zur domänenbeschränkten Freigabe abweist. Ein solches Projekt braucht eine Ausnahme.

<!-- source: costs 1ab8d07f6847 -->
## Kosten

Ein Server kostet Geld. Diese Zahlen sind ein Anhaltspunkt dafür, was dich erwartet und wo Sorgfalt sie senkt, keine Grenzen, an die sich die Bereitstellung hält. Es sind ungefähre Listenpreise für eine Region, von 2026-10 und 2026-11. Prüfe sie auf den Preisseiten von [Cloud Run](https://cloud.google.com/run/pricing) und [Neon](https://neon.com/pricing). Compute ist pro Rechnungskonto bis zu 180.000 vCPU-Sekunden und 360.000 GiB-Sekunden im Monat kostenlos.

| Teil | Leerlauf | Was dazukommt |
| --- | --- | --- |
| Load Balancer | Etwa 18 $ im Monat, auf der Preisseite nicht bestätigt | Eine Gebühr pro GB; siehe [Wie der Traffic den Dienst erreicht](#how-traffic-reaches-the-service) |
| Neon, im Launch-Tarif | Etwa 19 $ im Monat, mit dem Standard-Tick | Speicher zu 0,35 $ pro GB-Monat. `db-gcp-cloud-sql` hat stattdessen feste Kosten von etwa 9,40 $ im Monat, für eine kleinere Maschine: Seine README hat die Aufschlüsselung |
| Cloud Run | 0 $: min instances ist 0, und die CPU wird nur während Anfragen abgerechnet | Jeder offene Stream hält eine Instanz in Rechnung; siehe unten |
| Cloud Tasks | 0 $ | 1 Million Operationen frei, danach 0,40 $ pro Million. Ein Job braucht etwa 2 |
| Cloud Scheduler | 0 $: 3 Jobs frei pro Rechnungskonto | 0,10 $ pro Job darüber |
| Cloud Storage | Etwa 0,02 $ pro GiB im Monat | 5 GiB sind nur in `us-east1`, `us-west1` und `us-central1` frei. Medien gehen über Cloud Run hinaus |
| Pub/Sub | 0 $: 10 GiB frei | Nur solange Streams offen sind |
| Secret Manager | Cent-Beträge | Einige freie Versionen, danach ein paar Cent pro Version |

**Der Tick hält Neon wach.** Jeder Tick liest und schreibt die Datenbank, und Neon stoppt eine Compute-Instanz erst nach 5 Minuten ohne Aktivität, daher stoppt sie mit dem Standard `* * * * *` nie. Das sind 730 Stunden im Monat oder 182,5 CU-Stunden bei 0,25 CU, was die 19 $ oben ergibt. Der Tick bleibt bei jeder Minute, damit periodische Jobs pünktlich laufen und ein verlorenes Weck-Signal bald wieder aufgeholt wird.

**Ein offener Stream hält eine Instanz in Rechnung.** Bei Abrechnung nach Anfragen wird eine Instanz abgerechnet, solange sie irgendeine Anfrage bearbeitet, und die Echtzeit-Aktualisierungen sind eine lange Anfrage. Ein Client, der seinen Stream den ganzen Monat offen hält, kostet eine Instanz rund um die Uhr: etwa 60 $ im Monat bei 1 vCPU und 512 MiB, nach dem Freikontingent. Mehr Streams teilen sich die Server, bis `concurrency`. Wenn Streams die meiste Zeit offen bleiben, vergleiche mit der Abrechnung nach Serveren oder mit einer kleinen VM und [Docker Compose](deploy-docker-compose.md).

<!-- source: where-care-reduces-it 5711f378aadb -->
### Wo Sorgfalt die Kosten senkt

- **Das Domain-Mapping** anstelle des Load Balancers nimmt die 18 $ weg, mit [den Abstrichen oben](#how-traffic-reaches-the-service). Es eignet sich, um eine Bereitstellung auszuprobieren.
- **Der Free-Tarif von Neon** nimmt die 19 $ weg, und ein kleiner Server kann darauf laufen, mit diesen Abstrichen:
  - Er erlaubt 100 CU-Stunden im Monat, die der Standard-Tick nach etwa 17 Tagen aufbraucht. Bleib darin mit einem längeren Zeitplan, etwa `tick_schedule = "*/10 * * * *"`: höchstens die Hälfte der Zeit wach, etwa 91 CU-Stunden, bevor irgendetwas anderes sie weckt. Periodische Jobs laufen dann bis zu 10 Minuten zu spät, und ein verlorenes Weck-Signal wird später aufgeholt.
  - Ein föderierter Server empfängt rund um die Uhr Auslieferungen, die auch die Datenbank wecken, sodass ein ausgelasteter Server ihm unabhängig vom Zeitplan entwächst.
  - Er fasst 1 GB pro Projekt und einen 6-Stunden-Verlauf zum Wiederherstellen, und seine Compute-Instanz stoppt nach 5 Minuten Leerlauf immer, sodass die erste Anfrage danach auf sie wartet.
