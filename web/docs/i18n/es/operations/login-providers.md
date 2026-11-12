<!-- source: external-login-providers 48a582ab87ec -->
# Proveedores externos de inicio de sesión

Un proveedor externo de inicio de sesión permite a los usuarios iniciar sesión con una cuenta que tienen en otro sitio: un proveedor
OpenID Connect (Google Workspace, Microsoft Entra ID, Keycloak y similares), GitHub, Discord o un servidor de Mastodon.
Los proveedores se configuran con el comando `my login-provider` en un host que tenga la configuración del servidor, o mediante la
API de administración (`/api/v1/admin/login_providers`) con el permiso `manage_login_providers`, que solo tiene el rol integrado
`admin`. El comando funciona antes de que ningún administrador pueda iniciar sesión, así que sirve para la configuración inicial.
En el cliente web, los administradores con ese permiso usan «Proveedores de inicio de sesión» en el menú de administración. La lista
muestra el tipo de cada proveedor, su estado y si permite registrarse; «Añadir un proveedor» abre el formulario que crea uno.
Al elegir un proveedor se abre su página, que muestra su URL de retorno con un botón que la copia y cuántos usuarios están
vinculados, y desde donde cambias sus configuraciones, le das un nuevo secreto de cliente, lo activas, lo desactivas y lo eliminas.

<!-- source: what-external-sign-in-does 25fa37e22a50 -->
## Qué hace el inicio de sesión externo

- Un proveedor solo demuestra quién es alguien en ese proveedor. Una cuenta nunca se vincula a un usuario por su dirección de
  correo electrónico: un usuario vincula una cuenta en «Cuentas vinculadas» de la configuración, o se registra con ella como usuario nuevo.
  Una dirección de correo electrónico que coincide con la de un usuario existente nunca vincula a ese usuario.
- Un usuario vinculado sigue introduciendo el código de dos factores (TOTP) tras iniciar sesión con un proveedor, y un usuario cuya cuenta
  no está activa no puede iniciar sesión.
- Los usuarios nuevos pueden registrarse con un proveedor solo cuando lo permites para ese proveedor (`-allow-signup`) y el
  [modo de registro](registration.md) les deja registrarse; el código de invitación, el CAPTCHA, los bloqueos de registro y el límite de red
  se aplican igual que en el formulario de contraseña. Quien se registra así no tiene contraseña e inicia sesión solo con el proveedor.
  El [diagrama de las vías de registro](registration.md#from-sign-up-to-the-app) muestra ambas formas de registrarse lado a lado.

<!-- source: kinds c9123566b5c5 -->
## Tipos

| Tipo | `-issuer` | Condiciones |
| --- | --- | --- |
| `oidc` | La URL del emisor (`https`), exactamente como la nombran los tokens de ID del proveedor, como `https://accounts.google.com` | `-claim <claim>=<value>`: solo los usuarios cuyo token de ID tiene la reclamación con uno de los valores indicados para ella |
| `github` | Ninguno: el servidor conoce los endpoints de github.com | `-org <organization>`: solo los miembros de una de las organizaciones (añade `-scope read:org`) |
| `discord` | Ninguno: el servidor conoce los endpoints de discord.com | `-guild <server ID>`: solo los miembros de uno de los servidores de Discord (guilds); si no se indica ninguno, entra cualquier usuario de Discord |
| `mastodon` | El dominio web del servidor, como `mastodon.example` (Mastodon 4.3 o posterior) | Ninguna |

Para Google Workspace, limita los usuarios a tu organización con `-claim hd=<your domain>`. No te fíes del dominio de la
dirección de correo electrónico.

<!-- source: examples 0d19ab463197 -->
## Ejemplos

<!-- source: google b99294b8539d -->
### Google

En la consola de Google Cloud, crea un cliente OAuth de tipo «Web application». Para limitarlo a tu organización, establece el
tipo de usuario de la pantalla de consentimiento de OAuth en «Internal» (disponible cuando el proyecto pertenece a una organización
de Google Workspace), y añade también la condición `hd`: el tipo de usuario restringe a quién deja entrar Google, y la condición hace que este
servidor compruebe la reclamación `hd` de cada token de ID.

```sh
printf '%s\n' "$CLIENT_SECRET" | my login-provider create -kind oidc -name Google \
  -issuer https://accounts.google.com -client-id 123.apps.googleusercontent.com -claim hd=example.com
```

<!-- source: keycloak-and-other-openid-connect-providers 0840dc38a4e0 -->
### Keycloak y otros proveedores OpenID Connect

El emisor es la URL del realm, como `https://sso.example.com/realms/main`, exactamente como la nombran los tokens de ID del proveedor.
Crea un cliente confidencial con el flujo estándar y los ámbitos `openid`, `email` y `profile`. Si el proveedor
está en una red privada, inclúyelo en `login_provider_allowed_destinations` ([más abajo](#providers-on-an-internal-network)).
Usa `-claim` para exigir una reclamación, como una reclamación de grupo, con uno de los valores indicados.

<!-- source: discord 00e9659d1eae -->
### Discord

Crea una aplicación en el [Portal de desarrolladores de Discord](https://discord.com/developers/applications), abre «OAuth2» y
entrega al servidor el ID de cliente y el secreto de cliente. El servidor pide los ámbitos `identify` y `email`, y también
`guilds` cuando indicas servidores con `-guild`. Un usuario se identifica por su ID numérico de Discord; el nombre de usuario y el nombre visible
solo sugieren el nombre de usuario en el formulario de registro. La dirección de correo electrónico se considera verificada solo cuando Discord dice que está
verificada; de lo contrario, el usuario la confirma por correo como cualquier otra dirección.

Con `-guild`, el servidor comprueba en cada inicio de sesión, registro y vinculación que el usuario pertenece a uno de los servidores indicados
y, si no, lo rechaza. Sin él, cualquier usuario de Discord puede iniciar sesión. Un ID de guild es un número de hasta 20 dígitos: en
Discord, activa el «Developer Mode» (User Settings, Advanced), luego haz clic con el botón derecho en el icono del servidor y elige «Copy Server
ID».

```sh
printf '%s\n' "$DISCORD_SECRET" | my login-provider create -kind discord -name Discord \
  -client-id 123456789012345678 -guild 987654321098765432
```

Añade la URL de retorno en «OAuth2», «Redirects» de la aplicación, después de que `add` la muestre.

<!-- source: github a7181ea6edd6 -->
### GitHub

Crea una OAuth App (Settings, Developer settings, OAuth Apps) y entrégale el ID de cliente y el secreto. El servidor pide
el ámbito `user:email`, y también `read:org` cuando indicas organizaciones con `-org`. Con `-org`, el servidor comprueba
en cada inicio de sesión que el usuario pertenece a una de las organizaciones, de modo que los miembros de otras organizaciones son rechazados.

<!-- source: set-up-a-provider 8e0bb73251d2 -->
## Configurar un proveedor

El URI de redirección que necesita el proveedor es la URL de retorno del proveedor, `https://<host>/auth/external/<id>/callback`,
que contiene el ID que el servidor asigna al proveedor. Por tanto:

1. Registra un cliente OAuth en el proveedor (una «OAuth App» en GitHub) con cualquier URI de redirección por ahora, y anota el
   ID de cliente y el secreto de cliente que te dé.
2. Ejecuta `my login-provider create` con ellos. Muestra la URL de retorno.
3. Cambia el URI de redirección del cliente en el proveedor a esa URL de retorno.

```sh
# The client secret is read from standard input, or prompted for in a terminal.
printf '%s\n' "$CLIENT_SECRET" | my login-provider create -kind oidc -name "Example SSO" \
  -issuer https://sso.example.com -client-id my-client-id -claim hd=example.com
printf '%s\n' "$GITHUB_SECRET" | my login-provider create -kind github -name GitHub \
  -client-id Iv1.0123456789abcdef -org example-org -scope read:org
```

<!-- source: mastodon 6f167bff1f67 -->
### Mastodon

Un proveedor `mastodon` es un servidor de Mastodon que tú nombras; los usuarios de otros servidores no pueden iniciar sesión con él. El servidor
debe ser Mastodon 4.3 o posterior: `add` lee los metadatos de su servidor de autorización
(`/.well-known/oauth-authorization-server`) y rechaza un servidor que no los tenga. Sin `-client-id`, `add` registra un cliente
en el propio servidor, con la URL de retorno y el ámbito `profile`, y guarda el ID de cliente y el secreto que obtiene, de modo que
no hay nada que hacer en el servidor:

```sh
my login-provider create -kind mastodon -name mastodon.example -issuer mastodon.example
```

Si el servidor no deja que otros registren clientes, `add` falla. En ese caso, crea una aplicación con el ámbito `profile` y la
URL de retorno en las configuraciones de desarrollo del servidor, y entrega su ID de cliente y su secreto como para los demás tipos
(la URL de retorno contiene el ID del proveedor, así que sigue los tres pasos anteriores).

Los usuarios inician sesión solo con el ámbito `profile`, que lee su cuenta y nada más. Mastodon no entrega ninguna dirección de correo electrónico,
así que quien se registra con él confirma la dirección por correo. El formulario de registro sugiere el nombre de usuario de la cuenta.
Mientras el dominio del servidor, o un dominio superior, tenga una acción de dominio ([acciones de dominio](moderation.md#domain-actions)), su botón desaparece y
nadie inicia sesión, se registra ni vincula una cuenta con él. El servidor de un proveedor no cambia; configura un proveedor nuevo
para otro servidor.

El secreto de cliente se guarda cifrado con la clave de cifrado de claves ([gestión de claves](key-management.md)) y nunca se vuelve a mostrar,
ni con el comando ni con la API de administración.

Un proveedor nuevo está activado. Por defecto, los usuarios pueden vincular su cuenta existente a él (`-allow-link`), pero nadie puede registrarse
con él: añade `-allow-signup` para que los usuarios nuevos puedan registrarse con él, algo que el modo de registro sigue limitando
([registro](registration.md)).

<!-- source: change-rotate-disable-and-delete 55a0d23ac0d0 -->
## Cambiar, rotar, desactivar y eliminar

```sh
my login-provider list                                 # every provider with its callback URL and linked users
my login-provider set -name "Corp SSO" <id>            # changes only the settings it names
my login-provider set -claim '' <id>                   # '' removes every -claim (or -scope, -org)
my login-provider rotate-secret <id>                   # reads the new client secret like add
my login-provider disable <id>
my login-provider enable <id>
my login-provider delete <id>
```

- `set` cambia solo las configuraciones que nombra. `-scope`, `-claim` y `-org` reemplazan la lista entera. El tipo de un proveedor
  no cambia; configura un proveedor nuevo en su lugar.
- `rotate-secret` reemplaza el secreto de cliente por el que introduzcas. Rótalo primero en el proveedor y luego aquí.
- `disable` conserva el proveedor y las cuentas vinculadas a él, pero nadie inicia sesión, se registra ni vincula con él, y la
  página de inicio de sesión no lo muestra, hasta que ejecutes `enable`. `disable` y `delete` muestran cuántos usuarios vinculados ya no pueden
  iniciar sesión con el proveedor y qué hace ahora la [limitación a proveedores de inicio de sesión](#limit-sign-in-to-login-providers) mientras su
  configuración está activada.
- `delete` elimina el proveedor definitivamente.

<!-- source: users-who-sign-in-only-through-a-provider 11dc6f466cb7 -->
### Usuarios que inician sesión solo mediante un proveedor

Desactivar o eliminar un proveedor no afecta a los usuarios. Un usuario que tiene una contraseña o una passkey, o tiene otra cuenta
vinculada, inicia sesión de ese modo, salvo que [el inicio de sesión esté limitado a proveedores de inicio de sesión](#limit-sign-in-to-login-providers) y otro
proveedor siga activado. Un usuario que no tiene nada de esto ya no puede iniciar sesión en absoluto: `disable` y `delete` muestran cuántos
usuarios vinculados pierden el proveedor, y el cliente web muestra el número en la confirmación. `enable` restablece a los
usuarios de un proveedor desactivado; un proveedor eliminado no puede volver.

Para recuperar a un usuario así, emite un enlace de restablecimiento de contraseña en la página de Gestión de la cuenta en las pantallas de administración, o con
`my admin account password-reset-link @alice`, y entrégaselo al usuario ([registro](registration.md#password-reset-and-change)).
Después, el usuario inicia sesión con la nueva contraseña.

Cada cambio se registra en el registro de auditoría con el nombre visible del proveedor, como la CLI o como el administrador que lo hizo
mediante la API de administración.

<!-- source: limit-sign-in-to-login-providers bb5ebf61b647 -->
## Limitar el inicio de sesión a proveedores de inicio de sesión

La configuración de instancia `login_providers_only` (desactivada por defecto) hace que los proveedores sean la única forma de iniciar sesión y registrarse,
mientras haya al menos un proveedor activado. Actívalo en la página «Registro» de «Configuración del servidor» del cliente web, o con la CLI:

```sh
my instance set login_providers_only true
```

La confirmación del cliente web y la salida del comando indican cuántos usuarios locales activos no tienen vínculo con un
proveedor activado. El cliente web indica también cuándo eso te incluye a ti. Avisa y no rechaza: decides tú.

Mientras la limitación se aplica:

- Las contraseñas y las passkeys no permiten iniciar sesión a nadie, y la página de inicio de sesión muestra solo los proveedores. También se rechaza un inicio de sesión que
  hubiera empezado con una contraseña o una passkey y esté esperando su código de dos factores.
- Nadie se registra con una contraseña. La página de registro muestra los proveedores que permiten registros (`-allow-signup`), según
  el [modo de registro](registration.md) como de costumbre. Si no hay ningún proveedor así, el registro figura como `closed`, también para los clientes
  y en NodeInfo.
- Nadie solicita ni usa un enlace de restablecimiento de contraseña. Los enlaces siguen siendo válidos hasta que caducan y funcionan si la limitación termina a tiempo.
- Los usuarios no pueden establecer ni cambiar una contraseña, ni añadir una passkey, porque ninguna permitiría iniciar sesión. Sí pueden eliminarlas.
- `my admin account password-reset-link` y las pantallas de administración siguen emitiendo un enlace, e indican que no funciona hasta que la
  limitación termine. `my admin create` sigue creando un administrador y avisa de que la contraseña no permite iniciar sesión.
- Un usuario no puede desvincular su última cuenta de un proveedor activado.

Lo que sigue funcionando:

- Las sesiones, los tokens de acceso de aplicaciones y los tokens de actualización siguen siendo válidos. La limitación no termina ninguno.
- Una contraseña o una passkey siguen confirmando operaciones dentro de una sesión. Así es como un usuario existente pasa a un proveedor: con
  la sesión iniciada, abre «Cuentas vinculadas» en la configuración, confirma con la contraseña o la passkey e inicia sesión en el proveedor.
- El inicio de sesión, el registro y la vinculación en un proveedor, y el código de dos factores tras el inicio de sesión de un proveedor.

Cada rechazo se redacta igual sea cual sea la dirección de correo electrónico, la contraseña o el token enviados, por lo que no dice nada sobre ninguna cuenta.

**Los usuarios sin vínculo no pueden iniciar sesión cuando sus sesiones terminan, hasta que actúes.** Una cuenta nunca se vincula por la dirección de correo electrónico,
así que el inicio de sesión de un proveedor tampoco los encuentra. Pídeles que vinculen una cuenta mientras tienen la sesión iniciada. Un usuario que
perdió su sesión necesita que termines la limitación durante un tiempo, o que lo mantengas fuera. Esto incluye a un usuario con `disable_login`,
`suspend` o `ban`, que sin vínculo no tiene página de inicio de sesión desde la que [apelar](moderation.md).
Vincular también requiere que el navegador tenga la sesión iniciada en las páginas de inicio de sesión: un usuario cuya aplicación sigue con la sesión iniciada, pero cuya sesión
de navegador terminó, no puede vincular mientras la limitación se aplica y tiene que esperar a que actúes. **Anuncia el cambio a tus usuarios antes de
activar la limitación,** para que vinculen una cuenta mientras sus sesiones duran.

<!-- source: end-the-limit 77f5c71d013b -->
### Terminar la limitación

La limitación sigue a la configuración y a los proveedores de una petición a la siguiente, sin reiniciar.

- Un administrador que se ha quedado fuera ejecuta esto en un host que tenga la configuración del servidor:

  ```sh
  my instance set login_providers_only false
  ```

- Cuando no hay ningún proveedor activado, la limitación no se aplica: desactivar o eliminar el último proveedor vuelve a abrir el inicio de sesión con contraseña y passkey,
  diga lo que diga la configuración. Los usuarios sin contraseña ni passkey siguen necesitando un
  [enlace de restablecimiento](registration.md#password-reset-and-change).
- Con la configuración activada y ningún proveedor activado, configurar o activar un proveedor inicia la limitación de inmediato; un proveedor nuevo está
  activado en cuanto se configura. El cliente web avisa antes, con cuántos usuarios activos no tienen vínculo con ese proveedor y
  si eso te incluye a ti. Avisa solo ante la acción que iniciaría la limitación. `my login-provider create`, `enable`,
  `disable` y `delete` muestran qué hace ahora la limitación mientras la configuración está activada.
- Cuando una caída del proveedor deja fuera a todos, usa el mismo comando.

<!-- source: re-authentication 8ec1742f8811 -->
## Reautenticación

Las operaciones que piden la contraseña o una passkey (establecer una contraseña, añadir una passkey, eliminar o mover la cuenta,
vincular otra cuenta) piden a un usuario que no tiene ninguna de las dos que vuelva a iniciar sesión en un proveedor vinculado ([registro](registration.md)).
A un proveedor OpenID Connect se le pide que autentique de nuevo al usuario (`max_age=0` y `prompt=login`) y el servidor
rechaza una autenticación anterior; a un servidor de Mastodon se le pide con `force_login=true`. GitHub y Discord no tienen forma de
pedir un inicio de sesión reciente, así que un usuario que sigue con la sesión iniciada allí queda confirmado sin introducir nada. El servidor sigue comprobando
que la cuenta es la vinculada, pero es una prueba más débil de que la persona está frente al teclado. Prefiere un proveedor OpenID
Connect, o haz que los usuarios conserven una contraseña o una passkey, cuando esto importe.

<!-- source: providers-on-an-internal-network 7f19e187a375 -->
## Proveedores en una red interna

El servidor llega a los proveedores a través de su protección de HTTP saliente, que rechaza las direcciones de bucle local y privadas. Para usar un
proveedor de tu propia red, como un Keycloak junto al servidor, incluye su host o rango de direcciones en
[`login_provider_allowed_destinations`](../reference/configuration.md#login_provider_allowed_destinations).
