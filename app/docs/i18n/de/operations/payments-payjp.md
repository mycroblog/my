<!-- source: supporter-payments-with-payjp 1f782a11b125 -->
# Unterstützer-Zahlungen mit PAY.JP

Mit `payments = payjp` abonnieren lokale Nutzer monatliche oder jährliche Unterstützungspläne auf der Zahlungsseite dieses Servers, und die Rolle des Tarifs wird vergeben und entzogen, wenn sich der Zahlungsstatus ändert.
Die Schlüssel des Deployments sind in der [Konfigurationsreferenz](../reference/configuration.md#payments) beschrieben. Für das Programm als Ganzes (Tarife, Rollen, die Liste der Mitglieder) siehe [supporters.md](supporters.md).
Ein Administrator aktiviert das Programm und legt in der Administration Unterstützungspläne an. Diese Einstellung erlaubt dem Server lediglich, mit PAY.JP zu sprechen.

Ein Server kann nur einen Anbieter nutzen. Um von Stripe zu PAY.JP zu wechseln (oder umgekehrt), bitte die bestehenden Abonnenten, bei dem neuen Anbieter erneut zu abonnieren.

<!-- source: what-the-server-does-at-payjp a8abbf012e10 -->
## Was der Server bei PAY.JP tut

PAY.JP hat die API v1 und v2. v2 hat noch keine API für wiederkehrende Abrechnung, daher nutzt der Server die wiederkehrende Abrechnung von **API v1** (Plan und Subscription). v1 hat weder eine gehostete Zahlungsseite noch ein Kundenportal, deshalb liefert dieser Server stattdessen eigene Seiten aus.

[Wie ein Abo funktioniert](supporters.md#how-a-subscription-works) zeigt in einem Diagramm, wie die Zahlungsseite, der Webhook, der Abgleich, die Rolle des Tarifs und die Einnahmen zusammenpassen.

- **Unterstützungspläne**: Für jeden Preis legt der Server einen Plan an, dessen ID `my_<plan ID>_<month|year>_<amount>` lautet (`metadata.my_plan_id`). Ein PAY.JP-Plan kann seinen Betrag nicht ändern; eine Änderung des Betrags legt also einen neuen Plan an, und der alte Plan wird nicht gelöscht, sondern erhält `metadata.my_archived=1`. Abos auf dem alten Plan werden zu dessen Betrag verlängert.
  Bearbeite oder lösche diese Pläne nicht in der Administration von PAY.JP. Die einzige Währung ist der japanische Yen, und die Beträge liegen zwischen 50 und 9.999.999 Yen.
- **Abonnieren**: Der Server schickt den Browser des Nutzers auf seine eigene Zahlungsseite (`/payments/pages/payjp/checkout`). Die Seite zeigt die Kartenfelder von payjp.js (dem iframe von PAY.JP) und führt die 3-D-Secure-Authentifizierung aus, wenn sie das Token erzeugt (der iframe-Typ).
  Der Server akzeptiert nur ein Token, dessen 3-D Secure abgeschlossen ist (`three_d_secure_status` ist `verified` oder `attempted`), fügt die Karte dem Kunden als Standardkarte hinzu (beim ersten Mal mit der ID `my_<the server's subscription ID>`) und legt ein Abo an, das die Abo-ID des Servers in `metadata.my_subscription_id` trägt. Die erste Abbuchung erfolgt, wenn das Abo angelegt wird.
  Die URL der Seite ist kurzlebig und trägt einen vom Server signierten Zustand (ab der Erstellung 1 bis 2 Stunden gültig).
  Ein Nutzer, der die Seite verlässt und neu beginnt, mit demselben Unterstützungsplan oder einem anderen, erhält eine neue URL für den Unterstützungsplan, den er jetzt gewählt hat. Eine frühere URL funktioniert weiter, bis sie abläuft, und die Seite, die zuerst bezahlt wird, erzeugt das einzige Abo: Die anderen leiten dorthin weiter. Wurde eine frühere Seite schon bezahlt, gelangt der Nutzer auf den Rückkehrbildschirm und erhält keine neue URL.
  Eine signierte URL lässt sich nicht zurückziehen, daher bleibt eine Seite bezahlbar, nachdem ihr ausstehendes Abo geschlossen wurde (ihr Unterstützungsplan eingestellt, ihr Konto gelöscht). Eine Zahlung darauf kauft nichts: Der Server löscht das Abo und erstattet seine Abbuchungen vollständig (PAY.JP akzeptiert eine Erstattung 180 Tage nach dem Verkauf).
  Die API v1 kennt keinen Idempotenzschlüssel; während also ein Absenden der Seite das Abo anlegt, markiert der Server den Checkout in der Datenbank als in Bearbeitung: Ein zweites Absenden (ein Doppelklick, ein anderer Tab oder ein anderer Serverprozess hinter demselben Load Balancer) wartet bis zu 30 Sekunden und geht dann zu dem Abo weiter, das das erste angelegt hat. Die Markierung läuft nach 5 Minuten ab, wenn der Prozess, der sie hielt, gestoppt wurde.
- **Die Seite für Karte und Zahlungsverlauf** (anstelle des Kundenportals von Stripe): Von seinem Abo-Bildschirm aus öffnet ein Abonnent `/payments/pages/payjp/portal`, um die Karte zu ändern und die letzten Zahlungen zu prüfen (die URL läuft nach einer Stunde ab). Das Formular beginnt mit der E-Mail-Adresse des PAY.JP-Kunden, also der Adresse, die bei der ersten Zahlung eingegeben wurde, nicht einer späteren Änderung der Adresse des Kontos.
  Das Ändern der Karte setzt ein Abo fort, das durch eine fehlgeschlagene Zahlung gestoppt wurde, und bucht sofort ab. Kündigen und Tarifwechsel erfolgen auf dem Abo-Bildschirm dieses Servers.
- **Zahlungsausfall**: PAY.JP pausiert (`paused`) ein Abo, dessen Verlängerungsabbuchung fehlgeschlagen ist, und **versucht es nicht erneut**. Der Server behandelt die Pause als verspätete Zahlung und behält die Rolle für eine Karenzzeit von 7 Tagen ab dem Zeitpunkt der Pause. Ändert der Abonnent in dieser Zeit die Karte, wird das Abo fortgesetzt; andernfalls wird die Rolle nach 7 Tagen entzogen (auf der Seite von PAY.JP bleibt das Abo pausiert und wird nicht abgebucht).
- **Wechsel des Unterstützungsplans**: Ein Upgrade bucht die Differenz für den Rest der laufenden Periode anteilig als Einmalzahlung ab (keine Abbuchung unter 50 Yen), ersetzt den Unterstützungsplan und bucht am Ende der Periode den vollen Betrag des neuen Unterstützungsplans ab (der Beginn der Periode verschiebt sich nicht). Wird die Abbuchung der Differenz abgelehnt, ändert sich der Unterstützungsplan nicht.
  Ein Downgrade und ein Wechsel von jährlich zu monatlich greifen mit `next_cycle_plan` am Ende der Periode.
- **Zustand zurücklesen**: Der Server behandelt einen Webhook als Signal und liest das Abo und die Geldbewegungen aus der PAY.JP-API zurück, um den Zustand zu bestimmen.
  Zusätzlich liest ein Abgleich alle `payments_reconcile_interval` die Ereignisliste, die Geldbewegungen und einen Durchlauf über alle Abos und behebt jeden Webhook, den der Server verpasst hat.
- **Einnahmen**: Für jede Zahlung erfasst der Server den Bruttobetrag, die Gebühr (berechnet aus dem `fee_rate` der Abbuchung, unter einem Yen abgerundet) und den Nettobetrag. Eine Erstattung und eine Rückbuchung erfassen die zurückgegebene Gebühr als negative Gebühr.
  Wenn eine Periode abgeschlossen und eine Abrechnung ausgestellt wird, wird die Differenz zwischen der Gebühr der Abrechnung und der Berechnung des Servers als Gebührenbewegung erfasst (ihre ID ist die ID der Abrechnung), sodass die Gebührensumme der Periode mit der Abrechnung übereinstimmt.
  Eine Abrechnung deckt die Gebühren des ganzen Kontos ab, daher **nutze ein PAY.JP-Konto, das diesem Server vorbehalten ist**.
- **Daten**: Einnahmedaten werden in japanischer Zeit gezählt.

Der Server sendet höchstens 20 Anfragen pro Sekunde (2 im Testmodus). Das bleibt innerhalb des Ratenlimits von PAY.JP (für Anfragen mit dem Secret Key 30 pro Sekunde im Live-Modus und 2 im Testmodus). Bei `429` wartet der Server 1 Sekunde, verdoppelt die Wartezeit jedes Mal und versucht es bis zu 3-mal erneut.
Im Testmodus ist ein einzelner Abgleich langsam (etwa 8 Minuten am Limit von 1.000 Anfragen). Ein Fortsetzungsjob übernimmt, es ist also keine Aktion nötig.

<!-- source: card-data-and-pci-dss e7247e5fea0f -->
## Kartendaten und PCI DSS

Kartennummern, Ablaufdaten und Sicherheitscodes werden nur im iframe von PAY.JP eingegeben, den payjp.js anzeigt. Die Seite und das Skript dieses Servers berühren sie nie. Der Server erhält nur die Token-ID, die payjp.js erzeugt hat, sowie den Namen des Karteninhabers und die E-Mail-Adresse, die 3-D Secure braucht.
Der Server speichert nur PAY.JP-Objekt-IDs (`cus_`, `sub_`, `ch_`, Plan-IDs, Ereignis-IDs), Beträge, Zeiträume und Zustände. Er speichert keine Kartennummern, keine letzten vier Ziffern und keine Ablaufdaten (die Kartenseite liest die letzten vier Ziffern, das Ablaufdatum und die E-Mail-Adresse des Kunden nur von PAY.JP und zeigt sie an). Webhook-Bodies und PAY.JP-Antworten werden weder in den Logs noch in der Datenbank aufbewahrt.

Die Zahlungsseite wird vom Origin dieses Servers ausgeliefert und ist daher im Hinblick auf die Verwaltung von Zahlungsseiten-Skripten (Anforderung 6.4.3) und die Erkennung von Manipulationen (Anforderung 11.6.1) von PCI DSS v4 gebaut.

- Die Seite lädt nur `https://js.pay.jp/v2/pay.js` und ein kleines Skript mit einer Nonce, das in die Seite eingebettet ist. Sie enthält weder das Bundle des Web-Clients noch Analyse- oder Werbeskripte.
- Die Seite trägt immer ihre eigene Content-Security-Policy (`script-src` nur die Nonce und `https://js.pay.jp`, `frame-src` nur `https://js.pay.jp` und `https://api.pay.jp`, `form-action 'self'`, `frame-ancestors 'none'`), `Referrer-Policy: no-referrer`, `Cache-Control: no-store` und `X-Content-Type-Options: nosniff`. Keine Einstellung ändert die CSP.
- Die Vorlage und das Skript sind in die Binärdatei des Servers eingebettet und lassen sich zur Laufzeit nicht ändern. Formulare werden nur vom selben Origin angenommen.

Daher ist zu erwarten, dass der Geltungsbereich des Betreibers dem eines Händlers entspricht, der die Zahlungseingabe dem iframe von PAY.JP überlässt (SAQ A). Prüfe den geltenden Geltungsbereich und die Art des Selbstbeurteilungsfragebogens anhand der Hinweise von PAY.JP und der Anforderungen deiner Kartenmarken.
Dabei hilft es, diese Punkte zu dokumentieren: die URLs der Zahlungsseiten (`/payments/pages/payjp/checkout` und `/portal`), die Liste der Skripte, die die Seite lädt (die beiden oben), die CSP der Seite und dass der Datenverkehr zum Server TLS nutzt (die Konfiguration des Reverse-Proxys).

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

<!-- source: 3-register-the-webhook 1afaf50fdaef -->
## 3. Den Webhook registrieren

Füge unter „Settings > Webhook“ im PAY.JP-Dashboard ein Ziel hinzu. Registriere Testmodus und Live-Modus getrennt.

- URL: `https://<server domain>/payments/webhooks/payjp`
- Modus: derselbe wie bei den Schlüsseln

Ein PAY.JP-Webhook identifiziert seinen Absender mit dem **Origin-Token** des Kontos (`whook_...`, im Header `X-Payjp-Webhook-Token`). Übergib das im Dashboard angezeigte Token in der Umgebungsvariable `MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN` (oder `_FILE`).
Der Endpunkt vergleicht das Token in konstanter Zeit und weist ein Testereignis, das mit einem Live-Schlüssel eingeht (und umgekehrt), mit `400` ab. Es gibt keine Signatur und keinen Zeitstempel; jeder, der das Token kennt, kann also eine Benachrichtigung senden, aber der Server schreibt nie einen Zustand aus dem Inhalt einer Benachrichtigung, sondern liest den Zustand nur von PAY.JP zurück.

Der Server nutzt diese Ereignisse (andere Typen werden angenommen, aber nicht erfasst).

- `subscription.created`, `subscription.updated`, `subscription.deleted`, `subscription.paused`, `subscription.resumed`, `subscription.canceled`, `subscription.renewed`
- `charge.succeeded`, `charge.failed`, `charge.refunded`
- `dispute.created`
- `statement.created`

PAY.JP sendet eine Benachrichtigung nur alle 3 Minuten erneut, höchstens 3-mal, und eine Benachrichtigung darüber hinaus kommt nicht an, bis du sie im Dashboard erneut sendest. Der Abgleich behebt, was verpasst wurde, daher **wird empfohlen, `payments_reconcile_interval` auf 1 Stunde zu setzen**.
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

<!-- source: 7-operation fbdd81461f80 -->
## 7. Betrieb

- Ein Abgleichsfehler der Art `permission` bedeutet einen falschen Secret Key.
- `duplicate_charge` in den Warnungen des Abschnitts „Zahlungsanbieter“ bedeutet, dass der Server die Erstattung einer Zahlung für ein geschlossenes Abo nicht abschließen konnte. Die Logzeile `a second subscription could not be canceled and refunded` nennt das Abo, und jeder Abgleich versucht es erneut, auch nachdem das Abo gelöscht wurde. Erstatte die Abbuchung im Dashboard, wenn es anhält: Der nächste Versuch findet sie erstattet vor und löscht die Warnung, solange PAY.JP die Abbuchungen des gelöschten Abos noch auflistet (siehe [Zu überprüfende Punkte](#items-to-verify)). Eine Abbuchung, die älter als 180 Tage ist, kann nicht erstattet werden: Der Server loggt sie und macht weiter.
- Andere Erstattungen werden im PAY.JP-Dashboard abgewickelt. Eine vollständige Erstattung und eine Rückbuchung entziehen die Rolle und löschen das Abo sofort, und eine Teilerstattung entzieht die Rolle nicht.
- **Eine Rückbuchungsstornierung** wird nicht automatisch erfasst, weil PAY.JP dafür weder ein Ereignis noch eine API sendet. Wenn eine eintritt, erfasse den stornierten Betrag auf dem Einnahmenbildschirm der Kostenseite (`finance`). Die Rolle wird nicht automatisch wiederhergestellt.
- **Die Gebühren für bezahlte Unterstützungspläne und Überweisungsgebühren von PAY.JP** werden nicht von den Einnahmen abgezogen. Erfasse sie als Kosten in `finance`.
- Wenn Abgleiche oft an das Anfragelimit stoßen, erhöhe `payments_reconcile_max_requests` oder verlängere `payments_reconcile_interval`.

<!-- source: plan-changes 29dec3be3d95 -->
## Wechsel des Unterstützungsplans

Ein Upgrade bucht zuerst die anteilige Differenz ab und verschiebt den Unterstützungsplan erst, nachdem diese Abbuchung gelungen ist. Wird die Abbuchung abgelehnt, bleibt der Unterstützungsplan bestehen, und der Nutzer sieht einen Fehler mit dem Weg, die Karte zu aktualisieren, ohne Karenzzeit und ohne Benachrichtigung über eine fehlgeschlagene Zahlung; ein neuer Versuch mit einer funktionierenden Karte bucht einmal ab. Gelingt das Verschieben des Unterstützungsplans nach der Abbuchung nicht, bucht ein neuer Versuch nicht erneut ab. Eine Differenz unter dem Minimum von 50 Yen und ein Wechsel auf den Jahrespreis desselben Unterstützungsplans, der keine positive Differenz hat, buchen jetzt nichts ab und wechseln sofort; der neue Preis wird am Ende der Periode abgebucht, und ein Fehlschlag dann ist eine fehlgeschlagene Verlängerung.

<!-- source: items-to-verify 3c2cbf6ed2b0 -->
## Zu überprüfende Punkte

Die folgenden Verhaltensweisen sind weder durch die Dokumentation von PAY.JP noch durch den Testmodus geklärt.

- Wie die bei einer Rückbuchung zurückgegebene Gebühr berechnet wird und ob die Abrechnung einer Periode mit den Gebühren des Servers übereinstimmt (der Testmodus zeigte eine abgerundete Gebühr und eine aus ihrem Betrag berechnete Gebühr einer Erstattung).
- Was eine echte Rückbuchung bewirkt: PAY.JP dokumentiert keinen Weg, im Testmodus eine auszulösen, und keinen Body für `dispute.created`. Ein Test-Senden des Ereignisses aus dem Dashboard wird mit `200` beantwortet und ändert nichts.
- Wie weit die Ereignisliste zurückreicht.
- Ob das Löschen eines Plans bestehende Abos betrifft (der Server löscht nie einen) und wie lang eine Plan-ID höchstens sein darf.
- Ob der CSP-Bericht beim Laden der Zahlungsseite eine Rolle spielt: `pay.js` wendet einen Inline-Stil gegen `style-src 'self'` an, während die Kartenfelder und 3-D Secure in Chromium funktionieren. Andere Browser wurden nicht geprüft.
- Ob payjp-cli v1-Ereignisse weiterleiten kann.
- Dass eine Abbuchung von der gespeicherten Karte eines Kunden kein 3-D Secure verlangt, es sei denn, die Anfrage verlangt es. Eine abgelehnte Abbuchung wird von `GET /v1/charges` mit `paid: false` aufgelistet, und der Server ignoriert eine, die nicht bezahlt ist.
- Dass `GET /v1/charges` mit `subscription` die Abbuchungen eines gelöschten Abos weiterhin auflistet. Der Server erstattet, bevor er löscht; nur ein Versuch nach einer endgültig fehlgeschlagenen Erstattung verlässt sich darauf. Die API-Referenz dokumentiert den Filter und sagt nichts zu gelöschten Abos. Kommt die Liste leer zurück, meldet der Server einen Fehler statt einer Erstattung, und `duplicate_charge` bleibt auch bestehen, nachdem die Abbuchung von Hand erstattet wurde.
