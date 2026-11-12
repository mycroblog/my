<!-- source: link-previews-ogp--x-card--fediversecreator 85d4b284637f -->
# Linkvorschauen (OGP / X Card / fediverse:creator)

Die Vorschauen, die andere Dienste von den Seiten dieses Servers zeigen, was der Server dafür zurückgibt und die Schritte, sie von Hand zu prüfen. Die Karten, die dieser Server für die Links in seinen eigenen Beiträgen zeigt,
stehen unten unter [Karten für Links in Beiträgen](#cards-of-links-in-posts); wie du das Abrufen von einer Domain stoppst, steht in den [Servereinstellungen](instance-settings.md#link-card-domains).

<!-- source: what-the-server-returns fe60560f1b91 -->
## Was der Server zurückgibt

`my serve` gibt das HTML der Startseite (`https://<host>/`), der Informationsseite der Instanz (`https://<host>/about`),
der Seiten der veröffentlichten Dokumente (`/about/terms`, `/about/privacy`, `/about/commerce`), der Seite über den Link-Karten-Crawler (`/about/crawler`), von Profilen
(`https://<host>/@alice`) und Beiträgen (`https://<host>/@alice/status/<post ID>`) zurück, indem es Meta-Tags in die Shell des
Webclients einbettet.
Eine Anfrage nach HTML wird immer als abgemeldet behandelt, sodass eine Vorschau nur zeigt, was ein abgemeldeter Besucher sehen kann.

| Seite | Antwort |
| --- | --- |
| Die Startseite (`/`) und die Informationsseite der Instanz (`/about`) | 200. Die Shell mit Meta-Tags, die die Instanz vorstellen (siehe „Die Startseite“ unten) |
| Die Seite über den Link-Karten-Crawler (`/about/crawler`) | 200. Die Shell mit Meta-Tags, die die Instanz vorstellen, mit dem Titel „Link-Karten-Crawler \| Instanzname“ |
| Die Seiten veröffentlichter Dokumente (`/about/terms`, `/about/privacy`, `/about/commerce`) | 200. Die Shell mit Meta-Tags, die die Instanz vorstellen, mit dem Titel „Dokumentname \| Instanzname“. Die Seite eines Dokuments, das nicht veröffentlicht ist, erhält die generische Shell |
| Ein `public`-Beitrag, das Profil eines aktiven lokalen Kontos | 200. Die Shell mit Meta-Tags |
| Ein `unlisted`-Beitrag | 200. Die Shell mit Meta-Tags, dazu `<meta name="robots" content="noindex, noarchive">` und `X-Robots-Tag: noindex, noarchive` |
| Ein `local`-, `private`-, `direct`-, gelöschter oder nicht vorhandener Beitrag sowie das Profil und die Beiträge eines gesperrten, deaktivierten, gelöschten oder eingeschränkten Kontos | 404. Die generische Shell und `X-Robots-Tag: noindex, noarchive`. Body und Header sind unabhängig vom Grund gleich |
| `/@<old username>/status/<post ID>` | 301, auf die URL mit dem aktuellen Benutzernamen |
| Die Seite eines Hashtags (`/tags/<tag>`) | 200. Die generische Shell, mit dem Link zum Atom-Feed des Hashtags (siehe „Die generische Shell“ unten) |
| Jede andere Seite des Webclients (Entdecken, die öffentlichen Seiten zu Kosten und Unterstützern, Anmeldung und so weiter) | 200. Die generische Shell, mit den Meta-Tags der Instanz (siehe „Die generische Shell“ unten) |

Jede Seite trägt außerdem die Erklärungen an Suchmaschinen und KI-Crawler (Meta-Tags wie `noindex` und `noai` sowie
`X-Robots-Tag`, `Content-Usage` und `tdm-reservation`), gemäß den Instanzeinstellungen und den Einstellungen der Inhaberin oder des Inhabers
([Suchmaschinen und KI-Crawler](crawlers.md)). Das `noindex` in der Tabelle oben wird immer hinzugefügt, was auch immer die Einstellungen sind.
`noindex` stoppt keine Link-Vorschauen.

Eine Anfrage mit einem ActivityPub-`Accept` erhält nicht die Shell; die ActivityPub-Seite beantwortet sie (ein Profil antwortet mit der
`id` des Akteurs, und ein Beitrag, den ein abgemeldeter Besucher sehen kann, antwortet mit einem 302 auf seine `id`). Jede Antwort trägt `Vary: Accept`.

<!-- source: meta-tags 1a514282794e -->
### Meta-Tags

| Tag | Inhalt |
| --- | --- |
| `og:title`, `<title>` | „Anzeigename (@Benutzername@Kontodomain)“. Der Benutzername, wenn der Anzeigename leer ist. Bei einem Artikel sein Titel |
| `og:type` | `profile` bei einem Profil, `article` bei einem Beitrag und einem Artikel |
| `og:url`, `<link rel="canonical">` | Die kanonische URL (die URL mit dem aktuellen Benutzernamen) |
| `og:description` | Der reine Text des Beitragstextes (die Bio bei einem Profil) ohne Tags, auf 200 Grapheme gekürzt. Bei einem Beitrag mit Inhaltswarnung nur der Text der Inhaltswarnung. Bei einem Artikel seine Zusammenfassung, auch wenn er als sensibel markiert ist; der Text eines Artikels steht nie in der Seite |
| `og:image`, `og:image:alt` / `width` / `height` | Das Bild, das in der Reihenfolge unten gewählt wird |
| `og:site_name` | Der Name der Instanz (der Text `name`, in jeder Sprache derselbe; `host`, wenn es keinen gibt; [Name und Domain](instance-settings.md#name-and-domain)) |
| `og:locale`, `<html lang dir>` | Die Sprache des Beitrags. Ist sie nicht gesetzt (`und`), die Standard-Beitragssprache des Autors und, falls auch die fehlt, das `default_locale` der Instanz. Sie ändert sich nicht mit `Accept-Language`. `og:locale` hat die Form `language_TERRITORY`: Eine Sprache ohne Region erhält ihre übliche (`ja` → `ja_JP`, `en` → `en_US`, `pt` → `pt_BR`), und `<html lang>` behält den Sprachtag, wie er ist |
| `article:published_time`, `article:author` | Nur Beiträge. Die Zeit des Beitrags (UTC) und die Profil-URL des Autors |
| `profile:username` | Nur Profile. `username@account domain` |
| `twitter:card` | `summary_large_image`, wenn das Bild breiter als etwa 2:1 ist (Breite / Höhe >= 1.7) und mindestens 300x157, sonst `summary` |
| `fediverse:creator` | `@username@account domain`. Mastodon zeigt es als „Autor“ |
| `<link rel="alternate" type="application/activity+json">` | Die AP-`id` (der Akteur oder das Objekt des Beitrags) |
| `<link rel="alternate" type="application/atom+xml">` | Nur Profile, die einen Atom-Feed haben: die Adresse des Feeds (`https://<host>/@<username>.atom`), betitelt wie `og:title`. Ein Profil, dessen Inhaber den Feed ausgeschaltet hat, und ein eingeschränktes oder umgezogenes Konto haben keinen |

Die Meta-Tags der Startseite stellen die Instanz selbst vor.

- `og:type` ist `website`, `og:title`, `<title>` und `og:site_name` sind der Name der Instanz, und `og:url` und
  `<link rel="canonical">` sind `https://<host>/`.
- `og:description` ist der Instanztext `short_description`, auf 200 Grapheme gekürzt. Er entfällt, wenn es keinen gibt.
- Die Sprache (`og:locale`, `<html lang dir>`) ist das `default_locale` der Instanz. Der Name und die Beschreibung sind derselbe Wert
  in jeder Sprache.
- `og:image` ist das Instanzbild: Nummer 3 unten oder Nummer 4, 5 oder 6, wenn es keines gibt.
- Sie gehört zu keinem Konto, daher werden `fediverse:creator` und das ActivityPub-`alternate` nicht ausgegeben.
- Die Informationsseite der Instanz (`/about`) hat dieselben Meta-Tags. `og:title` und `<title>` sind „Über {Instanzname}“
  (`server:preview.about_title`), und `og:url` und `<link rel="canonical">` sind `https://<host>/about`.
- Die Seiten veröffentlichter Dokumente (`/about/terms`, `/about/privacy`, `/about/commerce`) haben ebenfalls dieselben Meta-Tags.
  `og:title` und `<title>` sind „{Dokumentname} | {Instanzname}“ (`server:preview.document_title`), und `og:url` und
  `<link rel="canonical">` sind die Adresse dieser Seite. Die Seite eines Dokuments, das nicht veröffentlicht ist, erhält die generische Shell.
- Die Seite über den Link-Karten-Crawler (`/about/crawler`) hat dieselben Meta-Tags, mit derselben Form von Titel und Adresse.

Die generische Shell, die jede andere Seite und das 404 eines verborgenen Beitrags zurückgeben, trägt dieselben Tags wie die Startseite
ohne eine eigene Adresse: `og:type` `website`, den Namen der Instanz als `og:site_name` und `og:title`, `short_description`
als `og:description`, `og:locale`, das Instanzbild und `twitter:card`. Sie hat kein `og:url` und kein
`<link rel="canonical">`, sodass eine Kopie jeden Pfad bedient und das 404 eines verborgenen Beitrags mit jeder anderen Seite identisch bleibt.
Die Seite eines Hashtags fügt nur `<link rel="alternate" type="application/atom+xml">` zum Feed des Hashtags hinzu
(`https://<host>/tags/<tag>.atom`), betitelt „#tag auf Instanzname“ (`server:feed.tag_title`), unabhängig davon, ob schon ein Beitrag
den Hashtag verwendet hat. Ein geteilter Link zu einem Hashtag oder zur Seite der öffentlichen Kosten zeigt die Karte der Instanz.

Die Feeds selbst sind Atom-1.0-Dokumente der 20 neuesten lokalen öffentlichen Beiträge, die keine Antworten oder Boosts sind, mit
`ETag`, `Last-Modified` und `Cache-Control: public, max-age=60`. Der Eintrag eines Artikels ist sein Titel und seine Zusammenfassung, mit dem
Link zum Artikel; sein Text bleibt auf der Seite des Artikels. Jedes Profil mit einem Feed hat außerdem einen reinen Artikel-Feed,
`https://<host>/@<username>/articles.atom`, betitelt „Artikel von Name (@Handle)“ (`server:feed.articles_title`), der
denselben Regeln folgt; die Seite des Profils verlinkt nur seinen Haupt-Feed. Der Server hält jeden gerenderten Feed eine Minute lang in seinem Key-Value-Speicher,
sodass eine Änderung einen Feed innerhalb von etwa einer Minute erreicht. Ein Feed trägt dasselbe `X-Robots-Tag` und dieselben KI-Erklärungen
wie sein Profil; der Feed eines Hashtags ist immer `noindex` und gibt die KI-Ablehnung jedes seiner Autoren an. Ein
Profil ohne Feed und ein Hashtag, den noch kein Beitrag verwendet hat, antworten mit 404.

`og:image` wird in dieser Reihenfolge gewählt.

1. Das erste an den Beitrag angehängte Bild oder das Titelbild eines Artikels (sein Alt-Text geht in `og:image:alt`). Es wird nicht
   verwendet bei einem Beitrag oder Artikel, der als sensibel markiert ist, oder bei einem Beitrag mit Inhaltswarnung.
2. Das Header-Bild des Profils oder sonst der Avatar (`og:image:alt` ist „Titelbild von {name}“ oder „Profilbild von {name}“).
3. Das Link-Vorschaubild der Instanz (die Instanzeinstellung `preview_image`, unter „Link-Vorschauen“ in Einstellungen → Allgemein der
   Administration; `og:image:alt` ist der Text `preview_image_alt` oder „Bild von {Instanzname}“, wenn es keinen gibt).
4. Das Thumbnail der Instanz (die Instanzeinstellung `thumbnail`, das Bild der Seite Einstellungen → Allgemein; `og:image:alt` ist der
   Text `thumbnail_alt` oder „Bild von {Instanzname}“, wenn es keinen gibt).
5. Die Link-Vorschau-Version des eigenen Logos der Instanz ([Logo und Symbol](instance-settings.md#logo-and-icon)), ein PNG mit 1200x630
   unter `https://<host>/brand/<revision>/preview-1200x630.png` (`og:image:alt` ist „Bild von {Instanzname}“).
6. Das mitgelieferte Standardbild (`https://<host>/preview-image.png`, ein PNG mit 1200x630 mit dem Schriftzug von mycroblog auf Weiß, ohne
   den Namen der Instanz; `og:image:alt` ist „Das Logo von mycroblog“).

Die Nummern 3 bis 6 sind das Instanzbild. Eine Seite, die eine Vorschau zeigt, hat also immer `og:image` und `og:image:alt`.
Die Bilder von 1 bis 4 sind die `large`-Variante des verarbeiteten Mediums, und Breite und Höhe sind die Werte aus der Verarbeitung.
Ist `preview_image` nicht gesetzt, wird noch verarbeitet, ist bei der Verarbeitung fehlgeschlagen oder ist kein Bild, wird das Vorschaubild verwendet, dann das Logo,
dann das mitgelieferte Bild. Instanzeinstellungen werden bei jeder Anfrage gelesen, sodass `my instance set preview_image ...` ohne Neustart wirkt
(das Verfahren steht unter [Instanzeinstellungen](instance-settings.md)).

Das mitgelieferte Bild ist Teil des Binarys (etwa 20 KB). Es wird mit `Cache-Control: no-cache` und einem ETag ausgeliefert.

<!-- source: caching e6ace7ad3fd0 -->
### Caching

- Die Antwort trägt `Cache-Control: public, max-age=60, stale-while-revalidate=300`. Eine Löschung, eine Änderung der Sichtbarkeit oder
  das Veröffentlichen oder Zurückziehen eines Dokuments bleibt höchstens so lange in geteilten Caches.
- Der ETag wird aus den Versionen des Templates, der Assets und der Instanzeinstellungen, der Sprache, der Version
  (Revision) des Beitrags, der wirksamen Sichtbarkeit, der Version des Profils und des gewählten Bildes gebildet (bei der Startseite, der
  Informationsseite, den Dokumentseiten und der Seite über den Crawler: der Adresse der Seite, ihrem Titel, der Instanzbeschreibung und dem gewählten Bild;
  bei der generischen Shell: dem Namen der Instanz, ihrer Beschreibung und dem Instanzbild).
  `If-None-Match` erhält `304`.
- Wenn du ein CDN davorsetzt, nimm `Accept` in den Cache-Key auf (damit AP-JSON und HTML nicht vermischt werden).

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
