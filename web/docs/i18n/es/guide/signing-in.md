<!-- source: signing-up-and-signing-in e881fcb0feaa -->
# Registrarse e iniciar sesión

Cómo crear una cuenta en un servidor de mycroblog, un servidor que ejecuta `my`, e iniciar sesión en él. Cada servidor lo
gestionan sus propios operadores, así que lo que ofrece un servidor (quién puede registrarse, qué cronologías pueden leer
los visitantes, si el chat de voz está activado) depende de ellos. La página «Acerca de» del servidor y los documentos
que enlaza al registrarte te indican sus reglas; esta guía solo describe cómo se comporta `my`.

<!-- source: creating-an-account 6f66cdf29cd2 -->
## Crear una cuenta

Elige «Registrarse» en la navegación (o «Crear una cuenta» en la página de inicio de sesión). El formulario pide:

- **Nombre de usuario**: letras (a-z), dígitos y guiones bajos. Tu identificador es `@username@this-server`, y no puedes cambiar el nombre de usuario después.
- **Correo electrónico**: inicias sesión con él.
- **Contraseña**: el formulario muestra la longitud permitida bajo el campo.
- **Código de invitación**, cuando el servidor lo pide o lo acepta.
- **¿Por qué quieres unirte? (opcional)**, cuando el servidor revisa las cuentas nuevas.
- Una casilla para confirmar que has leído los términos y las políticas del servidor, cuando el servidor publica alguno.

Cuáles de estos campos ves depende de cómo acepta el servidor las cuentas nuevas:

| El servidor | Qué haces |
| --- | --- |
| Es abierto | Te registras. Un código de invitación es opcional. |
| Revisa las cuentas nuevas | Te registras y dices por qué quieres unirte. Un moderador aprueba la cuenta antes de que puedas usarla. Un código de invitación te permite usar la cuenta de inmediato. |
| Acepta solo invitaciones | Escribes el código de invitación que te dieron, o abres el enlace de invitación, que lo rellena. |
| Está cerrado | La página dice que el servidor no acepta cuentas nuevas. |
| Registra solo mediante proveedores de inicio de sesión | Te registras con un proveedor que ofrece el servidor (por ejemplo «Iniciar sesión con GitHub») en lugar de una contraseña. |

Si el servidor envía correo, te envía un enlace para confirmar tu dirección de correo. Ábrelo y elige «Confirmar la dirección de correo».
Hasta que la cuenta se apruebe y la dirección se confirme, puedes iniciar sesión pero solo ves aquello que estás esperando
(«Tu cuenta aún no está lista»).

Después del registro, un paso de bienvenida pide un nombre para mostrar y un avatar, ambos opcionales («Omitir por ahora»); mientras no sigues a nadie, tu
cronología de inicio muestra entonces personas que el servidor sugiere, si sugiere alguna.

<!-- source: signing-in fcfbfa76f1d0 -->
## Iniciar sesión

Elige «Iniciar sesión» en la navegación. El cliente web te lleva a la página de inicio de sesión del servidor y te
devuelve con la sesión iniciada. En esa página puedes:

- Escribir tu dirección de correo electrónico y tu contraseña.
- Elegir «Iniciar sesión con una llave de acceso», una vez que hayas añadido una llave de acceso en «Llaves de acceso» de la configuración.
- Elegir un botón de «O inicia sesión con», cuando el servidor ofrece proveedores de inicio de sesión y has vinculado una cuenta de ese proveedor en «Cuentas vinculadas» de la configuración.
- Elegir «¿Has olvidado tu contraseña?» para recibir un enlace que establece una contraseña nueva. Establecer una contraseña nueva cierra tus sesiones en todas partes. Un servidor que no envía correo electrónico no puede enviar el enlace: pregunta a sus administradores.

Si activaste la autenticación en dos pasos, la página te pide entonces el código de seis dígitos de tu aplicación de
autenticación, o uno de tus códigos de recuperación. Tras demasiados intentos fallidos, la página te hace esperar unos
minutos e indica cuánto.

<!-- source: an-account-that-cannot-be-used-yet 8e6bc1791f14 -->
### Una cuenta que aún no se puede usar

Después de iniciar sesión puede que veas una página en lugar de la aplicación:

- Tu registro está esperando la aprobación de un moderador, o no fue aprobado.
- Tienes que confirmar tu dirección de correo electrónico. «Enviar el enlace de nuevo» envía un enlace nuevo.
- Un moderador ha restringido tu cuenta. La página lista las acciones vigentes y te permite apelar una vez («Apelar esta acción»). Las mismas acciones aparecen en «Estado de la cuenta», en la configuración.
- Desactivaste la cuenta. «Reactivar tu cuenta» la recupera hasta la fecha en que se elimina; «Eliminar cuenta» la elimina de inmediato.

<!-- source: signing-out 053a33c01d8e -->
## Cerrar sesión

Elige «Cerrar sesión…», la última fila de la configuración, y confirma con «Cerrar sesión». Cierra la sesión de este navegador; tus otros
navegadores y aplicaciones siguen con la sesión iniciada. «Navegadores con sesión iniciada», en «Seguridad» de la configuración, enumera los navegadores con sesión iniciada
en tu cuenta y les cierra la sesión.

<!-- source: settings 4cd29f4ed978 -->
## Configuración

«Configuración», en la navegación, abre la configuración. Contiene «Perfil», que cambia cómo se ve tu cuenta y dónde
se encuentra; «Visualización y publicación»; «Notificaciones»; «Relaciones», que enumera las cuentas que bloqueas y silencias, los
servidores que silencias y las etiquetas que sigues, cada una con el botón que lo deshace; «Filtros de palabras clave», que enumera tus
filtros y los añade, cambia y elimina; «Seguridad», donde gestionas tu contraseña, passkeys, autenticación en dos
pasos, cuentas vinculadas y navegadores con sesión iniciada; «Aplicaciones», que enumera las aplicaciones a las que permitiste usar tu cuenta,
que puedes revocar, y las aplicaciones que registraste; «Invitaciones», cuando puedes crear invitaciones, que crea enlaces de invitación
y los revoca; y «Cuenta y datos», que contiene «Estado de la cuenta», tus datos y publicaciones, y la mudanza, desactivación o
eliminación de la cuenta ([Mudar tu cuenta](moving-your-account.md)).
