<!-- source: signing-up-and-signing-in e881fcb0feaa -->
# Registrarse e iniciar sesión

Cómo crear una cuenta en un servidor de mycroblog, un servidor que ejecuta `my`, e iniciar sesión en él. Cada servidor lo
gestionan sus propios operadores, así que lo que ofrece un servidor (quién puede registrarse, qué cronologías pueden leer
los visitantes, si el chat de voz está activado) depende de ellos. La página «Acerca de» del servidor y los documentos
que enlaza al registrarte te indican sus reglas; esta guía solo describe cómo se comporta `my`.

<!-- source: creating-an-account 965073ac4cf5 -->
## Crear una cuenta

Elige «Registrarse» en la navegación (o «Crear una cuenta» en la página de inicio de sesión). El formulario pide:

- **Nombre de usuario**: letras (a-z), dígitos y guiones bajos. Tu identificador es `@username@this-server`, y no puedes cambiar el nombre de usuario después.
- **Dirección de correo electrónico**: con ella inicias sesión.
- **Contraseña**: el formulario muestra la longitud permitida debajo del campo.
- **Código de invitación**, cuando el servidor lo pide o lo acepta.
- **¿Por qué quieres unirte? (opcional)**, cuando el servidor revisa las cuentas nuevas.
- Una casilla para confirmar que has leído los términos y las políticas del servidor, cuando el servidor publica alguno.

Lo que ves de todo esto depende de cómo acepta el servidor las cuentas nuevas:

| El servidor | Qué haces |
| --- | --- |
| Está abierto | Te registras. El código de invitación es opcional. |
| Revisa las cuentas nuevas | Te registras e indicas por qué quieres unirte. Un moderador aprueba la cuenta antes de que puedas usarla. Un código de invitación te permite usar la cuenta de inmediato. |
| Acepta solo invitaciones | Escribes el código de invitación que te dieron, o abres el enlace de invitación, que lo rellena. |
| Está cerrado | La página indica que el servidor no acepta cuentas nuevas. |
| Registra solo mediante proveedores de inicio de sesión | Te registras con un proveedor que ofrece el servidor (por ejemplo «Iniciar sesión con GitHub») en lugar de una contraseña. |

Si el servidor envía correo electrónico, te envía un enlace para confirmar tu dirección de correo electrónico. Ábrelo y
elige «Confirmar correo electrónico». Hasta que la cuenta se apruebe y la dirección se confirme, puedes
iniciar sesión, pero solo ves aquello que estás esperando («Tu cuenta aún no está lista»).

Tras el registro, una página de bienvenida te permite elegir un avatar y un nombre para mostrar («Continuar» los guarda,
«Más tarde» omite este paso). Tu cronología de inicio muestra entonces una lista «Primeros pasos»: «Configura tu perfil»,
«Sigue a alguien» y «Escribe tu primera publicación». «Ocultar estos pasos» la cierra en ese navegador. «Cuentas
sugeridas» lista personas a las que puedes seguir, salvo en un servidor de una sola persona, donde «Buscar personas» abre
la búsqueda.

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

<!-- source: signing-out fb81956ae999 -->
## Cerrar sesión

Elige «Cerrar sesión» en la sección «Cuenta» de la aplicación. La página «Navegadores con sesión iniciada» de la
configuración lista los navegadores que tienen la sesión iniciada en tu cuenta y la cierra en ellos.

<!-- source: settings 35cf43b05962 -->
## Configuración

La configuración se agrupa en «Perfil y comunidad», «Visualización y notificaciones», «Relaciones», «Seguridad»,
«Aplicaciones» y «Cuenta y datos». En «Seguridad» gestionas tu contraseña, la autenticación en dos pasos, las llaves de
acceso, las cuentas vinculadas y los navegadores con sesión iniciada.
