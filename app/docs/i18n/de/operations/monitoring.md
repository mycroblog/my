<!-- source: monitoring-the-server-from-outside 6cd7b2b809fa -->
# Den Server von außen überwachen

Wie du einen Uptime-Monitor auf den Server richtest, wie du verhinderst, dass die Überwachung die laufenden Kosten erhöht,
und wie du den Wartungsmodus ([Wartung](maintenance.md)) von einem Ausfall unterscheidest. Die Probe-Pfade stehen im
[Abschnitt zu Health-Checks im Betriebsleitfaden](running.md#health-checks).

Fakten zu Drittanbietern unten tragen das Datum, an dem sie bestätigt wurden, und ihre Quelle. Dienste ändern ihre Tarife und Oberflächen; prüfe die verlinkte Seite, bevor du dich auf ein Limit verlässt.

<!-- source: which-url-to-monitor 50bdaa5c9f31 -->
## Welche URL überwacht werden soll

| Pfad | Wofür | Greift auf die DB zu | Im Wartungsmodus |
| --- | --- | --- | --- |
| `GET /api/v1/instance/health` | Uptime-Monitore, Statusseiten, der Web-Client | Höchstens einmal pro 60 s pro Prozess | `503` mit `Retry-After` und `{"status":"maintenance"}` |
| `GET /healthz` | Der Liveness-Probe der Plattform | Nie | `200` `{"status":"maintenance"}` |
| `GET /readyz` | Der Readiness-Probe des Load Balancers oder von Kubernetes | Bei jeder Anfrage (gleichzeitige Anfragen teilen sich eine Prüfung) | `200` `{"status":"maintenance"}` |

Richte den externen Monitor auf **`/api/v1/instance/health`**, nicht auf `/readyz`: `/readyz` fragt bei jeder Anfrage die Datenbank ab,
sodass ein Monitor mit kurzem Intervall eine Scale-to-zero-Datenbank wach hält. Die Probes sind für deine Plattform.
Die Module [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) und [`monitoring-aws`](../../deploy/opentofu/modules/monitoring-aws/README.md) prüfen `/readyz` vom plattformeigenen Monitor aus in einem langen Intervall, und ihre READMEs sagen, was das kostet.

Antworten von `/api/v1/instance/health` (ohne Authentifizierung, `Cache-Control: no-store`):

| Zustand | HTTP | Body |
| --- | --- | --- |
| Gesund | `200` | `{"status":"ok","checked_at":"<RFC 3339>"}` |
| Nicht verfügbar (die Datenbank ist nicht erreichbar oder das Schema ist nicht angewendet) | `503` | `{"status":"unavailable","checked_at":"<RFC 3339>"}` |
| Wartungsmodus | `503` mit `Retry-After` | `{"status":"maintenance"}` |

`/healthz` und `/readyz` antworten bei Gesundheit mit `200` und `{"status":"ok"}`, und `/readyz` antwortet mit `503` und einem Problem-Dokument, wenn die Datenbank nicht bereit ist.
Der Body enthält nie einen Grund, Hostnamen oder eine Version. Die Ursache steht im Server-Log.

`checked_at` ist der Zeitpunkt der Prüfung, auf der die Antwort beruht. Der Server führt die Datenbankprüfung höchstens einmal pro Minute und Prozess aus und verwendet das Ergebnis wieder,
sodass ein Monitor, der öfter als alle 60 s abfragt, dasselbe `checked_at` sieht und nichts gewinnt. Bei mehreren Prozessen führt jeder seine eigene Prüfung aus.

Ist [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn) gesetzt, antworten diese drei Pfade weiterhin ohne den Header, sie zeigen also nicht, dass das CDN ihn verloren hat. Überwache zusätzlich eine Seite über das CDN, etwa `https://<host>/about`, wenn ein Monitor dein erstes Warnzeichen ist.

<!-- source: keep-the-monitoring-cheap 29d1da3d6dc2 -->
## Die Überwachung günstig halten

- **Scale-to-zero-Datenbanken.** Ist das Intervall des Monitors kürzer als die Zeit, die die Datenbank vor dem Stoppen wartet, stoppt die Datenbank nie.
  Neon setzt eine Compute-Server nach 5 Minuten ohne Aktivität aus; der Free-Tarif kann das nicht abschalten
  (bestätigt 2026-10-25, [Neon: Scale to zero](https://neon.com/docs/introduction/scale-to-zero)).
  Als Faustregel lässt ein Intervall von 5 Minuten oder mehr eine untätige Datenbank zwischen den Prüfungen stoppen, und jede Prüfung weckt sie für die Prüfung plus die Leerlaufzeit.
  Ein föderierter Server empfängt rund um die Uhr Zustellungen von anderen Servern, seine Datenbank ist also ohnehin einen großen Teil des Tages wach;
  die zusätzlichen Kosten beschränken sich auf die ruhigen Stunden.
- **Bevorzuge eine günstigere Aufteilung, wenn die Leerlaufkosten am wichtigsten sind.** Überwache `/healthz` (es greift nie auf die Datenbank zu) in einem kurzen Intervall und das datenbankgestützte
  `/api/v1/instance/health` in einem langen (15 Minuten oder mehr). `/healthz` antwortet auch im Wartungsmodus mit `200`, gleiche also `"status":"ok"` im Body ab.
- **Die Anfragen selbst sind klein.** Eine Prüfung alle 5 Minuten sind etwa 8.640 Anfragen im Monat, jede Minute etwa 43.200. Das passt in das Freikontingent
  von Cloud Run (2 Millionen Anfragen, 180.000 vCPU-Sekunden und 360.000 GiB-Sekunden im Monat; Zeit auf 100 ms aufgerundet; bestätigt 2026-10-23, [Cloud-Run-Preise](https://cloud.google.com/run/pricing)).

<!-- source: telling-the-maintenance-mode-from-an-outage 15833e5da3fc -->
## Den Wartungsmodus von einem Ausfall unterscheiden

Im Wartungsmodus ([Wartungsleitfaden](maintenance.md)) antwortet der Health-Endpunkt mit `503`, was die meisten Monitore wie einen Ausfall behandeln.
Der Statuscode allein kann sie nicht unterscheiden, entscheide also anhand des Bodys:

- Behandle `"status":"ok"` im Body als gesund und alles andere als ausgefallen. Ein Keyword- oder JSON-Wert-Monitor tut das.
- Um Alarme bei geplanter Wartung zu vermeiden, setze das Wartungsfenster des Monitors, falls der Dienst eines hat (siehe die Tabelle unten),
  oder pausiere den Monitor vor dem Start von Hand.
- Der Free-Tarif von UptimeRobot hat kein Wartungsfenster: Pausiere den Monitor oder nimm den Alarm in Kauf.

<!-- source: setting-up-the-monitors e91307d78a94 -->
## Die Monitore einrichten

Alle prüfen dasselbe: `GET https://<your-host>/api/v1/instance/health` ist gesund, wenn die Antwort `200` ist und der Body `"status":"ok"` enthält.

| Dienst | Monitor-Typ | Gesund, wenn | Wartungsfenster | Bestätigt |
| --- | --- | --- | --- | --- |
| UptimeRobot | Keyword | Der Body enthält `"status":"ok"` | Nicht in Free (Solo und höher) | 2026-10-25, [Preise](https://uptimerobot.com/pricing.md) |
| Better Stack | HTTP-Status oder Keyword | `2xx`, oder der Body enthält `"status":"ok"` | Ungeprüft | 2026-10-25 |
| Gatus | Endpoint-Bedingungen | `[STATUS] == 200` und `[BODY].status == ok` | `maintenance` in der Konfiguration | 2026-10-25, [README](https://github.com/TwiN/gatus) |
| Uptime Kuma | HTTP(s) - Keyword oder JSON Query | Der Body enthält `"status":"ok"`, oder `status` ist `ok` | Eingebaut | 2026-10-23, [README](https://github.com/louislam/uptime-kuma) |
| Upptime | `sites` in `.upptimerc.yml` | `200` und der Body enthält `"status":"ok"` | Keines | 2026-10-25, [Konfiguration](https://upptime.js.org/docs/configuration/) |

<!-- source: uptimerobot b2350308dfab -->
### UptimeRobot

Lege einen Monitor vom Typ **Keyword** an: URL `https://<your-host>/api/v1/instance/health`, Keyword `"status":"ok"`, Alarm, wenn das Keyword **nicht** vorhanden ist.
Der Free-Tarif erlaubt 50 Monitore in einem Intervall von 5 Minuten und enthält Keyword-Monitore.
„Custom headers & statuses“ und Wartungsfenster beginnen im Solo-Tarif (kostenpflichtig, Intervall von 60 Sekunden)
(bestätigt 2026-10-25, [Preise](https://uptimerobot.com/pricing.md)). Ein Keyword-Monitor genügt hier, weil die Prüfung den Body betrifft.

<!-- source: better-stack 68d7ade20dca -->
### Better Stack

Zwei Formen funktionieren (bestätigt 2026-10-25, [Uptime monitor](https://betterstack.com/docs/uptime/uptime-monitor/), [Keyword monitor](https://betterstack.com/docs/uptime/keyword-monitor/)):

- Ein **HTTP-Status**-Monitor: Er behandelt eine `2xx`-Antwort als erreichbar, also ist `200` erreichbar, und `503` (nicht verfügbar oder Wartung) öffnet einen Vorfall.
- Ein **Keyword**-Monitor: „Alert us when“ ist „URL doesn't contain keyword“, Keyword `"status":"ok"`. Die Übereinstimmung unterscheidet nicht zwischen Groß- und Kleinschreibung.

Die Prüfhäufigkeit und die Wartungsfenster des Free-Tarifs sind ungeprüft: Die offiziellen Seiten nennen sie für den Free-Tarif nicht. Die Zahlen des kostenlosen Tarifs (10 Monitore und Heartbeats zusammen, eine Statusseite) stammen von der [Preisseite](https://betterstack.com/pricing), bestätigt 2026-10-23.

<!-- source: gatus 2aa06e9f2812 -->
### Gatus

In `config.yaml` (Standardintervall `60s`; bestätigt 2026-10-25, [README](https://github.com/TwiN/gatus)):

```yaml
endpoints:
  - name: my
    url: "https://<your-host>/api/v1/instance/health"
    interval: 5m
    conditions:
      - "[STATUS] == 200"
      - "[BODY].status == ok"
```

Gatus hat einen `maintenance`-Abschnitt auf oberster Ebene (`start` als `hh:mm`, `duration`, `timezone` mit Standard `UTC`, `every` als Liste von Wochentagen, leer bedeutet jeden Tag)
für geplante Arbeiten. Die README sagt nur, dass er dazu dient, Alarme während Wartungsfenstern zu vermeiden; prüfe das Verhalten in deiner Version.

<!-- source: uptime-kuma 7dd44fb38cab -->
### Uptime Kuma

Füge einen Monitor vom Typ **HTTP(s) - Keyword** mit dem Keyword `"status":"ok"` hinzu, oder **HTTP(s) - Json Query** mit der Abfrage `status` und dem erwarteten Wert `ok`.
Die akzeptierten Statuscodes sind standardmäßig `200-299`, sodass das `503` der Wartung und das `503` des Ausfalls beide als ausgefallen gelten. Uptime Kuma hat eine Wartungsfunktion
für Monitor-Fenster ([das Wiki](https://github.com/louislam/uptime-kuma/wiki/Monitor-Types) nennt sie; ihre Details sind ungeprüft).
Die Monitor-Typen wurden 2026-10-23 im Quellcode bestätigt ([monitor.js](https://github.com/louislam/uptime-kuma/blob/0c287ac4b3a6723c56ab7d8d731017e58deff533/server/model/monitor.js#L478-L720),
[EditMonitor.vue](https://github.com/louislam/uptime-kuma/blob/0c287ac4b3a6723c56ab7d8d731017e58deff533/src/pages/EditMonitor.vue#L3349)).

<!-- source: upptime 90c96c946d06 -->
### Upptime

In `.upptimerc.yml` (bestätigt 2026-10-25, [Konfiguration](https://upptime.js.org/docs/configuration/)):

```yaml
sites:
  - name: My server
    url: https://<your-host>/api/v1/instance/health
    __dangerous__body_down_if_text_missing: '"status":"ok"'
```

Standardmäßig gelten `20x` und `30x` als erreichbar; setze `expectedStatusCodes: [200]`, um nur `200` zu akzeptieren. Die Prüfung läuft aus GitHub Actions nach einem Zeitplan, der für alle Sites gilt
(`workflowSchedule.uptime`, standardmäßig alle 5 Minuten, was auch das Kürzeste ist, das GitHub Actions erlaubt). Der Optionsname sagt, dass sie nicht empfohlen ist,
aber sie ist der Weg, den Body zu testen.

<!-- source: a-status-page 64712ed6d8a5 -->
## Eine Statusseite

Setze `status_page_url` (`MY_STATUS_PAGE_URL`, eine `https:`-URL; siehe die [Konfigurationsreferenz](../reference/configuration.md#status_page_url))
auf die öffentliche Seite deines Monitors. Der Server zeigt den Link in `GET /api/v1/instance`, in den Anmelde- und Fehlerseiten, die er rendert, im Web-Client
(auch wenn der Server nicht antwortet) und auf der Wartungsseite, sodass ein Nutzer, der den Server nicht erreicht, herausfinden kann, warum.
Er wird aus der Konfiguration gelesen, nicht aus der Datenbank, funktioniert also, wenn die Datenbank ausgefallen ist. Eine Änderung wirkt nach einem Neustart.

Optionen für die Seite selbst (dieser Server übernimmt den Zustand einer Statusseite nicht, veröffentliche Meldungen also zusätzlich als serverweiten Hinweis):

- Die gehostete Statusseite des gewählten Überwachungsdienstes (der Free-Tarif von UptimeRobot hat eine Statusseite ohne Anpassung; der kostenlose Tarif von Better Stack hat eine),
  die Seiten, die Gatus und Uptime Kuma selbst ausliefern, oder die statische Site von Upptime auf GitHub Pages.
- Atlassian Statuspage, das ein Ort zum Veröffentlichen und kein Monitor ist. Eine Komponente hat einen von fünf Zuständen: Operational, Under maintenance, Degraded performance,
  Partial outage, Major outage (API-Werte `operational`, `under_maintenance`, `degraded_performance`, `partial_outage`, `major_outage`;
  bestätigt 2026-10-23, [What is a component?](https://support.atlassian.com/statuspage/docs/what-is-a-component/), [Statuspage API](https://developer.statuspage.io/)).
  Wie sich der Zustand einer Komponente automatisch aus dem Alarm eines Monitors ändern lässt, ist ungeprüft (es braucht eine dienstspezifische Integration, die auf einem echten Konto eingerichtet wird).
