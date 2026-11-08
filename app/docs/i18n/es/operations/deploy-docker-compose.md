<!-- source: deploying-with-docker-compose 29ec05aaf767 -->
# Desplegar con Docker Compose

Cómo ejecutar `my` en una sola máquina con Docker Compose: la configuración más pequeña, un proceso de servidor con
SQLite y los archivos multimedia en un volumen con nombre. PostgreSQL, el almacenamiento de objetos y TLS son archivos
que se añaden después ([archivos de Docker Compose](running.md#docker-compose-files)).

Necesitas Docker y Docker Compose v2.

<!-- source: start-the-server ba6cb34daaa0 -->
## Arrancar el servidor

Los archivos de Compose están en [`deploy/docker`](../../deploy/docker/compose.yaml) del código fuente. Tómalos de la
etiqueta de la versión que ejecutas, para que coincidan con su imagen:

```sh
git clone --depth 1 --branch v0.1.0 https://github.com/mycroblog/my.git
cd my/deploy/docker
cp example.env .env
# Set MY_VERSION in .env to the same version, such as 0.1.0
mkdir -m 700 secrets
(umask 022; openssl rand -base64 32 > secrets/kek)   # the key encryption key: back it up apart from the database
docker compose up -d
```

- Compose descarga la imagen `ghcr.io/mycroblog/my` de `MY_VERSION`. No existe la etiqueta `latest`, así que hay que indicar la versión: una versión puede requerir pasos que una actualización silenciosa omitiría ([las etiquetas de la imagen](running.md#get-the-image)). Para compilar la imagen desde el código descargado, añade el archivo de sustitución: `docker compose -f compose.yaml -f compose.build.yaml up -d --build`.
- El servicio `schema` ejecuta `my schema apply` de la imagen del servidor, que lleva la base de datos de un volumen con nombre al esquema de esa versión, y termina; el servidor (`role=all`) arranca cuando ha terminado con éxito. El esquema se aplica de nuevo en cada `up`, así que tras una actualización arrancas el servidor del mismo modo.
- El servidor escucha en `127.0.0.1:8080` (el puerto es `MY_HTTP_PORT` en `.env`). Está listo cuando `curl localhost:8080/readyz` devuelve `{"status":"ok"}`.
- La clave de cifrado de claves (KEK) llega al servidor desde `secrets/kek` como el secreto de Docker `/run/secrets/my_kek`, nunca como variable de entorno ([gestionar la clave de cifrado de claves](key-management.md)).
- Todos las demás configuraciones de la [referencia de configuración](../reference/configuration.md) se definen en `.env`, cuya plantilla los describe.

<!-- source: create-an-administrator c3d30824f7c0 -->
## Crear un administrador

Crea un administrador, busca después la cuenta con WebFinger y recupera su actor. La contraseña se lee de la primera
línea de la entrada estándar.

```sh
docker compose exec -T my my admin create -username alice -email alice@example.com < password.txt
curl 'http://localhost:8080/.well-known/webfinger?resource=acct:alice@localhost:8080'
curl -H 'Accept: application/activity+json' http://localhost:8080/ap/actors/<the UUID of the self link from WebFinger>
```

<!-- source: sign-in-from-a-browser 3028115543b0 -->
## Iniciar sesión desde un navegador

Con el par `MY_HOST=localhost:8080` y `MY_WEB_ORIGIN=http://localhost:8080` de `example.env`, el inicio de sesión, la API
de cliente y el contenido multimedia funcionan por HTTP sin cifrar en `http://localhost:8080` (los ids de ActivityPub
siguen siendo `https://localhost:8080/...`). Si cambias `MY_HTTP_PORT`, cambia también el puerto de `MY_HOST` y de
`MY_WEB_ORIGIN`.

1. Abre `http://localhost:8080/login` en un navegador (o pulsa **Iniciar sesión** en la barra superior del cliente web); se abre la pantalla de inicio de sesión del servidor.
2. En la pantalla de inicio de sesión, escribe la dirección de correo electrónico y la contraseña del administrador que has creado antes.
3. Vuelves al cliente web, a tu cronología de inicio.

- La cookie de inicio de sesión es `Secure`. Chrome y Firefox la aceptan en `http://localhost`; Safari (WebKit) no, así que usa Chrome o Firefox en local.
- Los enlaces de etiquetas y menciones en el cuerpo de las publicaciones usan `https://`, ya que son las URL federadas.

<!-- source: going-public be2475df6407 -->
## Hacerlo público

- Define `MY_HOST` en `.env` con tu dominio real, elimina `MY_WEB_ORIGIN` (por defecto es `https://{MY_HOST}`) y termina TLS delante del servidor: la capa `compose.caddy.yaml` ejecuta Caddy para ello ([archivos de Docker Compose](running.md#docker-compose-files)). `MY_HOST` se guarda en la base de datos en el primer arranque y no se puede cambiar después.
- Detrás de un proxy inverso propio, define `MY_TRUSTED_PROXIES` en `.env` para que los límites de frecuencia, como el del inicio de sesión, se apliquen por IP de cliente y no al proxy ([`trusted_proxies`](../reference/configuration.md#trusted_proxies)).
- Antes de abrir el servidor a los usuarios, repasa la [lista de comprobación para producción](production-checklist.md).
- `docker compose down` detiene la pila. `down -v` también elimina los volúmenes de datos.

Para ver todos las configuraciones, consulta la [referencia de configuración](../reference/configuration.md); para la operación
diaria, consulta [ejecutar el servidor](running.md).
