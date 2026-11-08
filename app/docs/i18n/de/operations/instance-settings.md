<!-- source: instance-settings 4eb467f0dbca -->
# Servereinstellungen

Die Einstellungen, die festlegen, wie dein Server betrieben wird: Registrierungsmodus, Beitragslänge,
Standard-Locale, wer die Timelines sehen kann, der Servername, die Beschreibung, die Bedingungen, Regeln, das Design und eigene Emojis.

<!-- source: configuration-and-instance-settings a0b0908cef1e -->
## Konfiguration und Servereinstellungen

| | Konfiguration | Servereinstellungen |
| --- | --- | --- |
| Was | Wie das Deployment aufgebaut ist: Rollen, Adapter, Endpunkte, Hostnamen, Secrets | Wie der Server betrieben wird: Registrierungsmodus, Längenlimit für Beiträge, Standard-Locale, wer die Timelines sehen kann, der Servername, die Beschreibung und die Bedingungen |
| Wo | Umgebungsvariablen `MY_*` und die Konfigurationsdatei ([Konfigurationsreferenz](../reference/configuration.md)) | Die Datenbank. Jede Änderung wird als Revision aufbewahrt |
| Ändern | Wirkt, wenn der Prozess neu startet | Wirkt sofort mit `my instance set`, den Admin-Bildschirmen des Webclients oder der Client-API ([Admin-Bildschirme und die Client-API](#admin-screens-and-the-client-api)) |
| Wenn nicht gesetzt | Der Standard im Schema (der Server startet nicht, wenn der Schlüssel erforderlich ist) | Der im Code definierte Standard |

- **Kein Schlüssel steht in beiden.** Keines überschreibt das andere. Du kannst eine Servereinstellung nicht über eine Umgebungsvariable oder
  die Konfigurationsdatei angeben, und du kannst keinen Konfigurationswert in der Datenbank speichern.
- Ein Wert, der pro Deployment feststeht und die Struktur des Prozesses ändert, ist Konfiguration; ein Wert, der sich ändert, während
  der Server läuft, ist eine Servereinstellung.

<!-- source: prerequisites 12391490eea0 -->
## Voraussetzungen

- Die Konfiguration ist wie in der [Konfigurationsreferenz](../reference/configuration.md) beschrieben angegeben (`my config check`).
- Das Schema ist auf die Datenbank angewendet (`my schema apply`; siehe [Datenbank](database.md)).

<!-- source: keys ae25d36239c2 -->
## Schlüssel

<!-- source: settings-one-key-one-value 4f827bf97480 -->
### Einstellungen (ein Schlüssel, ein Wert)

| Schlüssel | Typ | Standard | Beschreibung |
| --- | --- | --- | --- |
| `registration_mode` | `open` / `approval` / `invite` / `closed` | `approval` | Der Registrierungsmodus ([Registrierung](registration.md)). Solange eine Einstellung des Dienstes angekündigt ist und nachdem er geschlossen wurde sowie auf einem [Ein-Personen-Server](#one-person-server) ist der wirksame Wert `closed`. Der gespeicherte Wert ändert sich nicht, und das Zurückziehen der Ankündigung oder das Ausschalten von `single_user` stellt ihn wieder her. `GET /instance` und NodeInfo zeigen den wirksamen Wert |
| `max_status_length` | Ganzzahl 1-100000 | `500` | Die maximale Länge eines Beitrags in Zeichen |
| `default_locale` | BCP-47-Sprachtag | `en` | Die Standard-Locale. Sie ist die anfängliche UI-Locale neuer Nutzerr und der Fallback, wenn kein Katalog zur Präferenz eines Nutzers passt |
| `local_timeline_visibility` | `off` / `signed_in` / `everyone` | `everyone` | Wer die lokale Timeline sehen kann (`GET /api/v1/timelines/local`): niemand, angemeldete Nutzer oder alle. `off` antwortet mit 404 und `signed_in` antwortet Besuchenden, die nicht angemeldet sind, mit 401; in beiden Fällen blendet die Navigation sie aus. Solange der Wert nicht `everyone` ist, zeigt die Landingpage keine Beispielbeiträge. Auf einem [Ein-Personen-Server](#one-person-server) gilt sie als `off` |
| `federated_timeline_visibility` | `off` / `signed_in` / `everyone` | `signed_in` | Wer die föderierte Timeline sehen kann (`GET /api/v1/timelines/federated`). Die Werte bedeuten dasselbe wie bei der lokalen Timeline, und auf einem Ein-Personen-Server gilt sie ebenfalls als `off` |
| `preview_image` | Medien-ID (leer = nicht gesetzt) | leer | Das Link-Vorschaubild (`og:image`) für Beiträge und Profile ohne Bild. Ist es nicht gesetzt, wird das mitgelieferte Standardbild verwendet ([Link-Vorschauen](link-previews.md)) |
| `contact_account` | Benutzername eines lokalen Kontos (leer = nicht gesetzt) | leer | Das Konto, über das die Betreiber kontaktiert werden. Siehe [Kontakt und Vorschaubild](#contact-and-thumbnail) |
| `contact_email` | E-Mail-Adresse (leer = nicht gesetzt) | leer | Die E-Mail-Adresse, über die die Betreiber kontaktiert werden. Sie ist auch das `Reply-To` und die Signatur der E-Mails, die der Server sendet |
| `thumbnail` | Medien-ID (leer = nicht gesetzt) | leer | Ein Bild, das der Server repräsentiert (Vorschaubild) |
| `remote_media_policy` | `none` / `avatars` / `all` | `none` | Ob Kopien entfernter Medien aufbewahrt werden. `none`: Der Media-Proxy leitet jede Anfrage weiter. `avatars`: Avatare, Header und eigene Emojis werden für `remote_media_cache_ttl_seconds` im Blob-Speicher zwischengespeichert. `all`: alle entfernten Medien. Die Gesamtgröße des Caches wird durch die Konfiguration `remote_media_cache_max_bytes` begrenzt |
| `remote_media_cache_ttl_seconds` | Ganzzahl 60-31536000 (Sekunden) | `86400` | Wie lange abgerufene entfernte Medien aufbewahrt werden dürfen. Es ist auch das `Cache-Control: max-age` der Antworten des Media-Proxys, CDNs und Browser cachen also so lange |
| `remote_media_cache_max_bytes` | Ganzzahl ab 1 (Bytes) | `1073741824` | Das Größenlimit der Kopien entfernter Medien, die der Media-Proxy aufbewahrt. Die stündliche Bereinigung löscht die abgelaufenen Kopien und dann die am längsten nicht abgerufenen über diesem Limit |
| `authorized_fetch` | `true` / `false` | `false` | Authorized Fetch (sicherer Modus). Bei `true` wird ein ActivityPub-GET ohne überprüfbare HTTP-Signatur für Objekte und Sammlungen mit `401` beantwortet und für den Actor eines Nutzers mit einem minimalen Dokument, das den Schlüssel enthält. Der Server-Actor gibt immer sein gesamtes Dokument zurück, damit jeder Server den Schlüssel zum Prüfen einer Signatur abrufen kann |
| `federation_mode` | `off` / `inbound` / `outbound` / `both` | `both` | Die Richtung der Föderation. `off`: keine Föderation. `inbound`: Du folgst entfernten Konten und liest sie, aber lokale Beiträge, Profile und Reaktionen werden nicht ausgesendet, und Follows von außen werden abgelehnt (`Reject`). `outbound`: Du nimmst Follows von außen an und stellst lokale Beiträge zu, aber entfernte Beiträge und Reaktionen werden nicht aufgenommen. `both`: senden und empfangen. Eine Änderung wirkt sofort und braucht die Berechtigung `manage_settings` |
| `dynamic_client_registration` | `true` / `false` | `true` | Dynamische Client-Registrierung (RFC 7591). Solange `true`, akzeptiert `POST /oauth/register` die Registrierung öffentlicher Clients ohne Authentifizierung. Bei `false` weist der Endpunkt jede Anfrage mit `403` (`access_denied`) ab; bereits registrierte Clients funktionieren weiter. Ein periodischer Job löscht dynamisch registrierte Clients, die nicht verwendet werden, gemäß der Konfiguration `dynamic_client_unredeemed_retention` und `dynamic_client_idle_retention`. Eine Änderung wirkt sofort und braucht `manage_settings` |
| `domain_actions_visibility` | `off` / `signed_in` / `everyone` | `off` | Wer die Liste der Domains mit ergriffenen Maßnahmen sehen kann (`GET /api/v1/instance/domain_actions`): niemand, angemeldete Nutzer oder alle ([Moderation](moderation.md#publishing-the-domains-you-act-on)) |
| `domain_actions_rationale_visibility` | `off` / `signed_in` / `everyone` | `off` | Wer die öffentlichen Kommentare dieser Liste sehen kann. Wer die Liste nicht sehen kann, sieht sie nie |
| `appeal_window_days` | Ganzzahl 20-365 (Tage) | `180` | Wie viele Tage nach einer Maßnahme die betroffene Person Einspruch einlegen kann ([Moderation](moderation.md#appeals)). Eine Änderung ändert auch die Frist bereits ergriffener Maßnahmen |
| `session_origins_retention_days` | Ganzzahl 90-3650 (Tage) | `180` | Wie viele Tage nach einer Anmeldung die IP-Adresse und der User-Agent der Anmeldung aufbewahrt werden ([Moderation](moderation.md#record-retention-and-holds)) |
| `registration_origins_retention_days` | Ganzzahl 90-3650 (Tage) | `180` | Wie viele Tage nach der Registrierung die IP-Adresse der Registrierung aufbewahrt wird |
| `reports_retention_days` | Ganzzahl 180-3650 (Tage) | `365` | Wie viele Tage nach der letzten Erledigung eine Meldung aufbewahrt wird. Eine Meldung, auf die eine Maßnahme verweist, wird so lange aufbewahrt wie die Maßnahme |
| `moderation_actions_retention_days` | Ganzzahl 180-3650 (Tage) | `365` | Wie viele Tage nach dem Ende einer Maßnahme (der Aufhebung, dem Ablauf oder der Prüfung eines Einspruchs, je nachdem, was später ist) Kontomaßnahmen und Einsprüche aufbewahrt werden |
| `audit_log_retention_days` | Ganzzahl 365-3650 (Tage) | `730` | Wie viele Tage nach dem Erfassen eine Audit-Log-Zeile aufbewahrt wird |
| `remote_status_retention_days` | Ganzzahl 1-3650 (Tage) | `90` | Wie viele Tage ein entfernter Beitrag nach seiner Veröffentlichung aufbewahrt wird, wenn auf diesem Server nichts auf ihn verweist (eine lokale Antwort, ein Boost, ein Favorit, ein Lesezeichen, eine Benachrichtigung oder ein Thread mit einem lokalen Beitrag) |
| `remote_status_max_retention_days` | Ganzzahl 0-3650 (Tage) | `0` | Die höchste Zahl von Tagen, die ein entfernter Beitrag nach seiner Veröffentlichung aufbewahrt wird, auch wenn dieser Server auf ihn verweist. Lokale Antworten, Boosts, Favoriten und Lesezeichen bleiben bestehen und zeigen ihn als gelöscht an. `0` setzt kein Limit |
| `relay_status_retention_days` | Ganzzahl 1-3650 (Tage) | `7` | Wie viele Tage ein entfernter Beitrag aufbewahrt wird, den nur ein Relay hereingebracht hat, wenn auf diesem Server nichts auf ihn verweist und kein lokales Konto seinem Autor folgt |
| `oauth_client_rate_limits` | durch Kommas getrennte Paare `client_id=requests` | leer | Obergrenzen für die Client-API-Anfragen von OAuth-Clients, um einen Client zu bremsen, der den Server überlastet: die Anfragen, die alle seine Nutzer zusammen in 5 Minuten stellen dürfen. Eine Änderung braucht bis zu einer Minute, um zu wirken |
| `search_indexing` | `allow` / `deny` | `deny` | Ob Suchmaschinen den Server indexieren dürfen. Auch bei `allow` werden nur die Profile von Konten mit `indexable` und `public`-Beiträge indexiert ([Suchmaschinen und KI-Crawler](crawlers.md)) |
| `ai_usage` | `allow` / `deny_training` / `deny` | `deny` | Die Erklärung an KI-Dienste. `deny_training` untersagt Training; `deny` untersagt Training und die Nutzung durch KI (KI-Suche, Assistenten, Agenten) |
| `indexable_default` | `true` / `false` | `false` | Der Standard von `indexable` für neue Konten. Eine Änderung ändert bestehende Konten nicht |
| `auto_delete_default_enabled` | `true` / `false` | `false` | Ob neue Konten mit eingeschalteter automatischer Löschung ihrer alten Beiträge beginnen. Zusammen mit den anderen `auto_delete_default_*`-Schlüsseln ist es die Richtlinie, die ein neues Konto erhält ([automatische Löschung alter Beiträge](#automatic-deletion-of-old-posts)) |
| `auto_delete_default_min_age_days` | Ganzzahl 1-3650 (Tage) | `30` | Wie viele Tage nach ihrer Veröffentlichung die Richtlinie eines neuen Kontos einen Beitrag löscht |
| `auto_delete_default_keep_pinned`, `auto_delete_default_keep_bookmarked`, `auto_delete_default_keep_favourited` | `true` / `false` | `true` | Ob die Richtlinie eines neuen Kontos die Beiträge behält, die ihr Autor angeheftet, mit einem Lesezeichen versehen oder favorisiert hat |
| `auto_delete_default_keep_media`, `auto_delete_default_keep_polls` | `true` / `false` | `false` | Ob die Richtlinie eines neuen Kontos die Beiträge mit Medien oder einer Umfrage behält |
| `auto_delete_default_keep_public`, `auto_delete_default_keep_local`, `auto_delete_default_keep_unlisted`, `auto_delete_default_keep_private` | `true` / `false` | `false` | Ob die Richtlinie eines neuen Kontos die Beiträge dieser Sichtbarkeit behält |
| `auto_delete_default_keep_direct` | `true` / `false` | `true` | Ob die Richtlinie eines neuen Kontos Direktbeiträge behält |
| `auto_delete_default_keep_min_boosts`, `auto_delete_default_keep_min_favourites` | Ganzzahl 0-1000000 | `0` | Die Richtlinie eines neuen Kontos behält einen Beitrag, der mindestens so oft geboostet oder favorisiert wurde. `0` setzt keine Schwelle |
| `auto_delete_max_age_days` | Ganzzahl 0-3650 (Tage) | `0` | Die höchste Zahl von Tagen nach der Veröffentlichung, die ein lokaler Beitrag aufbewahrt wird ([Höchstdauer](#a-maximum-for-keeping-posts)). `0` setzt keine Höchstdauer |
| `auto_delete_max_allow_pinned` | `true` / `false` | `true` | Ob angeheftete Beiträge unter der Höchstdauer weiter behalten werden dürfen |
| `auto_delete_max_kept_pins` | Ganzzahl 1-5 | `5` | Wie viele Anheftungen Beiträge unter der Höchstdauer behalten, die zuletzt angeheftete zuerst. `5`, die höchste Zahl von Anheftungen eines Kontos, behält jeden angehefteten Beitrag |
| `auto_delete_max_allow_bookmarked`, `auto_delete_max_allow_favourited`, `auto_delete_max_allow_media`, `auto_delete_max_allow_polls` | `true` / `false` | `false` | Ob die Beiträge, die der Autor mit einem Lesezeichen versehen oder favorisiert hat oder die Medien oder eine Umfrage enthalten, unter der Höchstdauer weiter behalten werden dürfen |
| `auto_delete_max_allow_visibilities` | `true` / `false` | `false` | Ob Beiträge unter der Höchstdauer anhand ihrer Sichtbarkeit weiter behalten werden dürfen |
| `auto_delete_max_allow_thresholds` | `true` / `false` | `false` | Ob Beiträge unter der Höchstdauer anhand ihrer Boosts und Favoriten weiter behalten werden dürfen |
| `auto_delete_grace_days` | Ganzzahl 0-365 (Tage) | `30` | Wie viele Tage ein Konto, für das die Höchstdauer neu gilt, seine eigenen Einstellungen behält, nachdem es informiert wurde. Eine Änderung verschiebt die Daten der bereits informierten Konten nicht |
| `search_visibilities` | `public` / `local` / `public,local` | `public` | Welche Beiträge die Volltextsuche eines Servers mit Such-Backend abdeckt. `unlisted`-, `private`- und `direct`-Beiträge werden nie abgedeckt, und nur Konten mit `indexable`. Das Weglassen einer Sichtbarkeit entfernt ihre Beiträge aus dem Index. Das Hinzufügen einer findet nur neue Beiträge dieser Sichtbarkeit, bis du [den Index neu aufbaust](search.md#changing-search_visibilities). Mit `search = none` bewirkt sie nichts |
| `security_contact` | `https:`- oder `mailto:`-URI (leer = nicht gesetzt) | leer | Wohin Sicherheitsprobleme dieses Servers gemeldet werden sollen: wie er konfiguriert und betrieben wird sowie seine Konten und Daten. Wenn gesetzt, liefert der Server `/.well-known/security.txt` (RFC 9116) aus. Das ist dein Kontakt, nicht der der Software: Leite eine Meldung über eine Schwachstelle in der Software selbst an das Projekt weiter, wie seine [Sicherheitsrichtlinie](../../SECURITY.md) es beschreibt |
| `require_media_description` | `true` / `false` | `false` | Ob jede an einen lokalen Beitrag angehängte Mediendatei (Bild, Video oder Audio) eine Beschreibung braucht. Bei `true` wird ein Beitrag oder eine Bearbeitung mit einer Datei, deren Beschreibung fehlt oder nur aus Leerraum besteht, mit 422 abgewiesen. Sie wird als `limits.require_media_description` in `GET /api/v1/instance` veröffentlicht, und der Webclient macht das Beschreibungsfeld zur Pflicht. Sie ist von der Einstellung `require_media_description` pro Nutzer getrennt, die nur vor dem Senden prüft |
| `link_cards` | `true` / `false` | `true` | Ob Karten für Links in Beiträgen angezeigt werden. Bei `true` ruft der Server die verlinkte Seite ab, um eine Karte zu erstellen. Bei `false` hört er mit dem Abrufen auf und blendet auch bereits erstellte Karten aus. Um es nur für einige Domains zu beenden, siehe [Domains für Link-Karten](#link-card-domains) |
| `link_cards_ignore_robots_txt` | `true` / `false` | `false` | Ob die Abrufe von Link-Karten `robots.txt` überspringen. Bei `false` beachtet der Server die `robots.txt` der verlinkten Websites, eine Website, die `my-linkcard` oder `*` untersagt, erhält also keine Karte ([Link-Vorschauen](link-previews.md#cards-of-links-in-posts)). `/about/crawler` sagt verlinkten Websites, welcher Fall gilt. Bei `true` ruft der Server `robots.txt` weder ab noch verwendet er sie, und eine verlinkte Website kann diese Abrufe damit nicht mehr ablehnen; der Server nennt sich weiterhin `my-linkcard`. Schalte sie nur ein, wenn zu viele Karten fehlen. Karten, die erstellt wurden, während sie eingeschaltet war, werden beim nächsten erneuten Abruf geprüft, nachdem sie ausgeschaltet ist. Die Direktiven einer Seite und die Domains für Link-Karten gelten in beiden Fällen |
| `oauth_metadata_document_clients` | `true` / `false` | `true` | Ob Client-ID-Metadata-Document-Clients akzeptiert werden. Bei `true` ruft der Server für eine Autorisierungsanfrage, deren `client_id` eine `https`-URL ist, die Metadaten unter dieser URL ab und behandelt sie als den Client (keine vorherige Registrierung). Der Abruf folgt keinen Weiterleitungen, liest höchstens 5 KB und verwendet die Antwort höchstens 24 Stunden gemäß ihren Cache-Direktiven wieder. Bei `false` hört er mit dem Abrufen auf und weist Autorisierungsanfragen, Token-Austausch und Aktualisierungen dieser Clients ab |
| `finance_visibility` | `private` / `totals` / `breakdown` | `private` | Wie viel der laufenden Kosten (erfasst unter `/admin/finance/costs` und eingestellt unter `/admin/finance/publication`) veröffentlicht wird. `private`: nichts wird veröffentlicht (`GET /api/v1/instance/cost_totals` ist 404). `totals`: nur die Summe pro Zeitraum und Währung. `breakdown`: die Summen und eine Aufschlüsselung nach Anbieter und Kategorie, für alle sichtbar auf den Informationsseiten (`/about` und `/about/costs`) und in der API. `breakdown` verrät, bei welchem Anbieter (zum Beispiel welcher Cloud) du läufst. Auf jeder Stufe bleiben Positionsnamen, Notizen und einzelne Zahlungen privat. Zeiträume bis einschließlich des Zeitraums, der heute (UTC) enthält, werden veröffentlicht |
| `finance_viewer_item_names` | `true` / `false` | `false` | Ob Nutzer mit der Berechtigung `view_finance` auf dem Finanzbildschirm (`/finance`, `GET /api/v1/finance/cost_totals`) die Namen von Kostenpositionen und die Dienstnamen importierter Kosten sehen. Summen und die Aufschlüsselung nach Anbieter und Kategorie werden immer angezeigt, was diese Einstellung auch sagt |
| `finance_viewer_item_memos` | `true` / `false` | `false` | Ob Nutzer mit `view_finance` die Notizen von Kostenpositionen auf dem Finanzbildschirm sehen. Ist die Einstellung für Name oder Notiz `true`, zeigt jeder Zeitraum Positionszeilen (Anbieter, Kategorie und den Betrag dieses Zeitraums, dazu Name und Notiz, wo aktiviert). Personen mit `manage_finance` sehen unabhängig davon alles |
| `finance_viewer_income` | `totals` / `plans` | `totals` | Ob Nutzer mit `view_finance` nur die Einnahmesummen (`totals`) oder auch die Zahl der Abonnenten pro Plan (`plans`) sehen. Der Betrag pro Plan und wer gezahlt hat werden nie angezeigt. Ist `income_visibility` `plans`, werden die veröffentlichten Zahlen angezeigt, was dieser Wert auch sagt. Einnahmen liest man auf dem Finanzbildschirm des Webclients (`/finance`) und in `GET /api/v1/finance/income_totals` |
| `income_visibility` | `private` / `totals` / `plans` | `private` | Wie viel der Unterstützer-Einnahmen (der Betrag nach den Gebühren des Zahlungsanbieters) veröffentlicht wird. Sie wird getrennt von `finance_visibility` gewählt. `private`: nichts wird veröffentlicht (`GET /api/v1/instance/income_totals` ist 404). `totals`: nur die Einnahmensumme pro Zeitraum und Währung. `plans`: außerdem die aktuelle Zahl der Abonnenten pro Plan, für alle sichtbar. Bei einem Plan mit wenigen Abonnenten kann die Zahl Rückschlüsse darauf zulassen, wer abonniert. Der Saldo (Einnahmen minus Kosten) wird nur angezeigt, wenn weder dieser Wert noch `finance_visibility` `private` ist; andernfalls würde der Saldo die Seite verraten, die du verborgen hast. Auf jeder Stufe bleiben einzelne Zahlungen, Abonnenten, der Betrag pro Plan sowie Bruttobeträge und Gebühren privat. Zeiträume bis einschließlich des Zeitraums, der heute (UTC) enthält, werden veröffentlicht |
| `supporter_list_visibility` | `off` / `signed_in` / `everyone` | `off` | Wer die Liste der Unterstützer sehen kann (`GET /api/v1/instance/supporters` und `/about/supporters`). `off`: niemand (die API antwortet mit 404, und `/about` hat keinen Link). `signed_in`: nur bei diesem Server angemeldete Personen (sonst antwortet die API mit 401). `everyone`: alle. Die Liste zeigt nur die Konten und Pläne aktiver Abonnenten, die sich für die Auflistung entschieden haben; Beträge, monatliche oder jährliche Abrechnung, Daten und Zahlen werden nie angezeigt. Die Listenseite und die API-Antwort sind immer `noindex, noarchive`, was `search_indexing` auch sagt |
| `user_app_registration` | `true` / `false` | `true` | Ob angemeldete Nutzer ihre eigenen OAuth-Apps (öffentlich oder vertraulich) über die Client-API registrieren können (`POST /api/v1/oauth_apps`). Bei `false` antwortet das Registrieren mit 403. Das Auflisten, Ändern und Löschen registrierter Apps, das erneute Ausstellen von Secrets, das Ausstellen eines Tokens für dich selbst und die Autorisierung durch andere Nutzer funktionieren weiter |
| `voice_room_starters` | `everyone` / `permitted` | `permitted` | Wer Sprachchat-Räume öffnen kann, solange die Konfiguration `voice` nicht `off` ist ([Konfiguration](../reference/configuration.md#voice)). `everyone`: jeder angemeldete lokale Nutzer, der nicht gesperrt oder eingeschränkt ist. `permitted`: nur die Inhaber der Berechtigung `start_voice_rooms`, die standardmäßig nur die eingebaute Rolle `admin` hat und die eine Rolle eines Unterstützungsplans haben kann |
| `voice_public_summary` | `true` / `false` | `false` | Ob die Seite eines Sprachchat-Raums mit der Sichtbarkeit `local` Besuchenden, die nicht bei diesem Server angemeldet sind, und entfernten Nutzer sowie in ihrer Link-Vorschau seinen Namen, seine Beschreibung, den Gastgeber und ob er noch offen ist zeigt. Die Teilnehmenden werden nie angezeigt. Bei `false` sehen diese Besuchenden nur, dass der Raum für die lokalen Nutzer dieses Servers ist |
| `public_counts` | `true` / `false` | `true` | Ob die Landingpage (`/`, abgemeldet) und `GET /api/v1/instance/overview` zeigen, wie viele Mitglieder der Server hat, wie viele öffentliche Beiträge seine Nutzer an jedem der letzten 7 Tage verfasst haben und wann der Server gestartet wurde. Es zählen nur öffentliche Beiträge von Nutzer dieses Servers; die Zahlen werden alle paar Minuten aktualisiert. Bei `false` lässt die API sie weg |
| `featured_accounts` | Benutzernamen von bis zu 20 lokalen Konten, getrennt durch Leerzeichen oder Kommas (leer = keine) | leer | Die Konten, die die Vorschläge von Personen zum Folgen zuerst zeigen, in dieser Reihenfolge, vor den Personen dieses Servers, die kürzlich Beiträge verfasst haben. Ein Konto, dessen Inhaber „Mein Konto vorschlagen und über einen Teil des Namens auffindbar machen“ ausgeschaltet hat, wird nicht vorgeschlagen. Siehe [Vorschläge](#suggestions-of-accounts) |
| `login_providers_only` | `true` / `false` | `false` | Ob Registrierung und Anmeldung auf die Anmeldeanbieter beschränkt sind, solange mindestens einer aktiviert ist: Passwörter und Passkeys melden nicht mehr an, und Nutzer ohne verknüpften Anbieter können sich nicht mehr anmelden, sobald ihre Sitzungen enden. Ohne aktivierten Anbieter gilt sie nicht. `my instance set` gibt aus, wie viele aktive Nutzer keine Verknüpfung zu einem aktivierten Anbieter haben. Siehe [Anmeldung auf Anmeldeanbieter beschränken](login-providers.md#limit-sign-in-to-login-providers). |
| `invite_creators` | `everyone` / `permitted` | `permitted` | Wer neben dem Team mit `manage_invites` eigene Einladungen erstellen kann ([Mitgliedereinladungen](registration.md#member-invites)). `everyone`: jeder angemeldete lokale Nutzer, der nicht gesperrt oder eingeschränkt ist. `permitted`: nur die Inhaber der Berechtigung `create_invites`, die standardmäßig nur die eingebaute Rolle `admin` hat und die eine Rolle eines Unterstützungsplans haben kann. Im Modus `approval` genehmigt eine verwendbare Einladung den neuen Nutzer sofort, `everyone` lässt also jeden Nutzer die Prüfung überspringen |
| `member_invite_max_active` | Ganzzahl 0-100 | `5` | Wie viele verwendbare Einladungen (nicht widerrufen, abgelaufen oder aufgebraucht) ein Mitglied gleichzeitig halten darf; bei `0` können Mitglieder keine erstellen. Einladungen des Teams zählen nicht |
| `member_invite_max_uses` | Ganzzahl 1-1000 | `10` | Wie viele Personen sich mit einer Einladung registrieren dürfen, die ein Mitglied erstellt |
| `member_invite_max_lifetime_seconds` | Ganzzahl 3600-31536000 (Sekunden) | `2592000` (30 Tage) | Wie lange eine Einladung, die ein Mitglied erstellt, höchstens gelten darf. Eine Senkung verkürzt bestehende Einladungen nicht |
| `translation_providers` | durch Kommas getrennte Anbieter-IDs | leer | Die serverseitigen Übersetzungsanbieter, die aktiviert sind, der erste wird zuerst versucht. Ist sie leer, wird nichts an einen Anbieter gesendet. Eine ID, für die dieser Server keinen Anbieter hat, bewirkt nichts. Die eingebauten [Übersetzungsanbieter](translation.md) haben die IDs `libretranslate`, `deepl`, `google` und `aws`, und ein [Übersetzer-Plugin](plugins.md#translating-posts) die ID `plugin` |
| `translation_provider_visibilities` | durch Kommas getrennte Paare `provider=visibility\|visibility` | leer | Was jeder Anbieter erhalten darf. Ein nicht genannter Anbieter erhält `public`-, `local`- und `unlisted`-Beiträge; `provider=` ohne etwas dahinter erhält keine. `private` und `direct` gehen nur an einen Anbieter, der mit ihnen genannt ist, die Erlaubnis für einen Anbieter erlaubt also nicht den nächsten |
| `translation_browser` | boolean | `false` | Ob Lesende die in ihren Browser eingebaute Übersetzung verwenden dürfen, die nichts an diesen Server oder an einen Anbieter sendet. Ist zusätzlich ein serverseitiger Anbieter aktiviert, kommt der Anbieter zuerst, und der Browser übersetzt, was der Server ablehnt: einen Beitrag oder eine Sprache, die kein Anbieter annimmt, ein Monatslimit oder einen Anbieter, der ausfällt |
| `translation_user_monthly_characters` | Ganzzahl 0-1000000000000 | `50000` | Wie viele Zeichen die Anfragen eines Nutzers in einem UTC-Monat an Übersetzungsanbieter senden dürfen. Bei `0` wird nichts gesendet. Es gibt keinen Wert für unbegrenzt: Setze eine große Zahl |
| `translation_server_monthly_characters` | Ganzzahl 0-1000000000000 | `500000` | Wie viele Zeichen der gesamte Server in einem UTC-Monat an Übersetzungsanbieter senden darf. Bei `0` wird nichts gesendet. Es gibt keinen Wert für unbegrenzt: Setze eine große Zahl |
| `translation_retention_days` | Ganzzahl 1-3650 (Tage) | `30` | Wie viele Tage eine Übersetzung nach ihrer Erstellung aufbewahrt wird. Das Lesen verlängert dies nicht, und sie verschwindet mit dem Beitrag |
| `translation_reports_to_server` | boolean | `false` | Ob Moderatoren den Text einer Meldung, die gemeldeten Beiträge und den Kommentar des Meldenden, an die serverseitigen Anbieter senden dürfen. Jeder Anbieter erhält weiterhin nur die Sichtbarkeiten, die `translation_provider_visibilities` ihm erlaubt, egal wer fragt: Ein gemeldeter Direktbeitrag geht nur an einen Anbieter, dem `direct` erlaubt ist, und der Kommentar zählt als `private`. Die Übersetzung des Kommentars wird nie aufbewahrt, ebenso wenig die eines gelöschten Beitrags, und die Zeichen zählen nur gegen `translation_server_monthly_characters`. Bei `false` erreicht nichts von einer Meldung einen Anbieter |
| `voice_participants_retention_days` | Ganzzahl 30-3650 (Tage) | `90` | Wie viele Tage nach dem Ende eines Sprachchat-Raums die Aufzeichnungen darüber aufbewahrt werden, wer teilgenommen hat: die Teilnahmen, die Sprechenden und Zuhörenden, die gehobenen Hände, die Einladungen und die Entfernungen. Der Raum selbst (sein Name, seine Beschreibung, die Gastgeber und wie er endete) wird aufbewahrt. Die Aufzeichnungen eines gemeldeten Raums werden so lange aufbewahrt wie die Meldung, und Sperren gelten wie bei den anderen Aufzeichnungen |
| `hook_inbound_status_mode`, `hook_inbound_follow_mode`, `hook_signup_check_mode`, `hook_media_check_mode`, `hook_local_status_mode`, `hook_inbound_actor_mode`, `hook_local_profile_mode` | `off`, `observe`, `enforce` | `observe` | Wie weit jeder [Entscheidungs-Hook](decision-hooks.md#modes) handelt, solange er ein Plugin hat. `off` geht nur über `observe` zu `enforce`. |
| `hook_inbound_status_on_failure`, `hook_inbound_follow_on_failure`, `hook_media_check_on_failure`, `hook_inbound_actor_on_failure` | `pass`, `retry` | `pass` | Was der Hook tut, wenn er durchsetzt und sein Plugin ausfällt ([Details](decision-hooks.md#when-the-plugin-fails)). |
| `hook_inbound_status_unfollowed_direct` | boolean | `false` | Ob `inbound.status` auch die Direktbeiträge von Autoren sendet, denen kein lokales Konto folgt ([Details](decision-hooks.md#what-inboundstatus-reviews)). Die Informationsseite (`/about`) nennt es, solange der Hook in Verwendung ist. |
| `hook_signup_check_on_failure` | `pass`, `require_approval` | `pass` | Was `signup.check` tut, wenn es durchsetzt und sein Plugin ausfällt. |
| `hook_signup_check_reject` | boolean | `false` | Ob ein `reject` von `signup.check` [die Registrierung ablehnt](decision-hooks.md#sign-ups), solange es durchsetzt. Ist sie aus, wird die Antwort nur festgehalten, wie bei `observe`. |
| `hook_local_status_account_age_days` | Ganzzahl 0-3650 (Tage) | `7` | `local.status` [prüft die Beiträge und ihre Bearbeitungen](decision-hooks.md#local-posts) von Konten, die jünger als dies sind; `0` prüft jedes Konto. Die Informationsseite (`/about`) nennt es, solange der Hook in Verwendung ist. |
| `hook_local_profile_account_age_days` | Ganzzahl 0-3650 (Tage) | `7` | `local.profile` [prüft die Profiländerungen](decision-hooks.md#local-profiles) von Konten, die jünger als dies sind; `0` prüft jedes Konto. Die Informationsseite (`/about`) nennt es, solange der Hook in Verwendung ist. |
| `hook_decisions_retention_days` | Ganzzahl 30-3650 (Tage) | `90` | Wie viele Tage die [Entscheidungen der Hooks](decision-hooks.md#the-decisions) aufbewahrt werden. |
| `single_user` | boolean | `false` | Ob der Server ein [Ein-Personen-Server](#one-person-server) ist. Sie lässt sich nur einschalten, solange der Server genau ein lokales Konto hat, und jederzeit ausschalten |

- Werte werden vor dem Speichern normalisiert (`TRUE` wird zu `true`, `0750` zu `750`, `ja_jp` zu `ja-JP`, Medien-IDs werden kleingeschrieben).
  Ein ungültiger Wert wird abgelehnt, und nichts wird geschrieben.
- `preview_image` und `thumbnail` akzeptieren nur die Medien eines Bildes, das existiert (auch eines, das noch verarbeitet wird).
  Bis die Verarbeitung abgeschlossen ist und anstelle eines Bildes, dessen Verarbeitung fehlschlug, fällt `preview_image` auf das Standardbild zurück.
  Sobald ein Medium in irgendeiner Revision gesetzt wurde, löscht die Bereinigung nicht angehängter Uploads es nicht.
- `contact_account` akzeptiert nur den Benutzernamen eines lokalen Nutzers, der nicht gelöscht ist (kleingeschrieben gespeichert). `contact_email`
  akzeptiert nur eine Adresse der Form `alice@example.com`.
- `featured_accounts` akzeptiert nur die Benutzernamen lokaler Nutzerr, die nicht gelöscht sind; ein führendes `@` entfällt, die Namen
  werden kleingeschrieben, ein wiederholter Name bleibt einmal erhalten, und sie werden durch einzelne Leerzeichen getrennt gespeichert.

<!-- source: suggestions-of-accounts b7d362466e33 -->
### Vorschläge von Konten

Angemeldete Nutzer sehen Vorschläge von Personen, denen sie folgen können (auf der leeren Home-Timeline nach der Registrierung und mit
`GET /api/v1/accounts/suggestions`). Die Vorschläge sind die Konten aus `featured_accounts` in ihrer Reihenfolge, dann die
Konten dieses Servers, die in den letzten 30 Tagen einen öffentlichen oder lokalen Beitrag veröffentlicht haben, das zuletzt aktive zuerst. Das
eigene Konto des Nutzers, die Konten, denen der Nutzer folgt, die er sperrt oder stummschaltet oder von denen er gesperrt wird, sowie gesperrte oder
deaktivierte Konten bleiben außen vor.

Neue Konten werden vorgeschlagen und lassen sich in der Kontosuche über einen Teil ihres Namens finden. Ein Nutzer, der in den Profileinstellungen „Mein
Konto vorschlagen und über einen Teil des Namens auffindbar machen“ ausschaltet, verlässt beides: Die Suche findet das Konto dann nur über seinen
vollständigen Benutzernamen oder für die Personen, die ihm folgen.

<!-- source: automatic-deletion-of-old-posts f0cf962c9de0 -->
### Automatische Löschung alter Beiträge

Nutzer können ihre eigenen Beiträge einige Tage nach der Veröffentlichung automatisch löschen lassen (Einstellungen, „Alte Beiträge löschen“).
Die Richtlinie jedes Nutzers ist seine eigene: ob sie eingeschaltet ist, nach wie vielen Tagen und welche Beiträge sie behält (angeheftete, mit Lesezeichen versehene und
favorisierte Beiträge, Beiträge mit Medien oder einer Umfrage, Beiträge ausgewählter Sichtbarkeiten und Beiträge mit mindestens einer bestimmten Zahl von Boosts oder
Favoriten). Einen Beitrag auf diese Weise zu löschen ist dasselbe, als würde sein Autor ihn löschen: Die Server, an die er ging, erhalten ein `Delete`.
Auf diese Weise werden nur die Beiträge lokaler Konten gelöscht, nie Boosts.

Die `auto_delete_default_*`-Schlüssel sind die Richtlinie, mit der jedes neue lokale Konto beginnt, auf welchem Weg es auch erstellt wird
(das Registrierungsformular, ein externer Anmeldeanbieter, eine Einladung oder `my admin`). Das Konto erhält sie bei seiner Erstellung und
besitzt sie dann: Eine spätere Änderung der Schlüssel ändert weder die bestehenden Konten noch diejenigen, die einen früheren Wert erhalten haben.
Der Einstellungsbildschirm zeigt die Werte als die eigenen des Nutzers. Sind die Schlüssel nicht gesetzt, beginnt ein neues Konto mit ausgeschalteter automatischer
Löschung, nach 30 Tagen, und behält angeheftete, mit Lesezeichen versehene und favorisierte Beiträge sowie Direktbeiträge. Der Standard wird Personen außerhalb des Servers
nicht angezeigt. Ist eine Höchstdauer gesetzt, erhält ein neues Konto den Standard auf die Höchstdauer zugeschnitten, wie
unten beschrieben, es beginnt also innerhalb davon.

<!-- source: a-maximum-for-keeping-posts b1d83e01feda -->
#### Eine Höchstdauer für das Aufbewahren von Beiträgen

Bei `auto_delete_max_age_days` über `0` wird kein lokaler Beitrag länger als so viele Tage nach seiner Veröffentlichung aufbewahrt.
Für jedes lokale Konto ohne die Berechtigung `keep_posts_longer`:

- Die automatische Löschung ist immer eingeschaltet: Ein Nutzer kann sie nicht ausschalten und nicht mehr Tage wählen als die Höchstdauer (weniger ist
  in Ordnung). Der Einstellungsbildschirm und `PUT /api/v1/preferences/auto_delete` weisen beides ab.
- Nur die Ausnahmen, die die `auto_delete_max_allow_*`-Schlüssel erlauben, behalten Beiträge. Standardmäßig tun das nur angeheftete Beiträge, alle
  von ihnen; bei `auto_delete_max_kept_pins` unter 5 behalten nur so viele der neuesten Anheftungen ihre Beiträge.
- Nutzer, die die automatische Löschung ausgeschaltet oder nie eingestellt haben, unterliegen ebenfalls der Höchstdauer.

Das Einschalten der Höchstdauer, ihre Senkung oder das Ändern der Ausnahmen schreibt die Einstellungen von niemandem um. Stattdessen findet der Job
`content.auto_delete_statuses` jede Stunde die Konten, deren eigene Einstellungen Beiträge behalten, die die Höchstdauer nicht behält (weil
die Höchstdauer eingeschaltet oder gesenkt wurde oder das Konto `keep_posts_longer` verloren hat). Er informiert jedes davon einmal, mit einer
Benachrichtigung und einer E-Mail, die standardmäßig eingeschaltet sind, und sagt, ab wann die Höchstdauer gilt: `auto_delete_grace_days` nach
der Mitteilung. Bis dahin bleiben die eigenen Einstellungen des Kontos in Kraft; danach richten sich seine Beiträge nach der Höchstdauer. Ein Konto,
das wieder innerhalb der Höchstdauer liegt (sein Nutzer ändert die Einstellungen, es erhält `keep_posts_longer`, oder die Höchstdauer entfällt oder
steigt), wird nicht mehr festgehalten, und eine spätere Änderung informiert es erneut mit einer neuen Karenzzeit.

Das Einschalten der Höchstdauer informiert innerhalb der Stunde jedes Konto, das betroffen ist. Löschungen behalten ihr Tempo: höchstens 100 Beiträge
eines Kontos pro Stunde, jeder als `Delete` an die Server gesendet, an die der Beitrag ging, ein Konto mit vielen alten Beiträgen
braucht also Stunden oder Tage, um leer zu werden. Ein Nutzer, der in der Zwischenzeit seine Einstellungen ändert, stoppt den Rest.

`keep_posts_longer` ist eine Mitgliederberechtigung: Gib sie einer Rolle, damit ihre Inhaber ihre Beiträge so lange behalten können, wie sie wollen,
und die automatische Löschung ausschalten können. Die eingebaute Rolle `admin` hat sie. Ein [Unterstützungsplan](supporters.md) kann sie tragen, sodass
das längere Aufbewahren von Beiträgen ein Vorteil des Plans sein kann; wenn das Abo endet, gilt die Höchstdauer nach der Karenzzeit
wieder.

Die Höchstdauer ist öffentlich: `GET /api/v1/instance` gibt sie in `auto_delete_limit` zurück, und die Info-Seite zeigt sie, damit Menschen
sie kennen, bevor sie sich registrieren. Änderungen dieser Schlüssel brauchen `manage_settings` und werden wie jede
andere Einstellung im Audit-Log festgehalten. Im Webclient stehen sie auf der Seite „Beitragslöschung“ der „Servereinstellungen“, die nachfragt, bevor sie eine
Höchstdauer speichert, die eingeschaltet oder verschärft wird.

<!-- source: texts-one-key-one-value c639fc7dfe23 -->
### Texte (ein Schlüssel, ein Wert)

| Schlüssel | Limit | Beschreibung |
| --- | --- | --- |
| `name` | 100 Zeichen, eine Zeile (Leerraum allein ist nicht erlaubt) | Der Servername (Anzeigename). Siehe [Name und Domain](#name-and-domain) |
| `short_description` | 500 Zeichen | Eine Kurzbeschreibung |
| `description` | 20000 Zeichen | Die Beschreibung |
| `terms` | 100000 Zeichen | Die Nutzungsbedingungen (Markdown) |
| `privacy_policy` | 100000 Zeichen | Die Datenschutzerklärung (Markdown) |
| `commerce_disclosure` | 100000 Zeichen | Die Pflichtangaben des Verkäufers für bezahlte Pläne, etwa die Mitteilung, die japanisches Recht verlangt (Markdown; [Unterstützer](supporters.md)) |
| `preview_image_alt` | 1500 Zeichen | Der Alternativtext des Bildes `preview_image` (`og:image:alt`). Ist er nicht gesetzt, wird „Image of {instance name}“ verwendet |
| `thumbnail_alt` | 1500 Zeichen | Der Alternativtext des Bildes `thumbnail` |

- Jeder Text hat einen Wert. Ein Administrator schreibt ihn einmal in einer beliebigen Sprache, und Lesende jeder Locale sehen
  denselben Wert; Lesende anderer Sprachen sind auf maschinelle Übersetzung oder Ähnliches angewiesen. Die Beschriftungen und Beschreibungen der Admin-Felder
  werden wie jeder andere Bildschirmtext in die Sprache des Betrachters übersetzt; nur der Wert, den ein Administrator schreibt, nicht.
- Der Server kennt die Sprache eines Textes nicht, daher geben der Webclient und die vom Server gerenderten Bildschirme Texte mit
  `dir="auto"` und `lang=""` (der HTML-Wert für eine unbekannte Sprache) aus, anstatt die Sprache des Bildschirms zu erben.
- Das Setzen eines leeren Textes löscht ihn.
- Neben der CLI lassen sich Texte auf den Admin-Bildschirmen des Webclients ändern (Name, Kurzbeschreibung und Beschreibung auf
  der Seite „Allgemein“ der „Servereinstellungen“, `/admin/settings`; die Bedingungen auf der ersten Seite von „Bedingungen und Richtlinien“,
  `/admin/documents`, die Datenschutzerklärung unter `/admin/documents/privacy-policy` und die Pflichtangaben für den Handel und Ähnliches unter
  `/admin/documents/commerce-disclosure`) und mit der Client-API (`GET /api/v1/admin/instance_texts`, `PUT /api/v1/admin/instance_texts/{key}`).
  Beides braucht die Rollenberechtigung `manage_settings`, und Änderungen werden im Audit-Log festgehalten (`text_changed`).

<!-- source: terms-and-privacy-policy 82e7bc325cf1 -->
### Bedingungen und Datenschutzerklärung

- Schreibe `terms`, `privacy_policy` und `commerce_disclosure` in Markdown (CommonMark plus die Tabellen, das Durchgestrichene
  und die Autolinks von GitHub Flavored Markdown). Ein Zeilenumbruch innerhalb eines Absatzes bleibt ein Zeilenumbruch, und `#`-Überschriften beginnen eine Ebene unter dem
  Seitentitel (`h2`).
- Der Server wandelt sie in HTML um und bereinigt es (anonymes `GET /api/v1/instance/documents/{kind}`), und die Webclient-Seiten
  `/about/terms`, `/about/privacy` und `/about/commerce` zeigen sie an. Rohes HTML, Bilder und andere Links als `http`,
  `https` und `mailto` werden entfernt. Die Seiten brauchen keine Anmeldung (es sind Webclient-Seiten, sie brauchen also JavaScript).
  `/about/commerce` ist die URL, die dem Zahlungsanbieter gegeben wird.
- Ein Dokument, das du schreibst, wird vom Registrierungsbildschirm, von der Fußzeile jedes vom Server gerenderten Bildschirms und vom Webclient verlinkt (die dritte Spalte, das Menü, die Einstellungen und der Anmeldebildschirm).
  Ein Dokument, das nicht geschrieben ist, wird nicht verlinkt, und seine API antwortet mit 404.
- Solange die Nutzungsbedingungen oder die Datenschutzerklärung geschrieben sind, listet der Registrierungsbildschirm sie vor der Schaltfläche zum Erstellen
  mit einem Pflichtkästchen auf, „Ich habe die obigen Dokumente gelesen und stimme ihnen zu“. Der Server weist eine Registrierung ohne dieses ab,
  auch eine über einen Anmeldeanbieter. Ohne eines der beiden Dokumente verlangt der Bildschirm keine Zustimmung.

<!-- source: name-and-domain 1e14aa3e6c58 -->
### Name und Domain

Ein Server hat getrennt eine Domain und einen Namen.

- Die **Domain** ist die Konfiguration `host` (und `account_domain`, die Domain der Handles). Sie lässt sich nicht ändern, solange die
  Server läuft. Handles (`@alice@example.com`), URLs, ActivityPub-IDs und `domain` in `GET /api/v1/instance` bleiben
  die Domain, auch wenn es einen Namen gibt.
- Der **Name** ist der Text `name`. Er erscheint überall dort, wo der Server Menschen gegenüber benannt wird: die Seitenleiste und „Über diesen Server“ des
  Webclients, der Titel des Browser-Tabs, die vom Server gerenderten Bildschirme für Anmeldung, Registrierung, E-Mail-Bestätigung, Kontostatus und Autorisierungszustimmung,
  E-Mails (Bestätigung der E-Mail-Adresse und Benachrichtigungen an Moderatoren), `og:site_name` von Link-Vorschauen,
  `name` / `short_name` des Web App Manifests, `metadata.nodeName` von NodeInfo und `name` in `GET /api/v1/instance`.
- Der Name ist in jeder Locale gleich. Gibt es keinen Namen, wird stattdessen die Domain (`host`) angezeigt.
- Eine Änderung wirkt ab der nächsten Anfrage, ohne Neustart. Der Tab-Titel und die Seitenleiste eines geöffneten Webclients werden aktualisiert, wenn
  du auf dem Admin-Bildschirm speicherst.

<!-- source: contact-and-thumbnail 71513ab82d51 -->
### Kontakt und Vorschaubild

Der Kontakt der Betreiber (`contact_account`, `contact_email`) und das Bild, das der Server repräsentiert (`thumbnail`
und der Text `thumbnail_alt`), erscheinen an den folgenden Stellen. Was nicht gesetzt ist, erscheint nicht.

- „Über diesen Server“ des Webclients (`/about`). Das Vorschaubild und das Kontaktkonto erscheinen außerdem auf dem ersten Bildschirm (`/`)
  für Personen, die nicht angemeldet sind.
- `contact` (`account` und `email`) und `thumbnail` (`url`, `width`, `height`, `alt`) in `GET /api/v1/instance`.
- `metadata` in NodeInfo: `maintainer` nach der Misskey-Konvention (`name` ist der Anzeigename des Kontaktkontos oder sein
  Benutzername, wenn es keinen hat; `email` ist `contact_email`), `staffAccounts` nach der Konvention von Pleroma und Akkoma (die Actor-ID des
  Kontaktkontos), `thumbnail` (`url` und `alt`) und `nodeDescription` aus der Kurzbeschreibung `short_description`.

Ist das Kontaktkonto gesperrt oder gelöscht, wird das Konto nicht angezeigt, sondern nur die E-Mail-Adresse.
Ändere sie auf der Seite „Allgemein“ des Admin-Bildschirms des Webclients („Servereinstellungen“, `/admin/settings`) oder mit der CLI oder der Client-API.

<!-- source: one-person-server 6ba524c58a4f -->
### Ein-Personen-Server

Ein Server, der nur seinem Betreiber gehört, das eigene Zuhause des Betreibers im Fediverse, ist ein Ein-Personen-Server: Schalte `single_user` ein. Im Webclient
schaltet ein Administrator ihn mit **Diesen Server zu einem Server für eine Person machen** auf der Seite **Registrierung** der **Servereinstellungen** ein und aus,
die sagt, warum der Server ihn abgelehnt hat, wenn er mehr als ein lokales Konto hat.

```sh
my instance set single_user true
my instance set single_user false   # back to a community server
```

- Er lässt sich nur einschalten, solange genau ein lokales Konto zählt. Jedes lokale Konto, das nicht gelöscht ist, zählt,
  auch solche, die auf Genehmigung warten, deaktiviert, zur Löschung vorgesehen, gesperrt oder eingeschränkt sind, da jedes als
  zweite Person zurückkommen könnte. Ein Konto, dessen Registrierung abgelehnt wurde, zählt nicht. Bei jeder anderen Zahl
  schlägt `my instance set` mit der Zahl fehl, und die Admin-API antwortet mit `422` und dem Typ `/api/v1/problems/single-user-requires-one-account`.
  Der Server-Actor des Servers ist kein Konto und zählt nie.
- Er lässt sich jederzeit ausschalten. Das Ein- oder Ausschalten löscht nichts und ändert den gespeicherten Wert keiner anderen Einstellung:
  Was er zurückhält, gilt wieder wie zuvor, sobald er aus ist. Wie jede Einstellung wird die Änderung im Audit-Log festgehalten.

Solange er eingeschaltet ist:

- **Die Registrierung ist geschlossen.** `registration_mode` gilt als `closed`, was es auch speichert, die Registrierungsseite, die Registrierung über einen
  Anmeldeanbieter und `my admin create` erstellen also kein Konto. Die Anmeldung mit einem Anmeldeanbieter funktioniert weiterhin.
- **Nur die Home-Timeline.** `local_timeline_visibility` und `federated_timeline_visibility` gelten als `off`. Hashtag- und Listen-Timelines
  bleiben.
- **Was eine Community braucht, ist verborgen.** Die lokale und die föderierte Timeline und ihre Streams, die Kontovorschläge,
  die Einladungen von Nutzer und von Administratoren, die Prüfung von Registrierungen und die Registrierungssperren antworten mit `404` und dem Typ
  `/api/v1/problems/single-user-mode`.
- **Keine neuen lokalen Beiträge.** Auf einem Server für eine Person erreicht ein Beitrag für „Dieser Server“ nur die Follower, daher kann ein neuer
  Beitrag nicht lokal sein: Die Client-API antwortet mit `422` und lässt es aus dem weg, was ein Konto veröffentlichen darf. Eine Standard-Sichtbarkeit
  von „Dieser Server“ gilt als „Follower“ und lässt sich nicht speichern, und ein Sprachchat-Raum für die Personen dieses Servers kündigt
  sich in einem Beitrag für die Follower an. Beiträge, die bereits lokal sind, bleiben lokal.
- **Der Server sagt, was er ist.** `single_user` in `GET /api/v1/instance` ist `true`, mit dem wirksamen Registrierungsmodus
  und den Timelines sowie dem Konto des Betreibers als `single_user_account`, und NodeInfo antwortet mit `openRegistrations: false` und
  `metadata.singleUser: true`.
- **Die Startseite ist das Profil des Betreibers.** Personen, die nicht angemeldet sind und die Adresse des Servers öffnen, sehen das Profil
  seines einen Kontos, mit dem Weg zur Anmeldung und Links zur Info-Seite, den Nutzungsbedingungen und der Datenschutzerklärung,
  die der Server veröffentlicht. Der Webclient blendet aus, was der Server nicht anbietet, aus Navigation, Menüs und
  Administrationsbildschirmen: die lokale und die föderierte Timeline, die Registrierung, vorgeschlagene Konten, Einladungen, die Prüfung von
  Registrierungen, die Registrierungssperren, die Sichtbarkeit „Dieser Server“ und die Einstellungen für Registrierung, Einladungen, die öffentlichen
  Timelines, die Landingpage und vorgeschlagene Konten.

Föderation, Moderation, die Volltextsuche, das Unterstützerprogramm, die Dokumente und die Admin-Bildschirme funktionieren wie auf jedem
Server.

<!-- source: usage 68ad8384438f -->
## Verwendung

```sh
my instance list                                      # all settings (and whether each is the default) and texts
my instance get registration_mode
my instance set registration_mode open
my instance set name "Example Social"                 # the same name in every locale
my instance set description - < description.txt       # the value "-" reads standard input
my instance get name
my instance set name ""                               # delete the name
my instance history registration_mode                 # revision history (newest first, -limit sets the count)
my instance history description
my instance set -owner admin preview_image ./preview.png   # upload the image as media of admin and use it as the preview image
my instance set preview_image 0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f   # give the ID of media that is already uploaded
my instance set preview_image_alt "A blue bird mascot"
my instance set preview_image ""                      # back to the bundled default image
my instance set contact_account admin                 # the contact account
my instance set contact_email admin@example.com
my instance set -owner admin thumbnail ./thumbnail.png  # the thumbnail
my instance set thumbnail_alt "A town by the sea"
```

- Schreibe Flags vor den Schlüssel und den Wert.
- Mit `-owner <username>` wird der Wert von `preview_image` oder `thumbnail` als Pfad einer Bilddatei gelesen und als
  Medium dieses lokalen Kontos hochgeladen. Der Worker verarbeitet das Bild, bevor es verwendet wird. Empfohlen wird ein Querformatbild von 1200x630.
- Eine Änderung erfasst eine Revision (`instance_*_revisions`) und aktualisiert den aktuellen Wert (`instance_settings` / `instance_texts`)
  in einer Transaktion. Eine Änderung über die CLI wird ohne Akteur erfasst (`BY` ist `cli` im Verlauf).
- Das erneute Setzen desselben Wertes fügt trotzdem eine Revision hinzu.
- Exit-Codes: 0 für Erfolg, 1 für einen abgelehnten Wert oder einen Datenbankfehler (nichts ändert sich), 2 für falsche Argumente.

<!-- source: changing-the-direction-of-federation 9b9b63689c84 -->
### Die Richtung der Föderation ändern

- Das Einengen behält Follow-Beziehungen, importierte Beiträge und Zustellungsaufzeichnungen und sendet kein `Undo` oder `Reject`. Der Fluss
  stoppt nur, und die Rückkehr zu `both` (oder einem anderen Modus) setzt ihn fort, wie er war.
- Beiträge und Reaktionen, die während des Einengens entstanden sind, werden nach dem erneuten Erweitern nicht nachträglich zugestellt.
- Auf der entfernten Seite erhalten die Follower lokaler Nutzerr nichts, solange der Modus `inbound` ist, und die Follows lokaler Nutzerr werden
  nicht aktualisiert, solange er `outbound` oder `off` ist.
- Löschungen (von Beiträgen oder Konten), die im Modus `off` erfolgen, werden nicht ausgesendet. In `inbound` und `outbound` geht der Widerruf
  dessen, was bereits ausgesendet wurde (`Delete` und Ähnliches), an die ursprünglichen Empfänger und die Follower zu diesem Zeitpunkt.
- Änderungen werden in `instance_setting_revisions` und im Audit-Log festgehalten (`target_kind = setting`).

Um auch die Beziehungen zu trennen, verwende die CLI `my admin federation sever`.

```sh
my admin federation sever -direction in     # end follows of local users by remote accounts
my admin federation sever -direction out    # end follows of remote accounts by local users
my admin federation sever -direction both
```

- Es beendet die ausstehenden und aktiven Follows zwischen lokalen und entfernten Konten als Beendigungen durch Moderation
  (`follow_terminations`, `reason = moderation`). Ein Moduswechsel ruft es nie automatisch auf.
- `Reject` (`in`) oder `Undo` (`out`) wird nur dann an die entfernte Seite gesendet, wenn der aktuelle Modus diese Richtung sendet. In einem Modus, der
  das nicht tut (etwa `off`), endet der Follow nur auf der lokalen Seite, und die entfernte Seite wird nicht informiert.
- Es ist gefahrlos wiederholbar. Es gibt die Zahl der beendeten Follows auf der Standardausgabe aus.

<!-- source: admin-screens-and-the-client-api 6a747f50af97 -->
## Admin-Bildschirme und die Client-API

Jede Einstellung, die die CLI ändern kann, lässt sich auch auf den Admin-Bildschirmen des Webclients und mit der Client-API ändern. Beides braucht
die Rollenberechtigung `manage_settings` (nur die eingebaute Rolle `admin` hat sie). Wie bei der CLI wird eine Änderung als Revision aufbewahrt
und pro Schlüssel im Audit-Log festgehalten (`setting_changed`, das Ziel ist die Einstellung und der Akteur dieser Nutzer). Das
Audit-Log findest du auf dem Moderationsbildschirm („Audit-Log“, `/moderation/audit-log`).

- **Webclient** („Servereinstellungen“, `/admin/settings`): Der Bildschirm hat pro Thema eine Seite, verlinkt unter seiner Überschrift. Jede
  Seite hat ein Formular und eine Speichern-Schaltfläche, und das Speichern sendet nur, was du auf dieser Seite geändert hast; Einstellungen auf den anderen Seiten
  bleiben, wie sie sind.

  | Seite | Bereiche und Einstellungen |
  | --- | --- |
  | Allgemein (`/admin/settings`) | Name, Beschreibungen (`name`, `short_description`, `description`); Kontakt und Vorschaubild (`contact_account`, `contact_email`, `thumbnail` und der Text `thumbnail_alt`) |
  | Symbol und Logo (`/admin/settings/branding`) | Das Symbol, das Logo und das Logo für dunkle Hintergründe, jeweils mit einer eigenen Schaltfläche gespeichert ([Logo und Symbol](#logo-and-icon)) |
  | Registrierung (`/admin/settings/registration`) | Registrierung (`registration_mode`, `login_providers_only`); Mitgliedereinladungen (`invite_creators`, `member_invite_max_active`, `member_invite_max_uses`, `member_invite_max_lifetime_seconds`); Apps (`user_app_registration`, `oauth_metadata_document_clients`, `dynamic_client_registration`) |
  | Inhalt (`/admin/settings/content`) | Sprache (`default_locale`, auswählbar aus den Sprachen, die die App hat); Timelines (`local_timeline_visibility`, `federated_timeline_visibility`); Beiträge (`require_media_description`); Landingpage (`public_counts`); Vorgeschlagene Konten (`featured_accounts`) |
  | Beitragslöschung (`/admin/settings/auto-delete`) | Standard für neue Konten (die Einstellungen `auto_delete_default_*`); Höchstdauer (`auto_delete_max_age_days`, die Einstellungen `auto_delete_max_allow_*`, `auto_delete_max_kept_pins`, `auto_delete_grace_days`). Das Speichern einer Höchstdauer, die eingeschaltet oder verschärft wird (weniger Tage, weniger Ausnahmen oder Anheftungen), fragt zuerst nach, da es die Konten benachrichtigt, für die sie neu gilt ([Eine Höchstdauer für das Aufbewahren von Beiträgen](#a-maximum-for-keeping-posts)) |
  | Föderation (`/admin/settings/federation`) | Föderation (`federation_mode`, `authorized_fetch`, `remote_media_policy`, `remote_media_cache_ttl_seconds` (wähle aus 1 Stunde, 1 Tag, 7 Tagen, 30 Tagen oder 365 Tagen), `link_cards`, `link_cards_ignore_robots_txt`); Eingeschränkte und gesperrte Server (`domain_actions_visibility`, `domain_actions_rationale_visibility`); Externe Prüfung (die Einstellungen `hook_*`, siehe [Entscheidungs-Hooks](decision-hooks.md)) |
  | Suche und Crawler (`/admin/settings/discovery`) | Suchmaschinen und KI (`search_indexing`, `indexable_default`, `ai_usage`, `security_contact`); Beitragssuche (`search_visibilities`, nur solange der Server eine Volltextsuche hat) |
  | Sprachchat (`/admin/settings/voice`) | `voice_room_starters`, `voice_public_summary`, `voice_participants_retention_days`; die Seite wird nur aufgeführt, solange der Server Sprachchat anbietet |

  Auf der Seite „Allgemein“ wird ein Bild hochgeladen, sobald du es auswählst, und wird zum Vorschaubild, wenn du speicherst. Wer die
  laufenden Kosten, die Einnahmen und die Unterstützerliste sehen darf, wird auf den Bildschirmen „Finanzen“ und „Unterstützer“ festgelegt.

  `max_status_length`, `preview_image`, `appeal_window_days` und die Aufbewahrungszeiträume (`*_retention_days`), `remote_media_cache_max_bytes` und `oauth_client_rate_limits` erscheinen nicht auf dem Bildschirm;
  ändere sie mit der CLI oder der Client-API. Die Einstellungen der Übersetzung haben unten einen eigenen Bildschirm.
- **Webclient** („Übersetzung“, `/admin/translation`): die Anbieter, die dieser Server hat, ihre Reihenfolge und was jeder
  erhalten darf, die Übersetzung des Browsers, die Monatslimits und wie lange Übersetzungen aufbewahrt werden, mit den in
  diesem Monat gesendeten Zeichen und der Cache-Trefferquote. Jeder Serverprozess addiert seine Zahl der Cache-Treffer höchstens einmal pro Minute zur Quote,
  die neuesten Anfragen können also eine Minute brauchen, bis sie erscheinen, und ein Prozess, der stoppt, kann seine letzte Minute ungezählt lassen. Aktualisiere deine Datenschutzerklärung ([Vorlage](privacy-policy-template.md#the-services-you-choose)),
  bevor du einen serverseitigen Anbieter aktivierst: Text, der an einen Anbieter gesendet wurde, lässt sich nicht zurückholen. API-Schlüssel von Anbietern und die Adresse eines
  [Übersetzer-Plugins](plugins.md#translating-posts) liegen in der [Deployment-Konfiguration](../reference/configuration.md);
  keine Einstellung und keine Antwort der Client-API enthält sie.
- **Übersetzungslimits.** Gezählt werden die gesendeten Zeichen, in Codepoints: Die Platzhalter, die für Erwähnungen,
  Links und Ähnliches stehen, zählen nicht, ebenso wenig eine Übersetzung, die der Server bereits aufbewahrt, ein Beitrag, der bereits in der Sprache des Lesenden
  ist, oder die Übersetzung des Browsers. Jeder Aufruf an einen Anbieter zählt, auch ein fehlgeschlagener, da der Anbieter den Text
  erhalten haben kann, die Zahl kann also von dem abweichen, was ein Anbieter abrechnet. Über einem Limit wird die Anfrage mit `429` abgewiesen,
  und kein Anbieter wird aufgerufen; der Webclient verwendet dann die Browser-Übersetzung des Lesenden, wenn du sie erlaubst und der Browser
  den Beitrag übersetzen kann. Monate gelten in UTC.
- **Client-API**: `GET /api/v1/admin/settings` gibt jede Einstellung mit ihrem aktuellen Wert, ihrem Standard, ob sie gesetzt ist,
  und ihrem Typ und den akzeptierten Werten (Auswahl oder Bereich) zurück. `PATCH /api/v1/admin/settings` ändert Einstellungen gemeinsam mit
  `{"values": {"<key>": "<value>", ...}}`. Alle Werte sind Strings, normalisiert und gespeichert wie bei der CLI. Ist ein einzelner Schlüssel
  unbekannt oder ein Wert ungültig, lautet die Antwort 422, die den Schlüssel nennt, und nichts ändert sich. Die geänderten Schlüssel werden in derselben Transaktion als
  Revisionen und im Audit-Log festgehalten.
- Einladungen, die in den Modi `open`, `approval` und `invite` funktionieren, werden auf dem Moderationsbildschirm („Einladungen“,
  `/moderation/invites`, Berechtigung `manage_invites`; siehe [Einladungscodes](registration.md#invite-codes)) und von Mitgliedern erstellt,
  soweit `invite_creators` es erlaubt ([Mitgliedereinladungen](registration.md#member-invites)).

<!-- source: rules b1076bb81e0c -->
## Regeln

Die Regeln des Servers erscheinen in `rules` von `GET /api/v1/instance`, und Meldungen zitieren sie über die ID.

- Eine Regel hat einen einzeiligen Satz (bis zu 300 Zeichen) und eine optionale Ergänzung (bis zu 1.000 Zeichen). Wie bei Texten schreibt der
  Administrator sie in einer beliebigen Sprache, und jede Locale sieht denselben Wert (`dir="auto"` und `lang=""`).
  Es kann höchstens 100 aktive Regeln geben.
- Jede Änderung (ein überarbeiteter Satz, eine Umsortierung) wird als Revision aufbewahrt (`instance_rule_revisions`), und die ID einer Regel
  ändert sich nicht.
- Eine eingestellte Regel verschwindet aus der Liste der aktiven Regeln. Sie lässt sich nicht zurückholen, aber Meldungen können sich weiterhin auf sie beziehen.

```sh
my instance rule create -hint "No harassment" "Be kind"   # add at the end of the active rules
my instance rule edit <rule> "Always be kind"          # change the sentence; the supplement changes only with -hint
my instance rule move <rule> 1                         # move to position 1 and renumber the active rules 1, 2, ...
my instance rule retire <rule>                         # retire it
my instance rule list                                  # active rules (-all includes retired ones)
```

- Die Admin-API ist `/api/v1/admin/rules` (auflisten, hinzufügen, abrufen, überarbeiten, verschieben, einstellen) und braucht die Rollenberechtigung `manage_rules`.
- Auf dem Admin-Bildschirm des Webclients („Regeln“, `/admin/rules`, `manage_rules`) zeigt die Liste die geltenden Regeln der Reihe nach,
  und „Nach oben“ und „Nach unten“ sortieren sie um; der Filter über der Liste zeigt stattdessen die eingestellten Regeln. „Regel
  hinzufügen“ öffnet eine Seite mit einem Satz und einer Ergänzung. Wähle den Satz einer Regel, um ihre Seite zu öffnen, auf der du sie bearbeitest und
  einstellst (nach einem Bestätigungsdialog). Jede Änderung wird im Audit-Log festgehalten (`rule_added`, `rule_revised`,
  `rule_moved`, `rule_retired`).
- Die Exit-Codes sind dieselben wie bei Einstellungen (0 für Erfolg, 1 für einen abgelehnten Wert oder einen Datenbankfehler, 2 für falsche Argumente).

<!-- source: link-card-domains c8f674387fa8 -->
## Domains für Link-Karten

Die Link-Karten von Beiträgen (`link_cards`) stammen von einem Crawler: Der Server ruft die verlinkte Seite und ihr Bild ab. Eine Regel
stoppt diesen Crawler bei einer Domain und ihren Subdomains und lässt alles andere an der Domain unberührt. Sie hat einen von zwei
Modi. Eine Regel ist die Entscheidung des Betreibers, und sie gilt zusätzlich zu dem, was die verlinkten Websites selbst ablehnen ([Link-Vorschauen](link-previews.md#cards-of-links-in-posts)).

| Modus | Was abgerufen wird | Bereits vorhandene Karten |
| --- | --- | --- |
| Abruf stoppen (`no_fetch`) | Nichts von der Domain, weder Seiten noch Bilder, auch wenn eine Weiterleitung dorthin führt | Ausgeblendet |
| Nur Bilder stoppen (`no_images`) | Seiten, aber keine Bilder von der Domain | Ohne ihr Bild angezeigt |

- Eine Regel betrifft nur Link-Karten. Die Domain bleibt föderiert, und die Medien ihrer Beiträge werden wie gewohnt abgerufen: Um einen Server abzutrennen, verwende
  eine [Domain-Aktion](moderation.md), die auch seine Link-Karten stoppt. Eine Regel wird nicht bei den Domain-Aktionen aufgeführt oder mit ihnen exportiert.
- Eine Regel gilt für die Domain und ihre Subdomains, und ein Bild wird gegen die Hosts der Seite und des Bildes geprüft. Jede Regel, die einen Host abdeckt, gilt, eine Regel für eine Subdomain
  lockert also die Regel für ihre übergeordnete Domain nicht. Eine Domain wird wie bei einer Domain-Aktion geschrieben und normalisiert: ein Hostname ohne Schema oder Port, in beliebiger Schreibweise einer internationalisierten Domain.
- Eine Änderung wirkt sofort, auch auf die Karten, die der Server bereits hat, und die über den Proxy geleiteten Bilder dieser Karten werden nicht mehr ausgeliefert, solange Bilder gestoppt sind.
- Das Entfernen einer Regel bringt die Karten, die der Server bereits hat, so zurück, wie sie waren. Ein Link, der übersprungen wurde, erhält seine Karte, wenn ein Beitrag sie das nächste Mal braucht; nichts wird massenhaft erneut abgerufen.
  Eine Karte, die erneut abgerufen wurde, während Bilder gestoppt waren, bleibt bis zu ihrem nächsten erneuten Abruf ohne Bild.
- Auf dem Admin-Bildschirm des Webclients („Domains für Link-Karten“, `/admin/link-card-domains`, `manage_settings`) legt „Regel hinzufügen“ eine Regel mit einer Notiz fest. Die Liste zeigt 40 Regeln pro Seite und lässt sich nach Domain durchsuchen; wähle eine Domain, um die Seite ihrer Regel zu öffnen, auf der du ihren Modus und ihre Notiz änderst und sie entfernst (nach einem Bestätigungsdialog).
- Die Admin-API ist `/api/v1/admin/link_card_domains` (auflisten, festlegen; ein `DELETE` mit der Domain entfernt sie) und braucht die Rollenberechtigung `manage_settings`. Ein Server behält höchstens 1.000 Regeln, jede mit einer Notiz von bis zu 5.000 Zeichen.
- Jede Änderung wird im Audit-Log festgehalten (`link_card_domain_fetch_stopped`, `link_card_domain_images_stopped`, `link_card_domain_rule_removed`).

<!-- source: theme 380636a0e583 -->
## Design

Das serverweite Design ist eine Menge von Überschreibungen der Design-Tokens des Webclients plus ein optionales Stück CSS (eigenes CSS). Es
gilt für den Webclient jedes Betrachters und für die Bildschirme, die der Server rendert (Anmeldung, Registrierung, Bedingungen und so weiter), sowohl
hell als auch dunkel.

- Du kannst jedes Token in `app/ui/src/theme.css` überschreiben, außer den primitiven Farben (`--palette-*`). Die Liste, die Standardwerte und die
  Bereiche stehen in [app/ui/README.md](../../app/ui/README.md#server-themes). Eine Farbe ist `#rrggbb` für hell und für dunkel
  (`color-overlay` darf auch `#rrggbbaa` sein); die anderen Tokens nehmen einen Wert der Form ihrer Art (Länge, Zahl,
  Dauer, Easing, Schriftnamenliste, Schatten) in ihrem Bereich. Wenn du `font-sans` oder `font-mono` überschreibst, lass
  `"my Noto Color Emoji"` an erster Stelle; andernfalls werden die Emojis des Webclients mit der Schrift des Betriebssystems gezeichnet.
- Beim Speichern prüft der Server die Form und den Bereich der Werte, den Kontrast jedes Paars von Farb-Tokens (Tokens, die du nicht
  überschreibst, verwenden ihre Standardwerte; WCAG 2.2 AA in hell und dunkel) und die Länge des eigenen CSS (64 KiB). Schlägt eine Prüfung
  fehl, wird nichts gespeichert.
- Der Server liest das eigene CSS nicht. Er legt es nach den Token-Regeln, außerhalb jedes `@layer`, in dasselbe Stylesheet. Es läuft
  unter derselben CSP wie der Webclient, daher können Bilder, Schriften und `@import` von einem anderen Origin nicht geladen werden.
  **Der Server prüft die Auswirkungen von eigenem CSS nicht (Kontrast, horizontales Scrollen, Größe der Tippziele, Fokusanzeige,
  Rechts-nach-links-Sprachen); das liegt in der Verantwortung des Administrators.** Klassennamen und die Seitenstruktur ändern sich zwischen
  Versionen, daher sind nur die Tokens stabil genug, um sich darauf zu verlassen.
- Ein Design wird als Revisionen aufbewahrt (`instance_theme_revisions`), und das Stylesheet der aktuellen Revision wird unter
  `/theme/{revision ID}.css` ausgeliefert (`immutable`, lange gecacht). Die HTML-Shell hat einen gemeinsamen Cache (60 Sekunden plus 300
  Sekunden Revalidierung), es dauert also nach dem Speichern einige Minuten, bis alle es erhalten. Eine Seite, die bereits geöffnet ist, behält
  das alte Aussehen, bis sie neu geladen wird.
- `<meta name="theme-color">` (hell und dunkel) in der Shell und `theme_color` / `background_color` (hell) des Web App
  Manifests übernehmen den Wert von `color-surface` des Designs.
- Das Speichern einer Revision ohne Tokens und ohne eigenes CSS setzt das Design auf den Standard zurück, und das Stylesheet wird nicht mehr geladen.
- Du kannst es auf dem Admin-Bildschirm des Webclients („Design“, `/admin/theme`), mit der Client-API (`GET` / `PUT
  /api/v1/admin/theme`) und mit der CLI ändern. Der Admin-Bildschirm und die Client-API brauchen die Rollenberechtigung `manage_settings`, und
  Änderungen werden im Audit-Log festgehalten (`setting_changed`, Ziel `theme`).

```sh
my instance theme get > theme.json                    # print the current theme as the same JSON as the admin API
my instance theme set < theme.json                    # replace the whole theme (if a check fails, it prints why and changes nothing)
echo '{"tokens":[],"custom_css":""}' | my instance theme set   # back to the default
```

```json
{
  "tokens": [
    { "token": "color-accent", "light": "#1447e6", "dark": "#92c5fe" },
    { "token": "radius-md", "value": "0.25rem" }
  ],
  "custom_css": ".brand { letter-spacing: 0.02em; }"
}
```

<!-- source: logo-and-icon a2d23def322b -->
## Logo und Symbol

Neben dem Vorschaubild und dem Link-Vorschaubild kann der Server ein eigenes Symbol, ein Logo und ein dunkles Logo haben, jeweils als eine SVG hochgeladen.
Bis eines gesetzt ist, werden die eingebauten mycroblog-Bilder verwendet, und das Löschen eines Bildes bringt das eingebaute Bild zurück.

| Bild | Form | Wo es erscheint |
| --- | --- | --- |
| `icon` | Quadratisch (innerhalb von 1 %) | Das Favicon, das Home-Bildschirm-Symbol von iOS (`apple-touch-icon`), die Symbole der installierten App (Web App Manifest), die Karte des Servers auf der Landingpage und neben seinem Namen im Seitenbereich des Webclients sowie überall dort, wo ein Client den Server als quadratisches Bild zeigt |
| `logo` | 1- bis 8-mal so breit wie hoch | Die Kopfzeile des Webclients und der obere Teil der Anmelde-, Registrierungs- und Zustimmungsseiten anstelle der eingebauten Wortmarke und des Servernamens (das Logo sollte den Namen enthalten, der sein Alternativtext ist), die HTML-Version jeder E-Mail und das Link-Vorschaubild, wenn `preview_image` nicht gesetzt ist |
| `logo_dark` | Wie `logo` | Dunkle Hintergründe; ohne es wird dort ebenfalls `logo` verwendet |

- **Der Server bereinigt die SVG.** Er liest sie als XML und schreibt ein neues Dokument nur aus den erlaubten Elementen (Formen, Gruppen,
  `defs`, `symbol`, `use`, Verläufe, `clipPath`, `mask`, `pattern`, `style`, `title`, `desc`) und Attributen (Geometrie,
  Transformationen, Darstellungsattribute, `id`, `class`, `style`). Skripte, `foreignObject`, eingebettete Bilder, Links,
  Animationen, Filter (Schatten und Unschärfen), Event-Handler, Verweise auf alles außerhalb der Datei und `data:`-URLs werden entfernt,
  und die Antwort listet auf, was entfernt wurde. Nur die bereinigte SVG wird gespeichert.
- Eine SVG wird abgelehnt (nichts wird gespeichert), wenn sie kein wohlgeformtes XML in UTF-8 ist, ein `DOCTYPE` oder Entity-Deklarationen hat, kein
  `viewBox` hat, über 256 KiB groß ist (128 KiB nach dem Bereinigen), über 5.000 Elemente hat, tiefer als 32 Ebenen verschachtelt ist, über 200 `use`-
  Elemente hat, die falsche Form für ihre Art hat, `text` enthält (wandle Text zuerst in deinem Editor in Pfade um) oder CSS hat, das
  etwas laden könnte (`@import`, `@font-face`, `url()` außer `url(#id)`, Escapes). `@media (prefers-color-scheme: dark)`
  in einem `<style>` ist erlaubt, ein Symbol kann sich also im Dunkelmodus ändern.
- Manche Stellen nehmen keine SVG (der iOS-Home-Bildschirm, ältere Safari-Favicons, Link-Vorschauen, E-Mail). Für sie zeichnet der Client, der die
  SVG hochlädt, aus der bereinigten SVG auch PNG-Versionen und sendet sie mit: 32, 48, 180, 192 und 512 px sowie ein maskierbares 512-px-
  Symbol für `icon`; ein 96 px hohes E-Mail-Logo und ein Link-Vorschaubild von 1200 × 630 für `logo`; ein 96 px hohes E-Mail-Logo für
  `logo_dark`. Der Server prüft, dass jede ein PNG der richtigen Größe ist (höchstens 1 MiB; das 180-px-Symbol, das maskierbare Symbol und das
  Link-Vorschaubild müssen deckend sein) und schreibt sie ohne Metadaten neu. Ist eine Datei falsch, wird nichts gespeichert. Die deckenden
  Versionen des Symbols werden auf eine Hintergrundfarbe gezeichnet, `#rrggbb`, die standardmäßig das helle `color-surface` des Designs ist. Der Server
  prüft nicht, ob die PNG-Versionen dasselbe Bild zeigen wie die SVG.
- Die Dateien werden im Blob-Store unter `instance/brand/<revision>/` gespeichert und unter
  `https://<host>/brand/<revision>/<version>.svg|.png` mit `Cache-Control: public, max-age=31536000, immutable` ausgeliefert, da sich der
  Inhalt einer Revision nie ändert. Eine SVG wird mit `Content-Security-Policy: default-src 'none'; style-src
  'unsafe-inline'; sandbox` und `X-Content-Type-Options: nosniff` ausgeliefert, sodass sie auch allein geöffnet nichts ausführt und nichts lädt.
  Die Dateien früherer Revisionen bleiben erhalten, sodass bereits gesendete E-Mails ihr Logo behalten.
- Mit gesetztem Symbol verlinkt die HTML-Shell seine PNG-Favicons, seine SVG und sein 180-px-`apple-touch-icon`, das Web App Manifest listet
  seine 512-px-, 192-px-, maskierbaren und SVG-Symbole in dieser Reihenfolge auf (andere Fediverse-Server nehmen das erste als Symbol dieses Servers),
  und `/apple-touch-icon.png` gibt die 180-px-Version zurück. Das ETag der Shell ändert sich mit den Bildern, und der gemeinsame Cache der Shell
  hält wie beim Design einige Minuten. Eine installierte App übernimmt unter Android ein neues Symbol, wenn Chrome das Manifest das nächste Mal prüft;
  auf Desktops behält die App ihr altes Symbol, bis sie erneut installiert wird.
- `GET /api/v1/instance` gibt `icon`, `logo` und `logo_dark` mit der URL der SVG, der URL und Größe jeder PNG-Version
  und der Revision zurück; jedes wird weggelassen, solange das eingebaute Bild in Verwendung ist.
- Lege sie im Webclient unter **Servereinstellungen** > **Symbol und Logo** (`/admin/settings/branding`, Rollenberechtigung `manage_settings`) fest. Wähle eine SVG-
  Datei für das Symbol, das Logo oder das Logo für dunkle Hintergründe: Der Server prüft sie, der Browser zeichnet die PNG-Versionen, und das
  Formular zeigt sie als Vorschau (einen Browser-Tab, den Home-Bildschirm, das App-Symbol, eine E-Mail auf hellem und dunklem Hintergrund und das Link-
  Vorschaubild) mit der Liste dessen, was der Server entfernt hat. Lehnt der Server die Datei ab, steht der Grund unter der Schaltfläche, mit dem Weg,
  ihn zu beheben. Lege für das Symbol dort auch die Hintergrundfarbe seiner deckenden Versionen fest. **Speichern** sendet die SVG mit jeder PNG-
  Version; **Eingebautes Symbol verwenden**, **Eingebautes Logo verwenden** und **Logo für dunkle Hintergründe entfernen** löschen sie. Der
  Webclient behält die Bilder, die er angezeigt hat, für die Offline-Nutzung.
- Die Client-API tut dasselbe, mit der Rollenberechtigung `manage_settings`:
  `POST /api/v1/admin/instance/brand/sanitize` prüft eine SVG und gibt die bereinigte SVG mit den zu zeichnenden PNG-Versionen zurück (sie
  speichert nichts), `PUT /api/v1/admin/instance/brand/{icon|logo|logo_dark}` speichert die SVG mit jeder PNG-Version
  (`multipart/form-data`, ein Teil pro Version), `DELETE` auf demselben Pfad löscht eines, und `GET /api/v1/admin/instance/brand` gibt die aktuellen
  Bilder mit der Hintergrundfarbe des Symbols zurück. Jede Änderung wird als Revision aufbewahrt (`instance_brand_revisions`) und im
  Audit-Log festgehalten (`setting_changed`, Ziel `brand_icon`, `brand_logo` oder `brand_logo_dark`).
- Die CLI kann sie nicht setzen, da die PNG-Versionen vom Client gezeichnet werden, aber sie kann sie anzeigen und ein fehlerhaftes löschen:

```sh
my instance brand show          # each image with its revision, when and by whom it was set, and the URL of its SVG
my instance brand clear icon    # back to the built-in icon (also logo, logo_dark)
```

<!-- source: custom-emoji 5e0ddb3251cf -->
## Eigene Emojis

Lokale eigene Emojis werden als Bilder angezeigt, wenn jemand `:shortcode:` im Text oder in der Inhaltswarnung eines Beitrags oder im
Anzeigenamen, in der Bio oder in den Profilfeldern schreibt. Sie erreichen über das `Emoji`-Tag der
Notes und Actors, die du sendest, auch Mastodon, Misskey, GoToSocial und Akkoma.

- Ein Shortcode besteht aus 2-30 Buchstaben, Ziffern oder `_`. Er unterscheidet Groß- und Kleinschreibung und lässt sich nach der Registrierung des Emojis nicht mehr ändern.
  Aliasse (bis zu 10) und eine Kategorie (bis zu 64 Zeichen) werden für die Suche und die Gruppierung in der Emoji-Auswahl verwendet.
- Das Bild ist PNG, GIF oder WebP, höchstens `custom_emoji_max_bytes` (Standard 256 KiB) groß und auf jeder Seite höchstens 1.024 px. Der
  Server erzeugt Ableitungen, die in Rahmen von 64, 128 und 256 px passen, und behält das Original nicht. Ein APNG wird zu einem Standbild
  seines ersten Frames. GoToSocial ruft standardmäßig keine Emojis ab, die größer als 100 KiB sind, daher werden kleine Bilder empfohlen.
- Ein deaktiviertes Emoji erscheint nirgends (Beiträge, Profile, Föderation, die Auswahl), und `:shortcode:` wird wieder als Text angezeigt. Ein Emoji,
  das in der Auswahl verborgen sein soll, fehlt nur in der Auswahl und lässt sich weiterhin verwenden, indem man es schreibt. Ein Emoji, das von
  einem anderen Server kam, lässt sich nur deaktivieren oder als Emoji dieses Servers kopieren.
- Das Kopieren ruft das Bild des Emojis eines anderen Servers unter denselben Limits wie der Media-Proxy ab (Bilder von Domains mit
  `reject_media` werden nicht abgerufen), führt dieselben Prüfungen wie oben aus und registriert es als neues lokales Emoji. Der Shortcode ist der
  des Originals oder der, den du angibst. Das Bild wird auf den 256-px-Rahmen verkleinert, und ein Bild, das sich nicht abrufen lässt oder die Prüfungen nicht besteht,
  hinterlässt nichts.
- Das Löschen lässt sich nicht rückgängig machen. Auch Emojis in früheren Beiträgen werden wieder zu Text, und das erneute Registrieren desselben Shortcodes ergibt ein
  anderes Emoji. Bilder, die föderierte Server kopiert haben, lassen sich nicht löschen.
- Der Admin-Bildschirm („Eigenes Emoji“, `/admin/custom-emojis`), die Client-API (`/api/v1/admin/custom_emojis`) und die CLI tun
  dasselbe. Auf dem Bildschirm öffnet „Eigenes Emoji registrieren“ das Registrierungsformular (`/admin/custom-emojis/new`), und das Auswählen
  eines Emojis in der Liste öffnet seine Seite (`/admin/custom-emojis/{id}`), auf der du seine Einstellungen änderst, sein Bild ersetzt, es aktivierst oder
  deaktivierst, es löschst oder eines von einem anderen Server kopierst; die Liste selbst aktiviert und deaktiviert ein Emoji mit einem Klick. Der Admin-Bildschirm und die Client-API brauchen die Rollenberechtigung `manage_custom_emojis` (nur die eingebaute `admin`
  hat sie). Das Registrieren, Kopieren, Ersetzen des Bildes, Ändern von Einstellungen und Aliassen, Aktivieren und Deaktivieren und Löschen werden
  im Audit-Log festgehalten (Ziel `custom_emoji`), auf welchem Weg sie auch erfolgen (ein Vorgang über die CLI wird als CLI-
  Vorgang festgehalten).

```sh
my admin emoji create -shortcode blobcat -category Cats -alias neko blobcat.png   # register one
my admin emoji list -local                                                        # list local emoji, newest first
my admin emoji update -category Animals -alias cat -alias neko <emoji id>         # change the category and aliases (aliases are replaced; -no-aliases removes them)
my admin emoji update -hidden <emoji id>                                          # hide it from the picker (-hidden=false restores it)
my admin emoji replace-image <emoji id> blobcat2.png                              # replace the image
my admin emoji disable <emoji id>                                                 # disable it (remote emoji too)
my admin emoji enable <emoji id>                                                  # enable it again
my admin emoji list -remote -domain mastodon.example                              # list emoji of another server
my admin emoji copy [-shortcode <name>] <emoji id>                                # copy a remote emoji to local
my admin emoji delete <emoji id>                                                  # delete it
my admin emoji requests list [-state pending|approved|rejected|withdrawn|lapsed|all] # list creation requests (pending by default)
my admin emoji requests approve <request id>                                      # approve a request
my admin emoji requests reject [-reason <text>] <request id>                      # reject a request (the requester sees the reason)
```

- Ein Nutzer mit der Rollenberechtigung `create_custom_emojis` (eine Nutzerfunktion) kann unter „Eigenes
  Emoji“ in den Einstellungen (`/settings/custom-emojis`) die Erstellung eines Emojis beantragen, die Aliasse, die Kategorie und die Sichtbarkeit in der Auswahl der von ihm
  erstellten Emojis ändern und sie löschen. Auch das Ersetzen eines Bildes ist ein Antrag. Ein Antrag erscheint nirgends (Auswahl, Beiträge, Reaktionen,
  Föderation), bis jemand mit `manage_custom_emojis` ihn genehmigt. Genehmige oder lehne ihn auf der Seite „Anfragen“ von „Eigenes
  Emoji“ auf dem Admin-Bildschirm (`/admin/custom-emojis/requests`), mit der Client-API (`/api/v1/admin/custom_emoji_requests`) oder mit der obigen CLI ab. Personen mit
  `manage_custom_emojis` werden über einen Antrag benachrichtigt (`admin_custom_emoji_request`), und der Antragsteller wird über die
  Genehmigung oder Ablehnung benachrichtigt. Beantragen, Zurückziehen, Genehmigen und Ablehnen werden im Audit-Log festgehalten (Ziel
  `custom_emoji_request`).
- Ein ausstehender Antrag auf Erstellung reserviert seinen Shortcode. Ein Antrag oder eine Registrierung mit demselben Shortcode ist nicht möglich, bis
  der ausstehende abgelehnt oder zurückgezogen wird. Das Bild eines genehmigten Emojis wird als Upload des Server-Actors kopiert, es
  bleibt also bestehen, auch wenn das Konto des Antragstellers gelöscht wird.
- Ein Ersteller kann seine Emojis nur ändern, solange er `create_custom_emojis` hält. Die Emojis bleiben, wenn die Rolle entzogen wird.
  Administratoren können jedes Emoji unabhängig vom Ersteller ändern und löschen.
- Stammt die Rolle aus einem Unterstützungsplan, wird die Zahl der Emojis, die ein Nutzer pro Monat erstellen kann, durch das monatliche Emoji-Limit
  des Plans begrenzt. Es zählt die Anträge auf Erstellung, die der Nutzer im laufenden Kalendermonat (UTC) gestellt hat und die
  genehmigt oder ausstehend sind; abgelehnte und zurückgezogene Anträge zählen nicht, und das Löschen eines Emojis gibt keinen Platz frei. Ein Antrag
  zählt in dem Monat, in dem er gestellt wurde, auch wenn er im nächsten genehmigt wird. Die Zählung wird am ersten Tag jedes Monats um 00:00 UTC zurückgesetzt,
  und die Einstellungsseite zeigt dem Nutzer, wann, in seiner eigenen Zeitzone. Das Ersetzen des Bildes eines Emojis und
  Emojis, die Administratoren registrieren, zählen nicht. Eine Rolle, die ein Administrator von Hand zugewiesen hat, und ein Server, auf dem Unterstützer
  deaktiviert sind, haben kein Limit.
