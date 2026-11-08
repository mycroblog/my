<!-- source: deploying-with-docker-compose 29ec05aaf767 -->
# Bereitstellung mit Docker Compose

So betreibst du `my` mit Docker Compose auf einer Maschine: das kleinste Setup, ein Serverprozess auf SQLite mit den
Mediendateien in einem benannten Volume. PostgreSQL, Objektspeicher und TLS kommen später als weitere Dateien obendrauf
([Docker-Compose-Dateien](running.md#docker-compose-files)).

Du brauchst Docker und Docker Compose v2.

<!-- source: start-the-server ba6cb34daaa0 -->
## Den Server starten

Die Compose-Dateien liegen im Quellcode unter [`deploy/docker`](../../deploy/docker/compose.yaml). Nimm sie aus dem Tag
des Releases, das du betreibst, damit sie zu seinem Image passen:

```sh
git clone --depth 1 --branch v0.1.0 https://github.com/mycroblog/my.git
cd my/deploy/docker
cp example.env .env
# Set MY_VERSION in .env to the same version, such as 0.1.0
mkdir -m 700 secrets
(umask 022; openssl rand -base64 32 > secrets/kek)   # the key encryption key: back it up apart from the database
docker compose up -d
```

- Compose lädt das Image `ghcr.io/mycroblog/my` in der Version `MY_VERSION`. Es gibt keinen Tag `latest`, die Version
  muss also genannt werden: Ein Release kann Schritte erfordern, die ein stilles Update überspringen würde
  ([die Tags des Images](running.md#get-the-image)).
  Um das Image stattdessen aus dem Checkout zu bauen, füge die Override-Datei hinzu: `docker compose -f compose.yaml -f compose.build.yaml up -d --build`.
- Der Dienst `schema` führt `my schema apply` des Server-Images aus, das die Datenbank in einem benannten Volume auf das
  Schema dieses Releases bringt, und beendet sich; der Server (`role=all`) startet, sobald das gelungen ist. Das Schema
  wird bei jedem `up` erneut angewendet, nach einem Upgrade startest du den Server also auf dieselbe Weise.
- Der Server lauscht auf `127.0.0.1:8080` (der Port ist `MY_HTTP_PORT` in `.env`). Er ist bereit, wenn
  `curl localhost:8080/readyz` `{"status":"ok"}` zurückgibt.
- Der Key Encryption Key (KEK) erreicht den Server aus `secrets/kek` als Docker-Secret `/run/secrets/my_kek`, nie als
  Umgebungsvariable ([den Key Encryption Key verwalten](key-management.md)).
- Jede andere Einstellung der [Konfigurationsreferenz](../reference/configuration.md) setzt du in `.env`, deren Vorlage
  sie beschreibt.

<!-- source: create-an-administrator c3d30824f7c0 -->
## Einen Administrator anlegen

Lege einen Administrator an und suche das Konto dann mit WebFinger heraus und rufe seinen Actor ab. Das Passwort wird
aus der ersten Zeile der Standardeingabe gelesen.

```sh
docker compose exec -T my my admin create -username alice -email alice@example.com < password.txt
curl 'http://localhost:8080/.well-known/webfinger?resource=acct:alice@localhost:8080'
curl -H 'Accept: application/activity+json' http://localhost:8080/ap/actors/<the UUID of the self link from WebFinger>
```

<!-- source: sign-in-from-a-browser 3028115543b0 -->
## Im Browser anmelden

Mit dem Paar `MY_HOST=localhost:8080` und `MY_WEB_ORIGIN=http://localhost:8080` aus `example.env` funktionieren
Anmeldung, Client-API und Medien über einfaches HTTP unter `http://localhost:8080` (ActivityPub-IDs bleiben
`https://localhost:8080/...`). Wenn du `MY_HTTP_PORT` änderst, ändere den Port von `MY_HOST` und `MY_WEB_ORIGIN`
entsprechend.

1. Öffne `http://localhost:8080/login` in einem Browser (oder drücke **Anmelden** in der oberen Leiste des Web-Clients); das öffnet den Anmeldebildschirm des Servers.
2. Gib auf dem Anmeldebildschirm die E-Mail-Adresse und das Passwort des Administrators ein, den du oben angelegt hast.
3. Du bist zurück im Web-Client, in deiner Home-Timeline.

- Das Anmelde-Cookie ist `Secure`. Chrome und Firefox akzeptieren es auf `http://localhost`; Safari (WebKit) nicht, nutze lokal also Chrome oder Firefox.
- Links von Hashtags und Erwähnungen in Beitragstexten verwenden `https://`, da es die föderierten URLs sind.

<!-- source: going-public be2475df6407 -->
## Öffentlich gehen

- Setze `MY_HOST` in `.env` auf deine echte Domain, entferne `MY_WEB_ORIGIN` (es ist standardmäßig `https://{MY_HOST}`)
  und beende TLS vor dem Server: Die Ebene `compose.caddy.yaml` startet dafür Caddy
  ([Docker-Compose-Dateien](running.md#docker-compose-files)). `MY_HOST` wird beim ersten Start in der Datenbank
  gespeichert und lässt sich später nicht mehr ändern.
- Setze hinter einem eigenen Reverse-Proxy `MY_TRUSTED_PROXIES` in `.env`, damit Ratenbegrenzungen wie die bei der
  Anmeldung pro Client-IP gelten statt für den Proxy ([`trusted_proxies`](../reference/configuration.md#trusted_proxies)).
- Gehe die [Checkliste für den Produktivbetrieb](production-checklist.md) durch, bevor du den Server für Nutzer öffnest.
- `docker compose down` stoppt den Stack. `down -v` löscht außerdem die Daten-Volumes.

Alle Einstellungen findest du in der [Konfigurationsreferenz](../reference/configuration.md); zum alltäglichen Betrieb
siehe [Den Server betreiben](running.md).
