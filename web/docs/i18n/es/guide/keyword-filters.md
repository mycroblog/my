<!-- source: keyword-filters 466c3876a3d1 -->
# Filtros de palabras clave

Un filtro de palabras clave pliega u oculta las publicaciones que contienen alguna de sus palabras, sin importar quién las escribió: spoilers de una serie que
aún no has visto, un tema del que necesitas descansar. El servidor busca las coincidencias en las publicaciones, así que todas las aplicaciones que uses muestran el mismo
resultado. Solo tú ves tus filtros, y nadie recibe aviso de ellos.

<!-- source: adding-a-filter 455f52213244 -->
## Añadir un filtro

Abre «Filtros de palabras clave», en «Personas y filtros» de la configuración, y elige «Añadir un filtro»:

1. **Nombre:** lo que muestra en su lugar una publicación plegada («Coincide con el filtro “Final de temporada”»), hasta 50 caracteres.
2. **Publicaciones que coinciden:** «Plegar tras el nombre del filtro» muestra solo ese nombre en lugar de la publicación, con «Mostrar» para leerla.
   «Ocultar por completo» deja la publicación fuera, y también su notificación.
3. **Termina:** «Nunca», después de una de las duraciones ofrecidas, de 30 minutos a una semana, o «En una fecha y hora».
4. **Palabras:** una o más palabras o frases. «Añadir una palabra» añade otra, hasta 100 en un filtro. «Palabra completa» junto a una palabra
   hace que coincida solo como palabra completa (consulta [Cómo coinciden las palabras](#how-words-match)).
5. **Dónde se aplica:** «Inicio y listas», «Notificaciones», «Cronologías públicas» (las cronologías local, federada y de
   etiquetas), «Hilos» y «Perfiles». Un filtro nuevo se aplica en todas partes; desmarca los lugares que no quieras.

Elige «Añadir». Al elegir un filtro de la lista se abre para cambiar cualquiera de estos datos, y «Eliminar…» allí lo borra con sus palabras tras
pedir confirmación. Un filtro se aplica al instante a todas las publicaciones, también a las antiguas; las cronologías abiertas en pantalla se cargan de nuevo para mostrar el cambio.

La búsqueda, tus marcadores y favoritos, y tus mensajes directos nunca se filtran. Tampoco tus propias publicaciones ni
impulsos, ni las publicaciones que citan.

<!-- source: what-a-filtered-post-looks-like 52e0483be326 -->
## Qué aspecto tiene una publicación filtrada

Una publicación que coincide con un filtro de plegado muestra solo «Coincide con el filtro» con los nombres de los filtros que coincidieron, y «Mostrar».
Su autor, su texto y sus imágenes quedan fuera de la vista, y un lector de pantalla lee solo esa línea, hasta que eliges «Mostrar»;
«Ocultar» la pliega de nuevo, igual que volver a la página.

Una notificación sobre una publicación así muestra la misma línea en lugar del texto de la publicación. Cuando solo coincide una publicación citada, solo
se pliega la cita, incluso con «Ocultar», con un «Mostrar» propio. Un impulso se filtra por la publicación que impulsa.

Una publicación que abres tú mismo, desde un enlace o una notificación, se pliega en lugar de ocultarse, para que el enlace siga mostrando
algo.

<!-- source: when-a-filter-ends d70431d92b82 -->
## Cuándo termina un filtro

Un filtro con fecha de fin deja de aplicarse en ese momento. Permanece en tu lista, marcado como «Caducado», hasta que lo elimines. Para usarlo
de nuevo, ábrelo y elige un nuevo final, o «Nunca». Mientras editas un filtro que tiene fin, «Sin cambios» deja el final
como está.

<!-- source: how-words-match aeca692b00c3 -->
## Cómo coinciden las palabras

Una palabra coincide con el texto de una publicación, su advertencia de contenido (y el texto que oculta), las opciones de su encuesta y las
descripciones de sus imágenes. Los enlaces coinciden por el texto que muestran, no por la dirección que hay detrás. Una frase con espacios
coincide con esas palabras en ese orden; cualquier serie de espacios o saltos de línea en una publicación equivale a un espacio.

Las mayúsculas y las minúsculas, las formas de ancho medio y de ancho completo, y el katakana y el hiragana coinciden entre sí:

| Palabra | También coincide con |
| --- | --- |
| `Finale` | `finale`, `FINALE`, `ｆｉｎａｌｅ` |
| `ネタバレ` | `ねたばれ`, `ﾈﾀﾊﾞﾚ` |
| `season finale` | `Season  Finale`, y las dos palabras separadas por un salto de línea |

Una palabra coincide en cualquier lugar donde aparece, incluso dentro de una palabra más larga. «Palabra completa» hace que coincida solo donde ninguna letra, dígito
o `_` la toca por ninguno de los dos lados:

| Palabra | Sin «Palabra completa» | Con «Palabra completa» |
| --- | --- | --- |
| `cat` | `cat`, `cats`, `concatenate` | `cat`, `cat!`, `#cat`, pero no `cats` |
| `vote` | `vote`, `voters`, `devoted` | `vote`, `Vote.`, pero no `voters` |

Algunas escrituras escriben las palabras sin espacios entre ellas: chino, japonés (kanji, hiragana y katakana), tailandés, lao,
jemer y birmano, entre otras. Un filtro no puede saber dónde termina una palabra en ellas, así que sus letras nunca cuentan como
contiguas, y «Palabra completa» funciona como si estuviera desactivada: `猫` también coincide con `子猫` y `猫舌`, y `選挙` también coincide con
`選挙速報`. Elige palabras más largas en esos idiomas para evitar coincidencias de más, por ejemplo `最終回のネタバレ`
en lugar de `最終回`. Junto a una letra de otra escritura, «Palabra completa» sigue aplicándose: `AI` con «Palabra completa» coincide con
`AIの話` pero no con `AIDS`.

Un filtro no admite comodines ni expresiones regulares. Sus palabras coinciden cuando lo hace cualquiera de ellas; para plegar publicaciones solo cuando dos
palabras aparecen juntas, escríbelas como una sola frase.

<!-- source: limits 2f4588a30027 -->
## Límites

Puedes tener hasta 100 filtros, con hasta 100 palabras cada uno y 1000 palabras en total. Una palabra tiene hasta 100 caracteres.

El archivo de «Exportar», en «Cuenta y datos» de la configuración, contiene tus filtros, también los caducados, como
`keyword_filters.json`.
