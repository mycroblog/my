<!-- source: moving-your-account 95d8d22a22bb -->
# Mover tu cuenta

Puedes mover tu cuenta a otra cuenta, del mismo servidor o de otro. Tus seguidores siguen a la cuenta nueva y la cuenta
antigua indica que se ha movido. Funciona igual en el otro sentido, cuando te mudas a este servidor desde otro.

<!-- source: what-moves-and-what-stays 4ea7b64d67bd -->
## Qué se mueve y qué se queda

| Se mueve | Se queda en la cuenta antigua |
| --- | --- |
| Tus seguidores | Tus publicaciones, las cuentas que sigues, tus bloqueos, silencios y marcadores |

![A la izquierda la cuenta antigua y a la derecha la cuenta nueva: un alias, el movimiento, tus seguidores pasando a la cuenta nueva y lo que se queda en la cuenta antigua. La lista de abajo dice lo mismo.](images/account-move.svg)

De arriba abajo en el diagrama:

1. **Añade un alias.** En la cuenta nueva, añade la cuenta antigua en «Alias».
2. **Mudar cuenta.** En la cuenta antigua, elige «Mudar cuenta» e indica la cuenta nueva.
3. **Qué se mueve.** Tus seguidores siguen a la cuenta nueva. La cuenta antigua muestra «Tu cuenta se ha mudado»: aún puedes iniciar sesión, leer y exportar tus datos allí.
4. **Qué se queda aquí.** Tus publicaciones, las cuentas que sigues, y tus bloqueos, silencios y marcadores se quedan en la cuenta antigua. Tus publicaciones no se mueven. Lleva el resto tú mismo: exporta tus seguidos, bloqueos, silencios y marcadores como archivos CSV con «Importar y exportar» en la cuenta antigua e impórtalos en la nueva ([llevar tus seguidos contigo](#taking-your-follows-with-you)).

<!-- source: moving-from-this-account 7836944c7996 -->
## Moverse desde esta cuenta

1. En la cuenta nueva, añade esta cuenta como alias: «Alias», en «Cuenta y datos» de la configuración, con «Añadir un alias». La página de mover muestra la dirección de esta cuenta, que debes escribir allí.
2. En esta cuenta, abre «Mudar cuenta» en «Cuenta y datos» de la configuración. Escribe el identificador de la cuenta nueva (un identificador como `@name@example.com`, o la dirección de la cuenta) y elige «Continuar».
3. El cliente muestra qué se mueve y qué se queda. Escribe tu contraseña para confirmar (o, si no tienes contraseña, confirma con una clave de acceso o iniciando sesión de nuevo en un proveedor vinculado) y elige «Mudar cuenta».

Después del movimiento, la cuenta antigua:

- Muestra «Tu cuenta se ha mudado». Aún puedes iniciar sesión, leer, exportar tus datos y eliminar la cuenta, pero no puedes publicar, impulsar, reaccionar, seguir ni votar.
- Aparece en su perfil como movida a la cuenta nueva.
- No puede volver a moverse durante 30 días.

«Cancelar la mudanza» devuelve la cuenta antigua a su estado anterior: puede volver a publicar y seguir, y se avisa a
otros servidores. Los seguidores que ya se habían movido se quedan con la cuenta nueva.

<!-- source: moving-to-this-account adb43ba4eef3 -->
## Moverse a esta cuenta

Añade la cuenta desde la que te mueves como alias en «Alias», en la configuración de esta cuenta («Añadir un alias»).
Un alias por sí solo no cambia nada; solo permite que la otra cuenta mueva sus seguidores hacia ti. Solo puedes tener
unos pocos alias, y «Quitar» elimina uno.

<!-- source: taking-your-follows-with-you ab2475388452 -->
## Llevar tus seguidos contigo

«Importar y exportar», en «Cuenta y datos» de la configuración, exporta e importa archivos CSV en el formato que usa
Mastodon. Puedes exportar «Seguimientos», «Cuentas bloqueadas», «Cuentas silenciadas», «Dominios silenciados» y
«Marcadores», e importarlos en la cuenta nueva. «Combinar» conserva lo que tienes y añade lo que indica el archivo;
«Sobrescribir» deja la lista exactamente como la indica el archivo. Las cuentas y publicaciones de otros servidores se
buscan en segundo plano, lo que puede tardar un poco.

«Archivo y eliminación» prepara un archivo ZIP con tus publicaciones, tu perfil, y tus seguidos, bloqueos, silencios y
marcadores.

<!-- source: when-an-account-you-follow-moves 81ab63afe43d -->
## Cuando se mueve una cuenta que sigues

Recibes una notificación («… se ha mudado a otra cuenta»). Cuando puedes seguir la cuenta nueva, la sigues de
inmediato o tu solicitud queda a la espera de su aprobación. No la sigues si la has bloqueado, si ella te ha bloqueado o
si has silenciado su servidor; la notificación indica cuál es el caso.
