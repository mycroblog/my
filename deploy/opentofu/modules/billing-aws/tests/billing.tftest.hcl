# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches AWS.

mock_provider "aws" {
  mock_data "aws_caller_identity" {
    defaults = {
      account_id = "111122223333"
    }
  }
  mock_data "aws_partition" {
    defaults = {
      partition = "aws"
    }
  }
}

run "on_an_aws_app_the_app_gets_the_permission" {
  command = plan

  assert {
    condition     = length(output.permissions) == 1 && output.permissions[0].permission == "ce:GetCostAndUsage" && output.permissions[0].scope == "policy" && output.permissions[0].resource == "*"
    error_message = "an AWS app's identity needs ce:GetCostAndUsage"
  }

  assert {
    condition     = length(output.env) == 0 && output.role_arn == null
    error_message = "without web identity the server must not be told to assume a role"
  }

  assert {
    condition     = aws_iam_role.import == null && aws_iam_role_policy.import == null
    error_message = "without web identity nothing is created"
  }

  assert {
    condition     = length(output.billing_sources) == 1 && output.billing_sources[0] == "aws"
    error_message = "the module adds aws to the sources"
  }
}

run "on_google_cloud_the_role_trusts_the_service_account" {
  command = plan

  variables {
    web_identity                     = true
    google_service_account_unique_id = "109876543210987654321"
  }

  assert {
    condition     = output.env["MY_BILLING_AWS_ROLE_ARN"] == "arn:aws:iam::111122223333:role/my-billing-import"
    error_message = "the setting must name the role"
  }

  assert {
    condition     = length(output.permissions) == 0
    error_message = "the app of Google Cloud refuses an IAM action, and the role holds it"
  }

  assert {
    condition = jsondecode(aws_iam_role.import.assume_role_policy).Statement[0].Condition.StringEquals == {
      "accounts.google.com:aud"  = "109876543210987654321"
      "accounts.google.com:oaud" = "arn:aws:iam::111122223333:role/my-billing-import"
    }
    error_message = "the trust policy must limit the role to the service account and to this role as the audience"
  }

  assert {
    condition     = jsondecode(aws_iam_role.import.assume_role_policy).Statement[0].Principal.Federated == "accounts.google.com"
    error_message = "the role must trust Google's identity provider"
  }

  assert {
    condition     = jsondecode(aws_iam_role_policy.import.policy).Statement[0].Action == "ce:GetCostAndUsage" && aws_iam_role_policy.import.role == "my-billing-import"
    error_message = "the role holds the one action"
  }
}

run "web_identity_needs_the_service_account" {
  command = plan

  variables {
    web_identity = true
  }

  expect_failures = [var.google_service_account_unique_id]
}

run "a_long_role_name_fails" {
  command = plan

  variables {
    role_name = "a-role-name-that-is-far-longer-than-the-sixty-four-characters-iam-allows"
  }

  expect_failures = [var.role_name]
}
