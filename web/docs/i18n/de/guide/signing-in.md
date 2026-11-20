<!-- source: signing-up-and-signing-in e881fcb0feaa -->
# Registrieren und anmelden

So erstellst du ein Konto auf einem mycroblog-Server, also einem Server, auf dem `my` läuft, und meldest dich dort an. Jeder Server wird von seinen eigenen Betreibern geführt, daher
bestimmen diese, was ein Server anbietet (wer sich registrieren darf, welche Timelines Besuchende lesen können, ob der Sprachchat aktiviert ist). Die Seite
„Info“ des Servers und die Dokumente, die er bei der Registrierung verlinkt, nennen seine Regeln; diese Anleitung beschreibt nur, wie sich `my` verhält.

<!-- source: creating-an-account 6f66cdf29cd2 -->
## Ein Konto erstellen

Wähle in der Navigation „Registrieren“ (oder auf der Anmeldeseite „Konto erstellen“). Das Formular fragt nach:

- **Benutzername**: Buchstaben (a-z), Ziffern und Unterstriche. Dein Handle ist `@username@this-server`, und du kannst den Benutzernamen später nicht ändern.
- **E-Mail-Adresse**: Damit meldest du dich an.
- **Passwort**: Das Formular zeigt die erlaubte Länge unter dem Feld.
- **Einladungscode**, wenn der Server einen verlangt oder akzeptiert.
- **Warum möchtest du beitreten? (optional)**, wenn der Server neue Konten prüft.
- Ein Kästchen, mit dem du bestätigst, dass du die Bedingungen und Richtlinien des Servers gelesen hast, wenn der Server welche veröffentlicht.

Was davon du siehst, hängt davon ab, wie der Server neue Konten annimmt:

| Der Server | Was du tust |
| --- | --- |
| Ist offen | Registriere dich. Ein Einladungscode ist optional. |
| Prüft neue Konten | Registriere dich und sage, warum du beitreten möchtest. Ein Moderator genehmigt das Konto, bevor du es nutzen kannst. Mit einem Einladungscode kannst du das Konto sofort nutzen. |
| Nimmt nur Einladungen an | Gib den Einladungscode ein, den du erhalten hast, oder öffne den Einladungslink, der ihn einträgt. |
| Ist geschlossen | Die Seite sagt, dass der Server keine neuen Konten annimmt. |
| Registriert nur über Anmeldeanbieter | Du registrierst dich bei einem Anbieter, den der Server anbietet (zum Beispiel „Mit GitHub anmelden“), statt mit einem Passwort. |

Sendet der Server E-Mails, schickt er einen Link zur Bestätigung deiner E-Mail-Adresse. Öffne ihn und wähle „E-Mail-Adresse bestätigen“.
Solange das Konto nicht genehmigt und die Adresse nicht bestätigt ist, kannst du dich anmelden, siehst aber nur, worauf du wartest
(„Dein Konto ist noch nicht bereit“).

Nach der Registrierung fragt ein Willkommensschritt nach einem Anzeigenamen und einem Avatar, beides optional („Vorerst überspringen“); solange du niemandem folgst, zeigt deine
Startseite dann Personen, die der Server vorschlägt, falls er welche vorschlägt.

<!-- source: signing-in fcfbfa76f1d0 -->
## Anmelden

Wähle in der Navigation „Anmelden“. Der Webclient leitet dich zur Anmeldeseite des Servers weiter und bringt dich angemeldet
zurück. Auf dieser Seite kannst du:

- Deine E-Mail-Adresse und dein Passwort eingeben.
- „Mit einem Passkey anmelden“ wählen, sobald du in den Einstellungen unter „Passkeys“ einen Passkey hinzugefügt hast.
- Eine Schaltfläche unter „Oder melde dich an mit“ wählen, wenn der Server Anmeldeanbieter anbietet und du in den Einstellungen unter „Verknüpfte Konten“ ein Konto dieses Anbieters verknüpft hast.
- „Passwort vergessen?“ wählen, um einen Link zum Festlegen eines neuen Passworts zu erhalten. Das Festlegen eines neuen Passworts meldet dich überall ab. Ein Server, der keine E-Mails versendet, kann den Link nicht senden: Frage seine Administratoren.

Wenn du die Zwei-Faktor-Authentifizierung eingeschaltet hast, fragt die Seite danach den sechsstelligen Code deiner Authenticator-App oder einen
deiner Wiederherstellungscodes ab. Nach zu vielen fehlgeschlagenen Versuchen lässt dich die Seite einige Minuten warten und sagt, wie lange.

<!-- source: an-account-that-cannot-be-used-yet 8e6bc1791f14 -->
### Ein Konto, das noch nicht genutzt werden kann

Nach dem Anmelden siehst du möglicherweise statt der App eine Seite:

- Deine Registrierung wartet auf die Genehmigung eines Moderators oder wurde nicht genehmigt.
- Du musst deine E-Mail-Adresse bestätigen. „Link erneut senden“ schickt einen neuen Link.
- Dein Konto ist von einem Moderator eingeschränkt. Die Seite listet die geltenden Maßnahmen auf und lässt dich einmal Einspruch einlegen („Einspruch gegen diese Maßnahme einlegen“). Dieselben Maßnahmen stehen in den Einstellungen unter „Kontostatus“.
- Du hast das Konto deaktiviert. „Konto reaktivieren“ holt es bis zum Datum der Löschung zurück; „Konto löschen“ löscht es sofort.

<!-- source: signing-out 053a33c01d8e -->
## Abmelden

Wähle „Abmelden …“, die letzte Zeile der Einstellungen, und bestätige mit „Abmelden“. Das meldet diesen Browser ab; deine anderen
Browser und Apps bleiben angemeldet. „Angemeldete Browser“ unter „Sicherheit“ in den Einstellungen listet die Browser auf, die in deinem Konto angemeldet
sind, und meldet sie ab.

<!-- source: settings 4cd29f4ed978 -->
## Einstellungen

„Einstellungen“ in der Navigation öffnet die Einstellungen. Sie enthalten „Profil“, das ändert, wie dein Konto aussieht und wo
es gefunden wird; „Anzeige und Verfassen“; „Benachrichtigungen“; „Beziehungen“, das die Konten auflistet, die du sperrst und stummschaltest, die
Server, die du stummschaltest, und die Hashtags, denen du folgst, jeweils mit der Schaltfläche, die es rückgängig macht; „Stichwortfilter“, das deine
Filter auflistet und hinzufügt, ändert und löscht; „Sicherheit“, wo du dein Passwort, deine Passkeys, die Zwei-Faktor-Authentifizierung, verknüpfte Konten und angemeldete Browser
verwaltest; „Apps“, das die Apps auflistet, denen du erlaubt hast, dein Konto zu nutzen,
die du widerrufen kannst, und die Apps, die du registriert hast; „Einladungen“, wenn du Einladungen erstellen kannst, was Einladungslinks erstellt
und widerruft; und „Konto und Daten“, das „Kontostatus“, deine Daten und Beiträge sowie den Umzug, die Deaktivierung oder
das Löschen des Kontos enthält ([Dein Konto umziehen](moving-your-account.md)).
