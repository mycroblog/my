<!-- source: managing-the-key-encryption-key-kek 571fb6e3c6b4 -->
# Den Key-Encryption-Key (KEK) verwalten

Der Server verschlüsselt die Secrets, die er in der Datenbank speichert, mit einem Key-Encryption-Key (KEK), den er dort nie ablegt.
Diese Seite beschreibt, wie du den KEK erzeugst, aufbewahrst, sicherst und rotierst, wie du ihn stattdessen in einem KMS hältst und was zu tun ist, wenn er verloren geht oder durchsickert.
Die [Konfigurationsreferenz](../reference/configuration.md) listet jede Einstellung auf.

<!-- source: how-it-works 37074c6237d0 -->
## So funktioniert es

![Die Schlüsselhierarchie: Der Key-Encryption-Key, der dort liegt, wo key_manager es sagt, und nie in der Datenbank, verpackt einen für jedes Secret erzeugten Data-Encryption-Key, der das Secret verschlüsselt; die Datenbank enthält die Schlüssel-ID, den verpackten Data-Encryption-Key und das verschlüsselte Secret in fünf versiegelten Spalten. Die Liste unten beschreibt jede Schicht.](images/key-hierarchy.svg)

- In der Datenbank gespeicherte Secrets, etwa die privaten Schlüssel der Actors, werden mit AES-256-GCM unter einem für jedes Secret erzeugten Data-Encryption-Key (DEK) verschlüsselt.
  Der DEK wird mit einem **Key-Encryption-Key (KEK)** verpackt und neben dem Chiffretext gespeichert (Envelope Encryption).
- Der KEK wird nicht in der Datenbank gespeichert, daher legt ein durchgesickerter Datenbank-Dump allein die Secrets nicht offen.
- Jeder Chiffretext trägt die **Schlüssel-ID** des KEK, der seinen DEK verpackt hat. Mit `key_manager = local` ist die Schlüssel-ID `local:` gefolgt von 16 hexadezimalen Ziffern, die aus dem KEK abgeleitet werden,
  derselbe KEK ergibt also immer dieselbe ID. Du vergibst nie IDs.
- Ein Chiffretext ist an die Zeilen-ID und den Spaltennamen gebunden, wo er gespeichert ist. Wird er in eine andere Zeile oder Spalte kopiert, lässt er sich nicht mehr entschlüsseln.
- Die versiegelten Spalten, benannt wie `my kek status` sie ausgibt, sind `local_account_private_keys.private_key_enc` (private Schlüssel der Actors),
  `totp_enrollments.secret_enc` (Secrets der Zwei-Faktor-Authentifizierung), `login_provider_secrets.sealed_secret` (Client-Secrets der Anmeldeanbieter),
  `webhook_secrets.sealed_secret` (Signatur-Secrets von Webhooks) und `instance_secrets.sealed_secret` (der Signaturschlüssel des Media-Proxys).
- Der Zustand einer Passkey-Anmeldung oder einer Anmeldung über einen externen Anbieter wird auf dieselbe Weise versiegelt, aber nicht gespeichert: Der Browser hält ihn höchstens 30 Minuten.
- Der Signaturschlüssel (HMAC-SHA256) für Media-Proxy-URLs ist ein zufälliger Schlüssel, den der Server beim ersten Start erzeugt und verschlüsselt wie die anderen Secrets in der Datenbank speichert.
  Er ändert sich nicht, wenn du den KEK rotierst, daher funktionieren Media-Proxy-URLs über Rotationen hinweg weiter, und `my kek rewrap` verpackt ihn zusammen mit den anderen Secrets neu.
- `key_manager` ist `local` (der Standard: er verwendet die Einstellung `kek`), `gcpkms` (der KEK bleibt in Google Cloud KMS; siehe [Google Cloud KMS](#google-cloud-kms)),
  `awskms` (der KEK bleibt in AWS KMS; siehe [AWS KMS](#aws-kms)) oder `plugin` (der KEK bleibt bei einem Programm von dir; siehe [Ein Plugin](#a-plugin)).

<!-- source: what-happens-if-you-lose-the-kek 76126b09f09c -->
## Was passiert, wenn du den KEK verlierst

Ohne den KEK kannst du nach dem Wiederherstellen der Datenbank die privaten Schlüssel der Actors nicht entschlüsseln. Ist ein privater Schlüssel verloren, können andere Server die Signaturen dieses Actors nicht mehr prüfen,
und **die Identität des Actors im Fediverse ist zerstört** (die Beziehungen der Follower lassen sich nicht übertragen). Behandle den KEK mit derselben Sorgfalt wie die Datenbank.

![Was mit dem KEK verloren geht: Nach einer Wiederherstellung ohne ihn ist alles, was nicht versiegelt ist, weiterhin lesbar, jeder versiegelte Wert lässt sich nicht öffnen, und die Actors verlieren ihre Identität. Der Absatz unten sagt dasselbe.](images/kek-loss.svg)

Was nicht versiegelt ist, bleibt lesbar: Beiträge, Konten, Follows und der Rest der Datenbank sowie die Mediendateien im Speicher.
Jeder versiegelte Wert schlägt mit dem Fehler `unknown key-encryption key` fehl: die privaten Schlüssel der Actors, die Secrets der Zwei-Faktor-Authentifizierung,
die Client-Secrets der Anmeldeanbieter, die Signatur-Secrets von Webhooks und der Signaturschlüssel des Media-Proxys.
Dasselbe passiert, wenn anstelle des KEK, der die Secrets verpackt hat, ein anderer gesetzt ist oder wenn der KMS-Schlüssel oder der Schlüssel des Plugins, der sie verpackt hat, deaktiviert oder zerstört ist.

<!-- source: generate-a-kek 44b338b7440b -->
## Einen KEK erzeugen

Kodiere 32 Zufallsbytes mit Standard-Base64.

```sh
openssl rand -base64 32
```

Wo `openssl` nicht verfügbar ist, funktioniert auch `head -c 32 /dev/urandom | base64`. Lass den erzeugten Wert nicht in deinem Terminalverlauf oder in Tickets stehen.

<!-- source: store-and-deliver-it 9300565c2a3c -->
## Aufbewahren und bereitstellen

Der KEK ist eine geheime Einstellung und kann daher nur über die Umgebungsvariable `MY_KEK` oder über `MY_KEK_FILE`, den Pfad einer Datei mit dem Wert, übergeben werden. Steht er in der Konfigurationsdatei (TOML), startet der Server nicht.

| Ziel | Beispiel, wie du ihn bereitstellst |
| --- | --- |
| Selbst gehostet (Binary / systemd) | Lege ihn in einer Datei ab, die nur der Eigentümer lesen kann (`chmod 600`), und richte `MY_KEK_FILE` darauf |
| Docker / Compose | Binde ein Docker-Secret unter `/run/secrets/my_kek` ein und setze `MY_KEK_FILE=/run/secrets/my_kek`. Die Dateien in `deploy/docker` tun das aus `secrets/kek` ([Docker-Compose-Dateien](running.md#docker-compose-files)): Zum Rotieren ersetzt du diese Datei und setzt `MY_KEK_PREVIOUS` in `.env` |
| Kubernetes / microk8s | Binde ein Secret als Datei ein und richte `MY_KEK_FILE` darauf |
| Cloud Run | Stelle ein Secret-Manager-Secret als Datei oder als Umgebungsvariable bereit |
| AWS ECS | Übergib ihn aus Secrets Manager oder SSM Parameter Store (SecureString) |

- Gib jedem Prozess, etwa Web und Worker, denselben KEK.
- Der KEK erscheint nie in Logs oder Fehlermeldungen (nur die Schlüssel-ID).
- `my config check` prüft das Format des KEK (Base64 von 32 Bytes) und gibt den Wert als `(redacted)` aus. Im Profil `production` schlägt es außerdem bei einem KEK fehl,
  der nicht zufällig ist, etwa nur aus Nullen oder einem wiederholten Byte, und `my serve` protokolliert dazu eine Warnung (siehe die [Produktions-Checkliste](production-checklist.md)).

<!-- source: back-it-up 29e7bdb3b114 -->
## Sichern

**Sichere die Datenbank und den KEK getrennt.**

- Bewahre den KEK an einem anderen Ort auf als die Datenbank-Backups (ein Passwortmanager, Offline-Medien, ein Secret-Store in einem anderen Konto und so weiter).
  Liegen beide an einem Ort, macht ein Leck dieses Ortes die Verschlüsselung der Datenbank sinnlos.
- Nimm bei einer Rotation die Schlüssel aus `kek_previous` ins Backup auf, solange in der Datenbank noch Chiffretext liegt, der damit verpackt wurde.
- Nimm den KEK in dein Wiederherstellungsverfahren auf und teste Wiederherstellungen regelmäßig ([Backup und Wiederherstellung](backup.md#recovery-drill)). Setze nach dem Wiederherstellen einer Datenbank den KEK (und `kek_previous`), der zur Zeit des Backups in Verwendung war, auf dieselben Werte.
  Mit einem anderen KEK schlägt alles, was ein Secret liest, mit einem Fehler `unknown key-encryption key` fehl, der die Schlüssel-ID nennt.

<!-- source: rotate-the-kek eb4af3189053 -->
## Den KEK rotieren

Ein KEK in `kek_previous` wird nur zum Entschlüsseln verwendet, und neue Secrets werden immer mit `kek` verschlüsselt.
Damit ein Rolling Deploy, bei dem alte und neue Prozesse nebeneinander laufen, nie Chiffretext erzeugt, den ein Prozess nicht lesen kann, wechselst du in zwei Schritten.

![Eine Rotation des lokalen KEK in fünf Schritten, mit den Werten von kek und kek_previous nach jedem: Der neue KEK kommt zuerst in kek_previous, tauscht dann den Platz mit dem alten, my kek rewrap verschiebt die gespeicherten Secrets, und der alte KEK verlässt die Konfiguration. Die Schritte unten sagen dasselbe.](images/kek-rotation.svg)

1. Erzeuge einen neuen KEK und sichere ihn.
2. Starte jeden Prozess mit `MY_KEK=<old KEK>` und `MY_KEK_PREVIOUS=<new KEK>` neu. Jetzt kann jeder Prozess mit dem neuen KEK entschlüsseln.
3. Starte jeden Prozess mit `MY_KEK=<new KEK>` und `MY_KEK_PREVIOUS=<old KEK>` neu. Ab jetzt werden Secrets mit dem neuen KEK verpackt, und Chiffretext des alten KEK ist weiterhin lesbar.
4. Verpacke die gespeicherten Secrets mit dem neuen KEK neu: Führe `my kek rewrap` einmal aus, mit derselben Konfiguration wie in Schritt 3 (siehe [Die gespeicherten Secrets neu verpacken](#re-wrap-the-stored-secrets)).
   Wenn es mit 0 endet, braucht kein Secret in der Datenbank den alten KEK mehr.
5. Warte nach Schritt 3 mindestens 30 Minuten, damit die kurzlebigen Anmelde-Tokens, die mit dem alten KEK ausgestellt wurden, abgelaufen sind, und starte dann jeden Prozess mit `MY_KEK=<new KEK>` und ohne `kek_previous` neu.
   Bewahre das Backup des alten KEK so lange auf, wie du Datenbank-Backups aufbewahrst, die vor Schritt 4 erstellt wurden: Zum Wiederherstellen eines davon braucht man ihn wieder.

`kek_previous` akzeptiert mehrere durch Kommas getrennte Schlüssel. Wird derselbe Schlüssel zweimal oder derselbe Schlüssel wie `kek` aufgeführt, startet der Server nicht.

<!-- source: re-wrap-the-stored-secrets 67af175f1d56 -->
### Die gespeicherten Secrets neu verpacken

`my kek rewrap` ersetzt nur den verpackten Data-Encryption-Key jedes Secrets und behält dessen Chiffretext; es schreibt oder gibt nie ein Secret aus.

```sh
my kek status   # count the stored secrets by the key ID of the KEK that wraps them; decrypts nothing
my kek rewrap   # re-wrap every secret that a key of kek_previous wraps with kek
```

- `my kek status` gibt eine Zeile pro Spalte und Schlüssel-ID aus, markiert die Schlüssel-ID von `kek` mit `(current)` und sagt am Ende, ob noch ein Secret einen Schlüssel aus `kek_previous` braucht.
  Eine Schlüssel-ID, die weder `kek` noch in `kek_previous` ist, bedeutet, dass in der Konfiguration ein KEK fehlt.
- `my kek rewrap` liest die Secrets in Stapeln von 100 und schreibt jeden Stapel in einer kurzen Transaktion. Es gibt pro Spalte aus, wie viele Secrets es neu verpackt hat und wie viele bereits mit `kek` verpackt waren.
  Es kann laufen, während der Server Anfragen bedient, und lässt sich jederzeit gefahrlos stoppen oder erneut ausführen: Es überspringt die Secrets, die bereits mit `kek` verpackt sind, und ein Secret, das der Server inzwischen neu geschrieben hat, bleibt so, wie der Server es geschrieben hat.
- Ein Secret, das sich nicht neu verpacken lässt, bleibt unverändert und wird mit seiner Zeilen-ID gemeldet, und der Befehl endet mit 1. Entweder ist sein KEK nicht konfiguriert
  (die Meldung nennt die Schlüssel-ID: Füge diesen KEK zu `kek_previous` hinzu und führe es erneut aus), oder der gespeicherte Wert ist beschädigt oder wurde aus einer anderen Zeile kopiert.
  Entferne keinen KEK aus `kek_previous`, solange `my kek status` noch Secrets darunter zählt.

<!-- source: google-cloud-kms 1c47dd463234 -->
## Google Cloud KMS

Mit `key_manager = gcpkms` verpackt ein symmetrischer Cloud-KMS-Schlüssel (ein CryptoKey mit dem Zweck `ENCRYPT_DECRYPT`) die Data-Encryption-Keys, und der KEK verlässt Cloud KMS nie.
Ein durchgesickerter Datenbank-Dump und eine durchgesickerte Konfiguration zusammen legen die Secrets trotzdem nicht offen: Zum Entschlüsseln braucht man die Berechtigung, den Schlüssel zu verwenden.

- Setze `kms_key` auf den Ressourcennamen des Schlüssels, ohne Version:
  `projects/{project}/locations/{location}/keyRings/{ring}/cryptoKeys/{key}`. Namen, die länger als 248 Bytes sind, werden beim Start abgelehnt.
  Die Schlüssel-ID der gespeicherten Secrets ist `gcpkms:` gefolgt von diesem Namen.
- Der Server authentifiziert sich mit Application Default Credentials: auf Cloud Run mit dem Servicekonto des Dienstes, anderswo mit `GOOGLE_APPLICATION_CREDENTIALS`.
- Gib diesem Servicekonto `roles/cloudkms.cryptoKeyEncrypterDecrypter` auf dem Schlüssel (nicht auf dem Projekt), für jeden Prozess, etwa Web und Worker, und für die Befehle `my kek status` und `my kek rewrap`.
  Ohne sie schlägt das Lesen eines Secrets mit einem Fehler fehl, der die Rolle nennt; eine fehlende oder deaktivierte Schlüsselversion schlägt ebenso fehl, und diese Fehler werden nicht wiederholt, bis die Konfiguration behoben ist.
  Ein Kontingentfehler oder ein nicht erreichbares Cloud KMS ist vorübergehend, und die Arbeit, die das Secret brauchte, wird wiederholt.
- Der Server behält einen Data-Encryption-Key, den Cloud KMS entschlüsselt hat, für `kms_dek_cache_ttl` (Standard `10m`), sodass die meisten Lesezugriffe auf ein Secret Cloud KMS nicht aufrufen.
- Das Build-Tag `no_gcpkms` lässt den Adapter aus dem Binary weg ([Build-Tags](../reference/build-tags.md)).

<!-- source: rotate-the-key-in-cloud-kms ba58f052af2a -->
### Den Schlüssel in Cloud KMS rotieren

Eine Rotation in Cloud KMS (von Hand oder mit einem Rotationszeitraum am Schlüssel) fügt eine neue primäre Version hinzu und lässt die alten Versionen aktiviert.
Neue Secrets werden mit der neuen Version verpackt, der Server entschlüsselt jedes Secret mit der Version, die es verpackt hat, und die Schlüssel-ID ändert sich nicht, **eine Rotation braucht also kein `my kek rewrap`**.

**Deaktiviere oder zerstöre keine alte Version, solange Secrets sie noch brauchen**: Ein Secret, das mit einer deaktivierten oder zerstörten Version verpackt wurde, lässt sich nicht mehr entschlüsseln.
Cloud KMS sagt nicht, welche Secrets eine Version verpackt hat. Um Schlüsselmaterial außer Betrieb zu nehmen, verschiebst du die Secrets daher zuerst auf einen neuen Schlüssel:

1. Erstelle einen neuen Schlüssel (er darf im selben Key Ring liegen) und gib dem Servicekonto die Rolle dafür.
2. Starte jeden Prozess mit `kms_key` auf den neuen Schlüssel und `kms_key_previous` auf den alten gesetzt neu. Der alte Schlüssel entschlüsselt nun nur noch.
3. Führe `my kek rewrap` mit derselben Konfiguration aus. Wenn `my kek status` unter der ID des alten Schlüssels keine Secrets mehr zählt, entferne `kms_key_previous` und starte neu.
4. Erst dann deaktivierst du die Versionen des alten Schlüssels und hältst sie wiederherstellbar, solange du Datenbank-Backups aufbewahrst, die vor Schritt 3 erstellt wurden: Zum Wiederherstellen eines davon braucht man sie wieder.

<!-- source: aws-kms f97406d91b4c -->
## AWS KMS

Mit `key_manager = awskms` verpackt ein symmetrischer AWS-KMS-Schlüssel (Key Spec `SYMMETRIC_DEFAULT`, Key Usage `ENCRYPT_DECRYPT`, vom Kunden verwaltet) die Data-Encryption-Keys, und der KEK verlässt AWS KMS nie.
Ein durchgesickerter Datenbank-Dump und eine durchgesickerte Konfiguration zusammen legen die Secrets trotzdem nicht offen: Zum Entschlüsseln braucht man die Berechtigung, den Schlüssel zu verwenden.

- Setze `kms_key` auf den ARN des Schlüssels (`arn:aws:kms:{region}:{account}:key/{key-id}`), einen Alias (`alias/{name}` oder dessen ARN) oder die Schlüssel-ID.
  Beim Start löst der Server den Namen in den ARN des Schlüssels auf, und die Schlüssel-ID der gespeicherten Secrets ist `awskms:` gefolgt von diesem ARN.
  Ein Alias, den du auf einen anderen Schlüssel zeigen lässt, ändert daher bis zum nächsten Start nichts und danach nur für neue Secrets: Die bereits gespeicherten Secrets nennen weiter den alten Schlüssel, setze ihn also in `kms_key_previous` (siehe [Einen AWS-KMS-Schlüssel außer Betrieb nehmen](#retire-an-aws-kms-key)).
  Von AWS verwaltete Schlüssel (`alias/aws/...`) werden abgelehnt; nur der AWS-Dienst, dem einer gehört, kann ihn verwenden.
- Der Server authentifiziert sich mit der Standard-Credential-Chain des AWS SDK: auf ECS mit der Rolle der Task, auf EC2 mit dem Instance Profile, anderswo mit der Umgebung (`AWS_ACCESS_KEY_ID` und Ähnlichem) oder den gemeinsamen Konfigurationsdateien.
  Die Region ist die des ARN des Schlüssels; setze für einen Alias oder eine Schlüssel-ID `AWS_REGION`.
- Erlaube `kms:Encrypt`, `kms:Decrypt` und `kms:DescribeKey` auf dem Schlüssel, für jeden Prozess, etwa Web und Worker, und für die Befehle `my kek status` und `my kek rewrap`. Eine Richtlinie der IAM-Rolle:

  ```json
  {
    "Effect": "Allow",
    "Action": ["kms:Encrypt", "kms:Decrypt", "kms:DescribeKey"],
    "Resource": "arn:aws:kms:us-east-1:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab"
  }
  ```

  Die Ressource ist der ARN des Schlüssels, auch wenn `kms_key` ein Alias ist. Die Key Policy muss außerdem zulassen, dass die IAM-Richtlinien des Kontos Zugriff gewähren (die Standard-Key-Policy tut das).
- Der Server startet nicht, wenn der Schlüssel nicht existiert, deaktiviert ist, zur Löschung ansteht oder kein symmetrischer Verschlüsselungsschlüssel ist. Später lässt eine fehlende Berechtigung oder ein deaktivierter Schlüssel das Lesen eines Secrets mit einem Fehler scheitern, der die Ursache nennt,
  und diese Fehler werden nicht wiederholt, bis die Konfiguration behoben ist. Drosselung (`ThrottlingException`) oder ein nicht erreichbares AWS KMS ist vorübergehend, und die Arbeit, die das Secret brauchte, wird wiederholt.
- Der Encryption Context jedes Aufrufs enthält die Zeilen-ID und den Spaltennamen des Secrets (`{"my-aad": ...}`, in base64url). CloudTrail protokolliert ihn im Klartext; er enthält kein Secret.
- Kosten: AWS berechnet eine monatliche Gebühr für jeden vom Kunden verwalteten Schlüssel (zum Zeitpunkt des Schreibens 1 USD, zuzüglich desselben Betrags für jede der ersten beiden Rotationen seines Schlüsselmaterials) und eine Gebühr pro Anfrage oberhalb eines monatlichen Freikontingents
  (siehe [AWS-KMS-Preise](https://aws.amazon.com/kms/pricing/)). Der Server behält einen Data-Encryption-Key, den AWS KMS entschlüsselt hat, für `kms_dek_cache_ttl` (Standard `10m`),
  ein Server mit zehn Actors, die ständig zustellen, stellt also etwa 43.000 Anfragen pro Monat und Prozess, einige Cent oberhalb des Freikontingents.
- Das Build-Tag `no_awskms` lässt den Adapter aus dem Binary weg ([Build-Tags](../reference/build-tags.md)).

<!-- source: retire-an-aws-kms-key b670091ad7f1 -->
### Einen AWS-KMS-Schlüssel außer Betrieb nehmen

Die automatische Schlüsselrotation in AWS KMS behält den ARN des Schlüssels und sein früheres Schlüsselmaterial, die Schlüssel-ID ändert sich also nicht, und **eine Rotation braucht kein `my kek rewrap`**.

Um einen Schlüssel nicht mehr zu verwenden oder um einen Alias auf einen anderen Schlüssel zeigen zu lassen, verschiebst du die Secrets zuerst auf den neuen Schlüssel. **Deaktiviere den alten Schlüssel nicht und plane seine Löschung nicht, solange Secrets ihn noch brauchen**: Ein damit verpacktes Secret lässt sich nicht mehr entschlüsseln.

1. Erstelle den neuen Schlüssel und erlaube der Rolle, ihn zu verwenden.
2. Starte jeden Prozess mit `kms_key` auf den neuen Schlüssel und `kms_key_previous` auf den alten gesetzt (sein ARN, nicht der Alias) neu. Der alte Schlüssel entschlüsselt nun nur noch.
3. Führe `my kek rewrap` mit derselben Konfiguration aus. Wenn `my kek status` unter der ID des alten Schlüssels keine Secrets mehr zählt, entferne `kms_key_previous` und starte neu.
4. Erst dann deaktivierst du den alten Schlüssel und behältst ihn, solange du Datenbank-Backups aufbewahrst, die vor Schritt 3 erstellt wurden: Zum Wiederherstellen eines davon braucht man ihn wieder.

<!-- source: a-plugin 43023e60bac9 -->
## Ein Plugin

Mit `key_manager = plugin` hält ein [Plugin](plugins.md) von dir den KEK, in Vault, in einem HSM oder wo es will, und verpackt die Data-Encryption-Keys für den Server.

**Das Plugin liegt in derselben Vertrauensgrenze wie der KEK.** Es sieht jeden Data-Encryption-Key im Klartext, also kann jeder, der es oder den Host, auf dem es läuft, kontrolliert, jedes Secret des Servers lesen, auch die privaten Schlüssel der Actors.
Gib ihm den Schutz, den du dem KEK gibst. Deshalb kann auch keine andere Einstellung als die Konfigurationsdatei und die Umgebung seine Adresse ändern.

```sh
MY_KEY_MANAGER=plugin
MY_PLUGIN_KEY_MANAGER_ADDRESS=unix:///run/my/key_manager.sock
```

- Die Adresse ist `unix://` oder `https://`. Der Server lehnt `http://` ab, auch auf Loopback, da die Schlüssel im Klartext übertragen würden. Die übrigen Einstellungen des Ziels stehen in [Plugins](plugins.md#the-destination).
- Führe `my plugin test --kind key_manager <address>` aus, bevor du den Server auf ein Plugin richtest ([Ein Plugin testen](plugins.md#test-a-plugin)). Es verpackt nur Schlüssel, die es sich ausdenkt und wegwirft.
- Die Schlüssel-ID der gespeicherten Secrets ist `plugin:` gefolgt von der ID des Plugins für seinen KEK, die aus 1 bis 248 Bytes sichtbarem ASCII besteht.
- **Behalte jeden KEK, den ein gespeichertes Secret noch nennt.** Eine Rotation im Plugin zeigt sich als neue ID beim nächsten Verpacken. Die zuvor verpackten Secrets nennen weiter die alte ID, und das Plugin muss sie weiterhin öffnen können. `my kek rewrap` verschiebt sie auf die aktuelle ID, und `my kek status` zählt die übrigen.
- Ein Plugin und ein KMS bedienen nie zusammen: `kms_key` und `kms_key_previous` werden mit einem Plugin abgelehnt, ebenso `plugin_key_manager_address` mit `key_manager = gcpkms` oder `awskms`.
- Das Build-Tag `no_plugins` lässt den Adapter aus dem Binary weg ([Build-Tags](../reference/build-tags.md)).

<!-- source: when-the-server-starts 5faeda92e43b -->
### Wenn der Server startet

Jeder Prozess und jeder `my`-Befehl, der die Datenbank öffnet, etwa `my kek status`, tut dies vor allem anderen. Es ist die einzige Art von Plugin, deren Ausfall einen Start verhindert.

1. Er fragt das Plugin nach der ID seines aktuellen KEK.
2. Er führt einen Selbsttest mit einem zufälligen Schlüssel aus, den er wegwirft: Ein Verpacken und ein Entpacken müssen den Schlüssel zurückgeben, ein Entpacken mit anderen authentifizierten Daten muss mit `OUTCOME_DECRYPT` antworten, zwei Verpackungen des Schlüssels müssen sich unterscheiden, und der verpackte Wert darf den Schlüssel nicht enthalten.

Ein Plugin, das nicht antwortet oder mit `503` antwortet, wird bis zu 30 Sekunden lang erneut versucht, für einen Sidecar, der nach dem Server hochkommt. Danach beendet sich der Prozess mit einem Fehler. Eine falsche Antwort und jeder andere Fehler, etwa ein abgelehntes Token, stoppt ihn sofort: Der Selbsttest soll ein Plugin aufhalten, das die authentifizierten Daten ignoriert oder Schlüssel durchreicht, und Warten behebt das nicht.

<!-- source: when-the-plugin-is-unreachable 71bf24801c2b -->
### Wenn das Plugin nicht erreichbar ist

Der Server schlägt geschlossen fehl. Was auch schiefgeht, ein abgelehnter Aufruf, ein Aufruf mit Zeitüberschreitung oder eine leere, unbekannte oder fehlerhafte Antwort, die Operation, die den Schlüssel brauchte, scheitert mit einem Fehler. Er speichert nie ein Secret im Klartext, wechselt nie zum lokalen KEK oder einem anderen Schlüssel und behandelt nie einen Fehler als Ergebnis. Ein Job, der so scheitert, wird später wiederholt, und eine Anfrage scheitert für die Person, die sie gestellt hat. Jeder fehlgeschlagene Aufruf zählt in [`my.plugin.failures`](running.md) nach Art und Connect-Code, und das Log enthält keinen Schlüssel, keine authentifizierten Daten und keinen verpackten Wert, auch nichts, was das Plugin gesagt hat.

Das Verpacken wird nie zwischengespeichert, und ein Wert, der gerade versiegelt wurde, hat nichts im Cache, daher stoppen diese sofort:

- Das Erstellen eines Kontos, das die Schlüssel des Actors erzeugt.
- Das Einrichten der Zwei-Faktor-Authentifizierung sowie das Registrieren eines Passkeys oder die Anmeldung mit einem, deren Abläufe ihren Zustand versiegelt halten.
- Die Anmeldung über einen externen Anbieter und das Speichern des Client-Secrets eines Anbieters.
- Das Registrieren eines Webhooks und `my kek rewrap`.

Das Lesen eines gespeicherten Secrets funktioniert nur, solange der Prozess seinen Data-Encryption-Key von einem früheren Lesezugriff hält, höchstens `kms_dek_cache_ttl` (Standard `10m`). Danach, oder für ein Secret, das er noch nicht gelesen hat, stoppen auch diese:

- Föderations-Zustellung und signierte Abrufe, die die privaten Schlüssel der Actors brauchen. Die Zustellung holt über die Wiederholungen ihrer Jobs auf, sobald das Plugin wieder da ist.
- Das Prüfen eines Zwei-Faktor-Codes bei der Anmeldung.
- Die Webhook-Zustellung.

Ein Neustart stoppt wie oben beschrieben, **starte den Server also nicht neu, um einen Ausfall des Plugins zu beheben**: Ein Server, der läuft, bedient weiter, was sein Cache abdeckt, und einer, der neu startet, bedient nichts, bis das Plugin antwortet. Was kein Secret braucht, etwa das Lesen von Timelines und das Verfassen von Beiträgen, läuft weiter. Der Signaturschlüssel des Media-Proxys wird beim Start gelesen und bleibt im Speicher. Auch ein erneuertes Client-Zertifikat oder eine CA-Datei braucht keinen Neustart: Der Server liest die Dateien erneut, sobald sie sich ändern ([Plugins](plugins.md#authentication)).

<!-- source: the-contract 6183eb7a7dfa -->
### Der Vertrag

Die RPCs sind die von `my.plugin.key_manager.v1.KeyManagerService`, mit einer Standard-Deadline von 5 Sekunden und Nachrichten von höchstens 128 KiB in jede Richtung.

```text
POST /my.plugin.key_manager.v1.KeyManagerService/Describe
{}
{"outcome":"OUTCOME_OK","keyId":"transit/my-kek-3"}

POST /my.plugin.key_manager.v1.KeyManagerService/WrapKey
{"dek":"<base64>","aad":"<base64>"}
{"outcome":"OUTCOME_OK","keyId":"transit/my-kek-3","wrapped":"<base64>"}

POST /my.plugin.key_manager.v1.KeyManagerService/UnwrapKey
{"keyId":"transit/my-kek-3","wrapped":"<base64>","aad":"<base64>"}
{"outcome":"OUTCOME_OK","dek":"<base64>"}
```

- `keyId` ist die eigene ID des Plugins, ohne `plugin:`. `UnwrapKey` darf eine ältere ID als die aktuelle nennen.
- `wrapped` hat 1 bis 65.535 Bytes und darf den Schlüssel nicht enthalten. Zwei Verpackungen eines Schlüssels unterscheiden sich.
- `aad` sind authentifizierte Daten, die Zeilen-IDs und Spaltennamen enthalten, und `UnwrapKey` muss scheitern, wenn sie nicht dem entsprechen, was `WrapKey` erhalten hat. Das verhindert, dass ein Secret in eine andere Zeile kopiert wird.
- **Protokolliere nie `dek`, `aad` oder `wrapped`.**

| `outcome` | Bedeutung |
| --- | --- |
| `OUTCOME_OK` | Der Aufruf war erfolgreich |
| `OUTCOME_UNKNOWN_KEY` | `UnwrapKey`: Das Plugin hält den KEK von `keyId` nicht |
| `OUTCOME_DECRYPT` | `UnwrapKey`: `wrapped` oder `aad` lässt sich nicht authentifizieren |

Antworte mit einem HTTP- oder Connect-Fehler, wenn du nicht antworten kannst, etwa weil der Schlüsselspeicher nicht erreichbar ist. Antworte dann nicht mit `OUTCOME_DECRYPT` oder `OUTCOME_UNKNOWN_KEY`: Für den Server besagen sie, dass der Wert selbst schlecht ist. Der Server wiederholt einen Aufruf nicht selbst, ein Plugin kann denselben Aufruf also von einem Job erneut erhalten.

<!-- source: move-between-the-local-kek-and-a-kms 81a909a14c05 -->
## Zwischen dem lokalen KEK und einem KMS wechseln

Eine Schlüssel-ID beginnt mit der Art des Key Managers, der sie verpackt hat (`local:`, `gcpkms:`, `awskms:`, `plugin:`). Der Server verpackt neue Secrets mit dem aktuellen `key_manager`
und entpackt jedes gespeicherte Secret mit dem Key Manager, den seine Schlüssel-ID nennt, ein Wechsel zwischen dem lokalen KEK und einem KMS ist also eine Rotation über Arten hinweg.

- Mit einem KMS entschlüsseln `kek` und `kek_previous` nur: Sie öffnen die Secrets, die noch mit dem lokalen KEK verpackt sind, und jedes neue Secret wird mit `kms_key` verpackt.
  `kms_key_previous` listet frühere Schlüssel desselben KMS auf, die ebenfalls nur entschlüsseln.
- Mit `key_manager = local` entschlüsseln `kms_key` und `kms_key_previous`, wenn gesetzt, nur: Sie öffnen die Secrets, die noch mit dem KMS verpackt sind.
- Vom lokalen KEK zu einem KMS:
  1. Erstelle den Schlüssel und gewähre die Berechtigungen wie in [Google Cloud KMS](#google-cloud-kms) oder [AWS KMS](#aws-kms).
  2. Starte jeden Prozess mit `key_manager = gcpkms` oder `awskms`, `kms_key` auf den Schlüssel gesetzt und dem aktuellen KEK von `kek` nach `kek_previous` verschoben neu.
     Secrets werden nun mit dem KMS verpackt, und die mit dem lokalen KEK verpackten lassen sich weiterhin öffnen.
  3. Führe `my kek rewrap` mit derselben Konfiguration aus.
  4. Wenn `my kek status` unter einer `local:`-Schlüssel-ID keine Secrets mehr zählt, entferne `kek_previous` und starte neu. Bewahre das Backup des KEK so lange auf, wie du Datenbank-Backups aufbewahrst, die vor Schritt 3 erstellt wurden.
- Von einem KMS zurück zum lokalen KEK machst du dasselbe in die andere Richtung: Setze `key_manager = local` und einen neuen `kek` und lass `kms_key` gesetzt, der nun nur entschlüsselt,
  bis `my kek rewrap` jedes Secret verschoben hat und `my kek status` unter einer `gcpkms:`- oder `awskms:`-Schlüssel-ID keine Secrets mehr zählt.
  Die Schlüssel in `kms_key` und `kms_key_previous` müssen dann alle zum selben KMS gehören.
- Um vom KMS einer Cloud zu dem einer anderen zu wechseln, wechselst du zuerst zum lokalen KEK und dann zum anderen KMS; einen direkten Wechsel gibt es nicht.
- Ein Plugin wechselt auf dieselbe Weise, allein über den lokalen KEK:
  - Zu ihm: Starte jeden Prozess mit `key_manager = plugin`, `plugin_key_manager_address` und dem aktuellen KEK von `kek` nach `kek_previous` verschoben neu und führe dann `my kek rewrap` aus. Wenn `my kek status` unter einer `local:`-Schlüssel-ID keine Secrets mehr zählt, entferne `kek_previous` und starte neu.
  - Von ihm: Starte jeden Prozess mit `key_manager = local`, einem neuen `kek` und beibehaltener `plugin_key_manager_address`, die nun nur entschlüsselt, neu und führe dann `my kek rewrap` aus. Wenn `my kek status` unter einer `plugin:`-Schlüssel-ID keine Secrets mehr zählt, entferne die Adresse und starte neu. Bis dahin muss das Plugin weiterhin [die Prüfungen beim Start](#when-the-server-starts) bestehen, zu denen ein Verpacken gehört.
- Der Server behält einen Data-Encryption-Key, den der KMS oder das Plugin entschlüsselt hat, für `kms_dek_cache_ttl` (Standard `10m`, höchstens `kms_dek_cache_entries`, Standard `1024`),
  ein verwendetes Secret erreicht ihn also etwa einmal pro Zeitraum und Prozess. `0` in einer der beiden Einstellungen deaktiviert den Cache.

<!-- source: if-the-kek-leaks be6e788e4cea -->
## Wenn der KEK durchsickert

Ist nur der KEK durchgesickert, rotiere ihn wie oben. Wenn sowohl der KEK als auch die Datenbank durchgesickert sein können, schützt dich eine Rotation nicht:
Die privaten Schlüssel der Actors in der Datenbank sind bereits lesbar, also müssen die Schlüssel der Actors selbst neu erzeugt werden.

<!-- source: web-push-vapid-key aa32321d1e4a -->
## VAPID-Schlüssel für Web Push

Web Push wird mit dem VAPID-Schlüssel des Servers signiert (P-256, RFC 8292). Er ist ein vom KEK getrenntes Secret und wird nicht in der Datenbank gespeichert.

- Erstelle ein Schlüsselpaar mit `my config vapid-key` und gib jedem Prozess den privaten Schlüssel (den Wert von `vapid_private_key = ...`) über `MY_VAPID_PRIVATE_KEY` oder `MY_VAPID_PRIVATE_KEY_FILE`.
  Der Server leitet den öffentlichen Schlüssel aus dem privaten ab und gibt ihn an Clients weiter, du konfigurierst ihn also nicht.
- Wenn du ihn nicht setzt, ist Web Push deaktiviert, und Clients bieten es nicht an. Im Profil `production` warnen `my config check` und der Start von `my serve` davor.
- Browser binden ein Abo an den öffentlichen Schlüssel, daher beendet eine Änderung des Schlüssels die Zustellung an bestehende Abos. Wenn Nutzer Push-Benachrichtigungen in den Einstellungen wieder einschalten, abonniert der Client mit dem neuen Schlüssel neu.
  Ändere den Schlüssel nur, wenn er durchgesickert ist; falls er durchsickert, wechsle zu einem neuen Schlüssel und bitte die Nutzer, Benachrichtigungen erneut einzuschalten.
