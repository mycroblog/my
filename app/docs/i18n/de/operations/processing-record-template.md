<!-- source: record-of-processing-activities-template 75f7037cf953 -->
# Vorlage für das Verzeichnis von Verarbeitungstätigkeiten

Das Verzeichnis von Verarbeitungstätigkeiten, das Artikel 30 Absatz 1 DSGVO von einem Verantwortlichen verlangt, ausgefüllt, soweit die Software entscheidet.

**Dies ist eine Vorlage und keine Rechtsberatung.** Sie beschreibt, was die Software mit ihren Standardeinstellungen tut, und entscheidet nicht, ob die DSGVO oder dieses Verzeichnis für dich gilt. Lass sie für deine Rechtsordnung prüfen.

Artikel 30 Absatz 5 nimmt eine Organisation mit weniger als 250 Beschäftigten aus, es sei denn, ihre Verarbeitung birgt wahrscheinlich ein Risiko für die Rechte und Freiheiten von Personen, erfolgt nicht nur gelegentlich oder umfasst besondere Datenkategorien oder Strafdaten. Wer einen Server betreibt, führt dauerhaft Konten, nimm die Ausnahme also nicht an, ohne sie zu prüfen. Das Verzeichnis zu führen hilft dir auch, Anfragen zu beantworten und die [Datenschutzerklärung](privacy-policy-template.md) zu schreiben.

<!-- source: how-to-use-it 32e30db3b6f3 -->
## So verwendest du sie

1. Kopiere alles ab [Verzeichnis](#record) in deine eigenen Unterlagen. Dies ist ein internes Dokument: Veröffentliche es nicht mit den Texten des Servers, und gib es einer Aufsichtsbehörde auf Verlangen.
2. Ersetze jedes `{{...}}`. Ein Wert in den Klammern, wie `{{180 days}}`, ist der Standard der Software. Bestätige ihn für deinen Server. Die [Vorlage der Datenschutzerklärung](privacy-policy-template.md#the-periods-and-their-settings) nennt die Einstellung jeder Frist.
3. Lösche eine Tätigkeit oder eine Zeile, die mit `{{IF ...}}` beginnt und für deine Einrichtung nicht zutrifft. [Die Dienste, die du wählst](privacy-policy-template.md#the-services-you-choose) sagt, welcher Einstellung jeder Dienst folgt.
4. Prüfe, dass nichts übrig ist: `grep -n '{{' <your file>` gibt keine Zeile aus.
5. Überprüfe es, wenn du eine Einstellung oder einen Dienst änderst, und datiere jede Überprüfung.

Artikel 30 verlangt die Rechtsgrundlage nicht, und das Verzeichnis hat eine Zeile dafür, weil du sie für die Erklärung brauchst. Behandle die Einträge dort als zu klärende Fragen.

Wenn du einen Server im Auftrag eines anderen betreibst, kannst du für ihn Auftragsverarbeiter sein. Artikel 30 Absatz 2 verlangt dann ein anderes Verzeichnis, das diese Vorlage nicht abdeckt.

<!-- source: record ed52538cf508 -->
## Verzeichnis

<!-- source: controller-article-301a 9720484d93ff -->
### Verantwortlicher (Artikel 30 Absatz 1 Buchstabe a)

| | |
| --- | --- |
| Verantwortlicher | {{NAME, POSTANSCHRIFT, E-MAIL}} |
| Gemeinsam Verantwortlicher | {{NAME UND KONTAKT, ODER „keiner“}} |
| Vertreter in der Union | {{NAME UND KONTAKT, ODER „nicht zutreffend“}} |
| Datenschutzbeauftragter | {{NAME UND KONTAKT, ODER „keiner“}} |
| Server | {{SERVERNAME UND URL}} |
| Software | my, Version {{VERSION}} |
| Zuletzt überprüft | {{DATUM, VON WEM}} |

<!-- source: recipients-used-in-this-record 8f1748f022f5 -->
### In diesem Verzeichnis verwendete Empfänger

Diese Namen werden unter den Tätigkeiten unten verwendet. Liste jeden auf, den du einsetzt.

| Empfänger | Name, Kontakt und Vertrag | Land |
| --- | --- | --- |
| Hosting (Server, Datenbank, Blob-Speicher, Queue, Schlüsselverwaltung) | {{...}} | {{...}} |
| Reverse-Proxy oder Load Balancer | {{...}} | {{...}} |
| {{IF cdn}} CDN | {{...}} | {{...}} |
| {{IF mail}} E-Mail-Anbieter | {{...}} | {{...}} |
| {{IF captcha}} CAPTCHA-Dienst | {{...}} | {{...}} |
| {{IF login providers}} Login-Anbieter | {{...}} | {{...}} |
| {{IF payments}} Zahlungsanbieter (Stripe oder PAY.JP) | {{...}} | {{...}} |
| {{IF error reports}} Fehler-Tracker | {{...}} | {{...}} |
| {{IF telemetry}} Metrik- und Tracing-Dienst | {{...}} | {{...}} |
| {{IF voice}} Voice-Media-Server, STUN und TURN | {{...}} | {{...}} |
| {{IF webhooks}} Webhook-Empfänger | {{...}} | {{...}} |
| {{IF follow review}} Follow-Prüfdienst | {{...}} | {{...}} |
| {{IF sign-up review}} Registrierungs-Prüfdienst | {{...}} | {{...}} |
| {{IF post review}} Beitrags-Prüfdienst | {{...}} | {{...}} |
| {{IF profile review}} Profil-Prüfdienst | {{...}} | {{...}} |
| {{IF media review}} Medien-Prüfdienst | {{...}} | {{...}} |
| {{IF translation}} Übersetzungsanbieter | {{...}} | {{...}} |
| {{IF external review of posts}} Externe Prüfung von Beiträgen anderer Server | {{...}} | {{...}} |
| Backup-Speicher | {{...}} | {{...}} |
| Föderierte Server | Andere Server des Netzwerks, betrieben von anderen Verantwortlichen, die sich mit der Zeit ändern. `my` führt die Liste bekannter Server | Jedes Land |
| {{IF relays}} Relays | {{JEDES RELAY, DEM DU BEIGETRETEN BIST}} | {{...}} |
| Push-Dienste | Vom Browser jedes Nutzers gewählt | Hängt vom Browser ab |
| Apps | Von jedem Nutzer autorisiert | Hängt von der App ab |

<!-- source: transfers-article-301e 82e1298af5f2 -->
### Übermittlungen (Artikel 30 Absatz 1 Buchstabe e)

- Die föderierte Zustellung sendet personenbezogene Daten an Server im Ausland, in Länder, die nicht im Voraus bekannt sind und sich nicht begrenzen lassen. {{DEINE BEURTEILUNG: ob dies eine Übermittlung nach Kapitel V DSGVO ist, und auf welcher Grundlage. Diese Vorlage lässt das offen.}}
- Übermittlungen an die Dienste oben: {{FÜR JEDEN DAS DRITTLAND UND DEN ANGEMESSENHEITSBESCHLUSS ODER DIE GARANTIE}}.

<!-- source: 1-accounts-and-signing-in 865bacb0785b -->
### 1. Konten und Anmeldung

| | |
| --- | --- |
| Zwecke | Konten anlegen, anmelden, Zugang wiederherstellen, über Registrierungen entscheiden |
| Betroffene | Leute mit einem Konto, Leute, die eines beantragen |
| Personenbezogene Daten | E-Mail-Adressen, Passwort-Hash, Zwei-Faktor-Secret und gehashte Wiederherstellungscodes, Passkeys, die Kennung bei einem Login-Anbieter, Einstellungen (Sprache, Zeitzone, Standardwerte beim Posten), Registrierungsgrund, Einladung und Entscheidung, Rollen, gehashte Sitzungs- und App-Token |
| Empfänger | Hosting, E-Mail-Anbieter, Login-Anbieter, Team mit `manage_accounts` |
| Löschung | Löschung des Kontos: 7 Tage nach dem Löschen. Ein deaktiviertes Konto wird nach 30 Tagen gelöscht. Sitzungen, Token und Codes: 30 Tage nach Ablauf. Einladungen und Rollenzuweisungen, die Konto-URI und der Nutzername bleiben. Ein Konto, das niemand löscht, bleibt |
| Rechtsgrundlage | {{z. B. Vertrag}} |

<!-- source: 2-protecting-sign-in-and-registration 66d632cbf187 -->
### 2. Anmeldung und Registrierung schützen

| | |
| --- | --- |
| Zwecke | Missbrauch verhindern, Konten derselben Person bei der Moderation finden, Ratenbegrenzung, Bots stoppen |
| Betroffene | Leute mit einem Konto, Antragsteller, Besucher der Anmeldeseiten |
| Personenbezogene Daten | IP-Adresse der Registrierung, IP-Adresse und User-Agent der Anmeldung, IP-Adresse (und bei Anfragen zum Zurücksetzen des Passworts die E-Mail-Adresse) in Zählern der Ratenbegrenzung, Registrierungssperren (E-Mail-Domain, gehashte Adresse, IP-Bereich). Mit CAPTCHA: IP-Adresse, Browser- und Geräteinformationen, Cookies. Mit einer Registrierungsprüfung: die E-Mail-Adresse und IP-Adresse jedes Antragstellers ohne Einladung |
| Empfänger | Hosting, Team mit `manage_accounts` (jede Offenlegung steht im Audit-Log), CAPTCHA-Dienst, Registrierungs-Prüfdienst |
| Löschung | IP-Adressen der Registrierung `{{180 days}}`, Anmeldeherkünfte `{{180 days}}`, Zähler innerhalb des Doppelten des Zeitfensters der Begrenzung, Registrierungssperren 30 Tage nach Aufheben oder Ablauf. Durch eine Aufbewahrungssperre gestoppt |
| Rechtsgrundlage | {{z. B. berechtigte Interessen}} |

<!-- source: 3-publishing-and-social-features 5e271f0279f9 -->
### 3. Veröffentlichen und soziale Funktionen

| | |
| --- | --- |
| Zwecke | Profile und Beiträge veröffentlichen und die Interaktionen, um die Leute bitten |
| Betroffene | Leute mit einem Konto, Leute, die sie erwähnen |
| Personenbezogene Daten | Profil und seine Versionen, Beiträge und ihre Versionen, Medien, Favoriten, Reaktionen, Boosts, Lesezeichen, angeheftete Beiträge, Umfragestimmen, Follows, Blockierungen, Stummschaltungen, Listen. Beiträge können beliebige Daten enthalten, die eine Person schreibt, auch besondere Kategorien. `{{IF post review}}` Der Text eines abgelehnten Beitrags, den sein Autor mit einem Einspruch sendet |
| Empfänger | Hosting, CDN, Medien-Prüfdienst (eine kleine verarbeitete Kopie jedes Uploads), andere Nutzer, das öffentliche Web für öffentliche Inhalte, föderierte Server (Tätigkeit 4), Suchmaschinen und KI-Crawler gemäß `search_indexing` und `ai_usage`, `{{the operator of the full-text search engine, if another company runs it}}`, von der Person autorisierte Apps, `{{IF post review}}` der Beitrags-Prüfdienst für die `public`-, `local`- und `unlisted`-Beiträge neuer Konten und ihre Bearbeitungen (ihr Text wie eingegeben und wie gerendert, Inhaltswarnung, Sprache, Sensibel-Markierung, Umfrageoptionen, Bildbeschreibungen, Hashtags und Zahl der Erwähnungen, ihre Adresse und die Adressen der Beiträge, auf die sie antworten oder die sie zitieren, die Adresse und der Nutzername des Autors und wann das Konto angelegt wurde; keine Medien), `{{IF profile review}}` der Profil-Prüfdienst für die Profiländerungen neuer Konten (der Anzeigename, die Bio wie eingegeben und wie gerendert, die Profilfelder, ob Avatar oder Header geändert wurden, die Adresse und der Nutzername des Kontos und wann das Konto angelegt wurde; keine Bilder) |
| Löschung | Wenn die Person ihn löscht, durch ihre automatische Löschrichtlinie oder bei der Löschung des Kontos. `{{IF auto_delete_max_age_days}}` Höchstens `{{auto_delete_max_age_days}}` Tage nach der Veröffentlichung, bei Konten ohne `keep_posts_longer`. An nichts angehängte Uploads `{{24 hours}}`. Alte Versionen von Bildern, nachdem eine neuere Version sie ersetzt hat, `{{30 days}}`. Der Eintrag der URI eines gelöschten Beitrags bleibt. `{{IF post review}}` Ein Einspruch geht mit seiner Entscheidung (`hook_decisions_retention_days`, behalten, solange gegen ihn Einspruch möglich ist oder er auf Prüfung wartet) oder bei der Löschung des Kontos |
| Rechtsgrundlage | {{z. B. Vertrag}} |

<!-- source: 4-federation 795a56004c37 -->
### 4. Föderation

| | |
| --- | --- |
| Zwecke | Lokale Aktivität an andere Server zustellen und ihre empfangen |
| Betroffene | Leute mit einem Konto, Leute auf anderen Servern |
| Personenbezogene Daten | Gesendet: Actor-Dokument, Beiträge, Follows, Blockierungen, Boosts, Likes, Reaktionen, Umfragestimmen, Zitate, Umzüge, weitergeleitete Meldungskommentare (ohne den Melder), Änderungen des Lebenszyklus. Behalten: Kopien gesendeter und empfangener Aktivitäten, Zustellversuche |
| Empfänger | Föderierte Server, Relays, Follow-Prüfdienst, Profil-Prüfdienst |
| Löschung | Hier: Empfangenes 30 Tage und ihre Dokumente 7 Tage. Gesendetes 30 Tage, und die Zustellung eines Beitrags bis 30 Tage nach dem Löschen des Beitrags oder seines Autors. Dort: nicht unter deiner Kontrolle. `Delete` wird gesendet, und von anderen Servern wird erwartet, aber nicht erzwungen, dass sie ihm folgen |
| Rechtsgrundlage | {{z. B. Vertrag, berechtigte Interessen}} |

<!-- source: 5-storing-data-of-people-on-other-servers 3708a84995bb -->
### 5. Daten von Leuten auf anderen Servern speichern

| | |
| --- | --- |
| Zwecke | Entfernte Profile und Beiträge lokalen Nutzern zeigen |
| Betroffene | Leute auf anderen Servern, die meist keine Beziehung zu dir haben |
| Personenbezogene Daten | Actor-Dokumente, Profilversionen, Handles, Beiträge, Medienreferenzen, Kopien von Medien, Interaktionen |
| Empfänger | Hosting, CDN, lokale Nutzer, öffentliche Seiten, `{{IF external review of posts}}` die externe Prüfung von Beiträgen anderer Server für die Beiträge und Revisionen, die sich auf ein lokales Konto beziehen, einschließlich Beiträgen nur für Follower (`private`), und Direktbeiträge nur, wenn ein lokales Konto dem Autor folgt, es sei denn, `hook_inbound_status_unfollowed_direct` ist eingeschaltet (ihr Text und ihre Sprachen, Inhaltswarnung, Sensibel-Markierung, Veröffentlichungszeit, Adresse und URL, die Adresse des Beitrags, auf den sie antworten, Umfrageoptionen, Hashtags, Adressen und Beschreibungen der Anhänge, der Autor und sein Server, Sichtbarkeit, die Zahl der genannten Konten und der lokalen darunter und ob ein lokales Konto dem Autor folgt) |
| Löschung | Beiträge `{{90 days}}`, wenn nichts hier auf sie verweist, `{{7 days}}`, wenn nur ein Relay sie gebracht hat, und `{{no limit}}` sonst für jeden Beitrag. Kopien von Medien `{{not kept}}`. Handle-Prüfungen 90 Tage. Alles zu einem Konto 7 Tage nachdem sein Server es löscht, oder bis zu 30 Tage, wenn nur ein `410 Gone` es sagte und der Server nicht erneut antwortet. Seine URI und Domain bleiben |
| Rechtsgrundlage | {{z. B. berechtigte Interessen}} |

<!-- source: 6-notifications ad34beea6ab2 -->
### 6. Benachrichtigungen

| | |
| --- | --- |
| Zwecke | Leute per E-Mail und Web-Push über Aktivität und ihr Konto informieren |
| Betroffene | Leute mit einem Konto, Leute, die in einer Benachrichtigung genannt werden |
| Personenbezogene Daten | Benachrichtigungen und Gelesen-Markierungen, Benachrichtigungseinstellungen, E-Mail-Adresse, der Text jeder E-Mail, Bounce- und Complaint-Aufzeichnungen, Web-Push-Adresse und -Schlüssel, Beitragsabonnements |
| Empfänger | Hosting, E-Mail-Anbieter, Push-Dienste (der Nachrichtentext ist verschlüsselt, und sie halten ihn bis zu 48 Stunden) |
| Löschung | Bei der Löschung des Kontos oder mit dem Beitrag, um den es geht. Web-Push: auch, wenn die Person oder der Push-Dienst es beendet. Der E-Mail-Anbieter: {{SEINE FRIST}} |
| Rechtsgrundlage | {{z. B. Vertrag, berechtigte Interessen}} |

<!-- source: 7-moderation-and-audit d72a42b6312e -->
### 7. Moderation und Audit

| | |
| --- | --- |
| Zwecke | Meldungen, Maßnahmen und Einsprüche bearbeiten, nachweisen, was das Team getan hat, Sperren für rechtliche Anfragen |
| Betroffene | Gemeldete Leute, Melder, Team, Leute, die in einem Kommentar oder einer Notiz genannt werden |
| Personenbezogene Daten | Meldungen (Kommentar, Beiträge, Regeln, Notizen, Zuweisungen, Abschlüsse), Maßnahmen (Begründung, Notiz, Beiträge), Einsprüche, Audit-Log (Akteur, Aktion, der Name des Ziels), Sperren und ihr Grund, als Beweis behaltene Beiträge. Kann Strafdaten oder besondere Kategorien enthalten, wenn eine Meldung oder ein Beitrag sie enthält |
| Empfänger | Hosting, Team mit der passenden Berechtigung, die betroffene Person über ihre eigenen Moderationsaufzeichnungen (nie die Identität des Melders), der Server des gemeldeten Kontos bei einer weitergeleiteten Meldung |
| Löschung | Meldungen `{{365 days}}` ab dem letzten Abschluss oder mit einer Maßnahme, die auf sie verweist. Maßnahmen und Einsprüche `{{365 days}}` ab dem Ende der Maßnahme. Audit-Log `{{730 days}}`. Offene Meldungen und Maßnahmen ohne Ende bleiben. Eine aufgehobene Sperre geht nach der Frist für Maßnahmen |
| Rechtsgrundlage | {{z. B. berechtigte Interessen, rechtliche Verpflichtung}} |

<!-- source: 8-requests-from-people 762c1e8bc31c -->
### 8. Anfragen von Leuten

| | |
| --- | --- |
| Zwecke | Anfragen auf Auskunft und Übertragbarkeit beantworten und Importe |
| Betroffene | Leute mit einem Konto |
| Personenbezogene Daten | Export-Archive (Actor, Beiträge, eigene Moderationsaufzeichnungen), CSV-Dateien, Importe und ihre Ergebnisse, der Schriftverkehr einer Anfrage |
| Empfänger | Hosting, die Person |
| Löschung | Archive und Importe 7 Tage. Schriftverkehr: {{FRIST}} |
| Rechtsgrundlage | {{z. B. rechtliche Verpflichtung, Vertrag}} |

Der Export des Nutzers selbst enthält keine E-Mail-Adressen, IP-Adressen, Benachrichtigungen oder Einstellungen. Für eine vollständige Antwort verwende `my admin account export-moderation` für Moderationsaufzeichnungen und [Offenlegen](moderation.md#account-management) für E-Mail- und IP-Adressen, und lies den Rest aus der Datenbank. Der Moderationsexport lässt die Identität der Melder aus: Prüfe, dass ein Kommentar keine verrät, bevor du ihn übergibst.

<!-- source: 9-voice-chat e0bf1be42da8 -->
### 9. Sprachchat

Lösche diese Tätigkeit, wenn `voice = off`.

| | |
| --- | --- |
| Zwecke | Sprachchat-Räume betreiben und sie moderieren |
| Betroffene | Hosts und Teilnehmer |
| Personenbezogene Daten | Raumname, Beschreibung, Hosts und wie der Raum endete; wer teilgenommen, gesprochen, die Hand gehoben hat, eingeladen oder entfernt wurde. IP-Adressen und Audio laufen über den Voice-Media-Server oder TURN-Server, oder erreichen andere Teilnehmer mit `voice = p2p` direkt. Der Server zeichnet kein Audio auf |
| Empfänger | Hosting, Voice-Media-Server, STUN und TURN, Teilnehmer |
| Löschung | Teilnehmer `{{90 days}}` ab dem Ende des Raums, oder so lange wie eine Meldung dazu. Räume bleiben, wobei Name und Beschreibung bei der Löschung des Autors geleert werden. Bei den Diensten: {{IHRE FRIST}} |
| Rechtsgrundlage | {{z. B. Vertrag, berechtigte Interessen}} |

<!-- source: 10-supporter-payments 99d64d001780 -->
### 10. Zahlungen von Unterstützern

Lösche diese Tätigkeit, wenn `payments = none`.

| | |
| --- | --- |
| Zwecke | Abos annehmen, Rollen erteilen, Buchhaltung |
| Betroffene | Abonnenten |
| Personenbezogene Daten | Objekt-IDs des Anbieters, Beträge, Zeiträume, Status, Erstattungen, Rückbuchungen, die Entscheidung, aufgelistet zu werden. An den Anbieter gesendete E-Mail-Adresse. Keine Kartennummern, letzten vier Ziffern oder Ablaufdaten |
| Empfänger | Hosting, Zahlungsanbieter, Leute, die die Liste der Unterstützer sehen können, Team mit der passenden Berechtigung |
| Löschung | Webhook-Belege 45 Tage. Die Kundenverknüpfung und der Listeneintrag bei der Löschung des Kontos. Abo- und Zahlungsaufzeichnungen haben kein automatisches Ende: {{FRIST UND DAS GESETZ, DAS SIE VERLANGT}}. Beim Anbieter: {{SEINE FRIST, UND OB DU DEN KUNDEN LÖSCHST}} |
| Rechtsgrundlage | {{z. B. Vertrag, rechtliche Verpflichtung}} |

<!-- source: 11-running-the-service 9c89577f9cee -->
### 11. Den Dienst betreiben

| | |
| --- | --- |
| Zwecke | Fehler finden, nach Ausfällen wiederherstellen, eigene Werkzeuge anbinden |
| Betroffene | Alle oben |
| Personenbezogene Daten | Logs (Pfade, die einen Nutzernamen oder Beitrag nennen können, Fehlertext), Proxy-Logs mit IP-Adressen, nach Muster bereinigte Fehlerberichte, Job-Aufzeichnungen mit IDs, Webhook-Bodys, Backups von allem |
| Empfänger | Hosting, Reverse-Proxy, Fehler-Tracker, Metrik- und Tracing-Dienst, Webhook-Empfänger, Backup-Speicher |
| Löschung | Fehlgeschlagene Jobs und Lauf-Aufzeichnungen `{{30 days}}`. Webhook-Nachrichten 14 Tage. Stündliche Fehlerzähler 90 Tage. Anwendungs- und Proxy-Logs: {{FRIST}}. Der letzte Bericht jeder Fehlergruppe: durch den nächsten ersetzt, ohne automatisches Ende. Backups: {{FRIST}}. Beim Fehler-Tracker und den Empfängern: {{IHRE FRIST}} |
| Rechtsgrundlage | {{z. B. berechtigte Interessen}} |

<!-- source: 12-translating-posts 17a9b7a81356 -->
### 12. Beiträge übersetzen

Lösche diese Tätigkeit, wenn `translation_providers` leer ist.

| | |
| --- | --- |
| Zwecke | Einen Beitrag übersetzen, wenn ein Leser darum bittet, {{IF translation of reports}} eine Meldung übersetzen, wenn ein Moderator darum bittet, und die Übersetzung in ihren Grenzen halten |
| Betroffene | Autoren von Beiträgen, hier und auf anderen Servern, und Leute, die der Text nennt. Leser, für die Zahl der Zeichen. {{IF translation of reports}} Melder, für den Text ihrer Kommentare |
| Personenbezogene Daten | Was auch immer der Text eines Beitrags enthält: sein Inhalt, Inhaltswarnung, Umfrageoptionen und Bildbeschreibungen. Die Adresse des Beitrags, sein Autor, der Leser, Erwähnungen und Links werden vor dem Senden ersetzt oder weggelassen. Behalten: die Übersetzungen und die Zeichen pro Konto und Monat |
| Empfänger | Übersetzungsanbieter, als deine Auftragsverarbeiter: standardmäßig `public`-, `local`- und `unlisted`-Beiträge. {{IF translation to private and direct}} Auch `private`- und `direct`-Beiträge, für {{DIE ANBIETER, DIE DU ZUGELASSEN HAST}}. {{IF translation of reports}} Die Kommentare von Meldungen, für die Anbieter, denen `private`-Beiträge erlaubt sind, und die Beiträge, die Meldungen nennen, wenn ein Moderator darum bittet |
| Löschung | Übersetzungen `{{30 days}}` ab der Übersetzung und mit dem Beitrag, wenn er gelöscht wird. Zeichen pro Konto bis zum Ende des Folgemonats oder bei der Löschung des Kontos. Bei den Anbietern: Was gesendet wurde, lässt sich nicht zurückrufen. {{IHRE FRISTEN}} |
| Rechtsgrundlage | {{z. B. berechtigte Interessen}} |

<!-- source: security-measures-article-301g 756bb9e562e3 -->
### Sicherheitsmaßnahmen (Artikel 30 Absatz 1 Buchstabe g)

Was die Software tut:

- Passwörter werden als Argon2id-Hashes gespeichert. Sitzungs-, Zugriffs- und Einmal-Token sowie Wiederherstellungscodes werden als Hashes gespeichert.
- Private Signaturschlüssel, Zwei-Faktor-Secrets sowie Anbieter- und Webhook-Secrets werden mit AES-256-GCM unter einem Key-Encryption-Key verschlüsselt, der außerhalb der Datenbank liegt ([Schlüsselverwaltung](key-management.md)).
- E-Mail- und IP-Adressen werden nur dem Team mit `manage_accounts` durch eine ausdrückliche Aktion gezeigt, die das Audit-Log festhält.
- Anfragen zu Anmeldung, Registrierung und Zurücksetzen des Passworts sind ratenbegrenzt. Berechtigungen des Teams laufen über Rollen.
- Ausgehende Anfragen, die entfernte Server beeinflussen können, laufen durch einen Schutz davor, interne Adressen zu erreichen.

Was du tust:

- TLS am Reverse-Proxy: {{KONFIGURATION}}
- Verschlüsselung von Datenbank, Blob-Speicher und Backups im Ruhezustand: {{MASSNAHMEN}}
- Wer den Host, die Datenbank und den KEK erreichen kann, und wie der Zugriff überprüft wird: {{MASSNAHMEN}}
- Konten des Teams, Rollen und Zwei-Faktor-Authentifizierung: {{RICHTLINIE}}
- Updates des Servers und seines Hosts: {{RICHTLINIE}}
- Umgang mit einer Verletzung des Schutzes personenbezogener Daten: {{VERFAHREN UND KONTAKT}}
