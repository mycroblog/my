<!-- source: administrators-and-roles 1baeba6d67b4 -->
# Administratoren und Rollen

So legst du den ersten Administrator an, verwaltest Rollen auf der Kommandozeile, kündigst das Ende des Dienstes an und führst es durch, verarbeitest hochgeladene Bilder erneut, baust den Suchindex neu auf und exportierst die Wartungsseite.

<!-- source: prerequisites b7ae62128809 -->
## Voraussetzungen

- Die Konfiguration (`host`, `database`, `database_url`, `kek` und so weiter) ist so gesetzt, wie die [Konfigurationsreferenz](../reference/configuration.md) es beschreibt. `my config check` prüft sie.
- Das Schema ist auf die Datenbank angewendet (`my schema apply`; siehe [database.md](database.md)).
- `host` und `account_domain` sind endgültig. Der erste Befehl, der die Datenbank verwendet, merkt sie sich, und jeder spätere Befehl weigert sich zu laufen, wenn die Konfiguration abweicht (siehe [Einstellungen, die du später nicht mehr ändern kannst](production-checklist.md#settings-you-cannot-change-later)).

<!-- source: creating-an-administrator e5fec2e3d287 -->
## Einen Administrator anlegen

```sh
my admin create -username alice -email alice@example.com
```

- In einem Terminal fragt der Befehl zweimal nach dem Passwort, ohne es anzuzeigen.
- Ist die Standardeingabe kein Terminal, ist die erste Zeile der Standardeingabe das Passwort (zum Beispiel `my admin create ... < password.txt`).
  Das Passwort lässt sich weder als Kommandozeilenargument noch als Umgebungsvariable übergeben, weil diese in Prozesslisten und der Shell-Historie landen.
- Wo ein einmaliger Befehl weder ein Terminal noch eine Standardeingabe hat, etwa bei einem [Cloud Run job](deploy-cloud-run.md#9-create-the-first-administrator), füge `-password-link` hinzu. Der Befehl liest dann kein Passwort: Er setzt ein zufälliges, das niemand erfährt, und gibt nach der Zeile `created admin` einen Link aus, der das Passwort einmalig und eine Stunde lang festlegen lässt, wie `my admin account password-reset-link` (das Audit-Log verzeichnet ihn als `password_reset_link_issued`).
  Wer die Ausgabe des Befehls lesen kann, kann den Link verwenden, und auf einer solchen Plattform bleibt diese Ausgabe in den Logs der Plattform, bis der Link benutzt wird oder abläuft. Öffne den Link sofort. Läuft er ab, stelle mit `my admin account password-reset-link @alice` einen neuen aus. Schlägt das Ausstellen des Links fehl, existiert der Administrator bereits: Der Befehl sagt das und endet mit Exit-Code 1, und derselbe Befehl liefert einen neuen Link.
- Ein Passwort hat 8 bis 128 Zeichen. Es erscheint nie in der Ausgabe, in den Logs oder in einer Fehlermeldung.
- Eine Transaktion legt den Nutzer, das Passwort, die primäre E-Mail-Adresse, das lokale Konto (mit seinen Schlüsseln) und die Zuweisung der Rolle `admin` an (ohne Vergebenden, das heißt: die CLI).
  Die eingebauten Rollen werden zuerst angelegt, falls sie fehlen.
- Bei Erfolg gibt der Befehl `created admin @alice@<account_domain> (user <id>, account <id>)` aus und endet mit Exit-Code 0.
- Auf einem [Ein-Personen-Server](instance-settings.md#one-person-server) legt der Befehl kein Konto an und endet mit Exit-Code 1.
  Solange die [Anmeldung auf Login-Anbieter beschränkt ist](login-providers.md#limit-sign-in-to-login-providers), warnt er außerdem, dass das Passwort nicht zum Anmelden taugt, bis die Beschränkung endet.

<!-- source: running-it-again 197692f7f103 -->
## Erneut ausführen

- Ist der Nutzername oder die E-Mail-Adresse (jeweils ohne Unterscheidung von Groß- und Kleinschreibung) schon vergeben, ändert der Befehl nichts, gibt aus, was vergeben ist, und endet mit Exit-Code 1.
  Er ändert nie das Passwort oder die Rollen des vorhandenen Nutzers.
- Ein fehlgeschlagener Lauf schreibt nichts, du kannst also die Ursache beheben und denselben Befehl erneut ausführen.
- Exit-Code 2 bedeutet einen Aufruffehler.

<!-- source: granting-and-revoking-roles b87e1adb131e -->
## Rollen vergeben und entziehen

Rollen enthalten Berechtigungen. Die eingebauten Rollen sind `admin` (`position` 1000, jede Berechtigung) und
`moderator` (`position` 100, mit `manage_reports`, `manage_accounts` und `view_audit`).
`my serve` prüft die eingebauten Rollen bei jedem Start (in einem Prozess mit beliebigem `role`), und `my admin role` prüft sie bei jedem Aufruf.
Beide legen an, was fehlt, und geben `admin` jede Berechtigung der laufenden Version.
Wenn ein Upgrade eine Berechtigung hinzufügt, ist kein manueller Schritt nötig. Ein `moderator` behält die Berechtigungen und die Position, die der Administrator gesetzt hat.

```sh
my admin role list                 # each role's position, permissions (grouped as moderation, server administration, and member features), and holders
my admin role list -user alice     # the roles of alice
my admin role grant alice moderator
my admin role revoke alice moderator
```

- Berechtigungen gehören zu einer von drei Gruppen.
  - Moderation: `manage_reports`, `manage_accounts`, `manage_domains`, `manage_invites`, `view_audit`.
  - Serveradministration: `manage_settings`, `manage_rules`, `manage_roles`, `manage_jobs`, `manage_login_recovery`, `manage_custom_emojis`, `manage_webhooks`, `manage_announcements`, `manage_finance`, `manage_supporters`, `manage_oauth_apps`, `manage_service_closure`, `manage_errors`, `manage_login_providers`.
  - Mitgliederfunktionen: `view_finance`, `create_custom_emojis`, `start_voice_rooms`, `create_invites`, `keep_posts_longer`.

  `list` gibt eine Spalte pro Gruppe aus (`MODERATION`, `SERVER`, `MEMBER`). Der Webclient zeigt „Moderation“ und „Administration“ für die ersten beiden Gruppen und rechnet die Mitgliederfunktionen keiner von beiden zu (siehe [moderation.md](moderation.md)).
- Eine Änderung wirkt ab der nächsten Anfrage des Nutzers. Benachrichtigungen für Moderatoren gehen an die Personen, die die Berechtigung innehaben, wenn die Benachrichtigung erstellt wird.
- Vergaben und Entzüge werden als `role_granted` und `role_revoked` ins Audit-Log geschrieben (Zielart `role`). Der Akteur einer CLI-Operation ist `cli` (`my admin audit list -target-kind role`).
- Eine Rolle zu vergeben, die der Nutzer schon hat, oder eine zu entziehen, die er nicht hat, ändert und verzeichnet nichts.
- Der letzte Inhaber von `admin` kann die Rolle nicht verlieren (Exit-Code 1). Vergib `admin` zuerst an einen anderen Nutzer.
- Die Admin-API besteht aus `GET /api/v1/admin/roles`, `GET /api/v1/admin/accounts/{id}/roles` und `PUT` / `DELETE /api/v1/admin/accounts/{id}/roles/{role}` und erfordert die Berechtigung `manage_roles`.
  Über die API kannst du keine Rolle vergeben oder entziehen, die über der höchsten `position` deiner eigenen Rollen liegt (403). Den letzten `admin` zu entziehen liefert 409.
- Das Team handelt nur gegenüber niedrigeren Rängen. Über die API erfordern Moderationsaktionen und ihre Aufhebung, das Entscheiden von Einsprüchen, das Aufheben von E-Mail-Sperren, das Offenlegen privater Daten eines Kontos, das Ausstellen eines Passwort-Reset-Links und das Entfernen der Zwei-Faktor-Authentifizierung, dass die höchste `position` unter deinen Rollen über der jeder Rolle des Ziels liegt; ein höherer oder gleicher Rang, auch dein eigenes Konto, liefert 403. Ein Moderator kann nicht auf einen Administrator einwirken, und ein Administrator nicht auf einen anderen. Diese Befehle (`my admin account ...`) kennen keine solche Grenze, nutze sie also, wenn Administratoren uneins sind.
- Der einzige Administrator kann nicht ausgesperrt oder entfernt werden: `disable_login`, `suspend` und `ban` auf ihn werden abgelehnt, über die API (409) und über diese Befehle (Exit-Code 1), und er kann sein eigenes Konto nicht löschen (409). Ein weiterer Inhaber von `admin` zählt nur, solange er sich anmelden kann: nicht nach dem Löschen seines Kontos, nicht während einer Sperre (suspended oder banned) und nicht unter `disable_login`.

<!-- source: announcing-the-end-of-the-service e1c40ec145f0 -->
## Das Ende des Dienstes ankündigen

Um den Server zu schließen, kündige die Schließung zuerst an. [service-closure.md](service-closure.md) beschreibt das gesamte Vorgehen, von der Ankündigung bis zum Verwerfen der Daten.
Während der Ankündigung arbeitet der Server wie gewohnt weiter. `closure` in `GET /api/v1/instance` und `metadata.serviceClosure` in NodeInfo zeigen den Zustand und die Termine.

```sh
my admin closure announce -closes-at 2027-01-31T15:00:00Z -message "Thank you for using this server."
my admin closure announce -closes-at 2027-02-28 -export-until 2027-04-30   # change the dates (a bare date means 00:00 UTC)
my admin closure status                                                   # the state and the schedule in effect
my admin closure withdraw                                                 # withdraw the announcement
```

- `-closes-at` hat keine Untergrenze. `-export-until` (die Frist, bis zu der Daten mitgenommen werden können) muss am oder nach `-closes-at` liegen und beträgt standardmäßig 30 Tage nach dem Schließungsdatum.
- Liegt das Schließungsdatum weniger als 30 Tage in der Zukunft, gibt der Befehl eine Warnung auf der Standardfehlerausgabe aus (die Ankündigung wird trotzdem verzeichnet).
- Eine Änderung der Termine wird als neue Ankündigung verzeichnet, und eine Ankündigung nach einem Widerruf ist eine neue. Sobald der Server geschlossen ist, kannst du weder ankündigen noch widerrufen (Exit-Code 1).
- Ankündigen, Termine ändern und Widerrufen werden als `service_closure_announced` und `service_closure_withdrawn` ins Audit-Log geschrieben (Zielart `instance`; siehe `my admin audit list -target-kind instance`).
- Die Admin-API besteht aus `GET /api/v1/admin/closure`, `PUT` / `DELETE /api/v1/admin/closure/announcement`, `GET /api/v1/admin/closure/check` und `POST /api/v1/admin/closure/close`. Sie erfordert die Berechtigung `manage_service_closure` (unter den eingebauten Rollen hat nur `admin` sie).
- Während der Ankündigung nimmt der Server keine neuen Nutzer und keine neuen Zahlungen an.
  - Der wirksame Registrierungsmodus ist `closed` (der Wert von `registration_mode` ändert sich nicht und gilt wieder, wenn die Ankündigung widerrufen wird). Weder Registrierungen noch Registrierungen mit Einladung funktionieren, und ausstehende Anträge können nicht genehmigt werden (`409`).
  - Das Ankündigen lehnt jeden ausstehenden Antrag ab (verzeichnet als entschieden durch `service_closure`) und kündigt alle laufenden Abos zum Ende ihres Zeitraums.
    Die Abonnenten erhalten eine Benachrichtigung (`supporter_service_closure`) und eine E-Mail. Neue Abos liefern `409`. Der Widerruf der Ankündigung macht die Ablehnungen und Kündigungen nicht rückgängig.
  - Jede Ankündigung, Terminänderung und jeder Widerruf sendet die Benachrichtigung `service_closure` an alle lokalen Nutzer. All das läuft als Jobs, halte den Worker also nicht an.
  - Jeder Nutzer mit bestätigter E-Mail-Adresse erhält eine E-Mail bei der Ankündigung, bei einer Terminänderung, bei einem Widerruf, 7 Tage und 1 Tag vor dem Schließungsdatum, bei der Schließung und 7 Tage vor der Exportfrist. Nutzer können sie nicht abbestellen (siehe [email.md](email.md)).
  - Nach dem Schließungsdatum erhalten die Personen mit `manage_service_closure` die Benachrichtigung `admin_service_closure_due`. Der Server schließt sich nicht von selbst.

Schließe den Server nach dem Schließungsdatum. Das Schließen lässt sich nicht rückgängig machen.

```sh
my admin closure check                                      # review what the closure ends
my admin closure close                                      # show the review, then ask for y and the domain name
my admin closure close -confirm-domain social.example       # for automation: pass the domain name instead of answering
```

- `check` und `close` zeigen die Anzahl der lokalen Konten, die Anzahl der Ziel-Inboxen (die bekannten Shared Inboxes), eine Schätzung der zu sendenden `Delete`-Aktivitäten (ihr Produkt), die Anzahl der laufenden Abos,
  die Anzahl der unfertigen Migrationen (`Move`) und die Anzahl der Konten unter Aufbewahrungssperre (`retention_holds`).
  Bleiben Migrationen übrig oder besteht eine Sperre, warnt der Befehl auf der Standardfehlerausgabe. Exportiere die Datensätze der Konten unter Sperre mit `my admin account export-moderation` und bewahre sie auf, bevor du die Daten verwirfst.
- `close` weigert sich ohne Nachfrage, wenn es keine Ankündigung gibt und vor dem Schließungsdatum (Exit-Code 1). Jede andere Antwort als `y` oder eine Eingabe, die von der Domain des Servers abweicht (`host` in der Konfiguration, ohne Unterscheidung von Groß- und Kleinschreibung), ändert nichts und endet mit Exit-Code 1.
- Die Schließung wird als `service_closure_closed` ins Audit-Log geschrieben. In derselben Transaktion beendet sie jedes laufende Abo (Grund `service_closed`) und stellt die Jobs in die Warteschlange, die die Abos beim Zahlungsanbieter sofort kündigen (`CancelNow`).
- Nach der Schließung lässt die Client-API nur noch diese Anfragen durch: das Lesen des eigenen Kontos und der eigenen Einstellungen, die Exporte (`/exports`, `/csv_exports`), Sitzungen (`/sessions`, `/authorized_apps`), `GET /instance` und die Verwaltung der Schließung und der Job-Warteschlange.
  Alles andere wird mit `403` abgelehnt (`/api/v1/problems/service-closed`). Streams enden, und Web Push sendet nichts. Anmelden und Zurücksetzen eines Passworts funktionieren weiterhin.
- Nach der Schließung stellt der Worker ein `Delete` des Akteurs jedes lokalen Kontos an die Föderationspartner zu, jeweils 100 auf einmal (migrierte Konten zuletzt). Er sendet sie auch, wenn `federation_mode` auf `off` oder `inbound` steht, und nie an eine gesperrte Domain.
  `my admin closure status` (und `progress` in `GET /api/v1/admin/closure`) zeigt, für wie viele Konten ein `Delete` verzeichnet ist, wie viele Zustellungen noch aufzufächern sind und die Anzahl der Zustellungen in jedem Zustand (`pending`, `succeeded`, `permanent_failure`, `skipped`, `dead`). Lass den Worker während der Zustellung laufen.
  Sind alle Zustellungen abgeschlossen, verzeichnet eine Prüfung, die alle 10 Minuten läuft, `delivery_completed` (`service_closure_delivery_completed` im Audit-Log), und die lokalen Akteure antworten danach mit `410`.
  Der Server wiederholt `dead`-Zustellungen nicht von selbst; wiederhole sie mit `my job retry`.

Schreibe nach Ablauf der Exportfrist die statische Abschlussseite, die deine Domain ausliefert, sobald der Server angehalten ist. Der Befehl liest die Datenbank, führe ihn also aus, bevor du die Datenbank verwirfst.

```sh
my admin closure export-html -out ./closure              # every locale (the instance's default locale first)
my admin closure export-html -out ./closure -lang ja,en  # choose the locales and the order in index.html
```

- Ausgabe: `<dir>/<locale>.html` für jede Locale und `<dir>/index.html` mit allen Locales nacheinander. Die Seiten verwenden dieselbe Vorlage wie `my maintenance export` und zeigen den Namen des Servers, das Schließungsdatum, die Nachricht des Betreibers aus der Ankündigung und `status_page_url`.
  Die Ausgabe enthält außerdem Konfigurationsbeispiele für das Front-End: `nginx.conf`, `Caddyfile` und `cloudfront-function.js`. Sie antworten mit `410` auf `/ap/*`, `/inbox`, WebFinger, host-meta, NodeInfo und `/@*` und liefern für alles andere die Abschlussseite aus.
- Der Befehl weigert sich (Exit-Code 1), solange der Server nicht geschlossen ist. Vor `delivery_completed` gibt er eine Warnung aus und schreibt die Seiten trotzdem. [service-closure.md](service-closure.md) beschreibt, wie du das Front-End auf die statische Seite umstellst.

<!-- source: processing-uploaded-images-again 65a9e3da0bd8 -->
## Hochgeladene Bilder erneut verarbeiten

Jedes hochgeladene Bild behält drei Größen, eine Animation auch ihre bewegten Versionen. Eine neue Version von `my` kann diese Größen oder Formate ändern, und ein vorhandenes Bild behält seine Dateien, bis du es erneut verarbeitest:

```sh
my admin media reprocess -before 2027-01-31T09:00:00Z
```

- `-before` nimmt den Zeitpunkt der Änderung, etwa den des Upgrades, im Format RFC 3339. Es werden nur Bilder erfasst, die zuletzt vor diesem Zeitpunkt verarbeitet wurden, und ein Zeitpunkt, der noch nicht erreicht ist, wird abgelehnt.
- Der Worker erledigt die Arbeit, 100 Bilder pro Minute, jedes in einem eigenen Job (`media.reprocess_walk` und `media.reprocess` in der [Job-Referenz](../reference/jobs.md)). Lass einen Worker laufen; die Admin-Seite für Jobs (`/admin/jobs`) zeigt die Warteschlange.
- Es ist sicher, den Befehl mit demselben Zeitpunkt erneut auszuführen. Bereits verarbeitete Bilder bleiben unberührt, und die, die der erste Lauf nicht erreicht hat, werden nachgeholt.
- Originale werden nicht aufbewahrt, deshalb wird jedes Bild aus seiner großen Version neu erzeugt. Details, die die große Version verloren hat, kommen nicht zurück, und jeder Lauf mit einem späteren Zeitpunkt kodiert die Bilder noch einmal neu und verliert etwas Qualität. Führe ihn pro Änderung nur einmal aus.
- Video- und Audiodateien werden nicht neu kodiert und bleiben unberührt. Ein Bild, das die aktuellen Grenzen nicht mehr akzeptieren, behält seine aktuelle Version, und der Worker schreibt eine Warnung ins Log.

Die neue Version eines Bildes hat neue URLs. Die alten Dateien bleiben für `media_replaced_retention` erhalten (standardmäßig 30 Tage, [Konfigurationsreferenz](../reference/configuration.md#media_replaced_retention)), damit geöffnete Seiten und Caches weiter funktionieren; danach löscht der stündliche Job `media.purge_replaced` sie. Andere Server erfahren nichts von den neuen URLs: Ein Server, der noch eine alte URL hält und keine eigene Kopie hat, zeigt das Bild nach dieser Zeit nicht mehr an. Plane den zusätzlichen Speicher ein, bis die alten Dateien verschwinden.

<!-- source: rebuilding-the-search-index 9061821656ac -->
## Den Suchindex neu aufbauen

`my search` verwaltet den Index der Volltextsuche von Beiträgen auf einem Server mit Suchmaschine. Mit `search = none` sagt jeder Befehl das, tut nichts und endet mit Exit-Code 0.

```sh
my search rebuild              # empty the index, then have the worker add every post that qualifies
my search rebuild --keep       # add them without emptying the index
my search rebuild --rate 50    # at most 50 posts a second (20 by default, 1000 at most)
my search clear                # empty the index and end any rebuild
my search status               # the backend and the progress and failures of the latest rebuild
```

Führe `my search rebuild` erneut aus, um einen abgebrochenen Aufbau fortzusetzen. Er macht beim zuletzt bearbeiteten Beitrag weiter und leert den Index nicht noch einmal. Solange einer unfertig ist, wird der jeweils andere Modus mit Exit-Code 1 abgelehnt. [Volltextsuche von Beiträgen](search.md) sagt, wann du welchen verwendest.

<!-- source: exporting-the-maintenance-page 1756fcf1e02e -->
## Die Wartungsseite exportieren

`my maintenance export` schreibt die Seite des Wartungsmodus als statisches HTML,
damit ein Load Balancer, CDN oder Reverse-Proxy sie ausliefern kann, während `my serve` selbst nicht laufen kann (ein Image wird ersetzt, eine Revision startet nicht).
Sie verwendet dieselbe Vorlage wie der Wartungsmodus, beide zeigen also dieselben Worte und denselben Look.

```sh
my maintenance export -out ./maintenance -lang en,ja            # reads the database for at most 5 seconds
my maintenance export -out ./maintenance -lang en,ja -offline   # the configuration and the default theme only
```

- Ausgabe: `<dir>/<locale>.html` für jede Locale aus `-lang`, für Front-Ends, die eine Seite per `Accept-Language` wählen,
  und `<dir>/index.html` mit allen Locales nacheinander, für solche, die das nicht können.
  Die Seiten haben Inline-CSS, kein Skript und keine externe Ressource; ihre Content-Security-Policy steht in einem `<meta>`-Element.
- Worte: die Einstellungen `maintenance_server_name`, `maintenance_message`, `maintenance_until` und `status_page_url`
  ([Konfigurationsreferenz](../reference/configuration.md)).
- Ohne `-offline` liest der Befehl außerdem den Namen des Servers (wenn `maintenance_server_name` nicht gesetzt ist), ihre Standard-Locale (wenn `-lang` nicht angegeben ist)
  und die Token-Überschreibungen ihres Themes. Das benutzerdefinierte CSS des Themes wird nie eingebunden. Kann die Datenbank nicht gelesen werden,
  gibt er eine Warnung aus, schreibt die Seiten aus der Konfiguration und dem Standard-Theme und endet mit 0.
- Führe ihn erneut aus, wenn sich diese Einstellungen oder das Theme ändern, und vor jeder Wartung, damit die Seiten im Front-End aktuell bleiben.
