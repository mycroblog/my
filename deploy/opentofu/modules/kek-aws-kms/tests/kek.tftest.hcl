# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches AWS. By plan alone: prevent_destroy refuses the destroy that follows an apply.

mock_provider "aws" {}

run "it_creates_a_key_that_rotates" {
  command = plan

  assert {
    condition     = aws_kms_key.kek.enable_key_rotation && aws_kms_alias.kek.name == "alias/my-kek" && output.env["MY_KEY_MANAGER"] == "awskms"
    error_message = "the key must rotate, under its alias"
  }

  assert {
    condition     = toset([for r in output.roles : r.role]) == toset(["kms:Encrypt", "kms:Decrypt", "kms:DescribeKey"])
    error_message = "the server needs exactly these on the key"
  }
}

run "a_key_made_elsewhere_creates_nothing" {
  command = plan

  variables {
    key_arn = "arn:aws:kms:ap-northeast-1:111122223333:key/abc"
  }

  assert {
    condition     = aws_kms_key.kek == null && aws_kms_alias.kek == null && output.env["MY_KMS_KEY"] == var.key_arn
    error_message = "a key made elsewhere must be used as it is"
  }
}
