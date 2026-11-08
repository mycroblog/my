<!-- source: importing-cloud-usage-costs db52fcd17ff9 -->
# Cloud-Nutzungskosten importieren

Der Server importiert alle 24 Stunden die Nutzungsgebühren der Anbieter, die in der Einstellung [`billing_sources`](../reference/configuration.md#billing_sources) aufgeführt sind, oder – für einen Anbieter, für den er keinen Adapter hat – [eines Plugins](#a-plugin-billing_sources--plugin) und fügt sie den Kosten des Servers hinzu. Der Standard ist leer, was nichts importiert.
Er importiert die Gebühren des laufenden und des vorherigen Abrechnungsmonats (bis zum 15. des Monats oder bis der Vormonat endgültig ist), als eine Zeile pro Dienst und Gebührenart (usage, credit, refund, tax, support, fee, other).
Solange der Anbieter einen Monat nicht abgeschlossen hat, werden die Werte als Schätzung erfasst; nach dem Abschluss als endgültig; und wenn sich ein Wert ändert, nachdem er endgültig ist, als Korrektur. Datensätze der Vergangenheit werden nie überschrieben.
Der Zustand der Importe (der letzte Erfolg, die Art des letzten Fehlers und die Zahl der Anfragen in diesem Monat) wird auf der Kostenseite der Administration und von `GET /api/v1/admin/billing_imports` angezeigt.
Die Metriken zählen jeden Versuch nach Quelle, Ergebnis und Fehlerart (`my.billing.imports`) und melden die Quellen, deren neuester Versuch fehlgeschlagen ist (`my.billing.import.failing`);
[Alerts und Runbook](alerts.md#mybillingimportfailing) enthält den Alert dazu.

In der Konfiguration dieses Servers werden keine Zugangsdaten gespeichert. Gib der Laufzeitumgebung das, was die Standard-Credential-Chain des SDK jedes Anbieters liest (bei AWS die Ausführungsrolle, Umgebungsvariablen oder gemeinsame Credential-Dateien; bei Google Cloud Application Default Credentials).

<!-- source: with-the-opentofu-modules 51ec4f78f058 -->
## Mit den OpenTofu-Modulen

Die Module [`billing-aws`](../../deploy/opentofu/modules/billing-aws/README.md) und [`billing-gcp`](../../deploy/opentofu/modules/billing-gcp/README.md) setzen `billing_sources` und erledigen die Teile dieses Leitfadens, die die API eines Anbieters erlaubt. `billing-gcp` funktioniert nur mit dem App-Modul von Google Cloud, bei AWS importieren die Module also allein die Kosten von AWS. `billing-aws` funktioniert mit den App-Modulen von AWS und mit dem von Cloud Run, wo beide in den `backends` stehen können. Keines von beiden wurde bisher auf einem echten Konto angewendet.

| Schritt in diesem Leitfaden | Mit den Modulen |
| --- | --- |
| Die IAM-Richtlinie und die Rolle aus „Von Cloud Run mit Web Identity aufrufen“ | `billing-aws` |
| Cost Explorer, Linked Account Access und Cost-Allocation-Tags aktivieren | Von Hand, in der AWS-Konsole |
| Das Dataset des Exports und die Rollen aus „IAM-Rollen“ | `billing-gcp` |
| Den Export aktivieren („Standard usage cost“) | Von Hand, in der Google-Cloud-Konsole, mit einer Rolle für das Rechnungskonto |
| Das Topic und die Pull-Subscription eines Budgets sowie die Subscriber-Rolle | `billing-gcp` mit `method = "budget"` |
| Das Budget anlegen und mit dem Topic verbinden | Von Hand, in der Google-Cloud-Konsole, mit einer Rolle für das Rechnungskonto |
| Die BigQuery- oder Pub/Sub-API aktivieren | Von Hand, wie in [Bereitstellung auf Cloud Run](deploy-cloud-run.md#1-enable-the-apis) |
| Ein Plugin | Nicht durch diese Module. Setze ohne Billing-Modul in den `backends` `MY_BILLING_SOURCES = plugin` und die Einstellungen [des Plugins](#a-plugin-billing_sources--plugin) im `env` des App-Moduls; neben einem Billing-Modul lehnt das App-Modul `MY_BILLING_SOURCES` im `env` ab, füge also ein eigenes Backend hinzu, das `billing_sources = ["plugin"]` zurückgibt ([die Felder eines Backends](../../deploy/opentofu/README.md)) |

<!-- source: aws-billing_sources--aws 9e5fa0132ca3 -->
## AWS (`billing_sources = aws`)

Der Server ruft `GetCostAndUsage` von AWS Cost Explorer einmal pro Abrechnungsmonat (ein UTC-Kalendermonat) auf, gruppiert nach Dienst (`SERVICE`) und Datensatzart (`RECORD_TYPE`). Der Endpunkt ist immer `us-east-1`.
Beträge sind in USD. Solange Cost Explorer einen Monat als geschätzt markiert (`Estimated`), erfasst der Server den Betrag bis zum Vortag als Schätzung, und sobald er das nicht mehr tut, als endgültig.
Eine Binärdatei, die mit dem Build-Tag `no_billing_aws` gebaut wurde, lehnt `aws` ab (siehe [build-tags.md](../reference/build-tags.md)).

<!-- source: cost-and-limits a555f91984c3 -->
### Kosten und Limits

Die Cost-Explorer-API berechnet 0,01 $ für jede seitenweise Anfrage. Ein kleines Konto braucht pro Tag eine Anfrage mit einer Seite, etwa 0,31 $ im Monat.
Um unerwartete Kosten zu vermeiden, zählt der Server die Anfragen pro UTC-Tag (`billing_aws_max_requests_per_day`, Standard 4) und pro UTC-Kalendermonat (`billing_aws_max_requests_per_month`, Standard 120 = 1,20 $) und hält an, ohne aufzurufen, sobald ein Limit erreicht ist.
Geplante Importe, Job-Wiederholungen und „Jetzt importieren“ in der Administration zählen alle. Ein Import, der ein Limit erreicht, wird als Fehler der Art `request_cap` erfasst, und der Import des nächsten Tages (beim Tageslimit) oder des ersten Tages des nächsten Monats (beim Monatslimit) versucht es erneut.
Bevor er aufruft, reserviert ein Import die Anfragen, die die Limits übrig lassen, sodass gleichzeitig laufende Importe die Limits nicht gemeinsam überschreiten können.
Importe eines Anbieters laufen nacheinander: Ein Import, der startet, während ein anderer noch nicht fertig ist, versucht es eine Minute später erneut, und ein Job, der bereits erfolgreich importiert hat, ruft nichts auf, wenn er erneut läuft.
Ein Import, der vor dem Ende abgebrochen wurde (der Prozess wurde beendet), zählt die Anfragen, die er reserviert hat, und der nächste Import wartet bis zu 15 Minuten auf ihn.

<!-- source: enable-cost-explorer 2f6cd1add48d -->
### Cost Explorer aktivieren

1. Öffne Cost Explorer in der Konsole Billing and Cost Management und aktiviere ihn. Er lässt sich nach der Aktivierung nicht mehr deaktivieren. Es dauert bis zu 24 Stunden, bis Daten bereit sind.
2. Ein Konto, bei dem er nicht aktiviert ist, gibt `DataUnavailableException` zurück, und der Import schlägt mit `not_enabled` fehl.

<!-- source: iam-permissions 9f1c23f3cb5c -->
### IAM-Berechtigungen

Die einzige benötigte Berechtigung ist `ce:GetCostAndUsage` (Ressource `*`).

```json
{
  "Version": "2012-10-17",
  "Statement": [
    { "Effect": "Allow", "Action": "ce:GetCostAndUsage", "Resource": "*" }
  ]
}
```

So übergibst du Zugangsdaten in jeder Laufzeitumgebung:

| Laufzeit | Zugangsdaten |
| --- | --- |
| AWS ECS | Hänge diese Richtlinie an die Task-Rolle. Es ist keine Konfiguration nötig |
| Google Cloud (Cloud Run) | Lege die IAM-Rolle aus „Von Cloud Run mit Web Identity aufrufen“ unten an und setze ihren ARN in `billing_aws_role_arn`. Es sind keine AWS-Schlüssel nötig |
| Dein eigener Server | Übergib den Zugriffsschlüssel eines IAM-Benutzers mit dieser Richtlinie in `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` (oder eine gemeinsame Credentials-Datei und `AWS_PROFILE`). Übergib ihn aus einem Secret Store |

Fehlende Berechtigungen oder ungültige Zugangsdaten schlagen mit `permission` fehl.

<!-- source: calling-from-cloud-run-with-web-identity 7d109914680a -->
### Von Cloud Run mit Web Identity aufrufen

Wenn `billing_aws_role_arn` gesetzt ist, bezieht der Server vom Metadatenserver ein Google-ID-Token des Cloud-Run-Dienstkontos (die Audience ist der ARN der Rolle) und tauscht es mit `AssumeRoleWithWebIdentity` gegen temporäre Zugangsdaten dieser Rolle ein.

1. Notiere in Google Cloud die eindeutige ID (die numerische `uniqueId`) des Dienstkontos des Cloud-Run-Dienstes: `gcloud iam service-accounts describe <email> --format='value(uniqueId)'`.
2. Lege in AWS IAM eine Rolle mit der Richtlinie der obigen Berechtigungen an und lass ihre Trust Policy `accounts.google.com` vertrauen.

   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Principal": { "Federated": "accounts.google.com" },
         "Action": "sts:AssumeRoleWithWebIdentity",
         "Condition": {
           "StringEquals": {
             "accounts.google.com:aud": "<uniqueId of the service account>",
             "accounts.google.com:oaud": "<ARN of this role>"
           }
         }
       }
     ]
   }
   ```

   `accounts.google.com:aud` entspricht dem `azp` des Tokens (der ID des Dienstkontos) und `accounts.google.com:oaud` dem `aud` des Tokens (der Audience, die der Server anfordert und die der ARN der Rolle ist).
3. Setze `MY_BILLING_AWS_ROLE_ARN=arn:aws:iam::<account ID>:role/<role name>`.

Dieses Setup wurde noch nicht auf einem echten Konto überprüft.

<!-- source: calling-from-a-linked-account-of-organizations d12e10b51af3 -->
### Von einem verknüpften Konto in Organizations aufrufen

Das Verwaltungskonto von AWS Organizations kann die Kosten jedes Mitgliedskontos der Organisation sehen. Um mit den Zugangsdaten eines Mitgliedskontos (verknüpften Kontos) aufzurufen,
schalte „Linked Account Access“ in den Cost-Explorer-Einstellungen des Verwaltungskontos ein. Ist es aus, wird der Aufruf unabhängig von den IAM-Berechtigungen abgelehnt, und der Import schlägt mit `not_enabled` oder `permission` fehl.

<!-- source: choosing-which-costs-to-import-filtering 60cfa862b6a2 -->
### Auswählen, welche Kosten importiert werden (Filter)

Wähle mit `billing_aws_filter`.

| `billing_aws_filter` | Importierte Kosten | Zusätzliche Einstellungen |
| --- | --- | --- |
| `none` (Standard) | Alle Kosten, die die Zugangsdaten sehen können | Keine |
| `linked_account` | Die Kosten der Mitgliedskonten, die du benennst (`LINKED_ACCOUNT`) | `billing_aws_linked_accounts` (12-stellige Konto-IDs, kommagetrennt) |
| `tag` | Die Kosten von Ressourcen, die den Cost-Allocation-Tag tragen, den du benennst | `billing_aws_tag` (`key=value`, zum Beispiel `app=my`) |

Das genaueste Setup ist ein AWS-Konto, das diesem Server vorbehalten ist (es kann ein Mitgliedskonto von Organizations sein), importiert mit `none` (Aufruf mit den Zugangsdaten dieses Kontos) oder `linked_account` (Aufruf vom Verwaltungskonto).
Steuern, Support-Gebühren und manche Gutschriften können keinen Tag tragen und können aus einem Tag-Filter herausfallen (noch nicht auf einem echten Konto überprüft).

<!-- source: activating-a-cost-allocation-tag-billing_aws_filter--tag 8e642123a388 -->
### Einen Cost-Allocation-Tag aktivieren (`billing_aws_filter = tag`)

Ein Tag auf einer Ressource erscheint erst in den Abrechnungsdaten, wenn du ihn als Cost-Allocation-Tag aktivierst.

1. Setze denselben Tag (zum Beispiel `app=my`) auf die Ressourcen dieses Servers (ECS-Dienste, RDS, S3-Buckets, Load Balancer und so weiter).
2. Wähle in „Cost allocation tags“ der Konsole Billing and Cost Management den Tag-Schlüssel aus und aktiviere ihn. Es dauert bis zu 24 Stunden, bis ein Tag in dieser Liste erscheint, nachdem du ihn angehängt hast, und weitere bis zu 24 Stunden nach der Aktivierung, bis er in den Abrechnungsdaten erscheint.
3. Um die Kosten des Zeitraums vor der Aktivierung zu berücksichtigen, nutze „Backfill“ auf demselben Bildschirm (die API `StartCostAllocationTagBackfill`).
4. Setze `MY_BILLING_AWS_FILTER=tag` und `MY_BILLING_AWS_TAG=app=my`.

<!-- source: metric 3e034dd351bd -->
### Metrik

`billing_aws_metric` ist entweder `NetUnblendedCost` (der Standard, näher an der Rechnung), also nach Rabatten (Savings Plans, Reserved Instances, ausgehandelte Preise), oder `UnblendedCost`, also vor ihnen.
Die Werte von Cost Explorer und die Rechnung können sich wegen des Zeitpunkts der Aktualisierung und wegen Rundung unterscheiden. Wenn sie sich unterscheiden, hat die Rechnung recht.

<!-- source: kinds-of-failure 2a6f9042f460 -->
### Fehlerarten

| Art | AWS-Antwort | Behebung |
| --- | --- | --- |
| `permission` | `AccessDeniedException`, ungültige Zugangsdaten | Prüfe die IAM-Berechtigungen und wie die Zugangsdaten übergeben werden |
| `not_enabled` | `DataUnavailableException` | Aktiviere Cost Explorer. Aktiviere bei einem verknüpften Konto Linked Account Access |
| `not_found` | `BillExpirationException` | Cost Explorer hat die Abrechnungsdaten des zu importierenden Zeitraums nicht mehr |
| `throttled` | `LimitExceededException` | Der Job versucht es nach einer Pause erneut (es zählt für das Anfragelimit) |
| `unavailable` | 5xx, Netzwerkfehler | Der Job versucht es nach einer Pause erneut (es zählt für das Anfragelimit) |
| `invalid_response` | Eine unlesbare Antwort, `ValidationException` und Ähnliches | Prüfe die Einstellungen (Tag, Konto-IDs) |
| `request_cap` | (es wird kein Aufruf gemacht) | Warte, bis der Tag oder Monat des Limits vorbei ist, oder erhöhe das Limit |

Eine Wiederholung kann einen anderen Fehler als `throttled` und `unavailable` nicht beheben, daher erfasst der Server ihn und hält an, und der geplante Import des nächsten Tages versucht es erneut.

<!-- source: google-cloud-billing_sources--gcp c74934cb4d9a -->
## Google Cloud (`billing_sources = gcp`)

Google Cloud hat keine API, die tatsächliche Kosten liefert, daher aggregiert der Server den Cloud-Billing-Export nach BigQuery (Standard usage cost) einmal am Tag mit einem BigQuery-`jobs.query` (`billing_gcp_method = bigquery`, der Standard).
Wenn du den Export nicht einrichten kannst, gibt es auch eine einfache Methode, die nur eine ungefähre Summe aus den Pub/Sub-Benachrichtigungen eines Budgets importiert (siehe [Aus Budget-Benachrichtigungen importieren](#importing-from-budget-notifications-billing_gcp_method--budget)).
Der Abrechnungsmonat ist der `invoice.month` des Exports, ein Kalendermonat in US-Pazifikzeit (mit Sommerzeit). Daten werden unverändert in Pazifikzeit erfasst.
Beträge sind in der Währung des Rechnungskontos und werden als `NUMERIC` pro Abrechnungsmonat, Dienst (`service.description`) und Gebührenart (`cost_type`) summiert; Gutschriften (`credits[]`) werden zu einer Gutschriftzeile pro Dienst. Die Korrekturzeilen des Exports, die frühere Zeilen aufheben, gehören zur Summe.
Der Export hat keine Markierung für Endgültigkeit, daher behandelt der Import, der ab dem 5. (Pazifikzeit) des Folgemonats läuft, den Monat als endgültig und erfasst ihn bis dahin als Schätzung vom ersten Tag des Monats bis zum Tag der letzten Nutzung. Nachdem er endgültig ist, liest der Import den Monat bis zum 15. weiterhin erneut und erfasst jede Differenz als Korrektur.
Eine Binärdatei, die mit dem Build-Tag `no_billing_gcp` gebaut wurde, lehnt `gcp` ab (siehe [build-tags.md](../reference/build-tags.md)).

<!-- source: cost-and-limits-1 ca56b5d7b34d -->
### Kosten und Limits

Ein Import ist eine Abfrage, und BigQuery berechnet die Bytes, die jede Abfrage scannt (mindestens 10 MB). Ein kleines Konto scannt etwa 0,3 GB im Monat, was in die ersten 1 TiB pro Monat des kostenlosen Kontingents passt (geteilt mit anderen Nutzungen des Rechnungskontos), er kostet also 0 $.
Um unerwartete Scans zu vermeiden, trägt jede Abfrage `maximumBytesBilled` (`billing_gcp_max_bytes_billed`, Standard 1 GiB). Eine Abfrage, die es überschreiten würde, wird nicht berechnet und schlägt fehl, und sie wird als Fehler der Art `bytes_cap` erfasst. Die Administration zeigt die abgerechneten Bytes.

<!-- source: enable-the-export-to-bigquery 40c7a176b9f1 -->
### Den Export nach BigQuery aktivieren

Der Betreiber richtet das einmal in der Konsole ein (es wurde kein Weg gefunden, es über eine API einzurichten).

1. Lege in einem Projekt, das mit dem Rechnungskonto verknüpft ist, ein BigQuery-Dataset an (zum Beispiel `billing_export`). Wähle einen Multi-Region-Standort (`US` oder `EU`). Die Daten des laufenden und des vorherigen Monats werden nachgefüllt (bei einer einzelnen Region beginnen die Daten an dem Tag, an dem du ihn aktivierst). Der Standort lässt sich später nicht ändern.
   Setze kein Ablaufdatum für die Tabellen. Zeilen, die ablaufen, kommen nicht zurück.
2. Aktiviere unter „Billing > Billing export > BigQuery export“ „Standard usage cost“ und wähle das Dataset aus Schritt 1. Die Einrichtung braucht „Billing Account Costs Manager“ oder „Billing Account Administrator“ für das Rechnungskonto und die Rolle BigQuery User für das Projekt.
3. Nach einigen Stunden bis einem Tag wird die Tabelle `gcp_billing_export_v1_<BILLING_ACCOUNT_ID>` angelegt (die Bindestriche der ID werden zu `_`).
4. Setze `MY_BILLING_GCP_TABLE=<project>.billing_export.gcp_billing_export_v1_<BILLING_ACCOUNT_ID>`.

Wenn das Dataset oder die Tabelle nicht gefunden werden kann, schlägt der Import mit `not_found` fehl.

<!-- source: iam-roles f5628acd0d94 -->
### IAM-Rollen

Gib dem Dienstkonto des importierenden Servers diese beiden Rollen.

| Rolle | Wo sie vergeben wird | Verwendung |
| --- | --- | --- |
| `roles/bigquery.jobUser` (BigQuery Job User) | Das Projekt, das die Abfrage ausführt (`billing_gcp_query_project`, oder das Projekt der Tabelle, wenn leer) | `bigquery.jobs.create`. Die gescannten Bytes werden diesem Projekt berechnet |
| `roles/bigquery.dataViewer` (BigQuery Data Viewer) | Das Dataset des Exports | `bigquery.tables.getData` |

```sh
gcloud projects add-iam-policy-binding <query project> \
  --member=serviceAccount:<service account> --role=roles/bigquery.jobUser
bq add-iam-policy-binding --member=serviceAccount:<service account> \
  --role=roles/bigquery.dataViewer <project>:billing_export
```

Wenn die BigQuery-API im Abfrageprojekt nicht aktiviert ist, schlägt der Import mit `not_enabled` fehl, und wenn eine Rolle fehlt, mit `permission`.

Die Authentifizierung erfolgt über Application Default Credentials (ADC).

| Laufzeit | Zugangsdaten |
| --- | --- |
| Google Cloud (Cloud Run) | ADC mit dem Dienstkonto des Dienstes. Kein Schlüssel und keine Konfiguration |
| AWS ECS | Lege eine Credential-Konfigurationsdatei der Workload Identity Federation (Typ AWS) ab und übergib ihren Pfad in `GOOGLE_APPLICATION_CREDENTIALS` (noch nicht auf einem echten Konto überprüft) |
| Dein eigener Server | Workload Identity Federation oder ein JSON-Schlüssel eines Dienstkontos, übergeben in `GOOGLE_APPLICATION_CREDENTIALS`. Übergib einen Schlüssel aus einem Secret Store |

<!-- source: choosing-which-costs-to-import-filtering-1 7067e430a4c7 -->
### Auswählen, welche Kosten importiert werden (Filter)

Standardmäßig importiert der Server alle Kosten des Rechnungskontos. Das genaueste Setup ist ein Rechnungskonto, das diesem Server vorbehalten ist, importiert ohne Filter.

| Einstellung | Importierte Kosten |
| --- | --- |
| `billing_gcp_projects` (Projekt-IDs, kommagetrennt) | Die Kosten der Projekte, die du benennst (`project.id`) |
| `billing_gcp_label` (`key=value`, zum Beispiel `app=my`) | Die Kosten von Ressourcen, die das Label tragen, das du benennst (`labels`) |

Wenn du beide setzt, importiert der Server nur die Kosten, die auf beide zutreffen.
Kosten auf Rechnungsebene, wie Steuern, tragen möglicherweise weder ein Projekt noch ein Label und fallen dann aus dem Filter heraus (noch nicht auf einem echten Konto überprüft).

<!-- source: kinds-of-failure-1 612d14e2ba5c -->
### Fehlerarten

| Art | BigQuery-Antwort | Behebung |
| --- | --- | --- |
| `permission` | 401, 403 (`accessDenied`) | Prüfe die Rollen des Dienstkontos und wie die Zugangsdaten übergeben werden |
| `not_enabled` | 403 (`accessNotConfigured` und Ähnliches) | Aktiviere die BigQuery-API im Abfrageprojekt |
| `not_found` | 404 | Prüfe `billing_gcp_table` und die Einrichtung des Exports |
| `bytes_cap` | 400 (`bytesBilledLimitExceeded`) | Der Scan hat `billing_gcp_max_bytes_billed` überschritten. Erhöhe das Limit, wenn die Tabelle groß ist |
| `throttled` | 429, 403 (`rateLimitExceeded`, `quotaExceeded`) | Der Job versucht es nach einer Pause erneut |
| `unavailable` | 5xx, Netzwerkfehler, eine Abfrage, die nicht endet | Der Job versucht es nach einer Pause erneut |
| `invalid_response` | Eine unlesbare Antwort, Zeilen, die Währungen innerhalb eines Monats mischen, jedes andere 400 | Prüfe die Exporttabelle (ist es der Standard usage cost?) |

Eine Wiederholung kann einen anderen Fehler als `throttled` und `unavailable` nicht beheben, daher erfasst der Server ihn und hält an, und der geplante Import des nächsten Tages versucht es erneut.

<!-- source: importing-from-budget-notifications-billing_gcp_method--budget b90e6393e3d3 -->
### Aus Budget-Benachrichtigungen importieren (`billing_gcp_method = budget`)

Das ist eine einfache Methode für den Fall, dass du den BigQuery-Export nicht einrichten kannst. Einmal am Tag liest der Server aus einer Pull-Subscription die Benachrichtigungen, die ein Cloud-Billing-Budget an ein Pub/Sub-Topic sendet.
Sie nutzt Pull statt Push, sodass eine Benachrichtigung den Server (zum Beispiel Cloud Run) nie aufweckt.

Diese Methode importiert **nur eine ungefähre Summe auf Basis von Schätzungen**.

- Der `costAmount` einer Benachrichtigung sind die geschätzten kumulierten Kosten des Budgetzeitraums, ohne Aufschlüsselung nach Dienst oder Gebührenart. Der Server importiert nur eine Zeile pro Monat und Budget (der Dienstname ist der Anzeigename des Budgets, die Gebührenart ist usage und die Kategorie ist other). Gutschriften und Steuern stecken in der Summe, wie das Budget konfiguriert ist.
- Wie bei BigQuery wird ein Monat in dem Import endgültig, der ab dem 5. (Pazifikzeit) des Folgemonats läuft. Eine Budget-Benachrichtigung weiß nichts vom Rechnungsabschluss, daher kann sich auch ein als endgültig behandelter Wert vom Rechnungsbetrag unterscheiden.
- Ein geschätzter Monat wird als die Kosten vom ersten Tag des Monats bis zu dem Tag (Pazifikzeit) erfasst, an dem die letzte Benachrichtigung veröffentlicht wurde.
- Die Filterung bestimmt der Geltungsbereich des Budgets (Projekte, Dienste, ein Label-Schlüssel und -Wert); `billing_gcp_projects` und `billing_gcp_label` werden nicht verwendet.

<!-- source: prepare-the-budget-the-topic-and-the-subscription ee38acf8cb72 -->
#### Das Budget, das Topic und die Subscription vorbereiten

1. Lege ein Topic und eine Pull-Subscription an, die nur dieser Server liest. Die Gründe für die Aufbewahrungsdauer und den Ablauf findest du unten unter „So werden Benachrichtigungen gelesen“.

   ```sh
   gcloud pubsub topics create billing-budget --project=<project>
   gcloud pubsub subscriptions create my-billing-budget --project=<project> \
     --topic=billing-budget --ack-deadline=60 \
     --message-retention-duration=31d --expiration-period=never
   ```

2. Lege in der Konsole unter „Billing > Budgets & alerts“ ein Budget an. Mache seinen Zeitraum **monatlich** (eine Benachrichtigung eines Budgets, das nicht monatlich ist, schlägt mit `invalid_response` fehl).
   Wähle das Projekt dieses Servers als Geltungsbereich und wähle unter „Manage notifications > Connect a Pub/Sub topic to this budget“ das Topic aus Schritt 1.
   Das Anlegen des Budgets braucht „Billing Account Costs Manager“ oder „Billing Account Administrator“ für das Rechnungskonto sowie `pubsub.topics.setIamPolicy` und `pubsub.topics.list` für das Projekt des Topics.
   Die erste Benachrichtigung braucht einige Stunden, bis sie ankommt, und danach kommen mehrere pro Tag an.
3. Gib dem Dienstkonto des importierenden Servers `roles/pubsub.subscriber` (Pub/Sub Subscriber) für die Subscription. Es wird zum Abrufen von Nachrichten und zu ihrer Bestätigung verwendet (`pubsub.subscriptions.consume`).

   ```sh
   gcloud pubsub subscriptions add-iam-policy-binding my-billing-budget --project=<project> \
     --member=serviceAccount:<service account> --role=roles/pubsub.subscriber
   ```

4. Setze `MY_BILLING_GCP_METHOD=budget` und `MY_BILLING_GCP_BUDGET_SUBSCRIPTION=projects/<project>/subscriptions/my-billing-budget`.

Die Authentifizierung erfolgt über dieselben Application Default Credentials wie bei BigQuery (siehe die Tabelle unter [IAM-Rollen](#iam-roles) oben).
Der Build-Tag `no_billing_gcp` entfernt auch diese Methode. Die Pub/Sub-Clientbibliothek wird mit `stream = pubsub` geteilt, sie verlässt die Binärdatei also nur, wenn du außerdem `no_pubsub` setzt (siehe [build-tags.md](../reference/build-tags.md)).

<!-- source: how-notifications-are-read 646a6dd9b2b6 -->
#### So werden Benachrichtigungen gelesen

- Ein Import wiederholt Pull, bis keine neue Nachricht mehr eintrifft, und nimmt für jedes Budget und jeden Monat die Benachrichtigung mit der jüngsten Veröffentlichungszeit. Das fängt doppelte Pub/Sub-Zustellungen, wiederholte Benachrichtigungen mit demselben Wert und Umsortierungen ab, sodass eine mehrfach empfangene Benachrichtigung nur einmal in die Summe eingeht.
  Die Zahl der für einen Import gezählten Anfragen ist die Zahl der Pulls, normalerweise zwei: einer, der Nachrichten liefert, und einer, der leer zurückkommt. Die Bestätigungen am Ende (`Acknowledge`, `ModifyAckDeadline`) werden nicht gezählt.
- Der Server bestätigt (ackt) nur eine Benachrichtigung, die durch eine neuere Benachrichtigung desselben Budgets und Monats ersetzt wurde, und eine Benachrichtigung eines Monats, der nicht mehr importiert wird (der Vormonat, sobald er als endgültig erfasst wurde). Die jüngste Benachrichtigung eines importierten Monats wird nicht geackt, sondern an die Subscription zurückgegeben, sodass der nächste Import dieselbe Benachrichtigung erneut liest und sich der Wert an einem Tag ohne neue Benachrichtigung nicht ändert.
- Setze daher die Nachrichtenaufbewahrung der Subscription auf **mindestens 14 Tage (31 Tage empfohlen)**. Die letzte Benachrichtigung des Budgets eines Monats kommt um den 2. des Folgemonats an, und der Vormonat wird bis zum 15. erneut gelesen. Ein Monat, dessen Benachrichtigungen abgelaufen sind, wird als Kosten von 0 erfasst.
- Standardmäßig läuft eine Subscription nach 31 Tagen ohne Nutzung ab. Setze den Ablauf auf „never“, damit sie einen Zeitraum übersteht, in dem Importe gestoppt waren.
- Sende keine anderen Nachrichten an das Topic. Eine Nachricht, die sich nicht als Budget-Benachrichtigung lesen lässt, lässt diesen Import mit `invalid_response` fehlschlagen und wird geackt, damit der nächste Import nicht an ihr hängen bleibt.
- Kosten: Einige Dutzend Nachrichten pro Tag passen in das kostenlose Pub/Sub-Kontingent (10 GiB pro Monat), es kostet also 0 $. Der Server setzt kein Limit.

<!-- source: kinds-of-failure-budget-notifications c96440fefa1a -->
#### Fehlerarten (Budget-Benachrichtigungen)

| Art | Pub/Sub-Antwort | Behebung |
| --- | --- | --- |
| `permission` | `PERMISSION_DENIED`, `UNAUTHENTICATED` | Prüfe `roles/pubsub.subscriber` für die Subscription und wie die Zugangsdaten übergeben werden |
| `not_enabled` | `PERMISSION_DENIED` (`SERVICE_DISABLED`) | Aktiviere die Pub/Sub-API im Projekt der Subscription |
| `not_found` | `NOT_FOUND` | Prüfe `billing_gcp_budget_subscription` und ob die Subscription abgelaufen ist |
| `throttled` | `RESOURCE_EXHAUSTED` | Der Job versucht es nach einer Pause erneut |
| `unavailable` | `UNAVAILABLE`, `INTERNAL` und Ähnliches, Netzwerkfehler | Der Job versucht es nach einer Pause erneut |
| `invalid_response` | Eine Nachricht, die sich nicht als Budget-Benachrichtigung lesen lässt, ein Budget, das nicht monatlich ist, zwei Budgets mit demselben Anzeigenamen | Mache das Budget monatlich, gib den Budgets verschiedene Anzeigenamen und sende keine anderen Nachrichten an das Topic |

<!-- source: a-plugin-billing_sources--plugin 7589c3333153 -->
## Ein Plugin (`billing_sources = plugin`)

Für einen Anbieter, für den der Server keinen Adapter hat, wie Azure, Hetzner oder Sakura, betreibe ein [Plugin](plugins.md) der Art `billing` und lass es die Daten des Anbieters lesen. Der Server hat eine `plugin`-Quelle. Ein Plugin mit mehreren Anbietern aggregiert sie selbst und unterscheidet sie an den Namen seiner Dienste.

```toml
billing_sources = "plugin"
plugin_billing_address = "unix:///run/my/billing.sock"
plugin_billing_location = "Europe/Berlin"
plugin_billing_provider = "Hetzner"
```

Die drei Einstellungen sind bei dieser Quelle erforderlich, und der Server startet, ohne das Plugin aufzurufen, auch wenn es ausgefallen ist.

- `plugin_billing_location` ist die Zeitzone, die die Abrechnungsmonate des Plugins abgrenzt.
- `plugin_billing_provider` ist der Name des Anbieters in den Summen, auf der Kostenseite und im Audit-Log.

Der Server fragt das Plugin nach dem laufenden Monat und, wie bei den anderen Quellen, nach dem vorherigen bis zum 15. oder bis er endgültig ist. Die Limits der anderen Quellen gelten nicht: `max_requests` ist `2147483647`, was kein Limit bedeutet, und das Plugin entscheidet, was es ausgeben darf. Es meldet die Anfragen und die Bytes, die ihm berechnet wurden, was der Server erfasst und anzeigt.

Die Art erlaubt einfaches TCP, mit einer Warnung beim Start, da sie nur Kosten trägt. Ein Bearer-Token braucht `https://` oder einen Unix-Socket: siehe [das Ziel eines Plugins](plugins.md#the-destination).

<!-- source: the-call 6a2bcce82ab2 -->
### Der Aufruf

Mit `plugin_billing_protocol = json`, dem Standard, braucht ein Plugin allein einen HTTP-Server und eine JSON-Bibliothek ([Ein Plugin ohne Bibliothek](plugins.md#a-plugin-with-no-library)). `Fetch` dauert höchstens 2 Minuten oder `plugin_billing_timeout`, unter 5 Minuten.

```text
POST /my.plugin.billing.v1.BillingService/Fetch
Content-Type: application/json

{"months":["2026-09","2026-10"],"maxRequests":2147483647}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{
  "outcome": "OUTCOME_OK",
  "requests": 1,
  "lines": [
    {
      "month": "2026-09",
      "coveredStart": "2026-09-01",
      "coveredEnd": "2026-09-30",
      "service": "Hetzner Cloud",
      "chargeType": "CHARGE_TYPE_USAGE",
      "category": "CATEGORY_COMPUTE",
      "amount": "12.3456",
      "currency": "EUR",
      "final": true
    }
  ]
}
```

- Eine Zeile ist ein Monat, ein Dienst und eine Gebührenart, mit ihren ausgleichenden Zeilen, Gutschriften und Erstattungen summiert. `service` hat höchstens 200 Zeichen und keine Steuerzeichen.
- `coveredStart` und `coveredEnd` liegen innerhalb des Monats: der ganze Monat, wenn `final` wahr ist, sonst bis zum letzten Tag mit Daten.
- `amount` ist eine ungerundete Dezimalzahl in Hauptwährungseinheiten, die negativ sein kann. `currency` ist ein ISO-4217-Code.
- `chargeType` ist `CHARGE_TYPE_` und eines von `USAGE`, `CREDIT`, `REFUND`, `TAX`, `SUPPORT`, `FEE`, `ADJUSTMENT`, `OTHER`.
- `category` ist `CATEGORY_` und eines von `COMPUTE`, `DATABASE`, `STORAGE`, `NETWORK`, `DOMAIN`, `EMAIL`, `SOFTWARE`, `OTHER`.
- `requests` ist höchstens `maxRequests`. `bytesBilled` ist ein Dezimal-String, für einen Anbieter, der nach Bytes abrechnet.
- Ein Schlüssel, den `OUTCOME_OK` nicht zurückgibt, hat in seinem Monat keine Kosten, daher erfasst der Server dafür eine Revision von 0. Ein Plugin, das einen Anbieter nicht lesen konnte, muss einen Fehler melden, nicht seine Zeilen weglassen.
- Aufrufe dürfen sich wiederholen: Eine Wiederholung eines fehlgeschlagenen Imports ist derselbe Aufruf noch einmal.

`maxRequests` 0 verbietet jede abgerechnete Anfrage. Der Server macht keinen Aufruf, wenn er 0 senden würde, aber `my plugin test --kind billing` tut es, mit weggelassenem Feld, wie es das Mapping von proto3 erlaubt, und erwartet `OUTCOME_REQUEST_CAP` ohne Zeilen und ohne Anfragen.

<!-- source: kinds-of-failure-2 205a6c0ecdd9 -->
### Fehlerarten

Ein Plugin meldet einen Fehler in `outcome`. Der Server behält die `requests` eines Fehlers, ignoriert seine Zeilen und lässt die früheren Importe unverändert, wie bei den anderen Quellen.

| Outcome | Art | Wiederholt |
| --- | --- | --- |
| `OUTCOME_PERMISSION` | `permission` | Nein |
| `OUTCOME_NOT_ENABLED` | `not_enabled` | Nein |
| `OUTCOME_NOT_FOUND` | `not_found` | Nein |
| `OUTCOME_THROTTLED` | `throttled` | Ja |
| `OUTCOME_REQUEST_CAP` | `request_cap` | Nein |
| `OUTCOME_BYTES_CAP` | `bytes_cap` | Nein |
| `OUTCOME_INVALID_RESPONSE` | `invalid_response` | Nein |
| `OUTCOME_UNAVAILABLE` | `unavailable` | Ja |
| Ein fehlendes oder unbekanntes Outcome oder eine Zeile oder Zahl, die die obigen Regeln verletzt | `invalid_response` | Nein |
| Ein HTTP- oder Connect-Fehler, eine Deadline, eine Antwort über 4 MiB, keine Antwort | `unavailable` | Ja |

Was nicht wiederholt wird, wartet auf den nächsten täglichen Import. Der Grund eines Fehlers steht im Log des Plugins, da der Server die Meldung eines Fehlers verwirft, und er zählt auch in [`my.plugin.failures`](running.md).

<!-- source: test-it 10a2c62a53d8 -->
### Testen

```sh
my plugin test --kind billing unix:///run/my/billing.sock
my plugin test --kind billing --billed unix:///run/my/billing.sock
```

Der erste macht keine abgerechnete Anfrage, wenn das Plugin den Vertrag einhält. Der zweite macht außerdem einen echten Abruf, der bei den Anbietern des Plugins Geld kosten kann. Siehe [Ein Plugin testen](plugins.md#test-a-plugin).
