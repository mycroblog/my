<!-- source: deploying-with-opentofu 178b85dbd9bd -->
# Mit OpenTofu bereitstellen

Was jede Bereitstellung mit den OpenTofu-Root-Modulen gemeinsam hat: [Auf Google Cloud bereitstellen](deploy-google-cloud.md) und
[Auf AWS bereitstellen](deploy-aws.md) ergänzen, was ihre Plattform braucht. Beide Module brauchen
[OpenTofu](https://opentofu.org) 1.11 oder neuer. Terraform wird nicht unterstützt.

<!-- source: the-root-modules 4e0bf528bd0e -->
## Die Root-Module

Ein Block eines Root-Moduls stellt den ganzen Server bereit: den Dienst, seine Jobs und ein Backend für jede Rolle.

| Root-Modul | Plattform | Beispiel zum Kopieren |
| --- | --- | --- |
| `mycroblog-gcp` | Cloud Run auf Google Cloud | `examples/gcp-cloud-run-neon` |
| `mycroblog-aws` | ECS auf Fargate, auf AWS. Es wurde noch nie angewendet | `examples/aws-ecs-neon` |

Ein Beispiel besteht aus dem Provider der Plattform und einem Block, `module "mycroblog"`, dessen Quelle eine auf ein Release
festgelegte Git-Quelle ist:

```hcl
module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-gcp?ref=v0.1.0&depth=1"

  host = "social.example.org"
  # the sections, below
}
```

Die Festlegung ist Pflicht: Ein nicht festgelegtes Modul könnte die Ressourcen des KEK unter dir ändern. Das Server-Image ist standardmäßig das
Release, das `ref` nennt, also bestimmt allein `ref`, was läuft, und es zu erhöhen ist ein Upgrade. Das Modul gibt es erst ab dem ersten
Release, das es enthält.

<!-- source: sections 728259c0b297 -->
### Abschnitte

Jede Rolle ist ein Abschnitt des Blocks, ein Objekt, dessen `kind` das Backend wählt, etwa `database = { kind = "external" }`.
Jedes Feld hat einen Standardwert, die empfohlene Einrichtung, du schreibst also nur, was du änderst. Eine Kombination, die nicht
funktionieren kann, scheitert vor dem Plan, mit einer Meldung, die die Eingaben nennt. [Die Referenz der Module](../../deploy/opentofu/README.md#sections)
listet die Abschnitte, Arten und Felder jedes Moduls auf.

<!-- source: other-settings-and-secrets 112fdabc5bb0 -->
### Andere Einstellungen und Secrets

`settings` nimmt jede andere Einstellung der [Konfigurationsreferenz](../reference/configuration.md) mit ihrem Schlüssel auf, und das
Modul gibt sie an den Dienst, die Worker und die Jobs weiter. `secrets` nimmt die Schlüssel geheimer Einstellungen auf: Für jeden legt das
Modul ein leeres Secret `<name>-<key>` an, mit Bindestrichen statt Unterstrichen, gibt es an den Server weiter und führt es zum Ausfüllen auf.

```hcl
  settings = { translator_deepl_url = "https://api.deepl.com" }
  secrets  = ["translator_deepl_api_key"]
```

Ein Schlüssel, den ein Abschnitt setzt oder das Modul festlegt, lässt den Plan scheitern, ebenso ein Schlüssel, den das Modul nicht annimmt, etwa `listen`:
Der Fehler nennt den Grund. Ein Abschnitt, der null ist, etwa `translation`, und ein Signal von `observability`, das auf `settings` steht,
setzen nichts, daher darf `settings` ihre Schlüssel nennen. Fehlerberichte an Sentry zum Beispiel:

```hcl
  observability = { errors = "settings" }
  settings      = { errors_exporter = "sentry" }
  secrets       = ["errors_sentry_dsn"]
```

Einstellungen, die du von Hand auf der Plattform änderst, halten nicht: Das nächste Apply stellt wieder her, was der Block sagt.

<!-- source: secrets d58c587dc144 -->
### Secrets

Kein Secret-Wert, den du angibst, erreicht den OpenTofu-Plan oder den State. Das Modul erzeugt, was es kann, über Write-only-Attribute:
den KEK, den VAPID-Schlüssel und das Passwort einer Datenbank, die es anlegt. Die einzige Ausnahme ist das Origin-Secret von
`ingress.kind = cloudflare`, das die Regel von Cloudflare hält, sodass es im State landet, der privat bleiben muss.

Die Secrets, die nur du hast, etwa die URL einer externen Datenbank, ein SMTP-Passwort, ein Zugriffsschlüssel oder die Schlüssel von
`secrets`, legt das Modul leer an. `tofu output secrets_to_fill` listet jedes auf, mit dem, was hineingehört, und dem Befehl, der es
hinzufügt. Ein Server, dessen Secret keinen Wert hat, startet nicht. Der Server liest seine Secrets, wenn eine Instanz oder ein Task startet,
ein neuer Wert wirkt also beim nächsten.

**OpenTofu erzeugt den KEK einmal und nie wieder.** Das Secret und seine Version haben `prevent_destroy`, und die Version, die
einen neuen Wert schreiben würde, ist im Modul festgelegt: Ein neuer zufälliger KEK macht alles unlesbar, was der alte verschlüsselt hat. Um ihn zu
rotieren, folge [Den KEK rotieren](key-management.md#rotate-the-kek) und füge den neuen Wert mit der CLI der Plattform hinzu, die
OpenTofu nicht verfolgt.

<!-- source: first-deployment 9ae06fd14fc1 -->
## Erste Bereitstellung

Du brauchst `tofu` 1.11 oder neuer, `jq` und die CLI der Plattform (`gcloud` oder `aws`) dort, wo du anwendest, da ein Apply die Migration von
dort ausführt; eine Domain für [`host`](../reference/configuration.md#host), die sich später nicht ändern lässt
([Einstellungen, die sich später nicht ändern lassen](production-checklist.md#settings-you-cannot-change-later)); und eine Version von `my` aus den
[Releases](running.md#get-the-image). Es gibt kein Tag `latest`. Erledige zuerst, was die Seite der Plattform verlangt, etwa die Datenbank
anzulegen.

<!-- source: 1-copy-the-example-and-set-the-variables 8f1ba80ba8d2 -->
### 1. Das Beispiel kopieren und die Variablen setzen

Kopiere das Beispiel mit seiner Provider-Lock-Datei aus dem Release, das du bereitstellst, in ein leeres Verzeichnis:

```sh
VERSION=v0.1.0
EXAMPLE=gcp-cloud-run-neon # or aws-ecs-neon
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r "my-source/deploy/opentofu/examples/$EXAMPLE/." my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

**Setze `ref` auf dieses Release** in `main.tf`; die Kopie eines stabilen Releases hat es bereits richtig. Bei einem Prerelease setze sowohl
`ref` als auch das Image, die Version ohne ihr `v`, da das Modul eines Prereleases nicht von sich aus sein eigenes Image verwendet:
`sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf` und `image = { version = "0.1.0-rc.4" }` im Block.

Fülle `terraform.tfvars` aus und füge dem Block in `main.tf` jeden Abschnitt hinzu, den du änderst. Entscheide dich jetzt für
[`account_domain`](../reference/configuration.md#account_domain), als `settings = { account_domain = "example.org" }`, wenn
Handles eine andere Domain verwenden.

Behalte dieses Verzeichnis: Es enthält deine Einstellungen und standardmäßig den State. Der State enthält keine Secret-Werte, ist aber die einzige
Aufzeichnung dessen, was angelegt wurde: Halte ihn in einem Remote-Backend, etwa einem `gcs`- oder `s3`-Bucket, wenn mehr als eine Person anwendet.

<!-- source: 2-create-the-secrets-and-add-their-values 9f13e8109bc9 -->
### 2. Die Secrets anlegen und ihre Werte hinzufügen

```sh
tofu init
SCRIPTS=.terraform/modules/mycroblog/deploy/opentofu
```

`tofu init` holt das Modul und seine Skripte in diesem Release, unter dem Namen deines Blocks: `migrate.sh`, das ein Apply
ausführt, und `console.sh`, das [die Konsole](#the-console) ausführt.

Wende zuerst nur die Secrets an, was sie anlegt und den KEK und den VAPID-Schlüssel erzeugt, mit dem Modul der Art des KEK:

| Root-Modul | Apply |
| --- | --- |
| `mycroblog-gcp` | `tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secret_manager` |
| `mycroblog-aws` | `tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secrets_manager` |

Mit `kek = { kind = "kms" }` nimm als Ziel `module.mycroblog.module.kek_kms` statt des Secrets des KEK.

**Kopiere jetzt den KEK** und bewahre die Kopie getrennt von der Datenbank auf, zum Beispiel in einem Passwortmanager: Ohne ihn lässt sich ein
[Backup](backup.md) nicht wiederherstellen, und die Schlüssel der Konten gehen verloren. `tofu output kek` zeigt den Befehl. Kopiere den VAPID-Schlüssel,
`<name>-vapid-key`, genauso: Sein Verlust beendet jedes Web-Push-Abonnement.

Führe dann jeden Befehl von `tofu output secrets_to_fill` mit seinem Wert aus. Füge alle hinzu, bevor du den nächsten Schritt machst.

<!-- source: 3-apply b9f715c7b55d -->
### 3. Anwenden

```sh
tofu apply
```

Das legt den Migrationsjob mit allem an, wovon er abhängt, führt darin über `migrate.sh` `my upgrade` aus und wartet darauf:
`my config check`, `my db upgrade -check`, `my schema apply` und `my db upgrade`, die beim ersten Fehlschlag anhalten. Seine
Ausgabe erscheint in der des Apply. Der erste Befehl, der die Datenbank verwendet, trägt `host` und `account_domain` darin ein. Erst dann
legt das Apply die Dienste an, deren Health Check fehlschlägt, bis das Schema existiert.

Schlägt die Migration fehl, hält das Apply vor den Diensten an. Behebe die Ursache, oft ein Secret ohne Wert, und führe
`tofu apply` erneut aus: Es führt die Migration erneut aus.

<!-- source: 4-point-the-domain-at-the-server b7b1ead901bc -->
### 4. Die Domain auf den Server richten

`tofu output dns_records` listet die Einträge auf, die du bei deinem DNS-Anbieter hinzufügen musst, und ist leer, wo das Modul sie anlegt.

<!-- source: 5-create-the-first-administrator bb599aec43c2 -->
### 5. Den ersten Administrator anlegen

Die Konsole hat kein Terminal und keine Standardeingabe, daher gibt `my admin create` einen Link aus, der das Passwort festlegt, statt
eines zu lesen:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

Öffne den Link sofort und lege das Passwort fest. Er funktioniert einmal, für eine Stunde, und **bleibt bis dahin in den Logs der Plattform**,
wo ihn jeder verwenden kann, der sie lesen kann. Läuft er ab, führe `my admin account password-reset-link @alice` über die
Konsole aus. Schalte dann die Zwei-Faktor-Authentifizierung ein.

<!-- source: 6-go-through-the-checklist 12999c41613d -->
### 6. Die Checkliste durchgehen

Gehe die [Checkliste für den Produktivbetrieb](production-checklist.md) durch, beginnend mit [E-Mail](production-checklist.md#email): SPF,
DKIM und DMARC auf der Domain von `mail.from`.

<!-- source: the-console 1b3d6dfa9c3c -->
## Die Konsole

Die Konsole führt einen `my`-Befehl mit dem Image, den Einstellungen und den Secrets des Dienstes aus. Sie existiert nur, solange
`console_enabled` eingeschaltet ist, schalte sie also aus, wenn du fertig bist:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` wartet auf den Befehl, gibt seine Ausgabe aus, die es aus den Logs der Plattform zurückliest, und endet mit seinem Exit-
Code. Es verwendet das Image, das der Dienst ausführt: Nach einem Upgrade, das bei der Migration angehalten hat, ist das das vorherige Release,
zu dem die Datenbank noch passt.

Der Befehl hat keine Standardeingabe und kein Terminal, ein Befehl, der ein Passwort oder eine Datei von dort liest, kann also auf diese Weise
nicht laufen. Was er ausgibt, bleibt in den Logs, führe also keinen Befehl aus, dessen Ausgabe du dort nicht schreiben würdest.

<!-- source: upgrading 4be8a01734ce -->
## Upgrade

Lies zuerst [Upgrade](database.md#upgrading) und die Release-Notizen und gehe jeweils eine Minor-Version weiter. Ein Upgrade ist ein
Apply des neuen Releases:

```sh
# 1. Back up the database while nothing writes: see "The maintenance mode" below, and Backup and restore.
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Name the new release in main.tf, as ?ref=v0.2.0, and fetch it.
tofu init

# 3. Read the plan, then apply it.
tofu plan
tofu apply
```

Lies den Plan: Das Modul eines neuen Releases kann auch Ressourcen ändern. Das Apply tut Folgendes und hält beim ersten Schritt an, der fehlschlägt:

1. Gibt den Migrations- und Konsolen-Jobs (auf AWS den Task-Definitionen) das neue Image, mit dem Rest des Plans, von dem die Dienste nicht abhängen.
2. Führt `my upgrade` im Migrationsjob über `migrate.sh` von dort aus, wo du anwendest, aus und wartet darauf, wie bei der
   [ersten Bereitstellung](#3-apply). Es wartet zuerst auf jeden Lauf, den ein unterbrochenes Apply hinterlassen hat.
3. Stellt jeden Worker auf das neue Image um, dann den Dienst. Eine neue Revision nimmt erst Traffic an, wenn ihr Health Check besteht.

**Der Ort, an dem du anwendest,** braucht `jq`, die CLI der Plattform und Zugangsdaten, die den Job ausführen und seine Logs lesen dürfen, was die
Seite der Plattform auflistet. Dasselbe gilt für einen CI-Job, der anwendet.

**Wenn ein Schritt fehlschlägt,** bleiben die Dienste auf ihren Revisionen, und die Release-Notizen oder die Ausgabe der Migration sagen, warum. Behebe
die Ursache und führe `tofu apply` erneut mit derselben Version aus: Es führt die Migration erneut aus, und die Schritte, die abgeschlossen wurden, finden
nichts mehr zu tun. Ein Apply desselben Images führt nichts aus. Um nach einer Schema-Änderung zurückzugehen, stelle das Backup wieder her: siehe
[Zurückrollen](database.md#rolling-back). Ein Apply einer früheren Version hält bei der Migration an, da dieses Release ein
neueres Schema ablehnt, und ändert keinen Dienst.

Um einen Release Candidate auszuprobieren oder das Image eines früheren Releases unter einem neueren Modul zu behalten, setze die Version selbst, das
Image-Tag ohne das `v` des Releases:

```hcl
  image = { version = "0.2.0-rc.1" }
```

<!-- source: checking-the-sql-first 84178f101ce0 -->
### Zuerst das SQL prüfen

Der Plan zeigt das SQL der Migration nicht. Um es vor dem Apply zu lesen, wende den Migrationsjob allein mit dem neuen
Image an, führe darin `my upgrade -dry-run` aus, was nichts ändert, und lies seine Logs. Die Seite der Plattform hat die Befehle.

<!-- source: destructive-changes 0bea88d60224 -->
### Destruktive Änderungen

Verlangen die Release-Notizen `-allow-destructive`, nenne diese Version im Block:

```hcl
  upgrade = { allow_destructive_for = "0.3.0" }
```

`my upgrade` erhält `-allow-destructive` nur beim Apply dieser Version, sodass ein stehengebliebener Wert kein späteres Release
Daten löschen lässt.

<!-- source: pinning-the-image-by-digest 9c3f49e5ee10 -->
### Das Image per Digest festlegen

Die [Checkliste für den Produktivbetrieb](production-checklist.md#operations) empfiehlt einen Digest, aus den Release-Notizen:

```hcl
  image = { digest = "sha256:..." }
```

Ein Digest benennt ein Image, was auch immer das Tag daneben sagt, ändere ihn also in derselben Bearbeitung wie `ref`: Ein Digest, der vom
vorherigen Release übrig ist, behält das Image dieses Releases unter der neuen Version.

<!-- source: the-maintenance-mode 57bd5487b045 -->
### Der Wartungsmodus

Der [Wartungsmodus](maintenance.md) braucht drei Applies, da sich die Dienste vor dem Upgrade und danach ändern. Füge ihn
den Einstellungen des Blocks hinzu und wende an, mache das Backup, erhöhe `ref` und wende an, dann entferne ihn und wende an:

```hcl
  settings = { maintenance = "true" }
```

Ohne ihn bedient die alte Revision weiter während der Migration, was
[nicht garantiert funktioniert](database.md#the-server-during-an-upgrade).

<!-- source: splitting-jobs-into-workers 6336c0873c82 -->
## Jobs auf Worker aufteilen

Hält eine Art von Job den Rest auf, gib ihr einen eigenen Dienst. Entscheide anhand der Metriken, was du aufteilst, wie
[Worker nach Job-Typ aufteilen](running.md#splitting-workers-by-job-type) es sagt. `workers` nimmt eine Map mit einem Schlüssel, der jeden
Worker benennt, mit seinen `job_types`, und das Modul macht daraus jede Queue, Route und Einstellung der Aufteilung, du schreibst also keine
davon. Damit Zustellungen den Rest nicht verzögern:

```hcl
  workers = {
    delivery = { job_types = ["federation.fanout", "federation.deliver"] }
  }
```

Damit E-Mails und Benachrichtigungen schnell bleiben:

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
    }
  }
```

Der Plan lehnt eine Aufteilung der Typen ab, die zusammenbleiben müssen. Lass die Typen von Einstellungen weg, die du nicht gesetzt hast, wie diese Seite
es sagt: Ein Typ, den der Server nicht registriert hat, hält den Start an, und der Plan kann nicht sehen, welche das sind. Der Standarddienst
führt jeden Typ aus, den kein Worker nennt, und keinen von denen, die einer nennt.

Ein Worker nimmt auch die Größe seines Dienstes an, jeweils die des Standarddienstes, wo sie nicht genannt ist:
[die Referenz der Module](../../deploy/opentofu/README.md#workers) listet die Felder jeder Plattform auf. Ein Worker nimmt keinen
Traffic von außen an, und ein Upgrade stellt ihn nach der Migration und vor dem Standarddienst um. Jeder Worker kann mitbringen, was
der Dienst kostet, und die Verbindungen seiner Instanzen zählen gegen das Limit der Datenbank.

<!-- source: changing-a-backend df886f733f8e -->
## Ein Backend wechseln

Ändere die `kind` seines Abschnitts. Jede Art setzt auf dem Server, was sie braucht, und der Plan sagt, was sich ändert. Die Datenbank zu wechseln
heißt auch, ihre Daten umzuziehen, da eine neue leer ist: Mache ein [Backup](backup.md) der alten und stelle es in der
neuen wieder her, während nichts schreibt.

<!-- source: the-kek 62356f591998 -->
### Der KEK

`kek.kind` zu ändern ist eine Zeile, aber auf einer Instanz mit Daten nicht umsonst: Die Datenbank hält Secrets, die mit dem
aktuellen Schlüssel verpackt sind, darunter die Schlüssel jedes Kontos, sodass eine Instanz, die ihn verliert, sie nicht mehr lesen kann. Folge
[Zwischen dem lokalen KEK und einem KMS wechseln](key-management.md#move-between-the-local-kek-and-a-kms). Behalte vom Secret zu einem KMS
den aktuellen KEK als `kek_previous`, während die Secrets umziehen: Füge `secrets = ["kek_previous"]` hinzu und fülle das Secret
`<name>-kek-previous` mit dem aktuellen KEK. Führe `my kek rewrap` über [die Konsole](#the-console) aus und entferne ihn dann, wenn
`my kek status` keine `local:`-Schlüssel mehr zählt. Behalte den KEK, solange du Datenbank-Backups von vor dem Rewrap behältst.

Auf einer neuen Instanz ohne Daten muss nichts umziehen.
