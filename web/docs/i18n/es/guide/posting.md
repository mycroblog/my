<!-- source: posting-and-choosing-who-sees-a-post 5c4adb3025f6 -->
# Publicar y elegir quién ve una publicación

<!-- source: writing-a-post b3b7ce030983 -->
## Escribir una publicación

Elige «Escribir una publicación» para abrir el formulario «Nueva publicación». Escribe en «Tu publicación» y elige «Publicar». El formulario muestra cuántos
caracteres te quedan. Además del texto puedes:

- Adjuntar imágenes, video o audio con «Añadir imágenes, video o audio». Describe cada archivo adjunto para las personas que no pueden verlo u oírlo; algunos servidores exigen una descripción para cada archivo adjunto. La ubicación y otros metadatos se eliminan de los archivos.
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

<!-- source: who-can-see-a-post 267cf73d0612 -->
## Quién puede ver una publicación

El botón «Visibilidad» del formulario abre «¿Quién puede ver esta publicación?» para esa publicación. El valor
predeterminado de las publicaciones nuevas es la opción «Quién ve tus publicaciones de forma predeterminada», en
«Visualización, publicación y cronologías» de la configuración.

| Opción | Quién puede verla |
| --- | --- |
| Pública | Cualquiera, aquí y en otros servidores. Aparece en las cronologías públicas. |
| Este servidor | Las personas que han iniciado sesión en este servidor y tus seguidores de otros servidores. Aparece en la cronología local de este servidor. |
| No listada | Cualquiera que tenga el enlace, pero no aparece en las cronologías públicas. |
| Seguidores | Solo tus seguidores y las personas que mencionas. |
| Personas mencionadas | Solo las personas que mencionas. |

![Quién puede ver una publicación en este servidor y en otros servidores, para cada una de las cinco visibilidades. La lista de abajo dice lo mismo.](images/visibility.svg)

Fila por fila, primero en este servidor y después en otros servidores:

- **Pública**: cualquiera, también quienes no han iniciado sesión, y la publicación aparece en las cronologías local y federada. En otros servidores cualquiera puede leerla, y se envía a tus seguidores y a las personas que mencionas.
- **Este servidor**: las personas que han iniciado sesión en este servidor, y la publicación aparece en la cronología local. Otros servidores la reciben solo para tus seguidores allí, que la ven como una publicación de «Seguidores»; no se envía a nadie más.
- **No listada**: cualquiera que tenga el enlace, pero la publicación no aparece en las cronologías públicas. También en otros servidores, cualquiera que tenga el enlace puede leerla, y se envía a tus seguidores y a las personas que mencionas.
- **Seguidores**: tus seguidores y las personas que mencionas, en este servidor y en otros servidores.
- **Personas mencionadas**: solo las personas que mencionas, en este servidor y en otros servidores.

En un servidor de una sola persona, donde nadie más ha iniciado sesión, «Este servidor» significa lo mismo que
«Seguidores»: las publicaciones nuevas no pueden tenerla, y un valor predeterminado de «Este servidor» funciona como
«Seguidores». El menú de visibilidad y el valor predeterminado de «Visualización, publicación y cronologías» la omiten, y una
respuesta a una publicación de «Este servidor» es para «Seguidores». Las publicaciones que ya la tienen la conservan.

Un bloqueo siempre se impone: quien te bloquea, o a quien bloqueas, no ve tu publicación, sea cual sea la opción que
hayas elegido.

<!-- source: this-server d39e8d3e2df6 -->
### «Este servidor»

Una publicación con la visibilidad «Este servidor» se queda en tu servidor. Las personas que no han iniciado sesión en
él no pueden leerla, ni siquiera con el enlace, y nunca aparece en la cronología federada. Otros servidores la reciben
solo para tus seguidores allí, y se la muestran a esos seguidores como una publicación solo para seguidores.

- Si mencionas a alguien de otro servidor que no te sigue, no recibe la publicación, y tus seguidores de allí ven la mención como un simple enlace. El formulario te avisa de ello.
- Las cuentas de tu servidor pueden impulsar una publicación de «Este servidor», y el impulso solo se muestra en este servidor.
- Una respuesta a una publicación de «Este servidor» empieza también como «Este servidor»; puedes cambiarlo.

<!-- source: other-things-to-know d1f569ac2d1d -->
### Otras cosas que conviene saber

- No puedes cambiar la visibilidad de una publicación después de publicarla. Cuando editas una publicación, la visibilidad y la publicación a la que responde se mantienen.
- Un moderador puede limitar el alcance con el que publica una cuenta. El formulario dice entonces «Ahora tu cuenta puede publicar como máximo con la visibilidad …» y una opción más amplia se publica con el nivel más amplio permitido.
- Cualquiera que pueda ver una publicación pública, de «Este servidor» o no listada puede impulsarla. Solo tú puedes impulsar tus propias publicaciones de «Seguidores», y solo tus seguidores ven ese impulso. Las publicaciones de «Personas mencionadas» no se pueden impulsar.
- El autor de una publicación puede elegir quién puede responderle y quién puede impulsarla; las publicaciones de algunos otros servidores también llevan esa opción. Una respuesta o un impulso fuera de ella se rechaza, con un mensaje como «El autor de esta publicación no te permite responderle.». El autor, las personas que menciona la publicación y el autor de la publicación a la que responde siempre pueden responder.
- El autor también puede permitir que algunas personas respondan o impulsen solo cuando él lo apruebe. Esa respuesta o ese impulso espera: recibes una notificación («… ha pedido responder a tu publicación») y permanece fuera de tu publicación hasta que lo apruebes. Una respuesta en espera de otro servidor no aparece en la conversación bajo tu publicación ni en su recuento de respuestas; sigue apareciendo en el perfil de su autor y en su propia página. Un impulso en espera no se cuenta. Aprobarlo lo añade a tu publicación; rechazarlo lo mantiene fuera, y se avisa al otro servidor en ambos casos.
- Una respuesta o un impulso de otro servidor que queda del todo fuera de tu opción se mantiene fuera de tu publicación del mismo modo y no te notifica nada.
- Cuando respondes a una publicación, o la impulsas, cuyo autor aprueba a mano las respuestas o los impulsos, la tuya espera esa aprobación. Hasta entonces, solo tú ves tu respuesta, bajo la publicación y en su propia página, marcada como en espera; los demás no ven nada de ella, y no se envía nada a otros servidores. Tu impulso no se realiza hasta que se aprueba. Recibes una notificación cuando el autor lo aprueba o lo rechaza («… ha aprobado tu respuesta, impulso o cita»). Eliminar una respuesta en espera, o deshacer un impulso en espera, retira la solicitud.
- Puedes fijar hasta cinco de tus publicaciones en tu perfil con «Fijar en el perfil». Una publicación de «Personas mencionadas» no se puede fijar.

<!-- source: who-can-reply-boost-and-quote c9ea034f7416 -->
## Quién puede responder, impulsar y citar

El botón «Quién puede responder, impulsar y citar» entre las herramientas del formulario abre las opciones de esa publicación. Para cada una de
las respuestas, los impulsos y las citas, elige quién puede hacerlo de una vez: «Cualquiera», «Seguidores y personas que menciones», «Personas que
menciones» o «Solo tú». Debajo de cada una, «Los demás pueden responder con tu aprobación» (y lo mismo para impulsos y citas) permite que
todos los demás lo pidan en lugar de ser rechazados. Cuando una opción admite a menos que cualquiera, el formulario muestra el resultado sobre sus
herramientas, por ejemplo «Respuestas: Cualquiera. Impulsos: Solo tú. Citas: Cualquiera.», antes de que publiques.

- Las personas que menciona una publicación siempre pueden responderle. Una publicación de «Seguidores» solo puedes impulsarla y citarla tú, y una de «Personas mencionadas» nadie, elijas lo que elijas.
- Los valores predeterminados de las publicaciones nuevas están en «Quién puede responder, impulsar y citar tus publicaciones nuevas» dentro de «Visualización, publicación y cronologías» en los ajustes.
- Para cambiarlos en una publicación que ya enviaste, elige «Cambiar quién puede responder, impulsar y citar» en su menú «Más acciones». Cada opción se guarda al instante. Las respuestas, los impulsos y las citas que aprobaste antes se mantienen, y las solicitudes que admite la nueva opción se aprueban.
- Las solicitudes que esperan tu respuesta están en «Solicitudes de aprobación» en los ajustes, junto a «Solicitudes de seguimiento»; la notificación de cada una enlaza allí con «Revisar las solicitudes». «Aprobar» añade la respuesta, el impulso o la cita a tu publicación, y la conversación bajo ella muestra de inmediato una respuesta aprobada; «Rechazar» la mantiene fuera.
- En la publicación de otra persona, «Responder» no se muestra y «Impulsar» no se puede pulsar cuando su autor no te lo permite. Cuando el autor los aprueba a mano, el formulario lo indica antes de que respondas, y después de impulsar, la publicación dice «Tu impulso espera la aprobación del autor.»

<!-- source: after-posting dc553fa0ead9 -->
## Después de publicar

Bajo una publicación encuentras «Responder», «Impulsar», «Citar», «Favorito» y «Marcador». El menú «Más acciones» tiene «Copiar enlace»
y, para tus propias publicaciones, «Editar», «Fijar en el perfil» y «Eliminar». Una publicación que se editó muestra «Editada», y sus versiones
anteriores están en su página «Historial de ediciones». «Eliminar» borra la publicación para todos, junto con sus impulsos y favoritos.

Para la publicación de otra persona, el mismo menú ofrece seguir, silenciar, bloquear o denunciar a su autor, y «Mensaje directo».
Tus marcadores se enumeran en la página «Marcadores»; solo tú los ves.

Cuando el servidor ofrece traducción, o tu navegador puede traducir, las publicaciones en otros idiomas tienen «Traducir» entre
sus acciones, y en el menú «Más acciones», que lo contiene cuando la publicación es demasiado estrecha para un botón más. Mientras una
publicación muestra su traducción, una línea bajo el texto indica de qué idioma se tradujo y quién lo hizo, con «Mostrar
original» para volver. Si el servidor envía el texto de las publicaciones a un servicio de traducción, el selector de visibilidad te dice
para qué opciones.

<!-- source: writing-an-article 0dfa0a80bb8e -->
## Escribir un artículo

«Escribir un artículo» bajo el formulario de nueva publicación abre el editor de artículos, una página propia. Dale al artículo un título, un
resumen si quieres (si lo dejas vacío, se genera con el comienzo del cuerpo), una imagen de portada con su descripción y el
cuerpo, escrito en Markdown: `#` inicia un encabezado, `**text**` es negrita, y «Negrita» y «Enlace» sobre el cuerpo hacen lo mismo con
el texto que seleccionas (Ctrl+B y Ctrl+K, o Cmd en un Mac). Suelta o pega imágenes en el cuerpo, o elígelas con «Añadir
imágenes»: cada una se sube y se coloca donde está el cursor como `![](media:…)`; escribe su descripción entre los corchetes.
Solo las imágenes que subes se muestran en un artículo; una imagen de otra dirección se convierte en un enlace con su descripción.

- Las etiquetas van en su propio campo, separadas por espacios; el artículo aparece listado bajo ellas, y un `#` en el cuerpo sigue siendo texto. Las menciones en el cuerpo enlazan a las personas, pero no las notifican.
- «Vista previa» muestra el artículo tal como lo verán los lectores, renderizado por el servidor exactamente como se publicará, con el tamaño del cuerpo y el número de sus imágenes frente a los límites de este servidor.
- «Marcar como sensible» muestra primero a los lectores el título y el resumen y pliega la portada y el cuerpo. Un artículo no puede ser solo para «Personas mencionadas».
- Mientras escribes, el artículo se guarda como borrador cada vez que haces una pausa («Borrador guardado»), así que se abre en cualquier dispositivo desde «Borradores y publicaciones programadas», donde «Editar» lo devuelve al editor. Si el borrador se cambió en otro dispositivo entretanto, no se sobrescribe nada: tu texto permanece en el editor, y «Guardar como borrador nuevo» lo conserva como otro borrador.
- «Publicar» publica el artículo ahora y abre su página; «Programar» lo publica a la hora que elijas, como en una publicación.
- Para cambiar un artículo que publicaste, elige «Editar» en su menú «Más acciones». El editor se abre con lo que escribiste; «Guardar cambios» lo reemplaza, y la versión anterior permanece en su «Historial de ediciones». La visibilidad de un artículo no cambia.

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

<!-- source: direct-messages 59062702f4be -->
## Mensajes directos

«Mensajes directos» en la navegación reúne tus publicaciones de «Personas mencionadas» y las que recibes, una fila por
conversación, con la que tiene la publicación más reciente primero. Cada fila muestra quién participa en la conversación, cuándo se escribió su publicación más reciente y el comienzo de
esa publicación. El número junto a «Mensajes directos» es cuántas conversaciones tienen una publicación que aún no has
leído; cada una de ellas tiene un punto antes de sus nombres.

Elige el comienzo de una publicación para abrir el hilo de la conversación; esto marca la conversación como leída en todos tus dispositivos. Una
respuesta a una publicación de «Personas mencionadas» es siempre también de «Personas mencionadas», así que no llega a nadie a quien no llegara la conversación. En
los teléfonos, «Mensajes directos» está en «Menú», a menos que lo muevas a la barra de pestañas en «Visualización, publicación y cronologías» dentro de
los ajustes.

<!-- source: deleting-old-posts-automatically e581c8d27380 -->
## Eliminar publicaciones antiguas automáticamente

«Eliminar publicaciones antiguas» en los ajustes elimina tus publicaciones cuando son más antiguas que la edad que elijas, como si las hubieras
eliminado tú mismo: desaparecen aquí y de los servidores a los que se enviaron, y no se pueden restaurar. Las publicaciones que conservas
(fijadas, marcadas o favoritas tuyas, con contenido multimedia o una encuesta, artículos, de ciertas visibilidades, o con muchos impulsos o
favoritos) permanecen. Tus impulsos de publicaciones de otras personas nunca se eliminan. El servidor comprueba cada hora y elimina como
máximo 100 de tus publicaciones cada vez, así que muchas publicaciones antiguas tardan varias horas en desaparecer.

Una cuenta nueva empieza con los ajustes que el servidor eligió para las cuentas nuevas; desde entonces son tuyos y puedes
cambiarlos como cualquier otro ajuste.

Un servidor también puede conservar las publicaciones como máximo cierto número de días. La página «Acerca de» del servidor lo indica antes de que te registres. Bajo ese
máximo, la eliminación automática está siempre activada para ti, la edad que elijas no puede ser mayor que el máximo, y solo se pueden conservar
los tipos de publicaciones que el servidor permite (de forma predeterminada solo las fijadas, así que tus artículos también se eliminan
a menos que el servidor permita conservarlos). Algunos servidores permiten que ciertas personas conserven sus publicaciones más tiempo, por ejemplo como ventaja de un plan de mecenas.

Cuando un máximo empieza a aplicarse a ti (el servidor lo introduce o lo reduce, o pierdes el derecho a conservar publicaciones más tiempo),
recibes una notificación, y un correo salvo que lo hayas desactivado, que indica desde cuándo se aplica. Hasta entonces tus propios
ajustes siguen vigentes. La página de ajustes muestra la fecha y, con su vista previa, cuáles de tus publicaciones se eliminarán entonces.
