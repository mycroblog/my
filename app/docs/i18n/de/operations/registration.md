<!-- source: user-registration-registration-modes-invites-approval-email-confirmation 26f54561fd7a -->
# Nutzerregistrierung (Registrierungsmodi, Einladungen, Freigabe, E-Mail-Bestätigung)

Wie sich Nutzer registrieren und ihre E-Mail-Adresse bestätigen, die Registrierungsmodi, Einladungscodes, die Freigabe von Anträgen, das Zurücksetzen von Passwörtern und Registrierungssperren.

<!-- source: from-sign-up-to-the-app fff50db25fa9 -->
## Von der Registrierung bis zur App

![Die Wege eines neuen Nutzers: das Registrierungsformular oder ein Login-Anbieter, die Prüfungen, die die Registrierung ablehnen können, der Status, den jeder Registrierungsmodus vergibt, und die Freigabe und E-Mail-Bestätigung vor der App. Die Liste unten sagt dasselbe.](images/sign-up-flow.svg)

Vom oberen Rand des Diagramms nach unten:

- **Start.** Ein Besucher legt auf der Registrierungsseite (`/auth/sign_up`) ein Konto mit Nutzername, E-Mail-Adresse und
  Passwort an, was die Seite nicht anbietet, solange [die Registrierung auf Login-Anbieter beschränkt ist](#registration-limited-to-login-providers).
  Oder der Besucher meldet sich mit einem [Login-Anbieter](login-providers.md) an, dessen Konto noch nicht mit einem Nutzer verknüpft ist; wenn
  der Anbieter Registrierungen erlaubt (`-allow-signup`), erhält der Besucher dasselbe Formular ohne das Passwortfeld.
- **Prüfungen, der Reihe nach.** Der Registrierungsmodus (`closed` lehnt jede Registrierung ab), das Limit pro Netzwerk (5 pro 30 Minuten),
  CAPTCHA (wenn `captcha` nicht `none` ist), die [Registrierungssperren](#sign-up-blocks) (`reject` lehnt ab, `require_approval`
  hält zur Freigabe zurück), der Einladungscode (er muss verwendbar sein, und der Modus `invite` verlangt einen), das Plugin von
  [`signup.check`](decision-hooks.md#sign-ups) für eine Registrierung ohne Einladung sowie ein Nutzername und eine E-Mail-Adresse, die
  nicht vergeben sind. Eine Registrierung, die eine Prüfung nicht besteht, wird mit einem Fehler im Formular abgelehnt, und nichts wird geschrieben.
- **Status direkt nach der Registrierung.** Der neue Nutzer ist sofort angemeldet. Im Modus `open` ist die Registrierung freigegeben,
  oder sie wartet bei `mail = none` auf Freigabe; im Modus `approval` wartet sie auf Freigabe oder ist mit einer verwendbaren Einladung freigegeben;
  im Modus `invite` ist sie freigegeben. Eine `require_approval`-Sperre oder ein Zurückhalten durch `signup.check` lässt sie in jedem Modus warten.
- **Bis das Konto bereit ist.** Eine wartende Registrierung braucht die Freigabe eines Moderators (`my registration approve` oder
  die Admin-Screens). Die E-Mail-Adresse muss bei `mail = smtp` über den Link in einer E-Mail bestätigt werden; eine Adresse, die
  der Login-Anbieter verifiziert hat, ist sofort bestätigt, und bei `mail = none` ist keine Bestätigung nötig. Eine abgelehnte
  Registrierung ist nicht freigegeben, und das Konto kann die App nicht nutzen.
- **Die App.** Ein bereites Konto geht zum Willkommensschritt (ein Anzeigename und ein Avatar), dann zur Home-Timeline.

Die Abschnitte unten beschreiben jeden Schritt.

<!-- source: the-sign-up-page 111fbe5ecf64 -->
## Die Registrierungsseite

Nutzer registrieren sich unter `https://<host>/auth/sign_up` mit Nutzername, E-Mail-Adresse und Passwort, es sei denn, [die Registrierung ist auf Login-Anbieter beschränkt](#registration-limited-to-login-providers).
Nach der Registrierung ist dieser Browser sofort angemeldet.
Die Seite funktioniert ohne JavaScript und ist wie die Anmeldeseite durch ein CSRF-Token und durch das Abweisen von Cross-Origin-Anfragen geschützt.
Wenn du CAPTCHA als Bot-Schutz wählst, zeigen die Registrierungs- und Anmeldeseite sein Widget und brauchen JavaScript ([CAPTCHA](captcha.md)).

Der Registrierungsmodus ist die Server-Einstellung `registration_mode`. Wähle ihn mit der CLI oder im Web-Client auf der Seite „Registrierung“ unter „Servereinstellungen“ (`/admin/settings/registration`) ([Server-Einstellungen](instance-settings.md)). Eine Änderung gilt ab der nächsten Registrierung.
Solange der Server seine Schließung ankündigt und nachdem er geschlossen hat, gilt die Registrierung als `closed`, was auch immer die Einstellung sagt ([Admin-CLI](admin-cli.md#announcing-the-end-of-the-service)).
Ebenso auf einem [Ein-Personen-Server](instance-settings.md#one-person-server), wo Einladungen, die Prüfung von Registrierungen und die Registrierungssperren ebenfalls nicht angeboten werden und kein Weg ein Konto anlegt, `my admin create` eingeschlossen.

```sh
my instance set registration_mode invite
```

| Modus | Seite | Status direkt nach der Registrierung |
| --- | --- | --- |
| `open` | Jeder kann sich registrieren; ein Einladungscode ist optional | Freigegeben, außer bei `mail = none`, wo sie auf die Freigabe eines Moderators wartet (siehe unten). Eine Einladung ändert nichts, hält aber fest, wer den Nutzer eingeladen hat |
| `approval` (Standard) | Fragt außerdem „Warum möchtest du beitreten?“ (optional, bis zu 1.000 Zeichen); ein Einladungscode ist optional | Wartet auf Freigabe; mit einer verwendbaren Einladung sofort freigegeben |
| `invite` | Ein Einladungscode ist erforderlich | Freigegeben |
| `closed` | Sagt nur, dass keine Registrierung angenommen wird | Keiner |

- In jedem Modus außer `closed` hat die Seite ein Feld für den Einladungscode, ausgefüllt aus dem Link einer Einladung (`?invite=<code>`). Ein Code, der nicht funktioniert (unbekannt, widerrufen, abgelaufen, aufgebraucht oder von einem Mitglied, das nicht mehr einladen kann), wird in jedem Modus abgelehnt, nicht ignoriert, damit ein vertippter Code nicht unbemerkt bleibt.
- Eine Registrierungssperre, die eine Freigabe verlangt, hält eine Registrierung auch mit einer Einladung zur Freigabe zurück ([Registrierungssperren](#sign-up-blocks)).
- Ist für den Decision Hook [`signup.check`](decision-hooks.md#sign-ups) ein Plugin gesetzt, wird eine Registrierung ohne Einladung nach dem CAPTCHA und den Sperren zusätzlich von diesem Plugin geprüft, das sie zur Freigabe zurückhalten oder ablehnen kann.

- Registrierungen aus demselben Netzwerk (IP-Adresse; `/64` bei IPv6) sind auf 5 pro 30 Minuten begrenzt. Darüber hinaus liefert der Server `429` mit `Retry-After`.
- Ist der Nutzername oder die E-Mail-Adresse vergeben, zeigt die Seite einen Fehler an diesem Feld und schreibt nichts (eine Einladung wird ebenfalls nicht verbraucht).
- Wenn du [externe Login-Anbieter](login-providers.md) (OpenID Connect, GitHub, Discord oder Mastodon) einrichtest und neue Nutzer sich mit einem registrieren lässt, zeigen die Anmelde- und Registrierungsseiten für jeden aktivierten Anbieter einen Button, beschriftet mit dem Anzeigenamen des Anbieters genau so, wie du ihn eingegeben hast (ein GitHub-Anbieter zeigt „Mit GitHub anmelden“ und ein Discord-Anbieter „Mit Discord anmelden“, in der Sprache des Besuchers).
  Jemand, dessen Konto beim Anbieter nicht mit einem Nutzer dieses Servers verknüpft ist, erhält dasselbe Registrierungsformular ohne das Passwortfeld, und der neue Nutzer meldet sich nur mit dem Anbieter an.
  Der Registrierungsmodus, der Einladungscode, der Grund, CAPTCHA, die Registrierungssperren, `signup.check` und das Netzwerklimit gelten wie beim Passwortformular; im Modus `closed` oder wenn der Anbieter keine Registrierungen erlaubt, sagt die Anmeldeseite, dass kein Konto verknüpft ist.
  Eine Adresse, die der Anbieter als verifiziert markiert (`email_verified`; bei GitHub die primäre Adresse, wenn sie verifiziert ist; bei Discord die Adresse, wenn Discord sie als verifiziert markiert), ist sofort bestätigt, wenn der Nutzer sie behält; jede andere Adresse und jede Adresse eines Nutzers, der sich mit einem Mastodon-Server registriert, der keine liefert, wird wie unten per E-Mail bestätigt.
  Eine E-Mail-Adresse, die zu einem bestehenden Nutzer passt, verknüpft diesen Nutzer nie: Das Formular sagt, dass die Adresse vergeben ist.
  Ein Nutzer mit Zwei-Faktor-Authentifizierung gibt nach der Anmeldung mit einem Anbieter weiterhin den Code ein.
  Ein GitHub-Anbieter fragt GitHub nach dem Scope `user:email` und zusätzlich nach `read:org`, wenn du erlaubte Organizations aufführst; der Server prüft dann bei jeder Anmeldung, dass der Nutzer zu einer davon gehört, und lehnt die Anmeldung sonst ab. Registriere `https://<host>/auth/external/<provider ID>/callback` als Callback-URL deiner GitHub-OAuth-App.
  Ein Discord-Anbieter fragt Discord nach den Scopes `identify` und `email` und zusätzlich nach `guilds`, wenn du erlaubte Server aufführst; der Server prüft dann bei jeder Anmeldung, dass der Nutzer zu einem davon gehört. Registriere dieselbe Callback-URL in deiner Discord-Anwendung.
- Ein angemeldeter Nutzer verknüpft ein Konto bei einem Anbieter unter „Verknüpfte Konten“ in den Einstellungen, nachdem er mit dem Passwort, einem Passkey oder einem anderen verknüpften Konto bestätigt hat, und nur bei Anbietern, die Verknüpfungen erlauben.
  Der Browser muss auf den Anmeldeseiten als derselbe Nutzer angemeldet sein; sonst bittet der Server, sich zuerst anzumelden.
  Ein Konto bei einem Anbieter, das ein anderer Nutzer verknüpft, wird abgelehnt, und ein Nutzer verknüpft höchstens ein Konto pro Anbieter.
  Ein Nutzer kann ein Konto trennen, es sei denn, es ist der letzte Weg zur Anmeldung (kein Passwort, kein Passkey und kein anderes verknüpftes Konto).
- Ein Nutzer ohne Passwort und ohne Passkeys bestätigt die Vorgänge, die sonst danach fragen (ein Passwort setzen, einen Passkey hinzufügen, das Konto löschen oder umziehen, ein weiteres Konto verknüpfen), indem er sich bei einem verknüpften Anbieter erneut anmeldet.
  Ein OpenID-Connect-Anbieter wird gebeten, den Nutzer erneut zu authentifizieren (`max_age=0` und `prompt=login`), und eine frühere Authentifizierung wird abgelehnt.
  Die Bestätigung funktioniert einmal, nur für diesen Vorgang, innerhalb von 10 Minuten; die Anmeldung mit einem Konto, das nicht das verknüpfte ist, zählt als Fehlversuch, gegen dasselbe Limit wie ein falsches Passwort.
- Bei jeder Registrierung und Anmeldung hält der Server die Client-IP-Adresse fest (und bei der Anmeldung den User-Agent). Die Aufzeichnungen werden nach 180 Tagen automatisch entfernt
  (die Server-Einstellungen `session_origins_retention_days` und `registration_origins_retention_days`, mindestens 90 Tage). Aufzeichnungen eines Kontos unter einer Aufbewahrungssperre bleiben erhalten ([Aufbewahrung von Aufzeichnungen und Sperren](moderation.md#record-retention-and-holds)).
  Nenne diese Aufbewahrungsfrist in deiner Datenschutzerklärung ([Vorlage der Datenschutzerklärung](privacy-policy-template.md)).

<!-- source: when-a-user-can-use-the-app 317d6d8b7688 -->
## Wann ein Nutzer die App nutzen kann

Nutzer, die auf Freigabe warten, abgelehnt wurden oder ihre E-Mail-Adresse nicht bestätigt haben, können sich auf der Anmeldeseite weiterhin anmelden. Ebenso Nutzer, die ihr eigenes Konto deaktiviert haben, bis zum Datum seines Löschens: Sie sehen dort dieses Datum und können das Konto reaktivieren oder löschen ([Moderation](moderation.md#accounts-that-their-owners-deactivated)).
Nach der Anmeldung sehen sie, worauf sie warten (die Freigabe eines Moderators oder die Bestätigung der E-Mail-Adresse) oder dass ihre Registrierung nicht freigegeben wurde, und sie kommen nicht weiter.
Nur freigegebene Nutzer können die App nutzen (die OAuth-Autorisierung und Token-Ausstellung der Client-API), und bei Einrichtungen, die eine E-Mail-Bestätigung verlangen, nur die, die ihre E-Mail-Adresse bestätigt haben.

Ein neuer Nutzer, der die App sofort nutzen kann, geht von der Registrierungsseite direkt in die App, zu einem Willkommensschritt, der nach
einem Anzeigenamen und einem Avatar fragt. Die Home-Timeline zeigt dann eine Checkliste mit ersten Schritten und Vorschläge, wem man
folgen kann ([Vorschläge](instance-settings.md#suggestions-of-accounts)). „Weiter“ auf der Seite, die eine E-Mail-Adresse bestätigt,
führt zum selben Willkommensschritt.

Ein mit `my admin create` angelegter Administrator gilt als freigegeben und bestätigt.

<!-- source: email-confirmation 6c1b7759c2cf -->
## E-Mail-Bestätigung

Die Einstellung `mail` ([Konfigurationsreferenz](../reference/configuration.md#mail)) wählt, wie E-Mails gesendet werden.
SMTP-Beispiele, die DNS-Einrichtung und wie du Zustellfehler liest, stehen unter [E-Mails senden](email.md).

- `mail = smtp`: Der Server sendet bei der Registrierung eine Bestätigungs-E-Mail. Der Link ist 24 Stunden gültig und funktioniert einmal.
  Das Öffnen des Links zeigt eine Seite mit einem Bestätigungs-Button, und die Adresse ist bestätigt, wenn der Nutzer ihn drückt (damit Scanner, die Links in E-Mails vorab abrufen, das Token nicht verbrauchen).
  Nach der Anmeldung kann der Nutzer die E-Mail bis zu 3-mal pro Stunde erneut senden lassen.
  Die Verbindung zum SMTP-Server ist immer mit STARTTLS verschlüsselt, und der Server prüft das Zertifikat, bevor er sich authentifiziert und sendet. Er sendet nie an einen Server, der kein STARTTLS anbietet.
- `mail = none` (Standard): Der Server sendet keine E-Mails und schreibt `mail not sent (mail = none)` mit der Kategorie der Nachricht ins Server-Log. Nur mit `profile = development` enthält die Zeile auch die Adresse und den Body, einschließlich des Bestätigungslinks.
  Weil E-Mails den Nutzer nie erreichen, verlangt der Server statt der E-Mail-Bestätigung die Freigabe eines Moderators.
  Daher beginnt auch eine Registrierung im Modus `open` als auf Freigabe wartend, und eine unbestätigte E-Mail-Adresse blockiert die Nutzung der App nicht.

Das Bestätigungs-Token wird nirgends festgehalten außer in dieser Entwicklungs-Logzeile (Zugriffslogs halten keine Query-Strings fest). Die Datenbank speichert nur das SHA-256 des Tokens.

<!-- source: registration-limited-to-login-providers 0d05572b3521 -->
## Auf Login-Anbieter beschränkte Registrierung

Solange die Server-Einstellung `login_providers_only` eingeschaltet und mindestens ein [Login-Anbieter](login-providers.md#limit-sign-in-to-login-providers) aktiviert ist, hat die Registrierungsseite kein Passwortformular und bietet nur die Anbieter an, die Registrierungen erlauben; ein Einladungscode im Link wird in das Registrierungsformular des Anbieters übernommen. Eine Registrierung mit Passwort wird abgelehnt, was auch immer das Formular enthält. Ohne einen Anbieter, der Registrierungen erlaubt, gilt die Registrierung als `closed`. Ohne aktivierten Anbieter gilt die Einstellung nicht.

<!-- source: password-reset-and-change bec2baef5ab7 -->
## Passwort zurücksetzen und ändern

Solange die Server-Einstellung `login_providers_only` gilt, werden Links zum Zurücksetzen sowie das Setzen oder Ändern eines Passworts abgelehnt ([was die Beschränkung bewirkt](login-providers.md#limit-sign-in-to-login-providers)).

- `mail = smtp`: Gibt ein Nutzer auf der Anmeldeseite unter „Passwort vergessen?“ eine E-Mail-Adresse ein, sendet der Server einen Link zum Setzen eines neuen Passworts an den Nutzer, dessen primäre Adresse sie ist.
  Die Antwort der Seite ist dieselbe, ob ein Konto existiert oder nicht. Der Link ist 1 Stunde gültig und funktioniert einmal, und er hört auf zu funktionieren, wenn sich das Passwort in der Zwischenzeit auf einem anderen Weg ändert.
  Anfragen sind auf 5 pro 30 Minuten pro Client-IP-Adresse begrenzt und E-Mails auf 3 pro Stunde pro Adresse (eine Anfrage über einem Limit sendet nichts und erhält dieselbe Antwort). Ist CAPTCHA aktiviert, wird auch dieses Formular verifiziert.
- `mail = none`: Die Seite sagt, dass sie keinen Link senden kann, und weist den Nutzer an, einen Administrator zu fragen. Der Administrator stellt auf der Verwaltungsseite des Kontos in den Admin-Screens einen Link aus, oder mit `my admin account password-reset-link @alice`, und gibt ihn dem Nutzer ([Moderation](moderation.md)).
- Ein angemeldeter Nutzer kann das Passwort unter „Passwort“ in den Einstellungen ändern, indem er das aktuelle Passwort eingibt (falsche Eingaben sind auf 5 pro Stunde pro Nutzer begrenzt).
- Ein Zurücksetzen oder eine Änderung widerruft die anderen Sitzungen und App-Token des Nutzers (eine Änderung aus den Einstellungen behält nur die App, die die Änderung vorgenommen hat). Passkeys und Zwei-Faktor-Authentifizierung bleiben.
  Eine Anmeldung, die auf ihren Zwei-Faktor-Code wartet, und eine noch nicht abgeschlossene App-Autorisierung hören ebenfalls auf zu funktionieren: Der Nutzer startet sie erneut.
  Bei `mail = smtp` sendet der Server dem Nutzer außerdem eine E-Mail, dass sich das Passwort geändert hat.

<!-- source: invite-codes 5f98f9eb7d6f -->
## Einladungscodes

Einladungen funktionieren in den Modi `open`, `approval` und `invite`. Einladungen des Teams werden mit der CLI, den Moderations-Screens und der Admin-API unten erstellt; Mitglieder können auch eigene Einladungen erstellen ([Einladungen von Mitgliedern](#member-invites)).
Im Modus `closed` lässt sich keine Einladung erstellen, und keine funktioniert.

```sh
my invite create                          # single use, expires in 7 days
my invite create -max-uses 10 -expires-in 72h
my invite create -max-uses 0 -expires-in 0  # no limit on uses or time
my invite list                            # newest first; states: usable / revoked / expired / used up
my invite revoke <code>
```

- `create` gibt den Code (16 Zeichen) und den Link zur Registrierungsseite mit eingetragenem Code aus (`https://<host>/auth/sign_up?invite=<code>`).
- Codes werden ohne Beachtung von Groß- und Kleinschreibung, Leerzeichen und Bindestrichen abgeglichen.
- Auch wenn sich mehrere Leute gleichzeitig registrieren, wird ein Code nie über sein Limit hinaus verwendet (eine Unique-Constraint auf `invite_redemptions`).
- Ein widerrufener, abgelaufener oder aufgebrauchter Code ergibt jeweils denselben Fehler (eine nicht verwendbare Einladung).
- Neben der CLI kannst du Einladungen in den Moderations-Screens des Web-Clients („Einladungen“, `/moderation/invites`) und über die Client-API erstellen, auflisten und widerrufen
  (`GET` / `POST /api/v1/admin/invites` und `POST /api/v1/admin/invites/{id}/revoke`). Beides braucht die Rollenberechtigung `manage_invites`
  (der eingebaute `moderator` hat sie nicht; um das zu delegieren, lege also eine Rolle mit dieser Berechtigung an und erteile sie).
  „Einladung erstellen“ in der Liste öffnet das Formular (`/moderation/invites/new`), das den Code und den Registrierungslink der neuen Einladung zeigt.
  Die Liste lässt dich den Link einer verwendbaren Einladung kopieren und sie widerrufen.
- Die Liste zeigt jede Einladung, die des Teams und die der Mitglieder, mit dem, der sie erstellt hat (`created_by`), und wie (`creator_kind`: `staff` oder `member`). Die Inhaber von `manage_invites` können jede davon widerrufen.
- Die Prüfseite des Kontos in den Moderations-Screens (`GET /api/v1/admin/accounts/{id}`, Berechtigung `manage_accounts`) zeigt die Einladung, mit der sich das Konto registriert hat, und wer sie erstellt hat, die Person, die den Nutzer eingeladen hat.
- Das Erstellen und Widerrufen einer Einladung wird im Audit-Log festgehalten (`invite_created`, `invite_revoked`), aus dem Screen, der API und der CLI gleichermaßen. Der Zielname sind die ersten 4 Zeichen des Codes (`ABCD…`); der vollständige Code wird nicht ins Audit-Log kopiert.
  Das Widerrufen einer schon widerrufenen Einladung ändert nichts und wird daher nicht festgehalten.

<!-- source: member-invites db3bff74ed69 -->
## Einladungen von Mitgliedern

Mitglieder können neben dem Team eigene Einladungen erstellen.
Wer das darf, bestimmt die Server-Einstellung `invite_creators` ([Server-Einstellungen](instance-settings.md)):

- `permitted` (Standard): nur die Nutzer, deren Rollen die Berechtigung `create_invites` haben. Standardmäßig hat nur die eingebaute Rolle `admin` sie. Du kannst sie einer Rolle hinzufügen, die du von Hand erteilst, oder der Rolle eines [Unterstützungsplans](supporters.md), sodass Unterstützer Leute einladen können.
- `everyone`: jeder angemeldete lokale Nutzer.

So oder so kann ein Nutzer, dessen Konto gesperrt oder eingeschränkt ist oder dessen Registrierung nicht freigegeben ist, keine Einladungen erstellen, und niemand kann es, solange der Registrierungsmodus `closed` ist.
Die Inhaber von `manage_invites` erstellen weiterhin Einladungen des Teams, auf die keines der Limits unten zutrifft.

Ein Mitglied erstellt, listet, kopiert und widerruft seine eigenen Einladungen in den Einstellungen des Web-Clients („Deine Einladungen“, `/settings/invites`) oder über die Client-API (`GET` / `POST /api/v1/invites` und `POST /api/v1/invites/{id}/revoke`).
Die Einstellungen listen die Seite nur für einen Nutzer auf, der jetzt Einladungen erstellen kann; ein Nutzer, der es nicht mehr kann, öffnet sie weiterhin über ihre Adresse, um die von ihm erstellten Einladungen zu kopieren und zu widerrufen.
Die Liste sagt für jede Einladung, ob sie verwendbar, widerrufen, abgelaufen, aufgebraucht oder nicht verwendbar ist, weil ihr Ersteller nicht mehr einladen kann.

Limits, ebenfalls Server-Einstellungen, gelten nur für Einladungen von Mitgliedern.
Administratoren ändern sie und `invite_creators` unter „Einladungen von Mitgliedern“ auf der Seite „Registrierung“ der Servereinstellungen (`/admin/settings/registration`):

| Einstellung | Bedeutung | Standard | Bereich |
| --- | --- | --- | --- |
| `member_invite_max_active` | Wie viele verwendbare Einladungen (nicht widerrufen, abgelaufen oder aufgebraucht) ein Mitglied gleichzeitig halten darf | `5` | 0-100 (0: Mitglieder können keine Einladungen erstellen) |
| `member_invite_max_uses` | Wie viele Leute sich mit einer Einladung registrieren dürfen | `10` | 1-1.000 |
| `member_invite_max_lifetime_seconds` | Wie lange eine Einladung funktionieren darf, in Sekunden | `2592000` (30 Tage) | 3.600 (1 Stunde) bis 31.536.000 (365 Tage) |

- Ein Mitglied wählt die Verwendungen und die Lebensdauer innerhalb der Limits; ohne Wahl erlaubt eine Einladung 1 Registrierung und funktioniert 7 Tage lang (oder kürzer, wenn ein Limit niedriger ist). Eine Einladung ohne Limit für Verwendungen oder Zeit lässt sich nicht erstellen.
- Ein Limit zu senken ändert bestehende Einladungen nicht; es gilt für die nächste erstellte Einladung.
- Jede Registrierung mit der Einladung eines Mitglieds prüft, dass ihr Ersteller noch Einladungen erstellen kann. Hat der Ersteller `create_invites` verloren (zum Beispiel weil sein Tarif endete), ist `invite_creators` auf `permitted` zurückgegangen, oder das Konto wurde gesperrt, eingeschränkt oder gelöscht, funktioniert die Einladung nicht, und die Registrierungsseite sagt, dass sie nicht gültig ist.
  Die Einladung selbst wird nicht verändert, sie funktioniert also innerhalb ihres Ablaufs wieder, wenn der Ersteller wieder einladen darf.
- Das Erstellen und Widerrufen durch ein Mitglied wird im Audit-Log als `invite_created` und `invite_revoked` festgehalten, mit dem Mitglied als Akteur.

Im Modus `approval` gibt eine verwendbare Einladung den neuen Nutzer sofort frei: Die Person, die ihn eingeladen hat, tritt an die Stelle der Prüfung durch die Moderatoren.
Mit `invite_creators = everyone` kann daher jeder Nutzer Leute ohne Prüfung hereinlassen, was den Freigabemodus schwächt.
Wenn du dich auf die Prüfung verlässt, behalte `permitted` und gib `create_invites` nur Leuten, denen du vertraust, oder halte `member_invite_max_active` und `member_invite_max_uses` niedrig.

<!-- source: approving-and-rejecting e68676f67aab -->
## Freigeben und Ablehnen

```sh
my registration list                      # waiting for approval (longest wait first); shows the reason and whether the email is confirmed
my registration list -state rejected
my registration approve alice
my registration reject -note "spam" bob
```

- Das Argument ist der Nutzername eines lokalen Kontos (das führende `@` kann weggelassen werden).
- Nur Registrierungen, die auf Freigabe warten, können entschieden werden. Bei einer entschiedenen Registrierung oder einem Nutzer, der ohne die Registrierungsseite angelegt wurde (`my admin create`), ändert der Befehl nichts und endet mit dem Exit-Code 1.
- Der Exit-Code 2 bedeutet ein falsches Argument.

Dieselben Entscheidungen lassen sich in den Admin-Screens (der Client-API) treffen. Alle brauchen die Berechtigung `manage_accounts`.

| Operation | Was sie tut |
| --- | --- |
| `GET /api/v1/admin/registrations` | Registrierungen, die auf Freigabe warten (längste Wartezeit zuerst). Liefert die ID der Registrierung, den Beitrittsgrund, die E-Mail-Adresse und ob sie bestätigt ist sowie das lokale Konto |
| `POST /api/v1/admin/registrations/{id}/approve` | Gibt frei. Der Body `{"note": "..."}` ist optional. Die Antwort zeigt die Notiz als `decision_note`, und eine Antwort auf eine entschiedene Registrierung hat keine E-Mail-Adresse |
| `POST /api/v1/admin/registrations/{id}/reject` | Lehnt ab. Der Body ist derselbe wie bei `approve` |

- Das ist derselbe Anwendungsfall wie die CLI, und er hält die Entscheidung in `registration_decisions` und im Audit-Log fest (`registration_approved`, `registration_rejected`). Der Akteur einer CLI-Entscheidung ist `cli`; der Akteur einer API-Entscheidung ist der Moderator, der entschieden hat.
- Eine bereits entschiedene Registrierung zu entscheiden ändert nichts und liefert den aktuellen Status (entscheiden zwei Leute gleichzeitig, wird nur eine Entscheidung festgehalten). Ein Nutzer, der ohne die Registrierungsseite angelegt wurde, ergibt 404.

<!-- source: sign-up-blocks b70eb18cd52f -->
## Registrierungssperren

Sperren lehnen Registrierungen von der Registrierungsseite ab oder stellen sie auf Freigabe um, nach E-Mail-Domain, E-Mail-Adresse oder IP-Bereich.

```sh
my admin signup-block create -kind email_domain spam.example          # also applies to subdomains; a U-label works too
my admin signup-block create -kind ip_range -severity require_approval -duration 720h 192.0.2.0/24
my admin signup-block create -kind email_hash spammer@example.com     # only a hash of the address is kept
my admin signup-block list -active
my admin signup-block lift <block id>
```

- Eine Registrierung, die zu einer Sperre mit `-severity reject` (dem Standard) passt, wird abgelehnt, und eine, die zu `require_approval` passt, beginnt als auf Freigabe wartend, welcher Registrierungsmodus auch gilt. Passen mehrere, gilt die stärkere. Die Registrierungsseite zeigt den Grund einer Ablehnung nicht.
- `email_hash` wird mit der Adresse in Kleinbuchstaben abgeglichen, ohne den Teil des lokalen Teils ab dem ersten `+`. Ein IP-Bereich wird mit der Registrierungs-IP abgeglichen (derselbe Wert wie `registration_origins`) und gilt nie für eine Registrierung, deren IP unbekannt ist.
- Abgelaufene und aufgehobene Sperren haben keine Wirkung. Hinzufügen und Aufheben werden im Audit-Log festgehalten (`signup_block_added`, `signup_block_lifted`).
- Wenn du ein lokales Konto mit `-block-email` bannst (`block_email: true` in der API), legt der Server außerdem eine `email_hash`-Sperre für die E-Mail-Adresse dieses Nutzers an.

Für eine Liste mit Tausenden Einträgen, etwa den Domains von Wegwerf-E-Mail-Diensten, füge sie mit `import` aus einer Datei in großer Zahl hinzu (`-` für die Standardeingabe). Jeder Wert erhält dieselbe Art, Schwere, denselben Kommentar und dieselbe Dauer.

```sh
my admin signup-block import -kind email_domain -comment "disposable email" disposable.txt
```

- Die Liste hat einen Wert pro Zeile (dieselbe Form wie `<value>` von `add`) oder CSV-Zeilen, deren erste Spalte der Wert ist. Leere Zeilen und Zeilen, die mit `#` beginnen, werden übersprungen. Ein Import nimmt bis zu 10.000 Einträge.
- Der Befehl gibt aus, wie viele hinzugefügt wurden, und für jede Zeile, die er nicht hinzugefügt hat, die Zeilennummer, den Wert und den Grund. Der Grund ist `invalid` (kein gültiger Wert dieser Art), `duplicate` (dieselbe Normalform wie eine frühere Zeile derselben Liste) oder `already_blocked` (eine Sperre derselben Art und desselben Werts, die gleich streng oder strenger ist, ist in Kraft).
- Jede hinzugefügte Sperre wird im Audit-Log festgehalten (`signup_block_added`) wie bei `add` und lässt sich einzeln aufheben.
- Der Import schreibt in Transaktionen von einigen Hundert, wenn er also mittendrin scheitert, bleibt, was zuvor hinzugefügt wurde. Dieselbe Liste erneut zu importieren fügt nur den Rest hinzu.
- Die Software liefert keine Liste mit und ruft auch nicht periodisch eine von einer URL ab. Öffentliche Listen (etwa disposable-email-domains auf GitHub) holst und aktualisierst du selbst und importierst die aktualisierte Liste erneut.

Dieselben Operationen sind in den Admin-Screens (der Client-API) verfügbar. Alle brauchen die Berechtigung `manage_accounts`.
Öffne im Web-Client Moderation → Registrierungssperren (`/moderation/signup-blocks`). Die Liste zeigt Wert, Art, Status und Ende jeder Sperre, die in Kraft befindlichen oder alle;
„Blockade hinzufügen“ öffnet das Formular (`/moderation/signup-blocks/new`), „Aus einer Liste hinzufügen“ importiert eine Liste und zeigt die übersprungenen Zeilen (`/moderation/signup-blocks/import`),
und die Wahl eines Werts öffnet die Seite der Sperre mit ihrer Wirkung und ihrem Kommentar, wo „Aufheben“ sie beendet.

| Operation | Was sie tut |
| --- | --- |
| `GET /api/v1/admin/signup_blocks` | Listet Sperren (neueste zuerst). Filtere mit `kind` und `active` |
| `POST /api/v1/admin/signup_blocks` | Fügt eine Sperre hinzu. Der Body hat `kind`, `value`, `severity`, `comment` und `expires_in` (Sekunden, höchstens zehn Jahre) |
| `POST /api/v1/admin/signup_blocks/import` | Fügt Sperren aus einer Liste hinzu. Der Body hat `kind`, `text` (die Liste), `severity`, `comment` und `expires_in`. Die Antwort hat `added`, die Zahl der hinzugefügten, und `skipped`, die nicht hinzugefügten Zeilen (`line`, `value`, `reason`) |
| `GET /api/v1/admin/signup_blocks/{id}` | Liefert eine Sperre |
| `POST /api/v1/admin/signup_blocks/{id}/lift` | Hebt eine Sperre auf. Eine bereits aufgehobene Sperre bleibt, wie sie ist |
