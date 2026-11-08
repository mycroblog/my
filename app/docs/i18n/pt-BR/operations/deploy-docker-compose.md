<!-- source: deploying-with-docker-compose 29ec05aaf767 -->
# Implantando com Docker Compose

Como executar o `my` em uma única máquina com Docker Compose: a configuração mais simples, um processo de servidor com SQLite e os arquivos de mídia em um volume nomeado. PostgreSQL, armazenamento de objetos e TLS são arquivos que você adiciona por cima depois
([arquivos do Docker Compose](running.md#docker-compose-files)).

Você precisa do Docker e do Docker Compose v2.

<!-- source: start-the-server ba6cb34daaa0 -->
## Iniciando o servidor

Os arquivos do Compose estão em [`deploy/docker`](../../deploy/docker/compose.yaml) do código-fonte. Pegue-os da tag da versão que você executa, para que combinem com a imagem dela:

```sh
git clone --depth 1 --branch v0.1.0 https://github.com/mycroblog/my.git
cd my/deploy/docker
cp example.env .env
# Set MY_VERSION in .env to the same version, such as 0.1.0
mkdir -m 700 secrets
(umask 022; openssl rand -base64 32 > secrets/kek)   # the key encryption key: back it up apart from the database
docker compose up -d
```

- O Compose baixa a imagem `ghcr.io/mycroblog/my` da `MY_VERSION`. Não existe tag `latest`, então a versão precisa ser informada: uma versão pode exigir passos que uma atualização silenciosa pularia ([as tags da imagem](running.md#get-the-image)).
  Para construir a imagem a partir do código-fonte em vez disso, adicione o override: `docker compose -f compose.yaml -f compose.build.yaml up -d --build`.
- O serviço `schema` executa `my schema apply` da imagem do servidor, que traz o banco de dados, em um volume nomeado, para o esquema daquela versão, e então termina; o servidor (`role=all`) inicia depois que ele tiver sucesso. O esquema é aplicado de novo a cada `up`, então, depois de uma atualização, você inicia o servidor do mesmo jeito.
- O servidor escuta em `127.0.0.1:8080` (a porta é `MY_HTTP_PORT` no `.env`). Ele está pronto quando `curl localhost:8080/readyz` retorna `{"status":"ok"}`.
- A chave de criptografia de chaves (KEK) chega ao servidor a partir de `secrets/kek`, como o segredo do Docker `/run/secrets/my_kek`, nunca como variável de ambiente ([gerenciando a chave de criptografia de chaves](key-management.md)).
- Toda outra configuração da [referência de configuração](../reference/configuration.md) é definida no `.env`, cujo modelo descreve cada uma delas.

<!-- source: create-an-administrator c3d30824f7c0 -->
## Criando um administrador

Crie um administrador e depois consulte a conta com o WebFinger e busque o seu ator. A senha é lida da primeira linha da entrada padrão.

```sh
docker compose exec -T my my admin create -username alice -email alice@example.com < password.txt
curl 'http://localhost:8080/.well-known/webfinger?resource=acct:alice@localhost:8080'
curl -H 'Accept: application/activity+json' http://localhost:8080/ap/actors/<the UUID of the self link from WebFinger>
```

<!-- source: sign-in-from-a-browser 3028115543b0 -->
## Entrando pelo navegador

Com o par `MY_HOST=localhost:8080` e `MY_WEB_ORIGIN=http://localhost:8080` do `example.env`, o login, a API do cliente e a mídia funcionam sobre HTTP simples em `http://localhost:8080` (os ids do ActivityPub continuam como `https://localhost:8080/...`). Se você alterar `MY_HTTP_PORT`, altere também a porta de `MY_HOST` e `MY_WEB_ORIGIN` para que correspondam.

1. Abra `http://localhost:8080/login` em um navegador (ou clique em **Entrar** na barra superior do cliente web); isso abre a tela de login do servidor.
2. Na tela de login, informe o e-mail e a senha do administrador que você criou acima.
3. Você volta ao cliente web, na sua linha do tempo inicial.

- O cookie de login é `Secure`. Chrome e Firefox aceitam esse cookie em `http://localhost`; o Safari (WebKit) não aceita, então use o Chrome ou o Firefox localmente.
- Os links de hashtags e menções nos corpos das publicações usam `https://`, pois são as URLs federadas.

<!-- source: going-public be2475df6407 -->
## Abrindo ao público

- Defina `MY_HOST` no `.env` com o seu domínio real, remova `MY_WEB_ORIGIN` (ele assume `https://{MY_HOST}` por padrão) e encerre o TLS na frente do servidor: a camada `compose.caddy.yaml` executa o Caddy para isso
  ([arquivos do Docker Compose](running.md#docker-compose-files)). `MY_HOST` é gravado no banco de dados na primeira inicialização e não pode ser alterado depois.
- Atrás de um proxy reverso próprio, defina `MY_TRUSTED_PROXIES` no `.env` para que os limites de requisições, como o do login, se apliquem por IP de cliente e não ao proxy
  ([`trusted_proxies`](../reference/configuration.md#trusted_proxies)).
- Antes de abrir o servidor para os usuários, siga a [lista de verificação de produção](production-checklist.md).
- `docker compose down` para a pilha. `down -v` também apaga os volumes de dados.

Para cada configuração, veja a [referência de configuração](../reference/configuration.md); para a operação do dia a dia, veja
[executando o servidor](running.md).
