<!-- source: federation-relays bbd5bb98dde3 -->
# Relays de federação

Um relay é um servidor que repassa as publicações públicas dos servidores que entram nele para os demais servidores
participantes. Até uma instância pequena, com poucos seguidos, passa a ter uma linha do tempo federada e linhas do tempo de
hashtags cheias, e as publicações públicas dela chegam mais longe. Uma instância que não entra em nenhum relay não ganha
linhas nem jobs.

<!-- source: join-a-relay 147b07e65534 -->
## Entrando em um relay

Use “Relays” na área de administração (`/admin/relays`). É preciso a permissão `manage_settings`, que a função `admin`
embutida tem. “Ingressar em um relay” abre o formulário que o registra. A lista mostra o estado de cada relay com um botão
que sai dele ou que volta a entrar; ao escolher um relay, abre-se a página dele, com os endereços, o que ele trouxe nas
últimas 24 horas, e o botão que o exclui. A CLI é `my admin relay` (`my admin relay -help`):

```sh
my admin relay list                                               # every relay, its state, and what it brought in during the last 24 hours
my admin relay create [-protocol mastodon|litepub] [-send-only] <url>  # register a relay and send it the Follow
my admin relay unfollow <relay id>                                # leave a pending or joined relay
my admin relay follow <relay id>                                  # join again a relay that the instance left or that rejected it
my admin relay delete <relay id>                                  # delete a relay that the instance left or that rejected it
```

1. Registre o URL do relay. Há dois protocolos:
   - **Mastodon**: o **URL da inbox** do relay (como `https://relay.example/inbox`). Muitos relays, incluindo pub-relay e Activity-Relay, aceitam esse formato.
   - **LitePub**: o **URL do ator** do relay (como `https://relay.example/relay`). É o protocolo dos relays do Pleroma e do Akkoma.
   Se você não escolher um protocolo (“Detectar automaticamente”), o servidor usa LitePub quando o URL serve um documento de ator, e Mastodon caso contrário.
2. Depois do registro, o ator da instância envia ao relay um `Follow`, e o estado fica como “Aguardando a resposta do relay” (`pending`). Quando o relay aceita, o estado passa a “Participando” (`accepted`); quando recusa, “Recusado pelo relay” (`rejected`). Alguns relays demoram porque um operador aprova cada pedido manualmente.
3. Se você desativar “Receber publicações do relay” no registro, o servidor apenas envia. Para mudar isso, saia do relay e registre-o de novo.

Você não pode registrar o relay de um servidor sob uma ação de domínio (`suspend`). Se o servidor de um relay do qual você participa for suspenso depois, o estado não muda, mas nada é enviado nem recebido desse relay, e a área de administração mostra um aviso.

<!-- source: what-the-server-sends 1db52c508f9c -->
## O que o servidor envia

- Somente a criação, a edição e a exclusão de publicações locais **públicas** (`public`) são enviadas. Publicações `unlisted`, somente para seguidores, direct ou somente locais (`local`), e os impulsionamentos, nunca são enviados.
  Para manter uma publicação fora dos relays, publique-a como `unlisted` ou `local`. Não há opção de desativação por usuário.
- Os documentos das publicações públicas trazem uma Linked Data Signature (`RsaSignature2017`), para que Mastodon e Misskey aceitem publicações que um relay repassa sem alterações.
- O número de entregas cresce pelo número de relays dos quais você participa vezes o número de publicações públicas (as entregas a um relay são unidas em uma só).

<!-- source: what-the-server-receives ff962150d1a7 -->
## O que o servidor recebe

- Uma publicação pública trazida por um relay do qual você participa é buscada de novo na origem e verificada antes de ser ingerida. Depois ela aparece na linha do tempo federada e nas linhas do tempo de hashtags. Ela não aparece nas linhas do tempo de início e não cria notificações.
- No máximo `relay_max_activities_per_hour` publicações por hora (padrão 1.000) são ingeridas por relay. O que passar do limite é descartado sem ser buscado.
  A página do relay na área de administração e `my admin relay list` mostram quantas foram ingeridas nas últimas 24 horas e quantas foram descartadas por passar do limite. Se muitas forem descartadas, aumente o limite ou reconsidere o relay.
- Uma publicação não é ingerida quando seu domínio ou sua conta tem um `suspend` (uma restrição de federação) ou um `limit` (uma restrição que a retira dos espaços públicos).
- Uma publicação que chegou somente por um relay sai da visualização pública depois da configuração da instância `relay_status_retention_days` (padrão 7). Uma publicação que uma pessoa local respondeu, impulsionou, favoritou ou salvou, e as publicações de autores que uma pessoa local segue, recebem a mesma retenção das outras publicações remotas (`remote_status_retention_days`).
- Ingerir custa uma busca na origem por publicação (mais uma busca do ator para um autor desconhecido). No limite de 1.000 por hora, isso dá no máximo 24.000 buscas por dia.

As configurações estão na [referência de configuração](../reference/configuration.md#relay_max_activities_per_hour). O resultado da ingestão também é exportado como a métrica `my.relay.activities` (veja [Rodando o servidor](running.md)).

<!-- source: leave-join-again-and-delete ea12da94f940 -->
## Sair, entrar de novo e excluir

- “Sair” envia um `Undo` do `Follow` e define o estado como “Saiu” (`left`). Depois disso, nada é enviado nem recebido do relay. As publicações já ingeridas são removidas quando termina a retenção delas.
- “Ingressar de novo” envia um novo `Follow` a um relay que recusou o servidor ou que o servidor deixou.
- “Excluir” remove o registro de um relay que foi deixado ou que recusou o servidor. Edições e exclusões de publicações já enviadas ao relay continuam sendo entregues a ele mesmo depois que você o exclui.

Registrar, entrar, sair e excluir, bem como a aceitação ou a recusa do relay, ficam registrados no registro de auditoria com o tipo de alvo “Relay” (aceitação e recusa como ações remotas).
