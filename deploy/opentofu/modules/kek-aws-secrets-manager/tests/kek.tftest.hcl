# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches AWS.

mock_provider "aws" {
  mock_resource "aws_secretsmanager_secret" {
    defaults = {
      arn = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:my-kek-AbCdEf"
    }
  }
}

mock_provider "random" {}

# By plan alone: prevent_destroy refuses the destroy that follows an apply, as it should.
run "it_generates_the_kek_through_the_write_only_attribute" {
  command = plan

  assert {
    condition     = keys(output.secret_env) == ["MY_KEK"]
    error_message = "the KEK is a variable that names the secret"
  }

  assert {
    condition     = aws_secretsmanager_secret_version.kek.secret_string_wo_version == 1 && aws_secretsmanager_secret_version.kek.secret_string == null
    error_message = "the value must go through the write-only attribute, at the fixed version"
  }
}

run "a_secret_made_elsewhere_creates_nothing" {
  command = plan

  variables {
    secret_arn = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:theirs-AbCdEf"
  }

  assert {
    condition     = aws_secretsmanager_secret.kek == null && aws_secretsmanager_secret_version.kek == null && output.secret_env["MY_KEK"] == var.secret_arn
    error_message = "a secret made elsewhere must be used as it is"
  }
}
