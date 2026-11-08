<!-- source: overview-of-a-mycroblog-server ce703d24935f -->
# Überblick über einen mycroblog-Server

Die erste Seite, die du liest, bevor du einen Server betreibst. mycroblog ist ein Microblog, der über ActivityPub mit dem Rest des Fediverse
föderiert; `my` ist seine Server-Software, eine einzige Binary, die den Server, den Job-Worker, den Web-Client und die Befehle enthält, die sich um sie
kümmern. Diese Seite zeigt, woraus ein Server besteht, was du per Konfiguration wählst, wie ein kleiner Server zu einem großen wächst
und wo du als Nächstes weiterliest.

<!-- source: what-a-server-is-made-of 1210fe6ac0c4 -->
## Woraus ein Server besteht

![Die Architektur eines mycroblog-Servers: Clients erreichen die my-Prozesse über HTTPS; die Prozesse nutzen die per Konfiguration gewählten Teile und die Dienste außerhalb des Servers, die du einschaltest. Die Abschnitte unten beschreiben jeden Teil.](images/architecture.svg)

Vom oberen Rand des Diagramms nach unten:

- **Wer sich verbindet.** Menschen nutzen den Web-Client in ihrem Browser (er lässt sich als App installieren), Apps und Bots nutzen die
  Client-API, und andere Fediverse-Server stellen Beiträge über ActivityPub zu und rufen sie ab. Alle erreichen den Server über
  HTTPS.
- **Vor dem Server.** `my` spricht einfaches HTTP, daher beendet ein Reverse-Proxy oder Load Balancer davor das HTTPS.
  Ein CDN vor den Mediendateien ist optional.
- **Die `my`-Prozesse.** Jeder Prozess führt `my serve` aus, und seine [`role`](../reference/configuration.md#role) bestimmt, was er
  ausführt: `web` den HTTP-Server, `worker` den Job-Worker, der Beiträge zustellt, von anderen Servern abruft, E-Mails sendet,
  Medien verarbeitet und die periodischen Jobs ausführt, und `all`, der Standard, beides in einem Prozess.
- **Die Teile, die du wählst.** Jeder Teil in der Mitte des Diagramms hat eine Einstellung, die seinen Adapter wählt, aufgelistet im
  [nächsten Abschnitt](#parts-you-choose-by-configuration).
- **Dienste außerhalb des Servers.** CAPTCHA, Sprach-Chat, Zahlungen, Plugins und der Import von Cloud-Kosten bleiben jeweils aus,
  bis du sie einstellst, und der Server ruft dann den Dienst auf, den du nennst.

<!-- source: parts-you-choose-by-configuration f7dbfce7a91c -->
## Teile, die du per Konfiguration wählst

Jeder Teil ist eine Einstellung der [Konfigurationsreferenz](../reference/configuration.md), gesetzt in einer TOML-Datei oder als
Umgebungsvariable (`MY_` und der Schlüssel in Großbuchstaben, etwa `MY_DATABASE`). Die erste Option jeder Liste ist der Standard.
Wo ein Teil Daten hält, wähle ihn vor dem ersten Start: Die Daten ziehen nicht von selbst um, wenn sich die Einstellung ändert.

| Teil | Einstellung | Optionen | Weiterlesen |
| --- | --- | --- | --- |
| Datenbank | [`database`](../reference/configuration.md#database) | `sqlite` (eine Datei), `postgres` (PostgreSQL und kompatible Dienste) | [Datenbank](database.md), [Backup](backup.md) |
| Job-Queue | [`queue`](../reference/configuration.md#queue) | `db` (eine Tabelle in der Datenbank), `inline` (führt jeden Job sofort im selben Prozess aus, für Tests und Entwicklung), `cloudtasks` (Google Cloud Tasks weckt den Server für jeden Job; braucht PostgreSQL) | [Die Job-Queue](running.md#the-job-queue-and-failed-dead-jobs) |
| Key-Value-Speicher, für Caches und Ratenbegrenzungen | [`kv`](../reference/configuration.md#kv) | `db` (eine Tabelle in der Datenbank), `memory` (jeder Prozess für sich), `valkey` (Valkey oder ein anderer Server mit Redis-Protokoll, von allen Prozessen geteilt) | [`kv`](../reference/configuration.md#kv) |
| Medienspeicher | [`blob`](../reference/configuration.md#blob) | `fs` (ein Verzeichnis), `s3` (Amazon S3, Cloudflare R2, MinIO, Garage und andere S3-kompatible Speicher), `gcs` (Google Cloud Storage) | [Docker-Compose-Dateien](running.md#docker-compose-files), [Medien sichern](backup.md#media-in-a-bucket) |
| Medienverarbeitung | [`media_image`](../reference/configuration.md#media_image), [`media_video`](../reference/configuration.md#media_video), [`media_audio`](../reference/configuration.md#media_audio), je eines für jede Art von Datei | `native` (im Server), `plugin` (ein Plugin von dir konvertiert die Dateien dieser Art) | [Medien konvertieren](plugins.md#converting-media) |
| Echtzeit-Stream zwischen Prozessen | [`stream`](../reference/configuration.md#stream) | `hub` (innerhalb eines Prozesses), `postgres` (`LISTEN` / `NOTIFY`), `valkey`, `sns` (Amazon SNS und SQS), `pubsub` (Google Cloud Pub/Sub) | [`stream`](../reference/configuration.md#stream) |
| Key Manager, der die privaten Schlüssel der Konten verschlüsselt | [`key_manager`](../reference/configuration.md#key_manager) | `local` (ein Key Encryption Key, den du aufbewahrst), `gcpkms` (Google Cloud KMS), `awskms` (AWS KMS), `plugin` | [Den Schlüssel verwalten](key-management.md) |
| E-Mail | [`mail`](../reference/configuration.md#mail) | `none`, `smtp`, `ses` (Amazon SES), `plugin` | [E-Mails versenden](email.md) |
| CAPTCHA | [`captcha`](../reference/configuration.md#captcha) | `none`, `turnstile` (Cloudflare Turnstile), `hcaptcha`, `recaptcha` (Google reCAPTCHA v2) | [CAPTCHA](captcha.md) |
| Sprach-Chat | [`voice`](../reference/configuration.md#voice) | `off`, `p2p` (Browser zu Browser, mit TURN aus `voice_turn`: `none`, `coturn` oder `cloudflare`), `livekit`, `sora` | [Sprach-Chat](voice.md) |
| Zahlungen von Unterstützern | [`payments`](../reference/configuration.md#payments) | `none`, `stripe`, `payjp` (PAY.JP) | [Unterstützer](supporters.md), [Stripe](payments-stripe.md), [PAY.JP](payments-payjp.md) |
| Plugins | `plugin_<kind>_address` | standardmäßig keine; eine Adresse für jede Art: `translator`, `search`, `billing`, `mailer`, `media_image`, `media_video`, `media_audio`, `key_manager` und die Entscheidungs-Hooks `inbound.status`, `local.status`, `inbound.follow`, `signup.check`, `media.check`, `inbound.actor`, `local.profile` | [Plugins](plugins.md), [Entscheidungs-Hooks](decision-hooks.md) |
| Übersetzung von Beiträgen | `translator_<provider>_*`, [`plugin_translator_address`](../reference/configuration.md#plugin_translator_address) | keine (nur die eigene Übersetzung des Browsers); `libretranslate`, `deepl`, `google` (Google Cloud Translation), `aws` (Amazon Translate) oder ein Plugin von dir vor dem Dienst deiner Wahl | [Beiträge übersetzen](translation.md), [ein Übersetzer-Plugin](plugins.md#translating-posts) |
| Volltextsuche in Beiträgen | [`search`](../reference/configuration.md#search) | `none`; oder `elasticsearch`, `opensearch` oder `meilisearch` ([Suchmaschinen-Backends](search-engines.md)); oder `plugin`, ein Plugin von dir vor einer anderen Engine | [Volltextsuche](search.md), [ein Such-Plugin](plugins.md#searching-posts) |
| CDN für Medien | [`media_origin`](../reference/configuration.md#media_origin) | nicht gesetzt (der Server liefert Medien von seiner eigenen Origin aus) oder die Origin deines CDN | [Medien über ein CDN ausliefern](cdn.md) |
| Cloud-Kosten | [`billing_sources`](../reference/configuration.md#billing_sources) | keine oder beliebige von `aws` (AWS Cost Explorer), `gcp` (der Billing-Export oder das Budget von Google Cloud), `plugin` | [Cloud-Kosten importieren](billing-import.md) |
| Metriken | [`metrics_exporter`](../reference/configuration.md#metrics_exporter) | `none`, `prometheus`, `otlp` (OpenTelemetry), `emf` (CloudWatch) | [Metriken](running.md#metrics), [Alarme](alerts.md) |
| Traces | [`traces_exporter`](../reference/configuration.md#traces_exporter) | `none`, `otlp` | [Traces](running.md#traces) |
| Fehlerberichte | [`errors_exporter`](../reference/configuration.md#errors_exporter) | `none` (nur die Logzeile und die Fehlergruppen), `sentry` (Sentry, GlitchTip, Datadog), `otlp` | [Fehlerberichte](error-reporting.md) |

Die veröffentlichte Binary und das Container-Image enthalten jeden Adapter der Tabelle. Eine Binary, die du selbst baust, kann die,
die du nicht nutzt, mit [Build-Tags](../reference/build-tags.md) weglassen und lehnt ihre Einstellungen dann beim Start ab.

`my config check` meldet alle Probleme der Konfiguration auf einmal, bevor du den Server startest
([Checkliste für den Produktivbetrieb](production-checklist.md)).

<!-- source: from-a-small-server-to-a-large-one 42bbd3f2a76e -->
## Von einem kleinen Server zu einem großen

![Drei Schritte von einem kleinen zu einem großen Server: ein Prozess auf SQLite und dem Dateisystem, dann PostgreSQL und Objektspeicher hinter einem CDN, dann getrennte Web- und Worker-Prozesse mit einem Stream-Broker. Die Liste unten beschreibt jeden Schritt.](images/growth.svg)

1. **Klein anfangen.** Ein Prozess mit `role = all`, auf SQLite, mit Medien im Dateisystem und den Standardwerten für den
   Rest: die Queue und der Key-Value-Speicher in der Datenbank und der Echtzeit-Stream innerhalb des Prozesses. Die Docker-
   Compose-Dateien in `deploy/docker` starten dieses Setup auf einer Maschine ([Deployment mit Docker Compose](deploy-docker-compose.md)).
   Es eignet sich für einen persönlichen oder kleinen Server. `stream = hub` erreicht nur den Prozess, in dem er läuft, daher bleibt dieses Setup bei einem
   Prozess.
2. **Die Daten auslagern.** PostgreSQL für die Datenbank, ein S3-kompatibler Bucket oder Google Cloud Storage für die Medien
   und ein CDN vor den Medien. Der Server ist weiterhin ein Prozess, hängt aber nicht mehr an einer Festplatte, und die
   Prozesse des nächsten Schritts können die Daten teilen. Entscheide dich dafür möglichst vor dem ersten Start: Die Daten
   später von SQLite und dem Dateisystem umzuziehen, ist eine eigene Aufgabe ([Was zu sichern ist](backup.md#what-to-back-up) listet auf,
   was umziehen muss).
3. **Horizontal skalieren.** Betreibe Prozesse mit `role = web` für die Anfragen und `role = worker` für die Jobs, so viele, wie jede Rolle
   braucht, und teile die Worker nach Job-Typ auf ([Worker aufteilen](running.md#splitting-workers-by-job-type)).
   Mit mehr als einem Prozess braucht der Echtzeit-Stream einen Broker, der jeden Prozess erreicht (`postgres`, `valkey`,
   `sns` oder `pubsub`), und der Key-Value-Speicher sollte geteilt sein (`db` oder `valkey`), da `memory` die Ratenbegrenzungen
   jedes Prozesses getrennt hält. Die Job-Queue bleibt in
   PostgreSQL oder zieht zu Google Cloud Tasks um ([Cloud Tasks](running.md#cloud-tasks)).

Das [Deployment auf Cloud Run](deploy-cloud-run.md) ist ein Beispiel für ein Setup zwischen dem zweiten und dem dritten Schritt:
`role = all` auf Cloud Run, PostgreSQL auf Neon, Medien in Cloud Storage, Cloud Tasks für die Jobs und Pub/Sub für den
Stream, ohne laufenden Server, solange niemand den Server nutzt.

<!-- source: where-to-go-next 2f1d25c01f8a -->
## Wie es weitergeht

Lies die Betreiber-Leitfäden in dieser Reihenfolge:

1. **Loslegen:** [den Server betreiben](running.md) oder [auf Cloud Run deployen](deploy-cloud-run.md), dann die
   [Checkliste für den Produktivbetrieb](production-checklist.md) durchgehen, bevor du ihn für Nutzer öffnest.
2. **Den Server konfigurieren:** ihre [Einstellungen](instance-settings.md), die [Registrierung](registration.md),
   [externe Login-Anbieter](login-providers.md), [E-Mail](email.md), [CAPTCHA](captcha.md), [das CDN](cdn.md),
   [Linkvorschauen](link-previews.md) und [Suchmaschinen und KI-Crawler](crawlers.md).
3. **Funktionen einschalten:** [Moderation](moderation.md), [Relays](relays.md), [Webhooks](webhooks.md),
   [Plugins](plugins.md) und [Entscheidungs-Hooks](decision-hooks.md), [Sprach-Chat](voice.md), [Volltextsuche](search.md),
   [Unterstützer](supporters.md) und ihre Zahlungen sowie [Cloud-Kosten importieren](billing-import.md).
4. **Im Alltag betreiben:** [Überwachung](monitoring.md), [Alarme](alerts.md), [Fehlerberichte](error-reporting.md),
   [Wartung](maintenance.md), [Backup](backup.md), [die Datenbank](database.md),
   [den Key Encryption Key](key-management.md) und [Administratoren und Rollen](admin-cli.md).
5. **Rechtliches und Schließung:** die Vorlagen für [Datenschutzerklärung](privacy-policy-template.md) und
   [Verarbeitungsverzeichnis](processing-record-template.md) sowie [den Dienst schließen](service-closure.md).

Die [Konfigurationsreferenz](../reference/configuration.md) listet jede Einstellung auf, und die [Job-Referenz](../reference/jobs.md)
jeden Job, den die Worker ausführen.
