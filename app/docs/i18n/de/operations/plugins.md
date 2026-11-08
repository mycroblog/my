<!-- source: plugins 4a60f7abbec9 -->
# Plugins

Ein Plugin ist ein separates Programm oder ein Dienst, den du betreibst und dem der Server über das Netzwerk eine eng umrissene Aufgabe übergibt.
Der Server verbindet sich nur mit einer Adresse: Er startet, stoppt oder überwacht das Plugin nicht. Betreibe es als Sidecar, unter systemd
oder auf einer anderen Maschine.

Standardmäßig wird nichts delegiert. Ein Server ohne Plugin-Adresse öffnet keine Verbindung und startet keinen Prozess.
Der Build-Tag [`no_plugins`](../reference/build-tags.md) nimmt den Plugin-Client aus der Binärdatei.

![Wo Plugins sitzen: Der my-Prozess übergibt sechs Ports an ein Plugin und fragt fünf Entscheidungs-Hooks, alles über einen Plugin-Client, der sich mit den Plugins verbindet, die du betreibst; der untere Teil des Diagramms zeigt, was jedes tut, wenn ein Aufruf fehlschlägt. Die Liste unten beschreibt jeden Teil.](images/plugins.svg)

Vom oberen Rand des Diagramms nach unten:

- **Der my-Prozess**, mit beliebiger `role`, macht zwei Arten von Aufrufen an ein Plugin.
  - **Ports, die er übergibt**: Ein Plugin erledigt eine Aufgabe anstelle eines eingebauten Teils, und jeder ist nur eingeschaltet, wenn eine Einstellung
    `plugin` nennt: `translator` (die Servereinstellung `translation_providers = plugin`), `billing` (`billing_sources` enthält
    `plugin`), `mailer` (`mail = plugin`), `media_image`, `media_video` und `media_audio` (`media_image = plugin` und so
    weiter), `key_manager` (`key_manager = plugin`) und `search` (`search = plugin`).
  - **[Entscheidungs-Hooks](decision-hooks.md)**: Ein Plugin antwortet, ob etwas angenommen werden soll, und wird gefragt, solange der Modus
    `hook_<hook>_mode` des Hooks nicht `off` ist: `inbound.status` für Beiträge von anderen Servern, `inbound.follow` für Follows von
    anderen Servern, `signup.check` für Registrierungen, `media.check` für Uploads, `local.status` für Beiträge lokaler Konten,
    `inbound.actor` für die Profile von Konten auf anderen Servern und `local.profile` für Profiländerungen lokaler
    Konten.
- **Der Plugin-Client** macht jeden Aufruf. Jede Art hat ein Ziel, `plugin_<kind>_address`, und eine Deadline,
  `plugin_<kind>_timeout`, und führt höchstens 8 Aufrufe gleichzeitig aus (2 für jede Medienart). Er verbindet sich über einen Unix-Socket, `https://` oder,
  nur für [einige Arten](#plain-tcp), `http://`.
- **Deine Plugins** sind separate Programme, die du betreibst, als Sidecar, unter systemd oder auf einer anderen Maschine. Der Server
  verbindet sich nur mit ihnen.
- **Wenn ein Aufruf fehlschlägt oder abläuft** ([Details](#when-a-call-fails)):
  - `billing`, `mailer` und die Medienarten: Der Job, der den Aufruf gemacht hat, läuft später erneut.
  - `translator`: Der Server fragt den nächsten Anbieter, und wenn keiner übrig ist, sieht der Leser, dass die Übersetzung fehlgeschlagen ist.
  - `key_manager` schlägt geschlossen fehl: Alles, was einen Schlüssel braucht, schlägt fehl, und der Server startet nicht ohne das Plugin.
  - `search`: Der Job eines Schreibvorgangs läuft später erneut, und eine Suche schlägt fehl, sodass die Person sieht, dass die Suche nicht verfügbar ist.
  - Ein Entscheidungs-Hook tut, was `hook_<hook>_on_failure` sagt: `pass`, `retry` oder, bei Registrierungen, `require_approval`.

<!-- source: kinds 73da92ef7563 -->
## Arten

Jede Art hat ihr eigenes Ziel, unter den Einstellungen `plugin_<kind>_*`. Ein Punkt im Namen einer Art wird im
Schlüssel zu `_`, sodass `inbound.status` `plugin_inbound_status_address` hat.

| Art | Was sie trägt | Einfaches TCP (`http://`) | Standardprotokoll | Standard-Deadline |
| --- | --- | --- | --- | --- |
| `translator` | Den Text eines Beitrags, den ein Leser übersetzen lassen wollte | **Abgelehnt** | `json` | 10 s |
| `search` | Text, Autor, Sichtbarkeit und Sprache von `public`- und `local`-Beiträgen sowie die Suchanfragen angemeldeter Personen | **Abgelehnt** | `json` | 10 s |
| `billing` | Die Kosten des Servers | Erlaubt | `json` | 2 min |
| `mailer` | Adressen und Secrets wie Bestätigungslinks | **Abgelehnt** | `json` | 30 s |
| `media_image` | Hochgeladene Bilder mit ihren Metadaten, die einen Standort enthalten können | Erlaubt | `proto` | 4 min |
| `media_video` | Hochgeladene Videos mit ihren Metadaten, die einen Standort enthalten können | Erlaubt | `proto` | 4 min |
| `media_audio` | Hochgeladene Audiodateien mit ihren Metadaten | Erlaubt | `proto` | 4 min |
| `key_manager` | Datenverschlüsselungsschlüssel im Klartext | **Abgelehnt** | `json` | 5 s |
| `inbound.status` | Den Text eingehender Beiträge | **Abgelehnt** | `json` | 3 s |
| `local.status` | Den Text lokaler Beiträge | **Abgelehnt** | `json` | 1 s |
| `local.profile` | Das öffentliche Profil lokaler Konten, ohne Bilder | Erlaubt | `json` | 1 s |
| `inbound.follow` | Die beiden Seiten eines Follows | Erlaubt | `json` | 3 s |
| `inbound.actor` | Die öffentlichen Profile von Konten auf anderen Servern | Erlaubt | `json` | 3 s |
| `signup.check` | E-Mail-Adressen und IP-Adressen | **Abgelehnt** | `json` | 2 s |
| `media.check` | Eine kleine verarbeitete Version des Mediums, nie das Original | Erlaubt | `json` | 10 s |

Die Einstellungen, die den Server ein Ziel nutzen lassen, gehören zur jeweiligen Funktion: Die letzten fünf Arten der Tabelle sind die
[Entscheidungs-Hooks](decision-hooks.md). Diese Seite behandelt, was sie gemeinsam haben.

Der Server sendet pro Nachricht höchstens das feste Limit einer Art und liest höchstens ein anderes. Ein Aufruf über dem Limit schlägt fehl, und
der Job, der ihn gemacht hat, versucht es erneut.

<!-- source: the-destination f397f6e80550 -->
## Das Ziel

Die Einstellungen gibt es nur in der Konfigurationsdatei und der Umgebung
([Konfigurationsreferenz](../reference/configuration.md)). Keine Servereinstellung, Admin-Seite oder API kann sie anzeigen oder ändern,
sodass ein gestohlenes Administratorkonto weder Beiträge noch Schlüssel umleiten kann. Eine Änderung wirkt nach einem Neustart.

`plugin_<kind>_address` hat eine von drei Formen. Der Server prüft die Form beim Start, ohne den Namen aufzulösen, und
löst den Namen bei jeder Verbindung auf.

- `unix:///run/my/mailer.sock`: ein Unix-Socket.
- `https://mailer.internal:8443`: TLS über TCP. Der Server verifiziert das Zertifikat immer, gegen
  `plugin_<kind>_tls_ca_file` oder die Wurzeln des Systems, und keine Einstellung schaltet das ab.
- `http://billing.internal:5000`: einfaches TCP.

<!-- source: authentication 2fe4df9369ac -->
### Authentifizierung

Wähle eine:

- **Ein Unix-Socket:** Die Berechtigungen der Datei des Sockets schützen ihn.
- **mTLS:** `plugin_<kind>_tls_cert_file` und `plugin_<kind>_tls_key_file`, mit einer `https://`-Adresse.
- **Ein Bearer-Token:** `plugin_<kind>_token` (oder `MY_PLUGIN_<KIND>_TOKEN_FILE`), gesendet als `Authorization: Bearer`, nur über
  `https://` oder einen Unix-Socket. Ein Token mit einer `http://`-Adresse ist ein Konfigurationsfehler.

Eine `https://`-Adresse ohne Token und ohne Client-Zertifikat startet mit einer Warnung, da das Plugin den Server nicht von
jedem anderen unterscheiden kann, der es erreicht.

Der Server liest die Dateien von `plugin_<kind>_tls_ca_file`, `plugin_<kind>_tls_cert_file` und
`plugin_<kind>_tls_key_file` beim Start, wobei eine fehlende oder unlesbare Datei ihn stoppt. Danach prüft er sie höchstens alle 5 Sekunden
auf Änderungen, wenn er eine neue Verbindung öffnet, sodass ein kurzlebiges Zertifikat oder eine CA, die ein
Zertifikatsmanager wie Vault, cert-manager oder SPIFFE an Ort und Stelle erneuert, ohne Neustart funktioniert. Bereits
offene Verbindungen behalten das Zertifikat, mit dem sie begonnen haben, bis sie schließen. Solange eine Erneuerung halb geschrieben ist, etwa ein neues
Zertifikat, dessen Schlüssel noch nicht bereitliegt, behält der Server die zuletzt gelesenen Dateien und versucht es bei der nächsten
Verbindung erneut. Das Token wird nur beim Start gelesen.

<!-- source: plain-tcp cbb88078173e -->
### Einfaches TCP

`mailer`, `key_manager`, `translator`, `search`, `inbound.status`, `local.status` und `signup.check` lehnen `http://` ab: Der Server startet nicht. Das gilt auch auf
Loopback (`http://127.0.0.1`), da eine Loopback-Adresse andere Prozesse auf dem Host nicht daran hindert, den
Datenverkehr zu lesen. Lege einen solchen Sidecar auf einen Unix-Socket in einem gemeinsamen Volume.

Die anderen Arten akzeptieren `http://`, und der Server loggt beim Start eine Warnung, die die Art und die Adresse nennt. Nichts
verschlüsselt oder authentifiziert diesen Datenverkehr, und es kann kein Bearer-Token darüber verwendet werden. Dieses Netzwerk
geschlossen zu halten, liegt in deiner Verantwortung: Was die Tabelle oben aufführt, überquert es im Klartext.

<!-- source: protocols eb5a03718f1b -->
## Protokolle

`plugin_<kind>_protocol` sagt, was der Server sendet, und es muss das sein, was das Plugin akzeptiert.

| Wert | Der Server sendet | Für |
| --- | --- | --- |
| `json` | Das Unary-Protokoll von Connect mit `application/json`, über HTTP/1.1 auf einfachen Verbindungen | Ein Plugin, das auf einer Connect-Bibliothek oder allein auf einem HTTP-Server und einer JSON-Bibliothek aufbaut |
| `proto` | Das Protokoll von Connect mit `application/proto` | Ein Plugin, das auf einer Connect-Bibliothek aufbaut |
| `grpc` | gRPC über HTTP/2 | Ein Plugin, das nur gRPC spricht |

Ein Plugin beantwortet das falsche Protokoll mit `415`. Die Medienarten streamen in beide Richtungen, was HTTP/2 braucht, daher nehmen sie `proto`
oder `grpc`, und `json` ist ein Konfigurationsfehler.

Die Definitionen des Protokolls sind die `.proto`-Dateien unter `api/plugin/` des Repositorys. Es gibt ein Paket für jede
Art von Port, und sie teilen keine Typen; die Entscheidungs-Hooks teilen sich alle ein Paket, `my.plugin.hook.v1`.
[Das Plugin-Protokoll](../plugins/overview.md) enthält die aus diesen Dateien erzeugte Referenz jedes Pakets.

<!-- source: when-a-call-fails 1caa41ab429e -->
## Wenn ein Aufruf fehlschlägt

Ein Aufruf wartet auf seinen Platz unter den gleichzeitig laufenden Aufrufen seiner Art, und die Wartezeit zählt für seine Deadline und für die
Metrik [`my.plugin.slot_wait`](running.md). Ein fehlgeschlagener Aufruf zählt in der Metrik [`my.plugin.failures`](running.md) nach Art und Connect-Code. Der Server verwirft die
Nachricht, die ein Plugin in einen Fehler legt, sodass die Gründe für die Fehler eines Plugins im eigenen Log des Plugins stehen. Der Server
wiederholt einen Aufruf nicht selbst: Das tut der Job, der ihn gemacht hat, ein Plugin muss also mit demselben Aufruf zweimal zurechtkommen.

Ein Plugin, das beim Start des Servers ausgefallen ist, stoppt nichts, außer `key_manager`: Der Server startet nicht ohne es, und
[was ohne es stoppt](key-management.md#when-the-plugin-is-unreachable) ist ein Teil seiner Einrichtung, den man zuerst lesen sollte.

Ein Aufruf hat die Deadline von `plugin_<kind>_timeout`, und ein Plugin erhält sie als `Connect-Timeout-Ms` (`grpc-timeout` bei
gRPC). Ein Plugin darf diesen Header ignorieren: Der Server hört ohnehin auf zu warten. Die Deadline muss für die
Arten, auf die eine Anfrage warten kann (`key_manager`, `translator`, `search`, `signup.check`, `media.check`, `local.status`, `inbound.actor` und `local.profile`), unter 30 Sekunden bleiben und
für die anderen unter 5 Minuten, so lange ein Job höchstens laufen darf.

<!-- source: test-a-plugin 5d8195bb27f5 -->
## Ein Plugin testen

`my plugin test` führt den Vertrag einer Art gegen ein laufendes Plugin aus und meldet jeden Fall. Es liest keine Konfiguration,
alles, was es braucht, ist also die Binärdatei `my`.

```sh
my plugin test --kind probe unix:///run/my/plugin.sock
my plugin test --kind probe --protocol grpc --tls-ca-file ca.pem --token-file token https://plugin.internal:8443
```

```text
PASS reachable
PASS echo
FAIL echo_empty: the outcome is OUTCOME_UNSPECIFIED, not OUTCOME_OK
2 passed, 1 failed, 0 skipped
```

Der Exit-Code ist 1, wenn ein Fall fehlschlägt. Die Flags bedeuten das, was die Einstellungen bedeuten: `--protocol`, `--token-file`, `--tls-ca-file`,
`--tls-cert-file`, `--tls-key-file` und `--timeout`. Es wendet dieselben Regeln an wie der Server, sodass `--kind mailer` eine
`http://`-Adresse ablehnt. Das getestete Plugin kann ein echtes sein, daher läuft ein Fall, der mehr tut als den Aufruf selbst, nur, wenn
ein Flag ihn verlangt. Eine Art, deren Vertrag die Version noch nicht hat, meldet `reachable` und ein `SKIP`.

`--kind billing` prüft nur, dass das Plugin sich weigert, etwas auszugeben, wenn keine abgerechnete Anfrage erlaubt ist. `--billed` macht außerdem
einen echten Abruf des laufenden und des vorherigen Monats und prüft die Antwort wie ein Import; das kann bei den Anbietern hinter dem Plugin
Geld kosten. [Cloud-Nutzungskosten importieren](billing-import.md#a-plugin-billing_sources--plugin) enthält den
Vertrag der Art.

`--kind mailer` macht keinen Aufruf des RPC, es sei denn, `--mail-to` nennt einen Empfänger. Damit sendet der Befehl eine echte Nachricht dorthin,
von `--mail-from` (Standard: dieselbe Adresse), und prüft, dass das Plugin eine Anfrage ohne Empfänger ablehnt.
[E-Mails senden](email.md#a-plugin-mail--plugin) enthält den Vertrag der Art.

`--kind translator` sendet ein paar feste englische Sätze und übersetzt sie ins Japanische (`--translate-from` und `--translate-to`
wählen ein anderes Paar für ein Plugin, das nur einige hat). Ein Dienst hinter dem Plugin, der pro Zeichen abrechnet, rechnet auch diese ab,
etwa 200 Zeichen. Es prüft die Form der Antworten und dass das Plugin die Sprache `qaa` ablehnt, einen Code, den ISO 639
für den privaten Gebrauch freilässt. Es kann die Wörter nicht beurteilen, lies also eine Übersetzung des Plugins selbst.
[Beiträge übersetzen](#translating-posts) enthält den Vertrag der Art.

`--kind search` sucht und schreibt und entfernt nur eigene Dokumente: einige mit IDs, die kein Beitrag hat, von einem
Autor, der kein Konto ist, mit einem für den Lauf erfundenen Wort, die es am Ende entfernt. Es prüft, dass eine Suchanfrage ohne
Wort nichts findet, dass eine Suchanfrage in der Abfragesprache einer Engine oder auf Japanisch nicht fehlschlägt, dass das Plugin einen
Cursor ablehnt, den es nicht ausgegeben hat, und dass Dokumente die neuesten zuerst, seitenweise, ersetzt und nach Beitrag und nach
Autor entfernt gefunden werden. `--clear-index` führt außerdem die Fälle aus, die jedes Dokument einer Sichtbarkeit und den ganzen Index entfernen: Nutze es nur
gegen einen Index zum Testen. [Beiträge durchsuchen](#searching-posts) enthält den Vertrag der Art.

`--kind media_image`, `--kind media_video` und `--kind media_audio` senden nur kleine Testdateien, die sie selbst erzeugen:
ein Foto, ein Emoji und ein animiertes GIF, ein MP4 mit Video und Tonspur oder ein MP3. Jede versteckt einen Standort oder einen
Titel in ihren Metadaten, und ein Fall schlägt fehl, wenn das in einer Ausgabe zurückkommt. Sie prüfen jede Ausgabe wie der Server.
[Medien konvertieren](#converting-media) enthält den Vertrag der Arten.

`--kind key_manager` führt die Prüfungen aus, die der Server bei jedem Start ausführt, und prüft, dass das Plugin einen geänderten Wert und einen
KEK ablehnt, den es nicht hält. Es sendet nur Schlüssel, die es sich ausdenkt und wegwirft, ist also sicher gegen einen echten Schlüsselspeicher, der
die Aufrufe abrechnen kann. Es prüft, dass die Antworten die richtige Form haben, nicht, wie das Plugin seinen KEK aufbewahrt.
[Den Schlüssel verwalten](key-management.md#a-plugin) enthält den Vertrag der Art.

`probe` ist die kleinste Art, und der Server delegiert nichts an sie. Stelle sie zuerst bereit, um den Transport, das
Protokoll und die Authentifizierung zu prüfen, bevor du den Rest des Plugins schreibst.

<!-- source: translating-posts c9d31b2b9157 -->
## Beiträge übersetzen

Ein Plugin der Art `translator` lässt den Server einen Beitrag übersetzen, wenn ein Leser darum bittet, über einen Dienst deiner Wahl
außer den [eingebauten Anbietern](translation.md) (LibreTranslate, DeepL, Google Cloud Translation und Amazon
Translate). Ohne serverseitigen Anbieter ist die einzige Übersetzung die im Browser des Lesers eingebaute, die es
nur in Desktop-Chrome, Edge und Opera gibt: Leser auf einem Handy, in Firefox oder in Safari erhalten keine Übersetzen-Schaltfläche, bis du
einen aktivierst.

```sh
MY_PLUGIN_TRANSLATOR_ADDRESS=unix:///run/my/translator.sock
MY_PLUGIN_TRANSLATOR_NAME="Our translator"    # what readers see next to a translation
```

1. Setze `plugin_translator_address` und `plugin_translator_name` und starte neu. Der Name ist erforderlich, da Leser einer
   Übersetzung je nach ihrer Herkunft vertrauen. Die Adresse verlässt die Konfiguration nie.
2. [Aktualisiere deine Datenschutzerklärung](privacy-policy-template.md#the-services-you-choose) und nenne den Dienst hinter dem Plugin
   als Übersetzungsdienst: Was das Plugin weitergibt, ist das, was es erhält.
3. Aktiviere es mit der [Servereinstellung](instance-settings.md) `translation_providers = plugin`. Eine Änderung wirkt sofort.

Eine Adresse zu haben aktiviert nichts: Solange die Einstellung nicht `plugin` nennt, sendet der Server nichts und Leser sehen keine Schaltfläche.

Das Plugin ist ein Anbieter wie jeder andere. [`translation_provider_visibilities`](instance-settings.md) bestimmt, was es
erhalten darf, die monatlichen Limits zählen, was ihm gesendet wird, und eine von ihm erstellte Übersetzung bleibt wie jede andere im Cache des Servers.
Ist `translation_reports_to_server` eingeschaltet, erhält es auch, was Moderatoren in einer Meldung übersetzen lassen, nach denselben Sichtbarkeiten.
Es kann in der Reihenfolge von `translation_providers` vor oder nach einem anderen Anbieter stehen.

<!-- source: what-the-plugin-receives 975c7e56a522 -->
### Was das Plugin erhält

Das Plugin erhält den Text eines Beitrags, wenn ein Leser um eine Übersetzung bittet: seinen Text, die Inhaltswarnung, Umfrageoptionen und
Bildbeschreibungen sowie die Sprache des Textes und die Sprache, in die übersetzt werden soll. Es erhält nicht die Adresse des Beitrags,
seinen Autor, den Leser, Erwähnungen, Hashtags, Links, eigene Emojis oder Code: Nummerierte Markierungen stehen für sie.

Standardmäßig sind das `public`-, `local`- und `unlisted`-Beiträge. **`private`- und `direct`-Beiträge erreichen das Plugin nur, wenn du sie
erlaubst**, mit `translation_provider_visibilities = plugin=public|local|unlisted|private|direct`. Dann liest das Plugin, und jeder
Dienst dahinter, Text, den der Autor für wenige Menschen gemeint hat; erlaube sie also nur, wenn du kontrollierst, wohin der Text geht,
und sage das in deiner Datenschutzerklärung. Der Server kann Text, den er gesendet hat, nicht zurückrufen.

Der Server lehnt für diese Art `http://` ab, auch auf Loopback, weil der Text der Text von Beiträgen ist. Verwende einen Unix-Socket oder
`https://`.

<!-- source: an-llm-behind-a-plugin 611b6fc77bb1 -->
### Ein LLM hinter einem Plugin

Der Server hat keinen LLM-Anbieter und bietet absichtlich keinen an. Jeder kann einen Beitrag schreiben, daher ist der zu übersetzende Text ein Weg,
einem LLM Anweisungen zu geben (eine Prompt Injection), und Leser halten eine Übersetzung für das, was der Beitrag sagt. Sanitizing schützt
das HTML einer Seite: Es kann nicht verhindern, dass eine Übersetzung etwas sagt, das der Beitrag nicht sagt.

Der Server kann nicht sehen, was ein Plugin aufruft. **Wenn dein Plugin ein LLM verwendet, liegen dieses Risiko und seine Folgen in deiner eigenen
Verantwortung.**

<!-- source: the-contract ef518f876dd2 -->
### Der Vertrag

Der RPC ist `Translate` von `my.plugin.translator.v1.TranslatorService`:

```text
POST /my.plugin.translator.v1.TranslatorService/Translate
Content-Type: application/json

{"fragments":["Good morning ⟦1⟧, this is ⟦2⟧important⟦/2⟧.","⟦3⟧"],"sourceLanguage":"en","targetLanguage":"ja"}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","translations":["⟦1⟧さん、おはようございます。これは⟦2⟧重要⟦/2⟧です。","⟦3⟧"],"detectedLanguage":"en"}
```

- `sourceLanguage` ist ein BCP-47-Tag oder `und`, wenn der Server die Sprache nicht kennt. Das Plugin erkennt sie dann und meldet sie
  in `detectedLanguage` oder antwortet mit `OUTCOME_UNSUPPORTED_LANGUAGE`. `detectedLanguage` darf fehlen, wenn das Plugin
  nichts hinzuzufügen hat, und der Server ignoriert einen Wert, der kein Sprach-Tag ist.
- `translations` hat einen Eintrag pro Fragment, in der Reihenfolge der Fragmente, keiner davon leer. Ein Fragment kann aus nichts als
  Markierungen bestehen, und seine Übersetzung sind dann diese Markierungen.
- `⟦1⟧` steht für ein Element oder öffnet ein Paar, das `⟦/1⟧` schließt. Jede Markierung eines Fragments kommt genau einmal in seiner
  Übersetzung zurück, unverändert. Elemente dürfen sich verschieben, wie es die Wortstellung verlangt, aber ein Paar bleibt offen, bevor es schließt, und Paare verschachteln sich.
- Eine Übersetzung ist einfacher Text. Der Server maskiert jedes Markup darin und bereinigt das Ergebnis, bevor er es speichert oder anzeigt.

| `outcome` | Bedeutung | Was der Server tut |
| --- | --- | --- |
| `OUTCOME_OK` | Die Übersetzungen stehen in der Antwort | Prüft sie, speichert sie und antwortet dem Leser |
| `OUTCOME_TEMPORARY` | Der Dienst ist ausgefallen oder hat nicht geantwortet | Versucht den nächsten Anbieter |
| `OUTCOME_PROVIDER_LIMIT` | Der Dienst meldet sein eigenes Ratenlimit oder Kontingent | Versucht den nächsten Anbieter |
| `OUTCOME_UNSUPPORTED_LANGUAGE` | Der Dienst hat die Quell- oder Zielsprache nicht oder kann sie nicht erkennen | Versucht den nächsten Anbieter und sagt es dem Leser, wenn keiner sie hat |

Eine Antwort, die diese Regeln verletzt, zählt als `OUTCOME_TEMPORARY`, und nichts davon wird gespeichert, wie bei einem fehlenden oder unbekannten
`outcome` und einem fehlgeschlagenen Aufruf. Antwortet kein Anbieter, sieht der Leser, dass die Übersetzung fehlgeschlagen ist, und der Beitrag bleibt, wie er ist, es sei denn,
`translation_browser` ist eingeschaltet und der Browser des Lesers kann den Beitrag übersetzen: Der Web-Client übersetzt ihn dann dort.
Die Zeichen jedes Aufrufs zählen für die monatlichen Limits, auch bei einem Aufruf, der fehlschlägt, da der Dienst sie erhalten haben kann.

Der Server hört nach `plugin_translator_timeout` auf zu warten und nach 20 Sekunden, was auch immer diese Einstellung sagt. Alle
Anbieter einer Anfrage erhalten zusammen 45 Sekunden. Er wiederholt einen
fehlgeschlagenen Aufruf nicht, und dieselben Fragmente können für einen anderen Leser erneut kommen.

<!-- source: searching-posts bd9dfba5a3e7 -->
## Beiträge durchsuchen

Ein Plugin der Art `search` pflegt den Index der [Volltextsuche für Beiträge](search.md) in einer Engine, für die der Server
keine eingebaute Unterstützung hat, etwa Groonga oder PGroonga, einen mit Sudachi gebauten Index, Typesense, Manticore oder einen gehosteten
Suchdienst. Für den Server ist es eine weitere Engine: Dieselben Beiträge kommen in den Index, dieselben Jobs halten ihn aktuell,
und der Server entscheidet weiterhin, was jede Person sehen darf.

```sh
MY_SEARCH=plugin
MY_PLUGIN_SEARCH_ADDRESS=unix:///run/my/search.sock
```

1. Starte das Plugin und prüfe es mit [`my plugin test --kind search`](#test-a-plugin).
2. Setze `search = plugin` und `plugin_search_address` und folge [Einschalten](search.md#turning-it-on), wie bei jeder
   Engine: Der Index füllt sich von da an, und `my search rebuild` fügt die vorhandenen Beiträge hinzu.
3. Wenn das Plugin die Beiträge an einen Dienst eines anderen Unternehmens schickt, nenne diesen Dienst in deinen
   [Datenschutzdokumenten](search.md#your-privacy-documents).

Der Server öffnet beim Start keine Verbindung, ein ausgefallenes Plugin stoppt also nichts außer der Suche. Um zum
Plugin oder von ihm weg zu wechseln, folge [Die Engine abschalten oder wechseln](search.md#switching-off-or-changing-the-engine).

<!-- source: what-the-plugin-receives-1 d2b09f214f3a -->
### Was das Plugin erhält

- **Die Beiträge des Index.** Für jeden Beitrag, den der Index enthält, erhält das Plugin seinen Text (den Text, die Inhalts-
  warnung und die Bildbeschreibungen, als einfachen Text), seine ID, die Konto-ID seines Autors, seine Sichtbarkeit, seine Sprache
  und wann er veröffentlicht wurde. Das sind nur `public`- und `local`-Beiträge, wie [`search_visibilities`](instance-settings.md)
  es sagt, von Autoren, die die Suche erlauben. `local`-Beiträge sind allein für die Konten dieses Servers gedacht: Das Plugin und jeder
  Dienst dahinter liest sie ebenfalls. `unlisted`-, `private`- und `direct`-Beiträge erreichen es nie.
- **Die Suchanfragen.** Wenn eine angemeldete Person Beiträge durchsucht, erhält das Plugin, was sie getippt hat, mit dem Cursor der
  Seite und der Zahl der Ergebnisse. Es erhält nicht, wer gesucht hat.
- **Die Entfernungen:** eines Beitrags, aller Beiträge eines Kontos, aller Beiträge einer Sichtbarkeit und des ganzen Index.

Das Plugin gibt nur Beitrags-IDs zurück. Der Server liest jeden Beitrag erneut aus seiner Datenbank und zeigt nur, was die Person jetzt
sehen darf, sodass ein Plugin keinem einen Beitrag zeigen kann, der ihn nicht sehen darf, und ein Index, der hinterherhinkt, nur weniger findet. Der
Server zeigt die Fehler eines Plugins außerdem niemandem.

Der Server lehnt für diese Art `http://` ab, auch auf Loopback, weil sie den Text von Beiträgen trägt und das, wonach Menschen
suchen. Verwende einen Unix-Socket oder `https://`.

<!-- source: the-contract-1 d500365750c4 -->
### Der Vertrag

Der Dienst ist `my.plugin.search.v1.SearchService`, mit einem RPC pro Operation:

| RPC | Was er tut |
| --- | --- |
| `Replace` | Fügt das Dokument eines Beitrags hinzu oder ersetzt das Dokument mit derselben `statusId` |
| `Remove` | Entfernt das Dokument eines Beitrags |
| `RemoveByAuthor` | Entfernt jedes Dokument eines Kontos |
| `RemoveByVisibility` | Entfernt jedes Dokument einer Sichtbarkeit, `VISIBILITY_PUBLIC` oder `VISIBILITY_LOCAL` |
| `Clear` | Entfernt jedes Dokument; der Index nimmt danach wieder Dokumente auf |
| `Search` | Gibt eine Seite der IDs der Dokumente zurück, die eine Suchanfrage findet, die neuesten zuerst |

```text
POST /my.plugin.search.v1.SearchService/Replace
Content-Type: application/json

{"document":{"statusId":"019a5f3e-8b2c-7d41-9a3e-2f6c1b8d4e07","authorId":"0199c2d4-1e5f-7a3b-8c9d-0e1f2a3b4c5d","visibility":"VISIBILITY_PUBLIC","language":"ja","publishedAt":"2026-11-07T05:12:30Z","text":"週末に図書館で全文検索の本を読んだ"}}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK"}
```

```text
POST /my.plugin.search.v1.SearchService/Search
Content-Type: application/json

{"text":"全文検索","limit":1}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","statusIds":["019a5f3e-8b2c-7d41-9a3e-2f6c1b8d4e07"],"nextCursor":"019a5f3e-8b2c-7d41-9a3e-2f6c1b8d4e07"}
```

- **Eine Suchanfrage ist einfacher Text.** Das Plugin zerlegt sie in Wörter und findet die Dokumente mit jedem Wort. Es liest den
  Text nie als Abfragesprache seiner Engine: Ein Operator, ein Platzhalter, ein Anführungszeichen oder ein Feldname darin ist nur Text. Eine Suchanfrage
  ohne Wort findet nichts. Wie das Plugin Wörter auf Japanisch und in anderen Sprachen findet, die ohne Leerzeichen geschrieben werden, ist seine eigene
  Entscheidung und der Grund, eines zu haben.
- **Eine Seite hat die neuesten zuerst.** `statusIds` sind in absteigender Reihenfolge, jede höchstens einmal, höchstens `limit` davon, und jede eine
  Status-ID, wie der Server sie gesendet hat (36 Kleinbuchstaben). Die IDs sind UUIDv7, ihre Reihenfolge als Strings ist also die Reihenfolge der
  Zeit.
- **Der Cursor gehört dem Plugin.** `nextCursor` ist auf der letzten Seite leer und sonst höchstens 256 Bytes druckbares
  ASCII ohne Leerzeichen, etwa die letzte ID der Seite. Der Server sendet ihn für die nächste Seite unverändert zurück, die
  nach dieser ID weitergeht, auch wenn Dokumente hinzukamen und verschwanden. Eine Seite, die nicht die letzte ist, kann weniger als `limit` IDs enthalten. Ein Cursor,
  den das Plugin nicht ausgegeben hat, erhält `OUTCOME_INVALID_CURSOR`.
- **Ein Schreibvorgang zeigt sich in der nächsten Suche**, und jeder Aufruf kann zweimal kommen: Das Entfernen von etwas, das nicht da ist, gelingt. Eine Engine, die
  Schreibvorgänge erst nach einem Refresh zeigt, lässt das Plugin darauf warten.

| `outcome` | Bedeutung | Was der Server tut |
| --- | --- | --- |
| `OUTCOME_OK` | Der Aufruf tat, was er verlangt | Wendet die Sichtbarkeitsrichtlinie auf die IDs einer Seite an |
| `OUTCOME_FAILED` | Die Engine ist ausgefallen oder hat nicht geantwortet | Wiederholt einen Schreibvorgang mit seinem Job; lässt eine Suche fehlschlagen, die die Person erneut versuchen kann |
| `OUTCOME_INVALID_CURSOR` | Nur `Search`: Der Cursor ist keiner, den das Plugin ausgegeben hat | Sagt dem Client, dass sein Cursor falsch ist |

Ein fehlendes oder unbekanntes `outcome`, ein fehlgeschlagener Aufruf und eine Seite, die diese Regeln verletzt, zählen als `OUTCOME_FAILED`, und nichts
von einer solchen Seite wird angezeigt. Ein Aufruf dauert höchstens `plugin_search_timeout` (standardmäßig 10 Sekunden) und 10 Sekunden, was auch immer diese
Einstellung sagt.

<!-- source: converting-media 36df3dbdb21b -->
## Medien konvertieren

Ein Medien-Plugin erledigt den schweren Teil der Verarbeitung von Uploads, sodass sie auf einer anderen Maschine laufen kann. Es gibt drei Arten,
und du wählst jede für sich:

- `media_image` kodiert Bilder in ihre Varianten neu.
- `media_video` remuxt Videos: Dateien mit einer Videospur, mit oder ohne Ton.
- `media_audio` remuxt reine Audiodateien.

Der Server erledigt den Rest weiterhin selbst. Er erkennt das Format jedes Uploads, unterscheidet Videos von Audiodateien, prüft jeden
Upload bei seinem Eintreffen gegen die Limits und erstellt die Vorschauen von Remote-Medien. Eine Art, deren Einstellung `native` bleibt, wird
im Server verarbeitet.

```sh
# Only videos go to a plugin, on a machine of its own; images and audio stay in the server.
MY_MEDIA_VIDEO=plugin
MY_PLUGIN_MEDIA_VIDEO_ADDRESS=https://media.internal:9000
```

Setze `media_<kind> = plugin` und `plugin_media_<kind>_address` für jede Art, die du übergibst, und starte neu. Ein Plugin kann
mehr als eine Art übernehmen: Gib ihm für jede dieselbe Adresse. Jede Art führt höchstens zwei Aufrufe gleichzeitig aus, und jeder Aufruf dauert höchstens
`plugin_media_<kind>_timeout` (standardmäßig 4 Minuten), sodass ein langsames Video die Bilder nicht aufhält.

- **Das Entfernen von Metadaten ist Aufgabe des Plugins.** Die Uploads erreichen das Plugin so, wie sie gesendet wurden, mit ihren EXIF-Daten und
  Tags, die enthalten können, wo ein Foto aufgenommen wurde. Der Server prüft das Format und die Größe jeder Ausgabe, kann aber nicht
  erkennen, dass jede Spur von Metadaten weg ist. `my plugin test --kind media_image` prüft, dass der Standort seiner Testdateien
  nicht zurückkommt; prüfe den Rest im Plugin.
- **Einfaches TCP sendet die Uploads unverschlüsselt.** Die Medienarten akzeptieren `http://`, mit einer Warnung beim Start. Uploads mit ihren
  Metadaten überqueren das Netzwerk dann im Klartext, und nichts authentifiziert eines der beiden Enden: Jeder in diesem Netzwerk kann sie
  lesen oder für das Plugin antworten. Verwende einen Unix-Socket oder `https://`, es sei denn, das Netzwerk gehört dir allein.
- **Der Server vertraut der Ausgabe nicht.** Er erkennt das Format jeder Ausgabe an ihrem Inhalt, nie an dem, was das
  Plugin angibt, und liefert sie mit dem Content-Type und der Dateiendung dieses Formats aus. Er dekodiert jede Ausgabe unter
  denselben Limits wie einen Upload und prüft, dass sie in ihre Größe passt. Eine Ausgabe, die größer ist als angegeben oder als diese
  Limits, ein Format hat, das das Plugin nicht angegeben hat, oder ein Format, das die Variante nicht aufnimmt, lässt den Upload als eine Datei fehlschlagen,
  die der Server nicht verarbeiten kann.
- **Ein Fehler ist eine Verzögerung.** Wenn das Plugin ausgefallen ist oder fehlschlägt, versucht der Job es später erneut, wie beim eingebauten Prozessor. Eine
  Aktion, die das Bild eines eigenen Emojis sofort braucht, etwa das Kopieren eines Remote-Emojis oder das Annehmen einer Anfrage dafür, schlägt fehl,
  solange das Plugin es tut.

<!-- source: the-contract-2 812c987c276c -->
### Der Vertrag

Der Dienst ist `my.plugin.media.v1.MediaService`, mit den RPCs `Process` für ein Bild und `Remux` für eine Video- oder Audiodatei.
Beide streamen in beide Richtungen, das Plugin braucht also eine Connect- oder gRPC-Bibliothek und HTTP/2: `proto` oder `grpc`, über TLS mit ALPN
`h2` oder über unverschlüsseltes HTTP/2 (h2c) auf einem Unix-Socket oder einfachem TCP.

Ein Aufruf ist halbduplex:

1. Der Server sendet einen `head`: die Limits und, bei einem Bild, seinen Zweck, die Boxen seiner drei Größen und seinen Fokus-
   punkt. Die Datei folgt in `chunk`-Nachrichten von höchstens 1 MiB. Der Server schließt dann seine Seite.
2. Das Plugin antwortet mit einem `head`, dessen `outcome` das Ergebnis ist. Mit `OUTCOME_OK` enthält der Head von `Process` außerdem die
   Abmessungen des Bildes nach seiner EXIF-Ausrichtung und den Blurhash der kleinen Standbild-Ausgabe.
3. Für jede Ausgabe sendet das Plugin einen `part`, der die Ausgabe benennt und ihr Format, ihre Abmessungen und ihre Länge angibt,
   gefolgt von `chunk`-Nachrichten von höchstens 1 MiB.

`Process` gibt die kleine, mittlere und große Standbild-Ausgabe zurück und die drei bewegten eines Bildes mit mehr als einem
Frame. Die kleine und die mittlere Ausgabe sind WebP; die große Standbild-Ausgabe ist ein JPEG oder ein PNG (bei einem eigenen Emoji immer ein PNG),
und die große bewegte ein GIF. `Remux` gibt die Datei in ihrem eigenen Container ohne Metadaten zurück und das Titelbild, das sie
einbettet, falls vorhanden. Der Server liest von `Process` höchstens das Doppelte des Bildlimits und von `Remux` das Videolimit für die
Datei und das Bildlimit für ihr Cover.

| `outcome` | Bedeutung | Was der Server tut |
| --- | --- | --- |
| `OUTCOME_OK` | Die Ausgaben folgen | Prüft sie und speichert sie |
| `OUTCOME_TEMPORARY` | Das Plugin ist fehlgeschlagen | Versucht den Job später erneut |
| `OUTCOME_UNSUPPORTED_FORMAT`, `OUTCOME_UNSUPPORTED_CODEC`, `OUTCOME_TOO_LARGE`, `OUTCOME_DIMENSIONS`, `OUTCOME_CORRUPT`, `OUTCOME_TOO_LONG` | Die Datei ist nicht akzeptabel | Markiert den Upload als fehlgeschlagen |

Ein fehlendes oder unbekanntes `outcome` und ein fehlgeschlagener Aufruf zählen als `OUTCOME_TEMPORARY`.

<!-- source: a-plugin-with-no-library be054eb80e74 -->
## Ein Plugin ohne Bibliothek

Mit `json` braucht ein Plugin allein einen HTTP-Server und eine JSON-Bibliothek. Ein RPC ist ein `POST` an
`/<package>.<Service>/<Method>` mit `Content-Type: application/json`, und das Plugin antwortet mit `200` und einem JSON-Objekt:

```text
POST /my.plugin.probe.v1.ProbeService/Echo
Content-Type: application/json
Connect-Protocol-Version: 1

{"payload":"AP8=","number":"9007199254740993"}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","payload":"AP8=","number":"9007199254740993"}
```

Das JSON folgt dem Standard-Mapping von proto3:

- Ein `int64` ist ein Dezimal-String, `bytes` sind Base64, und ein Enum ist sein Name.
- Ein Feld mit seinem Nullwert darf fehlen, in beide Richtungen. Ein Plugin muss ein fehlendes Feld als null lesen.
- Der Server ignoriert Felder, die er nicht kennt.
- Ein Ergebnis ist das `outcome` der Antwort. Ein fehlendes oder unbekanntes `outcome` ist ein Fehler, nie ein Erfolg.
- Ein Plugin, das fehlschlägt, antwortet mit einem HTTP-Fehler oder mit `{"code":"unavailable"}` von Connect mit dem passenden Status. Der Job versucht es erneut.

`Connect-Timeout-Ms` und `Connect-Protocol-Version` dürfen ignoriert werden.
