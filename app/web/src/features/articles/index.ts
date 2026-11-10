// SPDX-License-Identifier: AGPL-3.0-only
// The article editor (ADR-0160, web-client/screens/article-editor.md): writing a new article, as a draft on the server
// that it autosaves, publishes, or schedules, and editing a published one, beside the preview the server renders.
// Routes use it through this file only (web-client/README.md A3); the loaders fetch what the editor starts from.
export { EditArticle, NewArticle } from "./components/ArticleEditor";
export { articleDraftQueryOptions, articleSourceQueryOptions, articleStatusQueryOptions } from "./api/articles";
