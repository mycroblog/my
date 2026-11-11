# CAPTCHA services with a siteverify API

- Changing the Turnstile check or its widget: [Cloudflare Turnstile documentation](https://developers.cloudflare.com/turnstile/llms.txt), search for: `siteverify`, `cf-turnstile-response`, `error-codes`, `idempotency_key`, `data-action`
- Changing the hCaptcha check or its widget: [hCaptcha developer guide](https://docs.hcaptcha.com/), search for: `h-captcha-response`, `sitekey`, `sitekey-secret-mismatch`, `https://api.hcaptcha.com/siteverify`
- Changing the reCAPTCHA check: [reCAPTCHA documentation, verifying the user's response](https://developers.google.com/recaptcha/docs/verify), search for: `https://www.google.com/recaptcha/api/siteverify`, `error-codes`, `missing-input-secret`, `hostname`
