<!-- source: supporter-payments-with-payjp 6d397d24cf93 -->
# Unterstützer-Zahlungen mit PAY.JP

Mit `payments = payjp` schließen lokale Benutzer monatliche oder jährliche Pläne auf der Zahlungsseite dieses Servers ab, und die Rolle des Plans wird vergeben und entzogen, wenn sich der Zahlungszustand ändert. Wenn Trinkgelder aktiviert sind, zahlen Benutzer einmalige Trinkgelder außerdem auf der gehosteten Seite von PAY.JP (siehe [Trinkgelder](#tips)).
Die Schlüssel der Bereitstellung sind in der [Konfigurationsreferenz](../reference/configuration.md#payments) beschrieben. Zum Programm als Ganzem (Pläne, Rollen, die Liste der Mitglieder) siehe [supporters.md](supporters.md).
Ein Administrator aktiviert das Programm und erstellt Pläne in der Administration. Diese Einstellung lässt den Server nur mit PAY.JP sprechen.

Ein Server kann nur einen Anbieter nutzen. Um von Stripe zu PAY.JP zu wechseln (oder umgekehrt), bitte die bestehenden Abonnenten, beim neuen Anbieter erneut zu abonnieren.

<!-- source: what-the-server-does-at-payjp afde8e828e80 -->
## Was der Server bei PAY.JP tut

PAY.JP hat die API v1 und v2. v2 hat noch keine API für wiederkehrende Abrechnung, daher nutzt der Server die wiederkehrende Abrechnung von **API v1** (Plan und Subscription). v1 hat weder eine gehostete Zahlungsseite noch ein Kundenportal, daher liefert dieser Server stattdessen Seiten aus.
Trinkgelder sind einmalige Zahlungen über die **API v2**, die eine gehostete Zahlungsseite hat; siehe [Trinkgelder](#tips). PAY.JP erlaubt beide Versionen auf einem Konto mit denselben Schlüsseln.

[Wie ein Abonnement funktioniert](supporters.md#how-a-subscription-works) zeigt in einem Diagramm, wie die Zahlungsseite, der Webhook, der Abgleich, die Rolle des Plans und die Einnahmen zusammenpassen.

- **Pläne**: Für jeden Preis erstellt der Server einen Plan, dessen ID `my_<plan ID>_<month|year>_<amount>` ist (`metadata.my_plan_id`). Ein PAY.JP-Plan kann seinen Betrag nicht ändern, daher erstellt eine Änderung des Betrags einen neuen Plan, und der alte Plan wird nicht gelöscht, sondern erhält `metadata.my_archived=1`. Abonnements auf dem alten Plan verlängern sich zu seinem Betrag.
  Bearbeite oder lösche diese Pläne nicht in der Administration von PAY.JP. Die einzige Währung ist der japanische Yen, und die Beträge liegen zwischen 50 und 9.999.999 Yen.
- **Abonnieren**: Der Server schickt den Browser des Benutzers zu seiner eigenen Zahlungsseite (`/payments/pages/payjp/checkout`). Die Seite zeigt die Kartenfelder von payjp.js (dem iframe von PAY.JP) und führt die 3-D-Secure-Authentifizierung aus, wenn sie das Token erstellt (der Typ iframe).
  Der Server akzeptiert nur ein Token, dessen 3-D Secure abgeschlossen ist (`three_d_secure_status` ist `verified` oder `attempted`), fügt die Karte dem Kunden als Standardkarte hinzu (beim ersten Mal mit der ID `my_<the server's subscription ID>`) und erstellt ein Abonnement mit der Abonnement-ID des Servers in `metadata.my_subscription_id`. Die erste Abbuchung erfolgt, wenn das Abonnement erstellt wird.
  Die URL der Seite ist kurzlebig und trägt einen vom Server signierten Zustand (ab der Erstellung 1 bis 2 Stunden gültig).
  Ein Benutzer, der die Seite verlässt und erneut beginnt, für denselben Plan oder einen anderen, erhält eine neue URL für den Plan, den er jetzt gewählt hat. Eine frühere URL funktioniert weiter, bis sie abläuft, und die Seite, die zuerst bezahlt wird, ergibt das einzige Abonnement: Die anderen führen zu ihm weiter. Wurde eine frühere Seite schon bezahlt, geht der Benutzer zum Rückkehrbildschirm und erhält keine neue URL.
  Eine signierte URL lässt sich nicht zurückziehen, daher bleibt eine Seite bezahlbar, nachdem ihr ausstehendes Abonnement geschlossen hat (sein Plan ist eingestellt, sein Konto gelöscht). Eine Zahlung darauf kauft nichts: Der Server löscht das Abonnement und erstattet seine Abbuchungen vollständig (PAY.JP akzeptiert eine Erstattung 180 Tage nach dem Verkauf).
  Die API v1 nimmt keinen Idempotency-Key, daher markiert der Server, während eine Abschickung der Seite das Abonnement erstellt, den Checkout in der Datenbank als in Arbeit: Eine zweite Abschickung (ein Doppelklick, ein anderer Tab oder ein anderer Serverprozess hinter demselben Load Balancer) wartet bis zu 30 Sekunden und geht dann zu dem Abonnement weiter, das die erste erstellt hat. Die Markierung läuft nach 5 Minuten ab, wenn der Prozess, der sie hielt, angehalten hat.
- **Die Seite für Karte und Zahlungsverlauf** (anstelle des Kundenportals von Stripe): Von seinem Abonnementbildschirm aus öffnet ein Abonnent `/payments/pages/payjp/portal`, um die Karte zu ändern und die letzten Zahlungen zu prüfen (die URL läuft nach einer Stunde ab). Das Formular beginnt mit der E-Mail-Adresse des PAY.JP-Kunden, der Adresse, die bei der ersten Zahlung eingegeben wurde, nicht einer späteren Änderung der Adresse des Kontos.
  Das Ändern der Karte nimmt ein durch eine fehlgeschlagene Zahlung gestopptes Abonnement wieder auf und bucht sofort ab. Kündigen und Pläne wechseln erfolgen auf dem Abonnementbildschirm dieses Servers.
- **Zahlungsausfall**: PAY.JP pausiert (`paused`) ein Abonnement, dessen Verlängerungsabbuchung fehlgeschlagen ist, und **wiederholt nicht**. Der Server behandelt die Pause als verspätete Zahlung und behält die Rolle für eine Kulanzfrist von 7 Tagen ab dem Zeitpunkt der Pause. Ändert der Abonnent in dieser Zeit die Karte, wird das Abonnement fortgesetzt; wenn nicht, wird die Rolle nach 7 Tagen entzogen (auf der Seite von PAY.JP bleibt das Abonnement pausiert und wird nicht abgebucht).
- **Planwechsel**: Ein Upgrade bucht die Differenz für den Rest des aktuellen Zeitraums anteilig als einmalige Abbuchung ab (keine Abbuchung unter 50 Yen), ersetzt den Plan und bucht den vollen Betrag des neuen Plans am Ende des Zeitraums ab (der Beginn des Zeitraums verschiebt sich nicht). Wird die Abbuchung der Differenz abgelehnt, ändert sich der Plan nicht.
  Ein Downgrade und ein Wechsel von jährlich auf monatlich wechseln zum Ende des Zeitraums mit `next_cycle_plan`.
- **Zustand zurücklesen**: Der Server behandelt einen Webhook als Signal und liest das Abonnement und die Geldbewegungen aus der PAY.JP-API zurück, um den Zustand zu bestimmen.
  Zusätzlich liest ein Abgleich alle `payments_reconcile_interval` die Ereignisliste, die Geldbewegungen und einen Durchlauf aller Abonnements und repariert jeden Webhook, den der Server verpasst hat.
- **Einnahmen**: Für jede Zahlung erfasst der Server den Bruttobetrag, die Gebühr (berechnet aus dem `fee_rate` der Abbuchung, unter einem Yen abgerundet) und den Nettobetrag. Eine Erstattung und eine Rückbuchung erfassen die zurückgegebene Gebühr als negative Gebühr.
  Wenn ein Zeitraum schließt und eine Abrechnung ausgestellt wird, wird die Differenz zwischen der Gebühr der Abrechnung und der Berechnung des Servers (die Gebühren der Abbuchungen der Abonnements und der Trinkgelder des Zeitraums) als Gebührenbewegung erfasst (ihre ID ist die ID der Abrechnung), damit die Gebührensumme des Zeitraums mit der Abrechnung übereinstimmt.
  Eine Abrechnung deckt die Gebühren des ganzen Kontos ab, **nutze also ein PAY.JP-Konto, das diesem Server gewidmet ist**.
- **Daten**: Einnahmedaten werden in japanischer Zeit gezählt.

Der Server sendet höchstens 20 Anfragen pro Sekunde (2 im Testmodus), API v1 und v2 zusammen. Das bleibt innerhalb des Ratenlimits von PAY.JP (für Anfragen mit dem geheimen Schlüssel 30 pro Sekunde im Live-Modus und 2 im Testmodus). Bei `429` wartet der Server 1 Sekunde, jedes Mal verdoppelt, und wiederholt bis zu 3 Mal.
Im Testmodus ist ein einzelner Abgleich langsam (etwa 8 Minuten beim Limit von 1.000 Anfragen). Ein Fortsetzungsjob übernimmt, sodass keine Aktion nötig ist.

<!-- source: tips b0917978ffa6 -->
## Trinkgelder

Trinkgelder (siehe [supporters.md](supporters.md#tips)) nutzen die API v2 von PAY.JP mit denselben Schlüsseln und demselben Webhook-Token wie die Abonnements. PAY.JP führt Trinkgeld unter den Waren auf, die es schwer genehmigt: Ob dein Konto Trinkgelder annehmen darf, regelst du mit PAY.JP im Rahmen seiner Prüfung und seiner Bedingungen.

- **Bezahlen**: Der Server erstellt eine Checkout Session (`mode=payment`) über den einen Betrag, nur per Karte, die nach einer Stunde abläuft, und schickt den Benutzer zu ihrer Seite bei PAY.JP (eine `https`-Seite auf `pay.jp`; der Server lehnt jede andere Adresse ab). Die Trinkgeld-ID des Servers steht im `client_reference_id` der Session und in `metadata.my_tip_id` der Session und ihres PaymentFlow. Die Erstellung trägt einen `Idempotency-Key`, sodass eine Wiederholung dieselbe Session erhält. PAY.JP führt auf der gehosteten Seite 3-D Secure aus, wenn der Kartenanbieter es verlangt.
  Ein Trinkgeld ist bezahlt, wenn seine Session `complete` ist und sein PaymentFlow erfolgreich war. Beginnt ein Benutzer ein weiteres Trinkgeld, lässt der Server zuerst die offene Session ablaufen (`POST /v2/checkout/sessions/{id}/expire`); hat der Benutzer sie in der Zwischenzeit bezahlt, wird stattdessen diese Zahlung erfasst.
- **Einnahmen**: Das Geld der Trinkgelder stammt aus den Zahlungstransaktionen des Kontos der API v2: einer Zahlung (ihre ID ist die des PaymentFlow), einer Erstattung, einer Rückbuchung und der Aufhebung einer Rückbuchung, jeweils mit der Gebühr, die PAY.JP meldet. Eine Erstattung und eine Rückbuchung erfassen die zurückgegebene Gebühr als negative Gebühr, wie bei den Abonnements.
- **Streitfälle**: PAY.JP sendet ein Ereignis nur, wenn ein Streitfall eröffnet wird. Jeder Abgleich liest außerdem die Streitfälle, die zu deinen Gunsten endeten (Status `cancel`), und erfasst die Aufhebung einer Rückbuchung, die die Zahlungstransaktionen nicht selbst melden. Ein Streitfall, der ohne Rückbuchung endete (eine Anfrage), erfasst nichts.
- **Erstattungen**: Erstatte ein Trinkgeld im PAY.JP-Dashboard. Eine vollständige Erstattung und eine Rückbuchung werden beim Trinkgeld erfasst; eine teilweise Erstattung nur bei den Einnahmen.

<!-- source: card-data-and-pci-dss 7be9976efb0b -->
## Kartendaten und PCI DSS

Kartennummern, Ablaufdaten und Sicherheitscodes werden nur im iframe von PAY.JP eingegeben, das payjp.js zeigt. Die Seite und das Skript dieses Servers berühren sie nie. Der Server erhält nur die Token-ID, die payjp.js erstellt hat, sowie den Namen des Karteninhabers und die E-Mail-Adresse, die 3-D Secure braucht.
Der Server speichert nur PAY.JP-Objekt-IDs (`cus_`, `sub_`, `ch_`, Plan-IDs, Ereignis-IDs sowie die Checkout Sessions, PaymentFlows, Erstattungen und Streitfälle der Trinkgelder), Beträge, Zeiträume und Zustände. Die Karte eines Trinkgelds wird auf der eigenen Seite von PAY.JP eingegeben, die dieser Server nie ausliefert. Er speichert keine Kartennummern, letzten vier Ziffern oder Ablaufdaten (die Kartenseite liest die letzten vier Ziffern, das Ablaufdatum und die E-Mail-Adresse des Kunden nur von PAY.JP und zeigt sie an). Webhook-Bodys und PAY.JP-Antworten werden weder in den Logs noch in der Datenbank behalten.

Die Zahlungsseite wird vom Origin dieses Servers ausgeliefert, daher ist sie im Hinblick auf die Verwaltung von Skripten auf Zahlungsseiten (Anforderung 6.4.3) und die Erkennung von Manipulation (Anforderung 11.6.1) von PCI DSS v4 gebaut.

- Die Seite lädt nur `https://js.pay.jp/v2/pay.js` und ein kleines Skript mit einer Nonce, das in die Seite eingebettet ist. Sie trägt weder das Bundle des Webclients noch ein Analyse- oder Werbeskript.
- Die Seite trägt immer ihre eigene Content-Security-Policy (`script-src` nur die Nonce und `https://js.pay.jp`, `frame-src` nur `https://js.pay.jp` und `https://api.pay.jp`, `form-action 'self'`, `frame-ancestors 'none'`), `Referrer-Policy: no-referrer`, `Cache-Control: no-store` und `X-Content-Type-Options: nosniff`. Keine Einstellung ändert die CSP.
- Das Template und das Skript sind in das Binary des Servers eingebettet und lassen sich zur Laufzeit nicht ändern. Formular-Abschickungen werden nur vom selben Origin akzeptiert.

Dadurch wird erwartet, dass der Umfang des Betreibers der eines Händlers ist, der die Zahlungseingabe dem iframe von PAY.JP überlässt (SAQ A). Prüfe den Umfang, der für dich gilt, und die Art des Selbstbeurteilungsfragebogens anhand der Hinweise von PAY.JP und der Anforderungen deiner Kartenmarken.
Dabei hilft es, einen Nachweis dieser Punkte zu führen: die URLs der Zahlungsseite (`/payments/pages/payjp/checkout` und `/portal`), die Liste der Skripte, die die Seite lädt (die beiden oben), die CSP der Seite und dass der Verkehr zum Server TLS nutzt (die Konfiguration des Reverse-Proxys).

<!-- source: 1-prepare-the-payjp-account b01b53a61e40 -->
## 1. Das PAY.JP-Konto vorbereiten

1. Lege ein PAY.JP-Konto an und beginne im Testmodus. Abbuchungen im Live-Modus brauchen einen Produktivantrag und eine Prüfung.
2. Prüfe „API settings > 3-D Secure“ im PAY.JP-Dashboard. Der Server fordert bei jeder Token-Erzeugung 3-D Secure an; du kannst außerdem festlegen, dass 3-D Secure für jede Token-Erzeugung verlangt wird.
3. Die Seite der Angaben nach dem Handelsgesetz wird in der Administration dieses Servers festgelegt (siehe [supporters.md](supporters.md)).

<!-- source: 2-keys-and-the-webhook-token 7bee0ef9256a -->
## 2. Schlüssel und das Webhook-Token

Prüfe unter „API“ im PAY.JP-Dashboard diese beiden Schlüssel. Das Präfix eines Schlüssels bestimmt den Modus, und Testschlüssel buchen nichts ab. Beide Schlüssel müssen im selben Modus sein.

| Schlüssel | Beispiel | Verwendung |
| --- | --- | --- |
| Secret Key | `sk_test_...` / `sk_live_...` | Der Server ruft damit die API auf. Er ist auch die Quelle des Schlüssels, der Seiten-URLs signiert |
| Public Key | `pk_test_...` / `pk_live_...` | payjp.js auf den Zahlungs- und Kartenseiten |

Übergib den Secret Key in der Umgebungsvariable `MY_PAYMENTS_PAYJP_SECRET_KEY` (oder den Pfad einer Datei in `MY_PAYMENTS_PAYJP_SECRET_KEY_FILE`). Er kann nicht in die Konfigurationsdatei geschrieben werden.
Wenn der Secret Key bekannt wird, stelle ihn im Dashboard neu aus und führe ein erneutes Deployment mit dem neuen Schlüssel durch. Die URLs von Zahlungs- und Kartenseiten, die vor dem erneuten Deployment geöffnet waren, funktionieren dann nicht mehr.

<!-- source: 3-register-the-webhook 836afecf8d0c -->
## 3. Den Webhook registrieren

Füge unter „Settings > Webhook“ im PAY.JP-Dashboard ein Ziel hinzu. Registriere Testmodus und Live-Modus getrennt.

- URL: `https://<server domain>/payments/webhooks/payjp`
- Modus: derselbe wie bei den Schlüsseln

Ein PAY.JP-Webhook identifiziert seinen Absender mit dem **Origin-Token** des Kontos (`whook_...`, im Header `X-Payjp-Webhook-Token`). Übergib das im Dashboard angezeigte Token in der Umgebungsvariable `MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN` (oder `_FILE`).
Der Endpunkt vergleicht das Token in konstanter Zeit und lehnt ein Testereignis, das mit einem Live-Schlüssel empfangen wird (und umgekehrt), mit `400` ab. Es gibt keine Signatur und keinen Zeitstempel, sodass jeder, der das Token kennt, eine Benachrichtigung senden kann, aber der Server schreibt nie Zustand aus dem Inhalt einer Benachrichtigung; er liest den Zustand nur von PAY.JP zurück.

Der Server nutzt diese Ereignisse (andere Typen werden akzeptiert, aber nicht erfasst).

- `subscription.created`, `subscription.updated`, `subscription.deleted`, `subscription.paused`, `subscription.resumed`, `subscription.canceled`, `subscription.renewed`
- `charge.succeeded`, `charge.failed`, `charge.refunded`
- `dispute.created`
- `statement.created`
- Für Trinkgelder (API v2): `checkout.session.completed`, `checkout.session.expired`, `payment_flow.succeeded`, `refund.created`, `refund.updated`, `refund.failed` und `dispute.created`

Der Server unterscheidet die Ereignisse der API v1 und v2 an ihrer Form, sodass beide Versionen mit demselben Token an dieselbe URL gehen.

PAY.JP sendet eine Benachrichtigung nur alle 3 Minuten erneut, höchstens 3 Mal, und eine Benachrichtigung darüber hinaus kommt nicht an, bis du sie im Dashboard erneut sendest. Der Abgleich repariert, was verpasst wurde, daher wird **empfohlen, `payments_reconcile_interval` auf 1 Stunde zu setzen**.
Der Zustand des Anbieters (der Modus, wann der letzte Webhook ankam, und das Ergebnis des letzten Abgleichs) wird im Abschnitt „Zahlungsanbieter“ von `/admin/supporters` in der Administration angezeigt.

<!-- source: 4-configure-the-server b0d24c7a2da0 -->
## 4. Den Server konfigurieren

```sh
MY_PAYMENTS=payjp
MY_PAYMENTS_PAYJP_SECRET_KEY=sk_test_...
MY_PAYMENTS_PAYJP_PUBLIC_KEY=pk_test_...
MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN=whook_...
# The recommended reconciliation interval (15m to 168h, default 6h) and the most requests one reconciliation sends (default 1000)
MY_PAYMENTS_RECONCILE_INTERVAL=1h
MY_PAYMENTS_RECONCILE_MAX_REQUESTS=1000
```

`my config check` prüft die Konfiguration. Eine Binärdatei, die mit dem Build-Tag `no_payments_payjp` gebaut wurde, lehnt `payments = payjp` ab (siehe [build-tags.md](../reference/build-tags.md)).

<!-- source: 5-try-it-in-test-mode f6ebbada0e0e -->
## 5. Im Testmodus ausprobieren

Beginne mit Testschlüsseln. Testmodus und Live-Modus haben getrennte Pläne, Kunden, Abos, Webhooks und Tokens. Um live zu gehen, registriere den Webhook im Live-Modus, ersetze die Schlüssel und das Token durch die Live-Varianten und führe ein erneutes Deployment durch. Die Pläne für die Unterstützungspläne werden im Live-Konto neu angelegt.

Testkarten ([PAY.JP-Dokumentation](https://pay.jp/docs/testcard)):

| Nummer | Verhalten |
| --- | --- |
| `4242 4242 4242 4242` | Erfolgreich |
| `4000 0000 0008 0319` | Die Karte lässt sich registrieren, aber die Abbuchung wird abgelehnt (ein fehlgeschlagenes Abo und eine fehlgeschlagene Fortsetzung nach einem Kartenwechsel) |
| `4000 0000 0000 4012` | Die Karte lässt sich registrieren, aber die Abbuchung scheitert als abgelaufen |
| `4000 0000 0000 0002` | Das Registrieren der Karte (das Erzeugen des Tokens) wird abgelehnt |

Verwende ein Ablaufdatum in der Zukunft; der Sicherheitscode und der Name sind beliebig. Im Testmodus lässt dich der Testauthentifizierungsbildschirm von PAY.JP für 3-D Secure das Ergebnis wählen.

Um eine Verlängerung schnell auszuprobieren, setze `trial_end` des Abos mit der API einige Minuten in die Zukunft. Die Verlängerungsabbuchung läuft einige Minuten nach diesem Zeitpunkt, nicht genau dann. Eine weitere Aktualisierung von `trial_end`, während die erste fällig ist, startet das Warten neu.
Um einen Zahlungsausfall und die Karenzzeit auszuprobieren, abonniere, wechsle auf der Kartenseite zu der Karte, die abgelehnt wird (`4000 0000 0008 0319`), und lass das Abo dann verlängern.

<!-- source: 6-try-it-locally 219997c1a489 -->
## 6. Lokal ausprobieren

PAY.JP-Webhooks gehen an eine öffentliche URL; um sie auf einem Server auf deiner Maschine zu empfangen, mache den Port über einen Tunnel (cloudflared, ngrok und Ähnliches) erreichbar und registriere diese URL als Ziel im Testmodus.
`payjp listen --forward-to http://localhost:8080/payments/webhooks/payjp` von payjp-cli leitet voraussichtlich ebenfalls Ereignisse weiter, aber ob es v1-Ereignisse weiterleiten kann, wurde nicht überprüft.

Öffne die Zahlungs- und Kartenseiten auf dem Origin dieses Servers (sie funktionieren auch auf `http://localhost:8080`, aber das Verhalten von payjp.js und 3-D Secure hängt von den Einstellungen des Browsers ab).

<!-- source: 7-operation fae168baab1c -->
## 7. Betrieb

- Ein Abgleichsfehler der Art `permission` bedeutet einen falschen geheimen Schlüssel.
- `duplicate_charge` in den Warnungen des Abschnitts „Zahlungsanbieter“ bedeutet, dass der Server die Erstattung einer Zahlung für ein geschlossenes Abonnement nicht abschließen konnte. Die Logzeile `a second subscription could not be canceled and refunded` nennt das Abonnement, und jeder Abgleich versucht es erneut, auch nachdem das Abonnement gelöscht ist. Erstatte die Abbuchung im Dashboard, wenn es anhält: Der nächste Versuch findet sie erstattet vor und löscht die Warnung, solange PAY.JP die Abbuchungen des gelöschten Abonnements noch auflistet (siehe [Zu prüfende Punkte](#items-to-verify)). Eine Abbuchung, die älter als 180 Tage ist, kann nicht erstattet werden: Der Server loggt sie und macht weiter.
- Andere Erstattungen werden im PAY.JP-Dashboard abgewickelt. Eine vollständige Erstattung und eine Rückbuchung entziehen die Rolle und löschen das Abonnement sofort, und eine teilweise Erstattung entzieht die Rolle nicht.
- **Eine Aufhebung einer Rückbuchung** einer Abbuchung eines Abonnements wird nicht automatisch erfasst, weil die API v1 dafür weder ein Ereignis noch eine API hat (bei einem Trinkgeld gibt es sie, siehe [Trinkgelder](#tips)). Tritt eine auf, erfasse den aufgehobenen Betrag auf dem Einnahmenbildschirm der Kostenseite (`finance`). Die Rolle wird nicht automatisch wiederhergestellt.
- **Die Gebühren für kostenpflichtige Pläne und Überweisungsgebühren von PAY.JP** werden nicht von den Einnahmen abgezogen. Erfasse sie als Kosten in `finance`.
- Stoßen Abgleiche oft an das Anfragelimit, erhöhe `payments_reconcile_max_requests` oder verlängere `payments_reconcile_interval`.

<!-- source: plan-changes 29dec3be3d95 -->
## Wechsel des Unterstützungsplans

Ein Upgrade bucht zuerst die anteilige Differenz ab und verschiebt den Unterstützungsplan erst, nachdem diese Abbuchung gelungen ist. Wird die Abbuchung abgelehnt, bleibt der Unterstützungsplan bestehen, und der Nutzer sieht einen Fehler mit dem Weg, die Karte zu aktualisieren, ohne Karenzzeit und ohne Benachrichtigung über eine fehlgeschlagene Zahlung; ein neuer Versuch mit einer funktionierenden Karte bucht einmal ab. Gelingt das Verschieben des Unterstützungsplans nach der Abbuchung nicht, bucht ein neuer Versuch nicht erneut ab. Eine Differenz unter dem Minimum von 50 Yen und ein Wechsel auf den Jahrespreis desselben Unterstützungsplans, der keine positive Differenz hat, buchen jetzt nichts ab und wechseln sofort; der neue Preis wird am Ende der Periode abgebucht, und ein Fehlschlag dann ist eine fehlgeschlagene Verlängerung.

<!-- source: items-to-verify d5fb7fd9927f -->
## Zu prüfende Punkte

Die folgenden Verhaltensweisen sind weder durch die Dokumentation von PAY.JP noch durch den Testmodus geklärt.

- Wie die Gebühr berechnet wird, die eine Rückbuchung zurückgibt, und ob die Abrechnung eines Zeitraums mit den Gebühren des Servers übereinstimmt (der Testmodus zeigte eine abgerundete Gebühr und eine aus ihrem Betrag berechnete Gebühr einer Erstattung).
- Was eine echte Rückbuchung bewirkt: PAY.JP dokumentiert keinen Weg, im Testmodus eine auszulösen, und keinen Body für `dispute.created`. Ein Test-Versand des Ereignisses aus dem Dashboard wird mit `200` beantwortet und ändert nichts.
- Wie weit die Ereignisliste zurückreicht.
- Ob das Löschen eines Plans bestehende Abonnements betrifft (der Server löscht nie einen) und die maximale Länge einer Plan-ID.
- Ob der CSP-Bericht beim Laden der Zahlungsseite eine Rolle spielt: `pay.js` wendet einen Inline-Style gegen `style-src 'self'` an, während die Kartenfelder und 3-D Secure in Chromium funktionieren. Andere Browser wurden nicht geprüft.
- Ob payjp-cli Ereignisse der v1 weiterleiten kann.
- Dass eine Abbuchung der gespeicherten Karte eines Kunden kein 3-D Secure verlangt, es sei denn, die Anfrage tut es. Eine abgelehnte Abbuchung wird von `GET /v1/charges` mit `paid: false` aufgelistet, und der Server ignoriert eine, die nicht bezahlt ist.
- Bei Trinkgeldern über die API v2, die nach den Anleitungen und dem offiziellen SDK von PAY.JP gebaut und noch nicht im Testmodus ausprobiert wurden: dass PAY.JP die Ereignisse der v2 an dasselbe Ziel mit demselben Token sendet wie die der v1; dass die gehostete Seite auf `pay.jp` oder einer seiner Subdomains liegt; dass die Erstellung einer Checkout Session `Idempotency-Key` beachtet; dass die `resource_id` einer Zahlungstransaktion bei einer Zahlung der PaymentFlow ist, bei einer Erstattung die Erstattung und bei einer Rückbuchung und ihrer Aufhebung der Streitfall; dass die Listen die neuesten zuerst haben, mit `starting_after` und `ending_before`; die Vorzeichen von Betrag und Gebühr einer Transaktion; und dass die Abrechnung eines Zeitraums der v1 die Gebühren der v2 einschließt.
- Dass `GET /v1/charges` mit `subscription` die Abbuchungen eines gelöschten Abonnements weiterhin auflistet. Der Server erstattet, bevor er löscht, daher verlässt sich nur ein Versuch nach einer endgültig fehlgeschlagenen Erstattung darauf. Die API-Referenz dokumentiert den Filter und sagt nichts über gelöschte Abonnements. Kommt die Liste leer zurück, meldet der Server einen Fehler statt einer Erstattung, und `duplicate_charge` bleibt auch dann, wenn die Abbuchung von Hand erstattet wird.
