<!-- source: decision-hooks 9945608ea473 -->
# Decision Hooks

Ein Decision Hook fragt ein [Plugin](plugins.md) von dir, ob etwas angenommen werden soll, bevor der Server es annimmt. Das Plugin
antwortet mit einem Verdikt aus einer festen Liste. Es schreibt nie etwas um und unternimmt nichts gegen ein Konto: Es kann höchstens
einen Beitrag, ein Folgen, eine Registrierung, einen Upload oder eine Profiländerung stoppen oder eine Meldung für deine Moderatoren erstellen. Menschen entscheiden, was mit einem Konto geschieht.

Standardmäßig wird nichts geprüft. Ein Server ohne Hook-Adresse ruft nichts auf und verbraucht dafür keine Zeit. Diese Version enthält
die hier beschriebenen Datensätze, Modi und Admin-Bildschirme. Von den Eingangswegen fragen Beiträge von anderen Servern
`inbound.status`, Folgen von anderen Servern `inbound.follow`, Registrierungen `signup.check`, Uploads
`media.check`, Beiträge lokaler Konten `local.status`, Profile von Konten auf anderen Servern `inbound.actor` und
Profiländerungen lokaler Konten `local.profile`; die anderen Hooks werden noch nicht aufgerufen.

![Der Weg eines Decision Hooks: Ein entfernter Beitrag, ein entferntes Folgen, eine Registrierung, ein Upload, ein lokaler Beitrag oder das geänderte Profil eines entfernten Akteurs geht durch den Modus des Hooks zu deinem Plugin, dessen Verdikt oder Fehlschlag zu einer verzeichneten Entscheidung führt. Die Liste unten beschreibt jeden Schritt.](images/decision-hooks.svg)

Vom oberen Rand des Diagramms nach unten:

1. **Was geprüft wird und wer wartet.** Ein Beitrag von einem anderen Server (`inbound.status`), ein Folgen von einem anderen Server
   (`inbound.follow`) und ein Upload (`media.check`) werden in einem Job geprüft. Eine Registrierung (`signup.check`) wird geprüft, während
   die antragstellende Person wartet, ein Beitrag eines lokalen Kontos (`local.status`), während sein Autor wartet, und eine Änderung des Profils eines lokalen
   Kontos (`local.profile`), während sein Inhaber wartet. Ein Profil von einem anderen Server (`inbound.actor`) wird
   dort geprüft, wo der Server das Konto abruft: in einem Job und auch, während eine Anfrage wartet, etwa die eines Servers,
   dessen Konto hier neu ist.
2. **Der Modus**, `hook_<hook>_mode`: `off` ruft nichts auf, `observe` verzeichnet das Verdikt und handelt nicht danach, und `enforce`
   handelt danach ([Modi](#modes)).
3. **Dein Plugin** erhält `POST /my.plugin.hook.v1.HookService/Check`. Der Server wartet bis zu `plugin_<hook>_timeout`, führt
   höchstens 8 Aufrufe gleichzeitig aus und überspringt das Plugin nach 5 Fehlschlägen in Folge für 30 Sekunden (`breaker_open`).
4. **Ein Verdikt**: `allow` nimmt den Gegenstand an, `flag` nimmt ihn an und erstellt eine Meldung, `mark_sensitive` nimmt ihn als
   sensibel markiert an, `require_approval` hält eine Registrierung zurück, und `drop` und `reject` stoppen ihn und behalten nichts (das `reject` einer Registrierung nur mit
   [`hook_signup_check_reject`](#sign-ups)). **Ein Fehlschlag** ist ein `timeout`, ein `error`, eine `invalid`e Antwort oder `breaker_open`, und eine Antwort, die der Hook nicht annimmt,
   zählt als einer. Setzt der Hook durch, entscheidet `hook_<hook>_on_failure`: `pass` nimmt den Gegenstand ungeprüft an, `retry`
   verzeichnet keine Entscheidung und führt den Job erneut aus, und `require_approval` hält eine Registrierung zurück
   ([Wenn das Plugin fehlschlägt](#when-the-plugin-fails)).
5. **Eine Entscheidung pro Hook und Gegenstand** wird verzeichnet, und ein Job, der erneut läuft, liest sie, statt das Plugin aufzurufen
   ([Die Entscheidungen](#the-decisions)).

<!-- source: the-hooks f1005c829ed5 -->
## Die Hooks

| Hook | Was er prüft | Was er antworten kann | Wenn das Plugin fehlschlägt |
| --- | --- | --- | --- |
| `inbound.status` | Beiträge von anderen Servern | `allow`, `drop`, `flag`, `mark_sensitive` | `pass` (Standard) oder `retry` |
| `inbound.follow` | Folgen von anderen Servern | `allow`, `drop`, `flag` | `pass` (Standard) oder `retry` |
| `signup.check` | Registrierungen, nach dem CAPTCHA und der Sperrliste | `allow`, `require_approval`, `reject` | `pass` (Standard) oder `require_approval` |
| `media.check` | Hochgeladene Medien, bevor jemand sie nutzen kann | `allow`, `mark_sensitive`, `flag`, `reject` | `pass` (Standard) oder `retry` |
| `local.status` | Beiträge lokaler Konten und ihre Bearbeitungen | `allow`, `flag`, `reject` | `pass` |
| `inbound.actor` | Profile von Konten auf anderen Servern, wenn sie sich ändern | `allow`, `drop`, `flag` | `pass` (Standard) oder `retry` |
| `local.profile` | Profiländerungen lokaler Konten | `allow`, `flag`, `reject` | `pass` |

Setze `plugin_<hook>_address`, um einem Hook ein Plugin zu geben, wobei ein Punkt im Namen zu `_` wird
(`plugin_inbound_status_address`). [Plugins](plugins.md) behandelt das Ziel, seine Authentifizierung und wie du ein
Plugin mit `my plugin test --kind inbound.status <address>` testest. Der Aufruf ist `POST /my.plugin.hook.v1.HookService/Check`.

Was jede Antwort bewirkt:

- `drop` stoppt den Gegenstand und speichert nichts davon. Bei `inbound.actor` behält es das zuvor gespeicherte Profil
  ([unten](#profiles-from-other-servers)). Ein falsches `drop` verbirgt einen Beitrag stillschweigend auch vor seinen Followern, lies also
  in `observe`, was ein Hook verwerfen würde, bevor du ihn durchsetzt.
- `flag` lässt ihn durch und erstellt eine Meldung pro Entscheidung. Bei `inbound.follow` betrifft die Meldung das Konto, das
  das Folgen gesendet hat, nennt keinen Beitrag und wird erstellt, sobald das Folgen verzeichnet ist; Melder, Kategorie und Kommentar sind die der
  [Meldung zu einem Beitrag](#what-inboundstatus-reviews). Bei `inbound.actor` betrifft die Meldung das Konto, dessen Profil es ist,
  auf dieselbe Weise.
- `mark_sensitive` lässt ihn durch und zeigt seine Anhänge als sensibel, bis ein Moderator die Entscheidung aufhebt.
- `require_approval` hält die Registrierung zur Genehmigung zurück.
- `reject` lehnt ihn ab, mit festem Text, der nichts von der Antwort des Plugins zeigt. Bei `signup.check` gilt außerdem
  [die Einstellung unten](#sign-ups). Bei `media.check` behält es nichts vom Upload, auch das Original nicht
  ([unten](#what-mediacheck-reviews)). Bei `local.status` kann der Autor [Einspruch einlegen](#local-posts). Bei `local.profile` bleibt das
  Profil, wie es war, und es gibt keinen Einspruch ([unten](#local-profiles)).

Eine Antwort kann einen kodierten Grund (`spam`, `harassment`, `sexual`, `violence`, `illegal`, `malware`, `other`) und eine Notiz von
bis zu 200 Zeichen für deine Moderatoren tragen. Eine Antwort, die der Hook nicht annimmt, zählt als Fehlschlag.

<!-- source: what-inboundstatus-reviews 1dcf4ccd16f9 -->
## Was `inbound.status` prüft

Der Server fragt bei jedem `Create` eines Beitrags und jedem `Update` eines solchen, den er annehmen würde, bevor er etwas speichert. Jede
Revision ist ein eigener Gegenstand mit einer eigenen Entscheidung.

- Nach einem Beitrag wird nur gefragt, wenn er mit einem lokalen Konto zu tun hat: Ein lokales Konto folgt seinem Autor, er nennt ein lokales
  Konto, oder er steht in einem Thread mit einem lokalen Beitrag. Ob ein lokales Konto dem Autor folgt, schränkt das nicht weiter ein:
  Das Plugin erhält `any_local_follower` und kann es abwägen. Nach einem Beitrag, den nur ein Relay hereingebracht hat, wird nie gefragt, ebenso wenig nach einem
  `Update` eines gespeicherten Beitrags, der nichts mehr mit einem lokalen Konto zu tun hat: Seine Revision wird ohne Frage gespeichert.
- Ein Direktbeitrag eines Autors, dem kein lokales Konto folgt, wird nicht gesendet, es sei denn, du schaltest
  `hook_inbound_status_unfollowed_direct` ein. Beiträge deiner lokalen Konten werden nie gesendet, bei keiner Einstellung.
- Nach Umfragestimmen, Löschungen, Undos, Likes, Reaktionen und Boosts wird nie gefragt, sodass das Anhalten eines Plugins nicht verhindern kann, dass eine
  Löschung eintrifft. Ein Thread-Abruf oder der Abruf eines Boosts kann denselben Beitrag trotzdem auf anderem Weg hereinbringen.

Was jede Antwort mit einem Beitrag macht:

- `drop` bei einem `Create` speichert nichts und erzeugt weder eine Benachrichtigung noch einen Timelines-Eintrag, und eine erneute Zustellung wird als
  Duplikat ignoriert. `drop` bei einem `Update` erzeugt keine Revision und behält die vorherige. Ein verworfener Beitrag wird nicht gespeichert, daher wird ein
  `Update` davon, das innerhalb des letzten Tages veröffentlicht wurde, als neuer Beitrag genommen: Das Plugin wird zu dieser Revision erneut gefragt, wie bei
  jeder anderen, und seine Antwort auf das frühere `Create` überträgt sich nicht.
- `flag` erstellt eine Meldung pro Entscheidung, mit deinem Server-Akteur als Melder, der Kategorie des Grundes (`spam` und
  `malware` als Spam, `illegal` als Legal, `harassment`, `sexual` und `violence` als Violation und alles andere als Other) und
  der Notiz des Plugins als Kommentar. Die Meldung gibt an, dass eine Prüfung sie erstellt hat. Sie wird nicht an den Server des Autors weitergeleitet,
  es sei denn, ein Moderator [leitet sie weiter](moderation.md#reports), und eine Weiterleitung sendet nie die Notiz: nur den Text, den der Moderator
  dann schreibt. Das Auflösen benachrichtigt niemanden. Jede Entscheidung erstellt ihre eigene
  Meldung, eine Spam-Welle kann also die Warteschlange füllen.
- `mark_sensitive` markiert den Beitrag, nicht die Revision: Die Markierung bleibt auf späteren Revisionen, was auch immer dazu geantwortet wird, bis ein
  Moderator die Entscheidung aufhebt. Sie zeigt Anhänge als sensibel und lässt den Text unberührt, ein Beitrag ohne Anhang
  sieht also nicht anders aus, und andere Server sehen sie nicht.

<!-- source: what-mediacheck-reviews f93110fe5a83 -->
## Was `media.check` prüft

Der Server fragt bei jeder Datei, die ein lokales Konto hochlädt, für jeden Zweck: einen Anhang, einen Avatar, ein Header-Bild oder ein eigenes
Emoji. Er fragt einmal, wenn die Datei zum ersten Mal verarbeitet wird und bevor sie bereit ist, sodass niemand sie nutzen kann, während das Plugin
entscheidet. Wird dieselbe Datei später für etwas anderes verwendet, liest das die bereits getroffene Entscheidung. Nach entfernten Medien wird nie gefragt.

Er sendet das kleinste Bild, das der Server aus der Datei erzeugt hat, ohne Metadaten, und nie das Original: bei einer Video- oder
Audiodatei das kleine Bild ihres Posters oder eingebetteten Covers oder kein Bild, wenn es keins gibt. Dazu gehören, wofür die Datei
gedacht ist, ob sie ein Bild, Video oder Audio ist, ihre Größe, wie lange sie abspielt und ob sie sich bewegt. Der Alternativtext und
alles zum Konto oder zu einem Beitrag werden nicht gesendet. Die Nachricht `MediaSubject` des `HookService`-Protokolls hat die Felder.

Was jede Antwort mit einem Upload macht:

- `reject` lässt die Verarbeitung fehlschlagen, wie bei einer Datei, die der Server nicht lesen kann: Die hochladende Person sieht, dass die Datei nicht
  verarbeitet werden konnte, und ein Beitrag kann sie nicht anhängen. **Der Server behält nichts von einer abgelehnten Datei.** Er löscht das Original, sobald
  die Verarbeitung endet, was auch immer die Antwort ist, und eine abgelehnte Datei erhält keine verarbeitete Version. Wenn du solche
  Dateien aufbewahren oder jemandem melden musst, muss dein Plugin das bei seiner Antwort tun: Der Server kann die Datei nicht herausgeben.
  Das Aufheben eines `reject` ändert nur seinen Datensatz.
- `mark_sensitive` markiert den Beitrag, der die Datei anhängt, in der Transaktion, die den Beitrag erstellt, oder in der Bearbeitung, die
  die Datei hinzufügt, und die Markierung bleibt, bis ein Moderator die Entscheidung aufhebt, wie bei `inbound.status`. Ein Avatar, ein Header oder ein
  Emoji hat keinen Beitrag, die Entscheidung wird also nur verzeichnet.
- `flag` erstellt eine Meldung pro Entscheidung, mit deinem Server-Akteur als Melder. Bei einem Anhang wird sie mit dem
  Beitrag erstellt, der die Datei anhängt, und verweist auf diesen Beitrag; eine Datei, die nie gepostet wird, erzeugt keine. Bei einem Avatar, einem Header oder
  einem Emoji wird sie erstellt, wenn die Datei verarbeitet wird, über das Konto, das sie hochgeladen hat.

Der Verarbeitungsjob wartet bis zu `plugin_media_check_timeout` auf das Plugin (standardmäßig 10 Sekunden). Das Hinzufügen eines eigenen
Emojis von einem anderen Server oder das Genehmigen einer Emoji-Anfrage verarbeitet das Bild, während der Moderator wartet, dieses Warten
geht also zu seinen Lasten.

<!-- source: sign-ups be7ddf1149a9 -->
## Registrierungen

`signup.check` läuft bei Registrierungen über die Registrierungsseite und über einen Login-Anbieter, nach dem CAPTCHA und der
[Sperrliste für Registrierungen](moderation.md), eine Anfrage, die eines von beiden ablehnt, erreicht das Plugin also nie. Eine Registrierung mit
Einladungscode ruft ihn nie auf. Er sendet die E-Mail-Adresse, die die antragstellende Person eingegeben hat, und die IP-Adresse des Clients; das Konto
existiert noch nicht, der Schlüssel des Gegenstands ist deshalb die ID, die das Konto bekäme, und wer es erneut versucht, ist ein neuer Gegenstand.

Die Antworten wirken wie die der Sperrliste, wenn der Hook durchsetzt:

- `require_approval` lässt die Registrierung unter **Moderation** > **Registrierungen** warten, wie es eine Sperre mit demselben Schweregrad
  tut, in jedem Registrierungsmodus. Genehmige oder lehne sie dort ab: Das Aufheben der Entscheidung ändert die Registrierung nicht.
- `reject` bewirkt nichts, bis du `hook_signup_check_reject` einschaltest (**Servereinstellungen** > **Föderation** > **Externe Prüfung**, standardmäßig
  aus). Solange es aus ist, wird ein `reject` in `observe` verzeichnet, da es nichts stoppt, und die Registrierung geht weiter. Ist es
  an, sieht die antragstellende Person denselben festen Text wie bei einer Sperre („Dieser Server kann diese Registrierung nicht annehmen.“). Es wird kein Konto
  geschrieben, das Aufheben einer solchen Entscheidung ändert also nur ihren Datensatz, und die Person registriert sich erneut.

Antragstellende sehen nie den Grund oder die Notiz einer Entscheidung.

Die Anfrage der antragstellenden Person wartet bis zu `plugin_signup_check_timeout` auf das Plugin (standardmäßig 2 Sekunden; halte es kurz, da
eine Person wartet). Das Warten ist nicht Teil einer Datenbanktransaktion. Eine Registrierung, die später scheitert, etwa wegen eines vergebenen
Nutzernamens, behält ihre Entscheidung, bis die Aufbewahrung sie entfernt.

<!-- source: profiles-from-other-servers e324990a50b1 -->
## Profile von anderen Servern

`inbound.actor` prüft das Profil eines Kontos auf einem anderen Server dort, wo der Server es aufnimmt: beim ersten Abruf
des Kontos, wenn das Konto ein `Update` seiner selbst sendet, und wenn der Server es erneut abruft, wie er es für die Konten,
denen deine Nutzer folgen, einmal am Tag tut. Er fragt nur, wenn das Profil sich vom gespeicherten unterscheidet, ein erneuter Abruf eines Kontos mit
demselben Profil löst also keinen Aufruf aus. Das Profil, das er prüft, besteht aus:

- dem Anzeigenamen als reinem Text,
- der Bio und den Profilfeldern, wie der Server sie speichert (bereinigtes HTML),
- den Adressen des Avatars und des Header-Bilds auf dem anderen Server. Der Server sendet kein Bild.

Das Plugin erhält außerdem die Adresse des Kontos, seinen Server, seinen Nutzernamen und ob das Konto für deinen Server neu ist. Der
Schlüssel einer Entscheidung ist die Adresse des Kontos, `#` und ein Digest des Profils, sodass jedes verschiedene Profil eine Entscheidung erhält und
dasselbe erneut gesendete Profil sie liest.

Was die Verdikte bewirken:

- `drop` behält das zuvor gespeicherte Profil: Der Anzeigename, die Bio, die Felder, der Avatar, der Header und ihre eigenen
  Emojis bleiben, wie sie waren. Der Server speichert weiterhin den Rest dessen, was er empfangen hat: die Schlüssel des Kontos, seine Inbox und andere
  Adressen, seinen Nutzernamen und ob es gesperrt (locked), ein Bot, auffindbar (discoverable) und indexierbar (indexable) ist. Ohne die Schlüssel und Adressen könnte der
  Server weder die Aktivitäten des Kontos prüfen noch an es zustellen, und die Einstellungen locked und indexable sagen, wie dem Konto
  zu folgen ist und ob sein Inhaber Indexierung erlaubt. Solange das Profil verworfen bleibt, liest ein erneuter Abruf
  dieselbe Entscheidung und behält das gespeicherte.
- Ein Konto, das zum ersten Mal abgerufen wird, wird immer gespeichert, weil Folgen und Erwähnungen es brauchen, sein `drop` wird daher als
  `flag` genommen: Das Profil wird gespeichert und eine Meldung erstellt. Die Entscheidung zeigt weiterhin `drop`.
- `flag` speichert das Profil und erstellt eine Meldung zum Konto pro Entscheidung.

Wenn das Plugin fehlschlägt, speichert `pass` das Profil ungeprüft, und der Server fragt erst wieder danach, wenn es sich ändert.
Mit `retry` speichert der Server kein Konto, dessen Profil sich geändert hat, während das Plugin fehlschlägt: Ein `Update` wartet in seinem
Job, der die anderen Aktivitäten dieses Kontos zurückhält wie bei [`inbound.status`](#when-the-plugin-fails), und ein Abruf,
der gemacht wird, während eine Anfrage wartet, schlägt fehl, sodass einem Server, dessen Konto hier neu ist, die Zustellung abgelehnt wird, bis das Plugin wieder da ist
und er sie erneut sendet.

Das Aufheben eines `drop` von `inbound.actor` ruft das Konto erneut von seinem Server ab, der das Profil speichert, das er jetzt ausliefert,
ohne das Plugin aufzurufen, wenn es das verworfene ist, und der danach fragt, wenn es sich seither geändert hat.

<!-- source: local-posts 37ba27537191 -->
## Lokale Beiträge

`local.status` prüft einen Beitrag eines Kontos auf deinem Server, bevor der Server ihn erstellt, und jede Bearbeitung davon, bevor der
Server sie speichert. Der Autor wartet auf das Plugin, deshalb
ist das Warten kurz: `plugin_local_status_timeout` (standardmäßig 1 Sekunde). Ein Fehlschlag, ein Timeout oder ein offener Breaker lässt den
Beitrag immer durch (`pass` ist sein einziges Fehlschlagsverhalten), und das Warten ist nicht Teil einer Datenbanktransaktion.

Was er sendet und wann:

- Nur die Beiträge von Konten, die jünger sind als `hook_local_status_account_age_days` (**Servereinstellungen** > **Föderation** >
  **Externe Prüfung**, standardmäßig 7 Tage; `0` sendet die Beiträge jedes Kontos). Neue Konten sind die Quelle der meisten Spams, und etablierte
  Konten werden nicht gesendet.
- Nur `public`-, `local`- und `unlisted`-Beiträge. `private`- und `direct`-Beiträge werden nie gesendet.
- Der Text, wie der Autor ihn getippt hat und wie der Server ihn rendert, die Inhaltswarnung, die Sprache, ob er als sensibel markiert ist, die Bildbeschreibungen, die Umfrageoptionen, die Hashtags, wie viele Konten er erwähnt, die Adresse, die der Beitrag
  bekäme, der Beitrag, auf den er antwortet oder den er zitiert (nur als Adresse), die Adresse und der Nutzername des Autors und wann das Konto
  erstellt wurde. Es werden keine Medien gesendet.
- Der Schlüssel des Gegenstands ist die ID, die der Beitrag bekäme. Das erneute Senden eines Beitrags mit demselben `Idempotency-Key` liefert den Beitrag
  zurück, der erstellt wurde, ohne das Plugin aufzurufen; ein neuer Versuch ist ein neuer Gegenstand.
- Eine Bearbeitung wird auf dieselbe Weise gesendet, mit dem Beitrag, wie er bearbeitet wurde, für dieselben Konten: die innerhalb von
  `hook_local_status_account_age_days`. Ihr Schlüssel ist die ID des Beitrags, `#` und die ID der neuen Revision, sodass jede Bearbeitung ein
  eigener Gegenstand ist.
- Das Starten eines Voice-Chat-Raums erstellt seinen ersten Beitrag ohne diesen Hook.

Wenn er durchsetzt, lehnt `reject` den Beitrag ab, und nichts wird gespeichert. Der Autor sieht festen Text („Dieser Beitrag wurde nicht veröffentlicht.“)
und eine Möglichkeit zum Einspruch; nie den Grund oder die Notiz. `flag` lässt den Beitrag durch und erstellt eine Meldung dazu, gegen seinen
Autor.

Bei einer Bearbeitung lehnt `reject` nur diese Bearbeitung ab: Der Beitrag bleibt, wie er war, und der Autor sieht festen Text („Diese Bearbeitung wurde nicht
gespeichert.“). Gegen eine Bearbeitung kann kein Einspruch eingelegt werden, da der Beitrag noch da ist und deine Moderatoren wie gewohnt handeln können. `flag` speichert die
Bearbeitung und erstellt eine Meldung zum Beitrag, und `allow` lässt sie durch.

<!-- source: appeals 79991f552151 -->
### Einsprüche

Der Autor eines abgelehnten Beitrags kann innerhalb von `appeal_window_days` (dasselbe Fenster wie bei Moderationsaktionen) Einspruch einlegen, einmal pro
Entscheidung. Der Einspruch enthält den Text des Beitrags, da der Server nichts davon gespeichert hat, und einen optionalen Kommentar für deine
Moderatoren. Es wird kein Plugin aufgerufen.

Deine Moderatoren (`manage_reports`) werden über jeden neuen Einspruch benachrichtigt („Einsprüche gegen abgelehnte Beiträge“ in ihren Benachrichtigungs-
einstellungen, die sie auch per E-Mail erhalten können; die Benachrichtigung öffnet den Einspruch) und prüfen Einsprüche unter **Moderation** >
**Automatische Prüfungen** > **Einsprüche gegen abgelehnte Beiträge**. Jeder Einspruch öffnet sich auf einer eigenen Seite, mit dem Text, den der Autor gesendet hat:

- **Maßnahme bestätigen** (Uphold) lässt die Entscheidung bestehen und kommt als `hook_appeal_upheld` ins Audit-Log.
- **Maßnahme aufheben** (Overturn) hebt die Entscheidung auf, was als `hook_decision_overridden` ins Audit-Log kommt, und hört auf, die
  Beiträge des Kontos an `local.status` zu senden, in einer Transaktion. Der abgelehnte Beitrag wird nicht wiederhergestellt: Der Autor postet ihn erneut.

So oder so wird der Autor über das Ergebnis benachrichtigt, unter den Benachrichtigungen der Moderatoren, und das Beitragsformular zeigt es unter
der Ablehnung. Die Benachrichtigung und das Formular sagen nur, ob die Entscheidung bleibt oder aufgehoben wurde, nie den Grund oder die
Notiz. Ein Erstanbieter-Client liest den Stand eines Einspruchs mit `GET /api/v1/hook_decisions/{id}/appeal`.

Ein Konto, das ein aufgehobener Einspruch aus dem Hook genommen hat, bleibt draußen, bis das Konto gelöscht wird. Wird es später
übernommen, prüft `local.status` es nicht mehr.

<!-- source: local-profiles dbafa1545e2a -->
## Lokale Profile

`local.profile` prüft eine Änderung des Profils eines Kontos auf deinem Server, bevor der Server sie speichert. Der Inhaber wartet
auf das Plugin, deshalb ist das Warten kurz: `plugin_local_profile_timeout` (standardmäßig 1 Sekunde). Ein Fehlschlag, ein Timeout oder ein offener
Breaker lässt die Änderung immer durch (`pass` ist sein einziges Fehlschlagsverhalten), und das Warten ist nicht Teil einer Datenbank-
transaktion.

Was er sendet und wann:

- Nur die Änderungen von Konten, die jünger sind als `hook_local_profile_account_age_days` (**Servereinstellungen** > **Föderation** >
  **Externe Prüfung**, standardmäßig 7 Tage; `0` sendet die jedes Kontos).
- Nur eine Änderung dessen, was der Hook sieht: der Anzeigename, die Bio, die Profilfelder, der Avatar oder der Header. Eine Änderung,
  die das Konto nur sperrt (locked) oder seine Auffindbarkeitseinstellungen ändert, wird nicht gesendet.
- Der Anzeigename, die Bio, wie getippt und wie der Server sie rendert, Name und Wert jedes Profilfelds (getippt und
  gerendert), ob die Änderung den Avatar oder den Header setzt, ersetzt oder entfernt, die Adresse und der Nutzername des Kontos
  und wann das Konto erstellt wurde. Es wird kein Bild gesendet. Das Profil ist öffentlich, der Hook darf also einfaches TCP verwenden.
- Der Schlüssel des Gegenstands ist die ID des Kontos, `#` und die ID der Profilrevision, die die Änderung speichern würde, sodass jede Änderung
  ein eigener Gegenstand ist.

Wenn er durchsetzt, lehnt `reject` nur diese Änderung ab: Das Profil bleibt, wie es war, und der Inhaber sieht festen Text („Diese
Änderung deines Profils wurde nicht gespeichert.“), nie den Grund oder die Notiz. Es gibt keinen Einspruch: Der Inhaber kann das Profil ändern
und erneut speichern. `flag` speichert die Änderung und erstellt eine Meldung zum Konto, die keinen Beitrag nennt; Melder,
Kategorie und Kommentar sind die der [Meldung zu einem Beitrag](#what-inboundstatus-reviews).

<!-- source: modes 6133f824d202 -->
## Modi

Jeder Hook hat einen Modus, eine [Servereinstellung](instance-settings.md) (`hook_inbound_status_mode` und so weiter), die du unter
**Servereinstellungen** > **Föderation** > **Externe Prüfung** änderst. Änderungen kommen als `setting_changed` ins Audit-Log.

| Modus | Was er tut |
| --- | --- |
| `off` | Ruft nichts auf. |
| `observe` | Verzeichnet das Verdikt und handelt nicht danach. Das ist der Standard jedes Hooks. |
| `enforce` | Handelt nach dem Verdikt. |

Ein Hook geht von `off` nur über `observe` zu `enforce`.

<!-- source: when-the-plugin-fails 7c987d71eb78 -->
## Wenn das Plugin fehlschlägt

Ein Timeout, ein Fehler und eine ungültige Antwort werden als eigene Ergebnisse verzeichnet (`timeout`, `error`, `invalid`), nie als
Verdikte. Nach 5 fehlgeschlagenen Aufrufen in Folge überspringt der Server das Plugin für 30 Sekunden und versucht dann einen Aufruf, sodass ein Ausfall
jeden Gegenstand keine Wartezeit kostet; ein übersprungener Aufruf wird als `breaker_open` verzeichnet. Jeder Serverprozess zählt für sich.

Was dann mit dem Gegenstand geschieht, hängt von `hook_<hook>_on_failure` ab:

- `pass` lässt ihn ungeprüft durch. Spam kann bei einem Ausfall hereinkommen, aber sonst bleibt nichts stehen. Das ist der Standard.
- `retry` lässt den Job fehlschlagen, der erneut läuft und das Plugin erneut aufruft, und verzeichnet keine Entscheidung. Jobs eines Akteurs laufen
  der Reihe nach, alles Spätere von diesem Akteur wartet in der Zwischenzeit, auch Löschungen, bis zu etwa 4 Stunden und 15 Minuten.
  Nach seinem letzten Lauf wird der Job zu einem [toten Job](running.md#the-job-queue-and-failed-dead-jobs), was die anderen freigibt.
- `require_approval` hält die Registrierung für einen Moderator zurück.

Bei `media.check` macht `pass` die Datei ungeprüft bereit. `retry` hält den Upload als noch in Verarbeitung, bis das Plugin
antwortet oder dem Job die Läufe ausgehen; die Datei ist in der Zwischenzeit nie bereit, und ein toter Job lässt sie unverarbeitet, was die
Bereinigung nicht angehängter Uploads dann löscht. Kein anderer Job wartet darauf.

Jeder Prozess macht höchstens 8 Aufrufe eines Plugins gleichzeitig, und ein Aufruf, der über die Frist des Hooks hinaus auf seinen Platz wartet, zählt als
Timeout. Wartezeiten zeigen sich in `my.plugin.slot_wait` ([Betrieb](running.md)). Halte `plugin_inbound_status_timeout` kurz: Während ein Aufruf
wartet, warten spätere Jobs desselben Akteurs mit.

In `observe` hält ein Fehlschlag nie etwas auf. Moderatoren erhalten einmal am Tag pro Hook eine Benachrichtigung, wenn seine Aufrufe weiter fehlschlagen.
Die Metriken sind `my.hook.decisions`, `my.hook.failures` und `my.hook.duration` ([Betrieb](running.md)).

<!-- source: the-decisions 4d41f7512468 -->
## Die Entscheidungen

Der Server verzeichnet eine Entscheidung pro Hook und Gegenstand, sodass ein Job, der erneut läuft, sie liest und das Plugin nicht erneut aufruft.
Eine Entscheidung enthält den Hook, den Schlüssel des Gegenstands, den Modus, das Ergebnis, den Grund, die Notiz und wie lange der Aufruf dauerte. Sie
enthält keinen Beitragstext, keine E-Mail-Adresse und keine IP-Adresse.

**Moderation** > **Automatische Prüfungen** (`manage_reports`) zeigt die Zahlen und Wartezeiten der letzten 7 Tage und was die Hooks
gestoppt haben, in `observe` gestoppt hätten oder nicht prüfen konnten. Jede Entscheidung öffnet sich auf einer eigenen Seite mit Grund, Notiz und
Wartezeit. Das Aufheben einer Entscheidung, aus der Liste oder von ihrer Seite, kommt als `hook_decision_overridden` ins Audit-Log. Ein `reject` von `local.status` wird dort nicht aufgehoben: Sein Autor [legt Einspruch ein](#appeals).

Was das Aufheben bewirkt, hängt von der Antwort ab. Ein `mark_sensitive` verliert seine Markierung, und eine Markierung aus einer Moderationsaktion am selben
Beitrag bleibt. Ein `flag` behält seine Meldung, die du wie gewohnt auflöst. Ein `drop` von `inbound.status` verarbeitet den empfangenen Beitrag
erneut, ohne das Plugin zu fragen, solange der Server die empfangene Aktivität behält (7 Tage); danach
sagt der Bildschirm das, und das Aufheben verzeichnet nur die Aufhebung.

Das Aufheben eines `drop` von `inbound.follow` führt das Folgen erneut aus, ohne das Plugin aufzurufen, solange der Server
die empfangene Aktivität behält (7 Tage). Danach kann das Folgen nicht zurückkommen, und die Aufhebung wird nur verzeichnet; der
andere Server muss ein neues Folgen senden.

Ein Folgen fragt den Hook nur, wenn es als neues Folgen oder neue Folgeanfrage verzeichnet würde. Das Zurücknehmen eines Folgens (`Undo`) fragt nie,
und ebenso wenig ein Folgen, das der Server ohnehin ablehnt: eines, das der Föderationsmodus ablehnt, eines von einem umgezogenen oder
deaktivierten Konto, eines des Server-Akteurs oder eines zwischen Konten, die einander blockieren und wie gewohnt ihre Ablehnung erhalten. Ein erneut gesendetes Folgen
zu einem bereits bestehenden Folgen oder einer bestehenden Anfrage fragt ebenfalls nicht: Der Server beantwortet es wie gewohnt
und nimmt es erneut an, wenn das Folgen schon angenommen ist. Ein verworfenes Folgen erhält keine Antwort, der andere Server zeigt es also weiter
als angefragt an. Dieselbe Aktivität erneut zu senden, wird als Duplikat ignoriert.

Ein automatisierter Effekt kommt mit dem System als Akteur nur ins Audit-Log, wenn er einen Zustand hinterlässt: `mark_sensitive`, ein
erstelltes `flag` und `require_approval` (`hook_decision_applied`). `drop` und `reject` speichern nichts, sie bleiben also allein in den
Entscheidungen.

Der Job `hook.purge_decisions` entfernt Entscheidungen nach `hook_decisions_retention_days` Tagen (Standard 90, mindestens 30). Er behält
die, auf denen eine Sensitiv-Markierung an einem bestehenden Beitrag beruht, die eines Kontos unter einer
[Aufbewahrungssperre](moderation.md#record-retention-and-holds) und jedes `reject` eines neuen Beitrags durch `local.status`, gegen das sein Autor noch
Einspruch einlegen kann oder dessen Einspruch auf Prüfung wartet. Gegen ein `reject` einer Bearbeitung kann kein Einspruch eingelegt werden, es verschwindet also, sobald die Aufbewahrungsfrist
vorbei ist. Ein Einspruch geht mit seiner Entscheidung. Das Löschen eines Kontos entfernt seine Entscheidungen, seine Einsprüche und
seine Ausnahme von `local.status`.

<!-- source: what-you-tell-your-users 2ab3b8f14096 -->
## Was du deinen Nutzern sagst

Solange ein Hook ein Plugin hat und nicht `off` ist, gibt deine Informationsseite (`/about`) an, dass eine externe Prüfung im Einsatz ist, was
sie abdeckt und was an sie gesendet wird. Der Server schreibt das aus deinen Einstellungen, einschließlich des Kontoalters aus
`hook_local_status_account_age_days` und `hook_local_profile_account_age_days` (oder dass die jedes Kontos gesendet werden, bei
`0`) und ob
`hook_inbound_status_unfollowed_direct` die Direktbeiträge von Autoren sendet, denen hier niemand folgt.

<!-- source: what-is-sent-and-what-it-costs fd966d9abd0d -->
## Was gesendet wird und was es kostet

Aufrufe, ihre Kosten und gesendete Daten wachsen mit dem Volumen, das der Server empfängt, vor allem bei `inbound.status`: Jeder Beitrag im Geltungsbereich
ist ein Aufruf und jede Revision ein weiterer. Prüfe, was du sendest, bevor du einen Hook einschaltest:

- `inbound.status` und `local.status` senden den Text von Beiträgen und lehnen einfaches TCP ab, auch auf Loopback. `inbound.status` sendet,
  was der entfernte Server gesendet hat: den Text in jeder seiner Sprachen, die Inhaltswarnung, ob er als sensibel markiert ist, wann
  er veröffentlicht wurde, die Adresse und URL des Beitrags, die Adresse des Beitrags, auf den er antwortet, die Adressen und Beschreibungen von
  Anhängen (keine Medien), Umfrageoptionen, Hashtags, den Autor und den Server des Autors, die Sichtbarkeit und wie viele Konten
  der Beitrag nennt und wie viele davon lokal sind. Er sendet auch, ob ein lokales Konto dem Autor folgt
  (`any_local_follower`), was Daten über deine Nutzer sind. Beiträge nur für Follower (`private`) werden wie öffentliche gesendet, und
  Direktbeiträge wie [oben](#what-inboundstatus-reviews). Vom Beitrag, auf den er antwortet, sendet er nur die Adresse.
  `local.status` sendet die unter [Lokale Beiträge](#local-posts) aufgeführten Felder. Die Nachricht `InboundStatus` des `HookService`-Protokolls hat die Felder.
- `signup.check` sendet die E-Mail-Adresse und die IP-Adresse jeder antragstellenden Person und lehnt einfaches TCP ab.
- `inbound.follow` sendet die beiden Parteien eines Folgens.
- `inbound.actor` sendet das öffentliche Profil eines Kontos auf einem anderen Server, wie unter
  [Profile von anderen Servern](#profiles-from-other-servers) aufgeführt, und darf einfaches TCP verwenden. Er wird immer aufgerufen, wenn sich ein solches Profil
  ändert, sein Volumen folgt also der Zahl der Konten, die dein Server kennt.
- `media.check` sendet das kleinste verarbeitete Bild jedes Uploads, nie das Original, mit dem Zweck der Datei, ihrer
  Art, Größe und Länge. Er darf einfaches TCP verwenden, da er keinen Text sendet: Ein verarbeitetes Bild kann trotzdem eine Person zeigen.
- `local.profile` sendet das unter [Lokale Profile](#local-profiles) aufgeführte öffentliche Profil und kein Bild. Er darf einfaches
  TCP verwenden.
