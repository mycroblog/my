<!-- source: translating-posts-with-a-translation-service bf15671669b4 -->
# Traduzindo publicações com um serviço de tradução

O servidor pode traduzir uma publicação quando um leitor pede, por meio de um serviço de tradução que você configura: LibreTranslate,
DeepL, Google Cloud Translation ou Amazon Translate. Um provedor é incorporado quando o servidor tem as suas chaves, e a
[configuração da instância](instance-settings.md) `translation_providers` o ativa.

<!-- source: libretranslate 0d5e448bb450 -->
## LibreTranslate

LibreTranslate é um sistema de tradução automática de código aberto que você pode executar por conta própria, ou usar como
serviço hospedado em [libretranslate.com](https://libretranslate.com). Aponte o servidor para ele com duas configurações da
[configuração](../reference/configuration.md):

```sh
MY_TRANSLATOR_LIBRETRANSLATE_URL=http://libretranslate:5000
# Only when the server asks for a key, as libretranslate.com does:
MY_TRANSLATOR_LIBRETRANSLATE_API_KEY=your-key
```

- `translator_libretranslate_url` é a URL base do servidor LibreTranslate, com `http:` ou `https:` e um caminho
  opcional. Um servidor em um endereço privado funciona, como um contêiner ao lado deste ou `http://10.0.0.5:5000`.
  O servidor não segue redirecionamentos vindos dele, então a chave não vai para nenhum outro lugar.
- `translator_libretranslate_api_key` é um segredo, lido somente do ambiente
  (`MY_TRANSLATOR_LIBRETRANSLATE_API_KEY` ou `MY_TRANSLATOR_LIBRETRANSLATE_API_KEY_FILE`). Deixe-a sem definir quando o
  servidor não precisar de chave; um servidor próprio geralmente não precisa. A chave nunca é mostrada aos clientes nem
  gravada nos logs.

Depois, adicione `libretranslate` à configuração da instância `translation_providers`, na ordem em que quiser que ele seja
tentado, como `translation_providers = libretranslate` sozinho, ou seguido do ID de outro provedor para usá-lo como alternativa.

O servidor pede ao LibreTranslate a lista de idiomas dele antes da primeira tradução e de novo a cada hora, e não envia uma
publicação em um par de idiomas que ele não lista. Quando uma publicação não tem idioma, o LibreTranslate o detecta.
Instale os modelos de idioma de que os seus leitores precisam em um servidor que você mantém.

**A qualidade da tradução varia conforme o idioma.** Os modelos do LibreTranslate são bons para alguns pares e precários para
outros, e uma tradução de um idioma com poucos dados de treinamento pode perder o sentido. Experimente os idiomas que a sua
comunidade usa antes de ativar, e mantenha um provedor comercial depois dele em `translation_providers` se precisar de um.

**Para onde vai o texto.** Um servidor que você mantém guarda o texto das publicações nas suas próprias máquinas, e ninguém
mais o recebe. Nesse caso, você pode permitir publicações `private` e `direct` com `translation_provider_visibilities`, como
`libretranslate=public|local|unlisted|private|direct`, e a
[política de privacidade](privacy-policy-template.md#the-services-you-choose) não lista nenhum serviço externo para tradução. Um
LibreTranslate hospedado, o libretranslate.com ou o de outra pessoa, recebe o texto como qualquer outro provedor: leia os termos
dele, mantenha as visibilidades padrão e cite-o na política.

<!-- source: deepl 80449fb800f7 -->
## DeepL

O DeepL traduz pela sua API com uma chave de um plano de API do DeepL. O servidor envia o texto de uma publicação com as marcas
que substituem menções, links e formatação como elementos XML, que o DeepL mantém no lugar, e nada mais da publicação.

1. Obtenha uma chave de autenticação na sua conta do DeepL. Uma chave do plano gratuito termina em `:fx`.
2. Defina-a na [configuração de implantação](../reference/configuration.md) e reinicie o servidor:

   ```sh
   MY_TRANSLATOR_DEEPL_API_KEY=your-key
   ```

   O servidor chama `https://api-free.deepl.com` para uma chave do plano gratuito e `https://api.deepl.com` para qualquer outra.
   Para passar por um proxy, defina `MY_TRANSLATOR_DEEPL_URL`. A chave é um segredo: o servidor nunca a registra em log, não a
   coloca em um erro nem a mostra em uma configuração ou resposta da API cliente, e a envia somente a esse endereço, sem seguir
   redirecionamentos.
3. Ative o provedor pela [configuração da instância](instance-settings.md) `translation_providers`, sozinho ou na sua ordem:

   ```sh
   my instance set translation_providers deepl
   ```

O servidor pergunta ao DeepL quais idiomas ele traduz quando faz a primeira tradução, e de novo a cada dia. O idioma de uma
publicação vai para a variante mais próxima que o DeepL tem: inglês para `en-US`, português para `pt-BR`, chinês de Taiwan para
`zh-Hant`. Um idioma que o DeepL não tem passa para o próximo provedor, ou o leitor é informado de que ele não pode ser traduzido.

**Cota.** Cada plano tem uma cota mensal de caracteres, e um plano pago pode ter um limite de custo que você define na sua conta
do DeepL. Quando o DeepL informa a cota ou o seu limite de taxa, o servidor tenta o próximo provedor de `translation_providers`.
Sem nenhum restante, o leitor vê que a publicação não pôde ser traduzida agora, e o cliente web usa a tradução do navegador
quando você permite. Os limites mensais do próprio servidor (`translation_user_monthly_characters` e
`translation_server_monthly_characters`) valem antes de qualquer chamada, então defina-os abaixo da sua cota do DeepL. O DeepL
pode contar os caracteres de forma diferente deles.

**Dados.** Leia os [termos do DeepL](https://www.deepl.com/en/pro-license) antes de ativá-lo:

- Em um plano pago, o DeepL guarda o texto e as traduções apenas pelo tempo necessário para traduzi-los, e por no máximo 72
  horas quando investiga um erro.
- No plano gratuito, o DeepL pode guardar o texto e as traduções para sempre, e os termos dele proíbem enviar dados pessoais
  por ele. As publicações costumam conter dados pessoais, então use o plano gratuito apenas para testar o provedor. Nunca
  permita publicações `private` ou `direct` nele em `translation_provider_visibilities`, e diga na sua política de privacidade
  que o DeepL guarda o que recebe.

Em um plano pago, você pode permitir publicações `private` e `direct`, provedor por provedor:

```sh
my instance set translation_provider_visibilities 'deepl=public|local|unlisted|private'
```

**Atribuição.** Os termos do DeepL exigem que o nome ou o logotipo dele apareça com cada tradução. O cliente web mostra
“DeepL” com um link para `https://www.deepl.com` ao lado de cada tradução vinda dele, então você não precisa acrescentar nada.

<!-- source: google-cloud-translation e351f57274b0 -->
## Google Cloud Translation

[Cloud Translation](https://cloud.google.com/translate) tem o ID de provedor `google`. O servidor chama o método `TranslateText`
dele com o modelo de tradução geral, e envia o texto como HTML para que as menções, os links e as outras partes que precisam
permanecer como estão voltem no lugar.

1. Ative a API Cloud Translation (`translate.googleapis.com`) no projeto que paga as traduções:
   `gcloud services enable translate.googleapis.com --project PROJECT`.
2. Conceda à conta de serviço do servidor `roles/cloudtranslate.user` nesse projeto:
   `gcloud projects add-iam-policy-binding PROJECT --member serviceAccount:SA_EMAIL --role roles/cloudtranslate.user`.
3. Defina `MY_TRANSLATOR_GOOGLE_PROJECT` com o ID do projeto. `MY_TRANSLATOR_GOOGLE_LOCATION` é `global` por padrão; ele pode
   indicar outra localização do Cloud Translation que sirva o modelo geral, como `us-central1`.
4. Reinicie o servidor e adicione `google` à configuração da instância `translation_providers`.

O servidor se autentica com Application Default Credentials, como os outros serviços do Google Cloud que ele usa: no Cloud Run,
a conta de serviço do serviço; em outros lugares, o arquivo de chave que `GOOGLE_APPLICATION_CREDENTIALS` indica. Nenhuma
chave de API é usada. O servidor não faz nenhuma chamada na inicialização, então uma API ou uma função ausente só aparece quando
um leitor pede uma tradução: o servidor então tenta o próximo provedor, e o leitor vê que a tradução falhou quando não resta
nenhum. Uma cota ou um limite de taxa do projeto que o Google informa também passa para o próximo provedor.

O Google cobra por caractere enviado, depois de uma franquia gratuita mensal; veja
[preços do Cloud Translation](https://cloud.google.com/translate/pricing). Os limites mensais do próprio servidor também contam
os caracteres, mas não exatamente como o Google os cobra, então configure também um alerta de orçamento no Cloud Billing.

O Google [declara](https://cloud.google.com/translate/data-usage) que usa o texto apenas para fornecer a tradução, o mantém na
memória só por pouco tempo, não o usa para treinar os seus modelos e não o compartilha. O servidor chama o endpoint global do
Cloud Translation, em que o Google não restringe o processamento a uma única região.

Os [requisitos de atribuição](https://cloud.google.com/translate/attribution) do Google pedem que uma tradução seja exibida
como tradução automática e que o selo “com tecnologia do Google Tradutor” apareça ao lado dela, com link para
`https://translate.google.com`. O cliente web mostra o nome “Google Tradutor” com esse link ao lado de cada tradução desse
provedor. O Google oferece as imagens do selo apenas para download, então o servidor não fornece aos clientes nenhum logotipo
para esse provedor; verifique se o que os leitores veem atende aos requisitos do Google para a sua instância.

<!-- source: amazon-translate a6cd83df7ef1 -->
## Amazon Translate

O Amazon Translate é o provedor `aws`. O servidor chama a operação `TranslateText` dele com as credenciais da cadeia padrão do
AWS SDK, como para os outros serviços da AWS: o perfil de execução da tarefa ou da instância, as variáveis de ambiente
`AWS_ACCESS_KEY_ID` e `AWS_SECRET_ACCESS_KEY`, ou os arquivos de credenciais compartilhadas. Nenhuma chave entra na
configuração do servidor.

1. Conceda ao perfil ou usuário com que o servidor roda a permissão IAM `translate:TranslateText`. Para uma publicação cujo
   idioma é desconhecido, o Amazon Translate detecta o idioma com o Amazon Comprehend; as políticas gerenciadas pela AWS para o
   Amazon Translate concedem `comprehend:DetectDominantLanguage` junto com ela, então conceda essa também.
2. Defina `translator_aws_region` (`MY_TRANSLATOR_AWS_REGION`) com a região do Amazon Translate, como `us-east-1`. Detectar o
   idioma exige uma região em que o Amazon Comprehend também funcione.
3. Reinicie o servidor e, depois, ative o provedor com `translation_providers = aws`, sozinho ou na sua ordem de preferência
   entre os outros ([configurações da instância](instance-settings.md)).

O Amazon Translate cobra por caractere enviado, com um nível gratuito nos primeiros 12 meses
([preços](https://aws.amazon.com/translate/pricing/)). O servidor envia apenas as palavras de uma publicação: menções,
hashtags, links e código ficam no servidor, e cada um deles vai como um marcador curto dentro da marcação HTML. Mantenha os
limites mensais de `translation_user_monthly_characters` e `translation_server_monthly_characters` dentro do que você quer pagar.

**A Amazon pode guardar o texto e usá-lo para melhorar os seus serviços, possivelmente em outra região, a menos que a política de
exclusão de serviços de IA do AWS Organizations
([AI services opt-out policy](https://docs.aws.amazon.com/organizations/latest/userguide/orgs_manage_policies_ai-opt-out.html))
cubra a conta.** Sem essa exclusão, a Amazon não é apenas o seu provedor de serviço para esse texto: diga isso na sua política de
privacidade ([modelo de política de privacidade](privacy-policy-template.md#the-services-you-choose)), e não permita
`private` nem `direct` para `aws` em `translation_provider_visibilities`. Por padrão, o provedor recebe apenas publicações
`public`, `local` e `unlisted`.

Um servidor compilado com a build tag `no_translator_aws` não tem o provedor e se recusa a iniciar com
`translator_aws_region` definido ([build tags](../reference/build-tags.md)).
