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

<!-- source: moderation-and-server-administration-in-the-web-client d7c65fc5b3d0 -->
## Moderation und Serveradministration im Web-Client

Der Web-Client teilt die Bildschirme für das Personal in zwei.
Die Navigation zeigt „Moderation“ und „Administration“ nur Leuten, deren Rolle mindestens eine Berechtigung dieser Gruppe hat. Jeder Bildschirm zeigt Leuten ohne die Berechtigung nur einen Grund, und der Server lehnt auch ihre Anfragen ab.

| Gruppe | Berechtigungen | Bildschirme |
| --- | --- | --- |
| Moderation (`/moderation`) | `manage_reports`, `manage_accounts`, `manage_domains`, `manage_invites`, `view_audit` | Meldungen (`/moderation/reports`), Einsprüche (`/moderation/appeals`), Konten (`/moderation/accounts`), Registrierungen (`/moderation/registrations`), Registrierungssperren (`/moderation/signup-blocks`), Einladungen (`/moderation/invites`), Domain-Maßnahmen (`/moderation/domain-actions`), Entfernte Server (`/moderation/instances`), Audit-Log (`/moderation/audit-log`) |
| Serveradministration (`/admin`) | `manage_settings`, `manage_rules`, `manage_roles`, `manage_jobs`, `manage_login_recovery`, `manage_custom_emojis`, `manage_webhooks`, `manage_announcements`, `manage_finance`, `manage_oauth_apps`, `manage_service_closure`, `manage_errors`, `manage_login_providers` | Servereinstellungen (`/admin/settings`), Regeln (`/admin/rules`), Bedingungen und Richtlinien (`/admin/documents`), die Job-Queue und fehlgeschlagene Jobs (`/admin/jobs`), Eigene Emojis (`/admin/custom-emojis`), Ankündigungen (`/admin/announcements`), Webhooks (`/admin/webhooks`), Kosten und Billing-Import (`/admin/finance`, `manage_finance`). `manage_login_recovery` (Login-Wiederherstellung: Zwei-Faktor-Authentifizierung entfernen und einen Link zum Zurücksetzen des Passworts ausstellen) und `manage_roles` (die Rollen eines Kontos vergeben und entziehen) erscheinen als Abschnitte der Verwaltungsseite eines Kontos (`/moderation/accounts/{id}/manage`, die außerdem `manage_accounts` braucht) |

- Das Badge an „Moderation“ ist die Summe der offenen Meldungen, der Registrierungen, die auf Genehmigung warten, und der Einsprüche, die auf Prüfung warten ([Benachrichtigungen für Moderatoren und die ausstehenden Zähler](#notifications-for-moderators-and-the-pending-counts)). „Administration“ zeigt keinen Zähler.
- Jede Gruppe öffnet sich auf einer Übersicht; die Bildschirme selbst stehen im Menü daneben. Die Moderationsübersicht hat für jede Warteschlange, die deine Rolle zählt, eine Kachel (offene Meldungen, Registrierungen, die auf Genehmigung warten, Einsprüche, die auf eine Entscheidung warten), jede ein Link zu ihrer Warteschlange, eine Kachel damit, wie lange die älteste offene Meldung gewartet hat (ein Link zur Meldungsliste, die älteste zuerst), und listet die offenen Meldungen auf, die zuerst zu bearbeiten sind, aus der ganzen Warteschlange: zuerst rechtswidrige Inhalte, dann die, die am längsten gewartet haben, mit der Wartezeit jeder einzelnen und wer sie bearbeitet.
  Die Meldungsliste filtert nach Zustand, Kategorie und Zuständigem (du oder niemand) und listet die neueste oder die älteste zuerst auf; die Filter bleiben in der Adresse der Seite.
  Die Administrationsübersicht zeigt den Zustand des Servers: die fehlgeschlagenen Jobs, wie lange die Job-Queue ihren ältesten Job hat warten lassen und die offenen Fehler, jede Kachel ein Link zu ihrem Bildschirm und markiert, wenn sie Aufmerksamkeit braucht.
- Der eingebaute `moderator` hat nur `manage_reports`, `manage_accounts` und `view_audit` und sieht „Administration“ nicht. Er sieht auch die Bildschirme Domain-Maßnahmen und Entfernte Server (`manage_domains`) nicht,
  erstelle also, um diese zu delegieren, eine Rolle mit dieser Berechtigung und vergib sie ([Rollen vergeben und entziehen](admin-cli.md#granting-and-revoking-roles)).
- Die Pfade der Admin-API bleiben `/api/v1/admin/...`, unabhängig von der Gruppe, und die Scopes sind dasselbe Paar `admin:read` / `admin:write`. Die Gruppen sind in der API-Referenz durch Tags getrennt (`admin-moderation`, `admin-server`).
  Ein Dienst außerhalb des Servers kann die Operationen einer Berechtigung stattdessen mit einem Berechtigungs-Scope aufrufen ([einen anderen Dienst die Admin-API nutzen lassen](#letting-another-service-use-the-admin-api)).
- Es gibt eine dritte Gruppe von Berechtigungen, „Nutzerfunktionen“ (`view_finance`, `create_custom_emojis`, `start_voice_rooms`, `create_invites`, `keep_posts_longer`, `create_articles`, `create_events`), die außerhalb der Bildschirme für das Personal lebt. Wer nur diese Berechtigungen hat, sieht weder „Moderation“ noch „Administration“,
  und für jemanden mit `view_finance` zeigt die Navigation „Finanzen“ (`/finance`) nach den Einstellungen.
  Ihre API liegt im Nutzer-Namespace (`GET /api/v1/finance/cost_totals` und `GET /api/v1/finance/income_totals`, Scope `read:finance`) und braucht keinen `admin:*`-Scope. Was sie zeigt, wird durch `finance_viewer_*` in den [Instanzeinstellungen](instance-settings.md) festgelegt.

<!-- source: letting-another-service-use-the-admin-api 6c266c1e1111 -->
### Einen anderen Dienst die Admin-API nutzen lassen

Ein Dienst, den du neben dem Server betreibst, etwa einer, der Einladungen verteilt oder Kosten erfasst, kann die Admin-API für eine Berechtigung ohne Plugin aufrufen.

1. Registriere ein Konto für den Dienst und gib ihm eine Rolle mit nur den Berechtigungen, die der Dienst braucht (für Einladungen eine Rolle mit `manage_invites`; [Rollen vergeben und entziehen](admin-cli.md#granting-and-revoking-roles)).
2. Registriere, angemeldet als dieses Konto, in **Einstellungen → Deine Apps** eine App, deren Scopes den Berechtigungs-Scope enthalten, `admin:` und die Berechtigung: `admin:manage_invites`.
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

<!-- source: appeals 8a6f1b4c288d -->
## Einsprüche

Ein lokaler Nutzer kann gegen eine Maßnahme gegen ihn Einspruch einlegen.
Ein Moderator prüft den Einspruch und bestätigt entweder die Maßnahme (`uphold`) oder hebt sie auf (`overturn`).

```sh
my admin appeal list -state pending
my admin appeal uphold -reason "The posts were spam." <appeal id>
my admin appeal overturn -reason "Sorry, our mistake." <appeal id>
```

Der Nutzer legt Einspruch ein mit `POST /api/v1/moderation/actions/{id}/appeal` (Scope `write:accounts`, Body `{"text": "..."}`).
Ein Nutzer unter `disable_login`, `suspend` oder `ban` kann keinen Access-Token erhalten und legt Einspruch über das Formular je Maßnahme auf der Anmeldeseite ein (`POST /auth/appeal`).
Solange [die Anmeldung auf Login-Anbieter beschränkt ist](login-providers.md#limit-sign-in-to-login-providers), erreicht ein solcher Nutzer das Formular nur über einen verknüpften Anbieter.
Die Seite zeigt das Formular und die Frist für eine Maßnahme, gegen die Einspruch möglich ist, den Inhalt und das Ergebnis für eine Maßnahme, gegen die schon Einspruch eingelegt wurde, und den Grund für eine Maßnahme, deren Frist verstrichen ist oder gegen die kein Einspruch möglich ist. Bei einem abgelehnten Einspruch zeigt sie den Grund.
In der Admin-API: `GET /api/v1/admin/appeals` (filtere mit `state`), `GET .../{id}` sowie `POST .../{id}/uphold` und `POST .../{id}/overturn` (der Body `{"public_reason": "..."}` ist optional). Sie brauchen die Berechtigung `manage_accounts`.

- Gegen eine Maßnahme kann einmal Einspruch eingelegt werden, und der Text hat 1 bis 2.000 Zeichen. Die Frist ist die Zahl der Tage der Server-Einstellung `appeal_window_days` nach der Maßnahme (Standard 180 Tage, 20 bis 365; siehe [Server-Einstellungen](instance-settings.md)).
  Eine Maßnahme nach Fristablauf und ein zweiter Einspruch ergeben `422` (`/api/v1/problems/appeal-window-closed` und `/api/v1/problems/already-appealed`).
- Gegen einen `ban` kann Einspruch eingelegt werden, bis das Konto gelöscht ist (30 Tage nach der Maßnahme, oder bis zur Frist von `appeal_window_days`, falls diese kürzer ist). Ein `ban` eines gelöschten Kontos ergibt `422` (`/api/v1/problems/not-appealable`). Gegen eine aufgehobene oder abgelaufene Maßnahme kann innerhalb der Frist weiterhin Einspruch eingelegt werden.
- Wenn ein Nutzer Einspruch einlegt, erhalten Leute mit `manage_accounts` eine Benachrichtigung (`admin_appeal`). Wenn er geprüft ist, erhält der Nutzer eine Benachrichtigung (`appeal_decided`) und kann das Ergebnis und `public_reason` sehen. Der Prüfer wird nicht gezeigt.
- Das Aufheben hebt die Maßnahme in derselben Transaktion auf (dieselbe Wirkung wie `lift`, mit leerem Aufhebungsgrund). Das Aufheben von `delete_statuses` bringt die gelöschten Beiträge nicht zurück. Eine Maßnahme, die vor der Prüfung schon aufgehoben war, bleibt, wie sie ist.
- Das Aufheben eines `ban` bricht außerdem das geplante Löschen des Kontos in derselben Transaktion ab (das Audit-Log hält `account_deletion_canceled` fest), beendet die Sperre und sendet das `Update` mit dem ursprünglichen Profil an andere Server. Widerrufene Sitzungen und Token kommen nicht zurück, der Nutzer meldet sich also erneut an.
  Wurde das Konto vor der Prüfung gelöscht, kann die Maßnahme nicht aufgehoben werden (`422`, `/api/v1/problems/not-appealable`; sie zu bestätigen ist weiterhin möglich).
- Eine Prüfung geschieht einmal. Eine erneute Prüfung mit demselben Ergebnis ändert nichts, und das gegenteilige Ergebnis ergibt `409` (`/api/v1/problems/appeal-decided`).

<!-- source: account-management 99149899a198 -->
## Kontoverwaltung

Konten suchen, ihre Details ansehen und IP-Adressen und E-Mail-Adressen offenlegen geschieht über die Admin-API und braucht die Berechtigung `manage_accounts`.

Im Web-Client hat ein Konto drei Seiten, mit Links dazwischen unter dem Seitentitel:

- **Prüfung** (`/moderation/accounts/{id}`): der Status und die Einschränkungen des Kontos, das Formular für eine neue Maßnahme, die neuesten Maßnahmen dagegen mit ihren Begründungen und seine neuesten 20 Meldungen. Ältere Meldungen stehen nur in der Meldungsliste (`/moderation/reports`), die keinen Filter nach Konto hat.
- **Maßnahmen** (`/moderation/accounts/{id}/actions`): jede Maßnahme gegen das Konto, eine Seite nach der anderen, mit „Aufheben“ für die geltenden. Jede Maßnahme öffnet ihre eigene Seite (`/moderation/accounts/{id}/actions/{action id}`) mit allen Details.
- **Verwaltung** (`/moderation/accounts/{id}/manage`, nur lokale Konten): die Rollen des Kontos (`manage_roles`), seine E-Mail-Unterdrückung, das Entfernen der Zwei-Faktor-Authentifizierung und das Ausstellen eines Links zum Zurücksetzen des Passworts (`manage_login_recovery`) sowie seine E-Mail- und IP-Adressen. Jeder Abschnitt erscheint nur Leuten mit der entsprechenden Berechtigung.

Das Team handelt nur gegenüber niedrigeren Rängen: Eine Maßnahme zu ergreifen oder aufzuheben, über einen Einspruch zu entscheiden, eine E-Mail-Unterdrückung aufzuheben, private Daten offenzulegen, einen Link zum Zurücksetzen des Passworts auszustellen und die Zwei-Faktor-Authentifizierung zu entfernen verlangt, dass die höchste `position` unter deinen Rollen über der jeder Rolle des Ziels liegt.
Ein höherer oder gleicher Rang, dein eigenes Konto eingeschlossen, ergibt `403` mit dem Problem-Typ `/api/v1/problems/target-outranks-actor`, sodass ein Moderator keinen Administrator bannen kann und ein Administrator keinen anderen.
Die `my admin`-Befehle haben keine solche Grenze.
Eine Maßnahme, die den einzigen Administrator, der sich anmelden kann, an der Anmeldung hindert (`disable_login`, `suspend`, `ban`), wird abgelehnt, wer auch immer sie ergreift, und dieser Administrator kann sein eigenes Konto nicht löschen (`409` mit `/api/v1/problems/last-administrator`). Die Rollen stehen in [admin-cli.md](admin-cli.md).

- `GET /api/v1/admin/accounts` liefert lokale und entfernte Konten, neueste zuerst (der Server-Actor ist ausgeschlossen).
  Du kannst nach Nutzernamen-Präfix (`username`), Domain (`domain`), lokal oder entfernt (`origin`), Status (`state`) und, bei lokalen Konten, nach E-Mail-Adresse (`email`), E-Mail-Domain (`email_domain`) und der IP-Adresse der Registrierung oder Anmeldung (`ip`) filtern.
  Du kannst nur eines von `email`, `email_domain` und `ip` angeben. Die Liste enthält keine IP-Adressen oder E-Mail-Adressen.
- Der Status ist der erste zutreffende aus dieser Liste: `deleted`, `suspended` (`suspend`, `ban`), `disabled` (`disable_login`), `rejected` und `pending` (Registrierungsprüfung), `limited` (`limit`), `active`.
  Der Status und die Einschränkungen (`restrictions`) stammen nur aus Maßnahmen gegen das Konto selbst und enthalten keine Domain-Maßnahmen.
- `GET /api/v1/admin/accounts/{id}` liefert den Status und die Einschränkungen und, für ein lokales Konto, den Registrierungsantrag und die Prüfung, die E-Mail-Bestätigung, die Rollen und bis zu 20 Maßnahmen und Meldungen, neueste zuerst.
- `POST /api/v1/admin/accounts/{id}/reveal` liefert die E-Mail-Adresse eines lokalen Kontos, die IP-Adressen und User-Agents von Registrierung und Anmeldungen (20 Einträge, neueste zuerst) und andere Konten, die dieselbe IP-Adresse verwendet haben (bis zu 20 pro IP-Adresse).
  Jeder Aufruf hält `revealed_private_data` im Audit-Log fest, und die Daten werden nur geliefert, wenn der Eintrag geschrieben werden konnte. IP-Adressen werden nach der Aufbewahrungsfrist entfernt (standardmäßig 180 Tage), sodass ältere Anmeldeherkünfte nicht gezeigt werden können.
- `two_factor_enabled` in `GET /api/v1/admin/accounts/{id}` zeigt, ob der Nutzer eines lokalen Kontos die Zwei-Faktor-Authentifizierung (TOTP) verwendet.
  Ein Nutzer, der sowohl die Authenticator-App als auch die Wiederherstellungscodes verloren hat, kann sich allein mit dem Passwort anmelden, sobald du die Zwei-Faktor-Authentifizierung mit `POST /api/v1/admin/accounts/{id}/two_factor/disable` entfernst.
  Das können nur Nutzer mit der Berechtigung `manage_login_recovery`, und standardmäßig hat nur die eingebaute Administratorrolle sie (sie gehört zur Serveradministration; der Web-Client zeigt den Abschnitt nur ihnen).
  Das Entfernen hält `two_factor_disabled` im Audit-Log fest. Prüfe auf einem anderen Weg, etwa per E-Mail, dass die Person die Inhaberin des Kontos ist, bevor du sie entfernst.
  Wenn ein Administrator ohne weiteren Administrator sie verloren hat, entfernt der Betreiber sie mit dem CLI-Befehl `my admin account disable-two-factor @alice` (der Akteur im Audit-Log ist die CLI).
- Auf einem Server, der E-Mails sendet, erhält ein Nutzer, der das Passwort vergessen hat, über „Passwort vergessen?“ auf der Anmeldeseite einen per E-Mail gesendeten Link, um ein neues zu setzen.
  Für einen Server, der keine E-Mails sendet (`mail = none`), oder einen Nutzer, den die E-Mail nicht erreicht, stellt ein Administrator den Link aus und übergibt ihn.
  In den Admin-Screens stellst du ihn mit „Passwort zurücksetzen“ auf der Verwaltungsseite des Kontos aus (`/moderation/accounts/{id}/manage`, `POST /api/v1/admin/accounts/{id}/password_reset_link`).
  Nur Nutzer mit derselben Berechtigung wie für das Entfernen der Zwei-Faktor-Authentifizierung, `manage_login_recovery`, können ihn ausstellen, und standardmäßig hat nur die eingebaute Administratorrolle sie (wer den Link besitzt, kommt in das Konto, daher genügt das `manage_accounts` eines Moderators nicht).
  Der ausgestellte Link wird einmal an Ort und Stelle gezeigt und lässt sich kopieren. Der Server speichert nur einen Hash des Links, du kannst ihn also später nicht sehen; stelle ihn erneut aus, wenn du ihn brauchst.
  Wenn kein Administrator verfügbar ist oder die Admin-Screens nicht, stellt der Betreiber ihn mit dem CLI-Befehl `my admin account password-reset-link @alice` aus (der Akteur im Audit-Log ist die CLI).
  Solange [die Anmeldung auf Login-Anbieter beschränkt ist](login-providers.md#limit-sign-in-to-login-providers), stellen beide den Link weiterhin aus und sagen, dass er nicht funktioniert, bis die Beschränkung endet.
  Der Link funktioniert 1 Stunde lang und einmal und hört auf zu funktionieren, wenn sich das Passwort danach auf einem anderen Weg ändert. Beide Wege des Ausstellens halten `password_reset_link_issued` im Audit-Log fest.
  Jeder, der den Link besitzt, kann das Passwort setzen, prüfe die Person also auf einem anderen Weg wie E-Mail und gib den Link nur ihr.
  Das Setzen des neuen Passworts widerruft alle Sitzungen und App-Token des Nutzers. Passkeys und Zwei-Faktor-Authentifizierung bleiben.
- `email_suppression` in `GET /api/v1/admin/accounts/{id}` zeigt, dass das Senden an die primäre Adresse eines lokalen Kontos gestoppt ist.
  Ein `hard_bounce` (das Ziel existiert nicht) stoppt alle E-Mails außer der Bestätigungs-E-Mail, und ein `complaint` (als Spam gemeldet) stoppt nur Benachrichtigungs- und Ankündigungs-E-Mails. E-Mails zum Zurücksetzen des Passworts stoppen ebenfalls, stelle also den obigen Link aus und übergib ihn bei Bedarf.
  Wenn der Nutzer eine Bestätigungs-E-Mail anfordert und die Adresse erneut bestätigt, wird die Hard-Bounce-Unterdrückung aufgehoben.
  Wenn du geprüft hast, dass das Senden in Ordnung ist, etwa weil der Nutzer das Postfach repariert hat, hebst du sie mit `POST /api/v1/admin/accounts/{id}/email_suppression/lift` auf (Berechtigung `manage_accounts`). Das hält `email_suppression_lifted` im Audit-Log fest.
- Eine Suche liest bis zu 400 Konten, um eine Seite zu füllen. Wenn wenige Konten passen, kann auch eine kurze oder leere Seite `next_cursor` tragen, lies also weiter.

<!-- source: domain-actions 13dd3d4d3a0f -->
## Domain-Maßnahmen

| Art | Wirkung |
| --- | --- |
| `suspend` | Stoppt alle Zustellung, jedes Abrufen und Empfangen (das Empfangen ergibt `403`, bevor die Signatur geprüft wird). Bestehende Konten und Beiträge werden aus Timelines und Profilen verborgen. Trennt Follows zwischen lokalen Nutzern und den Konten der Domain in beide Richtungen (an die andere Seite wird nichts gesendet) |
| `limit` | Hält Beiträge aus den öffentlichen Timelines und der Suche heraus und erzeugt keine Benachrichtigungen für Nutzer, die dem Konto nicht folgen |
| `reject_media` | Ruft keine Medien ab |
| `reject_reports` | Verwirft Meldungen (`Flag`) der Domain |
| `force_sensitive` | Zeigt Medien als sensibel |

```sh
my admin domain issue -kind suspend -public-comment "spam" spam.example
my admin domain issue -kind limit -duration 720h noisy.example
my admin domain list -active
my admin domain lift <action id>
```

In der Admin-API: `POST /api/v1/admin/domain_actions` (der Body hat `domain`, `kind`, `public_comment`, `private_note`, `obfuscate` und `expires_in` in Sekunden),
`GET /api/v1/admin/domain_actions` (filtere mit `domain` und `active`), `GET .../{id}` und `POST .../{id}/lift`.

Öffne im Web-Client Moderation → Domain-Maßnahmen (`/moderation/domain-actions`). Die Liste zeigt Domain, Art, Status und Ende jeder Maßnahme, die geltenden oder alle;
„Neue Maßnahme“ öffnet das Formular (`/moderation/domain-actions/new`), und die Wahl einer Domain öffnet die Seite der Maßnahme mit ihren Kommentaren und Daten, wo „Aufheben“ sie beendet.
Der Link „Domain-Maßnahmen“ auf der Seite eines entfernten Servers listet nur die Maßnahmen dieser Domain, und „Neue Maßnahme“ startet dort das Formular mit ausgefüllter Domain.

- Domains werden normalisiert zum IDNA-A-Label festgehalten (Kleinbuchstaben, kein abschließender Punkt). `例え.jp` und `xn--r8jz45g.jp` sind dieselbe Domain.
- Eine Maßnahme gilt auch für Subdomains (eine Maßnahme gegen `spam.example` gilt auch für `a.spam.example`).
- Eine Maßnahme wird in der Transaktion, die sie festhält, auf die Einschränkungs-Projektion (`domain_restrictions`) und auf die Timeline-Einschränkungen angewendet und wirkt ab der nächsten Zustellung, dem nächsten Abruf oder Empfang.
- Eine Maßnahme mit Endzeit (`-duration` / `expires_in`, höchstens zehn Jahre) endet zu dieser Zeit von selbst. Der Ablauf ist kein „Aufheben“, daher zeigen Listen ihn als `expired`.
- Das Aufheben entfernt nur die Einschränkungen dieser Maßnahme. Andere Maßnahmen gegen dieselbe Domain bleiben. Das Aufheben einer schon aufgehobenen Maßnahme ändert nichts.
- Listen sind neueste zuerst, und `-active` (`active=true`) zeigt nur die geltenden Maßnahmen.

<!-- source: importing-and-exporting-mastodon-csv ba9ed39bd32e -->
### Mastodon-CSV importieren und exportieren

Du kannst die Liste der Domain-Maßnahmen im CSV-Format der Domain-Sperren von Mastodon (`#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate`) an Mastodon übergeben und von dort übernehmen.
Du kannst eine CSV importieren, die Mastodon exportiert hat, und Mastodon kann eine CSV importieren, die dieser Server exportiert hat.

```sh
my admin domain export > domain_blocks.csv
my admin domain import domain_blocks.csv                       # only shows what it would do
my admin domain import -apply -only spam.example,bad.example domain_blocks.csv
```

In der Admin-API: `GET /api/v1/admin/domain_actions/export` (`text/csv`), `POST .../import/preview` (der Body hat `csv`) und `POST .../import` (der Body hat `blocks`, die gewählten Zeilen, und `private_note`).
Eine Zeile für die eigene Domain dieses Servers wird übersprungen (`own_domain`), und eine Domain-Maßnahme dagegen wird abgelehnt.
Öffne im Web-Client Moderation → Domain-Maßnahmen und wähle „CSV von Mastodon“ (`/moderation/domain-actions/import`): „CSV exportieren“ speichert die Datei, und der Import zeigt eine eingefügte oder gewählte Datei vorab an, bevor er die Zeilen importiert, die du behältst.

| CSV-Zeile | Maßnahme auf diesem Server |
| --- | --- |
| `#severity` ist `suspend` (oder leer) | `suspend` |
| `#severity` ist `silence` | `limit` |
| `#severity` ist `noop` | Keine |
| `#reject_media` / `#reject_reports` ist true | Fügt `reject_media` / `reject_reports` hinzu |

- Ein Import hat zwei Schritte. Prüfe zuerst in der Vorschau, welche Maßnahmen jede Zeile ergreifen würde, wähle die Zeilen, die du behältst, und importiere dann (die CLI ändert nichts, bis du `-apply` hinzufügst, und `-only` wählt Zeilen nach Domain).
- Eine Zeile, deren Domain oder eine übergeordnete Domain schon eine geltende Maßnahme derselben Art hat, ergreift diese Art nicht (ist jede Art schon da, ist sie „bereits angewendet“). Eine zweite Zeile für dieselbe Domain, eine ungültige Zeile und eine Zeile, die nichts ergreifen würde (nur `noop`), werden ebenfalls übersprungen. Dieselbe CSV erneut zu importieren fügt keine Maßnahmen hinzu.
- Eine CSV ohne `#domain`-Kopfzeile wird als Liste mit einer Domain pro Zeile gelesen, und jede Domain erhält `suspend` (wie bei Mastodon). Ein Import nimmt bis zu 5.000 Zeilen.
- Importierte Maßnahmen gelten „bis zur Aufhebung“ und tragen den öffentlichen Kommentar und die Verschleierung der Zeile sowie die private Notiz, die du angegeben hast. Sie erscheinen im Audit-Log wie einzeln ergriffene Maßnahmen (der Akteur ist die Person bei der API und `cli` bei der CLI).
- Ein Export schreibt eine Zeile pro Domain für die geltenden Maßnahmen. Eine Domain mit einem `suspend` erhält `suspend`, eine mit einem `limit` erhält `silence`, und eine mit keinem von beiden erhält `noop`. Eine Domain mit nur `force_sensitive`, das die Domain-Sperren von Mastodon nicht haben, wird nicht exportiert. Private Notizen und Endzeiten werden nicht exportiert.

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

<!-- source: federating-only-with-allowed-servers 042f518cd527 -->
### Nur mit erlaubten Servern föderieren

Die Instanzeinstellung `federation_scope` bestimmt, mit welchen Servern dieser Server föderiert: `open` (der Standard) föderiert mit jedem Server, den keine Domain-Maßnahme sperrt,
und `allowlist` nur mit den Domains der Allowlist und ihren Subdomains, für eine Organisation oder einen persönlichen Server, der nur mit Servern spricht, die er kennt.

Solange `federation_scope` `allowlist` ist:

- Ein Server ohne Erlaubnis wird wie eine gesperrte Domain behandelt: Seine Zustellungen erhalten `403`, bevor ihre Signaturen geprüft werden, nichts wird an ihn zugestellt oder von ihm abgerufen, und Relays bringen nichts von ihm herein.
- Eine Erlaubnis deckt ihre Subdomains ab: Wer `friend.example` erlaubt, erlaubt auch `social.friend.example`.
- Die Domain-Maßnahmen gelten weiterhin für die erlaubten Server: Eine erlaubte Domain mit einem `suspend` bleibt gesperrt, und eine mit einem `limit` oder `reject_media` bleibt eingeschränkt.
- ActivityPub-GETs brauchen eine Signatur eines erlaubten Servers, was auch immer `authorized_fetch` sagt, damit ein Server ohne Erlaubnis die öffentlichen Beiträge nicht lesen kann. Server und Crawler, die ihre GETs nicht signieren, erhalten `401`.
- NodeInfo gibt `metadata.federationScope` an, damit andere Server es erkennen können.

Solange sie `open` ist, ändert die Allowlist nichts, und sie wird für das nächste Mal behalten, wenn du umschaltest.

```sh
my admin domain allow add -comment "partner" friend.example
my admin domain allow list
my admin domain allow remove friend.example
my admin federation scope                   # prints the setting
my admin federation scope allowlist         # shows what the switch would end, and changes nothing
my admin federation scope -apply allowlist  # switches
my admin federation scope open
```

In der Admin-API: `GET /api/v1/admin/domain_allows`, `POST /api/v1/admin/domain_allows` (der Body hat `domain` und `comment`; eine Domain, die bereits erlaubt ist, antwortet mit `200` und behält ihre Erlaubnis) und `DELETE /api/v1/admin/domain_allows/{domain}`, mit der Berechtigung `manage_domains`.
`GET /api/v1/admin/settings/federation_scope/impact` zählt, was der Wechsel beenden würde, mit der Berechtigung `manage_settings`, und `PATCH /api/v1/admin/settings` ändert `federation_scope` wie jede andere Einstellung.
Die Allowlist enthält höchstens 5.000 Domains, und ein Kommentar, der nie veröffentlicht wird, höchstens 5.000 Zeichen. Die eigene Domain dieses Servers kann nicht erlaubt werden.

Das Umschalten auf `allowlist` und das Entfernen einer Erlaubnis, während sie an ist, beendet Follows endgültig:

- Lies vor dem Umschalten, wie viele genehmigte Follows zwischen lokalen Nutzern und den Servern ohne Erlaubnis der Wechsel beenden würde, wie vielen lokalen Nutzern einer verloren geht und welche Server die meisten davon haben (die Auswirkung oben oder `my admin federation scope allowlist` ohne `-apply`).
- Der Wechsel trennt die Follows mit jedem Server, den die Allowlist nicht erlaubt, in beide Richtungen und ohne diese Server zu informieren, wie es ein Domain-`suspend` tut: Ein Job (`moderation.sever_allowlist`) trennt sie einen lokalen Nutzer nach dem anderen, und jeder Nutzer, der genehmigte Follows verloren hat, erhält eine Benachrichtigung (`relationships_severed`) mit den Zahlen.
- Das Entfernen einer Erlaubnis, während `federation_scope` `allowlist` ist, trennt auf dieselbe Weise die Follows mit den Konten dieser Domain und ihrer Subdomains, die keine andere Erlaubnis abdeckt. Das Entfernen der Erlaubnis einer Subdomain, deren übergeordnete Domain erlaubt ist, trennt nichts.
- Das Zurückschalten auf `open` oder das erneute Erlauben der Domain bringt die getrennten Follows nicht zurück. Nutzer folgen erneut.
- Wird `allowlist` gesetzt, wenn es bereits `allowlist` ist, wird nichts weiter getrennt.

Öffne im Web-Client Moderation → Domain-Maßnahmen und wähle „Zugelassene Domains“ (`/moderation/domain-actions/allowed`), um Erlaubnisse zu lesen,
hinzuzufügen und zu entfernen; das Entfernen einer fragt zuerst nach, da es die Domain abschneidet, solange `federation_scope` `allowlist` ist. Die
Einstellung selbst ist „Föderationsbereich“ auf der Föderationsseite der Servereinstellungen (`/admin/settings/federation`, mit der
Berechtigung `manage_settings`): Wählst du „Nur zugelassene Domains“ und speicherst, zeigt es die Follows, die der Wechsel beenden würde, die lokalen
Nutzer, die einen verlieren, und die Server mit den meisten davon, und speichert erst, wenn du bestätigst.

Das Hinzufügen und Entfernen einer Erlaubnis stehen im Audit-Log (`domain_allow_added` und `domain_allow_removed`), und der Wechsel ist das `setting_changed` von `federation_scope`.

<!-- source: publishing-the-domains-you-act-on b647d2c8c84c -->
### Die Domains veröffentlichen, gegen die du Maßnahmen ergreifst

Du kannst die geltenden `limit`- und `suspend`-Maßnahmen über `GET /api/v1/instance/domain_actions` veröffentlichen.
Die Server-Einstellung `domain_actions_visibility` (`off` / `signed_in` / `everyone`, Standard `off`) bestimmt, wer die Liste sieht, und
`domain_actions_rationale_visibility` (dieselben Werte, Standard `off`) bestimmt, wer die öffentlichen Kommentare sieht ([Server-Einstellungen](instance-settings.md)).

```sh
my instance set domain_actions_visibility signed_in
my instance set domain_actions_rationale_visibility signed_in
```

- Bei `off` liefert die API `404`, und bei `signed_in` liefert sie `401` auf eine Anfrage ohne Access-Token.
- Es gibt einen Eintrag pro Maßnahme, neueste zuerst. Ein Eintrag hat die Domain, das SHA-256 der Domain (hex) und die Art sowie den öffentlichen Kommentar für Leute, die ihn sehen dürfen. Private Notizen werden nie geliefert.
- Bei einer Maßnahme mit `-obfuscate` (`obfuscate`) wird die mittlere Hälfte der Zeichen der Domain (außer Punkten) durch `*` ersetzt (`bad.example` wird zu `bad.*****le`).
  Jeder kann das SHA-256 einer Domain, die er kennt, vergleichen, um zu prüfen, ob es eine verschleierte ist.
- `reject_media`-, `reject_reports`- und `force_sensitive`-Maßnahmen sowie aufgehobene oder abgelaufene Maßnahmen werden nicht veröffentlicht.
- Die Informationsseite des Servers im Web-Client (`https://<host>/about`) zeigt diese Liste unter denselben Einstellungen. Bei `off` wird der Abschnitt nicht gezeigt, und bei `users` sehen Leute, die nicht angemeldet sind, einen Hinweis, dass nur angemeldete Nutzer sie sehen können.

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

<!-- source: checking-what-reached-a-remote-server b659a7421ff9 -->
## Prüfen, was einen entfernten Server erreicht hat

Fragt ein Nutzer, ob ein Beitrag, ein Follow oder eine Annahme einen anderen Server erreicht hat, liste die Zustellungen an diesen Server mit der Admin-API auf.
Sie zeigt jede Zustellung einer Aktivität an eine Inbox des Servers, die neueste zuerst, mit jedem Versuch und dem, was der Server antwortete, einschließlich der erfolgreichen Zustellungen.
Sie braucht die Berechtigung `manage_domains`.
Im Web-Client zeigt der Abschnitt **Zustellungen** des Bildschirms eines entfernten Servers (`/moderation/instances/{domain}`) dieselbe Liste, mit ihren Filtern nach **Aktivitäts-ID** und **Lokalem Konto** und **Maskierte Felder anzeigen** bei jeder maskierten Zustellung; der Bildschirm sagt, welche davon im Audit-Log erfasst werden, bevor sie ausgeführt werden.

```sh
curl -H "Authorization: Bearer $TOKEN" \
  "https://example.com/api/v1/admin/instances/misskey.io/deliveries"
```

- Jede Zustellung hat den Aktivitätstyp (`Create`, `Follow`, `Accept` und so weiter), wann die Aktivität erfasst wurde und wann die Inbox hinzugefügt wurde, und ihre `attempts`, die älteste zuerst.
  Jeder Versuch hat sein `outcome` (`succeeded`, `retryable_failure`, `permanent_failure`, `skipped`), das `signature_scheme` und den `status_code` der Antwort, den `error` eines Fehlers ohne Antwort oder warum nichts gesendet wurde und die Zeit.
  Ein Versuch mit `succeeded` und `202` bedeutet, dass der andere Server die Aktivität angenommen hat; ob er sie dann zeigt, liegt bei diesem Server.
- Versuche werden 30 Tage behalten, sodass der Verlauf 30 Tage zurückreicht.
- Um die Liste einzugrenzen, füge `activity_uri=<the activity's ID>` hinzu oder `account_id=<a local account's ID>` für die Zustellungen eines Kontos.
  Eine Abfrage mit `account_id` wird im Audit-Log als `account_deliveries_viewed` erfasst, mit dem Konto und dem Server; die anderen Abfragen werden nicht erfasst.
- Der Body der Aktivität wird nie gezeigt.

Manche Zustellungen sind maskiert: Ihr Absender, ihre Aktivitäts-URI und ihre Inbox sind weggelassen, und `mask` sagt, warum.

| `mask` | Die Aktivität betrifft |
| --- | --- |
| `private`, `direct`, `local` | einen Beitrag mit dieser Sichtbarkeit |
| `approves_followers` | einen Follow, sein Undo, eine Annahme oder eine Ablehnung eines lokalen Kontos, das seine Follower genehmigt |
| `unknown` | einen Beitrag oder ein Konto, das weg ist, sodass nicht mehr bekannt ist, was es war |

Eine geteilte Inbox nennt keinen Empfänger und wird immer gezeigt; eine maskierte persönliche Inbox im Fehler eines Versuchs lautet `[masked]`.
Der Typ, die Zeiten und jeder Versuch einer maskierten Zustellung werden gezeigt, sodass du weiterhin siehst, ob sie den Server erreicht hat.
Um die maskierten Felder einer Zustellung zu sehen, nutze `POST /api/v1/admin/instances/{domain}/deliveries/{id}/reveal`; jeder Aufruf wird als `delivery_revealed` im Audit-Log erfasst, bevor etwas gezeigt wird.

<!-- source: reports 8b0684a0561c -->
## Meldungen

Meldungen (`Flag`), die von anderen Servern empfangen werden, gelangen als offen (`open`) in die Meldungsliste.

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

In der Admin-API: `GET /api/v1/admin/reports` (filtere mit `state`, `category` und `assignee`: `me`, `none` oder die ID eines lokalen Kontos; `order=oldest` listet stattdessen die am längsten wartende Meldung zuerst auf statt der neuesten), `GET .../{id}`, `POST .../{id}/assign_to_self`, `POST .../{id}/unassign`,
`POST .../{id}/resolve` (Body `{"outcome": "actioned" | "dismissed"}`), `POST .../{id}/notes` (Body `{"body": "..."}`), `POST .../{id}/reopen` und `POST .../{id}/forward` (Body `{"comment": "..."}`, optional).

- Eine Zuweisung (`assign`) kann nur an einen lokalen Nutzer gehen, dessen Rolle `manage_reports` hat. In der Admin-API weist du dir selbst zu.
- Das Ergebnis einer Erledigung (`resolve`) ist `actioned` (es wurde eine Maßnahme gegen das Ziel ergriffen) oder `dismissed` (es war keine Maßnahme nötig). Das erneute Erledigen einer erledigten Meldung ändert das Ergebnis nicht.
- Löst du eine Maßnahme aus einer Meldung aus (`report_id` / `-report` der Maßnahme), wird die Meldung in derselben Operation als `actioned` erledigt.
- Eine Notiz (`note`) ist eine interne Notiz, die nur Moderatoren sehen, bis zu 2.000 Zeichen. Sie wird weder dem Ziel noch der meldenden Person gezeigt. Sie lässt sich nicht bearbeiten oder löschen.
- Das Wiedereröffnen (`reopen`) setzt eine erledigte Meldung zurück auf `open` (die Zuweisung bleibt). Eine offene Meldung kann nicht wiedereröffnet werden (`409` in der Admin-API).
- Das Weiterleiten (`forward`, die Schaltfläche **An ihren Server weiterleiten** der Meldungsdetails) sendet eine Meldung über ein Konto auf einem anderen Server als `Flag` deines Instanz-Akteurs mit dem Konto und den gemeldeten Beiträgen, die noch existieren, an diesen Server. Der einzige Text, den sie trägt, ist das, was du für die Moderatoren jenes Servers schreibst, wenn du sie weiterleitest (`-comment`, `comment` in der Admin-API oder **Text für ihre Moderatoren** auf der Seite der Meldung; bis zu 1.000 Zeichen, standardmäßig keiner). Der Kommentar der meldenden Person und die Notiz einer [externen Prüfung](#reports-that-an-external-review-filed) verlassen deinen Server auf diesem Weg nie, und nichts von der meldenden Person wird gesendet. Eine Meldung, die ihre meldende Person beim Einreichen zum Weiterleiten gewählt hat, ging sofort hinaus, mit dem Kommentar der meldenden Person. Das Weiterleiten funktioniert bei einer Meldung, die auf diesem Server von einer Person oder von einer [externen Prüfung](#reports-that-an-external-review-filed) eingereicht wurde, offen oder erledigt, die noch nicht weitergeleitet wurde; eine Meldung, die von einem anderen Server empfangen wurde oder einen Sprachchat-Raum oder eine Veranstaltung betrifft, kann nicht weitergeleitet werden (`409` `report-not-forwardable` in der Admin-API, und die Meldungsdetails zeigen keine Schaltfläche). Eine Meldung wird einmal gesendet: Sie erneut weiterzuleiten, mit beliebigem Text, oder eine weiterzuleiten, die ihre meldende Person beim Einreichen weitergeleitet hatte, ändert nichts. `forwarded` in der Admin-API sagt, ob sie gesendet wurde, `forwardable`, ob du sie jetzt senden kannst.
- Die Meldungsdetails (`show`, `GET .../{id}`) geben die Notizen zurück, die älteste zuerst, mit der Zahl der anderen Meldungen über das Ziel und der Zahl der Maßnahmen gegen das Ziel (aufgehobene und abgelaufene eingeschlossen). Die Liste gibt keine Notizen zurück.
- Zuweisungen, Erledigungen, Wiedereröffnungen, Notizen und Weiterleitungen werden als Ereignisse festgehalten (`report_assignments`, `report_resolutions`, `report_reopenings`, `report_notes`, `report_forwards`), und `report_states` hält den aktuellen Zustand.
- Moderatoren können die gemeldeten Beiträge und den Kommentar der meldenden Person auf der Seite der Meldung übersetzen oder mit `POST .../{id}/statuses/{status_id}/translation` und `POST .../{id}/comment/translation` (Body `{"target_language": "en"}`).
  Der Text geht nur an einen Übersetzungsanbieter, solange die [Instanzeinstellung](instance-settings.md) `translation_reports_to_server` an ist, und dann nur an einen Anbieter, dem die Sichtbarkeit des Beitrags erlaubt ist: Der Zugriff eines Moderators erweitert nichts, daher geht ein Direktbeitrag nur an einen Anbieter, dem `direct` erlaubt ist, und der Kommentar zählt als `private`.
  Die Übersetzung des Kommentars wird nicht behalten, ebenso wenig die eines gelöschten Beitrags, und die Zeichen zählen nur gegen das Monatslimit des Servers. Die im Browser eingebaute Übersetzung funktioniert, wenn `translation_browser` an ist, bei Meldungen, was auch immer `translation_reports_to_server` sagt.
- Eine meldende Person sieht vom Ziel nur das, was sie sonst vom Konto sehen würde: Solange es gesperrt ist oder sie blockiert, bleibt ihre Meldung ohne das Konto aufgelistet. Moderatoren sehen immer das ganze Ziel.
- Eine lokale meldende Person erhält bei der ersten Erledigung eine Benachrichtigung (`report_resolved`). Erneutes Wiedereröffnen und Erledigen fügt keine Benachrichtigung hinzu.
  Eine meldende Person kann ihre eigenen Meldungen mit `GET /api/v1/reports` auflisten und sieht nur, ob jede offen oder erledigt ist. Das Ergebnis, die ergriffene Maßnahme, der dem Ziel genannte Grund und der Zuständige werden nicht gezeigt.

<!-- source: reports-that-an-external-review-filed 8563536b46e4 -->
### Meldungen, die eine externe Prüfung eingereicht hat

Ein [Decision Hook](decision-hooks.md), der mit `flag` antwortet, reicht eine Meldung mit deinem Server-Actor als Melder ein. Die Meldung
sagt das: `hook_origin` in der Admin-API und im `report.created`-Webhook, eine Zeile in den Meldungsdetails und `automated` in
`my admin report show`. Ihr Kommentar ist die Notiz des Plugins, und ihre Kategorie stammt aus der Begründung. Sie wird beim
Einreichen nicht weitergeleitet, und ihr Abschluss benachrichtigt niemanden. Wenn das Ziel auf einem anderen Server liegt, kannst du sie
selbst weiterleiten (`forward`): Die Notiz des Plugins bleibt auf deinem Server, und nur der Text, den du schreibst, geht hinaus. Das Abbrechen der Entscheidung
lässt die Meldung, wie sie ist.

<!-- source: voice-chat-rooms 7fd142752b07 -->
### Sprachchat-Räume

- Ein lokaler Nutzer kann einen offenen Sprachchat-Raum melden, den er sehen kann (`POST /api/v1/reports` mit `voice_room_id` und `account_id` auf den Host des Raums gesetzt).
  Die Meldung zielt auf den Host in diesem Moment und verweist auf die Einstellungen des Raums (Name und Beschreibung) so, wie sie damals waren. Sie verweist auf keine Beiträge und wird nie an einen anderen Server weitergeleitet.
- Die Meldungsdetails (`GET /api/v1/admin/reports/{id}`) zeigen den Raum (`voice_room`): jede Änderung seiner Einstellungen, wer teilgenommen hat und wann er ging, wer daraus entfernt wurde und wie er endete.
  Die Aufzeichnungen darüber, wer teilgenommen hat, werden 90 Tage nach dem Ende des Raums aufbewahrt (`voice_participants_retention_days`) und so lange, wie eine Meldung auf den Raum verweist ([Aufbewahrung von Aufzeichnungen](#record-retention-and-holds)).
  Es gibt keine Aufnahmen und keinen Text-Chat, diese Aufzeichnungen sind also der einzige Beleg.
- Ein Inhaber von `manage_reports` kann einen offenen Raum für alle Teilnehmer mit `POST /api/v1/admin/voice_rooms/{id}/end` beenden (Body `{"reason": "..."}`, optional, bis zu 500 Zeichen).
  Der Grund ist nur für die Moderatoren; die Teilnehmer sehen nur, dass der Raum beendet wurde. Es funktioniert auch bei ausgeschaltetem Sprachchat, und das Audit-Log hält `voice_room_ended` fest.

<!-- source: events d37a09a66811 -->
### Veranstaltungen

- Ein lokaler Nutzer kann eine Veranstaltung melden, die er sehen kann, auch eine abgesagte (`POST /api/v1/reports` mit `event_id` und `account_id`, gesetzt auf den Organisator der Veranstaltung).
  Die Meldung zielt auf den Organisator und verweist auf die Einstellungen der Veranstaltung (Titel, Beschreibung und Orte), wie die meldende Person sie sah. Eine Veranstaltung, die noch über ein Datum abstimmt, kann wie jede andere gemeldet und abgesagt werden. Sie verweist auf keine Beiträge und wird nie an einen anderen Server weitergeleitet.
- Die Meldungsdetails (`GET /api/v1/admin/reports/{id}`) zeigen die Veranstaltung (`event`): ihr Datum, ob und wie sie abgesagt wurde und jede Änderung ihrer Einstellungen, die älteste zuerst, mit allen ihren Orten, auch denen, die nur die genehmigten Teilnehmer sahen.
  `reported_revision_id` ist die Version, die die meldende Person sah, sodass ein nach der Meldung geänderter Titel oder eine geänderte Adresse weiterhin vor dir liegt.
- Ein Inhaber von `manage_reports` kann eine Veranstaltung mit `POST /api/v1/admin/events/{id}/cancel` absagen (Body `{"reason": "..."}`, optional, bis zu 500 Zeichen).
  Der Grund wird mit der Absage allen gezeigt, die die Veranstaltung sehen. Die Nutzer, die „Ich komme“ oder „Vielleicht“ geantwortet haben, und die eingeladenen Nutzer werden benachrichtigt, wie bei der eigenen Absage des Organisators, und das Audit-Log erfasst `event_cancelled`.
  Eine bereits abgesagte Veranstaltung antwortet mit `409` `event-closed`.
- Das Sperren oder Löschen des Organisators sagt dessen Veranstaltungen ab, die noch nicht begonnen haben, auch die, die noch über ein Datum abstimmen (`gathering.cancel_organized`), und benachrichtigt ihre Teilnehmer. Diese Absagen nennen niemanden als den, der sie abgesagt hat, und das Aufheben der Sperre stellt sie nicht wieder her. Bereits begonnene Veranstaltungen werden nicht abgesagt.

<!-- source: notifications-for-moderators-and-the-pending-counts a9a32ea8cccb -->
## Benachrichtigungen für Moderatoren und die ausstehenden Zähler

Neue Meldungen, Registrierungen, die auf Genehmigung warten, und Einsprüche erreichen Leute mit der Berechtigung als In-App-Benachrichtigungen.

| Benachrichtigung | Wann sie ankommt | Wer sie erhält |
| --- | --- | --- |
| `admin_report` | Wenn das Ziel der Meldung keine andere offene Meldung hat (eine zweite oder spätere Meldung über dasselbe Konto benachrichtigt nicht) | Leute, deren Rolle `manage_reports` hat, außer der meldenden Person und dem Ziel der Meldung |
| `admin_sign_up` | Wenn eine Registrierung als auf Genehmigung wartend beginnt (Genehmigungsmodus oder `open` bei einem Setup, das keine E-Mails senden kann) | Leute, deren Rolle `manage_accounts` hat |
| `admin_appeal` | Wenn gegen eine Maßnahme Einspruch eingelegt wird | Leute, deren Rolle `manage_accounts` hat, außer der Person, die Einspruch eingelegt hat |
| `admin_custom_emoji_request` | Wenn das Erstellen eines eigenen Emojis oder das Ersetzen seines Bildes angefragt wird | Leute, deren Rolle `manage_custom_emojis` hat, außer der anfragenden Person |
| `admin_error_group` | Wenn der Server oder der Web-Client einen Fehler einer neuen Art meldet oder eine Fehlergruppe, die erledigt war, auf einer anderen Version des Servers erneut auftritt | Leute, deren Rolle `manage_errors` hat |
| `admin_domain_list_failing` | Wenn der Abruf einer abonnierten Blockliste zum dritten Mal in Folge fehlschlägt | Leute, deren Rolle `manage_domains` hat |

- `admin_error_group` kommt höchstens 10 Mal pro Stunde für den ganzen Server an. Der Rest der Stunde wird zu einer Benachrichtigung zusammengefasst,
  die sagt, wie viele Fehlergruppen es zusätzlich gab. Eine stummgeschaltete Gruppe benachrichtigt nie. Ein Fehler, den der Web-Client für einen
  Besucher meldet, der nicht angemeldet ist, benachrichtigt erst, wenn auch der Browser eines angemeldeten Nutzers ihn meldet, und sein erneutes Auftreten benachrichtigt niemanden.
  Die Fehlergruppen werden über `GET /api/v1/admin/error_groups` und
  `POST /api/v1/admin/error_groups/{id}/resolve` (`/mute`, `/unmute`) gelesen und erledigt, stummgeschaltet oder wieder laut gestellt, die `manage_errors` brauchen.
- Wer eine Benachrichtigung erhält, wird durch seine Berechtigung bei der Erstellung der Benachrichtigung bestimmt. Eine Person, die die Berechtigung später verliert, sieht diese Art nicht mehr in ihrer Liste und ihrem Zähler für Ungelesenes. Kommt die Berechtigung zurück, sieht sie sie wieder.
- Jede Art von Benachrichtigung oben kommt auch per E-Mail, standardmäßig an. Jede Person schaltet jede Art in der App und per
  E-Mail auf der Seite **Benachrichtigungen** der Einstellungen (`/settings/notifications` im Web-Client) an oder aus, die nur die
  Arten auflistet, die ihre Rolle erhält. Eine Art, die in der App ausgeschaltet ist, wird für sie nicht erzeugt, daher wird sie weder gepusht noch per E-Mail gesendet; die Zähler von
  `GET /api/v1/admin/counts` ändern sich nicht, weil sie die Meldungen, Registrierungen und Einsprüche selbst zählen.
  Bei den offenen Meldungen geben die Zähler auch zurück, wann die älteste offene Meldung eingereicht wurde (`oldest_open_report_filed_at`).
  Der Server reiht den E-Mail-Job in der Transaktion ein, die die Benachrichtigung erfasst, und der Job sendet die E-Mail direkt nach dem
  Commit, ohne zu warten, bis die Benachrichtigung gelesen wird. Hat die Person bis zum Senden die Berechtigung verloren oder die E-Mail dieser Art
  ausgeschaltet oder ist ihre primäre Adresse nicht bestätigt oder sind ihre E-Mails gestoppt, wird nichts gesendet.
  Die E-Mail enthält nur die Art der Benachrichtigung und einen Link zum Benachrichtigungsbildschirm und nichts vom Inhalt der Meldung oder des Kontos. Ihr Abmeldelink schaltet nur die E-Mail dieser Art aus.
  Bei `mail = none` wird keine E-Mail gesendet, und `sends_email` in `GET /api/v1/instance` ist `false`.

`GET /api/v1/admin/counts` gibt die Zahl der offenen Meldungen (`open_reports`, braucht `manage_reports`), die Zahl der Registrierungen, die auf Genehmigung warten (`pending_registrations`), und die Zahl der Einsprüche, die auf Prüfung warten (`pending_appeals`), zurück (beide brauchen `manage_accounts`).
Elemente, für die dir die Berechtigung fehlt, werden nicht zurückgegeben.

<!-- source: audit-log e8f33b4d2da4 -->
## Audit-Log

Operationen von Moderatoren und Administratoren werden in derselben Transaktion wie die Operation je eine Zeile im Audit-Log (`audit_log_entries`) erfasst.
Fehlgeschlagene Operationen und Operationen, die nichts geändert haben, werden nicht erfasst: eine bereits aufgehobene Maßnahme aufheben, eine erledigte Meldung erledigen, demselben Zuständigen zuweisen, eine bereits eingestellte Regel einstellen, an dieselbe Position verschieben, eine Rolle vergeben, die der Nutzer schon hat, und eine Rolle entziehen, die der Nutzer nicht hat.

| Zielart | Operationen |
| --- | --- |
| `account` | `account_action_issued` und `account_action_lifted` (eine Maßnahme gegen ein Konto ergreifen und aufheben), `account_deletion_canceled` (die Löschung wurde abgebrochen, weil ein Einspruch gegen ein `ban` aufgehoben wurde), `revealed_private_data` (IP-Adressen und E-Mail-Adressen eingesehen), `two_factor_disabled` (Zwei-Faktor-Authentifizierung entfernt), `password_reset_link_issued` (ein Link zum Zurücksetzen des Passworts ausgestellt), `email_suppression_lifted` (E-Mail-Sperre aufgehoben), `account_moved` (ein eingeschränktes lokales Konto zu einem lokalen Konto umgezogen; Einschränkungen werden nicht auf das Ziel übertragen, du entscheidest also, ob du sie dort anwendest), `account_statuses_refetch_queued` (erneuter Abruf der Beiträge eines entfernten Kontos), `account_deliveries_viewed` (die Zustellungen eines lokalen Kontos an einen Server aufgelistet; der Zielname ist der Handle und der Server) |
| `status` | `status_refetch_queued` (erneuter Abruf eines entfernten Beitrags; der Zielname ist die Beitrags-URI) |
| `domain` | `domain_action_issued` und `domain_action_lifted` (eine Domain-Maßnahme ergreifen und aufheben), `domain_statuses_refetch_queued` (erneuter Abruf von Beiträgen der Konten der Domain), `instance_marked_available` (die Fehler eines Servers löschen), `delivery_revealed` (der maskierte Absender, die Aktivitäts-URI und die Inbox einer Zustellung an den Server gezeigt) |
| `report` | `report_assigned`, `report_unassigned`, `report_resolved`, `report_reopened`, `report_note_added`, `report_forwarded` (eine Meldung zuweisen, die Zuweisung aufheben, erledigen und wiedereröffnen, eine Notiz hinzufügen und sie weiterleiten) |
| `user` | `registration_approved`, `registration_rejected` (Prüfung der Registrierung) |
| `setting` | `setting_changed` (Instanzeinstellungen), `text_changed` (Instanztexte) |
| `rule` | `rule_added`, `rule_revised`, `rule_moved`, `rule_retired` (Instanzregeln) |
| `role` | `role_granted`, `role_revoked` (eine Rolle für einen Nutzer vergeben und entziehen), `role_created`, `role_updated`, `role_deleted` (eine Rolle erstellen, ändern und löschen) |
| `appeal` | `appeal_upheld`, `appeal_overturned` (Prüfung eines Einspruchs; das Aufheben erfasst auch `account_action_lifted` für die Maßnahme und `account_deletion_canceled` bei einem `ban`) |
| `signup_block` | `signup_block_added`, `signup_block_lifted` (eine Registrierungssperre hinzufügen und aufheben) |
| `hook_decision` | `hook_decision_applied` (der Server handelte nach dem Urteil einer externen Prüfung und ließ Zustand zurück; der Akteur ist das System), `hook_decision_overridden` (ein Moderator hob die Entscheidung auf, auch durch das Aufheben ihres Einspruchs), `hook_appeal_upheld` (ein Moderator bestätigte den Einspruch gegen einen abgelehnten Beitrag, und die Entscheidung bleibt) |
| `custom_emoji` | `custom_emoji_created`, `custom_emoji_image_replaced`, `custom_emoji_copied`, `custom_emoji_updated`, `custom_emoji_enabled`, `custom_emoji_disabled`, `custom_emoji_deleted` (ein eigenes Emoji registrieren, sein Bild ersetzen, ein Emoji von einem anderen Server kopieren, es ändern, aktivieren, deaktivieren und löschen) |
| `webhook` | `webhook_created`, `webhook_updated`, `webhook_deleted`, `webhook_enabled`, `webhook_disabled`, `webhook_secret_rotated` (einen Webhook erstellen, ändern, löschen, aktivieren und deaktivieren und sein Secret neu erzeugen) |
| `announcement` | `announcement_created`, `announcement_updated`, `announcement_published`, `announcement_withdrawn` (eine Ankündigung erstellen, ändern, veröffentlichen und zurückziehen) |
| `invite` | `invite_created`, `invite_revoked` (eine Einladung erstellen und widerrufen; der Zielname sind die ersten 4 Zeichen des Codes) |
| `billing_source` | `billing_import_queued` (ein Billing-Import wurde von Hand gestartet; der Zielname ist der Name des Anbieters) |
| `cost_item` | `cost_item_created`, `cost_item_relabeled`, `cost_item_price_revised`, `cost_item_end_changed`, `cost_item_deleted` (eine Kostenposition registrieren, ihren Namen und Ähnliches ändern, ihren Betrag überarbeiten, ihr Enddatum ändern und sie löschen; der Zielname ist der Name der Position) |
| `supporter_program` | `supporter_program_enabled`, `supporter_program_disabled` (die Abrechnung aktivieren und deaktivieren; der Zielname ist der Name des Zahlungsanbieters) und `supporter_tips_enabled`, `supporter_tips_revised`, `supporter_tips_disabled` (die Trinkgelder aktivieren, ihre Beträge ändern und sie deaktivieren; der Zielname sind die Währung und die Beträge in der kleinsten Einheit) |
| `supporter_plan` | `supporter_plan_created`, `supporter_plan_revised`, `supporter_plan_retired` (einen Unterstützungsplan erstellen, ändern und einstellen; der Zielname ist der Name des Plans) |
| `supporter_subscription` | `supporter_subscription_ended` (ein Administrator beendete ein Abonnement sofort; der Zielname sind der Name des Plans und der Handle des Nutzers) |
| `custom_emoji_request` | `custom_emoji_request_submitted`, `custom_emoji_request_withdrawn`, `custom_emoji_request_approved`, `custom_emoji_request_rejected` (eine Anfrage für ein eigenes Emoji einreichen, zurückziehen, genehmigen und ablehnen; der Zielname ist der `:shortcode:`) |
| `payment_provider` | `payment_reconcile_queued` (der Abgleich mit dem Zahlungsanbieter wurde von Hand gestartet; der Zielname ist der Name des Anbieters) |
| `oauth_client` | `oauth_client_disabled`, `oauth_client_enabled` (einen OAuth-Client deaktivieren und widerrufen; das Deaktivieren widerruft alle seine Tokens; der Zielname sind der Name des Clients und `client_id`) |
| `oauth_client_host` | `oauth_client_host_denied`, `oauth_client_host_allowed` (einen Host für Client ID Metadata Documents verbieten und das Verbot widerrufen; das Verbieten widerruft die Tokens der Clients dieses Hosts; der Zielname ist der Host) |
| `instance` | `service_closure_announced`, `service_closure_withdrawn`, `service_closure_closed`, `service_closure_delivery_completed` (die Schließung des Dienstes ankündigen oder das Datum ändern, die Ankündigung zurückziehen, schließen und die Zustellung der Schließung abschließen; der Zielname ist `service_closure`) |
| `voice_room` | `voice_room_ended` (ein Moderator beendete einen offenen Sprachchat-Raum; der Zielname ist die Raum-ID, da der Name verschwindet, wenn das Konto, das ihn geschrieben hat, gelöscht wird; die Meldungsdetails zeigen den Namen) |
| `event` | `event_cancelled` (ein Moderator sagte eine Veranstaltung ab; der Zielname ist die Veranstaltungs-ID, da der Titel verschwindet, wenn das Konto, das ihn geschrieben hat, gelöscht wird; die Meldungsdetails zeigen den Titel) |
| `domain_allow` | `domain_allow_added`, `domain_allow_removed` (eine Domain zur Allowlist der Föderation hinzufügen und sie entfernen; siehe [nur mit erlaubten Servern föderieren](#federating-only-with-allowed-servers); der Zielname ist die Domain) |
| `link_card_domain` | `link_card_domain_fetch_stopped`, `link_card_domain_images_stopped`, `link_card_domain_rule_removed` (eine Regel des Link-Karten-Crawlers für eine Domain setzen und sie entfernen; siehe [Domains für Link-Karten](instance-settings.md#link-card-domains)) |
| `login_provider` | `login_provider_created`, `login_provider_updated`, `login_provider_deleted`, `login_provider_enabled`, `login_provider_disabled`, `login_provider_secret_rotated` (einen externen Login-Anbieter hinzufügen, ändern, löschen, aktivieren und deaktivieren und sein Client-Secret ersetzen; der Zielname ist der Anzeigename des Anbieters) |
| `domain_list_subscription` | `domain_list_subscription_created`, `domain_list_subscription_updated`, `domain_list_subscription_deleted` (eine Blockliste abonnieren, ihre Einstellungen ändern und sie entfernen), `domain_list_draft_accepted` und `domain_list_draft_rejected` (einen Entwurf annehmen, was auch die Domain-Maßnahme erfasst, und einen ablehnen), `domain_list_fetch_queued` (ein von Hand verlangter Abruf); der Zielname ist der Titel des Abonnements; siehe [Blocklisten abonnieren](#subscribing-to-blocklists) |

Die Liste zeigt, ob das Ziel noch existiert. Konten, Meldungen, Einsprüche und Registrierungssperren, die nach ihrem Aufbewahrungszeitraum verschwunden sind, entfernte Erlaubnisse sowie gelöschte Rollen, eigene Emojis und Webhooks zeigen nur den Namen zum Zeitpunkt der Erfassung und keinen Link zu einem Bildschirm.

```sh
my admin audit list
my admin audit list -actor alice -action domain_action_issued
my admin audit list -target-kind domain -target-id spam.example
```

In der Admin-API: `GET /api/v1/admin/audit_log` (filtere mit `actor_account_id`, `action`, `target_kind` und `target_id`). Sie braucht die Berechtigung `view_audit`.

- Eine Zeile ist der Akteur (der Nutzer bei `user` oder `cli` bei der CLI), die Operation, die Zielart und -ID, der Anzeigename des Ziels, die ID des ursprünglichen Datensatzes und die Zeit.
  Die Ziel-ID ist die ID des Kontos, der Meldung, des Einspruchs, des Nutzers, der Regel, der Rolle oder der Registrierungssperre; bei einer Domain-Maßnahme ist es die Domain; bei Einstellungen und Texten ist es der Schlüssel.
- Der Anzeigename des Ziels (`@alice`, `@bob@example.com`, eine Domain, ein Einstellungsschlüssel, der Text einer Regel; bei einem Text sein Schlüssel, bei einer Rolle ihr Schlüssel und der Handle des Nutzers, bei einer Registrierungssperre ihre Art und ihr Wert) ist eine zum Zeitpunkt der Erfassung gemachte Kopie, sodass er auch lesbar bleibt, nachdem das Ziel weg ist.
  Jede API-Zeile gibt außerdem zurück, ob das Ziel noch existiert (`target_exists`), was bei Konten, Meldungen, Einsprüchen und Registrierungssperren, die weg sind, `false` ist. Das Web-Audit-Log zeigt bei einem Ziel, das weg ist, nur den Anzeigenamen, ohne Link.
- Der Text von Gründen und Notizen wird nicht kopiert. Schlage ihn über die ID des ursprünglichen Datensatzes nach (die Maßnahme, Zuweisung, Erledigung, Wiedereröffnung, Notiz, Prüfung oder die Version einer Einstellung oder Regel).
- Die Liste ist neueste zuerst mit Keyset-Paging (`-before <entry id>` in der CLI).
- Es gibt keine API und keine CLI, um das Audit-Log zu aktualisieren oder zu löschen. Alte Zeilen werden nach dem [Aufbewahrungszeitraum](#record-retention-and-holds) entfernt.

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
