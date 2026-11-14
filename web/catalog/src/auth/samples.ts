// SPDX-License-Identifier: AGPL-3.0-only

// The sample data of the auth screens' preview: what the server's shell would carry for each screen and state
// (internal/interface/authpages), on a server with no logo of its own and on one with its own logo and banner. What
// the server words (a form's outcome, the consent's permissions, a moderation action) is sampled in en and ja.

import type { AuthPage } from "@my/app-features";

const svg = (body: string, w: number, h: number) =>
  `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}">${body}</svg>`)}`;

type Lang = "en" | "ja";

/** A sample of what the server words, in the preview's language. */
type Worded = Record<Lang, string>;

const plain = {
  name: "my.example",
  description: {
    en: "A small server for friends who like books, walks and slow mornings. Everyone is welcome to read along.",
    ja: "本と散歩とゆっくりした朝が好きな仲間のための小さなサーバーです。どなたでも気軽に読みに来てください。",
  },
};

const branded = {
  name: "Hanami Town",
  logo: svg(
    '<rect width="64" height="64" rx="14" fill="#2f6f5e"/><circle cx="32" cy="22" r="9" fill="#f4efe6"/><circle cx="22" cy="36" r="9" fill="#f4efe6"/><circle cx="42" cy="36" r="9" fill="#f4efe6"/><circle cx="32" cy="31" r="5" fill="#e3b341"/>',
    64,
    64,
  ),
  banner: svg(
    '<rect width="600" height="200" fill="#cfe3d8"/><path d="M0 140 Q150 80 300 130 T600 110 V200 H0Z" fill="#7fb59c"/><path d="M0 170 Q200 120 400 165 T600 150 V200 H0Z" fill="#2f6f5e"/><circle cx="480" cy="55" r="22" fill="#f4efe6"/>',
    600,
    200,
  ),
  description: {
    en: "A town square for people around the river park: events, lost and found, and photos of the seasons.",
    ja: "川沿いの公園のまわりに暮らす人の広場です。催し、落とし物、季節の写真をどうぞ。",
  },
};

const words = {
  signInFailed: {
    en: "The email address or password is incorrect.",
    ja: "メールアドレスまたはパスワードが正しくありません。",
  },
  signInLocked: {
    en: "Too many failed attempts: try again in 15 minutes.",
    ja: "失敗が多すぎるため、15 分後にもう一度試してください。",
  },
  passkeyFailed: {
    en: "That passkey could not sign you in. Try another one or use your password.",
    ja: "そのパスキーではログインできませんでした。別のパスキーかパスワードを使ってください。",
  },
  providersOnly: {
    en: "This server signs in only through the login providers. If your account has none linked, ask the administrators.",
    ja: "このサーバーではログインプロバイダーでのみログインできます。アカウントにプロバイダーを連携していない場合は、管理者にお問い合わせください。",
  },
  wrongCode: { en: "The code is incorrect or was already used.", ja: "コードが正しくないか、すでに使われています。" },
  usernameHint: {
    en: "Up to 30 letters (a to z), digits, and underscores. It cannot be changed later.",
    ja: "半角英字 (a〜z)・数字・アンダースコアで 30 文字まで。あとから変更できません。",
  },
  passwordHint: { en: "8 to 128 characters.", ja: "8〜128 文字。" },
  passwordInvalid: { en: "Use 8 to 128 characters.", ja: "8〜128 文字にしてください。" },
  inviteHint: {
    en: "If someone invited you, enter the invite code. You can also sign up without one.",
    ja: "招待を受けた人は招待コードを入力してください。招待コードなしでも登録できます。",
  },
  reasonHint: {
    en: "A moderator reviews new accounts before they can be used. Up to 500 characters.",
    ja: "新しいアカウントは、モデレーターの承認後に使えるようになります。500 文字まで。",
  },
  usernameTaken: { en: "This username is already taken.", ja: "このユーザー名はすでに使われています。" },
  agreementRequired: {
    en: "Agree to the documents above to create an account.",
    ja: "アカウントを作成するには、上の文書に同意してください。",
  },
  signUpSummary: {
    en: "Fix the marked fields to create the account.",
    ja: "印の付いた項目を直すと、アカウントを作成できます。",
  },
  emailVerified: {
    en: "GitHub confirmed this address. If you change it, we send a link to confirm the new one.",
    ja: "GitHub で確認済みのアドレスです。変えた場合は、新しいアドレスに確認のためのリンクを送ります。",
  },
  humanNotice: {
    en: "This page uses Cloudflare Turnstile to check that you are not a bot. Cloudflare receives your IP address and information about your browser.",
    ja: "このページは Cloudflare Turnstile でボットかどうかを確認します。Cloudflare にはあなたの IP アドレスとブラウザの情報が送られます。",
  },
  humanPrivacy: { en: "Cloudflare Privacy Policy", ja: "Cloudflare のプライバシーポリシー" },
  forgotSent: {
    en: "If an account uses alice@example.com, a link to set a new password is on its way. The link works for 60 minutes. If no email arrives, check the spam folder and the address.",
    ja: "alice@example.com を使っているアカウントがあれば、新しいパスワードを設定するリンクを送りました。リンクの有効期限は 60 分です。メールが届かないときは、迷惑メールのフォルダーとアドレスを確かめてください。",
  },
  forgotLocked: {
    en: "Too many requests from your network: try again in 10 minutes.",
    ja: "このネットワークからの要求が多すぎるため、10 分後にもう一度試してください。",
  },
  verifyInvalid: {
    en: "This link is not valid. It may have expired or been used already. Sign in to get a new one.",
    ja: "このリンクは使えません。期限切れか、すでに使われた可能性があります。ログインすると新しいリンクを受け取れます。",
  },
  resetInvalid: {
    en: "This link is not valid. It may have expired, been used already, or been replaced by a newer password. Ask for a new link.",
    ja: "このリンクは使えません。期限切れか、すでに使われたか、その後にパスワードが変わった可能性があります。新しいリンクを受け取ってください。",
  },
  formExpired: {
    en: "This form has expired. Please try again.",
    ja: "フォームの有効期限が切れました。もう一度試してください。",
  },
  unverified: {
    en: "This server has not verified this application. Its name is the application's own claim.",
    ja: "このサーバーが確認していないアプリです。名前はアプリ自身の申告です。",
  },
  origin: { en: "Comes from tusky.example", ja: "出どころ: tusky.example" },
  website: { en: "Website it declared: https://tusky.example", ja: "申告されたサイト: https://tusky.example" },
  impersonation: {
    en: "After you allow, the answer goes to tusky, which other apps on your device can also claim. It may be a different app from the one named above. Allow only if you started this from that app.",
    ja: "許可すると、答えは tusky に送られます。これはあなたの端末のほかのアプリも名乗れる宛先で、上に出ている名前とは別のアプリかもしれません。そのアプリから始めた場合だけ許可してください。",
  },
  groupCaution: { en: "Permissions that need your attention", ja: "特に注意が必要な権限" },
  groupWrite: { en: "Permissions that change things", ja: "変更できる権限" },
  groupRead: { en: "Permissions that only read", ja: "読み取りだけの権限" },
  scopeWriteStatuses: {
    en: "Post, edit, delete, boost, and pin posts, vote in polls, and set who may quote a post",
    ja: "投稿の作成・編集・削除・ブースト・ピン留め、アンケートへの投票、引用できる相手の設定",
  },
  cautionPost: { en: "Can post, edit and delete posts as you", ja: "あなたとして投稿・編集・削除ができます" },
  scopeReadDirect: {
    en: "Read the direct messages you send and receive",
    ja: "あなたが送受信するダイレクトメッセージの読み取り",
  },
  cautionDm: { en: "Can read your direct messages", ja: "ダイレクトメッセージを読めます" },
  scopeWriteFavourites: {
    en: "Add and remove favourites and emoji reactions",
    ja: "お気に入りと絵文字リアクションの追加と削除",
  },
  scopeReadStatuses: { en: "Read posts and timelines", ja: "投稿とタイムラインの読み取り" },
  scopeReadNotifications: { en: "Read your notifications", ja: "通知の読み取り" },
  verifyEmailStatus: {
    en: "Confirm your email address: open the link we sent to alice@example.com.",
    ja: "alice@example.com に送ったリンクを開いて、メールアドレスを確認してください。",
  },
  suspended: { en: "Suspended", ja: "凍結" },
  endsIn: { en: "Ends in 6 days", ja: "あと 6 日で終わります" },
  spam: { en: "Reason: Spam", ja: "理由: スパム" },
  appealHint: {
    en: "You can appeal once, within 14 days. Up to 2,000 characters.",
    ja: "異議の申し立ては 1 回だけ、あと 14 日以内にできます。2,000 文字まで。",
  },
  deletionDate: { en: "2026-12-14 09:00 (UTC)", ja: "2026-12-14 09:00 (UTC)" },
  unsubscribePrompt: {
    en: "Stop getting emails of “Mentions” notifications? They still appear in the app.",
    ja: "「メンション」の通知のメールを止めますか？アプリ内の通知は引き続き届きます。",
  },
  unsubscribed: {
    en: "You no longer get emails of “Mentions” notifications. You can turn them back on in your notification settings.",
    ja: "「メンション」の通知のメールを止めました。通知の設定からもう一度受け取るようにできます。",
  },
  terms: { en: "Terms of service", ja: "利用規約" },
  privacy: { en: "Privacy policy", ja: "プライバシーポリシー" },
  github: { en: "Sign in with GitHub", ja: "GitHub でログイン" },
  discord: { en: "Sign in with Discord", ja: "Discord でログイン" },
} satisfies Record<string, Worded>;

type Specimen = { id: string; name: string; page: AuthPage };
export type SampleScreen = { id: string; name: string; states: Specimen[] };

/** Every screen and its states, in the order of the flow, with what the server words in `lang`. */
export function samples(lang: Lang): SampleScreen[] {
  const w = (k: keyof typeof words) => words[k][lang];
  const server = { name: plain.name, description: plain.description[lang] };
  const own = { ...branded, description: branded.description[lang] };
  const base = {
    server,
    csrfToken: "sample",
    documents: [
      { path: "/about/terms", title: w("terms") },
      { path: "/about/privacy", title: w("privacy") },
    ],
    statusPageUrl: "https://status.example",
  };
  const external = [
    { label: w("github"), url: "#github", kind: "github" },
    { label: w("discord"), url: "#discord", kind: "discord" },
  ];
  const passkey = { options: { challenge: "c2FtcGxl", rpId: "my.example" }, challenge: "sample" };
  const humanCheck = {
    scriptSrc: "",
    containerClass: "cf-turnstile",
    siteKey: "sample",
    theme: "auto",
    notice: w("humanNotice"),
    links: [{ url: "https://www.cloudflare.com/privacypolicy/", title: w("humanPrivacy") }],
  };
  const signIn = { ...base, screen: "sign_in", canSignUp: true, canResetPassword: true, passkey, external } as const;
  const signUpForm = {
    askInvite: true,
    needAgreement: true,
    usernameHint: w("usernameHint"),
    passwordHint: w("passwordHint"),
    reasonHint: w("reasonHint"),
    inviteHint: w("inviteHint"),
    usernameMax: 30,
    passwordMin: 8,
    passwordMax: 128,
    reasonMax: 500,
  };
  const signUp = { ...base, screen: "sign_up", signUp: signUpForm, humanCheck, external } as const;
  const consent = {
    clientName: "Tusky Next",
    redirectHost: "tusky.example",
    unverified: w("unverified"),
    origin: w("origin"),
    website: w("website"),
    scopeGroups: [
      {
        risk: "caution",
        title: w("groupCaution"),
        scopes: [
          { text: w("scopeWriteStatuses"), cautions: [w("cautionPost")] },
          { text: w("scopeReadDirect"), cautions: [w("cautionDm")] },
        ],
      },
      { risk: "write", title: w("groupWrite"), scopes: [{ text: w("scopeWriteFavourites") }] },
      {
        risk: "read",
        title: w("groupRead"),
        scopes: [{ text: w("scopeReadStatuses") }, { text: w("scopeReadNotifications") }],
      },
    ],
    account: { handle: "@kumi@my.example", displayName: "Kumi" },
    responseType: "code",
    clientId: "https://tusky.example/client.json",
    redirectUri: "https://tusky.example/callback",
    scope: "read write",
    state: "sample",
    codeChallenge: "sample",
    codeChallengeMethod: "S256",
  };
  const authorize = { ...base, screen: "authorize", consent } as const;
  const status = { ...base, screen: "status" } as const;
  const state = (id: string, page: AuthPage): Specimen => ({ id, name: id.replace(/-/g, " "), page });
  return [
    {
      id: "sign-in",
      name: "Sign in",
      states: [
        state("empty", signIn),
        state("filled", { ...signIn, email: "alice@example.com" }),
        state("server-error", { ...signIn, email: "alice@example.com", error: w("signInFailed") }),
        state("locked", { ...signIn, email: "alice@example.com", error: w("signInLocked") }),
        state("passkey-error", { ...signIn, passkey: { ...passkey, error: w("passkeyFailed") } }),
        state("human-check", { ...signIn, humanCheck }),
        state("providers-only", { ...base, screen: "sign_in", loginProvidersOnly: true, external }),
        state("own-logo", { ...signIn, server: own }),
      ],
    },
    {
      id: "two-factor",
      name: "Two-factor code",
      states: [
        state("empty", { ...base, screen: "two_factor" }),
        state("wrong-code", { ...base, screen: "two_factor", error: w("wrongCode") }),
      ],
    },
    {
      id: "sign-up",
      name: "Sign up",
      states: [
        state("empty", signUp),
        state("field-error", {
          ...signUp,
          error: w("signUpSummary"),
          signUp: {
            ...signUpForm,
            username: "alice",
            email: "alice@example.com",
            errors: { username: w("usernameTaken"), password: w("passwordInvalid"), agreement: w("agreementRequired") },
          },
        }),
        state("approval", { ...signUp, signUp: { ...signUpForm, askReason: true } }),
        state("external", {
          ...base,
          screen: "sign_up",
          signUp: {
            ...signUpForm,
            username: "alice",
            email: "alice@example.com",
            external: { ticket: "sample", provider: "GitHub", emailVerifiedHint: w("emailVerified") },
          },
        }),
        state("providers-only", {
          ...base,
          screen: "sign_up",
          loginProvidersOnly: true,
          signUp: { ...signUpForm, loginProvidersOnly: true },
          external,
        }),
        state("closed", { ...base, screen: "sign_up", signUp: { ...signUpForm, closed: true } }),
      ],
    },
    {
      id: "verify-email",
      name: "Verify email",
      states: [
        state("prompt", { ...base, screen: "verify_email", token: "sample" }),
        state("done", { ...base, screen: "verified", returnTo: "/login?return_to=%2Fwelcome" }),
        state("invalid", { ...base, screen: "error", error: w("verifyInvalid") }),
      ],
    },
    {
      id: "forgot-password",
      name: "Forgot password",
      states: [
        state("empty", { ...base, screen: "forgot_password", forgotPassword: {}, humanCheck }),
        state("locked", {
          ...base,
          screen: "forgot_password",
          forgotPassword: {},
          humanCheck,
          email: "alice@example.com",
          error: w("forgotLocked"),
        }),
        state("sent", { ...base, screen: "forgot_password", forgotPassword: { sent: true }, notice: w("forgotSent") }),
        state("unavailable", { ...base, screen: "forgot_password", forgotPassword: { unavailable: true } }),
      ],
    },
    {
      id: "reset-password",
      name: "Reset password",
      states: [
        state("empty", {
          ...base,
          screen: "reset_password",
          token: "sample",
          newPassword: { hint: w("passwordHint"), min: 8, max: 128 },
        }),
        state("field-error", {
          ...base,
          screen: "reset_password",
          token: "sample",
          newPassword: { hint: w("passwordHint"), error: w("passwordInvalid"), min: 8, max: 128 },
        }),
        state("done", { ...base, screen: "password_reset" }),
        state("invalid", { ...base, screen: "error", error: w("resetInvalid") }),
      ],
    },
    {
      id: "authorize",
      name: "OAuth consent",
      states: [
        state("consent", authorize),
        state("native", {
          ...authorize,
          consent: { ...consent, redirectHost: "tusky", impersonation: w("impersonation") },
        }),
        state("own-logo", { ...authorize, server: own }),
      ],
    },
    { id: "signed-in", name: "Signed in", states: [state("default", { ...base, screen: "signed_in", returnTo: "/" })] },
    {
      id: "status",
      name: "Account status",
      states: [
        state("pending", { ...status, status: { pending: true, verifyEmail: w("verifyEmailStatus") } }),
        state("restricted", {
          ...status,
          status: {
            restricted: true,
            actions: [
              {
                id: "1",
                kind: w("suspended"),
                category: w("spam"),
                ends: w("endsIn"),
                endsAt: "2026-11-20T09:00:00Z",
                canAppeal: true,
                appealHint: w("appealHint"),
                appealMax: 2000,
              },
            ],
          },
        }),
        state("deactivated", {
          ...status,
          status: {
            deactivated: true,
            deletionDate: w("deletionDate"),
            deletionAt: "2026-12-14T09:00:00Z",
            deleteAsksForPassword: true,
          },
        }),
      ],
    },
    { id: "error", name: "Error", states: [state("default", { ...base, screen: "error", error: w("formExpired") })] },
    {
      id: "account-deleted",
      name: "Account deleted",
      states: [state("default", { ...base, screen: "account_deleted" })],
    },
    {
      id: "unsubscribe",
      name: "Mail unsubscribe",
      states: [
        state("prompt", {
          ...base,
          screen: "mail_unsubscribe",
          token: "sample",
          unsubscribe: { prompt: w("unsubscribePrompt") },
        }),
        state("done", { ...base, screen: "mail_unsubscribed", unsubscribe: { done: w("unsubscribed") } }),
      ],
    },
  ];
}
