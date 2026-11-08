<!-- source: captcha-bot-protection-turnstile--hcaptcha--recaptcha 2c065ac1d2fb -->
# Proteção contra bots com CAPTCHA (Turnstile / hCaptcha / reCAPTCHA)

O CAPTCHA vem desativado por padrão: a configuração [`captcha`](../reference/configuration.md#captcha) é `none`. As páginas de login, de cadastro e de redefinição de senha então funcionam sem JavaScript e não enviam nada a terceiros.
Quando você escolhe um serviço de CAPTCHA, o widget do serviço aparece em três formulários: entrar (`/auth/sign_in`), cadastro (`/auth/sign_up`) e o pedido de link de redefinição de senha (`/auth/forgot_password`, apenas em servidores que enviam e-mail).
O servidor verifica o token que o widget produz com a API siteverify do serviço. Não é possível proteger apenas alguns dos formulários.

<!-- source: choosing-a-service 22ddd3cc639f -->
## Escolhendo um serviço

| `captcha` | Serviço | Custo | O que o usuário faz |
| --- | --- | --- | --- |
| `turnstile` (recomendado) | Cloudflare Turnstile | Gratuito, sem limite | Geralmente nada; uma caixa de seleção só quando necessário |
| `hcaptcha` | hCaptcha | Tem um nível gratuito | Uma caixa de seleção, e um desafio de imagem quando necessário |
| `recaptcha` | Google reCAPTCHA v2 (caixa de seleção) | Gratuito até 10.000 avaliações por mês (veja abaixo) | Uma caixa de seleção, e um desafio de imagem ou de áudio quando necessário |

reCAPTCHA v3 (apenas pontuação) e reCAPTCHA Enterprise não são suportados.

<!-- source: creating-keys 38cd93394638 -->
## Criando as chaves

Cada serviço fornece um par de chaves: uma **chave de site** pública e uma **chave secreta** que só o servidor possui. Cadastre o nome do host da configuração `host` (sem a porta) como domínio.
O servidor rejeita um token quando o nome do host na resposta siteverify não corresponde a `host`.

- **Turnstile**: no painel da Cloudflare, adicione um widget em "Turnstile" e cadastre o nome do host. Escolha o modo de widget "Managed".
- **hCaptcha**: adicione um site no painel do hCaptcha e cadastre o nome do host. A chave secreta fica nas configurações da conta.
- **reCAPTCHA**: no console de administração do Google reCAPTCHA, escolha o tipo "Challenge (v2)" com a caixa "I'm not a robot" e cadastre o domínio.
  As chaves pertencem a um projeto do Google Cloud.

As chaves de teste de cada serviço (as que sempre têm sucesso) servem para verificar se o widget aparece e produz um token.
No entanto, o siteverify com chaves de teste retorna um nome de host fixo (`example.com` para o Turnstile, `dummy-key-pass` para o hCaptcha, e assim por diante), então entrar e se cadastrar neste servidor falham com "Verifique novamente" quando você as usa.

<!-- source: configuring 3301db5c6286 -->
## Configurando

```sh
MY_CAPTCHA=turnstile
MY_CAPTCHA_SITE_KEY=0x4AAAAAAA...
MY_CAPTCHA_SECRET_KEY_FILE=/run/secrets/my_captcha_secret_key
```

- Passe a chave secreta apenas pela variável de ambiente `MY_CAPTCHA_SECRET_KEY` ou por `MY_CAPTCHA_SECRET_KEY_FILE`, que aponta para um arquivo. Os logs mostram `[REDACTED]`.
- Se você escolher um `captcha` e uma chave estiver faltando, `my config check` reporta um problema e o servidor não inicia.
- O servidor não testa as chaves na inicialização. Quando o serviço recusa a chave secreta ou a chave de site, a primeira verificação registra o ERROR
  "the CAPTCHA service refused the secret key or the site key". Até você corrigir, as requisições são tratadas como em "Quando o serviço está inacessível", abaixo.
- As mudanças passam a valer quando você reinicia o servidor.

<!-- source: how-verification-works c5cc12dd3769 -->
## Como a verificação funciona

- A ordem é: limite de taxa, depois a verificação do CAPTCHA, depois a checagem da senha, o processamento do cadastro ou o envio do e-mail de redefinição. Uma requisição acima do limite de taxa nunca é enviada ao serviço.
- Um cadastro com um código de convite válido também é verificado.
- Um login que falha na verificação conta como uma tentativa falha para o endereço IP do cliente. Não conta contra a conta, então ninguém consegue trancar outra pessoa para fora.
  Um cadastro que falha na verificação conta como uma tentativa de cadastro (veja o limite de taxa em [cadastro](registration.md)). Um pedido de link de redefinição conta como uma tentativa de pedido da mesma forma.
- Para o Turnstile, o widget carrega um nome de ação (`sign_in` / `sign_up` / `password_reset`) e o servidor verifica se a resposta tem a mesma ação. Um token de um formulário não pode ser usado em outro.
- O servidor não envia o endereço IP do usuário (`remoteip`) ao serviço. Um token é de uso único, então o servidor não repete a chamada. O tempo limite é de 10 segundos.

<!-- source: when-the-service-is-unreachable fe6a6d5915ea -->
### Quando o serviço está inacessível

O serviço está inacessível, excede o tempo limite, retorna um erro 5xx ou recusa as chaves:

| Formulário | Tratamento |
| --- | --- |
| Cadastro e pedido de link de redefinição | Rejeitado (`503`) com "Tente novamente mais tarde". Do contrário, bots poderiam disparar e-mails e cadastros. |
| Login | Deixado passar sem CAPTCHA, e o servidor registra o WARN "the CAPTCHA service could not check a sign-in; letting it through". O limite de taxa e a verificação da senha continuam valendo. |

O login é deixado passar para que uma falha do serviço ou uma chave errada não tranque todo mundo para fora, administradores inclusive. Se o WARN continuar aparecendo, verifique o status do serviço e as chaves.

<!-- source: recaptcha-free-tier 4f413b07dbb9 -->
### Nível gratuito do reCAPTCHA

Acima do nível gratuito de 10.000 avaliações por mês, o siteverify do reCAPTCHA **sempre tem sucesso** (então o CAPTCHA não oferece proteção nesse período).
O servidor não consegue detectar o excedente, então verifique o seu uso no console do Google Cloud.

<!-- source: pages-and-content-security-policy b0cf9741b2ce -->
## Páginas e Content-Security-Policy

- O idioma do widget segue o locale da página, e o tema segue o esquema de cores do navegador (claro ou escuro).
  O widget do Turnstile segue o próprio esquema de cores. hCaptcha e reCAPTCHA são escolhidos pelo client hint `Sec-CH-Prefers-Color-Scheme`, que o navegador envia; navegadores que não o enviam recebem o tema claro.
- Ao lado do widget, a página informa qual serviço faz a verificação, que o serviço recebe o endereço IP e as informações do navegador, e traz um link para a política de privacidade do serviço (e para os termos do hCaptcha e do reCAPTCHA). Para o reCAPTCHA, isso também serve como a frase que o Google exige: "This site is protected by reCAPTCHA and the Google Privacy Policy and Terms of Service apply."
- Em um navegador sem JavaScript, uma mensagem `<noscript>` diz que a verificação precisa de JavaScript. Um usuário que não consegue resolver o CAPTCHA não consegue se cadastrar nem entrar, e você só pode ajudá-lo por outros meios, como criar a conta com `my admin create` ou definir `captcha = none` por um tempo.
- O `Content-Security-Policy` (CSP) só se amplia nas páginas de login, de cadastro e de pedido de redefinição de senha enquanto o CAPTCHA está ativo. `'unsafe-inline'` e `'unsafe-eval'` nunca são adicionados.
  A CSP do cliente web, da API de cliente, da tela de consentimento, da confirmação de e-mail e de outras páginas não muda.

| `captcha` | Fontes adicionadas |
| --- | --- |
| `turnstile` | `script-src`, `frame-src`: `https://challenges.cloudflare.com` |
| `hcaptcha` | `script-src`, `frame-src`, `style-src`, `connect-src`: `https://hcaptcha.com`, `https://*.hcaptcha.com` |
| `recaptcha` | `script-src`: `https://www.google.com/recaptcha/`, `https://www.gstatic.com/recaptcha/`; `frame-src`: `https://www.google.com/recaptcha/`, `https://recaptcha.google.com/recaptcha/`; `connect-src`: `https://www.google.com/recaptcha/` |

Se um proxy reverso ou um CDN na frente do servidor sobrescrever a CSP, adicione as mesmas fontes nessas três páginas.

<!-- source: what-to-write-in-the-privacy-policy d601071ebd4c -->
## O que escrever na política de privacidade

Com o CAPTCHA ativado, o navegador de quem abre a página de login, de cadastro ou de pedido de redefinição de senha se conecta diretamente ao serviço, e o serviço recebe o endereço IP, as informações do navegador e os cookies.
A política de privacidade da instância (`my instance set privacy_policy`, veja [configurações da instância](instance-settings.md)) é de sua responsabilidade; o servidor não acrescenta nada a ela. O [modelo de política de privacidade](privacy-policy-template.md) tem uma linha para o serviço. Escreva pelo menos o seguinte.

- Qual serviço (o nome do provedor) você usa e por quê: para impedir login, cadastro e pedidos de redefinição de senha automatizados.
- O que chega ao serviço (endereço IP, informações de navegador e dispositivo, cookies, interações com o widget), e que somente as páginas de login, de cadastro e de pedido de redefinição de senha o enviam.
- Um link para a política de privacidade do serviço (Turnstile: <https://www.cloudflare.com/privacypolicy/>, hCaptcha: <https://www.hcaptcha.com/privacy>, reCAPTCHA: <https://policies.google.com/privacy>).
- Para o reCAPTCHA, que a política de privacidade e os termos do Google (<https://policies.google.com/terms>) se aplicam.
