<!-- source: link-previews-ogp--x-card--fediversecreator 85d4b284637f -->
# Linkvorschauen (OGP / X Card / fediverse:creator)

Die Vorschauen, die andere Dienste von den Seiten dieses Servers zeigen, was der Server dafür zurückgibt und die Schritte, sie von Hand zu prüfen. Die Karten, die dieser Server für die Links in seinen eigenen Beiträgen zeigt,
stehen unten unter [Karten für Links in Beiträgen](#cards-of-links-in-posts); wie du das Abrufen von einer Domain stoppst, steht in den [Servereinstellungen](instance-settings.md#link-card-domains).

<!-- source: what-the-server-returns bb9326f9729a -->
## Was der Server zurückgibt

`my serve` gibt das HTML der Startseite (`https://<host>/`), der Server-Informationsseite (`https://<host>/about`),
der Seiten der veröffentlichten Dokumente (`/about/terms`, `/about/privacy`, `/about/commerce`), der Seite über den Link-Karten-Crawler (`/about/crawler`), von Profilen
(`https://<host>/@alice`) und Beiträgen (`https://<host>/@alice/status/<post ID>`) zurück, indem er Meta-Tags in die Shell des
Web-Clients einbettet.
Eine Anfrage nach HTML wird immer als abgemeldet behandelt, sodass eine Vorschau nur zeigt, was ein abgemeldeter Besucher sehen kann.

| Seite | Antwort |
| --- | --- |
| Die Startseite (`/`) und die Server-Informationsseite (`/about`) | 200. Die Shell mit Meta-Tags, die den Server vorstellen (siehe „Die Startseite“ unten) |
| Die Seite über den Link-Karten-Crawler (`/about/crawler`) | 200. Die Shell mit Meta-Tags, die den Server vorstellen, mit dem Titel „Link card crawler \| Servername“ |
| Die Seiten veröffentlichter Dokumente (`/about/terms`, `/about/privacy`, `/about/commerce`) | 200. Die Shell mit Meta-Tags, die den Server vorstellen, mit dem Titel „Dokumentname \| Servername“. Die Seite eines nicht veröffentlichten Dokuments bekommt die generische Shell |
| Ein `public`-Beitrag, das Profil eines aktiven lokalen Kontos | 200. Die Shell mit Meta-Tags |
| Ein `unlisted`-Beitrag | 200. Die Shell mit Meta-Tags sowie `<meta name="robots" content="noindex, noarchive">` und `X-Robots-Tag: noindex, noarchive` |
| Ein `local`-, `private`-, `direct`-, gelöschter oder nicht existierender Beitrag sowie das Profil und die Beiträge eines gesperrten, deaktivierten, gelöschten oder eingeschränkten Kontos | 404. Die generische Shell und `X-Robots-Tag: noindex, noarchive`. Body und Header sind unabhängig vom Grund gleich |
| `/@<old username>/status/<post ID>` | 301, auf die URL mit dem aktuellen Benutzernamen |

Jede Seite trägt außerdem die Angaben für Suchmaschinen und KI-Crawler (Meta-Tags wie `noindex` und `noai` sowie
`X-Robots-Tag`, `Content-Usage` und `tdm-reservation`), gemäß den Servereinstellungen und den Einstellungen des Eigentümers
([Suchmaschinen und KI-Crawler](crawlers.md)). Das `noindex` in der Tabelle oben wird immer hinzugefügt, egal wie die Einstellungen sind.
`noindex` stoppt keine Linkvorschauen.

Eine Anfrage mit einem ActivityPub-`Accept` bekommt nicht die Shell; die ActivityPub-Seite beantwortet sie (ein Profil antwortet mit der
`id` des Actors, und ein Beitrag, den ein abgemeldeter Besucher sehen kann, antwortet mit einem 302 auf seine `id`). Jede Antwort trägt `Vary: Accept`.

<!-- source: meta-tags 00a4a8115b9e -->
### Meta-Tags

| Tag | Inhalt |
| --- | --- |
| `og:title`, `<title>` | „Anzeigename (@Benutzername@Konto-Domain)“. Der Benutzername, wenn der Anzeigename leer ist |
| `og:type` | `profile` für ein Profil, `article` für einen Beitrag |
| `og:url`, `<link rel="canonical">` | Die kanonische URL (die URL mit dem aktuellen Benutzernamen) |
| `og:description` | Der reine Text des Beitrags (die Bio bei einem Profil) ohne Tags, auf 200 Grapheme gekürzt. Bei einem Beitrag mit Inhaltswarnung nur der Text der Inhaltswarnung |
| `og:image`, `og:image:alt` / `width` / `height` | Das Bild, das in der Reihenfolge unten gewählt wird |
| `og:site_name` | Der Servername (der Text `name`, in jeder Sprache gleich; `host`, wenn es keinen gibt; [Name und Domain](instance-settings.md#name-and-domain)) |
| `og:locale`, `<html lang dir>` | Die Sprache des Beitrags. Wenn sie nicht gesetzt ist (`und`), die Standard-Beitragssprache der Autorin oder des Autors, und andernfalls das `default_locale` des Servers. Sie ändert sich nicht mit `Accept-Language` |
| `article:published_time`, `article:author` | Nur bei Beiträgen. Der Zeitpunkt des Beitrags (UTC) und die Profil-URL der Autorin oder des Autors |
| `profile:username` | Nur bei Profilen. `username@account domain` |
| `twitter:card` | `summary_large_image`, wenn das Bild breiter als etwa 2:1 ist (Breite / Höhe >= 1,7) und mindestens 300x157, sonst `summary` |
| `fediverse:creator` | `@username@account domain`. Mastodon zeigt es als „Autor“ |
| `<link rel="alternate" type="application/activity+json">` | Die AP-`id` (der Actor oder das Objekt des Beitrags) |

Die Meta-Tags der Startseite stellen den Server selbst vor.

- `og:type` ist `website`, `og:title`, `<title>` und `og:site_name` sind der Servername, und `og:url` und
  `<link rel="canonical">` sind `https://<host>/`.
- `og:description` ist der Servertext `short_description`, auf 200 Grapheme gekürzt. Er entfällt, wenn es keinen gibt.
- Die Sprache (`og:locale`, `<html lang dir>`) ist das `default_locale` des Servers. Name und Beschreibung sind in jeder Sprache
  derselbe Wert.
- `og:image` ist Nummer 3 unten (das Vorschaubild des Servers) oder Nummer 4 oder 5, wenn es keines gibt.
- Sie gehört zu keinem Konto, daher werden `fediverse:creator` und das ActivityPub-`alternate` nicht ausgegeben.
- Die Server-Informationsseite (`/about`) hat dieselben Meta-Tags. `og:title` und `<title>` sind „Über {Servername}“
  (`server:preview.about_title`), und `og:url` und `<link rel="canonical">` sind `https://<host>/about`.
- Auch die Seiten veröffentlichter Dokumente (`/about/terms`, `/about/privacy`, `/about/commerce`) haben dieselben Meta-Tags.
  `og:title` und `<title>` sind „{Dokumentname} | {Servername}“ (`server:preview.document_title`), und `og:url` und
  `<link rel="canonical">` sind die Adresse dieser Seite. Die Seite eines nicht veröffentlichten Dokuments bekommt die generische Shell.
- Die Seite über den Link-Karten-Crawler (`/about/crawler`) hat dieselben Meta-Tags, mit derselben Form von Titel und Adresse.

`og:image` wird in dieser Reihenfolge gewählt.

1. Das erste an den Beitrag angehängte Bild (sein Alt-Text kommt in `og:image:alt`). Es wird nicht für einen als sensibel markierten Beitrag oder
   einen mit Inhaltswarnung verwendet.
2. Das Titelbild des Profils oder sonst das Profilbild (`og:image:alt` ist „Titelbild von {name}“ oder „Profilbild von {name}“).
3. Das Vorschaubild des Servers (die Servereinstellung `preview_image`; `og:image:alt` ist der Text `preview_image_alt` oder „Bild
   von {Servername}“, wenn es keinen gibt).
4. Die Linkvorschau-Version des eigenen Logos des Servers ([Logo und Icon](instance-settings.md#logo-and-icon)), ein 1200x630-PNG
   unter `https://<host>/brand/<revision>/preview-1200x630.png` (`og:image:alt` ist „Bild von {Servername}“).
5. Das mitgelieferte Standardbild (`https://<host>/preview-image.png`, ein 1200x630-PNG. Es ist ein generisches Bild ohne den Servernamen,
   und `og:image:alt` beschreibt das Bild).

Eine Seite, die eine Vorschau zeigt, hat also immer `og:image` und `og:image:alt`.
Die Bilder von 1 bis 3 sind die `full`-Variante der verarbeiteten Medien, und Breite und Höhe sind die Werte aus der Verarbeitung.
Wenn `preview_image` noch verarbeitet wird, die Verarbeitung fehlgeschlagen ist oder es kein Bild ist, wird Nummer 4 (oder 5 ohne Logo) verwendet.
Servereinstellungen werden bei jeder Anfrage gelesen, daher wirkt `my instance set preview_image ...` ohne Neustart (das
Vorgehen steht in den [Servereinstellungen](instance-settings.md)).

Das Standardbild wird bei der ersten Anfrage nach dem Start mit der Standardbibliothek gezeichnet und im Prozess gehalten (etwa 10 KB).
Es wird mit `Cache-Control: no-cache` und einem ETag ausgeliefert.

<!-- source: caching 5bc906452b10 -->
### Caching

- Die Antwort trägt `Cache-Control: public, max-age=60, stale-while-revalidate=300`. Eine Löschung, eine Änderung der Sichtbarkeit oder
  das Veröffentlichen oder Zurückziehen eines Dokuments bleibt höchstens so lange in gemeinsam genutzten Caches.
- Der ETag wird aus den Versionen des Templates, der Assets und der Servereinstellungen, der Sprache, der Version
  (Revision) des Beitrags, der effektiven Sichtbarkeit, der Version des Profils und des gewählten Bildes gebildet (bei der Startseite, der
  Informationsseite, den Dokumentseiten und der Seite über den Crawler: die Adresse der Seite, ihr Titel, die Serverbeschreibung und das gewählte Bild).
  `If-None-Match` bekommt `304`.
- Wenn du ein CDN davorsetzt, nimm `Accept` in den Cache-Schlüssel auf (damit AP-JSON und HTML nicht vermischt werden).

<!-- source: manual-check bf2c7a86a5de -->
## Manuelle Prüfung

Prüfe jeden Dienst nach einem Upgrade oder einer Änderung der Vorschau-Einstellungen auf einem echten Gerät. Du brauchst einen Server, der von außen über HTTPS erreichbar ist (die
Vorschau-Bots erreichen `localhost` nicht).

<!-- source: preparation 9fd604dc1529 -->
### Vorbereitung

1. Bereite mit einem Konto zum Testen diese Beiträge vor.
   - Ein `public`-Beitrag mit einem Querformat-Bild (zum Beispiel 1200x630) und Alt-Text
   - Ein `public`-Beitrag ohne Bild (von einem Konto mit Titelbild)
   - Ein `public`-Beitrag, dessen Text länger als 200 Zeichen ist und Emoji enthält (wie 👨‍👩‍👧‍👦)
   - Ein `unlisted`-Beitrag
   - Ein `private`-Beitrag
   - Ein Beitrag von einem Konto, dessen Anzeigename `"><script>alert(1)</script>` enthält
2. Prüfe das HTML jeder URL.

   ```sh
   curl -sS -D - -H 'Accept: text/html' https://<host>/@alice/status/<post ID>
   ```

   - Der Status ist 200, und die Meta-Tags aus der Tabelle oben sind vorhanden. Der `unlisted`-Beitrag hat `noindex`.
   - Ein `private`-Beitrag und eine nicht existierende ID liefern denselben 404-Body mit `X-Robots-Tag: noindex, noarchive` (und auch `noai,
     noimageai`, wenn die Einstellungen KI ablehnen).

     ```sh
     diff <(curl -sS https://<host>/@alice/status/<private post ID>) <(curl -sS https://<host>/@alice/status/0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f)
     ```

   - Das `<script>` im Anzeigenamen wird als `&lt;script&gt;` maskiert.

<!-- source: x 73a2dffdfdbb -->
### X

1. Füge die URL des Beitrags mit Bild in den Verfassen-Bildschirm (oder eine DM) ein.
2. Prüfe, dass die große Bildkarte (`summary_large_image`) den Titel, die Domain und das Bild zeigt.
3. Prüfe bei der URL des Beitrags ohne Bild, dass die Karte das Titelbild zeigt. Prüfe bei einem Konto weder mit Titelbild
   noch mit Profilbild, dass die große Karte das Standardbild zeigt (`preview_image` oder das mitgelieferte Bild, wenn es
   nicht gesetzt ist).
4. Prüfe bei einem Konto mit nur einem quadratischen Profilbild, dass die Karte die kleine Bildkarte (`summary`) ist.

X speichert Karten im Cache. Um eine Änderung zu prüfen, füge die URL mit einer Query (wie `?v=2`) ein, damit sie als andere URL zählt.

<!-- source: mastodon 02868aebd430 -->
### Mastodon

1. Erstelle von einem Konto auf einem anderen Server einen Beitrag, der die URL des Beitrags mit Bild enthält.
2. Prüfe, dass unter dem Beitrag eine Vorschaukarte mit dem Titel, der Beschreibung und dem Bild erscheint.
3. Prüfe, dass das Testkonto (`fediverse:creator`) unter der Karte als „Autor“ angezeigt wird und dass du zu seinem
   Profil gehen kannst.

Mastodon speichert die Karte auf jedem Server, der sie abgerufen hat, im Cache. Um sie neu zu erzeugen, poste mit einer neuen URL (mit einer Query).

<!-- source: discord 4df04642bb64 -->
### Discord

1. Füge die URL des Beitrags mit Bild in einen Kanal ein.
2. Prüfe, dass das Embed den Seitennamen (den Servernamen), den Titel, die Beschreibung und ein großes Bild zeigt.
3. Prüfe bei einem Beitrag mit mehr als 200 Zeichen, dass die Beschreibung mit `…` endet und dass kein Emoji in der Mitte abgeschnitten ist.

<!-- source: slack 9f1e524968f3 -->
### Slack

1. Füge die URL des Beitrags mit Bild in einen Kanal ein.
2. Prüfe, dass die entfaltete Vorschau den Seitennamen, den Titel, die Beschreibung und das Bild zeigt.
3. Prüfe, dass auch für die URL eines `unlisted`-Beitrags eine Vorschau erscheint (`noindex` gilt für Suchmaschinen und stoppt
   keine Vorschauen).

Auch Slack speichert Vorschauen im Cache. Um eine Änderung zu prüfen, füge der URL eine Query hinzu.

<!-- source: line ef9e1e682603 -->
### LINE

1. Sende die URL des Beitrags mit Bild in einem Chat.
2. Prüfe, dass eine Vorschau mit dem Titel, der Beschreibung und dem Bild erscheint.

Auch LINE speichert Vorschauen im Cache. Um eine Änderung zu prüfen, füge der URL eine Query hinzu.

<!-- source: what-must-not-be-visible 187905fe778d -->
### Was nicht sichtbar sein darf

Prüfe bei jedem Dienst, dass die URL eines `private`-Beitrags und die URL eines nicht existierenden Beitrags dieselbe generische Vorschau liefern
(nur der Servername oder nichts) und dass weder Text noch Anzeigename noch Bild erscheint.

<!-- source: cards-of-links-in-posts 6394a6bf5641 -->
## Karten für Links in Beiträgen

Um eine Karte für einen Link in einem Beitrag zu zeigen, ruft der Job dieses Servers die verlinkte Seite und ihr Bild ab und befolgt, was die verlinkte Seite dazu sagt.
Seine Anfragen sind `my-linkcard/<version> (+https://<host>/)` für die Seite, das Bild und `robots.txt`. Jede andere Anfrage des Servers, auch die des Medien-Proxys, bleibt `my/<version> (+https://<host>/)`.
Die Anfragen senden keine Cookies und keinen `Referer`, und nichts sagt, welches Konto oder welcher Beitrag die Seite verlinkt hat.

<!-- source: the-page-for-site-operators 08a157cd9eac -->
### Die Seite für Seitenbetreiber

`https://<host>/about/crawler` sagt den Betreibern verlinkter Seiten, was der Crawler anfragt, was er sendet, wie oft und wie sie sich abmelden können, und dass sie dich bitten können, das Abrufen von ihrer Domain zu stoppen. Jeder kann sie ohne Konto lesen. Der Abschnitt „Suchmaschinen und KI“ der Server-Informationsseite (`/about`) verlinkt auf sie; der `User-Agent` nennt nur `https://<host>/`.

Sie sagt, was dieser Server tut, anhand von `link_cards` und `link_cards_ignore_robots_txt`, und sie zeigt den Kontakt des Servers (`contact_account`, `contact_email`). Setze einen Kontakt, damit eine Seite eine Anlaufstelle für Fragen hat: Ohne einen sagt die Seite, dass dieser Server keinen veröffentlicht.
Bei eingeschaltetem `link_cards_ignore_robots_txt` sagt die Seite, dass eine Seite diesen Server nicht in `robots.txt` ablehnen kann.

<!-- source: robotstxt e8d2deaef93d -->
### robots.txt

Der Name in `robots.txt` ist `my-linkcard`, in beliebiger Schreibweise. Ohne eine Gruppe dieses Namens gilt die Gruppe `*`, wie RFC 9309 es vorsieht. `Allow`, `Disallow`, `*` und `$` werden gelesen, wobei die längste Übereinstimmung gewinnt und bei Gleichstand `Allow`; `Crawl-delay` und andere Einträge werden nicht gelesen.
`robots.txt` wird bis zu 512 KiB in 10 Sekunden gelesen, und was von einer größeren Datei gelesen wurde, wird verwendet. Der Server behält nur die Regeln für `my-linkcard`, pro Origin (Schema, Host und Port).

| Antwort für `robots.txt` | Was der Server tut | Aufbewahrt für |
| --- | --- | --- |
| 200 | Befolgt die Datei | 24 Stunden |
| 4xx außer 429 | Keine Datei: ruft ab | 24 Stunden |
| 429, 5xx, ein Netzwerkfehler oder eine Weiterleitung, die der Server ablehnt | Ruft nichts von der Origin ab | 1 Stunde |
| Mehr als 5 Weiterleitungen | Keine Datei: ruft ab | 24 Stunden |

- Die URL der Seite wird geprüft, bevor sie angefragt wird, egal welche Sichtbarkeit der Beitrag hat. Das gilt auch für jede URL, auf die sie weiterleitet, gegen die `robots.txt` der Origin dieser URL, und eine Weiterleitung auf eine abgelehnte URL endet dort: Der Server fragt sie nicht an und erzeugt keine Karte.
- Eine URL, die `robots.txt` verbietet, wird 24 Stunden lang nicht erneut abgerufen, wie jeder andere Fehlschlag einer Seite. Wenn die Datei nur nicht gelesen werden konnte, versucht der Server die Seite eine Stunde später erneut, bis zum Limit der Versuche des Jobs.
- Ein Bild wird auf dieselbe Weise gegen seine eigene Origin geprüft. Ein verbotenes Bild hinterlässt eine Karte ohne Bild. Wenn die Datei nur nicht gelesen werden konnte, bleibt ein früher aufgezeichnetes Bild erhalten, und ein neues Bild wird bis zum nächsten erneuten Abruf weggelassen.
- Ein erneuter Abruf, den ein Beitrag 14 Tage nach dem letzten Abruf auslösen kann, entfernt die Karte, wenn `robots.txt` ihre URL verbietet. Wenn die Datei nur nicht gelesen werden konnte, bleibt die Karte.
- Der Server geht seine Karten nicht durch, um sie erneut zu prüfen, sodass eine Seite, die ihre Regeln ändert, eine Karte beim nächsten erneuten Abruf dieser Karte verliert.

`link_cards_ignore_robots_txt` ([Servereinstellungen](instance-settings.md#settings-one-key-one-value)) schaltet all das ab: Der Server ruft `robots.txt` weder ab noch verwendet er sie. Er nennt sich weiterhin `my-linkcard`, und verlinkte Seiten können die Abrufe dann nicht mit `robots.txt` ablehnen: Es bleiben nur die Direktiven ihrer Seiten und eine Bitte an dich um eine [Regel für ihre Domain](instance-settings.md#link-card-domains), wie [die Seite für Seitenbetreiber](#the-page-for-site-operators) es ihnen sagt.

<!-- source: directives-of-a-page a854ef2ae8ea -->
### Direktiven einer Seite

Der Server liest auch die Direktiven der Seite, die er abgerufen hat: `<meta name="robots">` und `<meta name="my-linkcard">` im `<head>` sowie `X-Robots-Tag` ohne Bot-Namen oder mit `my-linkcard:`. Direktiven für einen anderen Bot gelten nicht. Gibt es mehrere, gewinnt die strengere.
Sie gelten unabhängig davon, was `link_cards_ignore_robots_txt` sagt.

| Direktive | Karte |
| --- | --- |
| `nosnippet`, `max-snippet:0` | Keine Beschreibung |
| `noimageindex`, `max-image-preview:none` | Kein Bild, das nicht abgerufen wird |
| `noindex`, `nofollow`, `noarchive`, `none` und die übrigen | Keine Änderung |

Titel, Seitenname und Autor bleiben bei jeder Direktive erhalten; eine Seite, die sie nicht will, lehnt den Abruf in `robots.txt` ab.
Die Direktiven sind erst bekannt, nachdem die Seite abgerufen wurde, daher stoppen sie die Anfrage der Seite nicht. Ein erneuter Abruf wendet die Direktiven an, die die Seite dann hat.

<!-- source: limits-per-host 41838af70d25 -->
### Limits pro Host

Der Crawler geht mit jedem Host schonend um, egal was `link_cards_ignore_robots_txt` sagt. Ein Host ist der Name in der URL, ohne den Port, sodass eine verlinkte Seite und der Host ihrer Bilder getrennte Limits haben.

- Er ruft von einem Host jeweils nur eines gleichzeitig ab, und der nächste beginnt frühestens 1 Sekunde nach dem Ende des letzten. Ein Abruf ist eine Anfrage mit ihren Weiterleitungen und, wenn sie gelesen werden muss, der `robots.txt`, die sie braucht. Eine Seite und ihr Bild sind zwei Abrufe, auch auf einem Host.
- Ein `429` oder `503` stoppt die Abrufe von dem Host, der geantwortet hat, auch wenn er auf eine Weiterleitung, ein Bild oder `robots.txt` geantwortet hat. Sie stoppen für sein `Retry-After`, in Sekunden oder als Datum, für mindestens 1 Sekunde und höchstens 1 Stunde. Ein fehlendes oder nicht lesbares `Retry-After` bedeutet 1 Minute.
- Ein Job, der auf seinen Zug warten muss, kommt später zurück. Kein Worker wartet, und er verbraucht keinen der Versuche des Jobs, sodass selbst ein Ansturm von Links auf eine Seite damit endet, dass jede Karte da ist und kein Job fehlschlägt. Die Jobs eines Hosts wachen etwa im Sekundentakt auf.
- Eine URL, deren Host den Crawler gestoppt hat, behält ihre Karte, wird nicht als fehlgeschlagen gemerkt und wird abgerufen, wenn die Pause endet. Nach 5 solchen Antworten für eine URL innerhalb von 24 Stunden zählt die Seite wie jede andere, die kein `200` liefert, als fehlgeschlagen, und ein Bild wird aus der Karte weggelassen.
- Eine Karte, die auf ihr Bild wartet, behält bis zu 2 Stunden, was sie von der Seite gelesen hat, damit die Seite nicht erneut angefragt wird.
- [Regeln für eine Domain](instance-settings.md#link-card-domains) und Domain-Aktionen haben Vorrang: Nichts wartet auf einen Host, von dem nichts abgerufen wird.

Die Limits liegen im Key-Value-Speicher und laufen alle ab: Ein Worker, der mitten in einem Abruf anhält, gibt seinen Host innerhalb von 2 Minuten frei.
Diese Zahlen sind keine Einstellungen. So siehst du die Limits in Aktion:

- `my.link_card.host_pauses` zählt die Pausen nach dem Status, den der Host geantwortet hat, und jede Pause protokolliert `link card host paused` auf `INFO` mit `host`, `status` und `pause_seconds`.
- `my.job.duration` mit `my.job.type="content.fetch_link_card"` und `my.outcome="postponed"` zählt die Läufe, die später zurückkamen. Eine große Zahl bedeutet viele Links auf wenige Seiten oder eine lange Pause, keinen Fehler.
