<!-- source: translating-posts-with-a-translation-service bf15671669b4 -->
# Beiträge mit einem Übersetzungsdienst übersetzen

Der Server kann einen Beitrag übersetzen, wenn ein Leser darum bittet, über einen Übersetzungsdienst, den du konfigurierst: LibreTranslate,
DeepL, Google Cloud Translation oder Amazon Translate. Ein Anbieter ist eingebaut, wenn der Server seine Schlüssel hat, und die
[Servereinstellung](instance-settings.md) `translation_providers` aktiviert ihn.

<!-- source: libretranslate 0d5e448bb450 -->
## LibreTranslate

LibreTranslate ist quelloffene maschinelle Übersetzung, die du selbst betreiben oder als gehosteten Dienst unter
[libretranslate.com](https://libretranslate.com) nutzen kannst. Richte den Server mit zwei Einstellungen der
[Konfiguration](../reference/configuration.md) darauf aus:

```sh
MY_TRANSLATOR_LIBRETRANSLATE_URL=http://libretranslate:5000
# Only when the server asks for a key, as libretranslate.com does:
MY_TRANSLATOR_LIBRETRANSLATE_API_KEY=your-key
```

- `translator_libretranslate_url` ist die Basis-URL des LibreTranslate-Servers, mit `http:` oder `https:` und einem
  optionalen Pfad. Ein Server auf einer privaten Adresse funktioniert, etwa ein Container neben diesem oder `http://10.0.0.5:5000`.
  Der Server folgt Weiterleitungen von dort nicht, damit der Schlüssel nirgendwo anders hingelangt.
- `translator_libretranslate_api_key` ist ein Geheimnis, das nur aus der Umgebung gelesen wird
  (`MY_TRANSLATOR_LIBRETRANSLATE_API_KEY` oder `MY_TRANSLATOR_LIBRETRANSLATE_API_KEY_FILE`). Lass es ungesetzt, wenn der
  Server keinen Schlüssel braucht; ein selbst gehosteter braucht meist keinen. Der Schlüssel wird nie Clients gezeigt oder in
  die Logs geschrieben.

Füge dann `libretranslate` zur Servereinstellung `translation_providers` hinzu, in der Reihenfolge, in der er probiert werden soll, etwa
`translation_providers = libretranslate` allein oder gefolgt von der ID eines anderen Anbieters, auf den zurückgegriffen wird.

Der Server fragt LibreTranslate vor der ersten Übersetzung und danach jede Stunde nach seiner Sprachliste und
sendet keinen Beitrag in einem Sprachpaar, das nicht aufgeführt ist. Hat ein Beitrag keine Sprache, erkennt LibreTranslate sie.
Installiere auf einem Server, den du betreibst, die Sprachmodelle, die deine Leser brauchen.

**Die Übersetzungsqualität variiert je nach Sprache.** Die Modelle von LibreTranslate sind für manche Paare gut und für andere grob,
und eine Übersetzung aus einer Sprache mit wenig Trainingsdaten kann den Sinn verfehlen. Probiere die Sprachen aus, die deine
Community nutzt, bevor du ihn aktivierst, und behalte, wenn du einen brauchst, einen kommerziellen Anbieter danach in `translation_providers`.

**Wohin der Text geht.** Ein Server, den du betreibst, behält den Text von Beiträgen auf deinen eigenen Rechnern, und niemand sonst
erhält ihn. Dann darfst du ihm mit `translation_provider_visibilities` auch `private`- und `direct`-Beiträge erlauben, etwa
`libretranslate=public|local|unlisted|private|direct`, und die
[Datenschutzerklärung](privacy-policy-template.md#the-services-you-choose) nennt keinen externen Dienst für die Übersetzung. Ein
gehostetes LibreTranslate, libretranslate.com oder das von jemand anderem, erhält den Text wie jeder andere Anbieter: Lies seine Bedingungen, behalte die Standard-
Sichtbarkeiten bei und nenne ihn in der Erklärung.

<!-- source: deepl 80449fb800f7 -->
## DeepL

DeepL übersetzt über seine API mit einem Schlüssel eines DeepL-API-Tarifs. Der Server sendet den Text eines Beitrags mit den Markern,
die für Erwähnungen, Links und Auszeichnungen stehen, als XML-Elemente, die DeepL an ihrem Platz lässt, und sonst nichts vom
Beitrag.

1. Hole dir einen Authentifizierungsschlüssel aus deinem DeepL-Konto. Ein Schlüssel des kostenlosen Tarifs endet auf `:fx`.
2. Setze ihn in der [Konfiguration der Bereitstellung](../reference/configuration.md) und starte den Server neu:

   ```sh
   MY_TRANSLATOR_DEEPL_API_KEY=your-key
   ```

   Der Server ruft `https://api-free.deepl.com` für einen Schlüssel des kostenlosen Tarifs und `https://api.deepl.com` für jeden anderen auf.
   Um über einen Proxy zu gehen, setze `MY_TRANSLATOR_DEEPL_URL`. Der Schlüssel ist ein Geheimnis: Der Server protokolliert ihn nie, legt ihn in
   keinen Fehler und zeigt ihn weder in einer Einstellung noch in einer Antwort der Client-API, und er sendet ihn nur an diese Adresse, ohne
   Weiterleitungen zu folgen.
3. Aktiviere den Anbieter mit der [Servereinstellung](instance-settings.md) `translation_providers`, allein oder in deiner Reihenfolge:

   ```sh
   my instance set translation_providers deepl
   ```

Der Server fragt DeepL bei der ersten Übersetzung, welche Sprachen es übersetzt, und danach jeden Tag. Die Sprache eines Beitrags
wird auf die nächste Variante abgebildet, die DeepL hat: Englisch auf `en-US`, Portugiesisch auf `pt-BR`, Chinesisch aus Taiwan auf `zh-Hant`. Eine
Sprache, die DeepL nicht hat, geht an den nächsten Anbieter, oder dem Leser wird gesagt, dass sie nicht übersetzt werden kann.

**Kontingent.** Jeder Tarif hat ein monatliches Zeichenkontingent, und ein bezahlter Tarif kann ein Kostenlimit haben, das du in deinem DeepL-
Konto setzt. Meldet DeepL das Kontingent oder sein Rate Limit, probiert der Server den nächsten Anbieter aus `translation_providers`.
Bleibt keiner übrig, sieht der Leser, dass der Beitrag gerade nicht übersetzt werden konnte, und der Webclient nutzt die Übersetzung des Browsers,
wenn du das erlaubst. Die eigenen Monatslimits des Servers (`translation_user_monthly_characters` und
`translation_server_monthly_characters`) gelten vor jedem Aufruf, setze sie also unter dein DeepL-Kontingent. DeepL kann die
Zeichen anders zählen als sie.

**Daten.** Lies [DeepLs Bedingungen](https://www.deepl.com/en/pro-license), bevor du ihn aktivierst:

- In einem bezahlten Tarif behält DeepL den Text und die Übersetzungen nur so lange, wie es sie zum Übersetzen braucht, und höchstens 72
  Stunden, wenn es einen Fehler untersucht.
- Im kostenlosen Tarif darf DeepL den Text und die Übersetzungen dauerhaft behalten, und seine Bedingungen verbieten, personenbezogene Daten
  darüber zu senden. Beiträge enthalten meist personenbezogene Daten, nutze den kostenlosen Tarif also nur, um den Anbieter auszuprobieren. Erlaube ihm nie
  `private`- oder `direct`-Beiträge in `translation_provider_visibilities`, und sage in deiner Datenschutzerklärung, dass DeepL behält, was es
  erhält.

In einem bezahlten Tarif kannst du `private`- und `direct`-Beiträge erlauben, Anbieter für Anbieter:

```sh
my instance set translation_provider_visibilities 'deepl=public|local|unlisted|private'
```

**Namensnennung.** DeepLs Bedingungen verlangen, dass sein Name oder Logo bei jeder Übersetzung angezeigt wird. Der Webclient zeigt
„DeepL“ mit einem Link auf `https://www.deepl.com` neben jeder Übersetzung von ihm, du musst also nichts hinzufügen.

<!-- source: google-cloud-translation e351f57274b0 -->
## Google Cloud Translation

[Cloud Translation](https://cloud.google.com/translate) hat die Anbieter-ID `google`. Der Server ruft seine
Methode `TranslateText` mit dem allgemeinen Übersetzungsmodell auf und sendet den Text als HTML, damit die Erwähnungen, Links und
anderen Teile, die unverändert bleiben müssen, an ihrem Platz zurückkommen.

1. Aktiviere die Cloud Translation API (`translate.googleapis.com`) in dem Projekt, das die Übersetzungen bezahlt:
   `gcloud services enable translate.googleapis.com --project PROJECT`.
2. Gib dem Service-Konto des Servers `roles/cloudtranslate.user` in diesem Projekt:
   `gcloud projects add-iam-policy-binding PROJECT --member serviceAccount:SA_EMAIL --role roles/cloudtranslate.user`.
3. Setze `MY_TRANSLATOR_GOOGLE_PROJECT` auf die Projekt-ID. `MY_TRANSLATOR_GOOGLE_LOCATION` ist standardmäßig `global`; es
   kann einen anderen Standort von Cloud Translation nennen, der das allgemeine Modell bedient, etwa `us-central1`.
4. Starte den Server neu und füge `google` zur Servereinstellung `translation_providers` hinzu.

Der Server authentifiziert sich mit Application Default Credentials, wie die anderen Google-Cloud-Dienste, die er nutzt: auf
Cloud Run mit dem Service-Konto des Dienstes, sonst mit der Schlüsseldatei, die `GOOGLE_APPLICATION_CREDENTIALS` nennt. Es wird kein
API-Schlüssel verwendet. Der Server macht beim Start keinen Aufruf, eine fehlende API oder Rolle zeigt sich also erst, wenn ein Leser um eine
Übersetzung bittet: Der Server probiert dann den nächsten Anbieter, und der Leser sieht, dass die Übersetzung fehlschlug, wenn keiner übrig ist.
Auch ein Kontingent oder Rate Limit des Projekts, das Google meldet, geht an den nächsten Anbieter weiter.

Google rechnet nach gesendeten Zeichen ab, nach einem monatlichen Freikontingent; siehe
[Preise von Cloud Translation](https://cloud.google.com/translate/pricing). Die eigenen Monatslimits des Servers zählen die
Zeichen ebenfalls, aber nicht genau so, wie Google sie abrechnet, setze also zusätzlich einen Budget-Alarm in Cloud Billing.

Google [gibt an](https://cloud.google.com/translate/data-usage), dass es den Text nur zur Bereitstellung der Übersetzung nutzt,
ihn nur kurz im Arbeitsspeicher hält, nicht zum Training seiner Modelle verwendet und nicht weitergibt. Der Server ruft den
globalen Endpunkt von Cloud Translation auf, bei dem Google die Verarbeitung nicht auf eine Region beschränkt.

Googles [Anforderungen an die Namensnennung](https://cloud.google.com/translate/attribution) verlangen, dass eine Übersetzung als
maschinell übersetzt angezeigt wird und das Badge „powered by Google Translate“ daneben erscheint, verlinkt auf
`https://translate.google.com`. Der Webclient zeigt den Namen „Google Translate“ mit diesem Link neben jeder
Übersetzung dieses Anbieters. Google bietet die Badge-Bilder nur als Download an, daher gibt der Server Clients kein Logo für
diesen Anbieter; prüfe, ob das, was Leser sehen, Googles Anforderungen für deinen Server erfüllt.

<!-- source: amazon-translate a6cd83df7ef1 -->
## Amazon Translate

Amazon Translate ist der Anbieter `aws`. Der Server ruft seine Operation `TranslateText` mit den Zugangsdaten der Standardkette des
AWS SDK auf, wie bei den anderen AWS-Diensten: die Ausführungsrolle des Tasks oder des Servers, die Umgebungsvariablen
`AWS_ACCESS_KEY_ID` und `AWS_SECRET_ACCESS_KEY` oder die gemeinsamen Credentials-Dateien. Kein Schlüssel kommt in die Konfiguration
des Servers.

1. Gib der Rolle oder dem Nutzer, als der der Server läuft, die IAM-Berechtigung `translate:TranslateText`. Bei einem Beitrag, dessen
   Sprache unbekannt ist, erkennt Amazon Translate die Sprache mit Amazon Comprehend; AWS' verwaltete Richtlinien für
   Amazon Translate gewähren dazu `comprehend:DetectDominantLanguage`, gib also auch das.
2. Setze `translator_aws_region` (`MY_TRANSLATOR_AWS_REGION`) auf die Region von Amazon Translate, etwa `us-east-1`. Das Erkennen
   der Sprache braucht eine Region, in der auch Amazon Comprehend läuft.
3. Starte den Server neu und aktiviere dann den Anbieter mit `translation_providers = aws`, allein oder in deiner bevorzugten Reihenfolge
   unter den anderen ([Servereinstellungen](instance-settings.md)).

Amazon Translate rechnet nach gesendeten Zeichen ab, mit einem kostenlosen Kontingent für die ersten 12 Monate
([Preise](https://aws.amazon.com/translate/pricing/)). Der Server sendet nur die Wörter eines Beitrags: Erwähnungen, Hashtags,
Links und Code bleiben auf dem Server, und jedes davon geht als kurzer Platzhalter in HTML-Markup. Halte die Monatslimits
von `translation_user_monthly_characters` und `translation_server_monthly_characters` innerhalb dessen, was du zahlen möchtest.

**Amazon darf den Text speichern und zur Verbesserung seiner Dienste nutzen, möglicherweise in einer anderen Region, es sei denn, die
[KI-Opt-out-Richtlinie](https://docs.aws.amazon.com/organizations/latest/userguide/orgs_manage_policies_ai-opt-out.html) von AWS
Organizations deckt das Konto ab.** Ohne das Opt-out ist Amazon für diesen Text nicht nur dein Dienstleister: Sage das in deiner
Datenschutzerklärung ([Vorlage der Datenschutzerklärung](privacy-policy-template.md#the-services-you-choose)) und erlaube
für `aws` nicht `private` oder `direct` in `translation_provider_visibilities`. Standardmäßig erhält der Anbieter nur `public`-,
`local`- und `unlisted`-Beiträge.

Ein Server, der mit dem Build-Tag `no_translator_aws` gebaut wurde, hat den Anbieter nicht und weigert sich zu starten, wenn
`translator_aws_region` gesetzt ist ([Build-Tags](../reference/build-tags.md)).
