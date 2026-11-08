<!-- source: full-text-search-of-posts fc16f55b56ea -->
# Volltextsuche für Beiträge

Was die Volltextsuche für Beiträge enthält, wie du sie einschaltest und änderst, wie du ihren Index neu aufbaust und was sie von deinen Logs und Backups verlangt.

<!-- source: choosing-an-engine 9cfd51361e0a -->
## Eine Engine wählen

Die Volltextsuche für Beiträge braucht eine Suchmaschine, die du selbst betreibst oder mietest. [`search`](../reference/configuration.md#search) wählt sie aus:

- `none`, der Standard: keine Volltextsuche. Der Server verbindet sich mit nichts, stellt keinen Suchjob in die Queue, und der Client bietet keine Möglichkeit, Beiträge zu durchsuchen. Auf einem solchen Server sagt jeder Befehl `my search` das und tut nichts.
- `elasticsearch`, `opensearch` oder `meilisearch`: diese Engine, mit den Einstellungen ihres Abschnitts in [Suchmaschinen](search-engines.md).
- `plugin`: ein Programm von dir, das den Index in einer beliebigen anderen Engine pflegt, etwa Groonga oder einem mit Sudachi gebauten Index, unter `plugin_search_address` ([Beiträge mit einem Plugin durchsuchen](plugins.md#searching-posts)).

Die Suche nach Konten und Hashtags arbeitet mit der Datenbank und hängt nicht von dieser Einstellung ab.

Der Server besitzt nur die Felder, die er schreibt, filtert, sortiert und seitenweise abruft. Wie die Engine Text analysiert, wie sie Ergebnisse gewichtet und wie groß ihr Index wird, stellst du selbst über eine Einstellungsdatei der Engine ein; [Suchmaschinen](search-engines.md) beschreibt, was jede Engine mit den eingebauten Einstellungen auf Japanisch findet und wie du das änderst.

<!-- source: what-the-index-holds 5e0884482ea8 -->
## Was der Index enthält

Die Datenbank bleibt die maßgebliche Quelle, und der Index ist eine Kopie, die du wegwerfen und neu aufbauen kannst. Eine Engine liefert nur Beitrags-IDs zurück: Der Server liest jeden Beitrag erneut aus der Datenbank und wendet die üblichen Sichtbarkeitsregeln an. Ein Index, der hinterherhinkt, kann also dazu führen, dass ein Beitrag nicht gefunden wird, aber nie dazu, dass einer sichtbar wird, der es nicht sein soll.

Ein Beitrag steht im Index, wenn alle diese Bedingungen zutreffen:

- Er ist nicht gelöscht.
- Seine Sichtbarkeit ist `public` oder `local`, und [`search_visibilities`](instance-settings.md) enthält sie. Beiträge mit `unlisted`, `private` und `direct` werden nie indexiert, egal wie die Einstellung lautet.
- Sein Autor hat `indexable` eingeschaltet, die Einstellung, die in [Suchmaschinen und KI-Crawler](crawlers.md) beschrieben ist.
- Sein Autor und die Domain des Autors haben keine Einschränkung, die sie aus der Entdeckung ausblendet.

Der Index enthält den Text, die Inhaltswarnung und die Alternativtexte der aktuellen Version jedes Beitrags, mit Autor und Sprache. Umfragen, Profile und ältere Versionen enthält er nicht.

`indexable_default` ist standardmäßig `false`, ein Server, der die Suche gerade erst eingeschaltet hat, findet also wenig, bis die Leute selbst `indexable` wählen.

<!-- source: turning-it-on 3d88170680a2 -->
## Einschalten

1. Starte die Engine und setze `search` und die Einstellungen ihres Backends.
2. Wenn du Worker getrennt betreibst, entscheide über [ihre Aufteilung](#keeping-search-from-delaying-other-jobs).
3. Konfiguriere [deine Logs](#the-query-appears-in-urls).
4. Wenn die Engine von jemand anderem betrieben wird, nimm sie in deine [Datenschutzdokumente](#your-privacy-documents) auf.
5. Starte den Server neu; von da an kommen neue Beiträge in den Index.
6. Führe `my search rebuild` aus, um die vorhandenen Beiträge hinzuzufügen.

Die Suche funktioniert während des Neuaufbaus, und ihre Ergebnisse sind unvollständig, bis er endet.

<!-- source: rebuilding-the-index 8a8ea41dfc98 -->
## Den Index neu aufbauen

```sh
my search rebuild              # empty the index, then add every post that qualifies
my search rebuild --keep       # add them without emptying the index
my search rebuild --rate 50    # at most 50 posts a second
my search status               # the progress and the failures
my search clear                # empty the index and end any rebuild
```

Der Befehl kehrt zurück, sobald der Neuaufbau läuft, und der Worker erledigt den Rest als `search.rebuild`-Jobs ([Job-Referenz](../reference/jobs.md#full-text-search)); lass also einen Worker laufen. Jeder Job deckt 100 Beiträge ab.

- **Rate.** Der Standard sind 20 Beiträge pro Sekunde, höchstens 1000, und es zählt jeder angesehene `public`- und `local`-Beitrag, auch die, die nicht infrage kommen. Beim Standard dauern eine Million Beiträge etwa 14 Stunden. So bleibt Platz für andere Jobs und die Datenbank; erhöhe die Rate also nur, wenn dein Server Reserven hat.
- **Welchen Modus du startest.** Der Standard entfernt alles, was nicht im Index sein soll: Dokumente, die von einer Löschung übrig sind, die nie ankam, oder vom Zurückspielen der Datenbank aus einem Backup. Bis er endet, findet die Suche weniger. Nimm `--keep` nach dem [Hinzufügen einer Sichtbarkeit](#changing-search_visibilities), wenn nichts entfernt werden muss und die Suche vollständig bleiben soll. `--keep` entfernt nie das Dokument eines Beitrags, der nicht mehr in der Datenbank ist.
- **Unterbrochen?** Führe denselben Befehl erneut aus. Er macht beim zuletzt bearbeiteten Beitrag weiter und leert den Index nicht noch einmal. Mit `--rate` änderst du die Rate. Ein Neuaufbau läuft auch von selbst weiter, wenn ein Worker neu startet.
- **Der andere Modus.** Solange ein Neuaufbau nicht beendet ist, wird der Befehl im anderen Modus abgelehnt, weil er nicht tun würde, was er sagt. Um in einem der beiden Modi von vorn zu beginnen, führe `my search clear` und danach den gewünschten Befehl aus.
- **Zweimal ausführen** ergibt denselben Index. Beiträge, die während eines Neuaufbaus erstellt, bearbeitet oder gelöscht werden, sind am Ende richtig, weil ihre eigenen Jobs weiterlaufen und jeder Job den Beitrag so liest, wie er in diesem Moment ist.

`my search status` zeigt das Backend, `search_visibilities` und für den letzten Neuaufbau seine Phase (`clearing`, `filling` oder `finished`), die bearbeiteten und fehlgeschlagenen Beiträge, die Rate und den letzten Fehler.

Wenn etwas schiefgeht:

- **Die Engine ist ausgefallen.** Wenn drei Beiträge hintereinander fehlschlagen, hält der Job dort an, speichert den Fehler und versucht es erneut. Ein Neuaufbau, dem die Wiederholungen ausgegangen sind, hinterlässt einen fehlgeschlagenen Job (`my job list`), und `updated` im Status bewegt sich nicht mehr. Sobald die Engine wieder da ist, führe denselben Befehl aus, um fortzufahren.
- **Die Engine lehnt einen Beitrag ab.** Der Beitrag wird als fehlgeschlagen vermerkt, und der Neuaufbau geht weiter. Der Status listet bis zu 20 davon auf. Behebe die Ursache und führe dann `my search rebuild --keep` aus, um alle Beiträge erneut zu versuchen.

<!-- source: changing-search_visibilities 4adf54d02fe9 -->
## `search_visibilities` ändern

Lässt du eine Sichtbarkeit weg, entfernt das ihre Beiträge von selbst aus dem Index. Fügst du eine hinzu, findet die Suche nur neue Beiträge, bis du `my search rebuild --keep` ausführst.

**Sag es deinen Nutzern, bevor du `local` hinzufügst.** `indexable` deckt dann die `local`-Beiträge aller ab, die es schon eingeschaltet haben, und der Neuaufbau fügt ihre früheren `local`-Beiträge hinzu, ohne sie erneut zu fragen. Die Profileinstellungen sagen, was `indexable` auf deinem Server abdeckt, aber Leute, die es vor langer Zeit gewählt haben, sehen das nicht, wenn du es ihnen nicht sagst.

<!-- source: switching-off-or-changing-the-engine 538a78a629e3 -->
## Die Engine abschalten oder wechseln

Wenn der Server zu `search = none` oder zu einer anderen Engine wechselt, verliert er die Verbindung zum alten Index. Dieser Index wird nicht mehr bereinigt, Beiträge, die danach gelöscht werden, bleiben also samt Text darin.

1. Stoppe den Server und seine Worker, damit nach dem nächsten Schritt nichts mehr in den Index schreibt.
2. Führe `my search clear` aus.
3. Ändere `search` und starte den Server.
4. Führe bei einer neuen Engine `my search rebuild` aus.

Wenn du schon gewechselt hast oder den Server nicht stoppen kannst, lösche den alten Index in der Engine selbst. `my search clear` erreicht ihn nicht mehr, sobald sich die Einstellung geändert hat. Jobs, die vor der Änderung in der Queue lagen, tun nach einem Wechsel zu `none` nichts.

**Rückkehr von `none`.** Führe `my search rebuild` ohne `--keep` aus: Der Server führt nicht Buch darüber, was gelöscht wurde, während er keine Engine hatte, und nur das Leeren des Index entfernt diese Beiträge. Wenn beim Abschalten ein Neuaufbau unfertig war, führe zuerst `my search clear` aus, denn eine Wiederaufnahme leert den Index nicht.

<!-- source: keeping-search-from-delaying-other-jobs c01b5c259acf -->
## Verhindern, dass die Suche andere Jobs verzögert

Standardmäßig führt ein Worker einen Job nach dem anderen aus; eine langsame Engine teilt sich also die Zeit dieses Workers mit `federation.inbox`, obwohl jeder Aufruf der Engine ein Limit von 10 Sekunden hat. Ein ausgelasteter Server sollte sie trennen:

- Starte einen Worker-Prozess mit `worker_job_types = search.*` und die anderen mit `worker_job_types_except = search.*` ([Worker nach Jobtyp aufteilen](running.md#splitting-workers-by-job-type)).
- Schicke bei `queue = cloudtasks` die Typen `search.*` mit einer [Queue-Route](running.md#routing-job-types-to-their-own-queues) in eine eigene Queue.

Ein kleiner Server kann sie zusammenlassen; die Suchjobs teilen sich dann die Zeit mit der Föderation.

<!-- source: the-query-appears-in-urls 488e2c1e91dc -->
## Die Suchanfrage erscheint in URLs

`GET /api/v1/statuses/search` trägt in seiner URL, als `?q=`, was Leute suchen. Die `http request`-Logzeile des Servers enthält den Pfad und die Route, nicht den Query-String, und auch Traces und Fehlerberichte lassen ihn weg. Was auch immer vor dem Server sitzt, kann ihn jedoch aufzeichnen, und eine Suche kann viel über einen Menschen verraten. Prüfe jede Schicht deines Setups und halte die Suchanfrage entweder aus ihrem Log heraus oder behandle dieses Log als personenbezogene Daten, mit passendem Zugriff und passender Aufbewahrung.

| Schicht | Zeichnet sie die Suchanfrage auf? | Was tun |
| --- | --- | --- |
| Cloud Run | Ja. Seine Request-Logs entstehen automatisch, und `httpRequest.requestUrl` enthält die Suchanfrage. | Schließe sie aus, wie unten. |
| Google-Cloud-Load-Balancer | Nur, wenn du das Logging für den Backend-Dienst einschaltest. Bei neuen ist es standardmäßig aus, und es nutzt dasselbe Feld. | Lass es aus oder schließe sie aus, wie unten. |
| AWS Application Load Balancer | Nur, wenn du Access-Logs einschaltest, die standardmäßig aus sind. Die Anfragezeile enthält die URL so, wie der Client sie gesendet hat. Teile einer Zeile lassen sich nicht weglassen. | Lass sie aus oder gib ihnen eine kurze Lifecycle-Regel und eingeschränkten Zugriff. |
| CloudFront | Nur, wenn du Standard-Logging einschaltest, im Feld `cs-uri-query`. | Lass dieses Feld bei Standard-Logging v2 weg, wenn du die Felder auswählst. |
| ECS | Kein eigenes Request-Log: Seine Logs enthalten, was der Server schreibt. | Nichts. |
| Cloudflare | Nur, wenn du Logpush nutzt, im Feld `ClientRequestURI`. | Wähle stattdessen `ClientRequestPath`. |
| Caddy | Nur mit einer `log`-Direktive, die `compose.caddy.yaml` nicht hat. | Wenn du eine hinzufügst, entferne den Parameter, wie unten. |
| Andere Reverse-Proxys | Meist ja: Ihre Standardformate protokollieren die ganze Anfragezeile. | Protokolliere für diese Route den Pfad ohne die Suchanfrage oder schalte ihr Access-Log aus. |

In Google Cloud hält das beide Logs aus dem Bucket `_Default` des Projekts heraus. Füge denselben Ausschluss zu jeder eigenen Senke hinzu:

```sh
gcloud logging sinks update _Default \
  --add-exclusion='name=my-search-queries,filter=httpRequest.requestUrl:"/api/v1/statuses/search"'
```

In Caddy:

```caddyfile
social.example {
  log {
    format filter {
      request>uri query {
        delete q
      }
    }
  }
  reverse_proxy my:8080
}
```

<!-- source: backups cb3b40906192 -->
## Backups

Der Index enthält den Text von Beiträgen, braucht also dieselbe Sorgfalt wie die Datenbank: wer ihn erreichen kann, wo er liegt und wie lange.

Ein Snapshot der Engine ist der Teil, auf den du achten musst. Ein Beitrag, der aus dem Index gelöscht wurde, bleibt in jedem Snapshot, der vorher entstanden ist, so wie in Datenbank-Backups; gib Snapshots also dieselbe Aufbewahrungsdauer wie deinen [Datenbank-Backups](backup.md).

Zur Wiederherstellung brauchst du keine Snapshots: `my search rebuild` baut den Index aus der Datenbank neu auf. Führe den Befehl nach dem Zurückspielen der Datenbank aus einem Backup ohne `--keep` aus, damit Beiträge, die neuer als das Backup sind, den Index verlassen.

<!-- source: your-privacy-documents 660de0e2a0c4 -->
## Deine Datenschutzdokumente

Wenn die Engine bei einem Dienst eines anderen Unternehmens läuft, erhält dieses Unternehmen den Text der Beiträge, die `search_visibilities` abdeckt, einschließlich der `local`-Beiträge, wenn du sie einschließt. Nenne es als Empfänger in der [Datenschutzerklärung](privacy-policy-template.md) und im [Verzeichnis von Verarbeitungstätigkeiten](processing-record-template.md). Eine Engine, die du selbst betreibst, ist kein neuer Empfänger.

Sage außerdem, was du zu [Logs, die die Suchanfrage enthalten](#the-query-appears-in-urls), entschieden hast, falls du sie aufbewahrst.

<!-- source: closing-the-service 343ec69df255 -->
## Den Dienst schließen

Der Server löscht beim Schließen keine Daten selbst; verwirf den Index und die Snapshots der Engine also zusammen mit der Datenbank und den Medien ([Den Dienst schließen](service-closure.md#7-discarding-the-db-blobs-and-backups)).
