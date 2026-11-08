<!-- source: maintenance 46ba28c229dd -->
# Wartung

Wie du den Server für Arbeiten an der Datenbank, am KV oder an der Queue herunterfährst und was du anzeigst, solange er nicht läuft.

Es gibt zwei Werkzeuge:

- **Der Wartungsmodus** (`maintenance = true`, `MY_MAINTENANCE=true` oder `my serve -maintenance`). `my serve` startet, ohne einen Adapter zu öffnen,
  und beantwortet jede Anfrage mit einem `503`, einem `Retry-After` und einer Antwort, die jeder Client versteht. Nutze ihn immer, wenn `my serve` selbst laufen kann.
- **Die exportierte Seite** (`my maintenance export`). Statisches HTML aus demselben Template, das ein Load Balancer, CDN oder Reverse-Proxy ausliefert,
  wenn `my serve` gar nicht laufen kann (ein Image wird ersetzt, eine Revision startet nicht). Siehe [Die Wartungsseite exportieren](admin-cli.md#exporting-the-maintenance-page).

<!-- source: what-the-maintenance-mode-answers 05dd30158a8e -->
## Was der Wartungsmodus antwortet

Der Prozess liest nur die Listen-Adresse, `host`, `account_domain`, `web_origin`, die Proxys (`trusted_proxies`, `trusted_proxy_header`),
die [Origin-Prüfung](cdn.md#refuse-requests-that-did-not-come-through-the-cdn) (`origin_secret`, `origin_secret_previous`, `origin_secret_header`), `status_page_url` und die `maintenance_*`-Einstellungen ([Konfigurationsreferenz](../reference/configuration.md)).
Die Einstellungen von Datenbank, Queue, Blob-Speicher, KV, Mail und Stream werden nicht gelesen oder geprüft, sodass sie in der Zwischenzeit falsch oder nicht erreichbar sein dürfen.
Unabhängig von der `role` liefert er nur HTTP: keine Jobs, keine periodischen Jobs, keinen Stream-Broker.
`my config check -maintenance` prüft genau das, was der Wartungsmodus liest.

Ist [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn) gesetzt, bekommt eine Anfrage, die den Header braucht und ihn nicht hat, das `403` dieser Prüfung statt der Tabelle unten.

| Anfrage | Status | Body |
| --- | --- | --- |
| `GET /healthz`, `GET /readyz` | `200` | `{"status":"maintenance"}` |
| `GET /api/v1/instance/health` | `503` | `{"status":"maintenance"}` |
| Der Rest der Client-API (`/api/*`) und die OAuth-Endpunkte, die JSON beantworten | `503` | `application/problem+json` vom Typ `/api/v1/problems/maintenance`, mit `message`, `until` und, wenn gesetzt, `status_page_url` |
| ActivityPub (nach Pfad, `Accept` oder `Content-Type`), WebFinger, host-meta, NodeInfo, die Zahlungs-Webhooks, die Aufrufe von Cloud Tasks und Cloud Scheduler (`/_internal/*`) | `503` | `{"error":"maintenance"}` |
| Alles andere (Seiten, der Web-Client, seine Assets, Formular-Posts) | `503` | Die Wartungsseite oder das Problem, wenn `Accept` nach JSON fragt |

Jede Antwort hat `Retry-After` und `Cache-Control: no-store`. `Retry-After` zählt die Sekunden bis `maintenance_until`, solange es in der Zukunft liegt
(höchstens 86.400), und ist sonst `maintenance_retry_after` (eine Go-Dauer, Standard `10m`).

Die Seite und das Problem zeigen:

| Einstellung | Angezeigt als |
| --- | --- |
| `maintenance_server_name` | Der Name des Servers. Der Hostname, wenn nicht gesetzt (der eigene Name des Servers steht in der Datenbank) |
| `maintenance_message` | Ein Hinweis als reiner Text, höchstens 2.000 Zeichen, in jedem Locale gleich |
| `maintenance_until` | Wann die Arbeiten voraussichtlich enden (RFC 3339). Die Seite zeigt es in UTC; der Web-Client in der Zeitzone des Nutzers |
| `status_page_url` | Ein Link zur Statusseite ([Überwachung](monitoring.md#a-status-page)) |

Die `maintenance_*`-Einstellungen dürfen im normalen Betrieb gesetzt bleiben: Sie werden nur verwendet, solange `maintenance` wahr ist.
Die Texte der Seite folgen `Accept-Language` (sonst Englisch).

Der Web-Client zeigt einen Wartungshinweis im Vollbild, wenn die API mit dem Wartungsproblem antwortet,
hält den Nutzer angemeldet und versucht es nach `Retry-After` erneut.

<!-- source: how-long a7b6a44f0269 -->
## Wie lange

Halte eine Wartung **auf wenige Stunden** begrenzt. Andere Server wiederholen ihre Zustellungen bei `503`, aber keiner von ihnen liest `Retry-After`,
und wenn ihre Wiederholungen aufgebraucht sind, gehen die in der Zwischenzeit gesendeten Aktivitäten für diesen Server verloren.
Das kürzeste Wiederholungsfenster unter den großen Implementierungen beträgt etwa einen Tag (Akkoma etwa 1 Tag, Misskey etwa 1,5 Tage, Mastodon etwa 2–3 Tage, Pleroma etwa 7 Tage,
nach ihren Wiederholungsplänen).
Suchmaschinen behandeln ein kurzes `503` als vorübergehenden Ausfall, aber Google rät davon ab, für `robots.txt` länger als ein oder zwei Tage `503` auszuliefern.
Ein Stillstand von mehr als ein oder zwei Tagen ist keine Wartung: Schließe stattdessen den Dienst ([Den Dienst schließen](service-closure.md)).

<!-- source: steps 1427d2c6d499 -->
## Schritte

1. **Kündige sie an.** Veröffentliche das Zeitfenster rechtzeitig vorher auf der Statusseite (`status_page_url`) und als Ankündigung (Administration → Ankündigungen).
   Setze `maintenance_message` und `maintenance_until` in der Konfiguration des Deployments und aktualisiere die exportierten Seiten
   (`my maintenance export`), wenn das Frontend sie ausliefert.
2. **Halte die Push-Queue an** (Cloud Tasks) — siehe [unten](#push-queues). Sonst ist jede Zustellung während des Fensters
   ein `503`, das einen der Versuche der Aufgabe verbraucht.
3. **Schalte in den Wartungsmodus**: Führe das Deployment mit `MY_MAINTENANCE=true` erneut aus (je Ziel unten). Jede Replika startet im Wartungsmodus neu;
   der Load Balancer behält sie, weil `/readyz` mit `200` antwortet.
4. **Erledige die Arbeit.**
5. **Schalte zurück**: Führe das Deployment ohne `MY_MAINTENANCE` (oder mit `false`) erneut aus, prüfe, dass `/readyz` und `/api/v1/instance/health` mit `{"status":"ok"}` antworten,
   setze dann die Push-Queue fort und schließe die Ankündigung.

![Was der Server vor, während und nach dem Wartungsmodus je Art der Anfrage antwortet: wie gewohnt davor und danach, und 503 mit Retry-After währenddessen, außer bei /healthz und /readyz, die 200 bleiben. Die Liste unten sagt dasselbe.](images/maintenance.svg)

Von links nach rechts:

- **Davor**, während du die Wartung ankündigst, wird jede Anfrage wie gewohnt beantwortet: `/healthz`, `/readyz` und
  `/api/v1/instance/health` antworten mit `200` und `{"status":"ok"}`.
- **Im Wartungsmodus** (`MY_MAINTENANCE=true`):
  - `/healthz` und `/readyz` antworten mit `200` und `{"status":"maintenance"}`.
  - `/api/v1/instance/health` antwortet mit `503` und `{"status":"maintenance"}`.
  - Der Rest der Client-API und die OAuth-Endpunkte, die JSON beantworten, bekommen `503` mit dem Wartungsproblem.
  - ActivityPub, WebFinger und NodeInfo antworten mit `503` und `{"error":"maintenance"}`.
  - Seiten und der Web-Client bekommen `503` mit der Wartungsseite.
  - Jede Antwort hat `Retry-After` und `Cache-Control: no-store`.
- **Danach**, wenn du das Deployment ohne `MY_MAINTENANCE` erneut ausgeführt hast, wird jede Anfrage wieder wie gewohnt beantwortet.

<!-- source: push-queues 0d0d1fc56a29 -->
### Push-Queues

- **Cloud Tasks**: Pausiere die Queue, die weiterhin neue Aufgaben annimmt, sie aber nicht mehr zustellt, und setze sie nach der Arbeit fort.
  Wie viele Versuche eine Aufgabe hat, bestimmt das `retryConfig.maxAttempts` der Queue.

  ```sh
  gcloud tasks queues pause my-jobs --location=asia-northeast1
  gcloud tasks queues resume my-jobs --location=asia-northeast1
  ```

  Pausiere auch den Cloud-Scheduler-Job von `/_internal/tick` (`gcloud scheduler jobs pause` / `resume`) oder lass seine Aufrufe in der Zwischenzeit mit `503` fehlschlagen.
  Die Jobs selbst liegen in der Datenbank: Eine Aufgabe, die ihre Versuche aufgebraucht hat, verliert nur ihr Signal, und der erste Tick nach der Arbeit signalisiert
  jeden Job, der fällig ist, erneut.

<!-- source: per-target 4e060394be4b -->
## Je Ziel

Die Fehlerseiten der Frontends unten beantworten die Fehler des Servers mit der exportierten Seite. Greife nur die Fehler eines Servers ab, der nicht läuft
(`502` und `504` sowie das eigene `503` des Load Balancers, wo er kein gesundes Backend hat), oder schalte die Regel nur ein, solange `my serve` nicht laufen kann:
Eine Regel auf jedes `503` würde auch die eigenen Antworten des Wartungsmodus ersetzen, etwa das Problem, das die Client-API und der Web-Client lesen.

<!-- source: cloud-run b728619281de -->
### Cloud Run

Deploye eine neue Revision mit der Variable und entferne sie danach:

```sh
gcloud run services update my --region=asia-northeast1 \
  --update-env-vars=MY_MAINTENANCE=true,MY_MAINTENANCE_UNTIL=2026-10-24T03:00:00Z
gcloud run services update my --region=asia-northeast1 --remove-env-vars=MY_MAINTENANCE,MY_MAINTENANCE_UNTIL
```

Wenn Cloud Run direkt ohne Load Balancer erreichbar ist, steht nichts davor, daher ist der Wartungsmodus das einzige Werkzeug.

**Wenn `my serve` nicht laufen kann**, hinter einem globalen externen Application Load Balancer: Lade das exportierte `index.html` in einen Cloud-Storage-Bucket hoch,
der als Backend-Bucket hinzugefügt ist, und füge eine Richtlinie für benutzerdefinierte Fehlerantworten hinzu, die das `502` und `504` des Backends damit beantwortet, mit `overrideResponseCode: 503`.
Nur der globale externe Application Load Balancer hat benutzerdefinierte Fehlerantworten (nicht die regionalen oder klassischen), und die Richtlinie gilt nur
für Antworten des Backends, nicht für solche, die Google Front End selbst erzeugt. Dieser Weg kann kein `Retry-After` senden: Die benutzerdefinierten Header der Fehlerquelle werden nicht hinzugefügt.

```yaml
# Part of the URL map (gcloud compute url-maps import).
defaultCustomErrorResponsePolicy:
  errorService: https://www.googleapis.com/compute/v1/projects/<project>/global/backendBuckets/my-maintenance
  errorResponseRules:
    - matchResponseCodes: ["502", "504"]
      path: /index.html
      overrideResponseCode: 503
```

<!-- source: aws-ecs 8c6aa496b517 -->
### AWS ECS

Registriere eine Revision der Task-Definition des Dienstes mit `MY_MAINTENANCE=true` in der Umgebung des Containers, aktualisiere den Dienst darauf
und tue danach dasselbe ohne die Variable. Mit den OpenTofu-Modulen schaltet `deploy/opentofu/upgrade.sh --platform aws-ecs --maintenance`
den Dienst vor einem Upgrade in den Wartungsmodus und mit der neuen Revision wieder zurück.

**Wenn `my serve` nicht laufen kann**, hinter CloudFront: Lade das exportierte `index.html` in einen S3-Bucket hoch, der eine zweite Origin ist
(die Fehlerseite kann nicht von der ausfallenden Origin kommen), und füge benutzerdefinierte Fehlerantworten für `502` und `504` hinzu (und `503` nur, solange `my serve` nicht laufen kann)
mit `ResponsePagePath: /maintenance/index.html` und `ResponseCode: 503`.
CloudFront kann `Retry-After` über eine Response Headers Policy mit einem benutzerdefinierten Header hinzufügen, aber die Richtlinie gilt für jede Antwort des Cache-Behaviors,
also hänge sie nur für die Wartung an und löse sie danach wieder.

Die feste Antwort eines Application Load Balancers kann die exportierte Seite nicht tragen: Ihr Body ist höchstens 1.024 Zeichen lang, und sie setzt keine Header wie `Retry-After`.
Nutze sie nur für einen kurzen Text und den Status:

```json
{
  "Type": "fixed-response",
  "FixedResponseConfig": {
    "StatusCode": "503",
    "ContentType": "text/plain",
    "MessageBody": "This server is under maintenance. Please try again later. / メンテナンス中です。しばらくしてからお試しください。"
  }
}
```

Ohne `Retry-After` wiederholen andere Server nach ihrem eigenen Zeitplan, den das Fenster oben bereits berücksichtigt.

<!-- source: docker-compose d564c0b39ae8 -->
### Docker Compose

`deploy/docker/compose.yaml` reicht die Einstellungen aus `.env` an den Server weiter und übernimmt `MY_MAINTENANCE`, `MY_MAINTENANCE_MESSAGE` und `MY_MAINTENANCE_UNTIL` auch aus der Shell.
Setze sie und erzeuge den Dienst `my` neu, entferne dann nach der Arbeit `MY_MAINTENANCE` und erzeuge ihn erneut.
`--no-deps` verhindert, dass die einmaligen Datenbankdienste (`upgrade-check`, `schema`, `upgrade`) laufen; ein Upgrade führt sie aus ([Upgrade](database.md#upgrading)):

```sh
MY_MAINTENANCE=true MY_MAINTENANCE_UNTIL=2026-10-24T03:00:00Z docker compose up -d --no-deps --force-recreate my
docker compose up -d --no-deps --force-recreate my
```

**Wenn `my serve` nicht laufen kann**, hinter nginx: Liefere die exportierten Seiten aus einem Verzeichnis aus, wähle eine nach `Accept-Language`
und beantworte das eigene `502` und `504` von nginx (der Server ist ausgefallen oder antwortet nicht) damit als `503`.
Die eigenen `503` des Wartungsmodus laufen unverändert durch, da `proxy_intercept_errors` aus bleibt.

```nginx
map $http_accept_language $maintenance_page {
    default  en.html;
    ~*^ja    ja.html;
}

server {
    # ...
    location / {
        proxy_pass http://my:8080;
        error_page 502 504 =503 /maintenance/$maintenance_page;
    }
    location /maintenance/ {
        internal;
        alias /srv/maintenance/;   # the -out directory of my maintenance export
        add_header Retry-After 600 always;
        add_header Cache-Control no-store always;
    }
}
```

<!-- source: microk8s-kubernetes a831623b2454 -->
### microk8s (Kubernetes)

Setze die Variable am Deployment, was die Pods neu ausrollt, und entferne sie danach:

```sh
microk8s kubectl set env deployment/my MY_MAINTENANCE=true MY_MAINTENANCE_UNTIL=2026-10-24T03:00:00Z
microk8s kubectl rollout status deployment/my
microk8s kubectl set env deployment/my MY_MAINTENANCE- MY_MAINTENANCE_UNTIL-
```

Die Readiness-Probe auf `/readyz` besteht im Wartungsmodus weiter, sodass der Rollout abgeschlossen wird und der Service weiter auf die Pods routet.

**Wenn `my serve` nicht laufen kann**, hinter dem NGINX Ingress Controller (das microk8s-Add-on `ingress`): Liefere die exportierten Seiten aus einem kleinen
`nginx`-Deployment aus (zum Beispiel aus einer ConfigMap mit `index.html`, `en.html` und `ja.html`) und mache es zum benutzerdefinierten Default-Backend,
wobei der Controller das `502` und `504` des Servers daraus beantwortet (füge `503` nur hinzu, solange `my serve` nicht laufen kann):

```yaml
metadata:
  annotations:
    nginx.ingress.kubernetes.io/custom-http-errors: "502,504"
    nginx.ingress.kubernetes.io/default-backend: my-maintenance   # a Service in the same namespace
```

<!-- source: monitoring-during-the-maintenance 91e64028447e -->
## Überwachung während der Wartung

- `/healthz` und `/readyz` bleiben `200` mit `{"status":"maintenance"}`: Load Balancer und Kubernetes behalten die Server.
- `/api/v1/instance/health` antwortet mit `503` und `{"status":"maintenance"}` ([Überwachung](monitoring.md)).
  Uptime-Monitore sehen den Server als ausgefallen, außer sie prüfen den Body oder haben ein Wartungsfenster; pausiere sie oder lege das Fenster fest.
