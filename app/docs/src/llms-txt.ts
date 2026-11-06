// SPDX-License-Identifier: AGPL-3.0-only
// The plain-text forms of the site for LLMs and coding agents (ADR-0150): starlight-llms-txt writes llms.txt (an
// index, https://llmstxt.org/), llms-full.txt (every page) and llms-small.txt (a shortened form) under the base path of
// each version (/next/llms.txt, /latest/llms.txt, ...). Their links start with that base, so each version describes itself.
import starlightLlmsTxt from "starlight-llms-txt";
import { base, site } from "./published";

export const llmsTxtPlugin = starlightLlmsTxt({
  // The brand; `my` is the name of the server software.
  projectName: "mycroblog",
  description:
    "mycroblog is a microblog for the fediverse that starts small and grows with your community. Its server software, `my`, speaks ActivityPub and starts on one small machine or a scale-to-zero container with SQLite, then moves to PostgreSQL, a job queue, and separate web and worker processes as the community grows, on Google Cloud, AWS, or a self-hosted VM, with the same code and data. It has its own client API and a web client.",
  details:
    "The pages cover using the service, operating a server, the configuration and jobs reference, and building clients and bots against the client API. The client API reference is made from its OpenAPI document, which every server serves at `GET /api/v1/openapi.yaml`.",
  // Pages are sorted to the top by slug. A page that does not exist (yet) is skipped, so the list can name the overview
  // of operation ahead of its page.
  promote: ["operations/overview", "operations/running", "guide/signing-in", "guide/posting", "developers/client-api"],
  // The client API reference is generated per operation by starlight-openapi, outside the content collection that the
  // plugin reads, so it is not in llms-full.txt (its pages are about 111 MB of HTML, against 1.4 MB for the guides): it is
  // linked instead, and the OpenAPI document that it is made from is the compact form of the same facts.
  optionalLinks: [
    {
      label: "Client API reference",
      url: `${site}${base}api/`,
      description: "Every operation of the client API with its parameters, request body, answers, and scopes",
    },
  ],
  // llms-small.txt is for small context windows: the generated references of configuration, jobs, and webhooks stay out of it.
  exclude: ["reference/**"],
});
