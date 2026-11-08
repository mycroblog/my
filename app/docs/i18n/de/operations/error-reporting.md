<!-- source: error-reporting 1ccf0c1c98a4 -->
# Fehlerberichte

Wie der Server dich über seine eigenen Bugs und Fehler informiert, wo du die Berichte auf jeder Plattform liest und wie du sie an einen Error-Tracker schickst.
Metriken sagen dir, dass etwas nicht stimmt; ein Fehlerbericht sagt dir, was fehlgeschlagen ist, samt Stack. Die [Konfigurationsreferenz](../reference/configuration.md) listet jede Einstellung auf.

Drei Dinge geschehen mit jedem Bericht, egal was du konfigurierst:

1. Eine `error report`-Zeile geht mit dem Stack an die Standardfehlerausgabe ([Die Logzeile](#the-log-line)).
2. Der Bericht wird in der Datenbank in einer Fehlergruppe gezählt. Die Administrationsoberfläche zeigt die Gruppen, und Personen mit der Berechtigung `manage_errors` werden über neue und wiederkehrende benachrichtigt ([Fehlergruppen und Benachrichtigungen](#error-groups-and-notifications)).
3. Wenn du `errors_exporter` setzt, geht der Bericht außerdem an einen externen Dienst ([Berichte an einen Dienst senden](#sending-reports-to-a-service)).

Nichts verlässt den Server, solange du `errors_exporter` nicht setzt.

<!-- source: what-is-reported 8e5ac9d14c36 -->
## Was gemeldet wird

| Gemeldet | Nicht gemeldet (die Metriken und die anderen Logzeilen decken es ab) |
| --- | --- |
| Eine Panic in einem HTTP-Handler, einem Job oder einem Hintergrundtask | 4xx-Antworten (fehlerhafte Eingabe, Autorisierung, Ratenbegrenzungen) und der Wartungsmodus |
| Ein Fehler hinter einer 5xx-Antwort der Client-API, der Admin-API, der Anmeldeseiten, von ActivityPub, OAuth oder Medien | Eine Anfrage, deren Client weg war |
| Ein Job, der endgültig fehlgeschlagen ist (`dead`) | Ein Job-Retry. Er wird gemeldet, wenn ihm die Wiederholungen ausgehen |
| Ein Fehler beim Start: eine ungültige Konfiguration oder eine Datenbank oder ein Adapter, die sich nicht einrichten lassen | Eine fehlgeschlagene Readiness-Prüfung |
| Ein Fehler, den der Web-Client nicht behandelt hat und den der Browser an den Server schickt | Fehler, die andere Server verursachen: Zustellwiederholungen und Aufgaben, abgelehnte Signaturen, fehlgeschlagene Remote-Abrufe |
| | Das eigene Rauschen des Browsers: ein verlorenes Netzwerk, Fehler von Browser-Erweiterungen und Chunks, die ein neues Deployment entfernt hat |

Ein Fehler eines externen Dienstes (Mail, Speicher, Zahlungen, Sprache) wird nur gemeldet, wenn er einen Job endgültig scheitern lässt oder eine Anfrage mit 5xx beantworten lässt. Ein vorübergehender Fehler zeigt sich stattdessen in den `my.adapter.*`-Metriken ([Den Server betreiben](running.md#metrics)).

Eine Panic in einem Hintergrundtask beendet den Prozess, nachdem sie gemeldet wurde, damit der Server nie in einem defekten Zustand weiterläuft. Deine Plattform startet ihn neu.

<!-- source: the-log-line 6b570d0bfd09 -->
## Die Logzeile

Die Zeile hat `msg` = `error report` und `level` `ERROR` (`severity` ist `CRITICAL`, wenn der Fehler den Prozess beendet).
Ihre Felder und was das Scrubbing mit ihnen macht, stehen in [Den Server betreiben](running.md#error-reports). Kurz gesagt:

- `my.error.fingerprint` ist bei jedem Auftreten desselben Fehlers gleich. Suche oder gruppiere danach.
- `my.error.where` sagt, wo es passiert ist, zum Beispiel `client_api GET /api/v1/statuses/{id}`.
- `request_id`, `job_id`, `trace_id` und `span_id` finden die anderen Zeilen derselben Anfrage oder desselben Jobs.
- `message` steht am Ende der Zeile: der Fehlertyp und die Meldung, ein Zeilenumbruch und der Go-Stack.

Keine andere Zeile trägt einen Stack.

<!-- source: google-cloud-cloud-run 925c99683713 -->
### Google Cloud (Cloud Run)

Cloud Error Reporting liest die Zeile ohne jede Einrichtung und listet die Fehler in der Konsole unter Error Reporting auf. Es ist kostenlos; du zahlst nur für Cloud Logging.

- Es gruppiert Fehler nach dem Typ und den obersten Frames des Stacks. Es kann den Fingerprint des Servers nicht übernehmen, daher kann sich eine Gruppe in Error Reporting von einer Gruppe in der Administrationsoberfläche unterscheiden.
- Es behält Gruppen 30 Tage lang und benachrichtigt dich über neue Fehler und über erledigte Fehler, die erneut auftreten, höchstens 20 Benachrichtigungen pro Stunde für das Projekt. Richte seinen Benachrichtigungskanal in der Google-Cloud-Konsole ein.
- Um herauszufinden, was um einen Bericht herum geschah, suche in Cloud Logging nach `jsonPayload."request_id"="<id>"` oder nach dem Gleichen mit `trace_id`.

<!-- source: aws 6ca6594afd7e -->
### AWS

CloudWatch Logs liest die Zeile als einfaches JSON.

- Schalte die Log-Anomalieerkennung für die Loggruppe ein. Sie markiert ein Zeilenmuster, das sie noch nie gesehen hat, und kann per SNS Alarm schlagen. Sie liest nur die ersten 1.500 Zeichen einer Zeile, weshalb `message` das letzte Feld ist. Einen Zustand „erledigt“ führt sie nicht.
- Um Berichte zu zählen, lege einen Metrikfilter an, etwa `{ $.msg = "error report" }`. Verwende keine Dimension, die aus `my.error.fingerprint` stammt: Jeder Wert einer Dimension wird als eigene Metrik abgerechnet. `my.error.source` und `my.error.kind` sind unbedenklich.
- Den Kontext findest du mit Logs Insights: ``filter `request_id` = "<id>"``.

<!-- source: self-hosted 90de7a419474 -->
### Selbst gehostet

Lies die Zeile aus dem Container-Log: `docker compose logs my | jq -c 'select(.msg == "error report")'`.
Mit `metrics_exporter = prometheus` warnt der Alert `MyErrorReportsIncreasing` ([Alerts und Runbook](alerts.md#myerrorreportsincreasing)), wenn die Berichte stark ansteigen; der Zähler dahinter ist `my.errors.reported`.
Für Benachrichtigungen, die keinen externen Dienst brauchen, nutze die Administrationsoberfläche und die Webhooks unten. Für einen Tracker nutze [`errors_exporter`](#sending-reports-to-a-service).

<!-- source: error-groups-and-notifications 8f740a328309 -->
## Fehlergruppen und Benachrichtigungen

Der Server zählt die Berichte nach Fingerprint in der Datenbank. Er braucht dafür keine Einstellung und keinen zusätzlichen Dienst und schreibt wenig: Der erste Bericht eines Fingerprints wird sofort geschrieben, die folgenden werden im Speicher addiert und einmal pro Minute geschrieben. Ein harter Abbruch des Prozesses kann die Zählungen der letzten Minute verlieren; die Logzeilen gehen auf diese Weise nicht verloren.
Zählungen, die älter als 90 Tage sind, werden täglich gelöscht. Wenn die Datenbank der Grund für den Fehler ist, bleibt der Bericht nur in der Logzeile.

<!-- source: the-administration-screen 09092782a7c1 -->
### Die Administrationsoberfläche

Personen, deren Rolle `manage_errors` hat (die eingebaute Rolle `admin` hat es), sehen „Fehler“ im Administrationsmenü (`/admin/errors`).

- Die Liste zeigt die Gruppen, standardmäßig die offenen. Filtere nach Zustand (offen, erledigt, stummgeschaltet, jeder Zustand) und nach Quelle (der Server, der Web-Client). Jede Zeile zeigt den Fehlertyp, wo er aufgetreten ist, die ersten Zeilen der Meldung, die Zählungen der letzten 24 Stunden und 7 Tage sowie den ersten und letzten Zeitpunkt und die erste und letzte Version.
- Die Seite einer Gruppe zeigt das jüngste Auftreten (Zeit, Version, Prozessrolle, Request-ID, Trace-ID, die Typen, die Meldung und den Stack), die Vorkommen pro Stunde der letzten 90 Tage und den Verlauf ihrer Zustandsänderungen. Die Request-ID und die Trace-ID haben eine Schaltfläche „Kopieren“: Füge sie in deine Log-Suche ein.
- „Als erledigt markieren“ ist für einen Fehler gedacht, den du behoben hast. Tritt er in einer anderen Version des Servers erneut auf, öffnet sich die Gruppe wieder und benachrichtigt erneut. Derselbe Fehler in der Version, die erledigt wurde, wird nur gezählt, damit die letzten Anfragen der alten Version ihn nicht wieder öffnen.
- „Stummschalten“ zählt weiter, sendet aber keine Benachrichtigung und kein Webhook-Ereignis. „Stummschaltung aufheben“ öffnet die Gruppe wieder.

Dieselben Aktionen sind als `GET /api/v1/admin/error_groups` und `POST /api/v1/admin/error_groups/{id}/resolve` (`/mute`, `/unmute`) verfügbar.

<!-- source: notifications-and-webhooks 99836c7f02ec -->
### Benachrichtigungen und Webhooks

- Personen mit `manage_errors` erhalten die Benachrichtigung `admin_error_group` für eine neue Fehlergruppe und für ein erneutes Auftreten. Für den ganzen Server kommen höchstens 10 pro Stunde an; der Rest der Stunde wird in einer Benachrichtigung zusammengefasst, die sagt, dass weitere Fehler gemeldet wurden. Sie können sie auch per E-Mail erhalten ([Moderation](moderation.md#notifications-for-moderators-and-the-pending-counts)).
- Die Webhook-Ereignisse `error_group.created` und `error_group.regressed` tragen dieselben Gruppen, du kannst sie also an Slack, Discord oder ein Automatisierungswerkzeug schicken ([Server-Webhooks](webhooks.md), [Webhook-Referenz](../reference/webhooks.md)). Der Webhook braucht `manage_webhooks`, und die Ereignisse brauchen `manage_errors`.
- Eine stummgeschaltete Gruppe benachrichtigt nie.

<!-- source: errors-from-the-web-client e5b2f45b62be -->
### Fehler aus dem Web-Client

Der Web-Client schickt die Fehler, die seine Bildschirme nicht behandelt haben, an `POST /api/v1/client_errors` auf deinem eigenen Server, sodass kein Dritter sie sieht und sich keine Content-Security-Einstellung ändert. Der Server antwortet auf alles mit `204` und verwirft, was er nicht annimmt: einen Body über 16 KiB, einen Stack ohne eine Datei deines eigenen Web-Clients und mehr als 10 Berichte pro Minute von einer IP-Adresse. Derselbe Fehler wird einmal pro Seitenaufruf gesendet, höchstens 20 pro Seitenaufruf.
Ein Bericht ohne angemeldete Sitzung wird gezählt und geloggt, benachrichtigt aber von sich aus niemanden, damit ein anonymes Skript dein Postfach nicht füllen kann. Die Gruppe benachrichtigt, wenn auch der Browser einer angemeldeten Person sie meldet.
Die Stacks stammen aus dem minifizierten Build und werden nicht in den Quellcode zurückübersetzt; lies also zuerst die Route und die Meldung. Das Release ist die Version des Servers, der die Seite ausgeliefert hat. Eine Seite, die offline oder nach 3 Sekunden ohne Antwort geöffnet wurde, stammt aus der Kopie des Browsers, die die Version nennt, aus der der Browser den Web-Client gespeichert hat: eine ältere Version mit demselben Web-Client.

<!-- source: sending-reports-to-a-service eb5f208ca8fc -->
## Berichte an einen Dienst senden

Setze `errors_exporter`, um jeden Bericht zusätzlich an einen Error-Tracker zu senden. Die Logzeile und die Gruppen in der Datenbank bleiben eingeschaltet. Ein Bericht wird einmal gesendet und nie wiederholt, und nichts wartet auf den Dienst.

| Wert | Sendet an |
| --- | --- |
| `none` (Standard) | Nirgendwohin |
| `sentry` | Jeden Sentry-kompatiblen Dienst unter `errors_sentry_dsn` |
| `otlp` | Einen OpenTelemetry-Collector oder ein Backend, als Log-Records |

`errors_environment` (Standard `production`) benennt das Deployment in jedem Bericht, etwa `staging`.

Beide Exporter senden dieselben bereinigten Felder wie die Logzeile, mit dem Fingerprint und den Trace- und Span-IDs, und nichts über den Host: keinen Hostnamen, keine IP-Adresse, keinen Nutzer, keinen Request-Body, keinen Header und kein Cookie.

Der Server begrenzt, was er sendet, damit der Dienst nicht überflutet wird:

- Derselbe Fingerprint geht höchstens einmal pro Minute und Prozess hinaus, und alle Berichte höchstens 60 pro Minute. Der Rest bleibt im Log und in den Gruppen.
- Bis zu 1.000 Berichte warten auf das Senden. Ein Bericht, der die Queue voll vorfindet, wird verworfen. Wenn der Server stoppt, wartet er bis zu 2 Sekunden, bis die Queue leer ist.
- Wenn der Dienst mit `429` antwortet oder um Verlangsamung bittet (`Retry-After`, `X-Sentry-Rate-Limits`), hört der Server für diese Dauer auf zu senden.
- Der Server folgt nie einer Weiterleitung des Dienstes, damit der DSN-Schlüssel und der Bericht nirgendwo anders hingehen: Eine Weiterleitung zählt als `failed`. Verwende den DSN, den der Dienst anzeigt und der selbst auf seinen Intake zeigt.

Beobachte den Zähler `my.errors.exported`: `my.outcome` ist `success`, `dropped` (über einem Limit, die Queue war voll oder der Dienst bat um Verlangsamung) oder `failed` (der Dienst war nicht erreichbar oder hat den Bericht abgelehnt) ([Den Server betreiben](running.md#metrics)).

<!-- source: sentry-glitchtip-and-datadog 12c935ec3c05 -->
### Sentry, GlitchTip und Datadog

```sh
MY_ERRORS_EXPORTER=sentry
MY_ERRORS_SENTRY_DSN_FILE=/run/secrets/sentry_dsn   # or MY_ERRORS_SENTRY_DSN; the DSN is a secret
```

Der Server spricht das Sentry-Protokoll selbst, es ist also kein Sentry-SDK beteiligt, und es wird keine Umgebungsvariable `SENTRY_*` gelesen. Er setzt den Fingerprint, damit der Dienst danach gruppiert, und die Tags `service` (`my`), `my.role`, `my.error.where` und `error.fingerprint`. Das Release ist die Version des Servers und die Umgebung ist `errors_environment`.

- **Sentry** (gehostet oder selbst gehostet): Kopiere den DSN aus den Client-Keys des Projekts.
- **GlitchTip**: Kopiere den DSN des Projekts. Es ist Open Source und läuft auf einem kleinen Server. Prüfe, welche Alert-Regeln deine Version für neue und wiederkehrende Issues anbietet.
- **Datadog Error Tracking**: Verwende den Sentry-DSN deiner Datadog-Site, `https://<DD_API_KEY>@sentry-intake.<DD_SITE>/1`. Datadog verlangt den Tag `service`, den der Server immer setzt. Datadog speichert diese Ereignisse als Logs und gruppiert sie in Error Tracking. Wenn du die Logzeile stattdessen mit einem Agent oder Forwarder an Datadog schickst (Error Tracking for Logs), steht der Stack in `message`; ordne ihn mit einer Log-Pipeline `error.stack` zu, sonst erkennt Datadog ihn nicht als Stack. Der Exporter `sentry` braucht keine solche Zuordnung.

<!-- source: otlp-backends 778540dabdc8 -->
### OTLP-Backends

```sh
MY_ERRORS_EXPORTER=otlp
OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4318   # or OTEL_EXPORTER_OTLP_LOGS_ENDPOINT
OTEL_EXPORTER_OTLP_HEADERS=authorization=Bearer%20...
```

Jeder Bericht ist ein OpenTelemetry-Log-Record namens `exception`, mit der Severity `ERROR` (`FATAL`, wenn er den Prozess beendet), `exception.type`, `exception.message`, `exception.stacktrace`, `my.error.fingerprint`, `my.error.where`, `my.error.source`, `my.error.kind` und den Trace- und Span-IDs. Er geht per OTLP/HTTP (Protobuf, gzip) mit derselben Resource wie die Metriken und Traces hinaus.

Ob das Backend diese Records in gruppierte Fehler umwandelt, hängt vom Backend ab: Manche bauen ihre Fehleransichten nur aus Trace-Daten, und Datadog braucht `error.stack`. Verwende für Sentry, GlitchTip und Datadog `sentry`. Verwende `otlp`, wenn du die Records neben deinen anderen Logs haben willst. Der Build-Tag `no_otel` lässt `otlp` weg ([Build-Tags](../reference/build-tags.md)).

<!-- source: what-scrubbing-does-and-does-not-guarantee ccf1b4716bdd -->
## Was das Scrubbing garantiert und was nicht

Das Scrubbing läuft einmal, wenn der Bericht erstellt wird; die Logzeile, die Datenbank und jeder Exporter erhalten also nur den bereinigten Text.

- In der Meldung werden URLs, E-Mail-Adressen, `@user@host`-Handles, IP-Adressen, UUIDs und ULIDs sowie Zahlen mit 10 oder mehr Ziffern durch `<url>`, `<email>`, `<acct>`, `<ip>`, `<id>` und `<n>` ersetzt, und die Meldung wird auf 1 KiB gekürzt.
- Im Stack werden die Werte von Funktionsargumenten durch `(...)` ersetzt, und der Stack wird auf 16 KiB gekürzt. Dateipositionen bleiben, relativ zum Quellbaum, sodass die Pfade der Build-Maschine nicht erscheinen.
- Kein Bericht hat einen Request-Body, Header, Query, Cookie, eine Konto-ID, einen Nutzernamen, eine IP-Adresse oder einen User-Agent.

Das Scrubbing arbeitet mit Mustern und garantiert deshalb nicht, dass eine Meldung keine personenbezogenen Daten enthält. Ein Name, ein Freitext oder eine Zahl mit weniger als 10 Ziffern, die eine Fehlermeldung zitiert, können bleiben. Das Feld `error` der gewöhnlichen Zeilen `http request` und `job` wird nicht auf diese Weise bereinigt. Behandle die Logs und den Error-Tracker als Orte, die Fragmente von Nutzerinhalten enthalten können, und gib ihnen dieselbe Zugriffskontrolle und Aufbewahrung wie dem Rest deiner Logs. Sobald du `errors_exporter` setzt, gelten für die Berichte der Standort, die Aufbewahrung und die Zugriffsregeln dieses Dienstes.
