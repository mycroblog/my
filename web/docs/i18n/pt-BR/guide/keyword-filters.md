<!-- source: keyword-filters 466c3876a3d1 -->
# Filtros de palavras-chave

Um filtro de palavras-chave recolhe ou oculta as publicações que contêm uma de suas palavras, seja quem for o autor: spoilers de uma série que você ainda não viu, um assunto do qual você precisa de uma pausa. O servidor faz a correspondência com as publicações, então todos os aplicativos que você usa mostram o mesmo resultado. Só você vê seus filtros, e ninguém é avisado deles.

<!-- source: adding-a-filter 455f52213244 -->
## Adicionando um filtro

Abra “Filtros de palavras-chave” em “Pessoas e filtros” nas configurações e escolha “Adicionar um filtro”:

1. **Nome:** o que uma publicação recolhida mostra no lugar dela (“Corresponde ao filtro “Final da temporada””), com até 50 caracteres.
2. **Publicações que correspondem:** “Recolher atrás do nome do filtro” mostra apenas esse nome no lugar da publicação, com “Mostrar” para lê-la.
   “Ocultar completamente” deixa a publicação de fora, e a notificação dela também.
3. **Termina:** “Nunca”, depois de uma das durações oferecidas, de 30 minutos a uma semana, ou “Em uma data e hora”.
4. **Palavras:** uma ou mais palavras ou expressões. “Adicionar uma palavra” acrescenta outra, até 100 em um filtro. “Palavra inteira” ao lado de uma palavra
   faz com que ela corresponda apenas como palavra inteira (veja [Como as palavras correspondem](#how-words-match)).
5. **Onde se aplica:** “Início e listas”, “Notificações”, “Linhas do tempo públicas” (as linhas do tempo local, federada e de
   hashtags), “Conversas” e “Perfis”. Um filtro novo se aplica em todos os lugares; desmarque os lugares em que você não quer que ele valha.

Escolha “Adicionar”. Um filtro da lista o abre para mudar qualquer um desses itens, e “Excluir…” ali remove o filtro com as palavras dele depois de
pedir confirmação. Um filtro se aplica na hora a todas as publicações, inclusive as antigas; as linhas do tempo são carregadas de novo na tela para mostrar a mudança.

A busca, seus itens salvos e favoritos e suas mensagens diretas nunca são filtrados. Suas próprias publicações e
impulsos também não, nem as publicações que eles citam.

<!-- source: what-a-filtered-post-looks-like 52e0483be326 -->
## Como fica uma publicação filtrada

Uma publicação que um filtro de recolhimento reconhece mostra apenas “Corresponde ao filtro” com os nomes dos filtros que a reconheceram, e “Mostrar”.
O autor, o texto e as imagens ficam fora de vista, e um leitor de tela lê apenas essa linha, até você escolher “Mostrar”;
“Ocultar” a recolhe de novo, assim como voltar à página.

Uma notificação sobre essa publicação mostra a mesma linha no lugar do texto dela. Quando apenas uma publicação citada corresponde, apenas
a citação é recolhida, mesmo com “Ocultar”, com um “Mostrar” próprio. Um impulso é filtrado pela publicação que ele impulsiona.

Uma publicação que você abre por conta própria, a partir de um link ou de uma notificação, é recolhida em vez de ocultada, para que o link ainda mostre
alguma coisa.

<!-- source: when-a-filter-ends d70431d92b82 -->
## Quando um filtro termina

Um filtro com data de término deixa de se aplicar nesse momento. Ele continua na sua lista, marcado como “Expirado”, até você excluí-lo. Para usá-lo
de novo, abra-o e escolha um novo término, ou “Nunca”. Ao editar um filtro que tem término, “Sem mudança” mantém o término como
está.

<!-- source: how-words-match aeca692b00c3 -->
## Como as palavras correspondem

Uma palavra corresponde ao texto de uma publicação, ao aviso de conteúdo dela (e ao texto por trás dele), às opções da enquete e às descrições das imagens. Os links correspondem pelo texto que mostram, não pelo endereço por trás deles. Uma expressão com espaços corresponde a essas palavras nessa ordem; qualquer sequência de espaços ou quebras de linha em uma publicação corresponde a um espaço.

Maiúsculas e minúsculas, formas de meia largura e de largura total, e katakana e hiragana correspondem entre si:

| Palavra | Também corresponde a |
| --- | --- |
| `Finale` | `finale`, `FINALE`, `ｆｉｎａｌｅ` |
| `ネタバレ` | `ねたばれ`, `ﾈﾀﾊﾞﾚ` |
| `season finale` | `Season  Finale`, e as duas palavras separadas por uma quebra de linha |

Uma palavra corresponde em qualquer lugar em que apareça, até dentro de uma palavra mais longa. “Palavra inteira” faz com que ela corresponda apenas onde nenhuma letra, dígito ou `_` a toque de nenhum dos lados:

| Palavra | Sem “Palavra inteira” | Com “Palavra inteira” |
| --- | --- | --- |
| `cat` | `cat`, `cats`, `concatenate` | `cat`, `cat!`, `#cat`, mas não `cats` |
| `vote` | `vote`, `voters`, `devoted` | `vote`, `Vote.`, mas não `voters` |

Algumas escritas formam palavras sem espaços entre elas: chinês, japonês (kanji, hiragana e katakana), tailandês, laosiano, khmer e birmanês, entre outras. Um filtro não consegue saber onde uma palavra termina nelas, então suas letras nunca contam como encostadas, e “Palavra inteira” funciona como se estivesse desativada: `猫` também corresponde a `子猫` e `猫舌`, e `選挙` também corresponde a `選挙速報`. Escolha palavras mais longas nesses idiomas para não corresponder a mais do que você pretende, por exemplo `最終回のネタバレ` em vez de `最終回`. Ao lado de uma letra de outra escrita, “Palavra inteira” continua valendo: `AI` com “Palavra inteira” corresponde a `AIの話`, mas não a `AIDS`.

Um filtro não tem curingas nem expressões regulares. Suas palavras correspondem quando qualquer uma delas corresponde; para recolher publicações apenas quando duas palavras aparecem juntas, escreva-as como uma única expressão.

<!-- source: limits 2f4588a30027 -->
## Limites

Você pode ter até 100 filtros, com até 100 palavras cada um e 1.000 palavras no total. Uma palavra tem até 100 caracteres.

O arquivo de “Exportar”, em “Conta e dados” nas configurações, contém seus filtros, inclusive os expirados, como
`keyword_filters.json`.
