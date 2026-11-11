# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches AWS.

mock_provider "aws" {
  mock_resource "aws_secretsmanager_secret" {
    defaults = {
      arn = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:s-AbCdEf"
    }
  }
}

mock_provider "random" {}

variables {
  secrets            = { MY_VAPID_PRIVATE_KEY = "my-vapid-key" }
  referenced         = { database_url = "my-database-url" }
  generate_p256_keys = ["MY_VAPID_PRIVATE_KEY"]
}

# By plan alone: prevent_destroy refuses the destroy that follows an apply, as it should.
run "a_value_is_named_by_its_arn" {
  command = plan

  assert {
    condition     = keys(output.secret_env) == ["MY_VAPID_PRIVATE_KEY"]
    error_message = "only var.secrets become settings"
  }

  assert {
    condition     = contains(keys(output.ids), "database_url") && contains(keys(output.ids), "MY_VAPID_PRIVATE_KEY")
    error_message = "every secret has an ID for another module"
  }

  assert {
    condition     = aws_secretsmanager_secret_version.generated["MY_VAPID_PRIVATE_KEY"].secret_string_wo_version == 1 && aws_secretsmanager_secret_version.generated["MY_VAPID_PRIVATE_KEY"].secret_string == null
    error_message = "a generated value goes through the write-only attribute, at the fixed version"
  }
}

run "a_generated_key_outside_secrets_fails" {
  command = plan

  variables {
    generate_p256_keys = ["other"]
  }

  expect_failures = [var.generate_p256_keys]
}

run "a_value_from_elsewhere_goes_through_the_write_only_attribute" {
  command = plan

  variables {
    referenced   = { database_url = "my-database-url" }
    write_keys   = ["database_url"]
    write_values = { database_url = "postgres://u:p@h/d" }
  }

  assert {
    condition     = aws_secretsmanager_secret_version.written["database_url"].secret_string == null && aws_secretsmanager_secret_version.written["database_url"].secret_string_wo_version == 1
    error_message = "the value must go through the write-only attribute"
  }
}

run "a_written_key_must_be_a_secret_of_this_module" {
  command = plan

  variables {
    write_keys = ["not_a_secret"]
  }

  expect_failures = [var.write_keys]
}
