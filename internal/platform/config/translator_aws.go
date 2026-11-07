// SPDX-License-Identifier: AGPL-3.0-only

package config

func init() { schema = append(schema, translatorAWSRegion()) }

// translatorAWSRegion returns the setting that registers Amazon Translate as a translation provider
// (ADR-0148 §15). The credentials come from the AWS SDK's default chain, so no key is a setting.
func translatorAWSRegion() field {
	return field{
		key: "translator_aws_region",
		doc: "AWS region of Amazon Translate, such as `us-east-1`. Setting it makes Amazon Translate the translation provider `aws`, which the instance settings can then enable ([translation](../operations/translation.md)); " +
			"the credentials come from the AWS SDK's default chain, such as the execution role. Empty leaves the provider out. The build tag `no_translator_aws` leaves it out.",
		check: checkTranslatorAWSRegion,
		set:   func(c *Config, v string) { c.TranslatorAWSRegion = v },
	}
}
