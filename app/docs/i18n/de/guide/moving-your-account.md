<!-- source: moving-your-account 95d8d22a22bb -->
# Dein Konto umziehen

Du kannst dein Konto auf ein anderes Konto umziehen, auf demselben oder auf einem anderen Server. Deine Follower folgen dem neuen Konto, und das alte Konto weist darauf hin, dass es umgezogen ist. Dasselbe funktioniert in die andere Richtung, wenn du von einem anderen Server auf diesen Server umziehst.

<!-- source: what-moves-and-what-stays 4ea7b64d67bd -->
## Was umzieht und was bleibt

| Zieht um | Bleibt auf dem alten Konto |
| --- | --- |
| Deine Follower | Deine Beiträge, die Konten, denen du folgst, deine Blockierungen, Stummschaltungen und Lesezeichen |

![Links das alte Konto, rechts das neue Konto: ein Alias, der Umzug, deine Follower, die zum neuen Konto wechseln, und was auf dem alten Konto bleibt. Die Liste unten sagt dasselbe.](images/account-move.svg)

Vom oberen Rand des Diagramms nach unten:

1. **Alias hinzufügen.** Füge auf dem neuen Konto das alte Konto unter „Aliase“ hinzu.
2. **Konto umziehen.** Wähle auf dem alten Konto „Konto umziehen“ und gib das neue Konto an.
3. **Was umzieht.** Deine Follower folgen dem neuen Konto. Das alte Konto zeigt „Dein Konto ist umgezogen“: Du kannst dich dort weiterhin anmelden, lesen und deine Daten exportieren.
4. **Was hier bleibt.** Deine Beiträge, die Konten, denen du folgst, sowie deine Blockierungen, Stummschaltungen und Lesezeichen bleiben auf dem alten Konto. Deine Beiträge ziehen nicht um. Den Rest nimmst du selbst mit: Exportiere deine Follows, Blockierungen, Stummschaltungen und Lesezeichen als CSV-Dateien über „Import und Export“ auf dem alten Konto und importiere sie auf dem neuen ([Follows mitnehmen](#taking-your-follows-with-you)).

<!-- source: moving-from-this-account 7836944c7996 -->
## Von diesem Konto umziehen

1. Füge auf dem neuen Konto dieses Konto als Alias hinzu: „Aliase“ unter „Konto und Daten“ in den Einstellungen, mit „Alias hinzufügen“. Die Seite zum Umzug zeigt die Adresse dieses Kontos, die du dort einträgst.
2. Öffne auf diesem Konto „Konto umziehen“ unter „Konto und Daten“ in den Einstellungen. Gib den Handle des neuen Kontos ein (einen Handle wie `@name@example.com` oder die Adresse des Kontos) und wähle „Weiter“.
3. Der Client zeigt, was umzieht und was bleibt. Gib zur Bestätigung dein Passwort ein (oder bestätige, wenn du kein Passwort hast, mit einem Passkey oder indem du dich erneut bei einem verknüpften Anbieter anmeldest) und wähle „Konto umziehen“.

Nach dem Umzug gilt für das alte Konto:

- Es zeigt „Dein Konto ist umgezogen“. Du kannst dich weiterhin anmelden, lesen, deine Daten exportieren und das Konto löschen, aber du kannst nicht posten, boosten, reagieren, folgen oder abstimmen.
- Sein Profil zeigt, dass es auf das neue Konto umgezogen ist.
- Es kann 30 Tage lang nicht erneut umziehen.

„Umzug abbrechen“ bringt das alte Konto zurück: Es kann wieder posten und folgen, und andere Server werden darüber informiert. Follower, die bereits umgezogen sind, bleiben beim neuen Konto.

<!-- source: moving-to-this-account adb43ba4eef3 -->
## Auf dieses Konto umziehen

Füge das Konto, von dem du umziehst, unter „Aliase“ in den Einstellungen dieses Kontos als Alias hinzu („Alias hinzufügen“). Ein Alias allein ändert nichts; er erlaubt dem anderen Konto lediglich, seine Follower zu dir umzuziehen. Du kannst nur wenige Aliase haben, und „Entfernen“ löscht einen.

<!-- source: taking-your-follows-with-you ab2475388452 -->
## Follows mitnehmen

„Import und Export“ unter „Konto und Daten“ in den Einstellungen exportiert und importiert CSV-Dateien in dem Format, das Mastodon verwendet. Du kannst „Follows“, „Blockierte Konten“, „Stummgeschaltete Konten“, „Stummgeschaltete Domains“ und „Lesezeichen“ exportieren und auf dem neuen Konto importieren. „Zusammenführen“ behält, was du hast, und fügt hinzu, was die Datei auflistet; „Überschreiben“ macht die Liste genau zu dem, was die Datei auflistet. Konten und Beiträge auf anderen Servern werden im Hintergrund nachgeschlagen, was eine Weile dauern kann.

„Archiv und Löschung“ erstellt ein ZIP-Archiv deiner Beiträge, deines Profils sowie deiner Follows, Blockierungen, Stummschaltungen und Lesezeichen.

<!-- source: when-an-account-you-follow-moves 81ab63afe43d -->
## Wenn ein Konto, dem du folgst, umzieht

Du wirst benachrichtigt („… ist zu einem anderen Konto umgezogen“). Wenn du dem neuen Konto folgen kannst, folgst du ihm sofort, oder deine Anfrage wartet auf seine Genehmigung. Du folgst ihm nicht, wenn du es blockierst, wenn es dich blockiert oder wenn du seinen Server stummgeschaltet hast; die Benachrichtigung sagt, welcher Fall vorliegt.
