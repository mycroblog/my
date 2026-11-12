<!-- source: captcha-bot-protection-turnstile--hcaptcha--recaptcha 2c065ac1d2fb -->
# Protección contra bots con CAPTCHA (Turnstile / hCaptcha / reCAPTCHA)

CAPTCHA está desactivado de forma predeterminada: la configuración [`captcha`](../reference/configuration.md#captcha) es `none`. Las páginas de inicio de sesión, registro y restablecimiento de contraseña funcionan entonces sin JavaScript y no envían nada a terceros.
Cuando eliges un servicio de CAPTCHA, el widget del servicio aparece en tres formularios: inicio de sesión (`/auth/sign_in`), registro (`/auth/sign_up`) y la solicitud de un enlace para restablecer la contraseña
(`/auth/forgot_password`, solo en servidores que envían correo). El servidor verifica el token que produce el widget con la API siteverify del servicio.
No puedes proteger solo algunos de los formularios.

<!-- source: choosing-a-service 22ddd3cc639f -->
## Elegir un servicio

| `captcha` | Servicio | Costo | Qué hace el usuario |
| --- | --- | --- | --- |
| `turnstile` (recomendado) | Cloudflare Turnstile | Gratuito, sin límite | Normalmente nada; una casilla solo cuando hace falta |
| `hcaptcha` | hCaptcha | Tiene un nivel gratuito | Una casilla, y un desafío con imágenes cuando hace falta |
| `recaptcha` | Google reCAPTCHA v2 (casilla) | Gratuito hasta 10 000 evaluaciones al mes (consulta más abajo) | Una casilla, y un desafío con imágenes o audio cuando hace falta |

reCAPTCHA v3 (solo puntuación) y reCAPTCHA Enterprise no son compatibles.

<!-- source: creating-keys 38cd93394638 -->
## Crear claves

Cada servicio te da un par de claves: una **clave del sitio** pública y una **clave secreta** que solo tiene el servidor. Registra como dominio el nombre de host de la configuración `host` (sin el puerto).
El servidor rechaza un token cuando el nombre de host de la respuesta de siteverify no coincide con `host`.

- **Turnstile**: en el panel de Cloudflare, añade un widget en «Turnstile» y registra el nombre de host. Elige el modo de widget «Managed».
- **hCaptcha**: añade un sitio en el panel de hCaptcha y registra el nombre de host. La clave secreta está en la configuración de la cuenta.
- **reCAPTCHA**: en la consola de administración de Google reCAPTCHA, elige el tipo «Challenge (v2)» con la casilla «I'm not a robot» y registra el dominio.
  Las claves pertenecen a un proyecto de Google Cloud.

Las claves de prueba de cada servicio (las que siempre tienen éxito) sirven para comprobar que el widget aparece y produce un token.
Sin embargo, siteverify con claves de prueba devuelve un nombre de host fijo (`example.com` para Turnstile, `dummy-key-pass` para hCaptcha, etc.), así que el inicio de sesión y el registro en este servidor fallan con «Completa la comprobación de que no eres un bot e inténtalo de nuevo.» si las usas.

<!-- source: configuring 3301db5c6286 -->
## Configurar

```sh
MY_CAPTCHA=turnstile
MY_CAPTCHA_SITE_KEY=0x4AAAAAAA...
MY_CAPTCHA_SECRET_KEY_FILE=/run/secrets/my_captcha_secret_key
```

- Pasa la clave secreta solo mediante la variable de entorno `MY_CAPTCHA_SECRET_KEY` o mediante `MY_CAPTCHA_SECRET_KEY_FILE`, que apunta a un archivo. Los registros muestran `[REDACTED]`.
- Si eliges un `captcha` y falta una clave, `my config check` informa de un problema y el servidor no arranca.
- El servidor no prueba las claves al arrancar. Cuando el servicio rechaza la clave secreta o la clave del sitio, la primera verificación registra el ERROR
  «the CAPTCHA service refused the secret key or the site key». Hasta que lo arregles, las peticiones se tratan como en «Cuando el servicio no está disponible», más abajo.
- Los cambios surten efecto cuando reinicias el servidor.

<!-- source: how-verification-works c5cc12dd3769 -->
## Cómo funciona la verificación

- El orden es: límite de frecuencia, después verificación de CAPTCHA, después comprobación de la contraseña, procesamiento del registro o envío del correo de restablecimiento. Una petición que supera el límite de frecuencia nunca se envía al servicio.
- Un registro con un código de invitación válido también se verifica.
- Un inicio de sesión que no supera la verificación cuenta como un inicio de sesión fallido para la dirección IP del cliente. No cuenta contra la cuenta, de modo que nadie puede bloquear a otra persona.
  Un registro que no supera la verificación cuenta como un intento de registro (consulta el límite de frecuencia en [registro](registration.md)). Una solicitud de enlace de restablecimiento cuenta como un intento de solicitud del mismo modo.
- En Turnstile, el widget lleva un nombre de acción (`sign_in` / `sign_up` / `password_reset`) y el servidor comprueba que la respuesta tiene la misma acción. Un token de un formulario no se puede usar en otro.
- El servidor no envía al servicio la dirección IP del usuario (`remoteip`). Un token es de un solo uso, así que el servidor no reintenta. El tiempo de espera es de 10 segundos.

<!-- source: when-the-service-is-unreachable fe6a6d5915ea -->
### Cuando el servicio no está disponible

El servicio no se puede alcanzar, agota el tiempo de espera, devuelve un 5xx o rechaza las claves:

| Formulario | Tratamiento |
| --- | --- |
| Registro y solicitud de un enlace de restablecimiento | Se rechaza (`503`) con «La comprobación de que no eres un bot no está disponible ahora. Inténtalo más tarde.». De lo contrario, los bots podrían provocar correos y registros. |
| Inicio de sesión | Se deja pasar sin CAPTCHA, y el servidor registra el WARN «the CAPTCHA service could not check a sign-in; letting it through». El límite de frecuencia y la comprobación de la contraseña se siguen aplicando. |

El inicio de sesión se deja pasar para que una caída del servicio o una clave incorrecta no bloquee a todos, administradores incluidos. Si el WARN sigue apareciendo, comprueba el estado del servicio y las claves.

<!-- source: recaptcha-free-tier 4f413b07dbb9 -->
### Nivel gratuito de reCAPTCHA

Por encima del nivel gratuito de 10 000 evaluaciones al mes, el siteverify de reCAPTCHA **siempre tiene éxito** (así que CAPTCHA no ofrece protección en ese tiempo).
El servidor no puede detectar el exceso, así que comprueba tu uso en la consola de Google Cloud.

<!-- source: pages-and-content-security-policy b0cf9741b2ce -->
## Páginas y Content-Security-Policy

- El idioma del widget sigue la configuración regional de la página y su tema sigue el esquema de color del navegador (claro u oscuro).
  El widget de Turnstile sigue el esquema de color por sí mismo. hCaptcha y reCAPTCHA se eligen con la pista de cliente `Sec-CH-Prefers-Color-Scheme`, que envía el navegador; los navegadores que no la envían reciben el tema claro.
- Junto al widget, la página indica qué servicio realiza la comprobación, que el servicio recibe la dirección IP y la información del navegador, y enlaza con la política de privacidad del servicio
  (y con los términos, en hCaptcha y reCAPTCHA). En reCAPTCHA esto también cumple la frase que exige Google: «This site is protected by reCAPTCHA and the Google Privacy Policy and Terms of Service apply.»
- En un navegador sin JavaScript, un mensaje de `<noscript>` indica que la comprobación necesita JavaScript. Un usuario que no puede resolver el CAPTCHA no puede registrarse ni iniciar sesión,
  y solo puedes ayudarle por otros medios, como crear la cuenta con `my admin create` o definir `captcha = none` durante un tiempo.
- La `Content-Security-Policy` (CSP) se amplía solo en las páginas de inicio de sesión, registro y solicitud de restablecimiento de contraseña mientras CAPTCHA está activado. Nunca se añaden `'unsafe-inline'` ni `'unsafe-eval'`.
  La CSP del cliente web, la API de cliente, la pantalla de consentimiento, la confirmación del correo electrónico y otras páginas no cambia.

| `captcha` | Orígenes añadidos |
| --- | --- |
| `turnstile` | `script-src`, `frame-src`: `https://challenges.cloudflare.com` |
| `hcaptcha` | `script-src`, `frame-src`, `style-src`, `connect-src`: `https://hcaptcha.com`, `https://*.hcaptcha.com` |
| `recaptcha` | `script-src`: `https://www.google.com/recaptcha/`, `https://www.gstatic.com/recaptcha/`; `frame-src`: `https://www.google.com/recaptcha/`, `https://recaptcha.google.com/recaptcha/`; `connect-src`: `https://www.google.com/recaptcha/` |

Si un proxy inverso o una CDN delante del servidor sobrescribe la CSP, añade los mismos orígenes para estas tres páginas.

<!-- source: what-to-write-in-the-privacy-policy d601071ebd4c -->
## Qué escribir en la política de privacidad

Con CAPTCHA activado, el navegador de cualquiera que abra la página de inicio de sesión, de registro o de solicitud de restablecimiento de contraseña se conecta directamente al servicio, y el servicio recibe la dirección IP, la información del navegador y las cookies.
La política de privacidad de la instancia (`my instance set privacy_policy`, consulta [configuración de la instancia](instance-settings.md)) la escribes tú; el servidor no le añade nada. La [plantilla de política de privacidad](privacy-policy-template.md) tiene una fila para el servicio. Escribe al menos lo siguiente.

- Qué servicio (el nombre del proveedor) usas y por qué: para evitar solicitudes automatizadas de inicio de sesión, registro y restablecimiento de contraseña.
- Qué llega al servicio (dirección IP, información del navegador y del dispositivo, cookies, interacciones con el widget), y que solo lo envían las páginas de inicio de sesión, registro y solicitud de restablecimiento de contraseña.
- Un enlace a la política de privacidad del servicio (Turnstile: <https://www.cloudflare.com/privacypolicy/>, hCaptcha: <https://www.hcaptcha.com/privacy>, reCAPTCHA: <https://policies.google.com/privacy>).
- En reCAPTCHA, que se aplican la política de privacidad y los términos de Google (<https://policies.google.com/terms>).
