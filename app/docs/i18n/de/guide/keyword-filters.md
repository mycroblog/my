<!-- source: keyword-filters 466c3876a3d1 -->
# Stichwortfilter

Ein Stichwortfilter klappt Beiträge ein oder blendet sie aus, die eines seiner Wörter enthalten, egal wer sie geschrieben hat: Spoiler zu einer Serie, die du noch nicht gesehen hast, ein Thema, von dem du eine Pause brauchst. Der Server gleicht die Beiträge ab, sodass jede App, die du nutzt, dasselbe Ergebnis zeigt. Nur du siehst deine Filter, und niemand erfährt davon.

<!-- source: adding-a-filter 28f5b63e22b2 -->
## Einen Filter hinzufügen

Öffne in den Einstellungen unter „Anzeige und Benachrichtigungen“ die „Stichwortfilter“ und wähle „Filter hinzufügen“:

1. **Name:** was ein eingeklappter Beitrag an seiner Stelle zeigt („Gefiltert: Staffelfinale“), bis zu 50 Zeichen.
2. **Wörter:** ein oder mehrere Wörter oder Wortgruppen. „Wort hinzufügen“ fügt ein weiteres hinzu, bis zu 100 in einem Filter. „Ganzes Wort“ neben einem Wort bewirkt, dass es nur als ganzes Wort passt (siehe [Wie Wörter passen](#how-words-match)).
3. **Wo er gilt:** deine Home-Timeline und Listen, deine Benachrichtigungen, die lokale, die föderierte und die Hashtag-Timeline, Threads und Profile. Ein neuer Filter gilt überall; entferne das Häkchen bei den Orten, an denen du ihn nicht willst.
4. **Was er bewirkt:** „Einklappen“ legt den Beitrag hinter den Namen des Filters, mit „Anzeigen“ zum Lesen. „Ausblenden“ lässt den Beitrag weg, und seine Benachrichtigung auch.
5. **Endet:** „Nie“ oder nach einer der angebotenen Zeitspannen, von 30 Minuten bis zu einer Woche.

Wähle „Hinzufügen“. „Bearbeiten“ an einem Filter ändert jede dieser Angaben, und „Löschen“ entfernt ihn mit seinen Wörtern nach einer Rückfrage. Ein Filter gilt sofort für jeden Beitrag, auch für ältere; die Timelines, die auf dem Bildschirm offen sind, laden neu, um die Änderung zu zeigen.

Die Suche, deine Lesezeichen und Favoriten sowie deine Direktnachrichten werden nie gefiltert. Deine eigenen Beiträge und Boosts werden es ebenso wenig wie die Beiträge, die sie zitieren.

<!-- source: what-a-filtered-post-looks-like 2a5fd3cd5b3c -->
## Wie ein gefilterter Beitrag aussieht

Ein Beitrag, auf den ein „Einklappen“-Filter passt, zeigt nur „Gefiltert:“ und die Namen der Filter, die gepasst haben, sowie „Anzeigen“. Sein Autor, sein Text und seine Bilder bleiben verborgen, und ein Screenreader liest nur diese Zeile vor, bis du „Anzeigen“ wählst. Ein Beitrag klappt wieder ein, wenn du zur Seite zurückkehrst.

Eine Benachrichtigung über einen solchen Beitrag zeigt dieselbe Zeile statt des Beitragstexts. Passt nur ein zitierter Beitrag, wird nur das Zitat eingeklappt, auch bei „Ausblenden“, mit „Zitierten Beitrag anzeigen“. Ein Boost wird anhand des Beitrags gefiltert, den er boostet.

Ein Beitrag, den du selbst öffnest, über einen Link oder eine Benachrichtigung, wird eingeklappt statt ausgeblendet, damit der Link trotzdem etwas zeigt.

<!-- source: when-a-filter-ends 2657ec875f16 -->
## Wann ein Filter endet

Ein Filter mit einem Ende gilt ab diesem Zeitpunkt nicht mehr. Er bleibt in deiner Liste, als „Abgelaufen“ markiert, bis du ihn löschst. Um ihn wieder zu verwenden, wähle „Bearbeiten“ und ein neues Ende oder „Nie“. Während du einen Filter mit einem Ende bearbeitest, lässt „Beibehalten“ das Ende unverändert.

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

<!-- source: limits e41fa1ca05b0 -->
## Grenzen

Du kannst bis zu 100 Filter haben, mit jeweils bis zu 100 Wörtern und insgesamt 1.000 Wörtern. Ein Wort ist bis zu 100 Zeichen lang.

Das Archiv unter „Archiv und Löschung“ in den Einstellungen enthält deine Filter, auch abgelaufene, als `keyword_filters.json`.
