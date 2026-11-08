<!-- source: supporters-paid-plans ea0039323475 -->
# Unterstützer (bezahlte Pläne)

Nur wenn ein Administrator es aktiviert, können lokale Nutzer im Web einen bezahlten Plan abonnieren und die an den Plan gebundene Rolle behalten, solange sie zahlen. Das Programm ist standardmäßig aus, und nirgends erscheint ein Einstieg zu Plänen oder Abos, bis es aktiviert ist.
Der Deployment-Schlüssel `payments` ist in der [Konfigurationsreferenz](../reference/configuration.md#payments) beschrieben.

**mycroblog garantiert nicht die Einhaltung irgendeines Gesetzes, einschließlich des japanischen Gesetzes über bestimmte Handelsgeschäfte (Act on Specified Commercial Transactions).** Betreiber müssen den Inhalt der Pflichtangaben, den abschließenden Bestätigungsbildschirm und den Umgang mit Umsatzsteuer und Rechnungen anhand der Gesetze ihres eigenen Landes prüfen (siehe „Was Betreiber prüfen müssen“ unten).

<!-- source: how-a-subscription-works 13d7fb3772b2 -->
## So funktioniert ein Abo

![Wie ein Abo abläuft: Der Nutzer zahlt beim Zahlungsanbieter, die Ereignisse des Anbieters veranlassen den Server, den Zustand zurückzulesen, und der Server erteilt oder entzieht die Rolle des Plans und erfasst die Einnahmen. Die Schritte stehen unten.](images/supporter-flow.svg)

Im Diagramm von oben nach unten:

- **Abonnieren.** Der Nutzer wählt unter `/settings/supporter` einen Plan und ein Intervall (Monat oder Jahr). Nach der abschließenden Bestätigung erstellt der Server (`my serve`) die Zahlungsseite beim Anbieter, den `payments` festlegt (`stripe` oder `payjp`), und der Nutzer zahlt auf dieser Seite.
- **Ereignisse.** Der Anbieter sendet seine Ereignisse an den Webhook-Endpunkt `POST /payments/webhooks/{provider}`, der jedes Ereignis einmal erfasst und einen Job einreiht. Ein Ereignis ist nur ein Signal: Der Server übernimmt den Zustand nie aus seinem Inhalt.
- **Zurücklesen.** Der Job `supporters.apply_payment_event` und der Abgleich `supporters.reconcile_payments` alle `payments_reconcile_interval` lesen das Abo und die Geldbewegungen aus der API des Anbieters zurück. Der Abgleich repariert jedes Ereignis, das nicht angekommen ist.
- **Der Zustand und die Rolle.** Der Server leitet den Zustand des Abos aus dem Gelesenen und aus jeder verstrichenen Frist ab (zum Beispiel der 7-tägigen Karenzzeit) und erteilt oder entzieht damit die Rolle des Plans.
- **Einnahmen.** Die Geldbewegungen werden zu den Einnahmedatensätzen (Brutto, Gebühr und Netto jeder Zahlung, Rückerstattung, Rückbuchung und Gebühr). Ihre Summen erscheinen im Finanzbildschirm (`/finance`, für Personen mit `view_finance`) und, soweit `income_visibility` es erlaubt, auf `/about/costs` (siehe [Servereinstellungen](instance-settings.md)).

![Die Zustände eines Abos: active, canceling und past_due behalten die Rolle des Plans; pending, lapsed, expired, revoked und abandoned nicht. Die Übergänge stehen unten.](images/supporter-states.svg)

Die Zustände eines Abos, wie die Administration und `my admin supporters subscriptions` sie zeigen:

- `pending`: gestartet, noch nicht bezahlt. Keine Rolle. Die erste Zahlung macht es zu `active`, und ein Abo, das vor der ersten Zahlung endet, wird zu `abandoned`.
- `active`: bezahlt. Es hält die Rolle des Plans, die jede Verlängerung erhält.
- `past_due`: Eine Zahlung ist fehlgeschlagen. Es behält die Rolle 7 Tage ab dem Fehlschlag; eine Zahlung in dieser Zeit macht es wieder zu `active`.
- `lapsed`: 7 Tage nach dem Fehlschlag sind verstrichen. Die Rolle wird entzogen. Eine Zahlung danach macht es wieder zu `active`, und die Rolle kommt zurück.
- `canceling`: zum Ende des Zeitraums gekündigt. Es behält die Rolle bis dahin und wird dann zu `expired`.
- `expired`, `revoked` (eine vollständige Rückerstattung oder eine Rückbuchung) und `abandoned` sind endgültig und halten keine Rolle.

<!-- source: enabling-the-program fe86b2e59df5 -->
## Das Programm aktivieren

1. Setze den Deployment-Schlüssel `payments` auf deinen Zahlungsanbieter und übergib die Schlüssel und das Webhook-Secret des Anbieters. Anbieterspezifische Schritte stehen in [Stripe-Zahlungen](payments-stripe.md) und [PAY.JP-Zahlungen](payments-payjp.md). Das Programm lässt sich nicht aktivieren, solange der Schlüssel `none` ist.
2. Konfiguriere den Server so, dass er E-Mails versendet (`mail` ungleich `none`). Nur Nutzer mit bestätigter E-Mail-Adresse können abonnieren, daher kann ein Server, der keine E-Mails versendet, das Programm nicht aktivieren.
3. Schreibe auf der Seite „Angaben zum Handelsrecht“ unter „Bedingungen und Richtlinien“ in der Administration (`/admin/documents/commerce-disclosure`) die Pflichtangaben für den Handel (für Japan die vom Act on Specified Commercial Transactions verlangte Mitteilung), den Servertext `commerce_disclosure`, in Markdown. Sie werden unter `/about/commerce` veröffentlicht, und du kannst diese URL im Dashboard des Anbieters eintragen.
   Das Programm lässt sich mit leerem Text aktivieren, aber die Administration zeigt eine Warnung.
4. Erstelle unter „Rollen“ in der Administration die Rolle, die an den Plan gebunden werden soll. Sie darf keine Inhaber haben, nur Mitgliederberechtigungen (Gruppe `member`) und muss eine `position` haben, die niedriger ist als die jeder Team-Rolle, `moderator` eingeschlossen.
   Die Mitgliederberechtigungen sind die Vorteile, die ein Plan geben kann: `create_custom_emojis` (eigene Emojis beantragen), `view_finance` (die Finanzen des Servers lesen), `start_voice_rooms` (Sprachchat-Räume öffnen, solange nur berechtigte Nutzer es dürfen), `create_invites` (Menschen einladen, solange nur berechtigte Nutzer es dürfen, im Rahmen der [Mitgliedereinladungen](registration.md#member-invites)) und `keep_posts_longer` (Beiträge über die [Höchstdauer der automatischen Löschung](instance-settings.md#automatic-deletion-of-old-posts) hinaus behalten und die automatische Löschung ausschalten, solange die Höchstdauer gilt).
   Wenn das Abo endet, geht die Rolle und mit ihr gehen die Vorteile: Die Einladungen, die der Unterstützer erstellt hat, funktionieren nicht mehr, bis er erneut abonniert, und die Höchstdauer der automatischen Löschung gilt nach ihrer Karenzzeit wieder für seine Beiträge.
5. Öffne unter „Unterstützer“ in der Administration (Berechtigung `manage_supporters`) die Seite „Pläne“ (`/admin/supporters/plans`) und wähle „Plan erstellen“. Lege den Namen, die Beschreibung, den Monatspreis, den Rabatt bei jährlicher Zahlung, das monatliche Limit für das Erstellen eigener Emojis (es wird am ersten Tag jedes Monats in UTC zurückgesetzt; siehe [eigene Emojis](instance-settings.md#custom-emoji)), die Reihenfolge und die Sichtbarkeit fest und wähle die Rolle.
   Der Server erstellt die Monats- und Jahrespreise beim Anbieter (erstelle keine Preise im Dashboard des Anbieters). Bis die Preise existieren, ist der Plan „in Vorbereitung“ und nimmt keine Abos an.
6. Aktiviere auf der Seite „Programm“ unter „Unterstützer“ (`/admin/supporters`) „Unterstützerprogramm aktivieren“ und wähle „Speichern“. Dasselbe Formular legt fest, wer die Unterstützerliste lesen darf.

Sobald ein Plan existiert, lässt sich seine Rolle nicht mehr von Hand erteilen oder entziehen, und du änderst ihre Berechtigungen und Position auf der Seite des Plans (wähle den Plan auf der Seite „Pläne“). Ein höherer Plan (eine größere Reihenfolge) steht über den Rollen der niedrigeren Pläne, hat alle ihre Berechtigungen und ein Emoji-Limit, das mindestens so hoch ist wie ihres.

<!-- source: setup-at-the-provider 74efcb74cf80 -->
## Einrichtung beim Anbieter

- **Deaktivierung**: Ein Abonnent kann sein Konto nicht deaktivieren, solange das Abo läuft, auch eines, das zum Ende seines Zeitraums endet. Der Einstellungsbildschirm führt ihn zum Abo, damit niemand belastet wird, während er verborgen ist.
- **Wiederholungen (Dunning)**: Sieben Tage nachdem eine Zahlung fehlgeschlagen ist, entzieht der Server die Rolle (unabhängig von den Wiederholungen des Anbieters). Stelle den Wiederholungszeitraum des Anbieters auf mindestens 7 Tage und die letzte Aktion auf „Kündigen“. Gelingt die Zahlung nach der Karenzzeit, kommt die Rolle zurück.
- **Kundenportal**: Das Portal wird für Zahlungsmethoden und Rechnungen geöffnet. Schalte Planwechsel im Portal aus und erlaube nur die Kündigung zum Ende des Zeitraums. Planwechsel und Kündigungen erfolgen auf den eigenen Bildschirmen des Servers.
- **Rückerstattungen**: Der Server erstattet nur eine Zahlung zurück, die nichts gekauft hat: ein zweites Abo einer Zahlung und eine Zahlung für ein ausstehendes Abo, das geschlossen wurde. Die Anleitung des Anbieters ([Stripe](payments-stripe.md), [PAY.JP](payments-payjp.md)) beschreibt beides. Jede andere Rückerstattung liegt beim Betreiber, im Dashboard des Anbieters. Eine vollständige Rückerstattung und eine Rückbuchung entziehen die Rolle sofort und kündigen das Abo des Anbieters sofort. Eine teilweise Rückerstattung entzieht die Rolle nicht. Das Gewinnen einer Rückbuchung stellt die Rolle nicht wieder her; der Nutzer abonniert erneut.

<!-- source: what-users-see d1216c59368a -->
## Was Nutzer sehen

- Im Web öffnet ein Nutzer „Unterstützungspläne“ (`/settings/supporter`), wählt einen Plan und ein Zahlungsintervall (Monat oder Jahr), durchläuft einen abschließenden Bestätigungsbildschirm und gelangt weiter zur Zahlungsseite des Anbieters. Kartendaten laufen nie über diesen Server. Die Zahlungsseite kehrt zu diesem Bildschirm zurück, der dann den Zustand des Abos erneut liest.
  Während er abonniert ist, kann der Nutzer auf demselben Bildschirm den Plan wechseln (ein Upgrade und ein Wechsel von monatlich auf jährlich gelten, sobald ihre anteilige Differenz bezahlt ist, während ein Downgrade und ein Wechsel von jährlich auf monatlich zum Ende des Zeitraums gelten), eine geplante Änderung abbrechen, das Abo kündigen (die Vorteile bleiben bis zum Ende des Zeitraums) und eine Kündigung zurücknehmen sowie das Kundenportal für Zahlungsmethoden und Rechnungen öffnen.
  Schlägt die Zahlung einer Änderung fehl oder verlangt die Karte eine Authentifizierung, die die Änderung nicht anfordern kann, bleibt der Plan bestehen, und der Nutzer sieht einen Fehler mit dem Weg, die Karte zu aktualisieren. Die Karenzzeit beginnt dadurch nicht, und die Benachrichtigung über die fehlgeschlagene Zahlung wird nicht gesendet; eine fehlgeschlagene Verlängerung tut beides weiterhin.
- Nur Nutzer dieses Servers mit bestätigter E-Mail-Adresse und einem Konto, das weder gesperrt noch zur Löschung vorgesehen ist, können abonnieren. Ein Nutzer kann jeweils einen Plan halten.
- `/about` zeigt einen Bereich mit den öffentlichen Plänen. In der Tauri-App erscheinen der Abobildschirm, der Planbereich von `/about` und die Links dorthin nicht. Die Vorteile (Abzeichen, eigene Emojis, Finanzen) funktionieren auch in der App.

<!-- source: notifications-to-subscribers 91f6ad77e546 -->
## Benachrichtigungen an Abonnenten

Abonnenten wird jedes der Folgenden einmal mitgeteilt, per Benachrichtigung, Web Push und E-Mail (an die Hauptadresse, in der Sprache des Nutzers). Einem Abonnenten, dessen Konto deaktiviert ist, wird nichts mitgeteilt, weder dann noch bei der Rückkehr.

- Zahlungsfehler (`supporter_payment_failed`): wenn das Abo überfällig wird (`past_due`). Sie nennt das Ende der Karenzzeit (in der E-Mail in UTC) und einen Link zum Bildschirm, auf dem die Zahlungsmethode korrigiert werden kann (`/settings/supporter`).
- Ende des Abos (`supporter_subscription_ended`): wenn ein Abo einen Zustand mit Vorteilen verlässt, weil die Karenzzeit ablief, es auslief oder mit einer Rückerstattung oder Rückbuchung endete. Die Nachricht nennt den Grund. Ein Ende durch das Löschen des Kontos wird nicht angekündigt.
- Einstellung eines Plans (`supporter_plan_retired`): an die Abonnenten eines eingestellten Plans (außer ausstehende). Sie sagt, dass das Abo nicht verlängert wird.
- Ankündigung des Endes des Dienstes (`supporter_service_closure`): an die Abonnenten, deren Abo zum Ende seines Zeitraums gekündigt wurde, als die Ankündigung begann (außer ausstehende). Sie sagt, dass das Abo nicht verlängert wird. Während der Ankündigung werden keine neuen Abos angenommen.

Der Text von Web Push und Benachrichtigungen führt nicht zum Zahlungsbildschirm. Nur der Webclient setzt in eine Benachrichtigung einen Link zum Abobildschirm; die Tauri-App tut das nicht. Die Links in E-Mails öffnen den Webclient in einem Browser.

<!-- source: cli 555871b40822 -->
## CLI

`my admin supporters` führt einen Teil der Vorgänge der Administration aus (`my admin supporters -help`).

- `status`: ob das Programm aktiviert ist, was es braucht, bevor es aktiviert werden kann (ein Anbieter, E-Mail), und die Zahl der laufenden Abos.
- `enable`, `disable`: das Programm aktivieren und deaktivieren. Das Deaktivieren wird abgelehnt, solange ein Abo läuft.
- `plans`: die Liste der Pläne (eingestellte eingeschlossen), mit der Zahl der Pläne in Vorbereitung und der eingestellten sowie der Abos.
- `subscriptions`: die Liste der Abos (filtere mit `-plan` und `-state`). `cancel <subscription ID>` kündigt ein Abo sofort (keine Rückerstattung, außer bei einer Zahlung für ein ausstehendes Abo, die nichts gekauft hat).

Aktivieren, Deaktivieren und Kündigen werden als CLI-Vorgänge im Audit-Log festgehalten. Das Erstellen, Ändern und Einstellen von Plänen erfolgt in der Administration oder über die Admin-API.

<!-- source: the-list-of-members f00ca3b391a3 -->
## Die Mitgliederliste

- Die Servereinstellung `supporter_list_visibility` (auf dem Bildschirm „Unterstützer“; siehe [Servereinstellungen](instance-settings.md)) legt fest, wer die Liste der aktuellen Abonnenten sehen kann (`/about/supporters`). Beim Standard `off` erscheinen weder die Liste noch der Link von `/about`.
- Die Liste zeigt nur die Abonnenten, die sich auf der Seite „Abzeichen und Unterstützerliste“ ihrer Einstellungen für die Auflistung entschieden haben (standardmäßig wird niemand gelistet). Sie zeigt den Plannamen sowie Avatar, Anzeigenamen und Handle jeder Person, und nie Beträge, ob die Zahlung monatlich oder jährlich ist, Daten oder Zahlen.
- Die Entscheidung, gelistet zu werden, bleibt bestehen, wenn der Abonnent den Plan wechselt, und verschwindet, wenn das Abo endet (die Rolle wird entzogen). Wer erneut abonniert, beginnt ungelistet.
- Die Listenseite und ihre API senden immer `noindex, noarchive`, was die Suchmaschineneinstellung (`search_indexing`) auch sagt.

<!-- source: changing-prices-hiding-plans-and-retiring-plans a65c92b1c371 -->
## Preise ändern, Pläne ausblenden und Pläne einstellen

- Wenn du den Monatspreis oder den Rabatt änderst, gilt der neue Preis für neue Abos und Planwechsel. Bestehende Abonnenten verlängern zu ihrem ursprünglichen Preis. Es gibt keinen Vorgang, bestehende Abonnenten auf einen neuen Preis umzustellen. Um sie umzustellen, erstelle einen neuen Plan und stelle den alten ein.
- Das Ausblenden eines Plans beendet neue Abos und Wechsel zu diesem Plan. Bestehende Abos laufen weiter.
- Das Einstellen eines Plans lässt sich nicht rückgängig machen. Seine Abos werden zum Ende ihres Zeitraums gekündigt (ausstehende werden sofort geschlossen, und ihre Zahlungsseite nimmt keine Zahlungen mehr an, wobei jede noch eintreffende Zahlung erstattet wird; und solche, deren Zahlung über die Karenzzeit hinaus überfällig ist, werden sofort gekündigt). Wenn das letzte Abo endet, wird die Rolle wieder zu einer gewöhnlichen Rolle.

<!-- source: accounts-and-subscriptions 8c5e10df3c18 -->
## Konten und Abos

- Das Löschen eines Kontos und seine Sperrung (die eine Löschung vorsieht) kündigen seine Abos sofort, ohne Rückerstattung (eine Zahlung für ein noch ausstehendes Abo hat nichts gekauft und wird erstattet). Das Aufheben einer Sperrung stellt die Abos nicht wieder her.
- Das Sperren eines Kontos sieht eine Kündigung zum Ende des Zeitraums vor. Das Aufheben der Sperre stellt es nicht automatisch wieder her; der Nutzer bricht die Kündigung vor dem Ende des Zeitraums ab oder abonniert erneut.
- Auf der Seite „Abos“ unter „Unterstützer“ (`/admin/supporters/subscriptions`) kann ein Administrator ein Abo öffnen und mit „Jetzt beenden“ sofort beenden (keine Rückerstattung, außer bei einer Zahlung für ein ausstehendes Abo, die nichts gekauft hat; es wird im Audit-Log festgehalten).

<!-- source: disabling-the-program-and-changing-providers ab1548c938f8 -->
## Das Programm deaktivieren und Anbieter wechseln

- Das Programm lässt sich nicht deaktivieren, solange ein Abo läuft (ausstehend, aktiv, zur Kündigung vorgesehen, überfällig oder über die Karenzzeit hinaus). Stelle zuerst jeden Plan ein und deaktiviere das Programm, nachdem die Abos geendet haben.
- Um den Anbieter zu wechseln, deaktiviere das Programm, ändere `payments` und aktiviere das Programm mit dem neuen Anbieter erneut. Bestehende Abonnenten abonnieren erneut. Solange der konfigurierte Anbieter von dem abweicht, mit dem das Programm aktiviert wurde, lehnt der Server neue Abos und Änderungen ab, und die Administration zeigt eine Warnung.

<!-- source: what-operators-must-check-not-yet-verified 659a7e51f9d3 -->
## Was Betreiber prüfen müssen (noch nicht verifiziert)

mycroblog beurteilt nichts davon. Prüfe es anhand der Gesetze deines Landes und der Bedingungen deines Anbieters.

- Die Punkte, die die Pflichtangaben für den Handel enthalten müssen (Name, Adresse und Telefonnummer des Verkäufers, der Preis, wann und wie bezahlt wird, wann der Dienst erbracht wird, wie man kündigt und so weiter) und wie sie veröffentlicht werden müssen.
- Dass der abschließende Bestätigungsbildschirm (Artikel 12-6 des Act on Specified Commercial Transactions) Menge, Preis, Zahlungszeitpunkt, Lieferzeitpunkt sowie den Weg der Kündigung und ihre Wirkung zeigt, wobei die Zahlungsseite des Anbieters und der Bestätigungsbildschirm von `my` zusammen betrachtet werden.
- Der Umgang mit der Umsatzsteuer und ob du qualifizierte Rechnungen ausstellst.
