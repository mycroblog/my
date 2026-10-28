// SPDX-License-Identifier: AGPL-3.0-only
// The data of the screen stories of the pages about the server (about.stories.tsx, and the stories of the documents,
// the costs and the supporters).
import { instanceFixture, otherAccountFixture, type Schemas } from "../../../test/fixtures";

/** A server that says everything about itself: a description, rules, documents, a contact and a status page. */
export const describedInstance: Schemas["Instance"] = {
  ...instanceFixture,
  short_description: "A small server for bakers, millers, and anyone who likes the smell of bread.",
  description:
    "We started in a kitchen in 2026 and grew one loaf at a time.\n\nPost your bakes, your failures and your questions. Everyone was a beginner once.",
  status_page_url: "https://status.my.example/",
  terms_url: "https://my.example/about/terms",
  privacy_policy_url: "https://my.example/about/privacy",
  commerce_disclosure_url: "https://my.example/about/commerce",
  contact: { email: "admin@my.example", account: { ...otherAccountFixture, display_name: "Bob (administrator)" } },
  search_indexing: "allow",
  ai_usage: "deny",
  rules: [
    { id: "0192f0c4-7a8e-7000-8000-0000000000e1", text: "Be kind.", hint: "Disagree with ideas, not with people." },
    { id: "0192f0c4-7a8e-7000-8000-0000000000e2", text: "No spam, advertising, or automated posting without a bot flag." },
    {
      id: "0192f0c4-7a8e-7000-8000-0000000000e3",
      text: "Put a content warning on pictures of food that went badly wrong, burns, and anything else people may not want to see while eating.",
    },
    { id: "0192f0c4-7a8e-7000-8000-0000000000e4", text: "No harassment, hate speech, or threats." },
    { id: "0192f0c4-7a8e-7000-8000-0000000000e5", text: "Credit the recipes you share." },
  ],
};
