// SPDX-License-Identifier: AGPL-3.0-only

// The CAPTCHA of the sign-in, registration and password reset forms (ADR-0106 §11): the service's widget, which its
// script draws into the element of its class and which puts its token into a field of the form, and the notice the
// service asks for with links to its documents. The script loads once the element is on the page, so that the
// service finds it, and once per page.

import { FormLink, FormLinks, FormText, WidgetSlot } from "@my/design-system";
import { useEffect } from "react";
import type { HumanCheck as Widget } from "./data";
import { useAuthStrings } from "./frame";

/** Loads the script at `src` into the page unless it is there; "" loads none (a preview's sample). */
function loadScript(src: string) {
  if (src === "" || [...document.scripts].some((script) => script.src === src)) return;
  const script = document.createElement("script");
  script.src = src;
  script.async = true;
  document.head.append(script);
}

/** The widget of `check`, inside the form whose token it gives. */
export function HumanCheck({ check }: { check: Widget }) {
  const t = useAuthStrings();
  useEffect(() => {
    loadScript(check.scriptSrc);
  }, [check.scriptSrc]);
  const data: Record<`data-${string}`, string> = { "data-sitekey": check.siteKey, "data-theme": check.theme };
  if (check.action) data["data-action"] = check.action;
  if (check.language) data["data-language"] = check.language;
  return (
    <>
      <WidgetSlot marker={check.containerClass} data={data} />
      <FormText muted>{check.notice}</FormText>
      {check.links && check.links.length > 0 && (
        <FormLinks>
          {check.links.map((link) => (
            <FormLink key={link.url} href={link.url} newTab={t("auth.link.new_tab")}>
              {link.title}
            </FormLink>
          ))}
        </FormLinks>
      )}
    </>
  );
}
