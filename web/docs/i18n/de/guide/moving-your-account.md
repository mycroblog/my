<!-- source: moving-your-account 95d8d22a22bb -->
# Dein Konto umziehen

Du kannst dein Konto auf ein anderes Konto umziehen, auf demselben oder auf einem anderen Server. Deine Follower folgen dem neuen Konto, und das alte Konto weist darauf hin, dass es umgezogen ist. Dasselbe funktioniert in die andere Richtung, wenn du von einem anderen Server auf diesen Server umziehst.

<!-- source: what-moves-and-what-stays a45d57421940 -->
## Was umzieht und was bleibt

| Zieht um | Bleibt beim alten Konto |
| --- | --- |
| Deine Follower | Deine Beiträge, die Konten, denen du folgst, deine Sperren, Stummschaltungen und Lesezeichen |

![Links das alte Konto, rechts das neue Konto: ein Alias, der Umzug, deine Follower, die zum neuen Konto wechseln, und was beim alten Konto bleibt. Die Liste unten sagt dasselbe.](images/account-move.svg)

Von oben nach unten im Diagramm:

1. **Einen Alias hinzufügen.** Füge im neuen Konto das alte Konto unter „Aliasse“ hinzu.
2. **Konto umziehen.** Wähle im alten Konto „Konto umziehen“ und nenne das neue Konto.
3. **Was umzieht.** Deine Follower folgen dem neuen Konto. Das alte Konto sagt „Dieses Konto ist umgezogen nach …“: Du kannst dich dort weiterhin anmelden, lesen und deine Daten exportieren.
4. **Was hier bleibt.** Deine Beiträge, die Konten und Hashtags, denen du folgst, und deine Sperren, Stummschaltungen und Lesezeichen bleiben beim alten Konto. Deine Beiträge ziehen nicht um. Nimm den Rest als CSV-Dateien mit, mit „Exportieren“ im alten Konto und „Importieren“ im neuen ([deine Folgen mitnehmen](#taking-your-follows-with-you)).

<!-- source: moving-from-this-account 8470ed8a1f13 -->
## Von diesem Konto umziehen

1. Füge im neuen Konto dieses Konto als Alias hinzu: „Aliasse“ unter „Konto und Daten“ in den Einstellungen, unter „Alias hinzufügen“. „Konto umziehen“ zeigt das Handle dieses Kontos, das du dort eingibst.
2. Öffne in diesem Konto „Konto umziehen“ unter „Konto und Daten“ in den Einstellungen, was sagt, was umzieht und was bleibt. Gib unter „Konto, zu dem du umziehst“ das Handle des neuen Kontos (wie `@name@example.com`) oder die Adresse seines Profils ein und wähle „Konto umziehen“.
3. Bestätige, dass du es bist, mit deinem Passwort, einem Passkey oder einem verknüpften Anbieter. Ein Umzug, den der Server ablehnt, nennt den Grund.

Nach dem Umzug gilt für das alte Konto:

- Es sagt, wohin es umgezogen ist, auf dieser Seite und auf seinem Profil. Du kannst dich weiterhin anmelden, lesen, deine Daten exportieren und das Konto löschen, aber nicht beitragen, boosten, reagieren, folgen oder abstimmen.
- Es kann 30 Tage lang nicht erneut umziehen.

„Umzug abbrechen“ auf derselben Seite holt das alte Konto zurück: Es kann wieder Beiträge verfassen und folgen, und andere Server
werden informiert. Follower, die schon umgezogen sind, bleiben beim neuen Konto.

<!-- source: deactivating-or-deleting-this-account 599007a2d409 -->
## Dieses Konto deaktivieren oder löschen

Unter „Konto und Daten“ in den Einstellungen blendet „Konto deaktivieren“ das Konto aus und meldet dich überall ab: Melde dich
bis zu dem Datum, das die Seite nennt, wieder an, um es zurückzuholen, sonst wird es dann gelöscht. „Konto löschen“ löscht es sofort und
endgültig. „Exportiere“ vorher deine Daten.

<!-- source: moving-to-this-account 4b769e76357c -->
## Zu diesem Konto umziehen

Füge das Konto, von dem du umziehst, als Alias unter „Aliasse“ in den Einstellungen dieses Kontos hinzu („Alias hinzufügen“). Ein Alias allein
ändert nichts; er erlaubt dem anderen Konto nur, seine Follower zu dir umzuziehen. Du kannst 5 Aliasse haben, und
„Entfernen“ löscht einen.

<!-- source: taking-your-follows-with-you 09ca5e285436 -->
## Deine Folgen mitnehmen

„Exportieren“ und „Importieren“ unter „Konto und Daten“ in den Einstellungen exportieren und importieren CSV-Dateien im Format, das Mastodon verwendet.
Du kannst „Folgt“, „Gefolgte Hashtags“, „Gesperrte Konten“, „Stummgeschaltete Konten“, „Stummgeschaltete Server“ und „Lesezeichen“ exportieren und
im neuen Konto importieren. Mastodon hat keine Datei für gefolgte Hashtags, daher importiert diese Datei nur ein Server, der dieselbe Software wie
dieser ausführt. „Zusammenführen“ behält, was du hast, und fügt hinzu, was die Datei auflistet; „Ersetzen“ macht die Liste genau zu dem, was die Datei
auflistet. „Vorschau“ zeigt, was eine Datei bewirken würde, bevor du „Importieren“ wählst; der Import läuft dann im Hintergrund.

„Archiv anfordern“ unter „Exportieren“ bereitet ein ZIP-Archiv deiner Beiträge, deines Profils, deiner Stichwortfilter, deiner privaten Notizen und deiner
Folgen, gefolgten Hashtags, Sperren, Stummschaltungen, stummgeschalteten Server und Lesezeichen vor, das du dort herunterlädst, sobald es bereit ist.

<!-- source: when-an-account-you-follow-moves 81ab63afe43d -->
## Wenn ein Konto, dem du folgst, umzieht

Du wirst benachrichtigt („… ist zu einem anderen Konto umgezogen“). Wenn du dem neuen Konto folgen kannst, folgst du ihm sofort, oder deine Anfrage wartet auf seine Genehmigung. Du folgst ihm nicht, wenn du es blockierst, wenn es dich blockiert oder wenn du seinen Server stummgeschaltet hast; die Benachrichtigung sagt, welcher Fall vorliegt.
