// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { credentialsQueryOptions, WelcomeForm } from "../features/settings";
import { isolate } from "../features/profile";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { useInstanceName } from "../lib/instanceName";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The welcome step after sign-up (web-client/screens/welcome.md): the server's registration page sends a new user who
 * can use the account at once here, through the app's sign-in; the user writes the basics of the profile and goes on
 * to the home timeline, whose checklist and suggestions show the next steps.
 */
export const Route = createFileRoute("/welcome")({
  staticData: { title: "client:welcome.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) => context.queryClient.query({ ...credentialsQueryOptions(context.api), staleTime: Infinity }),
  component: WelcomePage,
});

function WelcomePage() {
  const t = useT();
  const instance = useInstanceName();
  const navigate = Route.useNavigate();
  return (
    <>
      <PageHeading>{t("client:welcome.heading", { server: isolate(instance) })}</PageHeading>
      <p>{t("client:welcome.lead")}</p>
      <WelcomeForm
        laterHref="/home"
        onDone={() => {
          void navigate({ to: "/home" });
        }}
      />
    </>
  );
}
