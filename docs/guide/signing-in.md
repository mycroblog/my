# Signing up and signing in

How to create an account on a mycroblog server, a server that runs `my`, and sign in to it. Each server is run by its own operators, so what a server
offers (who may sign up, which timelines visitors can read, whether voice chat is on) is up to them. The server's
"About" page and the documents it links at sign-up tell you its rules; this guide only describes how `my` behaves.

## Creating an account

Choose "Sign up" in the navigation (or "Create an account" on the sign-in page). The form asks for:

- **Username**: letters (a-z), digits, and underscores. Your handle is `@username@this-server`, and you cannot change the username later.
- **Email address**: you sign in with it.
- **Password**: the form shows the allowed length under the field.
- **Invite code**, when the server asks for one or accepts one.
- **Why do you want to join? (optional)**, when the server reviews new accounts.
- A box to confirm that you have read the server's terms and policies, when the server publishes any.

Which of these you see depends on how the server accepts new accounts:

| The server | What you do |
| --- | --- |
| Is open | Sign up. An invite code is optional. |
| Reviews new accounts | Sign up and say why you want to join. A moderator approves the account before you can use it. An invite code lets you use the account at once. |
| Accepts invitations only | Enter the invite code you were given, or open the invite link, which fills it in. |
| Is closed | The page says that the server is not accepting new accounts. |
| Registers through login providers only | You sign up with a provider the server offers (for example "Sign in with GitHub") instead of a password. |

If the server sends email, it sends a link to confirm your email address. Open it and choose "Confirm email address".
Until the account is approved and the address is confirmed, you can sign in but you only see what you are waiting for
("Your account is not ready yet").

After sign-up, the app offers to set a display name and an avatar, which you can skip; while you follow no one, your
home timeline then shows people the server suggests, if it suggests any.

## Signing in

Choose "Sign in" in the navigation. The web client sends you to the server's sign-in page and brings you back signed
in. On that page you can:

- Enter your email address and password.
- Choose "Sign in with a passkey", once you have added a passkey under "Passkeys" in the settings.
- Choose a button under "Or sign in with", when the server offers login providers and you linked an account of that provider under "Linked accounts" in the settings.
- Choose "Forgot your password?" to get a link that sets a new password. Setting a new password signs you out everywhere. A server that sends no email cannot send the link: ask its administrators.

If you turned on two-factor authentication, the page then asks for the six-digit code of your authenticator app, or one
of your recovery codes. After too many failed attempts the page makes you wait a few minutes and says how long.

### An account that cannot be used yet

After signing in you may see a page instead of the app:

- Your registration is waiting for a moderator's approval, or was not approved.
- You have to confirm your email address. "Send the link again" sends a new link.
- Your account is restricted by a moderator. The page lists the actions in effect and lets you appeal once ("Appeal this action"). The same actions are listed under "Account status" in the settings.
- You deactivated the account. "Reactivate your account" brings it back until the date it is deleted; "Delete account" deletes it at once.

## Signing out

Choose "Sign out…", the last row of the settings, and confirm with "Sign out". It signs this browser out; your other
browsers and apps stay signed in. "Signed-in browsers", under "Security" in the settings, lists the browsers signed in
to your account and signs them out.

## Settings

"Settings" in the navigation opens the settings. They hold "Profile", which changes how your account looks and where
it is found; "Display and posting"; "Notifications"; "Relationships", which lists the accounts you block and mute, the
servers you mute and the hashtags you follow, each with the button that undoes it; "Keyword filters", which lists your
filters and adds, changes and deletes them; "Security", where you manage your password, passkeys, two-factor
authentication, linked accounts, and signed-in browsers; "Apps", which lists the apps you allowed to use your account,
which you can revoke, and the apps you registered; "Invites", when you can create invites, which creates invite links
and revokes them; and "Account and data", which leads to "Account status".
