<!-- source: privacy-policy-template 9966bbc6844c -->
# Vorlage für die Datenschutzerklärung

Eine Datenschutzerklärung zum Anpassen für deinen Server: Sie beschreibt, was dieser Server speichert, warum, wie lange und wer es erhält.

**Dies ist eine Vorlage und keine Rechtsberatung.** Sie beschreibt, was die Software mit ihren Standardeinstellungen tut. Sie kann nicht wissen, wie du deinen Server betreibst, und sie entscheidet nicht, welche Gesetze für dich gelten. Was eine Erklärung sagen muss, hängt von deiner Rechtsordnung und von der deiner Nutzer ab (zum Beispiel die DSGVO in der EU, die britische UK GDPR oder Japans Gesetz zum Schutz personenbezogener Daten). Lass sie für deine Rechtsordnung prüfen, bevor du sie veröffentlichst, und behandle die Rechtsgrundlagen und die Erklärung zu Übermittlungen unten als Fragen, die zu klären sind, nicht als Antworten.

Das zugehörige Verzeichnis von Verarbeitungstätigkeiten findest du unter [Vorlage für das Verzeichnis von Verarbeitungstätigkeiten](processing-record-template.md).

<!-- source: how-to-use-it 9cb843cc1ba2 -->
## So verwendest du sie

1. Kopiere [die Vorlage](#the-template) in eine Datei, `privacy-policy.md`.
2. Ersetze jedes `{{...}}`. Ein Wert in den Klammern, wie `{{180 days}}`, ist der Standard der Software: Bestätige ihn anhand deines Servers ([Die Fristen und ihre Einstellungen](#the-periods-and-their-settings)) und entferne die Klammern.
3. Eine Zeile oder ein Satz, die mit `{{IF ...}}` beginnen, gelten nur für dieses Setup ([Die Dienste, die du wählst](#the-services-you-choose)). Lösche sie, wenn sie nicht zutreffen. Wenn sie zutreffen, entferne die Markierung.
4. Prüfe, dass nichts übrig ist: `grep -n '{{' privacy-policy.md` gibt keine Zeile aus.
5. Veröffentliche sie mit `my instance set privacy_policy - < privacy-policy.md` oder auf der Seite „Datenschutzerklärung“ unter „Bedingungen und Richtlinien“ (`/admin/documents/privacy-policy`) in der Administration. Leser finden sie unter `/about/privacy` ([Servereinstellungen](instance-settings.md#terms-and-privacy-policy)).

Eine geschriebene Datenschutzerklärung fügt dem Registrierungsbildschirm ein verpflichtendes Zustimmungs-Kontrollkästchen hinzu. Dieses Kontrollkästchen zeigt, dass jemand das Dokument gelesen hat. Gehe nicht davon aus, dass es eine Einwilligung in die Verarbeitung ist, wo dein Recht eine Einwilligung verlangt.

Der Text hat für jede Sprache denselben Wert. Wenn du Leser in mehreren Sprachen hast, schreibe die Sprachen, die du brauchst, in den einen Text.

Prüfe die Erklärung, wenn du eine Einstellung unten oder einen Dienst änderst, und nach jedem Upgrade, dessen Release Notes personenbezogene Daten erwähnen.

<!-- source: the-periods-and-their-settings 111d5a4b6af8 -->
## Die Fristen und ihre Einstellungen

`my instance list` zeigt jede Servereinstellung und ob sie dem Standard entspricht. Eine Änderung einer Frist wirkt beim nächsten Lauf ihres stündlichen Jobs. Werte außerhalb des Bereichs werden abgelehnt.

| Datensatz | Standard | Servereinstellung | Bereich (Tage) | Beginnt mit |
| --- | --- | --- | --- | --- |
| IP-Adresse und User-Agent bei der Anmeldung | 180 Tage | `session_origins_retention_days` | 90-3650 | Der Anmeldung |
| IP-Adresse bei der Registrierung | 180 Tage | `registration_origins_retention_days` | 90-3650 | Der Registrierung |
| Meldungen | 365 Tage | `reports_retention_days` | 180-3650 | Der letzten Erledigung |
| Maßnahmen gegen Konten und Einsprüche | 365 Tage | `moderation_actions_retention_days` | 180-3650 | Dem Ende der Maßnahme |
| Audit-Log | 730 Tage | `audit_log_retention_days` | 365-3650 | Dem Datensatz |
| Wer an einem Sprachchat-Raum teilgenommen hat | 90 Tage | `voice_participants_retention_days` | 30-3650 | Dem Ende des Raums |
| Die Entscheidungen einer externen Prüfung, wenn du eine nutzt | 90 Tage | `hook_decisions_retention_days` | 30-3650 | Der Entscheidung |
| Remote-Beiträge, auf die hier nichts verweist | 90 Tage | `remote_status_retention_days` | 1-3650 | Der Veröffentlichung des Beitrags |
| Jeder Remote-Beitrag | Kein Limit (`0`) | `remote_status_max_retention_days` | 0-3650 | Der Veröffentlichung des Beitrags |
| Remote-Beiträge, die nur ein Relay gebracht hat | 7 Tage | `relay_status_retention_days` | 1-3650 | Der Veröffentlichung des Beitrags |
| Kopien von Remote-Medien | 1 Tag, und keine werden aufbewahrt, solange `remote_media_policy` `none` ist | `remote_media_cache_ttl_seconds`, `remote_media_policy` | 60-31536000 Sekunden | Dem Abruf |
| Übersetzungen von Beiträgen | 30 Tage | `translation_retention_days` | 1-3650 | Der Übersetzung. Das Lesen verlängert die Frist nicht, und sie verschwindet, wenn der Beitrag gelöscht wird |

Dies sind Einstellungen des Deployments ([Konfigurationsreferenz](../reference/configuration.md)), keine Servereinstellungen:

| Datensatz | Standard | Einstellung |
| --- | --- | --- |
| Uploads, die an nichts angehängt sind | 24 Stunden | `media_unattached_retention` |
| Alte Versionen von Bildern, nachdem eine neuere Version sie ersetzt hat | 30 Tage | `media_replaced_retention` |
| Jobs, die endgültig fehlgeschlagen sind | 30 Tage | `dead_job_retention` |
| Aufzeichnungen von Job-Läufen, die nicht erfolgreich waren | 30 Tage | `job_attempt_retention` |

Diese Fristen legt die Software fest, und keine Einstellung ändert sie:

| Datensatz | Frist |
| --- | --- |
| Ein gelöschtes Konto endgültig löschen | 7 Tage nach der Löschung; bis zu 30 Tage bei einem Remote-Konto, wenn nur ein `410 Gone` von seinem Server das gesagt hat und der Server nicht erneut antwortet |
| Ein deaktiviertes Konto löschen | 30 Tage nach der Deaktivierung |
| Registrierungssperren | 30 Tage nach der Aufhebung oder dem Ablauf; ohne beides, solange sie gelten |
| Sitzungen, Tokens und Autorisierungscodes | 30 Tage nach ihrem Ablauf |
| Archive von Datenexporten und Datenimporte | 7 Tage |
| Aufzeichnungen von Aktivitäten, die von anderen Servern empfangen wurden | 30 Tage, und das Dokument selbst 7 Tage |
| Aufzeichnungen von Aktivitäten, die an andere Server zugestellt wurden | 30 Tage; die Zustellung eines Beitrags bis 30 Tage nach dem Löschen des Beitrags oder seines Autors |
| Prüfungen von Remote-Handles | 90 Tage, wobei die neueste jedes Kontos behalten wird |
| Webhook-Nachrichten und -Zustellungen | 14 Tage |
| Belege von Zahlungs-Webhooks | 45 Tage |
| Stündliche Fehlerzählungen und Fehlerhinweise | 90 Tage |

Eine [Aufbewahrungssperre](moderation.md#holds) stoppt das Löschen für ein Konto, sodass jede der obigen Fristen für ein Konto unter einer Sperre länger laufen kann. Wenn du Sperren nutzt, sage das, wie es die Vorlage tut.

<!-- source: records-with-no-automatic-end 90a13d1f648e -->
## Datensätze ohne automatisches Ende

Der Server entfernt diese nur, wenn ein Konto gelöscht wird, oder nie. Ein Gesetz kann von dir verlangen, ihre Aufbewahrung zu rechtfertigen oder sie auf anderem Weg zu entfernen; entscheide also, was deine Erklärung zu jedem sagt.

- Die Zeilen eines aktiven Kontos: E-Mail-Adressen, frühere Profilversionen, frühere Beitragsversionen, Benachrichtigungen, Follows und der Registrierungsantrag eines abgelehnten Nutzers. Ein Konto, das niemand löscht, bleibt bestehen.
- Was nach dem Löschen bleibt: die URI und der Benutzername des Kontos (damit keines von beiden wiederverwendet werden kann), seine Lebenszyklus-Ereignisse, der Vermerk, dass ein Beitrag gelöscht wurde, Moderationsdatensätze bis zum Ende ihrer eigenen Frist, Einladungen und Rollenvergaben, Kontoumzüge, Sprachchat-Räume ohne ihre Namen und Beschreibungen.
- Unterstützer-Datensätze (Abos, Zustandsänderungen, Zahlungen, Erstattungen, Rückbuchungen), aufbewahrt für die Buchhaltung und den Abgleich mit dem Anbieter. Der Server löscht keinen Kunden beim Zahlungsanbieter: Tu das im Dashboard des Anbieters, wenn deine Pflichten es erlauben.
- Eine Maßnahme ohne Ende (ein `ban` oder ein `suspend` ohne Endzeit), bis sie aufgehoben wird. Offene Meldungen.
- Remote-Konten, bis ihr Server sie löscht.
- Der jüngste Bericht jeder Fehlergruppe, den der nächste Bericht dieser Gruppe ersetzt. Er wird nach Mustern bereinigt, sodass ein Fragment von Nutzertext bleiben kann.
- Backups, deine Logs und die Logs deiner Plattform und deines Proxys. Der Server verwaltet diese nicht: [Backup](backup.md#how-often-and-where) beschreibt die aufzubewahrenden Generationen, und ein wiederhergestelltes Backup bringt gelöschte Daten zurück.

<!-- source: the-services-you-choose 81461216c966 -->
## Die Dienste, die du wählst

Jeder Dienst, der personenbezogene Daten erhält, erscheint nur in der Erklärung, wenn du ihn eingeschaltet hast. Fülle die Tabelle von Abschnitt 6 anhand dieser Liste und lies die eigene Datenschutzerklärung und den Vertrag des Dienstes: Was der Dienst mit den Daten tut, nachdem sie angekommen sind, kann dieses Dokument dir nicht sagen.

| Markierung | Einstellung | Was der Dienst erhält |
| --- | --- | --- |
| Hosting | `database`, `blob`, `queue`, `key_manager` | Alles, was die Datenbank und der Blob-Speicher enthalten, in der Form, in der der Anbieter es speichert. Job-Payloads in einer Cloud-Queue enthalten IDs und Kategorien, ohne Adressen, Nachrichtentexte oder Secrets. |
| `{{IF cdn}}` | `media_origin` | IP-Adressen der Leser und Anfragen nach Medien ([CDN](cdn.md)). |
| Proxy | Dein Reverse-Proxy oder Load Balancer, den der Server für TLS braucht | Jede Anfrage mit ihrer IP-Adresse, je nach dem Logging, das du konfigurierst. |
| `{{IF mail}}` | `mail` ist `smtp` oder `ses` | Empfängeradressen, Betreffs und Texte. E-Mail-Benachrichtigungen tragen denselben Text wie die Benachrichtigung ([E-Mail](email.md)). |
| `{{IF captcha}}` | `captcha` ist nicht `none` | Die IP-Adresse, Browser- und Geräteinformationen, Cookies und Widget-Interaktionen jedes Menschen, der die Anmelde-, Registrierungs- oder Passwort-Zurücksetzen-Seite öffnet, direkt aus dem Browser. Der Server sendet dem Dienst nur das Token und fügt keine IP-Adresse hinzu ([CAPTCHA](captcha.md#what-to-write-in-the-privacy-policy)). |
| `{{IF login providers}}` | [Login-Anbieter](login-providers.md) | Der Anbieter sieht die Anmeldung. Der Server erhält die Kennung des Anbieters für die Person, speichert sie und verwendet bei der Registrierung die E-Mail-Adresse und den Benutzernamen, die der Anbieter liefert. |
| `{{IF stripe}}` | `payments = stripe` | Die E-Mail-Adresse, die Checkout vorausfüllt, der Unterstützungsplan und die lokale Abo-ID. Stripe erfasst die Zahlungsdaten auf seinen eigenen Seiten ([Stripe](payments-stripe.md#card-data-and-pci-dss)). |
| `{{IF payjp}}` | `payments = payjp` | Die E-Mail-Adresse und der Unterstützungsplan. Der Browser lädt payjp.js auf den Zahlungsseiten des Servers, die Karte kommt in den iframe von PAY.JP, und der Name und die E-Mail-Adresse gehen für 3-D Secure an PAY.JP ([PAY.JP](payments-payjp.md#card-data-and-pci-dss)). |
| `{{IF error reports}}` | `errors_exporter` | Bereinigte Berichte ohne Konto, IP-Adresse, Body oder Header. Das Scrubbing arbeitet mit Mustern, sodass ein Fragment von Nutzertext bleiben kann ([Fehlerberichte](error-reporting.md#what-scrubbing-does-and-does-not-guarantee)). |
| `{{IF telemetry}}` | `metrics_exporter`, `traces_exporter` | Metriken und Spans ohne Konten oder Client-IP-Adressen. Spans ausgehender Anfragen nennen die Hostnamen von Remote-Servern ([Betrieb](running.md)). |
| `{{IF voice media server}}` | `voice` ist `livekit` oder `sora` | IP-Adressen der Teilnehmer, ihr Audio, während sie sprechen, eine Sitzungs-ID und die Raum-ID. Die ID des Kontos wird nicht gesendet ([Sprachchat](voice.md)). |
| `{{IF turn}}` | `voice_stun_urls`, `voice_turn` | STUN- und TURN-Server sehen die IP-Adressen der Teilnehmer, und ein TURN-Server leitet ihr verschlüsseltes Audio weiter. |
| `{{IF webhooks}}` | [Server-Webhooks](webhooks.md) | Dein Empfänger erhält Ereignis-Bodys. Manche tragen die E-Mail-Adresse eines Antragstellers oder den Kommentar einer Meldung. |
| `{{IF follow review}}` | `plugin_inbound_follow_address` ist gesetzt und `hook_inbound_follow_mode` ist nicht `off` | Die Adressen der beiden Konten jedes Follows von einem anderen Server, vom Server gesendet, wenn der Follow eintrifft ([Entscheidungs-Hooks](decision-hooks.md)). Die erfasste Entscheidung behält die Adresse der Aktivität und welches Konto sie gesendet hat. |
| `{{IF sign-up review}}` | `plugin_signup_check_address` ist gesetzt und `hook_signup_check_mode` ist nicht `off` | Die E-Mail-Adresse und IP-Adresse jedes Antragstellers ohne Einladung, vom Server gesendet, wenn er sich registriert ([Entscheidungs-Hooks](decision-hooks.md#sign-ups)). Die erfasste Entscheidung enthält keines von beiden. Nenne den Dienst in Abschnitt 6, es sei denn, du betreibst das Plugin selbst. |
| `{{IF post review}}` | `plugin_local_status_address` ist gesetzt und `hook_local_status_mode` ist nicht `off` | Der Text jedes `public`-, `local`- und `unlisted`-Beitrags eines Kontos, das jünger als `hook_local_status_account_age_days` ist (standardmäßig 7 Tage; `0` ist jedes Konto), so wie getippt und so wie gerendert, mit seiner Inhaltswarnung, Sprache, ob er als sensibel markiert ist, den Umfrageoptionen, Bildbeschreibungen, Hashtags, wie viele Konten er erwähnt, der Adresse, die er erhalten würde, den Adressen der Beiträge, auf die er antwortet und die er zitiert, der Adresse und dem Benutzernamen des Autors und wann das Konto angelegt wurde, vom Server gesendet, wenn der Beitrag verfasst wird, und erneut bei jeder Bearbeitung ([Entscheidungs-Hooks](decision-hooks.md#local-posts)). `private`- und `direct`-Beiträge werden nie gesendet. Die erfasste Entscheidung enthält keinen Text. Ein Autor, der gegen einen abgelehnten Beitrag Einspruch einlegt, sendet dessen Text, den der Server mit der Entscheidung aufbewahrt. Nenne den Dienst in Abschnitt 6, es sei denn, du betreibst das Plugin selbst. |
| `{{IF profile review}}` | `plugin_inbound_actor_address` ist gesetzt und `hook_inbound_actor_mode` ist nicht `off` | Die öffentlichen Profile von Konten auf anderen Servern, jedes Mal, wenn eines sich ändert: der Anzeigename, die Bio, die Profilfelder, die Adressen des Avatars und des Headerbilds (keine Bilder), die Adresse, der Server und der Benutzername des Kontos und ob das Konto hier neu ist ([Entscheidungs-Hooks](decision-hooks.md#profiles-from-other-servers)). Von den Konten auf deinem Server wird nichts gesendet. Die erfasste Entscheidung behält die Adresse des Kontos und einen Digest des Profils. |
| `{{IF profile review}}` | `plugin_local_profile_address` ist gesetzt und `hook_local_profile_mode` ist nicht `off` | Der Anzeigename, die Bio so wie getippt und so wie gerendert, die Profilfelder, ob sich der Avatar oder der Header geändert hat, die Adresse und der Benutzername des Kontos und wann das Konto angelegt wurde, vom Server gesendet, jedes Mal, wenn ein Konto, das jünger als `hook_local_profile_account_age_days` ist (standardmäßig 7 Tage; `0` ist jedes Konto), sie ändert ([Entscheidungs-Hooks](decision-hooks.md#local-profiles)). Es wird kein Bild gesendet. Die erfasste Entscheidung enthält nichts davon. Nenne den Dienst in Abschnitt 6, es sei denn, du betreibst das Plugin selbst. |
| `{{IF external review of posts}}` | `plugin_inbound_status_address` ist gesetzt und `hook_inbound_status_mode` ist nicht `off` | Die Beiträge und Revisionen, die andere Server senden und die sich auf ein Konto hier beziehen, einschließlich Nur-Follower-Beiträgen (`private`): der Text in jeder seiner Sprachen, die Inhaltswarnung, ob er als sensibel markiert ist, wann er veröffentlicht wurde, seine Adresse und URL, die Adresse des Beitrags, auf den er antwortet, Umfrageoptionen, Hashtags, Adressen und Beschreibungen von Anhängen (keine Medien), sein Autor und der Server des Autors, seine Sichtbarkeit, wie viele Konten er nennt und wie viele davon hier sind und ob hier jemand dem Autor folgt ([Entscheidungs-Hooks](decision-hooks.md#what-is-sent-and-what-it-costs)). Direkte Beiträge gehen nur, wenn hier jemand dem Autor folgt, oder mit `hook_inbound_status_unfollowed_direct` von jedem Autor. |
| `{{IF media review}}` | `plugin_media_check_address` ist gesetzt und `hook_media_check_mode` ist nicht `off` | Das kleinste verarbeitete Bild jedes Uploads, ohne Metadaten, nie das Original, mit dem Zweck der Datei und ihrer Art, Größe und Länge ([Entscheidungs-Hooks](decision-hooks.md#what-mediacheck-reviews)). Die erfasste Entscheidung behält die ID der Datei und ihren Uploader. Ein abgelehnter Upload wird gar nicht aufbewahrt: Wenn du einen sichern oder melden musst, muss dein Plugin das tun. Nenne den Dienst in Abschnitt 6, es sei denn, du betreibst das Plugin selbst. |
| `{{IF translation}}` | `translation_providers` ist nicht leer | Der Text eines Beitrags, wenn ein Leser darum bittet, ihn zu übersetzen: sein Text, die Inhaltswarnung, Umfrageoptionen und Bildbeschreibungen. Die Adresse des Beitrags, sein Autor, der Leser, Erwähnungen und Links werden nicht gesendet. Standardmäßig erhält ein Anbieter `public`-, `local`- und `unlisted`-Beiträge. Er arbeitet als dein Dienstleister, wie das Hosting, sodass `local`-Beiträge ihn erreichen, obwohl sie nicht föderieren: Behalte den `local`-Satz in Abschnitt 5. Ein Anbieter, der den Text für eigene Zwecke aufbewahrt, ist in diesem Sinne kein Dienstleister, und die Erklärung muss das sagen. |
| `{{IF translation}}` mit dem kostenlosen Tarif von DeepL | `translation_providers` enthält `deepl` mit einem Schlüssel, der auf `:fx` endet | Dasselbe. DeepL kann den Text und die Übersetzungen dauerhaft aufbewahren, und die Erklärung muss das sagen ([Übersetzung](translation.md#deepl)). |
| `{{IF translation}}` mit Amazon Translate | `translation_providers` enthält `aws` | Dasselbe. Sofern die Opt-out-Richtlinie für KI-Dienste von AWS Organizations dein Konto nicht abdeckt, kann Amazon den Text speichern und zur Verbesserung seiner Dienste verwenden, möglicherweise in einer anderen Region, und die Erklärung muss das sagen ([Übersetzung](translation.md#amazon-translate)). |
| `{{IF translation to private and direct}}` | `translation_provider_visibilities` erlaubt einem Anbieter `private` oder `direct` | Dasselbe, für diese Beiträge. Verwende diese Markierung nur, wenn du es erlaubt hast, pro Anbieter. Sie fügt den Abschnitten 2 und 5 Zeilen hinzu und weitet dein Versprechen zu diesen Beiträgen aus, kündige es also an, bevor du die Einstellung änderst. |
| `{{IF translation of reports}}` | `translation_reports_to_server` | Der Kommentar einer Meldung und die Beiträge, die sie nennt, wenn ein Moderator darum bittet, sie zu übersetzen. Jeder Anbieter erhält weiterhin nur die Sichtbarkeiten, die ihm erlaubt sind, und der Kommentar zählt als `private`: Ist keinem Anbieter `private` erlaubt, erreichen Kommentare keinen. Die Übersetzung des Kommentars wird nicht gespeichert. |
| `{{IF browser translation}}` | `translation_browser` | Nichts erreicht diesen Server oder einen Anbieter. Der Browser übersetzt auf dem Gerät des Lesers, und was der Hersteller des Browsers tut, liegt bei diesem Browser. |

Die Dienste sind deine Wahl, daher sagt diese Vorlage nicht, wo jeder ansässig ist oder welcher Vertrag ihn abdeckt. Bestätige beides selbst.

Zwei Dinge erreichen Dritte ohne eine Einstellung von dir:

- **Web Push.** Wenn ein Nutzer es einschaltet, wählt sein Browser den Push-Dienst und gibt dem Server dessen Adresse. Der Nachrichtentext ist für diesen Browser verschlüsselt, der Push-Dienst sollte ihn also nicht lesen können. Er sieht die Subscription-Adresse, die Größe und Zeit jeder Nachricht und den Kontakt (VAPID-Subject) deines Servers, und er hält eine Nachricht bis zu 48 Stunden, während der Browser offline ist.
- **Föderation.** Siehe Abschnitt 5 der Vorlage. `federation_mode` schaltet sie aus oder begrenzt ihre Richtung, und Relays kannst du selbst hinzufügen ([Relays](relays.md)).

<!-- source: the-template 8b6bd2fc983a -->
## Die Vorlage

```markdown
Last updated: {{DATE}}

This policy describes how {{OPERATOR NAME}} ("we") handles personal data on {{INSTANCE NAME}} ({{https://INSTANCE DOMAIN}}).

# 1. Who is responsible

- Controller: {{OPERATOR NAME, POSTAL ADDRESS}}
- Contact for privacy questions: {{EMAIL OR FORM}}
- {{IF you have a representative in the EU or the UK: name and contact}}
- {{IF you have a data protection officer: name and contact}}

# 2. Whom this covers

This policy covers people with an account here, people who read pages here without an account, and people on other servers whose posts and profiles reach this server.

{{IF translation}} If you are on another server, the text of your posts that reach this server can go to the translation services in section 6 when a reader here asks for a translation. Your server cannot tell you when this happens.

{{IF external review of posts}} If you are on another server, your posts that relate to an account here, followers-only posts included, go with their text to the external review in section 6 before this server accepts them. It can stop a post or mark it, or file a report for our moderators; people decide on any action against an account.

# 3. What we keep, why, and for how long

"Until your account is erased" means: deleting your account hides it at once and erases it 7 days later. A deactivated account stays hidden, can be reactivated for 30 days, and is then deleted the same way.

| Data | Why | How long |
| --- | --- | --- |
| Email addresses | Signing in, confirming the address, resetting the password, and the email you choose to get | Until your account is erased |
| Password hash, two-factor secret (encrypted) and recovery codes (hashed), passkeys, the identifier of an account you linked at a login provider | Signing you in | Until your account is erased |
| Language, time zone, default post settings, notification settings, and other preferences | Running the service the way you asked | Until your account is erased |
| Registration application: the reason you gave, the invite you used, and the decision | Deciding who joins | Until your account is erased |
| IP address at registration | Preventing abuse | {{180 days}} from registration |
| IP address and browser name (User-Agent) at each sign-in | Security and preventing abuse | {{180 days}} from the sign-in |
| Sessions and app access (stored as hashes) | Keeping you signed in | 30 days after they expire. A session lasts 30 days |
| IP address, and for password-reset requests the email address, in rate limit counters | Protecting the service | Removed automatically, within twice the window of the limit |
| Profile: display name, bio, extra fields, avatar, header, aliases, and earlier versions of them | Showing your profile, which is public | Until your account is erased |
| Posts, their earlier versions, and attached media | Publishing what you write | Until you delete them, your automatic deletion policy deletes them, or your account is erased. {{IF you set a maximum for keeping posts}} At most {{N days}} after you publish them, unless your account may keep them longer. Uploads attached to nothing go after {{24 hours}} |
| Favourites, reactions, boosts, bookmarks, pins, poll votes, follows, blocks, mutes, lists | Running the features you use | Until you undo them, or your account is erased |
| Notifications and what you have read | Telling you what happened | Until your account is erased or the post they are about is removed |
| Web Push subscription: your browser's push address and keys | Sending you push notifications | Until you turn it off, the push service ends it, or your account is erased |
| Records that an email bounced or was reported as spam | Not sending mail that is unwanted or cannot arrive | Until your account is erased |
| Data export archives and data imports | Giving you your data, and bringing it in | 7 days |
| Reports: the comment, the posts and rules named, and moderators' notes | Moderation | {{365 days}} from the last resolution, or as long as an action refers to them. Open reports stay |
| Actions against accounts, and appeals | Moderation, and handling repeat abuse | {{365 days}} from the end of the action. An action with no end stays until it is lifted |
| Posts that a report or an action points to | Evidence for moderation | Hidden when deleted, and removed with that record |
| Audit log of staff actions, which can name accounts | Accountability of staff | {{730 days}} |
| Sign-up blocks: an email domain, a hashed email address, or an IP range | Preventing repeat abuse | While in effect, and 30 days after the lift or expiry |
| {{IF voice}} Voice chat rooms: name, description, hosts, and how they ended | Showing and moderating rooms | Kept. The name and description you wrote are emptied when your account is erased |
| {{IF voice}} Who took part in a room: participation, speakers, raised hands, invitations, removals | Moderating rooms | {{90 days}} from the end of the room, or as long as a report about the room. We record neither audio nor text chat |
| {{IF payments}} Supporter plans: the provider's IDs for you and your subscription, amounts, periods, and states. Not your card details | Running your subscription, and accounting | {{PERIOD AND REASON, e.g. as long as tax law requires}}. The link to the provider's customer goes when your account is erased |
| {{IF payments}} Your choice to appear in the list of supporters | Showing the list | Until the subscription ends or your account is erased |
| Copies of activities sent to and received from other servers | Delivering, retrying, and rejecting duplicates | 30 days. The first delivery of a post stays until 30 days after the post or its author is deleted |
| Posts and profiles of people on other servers | Showing them here | Posts: {{90 days}} when nothing here refers to them, {{7 days}} when only a relay brought them, and {{no limit}} otherwise. Profiles: until their server deletes the account. Then erased after 7 days |
| Copies of remote media | Showing it | {{Not kept, or the period you set}} |
| Server logs: request paths, which can contain a username or a post ID. Our server software writes no IP address in them | Running and debugging the service | {{PERIOD}} |
| {{IF proxy logs}} Logs of our proxy or load balancer: IP addresses and requests | Running and protecting the service | {{PERIOD}} |
| Error reports, with personal details removed by pattern | Fixing faults | The latest report of each kind of error stays until a newer one replaces it. Hourly counts: 90 days |
| {{IF webhooks}} Messages to our webhook receivers, which can carry an applicant's email address or a report's comment | Connecting our own tools | 14 days here. {{PERIOD AT THE RECEIVER}} |
| {{IF post review}} The text of a refused post that you send with an appeal, and the decision | Letting staff review the appeal | {{90 days}} from the refusal, and longer while you can still appeal or staff have not decided. Removed when your account is erased |
| {{IF translation}} Translations of posts that readers asked for, which every reader who may see the post can reuse | Showing a translation again without sending the post again | {{30 days}} from the translation, and no longer than the post |
| {{IF translation}} How many characters your requests sent for translation in a month | Keeping translation within its limits | Until the end of the following month, or until your account is erased |
| Backups of all the above | Restoring the service after a failure | {{PERIOD}}. Data you erased stays in a backup until the backup expires |

We can stop erasure for one account when a court order or a legal request requires it. The records of that account then stay until the hold is released.

After erasure we keep the account's address and username so that nobody can reuse them, the record that its posts were deleted, its moderation records until their period ends, invites and roles it took part in, moves it made, and {{IF payments}} the supporter records above.

# 4. Why we are allowed to

{{CHOOSE WITH YOUR ADVISER. A common starting point under GDPR Article 6(1), not a conclusion:}}

- Running your account and the features you use: {{performance of a contract, point (b)}}.
- Security, preventing abuse, moderation, the audit log, logs, and backups: {{legitimate interests, point (f): keeping the service safe and working}}.
- Storing and showing what people on other servers publish: {{legitimate interests, point (f)}}.
- Records the law makes us keep: {{legal obligation, point (c)}}.
- {{IF you rely on consent for anything: what, and how to withdraw it}}.

You need an email address {{OR a login provider}} to create an account here, and without one we cannot provide an account.

# 5. What other people and other servers receive

**This server is part of a federated network.** Other servers run by other people, in other countries, receive your data as part of how the service works, and their own policies apply to it. We cannot make them delete it.

| What | Who receives it |
| --- | --- |
| Your profile, including your avatar, header, and aliases | Anyone, on the web and through other servers |
| `public` and `unlisted` posts | Anyone with the link. The servers of your followers and of the people you mention. {{IF relays}} `public` posts also go to relays we joined, and through them to other servers |
| `local` posts | People signed in to this server, and your followers on other servers. {{IF translation}} When a reader asks for a translation, the text also goes to the translation services in section 6, which work for us. They do not receive the post's address or your handle |
| `private` posts | Your followers and the people you mention, on any server |
| `direct` posts | The people you mention, on any server. They are not end-to-end encrypted: we can technically read them |
| {{IF translation to private and direct}} {{`private` AND/OR `direct`, AS YOU ALLOWED}} posts | {{TRANSLATION SERVICES YOU ALLOWED THEM}}, when a reader who may see the post asks for a translation. They do not receive the post's address, your handle, the reader, or who is mentioned |
| Follows, blocks, boosts, favourites, reactions, poll votes, quotes | The other person's server. Boosts also go to your followers' servers |
| That your account moved, was deactivated, suspended, or deleted | Your followers' servers and, except for a move, other servers we know |
| A report you chose to forward | The reported account's server receives your comment and the posts named. It does not receive your name, IP address, or email address, so do not put those in the comment |
| {{IF translation of reports}} A report's comment and the posts it names | {{TRANSLATION SERVICES YOU ALLOWED `private` POSTS}} receive the comment, and the services in section 6 receive the posts as for a reader, when a moderator asks for a translation. They do not receive who reported, the report, or the posts' addresses |

When you edit or delete a post or delete your account, we tell those servers. Most honour it, but we cannot guarantee it, and copies, including ones made by others, can remain.

Other people here see what you publish. Staff with the right permission can reveal your email address and your registration and sign-in IP addresses, and each reveal is recorded in the audit log. Staff can also see email addresses of pending registrations.

Search engines and AI services: {{DESCRIBE YOUR search_indexing, indexable_default, AND ai_usage SETTINGS. By default we ask them not to index and not to use the content}}. These are requests, and not every crawler follows them. You can change whether your own profile appears in search in your profile settings.

{{IF the full-text search runs on another company's service}} Full-text search: the text of posts in search ({{public / local}} posts of people who chose to appear in search) goes to {{SERVICE}} to make them searchable, and is removed there when the post is deleted or you stop appearing in search. {{SAY HOW LONG ITS BACKUPS KEEP IT}}. {{IF you keep logs that hold search terms}} What you search for can appear in our request logs, which we keep for {{PERIOD}}.

Apps you authorize can read and do what the permissions you granted allow. Their own policies apply. You can revoke them in your settings.

{{IF the list of supporters is public}} If you choose to appear in the list of supporters, it shows your avatar, display name, handle, and plan.

{{IF browser translation}} A reader can also translate a post with the translation built into their browser. That sends nothing to us or to a translation service, and the translation is not stored.

# 6. Service providers

We use these services to run the instance. {{SAY WHETHER EACH ACTS ON YOUR BEHALF AND WHAT CONTRACT APPLIES.}}

| Service | Purpose | Data | Location | Their policy |
| --- | --- | --- | --- | --- |
| {{HOSTING PROVIDER}} | Server, database, and file storage | Everything in section 3 | {{COUNTRY}} | {{LINK}} |
| {{IF cdn}} {{CDN PROVIDER}} | Delivering media | IP address and requests for media | {{COUNTRY}} | {{LINK}} |
| {{IF mail}} {{MAIL PROVIDER}} | Sending email | Your address, and the subject and text of each email | {{COUNTRY}} | {{LINK}} |
| {{IF captcha}} {{CAPTCHA SERVICE}} | Stopping automated sign-ins, sign-ups, and password-reset requests | IP address, browser and device information, cookies, and interactions with the widget. Your browser sends these directly, from the sign-in, sign-up, and password-reset request pages only | {{COUNTRY}} | {{LINK}} |
| {{IF login providers}} {{PROVIDER}} | Signing in with an account there | That you sign in. We receive their identifier for you, and at sign-up your email address and username | {{COUNTRY}} | {{LINK}} |
| {{IF follow review}} {{FOLLOW REVIEW SERVICE}} | Screening follows from other servers for abuse | The addresses of your account and of the account on the other server, when someone there follows you | {{COUNTRY}} | {{LINK}} |
| {{IF sign-up review}} {{SIGN-UP REVIEW SERVICE}} | Screening sign-ups for abuse | The email address and IP address you sign up with, unless you sign up with an invite | {{COUNTRY}} | {{LINK}} |
| {{IF post review}} {{POST REVIEW SERVICE}} | Screening new posts and their edits for abuse | Your `public`, `local`, and `unlisted` posts, each time you post or edit one, while your account is younger than {{7 days}}: their text, content warning, language, whether they are marked sensitive, poll options, image descriptions, hashtags, and how many accounts they mention, their address and the addresses of the posts they reply to or quote, your address and username, and when your account was created. Not your media | {{COUNTRY}} | {{LINK}} |
| {{IF profile review}} {{PROFILE REVIEW SERVICE}} | Screening the profiles of accounts on other servers for abuse | Nothing about your account on this server. The public profile of an account on another server when this server takes in a change of it: its display name, bio, profile fields, the addresses of its avatar and header image, its address and username | {{COUNTRY}} | {{LINK}} |
| {{IF profile review}} {{PROFILE REVIEW SERVICE}} | Screening changes of profiles for abuse | Your display name, bio, and profile fields, and whether you changed your avatar or header, each time you change them while your account is younger than {{7 days}}, with your address and username and when your account was created. Not your images | {{COUNTRY}} | {{LINK}} |
| {{IF media review}} {{MEDIA REVIEW SERVICE}} | Screening uploads for abuse before anyone can see them | A small processed copy of each image you upload, or of the poster or cover of a video or audio file, without its metadata, and what the file is for. Not the original, its description, or your account | {{COUNTRY}} | {{LINK}} |
| {{IF translation}} {{TRANSLATION SERVICE}} | Translating a post when a reader asks | The text of the post: its body, content warning, poll options, and image descriptions. Not its address, its author, the reader, mentions, or links. Text sent to the service cannot be recalled, and deleting the post does not remove it there: {{HOW LONG THE SERVICE KEEPS IT}} | {{COUNTRY}} | {{LINK}} |
| {{IF external review of posts}} {{REVIEW SERVICE}} | Reviewing posts from other servers before they are accepted | The post as its server sent it, followers-only posts included: its text and language, content warning, whether it is marked sensitive, when it was published, its address, the address of the post it replies to, poll options, hashtags, its author, its visibility, attachments' addresses and descriptions, how many accounts it names and how many of them are here, and whether anyone here follows its author. Direct posts {{only when someone here follows their author, or from any author if you turned on `hook_inbound_status_unfollowed_direct`}}. No post of this server's accounts | {{COUNTRY}} | {{LINK}} |
| {{IF stripe}} Stripe | Taking payments for supporter plans | Your email address and plan. You enter payment details on Stripe's pages | {{COUNTRY}} | <https://stripe.com/privacy> |
| {{IF payjp}} PAY.JP | Taking payments for supporter plans | Your email address, cardholder name, and plan. You enter card details in PAY.JP's frame, whose script runs on our payment pages | {{COUNTRY}} | {{LINK}} |
| {{IF error reports}} {{ERROR TRACKER}} | Fixing faults | Error reports with personal details removed by pattern | {{COUNTRY}} | {{LINK}} |
| {{IF telemetry}} {{METRICS OR TRACING SERVICE}} | Watching the health of the service | Measurements, and the names of other servers we contact | {{COUNTRY}} | {{LINK}} |
| {{IF voice media server}} {{VOICE SERVICE}} | Carrying voice chat | Your IP address and audio while you speak in a room, and a session ID. Not your account | {{COUNTRY}} | {{LINK}} |
| {{IF turn}} {{STUN OR TURN SERVICE}} | Connecting voice chat | Your IP address, and encrypted audio when relayed | {{COUNTRY}} | {{LINK}} |

{{IF peer-to-peer voice}} In voice chat rooms, your browser connects directly to other participants, so they can see your IP address.

If you turn on push notifications, your browser's maker or its push service delivers them. The content is encrypted for your browser, but the service sees your browser's push address and when messages are sent.

# 7. Transfers abroad

Delivery to other servers (section 5) sends your data to servers abroad, wherever those servers are. We do not choose or know their countries in advance, and we cannot limit delivery by country.

{{YOUR STATEMENT ON THIS AND ON THE SERVICES OF SECTION 6: which transfer rules you consider apply, and the safeguards you rely on, if any. Settle this with your adviser: whether federated delivery is a restricted transfer is not settled, and this template takes no position.}}

# 8. Cookies and storage in your browser

We use no advertising or analytics cookies. Our pages use these strictly to work:

| Name | Purpose | Lasts |
| --- | --- | --- |
| `__Host-my_session` | Keeps you signed in to the sign-in pages | Up to 30 days |
| `__Host-my_rt` | Keeps the app signed in | Up to 30 days |
| `__Host-my_csrf` | Protects forms against forged requests | Until you close your browser |
| `__Host-my_2fa` | Carries a sign-in to the two-factor step | 5 minutes |
| `__Host-my_external` | Carries a sign-in through a login provider | At most 30 minutes |

The app also stores data in your browser only: display settings, recently used emoji and hashtags, the draft of a post (for the tab), and the newest posts of your home timeline for offline reading, without followers-only and direct posts. The drafts and the timeline are removed when you sign out.

{{IF captcha}} The CAPTCHA service sets its own cookies on the sign-in, sign-up, and password-reset request pages.

{{IF payjp}} PAY.JP's script runs on our payment pages and may set its own cookies. {{CHECK AGAINST PAY.JP'S POLICY.}}

# 9. How we protect it

Passwords are stored as Argon2id hashes. Two-factor secrets and the keys that sign your activities are encrypted, and the encryption key is kept outside the database. Session and access tokens are stored as hashes. Traffic uses HTTPS. {{YOUR MEASURES: encryption at rest and of backups, who has access, and how you handle a breach.}}

# 10. Your rights

{{ADJUST TO THE RIGHTS OF YOUR JURISDICTION. Under the GDPR these are access, rectification, erasure, restriction, objection, and portability, and the right to withdraw consent.}}

- **See and take your data:** your settings offer a data export (your profile, your posts, and your moderation records) and CSV files of follows, blocks, mutes, muted domains, and bookmarks. For everything else, including your email and IP addresses, write to us.
- **Correct it:** edit your profile and account settings, or write to us.
- **Delete it:** delete your account in your settings, or deactivate it first. This is what section 3 and section 5 describe.
- **Move:** you can move your followers to an account on another server.
- **Limit it:** choose your notifications in your settings. Every notification email has an unsubscribe link. You can also object to or ask us to restrict a use of your data by writing to us.
- **Reports about you:** we do not reveal who reported you.

Write to {{CONTACT}}. We answer within {{one month, or the period your law sets}}. You can complain to a data protection authority: {{YOUR AUTHORITY, OR "the authority where you live"}}.

We keep some records even when you ask us to delete them, where we need them for moderation or the law requires them (section 3).

# 11. Children

{{YOUR MINIMUM AGE AND HOW YOU APPLY IT. The server does not ask for a birth date or check age.}}

# 12. Automated decisions

We make no decisions about you by automated means that have legal or similarly significant effects, {{CONFIRM FOR YOUR USE. These are automatic: a sign-up that matches a block, or that an external review answers about, is refused or sent for review, an upload that an external review rejects is refused, {{IF post review}} a post that an external review refuses, which you can appeal to our staff, {{IF profile review}} a change of your profile that an external review refuses, which you can change and save again, the CAPTCHA check, and rate limits. Your staff decide the rest.}}

# 13. Changes

We change this policy when what we do changes, and show the new date at the top. {{HOW YOU TELL PEOPLE, for example an announcement.}} If we close the service, we tell you in advance, with a deadline for taking your data. {{HOW YOU TELL PEOPLE, for example by email and an announcement.}}
```
