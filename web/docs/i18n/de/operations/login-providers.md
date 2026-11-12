<!-- source: external-login-providers 48a582ab87ec -->
# Externe Anmeldeanbieter

Ein externer Anmeldeanbieter lässt Nutzer sich mit einem Konto anmelden, das sie anderswo haben: ein OpenID-Connect-Anbieter
(Google Workspace, Microsoft Entra ID, Keycloak und Ähnliche), GitHub, Discord oder ein einzelner Mastodon-Server.
Du richtest Anbieter mit dem Befehl `my login-provider` auf einem Host ein, der die Konfiguration des Servers hat, oder über die
Admin-API (`/api/v1/admin/login_providers`) mit der Berechtigung `manage_login_providers`, die nur die eingebaute Rolle
`admin` hat. Der Befehl funktioniert, bevor sich ein Administrator anmelden kann, du kannst ihn also für die erste Einrichtung verwenden.
Im Webclient nutzen Administratoren mit dieser Berechtigung „Anmeldeanbieter“ im Administrationsmenü. Die Liste
zeigt für jeden Anbieter die Art, den Zustand und ob die Registrierung erlaubt ist; „Anbieter hinzufügen“ öffnet das Formular, das einen erstellt.
Wenn du einen Anbieter auswählst, öffnet sich seine Seite, die seine Callback-URL mit einer Schaltfläche zum Kopieren zeigt, dazu, wie viele Nutzer
verknüpft sind, und auf der du seine Einstellungen änderst, ihm ein neues Client-Secret gibst, ihn aktivierst, deaktivierst und löschst.

<!-- source: what-external-sign-in-does 25fa37e22a50 -->
## Was die externe Anmeldung tut

- Ein Anbieter beweist nur, wer jemand bei diesem Anbieter ist. Ein Konto wird nie anhand der E-Mail-Adresse mit einem Nutzer verknüpft: Ein Nutzer
  verknüpft ein Konto unter „Verknüpfte Konten“ in den Einstellungen oder registriert sich damit als neuer Nutzer. Eine E-Mail-Adresse, die zu
  einem bestehenden Nutzer passt, verknüpft diesen Nutzer nie.
- Ein verknüpfter Nutzer gibt nach der Anmeldung über einen Anbieter weiterhin den Zwei-Faktor-Code (TOTP) ein, und ein Nutzer, dessen Konto
  nicht aktiv ist, kann sich nicht anmelden.
- Neue Nutzer können sich nur dann mit einem Anbieter registrieren, wenn du es für diesen Anbieter erlaubst (`-allow-signup`) und der
  [Registrierungsmodus](registration.md) ihnen die Registrierung erlaubt; Einladungscode, CAPTCHA, Registrierungssperren und Netzwerkbegrenzung
  gelten wie beim Passwortformular. Wer sich so registriert, hat kein Passwort und meldet sich nur über den Anbieter an.
  Das [Diagramm der Registrierungswege](registration.md#from-sign-up-to-the-app) zeigt beide Arten der Registrierung nebeneinander.

<!-- source: kinds c9123566b5c5 -->
## Arten

| Art | `-issuer` | Bedingungen |
| --- | --- | --- |
| `oidc` | Die Issuer-URL (`https`), genau so, wie die ID-Tokens des Anbieters sie nennen, etwa `https://accounts.google.com` | `-claim <claim>=<value>`: nur Nutzer, deren ID-Token den Claim mit einem der dafür angegebenen Werte hat |
| `github` | Keiner: Der Server kennt die Endpunkte von github.com | `-org <organization>`: nur Mitglieder einer der Organisationen (füge `-scope read:org` hinzu) |
| `discord` | Keiner: Der Server kennt die Endpunkte von discord.com | `-guild <server ID>`: nur Mitglieder eines der Discord-Server (Guilds); ohne Angabe darf jeder Discord-Nutzer hinein |
| `mastodon` | Die Webdomain des Servers, etwa `mastodon.example` (Mastodon 4.3 oder neuer) | Keine |

Beschränke bei Google Workspace die Nutzer mit `-claim hd=<your domain>` auf deine Organisation. Verlasse dich nicht auf die Domain der
E-Mail-Adresse.

<!-- source: examples 0d19ab463197 -->
## Beispiele

<!-- source: google b99294b8539d -->
### Google

Erstelle in der Google Cloud Console einen OAuth-Client vom Typ „Webanwendung“. Um ihn auf deine Organisation zu beschränken, setze den
Nutzertyp des OAuth-Zustimmungsbildschirms auf „Intern“ (verfügbar, wenn das Projekt zu einer Google-Workspace-Organisation
gehört), und füge außerdem die Bedingung `hd` hinzu: Der Nutzertyp beschränkt, wen Google hineinlässt, und die Bedingung lässt diesen
Server den Claim `hd` jedes ID-Tokens prüfen.

```sh
printf '%s\n' "$CLIENT_SECRET" | my login-provider create -kind oidc -name Google \
  -issuer https://accounts.google.com -client-id 123.apps.googleusercontent.com -claim hd=example.com
```

<!-- source: keycloak-and-other-openid-connect-providers 0840dc38a4e0 -->
### Keycloak und andere OpenID-Connect-Anbieter

Der Issuer ist die Realm-URL, etwa `https://sso.example.com/realms/main`, genau so, wie die ID-Tokens des Anbieters sie nennen.
Erstelle einen vertraulichen Client mit dem Standard-Flow und den Scopes `openid`, `email` und `profile`. Wenn der Anbieter
in einem privaten Netzwerk steht, führe ihn in `login_provider_allowed_destinations` auf ([unten](#providers-on-an-internal-network)).
Verwende `-claim`, um einen Claim zu verlangen, etwa einen Gruppen-Claim, mit einem der angegebenen Werte.

<!-- source: discord 00e9659d1eae -->
### Discord

Erstelle im [Discord Developer Portal](https://discord.com/developers/applications) eine Anwendung, öffne „OAuth2“ und
gib dem Server die Client-ID und das Client-Secret. Der Server fragt die Scopes `identify` und `email` an, und zusätzlich
`guilds`, wenn du Server mit `-guild` auflistest. Ein Nutzer wird an seiner numerischen Discord-ID erkannt; der Benutzername und der Anzeigename
schlagen nur den Benutzernamen im Registrierungsformular vor. Die E-Mail-Adresse gilt nur dann als bestätigt, wenn Discord sagt, sie sei
bestätigt; andernfalls bestätigt der Nutzer sie per E-Mail wie jede andere Adresse.

Mit `-guild` prüft der Server bei jeder Anmeldung, Registrierung und Verknüpfung, dass der Nutzer zu einem der aufgeführten Server gehört,
und weist ihn sonst ab. Ohne sie darf sich jeder Discord-Nutzer anmelden. Eine Guild-ID ist eine Zahl mit bis zu 20 Ziffern: Schalte in
Discord den „Developer Mode“ ein (User Settings, Advanced), klicke dann mit der rechten Maustaste auf das Symbol des Servers und wähle „Copy Server
ID“.

```sh
printf '%s\n' "$DISCORD_SECRET" | my login-provider create -kind discord -name Discord \
  -client-id 123456789012345678 -guild 987654321098765432
```

Füge die Callback-URL in der Anwendung unter „OAuth2“, „Redirects“ hinzu, nachdem `add` sie ausgegeben hat.

<!-- source: github a7181ea6edd6 -->
### GitHub

Erstelle eine OAuth App (Settings, Developer settings, OAuth Apps) und gib ihr die Client-ID und das Secret. Der Server fragt
den Scope `user:email` an, und zusätzlich `read:org`, wenn du Organisationen mit `-org` auflistest. Mit `-org` prüft der Server
bei jeder Anmeldung, dass der Nutzer zu einer der Organisationen gehört, Mitglieder anderer Organisationen werden also abgewiesen.

<!-- source: set-up-a-provider 8e0bb73251d2 -->
## Einen Anbieter einrichten

Die Redirect-URI, die der Anbieter braucht, ist die Callback-URL des Anbieters, `https://<host>/auth/external/<id>/callback`,
die die ID enthält, die der Server dem Anbieter gibt. Gehe also so vor:

1. Registriere beim Anbieter einen OAuth-Client (eine „OAuth App“ bei GitHub) vorerst mit einer beliebigen Redirect-URI und notiere dir die
   Client-ID und das Client-Secret, die du erhältst.
2. Führe `my login-provider create` mit ihnen aus. Es gibt die Callback-URL aus.
3. Ändere die Redirect-URI des Clients beim Anbieter auf diese Callback-URL.

```sh
# The client secret is read from standard input, or prompted for in a terminal.
printf '%s\n' "$CLIENT_SECRET" | my login-provider create -kind oidc -name "Example SSO" \
  -issuer https://sso.example.com -client-id my-client-id -claim hd=example.com
printf '%s\n' "$GITHUB_SECRET" | my login-provider create -kind github -name GitHub \
  -client-id Iv1.0123456789abcdef -org example-org -scope read:org
```

<!-- source: mastodon 6f167bff1f67 -->
### Mastodon

Ein `mastodon`-Anbieter ist ein einzelner Mastodon-Server, den du benennst; Nutzer anderer Server können sich damit nicht anmelden. Der Server
muss Mastodon 4.3 oder neuer sein: `add` liest seine Metadaten des Autorisierungsservers
(`/.well-known/oauth-authorization-server`) und weist einen Server ohne sie ab. Ohne `-client-id` registriert `add` einen Client
auf dem Server selbst, mit der Callback-URL und dem Scope `profile`, und speichert die Client-ID und das Secret, die es erhält, sodass
beim Server nichts zu tun ist:

```sh
my login-provider create -kind mastodon -name mastodon.example -issuer mastodon.example
```

Wenn der Server anderen nicht erlaubt, Clients zu registrieren, schlägt `add` fehl. Erstelle stattdessen in den Entwicklungseinstellungen des Servers eine Anwendung mit dem Scope `profile` und der
Callback-URL und gib ihre Client-ID und ihr Secret an wie bei den anderen Arten
(die Callback-URL enthält die ID des Anbieters, folge also den drei Schritten oben).

Nutzer melden sich nur mit dem Scope `profile` an, der ihr Konto liest und sonst nichts. Mastodon gibt keine E-Mail-Adresse heraus,
wer sich damit registriert, bestätigt die Adresse also per E-Mail. Das Registrierungsformular schlägt den Benutzernamen des Kontos vor.
Solange für die Domain des Servers oder eine übergeordnete Domain eine Domain-Aktion gilt ([Domain-Aktionen](moderation.md#domain-actions)), ist seine Schaltfläche weg und
niemand kann sich damit anmelden, registrieren oder ein Konto verknüpfen. Der Server eines Anbieters ändert sich nicht; richte für einen anderen Server einen neuen Anbieter ein.

Das Client-Secret wird mit dem Key-Encryption-Key verschlüsselt gespeichert ([Schlüsselverwaltung](key-management.md)) und nie wieder angezeigt,
weder vom Befehl noch von der Admin-API.

Ein neuer Anbieter ist aktiviert. Standardmäßig dürfen Nutzer ihr bestehendes Konto damit verknüpfen (`-allow-link`), aber niemand darf sich
damit registrieren: Füge `-allow-signup` hinzu, damit sich neue Nutzer damit registrieren können, was der Registrierungsmodus weiterhin begrenzt
([Registrierung](registration.md)).

<!-- source: change-rotate-disable-and-delete 55a0d23ac0d0 -->
## Ändern, rotieren, deaktivieren und löschen

```sh
my login-provider list                                 # every provider with its callback URL and linked users
my login-provider set -name "Corp SSO" <id>            # changes only the settings it names
my login-provider set -claim '' <id>                   # '' removes every -claim (or -scope, -org)
my login-provider rotate-secret <id>                   # reads the new client secret like add
my login-provider disable <id>
my login-provider enable <id>
my login-provider delete <id>
```

- `set` ändert nur die Einstellungen, die es nennt. `-scope`, `-claim` und `-org` ersetzen die gesamte Liste. Die Art eines Anbieters
  ändert sich nicht; richte stattdessen einen neuen Anbieter ein.
- `rotate-secret` ersetzt das Client-Secret durch das, das du eingibst. Rotiere es zuerst beim Anbieter, dann hier.
- `disable` behält den Anbieter und die mit ihm verknüpften Konten, aber niemand kann sich damit anmelden, registrieren oder verknüpfen, und die
  Anmeldeseite zeigt ihn nicht an, bis du `enable` ausführst. `disable` und `delete` geben aus, wie viele verknüpfte Nutzer sich nicht mehr
  mit dem Anbieter anmelden können und was die [Beschränkung auf Anmeldeanbieter](#limit-sign-in-to-login-providers) jetzt tut, solange ihre
  Einstellung eingeschaltet ist.
- `delete` entfernt den Anbieter endgültig.

<!-- source: users-who-sign-in-only-through-a-provider 11dc6f466cb7 -->
### Nutzer, die sich nur über einen Anbieter anmelden

Das Deaktivieren oder Löschen eines Anbieters berührt die Nutzer nicht. Ein Nutzer, der ein Passwort oder einen Passkey oder ein anderes verknüpftes
Konto hat, meldet sich auf diesem Weg an, es sei denn, die [Anmeldung ist auf Anmeldeanbieter beschränkt](#limit-sign-in-to-login-providers) und ein anderer
Anbieter bleibt aktiviert. Ein Nutzer, der nichts davon hat, kann sich überhaupt nicht mehr anmelden: `disable` und `delete` geben aus, wie
viele verknüpfte Nutzer den Anbieter verlieren, und der Webclient zeigt die Zahl in der Bestätigung. `enable` stellt die
Nutzer eines deaktivierten Anbieters wieder her; ein gelöschter Anbieter kommt nicht zurück.

Um einen solchen Nutzer zu retten, stelle auf der Verwaltungsseite des Kontos in den Admin-Bildschirmen oder mit
`my admin account password-reset-link @alice` einen Link zum Zurücksetzen des Passworts aus und gib ihn dem Nutzer ([Registrierung](registration.md#password-reset-and-change)).
Der Nutzer meldet sich dann mit dem neuen Passwort an.

Jede Änderung wird im Audit-Log unter dem Anzeigenamen des Anbieters festgehalten, als CLI oder als der Administrator, der sie
über die Admin-API vorgenommen hat.

<!-- source: limit-sign-in-to-login-providers bb5ebf61b647 -->
## Die Anmeldung auf Anmeldeanbieter beschränken

Die Servereinstellung `login_providers_only` (standardmäßig aus) macht die Anbieter zum einzigen Weg, sich anzumelden und zu registrieren,
solange mindestens ein Anbieter aktiviert ist. Schalte sie auf der Seite „Registrierung“ der „Servereinstellungen“ im Webclient ein oder mit der CLI:

```sh
my instance set login_providers_only true
```

Die Bestätigung im Webclient und die Ausgabe des Befehls sagen, wie viele aktive lokale Nutzer keine Verknüpfung zu einem
aktivierten Anbieter haben. Der Webclient sagt auch, wenn das dich einschließt. Er warnt und verweigert nicht: Du entscheidest.

Solange die Beschränkung gilt:

- Passwörter und Passkeys melden niemanden an, und die Anmeldeseite zeigt nur die Anbieter. Eine Anmeldung, die mit einem
  Passwort oder einem Passkey begonnen hat und auf ihren Zwei-Faktor-Code wartet, wird ebenfalls abgewiesen.
- Niemand registriert sich mit einem Passwort. Die Registrierungsseite zeigt die Anbieter, die Registrierungen erlauben (`-allow-signup`), wie gewohnt unter dem
  [Registrierungsmodus](registration.md). Gibt es keinen solchen Anbieter, lautet die Registrierung `closed`, auch für Clients
  und in NodeInfo.
- Niemand fordert einen Link zum Zurücksetzen des Passworts an oder verwendet ihn. Links bleiben gültig, bis sie ablaufen, und funktionieren, wenn die Beschränkung rechtzeitig endet.
- Nutzer können kein Passwort festlegen oder ändern und keinen Passkey hinzufügen, da damit ohnehin niemand angemeldet würde. Sie können sie aber noch entfernen.
- `my admin account password-reset-link` und die Admin-Bildschirme stellen weiterhin einen Link aus und sagen, dass er erst funktioniert, wenn die
  Beschränkung endet. `my admin create` erstellt weiterhin einen Administrator und warnt, dass sich mit dem Passwort niemand anmelden kann.
- Ein Nutzer kann sein letztes Konto bei einem aktivierten Anbieter nicht trennen.

Was weiter funktioniert:

- Sitzungen, App-Zugriffs-Tokens und Refresh-Tokens bleiben gültig. Die Beschränkung beendet keines davon.
- Ein Passwort oder ein Passkey bestätigt weiterhin Vorgänge in einer Sitzung. So wechselt ein bestehender Nutzer zu einem Anbieter: Angemeldet
  öffnet er „Verknüpfte Konten“ in den Einstellungen, bestätigt mit dem Passwort oder Passkey und meldet sich beim Anbieter an.
- Anmeldung, Registrierung und Verknüpfung bei einem Anbieter sowie der Zwei-Faktor-Code nach der Anmeldung über einen Anbieter.

Jede Abweisung liest sich gleich, welche E-Mail-Adresse, welches Passwort oder welches Token auch gesendet wurde, sie sagt also nichts über irgendein Konto.

**Nutzer ohne Verknüpfung können sich nicht mehr anmelden, sobald ihre Sitzungen enden, bis du handelst.** Ein Konto wird nie anhand der E-Mail-Adresse
verknüpft, daher findet die Anmeldung eines Anbieters sie auch nicht. Bitte sie, ein Konto zu verknüpfen, solange sie angemeldet sind. Ein Nutzer, der
seine Sitzung verloren hat, braucht dich: Beende die Beschränkung für eine Weile oder halte ihn draußen. Das schließt einen Nutzer unter `disable_login`,
`suspend` oder `ban` ein, der ohne Verknüpfung keine Anmeldeseite hat, von der aus er [Einspruch einlegen](moderation.md) kann.
Das Verknüpfen setzt außerdem voraus, dass der Browser bei den Anmeldeseiten angemeldet ist: Ein Nutzer, dessen App noch angemeldet ist, dessen Browser-Sitzung aber
beendet wurde, kann nicht verknüpfen, solange die Beschränkung gilt, und muss auf dich warten. **Kündige die Änderung deinen Nutzer an, bevor du die Beschränkung
einschaltest,** damit sie ein Konto verknüpfen, solange ihre Sitzungen bestehen.

<!-- source: end-the-limit 77f5c71d013b -->
### Die Beschränkung beenden

Die Beschränkung folgt der Einstellung und den Anbietern von einer Anfrage zur nächsten, ohne Neustart.

- Ein ausgesperrter Administrator führt dies auf einem Host aus, der die Konfiguration des Servers hat:

  ```sh
  my instance set login_providers_only false
  ```

- Ist kein Anbieter aktiviert, gilt die Beschränkung nicht: Das Deaktivieren oder Löschen des letzten Anbieters öffnet die Anmeldung mit Passwort und Passkey
  wieder, was die Einstellung auch sagt. Nutzer ohne Passwort und ohne Passkey brauchen weiterhin einen
  [Link zum Zurücksetzen](registration.md#password-reset-and-change).
- Ist die Einstellung eingeschaltet und kein Anbieter aktiviert, startet das Einrichten oder Aktivieren eines Anbieters die Beschränkung sofort; ein neuer Anbieter ist
  aktiviert, sobald er eingerichtet ist. Der Webclient warnt zuvor, mit der Zahl der aktiven Nutzer ohne Verknüpfung zu diesem Anbieter und
  ob das dich einschließt. Er warnt nur bei der Aktion, die die Beschränkung starten würde. `my login-provider create`, `enable`,
  `disable` und `delete` geben aus, was die Beschränkung jetzt tut, solange die Einstellung eingeschaltet ist.
- Wenn ein Ausfall beim Anbieter alle aussperrt, verwende denselben Befehl.

<!-- source: re-authentication 8ec1742f8811 -->
## Erneute Authentifizierung

Vorgänge, die nach dem Passwort oder einem Passkey fragen (ein Passwort festlegen, einen Passkey hinzufügen, das Konto löschen oder umziehen,
ein weiteres Konto verknüpfen), lassen einen Nutzer, der keines von beiden hat, sich bei einem verknüpften Anbieter erneut anmelden ([Registrierung](registration.md)).
Ein OpenID-Connect-Anbieter wird gebeten, den Nutzer erneut zu authentifizieren (`max_age=0` und `prompt=login`), und der Server
weist eine frühere Authentifizierung ab; ein Mastodon-Server wird mit `force_login=true` gefragt. GitHub und Discord haben keine Möglichkeit,
eine frische Anmeldung zu verlangen, daher wird ein Nutzer, der dort noch angemeldet ist, ohne Eingabe bestätigt. Der Server prüft weiterhin,
dass das Konto das verknüpfte ist, aber das ist ein schwächerer Beweis dafür, dass die Person an der Tastatur sitzt. Bevorzuge einen OpenID-Connect-Anbieter
oder lass Nutzer ein Passwort oder einen Passkey behalten, wenn es darauf ankommt.

<!-- source: providers-on-an-internal-network 7f19e187a375 -->
## Anbieter in einem internen Netzwerk

Der Server erreicht Anbieter über seinen Wächter für ausgehendes HTTP, der Loopback- und private Adressen ablehnt. Um einen
Anbieter in deinem eigenen Netzwerk zu verwenden, etwa einen Keycloak neben dem Server, führe seinen Host oder Adressbereich in
[`login_provider_allowed_destinations`](../reference/configuration.md#login_provider_allowed_destinations) auf.
