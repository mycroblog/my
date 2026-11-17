<!-- source: captcha-bot-protection-turnstile--hcaptcha--recaptcha 2c065ac1d2fb -->
# Protection contre les bots par CAPTCHA (Turnstile / hCaptcha / reCAPTCHA)

Le CAPTCHA est désactivé par défaut : le paramètre [`captcha`](../reference/configuration.md#captcha) vaut `none`. Les pages de connexion, d’inscription et de réinitialisation du mot de passe fonctionnent alors sans JavaScript et n’envoient rien à des tiers.
Lorsque vous choisissez un service de CAPTCHA, le widget du service apparaît sur trois formulaires : la connexion (`/auth/sign_in`), l’inscription (`/auth/sign_up`) et la demande de lien de réinitialisation du mot de passe
(`/auth/forgot_password`, uniquement sur les serveurs qui envoient des e-mails). Le serveur vérifie le jeton produit par le widget auprès de l’API siteverify du service.
Vous ne pouvez pas protéger seulement certains formulaires.

<!-- source: choosing-a-service 22ddd3cc639f -->
## Choisir un service

| `captcha` | Service | Coût | Ce que fait l’utilisateur |
| --- | --- | --- | --- |
| `turnstile` (recommandé) | Cloudflare Turnstile | Gratuit, sans limite | En général rien ; une case à cocher uniquement si nécessaire |
| `hcaptcha` | hCaptcha | Offre gratuite disponible | Une case à cocher, plus un défi d’images si nécessaire |
| `recaptcha` | Google reCAPTCHA v2 (case à cocher) | Gratuit jusqu’à 10 000 évaluations par mois (voir plus bas) | Une case à cocher, plus un défi d’images ou audio si nécessaire |

reCAPTCHA v3 (score seul) et reCAPTCHA Enterprise ne sont pas pris en charge.

<!-- source: creating-keys 38cd93394638 -->
## Créer les clés

Chaque service vous fournit une paire de clés : une **clé de site** publique et une **clé secrète** que seul le serveur détient. Enregistrez comme domaine le nom d’hôte du paramètre `host` (sans le port).
Le serveur rejette un jeton lorsque le nom d’hôte de la réponse siteverify ne correspond pas à `host`.

- **Turnstile** : dans le tableau de bord Cloudflare, ajoutez un widget sous « Turnstile » et enregistrez le nom d’hôte. Choisissez le mode de widget « Managed ».
- **hCaptcha** : ajoutez un site dans le tableau de bord hCaptcha et enregistrez le nom d’hôte. La clé secrète se trouve dans les paramètres du compte.
- **reCAPTCHA** : dans la console d’administration Google reCAPTCHA, choisissez le type « Challenge (v2) » avec la case à cocher « I’m not a robot », et enregistrez le domaine.
  Les clés appartiennent à un projet Google Cloud.

Les clés de test de chaque service (celles qui réussissent toujours) permettent de vérifier que le widget apparaît et produit un jeton.
Toutefois, siteverify renvoie avec les clés de test un nom d’hôte fixe (`example.com` pour Turnstile, `dummy-key-pass` pour hCaptcha, etc.), si bien que la connexion et l’inscription sur ce serveur échouent avec « Vérifiez à nouveau » lorsque vous les utilisez.

<!-- source: configuring 3301db5c6286 -->
## Configurer

```sh
MY_CAPTCHA=turnstile
MY_CAPTCHA_SITE_KEY=0x4AAAAAAA...
MY_CAPTCHA_SECRET_KEY_FILE=/run/secrets/my_captcha_secret_key
```

- Transmettez la clé secrète uniquement par la variable d’environnement `MY_CAPTCHA_SECRET_KEY` ou par `MY_CAPTCHA_SECRET_KEY_FILE`, qui désigne un fichier. Les journaux affichent `[REDACTED]`.
- Si vous choisissez un `captcha` alors qu’une clé manque, `my config check` signale un problème et le serveur ne démarre pas.
- Le serveur n’essaie pas les clés au démarrage. Lorsque le service refuse la clé secrète ou la clé de site, la première vérification consigne l’erreur ERROR
  « the CAPTCHA service refused the secret key or the site key ». Tant que vous ne l’avez pas corrigée, les requêtes sont traitées comme dans « Quand le service est injoignable » ci-dessous.
- Les modifications prennent effet au redémarrage du serveur.

<!-- source: how-verification-works c5cc12dd3769 -->
## Comment fonctionne la vérification

- L’ordre est le suivant : limitation de débit, puis vérification du CAPTCHA, puis contrôle du mot de passe, traitement de l’inscription ou envoi de l’e-mail de réinitialisation. Une requête qui dépasse la limite de débit n’est jamais envoyée au service.
- Une inscription avec un code d’invitation valide est elle aussi vérifiée.
- Une connexion dont la vérification échoue compte comme une connexion échouée pour l’adresse IP du client. Elle ne compte pas contre le compte, si bien que personne ne peut verrouiller le compte d’une autre personne.
  Une inscription dont la vérification échoue compte comme une tentative d’inscription (voir la limite de débit dans [inscription](registration.md)). Une demande de lien de réinitialisation compte de la même façon comme une tentative de demande.
- Pour Turnstile, le widget porte un nom d’action (`sign_in` / `sign_up` / `password_reset`) et le serveur vérifie que la réponse porte la même action. Un jeton issu d’un formulaire ne peut pas servir sur un autre.
- Le serveur n’envoie pas l’adresse IP de l’utilisateur (`remoteip`) au service. Un jeton est à usage unique, donc le serveur ne réessaie pas. Le délai d’expiration est de 10 secondes.

<!-- source: when-the-service-is-unreachable fe6a6d5915ea -->
### Quand le service est injoignable

Le service est injoignable, dépasse le délai, renvoie une erreur 5xx ou rejette les clés :

| Formulaire | Traitement |
| --- | --- |
| Inscription et demande de lien de réinitialisation | Rejetée (`503`) avec « Réessayez plus tard ». Sinon, des bots pourraient déclencher l’envoi d’e-mails et des inscriptions. |
| Connexion | Laissée passer sans CAPTCHA, et le serveur consigne l’avertissement WARN « the CAPTCHA service could not check a sign-in; letting it through ». La limitation de débit et le contrôle du mot de passe s’appliquent toujours. |

La connexion est laissée passer afin qu’une panne du service ou une clé erronée n’enferme pas tout le monde dehors, administrateurs compris. Si le WARN continue d’apparaître, vérifiez l’état du service et les clés.

<!-- source: recaptcha-free-tier 4f413b07dbb9 -->
### Offre gratuite de reCAPTCHA

Au-delà de l’offre gratuite de 10 000 évaluations par mois, le siteverify de reCAPTCHA **réussit toujours** (le CAPTCHA n’offre alors aucune protection).
Le serveur ne peut pas détecter ce dépassement : surveillez donc votre consommation dans la console Google Cloud.

<!-- source: pages-and-content-security-policy b0cf9741b2ce -->
## Pages et Content-Security-Policy

- La langue du widget suit la langue de la page et son thème suit le jeu de couleurs du navigateur (clair ou sombre).
  Le widget Turnstile suit lui-même le jeu de couleurs. hCaptcha et reCAPTCHA sont réglés grâce à l’indication client `Sec-CH-Prefers-Color-Scheme`, que le navigateur envoie ; les navigateurs qui ne l’envoient pas obtiennent le thème clair.
- À côté du widget, la page indique quel service effectue la vérification, que le service reçoit l’adresse IP et des informations sur le navigateur, et renvoie vers la politique de confidentialité du service
  (et vers les conditions d’utilisation pour hCaptcha et reCAPTCHA). Pour reCAPTCHA, cela tient aussi lieu de la phrase exigée par Google : « This site is protected by reCAPTCHA and the Google Privacy Policy and Terms of Service apply. »
- Dans un navigateur sans JavaScript, un message `<noscript>` indique que la vérification nécessite JavaScript. Un utilisateur qui ne peut pas résoudre le CAPTCHA ne peut ni s’inscrire ni se connecter,
  et vous ne pouvez l’aider que par d’autres moyens, par exemple en créant le compte avec `my admin create` ou en réglant `captcha = none` pendant un moment.
- La `Content-Security-Policy` (CSP) ne s’élargit que sur les pages de connexion, d’inscription et de demande de réinitialisation du mot de passe lorsque le CAPTCHA est activé. `'unsafe-inline'` et `'unsafe-eval'` ne sont jamais ajoutés.
  La CSP du client web, de l’API cliente, de l’écran de consentement, de la confirmation de l’adresse e-mail et des autres pages ne change pas.

| `captcha` | Sources ajoutées |
| --- | --- |
| `turnstile` | `script-src`, `frame-src`: `https://challenges.cloudflare.com` |
| `hcaptcha` | `script-src`, `frame-src`, `style-src`, `connect-src`: `https://hcaptcha.com`, `https://*.hcaptcha.com` |
| `recaptcha` | `script-src`: `https://www.google.com/recaptcha/`, `https://www.gstatic.com/recaptcha/`; `frame-src`: `https://www.google.com/recaptcha/`, `https://recaptcha.google.com/recaptcha/`; `connect-src`: `https://www.google.com/recaptcha/` |

Si un proxy inverse ou un CDN placé devant le serveur remplace la CSP, ajoutez les mêmes sources pour ces trois pages.

<!-- source: what-to-write-in-the-privacy-policy d601071ebd4c -->
## Que mettre dans la politique de confidentialité

Lorsque le CAPTCHA est activé, le navigateur de toute personne qui ouvre la page de connexion, d’inscription ou de demande de réinitialisation du mot de passe se connecte directement au service, et le service reçoit l’adresse IP, des informations sur le navigateur et des cookies.
La politique de confidentialité de l’instance (`my instance set privacy_policy`, voir [paramètres de l’instance](instance-settings.md)) est à rédiger par vous ; le serveur n’y ajoute rien. Le [modèle de politique de confidentialité](privacy-policy-template.md) comporte une ligne pour le service. Écrivez au minimum ce qui suit.

- Quel service (le nom du fournisseur) vous utilisez et pourquoi : pour empêcher les connexions, inscriptions et demandes de réinitialisation du mot de passe automatisées.
- Ce qui parvient au service (adresse IP, informations sur le navigateur et l’appareil, cookies, interactions avec le widget), et que seules les pages de connexion, d’inscription et de demande de réinitialisation du mot de passe l’envoient.
- Un lien vers la politique de confidentialité du service (Turnstile : <https://www.cloudflare.com/privacypolicy/>, hCaptcha : <https://www.hcaptcha.com/privacy>, reCAPTCHA : <https://policies.google.com/privacy>).
- Pour reCAPTCHA, que la politique de confidentialité et les conditions d’utilisation de Google (<https://policies.google.com/terms>) s’appliquent.
