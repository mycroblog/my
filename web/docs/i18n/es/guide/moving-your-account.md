<!-- source: moving-your-account 95d8d22a22bb -->
# Mover tu cuenta

Puedes mover tu cuenta a otra cuenta, del mismo servidor o de otro. Tus seguidores siguen a la cuenta nueva y la cuenta
antigua indica que se ha movido. Funciona igual en el otro sentido, cuando te mudas a este servidor desde otro.

<!-- source: what-moves-and-what-stays a45d57421940 -->
## Qué se mueve y qué se queda

| Se mueve | Se queda en la cuenta antigua |
| --- | --- |
| Tus seguidores | Tus publicaciones, las cuentas que sigues, tus bloqueos, silenciados y marcadores |

![La cuenta antigua a la izquierda y la cuenta nueva a la derecha: un alias, la mudanza, tus seguidores pasando a la cuenta nueva y lo que se queda en la cuenta antigua. La lista de abajo dice lo mismo.](images/account-move.svg)

De arriba abajo en el diagrama:

1. **Añade un alias.** En la cuenta nueva, añade la cuenta antigua en «Alias».
2. **Mudar la cuenta.** En la cuenta antigua, elige «Mudar la cuenta» e indica la cuenta nueva.
3. **Qué se mueve.** Tus seguidores siguen la cuenta nueva. La cuenta antigua dice «Esta cuenta se ha mudado a …»: allí aún puedes iniciar sesión, leer y exportar tus datos.
4. **Qué se queda aquí.** Tus publicaciones, las cuentas y etiquetas que sigues, y tus bloqueos, silenciados y marcadores se quedan en la cuenta antigua. Tus publicaciones no se mueven. Lleva el resto como archivos CSV, con «Exportar» en la cuenta antigua e «Importar» en la nueva ([llevarte lo que sigues](#taking-your-follows-with-you)).

<!-- source: moving-from-this-account 8470ed8a1f13 -->
## Mudarte desde esta cuenta

1. En la cuenta nueva, añade esta cuenta como alias: «Alias», en «Cuenta y datos» de la configuración, en «Añadir un alias». «Mudar la cuenta» muestra el identificador de esta cuenta para escribirlo allí.
2. En esta cuenta, abre «Mudar la cuenta» en «Cuenta y datos» de la configuración, que dice qué se mueve y qué se queda. En «Cuenta a la que te mudas», escribe el identificador de la cuenta nueva (como `@name@example.com`) o la dirección de su perfil, y elige «Mudar la cuenta».
3. Confirma que eres tú con tu contraseña, una passkey o un proveedor vinculado. Si el servidor rechaza la mudanza, dice por qué.

Después de la mudanza, la cuenta antigua:

- Dice adónde se mudó, en esa página y en su perfil. Aún puedes iniciar sesión, leer, exportar tus datos y eliminar la cuenta, pero no publicar, impulsar, reaccionar, seguir ni votar.
- No puede mudarse de nuevo durante 30 días.

«Cancelar la mudanza», en la misma página, devuelve la cuenta antigua: puede volver a publicar y a seguir, y se avisa a los otros
servidores. Los seguidores que ya se mudaron se quedan con la cuenta nueva.

<!-- source: deactivating-or-deleting-this-account 599007a2d409 -->
## Desactivar o eliminar esta cuenta

En «Cuenta y datos» de la configuración, «Desactivar la cuenta» oculta la cuenta y cierra tu sesión en todas partes: inicia sesión
de nuevo antes de la fecha que indica la página para recuperarla, o se eliminará entonces. «Eliminar la cuenta» la elimina de inmediato, para
siempre. «Exporta» antes tus datos.

<!-- source: moving-to-this-account 4b769e76357c -->
## Mudarte a esta cuenta

Añade la cuenta desde la que te mudas como alias en «Alias», en la configuración de esta cuenta («Añadir un alias»). Un alias por sí solo
no cambia nada; solo permite que la otra cuenta mueva sus seguidores hacia ti. Puedes tener 5 alias, y
«Quitar» elimina uno.

<!-- source: taking-your-follows-with-you 09ca5e285436 -->
## Llevarte lo que sigues

«Exportar» e «Importar», en «Cuenta y datos» de la configuración, exportan e importan archivos CSV en el formato que usa Mastodon.
Puedes exportar «Seguidos», «Etiquetas seguidas», «Cuentas bloqueadas», «Cuentas silenciadas», «Servidores silenciados» y «Marcadores», y
importarlos en la cuenta nueva. Mastodon no tiene un archivo de etiquetas seguidas, así que solo un servidor que ejecuta el mismo software que
este importa ese archivo. «Combinar» conserva lo que tienes y añade lo que enumera el archivo; «Reemplazar» deja la lista exactamente como la
del archivo. «Vista previa» muestra lo que haría un archivo antes de elegir «Importar»; la importación se ejecuta entonces en segundo plano.

«Solicitar un archivo», en «Exportar», prepara un archivo ZIP con tus publicaciones, tu perfil, tus filtros de palabras clave, tus notas privadas y tus
seguidos, etiquetas seguidas, bloqueos, silenciados, servidores silenciados y marcadores, para descargarlo allí cuando esté listo.

<!-- source: when-an-account-you-follow-moves 81ab63afe43d -->
## Cuando se mueve una cuenta que sigues

Recibes una notificación («… se ha mudado a otra cuenta»). Cuando puedes seguir la cuenta nueva, la sigues de
inmediato o tu solicitud queda a la espera de su aprobación. No la sigues si la has bloqueado, si ella te ha bloqueado o
si has silenciado su servidor; la notificación indica cuál es el caso.
