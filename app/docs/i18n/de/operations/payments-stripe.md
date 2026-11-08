<!-- source: supporter-payments-with-stripe f12ff4a0af71 -->
# Unterstützer-Zahlungen mit Stripe

Mit `payments = stripe` schließen lokale Nutzer Monats- oder Jahrespläne auf der gehosteten Zahlungsseite von Stripe (Checkout) ab, und die Rolle des Plans wird vergeben und entzogen, wenn sich der Zahlungsstatus ändert.
Die Schlüssel für die Bereitstellung stehen in der [Konfigurationsreferenz](../reference/configuration.md#payments). Zum Programm als Ganzem (Pläne, Rollen, die Mitgliederliste) siehe [supporters.md](supporters.md).
Ein Administrator aktiviert das Programm und legt Pläne in der Administration an. Diese Einstellung erlaubt dem Server nur, mit Stripe zu sprechen.

Ein Server kann nur einen Anbieter verwenden. Um später zu einem anderen Anbieter zu wechseln, bitte die bestehenden Abonnenten, mit dem neuen erneut zu abonnieren.

<!-- source: what-the-server-does-at-stripe ca3e0d4f59f3 -->
## Was der Server bei Stripe tut

[So funktioniert ein Abo](supporters.md#how-a-subscription-works) zeigt in einem Diagramm, wie die Zahlungsseite, der Webhook, der Abgleich, die Rolle des Plans und die Einnahmen zusammenspielen.

- **Pläne und Preise**: Für jeden Plan legt der Server über die API ein Product an (ID `my_plan_<plan ID>`, mit `metadata.my_plan_id`) und je ein Price pro Intervall und Währung (`lookup_key` ist `my_<plan ID>_<month|year>_<currency>`).
  Ein Stripe-Price kann seinen Betrag nicht ändern, daher legt eine Änderung des Betrags ein neues Price an und archiviert das alte. Abos auf dem alten Price verlängern sich zu dessen Betrag. Der Rabatt für Jahreszahlung wird im Betrag des Jahres-Prices ausgedrückt; Coupons werden nicht verwendet.
  Bearbeite oder lösche diese Products und Prices nicht im Dashboard.
  Ein Plan kann die Währungen aus [Stripes Liste der Mindestbeträge](https://docs.stripe.com/currencies#minimum-and-maximum-charge-amounts) nutzen, und sein Monats- und sein Jahresbetrag müssen jeweils dieses Minimum erreichen: USD, AED, ARS, AUD, BRL, CAD, CHF, COP, CZK, DKK, EUR, GBP, HKD, HUF, IDR, ILS, INR, JPY, KRW, MXN, MYR, NOK, NZD, PHP, PLN, RON, RUB, SEK, SGD, THB und ZAR. Der Server lehnt jede andere Währung ab.
  Die Liste gilt für eine Abbuchung in der Abrechnungswährung des Kontos. Eine Abbuchung, die Stripe umrechnet, muss nach der Umrechnung das Minimum der Abrechnungswährung erreichen, und der Server prüft das nicht; wähle den Betrag eines Plans also deutlich über dem Minimum, wenn seine Währung von der deines Bankkontos abweicht.
- **Abonnieren**: Der Server erstellt eine Checkout-Session mit `mode=subscription` und schickt den Browser des Nutzers zu ihrer URL (`checkout.stripe.com`). Die Zahlungsseite läuft nach einer Stunde ab.
  Die Abo-ID des Servers kommt in `client_reference_id`, `metadata.my_subscription_id` und in die `metadata.my_subscription_id` des Stripe-Abos. Die automatische Steuerberechnung (Stripe Tax) ist nicht aktiviert.
- **Erneut beginnen**: Höchstens eine Seite eines ausstehenden Abos kann eine Zahlung entgegennehmen. Ein Nutzer, der für denselben Plan und dasselbe Intervall erneut beginnt, kehrt zur offenen Seite zurück, die wieder leer öffnet, es sei denn, ihr bleiben weniger als 15 Minuten. Jeder andere Start erhält eine neue Seite, und der Server lässt die frühere ablaufen, die dann Stripes Ablaufmeldung zeigt. Wurde eine frühere Seite bereits bezahlt, erhält der Nutzer keine Seite: Er geht zum Rückkehrbildschirm, der das Abo zeigt. Kann der Server eine frühere Seite nicht ablaufen lassen, schlägt der Start fehl, und der Nutzer kann es erneut versuchen.
  Ein ausstehendes Abo, das endet (sein Plan eingestellt, sein Konto gelöscht), lässt seine Seite auf dieselbe Weise ablaufen.
- **Ein zweites Abo**: Zwei Zahlungen können sich trotzdem treffen, wenn beide Seiten im selben Moment abgeschickt werden. Das Abo, das den Server zuerst erreicht, ist das des Nutzers. Der Server kündigt das andere sofort und erstattet alle seine bezahlten Rechnungen vollständig, Verlängerungen eingeschlossen, da er für keinen Zeitraum etwas geliefert hat, mit dem Grund `duplicate`, und protokolliert `a second subscription was canceled and refunded`. Eine Zahlung für ein ausstehendes Abo, das geschlossen wurde, wird genauso behandelt, auch wenn sie den Server kurz vor dem Schließen erreichte. Der Nutzer erhält Stripes Erstattung und sieht ein Abo. Stripe erstattet die Gebühr für die erstattete Abbuchung nicht, und diese Gebühr erscheint in den Einnahmen.
  Eine Abbuchung in einem Dispute wird nicht erstattet, ebenso wenig eine Zahlung, die noch nicht abgewickelt war, als der Server nachsah (zum Beispiel bei einer Lastschrift). Ein Abo, das jemand bereits im Dashboard gekündigt hat, bleibt mit seinen Abbuchungen, wie es ist, und wird nie das Abo des Nutzers.
  **Bevor du einen Server von einer Version ohne dies aktualisierst**, suche in Stripe nach Kunden mit zwei Abos, deren `metadata.my_subscription_id` gleich ist. Der erste Abgleich kündigt das nicht genutzte und erstattet alles, was es abgebucht hat. Kündige im Dashboard alle, die du lieber von Hand regeln möchtest, und der Server lässt sie in Ruhe.
- **Kundenportal**: Bei der ersten Nutzung legt der Server über die API eine Portal-Konfiguration an (`metadata.my=1`), die nur das Aktualisieren von Zahlungsmethoden, den Rechnungsverlauf und die Kündigung zum Ende des Zeitraums aktiviert. Planwechsel erfolgen in den Bildschirmen dieses Servers, nicht im Portal.
  Eine Kündigung im Portal erscheint auf diesem Server als zur Kündigung vorgemerkt, und der Nutzer kann sie im Bildschirm des Servers rückgängig machen. Ein Planwechsel wird abgelehnt, bis sie rückgängig gemacht ist.
- **Ein von Hand gesetztes Kündigungsdatum**: Das Portal setzt `cancel_at` des Abos, nicht `cancel_at_period_end`, und der Server liest beide. Ein `cancel_at`, das du im Dashboard oder über die API setzt, zählt genauso, wenn es nicht nach dem Ende des laufenden Zeitraums liegt. Liegt es vor diesem Ende, zeigt der Server dieses Datum als Ende des Abos, und der Nutzer behält die Rolle bis dahin. Ein späteres `cancel_at` ist noch keine Kündigung: Das Abo bleibt aktiv bis zu dem Zeitraum, der das Datum enthält.
- **Planwechsel**: Ein Upgrade und ein Wechsel von monatlich zu jährlich berechnet die Differenz anteilig sofort (`proration_behavior=always_invoice`) und wechselt erst, wenn diese Rechnung bezahlt ist (`payment_behavior=pending_if_incomplete`). Ein Downgrade und ein Wechsel von jährlich zu monatlich wechseln zum Ende des Zeitraums über einen Subscription Schedule.
- **Ein fehlgeschlagener Planwechsel**: Schlägt die Zahlung für einen Wechsel fehl oder verlangt die Karte eine Authentifizierung, storniert der Server diese Rechnung (void), was den Wechsel verwirft, und der Nutzer sieht einen Fehler. Das Abo bleibt auf seinem Plan und aktiv, ohne Kulanzfrist und ohne Benachrichtigung über eine fehlgeschlagene Zahlung. Der Nutzer kann die Karte im Portal aktualisieren und es erneut versuchen und wird einmal belastet. Wird die Zahlung noch verarbeitet, lässt der Server sie, und der Plan wechselt, wenn sie gelingt. Verlängerungen sind nicht betroffen: Eine fehlgeschlagene Verlängerung geht wie unten beschrieben in die Kulanzfrist.
- **Den Zustand zurücklesen**: Der Server behandelt einen Webhook als Signal und liest das Abo, die Checkout-Session und die Geldbewegungen aus der Stripe-API zurück, um den Zustand zu bestimmen.
  Zusätzlich liest ein Abgleich alle `payments_reconcile_interval` (Standard 6 Stunden) die Ereignisliste, die unfertigen Checkout-Sessions, die Balance Transactions und einen Durchlauf aller Abos und repariert jeden Webhook, den der Server verpasst hat.
- **Einnahmen**: Der Server verzeichnet Brutto, Gebühr und Netto jeder Balance Transaction. Gebühren wie die 0,7 % von Billing erscheinen als eigene `stripe_fee`-Transaktionen und werden von den Einnahmen abgezogen.
  `stripe_fee` deckt die Gebühren des ganzen Kontos ab, **nutze also ein Stripe-Konto, das diesem Server vorbehalten ist**. Auf einem Konto, das für andere Verkäufe genutzt wird, werden diese Gebühren ebenfalls von den Einnahmen abgezogen.
- **Daten**: Einnahmedaten werden in der Zeitzone des Stripe-Kontos gezählt (die Zeitzone unter „Settings > Business > Account details“ im Dashboard, `settings.dashboard.timezone` in der API). Kann sie nicht gelesen werden, zählt der Server in UTC und liest sie jede Stunde erneut.

Jeder Serverprozess sendet höchstens 20 Anfragen pro Sekunde (10 im Testmodus). [Stripes Limits](https://docs.stripe.com/rate-limits) gelten pro Konto: 100 Anfragen pro Sekunde im Live-Modus, 25 in einer Sandbox und 25 pro Sekunde für jeden Endpunkt. Prozesse eines Servers teilen sie sich, ebenso alles andere, was das Konto nutzt.
Gibt Stripe `429` zurück, wartet der Server so lange, wie `Retry-After` sagt, wenn das 30 Sekunden oder weniger sind (1 Sekunde ohne den Header), und versucht es bis zu 2 Mal erneut; darüber hinaus überlässt er die Wiederholung dem Job.

Stripe teilt außerdem Leseanfragen pro Konto zu: durchschnittlich 500 pro Transaktion über 30 Tage und mindestens 10.000 im Monat, egal wie viele Transaktionen es gibt. Beim Standardintervall bleibt der Abgleich auf einem kleinen Server weit darunter. Ein Abgleich, der nichts findet, liest trotzdem einige Seiten, daher kommt das kürzeste Intervall von 15 Minuten (2.880 Läufe in 30 Tagen) auf einem Konto mit wenigen Transaktionen nahe an die 10.000 heran.
Stripe begrenzt jedes Abo auf 10 neue Rechnungen pro Minute und 20 pro Tag, und ein sofort abgerechneter Planwechsel erzeugt eine neue Rechnung.

<!-- source: card-data-and-pci-dss f45a3c7d1846 -->
## Kartendaten und PCI DSS

Zahlungsmethoden werden nur in Stripes Checkout und im Kundenportal eingegeben, und der Webclient dieses Servers lädt weder Stripes Skript noch einen Iframe.
Der Server speichert nur Stripe-Objekt-IDs (`cus_`, `sub_`, `price_`, `prod_`, `cs_`, `txn_`), Beträge, Zeiträume und Zustände. Er speichert keine Kartennummern, letzten vier Ziffern, Ablaufdaten oder Rechnungsadressen. Webhook-Bodys und Stripe-Antworten werden weder in den Logs noch in der Datenbank aufbewahrt.
Deshalb wird erwartet, dass der PCI-DSS-Geltungsbereich des Betreibers der eines Händlers ist, der eine gehostete Zahlungsseite nutzt (SAQ A). Prüfe den für dich geltenden Geltungsbereich und die Art des Selbstbeurteilungsfragebogens in den Hinweisen in Stripes Dashboard („Settings > Compliance“).

<!-- source: 1-prepare-the-stripe-account 06eefed5779b -->
## 1. Das Stripe-Konto vorbereiten

1. Erstelle ein Stripe-Konto und trage deine Geschäftsdaten im Dashboard ein. Wenn du in Japan verkaufst, registriere die URL deiner Seite mit den Angaben nach dem Gesetz über bestimmte Handelsgeschäfte unter „Settings > Business > Public information > Notice based on the Act on Specified Commercial Transactions“, und unter Checkout erscheint ein Link.
2. Prüfe die Zeitzone unter „Settings > Business > Account details“. Einnahmedaten werden in ihr gezählt.
3. Wähle unter „Settings > Billing > Subscriptions and emails > Manage failed payments“ das Wiederholungsverhalten (Smart Retries) und was geschieht, wenn die Wiederholungen aufgebraucht sind.
   Die Kulanzfrist, bevor die Rolle entzogen wird (7 Tage ab dem ersten Fehlschlag), zählt dieser Server, Stripes Wiederholungseinstellungen beeinflussen sie also nicht. Wenn die Wiederholungen aufgebraucht sind, wird „Cancel the subscription“ empfohlen.
4. Um Belege an Abonnenten zu senden, aktiviere den Beleg für erfolgreiche Zahlungen unter „Settings > Customer emails“.

<!-- source: 2-create-an-api-key 95e2c858acca -->
## 2. Einen API-Schlüssel erstellen

Unter „Developers > API keys“ wird empfohlen, einen **eingeschränkten Schlüssel** (restricted key, `rk_test_...` / `rk_live_...`) zu erstellen. Ein geheimer Schlüssel (`sk_test_...` / `sk_live_...`) funktioniert auch.
Das Präfix des Schlüssels legt den Modus fest, und ein Testschlüssel belastet nichts. Ein veröffentlichbarer Schlüssel (`pk_`) kann nicht verwendet werden.

Gib einem eingeschränkten Schlüssel diese Berechtigungen. Ein eingeschränkter Schlüssel mit den Berechtigungen dieser Tabelle hat in einer Sandbox Abonnieren, Planwechsel, Kündigung, Abgleich und das Stornieren von Rechnungen ausgeführt. Erstattungen, das Ablaufenlassen einer Seite und Disputes wurden damit nicht ausgeführt, und niemand hat geprüft, ob die Tabelle die kleinste funktionierende Menge ist.

| Ressource | Berechtigung | Verwendete APIs |
| --- | --- | --- |
| Products | Write | `GET`, `POST /v1/products`, `POST /v1/products/{id}` |
| Prices | Write | `GET`, `POST /v1/prices`, `POST /v1/prices/{id}` |
| Checkout Sessions | Write | `POST /v1/checkout/sessions`, `GET /v1/checkout/sessions/{id}`, `POST /v1/checkout/sessions/{id}/expire` |
| Customer portal | Write | `GET`, `POST /v1/billing_portal/configurations`, `POST /v1/billing_portal/sessions` |
| Subscriptions | Write | `GET /v1/subscriptions`, `POST`, `DELETE /v1/subscriptions/{id}` |
| Subscription Schedules (Teil von Subscriptions) | Write | `POST /v1/subscription_schedules`, `POST /v1/subscription_schedules/{id}`, `POST /v1/subscription_schedules/{id}/release` |
| Invoices | Write | `GET /v1/invoices/{id}`, `GET /v1/invoice_payments`, `POST /v1/invoices/{id}/void` |
| Charges, Refunds | Write | `GET /v1/charges/{id}`, `GET /v1/refunds`, `POST /v1/refunds` |
| Disputes | Read | Lesen über die Balance Transactions und Charges |
| Balance transaction sources | Read | `GET /v1/balance_transactions` |
| Events | Read | `GET /v1/events` |
| Account (Informationen eines Kontos außerhalb von Connect) | Read | `GET /v1/account` (Zeitzone) |

Ohne die Berechtigung, Erstattungen zu schreiben, kündigt der Server ein zweites Abo trotzdem, und `duplicate_charge` bleibt in den Warnungen (siehe [Den Webhook-Endpunkt registrieren](#3-register-the-webhook-endpoint)), bis er erstatten kann. Ein Schlüssel, der erstellt wurde, bevor diese Berechtigung aufgeführt war, braucht sie zusätzlich.

Übergib den Schlüssel in der Umgebungsvariable `MY_PAYMENTS_STRIPE_SECRET_KEY` (oder den Pfad einer Datei in `MY_PAYMENTS_STRIPE_SECRET_KEY_FILE`). Er darf nicht in der Konfigurationsdatei stehen.
Wenn der Schlüssel durchsickert, rotiere ihn im Dashboard und stelle mit dem neuen Schlüssel erneut bereit.

<!-- source: 3-register-the-webhook-endpoint 1cbc2cf9ace3 -->
## 3. Den Webhook-Endpunkt registrieren

Registriere unter „Developers > Webhooks > Add endpoint“ Folgendes. Der Server legt den Endpunkt nicht über die API an.

- URL: `https://<server domain>/payments/webhooks/stripe`
- Events from: „Your account“ (nicht Connected accounts)
- Payload style: snapshot. Der Server liest nur `id`, `type`, `livemode`, `created`, `data.object.id` und `data.object.object` des Bodys, die API-Version des Endpunkts spielt also keine Rolle.
- Zu sendende Ereignisse (16 Typen):

  - `checkout.session.completed`, `checkout.session.expired`
  - `customer.subscription.created`, `customer.subscription.updated`, `customer.subscription.deleted`, `customer.subscription.paused`, `customer.subscription.resumed`
  - `invoice.paid`, `invoice.payment_failed`
  - `charge.refunded`, `refund.created`, `refund.updated`
  - `charge.dispute.created`, `charge.dispute.closed`, `charge.dispute.funds_withdrawn`, `charge.dispute.funds_reinstated`

Übergib das Signing Secret des Endpunkts (`whsec_...`) in der Umgebungsvariable `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` (oder `_FILE`).

Der Endpunkt prüft die Signatur (HMAC-SHA256 in `Stripe-Signature`) und den Zeitstempel (in beide Richtungen innerhalb von 5 Minuten) und lehnt ein Testereignis, das mit einem Live-Schlüssel empfangen wird (und umgekehrt), mit `400` ab.
Er verzeichnet jedes Ereignis einmal anhand der ID, stellt einen Job in die Warteschlange und gibt `2xx` zurück. Kann er nicht in die Datenbank schreiben, gibt er `503` zurück, und Stripe sendet das Ereignis erneut. Der Body ist auf 1 MiB begrenzt.
Während du das Signing Secret rotierst, signiert Stripe mit dem alten und dem neuen, es genügt also, das neue Secret in die Konfiguration zu setzen.

Der Zustand des Anbieters (der Modus, wann der letzte Webhook ankam, sowie das Ergebnis und die Fehlerart des letzten Abgleichs) wird im Abschnitt „Zahlungsanbieter“ von `/admin/supporters` in der Administration und von der Admin-API `GET /api/v1/admin/payment_status` (Berechtigung `manage_supporters`) angezeigt. Der Abschnitt erklärt, wie jede Art von Abgleichsfehler zu beheben ist.
`warnings` listet `no_webhook` (es wurde noch nie ein Webhook verzeichnet) und `reconciliation_corrected` (der letzte Abgleich hat ein Abo repariert). Vermute einen falsch konfigurierten Webhook, wenn eines davon erscheint.
Es listet auch `duplicate_charge`: Ein zweites Abo ist noch nicht gekündigt und erstattet. Siehe [Betrieb](#7-operation).
„Jetzt abgleichen“ im Abschnitt (`POST /api/v1/admin/payment_reconciliation/runs`) gleicht sofort ab, statt auf den geplanten Lauf zu warten. Es ist auf einmal pro UTC-Stunde begrenzt (ein zweiter Aufruf in derselben Stunde gibt `429` zurück), und das Audit-Log verzeichnet `payment_reconcile_queued`.

<!-- source: 4-configure-the-server 7b3f7d01839f -->
## 4. Den Server konfigurieren

```sh
MY_PAYMENTS=stripe
MY_PAYMENTS_STRIPE_SECRET_KEY=rk_test_...
MY_PAYMENTS_STRIPE_WEBHOOK_SECRET=whsec_...
# Optional: the reconciliation interval (15m to 168h, default 6h) and the most requests one reconciliation sends (default 1000)
MY_PAYMENTS_RECONCILE_INTERVAL=6h
MY_PAYMENTS_RECONCILE_MAX_REQUESTS=1000
```

`my config check` prüft die Konfiguration. Eine Binärdatei, die mit dem Build-Tag `no_payments_stripe` gebaut wurde, lehnt `payments = stripe` ab (siehe [build-tags.md](../reference/build-tags.md)).

<!-- source: 5-try-it-in-test-mode 94db104964c3 -->
## 5. Im Testmodus ausprobieren

Beginne mit einem Testschlüssel (`rk_test_` / `sk_test_` oder einem Sandbox-Schlüssel). Der Testmodus belastet nichts.
Testschlüssel und Live-Schlüssel haben getrennte Products, Prices, Kunden, Abos und Webhook-Endpunkte. Um live zu gehen, registriere den Endpunkt im Live-Modus erneut, ersetze den Schlüssel und das Signing Secret durch die Live-Werte und stelle erneut bereit. Die Prices der Pläne werden im Live-Konto neu angelegt.

Testkarten ([Stripe-Dokumentation](https://docs.stripe.com/testing)):

| Nummer | Verhalten |
| --- | --- |
| `4242 4242 4242 4242` | Gelingt |
| `4000 0000 0000 0341` | Lässt sich hinterlegen, aber die Abbuchung schlägt fehl (um Zahlungsfehlschlag und den Beginn der Kulanzfrist zu prüfen) |
| `4000 0000 0000 9995` | Schlägt wegen unzureichender Deckung fehl |
| `4000 0000 0000 0259` | Nach der Zahlung wird ein Dispute erstellt |

Verwende ein beliebiges Ablaufdatum in der Zukunft und einen beliebigen dreistelligen Sicherheitscode.

Um Verlängerung und Zahlungsfehlschlag auszuprobieren, nutze eine [Test Clock](https://docs.stripe.com/billing/testing/test-clocks/api-advanced-usage). Ein von Checkout angelegter Kunde hat bei der Erstellung keine Clock, abonniere also zuerst und lege dann über die API eine Clock für diesen Kunden an, indem du `customer` an `POST /v1/test_helpers/test_clocks` übergibst (API-Version `2026-05-27.dahlia` oder neuer).
Stripe erlaubt das nur, wenn `frozen_time` nicht in der Vergangenheit liegt, das Konto keine Automations konfiguriert hat und der Kunde höchstens drei Abos hat. Der Kunde kann die Clock danach nicht mehr verlassen, und das Löschen der Clock löscht den Kunden und seine Abos. Stelle die Zeit unter „Billing > Test clocks“ im Dashboard vor, um die Verlängerungsabbuchung und den Fehlschlag auszulösen.

Eine Clock kann nicht alles zeigen:

- **Das Ende der Kulanzfrist**: Der Server vergleicht die reale Zeit mit der Zeit, die Stripe für den Fehlschlag meldet (`finalized_at`), die simuliert ist und der realen Zeit voraus. Das Abo tritt in die Kulanzfrist ein, und weiteres Vorstellen der Clock beendet sie nicht. Die Rolle bleibt, bis die reale Zeit 7 Tage nach dem simulierten Fehlschlag überschreitet oder bis Stripe das Abo kündigt, wenn seine Wiederholungen aufgebraucht sind, sofern das Konto dafür eingestellt ist.
- **Der Durchlauf aller Abos**: Stripes Abo-Liste lässt Abos auf einer Clock aus, es sei denn, die Anfrage nennt die Clock oder den Kunden, und der Durchlauf des Servers nennt keines von beiden. Nur Webhooks und die Ereignisliste bringen ihre Änderungen zum Server.

<!-- source: 6-try-it-locally-stripe-cli 8a3e87b02ce0 -->
## 6. Lokal ausprobieren (Stripe CLI)

Die [Stripe CLI](https://docs.stripe.com/stripe-cli) (Apache-2.0) leitet Stripe-Ereignisse an einen Server auf deinem Rechner weiter. Es ist keine öffentliche URL nötig.

```sh
stripe login
stripe listen --forward-to http://localhost:8080/payments/webhooks/stripe \
  --events checkout.session.completed,checkout.session.expired,customer.subscription.created,customer.subscription.updated,customer.subscription.deleted,customer.subscription.paused,customer.subscription.resumed,invoice.paid,invoice.payment_failed,charge.refunded,refund.created,refund.updated,charge.dispute.created,charge.dispute.closed,charge.dispute.funds_withdrawn,charge.dispute.funds_reinstated
```

Setze das `whsec_...`, das `stripe listen` ausgibt, in `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` und starte den Server (passe den Port an deine `listen`-Einstellung an).

Mit Docker Compose setze `MY_PAYMENTS=stripe`, `MY_PAYMENTS_STRIPE_SECRET_KEY` und `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` in `deploy/docker/.env` und führe dann `docker compose up -d` aus.

In einem anderen Terminal kannst du einzelne Ereignisse auslösen.

```sh
stripe trigger checkout.session.completed
stripe trigger invoice.payment_failed
stripe trigger charge.refunded
stripe trigger charge.dispute.created
stripe events resend evt_...   # send an event you missed again (up to 30 days)
```

Die Objekte, die `stripe trigger` anlegt, tragen keine Abo-ID dieses Servers, daher ändert kein Abo seinen Zustand (der Server liest sie zurück und ignoriert, was nicht sein eigenes Abo ist). Um den gesamten Abo-Ablauf zu prüfen, abonniere im Webclient und bezahle mit einer Testkarte.

<!-- source: 7-operation f70cd2967b9f -->
## 7. Betrieb

- Ein Abgleichsfehler der Art `permission` bedeutet einen falschen Schlüssel oder fehlende Berechtigungen. Die Logzeile des Servers (`payment reconciliation failed`) enthält Stripes Meldung, die die hinzuzufügende Berechtigung nennt.
- Wenn Abgleiche oft ans Anfragelimit stoßen, erhöhe `payments_reconcile_max_requests` oder verlängere `payments_reconcile_interval`. Jedes Lesen zählt auf Stripes Lesekontingent (siehe [Was der Server bei Stripe tut](#what-the-server-does-at-stripe)), bevorzuge daher auf einem Konto mit wenigen Transaktionen das längere Intervall.
- Wenn der Abgleich länger als 30 Tage angehalten war, kann der Server nicht aus Stripes Ereignisliste aufholen (die 30 Tage zurückreicht). Der Durchlauf aller Abos repariert den Zustand dann.
- `duplicate_charge` in den Warnungen bedeutet, dass ein Nutzer zweimal bezahlt hat und der Server das Kündigen und Erstatten des zweiten Abos nicht beenden konnte. Die Logzeile `a second subscription could not be canceled and refunded` nennt beide Abos und die Fehlerart und erscheint bei jedem Abgleich erneut, der es noch einmal versucht. Bei der Art `permission` füge die Berechtigung zum Schreiben von Erstattungen hinzu. Andernfalls erstatte die Abbuchung im Dashboard: Der nächste Versuch findet nichts mehr zu erstatten und löscht die Warnung. „Jetzt abgleichen“ versucht es sofort.
- Andere Erstattungen und Rückbuchungen (Chargebacks) werden in Stripes Dashboard bearbeitet. Eine vollständige Erstattung und eine Rückbuchung entziehen die Rolle und kündigen das Abo sofort, eine Teilerstattung entzieht die Rolle nicht. Einen Dispute zu gewinnen stellt die Rolle nicht automatisch wieder her.

<!-- source: items-to-verify 8960333fa514 -->
## Zu prüfende Punkte

Diese wurden gegen Stripe weder in einer Sandbox noch live ausprobiert und beruhen auf Stripes Dokumentation.

- Eine Verlängerung eines Abos auf einem archivierten Price, das auf diesem Price bleiben sollte.
- `status_transitions.finalized_at` bei einer echten Verlängerung. Der Server nimmt es als den Zeitpunkt, zu dem die Zahlung zum ersten Mal fehlschlug, und zählt die Kulanzfrist ab dort.
- Ob eine `stripe_fee`-Transaktion einem Abo zugeordnet werden kann. Der Server versucht es nicht: Er zieht diese Gebühren als Ganzes von den Einnahmen ab.
- Ob `settings.dashboard.timezone` von `GET /v1/account` mit einem eingeschränkten Schlüssel gelesen wird. Ein fehlgeschlagenes Lesen hält den Server nicht an, der in UTC zählt, ein funktionierender Server zeigt es also nicht. Prüfe, dass Einnahmedaten in die Zeitzone des Kontos fallen.
- Dass `GET /v1/invoice_payments` nach `payment[payment_intent]` filtert, was eine Zahlung mit ihrem Abo verknüpft.
- Vollständige und teilweise Erstattungen und Rückbuchungen, vom Dashboard bis zum Zustand auf dem Server.
- Dass der Durchlauf aller Abos ein Abo repariert, dessen Zustand der Server verpasst hat.
- Was ein Upgrade für ein Abo tut, das mit einer Methode zahlt, die Pending Updates nicht aufführen (ACH-, SEPA- und Bacs-Lastschriften). Lass diese Methoden im Dashboard ausgeschaltet, bis dies bestätigt ist.
- Dass `cancel_at_period_end=true` ein `cancel_at` in einem späteren Zeitraum ersetzt.
- Die Rückkehr zu einer offenen Seite. Der Server vergleicht, was `GET /v1/checkout/sessions/{id}` zurückgibt (`line_items`, `success_url`, `cancel_url`, `customer`, `customer_email`), mit der neuen Anfrage, und jeder Unterschied ergibt eine neue Seite. Schreibt Stripe `success_url` oder `customer_email` beim Speichern um, erhält der Nutzer jedes Mal eine neue Seite und verliert, was er eingetippt hat, zahlt aber einmal.
- Das Ablaufenlassen einer Session, während ihre Zahlung abgeschlossen wird. Stripes Dokumentation sagt, dass eine Session, die nicht offen ist, einen Fehler zurückgibt, und der Server liest dann die Session. Der genaue Code des Fehlers und der Status, während eine Kartenauthentifizierung läuft, sind nicht dokumentiert.
- Dass eine Session nicht aus zwei Tabs zweimal bezahlt werden kann. Eine Session enthält ein Abo und wird einmal abgeschlossen, aber die Dokumentation sagt es für zwei Abschickungen im selben Moment nicht.
- Ob eine fehlgeschlagene Authentifizierung auf der Zahlungsseite ein `incomplete`-Abo hinterlässt, das das Abo des Servers nennt. Der Server würde es verknüpfen und den Nutzer ausstehend halten, und eine Zahlung auf einer anderen Seite würde als zweite erstattet.
- Eine Erstattung eines zweiten Abos, mit einem eingeschränkten Schlüssel, dessen Berechtigungen „Charges“ und „Refunds“ wie in der Tabelle sind, und die darauf folgenden Geldbewegungen.
