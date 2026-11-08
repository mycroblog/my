<!-- source: production-checklist e5f6c0c5b10a -->
# Produktions-Checkliste

Gehe diese Liste durch, bevor du den Server für Nutzer und für andere Server öffnest, und erneut nach einer Änderung an der Bereitstellung.
Manche Einstellungen lassen sich nicht mehr ändern, sobald der Server föderiert hat, und manche Fehler, etwa der Verlust des Key-Encryption-Keys,
lassen sich gar nicht rückgängig machen.

Jeder Punkt sagt, warum er wichtig ist, wie du ihn prüfst und wo die Details stehen.
Beginne mit dem automatisierten Teil:

```sh
my config check            # the configuration, and the settings that do not suit production
my config check -instance  # also the instance settings in the database
```

<!-- source: what-my-config-check-checks f5feea1abf22 -->
## Was `my config check` prüft

Im Profil `production` (der Standard von [`profile`](../reference/configuration.md#profile)) validiert `my config check`
zuerst jede Einstellung und prüft dann, ob die gültigen Einstellungen zu einem Produktions-Server passen.
Die Probleme werden in zwei Gruppen aufgelistet, unter einer Zusammenfassung in der Sprache der Prozess-Locale, jeweils mit dem technischen
Detail auf Englisch:

- **Fehler** lassen `my config check` mit 1 enden. Es sind Einstellungen, die einen Produktions-Server kaputt machen oder seine Sicherheit schwächen:
  - `host` oder `account_domain`, die andere Server nicht erreichen können: mit einem Port, `localhost`, einem Namen ohne Punkt oder einem Namen unter
    einer Top-Level-Domain, die für lokale Nutzung oder Tests reserviert ist (`.localhost`, `.local`, `.test`, `.example`, `.invalid`, `.internal`, `.home.arpa`).
  - `web_origin` über `http`.
  - Eine SQLite-`database_url`, die die Datenbank im Arbeitsspeicher hält.
  - Ein `kek`, der nicht zufällig ist, etwa lauter Nullen oder ein wiederholtes Byte.
  - Ein `captcha`-Site-Key oder -Secret-Key, der einer der Testschlüssel ist, die die CAPTCHA-Dienste veröffentlichen.
  - `worker_job_types`, `worker_job_types_except` oder eine Queue-Route, die keinen Job-Typ nennt (in jedem Profil; `my job types` listet sie auf).
  - Mit `-instance` ein `worker_job_concurrency`, das der Pool der Datenbank nicht bedienen kann.
- **Warnungen** lassen den Exit-Code bei 0. Es sind Einstellungen, die in der Entwicklung üblich sind und in der Produktion Absicht sein können:
  - `mail = none`.
  - Eine SQLite-Datenbank oder der `fs`-Blob-Store in einem temporären Verzeichnis (`/tmp`, `/var/tmp`, `/dev/shm` oder `$TMPDIR`).
  - `queue = inline`.
  - Kein `vapid_private_key`.
  - `stream = hub` oder `kv = memory` mit einer anderen `role` als `all`: Sie funktionieren nur innerhalb eines Prozesses.
  - Voice-Einstellungen, die wahrscheinlich nicht gewollt waren, etwa `voice = p2p` ohne STUN oder TURN.
  - Ein [Plugin](plugins.md) unter einer `http://`-Adresse oder unter einer `https://`-Adresse ohne Token und ohne Client-Zertifikat.
  - Ein Testmodus-Schlüssel des Zahlungsanbieters.
  - `origin_secret` gesetzt bei leerem `trusted_proxies`, und `origin_secret_previous`, das nach einer Rotation gesetzt blieb.
  - Mit `-instance`: die Server-Texte `name`, `short_description`, `terms` und `privacy_policy`, die Server-Einstellungen
    `contact_account`, `contact_email` und `security_contact`, wenn sie nicht gesetzt sind, und ein Server ohne Regeln.
    Wenn die Datenbank nicht erreichbar ist, ist das eine weitere Warnung, und der Rest der Prüfung läuft trotzdem.

`my serve` protokolliert beim Start dieselben Befunde als Warnungen, startet aber trotzdem.
Das Profil `development` überspringt diese Prüfungen; verwende es für lokale Entwicklung und Tests, nie für einen Server, den andere nutzen.

Führe `my config check` in derselben Umgebung aus wie den Server (dieselben Umgebungsvariablen, Secret-Dateien und Konfigurationsdatei),
zum Beispiel als Schritt der Bereitstellung, bevor die neue Version Traffic annimmt.

<!-- source: settings-you-cannot-change-later b2c961cb26f5 -->
## Einstellungen, die sich später nicht mehr ändern lassen

<!-- source: the-servers-domain-and-the-handle-domain b01ce235bcc0 -->
### Die Domain des Servers und die Handle-Domain

- **Warum**: `host` ist Teil jeder ActivityPub-ID des Servers, und andere Server speichern diese IDs.
  `account_domain` ist die Domain der Handles (`@alice@example.org`). Das erste `my serve` oder `my`-Kommando, das die
  Datenbank verwendet, trägt beide darin ein, und von da an weigern sich `my serve` und diese Kommandos zu laufen, wenn die Konfiguration abweicht:
  Eine Änderung nach dem Föderieren zerstört jeden Follow und jeden Verweis auf die Beiträge und Konten des Servers.
  Setze beide vor dem ersten Start. Ein Fehler, der vor dem Föderieren auffällt, wird behoben, indem du
  [die Datenbank neu anlegst](database.md#recreating-erasing-every-row), was jede Zeile löscht.
- **Entscheide**: ob die Handles die Domain des Servers verwenden oder eine eigene (zum Beispiel Handles auf `example.org`
  und der Server auf `social.example.org`). Eine eigene Handle-Domain muss ihr WebFinger, host-meta und NodeInfo
  auf den Server weiterleiten.
- **Prüfe**: `my config check` zeigt `host` und `account_domain` und meldet eine Domain, die andere Server nicht erreichen können.
  `my config check -instance` scheitert außerdem, wenn sie von den Werten in der Datenbank abweichen, und trägt nichts ein.
  Bei einer eigenen Handle-Domain antwortet `curl -sI "https://<account_domain>/.well-known/webfinger?resource=acct:<user>@<account_domain>"`
  mit `301` auf `https://<host>/.well-known/webfinger?...`.
- **Details**: [den Server betreiben](running.md) (die Föderations-Endpunkte und die Auslieferung der Handles von einer anderen Domain),
  [`host`](../reference/configuration.md#host) und [`account_domain`](../reference/configuration.md#account_domain).

<!-- source: reachable-over-https 1cf53c613f7b -->
### Über `https` erreichbar

- **Warum**: Andere Server rufen die IDs unter `https://<host>/` auf Port 443 ab, und Browser senden die Anmelde-Cookies nur über `https`.
- **Prüfe**: `curl -sI https://<host>/healthz` antwortet von außerhalb deines Netzwerks mit `200` und einem für `host` gültigen Zertifikat,
  und `my config check` meldet keinen Fehler zu `web_origin`. `http://<host>/` leitet auf `https` um, was der Reverse-Proxy,
  Load Balancer oder das CDN vor dem Server übernimmt (bei Docker Compose ist die Schicht `compose.caddy.yaml` ein solcher Proxy).
- **Details**: [den Server betreiben](running.md), [`web_origin`](../reference/configuration.md#web_origin).

<!-- source: not-reachable-past-the-cdn 58310b9f29d7 -->
### Nicht am CDN vorbei erreichbar

- **Warum**: Mit einem CDN davor und der eigenen Adresse des Servers weiterhin öffentlich (`*.run.app` auf Cloud Run) kann jeder den Server an WAF und Cache vorbei aufrufen,
  und ein Aufrufer auf der eigenen Plattform des CDN kann die Client-IP wählen, die die Ratenbegrenzungen sehen. Nur in diesem Fall nötig: nicht, wenn eine Firewall oder die Plattform schon einschränkt, wer sich verbinden kann,
  und ohne CDN gibt es nichts zu umgehen.
- **Prüfe**: `origin_secret` und `origin_secret_header` sind gesetzt, und das CDN setzt den Header bei jedem Hostnamen. `curl -s -o /dev/null -w '%{http_code}\n' https://<origin address>/`
  antwortet `403`, sein `/healthz` antwortet `200`, und `https://<host>/` antwortet `200`.
- **Details**: [Anfragen abweisen, die nicht über das CDN kamen](cdn.md#refuse-requests-that-did-not-come-through-the-cdn), einschließlich der Wiederherstellung, wenn es alle aussperrt.

<!-- source: secrets ec71f1f20574 -->
## Secrets

<!-- source: the-key-encryption-key-kek 89df548c5bf9 -->
### Der Key-Encryption-Key (KEK)

- **Warum**: Der KEK verschlüsselt die privaten Schlüssel jedes Actors. Ohne ihn kann der Server nichts signieren, und die Schlüssel lassen sich nicht
  aus der Datenbank wiederherstellen: Jedes Konto müsste neu angelegt werden, und andere Server würden ihm nicht mehr vertrauen.
- **Prüfe**:
  - Er wurde zufällig erzeugt (`openssl rand -base64 32`); `my config check` meldet einen Schlüssel, der nicht zufällig ist.
  - Er ist an einem anderen Ort als die Datenbank und ihre Backups gespeichert, zum Beispiel in einem Passwortmanager oder dem
    Secret-Speicher der Cloud, und mindestens eine weitere Person, die den Server betreibt, kann ihn erreichen.
  - Er erreicht den Server nur über `MY_KEK` oder `MY_KEK_FILE` (die Konfigurationsdatei weist Secrets ab). Bei Docker Compose ist er
    die Datei `secrets/kek` und steht nicht in `.env` ([Docker-Compose-Dateien](running.md#docker-compose-files)).
- **Details**: [Schlüsselverwaltung](key-management.md).

<!-- source: the-vapid-key-of-web-push 76bc2a1dd030 -->
### Der VAPID-Schlüssel für Web-Push

- **Warum**: Ohne ihn sendet der Server keine Web-Push-Benachrichtigungen. Ihn später zu ändern beendet jedes Abo, also erzeuge ihn einmal
  und bewahre ihn wie den KEK auf.
- **Prüfe**: `my config check` warnt, wenn `vapid_private_key` nicht gesetzt ist. Erzeuge einen mit `my config vapid-key`.
- **Details**: [Schlüsselverwaltung](key-management.md) (der VAPID-Schlüssel für Web-Push).

<!-- source: other-secrets-stay-out-of-the-repository-and-the-image 94f290afcdbb -->
### Andere Secrets bleiben außerhalb des Repositorys und des Images

- **Warum**: Ein Secret in einem Repository, einem Container-Image oder einer Konfigurationsdatei wird an jeden Ort kopiert, an den diese gelangen.
- **Prüfe**: `database_url`, `mail_smtp_password`, `captcha_secret_key`, die Schlüssel des Zahlungsanbieters, `valkey_url` und jede andere
  in der Konfigurationsreferenz als Secret markierte Einstellung kommen aus Umgebungsvariablen oder `<NAME>_FILE`-Dateien, die der
  Secret-Speicher der Plattform befüllt (Docker- oder Kubernetes-Secrets, Secret Manager, Parameter Store). Die Konfigurationsdatei kann sie nicht enthalten:
  `my config check` weist dort ein Secret ab. Bei Docker Compose kommen sie aus `.env`, die nur du lesen kannst (`chmod 600`). Durchsuche das Repository und das Image nach den Werten, um sicherzugehen, dass keine Kopie übrig ist.
  `my config check` gibt für jedes gesetzte Secret `(redacted)` aus.
- **Details**: [Konfigurationsreferenz](../reference/configuration.md) (die Spalte Secret), [CAPTCHA](captcha.md),
  [Stripe](payments-stripe.md), [PAY.JP](payments-payjp.md).

<!-- source: instance-settings baae9cc3e09e -->
## Server-Einstellungen

Die Server-Einstellungen liegen in der Datenbank und lassen sich ändern, während der Server läuft, in den Administrations-Screens des Servers
oder mit `my instance set`. `my config check -instance` warnt vor den Texten, Kontakten und Regeln, die nicht gesetzt sind.

- **Name, Kurzbeschreibung und Beschreibung**: werden auf der Startseite, der Informationsseite (`/about`), in Link-Vorschauen
  und in NodeInfo gezeigt. Prüfe auf `/about`.
- **Kontakte**: `contact_account` (das Konto, an das sich Nutzer wegen des Servers wenden) und `contact_email` (die Adresse, an die Nutzer
  und andere Server schreiben; E-Mails des Servers senden Antworten ebenfalls dorthin). Prüfe auf `/about`.
- **Kontakt für Sicherheitslücken**: `security_contact`, eine `https:`- oder `mailto:`-URI. Ist sie gesetzt, antwortet der Server auf
  `/.well-known/security.txt`. Prüfe mit `curl -s https://<host>/.well-known/security.txt`.
- **Nutzungsbedingungen und Datenschutzerklärung**: `terms` und `privacy_policy`. Die Datenschutzerklärung muss die Dienste nennen, die
  personenbezogene Daten erhalten, etwa den CAPTCHA-Dienst und den E-Mail-Anbieter. Prüfe die Links auf `/about` und auf dem Registrierungs-Screen.
- **Serverregeln**: dem, was Nutzer bei der Registrierung zustimmen und was Moderatoren durchsetzen. Füge sie mit `my instance rule create`
  oder in den Administrations-Screens hinzu und prüfe sie auf `/about`.
- **Registrierungsmodus**: `registration_mode` ist `approval`, bis du ihn änderst; wähle `open`, `approval`, `invite` oder `closed`,
  je nachdem, wie Leute beitreten. Mit `mail = none` warten auch `open`-Registrierungen auf die Freigabe eines Moderators.
- **Suchmaschinen und KI**: `search_indexing`, `ai_usage` und `indexable_default` verweigern beides, bis du es erlaubst.
  Prüfe `/robots.txt` und `/.well-known/tdmrep.json`.
- **Statusseite**: Setze [`status_page_url`](../reference/configuration.md#status_page_url) (eine Einstellung, keine Server-Einstellung,
  damit der Link während der Wartung angezeigt wird) auf eine Statusseite bei einem Dienst abseits des Servers.

**Details**: [Server-Einstellungen](instance-settings.md), [Registrierung](registration.md),
[Suchmaschinen und KI-Crawler](crawlers.md), [CAPTCHA](captcha.md) (was in die Datenschutzerklärung gehört),
[Vorlage der Datenschutzerklärung](privacy-policy-template.md), [Vorlage für das Verzeichnis von Verarbeitungstätigkeiten](processing-record-template.md),
[Überwachung](monitoring.md) (die Statusseite).

<!-- source: email 40c18999c46f -->
## E-Mail

- **Warum**: E-Mail bestätigt Adressen und sendet Passwort-Zurücksetzungen und Benachrichtigungen. Mit `mail = none` erreicht keine E-Mail die Nutzer.
- **Prüfe**:
  - `mail` ist `smtp`, `ses` oder `plugin`, und `mail_from` ist eine Adresse auf einer Domain, die du kontrollierst; `my config check` warnt vor `mail = none`.
  - Die Domain von `mail_from` hat SPF-, DKIM- und DMARC-Records, und der Anbieter signiert mit DKIM. Ohne sie landen E-Mails im Spam.
  - Registriere dich mit einem Testkonto und empfange die Bestätigungs-E-Mail; fordere ein Zurücksetzen des Passworts an und empfange auch das.
    Prüfe die Header der empfangenen Nachricht auf `spf=pass`, `dkim=pass` und `dmarc=pass`.
  - Bei SES ist das Konto aus der Sandbox, und die Bounce- und Complaint-Benachrichtigungen erreichen den Server.
- **Details**: [E-Mail](email.md), [Registrierung](registration.md) (E-Mail-Bestätigung).

<!-- source: administrators 2c05e6644d88 -->
## Administratoren

- **Warum**: Das Konto eines Administrators kann jede Einstellung ändern, private Daten lesen und jeden sperren.
- **Prüfe**:
  - Jeder Administrator hat die Zwei-Faktor-Authentifizierung eingeschaltet (Einstellungen, Zwei-Faktor-Authentifizierung im Web-Client)
    und bewahrt die Wiederherstellungscodes auf.
  - Nur die Leute, die sie brauchen, haben die Rolle `admin`; Moderatoren haben `moderator` oder eine Rolle mit nur den Berechtigungen, die sie verwenden.
    Liste sie mit `my admin role` auf und überprüfe sie in den Administrations-Screens.
- **Details**: [Administratoren und Rollen](admin-cli.md), [Moderation](moderation.md) (Berechtigungen und Anmelde-Wiederherstellung).

<!-- source: operations 56bd643c2241 -->
## Betrieb

- **Backups und Wiederherstellung**: Die Datenbank, der Blob-Store (`blob_fs_dir` oder der Bucket) und der KEK werden nach Zeitplan gesichert,
  an einen Ort abseits des Servers, und eine Wiederherstellung auf einem frischen Server wurde geübt: Der wiederhergestellte Server startet, und die Konten
  können sich anmelden und posten. Von einem Backup, das nie wiederhergestellt wurde, weiß man nicht, ob es funktioniert. Die Datenbank und die Medien dürfen nicht in einem
  temporären Verzeichnis liegen; `my config check` warnt, wenn sie es tun.
  Siehe [Schlüsselverwaltung](key-management.md) (den KEK sichern) und [Datenbank](database.md).
- **Überwachung und Alarme**: Ein externer Monitor prüft `/healthz` und alarmiert jemanden, der handeln kann, und die toten Jobs der Job-Queue
  werden beobachtet. Siehe [Überwachung](monitoring.md) und [den Server betreiben](running.md) (Health-Checks, Metriken und die Job-Queue).
- **Das Image**: Der Server führt ein veröffentlichtes Image aus, das per Digest festgelegt ist (`ghcr.io/mycroblog/my:0.1.0@sha256:...`, aus den Release Notes), nicht eines,
  das sich bei einem Neustart ändert, und [seine Herkunft wurde geprüft](running.md#verify-an-image). Es gibt kein `latest`-Tag.
  Ein Upgrade ändert diese Version absichtlich und folgt [Aktualisieren](database.md#upgrading), jeweils eine Minor-Version.
- **Quellcode (AGPL-3.0)**: Der Server muss jedem, der ihn nutzt, seinen Quellcode anbieten. Lass
  [`source_url`](../reference/configuration.md#source_url) bei einem unveränderten Release auf seinem Standard; richte ihn auf deinen eigenen
  Quellcode, wenn du einen Fork oder eine veränderte Version betreibst. Prüfe den Link auf `/about`.
  Siehe [den Server betreiben](running.md) (den Quellcode veröffentlichen).
- **Wartung**: Wisse, wie du den Server in den Wartungsmodus versetzt, bevor du es brauchst. Siehe [Wartung](maintenance.md).

<!-- source: federation a5ab186ec04b -->
## Föderation

- **Warum**: Andere Server finden Konten über WebFinger und NodeInfo und rufen dann den Actor ab. Scheitert eines davon von außen,
  ist der Server für das Fediverse unsichtbar.
- **Prüfe** von einem Rechner außerhalb deines Netzwerks:
  - `curl -s "https://<account_domain>/.well-known/webfinger?resource=acct:<user>@<account_domain>"` liefert das Konto,
    wobei die Weiterleitung zu `host` verfolgt wird, wenn die Domains abweichen (`curl -L`).
  - `curl -s https://<host>/.well-known/nodeinfo` verlinkt `/nodeinfo/2.1`, das die Software und ihren Quellcode nennt.
  - `curl -s -H 'Accept: application/activity+json' https://<host>/@<user>` leitet auf die ID des Actors um,
    und der Actor unter dieser ID ist JSON mit einem `publicKey`.
  - Suche von einem Konto auf Mastodon oder einem anderen Server nach `@<user>@<account_domain>`, folge ihm und sieh einen neuen Beitrag eintreffen.
  - Ein CDN vor dem Server speichert `/ap/*` und `/@*` nicht zwischen, es sei denn, es beachtet `Vary: Accept`.
- **Details**: [den Server betreiben](running.md) (die Föderations-Endpunkte), [CDN](cdn.md), [Relays](relays.md).
