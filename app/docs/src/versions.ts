// SPDX-License-Identifier: AGPL-3.0-only
// The versions of the site (ADR-0150 §6): which one is being built, and the list the version switcher in the header
// and the end-of-support banner show. The list is versions.json, resolved by `scripts/docs-site.sh resolve` against the
// published releases into the file DOCS_SITE_VERSIONS names. The deploy builds every version from its own tag with
// main's resolved list, so that an old version's switcher and banner follow main. Without DOCS_SITE_VERSIONS (a local
// build) the site reads versions.json of its own tree, and every listed version counts as released.
//
// The deploy sets, for each build: DOCS_SITE_VERSION (next, latest, or a minor version vX.Y), DOCS_SITE_BASE (its path,
// /next/, /latest/, /vX.Y/), DOCS_SITE_REF (its git ref) and DOCS_SITE_VERSIONS. Builds of older tags read the same
// variables, so their names and the format of the resolved file only ever grow.
import { readFileSync } from "node:fs";
import { join } from "node:path";

type SupportStatus = "supported" | "ended";

interface ReleasedVersion {
  /** The minor version, vX.Y: the version's path is /vX.Y/. */
  minor: string;
  /** The newest published stable release of the minor version (vX.Y.Z); unknown in a local build. */
  tag?: string;
  status: SupportStatus;
}

interface Versions {
  /** The minor version /latest/ serves, the newest released one; undefined before the first release. */
  latest?: string;
  /** The released minor versions, newest first. */
  versions: ReleasedVersion[];
}

/** The version being built: next (main), latest, or a minor version vX.Y. */
export const currentVersion = process.env["DOCS_SITE_VERSION"] || "next";

/** Reads and checks a versions file: versions.json, or the resolved one of DOCS_SITE_VERSIONS. */
function parseVersions(source: string): Versions {
  const data = JSON.parse(source) as { latest?: unknown; versions?: unknown };
  if (!Array.isArray(data.versions)) {
    throw new Error("versions: `versions` is not a list");
  }
  const versions = data.versions.map((entry: { minor?: unknown; tag?: unknown; status?: unknown }) => {
    if (typeof entry.minor !== "string" || !/^v\d+\.\d+$/.test(entry.minor)) {
      throw new Error(`versions: ${JSON.stringify(entry.minor)} is not a minor version (vX.Y)`);
    }
    if (entry.status !== "supported" && entry.status !== "ended") {
      throw new Error(`versions: the status of ${entry.minor} is neither supported nor ended`);
    }
    const version: ReleasedVersion = { minor: entry.minor, status: entry.status };
    if (typeof entry.tag === "string") {
      version.tag = entry.tag;
    }
    return version;
  });
  const result: Versions = { versions };
  if (typeof data.latest === "string") {
    result.latest = data.latest;
  } else if (data.latest === undefined && versions[0] !== undefined) {
    // versions.json itself names no latest: in a local build, the first entry is.
    result.latest = versions[0].minor;
  }
  return result;
}

function readVersions(): Versions {
  // From the working directory, app/docs, where the build and the dev server run: the pages are prerendered from a
  // bundle under dist/, so a path relative to this module (import.meta.url) would not find the file.
  const file = process.env["DOCS_SITE_VERSIONS"] || join(process.cwd(), "versions.json");
  return parseVersions(readFileSync(file, "utf8"));
}

const versions = readVersions();

/** The path prefix of a version: /next/, /latest/, /v0.1/. */
export function versionBase(version: string): string {
  return `/${version}/`;
}

interface VersionOption {
  /** The path prefix of the version. */
  base: string;
  label: string;
  selected: boolean;
}

/** The choices of the version switcher: latest, each released minor version, and next. */
export function versionOptions(): VersionOption[] {
  const options: VersionOption[] = [];
  const latest = versions.versions.find((version) => version.minor === versions.latest);
  if (latest !== undefined) {
    options.push({
      base: versionBase("latest"),
      label: `latest (${latest.tag ?? latest.minor})`,
      selected: currentVersion === "latest",
    });
  }
  for (const version of versions.versions) {
    options.push({
      base: versionBase(version.minor),
      label: version.status === "ended" ? `${version.minor} (end of support)` : version.minor,
      selected: currentVersion === version.minor,
    });
  }
  options.push({ base: versionBase("next"), label: "next (main)", selected: currentVersion === "next" });
  return options;
}

/**
 * Whether search engines index the version being built: /latest/ once a release exists, /next/ before the first one
 * (while /latest/ redirects to it), and never a Preview of a pull request (DOCS_SITE_PREVIEW=1, set by
 * `scripts/docs-site.sh build-preview`). Every other page carries noindex (src/indexing.ts).
 */
export function indexedVersion(): boolean {
  if (process.env["DOCS_SITE_PREVIEW"] === "1") {
    return false;
  }
  return currentVersion === (versions.latest === undefined ? "next" : "latest");
}

/** Whether the version being built is a minor version whose support ended (every page then carries the banner). */
export function supportEnded(): boolean {
  return versions.versions.some((version) => version.minor === currentVersion && version.status === "ended");
}
