<!-- source: signing-up-and-signing-in e881fcb0feaa -->
# Registrieren und anmelden

So erstellst du ein Konto auf einem mycroblog-Server, also einem Server, auf dem `my` läuft, und meldest dich dort an. Jeder Server wird von seinen eigenen Betreibern geführt, daher
bestimmen diese, was ein Server anbietet (wer sich registrieren darf, welche Timelines Besuchende lesen können, ob der Sprachchat aktiviert ist). Die Seite
„Info“ des Servers und die Dokumente, die er bei der Registrierung verlinkt, nennen seine Regeln; diese Anleitung beschreibt nur, wie sich `my` verhält.

<!-- source: creating-an-account 965073ac4cf5 -->
## Ein Konto erstellen

Wähle in der Navigation „Registrieren“ (oder auf der Anmeldeseite „Konto erstellen“). Das Formular fragt nach:

- **Benutzername**: Buchstaben (a-z), Ziffern und Unterstriche. Dein Handle ist `@username@this-server`, und den Benutzernamen kannst du später nicht mehr ändern.
- **E-Mail-Adresse**: Mit ihr meldest du dich an.
- **Passwort**: Das Formular zeigt unter dem Feld die erlaubte Länge.
- **Einladungscode**, wenn der Server einen verlangt oder akzeptiert.
- **Warum möchtest du beitreten? (optional)**, wenn der Server neue Konten prüft.
- Ein Kästchen, mit dem du bestätigst, dass du die Nutzungsbedingungen und Richtlinien des Servers gelesen hast, wenn der Server welche veröffentlicht.

Was davon du siehst, hängt davon ab, wie der Server neue Konten annimmt:

| Der Server | Was du tust |
| --- | --- |
| Ist offen | Registriere dich. Ein Einladungscode ist optional. |
| Prüft neue Konten | Registriere dich und sage, warum du beitreten möchtest. Ein Moderator genehmigt das Konto, bevor du es nutzen kannst. Mit einem Einladungscode kannst du das Konto sofort nutzen. |
| Nimmt nur auf Einladung auf | Gib den Einladungscode ein, den du erhalten hast, oder öffne den Einladungslink, der ihn einträgt. |
| Ist geschlossen | Die Seite sagt, dass der Server keine neuen Konten annimmt. |
| Registriert nur über Anmeldeanbieter | Du registrierst dich statt mit einem Passwort bei einem Anbieter, den der Server anbietet (zum Beispiel „Mit GitHub anmelden“). |

Wenn der Server E-Mails versendet, schickt er dir einen Link zum Bestätigen deiner E-Mail-Adresse. Öffne ihn und wähle „E-Mail-Adresse bestätigen“.
Solange das Konto nicht genehmigt und die Adresse nicht bestätigt ist, kannst du dich anmelden, siehst aber nur, worauf du wartest
(„Dein Konto ist noch nicht bereit“).

Nach der Registrierung kannst du auf einer Willkommensseite einen Avatar und einen Anzeigenamen wählen („Weiter“ speichert sie, „Später“ überspringt das).
Deine Home-Timeline zeigt dann eine Checkliste „Erste Schritte“: „Profil einrichten“, „Jemandem folgen“ und „Ersten Beitrag verfassen“.
„Diese Schritte ausblenden“ schließt sie in diesem Browser. „Vorgeschlagene Konten“ listet Personen auf, denen du folgen kannst, außer auf einem Server
mit nur einer Person, wo stattdessen „Personen finden“ die Suche öffnet.

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

<!-- source: signing-out fb81956ae999 -->
## Abmelden

Wähle „Abmelden“ im Bereich „Konto“ der App. Die Seite „Angemeldete Browser“ der Einstellungen listet die Browser auf,
die bei deinem Konto angemeldet sind, und meldet sie ab.

<!-- source: settings 35cf43b05962 -->
## Einstellungen

Die Einstellungen sind gegliedert in „Profil und Community“, „Anzeige und Benachrichtigungen“, „Beziehungen“, „Sicherheit“, „Apps“
und „Konto und Daten“. Unter „Sicherheit“ verwaltest du dein Passwort, die Zwei-Faktor-Authentifizierung, Passkeys, verknüpfte Konten
und angemeldete Browser.
