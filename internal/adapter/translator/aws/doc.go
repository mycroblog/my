// SPDX-License-Identifier: AGPL-3.0-only

// Package aws implements the translation.Translator port on Amazon Translate (ADR-0148 §15), with
// the real-time TranslateText operation.
//
// The markers of the port go out as HTML: each marker sits alone in an element with
// translate="no", which Amazon Translate documents as content that it keeps as it is in
// TranslateText, and the text around them is escaped, with line breaks as <br>. Each fragment is
// one <div>, so that several fragments share a call, and the answer is split at the same <div>
// elements. A pair of markers is two such elements rather than one element around its words: the
// translate="no" behaviour is the documented one, and whether TranslateText keeps the attributes
// of other elements is not, so the markers stay recognizable whatever happens to the rest of the
// markup. Every translation passes translation.MarkersIntact before it is returned.
//
// TranslateText takes at most 10,000 bytes of text, so the fragments are packed into calls under
// that size, and a fragment that is larger alone is cut at a line break, the end of a sentence,
// or a space, never inside a marker. With an undetermined source the first call has Amazon
// detect the language (SourceLanguageCode "auto", through Amazon Comprehend in the same region),
// and the next calls of the request send the language that it detected.
//
// The credentials come from the SDK's default chain (the execution role, the environment, the
// shared files); the role needs translate:TranslateText, and, as the managed policies of Amazon
// Translate grant it, comprehend:DetectDominantLanguage for an undetermined source. By default Amazon may store and use the text to improve its
// services unless the AWS Organizations AI services opt-out policy covers the account
// (ADR-0148, Open Questions).
//
// TooManyRequestsException, ThrottlingException, LimitExceededException, and
// ServiceQuotaExceededException are the provider's limits; UnsupportedLanguagePairException
// and DetectedLanguageLowConfidenceException an unsupported language; every other failure is
// temporary. The errors name the error code of Amazon Translate, never its message or the text.
package aws
