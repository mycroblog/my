<!-- source: search-engines 6816d50f42dd -->
# Mecanismos de busca

Como executar cada mecanismo de busca que a busca de texto completo das publicações pode usar, qual o tamanho necessário e até onde ele encontra japonês e outros idiomas escritos sem espaços. Para um mecanismo que não está aqui, como o Groonga ou um índice construído com o Sudachi, [um plugin de busca](plugins.md#searching-posts) seu pode manter o índice. A [busca de texto completo das publicações](search.md) cobre o que o índice guarda, como ativar a busca, reconstruir o índice e trocar de mecanismo, seja qual for o mecanismo.

<!-- source: elasticsearch ed41ee8526d2 -->
## Elasticsearch

`search = elasticsearch` mantém o índice em um cluster Elasticsearch de versão 8 ou posterior, autogerenciado ou no Elastic Cloud Hosted. Ele foi verificado com o Elasticsearch 9.5 e 8.19. O servidor é dono dos campos do índice, nos quais escreve, filtra, ordena e pagina. Você pode ser dono das configurações do índice: a análise, os shards e as réplicas. Sem configurações próprias, as incluídas não precisam de plugin e encontram japonês como [descrito abaixo](#how-the-built-in-analysis-finds-japanese).

<!-- source: setting-it-up 3e02c35ff319 -->
### Configuração

1. Execute o Elasticsearch. Um único nó basta para a maioria dos servidores. Para um início rápido no mesmo host:

   ```sh
   docker run -d --name elasticsearch -p 127.0.0.1:9200:9200 \
     -e discovery.type=single-node -e ES_JAVA_OPTS="-Xms1g -Xmx1g" \
     -e ELASTIC_PASSWORD=change-me \
     -v esdata:/usr/share/elasticsearch/data \
     elasticsearch:9.5.3
   ```

   Com a segurança ativada, que é o padrão, o Elasticsearch serve HTTPS com um certificado de uma autoridade própria. Copie essa autoridade para fora do contêiner para `search_elasticsearch_ca_file`:

   ```sh
   docker cp elasticsearch:/usr/share/elasticsearch/config/certs/http_ca.crt /etc/my/elasticsearch-ca.crt
   ```

2. Crie uma chave de API que possa gerenciar apenas os índices do servidor. `manage`, `read` e `write` no alias e nos índices por trás dele é tudo de que ela precisa:

   ```sh
   curl --cacert /etc/my/elasticsearch-ca.crt -u elastic:change-me -X POST https://localhost:9200/_security/api_key \
     -H 'Content-Type: application/json' -d '{
       "name": "my",
       "role_descriptors": {"my": {"indices": [{"names": ["my-statuses*"], "privileges": ["manage", "read", "write"]}]}}
     }'
   ```

   O valor `encoded` da resposta é a chave.

3. Defina a configuração do servidor e reinicie-o:

   ```sh
   MY_SEARCH=elasticsearch
   MY_SEARCH_ELASTICSEARCH_URL=https://localhost:9200
   MY_SEARCH_ELASTICSEARCH_API_KEY=<the encoded value>
   MY_SEARCH_ELASTICSEARCH_CA_FILE=/etc/my/elasticsearch-ca.crt
   ```

4. Execute `my search rebuild` para adicionar as publicações existentes, como em [ativar a busca](search.md#turning-it-on).

As configurações:

| Configuração | O que faz |
| --- | --- |
| [`search_elasticsearch_url`](../reference/configuration.md#search_elasticsearch_url) | A URL da API REST. Use `https:` sempre que as requisições atravessarem uma rede compartilhada com outros; `http:` é aceito para uma rede privada ou um cluster com a segurança desativada. |
| [`search_elasticsearch_api_key`](../reference/configuration.md#search_elasticsearch_api_key) | A chave de API, que é a melhor escolha. |
| [`search_elasticsearch_username`](../reference/configuration.md#search_elasticsearch_username), [`search_elasticsearch_password`](../reference/configuration.md#search_elasticsearch_password) | Um usuário e uma senha para autenticação básica no lugar de uma chave. |
| [`search_elasticsearch_index`](../reference/configuration.md#search_elasticsearch_index) | O nome do alias, `my-statuses` por padrão. Dê a cada servidor que compartilha um cluster o seu próprio nome e conceda à chave dele esse nome seguido de `*`. |
| [`search_elasticsearch_index_settings_file`](../reference/configuration.md#search_elasticsearch_index_settings_file) | Um arquivo JSON com configurações próprias do índice, como em [ajustar a análise](#tuning-the-analysis-of-elasticsearch). |
| [`search_elasticsearch_ca_file`](../reference/configuration.md#search_elasticsearch_ca_file) | A autoridade certificadora de um cluster HTTPS cujo certificado o sistema não reconhece. |

As credenciais são lidas somente do ambiente, como segredos, e nunca aparecem nos logs ou nos erros do servidor. Um erro do cluster chega aos logs como o caminho da requisição, seu status e o tipo do erro, como `index_not_found_exception`, nunca com o texto de uma publicação ou de uma consulta. Somente quando o cluster se recusa a criar o índice o erro cita o motivo, que nomeia apenas as configurações e os campos.

<!-- source: the-index-and-its-upgrades 319d3f3a4bd2 -->
### O índice e suas atualizações

O servidor cria e mantém o índice por conta própria. Não o crie manualmente e não deixe mais nada escrever nele.

- Na inicialização, o servidor procura o alias. Quando não há nenhum, ele cria um índice com o nome do alias, a versão de seus campos e um hash de suas configurações, como `my-statuses-v1-1f0c2e9a7b3d4c5e`, e aponta o alias para ele. A versão e o hash ficam no `_meta` do índice.
- O servidor não consegue iniciar enquanto o cluster está inacessível, recusa a chave dele ou recusa as configurações. `my config check` verifica a configuração e se o arquivo de configurações é um único objeto JSON, não o que o Elasticsearch pensa dele, então verifique a conexão e as configurações iniciando o servidor.
- Um índice com o nome do alias que o servidor não criou, como um que o próprio Elasticsearch criou, interrompe a inicialização com um erro que o nomeia. Exclua esse índice ou escolha outro `search_elasticsearch_index`.
- Quando as configurações diferem daquelas com que o índice foi criado, porque você editou o arquivo de configurações ou uma nova versão do `my` mudou as incluídas, o servidor cria um novo índice vazio com as novas configurações, aponta o alias para ele, exclui o antigo e registra um aviso pedindo `my search rebuild`. Execute-o. Até a reconstrução terminar, a busca encontra apenas as publicações escritas desde então. Quando o Elasticsearch recusa as novas configurações, o servidor não inicia e o índice antigo permanece.
- Quando uma nova versão do `my` muda os campos, o servidor atualiza o índice na inicialização. Uma mudança que apenas adiciona campos é feita no próprio índice, e a busca continua funcionando; as notas de versão dizem quando as publicações que já estão no índice precisam de `my search rebuild --keep` para ganhar os novos campos. Qualquer outra mudança nos campos exige um novo índice vazio e uma reconstrução, como acima. As notas de versão dizem quando uma versão faz isso.
- Um índice criado por uma versão mais nova do `my` impede que uma mais antiga inicie. Para voltar atrás através de uma mudança assim, exclua o índice do alias no Elasticsearch (encontre-o com `GET _alias/my-statuses`), inicie a versão mais antiga, que cria o seu próprio, e execute `my search rebuild`.
- `my search clear` aponta o alias para um novo índice vazio e exclui o antigo em uma única etapa, o que é mais rápido que excluir cada documento e devolve o espaço em disco.

<!-- source: sizing 2b1d9b604953 -->
### Dimensionamento

O índice guarda apenas o que uma busca precisa e nenhuma cópia do texto (`_source` está desativado). Com as configurações incluídas, publicações curtas ocupam cerca de 80 a 150 bytes de disco cada, depois que o Elasticsearch mescla seus segmentos, e publicações mais longas com vocabulário mais amplo ocupam mais, então planeje de 150 a 300 bytes por publicação: algumas centenas de megabytes para um milhão de publicações. Verifique o tamanho real com `GET _cat/indices/my-statuses*?v`.

As configurações incluídas dão ao índice um shard primário, que comporta dezenas de milhões de publicações, e uma réplica quando o cluster tem um segundo nó (`auto_expand_replicas: 0-1`), de modo que um único nó permanece verde. Defina `number_of_shards` e `number_of_replicas` em um arquivo de configurações para alterá-los.

Um nó com 1 GB de heap Java (`ES_JAVA_OPTS=-Xms1g -Xmx1g`) e cerca de 2 GB de memória no total atende um servidor pequeno ou médio. Dê ao Elasticsearch metade da memória da máquina como heap e deixe o resto para o cache de arquivos, e dê mais quando as buscas ficarem lentas. As réplicas precisam de mais nós e do mesmo disco de novo para cada réplica.

<!-- source: how-a-query-uses-the-analysis 20b451dd32c6 -->
### Como uma consulta usa a análise

Toda publicação é indexada em um único campo de texto, seja qual for o idioma, e a mesma análise se aplica a todas. O idioma de uma publicação não escolhe outro analisador.

- O texto de uma publicação é analisado com o analisador chamado `my_text`.
- Uma consulta é dividida em palavras nos espaços, inclusive no espaço ideográfico. Cada palavra é analisada com `my_text_search` quando as configurações o definem, e com `my_text` caso contrário, e deve aparecer na publicação como uma frase: seus tokens na mesma ordem e um ao lado do outro. Toda palavra deve corresponder. Nada em uma consulta é lido como a linguagem de consulta do Elasticsearch.
- Os resultados são ordenados do mais recente para o mais antigo, não por relevância, então as opções de pontuação não fazem diferença.

Assim, um analisador decide o que é uma palavra. Um analisador que faz um token de cada caractere encontra qualquer sequência de caracteres. Um que faz palavras de dicionário encontra palavras inteiras e suas outras formas.

<!-- source: how-the-built-in-analysis-finds-japanese e260562ab16a -->
### Como a análise incluída encontra japonês

O `my_text` incluído é o tokenizador padrão do Elasticsearch com os filtros `cjk_width` e `lowercase`, todos incluídos. O tokenizador faz um token de cada kanji e de cada hiragana, de cada sequência de katakana e de cada palavra dos idiomas que usam espaços.

- Uma palavra de kanji ou hiragana é encontrada onde quer que seus caracteres estejam juntos, mesmo dentro de uma palavra mais longa: `検索` e `本` encontram `全文検索の本文です`, e `東京 天気` encontra `東京都の天気は晴れ`. Caracteres que só aparecem separados não correspondem.
- Uma sequência de katakana é comparada inteira: `カタカナ` encontra `カタカナとABC`, mas `カタ` não a encontra, e `ツリー` não encontra `スカイツリー`.
- Letras latinas de largura total e katakana de meia largura correspondem às suas formas usuais: `abc` encontra `ＡＢＣ`, e `カタカナ` encontra `ｶﾀｶﾅ`.
- O hiragana não encontra o kanji da mesma leitura: `ぜんぶん` não encontra `全文`. Outras formas de uma palavra não são encontradas: `走る` não encontra `走った`.
- Uma consulta de um único caractere muito comum, como `の`, encontra um grande número de publicações.

Chinês e coreano são encontrados da mesma forma, por seus caracteres. Palavras nos idiomas escritos com espaços são encontradas inteiras, sem lematização: `run` não encontra `running`.

<!-- source: tuning-the-analysis-of-elasticsearch fb5ddcfd414e -->
### Ajustar a análise do Elasticsearch

Coloque as configurações do índice em um arquivo JSON e nomeie-o em `search_elasticsearch_index_settings_file`. O servidor usa o arquivo inteiro como o `settings` de cada índice que cria, no lugar das incluídas, então ele deve conter tudo de que o índice precisa:

- O analisador `my_text`, que o campo de texto usa. Sem ele, o Elasticsearch se recusa a criar o índice e o servidor não inicia.
- Opcionalmente o analisador `my_text_search`, que então analisa as palavras das consultas.
- Qualquer outra configuração do índice, como `number_of_shards` e `number_of_replicas`. O que o arquivo omite assume os padrões do cluster.
- Somente análise que o Elasticsearch tem: um plugin que o arquivo nomeia deve estar instalado em todos os nós.

O servidor lê o arquivo na inicialização. Editá-lo dá à próxima inicialização um novo índice vazio, como em [o índice e suas atualizações](#the-index-and-its-upgrades), então execute `my search rebuild` depois de cada mudança. Teste uma mudança antes em outro cluster ou em outro `search_elasticsearch_index`.

Palavras de dicionário com o plugin `analysis-kuromoji`, que você instala em todos os nós (`bin/elasticsearch-plugin install analysis-kuromoji`) ou que o Elastic Cloud Hosted oferece como extensão:

```json
{
  "number_of_shards": 1,
  "number_of_replicas": 0,
  "analysis": {
    "analyzer": {
      "my_text": {
        "type": "custom",
        "tokenizer": "kuromoji_tokenizer",
        "filter": ["cjk_width", "lowercase", "kuromoji_baseform"]
      },
      "my_text_search": {
        "type": "custom",
        "tokenizer": "kuromoji_tokenizer",
        "filter": ["cjk_width", "lowercase", "kuromoji_baseform"]
      }
    }
  }
}
```

Toda sequência de um ou dois caracteres, apenas com a análise incluída no Elasticsearch:

```json
{
  "number_of_shards": 1,
  "number_of_replicas": 0,
  "max_ngram_diff": 1,
  "analysis": {
    "tokenizer": {
      "my_grams": { "type": "ngram", "min_gram": 1, "max_gram": 2, "token_chars": ["letter", "digit"] }
    },
    "analyzer": {
      "my_text": { "type": "custom", "tokenizer": "my_grams", "filter": ["cjk_width", "lowercase"] }
    }
  }
}
```

Como se comparam, medido em 20.000 publicações, metade em inglês e metade em japonês, no Elasticsearch 9.5:

| | Incluída | Kuromoji | Um e dois caracteres |
| --- | --- | --- | --- |
| Plugin | Nenhum | `analysis-kuromoji` | Nenhum |
| Tamanho do índice | 1,5 MB | 1,4 MB | 3,7 MB |
| Parte de uma palavra em kanji: `京都` em `東京都` | Encontrada | Não encontrada | Encontrada |
| Parte de uma palavra em katakana: `ツリー` em `スカイツリー` | Não encontrada | Encontrada | Encontrada |
| Parte de uma palavra em katakana: `コンピュータ` em `コンピューター` | Não encontrada | Não encontrada | Encontrada |
| Outra forma de uma palavra: `走る` encontra `走った` | Não encontrada | Encontrada | Não encontrada |
| Parte de uma palavra em inglês: `run` em `running` | Não encontrada | Não encontrada | Encontrada |

O Kuromoji divide o japonês nas palavras do seu dicionário e reduz cada uma à forma base, então encontra uma palavra em qualquer uma de suas formas, e uma palavra que o dicionário conhece dentro de um composto. Ele não encontra uma parte de uma palavra que o dicionário mantém inteira, e a forma como ele divide uma palavra nova ou um nome depende do dicionário.

Sequências de um e dois caracteres encontram qualquer parte de qualquer palavra, em todos os idiomas, inclusive partes que não são palavras: `run` também encontra `brunch`. O índice fica cerca de duas vezes e meia maior, e uma busca lê mais dele.

As buscas levaram alguns milissegundos com cada uma delas neste índice pequeno, e as diferenças crescem com o tamanho do índice.

<!-- source: opensearch fe2924a87307 -->
## OpenSearch

`search = opensearch` mantém o índice em um cluster OpenSearch que você executa, ou em um domínio do Amazon OpenSearch Service. O servidor conversa com a API REST do cluster. Ele é dono dos campos do índice e das consultas que envia, e você pode ser dono do resto: os analisadores, os shards, as réplicas e qualquer outra configuração do índice. Sem configurações próprias, ele usa analisadores incluídos no OpenSearch e não precisa de plugin.

<!-- source: setting-it-up-1 11097d66bccc -->
### Configuração

1. Execute o OpenSearch 2 ou 3. Um nó basta para começar; o servidor só alcança o endereço que você dá a ele.
2. Defina [`search`](../reference/configuration.md#search) como `opensearch` e [`search_opensearch_url`](../reference/configuration.md#search_opensearch_url) como a URL da API REST, como `https://search.internal.example.org:9200`. Um caminho é mantido, para um cluster atrás de um proxy reverso.
3. Dê ao servidor uma forma de entrar:
   - Com o plugin de segurança, um usuário com [`search_opensearch_username`](../reference/configuration.md#search_opensearch_username) e [`search_opensearch_password`](../reference/configuration.md#search_opensearch_password), enviados com autenticação HTTP Basic. Use `https` quando o cluster não estiver no mesmo host ou em uma rede privada em que você confie. O usuário precisa criar, ler o mapeamento e excluir os índices cujos nomes começam com [`search_opensearch_index`](../reference/configuration.md#search_opensearch_index), gerenciar esse alias e ler, escrever e excluir os documentos deles.
   - No Amazon OpenSearch Service com uma política de acesso do IAM, [`search_opensearch_aws_signing`](../reference/configuration.md#search_opensearch_aws_signing) definido como `es`, que assina cada requisição com o AWS Signature Version 4. As credenciais vêm da cadeia padrão do SDK da AWS (a função de execução, o ambiente, os arquivos compartilhados), e a região vem de [`search_opensearch_aws_region`](../reference/configuration.md#search_opensearch_aws_region) ou de `AWS_REGION`. A função precisa de `es:ESHttpGet`, `es:ESHttpPost`, `es:ESHttpPut` e `es:ESHttpDelete` no domínio. Um domínio com controle de acesso refinado e um usuário mestre interno usa o usuário e a senha no lugar.
   - O Amazon OpenSearch Serverless não é compatível: ele não tem exclusão por consulta, de que a remoção das publicações de um autor ou de uma visibilidade precisa.
4. Opcionalmente, [ajuste a análise](#tuning-the-analysis-of-opensearch) com um arquivo de configurações.
5. Inicie o servidor. Ele cria o índice na inicialização e se recusa a iniciar quando não consegue alcançar o cluster ou criar o índice.
6. Execute `my search rebuild` para indexar as publicações que já existem ([reconstruir o índice](search.md#rebuilding-the-index)).

A URL, o usuário, as credenciais da AWS e o conteúdo do arquivo de configurações nunca aparecem nos logs ou nos erros do servidor. Um erro do mecanismo chega ao log apenas como a operação, o status HTTP e o tipo do erro do mecanismo, já que a mensagem do próprio mecanismo pode citar uma publicação ou uma consulta. A única exceção é uma criação de índice recusada, cuja mensagem nomeia a configuração com problema.

<!-- source: the-index-and-its-upgrades-1 5529569168b1 -->
### O índice e suas atualizações

[`search_opensearch_index`](../reference/configuration.md#search_opensearch_index), `my-statuses` por padrão, nomeia um alias. Os documentos ficam em um índice que o servidor cria e para o qual aponta o alias, com o nome do alias mais a versão do mapeamento do servidor e um hash das configurações do índice, como `my-statuses-v1-3f2a9c41b07e`. O mapeamento registra os mesmos dois em seu `_meta` (`my_mapping_version` e `my_settings_hash`). Toda escrita passa pelo alias e falha enquanto ele não existe, de modo que o OpenSearch nunca cria um índice próprio com um mapeamento presumido. Não crie você mesmo um índice com esse nome: o servidor se recusa a iniciar quando o nome é um índice em vez de um alias, ou quando o alias aponta para um índice que ele não criou.

Na inicialização, o servidor compara o índice por trás do alias com a própria versão de mapeamento e as próprias configurações. Quando qualquer um dos dois difere (uma versão cujo mapeamento mudou, o que as notas de versão dizem, ou um arquivo de configurações editado), ele cria um novo índice e move o alias para ele. O novo índice começa vazio, então:

1. Execute `my search rebuild`. Até ele terminar, a busca funciona, mas encontra apenas o que foi publicado ou reconstruído desde a mudança.
2. Exclua o índice antigo (`DELETE /my-statuses-v1-…`) quando a reconstrução terminar. O servidor o deixa onde está e registra um aviso que o nomeia.

Inicie todos os processos do servidor com o mesmo arquivo de configurações: processos com arquivos diferentes movem o alias de um lado para o outro. Um servidor de uma versão mais antiga que inicia contra um índice de um mapeamento mais novo usa esse índice como está.

`my search clear`, e `my search rebuild` sem `--keep`, que começa com ele, criam um novo índice vazio com o mesmo mapeamento e as mesmas configurações, movem o alias para ele e excluem o antigo em uma única requisição.

<!-- source: sizing-1 4ad4bbcd9a2b -->
### Dimensionamento

- **Memória**: o nó precisa de um heap da JVM de pelo menos 512 MB (`-Xms512m -Xmx512m`) e outro tanto para o sistema operacional; dê 1 GB para um servidor pequeno e mais conforme o índice cresce. O melhor é manter o heap em metade da memória do nó.
- **Disco**: o índice ocupa aproximadamente o tamanho do texto das publicações indexadas com as configurações incluídas, mais para idiomas escritos sem espaços, cujo texto é indexado como caracteres isolados e pares, e mais ainda com n-gramas. Mantenha pelo menos 15% do disco livre: por padrão, o OpenSearch para de alocar shards em um nó acima de 85% e bloqueia as escritas em seus índices acima de 95%.
- **Shards e réplicas**: sem um arquivo de configurações, o índice tem os padrões do cluster, um shard e uma réplica, a menos que você os tenha mudado. Um shard atende milhões de publicações. Uma réplica deixa um cluster de nó único amarelo; funciona, e `"index": {"number_of_replicas": 0}` em um arquivo de configurações o deixa verde. Em um cluster de vários nós, mantenha uma réplica. O número de réplicas também pode ser alterado no índice em operação (`PUT /my-statuses/_settings`), o que não precisa de reconstrução; o servidor continua usando o índice enquanto o arquivo de configurações não mudar.
- **Carga**: toda busca é uma consulta ao índice, das mais recentes para as mais antigas por ID de publicação. As escritas são uma requisição por publicação, e uma reconstrução não escreve mais rápido que [a sua taxa](search.md#rebuilding-the-index).

<!-- source: japanese-and-other-languages a48cff99706b -->
### Japonês e outros idiomas

Com as configurações incluídas, toda publicação, seja qual for o idioma, é analisada da mesma forma: o texto é dividido em palavras nos espaços e na pontuação, convertido para minúsculas, e letras latinas de largura total e katakana de meia largura são normalizados para suas formas usuais. O texto em chinês, japonês e coreano, que não tem espaços, é dividido em caracteres, e cada par de caracteres vizinhos é indexado além de cada caractere (o filtro incluído `cjk_bigram` do OpenSearch).

- Uma consulta encontra as publicações que contêm todas as suas palavras. Em japonês, uma palavra de dois ou mais caracteres encontra as publicações que contêm cada par de seus caracteres: `東京` encontra `東京都の天気`, e `全文検索` encontra `全文検索の本文です`. Um único caractere, como `本`, encontra toda publicação com esse caractere.
- Os pares não sabem onde as palavras terminam, então uma consulta pode encontrar uma publicação em que seus caracteres apenas aparecem lado a lado: `京都` encontra `東京都` também. Uma consulta de katakana escrita em letras de meia largura ou de largura total encontra ambas.
- As leituras não são comparadas: `ぜんぶん` não encontra `全文`, e as formas flexionadas não são associadas às suas formas de dicionário, em japonês ou em inglês (`running` não encontra `run`).

Para encontrar japonês por palavras, use um analisador morfológico como o kuromoji em um [arquivo de configurações](#tuning-the-analysis-of-opensearch).

<!-- source: tuning-the-analysis-of-opensearch 1289083a07ec -->
### Ajustar a análise do OpenSearch

[`search_opensearch_index_settings_file`](../reference/configuration.md#search_opensearch_index_settings_file) nomeia um arquivo JSON cujo objeto se torna o `settings` de todo índice que o servidor cria: `analysis`, `index.number_of_shards`, `index.number_of_replicas` e qualquer outra configuração do índice. Ele contém apenas configurações; o servidor é dono do mapeamento e recusa um arquivo com `mappings`.

O arquivo deve definir dois analisadores, que o mapeamento nomeia:

| Analisador | Analisa |
| --- | --- |
| `my_text` | O texto de cada publicação: o corpo, o aviso de conteúdo e os textos alternativos de suas mídias |
| `my_text_search` | O texto de cada consulta |

Eles podem ter a mesma definição. Cada consulta é uma consulta `match` do campo `text` com `"operator": "and"`: o texto da consulta passa por `my_text_search`, e uma publicação é encontrada quando seu texto contém todos os termos que saem, das mais recentes para as mais antigas por ID de publicação, sem pontuação. Uma consulta cujo analisador não deixa nenhum termo não encontra nada. Correspondência de frase, aproximada e por prefixo não são usadas, então um analisador decide sozinho o que é uma correspondência. O idioma de uma publicação é armazenado, mas não escolhe um analisador: toda publicação passa por `my_text`.

Quando o servidor inicia com um arquivo cujas configurações diferem das do índice, mesmo por uma réplica, ele cria um novo índice vazio, e o índice precisa de uma reconstrução ([o índice e suas atualizações](#the-index-and-its-upgrades)). A formatação e a ordem das chaves não contam. Um arquivo que o OpenSearch recusa, como um sem os dois analisadores ou com um plugin que o cluster não tem, interrompe a inicialização com o motivo do OpenSearch.

O Kuromoji divide o japonês em palavras com um dicionário e associa as formas flexionadas às suas formas de dicionário. O Amazon OpenSearch Service tem o plugin em todo domínio; um cluster que você executa precisa dele instalado (`opensearch-plugin install analysis-kuromoji`, por exemplo em um Dockerfile `FROM opensearchproject/opensearch`). Um arquivo com kuromoji:

```json
{
  "index": {
    "number_of_shards": 1,
    "number_of_replicas": 0
  },
  "analysis": {
    "analyzer": {
      "my_text": {
        "type": "custom",
        "tokenizer": "kuromoji_tokenizer",
        "filter": ["kuromoji_baseform", "kuromoji_part_of_speech", "cjk_width", "kuromoji_stemmer", "lowercase"]
      },
      "my_text_search": {
        "type": "custom",
        "tokenizer": "kuromoji_tokenizer",
        "filter": ["kuromoji_baseform", "kuromoji_part_of_speech", "cjk_width", "kuromoji_stemmer", "lowercase"]
      }
    }
  }
}
```

Uma palavra que o dicionário divide de forma diferente da consulta não é encontrada, então o kuromoji pode perder o que os pares incluídos encontram, como uma parte de um substantivo composto ou uma palavra nova. Um tokenizador de n-gramas, incluído no OpenSearch, encontra qualquer sequência de caracteres ao custo de um índice maior e de correspondências dentro de palavras mais longas; com `"min_gram": 2` e `"max_gram": 3` (e `"index": {"max_ngram_diff": 1}`), uma consulta de um único caractere não encontra nada.

<!-- source: meilisearch 51ac396ba9d0 -->
## Meilisearch

`search = meilisearch` mantém o índice em um servidor [Meilisearch](https://www.meilisearch.com/docs), autohospedado ou no Meilisearch Cloud. É um único processo, sem cluster para executar, o que serve a um servidor pequeno, e a imagem oficial segmenta japonês, chinês e coreano sem plugin. O servidor foi verificado com o Meilisearch 1.56.

<!-- source: running-meilisearch 4f7d5e17c255 -->
### Executar o Meilisearch

Execute a imagem oficial com uma chave mestra de pelo menos 16 bytes, em uma rede que apenas o `my` alcance, e mantenha o diretório de dados em um volume persistente:

```sh
docker run -d --name meilisearch --restart unless-stopped \
  -p 127.0.0.1:7700:7700 \
  -e MEILI_ENV=production -e MEILI_NO_ANALYTICS=true \
  -e MEILI_MASTER_KEY="$(openssl rand -base64 32)" \
  -v meili_data:/meili_data \
  getmeili/meilisearch:v1.56.1
```

`MEILI_ENV=production` se recusa a iniciar sem uma chave mestra e desativa a prévia de busca incluída. O Meilisearch fala HTTP simples: coloque-o atrás de TLS, ou mantenha-o em uma rede privada, quando o `my` o alcançar por qualquer coisa que não seja um link local ou privado.

<!-- source: settings 74547d71e962 -->
### Configurações

```toml
search = "meilisearch"
search_meilisearch_url = "http://meilisearch:7700"
# search_meilisearch_index = "my_statuses"
# search_meilisearch_settings_file = "/etc/my/meilisearch-settings.json"
```

Defina a chave pelo ambiente (`MY_SEARCH_MEILISEARCH_API_KEY` ou `MY_SEARCH_MEILISEARCH_API_KEY_FILE`), nunca no arquivo de configuração. Pode ser a chave mestra, mas uma chave que faz apenas o que o `my` precisa é mais segura. Crie uma com a chave mestra:

```sh
curl -X POST http://meilisearch:7700/keys \
  -H "Authorization: Bearer $MEILI_MASTER_KEY" -H 'Content-Type: application/json' \
  --data '{"description": "my", "indexes": ["my_statuses"],
    "actions": ["search", "documents.add", "documents.get", "documents.delete", "indexes.create", "indexes.get",
      "indexes.delete", "settings.update", "tasks.get"],
    "expiresAt": null}'
```

Use o `key` da resposta como `search_meilisearch_api_key`. A chave é enviada apenas no cabeçalho `Authorization`: os logs e os erros do servidor mostram o código de erro do Meilisearch (como `invalid_api_key`) e nunca a chave, uma publicação ou uma consulta. Todas [as configurações](../reference/configuration.md#search_meilisearch_url) estão na referência de configuração.

Vários servidores podem compartilhar um servidor Meilisearch com um `search_meilisearch_index` diferente cada, e uma chave limitada ao próprio índice.

<!-- source: the-index-and-its-settings 2657dc55a7bc -->
### O índice e suas configurações

O servidor cria o índice quando ele não existe. Na primeira vez que usa o índice depois de uma inicialização, ele compara as configurações que enviaria com as que aplicou por último, que registra como um hash em um documento do índice que nenhuma busca encontra, e só as envia quando diferem: uma reinicialização sem nada alterado não envia nada. As configurações são:

- **O que o próprio servidor define:** `searchableAttributes` (os campos de texto das publicações), `displayedAttributes` (todos), `filterableAttributes` (o ID da publicação, o autor e a visibilidade) e `sortableAttributes` (o ID da publicação). A busca depende delas, e um arquivo de configurações que define uma delas é recusado.
- **O que você pode ajustar:** todo o resto, no [arquivo de configurações](#tuning-meilisearch). Sem um, ou para o que o arquivo omite, o servidor usa seus padrões: `rankingRules` de `sort`, `words`, `typo`, `proximity`, `attribute` e `exactness`; sem tolerância a erros de digitação; os idiomas dos campos de japonês, chinês e coreano em `localizedAttributes`; e os próprios padrões do Meilisearch para o resto (`stopWords`, `synonyms`, `dictionary`, `separatorTokens`, `nonSeparatorTokens`, `proximityPrecision`, `pagination`, `faceting`, `distinctAttribute`, `searchCutoffMs`, `prefixSearch`). As configurações que você altera manualmente no mecanismo permanecem apenas até o servidor enviar suas configurações de novo.

**Uma mudança nas configurações não precisa de `my search rebuild`.** O Meilisearch reindexa por conta própria os documentos que guarda, em segundo plano, e as publicações permanecem no índice. Isso só leva tempo, aproximadamente o mesmo que indexá-los levou. A inicialização do servidor não espera por isso: o servidor envia as configurações e segue em frente. Enquanto isso, as buscas respondem com as configurações anteriores, e as escritas do índice esperam atrás da reindexação, já que o Meilisearch executa as tarefas de um índice em ordem; um job cuja escrita não termina em 10 segundos tenta de novo mais tarde, então as publicações escritas durante a reindexação chegam ao índice depois dela. `my search clear` espera pelas configurações do novo índice vazio, que não levam tempo.

Uma reconstrução só é necessária quando o índice começa vazio:

- Você muda `search_meilisearch_index`.
- Você inicia um servidor Meilisearch novo e vazio, por exemplo em vez de atualizar o antigo.
- As notas de versão de uma versão do `my` dizem que ela escreve os documentos de forma diferente.

As escritas são tarefas que o Meilisearch executa em ordem. O servidor espera pela tarefa de cada escrita, e uma tarefa que falha ou não termina no tempo do job faz o job tentar de novo, então um Meilisearch fora do ar ou ocupado atrasa o índice sem perder uma mudança. Uma tarefa de configurações que falha, por exemplo por um valor do arquivo de configurações que o Meilisearch recusa, faz as escritas falharem até ela ter sucesso; `GET /tasks?types=settingsUpdate` no mecanismo mostra o erro dela. `my search clear` exclui o índice e o cria de novo com as configurações.

Para atualizar o próprio Meilisearch, siga o guia de atualização dele (um dump, ou a atualização sem dump), ou inicie a nova versão vazia e execute `my search rebuild`.

<!-- source: what-a-search-sends 575b89133606 -->
### O que uma busca envia

A consulta de uma busca é fixa, seja qual for o conteúdo do arquivo de configurações: `matchingStrategy: all`, para que uma publicação tenha todas as palavras; `sort: status_id:desc`, das mais recentes para as mais antigas; um `filter` no ID da publicação para a próxima página; um `limit` da página mais um; `attributesToRetrieve` do ID da publicação; e `showMatchesPosition`, que identifica uma consulta sem nenhuma palavra. As aspas duplas de uma frase e o sinal de menos antes de uma palavra, que o Meilisearch leria como operadores, são descartados, então uma consulta é sempre de palavras simples.

<!-- source: tuning-meilisearch df827448e06c -->
### Ajustar o Meilisearch

Ajuste o mecanismo com um arquivo JSON de [configurações de índice do Meilisearch](https://www.meilisearch.com/docs/reference/api/settings), nomeado por `search_meilisearch_settings_file`. O servidor o verifica na inicialização, quando `search = meilisearch`: ele deve existir e ser um objeto JSON, não deve definir as quatro configurações que o próprio servidor define, e seus `rankingRules`, quando os tem, devem começar com `sort`, o que mantém os resultados das mais recentes para as mais antigas. O Meilisearch verifica os valores quando o servidor os envia. O servidor nunca registra o conteúdo do arquivo no log.

Um exemplo que restringe os idiomas ao japonês, trata `・` como separador, torna a proximidade mais barata de indexar e ativa a tolerância a erros de digitação para palavras longas:

```json
{
  "localizedAttributes": [
    {"attributePatterns": ["text_ja"], "locales": ["jpn"]}
  ],
  "separatorTokens": ["・"],
  "proximityPrecision": "byAttribute",
  "typoTolerance": {"enabled": true, "minWordSizeForTypos": {"oneTypo": 8, "twoTypos": 12}},
  "synonyms": {"ご飯": ["ごはん"], "ごはん": ["ご飯"]}
}
```

O que uma mudança custa:

- Toda mudança no arquivo, na próxima inicialização, faz o Meilisearch reindexar todos os documentos, como acima: nenhum dado é perdido e nada precisa de reconstrução, mas as escritas esperam até terminar. Faça a mudança em um horário calmo e acompanhe a tarefa com `GET /tasks?types=settingsUpdate`.
- `localizedAttributes` e os tokens decidem como o texto é dividido em palavras: uma lista de idiomas mais estreita pode segmentar uma publicação em outro idioma como se fosse o errado (veja abaixo), então mantenha os campos de japonês, chinês e coreano, a menos que o servidor não tenha nenhuma publicação nesses idiomas.
- A tolerância a erros de digitação e os sinônimos fazem uma busca encontrar mais que as palavras exatas. As regras de ranking depois de `sort` não mudam nada, já que duas publicações não compartilham um ID; omiti-las apenas poupa um pouco de trabalho ao Meilisearch.

<!-- source: sizing-2 1d581fac5316 -->
### Dimensionamento

O Meilisearch mantém o índice em arquivos mapeados em memória no disco: ele funciona com menos memória que o índice e fica mais rápido quanto mais do índice cabe na memória.

- **Memória.** 1 GB é um começo razoável para um servidor pequeno; dê mais conforme o índice cresce, idealmente tanto quanto o tamanho do índice em disco. A indexação usa por padrão até dois terços da memória da máquina; em uma máquina compartilhada com outros serviços, limite-a com `MEILI_MAX_INDEXING_MEMORY` (como `512Mb`).
- **Disco.** O índice é várias vezes maior que o texto que guarda, porque mantém as listas de palavras de todos os campos. Meça-o depois da primeira reconstrução com `GET /stats` (`databaseSize`) e mantenha espaço livre bem acima dele: a reindexação e as atualizações precisam de espaço para trabalhar.
- **CPU.** Um ou dois núcleos bastam para um servidor pequeno. Uma reconstrução é a carga mais pesada; deixe-a mais lenta com `my search rebuild --rate`.

<!-- source: japanese-and-other-languages-1 ad5184139aeb -->
### Japonês e outros idiomas

O servidor coloca o texto de uma publicação no campo do idioma dela: japonês (`ja`, inclusive tags como `ja-JP`), chinês (`zh`), coreano (`ko`), ou um campo para todos os outros idiomas e para nenhum idioma. Ele informa ao Meilisearch o idioma dos três primeiros, e o Meilisearch segmenta cada um em palavras com um dicionário:

- **Japonês** é segmentado pelo Lindera com o dicionário UniDic, seja o texto com kana ou apenas kanji. `検索`, `全文検索` e `全文　検索` encontram todos `全文検索の本文です`, e `東京` encontra `東京都庁舎見学`.
- **Chinês** é segmentado pelo jieba, e **coreano** pelo Lindera com o dicionário ko-dic.
- **Outros idiomas** são divididos nos espaços e na pontuação; o Meilisearch detecta por conta própria a escrita e o idioma do texto.

Uma consulta é dividida em palavras da mesma forma, e uma publicação é encontrada quando tem todas as palavras da consulta, a última também como início de uma palavra mais longa (`テキ` encontra `テキスト`). Apenas as primeiras 10 palavras de uma consulta contam.

O que a busca não consegue encontrar:

- **Uma sequência que não é uma palavra da segmentação do dicionário.** A busca encontra palavras, não subsequências: `京都` não encontra `東京都`, e uma parte que o dicionário mantém dentro de uma palavra mais longa, como `庁舎` em `東京都庁舎見学`, também não é encontrada.
- **Leituras.** O hiragana encontra a mesma palavra em katakana (`かたかな` encontra `カタカナ`), mas uma leitura não encontra kanji (`ぜんぶん` não encontra `全文`).
- **Grafias próximas.** A tolerância a erros de digitação está desativada, então toda palavra deve ser exata.
- **Publicações em japonês sem idioma.** Uma publicação cujo idioma está ausente ou errado é segmentada pelo que o Meilisearch detecta, e um texto apenas em kanji é então muitas vezes tomado por chinês e dividido de forma diferente: tal publicação pode ser encontrada por `京都` para `東京都`, ou não ser encontrada por `東京`. Uma publicação leva o idioma que o autor escolheu, ou o que o servidor do autor enviou.

O servidor normaliza as formas de compatibilidade do Unicode nas publicações e nas consultas antes que cheguem ao Meilisearch, então katakana de meia largura (`ｶﾀｶﾅ`) e letras e dígitos de largura total (`ＡＢＣ`) encontram e são encontrados por suas formas usuais. Emoji também são palavras: `👍` encontra as publicações que o têm.

A busca não precisa de plugin, dicionário ou imagem própria para nada disso: a imagem oficial tem os dicionários de japonês, chinês e coreano.
