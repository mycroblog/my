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
| Accepts invitations only | Enter the invite code you were given, or open the invitation link, which fills it in. |
| Is closed | The page says that the server is not accepting new accounts. |
| Registers through login providers only | You sign up with a provider the server offers (for example "Sign in with GitHub") instead of a password. |

If the server sends email, it sends a link to confirm your email address. Open it and choose "Confirm email address".
Until the account is approved and the address is confirmed, you can sign in but you only see what you are waiting for
("Your account is not ready yet").

After sign-up, a welcome page lets you choose an avatar and a display name ("Continue" saves them, "Later" skips this).
Your home timeline then shows a "Get started" checklist: "Set up your profile", "Follow someone", and "Write your first
post". "Hide these steps" closes it on that browser. "Suggested accounts" lists people you can follow.

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

Choose "Sign out" in the "Account" section of the app. The "Signed-in browsers" page of the settings lists the browsers
signed in to your account and signs them out.

## Settings

The settings are grouped as "Profile and community", "Display and notifications", "Relationships", "Security", "Apps",
and "Account and data". Under "Security" you manage your password, two-factor authentication, passkeys, linked accounts,
and signed-in browsers.
