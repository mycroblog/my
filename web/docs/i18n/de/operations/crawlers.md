<!-- source: search-engines-and-ai-crawlers 648c61a01042 -->
# Suchmaschinen und KI-Crawler

Was der Server Suchmaschinen und KI-Crawlern mitteilt, welche Einstellungen das ändern und wie du prüfst, was er ausliefert.

robots.txt und Meta-Tags sind Aussagen, die Crawler beachten können oder nicht; sie sind keine technische Sperre. Wenn du sperren musst, nutze eine WAF oder Rate Limiting.

<!-- source: settings 13dfb1b04948 -->
## Einstellungen

| Bereich | Einstellung | Wo du sie änderst | Wenn nicht gesetzt |
| --- | --- | --- | --- |
| Server | `search_indexing` (`allow` / `deny`) | Der Abschnitt „Suchmaschinen und KI“ auf der Seite „Suche und Crawler“ der Server-Einstellungen für Administratoren, `my instance set`, `PATCH /api/v1/admin/settings` ([Servereinstellungen](instance-settings.md)) | `deny` |
| Server | `ai_usage` (`allow` / `deny_training` / `deny`) | Gleich | `deny` |
| Server | `indexable_default` (das `indexable` neuer Konten) | Gleich | `false` |
| Server | `security_contact` (wohin Sicherheitslücken gemeldet werden) | Gleich | Leer (es wird kein `security.txt` ausgeliefert) |
| Person | `indexable` (in der Suche erscheinen) | „Suchmaschinen und KI“ in den Profileinstellungen, `PATCH /api/v1/accounts/update_credentials` | `indexable_default` bei der Registrierung |
| Person | `ai_opt_out` (KI-Training und -Nutzung ablehnen) | Gleich | `false` |

- Die Aussage einer Seite ist die strengere aus der Einstellung des Servers und der Einstellung des Seiteninhabers. Eine Person kann nicht mehr erlauben als der Server.
- Suchmaschinen wird mitgeteilt, dass sie nur die Profile und `public`-Beiträge von Konten mit `indexable = true` indexieren dürfen, auf einem Server mit `search_indexing = allow`.
  Ein Server, der durchsuchbar sein will, ändert sowohl `search_indexing` als auch `indexable_default`. Personen, die sich registriert haben, bevor du `indexable_default` geändert hast, sind nicht eingeschlossen, bis sie selbst `indexable` wählen.
- Eine geänderte Einstellung wirkt ohne Neustart in `/robots.txt`, in Seiten und in Medienantworten.

<!-- source: what-the-server-returns 31584e03601b -->
## Was der Server zurückgibt

| Antwort | Bei Ablehnung der Suche | Bei Ablehnung von KI |
| --- | --- | --- |
| Seiten von Profilen und `public`-Beiträgen | `noindex, noarchive` in `<meta name="robots">` und `X-Robots-Tag` | `noai, noimageai`, `Content-Usage: train-ai=n` (`train-ai=n, ai-use=n` bei `deny`) und `tdm-reservation: 1` (Header und Meta) |
| `unlisted`-Beiträge, 404-Seiten für versteckte oder nicht vorhandene Seiten, Seiten entfernter Konten, App-Bildschirme (`/home`, `/settings` und so weiter) | `noindex, noarchive` unabhängig von der Einstellung | Folgt der Servereinstellung |
| `/`, `/about`, `/about/crawler` | Folgt der Servereinstellung | Folgt der Servereinstellung |
| `/media/` (wenn dieser Server es ausliefert) | Folgt der Servereinstellung. Keine Aussage je Inhaber | Folgt der Servereinstellung. Keine Aussage je Inhaber |
| Die Client-API, ActivityPub-Dokumente, `/.well-known/`, `/nodeinfo/`, Anmelde- und OAuth-Bildschirme | `X-Robots-Tag: noindex` | Nicht hinzugefügt (robots.txt genügt) |

`noindex` hindert Link-Vorschauen nicht (siehe [Link-Vorschauen](link-previews.md)).

<!-- source: robotstxt 636c85dabf63 -->
### `/robots.txt`

`GET /robots.txt` wird aus den Servereinstellungen und der mitgelieferten Liste von KI-Crawlern gebaut (`text/plain; charset=utf-8`, `Cache-Control: public, max-age=3600`, ETag).
Betreiber können keine eigenen Regeln hinzufügen.

- Ist `ai_usage` `deny_training` oder `deny`, erhalten Trainings-Crawler (`GPTBot`, `ClaudeBot`, `CCBot`, `Google-Extended`, `Applebot-Extended` und so weiter) `Disallow: /`.
- Ist `ai_usage` `deny_training` oder `deny`, bekundet eine Gruppe `User-agent: __usage_ai-training` mit `Disallow: /` für jeden Pfad eine Präferenz gegen KI-Training, egal welcher Crawler den Inhalt abruft
  (das Internet-Draft `draft-kelley-aipref-training-preference-signal`). Sie benennt eine Nutzung, keinen Crawler, und stoppt daher keine Abrufe. Bei `ai_usage = allow` entfällt die Gruppe und bekundet damit keine Präferenz, denn ihr `Allow: /` würde für jeden Pfad eine Präferenz zugunsten von Training bekunden, auch für die Seiten von Personen, die `ai_opt_out` gesetzt haben.
- Ist `ai_usage` `deny`, erhalten auch die Crawler von KI-Suche, Assistenten und Agenten (`OAI-SearchBot`, `ChatGPT-User`, `Claude-User`, `PerplexityBot` und so weiter) `Disallow: /`.
- Allen Crawlern (`User-agent: *`) werden nur `/api/`, `/oauth/`, `/auth/`, `/ap/` und `/media/proxy/` verwehrt, und die Einstellungen werden in Zeilen `Content-Usage` (das Draft der IETF AI Preferences) und
  `Content-Signal` (Content Signals) ausgedrückt. Das Ablehnen der Suche erzeugt kein `Disallow: /`: Blockierte Abrufe würden Crawler daran hindern, `noindex` zu lesen,
  und würden auch die Link-Vorschau-Abrufer stoppen, die robots.txt beachten.
- `/.well-known/` und `/nodeinfo` werden nicht verwehrt, damit Serverlisten und Statistiken weiter funktionieren.
- Ist `account_domain` vom Host getrennt, liefert die Host-Seite sie aus. Die robots.txt der Domain des Handles ist Sache des Betreibers.

<!-- source: well-known 72d7e8cbbc33 -->
### `/.well-known/`

| Pfad | Ausgeliefert, wenn | Inhalt |
| --- | --- | --- |
| `/.well-known/tdmrep.json` | `ai_usage` alles außer `allow` ist | `[{"location": "/*", "tdm-reservation": 1}]` (TDM Reservation Protocol). 404 bei `allow` |
| `/.well-known/security.txt` | `security_contact` gesetzt ist | `Contact`, `Expires` (30 Tage nach dem Zeitpunkt der Antwort), `Preferred-Languages` (`default_locale` und `en`), `Canonical` (RFC 9116) |

<!-- source: information-page 0f89d6c3c3ce -->
### Informationsseite

Die Informationsseite des Servers (`/about`) zeigt anhand von `search_indexing` und `ai_usage` in `GET /api/v1/instance`, ob Suche und KI erlaubt sind und was die Aussage bedeutet.
Die Erläuterung ordnet die Aussagen gegenüber Artikel 30-4 des japanischen Urheberrechtsgesetzes (solche Aussagen werden womöglich nicht als rechtliche Ablehnung behandelt) und Artikel 4 der EU-DSM-Richtlinie (ein maschinenlesbarer Vorbehalt für Text und Data Mining) ein.
Sie garantiert keine Rechtswirkung.

<!-- source: this-servers-own-fetching 0cb9510e19e5 -->
## Die eigenen Abrufe dieses Servers

Diese Einstellungen betreffen, was diesen Server durchsucht. Der Server durchsucht auch andere für die Karten von Links in Beiträgen, als `my-linkcard`: Er beachtet `robots.txt` und die Direktiven der verlinkten Seiten und begrenzt seine Anfragen pro Host, wie [Link-Vorschauen](link-previews.md#cards-of-links-in-posts) beschreibt.
Dieser Crawler wird mit `link_cards`, `link_cards_ignore_robots_txt` und pro Domain mit [Link-Karten-Domains](instance-settings.md#link-card-domains) eingestellt.

Die Betreiber dieser Websites lesen darüber unter `https://<host>/about/crawler`, worauf `/about` verlinkt ([die Seite für Website-Betreiber](link-previews.md#the-page-for-site-operators)). Ist `link_cards_ignore_robots_txt` an, können sie nicht per `robots.txt` widersprechen, und die Seite sagt das: Sie können die Karte nur mit den Direktiven ihrer Seiten einschränken oder dich um eine Regel für ihre Domain bitten.

<!-- source: limits 93da7e422ee8 -->
## Grenzen

- Das `ai_opt_out` einer Person wird nur auf Seiten ausgedrückt, die dieser Server ausliefert. Medien haben keinen Inhaber im Schlüssel, und manche Setups lassen ein CDN sie direkt ausliefern, deshalb tragen Medien nur die serverweite Aussage.
  Aussagen auf Seiten, die andere Server ausliefern, bleiben diesen Servern überlassen (es wird keine eigene ActivityPub-Erweiterung gesendet).
- `ai_usage = deny` bekundet auch eine Ablehnung der Vorgänge, bei denen ein Nutzer von ChatGPT oder einem ähnlichen Dienst eine Seite dieses Servers lesen lässt.
- Das `indexable` eines entfernten Kontos (Mastodons `toot:indexable`) wird empfangen und gespeichert. Seiten entfernter Konten sind auf diesem Server immer `noindex`.
- `indexable` entscheidet auch, ob die Beiträge einer Person in der eigenen [Volltextsuche](search.md) dieses Servers stehen, auf einem Server mit Suchmaschine. Dort deckt es die Sichtbarkeiten aus `search_visibilities` ab, zu denen `local` gehören kann. `search_indexing` hat auf diese Suche keine Wirkung, und was Suchmaschinen mitgeteilt wird, bleibt so, wie diese Seite es beschreibt.

<!-- source: serving-media-from-object-storage-or-a-cdn 98c7ab4994ab -->
## Medien aus Object Storage oder einem CDN ausliefern

Wenn Object Storage oder ein CDN Medien direkt ausliefert, laufen die `/media/`-Antworten nicht über diesen Server und tragen die Header aus der Tabelle oben nicht.
Die serverweite Aussage gilt über `/robots.txt` weiterhin für alles; um auch die Header hinzuzufügen, ergänze Folgendes für `/media/*` in den Response-Header-Regeln des CDN (ein Beispiel für `ai_usage = deny`).

```text
X-Robots-Tag: noindex, noarchive, noai, noimageai
Content-Usage: train-ai=n, ai-use=n
tdm-reservation: 1
```

Wenn du die Einstellungen änderst, passe die CDN-Regeln entsprechend an. `/media/proxy/` liefert dieser Server aus, es braucht also keine Regel.

<!-- source: the-ai-crawler-list 675a37cb0caf -->
## Die Liste der KI-Crawler

Die Liste wird aus `robots.json` von [ai-robots-txt/ai.robots.txt](https://github.com/ai-robots-txt/ai.robots.txt) (MIT) erzeugt und in die Binärdatei eingebunden. Sie wird zur Laufzeit nicht abgerufen, eine neuere Liste kommt also mit einem Upgrade.

<!-- source: verify 1633f03abedb -->
## Prüfen

```sh
curl -sS https://<host>/robots.txt
curl -sS -o /dev/null -D - -H 'Accept: text/html' https://<host>/@alice | grep -iE 'x-robots-tag|content-usage|tdm-reservation'
curl -sS https://<host>/.well-known/tdmrep.json
curl -sS https://<host>/.well-known/security.txt
```

- Auf einem Server ohne Einstellungen enthält `/robots.txt` `User-agent: GPTBot`, `User-agent: __usage_ai-training` und `Content-Usage: search=n, train-ai=n, ai-use=n`,
  und Profile tragen `X-Robots-Tag: noindex, noarchive, noai, noimageai`.
- Mit `search_indexing = allow` und `ai_usage = allow` trägt das Profil eines Kontos, das `indexable` gewählt hat, kein `X-Robots-Tag`, und `/.well-known/tdmrep.json` ist 404.
