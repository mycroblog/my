<!-- source: put-a-cdn-in-front-of-media-delivery 20ee3d2cefe9 -->
# Ein CDN vor die Medienauslieferung setzen

Ein CDN vor dem Server (Cloudflare, Google Cloud CDN, CloudFront und so weiter) kann Medien ausliefern (`/media/` und `/media/proxy/`).
Anfragen, die aus dem CDN-Cache beantwortet werden, brauchen weder CPU noch Transfervolumen des Servers.
Diese Anleitung behandelt den Aufbau, in dem der Server der Origin des CDN ist (Origin Pull). Sie gilt für jeden Blob-Speicher: Mit `blob = s3` oder `blob = gcs` liest der Server
die Dateien aus dem privaten Bucket und liefert sie unter `/media/` aus wie mit `blob = fs`, der Bucket ist also nie die Origin des CDN.
Wenn der ganze Server hinter dem CDN liegt, kann er außerdem [Anfragen ablehnen, die nicht über das CDN kamen](#refuse-requests-that-did-not-come-through-the-cdn).

<!-- source: configure-it e915ce01da94 -->
## Einrichten

Setze `media_origin` (`MY_MEDIA_ORIGIN`) auf den Hostnamen des CDN (zum Beispiel `media.example.org`).

```sh
MY_MEDIA_ORIGIN=https://media.example.org
```

- Der Wert ist nur eine Origin (`scheme://host[:port]`), ohne Pfad, Query oder abschließenden Schrägstrich. `https` ist Pflicht, und `http` ist nur für Loopback-Hosts wie `localhost` erlaubt. Ein ungültiger Wert lässt den Start fehlschlagen.
- Ist er nicht gesetzt, werden Medien von der eigenen Origin des Servers ausgeliefert (`https://{host}`).
- Diese URLs verwenden diese Origin: `{media_origin}/media/...` für lokale Medien und `{media_origin}/media/proxy/...` für den Proxy entfernter Medien.
  Erfasst ist jede URL, die die Client-API, ActivityPub (`attachment`, `icon`, `image` und das `icon` eigener Emojis) und Link-Vorschauen (OGP) ausgeben.
- Die Content-Security-Policy des Webclients fügt diese Origin zu `img-src` und `media-src` hinzu.
- Föderierte URIs, etwa die `id` eines ActivityPub-Objekts, bleiben unter `https://{host}/`, was auch immer `media_origin` ist. Es ändert sich nur der Ort, an dem Dateien ausgeliefert werden, nicht die Bezeichner.
- URLs werden bei jedem Lesen neu gebildet und nicht in der Datenbank gespeichert, eine Änderung von `media_origin` ändert also auch die URLs vorhandener Medien.
  Alte URLs, die entfernte Server gespeichert haben, funktionieren weiter, solange `/media/` auf der alten Origin weiter ausliefert.

<!-- source: point-the-cdn-at-the-server ffb279f38f08 -->
## Das CDN auf den Server richten

- Mache den öffentlichen Hostnamen des Servers (`https://{host}`) zur Origin (zum Backend). Das CDN muss vom Server nur Pfade unter `/media/` abrufen.
- Der Server beschränkt `/media/` und `/media/proxy/` nicht nach Hostnamen. Er liefert aus, welchen `Host` das CDN auch sendet.
- `/media/` braucht keine Authentifizierung, und Authorized Fetch gilt dafür nicht. Der Server beantwortet unsignierte Anfragen des CDN, wie sie sind.
- Bei entfernten Medien (`/media/proxy/{id}/{variant}?sig=...`) ist `sig` Teil der URL. **Nimm den Query-String in den Cache-Schlüssel auf.** Sonst teilen sich URLs mit verschiedenen Signaturen einen Eintrag.
- Lass das `Cache-Control` der Origin bestimmen, wie lange gecacht wird (überschreibe es nicht im CDN).

| Pfad | `Cache-Control` |
| --- | --- |
| `/media/{...}` (lokale Medien) | `public, max-age=31536000, immutable` (ein Jahr) |
| `/media/proxy/{...}` (entfernte Medien) | `public, max-age=` die Sekundenzahl der Servereinstellung `remote_media_cache_ttl_seconds` |

Beide werden mit `Cross-Origin-Resource-Policy: cross-origin` zurückgegeben, damit Seiten anderer Origins sie lesen können.

Auf Cloud Run richtet `ingress-cloudflare-media` das mit OpenTofu ein: siehe [Auf Cloud Run bereitstellen](deploy-cloud-run.md#cloudflare).

<!-- source: who-can-open-media-urls 27306b0bc9be -->
## Wer Medien-URLs öffnen kann

Medien-URLs sind nicht zugriffsgeschützt. Jede Person, die die URL einer Datei hat, kann sie ohne Anmeldung öffnen, unabhängig von der Sichtbarkeit des Beitrags, an den sie angehängt ist.
Das gilt mit und ohne CDN und schließt die Anhänge von Beiträgen nur für Follower und von Direktnachrichten sowie die Proxy-URLs entfernter Medien ein.

- Die URLs sind schwer zu erraten: Die URL einer lokalen Datei enthält Bezeichner mit einem zufälligen Anteil, und eine Proxy-URL trägt eine Signatur. Eine bekannte URL verrät keine anderen.
- Wer einen Beitrag gesehen hat, kann seine Medien-URLs weitergeben, und die URLs funktionieren für andere weiter.
- Antworten sind als `public` markiert, sodass das CDN und jeder gemeinsame Cache zwischen Server und Leser eine Kopie behalten dürfen (wie lange, steht in der Tabelle oben).
  Genau das lässt ein CDN Medien ausliefern, ohne den Server zu fragen.

Sag deinen Nutzern, dass ein nicht öffentlicher Beitrag zwar einschränkt, wer den Beitrag sieht, aber niemanden daran hindert, die Datei zu öffnen, wenn er den Link einer angehängten Datei hat.

<!-- source: deletion-and-the-cache baac405df923 -->
## Löschen und der Cache

Nachdem du Medien gelöscht hast, behält der Cache des CDN sie für `max-age`. Eine gelöschte Datei kann bei lokalen Medien bis zu einem Jahr sichtbar bleiben, beim Proxy bis zu `remote_media_cache_ttl_seconds`.
Der Server leert den Cache des CDN nicht. Wenn etwas direkt nach einem Löschen oder einer Moderationsaktion verschwinden muss, leere ihn von Hand in der Konsole oder API des CDN.

<!-- source: service-worker f6efdae181a5 -->
## Service Worker

Der Service Worker des Webclients cacht keine Medien einer anderen Origin und überlässt sie der Netzwerkverarbeitung des Browsers, sodass nur der Cache des CDN gilt.

<!-- source: refuse-requests-that-did-not-come-through-the-cdn 5df4225c62b3 -->
## Anfragen ablehnen, die nicht über das CDN kamen

Ein CDN schützt den Server nur vor den Anfragen, die durch es hindurchgehen. Ist die Origin zusätzlich unter ihrer eigenen Adresse erreichbar, wie `*.run.app` auf Cloud Run mit Verkehr aus dem Internet, kann jeder sie direkt aufrufen, an WAF und Cache vorbei.
Stehen die Adressen des CDN in `trusted_proxies`, verbindet sich auch ein Aufrufer auf der Serverless-Plattform des CDN selbst (etwa ein eigener Cloudflare Worker) von einer vertrauenswürdigen Adresse und kann die Client-IP wählen, die die Rate Limits sehen.

Die drei Einstellungen unten schließen das. Das CDN fügt jeder Anfrage, die es weiterleitet, einen geheimen Header hinzu, und der Server antwortet auf jede Anfrage ohne ihn mit `403`, bevor er irgendetwas anderes mit ihr tut.

![Die Wege von Anfragen bei aktivierter Origin-Prüfung: Browser, Apps und andere Server gehen durch das CDN, das den geheimen Header hinzufügt; die Plattform und alle anderen erreichen den Server direkt, ohne ihn; die Origin-Prüfung des Servers lässt den Header und die ausgenommenen Anfragen durch und lehnt den Rest mit 403 ab. Die Liste unten beschreibt jeden Weg.](images/cdn-request-flow.svg)

- Browser und Apps (Seiten, die Client-API, OAuth und Medien-URLs) und andere Server (ActivityPub-Zustellungen und -Abrufe)
  erreichen das CDN unter `host` und unter dem Host von `media_origin`. Das CDN beantwortet gecachte `/media/`-Dateien selbst und setzt
  `origin_secret_header` bei jeder Anfrage, die es an den Server weiterleitet, auf `origin_secret`.
- Die Plattform (Health-Probes, Uptime-Monitore, Cloud Tasks und Cloud Scheduler) und alle anderen, etwa ein Skript oder ein
  eigener Webhook, erreichen die eigene Adresse des Servers direkt, ohne den Header.
- Die Origin-Prüfung läuft im Server zuerst. Sie lässt eine Anfrage durch, die den Header einmal mit `origin_secret` oder
  `origin_secret_previous` trägt, und ohne den Header nur `GET` und `HEAD` der Health-Checks und `POST` auf `/_internal/`.
  Sie entfernt den Header aus jeder Anfrage und lehnt den Rest mit `403` ab.
- Eine Anfrage, die durchkommt, erreicht den Webclient, die Client-API, OAuth, ActivityPub, die Health-Checks oder `/_internal/`
  oder `/media/` und `/media/proxy/`, die den Medienspeicher lesen (`blob = fs`, `s3` oder `gcs`) oder die Datei eines
  anderen Servers weiterreichen. Eine abgelehnte Anfrage erhält `403` mit `Cache-Control: no-store` und erhöht `my.http.origin_refused`.

Nutze es, wenn der Server eine öffentliche Adresse hat, die die Plattform nicht für alles außer dem CDN schließen kann. Du brauchst es nicht, wenn eine Firewall oder die Plattform bereits einschränkt, wer sich verbinden kann, zum Beispiel bei einem Server, der nur in einem privaten Netzwerk hinter deinem Load Balancer lauscht.
Auch nicht, wenn dieser Load Balancer den Header des CDN selbst prüft und nur die Adressen des CDN zulässt, wie es ein AWS Application Load Balancer hinter CloudFront mit einer Listener-Regel und der verwalteten Präfixliste `com.amazonaws.global.cloudfront.origin-facing` kann. Trage dann sowohl die Adressen des Load Balancers als auch die des CDN in `trusted_proxies` ein, da jeder an `X-Forwarded-For` anhängt.

Es hilft nicht, wenn eine eigene Funktion der Plattform den Verkehr weiterleitet und keinen Header hinzufügt, den du kontrollierst, etwa ein Cloud-Run-Domain-Mapping. Nur ein CDN oder Load Balancer, der einen Request-Header hinzufügen kann, hilft.

| Einstellung | Was sie enthält |
| --- | --- |
| [`origin_secret`](../reference/configuration.md#origin_secret) (`MY_ORIGIN_SECRET`) | Der Wert, mindestens 32 Bytes sichtbares ASCII. Ein Geheimnis, gelesen aus der Umgebung oder aus `MY_ORIGIN_SECRET_FILE` |
| [`origin_secret_header`](../reference/configuration.md#origin_secret_header) (`MY_ORIGIN_SECRET_HEADER`) | Der Name des Headers, dein eigener. Pflicht zusammen mit `origin_secret`. Namen, die ein CDN nicht setzen darf oder die der Server oder ein Proxy liest, etwa solche mit dem Anfang `cf-` oder `x-cf-`, sind ungültig |
| [`origin_secret_previous`](../reference/configuration.md#origin_secret_previous) (`MY_ORIGIN_SECRET_PREVIOUS`) | Ein zweiter akzeptierter Wert, nur [während einer Rotation](#rotate-the-value) |

Ist keine davon gesetzt, wird nichts geprüft, und der Server verhält sich, als gäbe es diesen Abschnitt nicht.

<!-- source: what-passes-without-the-header 38e1ceda6e34 -->
### Was ohne den Header durchkommt

| Anfrage | Warum |
| --- | --- |
| `GET` oder `HEAD` auf `/healthz`, `/readyz`, `/api/v1/instance/health` | Die Probes der Plattform und Uptime-Monitore rufen sie direkt auf |
| `POST` auf Pfade unter `/_internal/` | Cloud Tasks und Cloud Scheduler rufen sie unter der eigenen URL des Servers auf. Sie tragen ein eigenes signiertes Token |

Alles andere braucht den Header, auch im Wartungsmodus: den Webclient und seine Assets, `/media/`, die Client-API, OAuth und ActivityPub, einschließlich der Zustellungen anderer Server.
Alles, was den Server direkt unter seiner eigenen Adresse statt über das CDN aufruft, wird also abgelehnt: ein eigener Webhook, ein Voice-Medienserver, ein Skript. Richte es auf den Hostnamen des CDN.

Der Metrik-Port (`metrics_listen`) ist ein separater Listener, der nicht öffentlich ist und nicht geprüft wird.

Eine Anfrage, die das Geheimnis nicht trägt, hat bei der übrigen Verarbeitung nichts mitzureden, selbst wenn sie durchkommt: Ihre Client-IP ist die Gegenstelle der Verbindung, was auch immer `X-Forwarded-For` sagt, und ihr `traceparent` und `X-Request-Id` werden ignoriert.

<!-- source: set-it-up ee0a17ad2d33 -->
### Einrichten

Jeder Hostname, den das CDN an den Server weiterleitet, braucht den Header: `host`, der Host von `media_origin` und eine separate `account_domain`, wenn das CDN sie ausliefert. Ein Medien-Host hinter demselben CDN setzt ihn ebenfalls, da `/media/` wie alles andere abgelehnt wird.

1. Erzeuge den Wert und wähle einen Namen, den sonst nichts verwendet:

   ```sh
   openssl rand -hex 32
   ```

2. **Setze den Header zuerst im CDN.** Der Server ignoriert einen Header, den er nicht kennt, dieser Schritt ändert also noch nichts.
   - **Cloudflare**: eine Transform Rule für Request-Header (Rules, dann Transform Rules, dann Modify Request Header), in jedem Tarif. Erfasse alle eingehenden Anfragen der Zone und wähle **Set static** mit deinem Namen und Wert. Sie ersetzt einen Wert, den ein Client gesendet hat. Eine Regel nur für einige Hostnamen sperrt die übrigen aus.
   - **Amazon CloudFront**: ein benutzerdefinierter Origin-Header an der Origin jeder Distribution. Auch er ersetzt den Wert eines Betrachters.
   - **Google Cloud Load Balancer**: ein benutzerdefinierter Request-Header am Backend-Service.
   - Jedes andere CDN oder jeder andere Proxy: Der Header muss bei jeder Anfrage an den Server gesetzt, nicht angehängt werden.
3. Setze `MY_ORIGIN_SECRET` und `MY_ORIGIN_SECRET_HEADER` und stelle bereit. `my config check` prüft sie und wiederholt, was geprüft wird, kann aber das CDN nicht sehen.
4. Prüfe von außen mit der Adresse der Origin selbst:

   ```sh
   curl -s -o /dev/null -w '%{http_code}\n' https://<host>/                 # 200: through the CDN
   curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/       # 403: directly
   curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/healthz # 200: a health check
   ```

Trage außerdem die Adressen des CDN in [`trusted_proxies`](../reference/configuration.md#trusted_proxies) ein, sonst sieht der Server einen einzigen Client, und alle Anfragen teilen sich die Rate Limits des CDN; `my config check` warnt davor.

Eine abgelehnte Anfrage erhält dasselbe `403`-Problemdokument wie jede andere verbotene Anfrage, mit `Cache-Control: no-store`, wie auch immer der Header falsch war. Nichts in der Antwort nennt den Header.

<!-- source: when-everyone-is-locked-out 47c8f706e5e7 -->
### Wenn alle ausgesperrt sind

Eine falsche Einrichtung lehnt jede Anfrage ab, während die Health-Checks `200` bleiben; die Plattform sieht also einen gesunden Server, und ein Uptime-Monitor auf `/healthz` merkt nichts. Die Anzeichen sind:

- `403` für jede Seite über das CDN.
- Die Metrik `my.http.origin_refused` steigt, während die Anfragen von `http.server.request.duration` ausbleiben.
- Zeilen `http request` im Log mit dem Status `403` und ohne `http.route`. Das Log und die Traces enthalten nie den Header, seinen Namen oder seinen Wert. Eine abgelehnte Anfrage erzeugt keinen Trace-Span.

Stelle zur Wiederherstellung ohne die Prüfung bereit: Entferne `MY_ORIGIN_SECRET` und `MY_ORIGIN_SECRET_HEADER`. Andere Server wiederholen ihre Zustellungen, aber Personen konnten den Server in der Zwischenzeit nicht erreichen.
Vergleiche dann Name und Wert im CDN mit den Einstellungen, prüfe, ob die Regel jeden Hostnamen abdeckt, und setze sie erneut.

<!-- source: rotate-the-value b88f695fde31 -->
### Den Wert rotieren

CDN und Server können sich nicht im selben Moment ändern, und jedes Replikat des Servers startet in seinem eigenen Tempo neu. Mit nur einem akzeptierten Wert würde eine Änderung den Verkehr ablehnen, solange sie sich unterscheiden. Der Server akzeptiert deshalb während einer Rotation zwei Werte:

1. Setze `MY_ORIGIN_SECRET` auf den neuen Wert und `MY_ORIGIN_SECRET_PREVIOUS` auf den alten und stelle bereit. Beide Werte kommen durch.
2. Ändere den Wert im CDN und warte, bis die Änderung alle seine Standorte erreicht hat.
3. Entferne `MY_ORIGIN_SECRET_PREVIOUS` und stelle bereit. `my config check` warnt, solange er gesetzt ist.

Der Name des Headers lässt sich so nicht rotieren. Um ihn zu ändern, wiederhole [die Einrichtung](#set-it-up) bei kurz abgeschalteter Prüfung des Servers, oder setze den neuen Namen im CDN neben dem alten, bevor du ihn bereitstellst.
