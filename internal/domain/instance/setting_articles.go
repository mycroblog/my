// SPDX-License-Identifier: AGPL-3.0-only

package instance

// The settings of articles, the long posts in Markdown (ADR-0160 §3, §5).
const (
	// SettingArticleAuthoring decides who may write new articles: every local user, or the
	// holders of the permission create_articles.
	SettingArticleAuthoring SettingKey = "article_authoring"
	// SettingMaxArticleBytes bounds the rendered HTML of an article's body, in UTF-8 bytes.
	SettingMaxArticleBytes SettingKey = "max_article_bytes"
	// SettingMaxArticleImages bounds the images in an article's body.
	SettingMaxArticleImages SettingKey = "max_article_images"
)

// ArticleAuthoring is the value of SettingArticleAuthoring (ADR-0160 §3).
type ArticleAuthoring string

// Article authoring values.
const (
	ArticleAuthoringEveryone   ArticleAuthoring = "everyone"   // every local user
	ArticleAuthoringPermission ArticleAuthoring = "permission" // the holders of create_articles
)

// Bounds of the article settings.
const (
	// MaxMaxArticleBytes is the most that SettingMaxArticleBytes may be, and its default: the
	// body HTML that the receiving servers keep (ADR-0160 §5).
	MaxMaxArticleBytes = 100000
	// MaxMaxArticleImages is the most that SettingMaxArticleImages may be.
	MaxMaxArticleImages = 100
	// DefaultMaxArticleImages is the default of SettingMaxArticleImages.
	DefaultMaxArticleImages = 20
)

// ArticleAuthoring returns the value of SettingArticleAuthoring; an unexpected value is everyone,
// the default.
func (s Settings) ArticleAuthoring() ArticleAuthoring {
	if v, _ := s.Value(SettingArticleAuthoring); v == string(ArticleAuthoringPermission) {
		return ArticleAuthoringPermission
	}
	return ArticleAuthoringEveryone
}

// MaxArticleBytes returns the value of SettingMaxArticleBytes.
func (s Settings) MaxArticleBytes() int { return s.intValue(SettingMaxArticleBytes) }

// MaxArticleImages returns the value of SettingMaxArticleImages.
func (s Settings) MaxArticleImages() int { return s.intValue(SettingMaxArticleImages) }
