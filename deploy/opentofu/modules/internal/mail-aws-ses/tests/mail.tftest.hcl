# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches AWS.

mock_provider "aws" {}

variables {
  from = "noreply@social.example.org"
}

run "without_a_configuration_set" {
  assert {
    condition     = aws_sesv2_configuration_set.this == null && length(output.permissions) == 1 && aws_sesv2_email_identity.domain.email_identity == "social.example.org" && !contains(keys(output.env), "MY_MAIL_SES_CONFIGURATION_SET")
    error_message = "the identity is the sender's domain, and nothing else is created"
  }
}

run "with_a_configuration_set" {
  variables {
    configuration_set = "my-mail"
  }

  assert {
    condition     = aws_sesv2_configuration_set.this.configuration_set_name == "my-mail" && length(output.permissions) == 2 && output.env["MY_MAIL_SES_CONFIGURATION_SET"] == "my-mail"
    error_message = "the permission must also reach the configuration set"
  }
}

run "with_an_existing_configuration_set" {
  override_data {
    target = data.aws_sesv2_configuration_set.existing
    values = {
      arn = "arn:aws:ses:ap-northeast-1:111122223333:configuration-set/shared-mail"
    }
  }

  variables {
    configuration_set          = "shared-mail"
    existing_configuration_set = true
  }

  assert {
    condition     = aws_sesv2_configuration_set.this == null && output.env["MY_MAIL_SES_CONFIGURATION_SET"] == "shared-mail"
    error_message = "the module must create no set, and apply the one named"
  }

  assert {
    condition     = output.permissions[1].resource == "arn:aws:ses:ap-northeast-1:111122223333:configuration-set/shared-mail"
    error_message = "the permission must reach the existing set"
  }
}

run "an_existing_configuration_set_without_a_name_fails" {
  command = plan

  variables {
    existing_configuration_set = true
  }

  expect_failures = [var.existing_configuration_set]
}
