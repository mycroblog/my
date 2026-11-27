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

<!-- source: enabling-the-program b07e6c07d73b -->
## Das Programm aktivieren

1. Setze den Bereitstellungsschlüssel `payments` auf deinen Zahlungsanbieter und übergib die Schlüssel und das Webhook-Secret des Anbieters. Die anbieterspezifischen Schritte stehen in [payments-stripe.md](payments-stripe.md) und [payments-payjp.md](payments-payjp.md). Das Programm kann nicht aktiviert werden, solange der Schlüssel `none` ist.
2. Konfiguriere den Server so, dass er E-Mails sendet (`mail` ungleich `none`). Nur Benutzer mit einer bestätigten E-Mail-Adresse können abonnieren, daher kann ein Server, der keine E-Mails sendet, das Programm nicht aktivieren.
3. Schreibe auf der Seite „Angaben zum Handel“ unter „Bedingungen und Richtlinien“ in der Administration (`/admin/documents/commerce-disclosure`) die Händlerangaben (für Japan die Mitteilung, die das Gesetz über bestimmte Handelsgeschäfte verlangt), den Instanztext `commerce_disclosure`, in Markdown. Sie wird unter `/about/commerce` veröffentlicht, und du kannst diese URL im Dashboard des Anbieters eintragen.
   Das Programm lässt sich mit leerem Text aktivieren, aber die Administration zeigt eine Warnung.
4. Erstelle unter „Rollen“ in der Administration die Rolle, die an den Plan gebunden werden soll. Sie darf keine Inhaber haben, nur Mitgliederberechtigungen (Gruppe `member`) und eine `position`, die niedriger ist als die jeder Personal-Rolle, `moderator` eingeschlossen.
   Die Mitgliederberechtigungen sind die Vorteile, die ein Plan geben kann: `create_custom_emojis` (eigene Emojis anfragen), `view_finance` (die Finanzen des Servers lesen), `start_voice_rooms` (Sprachchat-Räume öffnen, solange nur berechtigte Benutzer es dürfen), `create_invites` (Leute einladen, solange nur berechtigte Benutzer es dürfen, innerhalb der Grenzen der [Einladungen von Mitgliedern](registration.md#member-invites)), `keep_posts_longer` (Beiträge über das [Maximum der automatischen Löschung](instance-settings.md#automatic-deletion-of-old-posts) hinaus behalten und die automatische Löschung ausschalten, solange das Maximum gilt), `create_articles` (Artikel schreiben, solange nur berechtigte Benutzer es dürfen; siehe [Artikel](instance-settings.md#articles)) und `create_events` (neue [Veranstaltungen](instance-settings.md#events) erstellen, solange nur berechtigte Benutzer es dürfen).
   Wenn das Abonnement endet, verschwindet die Rolle und mit ihr die Vorteile: Die Einladungen, die der Unterstützer erstellt hat, funktionieren nicht mehr, bis er wieder abonniert, und das Maximum der automatischen Löschung gilt nach seiner Kulanzfrist wieder für seine Beiträge.
5. Öffne unter „Unterstützer“ in der Administration (Berechtigung `manage_supporters`) die Seite „Pläne“ (`/admin/supporters/plans`) und wähle „Plan erstellen“. Lege den Namen, die Beschreibung, den monatlichen Preis, den Rabatt für jährliche Zahlung, das monatliche Limit für das Erstellen eigener Emojis (es wird am ersten Tag jedes Monats, UTC, zurückgesetzt; siehe [eigene Emojis](instance-settings.md#custom-emoji)), die Reihenfolge und die Sichtbarkeit fest und wähle die Rolle: Das Formular listet die Rollen auf, die einen Plan übernehmen können, oder fragt ohne die Berechtigung `manage_roles` nach dem Schlüssel der Rolle.
   Der Server erstellt die monatlichen und jährlichen Preise beim Anbieter (erstelle keine Preise im Dashboard des Anbieters). Solange die Preise nicht existieren, ist der Plan „in Vorbereitung“ und nimmt keine Abonnements an.
6. Wähle auf der ersten Seite von „Unterstützer“ (`/admin/supporters`) „Programm aktivieren“. Die Seite sagt, was fehlt, solange sich das Programm nicht aktivieren lässt, und zeigt den Zahlungsanbieter der Konfiguration, den nur die Konfiguration ändert.

Sobald ein Plan existiert, lässt sich seine Rolle nicht mehr von Hand vergeben oder entziehen, und du änderst ihre Berechtigungen und Position auf der Seite des Plans (wähle den Plan auf der Seite „Pläne“). Ein höherer Plan (eine größere Reihenfolge) steht über den Rollen der niedrigeren Pläne, hat alle ihre Berechtigungen und ein Emoji-Limit, das mindestens so hoch ist wie ihres.

<!-- source: setup-at-the-provider 5821be7cae27 -->
## Einrichtung beim Anbieter

- **Deaktivierung**: Ein Abonnent kann sein Konto nicht deaktivieren, solange das Abo läuft, auch eines, das zum Ende seines Zeitraums endet. Der Einstellungsbildschirm führt ihn zum Abo, damit niemand belastet wird, während er verborgen ist.
- **Wiederholungen (Dunning)**: Sieben Tage nachdem eine Zahlung fehlgeschlagen ist, entzieht der Server die Rolle (unabhängig von den Wiederholungen des Anbieters). Stelle den Wiederholungszeitraum des Anbieters auf mindestens 7 Tage und die letzte Aktion auf „Kündigen“. Gelingt die Zahlung nach der Karenzzeit, kommt die Rolle zurück.
- **Den Anbieter prüfen**: Der Abschnitt „Zahlungsanbieter“ auf der ersten Seite von „Unterstützer“ zeigt den Modus des Anbieters (Test oder live), die letzte Webhook-Zustellung und den letzten Abgleich samt dem Grund, warum er endete, und warnt, wenn der Webhook nicht gesetzt zu sein scheint. „Jetzt abgleichen“ führt sofort einen Abgleich aus, höchstens einmal pro Stunde.
- **Kundenportal**: Das Portal wird für Zahlungsmethoden und Rechnungen geöffnet. Schalte Planwechsel im Portal aus und erlaube nur die Kündigung zum Ende des Zeitraums. Planwechsel und Kündigungen erfolgen auf den eigenen Bildschirmen des Servers.
- **Rückerstattungen**: Der Server erstattet nur eine Zahlung zurück, die nichts gekauft hat: ein zweites Abo einer Zahlung und eine Zahlung für ein ausstehendes Abo, das geschlossen wurde. Die Anleitung des Anbieters ([Stripe](payments-stripe.md), [PAY.JP](payments-payjp.md)) beschreibt beides. Jede andere Rückerstattung liegt beim Betreiber, im Dashboard des Anbieters. Eine vollständige Rückerstattung und eine Rückbuchung entziehen die Rolle sofort und kündigen das Abo des Anbieters sofort. Eine teilweise Rückerstattung entzieht die Rolle nicht. Das Gewinnen einer Rückbuchung stellt die Rolle nicht wieder her; der Nutzer abonniert erneut.

<!-- source: what-users-see 29a1564c7109 -->
## Was Nutzer sehen

- Im Web öffnet ein Nutzer „Unterstützungspläne“ (`/settings/supporter`), wählt einen Plan und ein Zahlungsintervall (Monat oder Jahr), durchläuft einen abschließenden Bestätigungsbildschirm und gelangt weiter zur Zahlungsseite des Anbieters. Kartendaten laufen nie über diesen Server. Die Zahlungsseite kehrt zu diesem Bildschirm zurück, der dann den Zustand des Abos erneut liest.
  Während er abonniert ist, kann der Nutzer auf demselben Bildschirm den Plan wechseln (ein Upgrade und ein Wechsel von monatlich auf jährlich gelten, sobald ihre anteilige Differenz bezahlt ist, während ein Downgrade und ein Wechsel von jährlich auf monatlich zum Ende des Zeitraums gelten), eine geplante Änderung abbrechen, das Abo kündigen (die Vorteile bleiben bis zum Ende des Zeitraums) und eine Kündigung zurücknehmen sowie das Kundenportal für Zahlungsmethoden und Rechnungen öffnen.
  Schlägt die Zahlung einer Änderung fehl oder verlangt die Karte eine Authentifizierung, die die Änderung nicht anfordern kann, bleibt der Plan bestehen, und der Nutzer sieht einen Fehler mit dem Weg, die Karte zu aktualisieren. Die Karenzzeit beginnt dadurch nicht, und die Benachrichtigung über die fehlgeschlagene Zahlung wird nicht gesendet; eine fehlgeschlagene Verlängerung tut beides weiterhin.
- Nur Nutzer dieses Servers mit bestätigter E-Mail-Adresse und einem Konto, das weder gesperrt noch zur Löschung vorgesehen ist, können abonnieren. Ein Nutzer kann jeweils einen Plan halten.
- `/about` zeigt einen Bereich mit den öffentlichen Plänen. In den offiziellen Mobil-Apps erscheinen der Abobildschirm, der Planbereich von `/about` und die Links dorthin nicht. Die Vorteile (Abzeichen, eigene Emojis, Finanzen) funktionieren auch in der App.

<!-- source: notifications-to-subscribers 73f507490bca -->
## Benachrichtigungen an Abonnenten

Abonnenten wird jedes der Folgenden einmal mitgeteilt, per Benachrichtigung, Web Push und E-Mail (an die Hauptadresse, in der Sprache des Nutzers). Einem Abonnenten, dessen Konto deaktiviert ist, wird nichts mitgeteilt, weder dann noch bei der Rückkehr.

- Zahlungsfehler (`supporter_payment_failed`): wenn das Abo überfällig wird (`past_due`). Sie nennt das Ende der Karenzzeit (in der E-Mail in UTC) und einen Link zum Bildschirm, auf dem die Zahlungsmethode korrigiert werden kann (`/settings/supporter`).
- Ende des Abos (`supporter_subscription_ended`): wenn ein Abo einen Zustand mit Vorteilen verlässt, weil die Karenzzeit ablief, es auslief oder mit einer Rückerstattung oder Rückbuchung endete. Die Nachricht nennt den Grund. Ein Ende durch das Löschen des Kontos wird nicht angekündigt.
- Einstellung eines Plans (`supporter_plan_retired`): an die Abonnenten eines eingestellten Plans (außer ausstehende). Sie sagt, dass das Abo nicht verlängert wird.
- Ankündigung des Endes des Dienstes (`supporter_service_closure`): an die Abonnenten, deren Abo zum Ende seines Zeitraums gekündigt wurde, als die Ankündigung begann (außer ausstehende). Sie sagt, dass das Abo nicht verlängert wird. Während der Ankündigung werden keine neuen Abos angenommen.

Der Text von Web Push und Benachrichtigungen führt nicht zum Zahlungsbildschirm. Nur der Webclient setzt in eine Benachrichtigung einen Link zum Abobildschirm; die offiziellen Mobil-Apps tun das nicht. Die Links in E-Mails öffnen den Webclient in einem Browser.

<!-- source: cli 555871b40822 -->
## CLI

`my admin supporters` führt einen Teil der Vorgänge der Administration aus (`my admin supporters -help`).

- `status`: ob das Programm aktiviert ist, was es braucht, bevor es aktiviert werden kann (ein Anbieter, E-Mail), und die Zahl der laufenden Abos.
- `enable`, `disable`: das Programm aktivieren und deaktivieren. Das Deaktivieren wird abgelehnt, solange ein Abo läuft.
- `plans`: die Liste der Pläne (eingestellte eingeschlossen), mit der Zahl der Pläne in Vorbereitung und der eingestellten sowie der Abos.
- `subscriptions`: die Liste der Abos (filtere mit `-plan` und `-state`). `cancel <subscription ID>` kündigt ein Abo sofort (keine Rückerstattung, außer bei einer Zahlung für ein ausstehendes Abo, die nichts gekauft hat).

Aktivieren, Deaktivieren und Kündigen werden als CLI-Vorgänge im Audit-Log festgehalten. Das Erstellen, Ändern und Einstellen von Plänen erfolgt in der Administration oder über die Admin-API.

<!-- source: the-list-of-members df4c915636dc -->
## Die Mitgliederliste

- Die Instanzeinstellung `supporter_list_visibility` (der Abschnitt „Unterstützerliste“ auf der ersten Seite von „Unterstützer“, für einen Administrator mit `manage_settings`; siehe [instance-settings.md](instance-settings.md)) legt fest, wer die Liste der aktuellen Abonnenten (`/about/supporters`) sehen kann. Beim Standardwert `off` erscheinen weder die Liste noch der Link von `/about`.
- Die Liste zeigt nur die Abonnenten, die sich auf der Seite „Abzeichen und Unterstützerliste“ ihrer Einstellungen dafür entschieden haben, aufgeführt zu werden (standardmäßig wird niemand aufgeführt). Sie zeigt den Namen des Plans sowie Avatar, Anzeigenamen und Handle jeder Person und nie Beträge, ob die Zahlung monatlich oder jährlich ist, Daten oder Zahlen.
- Die Entscheidung, aufgeführt zu werden, bleibt beim Planwechsel des Abonnenten bestehen und verschwindet, wenn das Abonnement endet (die Rolle wird entzogen). Wer erneut abonniert, beginnt nicht aufgeführt.
- Die Listenseite und ihre API senden immer `noindex, noarchive`, was auch immer die Suchmaschinen-Einstellung (`search_indexing`) sagt.

<!-- source: changing-prices-hiding-plans-and-retiring-plans fdeeb52e52bb -->
## Preise ändern, Pläne verbergen und Pläne einstellen

- Wenn du den monatlichen Preis oder den Rabatt änderst, gilt der neue Preis für neue Abonnements und Planwechsel. Bestehende Abonnenten verlängern zu ihrem ursprünglichen Preis. Es gibt keine Operation, die bestehende Abonnenten auf einen neuen Preis umstellt. Um sie umzustellen, erstelle einen neuen Plan und stelle den alten ein.
- Einen Plan zu verbergen stoppt neue Abonnements und Wechsel zu diesem Plan. Bestehende Abonnements laufen weiter.
- Einen Plan einzustellen („Plan einstellen…“ auf seiner Seite, nach Eingabe seines Namens) lässt sich nicht rückgängig machen. Seine Abonnements werden zum Ende ihres Zeitraums gekündigt (ausstehende werden sofort geschlossen, und ihre Zahlungsseite nimmt keine Zahlungen mehr an, wobei eine Zahlung, die doch noch eintrifft, erstattet wird; und solche, deren Zahlung über die Karenzzeit hinaus überfällig ist, werden sofort gekündigt). Wenn das letzte Abonnement endet, wird die Rolle wieder zu einer gewöhnlichen Rolle.

<!-- source: accounts-and-subscriptions 480ba9a583b1 -->
## Konten und Abonnements

- Ein Konto zu löschen und es zu verbannen (was die Löschung einplant) kündigt seine Abonnements sofort, ohne Rückerstattung (eine Zahlung für ein noch ausstehendes Abonnement hat nichts gekauft und wird erstattet). Eine Verbannung aufzuheben stellt die Abonnements nicht wieder her.
- Ein Konto zu sperren plant die Kündigung zum Ende des Zeitraums ein. Die Sperre aufzuheben stellt sie nicht automatisch wieder her; der Nutzer nimmt die Kündigung vor dem Ende des Zeitraums zurück oder abonniert erneut.
- Auf der Seite „Abonnements“ unter „Unterstützer“ (`/admin/supporters/subscriptions`, gefiltert nach Plan und Zustand) kann ein Administrator ein einzelnes Abonnement öffnen und es mit „Jetzt beenden…“ sofort beenden, nachdem er das Handle des Abonnenten eingegeben hat (keine Rückerstattung, außer bei einer Zahlung für ein ausstehendes Abonnement, die nichts gekauft hat; es wird ins Audit-Log geschrieben).

<!-- source: disabling-the-program-and-changing-providers ab1548c938f8 -->
## Das Programm deaktivieren und Anbieter wechseln

- Das Programm lässt sich nicht deaktivieren, solange ein Abo läuft (ausstehend, aktiv, zur Kündigung vorgesehen, überfällig oder über die Karenzzeit hinaus). Stelle zuerst jeden Plan ein und deaktiviere das Programm, nachdem die Abos geendet haben.
- Um den Anbieter zu wechseln, deaktiviere das Programm, ändere `payments` und aktiviere das Programm mit dem neuen Anbieter erneut. Bestehende Abonnenten abonnieren erneut. Solange der konfigurierte Anbieter von dem abweicht, mit dem das Programm aktiviert wurde, lehnt der Server neue Abos und Änderungen ab, und die Administration zeigt eine Warnung.

<!-- source: tips 106dc656074d -->
## Trinkgelder

Ein Trinkgeld ist eine einmalige Zahlung eines lokalen Nutzers für den Betrieb des Servers ohne Gegenleistung: Es gibt keine Rolle, kein Abzeichen und keinen anderen Vorteil, und der Nutzer sieht nur ein Dankeschön, sobald es bezahlt ist. Trinkgelder sind standardmäßig aus, brauchen keinen Plan und gehen nur an den Betreiber: Nutzer können einander kein Trinkgeld geben, und es gibt keine Ziele, Kampagnen oder Kontostände.

- **Was sie brauchen**: das Unterstützerprogramm, aktiviert beim konfigurierten Anbieter, und einen Anbieter, der Einmalzahlungen annimmt. Der Abschnitt **Trinkgelder** auf der ersten Seite des Bildschirms **Unterstützer** (Berechtigung `manage_supporters`) zeigt, ob Nutzer jetzt Trinkgeld geben können, und warnt, wenn der Anbieter keine Einmalzahlungen annimmt; `GET /api/v1/admin/supporter_tips` meldet dasselbe: ob dein Anbieter sie annimmt (`one_time_payments`), den kleinsten und den größten Betrag, den er in jeder Währung erlaubt (`limits`), und ob Nutzer jetzt Trinkgeld geben dürfen (`accepting`). Stripe nimmt sie an (siehe [Trinkgelder bei Stripe](payments-stripe.md#tips)), PAY.JP nimmt sie auf seiner gehosteten Seite der API v2 an (siehe [Trinkgelder bei PAY.JP](payments-payjp.md#tips)), und ebenso der Testanbieter der Test-Builds.
- **Beträge**: Du wählst 1 bis 6 Beträge in einer Währung, Yen oder US-Dollar, jeweils von ¥100 bis ¥100.000 oder von US$1 bis US$1.000 und mindestens die kleinste Einmalbelastung des Anbieters. Nutzer wählen einen davon und geben nie einen Betrag ein; die Untergrenze verhindert, dass ein kleines Trinkgeld im Streitfall mehr kostet, als es einbringt.
- **Aktivieren, ändern und deaktivieren**: Hake im Abschnitt **Trinkgelder** **Trinkgelder annehmen** an, wähle die Währung, schreibe die Beträge je einen pro Zeile und wähle **Trinkgelder speichern**; entferne den Haken und speichere, um sie zu deaktivieren. Die API ist `PUT /api/v1/admin/supporter_tips` mit `{"enabled": true, "amounts": [{"amount": "500", "currency": "JPY"}, ...]}` oder `{"enabled": false}`. Jede Änderung wird ins Audit-Log geschrieben (`supporter_tips_enabled`, `supporter_tips_revised`, `supporter_tips_disabled`). Das Deaktivieren des Unterstützerprogramms stoppt auch die Trinkgelder; ein bereits geöffneter Checkout bleibt bis zu seinem Ende bezahlbar, und eine Zahlung darauf wird weiterhin erfasst.
- **Wer Trinkgeld geben kann**: Nutzer dieses Servers mit einer bestätigten E-Mail-Adresse, deren Konto weder gesperrt noch zur Löschung eingeplant ist. Ein Abonnement ist nicht nötig. Die Starts sind auf 5 pro Stunde und 20 pro Tag für jeden Nutzer sowie auf 100 pro Stunde für den ganzen Server begrenzt, was Card Testing (das Ausprobieren gestohlener Karten mit kleinen Zahlungen) stoppt; `server_limit_reached` sagt, dass das Limit des Servers in der letzten Stunde erreicht wurde.
- **Bezahlen**: Nutzer erreichen die Trinkgeld-Seite (`/settings/supporter/tip`) über den Abschnitt **Trinkgelder** der Info-Seite, unter der Finanzierungsanzeige in der Seitenspalte, und über die Seite der Unterstützungspläne; einem Besucher, der nicht angemeldet ist, wird angeboten, sich zuerst anzumelden. Die Seite bestätigt vor der Seite des Anbieters den Betrag, dass er einmal sofort bezahlt wird und dass nichts dafür geliefert wird, mit einem Link zu deinen Händlerangaben. Die gehostete Seite des Anbieters nimmt die eine Zahlung an, nur per Karte, und bleibt eine Stunde lang bezahlbar. Ein Nutzer hat höchstens ein offenes Trinkgeld: Ein weiteres zu starten macht zuerst die frühere Seite unbezahlbar, und wurde die frühere inzwischen bezahlt, wird der Nutzer zu deren Dankeseite geleitet, statt zweimal zu zahlen. In den offiziellen Mobil-Apps erscheinen die Trinkgelder nicht.
- **Einnahmen**: Ein bezahltes Trinkgeld wird einmal in dem Monat gebucht, in dem es bezahlt wurde, unabhängig davon, ob das Ereignis des Anbieters, die Rückkehr von der Seite oder der Abgleich es zuerst meldet. Seine Erstattungen und Rückbuchungen werden in ihrem eigenen Monat gebucht und von den Einnahmen abgezogen. Die Summen der Administratoren zeigen den Nettobetrag der Abonnements, der Trinkgelder und des übrigen Geldes des Kontos des Anbieters getrennt, und ihre Liste der Geldbewegungen zeigt, wer jedes Trinkgeld bezahlt hat, solange das Konto existiert. Die öffentlichen Summen, `/finance` und `/about/costs` zeigen nie ein einzelnes Trinkgeld, den Betrag der Trinkgelder oder wie viele es waren; steht `income_visibility` auf `plans`, endet die Liste der Pläne mit einer Zeile „Trinkgelder“ ohne Zahl.
- **Erstattungen und Streitfälle**: Der Server hat keine Erstattungsfunktion für Trinkgelder. Erstatte im Dashboard des Anbieters; eine vollständige Erstattung, eine Rückbuchung und ein gewonnener Streitfall werden beim Trinkgeld erfasst, eine teilweise Erstattung nur bei den Einnahmen. Jeder Streitfall kostet außerdem die Streitfallgebühr des Anbieters (bei Stripe in Japan zum Zeitpunkt des Schreibens ¥1.500), unabhängig vom Betrag des Trinkgelds.
- **Konten**: Das Löschen eines Kontos und seine Verbannung machen die Seite seines offenen Trinkgelds unbezahlbar. Das Löschen des Kontos löscht seine Trinkgelder; ihre Einnahmen bleiben, ohne den Nutzer. Der Anbieter behält die Zahlung und die E-Mail-Adresse, die seine Seite vorausgefüllt hat, was der Server nicht löschen kann: Sage das in deiner Datenschutzerklärung.

Die Metrik `my.supporter.tip_checkouts` zählt die Starts (`started`), die Starts, die die Limits abgelehnt haben (`rate_limited`), und die Trinkgelder, die bezahlt wurden (`paid`) oder unbezahlt endeten (`expired`). Zwei Beispiele für Alarme:

- Ein Anstieg der Starts: `sum(rate(my_supporter_tip_checkouts_total{my_outcome=~"started|rate_limited"}[15m]))` deutlich über seinem üblichen Niveau für 15 Minuten.
- Viele Starts, die nie bezahlt werden: `sum(increase(my_supporter_tip_checkouts_total{my_outcome="expired"}[1h])) / sum(increase(my_supporter_tip_checkouts_total{my_outcome="started"}[1h]))` über 0,8 bei mindestens 20 Starts.

Ob ein Anbieter Trinkgelder akzeptiert, klärst du mit ihm im Rahmen seiner Prüfung und seiner Bedingungen. Formuliere jeden Text der Trinkgelder, deine Händlerangaben eingeschlossen, als Trinkgeld für den Betrieb des Dienstes, nie als Spende, als Abzug oder als Spendenaktion für einen Zweck.

<!-- source: what-operators-must-check-not-yet-verified 4981ac7a2d03 -->
## Was Betreiber prüfen müssen (noch nicht verifiziert)

mycroblog beurteilt nichts davon. Prüfe es anhand der Gesetze deines Landes und der Bedingungen deines Anbieters.

- Die Angaben, die die Händlerangaben enthalten müssen (Name, Adresse und Telefonnummer des Verkäufers, der Preis, wann und wie bezahlt wird, wann der Dienst erbracht wird, wie gekündigt wird und so weiter) und wie sie veröffentlicht werden müssen.
- Dass der abschließende Bestätigungsbildschirm (Artikel 12-6 des Gesetzes über bestimmte Handelsgeschäfte) die Menge, den Preis, den Zahlungszeitpunkt, den Lieferzeitpunkt sowie die Art der Kündigung und ihre Wirkung zeigt, wenn man die Zahlungsseite des Anbieters und den Bestätigungsbildschirm von `my` zusammen betrachtet.
- Die Behandlung der Verbrauchsteuer und ob du qualifizierte Rechnungen ausstellst.
- Bei Trinkgeldern: ob dein Anbieter Trinkgelder für den Betrieb eines Dienstes akzeptiert (PAY.JP zählt „Trinkgeld“ zu den Waren, die es schwer genehmigt, und Stripe in Japan nimmt sie bei einem Einzelunternehmer möglicherweise nicht an; frage den Anbieter), was die Händlerangaben dazu sagen (was ein Trinkgeld ist, die Beträge, dass es sofort bezahlt wird, dass nichts dafür geliefert wird und wie Erstattungen gehandhabt werden) sowie ihre Verbrauchsteuer und Einkommensteuer, die ein Steuerberater bestätigen sollte.
