<!-- source: plugins 4a60f7abbec9 -->
# Plugins

Um plugin é um programa ou serviço separado que você executa e ao qual o servidor entrega uma tarefa específica pela rede.
O servidor apenas se conecta a um endereço: ele não inicia, não para nem monitora o plugin. Execute-o como sidecar, sob o systemd
ou em outra máquina.

Nada é delegado por padrão. Um servidor sem endereço de plugin não abre nenhuma conexão e não inicia nenhum processo.
A build tag [`no_plugins`](../reference/build-tags.md) tira o cliente de plugins do binário.

![Onde ficam os plugins: o processo my entrega seis portas a um plugin e consulta cinco hooks de decisão, tudo por um único cliente de plugins que se conecta aos plugins que você executa; a parte de baixo do diagrama mostra o que cada um faz quando uma chamada falha. A lista abaixo descreve cada parte.](images/plugins.svg)

Do topo do diagrama para baixo:

- **O processo my**, com qualquer `role`, faz dois tipos de chamadas a um plugin.
  - **Portas que ele entrega**: um plugin faz uma tarefa no lugar de uma parte embutida, e cada uma só está ativa quando uma configuração indica
    `plugin`: `translator` (a configuração da instância `translation_providers = plugin`), `billing` (`billing_sources` inclui
    `plugin`), `mailer` (`mail = plugin`), `media_image`, `media_video` e `media_audio` (`media_image = plugin` e assim
    por diante), `key_manager` (`key_manager = plugin`) e `search` (`search = plugin`).
  - **[Hooks de decisão](decision-hooks.md)**: um plugin responde se algo deve ser aceito, e é consultado a menos que o
    modo `hook_<hook>_mode` do hook seja `off`: `inbound.status` para publicações de outros servidores, `inbound.follow` para follows de
    outros servidores, `signup.check` para cadastros, `media.check` para envios, `local.status` para publicações de contas locais,
    `inbound.actor` para os perfis de contas de outros servidores e `local.profile` para alterações de perfil de contas
    locais.
- **O cliente de plugins** faz todas as chamadas. Cada tipo tem um destino, `plugin_<kind>_address`, e um prazo,
  `plugin_<kind>_timeout`, e executa no máximo 8 chamadas ao mesmo tempo (2 para cada tipo de mídia). Ele se conecta por um socket Unix, `https://` ou,
  apenas para [alguns tipos](#plain-tcp), `http://`.
- **Seus plugins** são programas separados que você executa, como sidecar, sob o systemd ou em outra máquina. O servidor apenas
  se conecta a eles.
- **Quando uma chamada falha ou expira** ([detalhes](#when-a-call-fails)):
  - `billing`, `mailer` e os tipos de mídia: o job que fez a chamada roda de novo mais tarde.
  - `translator`: o servidor consulta o próximo provedor, e quando não resta nenhum o leitor vê que a tradução falhou.
  - `key_manager` falha de forma fechada: tudo o que precisa de uma chave falha, e o servidor não inicia sem o plugin.
  - `search`: o job de uma gravação roda de novo mais tarde, e uma busca falha, de modo que a pessoa vê que a busca está indisponível.
  - Um hook de decisão faz o que `hook_<hook>_on_failure` diz: `pass`, `retry` ou, para cadastros, `require_approval`.

<!-- source: kinds 73da92ef7563 -->
## Tipos

Cada tipo tem seu próprio destino, nas configurações `plugin_<kind>_*`. Um ponto no nome de um tipo vira `_` na
chave, então `inbound.status` tem `plugin_inbound_status_address`.

| Tipo | O que ele transporta | TCP simples (`http://`) | Protocolo padrão | Prazo padrão |
| --- | --- | --- | --- | --- |
| `translator` | O texto de uma publicação que um leitor pediu para traduzir | **Recusado** | `json` | 10 s |
| `search` | O texto, o autor, a visibilidade e o idioma das publicações `public` e `local`, e as consultas de pessoas conectadas | **Recusado** | `json` | 10 s |
| `billing` | Os custos da instância | Permitido | `json` | 2 min |
| `mailer` | Endereços e segredos, como links de confirmação | **Recusado** | `json` | 30 s |
| `media_image` | Imagens enviadas, com seus metadados, que podem conter uma localização | Permitido | `proto` | 4 min |
| `media_video` | Vídeos enviados, com seus metadados, que podem conter uma localização | Permitido | `proto` | 4 min |
| `media_audio` | Arquivos de áudio enviados, com seus metadados | Permitido | `proto` | 4 min |
| `key_manager` | Chaves de criptografia de dados em texto simples | **Recusado** | `json` | 5 s |
| `inbound.status` | O texto das publicações recebidas | **Recusado** | `json` | 3 s |
| `local.status` | O texto das publicações locais | **Recusado** | `json` | 1 s |
| `local.profile` | O perfil público das contas locais, sem imagens | Permitido | `json` | 1 s |
| `inbound.follow` | As duas partes de um follow | Permitido | `json` | 3 s |
| `inbound.actor` | Os perfis públicos de contas de outros servidores | Permitido | `json` | 3 s |
| `signup.check` | Endereços de e-mail e endereços IP | **Recusado** | `json` | 2 s |
| `media.check` | Uma pequena versão processada da mídia, nunca o original | Permitido | `json` | 10 s |

As configurações que fazem o servidor usar um destino pertencem a cada recurso: os cinco últimos tipos da tabela são os
[hooks de decisão](decision-hooks.md). Esta página cobre o que eles têm em comum.

O servidor envia no máximo o limite fixo de um tipo por mensagem e lê no máximo outro. Uma chamada acima do limite falha, e
o job que a fez tenta de novo.

<!-- source: the-destination f397f6e80550 -->
## O destino

As configurações existem apenas no arquivo de configuração e no ambiente
([referência de configuração](../reference/configuration.md)). Nenhuma configuração da instância, página de administração ou API pode mostrá-las
nem alterá-las, de modo que uma conta de administrador roubada não pode redirecionar publicações ou chaves. Uma alteração passa a valer ao reiniciar.

`plugin_<kind>_address` tem uma de três formas. O servidor verifica a forma ao iniciar sem resolver o nome, e
resolve o nome a cada conexão.

- `unix:///run/my/mailer.sock`: um socket Unix.
- `https://mailer.internal:8443`: TLS sobre TCP. O servidor sempre verifica o certificado, com
  `plugin_<kind>_tls_ca_file` ou as raízes do sistema, e nenhuma configuração desativa isso.
- `http://billing.internal:5000`: TCP simples.

<!-- source: authentication 2fe4df9369ac -->
### Autenticação

Escolha uma:

- **Um socket Unix:** as permissões do arquivo do socket o protegem.
- **mTLS:** `plugin_<kind>_tls_cert_file` e `plugin_<kind>_tls_key_file`, com um endereço `https://`.
- **Um bearer token:** `plugin_<kind>_token` (ou `MY_PLUGIN_<KIND>_TOKEN_FILE`), enviado como `Authorization: Bearer`, apenas
  por `https://` ou por um socket Unix. Um token com um endereço `http://` é um erro de configuração.

Um endereço `https://` sem token nem certificado de cliente inicia com um aviso, já que o plugin não consegue distinguir
o servidor de qualquer outra pessoa que o alcance.

O servidor lê os arquivos de `plugin_<kind>_tls_ca_file`, `plugin_<kind>_tls_cert_file` e
`plugin_<kind>_tls_key_file` ao iniciar, e um arquivo ausente ou ilegível o impede de iniciar. Depois disso, ele os verifica de novo
em busca de mudanças no máximo a cada 5 segundos, quando abre uma nova conexão, de modo que um certificado de curta duração ou uma CA que
um gerenciador de certificados como Vault, cert-manager ou SPIFFE renova no lugar funciona sem reiniciar. As conexões já
abertas mantêm o certificado com que começaram até serem fechadas. Enquanto uma renovação está pela metade, como um novo
certificado cuja chave ainda não está no lugar, o servidor mantém os arquivos que leu por último e tenta de novo na próxima
conexão. O token é lido apenas ao iniciar.

<!-- source: plain-tcp cbb88078173e -->
### TCP simples

`mailer`, `key_manager`, `translator`, `search`, `inbound.status`, `local.status` e `signup.check` recusam `http://`: o servidor não inicia. Isso vale também para
o loopback (`http://127.0.0.1`), já que um endereço de loopback não impede que outros processos do host leiam o
tráfego. Coloque esse sidecar em um socket Unix em um volume compartilhado.

Os outros tipos aceitam `http://`, e o servidor registra um aviso ao iniciar que nomeia o tipo e o endereço. Nada
criptografa nem autentica esse tráfego, e nenhum bearer token pode ser usado nele. Manter essa rede fechada é de sua
responsabilidade: o que a tabela acima lista atravessa-a em texto simples.

<!-- source: protocols eb5a03718f1b -->
## Protocolos

`plugin_<kind>_protocol` diz o que o servidor envia, e deve ser o que o plugin aceita.

| Valor | O que o servidor envia | Para |
| --- | --- | --- |
| `json` | O protocolo unário do Connect com `application/json`, sobre HTTP/1.1 em conexões simples | Um plugin construído com uma biblioteca Connect, ou apenas com um servidor HTTP e uma biblioteca JSON |
| `proto` | O protocolo do Connect com `application/proto` | Um plugin construído com uma biblioteca Connect |
| `grpc` | gRPC sobre HTTP/2 | Um plugin que fala apenas gRPC |

Um plugin responde ao protocolo errado com `415`. Os tipos de mídia transmitem nos dois sentidos, o que exige HTTP/2, então aceitam `proto`
ou `grpc`, e `json` é um erro de configuração.

As definições do protocolo são os arquivos `.proto` em `api/plugin/` do repositório. Há um pacote para cada
tipo de porta, e eles não compartilham tipos; os hooks de decisão compartilham todos um único pacote, `my.plugin.hook.v1`.
[O protocolo de plugins](../plugins/overview.md) tem a referência de cada pacote, gerada a partir desses arquivos.

<!-- source: when-a-call-fails 1caa41ab429e -->
## Quando uma chamada falha

Uma chamada espera sua vez entre as chamadas do seu tipo que rodam ao mesmo tempo, e a espera conta no seu prazo e na
métrica [`my.plugin.slot_wait`](running.md). Uma chamada que falha conta na métrica [`my.plugin.failures`](running.md) por tipo e código Connect. O servidor descarta a
mensagem que um plugin coloca em um erro, então os motivos das falhas de um plugin ficam no log do próprio plugin. O servidor
não tenta a chamada de novo por conta própria: quem o faz é o job que a fez, então um plugin precisa lidar com a mesma chamada duas vezes.

Um plugin fora do ar quando o servidor inicia não impede nada, exceto `key_manager`: o servidor não inicia sem ele, e
[o que para sem ele](key-management.md#when-the-plugin-is-unreachable) é uma parte da configuração que vale ler primeiro.

Uma chamada tem o prazo de `plugin_<kind>_timeout`, e um plugin o recebe como `Connect-Timeout-Ms` (`grpc-timeout` no
gRPC). Um plugin pode ignorar esse cabeçalho: o servidor para de esperar de qualquer modo. O prazo deve ficar abaixo de 30 segundos para os
tipos que uma requisição pode ficar esperando (`key_manager`, `translator`, `search`, `signup.check`, `media.check`, `local.status`, `inbound.actor` e `local.profile`), e
abaixo de 5 minutos, quanto um job pode durar, para os demais.

<!-- source: test-a-plugin 5d8195bb27f5 -->
## Testar um plugin

`my plugin test` executa o contrato de um tipo contra um plugin em execução e informa cada caso. Ele não lê nenhuma configuração,
então tudo de que precisa é o binário `my`.

```sh
my plugin test --kind probe unix:///run/my/plugin.sock
my plugin test --kind probe --protocol grpc --tls-ca-file ca.pem --token-file token https://plugin.internal:8443
```

```text
PASS reachable
PASS echo
FAIL echo_empty: the outcome is OUTCOME_UNSPECIFIED, not OUTCOME_OK
2 passed, 1 failed, 0 skipped
```

O código de saída é 1 quando um caso falha. As flags significam o mesmo que as configurações: `--protocol`, `--token-file`, `--tls-ca-file`,
`--tls-cert-file`, `--tls-key-file` e `--timeout`. Ele aplica as mesmas regras do servidor, então `--kind mailer` recusa um
endereço `http://`. O plugin testado pode ser um real, então um caso que faz mais do que a própria chamada só roda quando
uma flag o pede. Um tipo cujo contrato a versão ainda não tem informa `reachable` e um `SKIP`.

`--kind billing` verifica apenas que o plugin se recusa a gastar qualquer coisa quando nenhuma requisição cobrada é permitida. `--billed` também
faz uma busca real do mês atual e do anterior, e verifica a resposta como uma importação o faz; isso pode custar dinheiro nos
provedores por trás do plugin. [Importar os custos de uso da nuvem](billing-import.md#a-plugin-billing_sources--plugin) tem o
contrato do tipo.

`--kind mailer` não faz nenhuma chamada do RPC a menos que `--mail-to` indique um destinatário. Com ele, o comando envia uma mensagem real a esse destinatário,
de `--mail-from` (padrão: o mesmo endereço), e verifica que o plugin recusa uma requisição sem destinatário.
[Enviar e-mails](email.md#a-plugin-mail--plugin) tem o contrato do tipo.

`--kind translator` envia algumas frases fixas em inglês, traduzindo-as para o japonês (`--translate-from` e `--translate-to`
escolhem outro par para um plugin que tem apenas alguns). Um serviço por trás do plugin que cobra por caractere cobra essas frases também,
cerca de 200 caracteres. Ele verifica a forma das respostas e que o plugin recusa o idioma `qaa`, um código que a ISO 639
reserva para uso privado. Ele não consegue julgar as palavras, então leia você mesmo uma tradução do plugin.
[Traduzir publicações](#translating-posts) tem o contrato do tipo.

`--kind search` faz buscas, e grava e remove apenas documentos próprios: alguns com IDs que nenhuma publicação tem, de um
autor que não é nenhuma conta, contendo uma palavra feita para a execução, que ele remove no fim. Ele verifica que uma consulta sem
palavra não encontra nada, que uma consulta na linguagem de consulta de um motor ou em japonês não falha, que o plugin recusa um
cursor que não emitiu, e que os documentos são encontrados do mais novo para o mais antigo, página por página, substituídos e removidos por publicação e por
autor. `--clear-index` também executa os casos que removem todos os documentos de uma visibilidade e o índice inteiro: use-o somente
contra um índice de teste. [Buscar publicações](#searching-posts) tem o contrato do tipo.

`--kind media_image`, `--kind media_video` e `--kind media_audio` enviam apenas pequenos arquivos de teste que eles mesmos criam:
uma foto, um emoji e um GIF animado, um MP4 com vídeo e trilha de som, ou um MP3. Cada um esconde uma localização ou um
título em seus metadados, e um caso falha quando isso volta em uma saída. Eles verificam cada saída como o servidor faz.
[Converter mídia](#converting-media) tem o contrato dos tipos.

`--kind key_manager` executa as verificações que o servidor executa a cada início, e verifica que o plugin recusa um valor alterado e uma
KEK que ele não possui. Ele envia apenas chaves que inventa e descarta, então é seguro contra um armazenamento de chaves real, que pode cobrar
as chamadas. Ele verifica que as respostas têm a forma correta, não como o plugin guarda sua KEK.
[Gerenciar a chave](key-management.md#a-plugin) tem o contrato do tipo.

`probe` é o menor tipo, e o servidor não delega nada a ele. Sirva-o primeiro, para verificar o transporte, o
protocolo e a autenticação antes de escrever o resto do plugin.

<!-- source: translating-posts c9d31b2b9157 -->
## Traduzir publicações

Um plugin do tipo `translator` permite que o servidor traduza uma publicação quando um leitor pede, por meio de um serviço que você escolhe
além dos [provedores embutidos](translation.md) (LibreTranslate, DeepL, Google Cloud Translation e Amazon
Translate). Sem um provedor no servidor, a única tradução é a embutida no navegador do leitor, que existe
apenas no Chrome, Edge e Opera para desktop: leitores em um celular, no Firefox ou no Safari não têm botão de tradução até você
ativar um.

```sh
MY_PLUGIN_TRANSLATOR_ADDRESS=unix:///run/my/translator.sock
MY_PLUGIN_TRANSLATOR_NAME="Our translator"    # what readers see next to a translation
```

1. Defina `plugin_translator_address` e `plugin_translator_name`, e reinicie. O nome é obrigatório, já que os leitores confiam em uma
   tradução de acordo com a origem dela. O endereço nunca sai da configuração.
2. [Atualize sua política de privacidade](privacy-policy-template.md#the-services-you-choose) e cite o serviço por trás do plugin
   como um serviço de tradução: o que o plugin repassa é o que ele recebe.
3. Ative-o com a [configuração da instância](instance-settings.md) `translation_providers = plugin`. Uma alteração passa a valer imediatamente.

Ter um endereço não ativa nada: até a configuração indicar `plugin`, o servidor não envia nada e os leitores não veem botão.

O plugin é um provedor como qualquer outro. [`translation_provider_visibilities`](instance-settings.md) decide o que ele pode
receber, os limites mensais contam o que é enviado a ele, e uma tradução que ele fez fica no cache do servidor como as demais.
Com `translation_reports_to_server` ativada, ele também recebe o que os moderadores pedem para traduzir em uma denúncia, pelas mesmas visibilidades.
Ele pode vir antes ou depois de outro provedor na ordem de `translation_providers`.

<!-- source: what-the-plugin-receives 975c7e56a522 -->
### O que o plugin recebe

O plugin recebe o texto de uma publicação quando um leitor pede uma tradução: o corpo, o aviso de conteúdo, as opções de enquete e
as descrições de imagens, além do idioma do texto e do idioma para o qual traduzir. Ele não recebe o endereço da publicação,
o autor, o leitor, menções, hashtags, links, emojis personalizados nem código: marcadores numerados os substituem.

Por padrão, isso são as publicações `public`, `local` e `unlisted`. **As publicações `private` e `direct` só chegam ao plugin quando você as
permite**, com `translation_provider_visibilities = plugin=public|local|unlisted|private|direct`. Então o plugin, e qualquer
serviço por trás dele, lê texto que o autor destinou a poucas pessoas, portanto só as permita se você controla para onde o texto vai,
e diga isso na sua política de privacidade. O servidor não pode recolher um texto que já enviou.

O servidor recusa `http://` para este tipo, também no loopback, porque o texto é o texto de publicações. Use um socket Unix ou
`https://`.

<!-- source: an-llm-behind-a-plugin 611b6fc77bb1 -->
### Um LLM por trás de um plugin

O servidor não tem provedor de LLM e não oferece nenhum, de propósito. Qualquer pessoa pode escrever uma publicação, então o texto a traduzir é uma forma de
dar instruções a um LLM (uma prompt injection), e os leitores tomam uma tradução como o que a publicação diz. A sanitização protege
o HTML de uma página: ela não consegue impedir que uma tradução diga algo que a publicação não diz.

O servidor não vê o que um plugin chama. **Se o seu plugin usa um LLM, esse risco e suas consequências são de sua
responsabilidade.**

<!-- source: the-contract ef518f876dd2 -->
### O contrato

O RPC é `Translate` de `my.plugin.translator.v1.TranslatorService`:

```text
POST /my.plugin.translator.v1.TranslatorService/Translate
Content-Type: application/json

{"fragments":["Good morning ⟦1⟧, this is ⟦2⟧important⟦/2⟧.","⟦3⟧"],"sourceLanguage":"en","targetLanguage":"ja"}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","translations":["⟦1⟧さん、おはようございます。これは⟦2⟧重要⟦/2⟧です。","⟦3⟧"],"detectedLanguage":"en"}
```

- `sourceLanguage` é uma tag BCP 47, ou `und` quando o servidor não conhece o idioma. O plugin então o detecta e o informa
  em `detectedLanguage`, ou responde `OUTCOME_UNSUPPORTED_LANGUAGE`. `detectedLanguage` pode estar ausente quando o plugin não tem
  nada a acrescentar, e o servidor ignora um valor que não seja uma tag de idioma.
- `translations` tem uma entrada por fragmento, na ordem dos fragmentos, nenhuma delas vazia. Um fragmento pode ser apenas
  marcadores, e sua tradução são então esses marcadores.
- `⟦1⟧` representa um item ou abre um par que `⟦/1⟧` fecha. Cada marcador de um fragmento volta exatamente uma vez na sua
  tradução, sem alteração. Os itens podem mudar de lugar conforme a ordem das palavras exige, mas um par fica aberto antes de fechar, e os pares se aninham.
- Uma tradução é texto simples. O servidor escapa qualquer marcação nela e sanitiza o resultado antes de armazená-lo ou mostrá-lo.

| `outcome` | Significado | O que o servidor faz |
| --- | --- | --- |
| `OUTCOME_OK` | As traduções estão na resposta | Verifica-as, armazena-as e responde ao leitor |
| `OUTCOME_TEMPORARY` | O serviço falhou ou não respondeu | Consulta o próximo provedor |
| `OUTCOME_PROVIDER_LIMIT` | O serviço informa seu próprio limite de taxa ou cota | Consulta o próximo provedor |
| `OUTCOME_UNSUPPORTED_LANGUAGE` | O serviço não tem o idioma de origem ou de destino, ou não consegue detectá-lo | Consulta o próximo provedor e avisa o leitor se nenhum o tem |

Uma resposta que quebra essas regras conta como `OUTCOME_TEMPORARY` e nada dela é armazenado, assim como um `outcome` ausente ou desconhecido
e uma chamada que falhou. Quando nenhum provedor responde, o leitor vê que a tradução falhou e a publicação fica como está, a menos que
`translation_browser` esteja ativada e o navegador do leitor consiga traduzir a publicação: o cliente web então a traduz ali.
Os caracteres de cada chamada contam para os limites mensais, também os de uma chamada que falha, já que o serviço pode tê-los recebido.

O servidor para de esperar após `plugin_translator_timeout`, e após 20 segundos seja qual for o valor dessa configuração. Todos os
provedores de uma requisição têm 45 segundos no total. Ele não tenta
de novo uma chamada que falhou, e os mesmos fragmentos podem voltar para outro leitor.

<!-- source: searching-posts bd9dfba5a3e7 -->
## Buscar publicações

Um plugin do tipo `search` mantém o índice da [busca de texto completo de publicações](search.md) em um motor que o servidor
não suporta de forma embutida, como Groonga ou PGroonga, um índice construído com Sudachi, Typesense, Manticore ou um serviço
de busca hospedado. Para o servidor, é só mais um motor: as mesmas publicações entram no índice, os mesmos jobs o mantêm atualizado
e o servidor continua decidindo o que cada pessoa pode ver.

```sh
MY_SEARCH=plugin
MY_PLUGIN_SEARCH_ADDRESS=unix:///run/my/search.sock
```

1. Execute o plugin e verifique-o com [`my plugin test --kind search`](#test-a-plugin).
2. Defina `search = plugin` e `plugin_search_address`, e siga [Ativar a busca](search.md#turning-it-on), como para qualquer
   motor: o índice passa a ser preenchido daí em diante, e `my search rebuild` adiciona as publicações existentes.
3. Se o plugin envia as publicações a um serviço de outra empresa, cite esse serviço nos seus
   [documentos de privacidade](search.md#your-privacy-documents).

O servidor não abre nenhuma conexão ao iniciar, então um plugin fora do ar não impede nada além da busca. Para migrar para o
plugin ou sair dele, siga [Desativar ou trocar o motor](search.md#switching-off-or-changing-the-engine).

<!-- source: what-the-plugin-receives-1 d2b09f214f3a -->
### O que o plugin recebe

- **As publicações do índice.** Para cada publicação que o índice guarda, o plugin recebe o texto (o corpo, o aviso de
  conteúdo e as descrições de imagens, como texto simples), o ID, o ID da conta do autor, a visibilidade, o idioma
  e quando foi publicada. São apenas publicações `public` e `local`, como diz [`search_visibilities`](instance-settings.md),
  de autores que permitem a busca. As publicações `local` se destinam apenas às contas deste servidor: o plugin, e qualquer
  serviço por trás dele, as lê também. As publicações `unlisted`, `private` e `direct` nunca chegam a ele.
- **As consultas.** Quando uma pessoa conectada busca publicações, o plugin recebe o que ela digitou, com o cursor da
  página e o número de resultados. Ele não recebe quem buscou.
- **As remoções:** de uma publicação, de todas as publicações de uma conta, de todas as publicações de uma visibilidade e do índice inteiro.

O plugin retorna apenas IDs de publicações. O servidor lê cada publicação de novo do seu banco de dados e mostra apenas o que a pessoa pode
ver agora, então um plugin não pode mostrar uma publicação a quem não pode vê-la, e um índice atrasado apenas encontra menos. O
servidor também nunca mostra os erros de um plugin a ninguém.

O servidor recusa `http://` para este tipo, também no loopback, porque ele transporta o texto das publicações e o que as pessoas buscam.
Use um socket Unix ou `https://`.

<!-- source: the-contract-1 d500365750c4 -->
### O contrato

O serviço é `my.plugin.search.v1.SearchService`, com um RPC por operação:

| RPC | O que faz |
| --- | --- |
| `Replace` | Adiciona o documento de uma publicação ou substitui o documento com o mesmo `statusId` |
| `Remove` | Remove o documento de uma publicação |
| `RemoveByAuthor` | Remove todos os documentos de uma conta |
| `RemoveByVisibility` | Remove todos os documentos de uma visibilidade, `VISIBILITY_PUBLIC` ou `VISIBILITY_LOCAL` |
| `Clear` | Remove todos os documentos; o índice volta a aceitar documentos depois |
| `Search` | Retorna uma página dos IDs dos documentos que uma consulta encontra, do mais novo para o mais antigo |

```text
POST /my.plugin.search.v1.SearchService/Replace
Content-Type: application/json

{"document":{"statusId":"019a5f3e-8b2c-7d41-9a3e-2f6c1b8d4e07","authorId":"0199c2d4-1e5f-7a3b-8c9d-0e1f2a3b4c5d","visibility":"VISIBILITY_PUBLIC","language":"ja","publishedAt":"2026-11-07T05:12:30Z","text":"週末に図書館で全文検索の本を読んだ"}}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK"}
```

```text
POST /my.plugin.search.v1.SearchService/Search
Content-Type: application/json

{"text":"全文検索","limit":1}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","statusIds":["019a5f3e-8b2c-7d41-9a3e-2f6c1b8d4e07"],"nextCursor":"019a5f3e-8b2c-7d41-9a3e-2f6c1b8d4e07"}
```

- **Uma consulta é texto simples.** O plugin a divide em palavras e encontra os documentos com todas as palavras. Ele nunca lê o
  texto como a linguagem de consulta do seu motor: um operador, um curinga, uma aspa ou um nome de campo nele é apenas texto. Uma consulta
  sem palavra não encontra nada. Como o plugin encontra palavras em japonês e em outros idiomas escritos sem espaços é uma
  escolha dele, e a razão de ter um plugin.
- **Uma página vem do mais novo para o mais antigo.** Os `statusIds` estão em ordem decrescente, cada um no máximo uma vez, no máximo `limit` deles, e cada um um
  ID de publicação como o servidor o enviou (36 caracteres minúsculos). Os IDs são UUIDv7, então a ordem deles como strings é a ordem do
  tempo.
- **O cursor é do plugin.** `nextCursor` é vazio na última página e, caso contrário, tem no máximo 256 bytes de ASCII
  imprimível sem espaço, como o último ID da página. O servidor o devolve sem alteração para a próxima página, que continua
  depois desse ID mesmo que documentos tenham entrado e saído. Uma página que não é a última pode ter menos de `limit` IDs. Um cursor
  que o plugin não emitiu recebe `OUTCOME_INVALID_CURSOR`.
- **Uma gravação aparece na próxima busca**, e toda chamada pode vir duas vezes: remover o que não existe tem sucesso. Um motor que
  mostra as gravações só depois de uma atualização faz o plugin esperar por ela.

| `outcome` | Significado | O que o servidor faz |
| --- | --- | --- |
| `OUTCOME_OK` | A chamada fez o que pede | Aplica a política de visualização aos IDs de uma página |
| `OUTCOME_FAILED` | O motor falhou ou não respondeu | Tenta a gravação de novo com o job; faz a busca falhar, e a pessoa pode tentar de novo |
| `OUTCOME_INVALID_CURSOR` | Apenas `Search`: o cursor não é um que o plugin emitiu | Informa ao cliente que seu cursor está errado |

Um `outcome` ausente ou desconhecido, uma chamada que falhou e uma página que quebra essas regras contam como `OUTCOME_FAILED`, e nada
dessa página é mostrado. Uma chamada dura no máximo `plugin_search_timeout` (10 segundos por padrão), e 10 segundos seja qual for o valor dessa
configuração.

<!-- source: converting-media 36df3dbdb21b -->
## Converter mídia

Um plugin de mídia faz a parte pesada do processamento de envios, para que possa rodar em outra máquina. Há três tipos,
e você escolhe cada um separadamente:

- `media_image` recodifica imagens em suas variantes.
- `media_video` faz o remux de vídeos: arquivos com uma faixa de vídeo, com ou sem som.
- `media_audio` faz o remux de arquivos somente de áudio.

O servidor ainda faz o resto por conta própria. Ele detecta o formato de cada envio, distingue vídeos de arquivos de áudio, verifica cada
envio contra os limites quando chega e cria as prévias de mídia remota. Um tipo cuja configuração continua `native` é
processado no servidor.

```sh
# Only videos go to a plugin, on a machine of its own; images and audio stay in the server.
MY_MEDIA_VIDEO=plugin
MY_PLUGIN_MEDIA_VIDEO_ADDRESS=https://media.internal:9000
```

Defina `media_<kind> = plugin` e `plugin_media_<kind>_address` para cada tipo que você entrega, e reinicie. Um plugin pode
assumir mais de um tipo: dê a ele o mesmo endereço para cada um. Cada tipo executa no máximo duas chamadas ao mesmo tempo, e cada chamada dura no máximo
`plugin_media_<kind>_timeout` (4 minutos por padrão), de modo que um vídeo lento não atrasa as imagens.

- **Remover metadados é tarefa do plugin.** Os envios chegam ao plugin como foram enviados, com seus dados EXIF e
  tags, que podem conter onde uma foto foi tirada. O servidor verifica o formato e o tamanho de cada saída, mas não consegue
  saber se todo vestígio de metadados sumiu. `my plugin test --kind media_image` verifica que a localização dos seus arquivos de teste
  não volta; verifique o resto no plugin.
- **O TCP simples envia os arquivos sem criptografia.** Os tipos de mídia aceitam `http://`, com um aviso ao iniciar. Os envios, com seus
  metadados, então atravessam a rede em texto simples, e nada autentica nenhuma das pontas: qualquer pessoa nessa rede pode lê-los
  ou responder pelo plugin. Use um socket Unix ou `https://`, a menos que a rede seja só sua.
- **O servidor não confia na saída.** Ele detecta o formato de cada saída pelo conteúdo, nunca pelo que o
  plugin declara, e a serve com o tipo de conteúdo e a extensão de arquivo desse formato. Ele decodifica cada saída sob
  os mesmos limites de um envio e verifica que ela cabe no seu tamanho. Uma saída maior que o declarado ou que esses
  limites, de um formato que o plugin não declarou ou de um formato que a variante não aceita, faz o envio falhar como um arquivo
  que o servidor não consegue processar.
- **Uma falha é um atraso.** Quando o plugin está fora do ar ou falha, o job tenta de novo mais tarde, como no processador embutido. Uma
  ação que precisa da imagem de um emoji personalizado na hora, como copiar um emoji remoto ou aceitar um pedido de emoji, falha
  enquanto o plugin falhar.

<!-- source: the-contract-2 812c987c276c -->
### O contrato

O serviço é `my.plugin.media.v1.MediaService`, com os RPCs `Process` para uma imagem e `Remux` para um arquivo de vídeo ou áudio.
Os dois transmitem nos dois sentidos, então o plugin precisa de uma biblioteca Connect ou gRPC e de HTTP/2: `proto` ou `grpc`, sobre TLS com ALPN
`h2`, ou sobre HTTP/2 em texto simples (h2c) em um socket Unix ou TCP simples.

Uma chamada é half-duplex:

1. O servidor envia um `head`: os limites e, para uma imagem, sua finalidade, as caixas de seus três tamanhos e seu ponto
   focal. O arquivo segue em mensagens `chunk` de no máximo 1 MiB. O servidor então fecha o seu lado.
2. O plugin responde com um `head` cujo `outcome` é o resultado. Com `OUTCOME_OK`, o head de `Process` também contém as
   dimensões da imagem após sua orientação EXIF e o blurhash da pequena saída estática.
3. Para cada saída, o plugin envia um `part`, que nomeia a saída e declara seu formato, dimensões e tamanho,
   seguido de mensagens `chunk` de no máximo 1 MiB.

`Process` retorna as saídas estáticas pequena, média e grande, e as três animadas de uma imagem com mais de um
quadro. As saídas pequena e média são WebP; a saída estática grande é um JPEG ou um PNG (sempre um PNG para um emoji personalizado),
e a animada grande, um GIF. `Remux` retorna o arquivo em seu próprio contêiner sem metadados, e a imagem de capa que ele
embute, se houver. O servidor lê no máximo o dobro do limite de imagem de `Process` e, de `Remux`, o limite de vídeo para o
arquivo e o limite de imagem para a capa.

| `outcome` | Significado | O que o servidor faz |
| --- | --- | --- |
| `OUTCOME_OK` | As saídas seguem | Verifica-as e as armazena |
| `OUTCOME_TEMPORARY` | O plugin falhou | Tenta o job de novo mais tarde |
| `OUTCOME_UNSUPPORTED_FORMAT`, `OUTCOME_UNSUPPORTED_CODEC`, `OUTCOME_TOO_LARGE`, `OUTCOME_DIMENSIONS`, `OUTCOME_CORRUPT`, `OUTCOME_TOO_LONG` | O arquivo não é aceitável | Marca o envio como falho |

Um `outcome` ausente ou desconhecido e uma chamada que falhou contam como `OUTCOME_TEMPORARY`.

<!-- source: a-plugin-with-no-library be054eb80e74 -->
## Um plugin sem biblioteca

Com `json`, um plugin precisa apenas de um servidor HTTP e de uma biblioteca JSON. Um RPC é um `POST` para
`/<package>.<Service>/<Method>` com `Content-Type: application/json`, e o plugin responde `200` com um objeto JSON:

```text
POST /my.plugin.probe.v1.ProbeService/Echo
Content-Type: application/json
Connect-Protocol-Version: 1

{"payload":"AP8=","number":"9007199254740993"}
```

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"outcome":"OUTCOME_OK","payload":"AP8=","number":"9007199254740993"}
```

O JSON segue o mapeamento padrão do proto3:

- Um `int64` é uma string decimal, `bytes` são base64 e um enum é o seu nome.
- Um campo em seu valor zero pode estar ausente, nos dois sentidos. Um plugin deve ler um campo ausente como zero.
- O servidor ignora campos que não conhece.
- Um resultado é o `outcome` da resposta. Um `outcome` ausente ou desconhecido é uma falha, nunca um sucesso.
- Um plugin que falha responde com um erro HTTP, ou com o `{"code":"unavailable"}` do Connect com o status correspondente. O job tenta de novo.

`Connect-Timeout-Ms` e `Connect-Protocol-Version` podem ser ignorados.
