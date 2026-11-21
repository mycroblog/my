<!-- source: sending-email-through-amazon-ses cb9101de6cc9 -->
# E-Mails über Amazon SES versenden

So machst du Amazon SES bereit, die E-Mails des Servers zu versenden: die Domain des Absenders mit DNS-Einträgen verifizieren, die Sandbox verlassen, in der ein neues Konto startet, und das Konto in gutem Stand halten. Das OpenTofu-Modul [`mycroblog-aws`](../../deploy/opentofu/README.md#aws) versendet standardmäßig über SES und legt die Identität und die Berechtigung an; die Einträge und die Beantragung des Produktionszugriffs sind deine eigenen Schritte.
Die Einstellungen selbst stehen unter [E-Mails versenden](email.md#amazon-ses-api).

**Das AWS-Deployment wurde noch nie angewendet**, deshalb stammen diese Schritte aus der Dokumentation von AWS und wurden auf einem Server dieses Projekts noch nicht von Anfang bis Ende nachvollzogen.

<!-- source: 1-add-the-records-of-the-domain b96b2365fdd4 -->
## 1. Die Einträge der Domain hinzufügen

SES versendet von einer verifizierten Identität aus. `mycroblog-aws` legt die Identität der Domain von `mail.from` an (für
`noreply@social.example` die Domain `social.example`), mit Easy DKIM, in der Region des Deployments. Nach dem Apply listet die
Ausgabe `dns_records` die Einträge auf, die du bei deinem DNS-Anbieter hinzufügen musst, mit dem Eintrag von `host`:

```sh
tofu output dns_records
```

```text
abcdef...._domainkey.social.example.  CNAME  abcdef....dkim.amazonses.com
ghijkl...._domainkey.social.example.  CNAME  ghijkl....dkim.amazonses.com
mnopqr...._domainkey.social.example.  CNAME  mnopqr....dkim.amazonses.com
```

Die drei CNAME-Einträge veröffentlichen die Schlüssel, mit denen SES signiert. SES prüft sie selbstständig und markiert die Identität als verifiziert, sobald sie
aufgelöst werden, in der Regel innerhalb einer Stunde und spätestens innerhalb von 72 Stunden; bis dahin verweigert es den Versand. Die SES-Konsole zeigt den Status
der Identität unter **Identities**.

Füge außerdem SPF- und DMARC-Einträge hinzu, die Empfänger zusätzlich zu DKIM prüfen: [SPF, DKIM und DMARC](email.md#spf-dkim-and-dmarc) sagt,
was in jeden Eintrag gehört. Das von SES signierte DKIM stimmt mit der Domain von `mail.from` überein (Alignment), sodass DMARC allein mit diesen Einträgen besteht; eine
eigene MAIL-FROM-Domain, die auch SPF ausrichtet, ist optional.

<!-- source: 2-leave-the-sandbox 7f27054422ae -->
## 2. Die Sandbox verlassen

Jedes neue Konto startet in der SES-Sandbox, in jeder Region getrennt. In der Sandbox versendet SES nur an Adressen und
Domains, die du selbst verifiziert hast, und höchstens 200 Nachrichten in 24 Stunden und eine pro Sekunde. Ein Server in der Sandbox kann seinem
eigenen Betreiber eine Testnachricht senden, aber die Registrierung aller anderen schlägt fehl.

Um an alle zu senden, beantrage den Produktionszugriff für die Region des Deployments:

1. Öffne die SES-Konsole in dieser Region und wähle im **Account dashboard** **Request production access** (oder **Get set up**).
2. Wähle **Transactional** als Nachrichtentyp, gib die URL des Servers an und beschreibe, was er versendet: Bestätigungen der
   Registrierung und einer neuen Adresse, Passwort-Zurücksetzungen und die Benachrichtigungen, die jede Person selbst einschaltet und mit einer
   Abmeldung per Klick wieder ausschalten kann.
3. Gib an, wie du mit Bounces und Beschwerden umgehst: Der Server versendet nichts mehr an eine Adresse, die einen Bounce oder eine Beschwerde auslöst, sobald die
   unten beschriebenen Benachrichtigungen ihn erreichen.

AWS antwortet per E-Mail, in der Regel innerhalb eines Tages, und fragt womöglich nach weiteren Angaben, bevor es den Zugriff gewährt. Sobald das geschehen ist, zeigt die Konsole
dein Versandkontingent, das mit deiner Nutzung wächst.

<!-- source: 3-watch-bounces-and-complaints b1182f32de88 -->
## 3. Bounces und Beschwerden im Blick behalten

SES beobachtet den Anteil der Nachrichten, die zurückgewiesen werden (Bounce) oder Beschwerden auslösen, und stellt ein Konto, dessen Raten
hoch bleiben, unter Prüfung und pausiert es dann. Der Server erfährt von einem Bounce oder einer Beschwerde aus den Benachrichtigungen von SES über Amazon SNS und versendet nichts mehr an diese
Adresse. `mycroblog-aws` legt das Topic nicht an: Richte es ein, wie [SES-Benachrichtigungen empfangen (SNS)](email.md#receive-ses-notifications-sns)
es beschreibt, und gib dem Server seine ARN über die `settings` des Moduls:

```hcl
  settings = { mail_ses_sns_topic_arn = "arn:aws:sns:ap-northeast-1:111122223333:my-ses-events" }
```

Die SES-Konsole zeigt die Bounce- und Beschwerderaten des Kontos unter **Reputation metrics**.

<!-- source: sending-from-another-region 61edc3b77ea9 -->
## Aus einer anderen Region versenden

Das Modul legt die Identität in der Region des Deployments an, und der Server versendet dort. Um über SES in einer anderen
Region oder stattdessen über einen SMTP-Dienst zu versenden, setze `mail = { kind = "smtp", ... }` mit der SMTP-Schnittstelle dieses Dienstes:
[Amazon SES (SMTP-Schnittstelle)](email.md#amazon-ses-smtp-interface) enthält die Einstellungen.
