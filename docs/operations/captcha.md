# CAPTCHA bot protection (Turnstile / hCaptcha / reCAPTCHA)

This guide is for operators. The configuration keys are listed in the [configuration reference](../reference/configuration.md#captcha).

CAPTCHA is off by default (`captcha = none`). The sign-in, sign-up and password-reset pages then work without JavaScript and send nothing to third parties.
When you pick a CAPTCHA service, the service's widget appears on three forms: sign-in (`/auth/sign_in`), sign-up (`/auth/sign_up`) and the request for a password-reset link
(`/auth/forgot_password`, only on servers that send mail). The server verifies the token the widget produces with the service's siteverify API.
You cannot protect only some of the forms.

## Choosing a service

| `captcha` | Service | Cost | What the user does |
| --- | --- | --- | --- |
| `turnstile` (recommended) | Cloudflare Turnstile | Free, no limit | Usually nothing; a checkbox only when needed |
| `hcaptcha` | hCaptcha | Has a free tier | A checkbox, plus an image challenge when needed |
| `recaptcha` | Google reCAPTCHA v2 (checkbox) | Free up to 10,000 assessments a month (see below) | A checkbox, plus an image or audio challenge when needed |

reCAPTCHA v3 (score only) and reCAPTCHA Enterprise are not supported.

## Creating keys

Every service gives you a pair of keys: a public **site key** and a **secret key** that only the server holds. Register the host name of the setting `host` (without the port) as the domain.
The server rejects a token when the host name in the siteverify response does not match `host`.

- **Turnstile**: in the Cloudflare dashboard, add a widget under "Turnstile" and register the host name. Choose the "Managed" widget mode.
- **hCaptcha**: add a site in the hCaptcha dashboard and register the host name. The secret key is in the account settings.
- **reCAPTCHA**: in the Google reCAPTCHA admin console, choose the type "Challenge (v2)" with the "I'm not a robot" checkbox, and register the domain.
  The keys belong to a Google Cloud project.

Each service's test keys (the ones that always succeed) are good for checking that the widget appears and produces a token.
However, siteverify with test keys returns a fixed host name (`example.com` for Turnstile, `dummy-key-pass` for hCaptcha, and so on), so sign-in and sign-up on this server fail with "Check again" when you use them.

## Configuring

```sh
MY_CAPTCHA=turnstile
MY_CAPTCHA_SITE_KEY=0x4AAAAAAA...
MY_CAPTCHA_SECRET_KEY_FILE=/run/secrets/my_captcha_secret_key
```

- Pass the secret key only through the environment variable `MY_CAPTCHA_SECRET_KEY` or through `MY_CAPTCHA_SECRET_KEY_FILE`, which points to a file. Logs show `[REDACTED]`.
- If you choose a `captcha` and a key is missing, `my config check` reports a problem and the server does not start.
- The server does not try the keys at startup. When the service refuses the secret key or the site key, the first verification logs the ERROR
  "the CAPTCHA service refused the secret key or the site key". Until you fix it, requests are handled as in "When the service is unreachable" below.
- Changes take effect when you restart the server.

## How verification works

- The order is rate limit, then CAPTCHA verification, then password check, sign-up processing or sending the reset mail. A request over the rate limit is never sent to the service.
- A sign-up with a valid invite code is verified too.
- A sign-in that fails verification counts as a failed login for the client IP address. It does not count against the account, so nobody can lock another person out.
  A sign-up that fails verification counts as a sign-up attempt (see the rate limit in [registration](registration.md)). A request for a reset link counts as a request attempt in the same way.
- For Turnstile, the widget carries an action name (`sign_in` / `sign_up` / `password_reset`) and the server checks that the response has the same action. A token from one form cannot be used on another.
- The server does not send the user's IP address (`remoteip`) to the service. A token is single-use, so the server does not retry. The timeout is 10 seconds.

### When the service is unreachable

The service is unreachable, times out, returns a 5xx, or rejects the keys:

| Form | Handling |
| --- | --- |
| Sign-up and request for a reset link | Rejected (`503`) with "Try again later". Bots could otherwise trigger mail and sign-ups. |
| Sign-in | Let through without CAPTCHA, and the server logs the WARN "the CAPTCHA service could not check a sign-in; letting it through". The rate limit and the password check still apply. |

Sign-in is let through so that an outage of the service or a wrong key does not lock everyone out, administrators included. If the WARN keeps appearing, check the service status and the keys.

### reCAPTCHA free tier

Above the free tier of 10,000 assessments a month, reCAPTCHA's siteverify **always succeeds** (so CAPTCHA gives no protection in that time).
The server cannot detect the excess, so check your usage in the Google Cloud console.

## Pages and Content-Security-Policy

- The widget's language follows the page locale and its theme follows the browser's color scheme (light or dark).
  The Turnstile widget follows the color scheme itself. hCaptcha and reCAPTCHA are chosen with the client hint `Sec-CH-Prefers-Color-Scheme`, which the browser sends; browsers that do not send it get the light theme.
- Next to the widget, the page says which service performs the check, that the service receives the IP address and browser information, and links to the service's privacy policy
  (and to the terms for hCaptcha and reCAPTCHA). For reCAPTCHA this also serves as the sentence Google requires: "This site is protected by reCAPTCHA and the Google Privacy Policy and Terms of Service apply."
- In a browser without JavaScript, a `<noscript>` message says the check needs JavaScript. A user who cannot solve the CAPTCHA can neither sign up nor sign in,
  and you can only help them by other means, such as creating the account with `my admin create` or setting `captcha = none` for a while.
- The `Content-Security-Policy` (CSP) widens only on the sign-in, sign-up and password-reset request pages while CAPTCHA is enabled. `'unsafe-inline'` and `'unsafe-eval'` are never added.
  The CSP of the web client, the client API, the consent screen, the email confirmation and other pages does not change.

| `captcha` | Sources added |
| --- | --- |
| `turnstile` | `script-src`, `frame-src`: `https://challenges.cloudflare.com` |
| `hcaptcha` | `script-src`, `frame-src`, `style-src`, `connect-src`: `https://hcaptcha.com`, `https://*.hcaptcha.com` |
| `recaptcha` | `script-src`: `https://www.google.com/recaptcha/`, `https://www.gstatic.com/recaptcha/`; `frame-src`: `https://www.google.com/recaptcha/`, `https://recaptcha.google.com/recaptcha/`; `connect-src`: `https://www.google.com/recaptcha/` |

If a reverse proxy or CDN in front of the server overrides the CSP, add the same sources for these three pages.

## What to write in the privacy policy

With CAPTCHA enabled, the browser of anyone who opens the sign-in, sign-up or password-reset request page connects directly to the service, and the service receives the IP address, browser information and cookies.
The instance's privacy policy (`my instance set privacy_policy`, see [instance settings](instance-settings.md)) is yours to write; the server does not add anything to it. Write at least the following.

- Which service (the provider's name) you use and why: to prevent automated sign-in, sign-up and password-reset requests.
- What reaches the service (IP address, browser and device information, cookies, widget interactions), and that only the sign-in, sign-up and password-reset request pages send it.
- A link to the service's privacy policy (Turnstile: <https://www.cloudflare.com/privacypolicy/>, hCaptcha: <https://www.hcaptcha.com/privacy>, reCAPTCHA: <https://policies.google.com/privacy>).
- For reCAPTCHA, that Google's privacy policy and terms (<https://policies.google.com/terms>) apply.
