<!-- source: managing-the-key-encryption-key-kek 571fb6e3c6b4 -->
# Gerenciamento da chave de criptografia de chaves (KEK)

O servidor criptografa os segredos que guarda no banco de dados com uma chave de criptografia de chaves (KEK) que ele nunca armazena ali.
Esta página explica como gerar, armazenar, fazer backup e rotacionar a KEK, como mantê-la em um KMS e o que fazer se ela for perdida ou vazar.
A [referência de configuração](../reference/configuration.md) lista todas as configurações.

<!-- source: how-it-works 37074c6237d0 -->
## Como funciona

![A hierarquia de chaves: a chave de criptografia de chaves, mantida onde key_manager indicar e nunca no banco de dados, envolve uma chave de criptografia de dados criada para cada segredo, que criptografa o segredo; o banco de dados guarda o ID da chave, a chave de criptografia de dados envolvida e o segredo criptografado em cinco colunas seladas. A lista abaixo descreve cada camada.](images/key-hierarchy.svg)

- Os segredos armazenados no banco de dados, como as chaves privadas dos atores, são criptografados com AES-256-GCM usando uma chave de criptografia de dados (DEK) gerada para cada segredo.
  A DEK é envolvida por uma **chave de criptografia de chaves (KEK)** e armazenada ao lado do texto cifrado (envelope encryption).
- A KEK não é armazenada no banco de dados, então um dump vazado do banco de dados, sozinho, não expõe os segredos.
- Cada texto cifrado carrega o **ID da chave** da KEK que envolveu sua DEK. Com `key_manager = local`, o ID da chave é `local:` seguido de 16 dígitos hexadecimais derivados da KEK,
  então a mesma KEK sempre gera o mesmo ID. Você nunca atribui IDs.
- Um texto cifrado fica vinculado ao ID da linha e ao nome da coluna em que é armazenado. Copiá-lo para outra linha ou coluna o torna impossível de decifrar.
- As colunas seladas, com os nomes que `my kek status` imprime, são `local_account_private_keys.private_key_enc` (chaves privadas dos atores),
  `totp_enrollments.secret_enc` (segredos da autenticação de dois fatores), `login_provider_secrets.sealed_secret` (segredos de cliente dos provedores de login),
  `webhook_secrets.sealed_secret` (segredos de assinatura de webhooks) e `instance_secrets.sealed_secret` (a chave de assinatura do proxy de mídia).
- O estado de um login com passkey ou de um login por meio de um provedor externo é selado da mesma forma, mas não é armazenado: o navegador o guarda por no máximo 30 minutos.
- A chave de assinatura (HMAC-SHA256) das URLs do proxy de mídia é uma chave aleatória que o servidor gera na primeira inicialização e armazena no banco de dados, criptografada como os outros segredos.
  Ela não muda quando você rotaciona a KEK, então as URLs do proxy de mídia continuam funcionando durante as rotações, e `my kek rewrap` a reenvolve junto com os outros segredos.
- `key_manager` é `local` (o padrão: usa a configuração `kek`), `gcpkms` (a KEK fica no Google Cloud KMS; veja [Google Cloud KMS](#google-cloud-kms)),
  `awskms` (a KEK fica no AWS KMS; veja [AWS KMS](#aws-kms)) ou `plugin` (a KEK fica com um programa seu; veja [Um plugin](#a-plugin)).

<!-- source: what-happens-if-you-lose-the-kek 76126b09f09c -->
## O que acontece se você perder a KEK

Sem a KEK, restaurar o banco de dados não permite decifrar as chaves privadas dos atores. Depois que uma chave privada é perdida, outros servidores não conseguem mais verificar as assinaturas desse ator,
e **a identidade do ator no fediverso é quebrada** (as relações com os seguidores não podem ser transferidas). Trate a KEK com o mesmo cuidado que o banco de dados.

![O que se perde com a KEK: depois de uma restauração sem ela, tudo o que não está selado continua legível, todo valor selado falha ao abrir e os atores perdem sua identidade. O parágrafo abaixo diz o mesmo.](images/kek-loss.svg)

O que não está selado continua legível: publicações, contas, seguidores e o restante do banco de dados, além dos arquivos de mídia no armazenamento.
Todo valor selado falha com um erro `unknown key-encryption key`: as chaves privadas dos atores, os segredos da autenticação de dois fatores,
os segredos de cliente dos provedores de login, os segredos de assinatura de webhooks e a chave de assinatura do proxy de mídia.
O mesmo acontece quando outra KEK é configurada no lugar daquela que envolveu os segredos, ou quando a chave do KMS ou a chave do plugin que os envolveu é desativada ou destruída.

<!-- source: generate-a-kek 44b338b7440b -->
## Gerar uma KEK

Codifique 32 bytes aleatórios com base64 padrão.

```sh
openssl rand -base64 32
```

Onde `openssl` não estiver disponível, `head -c 32 /dev/urandom | base64` também funciona. Não deixe o valor gerado no histórico do terminal nem em tickets.

<!-- source: store-and-deliver-it 9300565c2a3c -->
## Armazenar e entregar

A KEK é uma configuração secreta, então só pode ser passada pela variável de ambiente `MY_KEK` ou por `MY_KEK_FILE`, o caminho de um arquivo que contém o valor. Colocá-la no arquivo de configuração (TOML) impede a inicialização.

| Destino | Exemplo de como entregá-la |
| --- | --- |
| Auto-hospedado (binário / systemd) | Coloque-a em um arquivo que só o dono possa ler (`chmod 600`) e aponte `MY_KEK_FILE` para ele |
| Docker / Compose | Monte um segredo do Docker em `/run/secrets/my_kek` e defina `MY_KEK_FILE=/run/secrets/my_kek`. Os arquivos de `deploy/docker` fazem isso a partir de `secrets/kek` ([Arquivos do Docker Compose](running.md#docker-compose-files)): para rotacionar, substitua esse arquivo e defina `MY_KEK_PREVIOUS` em `.env` |
| Kubernetes / microk8s | Monte um Secret como arquivo e aponte `MY_KEK_FILE` para ele |
| Cloud Run | Exponha um segredo do Secret Manager como arquivo ou como variável de ambiente |
| AWS ECS | Passe-a a partir do Secrets Manager ou do SSM Parameter Store (SecureString) |

- Dê a mesma KEK a todos os processos, como web e worker.
- A KEK nunca aparece em logs nem em mensagens de erro (apenas o ID da chave aparece).
- `my config check` valida o formato da KEK (base64 de 32 bytes) e imprime o valor como `(redacted)`. No perfil `production`, ele também falha com uma KEK
  que não seja aleatória, como todos os bytes zero ou um byte repetido, e `my serve` registra um aviso sobre ela (veja a [lista de verificação de produção](production-checklist.md)).

<!-- source: back-it-up 29e7bdb3b114 -->
## Faça o backup

**Faça o backup do banco de dados e da KEK, separadamente.**

- Mantenha a KEK em um lugar diferente de onde ficam os backups do banco de dados (um gerenciador de senhas, mídia offline, um armazenamento de segredos em outra conta e assim por diante).
  Se os dois estiverem no mesmo lugar, o vazamento desse lugar torna inútil a criptografia do banco de dados.
- Durante uma rotação, inclua as chaves de `kek_previous` no backup enquanto ainda houver no banco de dados texto cifrado envolvido por elas.
- Inclua a KEK no seu procedimento de restauração e teste as restaurações regularmente ([Backup e restauração](backup.md#recovery-drill)). Depois de restaurar um banco de dados, defina a KEK (e `kek_previous`) que estava em uso na hora do backup com os mesmos valores.
  Com uma KEK diferente, tudo o que lê um segredo falha com um erro `unknown key-encryption key` que nomeia o ID da chave.

<!-- source: rotate-the-kek eb4af3189053 -->
## Rotacionar a KEK

Uma KEK em `kek_previous` é usada apenas para decifrar, e os novos segredos são sempre criptografados com `kek`.
Para que um deploy gradual, em que processos antigos e novos rodam lado a lado, nunca produza texto cifrado que algum processo não consiga ler, faça a troca em duas etapas.

![Uma rotação da KEK local em cinco etapas, com os valores de kek e kek_previous depois de cada uma: a nova KEK entra primeiro em kek_previous, depois troca de lugar com a antiga, my kek rewrap move os segredos armazenados e a KEK antiga sai da configuração. As etapas abaixo dizem o mesmo.](images/kek-rotation.svg)

1. Gere uma nova KEK e faça o backup dela.
2. Reinicie todos os processos com `MY_KEK=<old KEK>` e `MY_KEK_PREVIOUS=<new KEK>`. Agora todos os processos conseguem decifrar com a nova KEK.
3. Reinicie todos os processos com `MY_KEK=<new KEK>` e `MY_KEK_PREVIOUS=<old KEK>`. A partir de agora os segredos são envolvidos com a nova KEK, e o texto cifrado da KEK antiga continua legível.
4. Reenvolva os segredos armazenados com a nova KEK: execute `my kek rewrap` uma vez, com a mesma configuração da etapa 3 (veja [Reenvolver os segredos armazenados](#re-wrap-the-stored-secrets)).
   Quando ele terminar com 0, nenhum segredo no banco de dados precisa mais da KEK antiga.
5. Espere pelo menos 30 minutos depois da etapa 3, para que os tokens de login de curta duração emitidos com a KEK antiga tenham expirado, e então reinicie todos os processos com `MY_KEK=<new KEK>` e sem `kek_previous`.
   Guarde o backup da KEK antiga enquanto você mantiver backups do banco de dados feitos antes da etapa 4: restaurar um deles volta a precisar dela.

`kek_previous` aceita várias chaves separadas por vírgula. Listar a mesma chave duas vezes, ou listar a mesma chave que `kek`, impede a inicialização.

<!-- source: re-wrap-the-stored-secrets 67af175f1d56 -->
### Reenvolver os segredos armazenados

`my kek rewrap` substitui apenas a chave de criptografia de dados envolvida de cada segredo e mantém o texto cifrado dele; nunca grava nem imprime um segredo.

```sh
my kek status   # count the stored secrets by the key ID of the KEK that wraps them; decrypts nothing
my kek rewrap   # re-wrap every secret that a key of kek_previous wraps with kek
```

- `my kek status` imprime uma linha por coluna e ID de chave, marca o ID da chave de `kek` com `(current)` e termina informando se algum segredo ainda precisa de uma chave de `kek_previous`.
  Um ID de chave que não é `kek` nem está em `kek_previous` significa que falta uma KEK na configuração.
- `my kek rewrap` lê os segredos em lotes de 100 e grava cada lote em uma transação curta. Ele imprime, por coluna, quantos segredos reenvolveu e quantos já estavam envolvidos com `kek`.
  Pode rodar enquanto o servidor está em operação e é seguro interrompê-lo ou executá-lo de novo a qualquer momento: ele pula os segredos já envolvidos com `kek`, e um segredo que o servidor reescreveu nesse meio-tempo é deixado como o servidor o gravou.
- Um segredo que não pode ser reenvolvido é deixado inalterado e informado com o ID da linha, e o comando termina com 1. Ou a KEK dele não está configurada
  (a mensagem nomeia o ID da chave: adicione essa KEK a `kek_previous` e execute de novo), ou o valor armazenado está danificado ou foi copiado de outra linha.
  Não remova uma KEK de `kek_previous` enquanto `my kek status` ainda contar segredos sob ela.

<!-- source: google-cloud-kms 1c47dd463234 -->
## Google Cloud KMS

Com `key_manager = gcpkms`, uma chave simétrica do Cloud KMS (uma CryptoKey com a finalidade `ENCRYPT_DECRYPT`) envolve as chaves de criptografia de dados, e a KEK nunca sai do Cloud KMS.
Um dump vazado do banco de dados e uma configuração vazada, juntos, ainda não expõem os segredos: decifrá-los exige a permissão para usar a chave.

- Defina `kms_key` com o nome do recurso da chave, sem versão:
  `projects/{project}/locations/{location}/keyRings/{ring}/cryptoKeys/{key}`. Nomes com mais de 248 bytes são rejeitados na inicialização.
  O ID da chave dos segredos armazenados é `gcpkms:` seguido desse nome.
- O servidor se autentica com as Application Default Credentials: no Cloud Run, a conta de serviço do serviço; em outros lugares, `GOOGLE_APPLICATION_CREDENTIALS`.
- Conceda a essa conta de serviço `roles/cloudkms.cryptoKeyEncrypterDecrypter` na chave (não no projeto), para todos os processos, como web e worker, e para os comandos `my kek status` e `my kek rewrap`.
  Sem isso, a leitura de um segredo falha com um erro que nomeia a função; uma versão de chave ausente ou desativada falha da mesma forma, e esses erros não são repetidos até que a configuração seja corrigida.
  Um erro de cota ou um Cloud KMS indisponível é temporário, e o trabalho que precisava do segredo é repetido.
- O servidor mantém por `kms_dek_cache_ttl` (padrão `10m`) uma chave de criptografia de dados que o Cloud KMS decifrou, então a maioria das leituras de um segredo não chama o Cloud KMS.
- A build tag `no_gcpkms` deixa o adaptador de fora do binário ([Build tags](../reference/build-tags.md)).

<!-- source: rotate-the-key-in-cloud-kms ba58f052af2a -->
### Rotacionar a chave no Cloud KMS

Uma rotação no Cloud KMS (manual, ou com um período de rotação na chave) adiciona uma nova versão primária e mantém as versões antigas habilitadas.
Os novos segredos são envolvidos com a nova versão, o servidor decifra cada segredo com a versão que o envolveu, e o ID da chave não muda, então **uma rotação não precisa de `my kek rewrap`**.

**Não desative nem destrua uma versão antiga enquanto ainda houver segredos que dependam dela**: um segredo envolvido com uma versão desativada ou destruída não pode mais ser decifrado.
O Cloud KMS não informa quais segredos uma versão envolveu, então, para aposentar material de chave, mova primeiro os segredos para uma nova chave:

1. Crie uma nova chave (ela pode estar no mesmo key ring) e conceda à conta de serviço a função nela.
2. Reinicie todos os processos com `kms_key` definido com a nova chave e `kms_key_previous` definido com a antiga. A chave antiga agora só decifra.
3. Execute `my kek rewrap`, com a mesma configuração. Quando `my kek status` não contar mais segredos sob o ID da chave antiga, remova `kms_key_previous` e reinicie.
4. Só então desative as versões da chave antiga e mantenha-as restauráveis enquanto você mantiver backups do banco de dados feitos antes da etapa 3: restaurar um deles volta a precisar delas.

<!-- source: aws-kms f97406d91b4c -->
## AWS KMS

Com `key_manager = awskms`, uma chave simétrica do AWS KMS (key spec `SYMMETRIC_DEFAULT`, key usage `ENCRYPT_DECRYPT`, gerenciada pelo cliente) envolve as chaves de criptografia de dados, e a KEK nunca sai do AWS KMS.
Um dump vazado do banco de dados e uma configuração vazada, juntos, ainda não expõem os segredos: decifrá-los exige a permissão para usar a chave.

- Defina `kms_key` com o ARN da chave (`arn:aws:kms:{region}:{account}:key/{key-id}`), um alias (`alias/{name}` ou o ARN dele) ou o ID da chave.
  Na inicialização, o servidor resolve o nome para o ARN da chave, e o ID da chave dos segredos armazenados é `awskms:` seguido desse ARN.
  Apontar um alias para outra chave, portanto, não muda nada até a próxima inicialização, e então apenas para novos segredos: os segredos já armazenados continuam nomeando a chave antiga, então defina-a em `kms_key_previous` (veja [Retirar uma chave do AWS KMS](#retire-an-aws-kms-key)).
  Chaves gerenciadas pela AWS (`alias/aws/...`) são rejeitadas; apenas o serviço da AWS que é dono de uma pode usá-la.
- O servidor se autentica com a cadeia de credenciais padrão do AWS SDK: no ECS, a role da tarefa; no EC2, o instance profile; em outros lugares, o ambiente (`AWS_ACCESS_KEY_ID` e semelhantes) ou os arquivos de configuração compartilhados.
  A região é a do ARN da chave; para um alias ou um ID de chave, defina `AWS_REGION`.
- Permita `kms:Encrypt`, `kms:Decrypt` e `kms:DescribeKey` na chave, para todos os processos, como web e worker, e para os comandos `my kek status` e `my kek rewrap`. Uma política da role do IAM:

  ```json
  {
    "Effect": "Allow",
    "Action": ["kms:Encrypt", "kms:Decrypt", "kms:DescribeKey"],
    "Resource": "arn:aws:kms:us-east-1:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab"
  }
  ```

  O recurso é o ARN da chave mesmo quando `kms_key` é um alias. A política da chave também deve permitir que as políticas do IAM da conta concedam acesso (a política de chave padrão permite).
- O servidor não inicia quando a chave não existe, está desativada ou pendente de exclusão, ou não é uma chave simétrica de criptografia. Mais tarde, uma permissão ausente ou uma chave desativada faz a leitura de um segredo falhar com um erro que nomeia a causa,
  e esses erros não são repetidos até que a configuração seja corrigida. Limitação de taxa (`ThrottlingException`) ou um AWS KMS indisponível é temporário, e o trabalho que precisava do segredo é repetido.
- O contexto de criptografia de cada chamada contém o ID da linha e o nome da coluna do segredo (`{"my-aad": ...}`, em base64url). O CloudTrail o registra em texto simples; ele não contém nenhum segredo.
- Custo: a AWS cobra uma taxa mensal por cada chave gerenciada pelo cliente (USD 1 na data desta escrita, mais o mesmo valor por cada uma das duas primeiras rotações do material da chave) e uma taxa por requisição além de uma camada gratuita mensal
  (veja os [preços do AWS KMS](https://aws.amazon.com/kms/pricing/)). O servidor mantém por `kms_dek_cache_ttl` (padrão `10m`) uma chave de criptografia de dados que o AWS KMS decifrou,
  então um servidor com dez atores que entregam o tempo todo faz cerca de 43.000 requisições por mês por processo, alguns centavos além da camada gratuita.
- A build tag `no_awskms` deixa o adaptador de fora do binário ([Build tags](../reference/build-tags.md)).

<!-- source: retire-an-aws-kms-key b670091ad7f1 -->
### Retirar uma chave do AWS KMS

A rotação automática de chaves no AWS KMS mantém o ARN da chave e o material de chave anterior, então o ID da chave não muda e **uma rotação não precisa de `my kek rewrap`**.

Para deixar de usar uma chave, ou para apontar um alias para outra chave, mova primeiro os segredos para a nova chave. **Não desative a chave antiga nem agende a exclusão dela enquanto ainda houver segredos que dependam dela**: um segredo envolvido com ela não pode mais ser decifrado.

1. Crie a nova chave e permita que a role a use.
2. Reinicie todos os processos com `kms_key` definido com a nova chave e `kms_key_previous` definido com a antiga (o ARN dela, não o alias). A chave antiga agora só decifra.
3. Execute `my kek rewrap`, com a mesma configuração. Quando `my kek status` não contar mais segredos sob o ID da chave antiga, remova `kms_key_previous` e reinicie.
4. Só então desative a chave antiga e mantenha-a enquanto você mantiver backups do banco de dados feitos antes da etapa 3: restaurar um deles volta a precisar dela.

<!-- source: a-plugin 43023e60bac9 -->
## Um plugin

Com `key_manager = plugin`, um [plugin](plugins.md) seu guarda a KEK, no Vault, em um HSM ou onde ele escolher, e envolve as chaves de criptografia de dados para o servidor.

**O plugin fica no mesmo limite de confiança que a KEK.** Ele vê todas as chaves de criptografia de dados em texto simples, então quem o controla, ou controla o host em que ele roda, pode ler todos os segredos do servidor, inclusive as chaves privadas dos atores.
Dê a ele a proteção que você dá à KEK. Por isso também nenhuma configuração além do arquivo de configuração e do ambiente pode alterar o endereço dele.

```sh
MY_KEY_MANAGER=plugin
MY_PLUGIN_KEY_MANAGER_ADDRESS=unix:///run/my/key_manager.sock
```

- O endereço é `unix://` ou `https://`. O servidor recusa `http://`, inclusive em loopback, já que as chaves trafegariam em texto simples. As outras configurações do destino estão em [Plugins](plugins.md#the-destination).
- Execute `my plugin test --kind key_manager <address>` antes de apontar o servidor para um plugin ([Testar um plugin](plugins.md#test-a-plugin)). Ele envolve apenas chaves que inventa e descarta.
- O ID da chave dos segredos armazenados é `plugin:` seguido do ID que o plugin dá à sua KEK, que tem de 1 a 248 bytes de ASCII visível.
- **Mantenha toda KEK que um segredo armazenado ainda nomeia.** Uma rotação no plugin aparece como um novo ID no próximo envolvimento. Os segredos envolvidos antes continuam nomeando o ID antigo, e o plugin ainda precisa conseguir abri-los. `my kek rewrap` os move para o ID atual, e `my kek status` conta os que restam.
- Um plugin e um KMS nunca atuam juntos: `kms_key` e `kms_key_previous` são recusados com um plugin, e `plugin_key_manager_address` também é recusado com `key_manager = gcpkms` ou `awskms`.
- A build tag `no_plugins` deixa o adaptador de fora do binário ([Build tags](../reference/build-tags.md)).

<!-- source: when-the-server-starts 5faeda92e43b -->
### Quando o servidor inicia

Todo processo, e todo comando `my` que abre o banco de dados, como `my kek status`, faz isto antes de qualquer outra coisa. É o único tipo de plugin cuja falha impede uma inicialização.

1. Ele pergunta ao plugin o ID da KEK atual dele.
2. Ele executa um autoteste com uma chave aleatória que descarta: um envolvimento e um desenvolvimento devem devolver a chave, um desenvolvimento com outros dados autenticados deve responder `OUTCOME_DECRYPT`, dois envolvimentos da chave devem diferir, e o valor envolvido não deve conter a chave.

Um plugin que não responde, ou responde `503`, é tentado de novo por até 30 segundos, para o caso de um sidecar que sobe depois do servidor. Depois disso, o processo termina com um erro. Uma resposta errada, e qualquer outra falha, como um token recusado, o interrompe imediatamente: o autoteste existe para barrar um plugin que ignora os dados autenticados ou repassa chaves, e esperar não conserta um plugin assim.

<!-- source: when-the-plugin-is-unreachable 71bf24801c2b -->
### Quando o plugin está inacessível

O servidor falha de forma fechada. Seja qual for o problema, uma chamada recusada ou com tempo esgotado, ou uma resposta vazia, desconhecida ou malformada, a operação que precisava da chave falha com um erro. Ele nunca armazena um segredo em texto simples, nunca passa para a KEK local ou para outra chave e nunca trata uma falha como um resultado. Um job que falha assim é repetido depois, e uma requisição falha para a pessoa que a fez. Cada chamada que falha é contada em [`my.plugin.failures`](running.md) por tipo e código Connect, e o log não contém chave, dados autenticados nem valor envolvido, nem nada do que o plugin disse.

O envolvimento nunca é armazenado em cache, e um valor selado há pouco não tem nada no cache, então estes param imediatamente:

- Criar uma conta, que gera as chaves do ator.
- Configurar a autenticação de dois fatores e registrar uma passkey ou entrar com uma, cujas cerimônias mantêm o estado selado.
- Entrar por meio de um provedor externo e salvar o segredo de cliente de um provedor.
- Registrar um webhook e `my kek rewrap`.

Ler um segredo armazenado só funciona enquanto o processo guarda a chave de criptografia de dados dele de uma leitura anterior, por no máximo `kms_dek_cache_ttl` (padrão `10m`). Depois disso, ou para um segredo que ele ainda não leu, estes também param:

- A entrega da federação e as buscas assinadas, que precisam das chaves privadas dos atores. A entrega se recupera pelas repetições dos jobs dela, assim que o plugin voltar.
- A verificação de um código de dois fatores no login.
- A entrega de webhooks.

Uma reinicialização para como descrito acima, então **não reinicie o servidor para consertar uma indisponibilidade do plugin**: um servidor que está no ar continua servindo o que o cache dele cobre, e um que reinicia não serve nada até o plugin responder. O que não precisa de segredo, como ler linhas do tempo e publicar, continua. A chave de assinatura do proxy de mídia é lida na inicialização e fica na memória. Um certificado de cliente ou um arquivo de CA renovado também não precisa de reinicialização: o servidor lê os arquivos de novo quando mudam ([Plugins](plugins.md#authentication)).

<!-- source: the-contract 6183eb7a7dfa -->
### O contrato

As RPCs são as de `my.plugin.key_manager.v1.KeyManagerService`, com um prazo padrão de 5 segundos e mensagens de no máximo 128 KiB em cada direção.

```text
POST /my.plugin.key_manager.v1.KeyManagerService/Describe
{}
{"outcome":"OUTCOME_OK","keyId":"transit/my-kek-3"}

POST /my.plugin.key_manager.v1.KeyManagerService/WrapKey
{"dek":"<base64>","aad":"<base64>"}
{"outcome":"OUTCOME_OK","keyId":"transit/my-kek-3","wrapped":"<base64>"}

POST /my.plugin.key_manager.v1.KeyManagerService/UnwrapKey
{"keyId":"transit/my-kek-3","wrapped":"<base64>","aad":"<base64>"}
{"outcome":"OUTCOME_OK","dek":"<base64>"}
```

- `keyId` é o ID próprio do plugin, sem `plugin:`. `UnwrapKey` pode nomear um ID mais antigo que o atual.
- `wrapped` tem de 1 a 65.535 bytes e não deve conter a chave. Dois envolvimentos de uma chave diferem.
- `aad` são os dados autenticados que contêm IDs de linha e nomes de coluna, e `UnwrapKey` deve falhar a menos que seja o que `WrapKey` recebeu. É isso que impede que um segredo seja copiado para outra linha.
- **Nunca registre `dek`, `aad` ou `wrapped` em log.**

| `outcome` | Significado |
| --- | --- |
| `OUTCOME_OK` | A chamada foi bem-sucedida |
| `OUTCOME_UNKNOWN_KEY` | `UnwrapKey`: o plugin não possui a KEK de `keyId` |
| `OUTCOME_DECRYPT` | `UnwrapKey`: `wrapped` ou `aad` não autentica |

Responda com um erro HTTP ou Connect quando não puder responder, como quando o armazenamento de chaves está fora de alcance. Não responda `OUTCOME_DECRYPT` nem `OUTCOME_UNKNOWN_KEY` nesse caso: para o servidor, eles dizem que o próprio valor é inválido. O servidor não repete uma chamada por conta própria, então um plugin pode receber a mesma chamada de novo a partir de um job.

<!-- source: move-between-the-local-kek-and-a-kms 81a909a14c05 -->
## Migrar entre a KEK local e um KMS

Um ID de chave começa com o tipo de gerenciador de chaves que o envolveu (`local:`, `gcpkms:`, `awskms:`, `plugin:`). O servidor envolve os novos segredos com o `key_manager` atual
e desenvolve cada segredo armazenado com o gerenciador de chaves que o ID da chave dele nomeia, então migrar entre a KEK local e um KMS é uma rotação entre tipos.

- Com um KMS, `kek` e `kek_previous` apenas decifram: eles abrem os segredos ainda envolvidos com a KEK local, e todo novo segredo é envolvido com `kms_key`.
  `kms_key_previous` lista chaves anteriores do mesmo KMS, que também apenas decifram.
- Com `key_manager = local`, `kms_key` e `kms_key_previous`, quando definidos, apenas decifram: eles abrem os segredos ainda envolvidos com o KMS.
- Da KEK local para um KMS:
  1. Crie a chave e conceda as permissões como em [Google Cloud KMS](#google-cloud-kms) ou [AWS KMS](#aws-kms).
  2. Reinicie todos os processos com `key_manager = gcpkms` ou `awskms`, `kms_key` definido com a chave e a KEK atual movida de `kek` para `kek_previous`.
     Os segredos agora são envolvidos com o KMS, e os que foram envolvidos com a KEK local continuam abrindo.
  3. Execute `my kek rewrap` com a mesma configuração.
  4. Quando `my kek status` não contar mais segredos sob um ID de chave `local:`, remova `kek_previous` e reinicie. Guarde o backup da KEK enquanto você mantiver backups do banco de dados feitos antes da etapa 3.
- De um KMS de volta para a KEK local, faça o mesmo no sentido contrário: defina `key_manager = local` e uma nova `kek`, e mantenha `kms_key` definido, que agora apenas decifra,
  até que `my kek rewrap` tenha movido todos os segredos e `my kek status` não conte mais segredos sob um ID de chave `gcpkms:` ou `awskms:`.
  As chaves em `kms_key` e `kms_key_previous` devem então pertencer todas ao mesmo KMS.
- Para migrar do KMS de uma nuvem para o de outra, migre primeiro para a KEK local e depois para o outro KMS; não há migração direta.
- Um plugin migra da mesma forma, passando apenas pela KEK local:
  - Para ele, reinicie todos os processos com `key_manager = plugin`, `plugin_key_manager_address` e a KEK atual movida de `kek` para `kek_previous`, e então execute `my kek rewrap`. Quando `my kek status` não contar mais segredos sob um ID de chave `local:`, remova `kek_previous` e reinicie.
  - A partir dele, reinicie todos os processos com `key_manager = local`, uma nova `kek` e `plugin_key_manager_address` mantido, que agora apenas decifra, e então execute `my kek rewrap`. Quando `my kek status` não contar mais segredos sob um ID de chave `plugin:`, remova o endereço e reinicie. Até lá, o plugin ainda precisa passar [as verificações na inicialização](#when-the-server-starts), que incluem um envolvimento.
- O servidor mantém por `kms_dek_cache_ttl` (padrão `10m`, no máximo `kms_dek_cache_entries`, padrão `1024`) uma chave de criptografia de dados que o KMS ou o plugin decifrou,
  então um segredo em uso chega a ele cerca de uma vez por período e por processo. `0` em qualquer uma das configurações desativa o cache.

<!-- source: if-the-kek-leaks be6e788e4cea -->
## Se a KEK vazar

Se apenas a KEK vazou, rotacione-a como acima. Se a KEK e o banco de dados podem ter vazado, a rotação não protege você:
as chaves privadas dos atores no banco de dados já estão legíveis, então as próprias chaves dos atores precisam ser geradas novamente.

<!-- source: web-push-vapid-key aa32321d1e4a -->
## Chave VAPID do Web Push

O Web Push é assinado com a chave VAPID do servidor (P-256, RFC 8292). É um segredo separado da KEK e não é armazenado no banco de dados.

- Crie um par de chaves com `my config vapid-key` e dê a todos os processos a chave privada (o valor de `vapid_private_key = ...`) por meio de `MY_VAPID_PRIVATE_KEY` ou `MY_VAPID_PRIVATE_KEY_FILE`.
  O servidor deriva a chave pública da chave privada e a entrega aos clientes, então você não a configura.
- Se você não a definir, o Web Push fica desativado e os clientes não o oferecem. No perfil `production`, `my config check` e a inicialização de `my serve` avisam sobre isso.
- Os navegadores vinculam uma assinatura à chave pública, então alterar a chave interrompe a entrega para as assinaturas existentes. Quando um usuário ativa as notificações push de novo nas configurações, o cliente se inscreve novamente com a nova chave.
  Não altere a chave a menos que ela tenha vazado; se vazar, passe para uma nova chave e peça aos usuários que ativem as notificações de novo.
