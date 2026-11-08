<!-- source: backup-and-restore 845c6a2e72dd -->
# Backup und Wiederherstellung

Was du sichern solltest, wie du jedes Backup erstellst, während der Server weiterläuft, wie du einen Server daraus wiederherstellst und wie du eine Wiederherstellung übst,
bevor du sie brauchst. Auch Upgrades erstellen ein Backup; [Upgrade](database.md#upgrading) beschreibt die Schritte drumherum.

<!-- source: what-to-back-up 916727936cca -->
## Was gesichert werden sollte

![Was gesichert werden sollte und in welcher Reihenfolge es wiederhergestellt wird: Datenbank und Medien werden zusammen gesichert, der KEK getrennt davon, und eine Wiederherstellung spielt Datenbank, Medien und Konfiguration mit demselben KEK zurück, bevor der Server startet. Die Liste unten sagt dasselbe.](images/backup-restore.svg)

Im Diagramm von oben nach unten:

- **Die Datenbank** (`database_url`: SQLite oder PostgreSQL) wird zuerst gesichert, mit `my db backup` oder `pg_dump`, jeden Tag und vor jedem Upgrade.
- **Die Medien** (`blob_fs_dir` oder der Bucket des Blob-Stores `s3` oder `gcs`) werden direkt danach gesichert, als Kopie, über die Versionen des Buckets oder über ein Replikat.
  Die beiden Backups sind eine Generation, werden zusammen aufbewahrt, getrennt vom Server und verschlüsselt.
- **Der KEK** (`MY_KEK` oder `MY_KEK_FILE`) wird einmal und nach jeder Rotation gesichert, zusammen mit `kek_previous`, und nie am selben Ort wie die Datenbank-Backups.
  Mit `key_manager = gcpkms` oder `awskms` bleibt der KEK im KMS: Lass den Schlüssel von `kms_key` aktiviert, solange du Backups aufbewahrst, die ihn brauchen
  ([Einen AWS-KMS-Schlüssel außer Betrieb nehmen](key-management.md#retire-an-aws-kms-key)).
- **Die Konfiguration** (die Umgebung, `MY_CONFIG`, `.env` und die Secrets) wird mit den Deployment-Dateien kopiert, wenn sie sich ändert, ohne den KEK.
- **Eine Wiederherstellung** erfolgt in eine leere Umgebung, in dieser Reihenfolge: zuerst die Datenbank, dann die Medien mit denselben Schlüsseln, bevor der Server startet,
  dann die Konfiguration mit demselben KEK, `kek_previous` und `host`; danach startet der Server, und du prüfst `/readyz`, `my projection verify` und die öffentlichen Schlüssel der Actors
  ([Wiederherstellen](#restoring)). Ohne den KEK startet der Server nicht, und mit einem anderen schlägt das Lesen eines Secrets mit `unknown key-encryption key` fehl
  ([Ohne den richtigen KEK](#without-the-right-kek)).
- **Kein Backup nötig** ist für den Key-Value-Store mit `kv = memory` oder `valkey` (Caches und Rate Limits), die Jobs (sie liegen in der Datenbank),
  Logs und Metriken sowie den Index einer Suchmaschine, den `my search rebuild` neu erzeugt.

| Was | Wo es liegt | Sichern | Ohne Backup verloren |
| --- | --- | --- | --- |
| Die Datenbank | die SQLite-Datei (`database_url`) oder die PostgreSQL-Datenbank | [SQLite](#sqlite) oder [PostgreSQL](#postgresql) | alles: Konten, Beiträge, Follows, Einstellungen |
| Die Medien | das Verzeichnis `blob_fs_dir` des Blob-Stores `fs`, oder der Bucket `blob_s3_bucket` des Blob-Stores `s3` bzw. `blob_gcs_bucket` des Blob-Stores `gcs` | [Medien](#media) oder [Medien in einem Bucket](#media-in-a-bucket) | die hochgeladenen Bilder-, Video- und Audiodateien sowie die zwischengespeicherten entfernten Dateien |
| Der KEK | dort, wo du `MY_KEK` oder die Datei von `MY_KEK_FILE` aufbewahrst (`secrets/kek` mit Compose; nie in der Datenbank) | [Der KEK](key-management.md#back-it-up), getrennt von der Datenbank | der private Schlüssel jedes Actors und damit die Identität des Actors im Fediverse sowie die anderen Secrets, die die Datenbank speichert |
| Die Konfiguration | die Umgebungsvariablen, die Konfigurationsdatei (`MY_CONFIG`), die Secrets (`MY_VAPID_PRIVATE_KEY`, Mail- und Zahlungsschlüssel), `.env` mit Compose | kopiere sie mit deinen Deployment-Dateien | nichts in den Daten, aber der Server kann nicht auf dieselbe Weise starten; eine Änderung von `host` macht jede föderierte URL ungültig |

Was kein Backup braucht:

- **Der Key-Value-Store** (`kv`). Mit `kv = db` ist er Teil der Datenbank. Mit `kv = memory` oder `kv = valkey` enthält er Caches, Rate-Limit-Zähler
  und kurzlebigen Zustand, etwa eine Anmeldung, die auf ihren zweiten Faktor wartet. Nach einer Wiederherstellung mit leerem Valkey füllen sich die Caches wieder,
  die Rate Limits beginnen von vorn, und eine laufende Anmeldung oder ein laufender Sprachanruf muss neu beginnen.
- **Die Job-Queue** (`queue`). Mit `queue = db` und `queue = cloudtasks` sind die Jobs Teil der Datenbank und kommen mit ihr zurück.
  Die Tasks von Cloud Tasks tragen nur ein Signal für einen Job der Datenbank; nach einer Wiederherstellung signalisiert der Server die fälligen Jobs innerhalb weniger Minuten erneut.
- **Logs und Metriken**, die der Server nicht zurückliest.
- **Der Index einer Suchmaschine**, falls du eine hast. `my search rebuild` erzeugt ihn aus der Datenbank neu. Wenn du Snapshots der Suchmaschine erstellst, enthalten sie den Text von Beiträgen, auch von Beiträgen, die nach dem Snapshot gelöscht wurden, behandle sie also wie Datenbank-Backups ([Volltextsuche](search.md#backups)).

Der VAPID-Schlüssel von Web Push ist ein Secret der Konfiguration. Ein neuer Schlüssel beendet Push-Benachrichtigungen an jedes bestehende Abo
([VAPID-Schlüssel für Web Push](key-management.md#web-push-vapid-key)), bewahre ihn also mit der Konfiguration auf.

<!-- source: how-often-and-where 17e55765e9a0 -->
## Wie oft und wo

- **Datenbank**: mindestens einmal am Tag und vor jedem Upgrade. Bewahre mehrere Generationen auf, zum Beispiel 7 tägliche, 4 wöchentliche und 3 monatliche Backups,
  damit sich ein spät bemerkter Fehler noch rückgängig machen lässt. Ein verwaltetes PostgreSQL mit Point-in-Time-Recovery deckt die Tage ab, die seine Aufbewahrung umfasst.
- **Medien**: jedes Mal zusammen mit der Datenbank (siehe [Die Reihenfolge von Datenbank und Medien](#the-order-of-the-database-and-the-media)).
- **KEK und Konfiguration**: wenn sie sich ändern. Eine KEK-Rotation ist eine Änderung: Sichere den neuen KEK, bevor du ihn verwendest, und bewahre den vorherigen
  auf, solange `kek_previous` ihn aufführt.
- **Wo**: getrennt vom Server und seiner Festplatte, in einer anderen Ausfalldomäne (anderer Rechner, andere Region, anderer Anbieter oder anderes Konto).
  Ein Backup auf der eigenen Festplatte des Servers überlebt den Verlust dieser Festplatte nicht.
- **Verschlüsselung**: Das Datenbank-Backup enthält E-Mail-Adressen, Passwort-Hashes und Direktnachrichten; verschlüssle es im Ruhezustand
  (ein verschlüsselter Bucket, oder verschlüssle die Datei vor dem Hochladen mit `age` oder `gpg`). Die darin enthaltenen Secrets, etwa die privaten Schlüssel der Actors,
  sind mit dem KEK verschlüsselt, **bewahre den KEK also nie am selben Ort wie die Datenbank-Backups auf**: Wer beides erhält, kann jedes Secret lesen.
- **Aufbewahrung**: Ein Backup enthält Daten, die Nutzer gelöscht haben oder die du auf Anfrage gelöscht hast. Lösche alte Generationen nach einem Zeitplan, den deine Datenschutz­erklärung
  nennt.

<!-- source: backing-up 0c38c68033e4 -->
## Sichern

Der Server kann während eines Backups weiterlaufen. Um ein Backup zu erstellen, das genau einem Zeitpunkt entspricht, zum Beispiel vor einem Upgrade,
wechsle zuerst in den [Wartungsmodus](maintenance.md): Im Wartungsmodus öffnet `my serve` keine Datenbankverbindung und schreibt nichts.

<!-- source: sqlite 7105c29de4c9 -->
### SQLite

`my db backup <file>` schreibt eine konsistente Kopie der SQLite-Datenbank in eine neue Datei, während der Server weiterläuft.
Die Kopie enthält jeden Schreibvorgang, der vor dem Start des Befehls committet wurde, auch die noch in der `-wal`-Datei, und keinen danach.
Es ist eine einzelne Datei, kleiner als das Original, weil freie Seiten weggelassen werden. Der Befehl lehnt eine bereits vorhandene Datei ab
und funktioniert nur mit `database = sqlite`.

```sh
my db backup /backup/my-$(date +%Y%m%d%H%M).db
```

Mit Docker Compose führst du ihn im Container des Servers aus. Die Kopie wird innerhalb des Containers geschrieben, schreibe sie also auf das Daten-Volume
und verschiebe sie von dort nach draußen:

```sh
cd deploy/docker
docker compose exec -T my my db backup /data/backup.db
docker run --rm -v my_data:/data -v "$PWD":/backup busybox sh -c 'mv /data/backup.db /backup/my-$(date +%Y%m%d%H%M).db'
```

Kopiere die Datenbankdatei nicht mit `cp` oder `tar`, während der Server läuft: Der Datei allein fehlen die Schreibvorgänge, die noch in der `-wal`-Datei stehen,
und eine Kopie, die mitten in einem Schreibvorgang entsteht, kann beschädigt sein. Eine einfache Kopie ist in Ordnung, wenn kein Prozess die Datenbank geöffnet hat
(der Server ist gestoppt oder im Wartungsmodus); kopiere dann die Dateien `-wal` und `-shm` mit.

<!-- source: postgresql 45b14a5a4970 -->
### PostgreSQL

Verwende `pg_dump` im Custom-Format, das `pg_restore` liest. Es liest einen einzelnen Snapshot, der Server kann also weiterlaufen.

```sh
pg_dump --format=custom --file=my-$(date +%Y%m%d%H%M).dump "postgres://my:...@db.example/my"
```

Mit dem Compose-Layer `compose.postgres.yaml` führst du es im Container der Datenbank aus (gib den Layer mit `-f` oder in `COMPOSE_FILE` an):

```sh
cd deploy/docker
docker compose exec -T postgres pg_dump --format=custom -U my my > my-$(date +%Y%m%d%H%M).dump
```

Verwende das `pg_dump` derselben Hauptversion wie der Server oder einer neueren.

Ein verwaltetes PostgreSQL hat eigene Backups, die für eine große Datenbank die bessere Wahl sind:

- **Cloud SQL** und **Amazon RDS** erstellen automatische tägliche Backups und stellen mit aktivierter Point-in-Time-Recovery jede Sekunde
  innerhalb der Aufbewahrungsdauer wieder her. Aktiviere beides, stelle die Aufbewahrung so ein, dass sie die gewünschten Generationen abdeckt, und erstelle vor jedem Upgrade
  ein On-Demand-Backup (ein On-Demand-Backup bei Cloud SQL, einen Snapshot bei RDS).
- Ihre Wiederherstellung erzeugt einen neuen Server (oder überschreibt einen): Richte danach `database_url` darauf.
- Diese Backups bleiben innerhalb des Kontos des Anbieters. Exportiere hin und wieder zusätzlich ein `pg_dump` an einen Ort außerhalb,
  damit der Verlust des Kontos nicht die Daten kostet.

<!-- source: media a2445f4d5501 -->
### Medien

Der Blob-Store `fs` legt jede Datei unter `blob_fs_dir` ab (`/data/blobs` mit Compose): die Uploads in `objects/` und Dateien, die gerade geschrieben werden, in `tmp/`.
Kopiere das Verzeichnis, außer `tmp/`, mit einem beliebigen Werkzeug, das Dateien kopiert (`tar`, `rsync`, ein Snapshot des Volumes oder der Festplatte).
Eine Datei wird unter `tmp/` geschrieben und dann in einem Schritt nach `objects/` umbenannt, daher enthält eine Kopie, die während des Serverbetriebs entsteht, nie eine halbe Datei in `objects/`.

```sh
docker run --rm -v my_data:/data -v "$PWD":/backup busybox \
  tar czf /backup/blobs-$(date +%Y%m%d%H%M).tar.gz -C /data --exclude=blobs/tmp blobs
```

`rsync` auf den Backup-Host hält die Kopie nach dem ersten Mal klein, da sich Mediendateien nach dem Schreiben nie ändern.

<!-- source: media-in-a-bucket 882f3a0f9a2b -->
### Medien in einem Bucket

Die Blob-Stores `s3` und `gcs` legen jede Datei als Objekt des Buckets `blob_s3_bucket` oder `blob_gcs_bucket` ab, benannt nach ihrem Schlüssel: die verarbeiteten Medien unter `media/`,
die Uploads in der empfangenen Form unter `uploads/` und die zwischengespeicherten entfernten Dateien unter `remote/`. Der Bucket bleibt privat: Der Server liest die
Dateien und liefert sie unter `/media/` aus. Ein Objekt erscheint erst, wenn es vollständig geschrieben ist, daher enthält eine Kopie, die während des Serverbetriebs entsteht, nie
eine halbe Datei. Schütze den Bucket in drei Schichten, von der günstigsten zur sichersten.

**Versionierung.** Aktiviere die Objektversionierung des Buckets, wo der Dienst sie anbietet (Amazon S3, Google Cloud Storage, MinIO und andere).
Ein gelöschtes oder überschriebenes Objekt bleibt dann als nicht aktuelle Version erhalten, und der Bucket kann auf den Zeitpunkt eines Datenbank-Backups zurückgehen.

```sh
aws s3api put-bucket-versioning --bucket my-media --versioning-configuration Status=Enabled
```

**Lifecycle-Regeln.** Mit Versionierung verursacht jede gelöschte Datei weiter Speicherkosten, bis eine Regel sie entfernt. Füge eine Regel hinzu, die nicht aktuelle Versionen löscht,
und zwar nach dem Alter deines ältesten Datenbank-Backups (35 Tage für die Generationen aus [Wie oft und wo](#how-often-and-where) ohne die monatlichen),
damit jedes Datenbank-Backup, das du aufbewahrst, seine Dateien noch findet und eine von Nutzer gelöschte Datei nicht länger bleibt, als deine Datenschutzerklärung sagt.
Füge eine Regel hinzu, die unvollständige Multipart-Uploads nach einem Tag abbricht: Der Server bricht einen fehlgeschlagenen Upload einer großen Datei selbst ab, aber ein Prozess,
der mittendrin stoppt, hinterlässt seine Teile.

```json
{
  "Rules": [
    {
      "ID": "noncurrent-versions",
      "Status": "Enabled",
      "Filter": {},
      "NoncurrentVersionExpiration": { "NoncurrentDays": 35 },
      "AbortIncompleteMultipartUpload": { "DaysAfterInitiation": 1 }
    }
  ]
}
```

```sh
aws s3api put-bucket-lifecycle-configuration --bucket my-media --lifecycle-configuration file://lifecycle.json
```

**Eine Kopie in einem anderen Konto oder einer anderen Region.** Versionierung schützt vor einem Fehler, nicht vor dem Verlust des Buckets oder des Kontos, dem er gehört,
und die eigenen Zugangsdaten des Servers können Objekte löschen. Bewahre eine Kopie auf, die diese Zugangsdaten nicht erreichen:

- **Replikation.** Amazon S3 repliziert einen versionierten Bucket in einen Bucket in einer anderen Region oder einem anderen Konto (Cross-Region oder Same-Region Replication,
  wobei das Ziel dem anderen Konto gehört). Aktiviere die Versionierung auch für das Ziel und repliziere keine Löschungen
  (lass die Replikation von Delete-Markern ausgeschaltet), damit eine Löschung auf dem Server die Kopie nicht erreicht.
  Andere Dienste haben eigene Replikation, etwa die Dual-Region-Buckets von Cloud Storage und die Bucket-Replikation von MinIO.
- **Eine geplante Kopie.** Wo es keine Replikation gibt oder um zu einem anderen Anbieter zu kopieren, kopiere den Bucket nach dem Zeitplan des Datenbank-Backups,
  direkt danach, mit Zugangsdaten, die nur den Bucket des Servers lesen und nur den Backup-Bucket beschreiben können:

  ```sh
  rclone copy --checksum source:my-media backup:my-media-backup
  ```

  `rclone copy` und `aws s3 sync` ohne `--delete` fügen neue Objekte hinzu und entfernen nie eines aus der Kopie. Dateien ändern sich nach dem Schreiben nie,
  daher überträgt jede Kopie nach dem ersten Lauf nur die neuen. Lösche alte Objekte aus der Kopie nach dem Zeitplan deiner Aufbewahrungsrichtlinie
  oder aktiviere dort ebenfalls Versionierung und eine Lifecycle-Regel.

Zugangsdaten gehören nicht in die Kopie: Der Bucket enthält kein Secret, aber die Konfiguration nennt den Schlüssel in `blob_s3_access_key_id`
und `blob_s3_secret_access_key`, die zur Konfiguration gehören. Der Blob-Store `gcs` speichert keinen Schlüssel: Er verwendet das Servicekonto
des Rechners oder von Cloud Run.

<!-- source: the-order-of-the-database-and-the-media 52b56171d6d9 -->
### Die Reihenfolge von Datenbank und Medien

Sichere **zuerst die Datenbank, dann die Medien**, und bewahre beides zusammen als eine Generation auf. Bei einem Bucket ist die Kopie oder die Replikation,
die nach dem Datenbank-Backup läuft, das Medien-Backup; mit Versionierung sind es die Versionen zum Zeitpunkt des Datenbank-Backups.

Die Datenbank nennt die Mediendateien, die sie verwendet, und die Medienkopie entsteht einen Moment später. In diesem Moment:

- Neue Uploads können in der Medienkopie landen, ohne dass es eine Zeile in der Datenbankkopie gibt. Das sind Dateien, die niemand verwendet; sie belegen Platz und schaden nicht.
- Eine Datei, die nach dem Datenbank-Backup gelöscht wurde (die Medien eines gelöschten Beitrags, ein Upload, der nie angehängt wurde), steht in der Datenbankkopie, aber nicht in der Medienkopie.
  Nach einer Wiederherstellung antwortet diese Datei mit `404`, und der Beitrag, der sie zeigte, zeigt ein fehlendes Bild.

Die andere Reihenfolge ist schlimmer: Jede Datei, die zwischen dem Medien-Backup und dem Datenbank-Backup hochgeladen wurde, würde nach einer Wiederherstellung fehlen,
auch die Medien neuer Beiträge. Beides im Wartungsmodus zu sichern vermeidet jede der Lücken.

<!-- source: the-kek-and-the-configuration b3d65ec5edb5 -->
### Der KEK und die Konfiguration

Der KEK liegt nicht in der Datenbank. Sichere ihn einmal, getrennt von den Datenbank-Backups, wie [Den Key-Encryption-Key verwalten](key-management.md#back-it-up)
beschreibt, und erneut nach jeder Rotation. Bewahre eine Kopie der Konfiguration (die Umgebung, die Konfigurationsdatei, `.env` und die Secret-Dateien)
mit deinen Deployment-Dateien auf, ohne den KEK.

<!-- source: restoring 4db94bba2bc4 -->
## Wiederherstellen

Ein Backup enthält `host` und `account_domain` seines Servers. Starte die wiederhergestellte Datenbank mit denselben Werten: Bei anderen weigert sich der Server zu starten.

Stelle in eine leere Umgebung wieder her (ein neuer Rechner, neue Volumes oder eine neue Datenbank), in dieser Reihenfolge: die Datenbank, die Medien (das Verzeichnis oder der Bucket),
dann die Konfiguration mit **demselben KEK** (und demselben `kek_previous`), der beim Erstellen des Backups in Verwendung war. Starte dann den Server wie üblich.

<!-- source: sqlite-1 62820e765e7f -->
### SQLite

Stoppe den Server, lege die Backup-Datei anstelle der Datenbankdatei ab und entferne die alten `-wal`- und `-shm`-Dateien, falls welche übrig sind.
Mit Docker Compose in das Volume `my_data` (ein vorhandenes oder ein neues auf einem anderen Rechner):

```sh
cd deploy/docker
docker compose down          # keeps the volume; on a new machine, `docker compose up --no-start` creates it
docker run --rm -v my_data:/data -v "$PWD":/backup busybox sh -c '
  rm -rf /data/my.db /data/my.db-wal /data/my.db-shm /data/blobs &&
  cp /backup/my-202601010000.db /data/my.db &&
  tar xzf /backup/blobs-202601010000.tar.gz -C /data &&
  chown -R 65532:65532 /data'
docker compose up -d
```

Das Server-Image läuft als Benutzer `65532`, dem die Dateien des Volumes gehören müssen (das `chown` oben).

<!-- source: postgresql-1 4a3134816771 -->
### PostgreSQL

Stelle in eine neue, leere Datenbank wieder her, die wie für einen neuen Server vorbereitet ist, aber ohne das Schema anzuwenden: Der Dump enthält es.

1. Erstelle die Rolle und die Datenbank.
2. Stelle den Dump wieder her. Als Superuser behalten die wiederhergestellten Tabellen ihren Eigentümer (die Rolle muss existieren); als Datenbankeigentümer füge `--no-owner` hinzu.

   ```sh
   pg_restore --exit-on-error --dbname="postgres://postgres:...@db.example/my" my-202601010000.dump
   ```

3. Mit dem eigenen Backup einer verwalteten Datenbank stellst du stattdessen auf die Weise des Anbieters wieder her und richtest `database_url` auf die wiederhergestellten Server.

Mit dem Compose-Layer `compose.postgres.yaml` startest du nur die Datenbank, auf einem neuen Volume, und stellst darin wieder her. Der Container hat die Rolle und die Datenbank erstellt:

```sh
cd deploy/docker
docker compose up -d --wait postgres
docker compose exec -T postgres pg_restore --exit-on-error --no-owner -U my -d my < my-202601010000.dump
docker compose up -d
```

Um stattdessen über eine bestehende Datenbank wiederherzustellen (ein Upgrade zurückrollen), verwende `pg_restore --clean --if-exists`, wie [Zurückrollen](database.md#rolling-back) zeigt.

<!-- source: media-1 4292939246df -->
### Medien

Entpacke die Medienkopie nach `blob_fs_dir`, bevor der Server startet, mit dem Benutzer als Eigentümer, unter dem der Server läuft.

<!-- source: media-in-a-bucket-1 2e8d8c7703b4 -->
### Medien in einem Bucket

Stelle die Objekte in einem leeren Bucket wieder her, bevor der Server startet, und setze dann `blob_s3_bucket` (und den Endpoint, die Region und die Zugangsdaten) oder `blob_gcs_bucket` darauf.
Die Schlüssel müssen gleich bleiben: Die Datenbank benennt jede Datei nach ihrem Schlüssel.

- Aus einer Kopie in einem anderen Bucket kopierst du sie zurück:

  ```sh
  rclone copy --checksum backup:my-media-backup target:my-media-restored
  ```

- Aus den Versionen des eigenen Buckets des Servers kopierst du die Objekte, wie sie zum Zeitpunkt des Datenbank-Backups waren, in einen neuen Bucket.
  `rclone` liest die Versionen dieses Zeitpunkts mit `--s3-version-at`:

  ```sh
  rclone copy --s3-version-at "2026-01-01 00:00:00" source:my-media target:my-media-restored
  ```

- Aus einem Replikat richtest du `blob_s3_bucket` oder `blob_gcs_bucket` auf das Replikat oder kopierst es in einen Bucket im ursprünglichen Konto zurück.

Die Zugangsdaten des Servers müssen Objekte des wiederhergestellten Buckets lesen, schreiben und löschen können (`s3:GetObject`, `s3:PutObject`, `s3:DeleteObject`
und `s3:AbortMultipartUpload` bei Amazon S3; die Rolle `roles/storage.objectUser` auf dem Bucket bei Google Cloud Storage).

<!-- source: starting-and-checking f656b59cc0ed -->
### Starten und prüfen

Starte den Server auf die übliche Weise. Mit Docker Compose führt `docker compose up -d` zuerst `my db upgrade -check`, das Schema und `my db upgrade` aus;
bei einem Backup derselben Version ändern sie nichts. Prüfe dann:

1. `/readyz` antwortet mit `200`, und der Server protokolliert keinen Fehler `unknown key-encryption key`.
2. `my projection verify` meldet keinen Unterschied ([Projektionen prüfen und neu aufbauen](database.md#checking-and-rebuilding-projections)).
3. Du kannst dich anmelden, und die neuesten Beiträge und ihre Medien werden angezeigt.
4. **Die Schlüssel der Actors haben sich nicht geändert**: Der `publicKey.publicKeyPem` eines Actors
   (`curl -H 'Accept: application/activity+json' https://<host>/ap/actors/<id>`) ist derselbe wie vor der Wiederherstellung.
   Andere Server prüfen die Signaturen des Servers mit diesem Schlüssel, ein unveränderter Schlüssel bedeutet also, dass die Föderation dort weitergeht, wo sie stehen geblieben ist.
5. Ein Beitrag, ein Follow oder ein Entfolgen erreicht einen anderen Server (ein Testkonto auf einem anderen Server sieht es).

Schreibvorgänge nach dem Backup gehen verloren: Beiträge, Follows und Uploads seitdem sind auf diesem Server weg, während andere Server die Aktivitäten, die sie erhalten haben, möglicherweise
noch haben. Das lässt sich von hier aus nicht reparieren; stelle aus dem neuesten Backup wieder her, das du hast.

Mit einer Suchmaschine führst du nach der Wiederherstellung `my search rebuild` aus, ohne `--keep`, damit Beiträge, die neuer sind als das Backup, den Index verlassen ([Volltextsuche](search.md#backups)).

<!-- source: without-the-right-kek 2a5031eadf18 -->
### Ohne den richtigen KEK

- **Kein KEK**: Der Server startet nicht. `my serve` und die anderen `my`-Befehle, die die Konfiguration lesen, brechen damit ab, mit
  `The required setting MY_KEK is not set.` und `kek: required when key_manager is local`.
- **Ein anderer KEK**: Der Server startet und antwortet auf `/readyz`, aber alles, was ein mit dem alten KEK verschlüsseltes Secret liest, schlägt mit
  `unknown key-encryption key` und der Schlüssel-ID des alten KEK fehl (`local key manager: key "local:…" is not configured`): Zustellungen an andere Server und signierte Abrufe (die privaten Schlüssel der Actors),
  Anmeldungen mit Zwei-Faktor-Authentifizierung, Webhook-Signaturen und die externen Anmeldeanbieter. Auch zuvor ausgestellte Media-Proxy-URLs funktionieren nicht mehr.
  Passwörter und Beiträge sind nicht mit dem KEK verschlüsselt, daher funktionieren die Anmeldung mit Passwort und das Lesen von Beiträgen weiter, was das Problem anfangs verbergen kann:
  Beobachte das Log auf den Fehler.
- Finde den KEK, dessen Schlüssel-ID der Fehler nennt, setze ihn als `kek` (oder in `kek_previous`) und starte neu. Ein Secret lässt sich ohne seinen KEK nicht wiederherstellen;
  wenn der KEK endgültig verloren ist, müssen die Schlüssel der Actors neu erzeugt werden, und andere Server sehen jeden Actor als neuen Schlüssel für dasselbe Konto.

<!-- source: recovery-drill f9adfe0e0d28 -->
## Wiederherstellungsübung

Von einem Backup weiß man erst, dass es funktioniert, wenn es wiederhergestellt wurde. Übe eine Wiederherstellung, bevor der Server öffentlich wird, und erneut nach einer Änderung daran, wie
Backups erstellt werden (mindestens alle paar Monate):

1. Erstelle wie oben ein Backup des laufenden Servers: die Datenbank, dann die Medien.
2. Stelle es auf einem anderen Rechner oder in einem anderen Compose-Projekt wieder her, mit dem KEK und der Konfiguration von dort, wo du sie aufbewahrst
   (nicht vom laufenden Server kopiert) und mit unverändertem `host`, damit die URLs der Actors übereinstimmen.
3. Lass die Kopie nicht föderieren: Halte sie vom Internet fern oder schalte den Wartungsmodus ein, sobald die folgenden Prüfungen bestanden sind.
4. Gehe [Starten und prüfen](#starting-and-checking) durch: Anmeldung, Beiträge und Medien sowie der `publicKey` einiger Actors im Vergleich mit dem laufenden Server.
5. Starte sie einmal ohne den KEK und einmal mit einem anderen und prüfe, dass sie so fehlschlägt, wie [Ohne den richtigen KEK](#without-the-right-kek) beschreibt.
6. Notiere, wie lange jeder Schritt gedauert hat: So lange bleibt der Server bei einer echten Wiederherstellung ausgefallen.
