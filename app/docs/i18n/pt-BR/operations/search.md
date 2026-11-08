<!-- source: full-text-search-of-posts fc16f55b56ea -->
# Busca de texto completo das publicações

O que guarda a busca de texto completo das publicações, como ativá-la e alterá-la, como reconstruir o seu índice e o que ela exige dos seus logs e backups.

<!-- source: choosing-an-engine 9cfd51361e0a -->
## Escolhendo um mecanismo de busca

A busca de texto completo das publicações precisa de um mecanismo de busca que você executa ou aluga. [`search`](../reference/configuration.md#search) escolhe qual:

- `none`, o padrão: sem busca de texto completo. O servidor não se conecta a nada, não enfileira nenhum job de busca e o cliente não mostra nenhuma forma de buscar publicações. Em um servidor assim, cada comando `my search` avisa isso e não faz nada.
- `elasticsearch`, `opensearch` ou `meilisearch`: esse mecanismo, com as configurações da seção dele em [Mecanismos de busca](search-engines.md).
- `plugin`: um programa seu que mantém o índice em qualquer outro mecanismo, como Groonga ou um índice criado com Sudachi, em `plugin_search_address` ([Buscando publicações com um plugin](plugins.md#searching-posts)).

A busca de contas e hashtags funciona a partir do banco de dados e não depende dessa configuração.

O servidor cuida apenas dos campos que ele mesmo grava, filtra, ordena e pagina. Como o mecanismo analisa o texto, como classifica os resultados e qual é o tamanho do seu índice são decisões suas, feitas pelo arquivo de configurações do mecanismo; [Mecanismos de busca](search-engines.md) descreve o que cada mecanismo encontra em japonês com as configurações padrão e como alterá-las.

<!-- source: what-the-index-holds 5e0884482ea8 -->
## O que o índice guarda

O banco de dados continua sendo a fonte da verdade, e o índice é uma cópia que você pode descartar e reconstruir. Um mecanismo devolve apenas IDs de publicações: o servidor lê cada publicação de novo no banco de dados e aplica as regras usuais de visibilidade, de modo que um índice atrasado pode deixar uma publicação sem ser encontrada, mas não pode mostrar uma que não deveria ser vista.

Uma publicação está no índice quando todas estas condições valem:

- Ela não está excluída.
- Sua visibilidade é `public` ou `local`, e [`search_visibilities`](instance-settings.md) a inclui. Publicações `unlisted`, `private` e `direct` nunca são indexadas, qualquer que seja a configuração.
- Seu autor ativou `indexable`, a configuração descrita em [Mecanismos de busca e rastreadores de IA](crawlers.md).
- Seu autor e o domínio do autor não têm nenhuma restrição que os oculte da descoberta.

O índice guarda o corpo, o aviso de conteúdo e os textos alternativos da versão atual de cada publicação, com o autor e o idioma. Ele não guarda enquetes, perfis nem versões antigas.

`indexable_default` é `false` por padrão, então um servidor que acabou de ativar a busca encontra pouco até que as pessoas escolham `indexable` por conta própria.

<!-- source: turning-it-on 3d88170680a2 -->
## Ativando a busca

1. Execute o mecanismo e configure `search` e as configurações do backend dele.
2. Se você executa os workers separadamente, decida sobre [a disposição deles](#keeping-search-from-delaying-other-jobs).
3. Configure [os seus logs](#the-query-appears-in-urls).
4. Se o mecanismo for executado por outra pessoa, inclua-o nos seus [documentos de privacidade](#your-privacy-documents).
5. Reinicie o servidor; a partir daí, as publicações novas entram no índice.
6. Execute `my search rebuild` para adicionar as publicações existentes.

A busca funciona durante a reconstrução, e os resultados ficam incompletos até ela terminar.

<!-- source: rebuilding-the-index 8a8ea41dfc98 -->
## Reconstruindo o índice

```sh
my search rebuild              # empty the index, then add every post that qualifies
my search rebuild --keep       # add them without emptying the index
my search rebuild --rate 50    # at most 50 posts a second
my search status               # the progress and the failures
my search clear                # empty the index and end any rebuild
```

O comando retorna quando a reconstrução começa, e o worker faz o resto como jobs `search.rebuild` ([referência de jobs](../reference/jobs.md#full-text-search)), então mantenha um worker em execução. Cada job cobre 100 publicações.

- **Taxa.** O padrão é 20 publicações por segundo, no máximo 1000, e ela conta cada publicação `public` e `local` examinada, inclusive as que não se qualificam. Com o padrão, um milhão de publicações levam cerca de 14 horas. A taxa deixa espaço para outros jobs e para o banco de dados, então aumente-a só se o seu servidor tiver folga.
- **Qual executar.** O padrão remove tudo o que não deveria estar no índice: documentos deixados por uma exclusão que nunca chegou, ou pela restauração do banco de dados a partir de um backup. A busca encontra menos até a reconstrução terminar. Use `--keep` depois de [adicionar uma visibilidade](#changing-search_visibilities), quando não houver nada a remover e a busca deve continuar completa. `--keep` nunca remove um documento de uma publicação que não está mais no banco de dados.
- **Interrompida?** Execute o mesmo comando de novo. Ela continua a partir da última publicação tratada e não esvazia o índice outra vez. Adicione `--rate` para mudar a taxa. Uma reconstrução também continua sozinha quando um worker reinicia.
- **O outro modo.** Enquanto uma reconstrução não terminou, o comando no outro modo é recusado, porque não faria o que diz. Para recomeçar em qualquer um dos modos, execute `my search clear` e depois o comando que você quer.
- **Executar duas vezes** dá o mesmo índice. Publicações criadas, editadas ou excluídas durante uma reconstrução acabam corretas, porque os jobs delas continuam rodando e cada job lê a publicação como ela está naquele momento.

`my search status` mostra o backend, `search_visibilities` e, para a última reconstrução, a fase dela (`clearing`, `filling` ou `finished`), as publicações tratadas e as que falharam, a taxa e o último erro.

Quando algo dá errado:

- **O mecanismo está fora do ar.** Se três publicações seguidas falharem, o job para onde está, registra o erro e tenta de novo. Uma reconstrução que esgotou as tentativas deixa um job com falha (`my job list`), e `updated` no status para de avançar. Quando o mecanismo voltar, execute o mesmo comando para continuar.
- **O mecanismo recusa uma publicação.** A publicação é registrada como falha e a reconstrução segue em frente. O status lista até 20 delas. Corrija a causa e depois execute `my search rebuild --keep` para tentar todas as publicações de novo.

<!-- source: changing-search_visibilities 4adf54d02fe9 -->
## Alterando `search_visibilities`

Deixar uma visibilidade de fora remove as suas publicações do índice automaticamente. Adicionar uma visibilidade encontra apenas publicações novas, até você executar `my search rebuild --keep`.

**Avise seus usuários antes de adicionar `local`.** `indexable` passa então a cobrir as publicações `local` de todos que já o ativaram, e a reconstrução adiciona as publicações `local` antigas deles, sem perguntar de novo. As configurações de perfil dizem o que `indexable` cobre no seu servidor, mas quem escolheu essa opção há muito tempo não vai ver isso a menos que você avise.

<!-- source: switching-off-or-changing-the-engine 538a78a629e3 -->
## Desligando ou trocando o mecanismo

Quando o servidor passa para `search = none` ou para outro mecanismo, ele perde a conexão com o índice antigo. Esse índice não é mais limpo, então publicações excluídas depois permanecem nele, com o texto.

1. Pare o servidor e os workers, para que nada escreva no índice depois do próximo passo.
2. Execute `my search clear`.
3. Altere `search` e inicie o servidor.
4. Com um mecanismo novo, execute `my search rebuild`.

Se você já trocou, ou não consegue parar o servidor, exclua o índice antigo no próprio mecanismo. `my search clear` não consegue alcançá-lo depois que a configuração mudou. Jobs enfileirados antes da mudança não fazem nada depois da passagem para `none`.

**Voltando de `none`.** Execute `my search rebuild` sem `--keep`: o servidor não guarda registro do que foi excluído enquanto não tinha mecanismo, e só esvaziar o índice remove essas publicações. Se uma reconstrução não tinha terminado quando você saiu, execute `my search clear` primeiro, porque uma retomada não esvazia o índice.

<!-- source: keeping-search-from-delaying-other-jobs c01b5c259acf -->
## Impedir que a busca atrase outros jobs

Por padrão, um worker executa um job por vez, então um mecanismo lento divide o tempo desse worker com `federation.inbox`, embora cada chamada ao mecanismo tenha um limite de 10 segundos. Um servidor movimentado deve separá-los:

- Execute um processo de worker com `worker_job_types = search.*` e os outros com `worker_job_types_except = search.*` ([Separando workers por tipo de job](running.md#splitting-workers-by-job-type)).
- Com `queue = cloudtasks`, envie os tipos `search.*` para a fila própria deles, com uma [rota de fila](running.md#routing-job-types-to-their-own-queues).

Um servidor pequeno pode deixá-los juntos, e os jobs de busca então dividem o tempo com a federação.

<!-- source: the-query-appears-in-urls 488e2c1e91dc -->
## A consulta aparece nas URLs

`GET /api/v1/statuses/search` leva o que as pessoas buscam na sua URL, como `?q=`. A linha de log `http request` do próprio servidor guarda o caminho e a rota, não a query string, e rastreamentos e relatórios de erro também a deixam de fora. Mas qualquer coisa colocada na frente do servidor pode registrá-la, e uma busca pode revelar muito sobre uma pessoa. Verifique cada camada da sua configuração e, ou mantenha a consulta fora do log dela, ou trate esse log como dado pessoal, com acesso e retenção compatíveis.

| Camada | Registra a consulta? | O que fazer |
| --- | --- | --- |
| Cloud Run | Sim. Seus logs de requisição são criados automaticamente, e `httpRequest.requestUrl` inclui a consulta. | Exclua-os, como abaixo. |
| Load balancer do Google Cloud | Só se você ativar o log do serviço de backend. Ele vem desativado por padrão nos novos, e usa o mesmo campo. | Deixe-o desativado, ou exclua-os, como abaixo. |
| AWS Application Load Balancer | Só se você ativar os access logs, que vêm desativados por padrão. A linha da requisição guarda a URL como o cliente a enviou. Não há como remover parte de uma linha. | Deixe-os desativados, ou configure uma regra de ciclo de vida curta e acesso limitado. |
| CloudFront | Só se você ativar o logging padrão, no campo `cs-uri-query`. | Com o logging padrão v2, deixe esse campo de fora ao selecionar os campos. |
| ECS | Não tem log de requisição próprio: seus logs guardam o que o servidor escreve. | Nada. |
| Cloudflare | Só se você usar Logpush, no campo `ClientRequestURI`. | Selecione `ClientRequestPath` no lugar. |
| Caddy | Só com uma diretiva `log`, que `compose.caddy.yaml` não tem. | Se você adicionar uma, remova o parâmetro, como abaixo. |
| Outros proxies reversos | Geralmente sim: seus formatos padrão registram a linha de requisição inteira. | Registre o caminho sem a consulta para esta rota, ou desative o access log dela. |

No Google Cloud, isso mantém os dois logs fora do bucket `_Default` do projeto. Adicione a mesma exclusão a qualquer sink seu:

```sh
gcloud logging sinks update _Default \
  --add-exclusion='name=my-search-queries,filter=httpRequest.requestUrl:"/api/v1/statuses/search"'
```

No Caddy:

```caddyfile
social.example {
  log {
    format filter {
      request>uri query {
        delete q
      }
    }
  }
  reverse_proxy my:8080
}
```

<!-- source: backups cb3b40906192 -->
## Backups

O índice guarda o texto das publicações, então precisa do mesmo cuidado que o banco de dados: quem pode acessá-lo, onde ele fica guardado e por quanto tempo.

Um snapshot do mecanismo é a parte que merece atenção. Uma publicação excluída do índice permanece em todos os snapshots tirados antes, assim como nos backups do banco de dados; então dê aos snapshots a mesma retenção dos seus [backups do banco de dados](backup.md).

Você não precisa de snapshots para se recuperar: `my search rebuild` refaz o índice a partir do banco de dados. Depois de restaurar o banco de dados a partir de um backup, execute-o sem `--keep`, para que publicações mais novas que o backup saiam do índice.

<!-- source: your-privacy-documents 660de0e2a0c4 -->
## Seus documentos de privacidade

Se o mecanismo roda em um serviço de outra empresa, essa empresa recebe o texto das publicações cobertas por `search_visibilities`, incluindo as `local` se você as incluir. Cite-a como destinatária na [política de privacidade](privacy-policy-template.md) e no [registro de atividades de tratamento](processing-record-template.md). Um mecanismo que você mesmo executa não é um destinatário novo.

Diga também o que você decidiu sobre [os logs que guardam a consulta](#the-query-appears-in-urls), se você mantê-los.

<!-- source: closing-the-service 343ec69df255 -->
## Encerrando o serviço

O servidor não apaga dados por conta própria ao encerrar, então descarte o índice e os snapshots do mecanismo junto com o banco de dados e a mídia ([Encerrando o serviço](service-closure.md#7-discarding-the-db-blobs-and-backups)).
