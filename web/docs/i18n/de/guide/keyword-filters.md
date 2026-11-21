<!-- source: keyword-filters 466c3876a3d1 -->
# Stichwortfilter

Ein Stichwortfilter klappt Beiträge ein oder blendet sie aus, die eines seiner Wörter enthalten, egal wer sie geschrieben hat: Spoiler zu einer Serie, die du noch nicht gesehen hast, ein Thema, von dem du eine Pause brauchst. Der Server gleicht die Beiträge ab, sodass jede App, die du nutzt, dasselbe Ergebnis zeigt. Nur du siehst deine Filter, und niemand erfährt davon.

<!-- source: adding-a-filter 455f52213244 -->
## Einen Filter hinzufügen

Öffne „Stichwortfilter“ unter „Personen und Filter“ in den Einstellungen und wähle „Filter hinzufügen“:

1. **Name:** was ein eingeklappter Beitrag an seiner Stelle zeigt („Passt zum Filter ‚Staffelfinale‘“), bis zu 50 Zeichen.
2. **Passende Beiträge:** „Hinter dem Namen des Filters einklappen“ zeigt an der Stelle des Beitrags nur diesen Namen, mit „Anzeigen“ zum Lesen. „Ganz ausblenden“ lässt den Beitrag weg, und seine Benachrichtigung auch.
3. **Endet:** „Nie“, nach einer der angebotenen Dauern von 30 Minuten bis zu einer Woche oder „Zu einem Datum und einer Uhrzeit“.
4. **Wörter:** ein oder mehrere Wörter oder Wendungen. „Wort hinzufügen“ fügt ein weiteres hinzu, bis zu 100 in einem Filter. „Ganzes Wort“ neben einem Wort lässt es nur als ganzes Wort passen (siehe [Wie Wörter passen](#how-words-match)).
5. **Wo er gilt:** „Startseite und Listen“, „Benachrichtigungen“, „Öffentliche Timelines“ (die lokale, die föderierte und die Hashtag-Timelines), „Threads“ und „Profile“. Ein neuer Filter gilt überall; entferne die Haken bei den Orten, die du nicht möchtest.

Wähle „Hinzufügen“. Ein Filter der Liste öffnet ihn, um etwas davon zu ändern, und „Löschen …“ dort entfernt ihn nach einer Rückfrage mit seinen Wörtern. Ein Filter gilt sofort für jeden Beitrag, auch für alte; die Timelines laden auf dem Bildschirm neu, um die Änderung zu zeigen.

Die Suche, deine Lesezeichen und Favoriten und deine Direktnachrichten werden nie gefiltert. Deine eigenen Beiträge und Boosts auch nicht, ebenso wenig die Beiträge, die sie zitieren.

<!-- source: what-a-filtered-post-looks-like 52e0483be326 -->
## Wie ein gefilterter Beitrag aussieht

Ein Beitrag, auf den ein einklappender Filter passt, zeigt nur „Passt zum Filter“ mit den Namen der Filter, die gepasst haben, und „Anzeigen“. Autor, Text und Bilder bleiben außer Sicht, und ein Screenreader liest nur diese Zeile, bis du „Anzeigen“ wählst; „Ausblenden“ klappt ihn wieder ein, ebenso wie das Zurückkehren zur Seite.

Eine Benachrichtigung über einen solchen Beitrag zeigt dieselbe Zeile anstelle des Beitragstextes. Passt nur ein zitierter Beitrag, wird nur das Zitat eingeklappt, auch bei „Ganz ausblenden“, mit einem eigenen „Anzeigen“. Ein Boost wird nach dem Beitrag gefiltert, den er boostet.

Ein Beitrag, den du selbst öffnest, über einen Link oder eine Benachrichtigung, wird eingeklappt statt ausgeblendet, damit der Link noch etwas zeigt.

<!-- source: when-a-filter-ends d70431d92b82 -->
## Wenn ein Filter endet

Ein Filter mit einem Ende gilt ab diesem Zeitpunkt nicht mehr. Er bleibt in deiner Liste, markiert mit „Abgelaufen“, bis du ihn löschst. Um ihn wieder zu verwenden, öffne ihn und wähle ein neues Ende oder „Nie“. Während du einen Filter mit Ende bearbeitest, lässt „Keine Änderung“ das Ende, wie es ist.

<!-- source: how-words-match aeca692b00c3 -->
## Wie Wörter passen

Ein Wort passt auf den Text eines Beitrags, seine Inhaltswarnung (und den Text dahinter), die Auswahlmöglichkeiten seiner Umfrage und die Beschreibungen seiner Bilder. Links passen anhand des Textes, den sie zeigen, nicht anhand der Adresse dahinter. Eine Wortgruppe mit Leerzeichen passt auf diese Wörter in dieser Reihenfolge; jede Folge von Leerzeichen oder Zeilenumbrüchen in einem Beitrag passt auf ein Leerzeichen.

Groß- und Kleinschreibung, halbbreite und vollbreite Formen sowie Katakana und Hiragana passen jeweils aufeinander:

| Wort | Passt auch auf |
| --- | --- |
| `Finale` | `finale`, `FINALE`, `ｆｉｎａｌｅ` |
| `ネタバレ` | `ねたばれ`, `ﾈﾀﾊﾞﾚ` |
| `season finale` | `Season  Finale` und die beiden Wörter über einen Zeilenumbruch hinweg |

Ein Wort passt überall, wo es vorkommt, auch innerhalb eines längeren Wortes. „Ganzes Wort“ bewirkt, dass es nur dort passt, wo es auf keiner Seite von einem Buchstaben, einer Ziffer oder `_` berührt wird:

| Wort | Ohne „Ganzes Wort“ | Mit „Ganzes Wort“ |
| --- | --- | --- |
| `cat` | `cat`, `cats`, `concatenate` | `cat`, `cat!`, `#cat`, aber nicht `cats` |
| `vote` | `vote`, `voters`, `devoted` | `vote`, `Vote.`, aber nicht `voters` |

Manche Schriften schreiben Wörter ohne Leerzeichen dazwischen: unter anderem Chinesisch, Japanisch (Kanji, Hiragana und Katakana), Thai, Laotisch, Khmer und Myanmar. Ein Filter kann dort nicht erkennen, wo ein Wort endet, deshalb gelten ihre Buchstaben nie als berührend, und „Ganzes Wort“ wirkt, als wäre es aus: `猫` passt auch auf `子猫` und `猫舌`, und `選挙` passt auch auf `選挙速報`. Wähle in diesen Sprachen längere Wörter, um nicht mehr zu treffen, als du meinst, zum Beispiel `最終回のネタバレ` statt `最終回`. Neben einem Buchstaben einer anderen Schrift gilt „Ganzes Wort“ weiterhin: `AI` mit „Ganzes Wort“ passt auf `AIの話`, aber nicht auf `AIDS`.

Ein Filter kennt keine Platzhalter und keine regulären Ausdrücke. Seine Wörter passen, wenn eines von ihnen passt; um Beiträge nur einzuklappen, wenn zwei Wörter zusammen vorkommen, schreibe sie als eine Wortgruppe.

<!-- source: limits 2f4588a30027 -->
## Grenzen

Du kannst bis zu 100 Filter haben, mit bis zu 100 Wörtern je Filter und insgesamt 1.000 Wörtern. Ein Wort hat bis zu 100 Zeichen.

Das Archiv von „Exportieren“ unter „Konto und Daten“ in den Einstellungen enthält deine Filter, auch abgelaufene, als `keyword_filters.json`.
