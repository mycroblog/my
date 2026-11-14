// SPDX-License-Identifier: AGPL-3.0-only

// The pages that take the place of every screen (ADR-0088 §2, §10): the maintenance, while the server answers with its
// maintenance problem, with the operator's words, when it should end and the status page; and the closed server, for a
// signed-in reader, who can only take their data away. Each is a page of its own outside the frame, with the server's
// mark, and its title as the document's.

import { useStatusPageUrl, type Closure, type Instance, type Maintenance } from "@my/app-data";
import { ClosedScreen } from "@my/app-features";
import {
  FormHead,
  FormLink,
  FormLinks,
  FormPage,
  FormPageFooter,
  FormText,
  useLocale,
  type FormPageServer,
} from "@my/design-system";
import { useEffect } from "react";
import wordmarkDark from "../../../brand/Wordmark-DarkBg.svg";
import wordmarkLight from "../../../brand/Wordmark-Primary-Color.svg";
import { useStrings } from "../locale";

const wordmark = { light: wordmarkLight, dark: wordmarkDark };

/** The server as a page of its own shows it: its name, its logo for light and dark, and its summary. */
export function pageServer(instance: Instance | undefined, name: string): FormPageServer {
  return {
    name: instance?.name ?? name,
    logo: instance?.logo?.svg_url,
    logoDark: instance?.logo_dark?.svg_url,
    description: instance?.short_description,
  };
}

/** The footer of a page of its own: the status page when there is one, and the server's about page. */
function PageFooter({ statusPage }: { statusPage: string | undefined }) {
  const t = useStrings();
  return (
    <FormPageFooter
      label={t("app.footer")}
      links={[
        ...(statusPage === undefined ? [] : [{ href: statusPage, label: t("app.status_page") }]),
        { href: "/about", label: t("app.footer_about") },
      ]}
    />
  );
}

/** Names the document after the page's title. */
function useTitle(title: string) {
  useEffect(() => {
    document.title = title;
  }, [title]);
}

/**
 * The page of the server's maintenance, in place of every screen. The server reads no database while it is under
 * maintenance (ADR-0088 §10), so the page shows nothing of it but what its maintenance problem says: no name, no logo,
 * no introduction, and no link to its pages, which cannot be read either.
 */
export function MaintenancePage({ maintenance }: { maintenance: Maintenance }) {
  const t = useStrings();
  const { locale } = useLocale();
  const statusPage = useStatusPageUrl();
  const title = t("app.maintenance_title");
  useTitle(title);
  const until =
    maintenance.until === undefined
      ? undefined
      : new Intl.DateTimeFormat(locale, {
          weekday: "long",
          hour: "numeric",
          minute: "2-digit",
          timeZoneName: "long",
        }).format(new Date(maintenance.until));
  return (
    <FormPage wordmark={wordmark}>
      <FormHead title={title}>
        {until !== undefined && (
          <FormText>
            <strong>
              <time dateTime={maintenance.until}>{t("app.maintenance_until", { time: until })}</time>
            </strong>
          </FormText>
        )}
        <FormText>{t("app.maintenance_body")}</FormText>
      </FormHead>
      {maintenance.message !== undefined && <FormText>{maintenance.message}</FormText>}
      {statusPage !== undefined && (
        <FormLinks>
          <FormLink href={statusPage} newTab={t("app.new_tab")}>
            {t("app.maintenance_status")}
          </FormLink>
        </FormLinks>
      )}
    </FormPage>
  );
}

/** The page of the closed server, in place of every screen of a signed-in reader. */
export function ClosedPage({ server, closure }: { server: FormPageServer; closure: Closure }) {
  const statusPage = useStatusPageUrl();
  return (
    <ClosedScreen
      server={server}
      wordmark={wordmark}
      closure={closure}
      footer={<PageFooter statusPage={statusPage} />}
    />
  );
}
