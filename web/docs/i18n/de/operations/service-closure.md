<!-- source: closing-the-service 30c438995ba1 -->
# Den Dienst einstellen

Was du der Reihe nach tust, wenn du den Betrieb des Servers beendest: Nutzer ihre Daten mitnehmen lassen, die Konten löschen, die auf anderen Servern bestehen bleiben,
dann den Server stoppen und auf eine statische Abschlussseite umstellen.
Jeder Befehl ist ausführlicher in [Administratoren und Rollen](admin-cli.md#announcing-the-end-of-the-service) beschrieben.

<!-- source: the-sequence 4557c35b6851 -->
## Der Ablauf

![Die Phasen der Einstellung des Dienstes von oben nach unten: was der Betreiber links tut und der Zustand des Servers rechts, mit dem Abschlussdatum und der Export-Frist dazwischen. Die Tabelle unten beschreibt jede Phase.](images/service-closure.svg)

Die Tabelle folgt dem Diagramm von oben nach unten; die Spalte „Server“ beginnt mit dem Zustand, den `my admin closure status` zeigt.
Das Abschlussdatum (`-closes-at`) beendet die Ankündigungsfrist, und die Export-Frist (`-export-until`, 30 Tage nach dem Abschlussdatum,
sofern nicht gesetzt) beendet Phase 5.

| Phase | Server | Was der Betreiber tut |
| --- | --- | --- |
| 1. Ankündigen | `announced`. Läuft wie gewohnt. Registrierungen, Einladungen und neue Unterstützer-Abos enden | `my admin closure announce`, eine Mitteilung auf der Statusseite veröffentlichen |
| 2. Ankündigungsfrist | `announced`. Läuft wie gewohnt. Nutzer können ihre Daten exportieren und zu einem anderen Server umziehen | Jahresunterstützern das Geld erstatten, Umzüge und Aufbewahrungssperren prüfen |
| 3. Schließen (nach dem Abschlussdatum) | `closed`. Nutzer können sich nur anmelden und exportieren. Der Server stellt für jedes Konto ein `Delete` zu | `my admin closure check` und `close` |
| 4. Zustellung abgeschlossen | Hält `delivery_completed` fest; lokale Actors geben außerdem `410` zurück | Mit `my admin closure status` bestätigen. Die Worker nicht stoppen |
| 5. Bis zur Export-Frist | `delivery_completed`. Bleibt geschlossen und bietet weiter den Export an | Die Datensätze unter Aufbewahrungssperren exportieren |
| 6. Statische Seite | Gestoppt. Das Front-End liefert die statische Seite und `410` für die Föderation | `my admin closure export-html`, das Front-End umstellen, `my serve` stoppen |
| 7. Verwerfen | Keiner. Liefere die statische Seite und `410` noch etwa ein Jahr weiter aus | Die DB, Blobs und Backups verwerfen. Die Domain etwa ein Jahr behalten |

Du kannst jede Phase mit dem Wartungsmodus kombinieren ([Wartung](maintenance.md)). Während der Wartung stoppen Exporte und die Zustellung von `Delete`-Aktivitäten.

<!-- source: 1-announce b34cb9474667 -->
## 1. Ankündigen

Lege das Abschlussdatum (`-closes-at`) und die Frist fürs Mitnehmen der Daten (`-export-until`, 30 Tage nach dem Abschlussdatum, wenn weggelassen) fest und kündige sie dann an.

```sh
my admin closure announce -closes-at 2027-01-31T15:00:00Z -message "Thank you for being here."
my admin closure status
```

- Das Abschlussdatum hat keine untere Grenze, aber eine Warnung erscheint, wenn die Zeit von der Ankündigung bis zum Abschlussdatum kürzer als 30 Tage ist. Gib den Nutzern Zeit zum Exportieren und Umziehen.
- Um das Datum zu ändern, führe `announce` erneut aus; um abzusagen, führe `my admin closure withdraw` aus. Beides sendet jedem Nutzer eine Benachrichtigung `service_closure`.
- Nach der Ankündigung wird der wirksame Registrierungsmodus `closed`, ausstehende Anträge werden abgelehnt, und jedes laufende Unterstützer-Abo wird zum Ende seines Zeitraums gekündigt (Abonnenten erhalten eine Benachrichtigung und eine E-Mail).
  Das Zurückziehen der Ankündigung macht die Ablehnungen und Kündigungen nicht rückgängig.
- Benachrichtigungen werden von Jobs erzeugt, stoppe die Worker also nicht.
- Veröffentliche auf der Statusseite ([Überwachung und Statusseite](monitoring.md)) das Abschlussdatum, die Export-Frist und wie man exportiert und umzieht.
  Die Statusseite überlebt den Server, sie führt also auch Menschen, die nach der Einstellung vorbeikommen.

<!-- source: 2-notice-period b18535a9c0f7 -->
## 2. Ankündigungsfrist

- Nutzer können auf dem Einstellungsbildschirm ihre Daten exportieren (den Datenexport und die CSV-Dateien) und zu einem anderen Server umziehen (`Move`).
  Ein Banner auf jedem Bildschirm und eine Benachrichtigung zeigen das Abschlussdatum, die Export-Frist und den Weg zu diesem Bildschirm.
- **Jahresunterstützer**: Die Ankündigung kündigt Abos zum Ende des Zeitraums, aber bei einem Abo, dessen Zeitraum nach dem Abschlussdatum endet, ist die Zeit, in der es nicht genutzt werden kann, bereits bezahlt.
  Der Server erstattet nichts, erstatte also jedem Abonnenten im Bildschirm des Zahlungsanbieters das Geld (in Stripe im Zahlungsbildschirm des Dashboards, in PAY.JP im Zahlungsbildschirm der Admin-Konsole) ([Unterstützer-Zahlungen (Stripe)](payments-stripe.md), [Unterstützer-Zahlungen (PAY.JP)](payments-payjp.md)).
- Nachdem das Abschlussdatum verstrichen ist, erhalten Personen mit `manage_service_closure` eine Benachrichtigung `admin_service_closure_due`. Der Server schließt nicht von selbst.

<!-- source: 3-close c851780cf430 -->
## 3. Schließen

Sobald das Abschlussdatum verstrichen ist, führe vor dem Schließen die Prüfungen aus.

```sh
my admin closure check
```

- **Unfertige Umzüge** erzeugen eine Warnung. Warte, bis sie abgeschlossen sind, bevor du schließt, denn nach dem Schließen kann niemand mehr einen `Move` starten.
- **Aufbewahrungssperren (`retention_holds`)** auf einem Konto erzeugen eine Warnung.
  Die Pflicht hinter einer Sperre endet nicht, wenn du den Server schließt, exportiere also die Datensätze unter einer Sperre und bewahre sie außerhalb des Servers auf (liste die Sperren mit `my admin retention list` auf).

  ```sh
  my admin account export-moderation @alice > alice-moderation.json
  ```

Wenn die Prüfungen abgeschlossen sind, schließe. Das lässt sich nicht rückgängig machen. Bestätige durch Eingabe von `y` und dem Domainnamen (verwende in der Automatisierung `-confirm-domain`).

```sh
my admin closure close
```

Nach dem Schließen können Nutzer sich nur anmelden und ihre Daten exportieren. Die verbleibenden Unterstützer-Abos werden sofort gekündigt,
und ein `Delete` (Actor) für jedes lokale Konto wird an andere Server zugestellt (auch wenn `federation_mode` `off` oder `inbound` ist).
Die Inbox und GET-Anfragen auf Beiträge und Sammlungen geben `410 Gone` zurück.

<!-- source: 4-confirming-that-delivery-is-complete b545acd9a55c -->
## 4. Bestätigen, dass die Zustellung abgeschlossen ist

**Stoppe `my serve` und die Worker (Prozesse, deren `role` `all` oder `worker` ist; bei einer Push-Queue die Cloud-Tasks-Queue) nicht, bevor der Zustand `delivery_completed` ist.**
Die Worker stellen die `Delete`-Aktivitäten zu, und solange sie das tun, liefert der Server weiter das Dokument und den Schlüssel des lokalen Actors aus, damit Empfänger die Signatur des `Delete` prüfen können.
Wenn du zu früh stoppst, bleiben die Konten auf anderen Servern bestehen.

```sh
my admin closure status
```

- `accounts with a Delete` ist die Zahl der Konten, für die ein `Delete` festgehalten wurde, `fan-outs pending` ist die verbleibende Fan-out-Arbeit, und `deliveries <state>` ist die Zahl der Zustellungen pro Zustand.
- Zustellungen werden nach der üblichen Richtlinie wiederholt (bis zu 9 Mal über etwa 4 Tage). Wenn alle abgeschlossen sind, hält die Prüfung, die alle 10 Minuten läuft, `delivery_completed` fest (`state: delivery_completed`).
  Von da an geben auch die lokalen Actors und der Server-Actor `410` zurück.
- Eine Zustellung, die `dead` wurde, wird nicht automatisch wiederholt. Um sie zu wiederholen, verwende `my job retry`.

<!-- source: 5-until-the-export-deadline 6464f67f67d1 -->
## 5. Bis zur Export-Frist

Auch nach Abschluss der Zustellung läuft der Server geschlossen bis `export_until` weiter und bedient weiter die Exporte der Nutzer.
Der Server löscht nie von selbst Daten, er läuft also über die Frist hinaus weiter, bis du ihn stoppst.
Wenn du die Datensätze unter Aufbewahrungssperren (§3) noch nicht exportiert hast, tu es jetzt.

<!-- source: 6-switching-to-the-static-page 9795c9aea7ae -->
## 6. Auf die statische Seite umstellen

Exportiere nach der Export-Frist die Abschlussseite. **Das liest die DB, führe es also aus, bevor du `my serve` stoppst und die DB verwirfst.**

```sh
my admin closure export-html -out ./closure             # all locales; index.html starts from the instance's default locale
my admin closure export-html -out ./closure -lang ja,en # set the locales and the order in index.html
```

- Was es schreibt:
  - `<dir>/<locale>.html` und `<dir>/index.html`, die alle Locales der Reihe nach auflistet. Sie verwenden dieselbe Vorlage und dasselbe CSS wie die Wartungsseite (`my maintenance export`) und zeigen
    den Servernamen (`maintenance_server_name`, sonst den Servernamen, sonst den Hostnamen), das Abschlussdatum, die Nachricht des Betreibers aus der Ankündigung und einen Link zur Statusseite (`status_page_url`).
    Das CSS ist nur inline, es gibt keine Skripte oder externen Ressourcen, und die CSP steht in einem `<meta>`-Tag. Überschreibungen der Themenfarben sind eingebaut; eigenes CSS ist nicht enthalten.
  - Beispiele für die Front-End-Konfiguration `nginx.conf`, `Caddyfile` und `cloudfront-function.js`. Jedes gibt `410` ohne Body für die Pfade der Föderation und der Discovery (`/ap/*`, `/inbox`, `/.well-known/webfinger`,
    `/.well-known/host-meta`, `/.well-known/host-meta.json`, `/.well-known/nodeinfo`, `/nodeinfo/*`) und für Profile (`/@*`) zurück,
    und liefert für jeden anderen Pfad die Seite der ersten Sprache in `Accept-Language` (oder `index.html`, wenn es keine gibt).
    Eine Sprache erreicht die Seite der Locale, in der die App sie anzeigen würde: `zh`, `zh-SG` und `zh-Hans` erhalten `zh-CN.html`, und `zh-Hant`, `zh-HK` und `zh-MO` erhalten `zh-TW.html`.
- Der Befehl lehnt ab, wenn der Server nicht geschlossen ist (Exit-Code 1). Vor `delivery_completed` warnt er, dass Zustellungen weiterlaufen (und schreibt die Dateien trotzdem).

Stelle das Front-End um. Lege nur die Seiten (`*.html`) an den öffentlichen Ort und passe den Hostnamen, das Verzeichnis und die Zertifikatspfade im Beispiel an deine Umgebung an.

| Front-End | So richtest du es ein |
| --- | --- |
| nginx | Lege die Seiten in `/srv/closure` ab und binde `nginx.conf` aus dem `http`-Block ein. Die Seiten werden auch mit `410` ausgeliefert |
| Caddy | Lege die Seiten in `/srv/closure` ab und verwende das `Caddyfile`. Caddy besorgt das Zertifikat, und die Seiten werden auch mit `410` ausgeliefert |
| CloudFront | Lege die Seiten direkt unter einen S3-Bucket, mache diesen Bucket zum Origin des Standard-Cache-Behaviors und verknüpfe `cloudfront-function.js` als CloudFront Function für Viewer Requests (`cloudfront-js-2.0`). Nimm `Accept-Language` in den Cache-Schlüssel der Cache-Richtlinie auf. Die Seiten werden mit `200` ausgeliefert (eine Viewer-Request-Funktion kann den Status der Antwort des Origins nicht ändern) |

Prüfe nach dem Umstellen, dass `curl -i https://social.example/.well-known/webfinger?resource=acct:alice@social.example` `410` zurückgibt und
dass `curl -i https://social.example/` die Abschlussseite zurückgibt. Stoppe dann `my serve` und die Worker.
Für die Verfügbarkeitsüberwachung stoppst du entweder die Health-Check-Monitore oder richtest sie auf die Abschlussseite.

<!-- source: 7-discarding-the-db-blobs-and-backups c7c55aac56e0 -->
## 7. DB, Blobs und Backups verwerfen

Verwirf nach dem Stoppen die DB, die Blobs (Medien und Exportdateien), den KV-Store und die Queues samt ihren Backups und Snapshots. Mit einer [Suchmaschine](search.md#closing-the-service) verwirfst du auch deren Index und Snapshots: Sie enthalten den Text von Beiträgen.
Bestätige, bevor du irgendetwas verwirfst, noch einmal, dass du die Datensätze unter Aufbewahrungssperren exportiert und aufbewahrt hast. Das Verwerfen lässt sich nicht rückgängig machen.

**Behalte die Domain und liefere die statische Seite und `410` noch etwa ein Jahr weiter aus.** Andere Server sehen das `410`, hören auf abzurufen und zuzustellen und leeren ihre Caches.
Föderierte URIs verwenden UUIDs, es besteht also wenig Risiko, dass ein Actor mit derselben `id` erneut erstellt wird,
aber wenn jemand anderes die Domain übernimmt, gehören das Handle (`acct:alice@social.example`) und die Profil-URLs ihm und können dazu dienen, deine Nutzer zu imitieren.
Verlängere die Domain und das Zertifikat weiter, und überlege es dir noch einmal, bevor du die Domain aufgibst, auch nach Ablauf des Jahres.
