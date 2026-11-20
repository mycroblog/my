<!-- source: account-and-domain-actions-and-handling-reports 7f21d5c6c925 -->
# Maßnahmen gegen Konten und Domains und die Bearbeitung von Meldungen

Wie Betreiber und Moderatoren Maßnahmen gegen Konten und Domains ergreifen, über Einsprüche entscheiden, Meldungen bearbeiten, prüfen, was einen entfernten Server erreicht hat, und das Audit-Log lesen.
Alles hier außer der Kontoverwaltung und der Prüfung von Zustellungen (nur über die Admin-API) lässt sich über die Admin-API (`/api/v1/admin/...`, Scopes `admin:read` / `admin:write`) und die CLI (`my admin account action` / `my admin domain` / `my admin report` / `my admin appeal`) erledigen.
Die Admin-API verlangt Rollenberechtigungen: Kontoverwaltung, Maßnahmen und Einsprüche brauchen `manage_accounts`, Domain-Maßnahmen brauchen `manage_domains`, und Meldungen brauchen `manage_reports`.
Die CLI verbindet sich direkt mit der DB, mit derselben Konfiguration wie der Server (`-config` oder Umgebungsvariablen), und hält nicht fest, welcher Nutzer sie bedient hat (`issued_by_user_id` und ähnliche Felder sind NULL, und der Akteur im [Audit-Log](#audit-log) ist `cli`).

<!-- source: from-a-report-to-an-action-and-an-appeal dabd0eabc7c7 -->
## Von einer Meldung zu einer Maßnahme und einem Einspruch

![Der Weg einer Meldung: Sie geht ein, ein Moderator prüft sie, eine Maßnahme kann folgen, mit einer Benachrichtigung an den Nutzer, und der Nutzer kann Einspruch einlegen; die rechte Spalte zeigt, was jeder Schritt an andere Server sendet. Die Liste unten beschreibt jeden Schritt.](images/moderation-lifecycle.svg)

Vom oberen Rand des Diagramms nach unten:

1. **Eine Meldung geht ein.** Ein lokaler Nutzer reicht eine ein, ein anderer Server sendet ein `Flag` (verworfen, wenn seine Domain
   `reject_reports` hat, siehe [Domain-Maßnahmen](#domain-actions)), oder ein [Decision Hook](decision-hooks.md) antwortet mit `flag`
   ([Meldungen, die eine externe Prüfung eingereicht hat](#reports-that-an-external-review-filed)). Ein lokaler Nutzer, der ein
   Konto auf einem anderen Server meldet, kann sich dafür entscheiden, die Meldung weiterzuleiten, was sofort ein `Flag` an diesen Server sendet.
2. **Prüfung.** Die Meldung ist `open`, und die Moderatoren erhalten eine `admin_report`-Benachrichtigung. Sie abzuschließen macht sie
   `resolved` mit dem Ergebnis `actioned` oder `dismissed`, und ein lokaler Melder erhält eine `report_resolved`-Benachrichtigung.
   Wiedereröffnen setzt sie zurück auf `open`. Eine Meldung über ein Konto auf einem anderen Server weiterzuleiten sendet ein `Flag` an diesen
   Server ([Meldungen](#reports)).
3. **Maßnahme.** Ein Moderator ergreift aus der Meldung heraus eine [Kontomaßnahme](#account-actions), was die Meldung als
   `actioned` abschließt; eine Maßnahme kann auch ohne Meldung ergriffen werden. Die Maßnahme hat eine Art, eine Begründung und eine Endzeit und
   endet, wenn sie aufgehoben wird oder abläuft. Mit `notify_target` erhält der Nutzer eine `moderation_notice`-Benachrichtigung
   ([was der Nutzer sieht](#what-the-user-sees)). Andere Server erhalten ein `Delete` der Beiträge bei `delete_statuses`, ein
   `Update` des Akteurs, wenn `suspend` oder `ban` eines lokalen Kontos beginnt oder endet, und ein `Reject` / `Undo` der
   Follows, die das `suspend` eines entfernten Kontos trennt.
4. **Einspruch.** Der lokale Nutzer legt Einspruch gegen die Maßnahme ein. Der Einspruch ist `pending`, und die Moderatoren erhalten eine `admin_appeal`-
   Benachrichtigung. Die Entscheidung lautet `upheld` oder `overturned`, und der Nutzer erhält eine `appeal_decided`-Benachrichtigung.
   Das Aufheben hebt die Maßnahme auf und bricht das Löschen bei einem `ban` ab, sodass ein aufgehobenes `suspend` oder `ban` eines lokalen
   Kontos ein `Update` des Akteurs sendet ([Einsprüche](#appeals)).

Zuweisungen, Notizen, Benachrichtigungen und Einsprüche bleiben auf diesem Server; nur die Aktivitäten in der rechten Spalte verlassen ihn.

<!-- source: moderation-and-server-administration-in-the-web-client 035f2c60c4a9 -->
## Moderation und Serveradministration im Web-Client

Der Web-Client teilt die Bildschirme für das Personal in zwei.
Die Navigation zeigt „Moderation“ und „Administration“ nur Leuten, deren Rolle mindestens eine Berechtigung dieser Gruppe hat. Jeder Bildschirm zeigt Leuten ohne die Berechtigung nur einen Grund, und der Server lehnt auch ihre Anfragen ab.

| Gruppe | Berechtigungen | Bildschirme |
| --- | --- | --- |
| Moderation (`/moderation`) | `manage_reports`, `manage_accounts`, `manage_domains`, `manage_invites`, `view_audit` | Meldungen (`/moderation/reports`), Einsprüche (`/moderation/appeals`), Registrierungen (`/moderation/registrations`), Automatische Prüfungen (`/moderation/hook-decisions`), Konten (`/moderation/accounts`), Registrierungssperren (`/moderation/signup-blocks`), Einladungen (`/moderation/invites`), Domain-Maßnahmen (`/moderation/domain-actions`), Entfernte Server (`/moderation/instances`), Audit-Log (`/moderation/audit-log`) |
| Serveradministration (`/admin`) | `manage_settings`, `manage_rules`, `manage_roles`, `manage_jobs`, `manage_login_recovery`, `manage_custom_emojis`, `manage_webhooks`, `manage_announcements`, `manage_finance`, `manage_supporters`, `manage_oauth_apps`, `manage_service_closure`, `manage_errors`, `manage_login_providers` | Servereinstellungen (`/admin/settings`), Regeln (`/admin/rules`), Bedingungen und Richtlinien (`/admin/documents`), Link-Karten-Domains (`/admin/link-card-domains`), die Job-Queue und fehlgeschlagene Jobs (`/admin/jobs`), Fehler (`/admin/errors`, `manage_errors`), UX-Messungen (`/admin/ux-measurements`, `manage_errors`), Eigene Emojis (`/admin/custom-emojis`), Ankündigungen (`/admin/announcements`), Webhooks (`/admin/webhooks`), Kosten und Einnahmen (`/admin/finance`, `manage_finance`), Unterstützer (`/admin/supporters`, `manage_supporters`). `manage_login_recovery` (Login-Wiederherstellung: Zwei-Faktor-Authentifizierung entfernen und einen Link zum Zurücksetzen des Passworts ausstellen) und `manage_roles` (die Rollen eines Kontos vergeben und entziehen) erscheinen als Abschnitte der Verwaltungsseite eines Kontos (`/moderation/accounts/{id}/manage`, die außerdem `manage_accounts` braucht) |

- Das Badge an „Moderation“ ist die Summe der offenen Meldungen, der Registrierungen, die auf Genehmigung warten, und der Einsprüche, die auf Prüfung warten ([Benachrichtigungen für Moderatoren und die ausstehenden Zähler](#notifications-for-moderators-and-the-pending-counts)). „Administration“ zeigt keinen Zähler.
- Jede Gruppe öffnet sich auf einer Liste ihrer Bildschirme, die die Berechtigungen deiner Rolle öffnen, jeder mit einer Zeile dazu, was er enthält, und bei einer Warteschlange damit, wie viele Dinge dort warten (Meldungen zählt die offenen Meldungen, Einsprüche die auf Prüfung wartenden, Registrierungen die auf Genehmigung wartenden, Eigene Emojis die auf Prüfung wartenden Anfragen, Fehler die offenen Fehler); eine Gruppe, von deren Bildschirmen dir keiner offensteht, sagt, warum.
  Die Adresse eines Bildschirms, den deine Rolle nicht öffnet, sagt das ebenfalls, mit einem Link zur Liste der Gruppe.
- Der eingebaute `moderator` hat nur `manage_reports`, `manage_accounts` und `view_audit` und sieht „Administration“ nicht. Er sieht auch die Bildschirme Domain-Maßnahmen und Entfernte Server (`manage_domains`) nicht,
  erstelle also, um diese zu delegieren, eine Rolle mit dieser Berechtigung und vergib sie ([Rollen vergeben und entziehen](admin-cli.md#granting-and-revoking-roles)).
- Die Pfade der Admin-API bleiben `/api/v1/admin/...`, unabhängig von der Gruppe, und die Scopes sind dasselbe Paar `admin:read` / `admin:write`. Die Gruppen sind in der API-Referenz durch Tags getrennt (`admin-moderation`, `admin-server`).
  Ein Dienst außerhalb des Servers kann die Operationen einer Berechtigung stattdessen mit einem Berechtigungs-Scope aufrufen ([einen anderen Dienst die Admin-API nutzen lassen](#letting-another-service-use-the-admin-api)).
- Es gibt eine dritte Gruppe von Berechtigungen, „Nutzerfunktionen“ (`view_finance`, `create_custom_emojis`, `start_voice_rooms`, `create_invites`, `keep_posts_longer`, `create_articles`, `create_events`), die außerhalb der Bildschirme für das Personal lebt. Wer nur diese Berechtigungen hat, sieht weder „Moderation“ noch „Administration“,
  und für jemanden mit `view_finance` zeigt die Navigation „Finanzen“ (`/finance`) nach den Einstellungen.
  Ihre API liegt im Nutzer-Namespace (`GET /api/v1/finance/cost_totals` und `GET /api/v1/finance/income_totals`, Scope `read:finance`) und braucht keinen `admin:*`-Scope. Was sie zeigt, wird durch `finance_viewer_*` in den [Instanzeinstellungen](instance-settings.md) festgelegt.

<!-- source: letting-another-service-use-the-admin-api d767e08e639c -->
### Einen anderen Dienst die Admin-API nutzen lassen

Ein Dienst, den du neben dem Server betreibst, etwa einer, der Einladungen verteilt oder Kosten erfasst, kann die Admin-API für eine Berechtigung ohne Plugin aufrufen.

1. Registriere ein Konto für den Dienst und gib ihm eine Rolle mit nur den Berechtigungen, die der Dienst braucht (für Einladungen eine Rolle mit `manage_invites`; [Rollen vergeben und entziehen](admin-cli.md#granting-and-revoking-roles)).
2. Registriere, angemeldet als dieses Konto, in **Einstellungen → Apps → Deine Apps** eine App, deren Scopes den Berechtigungs-Scope enthalten, `admin:` und die Berechtigung: `admin:manage_invites`.
3. Stelle dem Konto von der App aus ein Token mit diesem Scope aus und gib dem Dienst das Zugriffstoken und das Refresh-Token.

- Das Token erreicht nur die Operationen der Berechtigungen, die seine Scopes nennen, und nur, solange die Rollen des Kontos sie behalten; das Audit-Log erfasst, was es tut, als Aktionen des Kontos.
- Das Zugriffstoken gilt eine Stunde. Der Dienst erneuert es mit dem Refresh-Token und behält jedes Mal das neue Refresh-Token: Ein altes, das erneut verwendet wird, widerruft das Token.
- `manage_roles`, `manage_oauth_apps`, `manage_login_recovery`, `manage_login_providers` und `manage_service_closure` haben keinen Berechtigungs-Scope, und keine App kann ein Mitglied des Personals auf dem Zustimmungsbildschirm um einen bitten.
- Um den Dienst zu stoppen, widerrufe die Tokens der App oder lösche die App, entziehe die Rolle oder deaktiviere die App unter Administration → OAuth-Apps.

<!-- source: account-actions a61dac7c5921 -->
## Kontomaßnahmen

Eine Maßnahme wird in der Transaktion, die sie festhält, auf die Einschränkungs-Projektion (`account_restrictions`) angewendet und wirkt ab der nächsten Anfrage.

| Art | Wirkung auf ein lokales Konto |
| --- | --- |
| `warn` | Keine Einschränkung (informiert nur den Nutzer) |
| `force_sensitive` | Zeigt alle Anhänge als sensibel |
| `limit` | Behandelt Beiträge (auch vergangene) als nur für Follower und hält sie aus den öffentlichen Timelines heraus |
| `restrict_posting` | Verbietet Posten, Antworten, Boosten und Bearbeiten |
| `restrict_interaction` | Verbietet Folgen, Favorisieren, Boosten und Antworten |
| `disable_login` | Verbietet die Nutzung der App und der API und widerruft aktive Sitzungen und Token (die Anmeldeseite lässt den Nutzer die Maßnahme weiterhin sehen) |
| `suspend` | Wendet alle obigen Einschränkungen an und verbirgt Profil und Beiträge vor allen außer dem Nutzer |
| `ban` | Wendet dieselben Einschränkungen an wie `suspend` und löscht das Konto nach 30 Tagen. Sie lässt sich nicht aufheben (sie wird nur abgebrochen, wenn ein Einspruch, der in diesen 30 Tagen eingelegt wurde, aufgehoben wird) |
| `delete_statuses` | Löscht die angegebenen Beiträge. Sie werden auch für den Nutzer unsichtbar, und der Server sendet anderen Servern dasselbe `Delete` wie beim Löschen durch den Nutzer |
| `mark_statuses_sensitive` | Zeigt die Anhänge der angegebenen Beiträge als sensibel |

Für ein entferntes Konto kannst du nur `limit`, `suspend` und `force_sensitive` ergreifen (jede andere Art ergibt `422` in der Admin-API und scheitert in der CLI).
Die Wirkung ist lokal für diesen Server. Das Einzige, was an den anderen Server gesendet wird, sind das `Reject` / `Undo` von Follows, die `suspend` trennt.

| Art | Wirkung auf ein entferntes Konto |
| --- | --- |
| `force_sensitive` | Zeigt alle Anhänge als sensibel |
| `limit` | Behandelt Beiträge (auch vergangene) als nur für Follower, hält sie aus den öffentlichen Timelines heraus und erzeugt keine Benachrichtigungen für Nutzer, die dem Konto nicht folgen |
| `suspend` | Verwirft alles, was vom Konto empfangen wird, und verbirgt sein Profil und seine Beiträge. Trennt Follows zwischen dem Konto und lokalen Nutzern in beide Richtungen (Folgeanfragen ebenfalls) und sendet dem anderen Server ein `Reject` der Follows des Kontos und ein `Undo` der Follows der Nutzer. Das Aufheben bringt die Anzeige zurück, und der Server empfängt wieder vom Konto (was verworfen wurde und die Follows, die während der Sperre getrennt wurden, kommen nicht zurück) |

- `notify_target` (`-no-notify`) hat für ein entferntes Konto keine Bedeutung (keine Benachrichtigung erreicht es).
- Ein lokaler Nutzer, der durch ein `suspend` Follows verloren hat, erhält eine Benachrichtigung (`relationships_severed`), die sagt, wie viele Follower und Follows verloren gingen ([getrennte Follows](#severed-follows)).
- Ergreifen und Aufheben werden im [Audit-Log](#audit-log) festgehalten, wie bei Maßnahmen gegen lokale Konten.

```sh
my admin account action issue -kind suspend -category spam -public-reason "spam" -duration 720h @alice
my admin account action issue -kind limit -report <report id> @bob@remote.example
my admin account action issue -kind delete_statuses -status <status id> -status <status id> @alice
my admin account action issue -kind ban -category legal -public-reason "illegal content" @alice
my admin account action list -active @alice
my admin account action lift -reason "appealed" <action id>
```

In der Admin-API: `POST /api/v1/admin/accounts/{id}/actions` (der Body hat `kind`, `reason_category`, `public_reason`, `private_note`, `report_id`, `notify_target`, `expires_in` in Sekunden und `status_ids`),
`GET /api/v1/admin/accounts/{id}/actions` (filtere mit `active`), `GET /api/v1/admin/account_actions/{id}` und `POST .../{id}/lift` (der Body `{"reason": "..."}` ist optional).

- Benenne ein Konto in der CLI mit `@username` (lokal), `@username@domain` (entfernt) oder der Konto-ID und in der Admin-API mit der Konto-ID.
- Mehrere Maßnahmen stapeln sich (die Vereinigung ihrer Einschränkungen). Das Aufheben entfernt nur die Einschränkungen dieser Maßnahme, und eine Maßnahme mit Endzeit endet zu dieser Zeit von selbst (in Listen als `expired` gezeigt). Eine Endzeit liegt höchstens zehn Jahre entfernt (`expires_in` bis 315360000); eine längere wird mit `422` abgelehnt.
- Sitzungen und Token, die `disable_login`, `suspend` und `ban` widerrufen haben, kommen beim Aufheben der Maßnahme nicht zurück. Der Nutzer meldet sich erneut an.
- Ein `suspend` oder `ban` eines lokalen Kontos sendet ein Actor-`Update` mit geleertem Profil an die Server der Follower und an bekannte Server, um die Sperre anzuzeigen (Mastodon zeigt sie als „suspended“).
  Aufheben und Ablauf senden ein `Update` mit dem ursprünglichen Profil. Der Ablauf wird von einem Job gesendet, der alle 5 Minuten läuft, sodass er bis zu 5 Minuten zu spät sein kann.
- `delete_statuses` und `mark_statuses_sensitive` nehmen 1 bis 100 ungelöschte Beiträge des Zielkontos über `-status` (`status_ids`). Sie können keine Endzeit haben.
  Das Aufheben von `delete_statuses` bringt die gelöschten Beiträge nicht zurück. Das Aufheben von `mark_statuses_sensitive` stellt die ursprüngliche Anzeige wieder her.
- Ein `ban` kann keine Endzeit haben und lässt sich nicht aufheben. Er wird nur abgebrochen, wenn ein Einspruch, den der Nutzer in den 30 Tagen vor dem Löschen eingelegt hat, aufgehoben wird ([Einsprüche](#appeals)). Verwende im Zweifel ein `suspend` mit Endzeit.
  Dreißig Tage nach der Maßnahme löscht ein Job, der jede Stunde läuft, das Konto (das Konto und die Beiträge verschwinden aus der Ansicht, und der Server sendet ein `Delete` an andere Server).
- `report_id` (`-report`) kann nur eine Meldung über das Zielkonto benennen. Eine offene Meldung wird zusammen mit der Maßnahme als `actioned` abgeschlossen (der Abschluss steht auch im Audit-Log). Eine abgeschlossene Meldung ändert sich nicht.
- `public_reason` ist die Begründung, die dem Nutzer gezeigt wird, und `private_note` ist eine Notiz nur für Moderatoren. `notify_target` (standardmäßig true; false mit `-no-notify` in der CLI) bestimmt, ob der Nutzer informiert wird.

<!-- source: what-the-user-sees 0ca06c59f8e1 -->
### Was der Nutzer sieht

- Eine Maßnahme mit `notify_target` true erreicht den Nutzer als Benachrichtigung (`moderation_notice`). Sie trägt die Art, die Begründungskategorie, `public_reason`, die Endzeit und den Status. Art und Kategorie kommen auch als Anzeigenamen in der UI-Sprache des Nutzers zurück.
- Der Nutzer kann die Maßnahmen gegen sich mit dem First-Party-Endpunkt `GET /api/v1/moderation/actions` (filtere mit `active`) prüfen, neueste zuerst. Maßnahmen mit `notify_target` false sowie aufgehobene oder abgelaufene Maßnahmen werden ebenfalls aufgelistet.
- `private_note`, die Meldung, wer die Maßnahme ergriffen hat, und der Grund für das Aufheben werden dem Nutzer nie gezeigt.
- Ein Nutzer kann gegen jede Maßnahme einmal Einspruch einlegen ([Einsprüche](#appeals)). Die Maßnahmenliste und die Maßnahme in einer Benachrichtigung enthalten, ob Einspruch möglich ist (`appealable`), die Frist (`appeal_deadline`) und den Einspruch samt Ergebnis (`appeal`).
- Ein Nutzer unter `disable_login`, `suspend` oder `ban` kann die App oder die API nicht nutzen, kann sich aber weiterhin auf der Anmeldeseite (`/auth/sign_in`) anmelden. Dort sieht er die Art, die Begründung und die Endzeit der geltenden Maßnahme
  (bei `ban` die Zahl der Tage bis zum Löschen des Kontos), und er kann von dieser Seite aus Einspruch einlegen ([Einsprüche](#appeals)). Der Autorisierungsendpunkt (`/oauth/authorize`) antwortet mit `403` und stellt keinen Token aus.
  Ein Nutzer unter `ban` kann sich bis zum Löschen des Kontos anmelden, und das Löschen widerruft auch diese Sitzung. Ein Nutzer, dessen eigenes Löschen geplant ist, und ein Konto, das deaktiviert wurde, können sich auch auf der Anmeldeseite nicht anmelden.

<!-- source: accounts-that-their-owners-deactivated ed010f49b40a -->
### Konten, die ihre Inhaber deaktiviert haben

Ein Nutzer kann sein eigenes Konto in den Kontoeinstellungen deaktivieren, nachdem er sein Passwort oder einen
Zwei-Faktor-Code erneut eingegeben oder mit einem Passkey bestätigt hat (ein Nutzer ohne eines davon meldet sich bei einem verknüpften
Login-Anbieter erneut an). Das Konto ist keine Moderationsmaßnahme, und Moderatoren können niemanden deaktivieren:

- Profil und Beiträge sind für alle außer Moderatoren verborgen, und andere Server erhalten ein `Update` des
  Akteurs, als `suspended` markiert, sodass sie das Konto als gesperrt anzeigen können. Jede Sitzung und jeder Token des Nutzers endet.
  Wo der Server E-Mails sendet, nennt eine E-Mail das Datum des Löschens; davor wird keine weitere E-Mail gesendet.
- Nichts erreicht das Konto: Erwähnungen und Direktnachrichten werden verworfen, es entstehen keine Benachrichtigungen, E-Mails oder Web-Push,
  und neue Follower werden abgewiesen. Favoriten und Boosts werden gezählt. Bestehende Follows bleiben.
- Der Nutzer meldet sich auf einer Serverseite an, die das Datum des Löschens zeigt und ihn das Konto reaktivieren oder
  löschen lässt. Das Reaktivieren bringt alles zurück. Die nächste Deaktivierung muss 24 Stunden warten.
- Dreißig Tage nach der Deaktivierung löscht der stündliche Job das Konto. Eine Reaktivierung nach diesem Datum wird abgelehnt,
  auch wenn der Job noch nicht gelaufen ist. Der Nutzername wird nicht freigegeben.

Moderatoren sehen den Status `deactivated` in der Kontenliste (`state=deactivated`) und können weiterhin Meldungen über
das Konto lesen und Maßnahmen dagegen ergreifen. Eine Moderationsmaßnahme ändert das Datum des Löschens nicht, außer einem `ban`, der es
durch sein eigenes ersetzt: Von da an kann der Nutzer nicht mehr reaktivieren. Eine Sperre sendet nichts an andere Server, solange das
Konto deaktiviert ist, und sie bleibt nach der Reaktivierung durch den Nutzer in Kraft. Ein Nutzer kann ein Konto nicht deaktivieren, solange
die Moderation es an der Anmeldung hindert, solange sein Löschen geplant ist, nachdem es umgezogen ist, solange seine Registrierung auf Prüfung
wartet, solange ein bezahltes Abo noch läuft (auch eines, das zum Ende seines Zeitraums endet), oder wenn
es der einzige Administrator ist, der sich anmelden kann. Ein deaktivierter Administrator zählt nicht als einer, der es kann. Wenn
das Datum eines deaktivierten Administrators verstreicht, während sich kein anderer Administrator anmelden kann, löscht der stündliche Job das
Konto nicht und protokolliert eine Warnung. Dieser Administrator kann sich ebenfalls nicht mehr anmelden, lege also einen weiteren Administrator mit
`my admin create` an; der nächste Lauf löscht dann das Konto.

<!-- source: appeals 3366a552f16e -->
## Einsprüche

Ein lokaler Nutzer kann gegen eine Maßnahme gegen ihn Einspruch einlegen.
Ein Moderator prüft den Einspruch und bestätigt ihn entweder (`uphold`) oder hebt die Maßnahme auf (`overturn`).

```sh
my admin appeal list -state pending
my admin appeal uphold -reason "The posts were spam." <appeal id>
my admin appeal overturn -reason "Sorry, our mistake." <appeal id>
```

Der Nutzer legt den Einspruch mit `POST /api/v1/moderation/actions/{id}/appeal` ein (Scope `write:accounts`, Body `{"text": "..."}`).
Ein Nutzer unter `disable_login`, `suspend` oder `ban` bekommt kein Zugriffstoken und legt den Einspruch über das Formular je Maßnahme auf der Anmeldeseite ein (`POST /auth/appeal`).
Solange die [Anmeldung auf Login-Anbieter beschränkt ist](login-providers.md#limit-sign-in-to-login-providers), erreicht ein solcher Nutzer das Formular nur über einen verknüpften Anbieter.
Die Seite zeigt das Formular und die Frist bei einer Maßnahme, gegen die Einspruch möglich ist, den Inhalt und das Ergebnis bei einer Maßnahme, gegen die schon Einspruch eingelegt wurde, und den Grund bei einer Maßnahme, deren Frist abgelaufen ist oder gegen die kein Einspruch möglich ist. Bei einem abgelehnten Einspruch zeigt sie den Grund.
In der Admin-API: `GET /api/v1/admin/appeals` (filtere mit `state`), `GET .../{id}` sowie `POST .../{id}/uphold` und `POST .../{id}/overturn` (der Body `{"public_reason": "..."}` ist optional). Sie brauchen die Berechtigung `manage_accounts`.
Im Web-Client listet Moderation → Einsprüche (`/moderation/appeals`) die Einsprüche, die auf Prüfung warten, oder alle auf; die Seite eines Einspruchs (`/moderation/appeals/{id}`) zeigt, was der Nutzer geschrieben hat, und die Maßnahme, mit dem Grund, der dem Nutzer mitgeteilt wird, und „Bestätigen“ und „Aufheben“, jeweils mit vorheriger Rückfrage.

- Gegen eine Maßnahme kann einmal Einspruch eingelegt werden, und der Text hat 1 bis 2.000 Zeichen. Die Frist ist die Zahl der Tage in der Instanzeinstellung `appeal_window_days` nach der Maßnahme (Standard 180 Tage, 20 bis 365; siehe [Instanzeinstellungen](instance-settings.md)).
  Eine Maßnahme nach Fristablauf und ein zweiter Einspruch ergeben `422` (`/api/v1/problems/appeal-window-closed` und `/api/v1/problems/already-appealed`).
- Gegen einen `ban` kann Einspruch eingelegt werden, bis das Konto gelöscht ist (30 Tage nach der Maßnahme oder bis zur Frist von `appeal_window_days`, wenn diese kürzer ist). Ein `ban` eines gelöschten Kontos ergibt `422` (`/api/v1/problems/not-appealable`). Gegen eine aufgehobene oder abgelaufene Maßnahme kann innerhalb der Frist weiterhin Einspruch eingelegt werden.
- Wenn ein Nutzer Einspruch einlegt, erhalten Leute mit `manage_accounts` eine Benachrichtigung (`admin_appeal`). Wenn er geprüft ist, erhält der Nutzer eine Benachrichtigung (`appeal_decided`) und kann das Ergebnis und `public_reason` sehen. Der Prüfer wird nicht angezeigt.
- Das Aufheben hebt die Maßnahme in derselben Transaktion auf (dieselbe Wirkung wie `lift`, mit leerem Aufhebungsgrund). Das Aufheben von `delete_statuses` bringt die gelöschten Beiträge nicht zurück. Eine Maßnahme, die schon vor der Prüfung aufgehoben war, bleibt, wie sie ist.
- Das Aufheben eines `ban` bricht in derselben Transaktion auch das geplante Löschen des Kontos ab (das Audit-Log erfasst `account_deletion_canceled`), beendet die Sperre und sendet das ursprüngliche Profil-`Update` an andere Server. Widerrufene Sitzungen und Token kommen nicht zurück, der Nutzer meldet sich also erneut an.
  Wurde das Konto vor der Prüfung gelöscht, lässt sich der Einspruch nicht aufheben (`422`, `/api/v1/problems/not-appealable`; Bestätigen ist weiterhin möglich).
- Eine Prüfung geschieht einmal. Eine erneute Prüfung mit demselben Ergebnis ändert nichts, und das entgegengesetzte Ergebnis ergibt `409` (`/api/v1/problems/appeal-decided`).

<!-- source: account-management aa223fe467e1 -->
## Kontoverwaltung

Konten suchen, ihre Details ansehen und IP-Adressen und E-Mail-Adressen offenlegen geschieht über die Admin-API und braucht die Berechtigung `manage_accounts`.

Im Web-Client hat ein Konto drei Seiten, mit Links zwischen ihnen unter dem Seitentitel:

- **Prüfung** (`/moderation/accounts/{id}`): der Zustand und die Einschränkungen des Kontos, seine Registrierung und wer es eingeladen hat, das Formular für eine neue Maßnahme (von einer Meldung aus mit „Gegen dieses Konto vorgehen“ geöffnet, bietet das Formular auch die Maßnahmen gegen die Beiträge der Meldung an, und die Maßnahme schließt die Meldung ab), die neuesten Maßnahmen dagegen mit ihren Gründen, mit „Alle Maßnahmen“ zu seiner Seite Maßnahmen, wenn es mehr geben kann, und seine Meldungen, die neuesten zuerst, der Rest unter „Mehr anzeigen“.
- **Maßnahmen** (`/moderation/accounts/{id}/actions`): jede Maßnahme gegen das Konto oder die wirksamen, jeweils eine Seite. Jede Maßnahme öffnet ihre eigene Seite (`/moderation/accounts/{id}/actions/{action id}`) mit allen Details und, solange sie wirksam ist, „Aufheben“ mit einem Grund für die Moderatoren. Die Seite eines Bans hat kein „Aufheben“: Nur das Aufheben seines Einspruchs macht ihn rückgängig.
- **Verwaltung** (`/moderation/accounts/{id}/manage`): bei einem lokalen Konto seine Rollen (`manage_roles`), seine E-Mail-Sperre, das Entfernen der Zwei-Faktor-Authentifizierung und das Ausstellen eines Links zum Zurücksetzen des Passworts (`manage_login_recovery`) sowie seine E-Mail- und IP-Adressen, die erst nach „Anzeigen“ erscheinen, mit seinen Anmeldungen und den Konten, die dieselben IP-Adressen verwendet haben, jede Liste mit den neuesten zuerst und dem Rest unter „Mehr anzeigen“ (jedes Drücken liest eine Seite und wird im Audit-Log festgehalten), und einem Link zu denen mit derselben E-Mail-Domain; bei einem entfernten Konto das erneute Abrufen seiner Beiträge von seinem Server (`manage_domains`). Jeder Abschnitt erscheint nur Leuten mit der dazugehörigen Berechtigung.

Das Personal handelt nur gegen niedrigere Ränge: Eine Maßnahme zu ergreifen oder aufzuheben, über einen Einspruch zu entscheiden, eine E-Mail-Sperre aufzuheben, private Daten offenzulegen, einen Link zum Zurücksetzen des Passworts auszustellen und die Zwei-Faktor-Authentifizierung zu entfernen setzt voraus, dass die höchste `position` unter deinen Rollen über der jeder Rolle des Ziels liegt.
Ein höherer oder gleicher Rang, dein eigenes Konto eingeschlossen, ergibt `403` mit dem Problemtyp `/api/v1/problems/target-outranks-actor`, sodass ein Moderator keinen Administrator bannen kann und ein Administrator keinen anderen. Die Befehle `my admin` haben keine solche Grenze.
Eine Maßnahme, die den einzigen Administrator, der sich anmelden kann, an der Anmeldung hindert (`disable_login`, `suspend`, `ban`), wird abgelehnt, wer auch immer sie ergreift, und dieser Administrator kann sein eigenes Konto nicht löschen (`409` mit `/api/v1/problems/last-administrator`). Zu den Rollen siehe [admin-cli.md](admin-cli.md).

- `GET /api/v1/admin/accounts` liefert lokale und entfernte Konten, die neuesten zuerst (der Instanz-Actor ist ausgenommen).
  Du kannst nach Benutzernamen-Präfix (`username`), Domain (`domain`), lokal oder entfernt (`origin`), Zustand (`state`) und bei lokalen Konten nach E-Mail-Adresse (`email`), E-Mail-Domain (`email_domain`) und der IP-Adresse der Registrierung oder Anmeldung (`ip`) filtern.
  Du kannst nur eines von `email`, `email_domain` und `ip` angeben. Die Liste enthält keine IP-Adressen oder E-Mail-Adressen.
- Der Zustand ist der erste zutreffende aus dieser Liste: `deleted`, `suspended` (`suspend`, `ban`), `disabled` (`disable_login`), `rejected` und `pending` (Registrierungsprüfung), `limited` (`limit`), `active`.
  Der Zustand und die Einschränkungen (`restrictions`) stammen nur aus Maßnahmen gegen das Konto selbst und enthalten keine Domain-Maßnahmen.
- `GET /api/v1/admin/accounts/{id}` liefert den Zustand und die Einschränkungen und bei einem lokalen Konto den Registrierungsantrag und die Prüfung, die E-Mail-Bestätigung, die Rollen und bis zu 20 Maßnahmen und Meldungen, die neuesten zuerst. `GET /api/v1/admin/reports?target_account_id={id}` blättert durch alle Meldungen über ein Konto.
- `POST /api/v1/admin/accounts/{id}/reveal` liefert die E-Mail-Adresse eines lokalen Kontos, die IP-Adressen und User-Agents von Registrierung und Anmeldungen (20 Einträge, die neuesten zuerst) und andere Konten, die dieselbe IP-Adresse verwendet haben (bis zu 20 je IP-Adresse).
  `GET /api/v1/admin/accounts/{id}/sign_ins` und `GET /api/v1/admin/accounts/{id}/same_ip_accounts?ip=` blättern durch alle davon.
  Jeder Aufruf dieser drei hält `revealed_private_data` im Audit-Log fest, und die Daten werden nur geliefert, wenn der Eintrag geschrieben werden konnte. IP-Adressen werden nach der Aufbewahrungsfrist (standardmäßig 180 Tage) entfernt, sodass ältere Herkünfte von Anmeldungen nicht gezeigt werden können.
- `two_factor_enabled` in `GET /api/v1/admin/accounts/{id}` zeigt, ob der Nutzer eines lokalen Kontos die Zwei-Faktor-Authentifizierung (TOTP) verwendet.
  Ein Nutzer, der sowohl die Authenticator-App als auch die Wiederherstellungscodes verloren hat, kann sich allein mit dem Passwort anmelden, sobald du die Zwei-Faktor-Authentifizierung mit `POST /api/v1/admin/accounts/{id}/two_factor/disable` entfernst.
  Das können nur Nutzer mit der Berechtigung `manage_login_recovery`, und standardmäßig hat nur die eingebaute Administrator-Rolle sie (sie gehört zur Serveradministration; der Web-Client zeigt den Abschnitt nur ihnen).
  Das Entfernen hält `two_factor_disabled` im Audit-Log fest. Überprüfe auf anderem Weg, etwa per E-Mail, dass die Person die Inhaberin des Kontos ist, bevor du sie entfernst.
  Wenn ein Administrator, neben dem es keinen anderen Administrator gibt, sie verloren hat, entfernt der Betreiber sie mit dem CLI-Befehl `my admin account disable-two-factor @alice` (der Akteur im Audit-Log ist die CLI).
- Auf einem Server, der Mail versendet, erhält ein Nutzer, der das Passwort vergessen hat, über „Passwort vergessen?“ auf der Anmeldeseite einen Link per E-Mail, um ein neues festzulegen.
  Bei einem Server, der keine Mail versendet (`mail = none`), oder einem Nutzer, den die Mail nicht erreicht, stellt ein Administrator den Link aus und übergibt ihn.
  Stelle ihn in den Admin-Bildschirmen mit „Passwort zurücksetzen“ auf der Verwaltungsseite des Kontos (`/moderation/accounts/{id}/manage`, `POST /api/v1/admin/accounts/{id}/password_reset_link`) aus.
  Nur Nutzer mit derselben Berechtigung wie für das Entfernen der Zwei-Faktor-Authentifizierung, `manage_login_recovery`, können ihn ausstellen, und standardmäßig hat nur die eingebaute Administrator-Rolle sie (wer den Link besitzt, kann in das Konto gelangen, das `manage_accounts` eines Moderators reicht also nicht).
  Der ausgestellte Link wird einmal an Ort und Stelle gezeigt und lässt sich kopieren. Der Server speichert nur einen Hash des Links, du kannst ihn also später nicht sehen; stelle ihn bei Bedarf erneut aus.
  Wenn kein Administrator verfügbar ist oder die Admin-Bildschirme es nicht sind, stellt der Betreiber ihn mit dem CLI-Befehl `my admin account password-reset-link @alice` aus (der Akteur im Audit-Log ist die CLI).
  Solange die [Anmeldung auf Login-Anbieter beschränkt ist](login-providers.md#limit-sign-in-to-login-providers), stellen beide den Link weiterhin aus und sagen, dass er erst funktioniert, wenn die Beschränkung endet.
  Der Link gilt 1 Stunde und einmal und wird ungültig, wenn das Passwort danach auf anderem Weg geändert wird. Beide Arten des Ausstellens halten `password_reset_link_issued` im Audit-Log fest.
  Jeder, der den Link besitzt, kann das Passwort festlegen. Überprüfe die Person also auf anderem Weg, etwa per E-Mail, und gib den Link nur ihr.
  Das Festlegen des neuen Passworts widerruft alle Sitzungen und App-Token des Nutzers. Passkeys und die Zwei-Faktor-Authentifizierung bleiben.
- `email_suppression` in `GET /api/v1/admin/accounts/{id}` zeigt, dass das Senden an die Hauptadresse eines lokalen Kontos gestoppt ist.
  Ein `hard_bounce` (das Ziel existiert nicht) stoppt alle Mails außer der Bestätigungsmail, und eine `complaint` (als Spam gemeldet) stoppt nur Benachrichtigungs- und Ankündigungsmails. Auch Mails zum Zurücksetzen des Passworts werden gestoppt, stelle also bei Bedarf den Link oben aus und übergib ihn.
  Wenn der Nutzer eine Bestätigungsmail anfordert und die Adresse erneut bestätigt, wird die Sperre nach einem Hard Bounce aufgehoben.
  Wenn du überprüft hast, dass das Senden in Ordnung ist, etwa weil der Nutzer das Postfach repariert hat, hebst du sie mit `POST /api/v1/admin/accounts/{id}/email_suppression/lift` auf (Berechtigung `manage_accounts`). Das hält `email_suppression_lifted` im Audit-Log fest.
- Eine Suche liest bis zu 400 Konten, um eine Seite zu füllen. Wenn wenige Konten passen, kann selbst eine kurze oder leere Seite `next_cursor` tragen, lies also weiter.

<!-- source: domain-actions d71386f1cd2d -->
## Domain-Maßnahmen

| Art | Wirkung |
| --- | --- |
| `suspend` | Stoppt jede Zustellung, jeden Abruf und jeden Empfang (der Empfang ergibt `403`, bevor die Signatur geprüft wird). Bestehende Konten und Beiträge werden aus Timelines und Profilen verborgen. Trennt Follows zwischen lokalen Nutzern und den Konten der Domain in beide Richtungen (der anderen Seite wird nichts gesendet) |
| `limit` | Hält Beiträge aus den öffentlichen Timelines und der Suche heraus und erzeugt keine Benachrichtigungen für Nutzer, die dem Konto nicht folgen |
| `reject_media` | Ruft keine Medien ab |
| `reject_reports` | Verwirft Meldungen (`Flag`) von der Domain |
| `force_sensitive` | Zeigt Medien als sensibel |

```sh
my admin domain issue -kind suspend -public-comment "spam" spam.example
my admin domain issue -kind limit -duration 720h noisy.example
my admin domain list -active
my admin domain lift <action id>
```

In der Admin-API: `POST /api/v1/admin/domain_actions` (der Body hat `domain`, `kind`, `public_comment`, `private_note`, `obfuscate` und `expires_in` in Sekunden),
`GET /api/v1/admin/domain_actions` (filtere mit `domain` und `active`), `GET .../{id}`, `POST .../{id}/lift`
und `GET .../impact?domain=`, das die Follows zählt, die ein `suspend` der Domain beenden würde.

Öffne im Web-Client Moderation → Domain-Maßnahmen (`/moderation/domain-actions`). Die Liste zeigt für jede Maßnahme Domain, öffentlichen Grund, Art, Ende und Zustand, wirksam oder alle, von jeder Domain oder von der, nach der du suchst;
„Neue Maßnahme“ öffnet das Formular (`/moderation/domain-actions/new`), und die Wahl einer Domain öffnet die Seite der Maßnahme mit ihren Gründen und Daten, wo „Aufheben“ sie beendet.
Ein `suspend` fragt zuerst nach: Die Bestätigung sagt, wie viele Follows zwischen lokalen Nutzern und den Konten der Domain es endgültig beendet, und bittet dich, die Domain einzutippen.
Der Link „Domain-Maßnahmen“ auf der Seite eines entfernten Servers listet nur die Maßnahmen dieser Domain, und „Neue Maßnahme“ startet dort das Formular mit ausgefüllter Domain.

- Domains werden normalisiert zum IDNA-A-Label festgehalten (Kleinbuchstaben, kein abschließender Punkt). `例え.jp` und `xn--r8jz45g.jp` sind dieselbe Domain.
- Eine Maßnahme gilt auch für Subdomains (eine Maßnahme gegen `spam.example` gilt auch für `a.spam.example`).
- Eine Maßnahme wird in der Transaktion, die sie festhält, auf die Einschränkungs-Projektion (`domain_restrictions`) und auf die Timeline-Einschränkungen angewendet und wirkt ab der nächsten Zustellung, dem nächsten Abruf oder Empfang.
- Eine Maßnahme mit Endzeit (`-duration` / `expires_in`, höchstens zehn Jahre) endet zu dieser Zeit von selbst. Ablauf ist kein „Aufheben“, Listen zeigen ihn daher als `expired`.
- Das Aufheben entfernt nur die Einschränkungen dieser Maßnahme. Andere Maßnahmen gegen dieselbe Domain bleiben. Das Aufheben einer bereits aufgehobenen Maßnahme ändert nichts.
- Listen zeigen die neuesten zuerst, und `-active` (`active=true`) zeigt nur die wirksamen Maßnahmen.

<!-- source: importing-and-exporting-mastodon-csv f9b0965f8f91 -->
### Mastodon-CSV importieren und exportieren

Du kannst die Liste der Domain-Maßnahmen im CSV-Format der Domain-Sperren von Mastodon (`#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate`) mit Mastodon austauschen.
Du kannst eine CSV importieren, die Mastodon exportiert hat, und Mastodon kann eine CSV importieren, die dieser Server exportiert hat.

```sh
my admin domain export > domain_blocks.csv
my admin domain import domain_blocks.csv                       # only shows what it would do
my admin domain import -apply -only spam.example,bad.example domain_blocks.csv
```

In der Admin-API: `GET /api/v1/admin/domain_actions/export` (`text/csv`), `POST .../import/preview` (der Body hat `csv`) und `POST .../import` (der Body hat `blocks`, die gewählten Zeilen, und `private_note`).
Eine Zeile für die eigene Domain dieses Servers wird übersprungen (`own_domain`), und eine Domain-Maßnahme gegen sie wird abgelehnt.
Öffne im Web-Client Moderation → Domain-Maßnahmen und wähle „CSV von Mastodon“ (`/moderation/domain-actions/import`): „Als CSV speichern“ speichert die Datei, und der Import zeigt eine gewählte Datei vorab an, bevor er die Zeilen importiert, die du behältst; ein `suspend` darunter fragt zuerst nach.

| CSV-Zeile | Maßnahme auf diesem Server |
| --- | --- |
| `#severity` ist `suspend` (oder leer) | `suspend` |
| `#severity` ist `silence` | `limit` |
| `#severity` ist `noop` | Keine |
| `#reject_media` / `#reject_reports` ist wahr | Fügt `reject_media` bzw. `reject_reports` hinzu |

- Ein Import hat zwei Schritte. Prüfe zuerst in der Vorschau, welche Maßnahmen jede Zeile ergreifen würde, wähle die Zeilen, die du behältst, und importiere dann (die CLI ändert nichts, bis du `-apply` hinzufügst, und `-only` wählt Zeilen nach Domain).
- Eine Zeile, deren Domain oder eine übergeordnete Domain schon eine wirksame Maßnahme derselben Art hat, ergreift diese Art nicht (ist jede Art schon da, gilt sie als „schon angewendet“). Eine zweite Zeile für dieselbe Domain, eine ungültige Zeile und eine Zeile, die nichts ergreifen würde (nur `noop`), werden ebenfalls übersprungen. Dieselbe CSV erneut zu importieren fügt keine Maßnahmen hinzu.
- Eine CSV ohne `#domain`-Kopfzeile wird als Liste mit einer Domain pro Zeile gelesen, und jede Domain erhält `suspend` (wie bei Mastodon). Ein Import nimmt bis zu 5.000 Zeilen.
- Importierte Maßnahmen gelten „bis zur Aufhebung“ und tragen den öffentlichen Kommentar und die Verschleierung der Zeile sowie die private Notiz, die du angegeben hast. Sie erscheinen im Audit-Log wie einzeln ergriffene Maßnahmen (der Akteur ist bei der API die Person und bei der CLI `cli`).
- Ein Export schreibt eine Zeile pro Domain für die wirksamen Maßnahmen. Eine Domain mit einem `suspend` erhält `suspend`, eine mit einem `limit` erhält `silence`, und eine mit keinem von beiden erhält `noop`. Eine Domain mit nur `force_sensitive`, das die Domain-Sperren von Mastodon nicht haben, wird nicht exportiert. Private Notizen und Endzeiten werden nicht exportiert.

<!-- source: subscribing-to-blocklists d49a93c3c44f -->
### Blocklisten abonnieren

Statt eine CSV von Hand zu importieren, kannst du eine Domain-Blockliste abonnieren, die ein anderer Server oder eine andere Person unter einer
`https`-URL veröffentlicht. Der Server ruft die Liste gleich nach dem Abonnieren ab, danach jeden Tag und wenn du es verlangst, und macht
aus ihren Zeilen Domain-Maßnahmen: als Entwürfe, die ein Moderator annimmt oder ablehnt (der Modus `draft`, der Standard), oder direkt bei
jedem Abruf (der Modus `automatic`).

```sh
my admin domain subscription add -format mastodon_csv -title "Shared blocklist" https://lists.example/blocklist.csv
my admin domain subscription list
my admin domain subscription drafts <subscription id>
my admin domain subscription accept -keys issue:suspend:spam.example <subscription id>
my admin domain subscription reject -all <subscription id>
my admin domain subscription update -mode automatic <subscription id>
my admin domain subscription fetch <subscription id>               # fetch now and print the outcome
my admin domain subscription fetches <subscription id>
my admin domain subscription remove [-lift-actions] <subscription id>
```

In der Admin-API: `GET` und `POST /api/v1/admin/domain_list_subscriptions`, `GET`, `PATCH` und `DELETE .../{id}`
(`lift_actions=true` hebt die Maßnahmen des Abonnements auf), `POST .../{id}/fetch`, `GET .../{id}/fetches`, `GET .../{id}/drafts`
und `POST .../{id}/drafts/accept` und `.../drafts/reject` (der Body hat `keys` oder nichts für jeden Entwurf). Alle brauchen
`manage_domains`.

Öffne im Web-Client Moderation → Domain-Maßnahmen und wähle „Blocklisten-Abonnements“
(`/moderation/domain-actions/subscriptions`). Die Liste zeigt Modus und letzten Abruf jedes Abonnements, mit den Fehlern in
Folge; „Abonnement hinzufügen“ öffnet das Formular für den Namen, die URL, das Format und die Einstellungen unten, und das Abonnieren öffnet die
Seite des Abonnements. Diese Seite hat vier Abschnitte: die Entwürfe, die du einzeln oder alle zusammen annimmst oder ablehnst (das Annehmen von
Entwürfen, die eine Domain sperren, fragt zuerst nach); die Abrufe, mit „Jetzt abrufen“ und dem Ergebnis jedes Abrufs; die Einstellungen; und „Nicht mehr
abonnieren“, das fragt, ob die Maßnahmen aufgehoben werden sollen, die das Abonnement ausgelöst hat. Die Benachrichtigung `admin_domain_list_failing`
verlinkt auf die Liste.

| Format | Was die Liste ist |
| --- | --- |
| `mastodon_csv` | Die Domain-Block-CSV von Mastodon, so gelesen, wie ein Import sie liest |
| `mastodon_json` | Die Antwort von `GET /api/v1/instance/domain_blocks` eines Mastodon-Servers, der seine Domain-Blockierungen veröffentlicht. Domains, die er verschleiert (mit `*`), werden übersprungen, und ein Server, der die Liste nicht veröffentlicht (`401`, `403`, `404`), lässt den Abruf fehlschlagen |
| `plain_text` | Eine Domain pro Zeile, jede ein `suspend` |

Was eine Zeile auslöst, folgt derselben Tabelle wie ein Import, verändert durch die Einstellungen des Abonnements: `suspend_as` (`suspend`,
`limit` oder `ignore`; Standard `suspend`), `silence_as` (`limit` oder `ignore`; Standard `limit`), ob `reject_media` und
`reject_reports` der Zeile gelten (standardmäßig beide) und ob die Maßnahmen den öffentlichen Kommentar der Zeile übernehmen (standardmäßig
nicht).

- Ein Abruf läuft durch denselben Schutz wie die Abrufe anderer Server: Eine URL oder eine Weiterleitung, die zu einer privaten Adresse führt,
  schlägt fehl. Er liest höchstens 2 MiB und 5.000 Zeilen; eine größere Liste lässt den Abruf fehlschlagen. Der Server sendet den `ETag` und das
  `Last-Modified` der Liste zurück, sodass eine unveränderte Liste wenig kostet, und eine Liste, die mit der zuletzt gelesenen identisch ist, ändert nichts.
- Ein Abruf, der fehlschlägt, ändert keine Maßnahme. Er wird mit dem Grund des Fehlschlagens erfasst (`http_status`, `network`,
  `forbidden_destination`, `too_large`, `too_many_entries` oder `malformed`) und in der Liste der Abonnements und ihren
  Abrufen gezeigt. Nach 3 Fehlern in Folge erhalten die Leute, deren Rolle `manage_domains` hat, die Benachrichtigung
  `admin_domain_list_failing`.
- Ein Abonnement hebt eine Maßnahme, die es nicht ausgelöst hat, nie auf und ersetzt sie nie. Deckt bereits eine Maßnahme derselben Art die Domain ab,
  auf ihr oder auf einer übergeordneten Domain, wer auch immer sie ausgelöst hat, löst das Abonnement dort nichts aus. Führen zwei Abonnements dieselbe
  Domain auf, ist die Maßnahme des Abonnements in Kraft, das sie zuerst ausgelöst hat; hebt dieses sie auf, löst das nächste seine eigene aus.
- Lässt die Liste eine Domain fallen, hebt der automatische Modus die Maßnahme, die das Abonnement dafür ausgelöst hat, beim nächsten Abruf auf. Würde ein
  Abruf mehr als 10 Maßnahmen und mehr als die Hälfte der Maßnahmen des Abonnements aufheben, warten die Aufhebungen stattdessen als Entwürfe, da eine
  kaputte oder leere Liste so aussieht.
- Eine Schwere, die der Server nicht kennt (weder `suspend`, `silence` noch `noop`), wartet immer als Entwurf, als ein `suspend`
  gemäß den Einstellungen des Abonnements.
- Entwürfe listen auf, was die Liste hinzufügen (`issue`) und entfernen würde (`lift`), jeweils mit einem Schlüssel wie `issue:suspend:spam.example`.
  Ein abgelehnter Entwurf kommt nicht wieder, bis die Liste die Zeile der Domain ändert (ein `issue`) oder die Maßnahme endet (ein `lift`).
- Hebt ein Moderator eine Maßnahme auf, die ein Abonnement ausgelöst hat, löst das Abonnement sie nicht erneut aus, bis die Liste
  die Zeile der Domain ändert.
- Das Ändern der Einstellungen wendet sie bei dem Abruf an, den die Änderung startet. Die URL und das Format ändern sich nicht; abonniere
  erneut, um sie zu ändern.
- Das Entfernen eines Abonnements behält die Maßnahmen, die es ausgelöst hat, die dann als die eines Moderators zählen, es sei denn, du hebst sie damit auf.
- Das Audit-Log erfasst das Abonnieren, Ändern und Entfernen, die angenommenen und abgelehnten Entwürfe und die von Hand verlangten Abrufe.
  Die Maßnahmen des automatischen Modus werden wie andere Domain-Maßnahmen mit `system` als Akteur erfasst und als `domain_moderation_action.created` und `.lifted`
  an die Webhooks gesendet, deren `data` die `subscription_id` hat.

<!-- source: severed-follows 51be435d8635 -->
### Getrennte Follows

Ein Domain-`suspend` und ein `suspend` eines entfernten Kontos trennen Follows zwischen lokalen Nutzern und den gesperrten Konten in beide Richtungen.

- Bei einem Domain-`suspend` trennt ein Job (`moderation.sever_domain`), der läuft, nachdem die Maßnahme festgehalten wurde, sie einen lokalen Nutzer nach dem anderen. Bis der Job fertig ist, erscheinen die Follows noch in Listen. Konten auf Subdomains werden ebenfalls getrennt.
- Bei einem `suspend` eines entfernten Kontos werden die Follows in der Transaktion getrennt, die die Maßnahme festhält. Ein zweites `suspend` eines schon gesperrten Kontos trennt nichts.
- Jeder Nutzer, der bestätigte Follows verloren hat, erhält eine Benachrichtigung (`relationships_severed`), die sagt, wie viele Follower und Follows verloren gingen. Ein Nutzer, der nur Folgeanfragen verloren hat, erhält keine.
- Das Aufheben der Maßnahme bringt die getrennten Follows nicht zurück. Nutzer folgen erneut.
- Dieselbe Maßnahme erneut zu verarbeiten (zum Beispiel, wenn ein Job erneut läuft) fügt keine weiteren Einträge getrennter Beziehungen oder Benachrichtigungen hinzu.

<!-- source: federating-only-with-allowed-servers 84209876856e -->
### Nur mit erlaubten Servern föderieren

Die Instanzeinstellung `federation_scope` bestimmt, mit welchen Servern dieser Server föderiert: `open` (der Standard) föderiert mit jedem Server, den keine Domain-Maßnahme sperrt,
und `allowlist` nur mit den Domains auf der Allowlist und ihren Subdomains, für eine Organisation oder einen persönlichen Server, der nur mit Servern spricht, die er kennt.

Solange `federation_scope` `allowlist` ist:

- Ein Server ohne Erlaubnis wird wie eine gesperrte Domain behandelt: Seine Zustellungen ergeben `403`, bevor ihre Signaturen geprüft werden, nichts wird an ihn zugestellt oder von ihm abgerufen, und Relays bringen nichts von ihm herein.
- Eine Erlaubnis deckt ihre Subdomains ab: Erlaubt du `friend.example`, ist auch `social.friend.example` erlaubt.
- Die Domain-Maßnahmen gelten weiterhin für die erlaubten Server: Eine erlaubte Domain mit einem `suspend` bleibt gesperrt, und eine mit einem `limit` oder `reject_media` bleibt eingeschränkt.
- ActivityPub-GETs brauchen eine Signatur von einem erlaubten Server, was auch immer `authorized_fetch` sagt, damit ein Server ohne Erlaubnis die öffentlichen Beiträge nicht lesen kann. Server und Crawler, die ihre GETs nicht signieren, erhalten `401`.
- NodeInfo nennt `metadata.federationScope`, damit andere Server es erkennen können.

Solange sie `open` ist, ändert die Allowlist nichts und wird für das nächste Umschalten aufbewahrt.

```sh
my admin domain allow add -comment "partner" friend.example
my admin domain allow list
my admin domain allow remove friend.example
my admin federation scope                   # prints the setting
my admin federation scope allowlist         # shows what the switch would end, and changes nothing
my admin federation scope -apply allowlist  # switches
my admin federation scope open
```

In der Admin-API: `GET /api/v1/admin/domain_allows`, `POST /api/v1/admin/domain_allows` (der Body hat `domain` und `comment`; eine bereits erlaubte Domain antwortet mit `200` und behält ihre Erlaubnis) und `DELETE /api/v1/admin/domain_allows/{domain}`, mit der Berechtigung `manage_domains`.
`GET /api/v1/admin/settings/federation_scope/impact` zählt, was das Umschalten beenden würde, mit der Berechtigung `manage_settings`, und `PATCH /api/v1/admin/settings` ändert `federation_scope` wie jede andere Einstellung.
Die Allowlist enthält höchstens 5.000 Domains und ein Kommentar, der nie veröffentlicht wird, höchstens 5.000 Zeichen. Die eigene Domain dieses Servers kann nicht erlaubt werden.

Das Umschalten auf `allowlist` und das Entfernen einer Erlaubnis, während sie aktiv ist, beendet Follows endgültig:

- Bevor du umschaltest, lies, wie viele genehmigte Follows zwischen lokalen Nutzern und den Servern ohne Erlaubnis das Umschalten beenden würde, wie viele lokale Nutzer eines verlieren und welche Server die meisten davon haben (die Auswirkung oben oder `my admin federation scope allowlist` ohne `-apply`).
- Das Umschalten trennt die Follows mit jedem Server, den die Allowlist nicht erlaubt, in beide Richtungen und ohne diese Server zu benachrichtigen, wie es ein `suspend` einer Domain tut: Ein Job (`moderation.sever_allowlist`) trennt sie einen lokalen Nutzer nach dem anderen, und jeder Nutzer, der genehmigte Follows verloren hat, erhält eine Benachrichtigung (`relationships_severed`) mit den Zahlen.
- Das Entfernen einer Erlaubnis, während `federation_scope` `allowlist` ist, trennt auf dieselbe Weise die Follows mit den Konten dieser Domain und ihrer Subdomains, die keine andere Erlaubnis abdeckt. Das Entfernen der Erlaubnis einer Subdomain, deren übergeordnete Domain erlaubt ist, trennt nichts.
- Das Zurückschalten auf `open` oder das erneute Erlauben der Domain bringt die getrennten Follows nicht zurück. Nutzer folgen erneut.
- `allowlist` zu setzen, wenn es schon `allowlist` ist, trennt nichts weiter.

Öffne im Web-Client Moderation → Domain-Maßnahmen und wähle „Erlaubte Domains“ (`/moderation/domain-actions/allowed`), um Erlaubnisse zu lesen,
hinzuzufügen und zu entfernen; das Entfernen fragt zuerst nach, da es die Domain abtrennt, solange `federation_scope` `allowlist` ist. Die
Einstellung selbst ist „Server“ auf der Seite Föderation der Servereinstellungen (`/admin/settings/federation`, mit der
Berechtigung `manage_settings`): Wählst du „Nur erlaubte Server“ und speicherst, zeigt es die Follows, die das Umschalten beenden würde, die lokalen
Konten, die eines verlieren, und die Server mit den meisten davon, und speichert erst, wenn du bestätigst.

Das Hinzufügen und Entfernen einer Erlaubnis stehen im Audit-Log (`domain_allow_added` und `domain_allow_removed`), und das Umschalten ist das `setting_changed` von `federation_scope`.

<!-- source: publishing-the-domains-you-act-on e8402b3a2c93 -->
### Die Domains veröffentlichen, gegen die du Maßnahmen ergreifst

Du kannst die wirksamen Maßnahmen `limit` und `suspend` über `GET /api/v1/instance/domain_actions` veröffentlichen.
Die Instanzeinstellung `domain_actions_visibility` (`off` / `signed_in` / `everyone`, Standard `off`) bestimmt, wer die Liste sieht, und
`domain_actions_rationale_visibility` (dieselben Werte, Standard `off`) bestimmt, wer die öffentlichen Kommentare sieht ([Instanzeinstellungen](instance-settings.md)).

```sh
my instance set domain_actions_visibility signed_in
my instance set domain_actions_rationale_visibility signed_in
```

- Bei `off` liefert die API `404`, und bei `signed_in` liefert sie `401` auf eine Anfrage ohne Zugriffstoken.
- Es gibt einen Eintrag pro Maßnahme, die neuesten zuerst. Ein Eintrag hat die Domain, den SHA-256 der Domain (hex) und die Art sowie den öffentlichen Kommentar für Leute, die ihn sehen dürfen. Private Notizen werden nie geliefert.
- Bei einer Maßnahme mit `-obfuscate` (`obfuscate`) wird die mittlere Hälfte der Zeichen der Domain (außer Punkten) durch `*` ersetzt (`bad.example` wird zu `bad.*****le`).
  Jeder kann den SHA-256 einer Domain, die er kennt, vergleichen, um zu prüfen, ob es eine verschleierte ist.
- Maßnahmen `reject_media`, `reject_reports` und `force_sensitive` sowie aufgehobene oder abgelaufene Maßnahmen werden nicht veröffentlicht.
- Die Informationsseite der Instanz im Web-Client (`https://<host>/about`) zeigt diese Liste unter denselben Einstellungen. Bei `off` wird der Abschnitt nicht gezeigt, und bei `signed_in` sehen nicht angemeldete Leute einen Hinweis, dass nur angemeldete Nutzer sie sehen können.

<!-- source: refetching-remote-posts 883740b40c9a -->
## Entfernte Beiträge erneut abrufen

Du kannst gespeicherte entfernte Beiträge erneut vom anderen Server abrufen.
Ein erneuter Abruf geschieht nur, wenn du ihn über die Admin-API, die CLI oder den Web-Client anforderst, nie periodisch.

```sh
my admin refetch status <post ID or URI>
my admin refetch account @bob@remote.example
my admin refetch domain remote.example
```

In der Admin-API: `POST /api/v1/admin/statuses/{id}/refetch`, `POST /api/v1/admin/accounts/{id}/refetch_statuses` und `POST /api/v1/admin/instances/{domain}/refetch_statuses`. Sie brauchen die Berechtigung `manage_domains`.
Verwende im Web-Client „Beiträge erneut abrufen“ auf der Prüfseite des Kontos (`/moderation/accounts/{id}`, entfernte Konten) und dem Screen des entfernten Servers (`/moderation/instances/{domain}`).

- Ein Beitrag wird wie ein empfangenes `Update` behandelt. Gibt es eine neuere Version, fügt der Server eine Version hinzu; er aktualisiert die Stimmenzahlen von Umfragen; und er tut nichts, wenn sich nichts geändert hat. Ein Beitrag, für den der andere Server 410 Gone oder einen `Tombstone` liefert, wird gelöscht. Ein Beitrag, der sich nicht abrufen lässt (404 eingeschlossen), bleibt, wie er ist.
- Bei einem Konto ruft der Server bis zu 1.000 seiner neuesten ungelöschten gespeicherten Beiträge erneut ab, 100 auf einmal, im Abstand von einer Minute.
- Bei einer Domain ruft der Server von den neuesten Konten der Domain erneut ab, bis zu den 20 neuesten Beiträgen pro Konto und bis zu 1.000 Beiträgen insgesamt, 5 Konten auf einmal, im Abstand von einer Minute.
- Er ruft nicht von einer Domain mit einem `suspend` ab und auch nicht von einer Domain, die `unavailable` ist, weil Zustellungen fehlgeschlagen sind. Ein fehlgeschlagener Abruf derselben URI wird 1 Stunde lang gemerkt, und ein erneuter Abruf in dieser Zeit ruft sie nicht ab.
- Deine Anforderung wird im Audit-Log als `status_refetch_queued`, `account_statuses_refetch_queued` oder `domain_statuses_refetch_queued` festgehalten.

<!-- source: checking-what-reached-a-remote-server 6c1ec9c8cef1 -->
## Prüfen, was einen entfernten Server erreicht hat

Wenn ein Nutzer fragt, ob ein Beitrag, ein Follow oder ein Accept einen anderen Server erreicht hat, liste die Zustellungen an diesen Server mit der Admin-API auf.
Sie zeigt jede Zustellung einer Aktivität an eine Inbox des Servers, die neueste zuerst, mit jedem Versuch und dem, was der Server geantwortet hat, die erfolgreichen Zustellungen eingeschlossen.
Sie braucht die Berechtigung `manage_domains`.
Im Web-Client zeigt der Abschnitt **Zustellungen** des Bildschirms eines entfernten Servers (`/moderation/instances/{domain}`) dieselbe Liste, mit ihren Filtern nach **Aktivitäts-ID** und **Lokalem Konto** und **Anzeigen** für jedes verborgene Feld unter **Details** einer maskierten Zustellung; der Bildschirm sagt, welche davon im Audit-Log festgehalten werden, bevor sie laufen.
Die Liste Entfernte Server zeigt nur die Server, die Zustellungen nicht erreicht haben. Um einen anderen Server zu öffnen, gib seine Domain, ein Handle auf ihm oder eine URL auf ihm unter **Server öffnen** über der Liste ein
oder nutze **Server des Kontos öffnen** auf der Prüfungsseite eines entfernten Kontos oder bei einer Meldung über ein entferntes Konto oder die Domain in einem Audit-Log-Eintrag über eine Domain.

```sh
curl -H "Authorization: Bearer $TOKEN" \
  "https://example.com/api/v1/admin/instances/misskey.io/deliveries"
```

- Jede Zustellung hat den Aktivitätstyp (`Create`, `Follow`, `Accept` und so weiter), wann die Aktivität festgehalten und wann die Inbox hinzugefügt wurde, und ihre `attempts`, die ältesten zuerst.
  Jeder Versuch hat sein `outcome` (`succeeded`, `retryable_failure`, `permanent_failure`, `skipped`), das `signature_scheme` und den `status_code` der Antwort, den `error` eines Fehlschlags ohne Antwort oder warum nichts gesendet wurde, und die Zeit.
  Ein Versuch `succeeded` mit `202` bedeutet, dass der andere Server die Aktivität angenommen hat; ob er sie dann anzeigt, liegt bei diesem Server.
- Versuche werden 30 Tage aufbewahrt, der Verlauf reicht also 30 Tage zurück.
- Um die Liste einzugrenzen, füge `activity_uri=<the activity's ID>` hinzu oder `account_id=<a local account's ID>` für die Zustellungen eines Kontos.
  Eine Abfrage mit `account_id` wird im Audit-Log als `account_deliveries_viewed` festgehalten, mit dem Konto und dem Server; die anderen Abfragen werden nicht festgehalten.
- Der Body der Aktivität wird nie gezeigt.

Manche Zustellungen sind maskiert: Ihr Absender, ihre Aktivitäts-URI und ihre Inbox werden weggelassen, und `mask` sagt, warum.

| `mask` | Die Aktivität betrifft |
| --- | --- |
| `private`, `direct`, `local` | einen Beitrag mit dieser Sichtbarkeit |
| `approves_followers` | ein Follow, sein Undo, ein Accept oder ein Reject eines lokalen Kontos, das seine Follower genehmigt |
| `unknown` | einen Beitrag oder ein Konto, die weg sind, sodass nicht mehr bekannt ist, was es war |

Eine gemeinsame Inbox nennt keinen Empfänger und wird immer gezeigt; eine maskierte persönliche Inbox im Fehler eines Versuchs lautet `[masked]`.
Der Typ, die Zeiten und jeder Versuch einer maskierten Zustellung werden gezeigt, du kannst also trotzdem sehen, ob sie den Server erreicht hat.
Um die maskierten Felder einer Zustellung zu sehen, nutze `POST /api/v1/admin/instances/{domain}/deliveries/{id}/reveal`; jeder Aufruf wird im Audit-Log als `delivery_revealed` festgehalten, bevor etwas gezeigt wird.

<!-- source: reports 5791186f6b24 -->
## Meldungen

Von anderen Servern empfangene Meldungen (`Flag`) kommen als offen (`open`) in die Meldungsliste.

```sh
my admin report list -state open
my admin report show <report id>
my admin report assign <report id> <username>
my admin report unassign <report id>
my admin report resolve -outcome dismissed <report id>
my admin report note <report id> "Same person as an earlier report"
my admin report reopen <report id>
my admin report forward -comment "Spam account, see the linked posts" <report id>
```

Im Web-Client listet **Moderation → Meldungen** (`/moderation/reports`) die offenen Meldungen, die neueste zuerst, mit wer sie eingereicht hat, ihrer Kategorie, wann und wer sie bearbeitet.
Ihre Filter (Zustand, Kategorie und wer sie bearbeitet: du oder niemand) und die Reihenfolge (die Spalte **Eingereicht**: die neueste oder die am längsten wartende zuerst) bleiben in der Adresse der Seite, und **Zurück** und **Weiter** blättern durch die Seiten.
Die Seite einer Meldung (`/moderation/reports/{id}`) zeigt ihren Zustand, ihre Kategorie, die angeführten Regeln (eine inzwischen zurückgezogene Regel sagt das), wer sie eingereicht hat und wer sie bearbeitet, ob sie an den Server des Kontos ging und wie viele andere Meldungen und Maßnahmen das Konto hat;
dann das Konto; den Kommentar; die gemeldeten Beiträge, einen gelöschten mit dem Zeitpunkt seiner Löschung; was im Sprachchat-Raum geschah oder die Änderungen der Veranstaltung; und die Notizen.
Ab `wide` steht die Seite der Liste, von der du kamst, daneben. **Mir zuweisen** und **Zuweisung aufheben**, **Abschließen** (**Maßnahme ergriffen** oder **Keine Maßnahme nötig**) und **Wieder öffnen** stehen unter ihrem Titel, **Notiz hinzufügen** unter den Notizen.

In der Admin-API: `GET /api/v1/admin/reports` (filtere mit `state`, `category` und `assignee`: `me`, `none` oder die ID eines lokalen Kontos; `order=oldest` listet statt der neuesten die am längsten wartende Meldung zuerst auf), `GET .../{id}`, `POST .../{id}/assign_to_self`, `POST .../{id}/unassign`,
`POST .../{id}/resolve` (Body `{"outcome": "actioned" | "dismissed"}`), `POST .../{id}/notes` (Body `{"body": "..."}`), `POST .../{id}/reopen` und `POST .../{id}/forward` (Body `{"comment": "..."}`, optional).

- Eine Zuweisung (`assign`) kann nur an einen lokalen Nutzer gehen, dessen Rolle `manage_reports` hat. In der Admin-API weist du dir selbst zu.
- Das Ergebnis eines Abschlusses (`resolve`) ist `actioned` (gegen das Ziel wurde eine Maßnahme ergriffen) oder `dismissed` (es war keine Maßnahme nötig). Eine abgeschlossene Meldung erneut abzuschließen ändert das Ergebnis nicht.
- Wenn du aus einer Meldung eine Maßnahme ergreifst (`report_id` / `-report` der Maßnahme), wird die Meldung im selben Vorgang als `actioned` abgeschlossen.
- Eine Notiz (`note`) ist eine interne Notiz, die nur Moderatoren sehen, bis zu 2.000 Zeichen. Sie wird weder dem Ziel noch dem Melder gezeigt. Sie lässt sich weder bearbeiten noch löschen.
- Das Wiedereröffnen (`reopen`) setzt eine abgeschlossene Meldung zurück auf `open` (die Zuweisung bleibt). Eine offene Meldung lässt sich nicht wieder öffnen (`409` in der Admin-API).
- Das Weiterleiten (`forward`, **An deren Server senden …** am Ende der Seite der Meldung, was zuerst nachfragt) sendet eine Meldung über ein Konto auf einem anderen Server als `Flag` deines Instanz-Actors an diesen Server, mit dem Konto und den gemeldeten Beiträgen, die noch existieren. Der einzige Text, den es trägt, ist das, was du beim Weiterleiten für die Moderatoren dieses Servers schreibst (`-comment`, `comment` in der Admin-API oder **Text für deren Moderatoren** auf der Seite der Meldung; bis zu 1.000 Zeichen, standardmäßig keiner). Der Kommentar des Melders und die Notiz einer [externen Prüfung](#reports-that-an-external-review-filed) verlassen deinen Server auf diesem Weg nie, und nichts vom Melder wird gesendet. Eine Meldung, die ihr Melder beim Einreichen zum Weiterleiten gewählt hat, ging sofort hinaus, mit dem Kommentar des Melders. Das Weiterleiten funktioniert bei einer Meldung, die auf diesem Server eingereicht wurde, von einer Person oder einer [externen Prüfung](#reports-that-an-external-review-filed), offen oder abgeschlossen, die noch nicht weitergeleitet wurde; eine von einem anderen Server empfangene Meldung oder eine über einen Sprachchat-Raum oder eine Veranstaltung lässt sich nicht weiterleiten (`409` `report-not-forwardable` in der Admin-API, und die Details der Meldung zeigen keine Schaltfläche). Eine Meldung wird einmal gesendet: Sie erneut weiterzuleiten, mit beliebigem Text, oder eine weiterzuleiten, die ihr Melder beim Einreichen weitergeleitet hatte, ändert nichts. `forwarded` in der Admin-API sagt, ob sie gesendet wurde, `forwardable`, ob du sie jetzt senden kannst.
- Die Details der Meldung (`show`, `GET .../{id}`) liefern die Notizen, die ältesten zuerst, mit der Zahl der anderen Meldungen über das Ziel und der Zahl der Maßnahmen gegen das Ziel (aufgehobene und abgelaufene eingeschlossen). Die Liste liefert keine Notizen.
- Zuweisungen, Abschlüsse, Wiedereröffnungen, Notizen und Weiterleitungen werden als Ereignisse festgehalten (`report_assignments`, `report_resolutions`, `report_reopenings`, `report_notes`, `report_forwards`), und `report_states` hält den aktuellen Zustand.
- Moderatoren können die gemeldeten Beiträge und den Kommentar des Melders auf der Seite der Meldung übersetzen oder mit `POST .../{id}/statuses/{status_id}/translation` und `POST .../{id}/comment/translation` (Body `{"target_language": "en"}`).
  Der Text geht nur an einen Übersetzungsanbieter, solange die [Instanzeinstellung](instance-settings.md) `translation_reports_to_server` an ist, und dann nur an einen Anbieter, dem die Sichtbarkeit des Beitrags erlaubt ist: Der Zugriff eines Moderators erweitert nichts, ein direkter Beitrag geht also nur an einen Anbieter, dem `direct` erlaubt ist, und der Kommentar zählt als `private`.
  Die Übersetzung des Kommentars wird nicht aufbewahrt, die eines gelöschten Beitrags ebenso wenig, und die Zeichen zählen nur auf das monatliche Limit des Servers. Die eingebaute Übersetzung des Browsers, wenn `translation_browser` an ist, wirkt auf die gemeldeten Beiträge, was auch immer `translation_reports_to_server` sagt; der Kommentar, der keine Sprache nennt, wird nur vom Server übersetzt.
- Ein Melder sieht vom Ziel nur das, was er sonst vom Konto sehen würde: Solange es gesperrt ist oder ihn blockt, bleibt seine Meldung ohne das Konto gelistet. Moderatoren sehen immer das ganze Ziel.
- Ein lokaler Melder erhält bei der ersten Bearbeitung eine Benachrichtigung (`report_resolved`). Wiedereröffnen und erneutes Abschließen fügt keine Benachrichtigung hinzu.
  Ein Melder kann seine eigenen Meldungen mit `GET /api/v1/reports` auflisten und sieht nur, ob jede offen oder abgeschlossen ist. Das Ergebnis, die ergriffene Maßnahme, der dem Ziel genannte Grund und der Zuständige werden nicht gezeigt.

<!-- source: reports-that-an-external-review-filed 8563536b46e4 -->
### Meldungen, die eine externe Prüfung eingereicht hat

Ein [Decision Hook](decision-hooks.md), der mit `flag` antwortet, reicht eine Meldung mit deinem Server-Actor als Melder ein. Die Meldung
sagt das: `hook_origin` in der Admin-API und im `report.created`-Webhook, eine Zeile in den Meldungsdetails und `automated` in
`my admin report show`. Ihr Kommentar ist die Notiz des Plugins, und ihre Kategorie stammt aus der Begründung. Sie wird beim
Einreichen nicht weitergeleitet, und ihr Abschluss benachrichtigt niemanden. Wenn das Ziel auf einem anderen Server liegt, kannst du sie
selbst weiterleiten (`forward`): Die Notiz des Plugins bleibt auf deinem Server, und nur der Text, den du schreibst, geht hinaus. Das Abbrechen der Entscheidung
lässt die Meldung, wie sie ist.

<!-- source: voice-chat-rooms d2dd76116ac4 -->
### Sprachchat-Räume

- Ein lokaler Nutzer kann einen offenen Sprachchat-Raum melden, den er sehen kann (`POST /api/v1/reports` mit `voice_room_id` und `account_id` auf den Host des Raums gesetzt).
  Die Meldung richtet sich gegen den Host zu diesem Zeitpunkt und verweist auf die Einstellungen des Raums (Name und Beschreibung), wie sie damals waren. Sie verweist auf keine Beiträge und wird nie an einen anderen Server weitergeleitet.
- Die Details der Meldung (`GET /api/v1/admin/reports/{id}`) zeigen den Raum (`voice_room`): jede Änderung seiner Einstellungen, wer teilgenommen und wann verlassen hat, wer daraus entfernt wurde und wie er endete.
  Die Aufzeichnungen darüber, wer teilgenommen hat, werden 90 Tage nach dem Ende des Raums (`voice_participants_retention_days`) aufbewahrt und so lange, wie eine Meldung auf den Raum verweist ([Aufbewahrung von Aufzeichnungen](#record-retention-and-holds)).
  Es gibt keine Aufnahmen und keinen Textchat, diese Aufzeichnungen sind also der einzige Beleg.
- Wer `manage_reports` hat, kann einen offenen Raum für alle Teilnehmer mit `POST /api/v1/admin/voice_rooms/{id}/end` beenden (Body `{"reason": "..."}`, optional, bis zu 500 Zeichen).
  Der Grund ist nur für die Moderatoren; die Teilnehmer sehen nur, dass der Raum beendet wurde. Es funktioniert auch, während der Sprachchat aus ist, und das Audit-Log hält `voice_room_ended` fest.
  Im Web-Client hat die Seite der Meldung eines offenen Raums „Raum beenden“, was zuerst nachfragt, mit dem Grund.

<!-- source: events cbc3e19bfdb6 -->
### Veranstaltungen

- Ein lokaler Nutzer kann eine Veranstaltung melden, die er sehen kann, auch eine abgesagte (`POST /api/v1/reports` mit `event_id` und `account_id` auf den Organisator der Veranstaltung gesetzt).
  Die Meldung richtet sich gegen den Organisator und verweist auf die Einstellungen der Veranstaltung (Titel, Beschreibung und Orte), wie der Melder sie sah. Eine Veranstaltung, für die noch ein Termin abgestimmt wird, kann wie jede andere gemeldet und abgesagt werden. Sie verweist auf keine Beiträge und wird nie an einen anderen Server weitergeleitet.
- Die Details der Meldung (`GET /api/v1/admin/reports/{id}`) zeigen die Veranstaltung (`event`): ihren Termin, ob und wie sie abgesagt wurde und jede Änderung ihrer Einstellungen, die älteste zuerst, mit allen ihren Orten, auch denen, die nur die genehmigten Teilnehmer sahen.
  `reported_revision_id` ist die Version, die der Melder sah, ein nach der Meldung geänderter Titel oder eine geänderte Adresse liegt dir also weiterhin vor.
- Wer `manage_reports` hat, kann eine Veranstaltung mit `POST /api/v1/admin/events/{id}/cancel` absagen (Body `{"reason": "..."}`, optional, bis zu 500 Zeichen).
  Der Grund wird mit der Absage allen gezeigt, die die Veranstaltung sehen. Die Nutzer, die mit „Ich komme“ oder „Vielleicht“ geantwortet haben, und die eingeladenen Nutzer werden benachrichtigt, wie bei der eigenen Absage des Organisators, und das Audit-Log hält `event_cancelled` fest.
  Eine bereits abgesagte Veranstaltung antwortet mit `409` `event-closed`. Im Web-Client hat die Seite der Meldung einer Veranstaltung „Veranstaltung absagen“, was zuerst nachfragt, mit dem Grund.
- Das Sperren oder Löschen des Organisators sagt die Veranstaltungen des Organisators ab, die noch nicht begonnen haben, auch die, für die noch ein Termin abgestimmt wird (`gathering.cancel_organized`), und benachrichtigt ihre Teilnehmer. Diese Absagen nennen niemanden als den, der sie abgesagt hat, und das Aufheben der Sperre stellt sie nicht wieder her. Bereits begonnene Veranstaltungen werden nicht abgesagt.

<!-- source: notifications-for-moderators-and-the-pending-counts 0d1c327abe83 -->
## Benachrichtigungen für Moderatoren und die ausstehenden Zähler

Neue Meldungen, Registrierungen, die auf Genehmigung warten, und Einsprüche erreichen Leute mit der Berechtigung als Benachrichtigungen in der App.

| Benachrichtigung | Wann sie eintrifft | Wer sie erhält |
| --- | --- | --- |
| `admin_report` | Wenn das Ziel der Meldung keine andere offene Meldung hat (eine zweite oder spätere Meldung über dasselbe Konto benachrichtigt nicht) | Leute, deren Rolle `manage_reports` hat, außer dem Melder und dem Ziel der Meldung |
| `admin_sign_up` | Wenn eine Registrierung als auf Genehmigung wartend beginnt (Genehmigungsmodus oder `open` bei einem Setup, das keine Mail senden kann) | Leute, deren Rolle `manage_accounts` hat |
| `admin_appeal` | Wenn gegen eine Maßnahme Einspruch eingelegt wird | Leute, deren Rolle `manage_accounts` hat, außer der Person, die Einspruch eingelegt hat |
| `admin_custom_emoji_request` | Wenn das Erstellen eines eigenen Emojis oder das Ersetzen seines Bildes beantragt wird | Leute, deren Rolle `manage_custom_emojis` hat, außer dem Antragsteller |
| `admin_error_group` | Wenn der Server oder der Web-Client einen Fehler einer neuen Art meldet oder eine Fehlergruppe, die gelöst war, auf einer anderen Version des Servers erneut auftritt | Leute, deren Rolle `manage_errors` hat |
| `admin_domain_list_failing` | Wenn der Abruf einer abonnierten Blockliste zum dritten Mal hintereinander fehlschlägt | Leute, deren Rolle `manage_domains` hat |

- `admin_error_group` trifft höchstens 10 Mal pro Stunde für den ganzen Server ein. Der Rest der Stunde wird in einer Benachrichtigung zusammengefasst,
  die sagt, wie viele weitere Fehlergruppen es gab. Eine stummgeschaltete Gruppe benachrichtigt nie. Ein Fehler, den der Web-Client für einen
  nicht angemeldeten Besucher meldet, benachrichtigt erst, wenn auch der Browser eines angemeldeten Nutzers ihn meldet, und sein erneutes Auftreten benachrichtigt niemanden.
  Die Fehlergruppen werden über `GET /api/v1/admin/error_groups` und
  `POST /api/v1/admin/error_groups/{id}/resolve` (`/mute`, `/unmute`) gelesen und gelöst, stummgeschaltet oder wieder aktiviert, was `manage_errors` braucht.
- Wer eine Benachrichtigung erhält, wird durch seine Berechtigung beim Erstellen der Benachrichtigung bestimmt. Wer die Berechtigung später verliert, sieht diese Art nicht mehr in seiner Liste und seinem Ungelesen-Zähler. Kommt die Berechtigung zurück, sieht er sie wieder.
- Jede Art von Benachrichtigung oben kommt auch per E-Mail, standardmäßig an. Jede Person schaltet jede Art in der App und per
  E-Mail auf der Seite **Benachrichtigungen** der Einstellungen (`/settings/notifications` im Web-Client) an oder aus, die nur die
  Arten listet, die ihre Rolle erhält. Eine in der App ausgeschaltete Art wird für sie nicht erstellt, sie wird also weder gepusht noch gemailt; die Zähler von
  `GET /api/v1/admin/counts` ändern sich nicht, weil sie die Meldungen, Registrierungen und Einsprüche selbst zählen.
  Mit den offenen Meldungen liefern die Zähler auch, wann die älteste offene Meldung eingereicht wurde (`oldest_open_report_filed_at`).
  Der Server reiht den E-Mail-Job in der Transaktion ein, die die Benachrichtigung festhält, und der Job sendet die Mail direkt nach dem
  Commit, ohne zu warten, bis die Benachrichtigung gelesen wurde. Hat die Person bis zum Senden die Berechtigung verloren oder die E-Mail dieser Art
  ausgeschaltet, oder ist ihre Hauptadresse nicht bestätigt oder sind ihre E-Mails gestoppt, wird nichts gesendet.
  Die Mail enthält nur die Art der Benachrichtigung und einen Link zum Benachrichtigungsbildschirm und nichts vom Inhalt der Meldung oder des Kontos. Ihr Abmeldelink schaltet nur die E-Mail dieser Art aus.
  Mit `mail = none` wird keine Mail gesendet, und `sends_email` in `GET /api/v1/instance` ist `false`.

`GET /api/v1/admin/counts` liefert die Zahl der offenen Meldungen (`open_reports`, braucht `manage_reports`), die Zahl der Registrierungen, die auf Genehmigung warten (`pending_registrations`), und die Zahl der Einsprüche, die auf Prüfung warten (`pending_appeals`) (beide brauchen `manage_accounts`), die Zahl der Anfragen für eigene Emojis, die auf eine Entscheidung warten (`pending_custom_emoji_requests`, braucht `manage_custom_emojis`), und die Zahl der offenen Fehlergruppen, neu, wieder aufgetreten oder wieder aktiviert (`open_error_groups`, braucht `manage_errors`).
Dinge, für die dir die Berechtigung fehlt, werden nicht geliefert.

<!-- source: audit-log 4d53a2539898 -->
## Audit-Log

Operationen von Moderatoren und Administratoren werden je eine Zeile im Audit-Log (`audit_log_entries`) in derselben Transaktion wie die Operation festgehalten.
Fehlgeschlagene Operationen und Operationen, die nichts geändert haben, werden nicht festgehalten: das Aufheben einer bereits aufgehobenen Maßnahme, das Abschließen einer abgeschlossenen Meldung, das Zuweisen an denselben Zuständigen, das Zurückziehen einer bereits zurückgezogenen Regel, das Verschieben an dieselbe Position, das Vergeben einer Rolle, die der Nutzer schon hat, und das Entziehen einer Rolle, die der Nutzer nicht hat.

| Zielart | Operationen |
| --- | --- |
| `account` | `account_action_issued` und `account_action_lifted` (Ergreifen und Aufheben einer Maßnahme gegen ein Konto), `account_deletion_canceled` (das Löschen abgebrochen, weil ein Einspruch gegen einen `ban` aufgehoben wurde), `revealed_private_data` (IP-Adressen und E-Mail-Adressen offengelegt), `two_factor_disabled` (Zwei-Faktor-Authentifizierung entfernt), `password_reset_link_issued` (ein Link zum Zurücksetzen des Passworts ausgestellt), `email_suppression_lifted` (Mailsperre aufgehoben), `account_moved` (ein eingeschränktes lokales Konto zu einem lokalen Konto umgezogen; Einschränkungen werden nicht zum Ziel mitgenommen, du entscheidest also, ob du sie dort anwendest), `account_statuses_refetch_queued` (erneuter Abruf der Beiträge eines entfernten Kontos), `account_deliveries_viewed` (die Zustellungen eines lokalen Kontos an einen Server aufgelistet; der Zielname ist das Handle und der Server) |
| `status` | `status_refetch_queued` (erneuter Abruf eines entfernten Beitrags; der Zielname ist die URI des Beitrags) |
| `domain` | `domain_action_issued` und `domain_action_lifted` (Ergreifen und Aufheben einer Domain-Maßnahme), `domain_statuses_refetch_queued` (erneuter Abruf von Beiträgen der Konten der Domain), `instance_marked_available` (die Fehlschläge eines Servers gelöscht), `delivery_revealed` (der maskierte Absender, die Aktivitäts-URI und die Inbox einer Zustellung an den Server gezeigt) |
| `report` | `report_assigned`, `report_unassigned`, `report_resolved`, `report_reopened`, `report_note_added`, `report_forwarded` (Zuweisen, Aufheben der Zuweisung, Abschließen und Wiedereröffnen einer Meldung, Hinzufügen einer Notiz und Weiterleiten) |
| `user` | `registration_approved`, `registration_rejected` (Registrierungsprüfung) |
| `setting` | `setting_changed` (Instanzeinstellungen), `text_changed` (Instanztexte) |
| `rule` | `rule_added`, `rule_revised`, `rule_moved`, `rule_retired` (Instanzregeln) |
| `role` | `role_granted`, `role_revoked` (Vergeben und Entziehen einer Rolle für einen Nutzer), `role_created`, `role_updated`, `role_deleted` (Erstellen, Ändern und Löschen einer Rolle) |
| `appeal` | `appeal_upheld`, `appeal_overturned` (Prüfung eines Einspruchs; das Aufheben hält auch `account_action_lifted` für die Maßnahme fest und `account_deletion_canceled` für einen `ban`) |
| `signup_block` | `signup_block_added`, `signup_block_lifted` (Hinzufügen und Aufheben einer Registrierungssperre) |
| `hook_decision` | `hook_decision_applied` (der Server handelte nach dem Urteil einer externen Prüfung und verließ den Zustand; der Akteur ist das System), `hook_decision_overridden` (ein Moderator hob die Entscheidung auf, auch durch das Aufheben ihres Einspruchs), `hook_appeal_upheld` (ein Moderator bestätigte den Einspruch gegen einen abgelehnten Beitrag, und die Entscheidung bleibt) |
| `custom_emoji` | `custom_emoji_created`, `custom_emoji_image_replaced`, `custom_emoji_copied`, `custom_emoji_updated`, `custom_emoji_enabled`, `custom_emoji_disabled`, `custom_emoji_deleted` (Registrieren eines eigenen Emojis, Ersetzen seines Bildes, Kopieren eines Emojis von einem anderen Server, Ändern, Aktivieren, Deaktivieren und Löschen) |
| `webhook` | `webhook_created`, `webhook_updated`, `webhook_deleted`, `webhook_enabled`, `webhook_disabled`, `webhook_secret_rotated` (Erstellen, Ändern, Löschen, Aktivieren und Deaktivieren eines Webhooks und Neuerzeugen seines Geheimnisses) |
| `announcement` | `announcement_created`, `announcement_updated`, `announcement_published`, `announcement_withdrawn`, `announcement_deleted` (Erstellen, Ändern, Veröffentlichen und Zurückziehen einer Ankündigung und Löschen eines Entwurfs) |
| `invite` | `invite_created`, `invite_revoked` (Erstellen und Widerrufen einer Einladung; der Zielname sind die ersten 4 Zeichen des Codes) |
| `billing_source` | `billing_import_queued` (ein Billing-Import wurde von Hand gestartet; der Zielname ist der Name des Anbieters) |
| `cost_item` | `cost_item_created`, `cost_item_relabeled`, `cost_item_price_revised`, `cost_item_end_changed`, `cost_item_deleted` (Registrieren einer Kostenposition, Ändern ihres Namens und dergleichen, Überarbeiten ihres Betrags, Ändern ihres Enddatums und Löschen; der Zielname ist der Name der Position) |
| `supporter_program` | `supporter_program_enabled`, `supporter_program_disabled` (Aktivieren und Deaktivieren der Abrechnung; der Zielname ist der Name des Zahlungsanbieters) und `supporter_tips_enabled`, `supporter_tips_revised`, `supporter_tips_disabled` (Aktivieren der Trinkgelder, Ändern ihrer Beträge und Deaktivieren; der Zielname ist die Währung und die Beträge in kleinsten Einheiten) |
| `supporter_plan` | `supporter_plan_created`, `supporter_plan_revised`, `supporter_plan_retired` (Erstellen, Ändern und Zurückziehen eines Unterstützer-Plans; der Zielname ist der Name des Plans) |
| `supporter_subscription` | `supporter_subscription_ended` (ein Administrator beendete ein Abonnement sofort; der Zielname ist der Name des Plans und das Handle des Nutzers) |
| `custom_emoji_request` | `custom_emoji_request_submitted`, `custom_emoji_request_withdrawn`, `custom_emoji_request_approved`, `custom_emoji_request_rejected` (Einreichen, Zurückziehen, Genehmigen und Ablehnen einer Anfrage für ein eigenes Emoji; der Zielname ist der `:shortcode:`) |
| `payment_provider` | `payment_reconcile_queued` (der Abgleich mit dem Zahlungsanbieter wurde von Hand gestartet; der Zielname ist der Name des Anbieters) |
| `oauth_client` | `oauth_client_disabled`, `oauth_client_enabled` (Deaktivieren und Aktivieren eines OAuth-Clients; das Deaktivieren widerruft alle seine Tokens; der Zielname ist der Name des Clients und `client_id`) |
| `oauth_client_host` | `oauth_client_host_denied`, `oauth_client_host_allowed` (Verweigern und Aufheben der Verweigerung eines Hosts von Client-ID-Metadatendokumenten; das Verweigern widerruft die Tokens der Clients dieses Hosts; der Zielname ist der Host) |
| `instance` | `service_closure_announced`, `service_closure_withdrawn`, `service_closure_closed`, `service_closure_delivery_completed` (Ankündigen der Schließung des Dienstes oder Ändern des Datums, Zurückziehen der Ankündigung, Schließen und Abschluss der Zustellung der Schließung; der Zielname ist `service_closure`) |
| `voice_room` | `voice_room_ended` (ein Moderator beendete einen offenen Sprachchat-Raum; der Zielname ist die Raum-ID, da der Name verschwindet, wenn das Konto, das ihn schrieb, gelöscht wird; die Details der Meldung zeigen den Namen) |
| `event` | `event_cancelled` (ein Moderator sagte eine Veranstaltung ab; der Zielname ist die Veranstaltungs-ID, da der Titel verschwindet, wenn das Konto, das ihn schrieb, gelöscht wird; die Details der Meldung zeigen den Titel) |
| `domain_allow` | `domain_allow_added`, `domain_allow_removed` (Hinzufügen einer Domain zur Allowlist der Föderation und Entfernen; siehe [nur mit erlaubten Servern föderieren](#federating-only-with-allowed-servers); der Zielname ist die Domain) |
| `link_card_domain` | `link_card_domain_fetch_stopped`, `link_card_domain_images_stopped`, `link_card_domain_rule_removed` (Setzen einer Regel des Link-Karten-Crawlers für eine Domain und Entfernen; siehe [Link-Karten-Domains](instance-settings.md#link-card-domains)) |
| `login_provider` | `login_provider_created`, `login_provider_updated`, `login_provider_deleted`, `login_provider_enabled`, `login_provider_disabled`, `login_provider_secret_rotated` (Hinzufügen, Ändern, Löschen, Aktivieren und Deaktivieren eines externen Login-Anbieters und Ersetzen seines Client-Secrets; der Zielname ist der Anzeigename des Anbieters) |
| `domain_list_subscription` | `domain_list_subscription_created`, `domain_list_subscription_updated`, `domain_list_subscription_deleted` (Abonnieren einer Blockliste, Ändern ihrer Einstellungen und Entfernen), `domain_list_draft_accepted` und `domain_list_draft_rejected` (Annehmen eines Entwurfs, was auch die Domain-Maßnahme festhält, und Ablehnen eines), `domain_list_fetch_queued` (ein von Hand angeforderter Abruf); der Zielname ist der Titel des Abonnements; siehe [Blocklisten abonnieren](#subscribing-to-blocklists) |

Die Liste zeigt, ob das Ziel noch existiert. Konten, Meldungen, Einsprüche und Registrierungssperren, die nach ihrer Aufbewahrungsfrist verschwunden sind, entfernte Erlaubnisse und gelöschte Rollen, eigene Emojis und Webhooks zeigen nur den Namen zum Zeitpunkt der Aufzeichnung und keinen Link zu einem Bildschirm.

```sh
my admin audit list
my admin audit list -actor alice -action domain_action_issued
my admin audit list -target-kind domain -target-id spam.example
```

In der Admin-API: `GET /api/v1/admin/audit_log` (filtere mit `actor_account_id`, `action`, `target_kind` und `target_id`). Sie braucht die Berechtigung `view_audit`.
Öffne im Web-Client Moderation → Audit-Log (`/moderation/audit-log`): Jeder Eintrag liest sich als Satz, wer was mit was getan hat, die neuesten zuerst, die Person und das Ziel mit den Bildschirmen verlinkt.
Filtere nach dem, der es getan hat (du oder die Person eines Eintrags unter „Weitere Operationen dieser Person“), nach der Art des Ziels und dann der Operation und nach einem Ziel (unter „Weitere Operationen darauf“ eines Eintrags).

- Eine Zeile ist der Akteur (der Nutzer bei `user` oder `cli` bei der CLI), die Operation, die Zielart und -ID, der Anzeigename des Ziels, die ID des ursprünglichen Datensatzes und die Zeit.
  Die Ziel-ID ist die ID des Kontos, der Meldung, des Einspruchs, des Nutzers, der Regel, der Rolle oder der Registrierungssperre; bei einer Domain-Maßnahme ist es die Domain; bei Einstellungen und Texten der Schlüssel.
- Der Anzeigename des Ziels (`@alice`, `@bob@example.com`, eine Domain, ein Einstellungsschlüssel, der Text einer Regel; bei einem Text sein Schlüssel, bei einer Rolle ihr Schlüssel und das Handle des Nutzers, bei einer Registrierungssperre ihre Art und ihr Wert) ist eine zum Zeitpunkt der Aufzeichnung gemachte Kopie, sodass er auch lesbar bleibt, wenn das Ziel weg ist.
  Jede Zeile der API liefert auch, ob das Ziel noch existiert (`target_exists`), was für Konten, Meldungen, Einsprüche und Registrierungssperren, die weg sind, `false` ist. Das Audit-Log im Web zeigt für ein Ziel, das weg ist, nur den Anzeigenamen, ohne Link.
- Der Text von Gründen und Notizen wird nicht kopiert. Schlage ihn über die ID des ursprünglichen Datensatzes nach (die Maßnahme, Zuweisung, Abschluss, Wiedereröffnung, Notiz, Prüfung oder die Version einer Einstellung oder Regel).
- Die Liste zeigt die neuesten zuerst mit Keyset-Paging (`-before <entry id>` in der CLI).
- Es gibt keine API und keine CLI, um das Audit-Log zu ändern oder zu löschen. Alte Zeilen werden nach der [Aufbewahrungsfrist](#record-retention-and-holds) entfernt.

<!-- source: record-retention-and-holds 9861d8570a91 -->
## Aufbewahrung von Aufzeichnungen und Sperren

Nach dem Aufbewahrungszeitraum für jede Art von Datensatz entfernen stündliche Jobs (`moderation.purge_records`, `identity.purge_origins`, `voice.purge_records` und `gathering.purge_records`) Moderationsdatensätze.
Du kannst die Zeiträume in den Instanzeinstellungen ändern, aber nicht unter die Untergrenze ([Instanzeinstellungen](instance-settings.md)). Ein geänderter Zeitraum wirkt ab dem nächsten Lauf. Eine aufgehobene Aufbewahrungssperre mit ihrem Grund wird entfernt, sobald seit ihrer Aufhebung der Aufbewahrungszeitraum für Moderationsmaßnahmen vergangen ist.

| Datensatz | Einstellung | Standard | Untergrenze | Beginnt mit |
| --- | --- | --- | --- | --- |
| IP-Adresse und User-Agent bei der Anmeldung | `session_origins_retention_days` | 180 Tage | 90 Tage | Der Anmeldung |
| IP-Adresse bei der Registrierung | `registration_origins_retention_days` | 180 Tage | 90 Tage | Der Registrierung |
| Meldungen (Kommentar, Zielbeiträge, Notizen, Zuweisungen, Erledigungen, Wiedereröffnungen) | `reports_retention_days` | 365 Tage | 180 Tage | Der letzten Erledigung |
| Maßnahmen gegen Konten (Grund, Notiz, Zielbeiträge, Aufhebung) und Einsprüche | `moderation_actions_retention_days` | 365 Tage | 180 Tage | Dem Ende der Maßnahme |
| Audit-Log | `audit_log_retention_days` | 730 Tage | 365 Tage | Der Erfassung |
| Registrierungssperren | Keine (nicht änderbar) | 30 Tage | n/a | Dem früheren Zeitpunkt von Aufhebung und Ablauf |
| Wer an einem Sprachchat-Raum teilgenommen hat (Teilnahmen, Sprecher und Zuhörer, gehobene Hände, Einladungen, Entfernungen) | `voice_participants_retention_days` | 90 Tage | 30 Tage | Dem Ende des Raums |
| Die Entscheidungen der [Entscheidungs-Hooks](decision-hooks.md#the-decisions) | `hook_decisions_retention_days` | 90 Tage | 30 Tage | Der Entscheidung |
| Die Antworten, Genehmigungen, Einladungen und Stimmen zu den Terminvorschlägen einer Veranstaltung | `event_records_retention_days` | 90 Tage | 30 Tage | Dem früheren Zeitpunkt von Ende der Veranstaltung (bei einer Veranstaltung, deren Datum nie festgelegt wurde, dem Ende ihres letzten Terminvorschlags) und ihrer Absage |

- Offene Meldungen werden nie entfernt. Eine Meldung, auf die eine Maßnahme verweist (`report_id` der Maßnahme), bleibt, bis diese Maßnahme entfernt wird.
- Das Ende einer Maßnahme ist der frühere Zeitpunkt von Aufhebung und Ablauf. Kommt bei einer Maßnahme, gegen die Einspruch eingelegt wurde, die Prüfung später als das, ist das Ende der Zeitpunkt der Prüfung, und die Maßnahme wird behalten, solange der Einspruch auf Prüfung wartet.
  `warn` und `delete_statuses` haben keine dauerhafte Wirkung, daher ist das Ende der Zeitpunkt, zu dem sie ergriffen wurden. Eine Maßnahme ohne Ende (ein `ban`, ein `suspend` ohne Endzeit und so weiter) wird nicht entfernt, bis sie aufgehoben wird.
- Beiträge, auf die eine Meldung oder eine Maßnahme verweist, werden vom Purge-Job mit ihren angehängten Medien nicht physisch gelöscht, bis diese Meldung oder Maßnahme entfernt wird, auch wenn der Autor sie gelöscht hat (sie verschwinden aus der Ansicht). Entfernte Beiträge bleiben auch über den Aufbewahrungszeitraum hinaus (einschließlich der Instanzeinstellung `remote_status_max_retention_days`).
- Eine Registrierungssperre, die weder eine Aufhebung noch eine Endzeit hat, bleibt, solange sie in Kraft ist.
- Die Datensätze darüber, wer an einem Sprachchat-Raum teilgenommen hat, auf den eine Meldung verweist, bleiben, bis diese Meldung entfernt wird. Der Raum selbst (sein Name, seine Beschreibung, seine Hosts und wie er endete) wird nie entfernt.
- Die Antworten, Stimmen und Einladungen einer Veranstaltung, auf die eine Meldung verweist, bleiben, bis diese Meldung entfernt wird, ebenso die, die ein Konto unter einer Sperre nennen. Die Veranstaltung selbst (ihre Einstellungen und Orte, ihr Datum und ihre Terminvorschläge, ihre Co-Veranstalter und ihre Absage) wird nie entfernt; sobald ihre Antworten weg sind, zählt sie niemanden als Zusage.
- Ein Job-Lauf entfernt nur eine begrenzte Zahl von Zeilen pro Art (100 auf einmal, 20 Mal, bei Maßnahmen, Meldungen und Registrierungssperren; 1.000 Zeilen auf einmal, 20 Mal, beim Audit-Log). Der nächste Lauf entfernt den Rest.

<!-- source: holds f3bf57c2c258 -->
### Sperren

Solange eine gerichtliche Anordnung das Löschen verbietet (Japans Information Distribution Platform Act, Artikel 16) oder eine Auskunftsanfrage bearbeitet wird, kannst du das Löschen pro Konto stoppen.

```sh
my admin retention hold -reason "court order 2026-123" @alice
my admin retention list -account @alice
my admin retention release <hold id>
```

- Bei einem Konto unter einer Sperre werden seine Maßnahmen und Einsprüche, die Meldungen über es und die Meldungen von ihm, deren Audit-Logs, die IP-Adressen von Anmeldung und Registrierung, seine Beiträge und die Aufzeichnungen der Sprachchat-Räume, an denen es teilgenommen oder in denen es gehandelt hat, nach der Aufbewahrungsfrist nicht entfernt.
  Eine Registrierungssperre, die aus dem Ban dieses Kontos entstanden ist, wird ebenfalls über 30 Tage nach Aufheben oder Ablauf hinaus behalten.
- Du kannst auf ein Konto mehrere Sperren legen. Wenn du alle aufhebst, werden Aufzeichnungen über ihrer Frist beim nächsten Lauf entfernt. Das Aufheben einer schon aufgehobenen Sperre ändert nichts.
- Sperren und ihre Aufhebung erfolgen nur mit der CLI, und der Akteur in der Aufzeichnung ist die CLI.
- Die Daten eines gelöschten Kontos (eines, dessen `ban` sein Datum erreicht hat, eines, das der Nutzer gelöscht hat, oder eines entfernten Kontos, das sein Server gelöscht hat) werden 7 Tage nach dem Löschen vom stündlichen Job `account.erase_deleted` gelöscht. Die Beiträge eines entfernten Kontos gehen ebenfalls, auch die, die lokale Nutzer mit einem Lesezeichen versehen oder beantwortet haben.
  Ein entferntes Konto, das nur als gelöscht markiert wurde, weil ein Abruf mit `410 Gone` antwortete, ohne ein eigenes signiertes `Delete`, wird zuerst noch einmal abgerufen. Ein zweites `410` löscht es. Wenn sein Server es wieder ausliefert, kommt es mit seinen Beiträgen zurück, und nichts wird gelöscht. Jede andere Antwort (`404` oder keine Antwort) wartet auf einen späteren Lauf, und 30 Tage nach dem Löschen wird das Konto trotzdem gelöscht.
  Ein Konto unter einer Sperre wird nicht gelöscht. Beiträge, auf die eine Meldung oder Maßnahme verweist, bleiben, bis diese Aufzeichnung entfernt wird. Die URI und der Nutzername des Kontos und die Maßnahmenaufzeichnungen bleiben.
  Die Namen und Beschreibungen, die das Konto seinen Sprachchat-Räumen gegeben hat, werden geleert; die Räume bleiben.

<!-- source: exporting-for-disclosure-requests 89f1074dd950 -->
### Für Auskunftsanfragen exportieren

Auf eine Auskunftsanfrage (DSGVO Artikel 15, Japans Gesetz zum Schutz personenbezogener Daten Artikel 33) antwortest du, indem du alle Moderationsaufzeichnungen eines Kontos als JSON exportierst.

```sh
my admin account export-moderation @alice > alice-moderation.json
```

- Der Export enthält die Maßnahmen gegen das Konto (einschließlich privater Notizen, Aufheben, Einsprüche und Prüfungen), die Meldungen über das Konto (Kommentar, Zielbeiträge, zitierte Regeln, Notizen der Moderatoren, Zuweisungen, Abschlüsse, Wiedereröffnungen), die Meldungen, die das Konto erstellt hat (nur ob jede offen oder abgeschlossen ist), und deren Audit-Logs.
- Die Identität der Melder (das meldende Konto sowie die URI und der Ursprungsserver einer empfangenen Meldung) wird nicht exportiert. Prüfe vor der Übergabe des Exports, dass der Kommentar einer Meldung nichts sagt, was den Melder identifiziert.
- Ein Moderator wird per Nutzer-ID exportiert. Lege zuerst eine Sperre, damit Aufzeichnungen nicht entfernt werden, während du die Anfrage bearbeitest.
