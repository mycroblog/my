// SPDX-License-Identifier: AGPL-3.0-only
// The part of MJML's API (mjml 5, which ships no types) that frame.ts uses.
declare module "mjml" {
  type Options = {
    fonts?: Record<string, string>;
    keepComments?: boolean;
    validationLevel?: "strict" | "soft" | "skip";
    filePath?: string;
  };
  type Result = { html: string; errors: { formattedMessage: string }[] };
  export default function mjml2html(source: string, options?: Options): Promise<Result>;
}
