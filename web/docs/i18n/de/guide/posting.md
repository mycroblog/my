<!-- source: posting-and-choosing-who-sees-a-post 5c4adb3025f6 -->
# Beiträge verfassen und festlegen, wer sie sieht

<!-- source: writing-a-post b3b7ce030983 -->
## Einen Beitrag verfassen

Wähle „Beitrag verfassen“, um das Formular „Neuer Beitrag“ zu öffnen. Schreibe in „Dein Beitrag“ und wähle „Veröffentlichen“. Das Formular zeigt, wie viele Zeichen noch übrig sind. Neben Text kannst du:

- Mit „Bilder, Video oder Audio hinzufügen“ Bilder, Videos oder Audio anhängen. Beschreibe jeden Anhang für Menschen, die ihn nicht sehen oder hören können; manche Server verlangen für jeden Anhang eine Beschreibung. Standort und andere Metadaten werden aus den Dateien entfernt.
- Eine „Umfrage hinzufügen“, mit Optionen und einer Dauer, sowie die Auswahlmöglichkeiten „Mehrfachauswahl erlauben“ und „Stimmen bis zum Ende der Umfrage ausblenden“.
- Eine „Inhaltswarnung hinzufügen“. Lesende sehen zuerst die Warnung und wählen „Inhalt anzeigen“.
- „Bilder ausblenden, bis Lesende sie anzeigen“ wählen, um die Medien als sensibel zu markieren.
- Die Sprache des Beitrags festlegen und wer darauf antworten, ihn boosten und zitieren darf (siehe [Wer antworten, boosten und zitieren darf](#who-can-reply-boost-and-quote)).
- Auf einen Beitrag antworten („Antworten“) oder einen zitieren („Zitieren“).

Ein Beitrag, der nicht gesendet werden konnte, bleibt unter „Beiträge, die gesendet werden“, wo du ihn „Erneut senden“, „Bearbeiten“ oder „Verwerfen“ kannst. Ein Server kann einen Beitrag nach einer automatischen Prüfung ablehnen. Lässt sich die Ablehnung anfechten, bietet der Beitrag „Einspruch einlegen“ an.

<!-- source: drafts-and-scheduled-posts 9cee5f0d17b2 -->
## Entwürfe und geplante Beiträge

Wähle „Entwurf speichern“, um das, was du gerade schreibst, auf dem Server zu behalten und das Formular zu leeren. Ein gespeicherter Entwurf öffnet sich auf jedem Gerät, auf dem du dich anmeldest: Der Link „Entwürfe und geplante Beiträge“ unter dem Formular führt zur Seite, die sie auflistet, wo „Bearbeiten“ einen Entwurf wieder im Formular öffnet. Enthält das Formular bereits etwas anderes, wirst du gefragt, bevor es ersetzt wird. Speicherst du einen Entwurf, den du von dieser Seite geöffnet hast, ersetzt das den Entwurf; veröffentlichst du ihn, wird er veröffentlicht und aus deinen Entwürfen entfernt. Wurde derselbe Entwurf seit dem Öffnen auf einem anderen Gerät gespeichert, wird nichts überschrieben oder veröffentlicht: Speichere erneut, um deine Version als neuen Entwurf zu behalten, oder veröffentliche erneut, um deine Version als neuen Beitrag zu veröffentlichen.

Um einen Beitrag später zu veröffentlichen, wähle „Beitrag planen“ (die Uhr unter den Werkzeugen des Formulars), wähle ein Datum und eine Uhrzeit und dann „Planen“. Datum und Uhrzeit gelten in deiner Zeitzone, der aus deinen Einstellungen, und das Formular nennt sie. Eine Zeit in der Vergangenheit, zu bald oder zu weit in der Zukunft wird abgelehnt, bevor etwas gesendet wird; der Server legt fest, wie bald und wie weit im Voraus du planen kannst. Der Beitrag wird zu dieser Zeit oder kurz danach veröffentlicht.

Die Seite „Entwürfe und geplante Beiträge“ listet zuerst deine geplanten Beiträge auf, mit der Zeit, zu der jeder veröffentlicht wird, dann deine Entwürfe. Ein geplanter Beitrag kann mit „Zeit ändern“ verschoben oder mit „Planung aufheben“ wieder in einen Entwurf verwandelt werden, und jeder Entwurf kann gelöscht werden. Nur du siehst deine Entwürfe.

Kann ein geplanter Beitrag zur vorgesehenen Zeit nicht veröffentlicht werden, zum Beispiel weil der Beitrag, auf den er antwortet, nicht mehr existiert, bleibt er mit dem Grund unter deinen Entwürfen, und du erhältst eine Benachrichtigung, die zu deinen Entwürfen führt, damit du das beheben und ihn erneut planen kannst.

<!-- source: who-can-see-a-post 267cf73d0612 -->
## Wer einen Beitrag sehen kann

Die Schaltfläche „Sichtbarkeit“ des Formulars öffnet „Wer kann diesen Beitrag sehen?“ für diesen Beitrag. Der Standard für neue Beiträge ist die
Einstellung „Wer deine Beiträge standardmäßig sieht“ unter „Anzeige, Verfassen und Timelines“ in den Einstellungen.

| Auswahl | Wer es sehen kann |
| --- | --- |
| Öffentlich | Alle, auf diesem und auf anderen Servern. Er wird auf den öffentlichen Timelines angezeigt. |
| Dieser Server | Personen, die bei diesem Server angemeldet sind, und deine Follower auf anderen Servern. Er wird auf der lokalen Timeline dieses Servers angezeigt. |
| Nicht gelistet | Alle mit dem Link, aber er wird nicht auf den öffentlichen Timelines angezeigt. |
| Follower | Nur deine Follower und die Personen, die du erwähnst. |
| Erwähnte Personen | Nur die Personen, die du erwähnst. |

![Wer einen Beitrag auf diesem Server und auf anderen Servern sehen kann, für jede der fünf Sichtbarkeiten. Die Liste unten sagt dasselbe.](images/visibility.svg)

Zeile für Zeile, zuerst auf diesem Server, dann auf anderen Servern:

- **Öffentlich**: alle, auch Personen, die nicht angemeldet sind, und der Beitrag wird auf der lokalen und der föderierten Timeline angezeigt. Auf anderen Servern kann ihn jeder lesen, und er wird an deine Follower und die Personen gesendet, die du erwähnst.
- **Dieser Server**: die Personen, die bei diesem Server angemeldet sind, und der Beitrag wird auf der lokalen Timeline angezeigt. Andere Server erhalten ihn nur für deine Follower dort, die ihn als „Follower“-Beitrag sehen; an niemanden sonst wird er gesendet.
- **Nicht gelistet**: alle mit dem Link, aber der Beitrag wird nicht auf den öffentlichen Timelines angezeigt. Auch auf anderen Servern kann ihn jeder mit dem Link lesen, und er wird an deine Follower und die Personen gesendet, die du erwähnst.
- **Follower**: deine Follower und die Personen, die du erwähnst, auf diesem und auf anderen Servern.
- **Erwähnte Personen**: nur die Personen, die du erwähnst, auf diesem und auf anderen Servern.

Auf einem Server mit nur einer Person, bei dem sonst niemand angemeldet ist, bedeutet „Dieser Server“ nicht mehr als „Follower“: Neue Beiträge
können es nicht haben, und ein Standard von „Dieser Server“ wirkt wie „Follower“. Das Sichtbarkeitsmenü und der Standard unter „Anzeige,
Verfassen und Timelines“ lassen es weg, und eine Antwort auf einen „Dieser Server“-Beitrag gilt für „Follower“. Beiträge, die es bereits haben,
behalten es.

Eine Sperre gewinnt immer: Wer dich sperrt oder wen du sperrst, sieht deinen Beitrag nicht, egal was du gewählt hast.

<!-- source: this-server d39e8d3e2df6 -->
### „Dieser Server“

Ein Beitrag mit der Einstellung „Dieser Server“ bleibt auf deinem Server. Personen, die dort nicht angemeldet sind, können ihn nicht lesen, auch nicht mit dem
Link, und er erscheint nie auf der föderierten Timeline. Andere Server erhalten ihn nur für deine Follower dort und
zeigen ihn diesen Followern als Beitrag nur für Follower an.

- Wenn du jemanden auf einem anderen Server erwähnst, der dir nicht folgt, erhält diese Person den Beitrag nicht, und deine Follower dort sehen die Erwähnung als einfachen Link. Das Formular weist dich darauf hin.
- Konten deines Servers können einen „Dieser Server“-Beitrag boosten, und der Boost wird nur auf diesem Server angezeigt.
- Eine Antwort auf einen „Dieser Server“-Beitrag beginnt ebenfalls als „Dieser Server“; du kannst das ändern.

<!-- source: other-things-to-know d1f569ac2d1d -->
### Weitere Hinweise

- Du kannst die Sichtbarkeit eines Beitrags nach dem Veröffentlichen nicht ändern. Wenn du einen Beitrag bearbeitest, bleiben die Sichtbarkeit und der Beitrag, auf den er antwortet, unverändert.
- Ein Moderator kann einschränken, wie weit ein Konto Beiträge streut. Das Formular sagt dann „Dein Konto kann derzeit höchstens als … veröffentlichen.“, und eine weitere Auswahl wird auf der weitesten erlaubten Stufe veröffentlicht.
- Jeder, der einen öffentlichen, einen „Dieser Server“- oder einen nicht gelisteten Beitrag sehen kann, kann ihn boosten. Nur du kannst deine eigenen „Follower“-Beiträge boosten, und nur deine Follower sehen diesen Boost. Beiträge für „Erwähnte Personen“ können nicht geboostet werden.
- Die Autorin oder der Autor eines Beitrags kann wählen, wer darauf antworten und wer ihn boosten darf; auch Beiträge von manchen anderen Servern tragen eine solche Wahl. Eine Antwort oder ein Boost außerhalb davon wird abgelehnt, mit einer Meldung wie „Der Autor dieses Beitrags erlaubt dir nicht, darauf zu antworten.“. Der Autor, die Personen, die der Beitrag erwähnt, und der Autor des Beitrags, auf den er antwortet, können immer antworten.
- Der Autor kann auch erlauben, dass manche Leute nur antworten oder boosten, wenn er es genehmigt. Eine solche Antwort oder ein solcher Boost wartet: Du wirst benachrichtigt („… hat darum gebeten, auf deinen Beitrag zu antworten“), und sie bleibt von deinem Beitrag fern, bis du sie genehmigst. Eine wartende Antwort von einem anderen Server steht weder in der Unterhaltung unter deinem Beitrag noch in seiner Zahl der Antworten; sie erscheint weiterhin im Profil ihres Autors und auf ihrer eigenen Seite. Ein wartender Boost wird nicht gezählt. Genehmigst du sie, wird sie zu deinem Beitrag hinzugefügt; lehnst du sie ab, bleibt sie fern, und der andere Server wird in beiden Fällen informiert.
- Eine Antwort oder ein Boost von einem anderen Server, die ganz außerhalb deiner Wahl liegen, wird auf dieselbe Weise von deinem Beitrag ferngehalten und benachrichtigt dich über nichts.
- Wenn du auf einen Beitrag antwortest oder ihn boostest, dessen Autor Antworten oder Boosts von Hand genehmigt, wartet deine Antwort auf diese Genehmigung. Bis dahin sieht nur du deine Antwort, unter dem Beitrag und auf ihrer eigenen Seite, als wartend markiert; andere sehen nichts davon, und an andere Server wird nichts gesendet. Dein Boost wird erst ausgeführt, wenn er genehmigt ist. Du wirst benachrichtigt, wenn der Autor ihn genehmigt oder ablehnt („… hat deine Antwort, deinen Boost oder dein Zitat genehmigt“). Löschst du eine wartende Antwort oder machst einen wartenden Boost rückgängig, nimmst du die Anfrage zurück.
- Du kannst bis zu fünf deiner Beiträge mit „An Profil anheften“ an dein Profil heften. Ein Beitrag für „Erwähnte Personen“ kann nicht angeheftet werden.

<!-- source: who-can-reply-boost-and-quote c9ea034f7416 -->
## Wer antworten, boosten und zitieren darf

Die Schaltfläche „Wer antworten, boosten und zitieren darf“ unter den Werkzeugen des Formulars öffnet die Auswahl für diesen Beitrag. Für Antworten, Boosts und Zitate wählst du jeweils, wer es sofort tun darf: „Alle“, „Follower und Personen, die du erwähnst“, „Personen, die du erwähnst“ oder „Nur du“. Darunter lässt „Andere können mit deiner Genehmigung antworten“ (und dasselbe für Boosts und Zitate) alle anderen anfragen, statt abgelehnt zu werden. Sobald eine Auswahl weniger als alle zulässt, zeigt das Formular das Ergebnis über seinen Werkzeugen, bevor du veröffentlichst, zum Beispiel „Antworten: Alle. Boosts: Nur du. Zitate: Alle.“.

- Die Personen, die ein Beitrag erwähnt, können immer darauf antworten. Ein „Follower“-Beitrag kann nur von dir geboostet und zitiert werden und ein Beitrag für „Erwähnte Personen“ von niemandem, was auch immer du wählst.
- Die Standardwerte für neue Beiträge findest du unter „Wer auf deine neuen Beiträge antworten, sie boosten und zitieren darf“ in „Anzeige, Verfassen und Timelines“ in den Einstellungen.
- Um sie für einen Beitrag zu ändern, den du schon gesendet hast, wähle „Ändern, wer antworten, boosten und zitieren darf“ in seinem Menü „Weitere Aktionen“. Jede Auswahl wird sofort gespeichert. Antworten, Boosts und Zitate, die du zuvor genehmigt hast, bleiben, und Anfragen, die die neue Auswahl zulässt, werden genehmigt.
- Anfragen, die auf dich warten, stehen unter „Genehmigungsanfragen“ in den Einstellungen, neben „Folgeanfragen“; die Benachrichtigung jeder einzelnen verlinkt dorthin mit „Anfragen prüfen“. „Genehmigen“ fügt die Antwort, den Boost oder das Zitat zu deinem Beitrag hinzu, und die Unterhaltung darunter zeigt eine genehmigte Antwort sofort; „Ablehnen“ hält sie fern.
- Unter dem Beitrag einer anderen Person wird „Antworten“ nicht angezeigt und „Boosten“ lässt sich nicht drücken, wenn der Autor es dir nicht erlaubt. Genehmigt der Autor sie von Hand, sagt das Formular das, bevor du antwortest, und nachdem du geboostet hast, steht am Beitrag „Dein Boost wartet auf die Genehmigung des Autors.“

<!-- source: after-posting dc553fa0ead9 -->
## Nach dem Veröffentlichen

Unter einem Beitrag findest du „Antworten“, „Boosten“, „Zitieren“, „Favorisieren“ und „Lesezeichen“. Das Menü „Weitere Aktionen“ enthält „Link kopieren“ und für deine eigenen Beiträge „Bearbeiten“, „An Profil anheften“ und „Löschen“. Ein Beitrag, der bearbeitet wurde, zeigt „Bearbeitet“, und seine früheren Versionen stehen auf seiner Seite „Bearbeitungsverlauf“. „Löschen“ entfernt den Beitrag für alle, zusammen mit seinen Boosts und Favoriten.

Bei einem Beitrag einer anderen Person bietet dasselbe Menü an, dem Autor zu folgen, ihn stummzuschalten, zu blockieren oder zu melden, sowie „Direktnachricht“. Deine Lesezeichen stehen auf der Seite „Lesezeichen“; nur du siehst sie.

Wenn der Server Übersetzung anbietet oder dein Browser übersetzen kann, haben Beiträge in anderen Sprachen „Übersetzen“ unter ihren Aktionen und im Menü „Weitere Aktionen“, das sie aufnimmt, wenn der Beitrag für eine weitere Schaltfläche zu schmal ist. Solange ein Beitrag seine Übersetzung zeigt, sagt eine Zeile unter dem Text, aus welcher Sprache und von wem er übersetzt wurde, mit „Original anzeigen“, um zurückzugehen. Sendet der Server den Text von Beiträgen an einen Übersetzungsdienst, sagt dir die Auswahl der Sichtbarkeit, bei welchen Auswahlmöglichkeiten.

<!-- source: writing-an-article 0dfa0a80bb8e -->
## Einen Artikel schreiben

„Artikel schreiben“ unter dem Formular für neue Beiträge öffnet den Artikeleditor, eine eigene Seite. Gib dem Artikel einen Titel, wenn du magst eine Zusammenfassung (bleibt sie leer, wird sie aus dem Anfang des Textes erstellt), ein Titelbild mit seiner Beschreibung und den Text, geschrieben in Markdown: `#` beginnt eine Überschrift, `**text**` ist fett, und „Fett“ und „Link“ über dem Text tun dasselbe für den Text, den du markierst (Strg+B und Strg+K, oder Cmd auf einem Mac). Ziehe Bilder in den Text oder füge sie ein, oder wähle sie mit „Bilder hinzufügen“: Jedes wird hochgeladen und an der Stelle des Cursors als `![](media:…)` eingefügt; schreibe seine Beschreibung zwischen die eckigen Klammern. Nur Bilder, die du hochlädst, werden in einem Artikel angezeigt; ein Bild von einer anderen Adresse wird zu einem Link mit seiner Beschreibung.

- Hashtags kommen in ein eigenes Feld, durch Leerzeichen getrennt; der Artikel wird unter ihnen aufgelistet, und ein `#` im Text bleibt Text. Erwähnungen im Text verlinken auf die Personen, benachrichtigen sie aber nicht.
- „Vorschau“ zeigt den Artikel so, wie Lesende ihn sehen werden, vom Server genau so gerendert, wie er veröffentlicht wird, mit der Größe des Textes und der Zahl seiner Bilder im Verhältnis zu den Grenzen dieses Servers.
- „Als sensibel markieren“ zeigt Lesenden zuerst Titel und Zusammenfassung und klappt das Titelbild und den Text ein. Ein Artikel kann nicht nur für „Erwähnte Personen“ sein.
- Während du schreibst, wird der Artikel bei jeder Pause als Entwurf gespeichert („Entwurf gespeichert“), sodass er sich auf jedem Gerät über „Entwürfe und geplante Beiträge“ öffnen lässt, wo „Bearbeiten“ ihn zurück in den Editor bringt. Wurde der Entwurf inzwischen auf einem anderen Gerät geändert, wird nichts überschrieben: Dein Text bleibt im Editor, und „Als neuen Entwurf speichern“ behält ihn als weiteren Entwurf.
- „Veröffentlichen“ veröffentlicht den Artikel jetzt und öffnet seine Seite; „Planen“ veröffentlicht ihn zu der Zeit, die du wählst, wie bei einem Beitrag.
- Um einen veröffentlichten Artikel zu ändern, wähle „Bearbeiten“ in seinem Menü „Weitere Aktionen“. Der Editor öffnet sich mit dem, was du geschrieben hast; „Änderungen speichern“ ersetzt es, und die frühere Version bleibt in seinem „Bearbeitungsverlauf“. Die Sichtbarkeit eines Artikels ändert sich nicht.

Ein Server kann nur manchen Leuten erlauben, neue Artikel zu schreiben; dann wird „Artikel schreiben“ den anderen nicht angezeigt, die aber die Artikel, die sie veröffentlicht haben, weiterhin bearbeiten können.

<!-- source: reading-articles f2811fffd462 -->
## Artikel lesen

Ein Artikel ist ein langer Beitrag mit einem Titel, einer Zusammenfassung und oft einem Titelbild, geschrieben auf diesem oder einem anderen Server. Timelines und Profile zeigen einen Artikel als Karte mit seinem Titelbild, dem Wort „Artikel“, seinem Titel und seiner Zusammenfassung; wähle den Titel, um den ganzen Artikel auf seiner eigenen Seite zu lesen, deren Überschrift der Titel ist. Antworten, Boosts, Zitate, Favoriten und Reaktionen funktionieren wie bei jedem Beitrag. Artikel haben kein „Übersetzen“.

Ein Artikel, den sein Autor als sensibel markiert hat, zeigt seinen Titel und seine Zusammenfassung und verbirgt den Rest: Auf der Karte zeigt „Sensible Medien anzeigen“ sein Titelbild, und auf seiner Seite zeigt „Artikel anzeigen“ sein Titelbild und seinen Text. Der Tab „Artikel“ eines Profils listet nur die Artikel dieser Person auf, die neuesten zuerst; bei jemandem auf einem anderen Server nur die Artikel, die dieser Server erhalten hat.

<!-- source: direct-messages 59062702f4be -->
## Direktnachrichten

„Direktnachrichten“ in der Navigation sammelt deine „Erwähnte Personen“-Beiträge und die, die du erhältst, eine Zeile pro Unterhaltung, die mit dem neuesten Beitrag zuerst. Jede Zeile zeigt, wer an der Unterhaltung beteiligt ist, wann ihr neuester Beitrag geschrieben wurde, und den Anfang dieses Beitrags. Die Zahl neben „Direktnachrichten“ ist, in wie vielen Unterhaltungen es einen Beitrag gibt, den du noch nicht gelesen hast; jede davon hat vor ihren Namen einen Punkt.

Wähle den Anfang eines Beitrags, um den Thread der Unterhaltung zu öffnen; das markiert die Unterhaltung auf allen deinen Geräten als gelesen. Eine Antwort auf einen Beitrag für „Erwähnte Personen“ ist immer ebenfalls für „Erwähnte Personen“, sodass sie niemanden erreicht, den die Unterhaltung nicht erreichte. Auf Smartphones steht „Direktnachrichten“ unter „Menü“, es sei denn, du verschiebst es in der Tab-Leiste unter „Anzeige, Verfassen und Timelines“ in den Einstellungen.

<!-- source: deleting-old-posts-automatically e581c8d27380 -->
## Alte Beiträge automatisch löschen

„Alte Beiträge löschen“ in den Einstellungen löscht deine Beiträge, sobald sie älter sind als das Alter, das du wählst, als hättest du sie selbst gelöscht: Sie verschwinden hier und auf den Servern, an die sie gesendet wurden, und lassen sich nicht wiederherstellen. Die Beiträge, die du behältst (angeheftet, von dir mit einem Lesezeichen versehen oder favorisiert, mit Medien oder einer Umfrage, Artikel, mit bestimmten Sichtbarkeiten oder mit vielen Boosts oder Favoriten), bleiben. Deine Boosts von Beiträgen anderer Leute werden nie gelöscht. Der Server prüft jede Stunde und löscht jedes Mal höchstens 100 deiner Beiträge, sodass viele alte Beiträge mehrere Stunden brauchen, bis sie weg sind.

Ein neues Konto beginnt mit den Einstellungen, die der Server für neue Konten gewählt hat; ab da gehören sie dir, und du kannst sie wie jede andere Einstellung ändern.

Ein Server kann Beiträge auch höchstens eine bestimmte Zahl von Tagen aufbewahren. Die Seite „Info“ des Servers sagt das, bevor du dich registrierst. Unter einem solchen Maximum ist die automatische Löschung für dich immer eingeschaltet, das Alter, das du wählst, darf nicht länger als das Maximum sein, und nur die Arten von Beiträgen, die der Server erlaubt, dürfen behalten werden (standardmäßig nur angeheftete Beiträge, sodass auch deine Artikel gelöscht werden, es sei denn, der Server erlaubt, sie zu behalten). Manche Server lassen manche Leute ihre Beiträge länger behalten, zum Beispiel als Vorteil eines Unterstützungsplans.

Wenn ein Maximum für dich zu gelten beginnt (der Server führt es ein oder senkt es, oder du verlierst das Recht, Beiträge länger zu behalten), erhältst du eine Benachrichtigung und, sofern du das nicht ausgeschaltet hast, eine E-Mail, die sagt, ab wann es gilt. Bis dahin bleiben deine eigenen Einstellungen in Kraft. Die Einstellungsseite zeigt das Datum und mit ihrer Vorschau, welche deiner Beiträge dann gelöscht werden.
