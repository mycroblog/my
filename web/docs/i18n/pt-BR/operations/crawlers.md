<!-- source: search-engines-and-ai-crawlers 648c61a01042 -->
# Mecanismos de busca e rastreadores de IA

O que o servidor informa aos mecanismos de busca e aos rastreadores de IA, as configurações que mudam isso e como verificar o que ele serve.

robots.txt e as meta tags são declarações que os rastreadores podem ou não respeitar; elas não são um bloqueio técnico. Se você precisa bloquear, use um WAF ou limitação de taxa.

<!-- source: settings 13dfb1b04948 -->
## Configurações

| Escopo | Configuração | Onde alterar | Quando não definida |
| --- | --- | --- | --- |
| Servidor | `search_indexing` (`allow` / `deny`) | A seção "Mecanismos de busca e IA" da página "Busca e rastreadores" das configurações administrativas do servidor, `my instance set`, `PATCH /api/v1/admin/settings` ([Configurações da instância](instance-settings.md)) | `deny` |
| Servidor | `ai_usage` (`allow` / `deny_training` / `deny`) | O mesmo | `deny` |
| Servidor | `indexable_default` (o `indexable` das contas novas) | O mesmo | `false` |
| Servidor | `security_contact` (para onde reportar vulnerabilidades) | O mesmo | Vazio (nenhum `security.txt` é servido) |
| Pessoa | `indexable` (aparecer na busca) | "Mecanismos de busca e IA" nas configurações de perfil, `PATCH /api/v1/accounts/update_credentials` | `indexable_default` no cadastro |
| Pessoa | `ai_opt_out` (recusar treinamento e uso por IA) | O mesmo | `false` |

- A declaração de uma página é a mais restritiva entre a configuração do servidor e a configuração do dono da página. Uma pessoa não pode permitir mais do que o servidor permite.
- Os mecanismos de busca são informados de que podem indexar apenas os perfis e as publicações `public` de contas com `indexable = true`, em um servidor com `search_indexing = allow`.
  Um servidor que quer ser pesquisável altera os dois: `search_indexing` e `indexable_default`. Pessoas que se cadastraram antes de você alterar `indexable_default` não são incluídas até escolherem `indexable` por conta própria.
- Uma configuração alterada passa a valer em `/robots.txt` e nas respostas de páginas e mídia sem reiniciar o servidor.

<!-- source: what-the-server-returns 31584e03601b -->
## O que o servidor retorna

| Resposta | Quando recusa a busca | Quando recusa IA |
| --- | --- | --- |
| Páginas de perfis e de publicações `public` | `noindex, noarchive` em `<meta name="robots">` e `X-Robots-Tag` | `noai, noimageai`, `Content-Usage: train-ai=n` (`train-ai=n, ai-use=n` para `deny`), e `tdm-reservation: 1` (cabeçalho e meta) |
| Publicações `unlisted`, páginas 404 de páginas ocultas ou inexistentes, páginas de contas remotas, telas do aplicativo (`/home`, `/settings` e assim por diante) | `noindex, noarchive`, qualquer que seja a configuração | Segue a configuração do servidor |
| `/`, `/about`, `/about/crawler` | Segue a configuração do servidor | Segue a configuração do servidor |
| `/media/` (quando este servidor a serve) | Segue a configuração do servidor. Sem declaração por dono | Segue a configuração do servidor. Sem declaração por dono |
| A API de cliente, documentos ActivityPub, `/.well-known/`, `/nodeinfo/`, telas de login e OAuth | `X-Robots-Tag: noindex` | Não adicionado (o robots.txt basta) |

`noindex` não impede as pré-visualizações de links (veja [Pré-visualizações de links](link-previews.md)).

<!-- source: robotstxt 636c85dabf63 -->
### `/robots.txt`

`GET /robots.txt` é montado a partir das configurações do servidor e da lista de rastreadores de IA incluída no binário (`text/plain; charset=utf-8`, `Cache-Control: public, max-age=3600`, ETag).
Os operadores não podem adicionar regras próprias.

- Se `ai_usage` for `deny_training` ou `deny`, os rastreadores de treinamento (`GPTBot`, `ClaudeBot`, `CCBot`, `Google-Extended`, `Applebot-Extended` e assim por diante) recebem `Disallow: /`.
- Se `ai_usage` for `deny_training` ou `deny`, um grupo `User-agent: __usage_ai-training` com `Disallow: /` declara uma preferência contra o treinamento de IA para todos os caminhos, qualquer que seja o rastreador que busca o conteúdo (o Internet-Draft `draft-kelley-aipref-training-preference-signal`). Ele nomeia um uso, não um rastreador, então não impede nenhuma busca. Com `ai_usage = allow`, o grupo fica de fora, porque não declara preferência alguma: o `Allow: /` dele declararia uma preferência a favor do treinamento para todos os caminhos, inclusive as páginas das pessoas que definiram `ai_opt_out`.
- Se `ai_usage` for `deny`, os rastreadores de busca de IA, de assistentes e de agentes (`OAI-SearchBot`, `ChatGPT-User`, `Claude-User`, `PerplexityBot` e assim por diante) também recebem `Disallow: /`.
- Todos os rastreadores (`User-agent: *`) são recusados apenas em `/api/`, `/oauth/`, `/auth/`, `/ap/` e `/media/proxy/`, e as configurações são declaradas nas linhas `Content-Usage` (o rascunho IETF AI Preferences) e `Content-Signal` (Content Signals). Recusar a busca não gera `Disallow: /`: bloquear as buscas impediria os rastreadores de ler o `noindex`, e também impediria os buscadores de pré-visualização de links que obedecem ao robots.txt.
- `/.well-known/` e `/nodeinfo` não são recusados, para que as listas de servidores e as estatísticas continuem funcionando.
- Quando `account_domain` é separado do host, o lado do host é quem o serve. O robots.txt do domínio do identificador (handle) é assunto do próprio operador.

<!-- source: well-known 72d7e8cbbc33 -->
### `/.well-known/`

| Caminho | Quando é servido | Conteúdo |
| --- | --- | --- |
| `/.well-known/tdmrep.json` | `ai_usage` é qualquer valor diferente de `allow` | `[{"location": "/*", "tdm-reservation": 1}]` (TDM Reservation Protocol). 404 para `allow` |
| `/.well-known/security.txt` | `security_contact` está definido | `Contact`, `Expires` (30 dias após o horário da resposta), `Preferred-Languages` (`default_locale` e `en`), `Canonical` (RFC 9116) |

<!-- source: information-page 0f89d6c3c3ce -->
### Página de informações

A página de informações da instância (`/about`) mostra se a busca e a IA são permitidas, e o que significa a declaração, a partir de `search_indexing` e `ai_usage` em `GET /api/v1/instance`.
A explicação coloca as declarações em relação ao artigo 30-4 da Lei de Direitos Autorais do Japão (tais declarações não podem ser tratadas como uma recusa legal) e ao artigo 4 da Diretiva DSM da UE (uma reserva legível por máquina para mineração de textos e dados).
Ela não garante nenhum efeito jurídico.

<!-- source: this-servers-own-fetching 0cb9510e19e5 -->
## A busca própria deste servidor

Essas configurações tratam do que rastreia este servidor. O servidor também rastreia outros sites para os cards de links nas publicações, como `my-linkcard`: ele obedece ao `robots.txt` e às diretivas das páginas vinculadas, e limita as suas requisições por host, como descreve [pré-visualizações de links](link-previews.md#cards-of-links-in-posts).
Esse rastreador é definido por `link_cards`, `link_cards_ignore_robots_txt` e, por domínio, com [domínios de link cards](instance-settings.md#link-card-domains).

Os operadores desses sites leem sobre isso em `https://<host>/about/crawler`, para onde `/about` aponta ([a página para operadores de sites](link-previews.md#the-page-for-site-operators)). Com `link_cards_ignore_robots_txt` ativado, eles não podem optar por sair pelo `robots.txt`, e a página diz isso: só podem limitar o card com as diretivas das páginas delas, ou pedir a você uma regra para o domínio delas.

<!-- source: limits 93da7e422ee8 -->
## Limites

- O `ai_opt_out` de uma pessoa é declarado apenas nas páginas que este servidor serve. A mídia não tem dono na chave, e algumas configurações deixam um CDN servi-la diretamente, então a mídia carrega apenas a declaração de todo o servidor.
  As declarações nas páginas servidas por outros servidores ficam por conta desses servidores (nenhuma extensão ActivityPub própria é enviada).
- `ai_usage = deny` também declara uma recusa das ações em que um usuário do ChatGPT ou de um serviço semelhante faz com que uma página deste servidor seja lida.
- O `indexable` de uma conta remota (`toot:indexable` do Mastodon) é recebido e armazenado. Páginas de contas remotas são sempre `noindex` neste servidor.
- O `indexable` também decide se as publicações de uma pessoa entram na [busca de texto completo](search.md) própria deste servidor, em um servidor com mecanismo de busca. Ali ele cobre as visibilidades de `search_visibilities`, que podem incluir `local`. `search_indexing` não tem efeito nessa busca, e o que os mecanismos de busca são informados continua como esta página descreve.

<!-- source: serving-media-from-object-storage-or-a-cdn 98c7ab4994ab -->
## Servindo mídia a partir de armazenamento de objetos ou de um CDN

Quando o armazenamento de objetos ou um CDN serve a mídia diretamente, as respostas de `/media/` não passam por este servidor e não carregam os cabeçalhos da tabela acima.
A declaração de todo o servidor continua valendo para tudo, por meio de `/robots.txt`; para adicionar os cabeçalhos também, inclua o seguinte em `/media/*` nas regras de cabeçalho de resposta do CDN (um exemplo para `ai_usage = deny`).

```text
X-Robots-Tag: noindex, noarchive, noai, noimageai
Content-Usage: train-ai=n, ai-use=n
tdm-reservation: 1
```

Quando você alterar as configurações, altere as regras do CDN para combinar. `/media/proxy/` é servido por este servidor, então não precisa de regra.

<!-- source: the-ai-crawler-list 675a37cb0caf -->
## A lista de rastreadores de IA

A lista é gerada a partir do `robots.json` de [ai-robots-txt/ai.robots.txt](https://github.com/ai-robots-txt/ai.robots.txt) (MIT) e incluída no binário. Ela não é buscada em tempo de execução, então uma lista mais nova vem com uma atualização.

<!-- source: verify 1633f03abedb -->
## Verificar

```sh
curl -sS https://<host>/robots.txt
curl -sS -o /dev/null -D - -H 'Accept: text/html' https://<host>/@alice | grep -iE 'x-robots-tag|content-usage|tdm-reservation'
curl -sS https://<host>/.well-known/tdmrep.json
curl -sS https://<host>/.well-known/security.txt
```

- Em um servidor sem configurações, `/robots.txt` tem `User-agent: GPTBot`, `User-agent: __usage_ai-training` e `Content-Usage: search=n, train-ai=n, ai-use=n`,
  e os perfis trazem `X-Robots-Tag: noindex, noarchive, noai, noimageai`.
- Com `search_indexing = allow` e `ai_usage = allow`, o perfil de uma conta que escolheu `indexable` não traz `X-Robots-Tag`, e `/.well-known/tdmrep.json` retorna 404.
