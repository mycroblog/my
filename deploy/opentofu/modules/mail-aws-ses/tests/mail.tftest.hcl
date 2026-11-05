# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches AWS.

mock_provider "aws" {}

variables {
  from = "noreply@social.example.org"
}

run "without_a_configuration_set" {
  assert {
    condition     = aws_sesv2_configuration_set.this == null && length(output.roles) == 1 && aws_sesv2_email_identity.domain.email_identity == "social.example.org" && !contains(keys(output.env), "MY_MAIL_SES_CONFIGURATION_SET")
    error_message = "the identity is the sender's domain, and nothing else is created"
  }
}

run "with_a_configuration_set" {
  variables {
    configuration_set = "my-mail"
  }

  assert {
    condition     = aws_sesv2_configuration_set.this.configuration_set_name == "my-mail" && length(output.roles) == 2 && output.env["MY_MAIL_SES_CONFIGURATION_SET"] == "my-mail"
    error_message = "the role must also reach the configuration set"
  }
}
