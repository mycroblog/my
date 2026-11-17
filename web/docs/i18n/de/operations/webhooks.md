<!-- source: server-webhooks e73d0e0966b7 -->
# Server-Webhooks

Webhooks senden Meldungen, Registrierungsanträge, Kontoanlagen und Profiländerungen, Moderationsmaßnahmen, Einsprüche,
Domain-Maßnahmen, lokale öffentliche Beiträge und Verwaltungsaktionen für eigene Emojis an ein Slack- oder Discord-Relay, ein Ticketsystem
oder ein Automatisierungswerkzeug wie n8n. Ein Server ohne konfigurierte Webhooks speichert keine zusätzlichen Zeilen und führt keine zusätzlichen Jobs aus.
Was ein Empfänger erhält (Header, Signatur, Ereignisse, Body), beschreibt die [Webhook-Referenz](../reference/webhooks.md).

<!-- source: set-up-a-webhook c8a80bc40038 -->
## Einen Webhook einrichten

Jeder mit der Berechtigung `manage_webhooks` kann Webhooks im Admin-Screen „Webhooks“ (`/admin/webhooks`) einrichten.
Die eingebaute Rolle `admin` hat sie. „Webhook hinzufügen“ öffnet das Formular; die Wahl eines Webhooks in der Liste öffnet seine Seite, wo du ihn
änderst, testest, aktivierst, deaktivierst und löschst, ihm ein neues Secret gibst und seine Zustellungen siehst.

1. Lege einen Webhook mit einer Ziel-URL, einer Beschreibung und den Ereignissen an, die er abonniert. Jedes Ereignis braucht außerdem seine eigene
   Berechtigung neben `manage_webhooks`: `manage_reports` für Meldungen, `manage_accounts` für Registrierungen, Konten,
   Moderationsmaßnahmen und Einsprüche, `manage_domains` für Domain-Maßnahmen, `manage_custom_emojis` für eigene Emojis und
   `manage_errors` für Fehlergruppen.
   Öffentliche Beiträge brauchen keine zusätzliche Berechtigung. Ein Ereignis, dessen Berechtigung dir fehlt, kannst du nicht auswählen.
2. Das Secret (`whsec_...`) wird einmal gezeigt, wenn der Webhook angelegt wird. Setze es beim Empfänger. Es lässt sich nicht erneut anzeigen.
3. Wähle auf der Seite des Webhooks „Test senden“, um `webhook.test` zu senden, und prüfe in seinen Zustellungen, dass er angekommen ist.

Das Anlegen, Ändern, Löschen, Aktivieren, Deaktivieren und Rotieren des Secrets eines Webhooks wird im Audit-Log festgehalten.
Das Ziel wird als Beschreibung und Hostname der URL gezeigt; die URL selbst wird nicht festgehalten.

<!-- source: rotate-the-secret 928c62f87538 -->
## Das Secret rotieren

„Neues Secret“ erzeugt ein neues Secret und zeigt es einmal. Während der nächsten 24 Stunden werden Nachrichten auch mit dem alten Secret signiert
(`webhook-signature` trägt zwei Signaturen), sodass du den Empfänger in diesem Fenster auf das neue Secret umstellen kannst.
Secrets werden mit `kernel.Sealer` versiegelt, bevor sie in der Datenbank gespeichert werden; der Schlüssel ist der
[KEK](key-management.md).

<!-- source: failures-and-redelivery 65aba0d2089e -->
## Fehler und erneute Zustellung

- Antwortet das Ziel mit etwas anderem als `2xx`, antwortet nicht innerhalb von 15 Sekunden oder ist nicht erreichbar,
  wird die Nachricht nach 5 Sekunden, 5 Minuten, 30 Minuten, 2 Stunden, 5 Stunden, 10 Stunden und 10 Stunden wiederholt (insgesamt 8 Versuche,
  etwa 27 Stunden). Eine Zustellung, die sie alle verbraucht hat, wird zu einem toten Job, und „Erneut senden“ in den Zustellungen sendet
  sie erneut mit demselben `webhook-id`.
- Antwortet das Ziel `410 Gone`, wird der Webhook sofort deaktiviert.
- Der Server deaktiviert ein Ziel, das seit 72 Stunden seit seinem ersten Fehler nicht erfolgreich war und 20 oder mehr Mal
  in Folge fehlgeschlagen ist. Das Audit-Log erhält einen `webhook_disabled`-Eintrag von `system`, und jeder mit `manage_webhooks` erhält eine
  In-App-Benachrichtigung (und eine E-Mail, wenn er Benachrichtigungen per E-Mail erhalten möchte).
- Die Liste im Admin-Screen zeigt die Zahl der Fehler in Folge und den Grund, aus dem der Server einen Webhook deaktiviert hat
  (`gone` oder `failing`). Aktiviere den Webhook erneut, nachdem du das Ziel repariert hast. Nachrichten aus der Zeit, in der er deaktiviert war, werden
  nicht erneut gesendet.
- Nachrichten und Zustellungen werden nach 14 Tagen gelöscht (fest). Bodys enthalten personenbezogene Daten wie den Kommentar einer Meldung.

<!-- source: destinations-on-an-internal-network 3f8b39001831 -->
## Ziele in einem internen Netzwerk

Die Zustellung läuft durch den SSRF-Schutz für ausgehendes HTTP. Standardmäßig erreicht sie nur öffentliche Adressen über `https` (Ports 443
und 80). Um an ein internes Ziel zu senden, etwa ein n8n auf demselben Host oder einen Chat in deinem VPC, liste Hostnamen,
IP-Adressen und CIDRs, durch Kommas getrennt, in der Einstellung `webhook_allowed_destinations` auf (Umgebungsvariable
`MY_WEBHOOK_ALLOWED_DESTINATIONS`; siehe die [Konfigurationsreferenz](../reference/configuration.md#webhook_allowed_destinations)).

```sh
MY_WEBHOOK_ALLOWED_DESTINATIONS=n8n.internal,10.0.0.0/8
```

- Nur die aufgelisteten Ziele, und nur für die Webhook-Zustellung, sind vom Schutz ausgenommen. Föderation, Medien und Link-
  Vorschauen bleiben unverändert. Ein ausgenommenes Ziel darf auch `http` und jeden Port verwenden.
- Ein Hostname ist erlaubt, auf welche Adresse er auch auflöst. Ein CIDR ist erlaubt, wenn die URL eine IP-Adresse in diesem
  Bereich nennt. Wenn du einen Hostnamen verwendest, der in den Bereich auflöst, liste auch den Hostnamen auf.
- Nur ein Betreiber mit Zugang zum Server kann die Einstellung ändern. Die Berechtigungen des Admin-Screens allein können
  ein internes Netzwerk nicht erreichen.
- Ein Versuch bei einem Ziel, das der Schutz abgelehnt hat, zeigt `forbidden_destination` in den Zustellungen.
- Die Entwicklungseinstellung `http_allow_private_networks` hebt den Schutz für jeden Zweck auf, Webhooks eingeschlossen.
