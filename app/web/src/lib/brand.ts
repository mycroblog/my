// SPDX-License-Identifier: AGPL-3.0-only
// The instance's own icon and logo (ADR-0116), which its administrators set in the server settings: the logo takes the
// place of the wordmark and the name in the app's header, and the icon that of the built-in mark where the server is
// shown as a square picture. Both are SVGs under /brand/, shown only as images (`@my/ui`'s `Wordmark` and `BrandIcon`).
import { useQuery } from "@tanstack/react-query";
import { useApi } from "./auth/SessionProvider";
import { useConfig } from "./config";
import { shellQueryReader } from "./query";

/** A logo as `@my/ui`'s `Wordmark` takes it (`BrandLogo`). */
type BrandLogo = { src: string; darkSrc: string | undefined; width: number; height: number };

export type Brand = {
  /** The URL of the instance's own icon, or undefined for the built-in mark. */
  icon: string | undefined;
  /** The instance's own logo with its dark variant, or undefined for the built-in wordmark. */
  logo: BrandLogo | undefined;
};

/**
 * The instance's brand images from GET /instance; until it loads, and when it cannot, the ones the server wrote into
 * the shell. A change shows once GET /instance is fetched again, without a reload.
 */
export function useBrand(): Brand {
  const api = useApi();
  const config = useConfig();
  // The shell reads GET /instance on every page and handles its failures; this only reads its answer.
  const instance = useQuery({ ...api.query.queryOptions("get", "/instance"), ...shellQueryReader });
  if (instance.data !== undefined) {
    const { icon, logo, logo_dark: logoDark } = instance.data;
    return {
      icon: icon?.svg_url,
      logo:
        logo === undefined
          ? undefined
          : { src: logo.svg_url, darkSrc: logoDark?.svg_url, width: logo.width, height: logo.height },
    };
  }
  return {
    icon: config.icon?.url,
    logo:
      config.logo === undefined
        ? undefined
        : { src: config.logo.url, darkSrc: config.logoDark?.url, width: config.logo.width, height: config.logo.height },
  };
}
