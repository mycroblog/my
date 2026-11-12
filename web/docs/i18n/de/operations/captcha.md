<!-- source: captcha-bot-protection-turnstile--hcaptcha--recaptcha 2c065ac1d2fb -->
# CAPTCHA-Bot-Schutz (Turnstile / hCaptcha / reCAPTCHA)

CAPTCHA ist standardmäßig aus: Die Einstellung [`captcha`](../reference/configuration.md#captcha) ist `none`. Die Seiten für Anmeldung, Registrierung und Zurücksetzen des Passworts funktionieren dann ohne JavaScript und senden nichts an Dritte.
Wenn du einen CAPTCHA-Dienst wählst, erscheint das Widget des Dienstes auf drei Formularen: Anmeldung (`/auth/sign_in`), Registrierung (`/auth/sign_up`) und der Anforderung eines Links zum Zurücksetzen des Passworts
(`/auth/forgot_password`, nur auf Servern, die E-Mails versenden). Der Server prüft das Token, das das Widget erzeugt, mit der siteverify-API des Dienstes.
Du kannst nicht nur einen Teil der Formulare schützen.

<!-- source: choosing-a-service 22ddd3cc639f -->
## Einen Dienst wählen

| `captcha` | Dienst | Kosten | Was der Nutzer tut |
| --- | --- | --- | --- |
| `turnstile` (empfohlen) | Cloudflare Turnstile | Kostenlos, ohne Limit | Meist nichts; ein Kontrollkästchen nur bei Bedarf |
| `hcaptcha` | hCaptcha | Hat einen kostenlosen Tarif | Ein Kontrollkästchen, bei Bedarf zusätzlich eine Bildaufgabe |
| `recaptcha` | Google reCAPTCHA v2 (Kontrollkästchen) | Kostenlos bis zu 10.000 Bewertungen im Monat (siehe unten) | Ein Kontrollkästchen, bei Bedarf zusätzlich eine Bild- oder Audioaufgabe |

reCAPTCHA v3 (nur Score) und reCAPTCHA Enterprise werden nicht unterstützt.

<!-- source: creating-keys 38cd93394638 -->
## Schlüssel erstellen

Jeder Dienst gibt dir ein Schlüsselpaar: einen öffentlichen **Site Key** und einen **Secret Key**, den nur der Server kennt. Registriere den Hostnamen der Einstellung `host` (ohne den Port) als Domain.
Der Server lehnt ein Token ab, wenn der Hostname in der siteverify-Antwort nicht zu `host` passt.

- **Turnstile**: Füge im Cloudflare-Dashboard unter „Turnstile“ ein Widget hinzu und registriere den Hostnamen. Wähle den Widget-Modus „Managed“.
- **hCaptcha**: Füge im hCaptcha-Dashboard eine Site hinzu und registriere den Hostnamen. Der Secret Key steht in den Kontoeinstellungen.
- **reCAPTCHA**: Wähle in der Google-reCAPTCHA-Admin-Konsole den Typ „Challenge (v2)“ mit dem Kontrollkästchen „I'm not a robot“ und registriere die Domain.
  Die Schlüssel gehören zu einem Google-Cloud-Projekt.

Die Testschlüssel jedes Dienstes (die, die immer erfolgreich sind) eignen sich, um zu prüfen, dass das Widget erscheint und ein Token erzeugt.
Allerdings liefert siteverify mit Testschlüsseln einen festen Hostnamen zurück (`example.com` bei Turnstile, `dummy-key-pass` bei hCaptcha und so weiter), daher schlagen Anmeldung und Registrierung auf diesem Server mit „Check again“ fehl, wenn du sie verwendest.

<!-- source: configuring 3301db5c6286 -->
## Konfigurieren

```sh
MY_CAPTCHA=turnstile
MY_CAPTCHA_SITE_KEY=0x4AAAAAAA...
MY_CAPTCHA_SECRET_KEY_FILE=/run/secrets/my_captcha_secret_key
```

- Übergib den Secret Key nur über die Umgebungsvariable `MY_CAPTCHA_SECRET_KEY` oder über `MY_CAPTCHA_SECRET_KEY_FILE`, die auf eine Datei zeigt. Logs zeigen `[REDACTED]`.
- Wenn du ein `captcha` wählst und ein Schlüssel fehlt, meldet `my config check` ein Problem, und der Server startet nicht.
- Der Server probiert die Schlüssel beim Start nicht aus. Wenn der Dienst den Secret Key oder den Site Key ablehnt, protokolliert die erste Prüfung den ERROR
  „the CAPTCHA service refused the secret key or the site key“. Bis du das behebst, werden Anfragen wie unter „Wenn der Dienst nicht erreichbar ist“ unten behandelt.
- Änderungen wirken, wenn du den Server neu startest.

<!-- source: how-verification-works c5cc12dd3769 -->
## Wie die Prüfung funktioniert

- Die Reihenfolge ist Ratenbegrenzung, dann CAPTCHA-Prüfung, dann Passwortprüfung, Verarbeitung der Registrierung oder Versand der Zurücksetzungs-E-Mail. Eine Anfrage über der Ratenbegrenzung wird nie an den Dienst gesendet.
- Auch eine Registrierung mit gültigem Einladungscode wird geprüft.
- Eine Anmeldung, die die Prüfung nicht besteht, zählt als fehlgeschlagene Anmeldung für die Client-IP-Adresse. Sie zählt nicht gegen das Konto, sodass niemand eine andere Person aussperren kann.
  Eine Registrierung, die die Prüfung nicht besteht, zählt als Registrierungsversuch (siehe die Ratenbegrenzung in [Registrierung](registration.md)). Eine Anforderung eines Zurücksetzungs-Links zählt auf dieselbe Weise als Anfrageversuch.
- Bei Turnstile trägt das Widget einen Aktionsnamen (`sign_in` / `sign_up` / `password_reset`), und der Server prüft, dass die Antwort dieselbe Aktion hat. Ein Token von einem Formular lässt sich nicht auf einem anderen verwenden.
- Der Server sendet die IP-Adresse des Nutzers (`remoteip`) nicht an den Dienst. Ein Token ist nur einmal verwendbar, daher wiederholt der Server nicht. Das Timeout beträgt 10 Sekunden.

<!-- source: when-the-service-is-unreachable fe6a6d5915ea -->
### Wenn der Dienst nicht erreichbar ist

Der Dienst ist nicht erreichbar, läuft in ein Timeout, liefert ein 5xx oder lehnt die Schlüssel ab:

| Formular | Behandlung |
| --- | --- |
| Registrierung und Anforderung eines Zurücksetzungs-Links | Abgelehnt (`503`) mit „Versuche es später erneut“. Bots könnten sonst E-Mails und Registrierungen auslösen. |
| Anmeldung | Ohne CAPTCHA durchgelassen, und der Server protokolliert das WARN „the CAPTCHA service could not check a sign-in; letting it through“. Ratenbegrenzung und Passwortprüfung gelten weiterhin. |

Die Anmeldung wird durchgelassen, damit ein Ausfall des Dienstes oder ein falscher Schlüssel nicht alle aussperrt, Administratoren eingeschlossen. Wenn das WARN immer wieder erscheint, prüfe den Status des Dienstes und die Schlüssel.

<!-- source: recaptcha-free-tier 4f413b07dbb9 -->
### Kostenloser reCAPTCHA-Tarif

Über dem kostenlosen Tarif von 10.000 Bewertungen im Monat ist siteverify von reCAPTCHA **immer erfolgreich** (CAPTCHA bietet in dieser Zeit also keinen Schutz).
Der Server kann die Überschreitung nicht erkennen, also prüfe deine Nutzung in der Google-Cloud-Konsole.

<!-- source: pages-and-content-security-policy b0cf9741b2ce -->
## Seiten und Content-Security-Policy

- Die Sprache des Widgets folgt dem Locale der Seite und sein Design dem Farbschema des Browsers (hell oder dunkel).
  Das Turnstile-Widget folgt dem Farbschema selbst. Bei hCaptcha und reCAPTCHA wird es über den Client Hint `Sec-CH-Prefers-Color-Scheme` gewählt, den der Browser sendet; Browser, die ihn nicht senden, bekommen das helle Design.
- Neben dem Widget sagt die Seite, welcher Dienst die Prüfung durchführt, dass der Dienst die IP-Adresse und Browserinformationen erhält, und verlinkt auf die Datenschutzerklärung des Dienstes
  (und auf die Nutzungsbedingungen bei hCaptcha und reCAPTCHA). Bei reCAPTCHA dient das auch als der Satz, den Google verlangt: „This site is protected by reCAPTCHA and the Google Privacy Policy and Terms of Service apply.“
- In einem Browser ohne JavaScript sagt eine `<noscript>`-Meldung, dass die Prüfung JavaScript braucht. Ein Nutzer, der das CAPTCHA nicht lösen kann, kann sich weder registrieren noch anmelden,
  und du kannst ihm nur auf anderem Wege helfen, etwa indem du das Konto mit `my admin create` anlegst oder für eine Weile `captcha = none` setzt.
- Die `Content-Security-Policy` (CSP) wird nur auf den Seiten für Anmeldung, Registrierung und Anforderung des Zurücksetzens des Passworts erweitert, solange CAPTCHA aktiviert ist. `'unsafe-inline'` und `'unsafe-eval'` werden nie hinzugefügt.
  Die CSP des Web-Clients, der Client-API, des Zustimmungsbildschirms, der E-Mail-Bestätigung und anderer Seiten ändert sich nicht.

| `captcha` | Hinzugefügte Quellen |
| --- | --- |
| `turnstile` | `script-src`, `frame-src`: `https://challenges.cloudflare.com` |
| `hcaptcha` | `script-src`, `frame-src`, `style-src`, `connect-src`: `https://hcaptcha.com`, `https://*.hcaptcha.com` |
| `recaptcha` | `script-src`: `https://www.google.com/recaptcha/`, `https://www.gstatic.com/recaptcha/`; `frame-src`: `https://www.google.com/recaptcha/`, `https://recaptcha.google.com/recaptcha/`; `connect-src`: `https://www.google.com/recaptcha/` |

Wenn ein Reverse-Proxy oder CDN vor dem Server die CSP überschreibt, füge dieselben Quellen für diese drei Seiten hinzu.

<!-- source: what-to-write-in-the-privacy-policy d601071ebd4c -->
## Was in die Datenschutzerklärung gehört

Bei aktiviertem CAPTCHA verbindet sich der Browser jeder Person, die die Seite für Anmeldung, Registrierung oder Anforderung des Zurücksetzens des Passworts öffnet, direkt mit dem Dienst, und der Dienst erhält die IP-Adresse, Browserinformationen und Cookies.
Die Datenschutzerklärung des Servers (`my instance set privacy_policy`, siehe [Servereinstellungen](instance-settings.md)) schreibst du selbst; der Server fügt ihr nichts hinzu. Die [Vorlage für die Datenschutzerklärung](privacy-policy-template.md) hat eine Zeile für den Dienst. Schreibe mindestens Folgendes.

- Welchen Dienst (den Namen des Anbieters) du nutzt und warum: um automatisierte Anmeldungen, Registrierungen und Anforderungen zum Zurücksetzen des Passworts zu verhindern.
- Was den Dienst erreicht (IP-Adresse, Browser- und Geräteinformationen, Cookies, Interaktionen mit dem Widget) und dass nur die Seiten für Anmeldung, Registrierung und Anforderung des Zurücksetzens des Passworts es senden.
- Einen Link zur Datenschutzerklärung des Dienstes (Turnstile: <https://www.cloudflare.com/privacypolicy/>, hCaptcha: <https://www.hcaptcha.com/privacy>, reCAPTCHA: <https://policies.google.com/privacy>).
- Bei reCAPTCHA, dass die Datenschutzerklärung und die Nutzungsbedingungen von Google (<https://policies.google.com/terms>) gelten.
