<!-- source: link-previews-ogp--x-card--fediversecreator 85d4b284637f -->
# Prévias de link (OGP / X Card / fediverse:creator)

As prévias que outros serviços mostram das páginas deste servidor, o que o servidor retorna para elas e os passos para verificá-las manualmente. Os cartões que este servidor mostra para os links nas próprias publicações
estão em [Cartões de links em publicações](#cards-of-links-in-posts) abaixo; como parar de buscá-los de um domínio está nas [configurações da instância](instance-settings.md#link-card-domains).

<!-- source: what-the-server-returns bb9326f9729a -->
## O que o servidor retorna

`my serve` retorna o HTML da página inicial (`https://<host>/`), da página de informações da instância (`https://<host>/about`),
das páginas dos documentos publicados (`/about/terms`, `/about/privacy`, `/about/commerce`), da página sobre o rastreador de cartões de link (`/about/crawler`), dos perfis
(`https://<host>/@alice`) e das publicações (`https://<host>/@alice/status/<post ID>`), embutindo meta tags no shell do
cliente web.
Uma requisição de HTML é sempre tratada como sem login, então uma prévia mostra apenas o que um visitante sem login pode ver.

| Página | Resposta |
| --- | --- |
| A página inicial (`/`) e a página de informações da instância (`/about`) | 200. O shell com meta tags que apresentam a instância (veja "A página inicial" abaixo) |
| A página sobre o rastreador de cartões de link (`/about/crawler`) | 200. O shell com meta tags que apresentam a instância, com o título "Rastreador de cartões de link \| nome da instância" |
| As páginas dos documentos publicados (`/about/terms`, `/about/privacy`, `/about/commerce`) | 200. O shell com meta tags que apresentam a instância, com o título "nome do documento \| nome da instância". A página de um documento que não está publicado recebe o shell genérico |
| Uma publicação `public`, o perfil de uma conta local ativa | 200. O shell com meta tags |
| Uma publicação `unlisted` | 200. O shell com meta tags, mais `<meta name="robots" content="noindex, noarchive">` e `X-Robots-Tag: noindex, noarchive` |
| Uma publicação `local`, `private`, `direct`, excluída ou inexistente, e o perfil e as publicações de uma conta suspensa, desativada, excluída ou limitada | 404. O shell genérico e `X-Robots-Tag: noindex, noarchive`. O corpo e os cabeçalhos são iguais seja qual for o motivo |
| `/@<old username>/status/<post ID>` | 301, para a URL com o nome de usuário atual |

Toda página também leva as declarações para mecanismos de busca e rastreadores de IA (meta tags como `noindex` e `noai`, e
`X-Robots-Tag`, `Content-Usage` e `tdm-reservation`), conforme as configurações da instância e as do dono
([mecanismos de busca e rastreadores de IA](crawlers.md)). O `noindex` da tabela acima é sempre adicionado, sejam quais forem as configurações.
`noindex` não impede as prévias de link.

Uma requisição com um `Accept` do ActivityPub não recebe o shell; o lado ActivityPub a responde (um perfil responde com o
`id` do ator, e uma publicação que um visitante sem login pode ver responde com um 302 para o seu `id`). Toda resposta leva `Vary: Accept`.

<!-- source: meta-tags 00a4a8115b9e -->
### Meta tags

| Tag | Conteúdo |
| --- | --- |
| `og:title`, `<title>` | "Nome de exibição (@nome de usuário@domínio da conta)". O nome de usuário se o nome de exibição estiver vazio |
| `og:type` | `profile` para um perfil, `article` para uma publicação |
| `og:url`, `<link rel="canonical">` | A URL canônica (a URL com o nome de usuário atual) |
| `og:description` | O texto simples do corpo da publicação (a bio, para um perfil) sem tags, cortado em 200 grafemas. Para uma publicação com aviso de conteúdo, apenas o texto do aviso de conteúdo |
| `og:image`, `og:image:alt` / `width` / `height` | A imagem escolhida na ordem abaixo |
| `og:site_name` | O nome da instância (o texto `name`, igual em todos os idiomas; `host` quando não há; [nome e domínio](instance-settings.md#name-and-domain)) |
| `og:locale`, `<html lang dir>` | O idioma da publicação. Quando não está definido (`und`), o idioma padrão de publicação do autor e, na falta dele, o `default_locale` da instância. Não muda com `Accept-Language` |
| `article:published_time`, `article:author` | Apenas publicações. O horário da publicação (UTC) e a URL do perfil do autor |
| `profile:username` | Apenas perfis. `username@account domain` |
| `twitter:card` | `summary_large_image` quando a imagem é mais larga que cerca de 2:1 (largura / altura >= 1,7) e tem pelo menos 300x157, caso contrário `summary` |
| `fediverse:creator` | `@username@account domain`. O Mastodon o mostra como "autor" |
| `<link rel="alternate" type="application/activity+json">` | O `id` do AP (o ator ou o objeto da publicação) |

As meta tags da página inicial apresentam a própria instância.

- `og:type` é `website`, `og:title`, `<title>` e `og:site_name` são o nome da instância, e `og:url` e
  `<link rel="canonical">` são `https://<host>/`.
- `og:description` é o texto da instância `short_description` cortado em 200 grafemas. É omitido quando não há.
- O idioma (`og:locale`, `<html lang dir>`) é o `default_locale` da instância. O nome e a descrição são o mesmo valor
  em todos os idiomas.
- `og:image` é o número 3 abaixo (a imagem de prévia da instância) ou o número 4 ou 5 se não houver.
- Ela não pertence a uma conta, então `fediverse:creator` e o `alternate` do ActivityPub não são gerados.
- A página de informações da instância (`/about`) tem as mesmas meta tags. `og:title` e `<title>` são "Sobre {nome da instância}"
  (`server:preview.about_title`), e `og:url` e `<link rel="canonical">` são `https://<host>/about`.
- As páginas dos documentos publicados (`/about/terms`, `/about/privacy`, `/about/commerce`) também têm as mesmas meta tags.
  `og:title` e `<title>` são "{nome do documento} | {nome da instância}" (`server:preview.document_title`), e `og:url` e
  `<link rel="canonical">` são o endereço dessa página. A página de um documento que não está publicado recebe o shell genérico.
- A página sobre o rastreador de cartões de link (`/about/crawler`) tem as mesmas meta tags, com a mesma forma de título e de endereço.

`og:image` é escolhida nesta ordem.

1. A primeira imagem anexada à publicação (o texto alternativo dela vai para `og:image:alt`). Não é usada para uma publicação marcada como sensível ou
   com aviso de conteúdo.
2. A imagem de cabeçalho do perfil ou, na falta dela, o avatar (`og:image:alt` é "Imagem de cabeçalho de {name}" ou "Avatar de {name}").
3. A imagem de prévia da instância (a configuração da instância `preview_image`; `og:image:alt` é o texto `preview_image_alt`, ou "Imagem
   de {nome da instância}" se não houver).
4. A versão de prévia de link do logotipo da própria instância ([logotipo e ícone](instance-settings.md#logo-and-icon)), um PNG de 1200x630
   em `https://<host>/brand/<revision>/preview-1200x630.png` (`og:image:alt` é "Imagem de {nome da instância}").
5. A imagem padrão incluída (`https://<host>/preview-image.png`, um PNG de 1200x630. É uma imagem genérica sem o nome da
   instância, e `og:image:alt` descreve a imagem).

Assim, uma página que mostra uma prévia sempre tem `og:image` e `og:image:alt`.
As imagens de 1 a 3 são a variante `full` da mídia processada, e a largura e a altura são os valores do processamento.
Se `preview_image` ainda está sendo processada, falhou no processamento ou não é uma imagem, usa-se o número 4 (ou o 5 sem um logotipo).
As configurações da instância são lidas a cada requisição, então `my instance set preview_image ...` entra em vigor sem reiniciar (para o
procedimento, veja as [configurações da instância](instance-settings.md)).

A imagem padrão é desenhada com a biblioteca padrão na primeira requisição após a inicialização e mantida no processo (cerca de 10 KB).
Ela é servida com `Cache-Control: no-cache` e um ETag.

<!-- source: caching 5bc906452b10 -->
### Cache

- A resposta leva `Cache-Control: public, max-age=60, stale-while-revalidate=300`. Uma exclusão, uma mudança de visibilidade ou a
  publicação ou despublicação de um documento permanece nos caches compartilhados por no máximo esse tempo.
- O ETag é montado a partir das versões do template, dos assets e das configurações da instância, do idioma, da versão
  (revisão) da publicação, da visibilidade efetiva, da versão do perfil e da imagem escolhida (para a página inicial, a página de
  informações, as páginas de documentos e a página sobre o rastreador: o endereço da página, o título, a descrição da instância e a imagem escolhida).
  `If-None-Match` recebe `304`.
- Se você colocar uma CDN na frente, inclua `Accept` na chave de cache (para que o JSON do AP e o HTML não se misturem).

<!-- source: manual-check bf2c7a86a5de -->
## Verificação manual

Verifique cada serviço em um dispositivo real após uma atualização ou uma mudança nas configurações de prévia. Você precisa de uma instância acessível de fora por HTTPS (os
bots de prévia não alcançam `localhost`).

<!-- source: preparation 9fd604dc1529 -->
### Preparação

1. Com uma conta de teste, prepare estas publicações.
   - Uma publicação `public` com uma imagem paisagem (por exemplo 1200x630) e texto alternativo
   - Uma publicação `public` sem imagem (de uma conta que tem imagem de cabeçalho)
   - Uma publicação `public` cujo corpo tem mais de 200 caracteres e contém emoji (como 👨‍👩‍👧‍👦)
   - Uma publicação `unlisted`
   - Uma publicação `private`
   - Uma publicação de uma conta cujo nome de exibição contém `"><script>alert(1)</script>`
2. Verifique o HTML de cada URL.

   ```sh
   curl -sS -D - -H 'Accept: text/html' https://<host>/@alice/status/<post ID>
   ```

   - O status é 200 e as meta tags da tabela acima estão lá. A publicação `unlisted` tem `noindex`.
   - Uma publicação `private` e um ID inexistente dão o mesmo corpo 404 com `X-Robots-Tag: noindex, noarchive` (e `noai,
     noimageai` também, quando as configurações recusam IA).

     ```sh
     diff <(curl -sS https://<host>/@alice/status/<private post ID>) <(curl -sS https://<host>/@alice/status/0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f)
     ```

   - O `<script>` no nome de exibição é escapado como `&lt;script&gt;`.

<!-- source: x 73a2dffdfdbb -->
### X

1. Cole a URL da publicação com imagem na tela de composição (ou em uma mensagem direta).
2. Verifique que o cartão de imagem grande (`summary_large_image`) mostra o título, o domínio e a imagem.
3. Para a URL da publicação sem imagem, verifique que o cartão mostra a imagem de cabeçalho. Para uma conta sem imagem de cabeçalho
   nem avatar, verifique que o cartão grande mostra a imagem padrão (`preview_image`, ou a imagem incluída quando ela
   não está definida).
4. Para uma conta com apenas um avatar quadrado, verifique que o cartão é o de imagem pequena (`summary`).

O X guarda cartões em cache. Para verificar uma mudança, cole a URL com uma query (como `?v=2`) para que ela conte como uma URL diferente.

<!-- source: mastodon 02868aebd430 -->
### Mastodon

1. A partir de uma conta em outra instância, faça uma publicação que contenha a URL da publicação com imagem.
2. Verifique que um cartão de prévia aparece sob a publicação com o título, a descrição e a imagem.
3. Verifique que a conta de teste (`fediverse:creator`) é mostrada como "autor" sob o cartão e que você consegue ir ao perfil
   dela.

O Mastodon guarda o cartão em cache em cada servidor que o buscou. Para gerá-lo de novo, publique com uma URL nova (com uma query).

<!-- source: discord 4df04642bb64 -->
### Discord

1. Cole a URL da publicação com imagem em um canal.
2. Verifique que o embed mostra o nome do site (o nome da instância), o título, a descrição e uma imagem grande.
3. Para uma publicação com mais de 200 caracteres, verifique que a descrição termina com `…` e que nenhum emoji é cortado no meio.

<!-- source: slack 9f1e524968f3 -->
### Slack

1. Cole a URL da publicação com imagem em um canal.
2. Verifique que a prévia expandida mostra o nome do site, o título, a descrição e a imagem.
3. Verifique que uma prévia também aparece para a URL de uma publicação `unlisted` (`noindex` é para mecanismos de busca e não impede
   as prévias).

O Slack também guarda prévias em cache. Para verificar uma mudança, adicione uma query à URL.

<!-- source: line ef9e1e682603 -->
### LINE

1. Envie a URL da publicação com imagem em um chat.
2. Verifique que aparece uma prévia com o título, a descrição e a imagem.

O LINE também guarda prévias em cache. Para verificar uma mudança, adicione uma query à URL.

<!-- source: what-must-not-be-visible 187905fe778d -->
### O que não deve ficar visível

Em todos os serviços, verifique que a URL de uma publicação `private` e a URL de uma publicação inexistente dão a mesma prévia genérica
(apenas o nome da instância, ou nada) e que nenhum corpo, nome de exibição ou imagem aparece.

<!-- source: cards-of-links-in-posts 6394a6bf5641 -->
## Cartões de links em publicações

Para mostrar um cartão para um link em uma publicação, o job deste servidor busca a página vinculada e a imagem dela, e obedece ao que o site vinculado diz a respeito.
Suas requisições são `my-linkcard/<version> (+https://<host>/)` para a página, a imagem e o `robots.txt`. Toda outra requisição do servidor, inclusive a do proxy de mídia, continua `my/<version> (+https://<host>/)`.
As requisições não enviam cookies nem `Referer`, e nada diz qual conta ou publicação vinculou a página.

<!-- source: the-page-for-site-operators 08a157cd9eac -->
### A página para operadores de sites

`https://<host>/about/crawler` conta aos operadores dos sites vinculados o que o rastreador requisita, o que ele envia, com que frequência e como optar por sair, e que eles podem pedir que você pare de buscar do domínio deles. Qualquer pessoa pode lê-la sem uma conta. A seção "Mecanismos de busca e IA" da página de informações da instância (`/about`) liga a ela; o `User-Agent` nomeia apenas `https://<host>/`.

Ela diz o que este servidor faz, a partir de `link_cards` e `link_cards_ignore_robots_txt`, e mostra o contato da instância (`contact_account`, `contact_email`). Defina um contato, para que um site tenha a quem pedir: sem um, a página diz que este servidor não publica nenhum.
Com `link_cards_ignore_robots_txt` ativado, a página diz que um site não pode recusar este servidor no `robots.txt`.

<!-- source: robotstxt e8d2deaef93d -->
### robots.txt

O nome no `robots.txt` é `my-linkcard`, em qualquer caixa. Sem um grupo com esse nome, o grupo `*` se aplica, como diz a RFC 9309. `Allow`, `Disallow`, `*` e `$` são lidos, vencendo a correspondência mais longa e, em caso de empate, `Allow`; `Crawl-delay` e outros registros não são lidos.
O `robots.txt` é lido até 512 KiB em 10 segundos, e o que foi lido de um arquivo maior é usado. O servidor guarda apenas as regras de `my-linkcard`, por origem (esquema, host e porta).

| Resposta para `robots.txt` | O que o servidor faz | Guardado por |
| --- | --- | --- |
| 200 | Obedece ao arquivo | 24 horas |
| 4xx exceto 429 | Sem arquivo: busca | 24 horas |
| 429, 5xx, uma falha de rede ou um redirecionamento que o servidor recusa | Não busca nada da origem | 1 hora |
| Mais de 5 redirecionamentos | Sem arquivo: busca | 24 horas |

- A URL da página é verificada antes de ser requisitada, seja qual for a visibilidade da publicação. O mesmo vale para cada URL para a qual ela redireciona, contra o `robots.txt` da origem dessa URL, e um redirecionamento para uma URL recusada termina ali: o servidor não a requisita e não cria cartão.
- Uma URL que o `robots.txt` proíbe não é buscada de novo por 24 horas, como qualquer outra falha de uma página. Quando o arquivo apenas não pôde ser lido, o servidor tenta a página de novo uma hora depois, até o limite de tentativas do job.
- Uma imagem é verificada da mesma forma contra a própria origem. Uma imagem proibida deixa um cartão sem imagem. Quando o arquivo apenas não pôde ser lido, uma imagem registrada antes permanece, e uma imagem nova fica de fora até a próxima nova busca.
- Uma nova busca, que uma publicação pode causar 14 dias após a última busca, remove o cartão quando o `robots.txt` proíbe a URL dele. Quando o arquivo apenas não pôde ser lido, o cartão permanece.
- O servidor não percorre os cartões para verificá-los de novo, então um site que muda suas regras perde um cartão na próxima nova busca desse cartão.

`link_cards_ignore_robots_txt` ([configurações da instância](instance-settings.md#settings-one-key-one-value)) desativa tudo isso: o servidor não busca nem usa o `robots.txt`. Ele ainda se identifica como `my-linkcard`, e os sites vinculados então não podem recusar as buscas com o `robots.txt`: restam apenas as diretivas das páginas deles e um pedido a você de uma [regra no domínio deles](instance-settings.md#link-card-domains), como [a página para operadores de sites](#the-page-for-site-operators) informa.

<!-- source: directives-of-a-page a854ef2ae8ea -->
### Diretivas de uma página

O servidor também lê as diretivas da página que buscou: `<meta name="robots">` e `<meta name="my-linkcard">` no `<head>`, e `X-Robots-Tag` sem o nome de um bot ou com `my-linkcard:`. Diretivas para outro bot não se aplicam. Quando há várias, vence a mais restritiva.
Elas se aplicam seja qual for o valor de `link_cards_ignore_robots_txt`.

| Diretiva | Cartão |
| --- | --- |
| `nosnippet`, `max-snippet:0` | Sem descrição |
| `noimageindex`, `max-image-preview:none` | Sem imagem, que não é buscada |
| `noindex`, `nofollow`, `noarchive`, `none` e as demais | Sem mudança |

O título, o nome do site e o autor permanecem em todas as diretivas; um site que não os quer recusa a busca no `robots.txt`.
As diretivas só são conhecidas depois que a página é buscada, então não impedem a requisição da página. Uma nova busca aplica as diretivas que a página tiver então.

<!-- source: limits-per-host 41838af70d25 -->
### Limites por host

O rastreador é gentil com cada host, seja qual for o valor de `link_cards_ignore_robots_txt`. Um host é o nome na URL, sem a porta, então um site vinculado e o host de suas imagens têm limites separados.

- Ele faz uma busca por vez em um host, e a seguinte começa pelo menos 1 segundo depois que a última terminou. Uma busca é uma requisição com seus redirecionamentos e, quando precisa ser lido, o `robots.txt` de que precisa. Uma página e sua imagem são duas buscas, também em um só host.
- Um `429` ou um `503` interrompe as buscas no host que respondeu, também quando ele respondeu a um redirecionamento, a uma imagem ou ao `robots.txt`. Elas param pelo `Retry-After` dele, em segundos ou como data, por pelo menos 1 segundo e no máximo 1 hora. Um `Retry-After` ausente ou ilegível significa 1 minuto.
- Um job que precisa esperar a sua vez volta mais tarde. Nenhum worker espera, e ele não usa nenhuma das tentativas do job, então mesmo uma rajada de links para um só site termina com todos os cartões e nenhum job com falha. Os jobs de um host acordam cerca de um por segundo.
- Uma URL cujo host interrompeu o rastreador mantém o cartão, não é lembrada como falha e é buscada quando a pausa termina. Depois de 5 respostas assim para uma URL em 24 horas, a página conta como falha, como qualquer outra que não seja `200`, e uma imagem fica de fora do cartão.
- Um cartão que espera pela imagem guarda o que leu da página por até 2 horas, para que a página não seja requisitada de novo.
- As [regras em um domínio](instance-settings.md#link-card-domains) e as ações de domínio vêm primeiro: nada espera por um host do qual nada é buscado.

Os limites ficam no armazenamento chave-valor e todos expiram: um worker que para no meio de uma busca libera o host dele em 2 minutos.
Esses números não são configurações. Para ver os limites em ação:

- `my.link_card.host_pauses` conta as pausas pelo status que o host respondeu, e cada pausa registra `link card host paused` em `INFO` com `host`, `status` e `pause_seconds`.
- `my.job.duration` com `my.job.type="content.fetch_link_card"` e `my.outcome="postponed"` conta as execuções que voltaram mais tarde. Um número grande significa muitos links para poucos sites ou uma pausa longa, não uma falha.
