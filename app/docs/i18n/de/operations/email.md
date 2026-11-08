<!-- source: sending-email 941ecea58cd9 -->
# E-Mails versenden

Welche E-Mails der Server sendet, wie du den Versand konfigurierst und wie du mit Fehlschlägen und Abmeldungen umgehst.
Die Einstellungen sind in der [Konfigurationsreferenz](../reference/configuration.md#mail) aufgelistet.
Die Abläufe für Nutzer bei E-Mail-Bestätigung und Zurücksetzen des Passworts stehen unter [Registrierung und E-Mail-Bestätigung](registration.md).

<!-- source: what-is-sent-and-how-to-configure-it 3f387cd1afcc -->
## Was gesendet wird und wie du es konfigurierst

Der Server sendet nie E-Mails innerhalb einer Anfrage. Er legt für jeden Zweck einen Job in derselben Transaktion an wie die Tatsache, die
die E-Mail auslöst, und ein Job-Worker sendet sie. Ein Fehlschlag beim Senden zeigt sich nicht in der Antwort auf eine Registrierung oder ein Zurücksetzen:
Dem Nutzer wird „we sent a confirmation email“ gesagt, und er fordert sie erneut an, wenn sie nicht ankommt.

| Klasse | E-Mail | Job-Typ | Kann der Nutzer sich abmelden? |
| --- | --- | --- | --- |
| Transaktional (`transactional`) | Bestätigung der E-Mail-Adresse | `identity.verification_email` | Nein |
| Transaktional | Zurücksetzen des Passworts | `identity.password_reset_email` | Nein |
| Transaktional | Hinweis auf eine Passwortänderung oder Löschung | `identity.password_notice_email` | Nein |
| Transaktional | Hinweis auf die Deaktivierung des Kontos, mit dem Datum seiner Löschung | `identity.deactivation_email` | Nein |
| Transaktional | Einstellung des Dienstes (die Ankündigung, eine Datumsänderung, eine Absage, 7 Tage und 1 Tag vor dem Schließungsdatum, die Schließung und 7 Tage vor der Export-Frist) | `identity.closure_email` | Nein |
| Benachrichtigung (`notification`) | Hinweis an Moderatoren (standardmäßig an, pro Art des Hinweises) | `notification.moderator_email` | Ja |
| Benachrichtigung (`notification`) | Eine Benachrichtigung, die 15 Minuten ungelesen blieb: Erwähnungen, Follows, Boosts und die anderen Benachrichtigungen von anderen Nutzern sowie Hinweise der Moderatoren (standardmäßig aus, pro Art) | `notification.email` | Ja |

Benachrichtigungs-E-Mails für Unterstützer (`notification.supporter_email`) laufen über dieselbe Job-Queue.

<!-- source: which-notifications-are-emailed 76c2b871762c -->
### Welche Benachrichtigungen per E-Mail gesendet werden

Jeder Nutzer wählt pro Art der Benachrichtigung, ob sie in der App und per E-Mail ankommt, auf der Seite **Benachrichtigungen** in
den Einstellungen (`/settings/notifications` im Web-Client). Die Spalte „E-Mail“ erscheint dort nur, solange der Server
E-Mails sendet. Eine in der App ausgeschaltete Art wird gar nicht erst erzeugt, also weder per E-Mail noch per Push gesendet.

| Art | E-Mail standardmäßig | Wann sie gesendet wird |
| --- | --- | --- |
| Benachrichtigungen von anderen Nutzern (Erwähnungen, neue Follower, Follow-Anfragen, angenommene Follow-Anfragen, Boosts, Favoriten, Emoji-Reaktionen, Zitate, beendete Umfragen, Umzüge von Konten, denen du folgst, Sprach-Chats) | Aus | 15 Minuten nach der Benachrichtigung, nur wenn sie noch ungelesen ist (nicht als gelesen markiert, nicht verworfen) und noch sichtbar ist und der Nutzer diese Art weiterhin per E-Mail erhält. Eine E-Mail pro Benachrichtigung |
| Hinweise der Moderatoren (eine Moderationsaktion zum Konto, eine gelöste Meldung, ein entschiedener Einspruch, durch eine Sperre verlorene Follows, eine entschiedene Anfrage für ein eigenes Emoji) | Aus | Wie oben |
| Hinweise an Moderatoren (neue Meldungen, Registrierungen, die auf Genehmigung warten, Einsprüche, deaktivierte Webhooks, Anfragen für eigene Emojis, Umzüge eingeschränkter Konten, das verstrichene Schließungsdatum, neue und wiederkehrende Fehler), an die Personen, deren Rolle die Berechtigung hat | An | Sofort, ohne darauf zu warten, dass sie gelesen wird |
| Einstellung des Dienstes und die Hinweise der Unterstützungspläne | Immer | Sofort; diese lassen sich nicht ausschalten |

Benachrichtigungs-E-Mails gehen nur an die primäre Adresse des Nutzers, sobald sie bestätigt ist, und nie an eine Adresse, deren E-Mails
gestoppt sind (siehe unten). Der Betreff der E-Mail zu einer Benachrichtigung eines anderen Nutzers ist derselbe Text wie seine Web-Push-Nachricht,
und die E-Mail verlinkt auf den Beitrag, das Profil oder die Benachrichtigungen.

E-Mails zur Einstellung des Dienstes gehen an jeden Nutzer mit bestätigter primärer Adresse. Eine zu verpassen kann Datenverlust bedeuten, daher werden sie
als transaktionale E-Mail gesendet, tragen keinen Abmeldelink und werden nicht durch eine Beschwerde gestoppt (ein Hard Bounce stoppt sie dagegen).
Die Ankündigung, eine Datumsänderung, eine Absage und die Schließung legen jeweils `identity.closure_email_fanout` in derselben
Transaktion wie ihren Datensatz an. Das stündliche `identity.queue_closure_reminders` entscheidet, wann die Erinnerungen für 7 Tage und
1 Tag vor dem Schließungsdatum und 7 Tage vor der Export-Frist gesendet werden. Eine Erinnerung, deren Zeitpunkt schon verstrichen war, als die Einstellung
angekündigt wurde, wird nicht gesendet, weil die Ankündigungs-E-Mail die Daten nennt.
Der Fan-out-Job zeichnet pro Nutzer eine Zeile in `service_closure_emails` auf und legt `identity.closure_email` nur für die Nutzer an, die er
aufzeichnen konnte, sodass jeder Nutzer jede E-Mail einmal erhält, egal wie oft der Job läuft. Vor dem Senden prüft der Job, dass die E-Mail
noch gültig ist (dass sie nicht zu einer abgesagten Ankündigung oder zu einem inzwischen geänderten Datum gehört).

Die Einstellung `mail` wählt, wie E-Mails gesendet werden.

| `mail` | Verhalten |
| --- | --- |
| `none` (Standard) | Sendet nichts und schreibt `mail not sent (mail = none)` mit der Kategorie der Nachricht ins Server-Log. Mit `profile = development` enthält die Zeile auch die Adressen, den Betreff und den Text samt Links. Für Setups, die keine E-Mails nutzen, und für die Entwicklung |
| `smtp` | Sendet per SMTP-Submission. Es verlangt STARTTLS und prüft das Zertifikat, bevor es sich authentifiziert und sendet |
| `ses` | Sendet mit der Amazon-SES-v2-API (`SendEmail` mit Rohinhalt). Es authentifiziert sich über die Standard-Credential-Kette des AWS SDK (zum Beispiel eine IAM-Rolle) und hält kein SMTP-Passwort |
| `plugin` | Übergibt jede Nachricht an ein [Plugin](#a-plugin-mail--plugin), das du betreibst, für einen Maildienst, der nur eine HTTP-API anbietet |

Die Einstellungen von `smtp` sind `mail_from`, `mail_smtp_host`, `mail_smtp_port` (Standard 587), `mail_smtp_username` und `mail_smtp_password`. Die
Einstellungen von `ses` sind `mail_from`, `mail_ses_region` und `mail_ses_configuration_set`. Die Einstellungen von `plugin` sind `mail_from`
und `plugin_mailer_address` mit dem Rest von [seinem Ziel](plugins.md#the-destination).
`mail_from` ist eine reine Adresse wie `noreply@social.example`. `From` wird zu `"<server name>" <mail_from>`, und die Domain
von `mail_from` ist auch die Domain der `Message-ID` und diejenige, auf die SPF, DKIM und DMARC angewendet werden (siehe unten).
`Reply-To` ist die Servereinstellung `contact_email`, wenn sie gesetzt ist ([Servereinstellungen](instance-settings.md)).
Jede E-Mail trägt `Auto-Submitted: auto-generated`.
`smtp` und `ses` bauen dieselbe Nachricht. SMTPUTF8 wird nicht verwendet, daher werden Adressen in ASCII gesendet und internationalisierte Domains
in Punycode umgewandelt. An eine Adresse, deren lokaler Teil (vor dem `@`) nicht ASCII ist, kann nicht gesendet werden, und ihr Job schlägt fehl.

<!-- source: examples-by-provider 7ab129a02c43 -->
## Beispiele nach Anbieter

Übergib Secrets per Umgebungsvariable oder Datei (`MY_MAIL_SMTP_PASSWORD` oder `MY_MAIL_SMTP_PASSWORD_FILE`). Schreibe sie nicht in die
Konfigurationsdatei.

<!-- source: amazon-ses-api 1cef4371e61c -->
### Amazon SES (API)

Wenn du auf AWS läufst, nutzt `mail = ses` die SES-v2-API. Die Zugangsdaten kommen aus der Standardkette des AWS SDK (die Task-Rolle von
ECS, Umgebungsvariablen, die gemeinsame Konfigurationsdatei), daher gibt es kein Secret zu konfigurieren.

1. Verifiziere in SES die Domain von `mail_from` als Identität und aktiviere Easy DKIM (siehe unten).
2. Beantrage den Produktionszugriff, um die Sandbox zu verlassen. In der Sandbox kannst du nur an verifizierte Adressen senden.
3. Erlaube `ses:SendEmail` für die Ausführungsrolle auf der Identität von `mail_from` (und auf dem Configuration Set, falls du eines nutzt).

```sh
MY_MAIL=ses
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SES_REGION=ap-northeast-1          # the region where you verified the identity; AWS_REGION when empty
MY_MAIL_SES_CONFIGURATION_SET=my-events    # optional; a configuration set with event destinations
```

Wenn `mail_ses_configuration_set` gesetzt ist, nutzt jede E-Mail dieses Configuration Set. SES überschreibt `Message-ID` und `Date` der
E-Mail mit eigenen Werten.
SES lehnt einen Empfänger beim Senden nie synchron ab. Es akzeptiert einen Versand an eine Adresse auf der Suppression-Liste des Kontos und
liefert ihn dann nicht aus, und der Bounce kommt später als Benachrichtigung (SNS) an.
Um Bounces und Beschwerden aufzuzeichnen, richte die SES-Benachrichtigungen unten so ein, dass sie über SNS ankommen.

<!-- source: a-plugin-mail--plugin 65b46feaf1cf -->
### Ein Plugin (`mail = plugin`)

Betreibe für einen Maildienst, der nur eine HTTP-API anbietet, ein [Plugin](plugins.md) der Art `mailer` und lass es den Dienst aufrufen.

```sh
MY_MAIL=plugin
MY_MAIL_FROM=noreply@social.example
MY_PLUGIN_MAILER_ADDRESS=unix:///run/my/mailer.sock
```

`plugin_mailer_address` ist bei diesem Wert erforderlich. Eine Nachricht kann ein Secret enthalten, etwa einen Bestätigungslink, daher
lehnt die Art `http://` ab, auch auf Loopback: Nutze einen Unix-Socket oder `https://`. Der Server startet, ohne das Plugin aufzurufen, auch wenn es
ausgefallen ist, und die E-Mail wartet in ihrem Job.

Mit `plugin_mailer_protocol = json`, dem Standard, braucht ein Plugin nur einen HTTP-Server und eine JSON-Bibliothek
([Ein Plugin ohne Bibliothek](plugins.md#a-plugin-with-no-library)). `Send` dauert höchstens 30 Sekunden oder `plugin_mailer_timeout`,
unter 5 Minuten. Eine Anfrage ist höchstens 2 MiB groß und eine Antwort höchstens 64 KiB.

```text
POST /my.plugin.mailer.v1.MailerService/Send
Content-Type: application/json
Connect-Protocol-Version: 1

{
  "category": "CATEGORY_NOTIFICATION",
  "to": "alice@example.com",
  "envelopeFrom": "noreply@social.example",
  "fromName": "My Server",
  "replyTo": "admin@social.example",
  "subject": "You have a new follower",
  "text": "Bob followed you.\n",
  "html": "<p>Bob followed you.</p>",
  "headers": {
    "List-Unsubscribe": "<https://social.example/unsubscribe?token=abc>",
    "List-Unsubscribe-Post": "List-Unsubscribe=One-Click"
  }
}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_RECIPIENT_REJECTED","enhancedStatus":"5.1.1"}
```

Der Server prüft eine Nachricht, bevor er das Plugin aufruft, sodass sich über keines dieser Felder ein Header einschleusen lässt. Die Anfrage
enthält die Teile der Nachricht, und das Plugin baut, was sein Dienst braucht:

- **Das Plugin fügt drei Header hinzu:** eine `Message-ID`, die pro Nachricht eindeutig ist, auf der Domain von `envelopeFrom`; ein `Date`; und
  `Auto-Submitted: auto-generated`, das verhindert, dass automatische Antworten in Schleifen laufen. Ein Dienst, der selbst `Message-ID` und
  `Date` schreibt, deckt diese beiden ab.
- `From` ist `fromName` und `envelopeFrom`. `fromName`, `replyTo`, `html` und `headers` können fehlen. `headers` enthält nur die
  beiden `List-Unsubscribe`-Header.
- Die Adressen sind UTF-8. Ein Plugin, dessen Dienst ASCII braucht, wandelt die Domain in Punycode um und antwortet mit `OUTCOME_INVALID` für einen
  lokalen Teil, an den es nicht senden kann.
- Dieselbe Nachricht kann zweimal kommen, da ein fehlgeschlagener Job wiederholt wird. Schreibe keine Nachricht in ein Log.

`outcome` sagt, was mit der Nachricht geschah:

| `outcome` | Bedeutung | Der Server |
| --- | --- | --- |
| `OUTCOME_SENT` | Der Dienst hat die Nachricht angenommen | Schließt den Job ab |
| `OUTCOME_INVALID` | Sie lässt sich so nicht senden | Lässt den Job sofort fehlschlagen |
| `OUTCOME_RECIPIENT_REJECTED` | Die Adresse existiert nicht | Lässt den Job sofort fehlschlagen und [unterdrückt die Adresse](#suppression-of-addresses-that-cannot-receive-email) |
| `OUTCOME_REJECTED` | Der Dienst hat sie aus einem anderen Grund endgültig abgelehnt | Lässt den Job sofort fehlschlagen |
| `OUTCOME_TEMPORARY` | Der Dienst hat sie vorerst abgelehnt oder nicht geantwortet | Wiederholt den Job |
| Etwas anderes oder nichts | | Wiederholt den Job |

Ein HTTP-Fehler oder das `{"code":"unavailable"}` von Connect mit dem passenden Status ist ebenfalls ein vorübergehender Fehlschlag.
`enhancedStatus` ist optional: der erweiterte Statuscode einer Ablehnung, etwa `5.1.1`. Der Server behält einen Code der Klasse 5 bei einer
Ablehnung und der Klasse 4 bei `OUTCOME_TEMPORARY` und ignoriert alles andere. Er behält keinen Text von einem Plugin, also gib dort keinen Grund an:
Ein Grund gehört in das eigene Log des Plugins.

Der Server erfährt von einem Bounce nur aus dieser Antwort. Er hat keinen Endpunkt für die späteren Bounces und Beschwerden des Dienstes eines Plugins,
daher unterdrückt ein Bounce, den der Dienst nachträglich meldet, die Adresse nicht.

Teste das Plugin mit der `my`-Binary:

```sh
my plugin test --kind mailer unix:///run/my/mailer.sock
my plugin test --kind mailer --mail-to you@example.com --mail-from noreply@social.example unix:///run/my/mailer.sock
```

Der erste prüft nur, dass das Plugin antwortet, und sendet nichts. Der zweite sendet eine echte Nachricht an `--mail-to` und schickt dem
Plugin eine Anfrage ohne Empfänger, die es ablehnen muss. Der Server kann die Nachricht nicht sehen, nachdem das Plugin sie hat, also sieh dir
die an, die ankommt: Ihre Header müssen `Message-ID`, `Date` und `Auto-Submitted: auto-generated` enthalten.
Siehe [Ein Plugin testen](plugins.md#test-a-plugin).

<!-- source: receive-ses-notifications-sns e21b650ab5fe -->
### SES-Benachrichtigungen empfangen (SNS)

SES-Bounces und -Beschwerden gehen an ein SNS-Topic, und ein HTTPS-Abo auf diesem Topic liefert sie an
`POST /mail/events/ses` des Servers.
Der Endpunkt öffnet sich nur, wenn `mail_ses_sns_topic_arn` gesetzt ist; sonst antwortet er mit `404`.

1. Lege ein Standard-SNS-Topic in derselben Region wie die SES-Identität an.
2. Setze das Topic-Attribut `SignatureVersion` auf `2` (SHA256withRSA). Eine Signatur mit dem Standardwert `1` wird nicht akzeptiert, und der
   Server antwortet mit `400`.
3. Erlaube in der Zugriffsrichtlinie des Topics `ses.amazonaws.com` das `SNS:Publish` aus deinem eigenen Konto (die SES-Konsole fügt das hinzu,
   wenn sie das Topic anlegt).
4. Richte die SES-Benachrichtigungen auf das Topic, auf eine der beiden Arten (der Server liest sowohl `notificationType` als auch `eventType` im Body).
   - Identitätsbenachrichtigungen: Nutze dieses Topic für die Feedback-Benachrichtigungen „Bounce“ und „Complaint“ der Identität `mail_from`.
   - Ereignisziel eines Configuration Sets: Füge dieses Topic als Ziel des Configuration Sets in `mail_ses_configuration_set` hinzu,
     für die Ereignistypen `BOUNCE` und `COMPLAINT`.
5. Setze `MY_MAIL_SES_SNS_TOPIC_ARN` auf den ARN des Topics, prüfe mit `my config check` und starte neu.
6. Lege ein Abo auf dem Topic mit dem Protokoll `https` und dem Endpunkt `https://<server host>/mail/events/ses` an.
   **Aktiviere nicht die Raw Message Delivery** (ein Body ohne Signatur wird abgelehnt).

```sh
aws sns create-topic --region ap-northeast-1 --name my-ses-feedback
aws sns set-topic-attributes --region ap-northeast-1 \
  --topic-arn arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback \
  --attribute-name SignatureVersion --attribute-value 2
aws ses set-identity-notification-topic --region ap-northeast-1 --identity social.example \
  --notification-type Bounce --sns-topic arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback
aws ses set-identity-notification-topic --region ap-northeast-1 --identity social.example \
  --notification-type Complaint --sns-topic arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback

MY_MAIL_SES_SNS_TOPIC_ARN=arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback   # restart, then create the subscription below

aws sns subscribe --region ap-northeast-1 \
  --topic-arn arn:aws:sns:ap-northeast-1:123456789012:my-ses-feedback \
  --protocol https --notification-endpoint https://social.example/mail/events/ses
```

Wenn der Server die Abo-Bestätigung (`SubscriptionConfirmation`) erhält, sendet er ein `GET` an ihre `SubscribeURL`, um das Abo
zu bestätigen, und protokolliert `SNS subscription of the SES notifications confirmed`.
Prüfe mit `aws sns list-subscriptions-by-topic`, dass das Abo nicht mehr `PendingConfirmation` ist.

Bevor der Endpunkt eine Nachricht verwendet, prüft er alles Folgende. Schlägt etwas fehl, antwortet er mit `400` und zeichnet nichts auf.

- `TopicArn` entspricht `mail_ses_sns_topic_arn`.
- `SignatureVersion` ist `2` und die Signatur ist gültig.
- Die Zertifikat-URL der Signatur (`SigningCertURL`) ist `https`, und ihr Host ist `sns.<region>.amazonaws.com` für die Region
  des Topics. Die `SubscribeURL` einer Abo-Bestätigung muss denselben Host haben.

Das Zertifikat wird mit dem ausgehenden HTTP-Client abgerufen, der vor SSRF schützt, und 24 Stunden im KV-Speicher zwischengespeichert. Der
Body wird bis zu 1 MiB gelesen, und ein größerer bekommt `413`.
Wenn das Zertifikat nicht abgerufen werden kann, die Abo-Bestätigung fehlschlägt oder der Datensatz nicht geschrieben werden kann, antwortet der Endpunkt
mit `503` und überlässt SNS die Wiederholung. Er antwortet auch im Wartungsmodus mit `503`.

Das sind die Benachrichtigungen, die er aufzeichnet. Er gleicht die Empfängeradresse mit der primären Adresse ab (ohne Beachtung der Groß- und Kleinschreibung, und eine
Punycode-Domain passt auch in ihrer Unicode-Form); passt keine, tut er nichts.

| Benachrichtigung | Aufgezeichnet als |
| --- | --- |
| Ein Bounce, dessen `bounceType` `Permanent` ist (jeder Untertyp wie `General`, `NoEmail`, `Suppressed` oder `OnAccountSuppressionList`) | `hard_bounce` |
| Eine Beschwerde (außer solchen, deren `complaintFeedbackType` `not-spam` ist) | `complaint` |
| Ein Bounce, der `Transient` oder `Undetermined` ist, und andere Benachrichtigungen wie Zustellung oder Versand | Nicht aufgezeichnet (antwortet mit `200`) |

Die SES-`feedbackId` ist der eindeutige Schlüssel eines Datensatzes, sodass es unabhängig davon, wie oft SNS dieselbe Benachrichtigung sendet, einen Datensatz gibt.

<!-- source: amazon-ses-smtp-interface f46a7a562966 -->
### Amazon SES (SMTP-Schnittstelle)

Wenn du keine IAM-Zugangsdaten vergeben kannst, etwa wenn du außerhalb von AWS läufst, nutze die SES-SMTP-Schnittstelle mit `mail = smtp`. Bei diesem
Setup wird nur eine Ablehnung des Empfängers beim Senden als Unterdrückung aufgezeichnet
([Unterdrückung von Adressen, die keine E-Mails empfangen können](#suppression-of-addresses-that-cannot-receive-email)).

1. Verifiziere in SES die Domain von `mail_from` als Identität und aktiviere Easy DKIM (siehe unten).
2. Beantrage den Produktionszugriff, um die Sandbox zu verlassen. In der Sandbox kannst du nur an verifizierte Adressen senden.
3. Erstelle SMTP-Zugangsdaten unter „SMTP settings“ in SES (diese unterscheiden sich von IAM-Zugriffsschlüsseln).

```sh
MY_MAIL=smtp
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SMTP_HOST=email-smtp.ap-northeast-1.amazonaws.com   # the endpoint of your SES region
MY_MAIL_SMTP_PORT=587                                       # STARTTLS
MY_MAIL_SMTP_USERNAME=<user name of the SMTP credentials>
MY_MAIL_SMTP_PASSWORD_FILE=/run/secrets/mail_smtp_password
```

SES überschreibt `Message-ID` und `Date` mit eigenen Werten.

<!-- source: a-general-smtp-submission-server 62921988a793 -->
### Ein allgemeiner SMTP-Submission-Server

Nutze das für deinen eigenen SMTP-Server wie Postfix oder für den Submission-Server eines E-Mail-Zustellanbieters.

```sh
MY_MAIL=smtp
MY_MAIL_FROM=noreply@social.example
MY_MAIL_SMTP_HOST=smtp.provider.example     # a STARTTLS server whose certificate is valid for this name
MY_MAIL_SMTP_PORT=587
MY_MAIL_SMTP_USERNAME=<user name>           # empty for a server without authentication
MY_MAIL_SMTP_PASSWORD_FILE=/run/secrets/mail_smtp_password
```

Der Server sendet nicht an einen Server, der kein STARTTLS anbietet oder dessen Zertifikat für den Namen in
`mail_smtp_host` nicht geprüft werden kann. Zugangsdaten werden erst nach STARTTLS gesendet.
Prüfe nach einer Änderung der Einstellungen mit `my config check` und starte neu.

<!-- source: spf-dkim-and-dmarc d6b36a4f4d74 -->
## SPF, DKIM und DMARC

Gmail und Yahoo verlangen SPF, DKIM, DMARC und Ein-Klick-Abmeldung von Absendern, die am Tag viele E-Mails senden. Selbst bei kleinem
Volumen wird eine E-Mail ohne sie eher als Spam eingestuft.
Setze alle in den DNS der Domain von `mail_from` (zum Beispiel `social.example`), mit den Werten, die dein Anbieter dir nennt.

- **SPF**: Lege einen TXT-Eintrag auf der Domain an, der den Anbieter autorisiert, über den du sendest (wenn du zum Beispiel eine eigene SES-
  MAIL-FROM-Domain nutzt, das `include:`, das der Anbieter vorgibt, auf dieser Domain).
- **DKIM**: Lege den öffentlichen Schlüssel, den der Anbieter ausstellt, im DNS ab und aktiviere das Signieren auf der Sendeseite. Bei SES sind das die drei
  Easy-DKIM-CNAME-Einträge. Der Server selbst signiert nicht, also signiert der Anbieter.
- **DMARC**: ein TXT-Eintrag bei `_dmarc.social.example`. **Beginne mit `p=none`**, prüfe anhand der Aggregatberichte (`rua=`), dass SPF
  und DKIM übereinstimmen (zur From-Domain passen), und verschärfe dann auf `quarantine` und `reject`.

```text
_dmarc.social.example.  TXT  "v=DMARC1; p=none; rua=mailto:dmarc@social.example"
```

Um SPF bei SES in Übereinstimmung zu bringen, setze eine eigene MAIL-FROM-Domain (die Standard-MAIL-FROM ist `amazonses.com`, die nicht zur From-
Domain passt). Wenn DKIM übereinstimmt, besteht DMARC.

<!-- source: look-at-send-failures ab9c4f356d3b -->
## Sendefehler ansehen

Sendefehler siehst du auf der Jobs-Seite (`/admin/jobs/list`, Berechtigung `manage_jobs`; siehe
[die Job-Queue und fehlgeschlagene Jobs](running.md#the-job-queue-and-failed-dead-jobs)).
Filtere nach Job-Typ, um nur E-Mail-Jobs zu sehen (in der Admin-API `GET /api/v1/admin/jobs?type=identity.verification_email`).
Die Typen stehen in der Tabelle oben.

- **Vorübergehende Fehler** (Verbindungsfehler, Timeouts, 4xx-Antworten) werden wiederholt. Transaktionale E-Mails werden 6-mal versucht (ein
  exponentielles Backoff ab 30 Sekunden, insgesamt etwa 15 Minuten); Benachrichtigungs-E-Mails nutzen die Standardrichtlinie (10-mal, etwa
  4 Stunden). Sind die Versuche aufgebraucht, wird der Job zu einem fehlgeschlagenen Job (Dead Job). Ein falsches `mail_smtp_host`, eine fehlgeschlagene
  Authentifizierung und ein blockiertes Netzwerk zeigen sich hier. Führe ihn nach der Behebung der Ursache mit `my job retry <id>` oder auf dem
  Jobs-Bildschirm erneut aus.
- **Endgültige Ablehnungen** (ein 5xx für den Empfänger oder den Inhalt) werden nicht wiederholt und schlagen sofort fehl.
- Mit `mail = ses` ist `MessageRejected` von SES (der Inhalt wurde abgelehnt) eine endgültige Ablehnung. `TooManyRequestsException`
  (das Limit der Senderate), `SendingPausedException`, `MailFromDomainNotVerifiedException`, `AccountSuspendedException` und
  Anfragen, die SES nicht erreichen, sind vorübergehende Fehler, und eine Wiederholung gelingt, sobald du das Konto oder die Berechtigungen behebst.
  Im Fehlertext bleibt nur der SES-Fehlercode erhalten.
- Mit `mail = plugin` entscheidet das [`outcome`](#a-plugin-mail--plugin) der Antwort. Ein Plugin, das nicht erreichbar ist oder das
  mit einem Fehler antwortet, ist ein vorübergehender Fehler und zählt in [`my.plugin.failures`](running.md). Im Fehlertext bleiben nur das Outcome und der erweiterte
  Statuscode erhalten, daher steht der Grund eines Fehlers im Log des Plugins.
- Eine transaktionale E-Mail, die älter ist als ein Zeitlimit ab ihrer Anforderung (15 Minuten bei einem Zurücksetzen, 1 Stunde bei einer Bestätigung), wird
  ohne Versand abgeschlossen. So kommen alte Links nicht verspätet an; das ist kein Fehler.
- Wenn der Job läuft und es keinen Grund zum Senden gibt (die Adresse ist unterdrückt, sie wurde inzwischen bestätigt oder der Nutzer
  hat die Berechtigung oder die Einstellung für die Benachrichtigung verloren), schließt er ohne Versand ab.

Eine Job-Payload enthält keine E-Mail-Adresse, keinen Text und kein Token, und ein Fehlertext ebenso wenig. Es bleiben nur IDs, der Job-Typ und der SMTP-
Antwortcode.
Ein erfolgreicher Versand lässt sich in der strukturierten `job`-Logzeile jedes Laufs verfolgen (`my.job.type`).

<!-- source: suppression-of-addresses-that-cannot-receive-email 18b4750f1531 -->
## Unterdrückung von Adressen, die keine E-Mails empfangen können

Wenn der SMTP-Server sagt, dass die Empfängeradresse nicht existiert (ein 5xx auf `RCPT TO` mit einem erweiterten Status `5.1.x`), zeichnet der
Server die Adresse als Hard Bounce auf und sendet nicht mehr an sie.
Mit `mail = plugin` zeichnet `OUTCOME_RECIPIENT_REJECTED` denselben Hard Bounce auf, und sonst nichts.
Mit `mail = smtp` wird nur diese Ablehnung beim Senden aufgezeichnet. Asynchrone Bounces, die zurückkommen, nachdem der SMTP-Server die E-Mail
angenommen hat, und Feedback-Loops für Beschwerden werden nicht empfangen.
Mit `mail = ses` lehnt SES einen Empfänger nie synchron ab, daher wird beim Senden nichts aufgezeichnet; endgültige Bounces und
Beschwerden werden aus den SES-Benachrichtigungen aufgezeichnet
([SES-Benachrichtigungen empfangen (SNS)](#receive-ses-notifications-sns)).

| Art | E-Mail, die gestoppt wird |
| --- | --- |
| `hard_bounce` | Alles außer der Bestätigungs-E-Mail. Auch die E-Mail zum Zurücksetzen des Passworts wird gestoppt |
| `complaint` (als Spam gemeldet) | Benachrichtigungs- und Ankündigungs-E-Mails. Transaktionale E-Mails werden weiterhin gesendet |

- Moderatoren sehen die Art und die Herkunft einer Unterdrückung auf der Verwaltungsseite des Kontos in den Admin-Bildschirmen (`email_suppression` in
  `GET /api/v1/admin/accounts/{id}`).
- Es gibt zwei Wege, sie **aufzuheben**. Wenn der Nutzer eine Bestätigungs-E-Mail anfordert und die Adresse erneut bestätigt, wird die Hard-Bounce-
  Unterdrückung aufgehoben (die Bestätigungs-E-Mail wird auch gesendet, solange die Adresse unterdrückt ist). Der Nutzer fordert sie über die
  Client-API an (`POST /api/v1/accounts/email/verification`, höchstens 3 pro Stunde) oder auf der Statusseite der Anmeldung. Wenn du geprüft hast, dass das
  Senden in Ordnung ist, zum Beispiel weil das Postfach repariert wurde, hebt ein Moderator sie mit
  `POST /api/v1/admin/accounts/{id}/email_suppression/lift` auf (Berechtigung `manage_accounts`). Das zeichnet
  `email_suppression_lifted` im Audit-Log auf ([Moderation](moderation.md)).
- Für einen Nutzer, dessen E-Mail zum Zurücksetzen des Passworts gestoppt ist, stellt ein Administrator einen Zurücksetzungs-Link aus und übergibt ihn
  (`my admin account password-reset-link @alice`).
- In den Einstellungen sehen Nutzer, dass E-Mails ihre primäre Adresse nicht erreichen, und seit wann.

<!-- source: unsubscribing 2806dbccc409 -->
## Abmelden

Benachrichtigungs-E-Mails tragen einen Abmeldelink und die Header `List-Unsubscribe` und `List-Unsubscribe-Post:
List-Unsubscribe=One-Click` (RFC 8058). Die Schaltfläche „unsubscribe“ von E-Mail-Clients nutzt sie.

- Die Abmeldeklassen entsprechen eins zu eins den Zeilen der Benachrichtigungseinstellungen, deren E-Mail der Nutzer wählen kann, etwa
  `mention` oder `admin_report` (eine pro Art des Hinweises an Moderatoren und `moderation` für die Hinweise der Moderatoren).
- Der Link ist `https://{host}/mail/unsubscribe?token=...` und braucht keine Anmeldung. Ein Token wird pro E-Mail ausgestellt, nur sein Hash wird
  in der Datenbank gespeichert, und er läuft nach 90 Tagen ab.
  - `GET` zeigt nur einen Bestätigungsbildschirm und ändert nichts (damit Scanner, die Links vorab abrufen, die E-Mail nicht stoppen).
    Die Schaltfläche sendet dasselbe `POST`.
  - `POST` (ein Klick) schaltet die E-Mail dieser Zeile der Benachrichtigungseinstellungen aus und sonst nichts. Bis das Token
    abläuft, liefert ein erneutes Senden dasselbe Ergebnis.
- **Von transaktionalen E-Mails kann man sich nicht abmelden.** Bestätigung, Zurücksetzen und der Hinweis auf eine Passwortänderung würden
  sicherheitsrelevante Hinweise am Ankommen hindern. Diese E-Mails tragen auch keine solchen Header.
- Nutzer können dieselbe Einstellung auf der Seite **Benachrichtigungen** in den Einstellungen umschalten.

<!-- source: the-html-version 2ca008b1a48a -->
## Die HTML-Version

Jede E-Mail ist `multipart/alternative` mit demselben Inhalt als `text/plain` und `text/html` (beide UTF-8). Mit `mail = none`
im Profil `development` wird nur die Textversion ins Log geschrieben.

- Es gibt ein äußeres Layout, mit Absätzen, Schaltflächen und einer Signatur (Servername, URL, Kontakt, der Grund für die E-Mail
  und der Abmeldelink). `<html lang dir>` stammt aus dem Locale des Empfängers, und Sprachen wie `ar` werden von rechts
  nach links gesetzt.
- Es lädt keine externen Schriften oder Stylesheets. Die Farben sind die Standard-Design-Tokens, und hell und dunkel folgen
  `color-scheme` und `prefers-color-scheme`. Die Theme-Überschreibungen des Administrators wirken sich nicht auf E-Mails aus.
- Es lädt keine Bilder, außer dem eigenen Logo des Servers, wenn eines gesetzt ist ([Logo und Icon](instance-settings.md#logo-and-icon)):
  Die HTML-Version beginnt dann mit dem Logo (`https://<host>/brand/...`, stattdessen dem dunklen Logo, wenn der E-Mail-Client einem dunklen Farbschema folgt), mit derselben URL für jeden Empfänger, sodass sie nichts darüber verrät, wer die E-Mail geöffnet hat. Clients, die keine
  externen Bilder laden, zeigen stattdessen den Servernamen (den Alt-Text des Bildes). Ohne Logo oder wenn die Server-URL nicht `https` ist,
  hat die E-Mail kein Bild.
- Nur `https`-URLs werden zu Links. Ist eine URL `http`, wie in einem Entwicklungs-Setup, in dem `web_origin` `http://{host}` ist, wird die URL
  als Text statt als Schaltfläche oder Abmeldelink geschrieben, und die Server-URL in der Signatur entfällt.
- Das Layout ist Teil der Binary, sodass zur Laufzeit im Server kein JavaScript läuft.
