<!-- source: federation-relays bbd5bb98dde3 -->
# Föderations-Relays

Ein Relay ist ein Server, der die öffentlichen Beiträge der Server, die ihm beitreten, an die anderen beigetretenen Server weitergibt. Selbst ein kleiner Server mit wenigen Follows erhält eine gefüllte föderierte Timeline und Hashtag-Timelines,
und seine eigenen öffentlichen Beiträge reichen weiter. Ein Server, der keinem Relay beitritt, gewinnt keine Zeilen und keine Jobs.

<!-- source: join-a-relay 147b07e65534 -->
## Einem Relay beitreten

Verwende „Relays“ im Admin-Bereich (`/admin/relays`). Das braucht die Berechtigung `manage_settings`, die die eingebaute Rolle `admin` hat.
„Relay beitreten“ öffnet das Formular, das eines registriert. Die Liste zeigt den Status jedes Relays mit einem Button, der es verlässt oder ihm erneut beitritt;
die Wahl eines Relays öffnet seine Seite mit seinen Adressen, dem, was es in den letzten 24 Stunden eingebracht hat, und dem Button, der es löscht.
Die CLI ist `my admin relay` (`my admin relay -help`):

```sh
my admin relay list                                               # every relay, its state, and what it brought in during the last 24 hours
my admin relay create [-protocol mastodon|litepub] [-send-only] <url>  # register a relay and send it the Follow
my admin relay unfollow <relay id>                                # leave a pending or joined relay
my admin relay follow <relay id>                                  # join again a relay that the instance left or that rejected it
my admin relay delete <relay id>                                  # delete a relay that the instance left or that rejected it
```

1. Registriere die URL des Relays. Es gibt zwei Protokolle:
   - **Mastodon**: die **Inbox-URL** des Relays (etwa `https://relay.example/inbox`). Viele Relays, darunter pub-relay und Activity-Relay, akzeptieren sie.
   - **LitePub**: die **Actor-URL** des Relays (etwa `https://relay.example/relay`). Das ist das Protokoll der Relays von Pleroma und Akkoma.
   Wenn du kein Protokoll wählst („Automatisch erkennen“), verwendet der Server LitePub, wenn die URL ein Actor-Dokument liefert, und sonst Mastodon.
2. Nachdem du es registriert hast, sendet der Server-Actor dem Relay ein `Follow`, und der Status ist „Warten auf die Antwort des Relays“ (`pending`).
   Wenn das Relay annimmt, wird der Status „Beigetreten“ (`accepted`); wenn es ablehnt, „Vom Relay abgelehnt“ (`rejected`). Manche Relays brauchen eine Weile, weil ein Betreiber jede Anfrage von Hand freigibt.
3. Wenn du bei der Registrierung „Beiträge vom Relay empfangen“ ausschaltest, sendet der Server nur. Um das zu ändern, verlasse das Relay und registriere es erneut.

Du kannst das Relay eines Servers unter einer Domain-Maßnahme (`suspend`) nicht registrieren. Wird der Server eines beigetretenen Relays später gesperrt, ändert sich der Status nicht,
aber an dieses Relay wird nichts gesendet und von ihm nichts empfangen, und der Admin-Bereich zeigt eine Warnung.

<!-- source: what-the-server-sends 1db52c508f9c -->
## Was der Server sendet

- Nur das Erstellen, Bearbeiten und Löschen lokaler **öffentlicher** (`public`) Beiträge wird gesendet. Beiträge, die `unlisted`, nur für Follower, direkt oder nur lokal (`local`) sind, sowie Boosts werden nie gesendet.
  Um einen Beitrag von Relays fernzuhalten, poste ihn als `unlisted` oder `local`. Es gibt kein Opt-out pro Nutzer.
- Die Dokumente öffentlicher Beiträge tragen eine Linked Data Signature (`RsaSignature2017`), damit Mastodon und Misskey Beiträge akzeptieren können, die ein Relay unverändert weitergibt.
- Die Zahl der Zustellungen wächst um die Zahl der beigetretenen Relays mal die Zahl der öffentlichen Beiträge (Zustellungen an ein Relay werden zu einer zusammengefasst).

<!-- source: what-the-server-receives ff962150d1a7 -->
## Was der Server empfängt

- Ein öffentlicher Beitrag, den ein beigetretenes Relay trägt, wird von seinem Ursprung erneut abgerufen und geprüft, bevor er aufgenommen wird. Er erscheint dann in der föderierten Timeline und den Hashtag-Timelines. In Home-Timelines erscheint er nicht und erzeugt keine Benachrichtigung.
- Pro Relay werden höchstens `relay_max_activities_per_hour` Beiträge pro Stunde aufgenommen (Standard 1.000). Alles über dem Limit wird verworfen, ohne abgerufen zu werden.
  Die Seite des Relays im Admin-Bereich und `my admin relay list` zeigen, wie viele in den letzten 24 Stunden aufgenommen und wie viele über dem Limit verworfen wurden. Werden viele verworfen, hebe das Limit an oder überdenke das Relay.
- Ein Beitrag wird nicht aufgenommen, wenn seine Domain oder sein Konto ein `suspend` (eine Föderationsbeschränkung) oder ein `limit` (eine Beschränkung, die ihn von öffentlichen Orten entfernt) hat.
- Ein Beitrag, der nur über ein Relay eintraf, wird nach der Server-Einstellung `relay_status_retention_days` (Standard 7) aus der öffentlichen Ansicht entfernt. Ein Beitrag, auf den eine lokale Person geantwortet hat, den sie geboostet, favorisiert oder mit einem Lesezeichen versehen hat,
  und die Beiträge von Autoren, denen eine lokale Person folgt, erhalten dieselbe Aufbewahrung wie andere entfernte Beiträge (`remote_status_retention_days`).
- Das Aufnehmen kostet einen Abruf vom Ursprung pro Beitrag (plus einen Abruf des Actors für einen unbekannten Autor). Beim Limit von 1.000 pro Stunde sind das höchstens 24.000 Abrufe am Tag.

Die Einstellungen stehen in der [Konfigurationsreferenz](../reference/configuration.md#relay_max_activities_per_hour). Das Ergebnis des Aufnehmens wird außerdem als Metrik `my.relay.activities` exportiert (siehe [Den Server betreiben](running.md)).

<!-- source: leave-join-again-and-delete ea12da94f940 -->
## Verlassen, erneut beitreten und löschen

- „Verlassen“ sendet ein `Undo` des `Follow` und setzt den Status auf „Verlassen“ (`left`). Danach wird nichts an das Relay gesendet oder von ihm empfangen. Bereits aufgenommene Beiträge werden entfernt, wenn ihre Aufbewahrung endet.
- „Erneut beitreten“ sendet ein neues `Follow` an ein Relay, das den Server abgelehnt hat oder das der Server verlassen hat.
- „Löschen“ entfernt die Registrierung eines Relays, das verlassen oder abgelehnt wurde. Bearbeitungen und Löschungen von Beiträgen, die bereits an das Relay gesendet wurden, werden ihm auch nach dem Löschen weiterhin zugestellt.

Das Registrieren, Beitreten, Verlassen und Löschen sowie die Annahme oder Ablehnung durch das Relay werden im Audit-Log mit dem Zieltyp „Relay“ festgehalten (Annahme und Ablehnung als entfernte Aktionen).
