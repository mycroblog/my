<!-- source: posting-and-choosing-who-sees-a-post 5c4adb3025f6 -->
# Publicar y elegir quién ve una publicación

<!-- source: writing-a-post 5f24a2f4b092 -->
## Escribir una publicación

Elige «Escribir una publicación» para abrir el formulario «Nueva publicación». Escribe en «Tu publicación» y elige «Publicar». El formulario muestra cuántos
caracteres te quedan. Además del texto puedes:

- Adjuntar imágenes, video o audio con «Añadir imágenes, video o audio». Describe cada archivo adjunto para las personas que no pueden verlo u oírlo; algunos servidores exigen una descripción para cada archivo adjunto, y «Exigir descripciones de los archivos multimedia», en los ajustes de «Visualización y publicación», lo exige de tus propias publicaciones. La ubicación y otros metadatos se eliminan de los archivos.
- «Añadir una encuesta», con opciones y una duración, y las opciones «Permitir elegir varias opciones» y «Ocultar los votos hasta que termine la encuesta».
- «Añadir una advertencia de contenido». Los lectores ven primero la advertencia y eligen «Mostrar contenido».
- Elegir «Ocultar las imágenes hasta que los lectores las muestren» para marcar el contenido multimedia como sensible.
- Definir el idioma de la publicación y quién puede responderle, impulsarla y citarla (consulta [Quién puede responder, impulsar y citar](#who-can-reply-boost-and-quote)).
- Responder a una publicación («Responder») o citar una («Citar»).

Una publicación que no pudo enviarse permanece en «Publicaciones en envío», donde puedes «Enviar de nuevo», «Editar» o «Descartar» la publicación.
Un servidor puede rechazar una publicación tras una revisión automática. Cuando el rechazo puede apelarse, la publicación ofrece «Apelar».

<!-- source: drafts-and-scheduled-posts 9cee5f0d17b2 -->
## Borradores y publicaciones programadas

Elige «Guardar borrador» para conservar lo que estás escribiendo en el servidor y vaciar el formulario. Un borrador guardado se abre en cualquier dispositivo
en el que inicies sesión: el enlace «Borradores y publicaciones programadas» bajo el formulario lleva a la página que los enumera, donde «Editar»
abre un borrador en el formulario de nuevo. Cuando el formulario ya contiene otra cosa, se te pregunta antes de reemplazarla.
Guardar un borrador que abriste desde esa página lo reemplaza; publicarlo lo publica y lo elimina de tus borradores. Si el
mismo borrador se guardó en otro dispositivo desde que lo abriste, no se sobrescribe ni se publica nada: guarda de nuevo para conservar tu
versión como un borrador nuevo, o publica de nuevo para publicar tu versión como una publicación nueva.

Para publicar una publicación más tarde, elige «Programar la publicación» (el reloj entre las herramientas del formulario), escoge una fecha y una hora, y
elige «Programar». La fecha y la hora están en tu zona horaria, la de tus preferencias, y el formulario la nombra. Una hora
pasada, demasiado próxima o demasiado lejana se rechaza antes de enviar nada; el servidor fija con cuánta antelación mínima y máxima
puedes programar. La publicación se publica a esa hora, o poco después.

La página «Borradores y publicaciones programadas» enumera primero tus publicaciones programadas, con la hora en que se publica cada una, y luego tus
borradores. Una publicación programada puede moverse con «Cambiar la hora» o volver a ser un borrador con «Cancelar programación», y cualquier
borrador puede eliminarse. Solo tú ves tus borradores.

Cuando una publicación programada no puede publicarse a su hora, por ejemplo porque la publicación a la que responde ya no existe, permanece
entre tus borradores con el motivo, y recibes una notificación que lleva a tus borradores, para que puedas corregirla y
programarla de nuevo.

<!-- source: who-can-see-a-post be247913f548 -->
## Quién puede ver una publicación

La herramienta «Quién puede verla» del formulario abre las opciones para esa publicación. El valor predeterminado de las publicaciones nuevas es «Quién puede verla»
en «Publicaciones nuevas», dentro de los ajustes de «Visualización y publicación».

| Opción | Quién puede verla |
| --- | --- |
| Pública | Cualquiera, aquí y en otros servidores. Aparece en las cronologías públicas y en las etiquetas. |
| Vecinos | Las personas que han iniciado sesión en este servidor y tus seguidores de otros servidores. Aparece en la cronología local de este servidor. |
| Pública discreta | Cualquiera que tenga el enlace o tu perfil, pero no aparece en las cronologías públicas ni en las etiquetas. |
| Solo seguidores | Solo tus seguidores y las personas que mencionas. |
| Directa | Solo las personas que mencionas. |

![Quién puede ver una publicación en este servidor y en otros servidores, para cada una de las cinco opciones. La lista de abajo dice lo mismo.](images/visibility.svg)

Fila por fila, primero en este servidor y después en otros servidores:

- **Pública**: cualquiera, también quienes no han iniciado sesión, y la publicación aparece en las cronologías local y federada. En otros servidores cualquiera puede leerla, y se envía a tus seguidores y a las personas que mencionas.
- **Vecinos**: las personas que han iniciado sesión en este servidor, y la publicación aparece en la cronología local. Otros servidores la reciben solo para tus seguidores allí, que la ven como una publicación de «Solo seguidores»; no se envía a nadie más.
- **Pública discreta**: cualquiera que tenga el enlace o tu perfil, pero la publicación no aparece en las cronologías públicas ni en las etiquetas. También en otros servidores, cualquiera que tenga el enlace puede leerla, y se envía a tus seguidores y a las personas que mencionas.
- **Solo seguidores**: tus seguidores y las personas que mencionas, en este servidor y en otros servidores.
- **Directa**: solo las personas que mencionas, en este servidor y en otros servidores.

En un servidor de una sola persona, donde nadie más ha iniciado sesión, «Vecinos» significa lo mismo que «Solo seguidores»: las publicaciones nuevas
no pueden tenerla, y un valor predeterminado de «Vecinos» funciona como «Solo seguidores». «Quién puede verla» y el valor predeterminado de los ajustes
la omiten, y una respuesta a una publicación para vecinos es de «Solo seguidores». Las publicaciones que ya la tienen la conservan.

Un bloqueo siempre se impone: quien te bloquea, o a quien bloqueas, no ve tu publicación, sea cual sea la opción que
hayas elegido.

<!-- source: neighbors 2581a57320f5 -->
### «Vecinos»

Una publicación para vecinos se queda en tu servidor. Las personas que no han iniciado sesión en él no pueden leerla, ni siquiera con el
enlace, y nunca aparece en la cronología federada. Otros servidores la reciben solo para tus seguidores allí, y
se la muestran a esos seguidores como una publicación solo para seguidores.

- Si mencionas a alguien de otro servidor que no te sigue, esa persona no recibe la publicación, y tus seguidores allí ven la mención como un enlace sin más. El formulario te lo advierte.
- Las cuentas de tu servidor pueden impulsar una publicación para vecinos, y el impulso se muestra solo en este servidor.
- Una respuesta a una publicación para vecinos empieza también como «Vecinos»; puedes cambiarla.

<!-- source: other-things-to-know b36265de1760 -->
### Otras cosas que conviene saber

- No puedes cambiar quién puede ver una publicación después de publicarla. Una edición conserva quién puede verla y la publicación a la que responde.
- Un moderador puede limitar el alcance de las publicaciones de una cuenta. El formulario dice entonces «Tu cuenta puede publicar como máximo con alcance ...» y una opción más amplia se publica con el alcance más amplio permitido.
- Cualquiera que pueda ver una publicación «Pública», «Vecinos» o «Pública discreta» puede impulsarla. Solo tú puedes impulsar tus propias publicaciones «Solo seguidores», y solo tus seguidores ven ese impulso. Las publicaciones «Directa» no se pueden impulsar.
- El autor de una publicación puede elegir quién puede responderle y quién puede impulsarla; las publicaciones de algunos otros servidores también llevan esa elección. Una respuesta o un impulso fuera de ella se rechaza, con un mensaje como «El autor de esta publicación no te permite responderle.». El autor, las personas que la publicación menciona y el autor de la publicación a la que responde siempre pueden responder.
- El autor también puede permitir que algunas personas respondan o impulsen solo cuando él lo apruebe. Una respuesta o impulso así espera: se te notifica («... ha pedido responder a tu publicación»), y no aparece en tu publicación hasta que lo apruebes. Una respuesta en espera de otro servidor no está en la conversación bajo tu publicación ni en su recuento de respuestas; sigue apareciendo en el perfil de su autor y en su propia página. Un impulso en espera no se cuenta. Aprobarlo lo añade a tu publicación; rechazarlo lo mantiene fuera, y se avisa al otro servidor en ambos casos.
- Una respuesta o un impulso de otro servidor que queda del todo fuera de tu elección se mantiene fuera de tu publicación de la misma manera, y no te notifica nada.
- Cuando respondes a una publicación o la impulsas y su autor aprueba a mano las respuestas o los impulsos, el tuyo espera esa aprobación. Hasta entonces, solo tú ves tu respuesta, bajo la publicación y en su propia página, marcada como «Esperando aprobación»; las demás personas no ven nada de ella, y no se envía nada a otros servidores. Tu impulso no se hace hasta que se aprueba. Se te notifica cuando el autor lo aprueba o lo rechaza («... ha aprobado tu respuesta, impulso o cita»). Eliminar una respuesta en espera, o deshacer un impulso en espera, retira la solicitud.

<!-- source: who-can-reply-boost-and-quote 5d5ece5f0cb2 -->
## Quién puede responder, impulsar y citar

«Quién puede responder, impulsar y citar», en el menú «Quién puede verla», abre las opciones para esa publicación, cada una con su valor,
por ejemplo «Quién puede impulsar: Seguidores, otras personas con tu aprobación». Para cada una de las respuestas, los impulsos y las citas, elige
quién puede hacerlo de inmediato: «Cualquiera», «Seguidores», «Personas que mencionas» o «Solo tú». Debajo de cada una, «Permitir a otras personas con tu
aprobación» deja que todas las demás lo soliciten en lugar de ser rechazadas.

- Las personas que una publicación menciona siempre pueden responderle. Una publicación «Solo seguidores» solo puedes impulsarla y citarla tú, y una publicación «Directa» nadie, elijas lo que elijas.
- Los valores predeterminados de las publicaciones nuevas, con las mismas opciones, están en «Quién puede responder, impulsar y citar», dentro de los ajustes de «Visualización y publicación».
- Las solicitudes que te esperan se enumeran en «Esperando aprobación», que abre la fila de arriba de las notificaciones. «Aprobar» añade la respuesta, el impulso o la cita a tu publicación, y la conversación bajo ella muestra de inmediato una respuesta aprobada; «Rechazar» la mantiene fuera.
- En la publicación de otra persona, «Responder» no se muestra y «Impulsar» no se puede pulsar cuando su autor no te lo permite. Cuando el autor los aprueba a mano, el formulario lo dice antes de que respondas, y después de impulsar, la publicación dice «Impulso esperando aprobación».

<!-- source: after-posting e11fd06a76e8 -->
## Después de publicar

Bajo una publicación encuentras «Responder», «Impulsar», «Citar», «Favorito», «Reaccionar» y «Más». «Más» tiene «Copiar el enlace», «Abrir la
página original» (para una publicación de otro servidor), «Compartir» y «Marcador»; tus marcadores están en la página «Marcadores»,
y solo tú los ves. En tu propia publicación también tiene «Editar» (no para una publicación con encuesta), «Eliminar y reescribir…» y
«Eliminar…». Ambas eliminaciones preguntan antes. «Eliminar…» quita la publicación para todos, junto con sus impulsos y favoritos;
«Eliminar y reescribir…» hace lo mismo y abre el formulario con su texto, imágenes y encuesta como una publicación nueva.

Una publicación editada muestra «· editada» después de su hora; púlsala para abrir «Historial de ediciones», con la versión más reciente primero y los
cambios marcados.

Una encuesta admite un voto, o varios cuando su autor lo permite: elige y luego «Votar». Los resultados se muestran cuando has votado o
la encuesta ha terminado, y a su autor en cualquier momento. «Reaccionar» abre un selector de emoji (búsqueda, categorías, tono de piel y los emoji
personalizados de este servidor); cada persona da a una publicación un favorito o una reacción. En la página propia de la publicación, un emoji enumera
quién reaccionó con él. El formulario también tiene un botón «Añadir emoji».

Una publicación en un idioma distinto del tuyo tiene «Traducir» cuando el servidor tiene un servicio de traducción o tu navegador puede
traducir; «Mostrar el original» vuelve atrás. Tras una advertencia de contenido, la línea queda en la banda de la advertencia. Si el servidor envía
el texto de las publicaciones a un servicio de traducción, «Quién puede verla» te dice para qué opciones.

Las imágenes, GIF, video y audio se muestran en la publicación. Pulsa una imagen para abrir el visor: desliza o usa las flechas, amplía y
lee su descripción. El contenido multimedia marcado como sensible permanece oculto bajo una línea como «Imagen sensible», hasta que eliges «Mostrar». Un enlace en una
publicación se muestra como una tarjeta.

<!-- source: writing-an-article 3bc498986329 -->
## Escribir un artículo

«Escribir un artículo», bajo el formulario de publicación nueva, abre el editor de artículos, una página propia. Dale al artículo un título, un
resumen si quieres (si lo dejas vacío, se genera con el comienzo del cuerpo), una imagen de portada con su descripción y el
cuerpo, escrito en Markdown: `#` inicia un encabezado, `**text**` es negrita, y «Negrita» y «Enlace» sobre el cuerpo hacen lo mismo con
el texto que seleccionas (Ctrl+B y Ctrl+K, o Cmd en un Mac). Suelta o pega imágenes en el cuerpo, o elígelas con «Añadir
imágenes»: cada una se sube y se coloca donde está el cursor como `![](media:…)`; escribe su descripción entre los corchetes.
Solo se muestran en un artículo las imágenes que subes; una imagen de otra dirección se convierte en un enlace con su descripción.

- Las etiquetas van en su propio campo, separadas por espacios; el artículo aparece listado bajo ellas, y un `#` en el cuerpo sigue siendo texto. Las menciones en el cuerpo enlazan a las personas pero no les envían notificación.
- «Vista previa» muestra el artículo tal como lo verán los lectores, generado por el servidor exactamente como se publicará, con el tamaño del cuerpo y el número de sus imágenes frente a los límites de este servidor.
- «Marcar como sensible» muestra primero a los lectores el título y el resumen y pliega la portada y el cuerpo. Un artículo no puede ser «Directa».
- Mientras escribes, el artículo se guarda como borrador cada vez que haces una pausa («Borrador guardado»), así que se abre en cualquier dispositivo desde «Borradores y publicaciones programadas», donde «Editar» lo devuelve al editor. Si el borrador se cambió entretanto en otro dispositivo, no se sobrescribe nada: tu texto permanece en el editor, y «Guardar como borrador nuevo» lo conserva como otro borrador.
- «Publicar» publica el artículo ahora y abre su página; «Programar» lo publica a la hora que elijas, como una publicación.
- Para cambiar un artículo que publicaste, elige «Editar» en su menú «Más acciones». El editor se abre con lo que escribiste; «Guardar los cambios» lo reemplaza, y la versión anterior permanece en su «Historial de ediciones». Quién puede ver un artículo no cambia.

Un servidor puede permitir que solo algunas personas escriban artículos nuevos; entonces «Escribir un artículo» no se muestra a las demás, que aún pueden
editar los artículos que publicaron.

<!-- source: reading-articles f2811fffd462 -->
## Leer artículos

Un artículo es una publicación larga con un título, un resumen y, a menudo, una imagen de portada, escrita en este servidor o en otro.
Las cronologías y los perfiles muestran un artículo como una tarjeta con su portada, la palabra «Artículo», su título y su resumen; selecciona
el título para leer el artículo completo en su propia página, que el título encabeza. Las respuestas, los impulsos, las citas, los favoritos y las
reacciones funcionan como en cualquier publicación. Los artículos no tienen «Traducir».

Un artículo que su autor marcó como sensible muestra su título y su resumen, y oculta el resto: en la tarjeta, «Mostrar multimedia
sensible» muestra su portada, y en su página «Mostrar el artículo» muestra su portada y su texto. La pestaña «Artículos» de un perfil
enumera solo los artículos de esa persona, los más recientes primero; para alguien de otro servidor, solo los artículos que este servidor ha
recibido.

<!-- source: direct-messages e262b7a83b69 -->
## Mensajes directos

«Mensajes directos», en la navegación, reúne tus publicaciones «Directa» y las que recibes, una fila por
conversación, con la que tiene la publicación más reciente primero. Cada fila muestra quién está en la conversación, cuándo se escribió su publicación
más reciente y el comienzo de esa publicación. El número junto a «Mensajes directos» es cuántas conversaciones tienen una publicación que
aún no has leído; cada una tiene un punto antes de sus nombres.

Pulsa el comienzo de una publicación para abrir el hilo de la conversación; esto marca la conversación como leída en todos tus dispositivos. Una
respuesta a una publicación «Directa» también es siempre «Directa», así que no llega a nadie a quien no llegara la conversación. En
los teléfonos, «Mensajes directos» está en «Menú».

<!-- source: deleting-old-posts-automatically ab68788c9a84 -->
## Eliminar publicaciones antiguas automáticamente

«Eliminar publicaciones antiguas automáticamente», en «Cuenta y datos» de la configuración, elimina tus publicaciones cuando son más antiguas que
la edad que eliges, como si las hubieras eliminado tú: desaparecen aquí y de los servidores a los que se enviaron, y no se pueden restaurar. Las publicaciones que conservas
(fijadas, con marcador o favorito tuyo, con contenido multimedia o encuesta, artículos, por quién puede verlas, o con muchos impulsos o
favoritos) permanecen. Tus impulsos de publicaciones de otras personas nunca se eliminan. El servidor comprueba cada hora y elimina como
máximo 100 de tus publicaciones cada vez, así que muchas publicaciones antiguas tardan varias horas en desaparecer.

Una cuenta nueva empieza con los ajustes que el servidor eligió para las cuentas nuevas; desde entonces son tuyos, y puedes
cambiarlos como cualquier otro ajuste.

Un servidor también puede conservar las publicaciones durante un máximo de días. La página «Acerca de» del servidor lo dice antes de que te registres. Con ese
máximo, la eliminación automática siempre está activada para ti, la edad que eliges no puede ser mayor que el máximo, y solo se pueden conservar los
tipos de publicaciones que el servidor permite (por defecto solo las publicaciones fijadas, así que tus artículos también se eliminan
salvo que el servidor permita conservarlos). Algunos servidores permiten que algunas personas conserven sus publicaciones más tiempo, por ejemplo como ventaja de un plan de colaborador.

Cuando un máximo empieza a aplicarse a ti (el servidor lo introduce o lo reduce, o pierdes el derecho a conservar publicaciones más tiempo),
recibes una notificación, que lleva a esa página, y un correo, salvo que lo hayas desactivado, que dice desde cuándo se aplica.
Hasta entonces tus ajustes siguen vigentes. La página muestra la fecha y, en «Publicaciones que se eliminarían», cuántas se eliminan entonces.
