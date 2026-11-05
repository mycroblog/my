# SPDX-License-Identifier: AGPL-3.0-only

# The billing modules in the backends of each app, on mocked providers: nothing here reaches a cloud.

mock_provider "google" {
  mock_resource "google_service_account" {
    defaults = {
      email     = "my-server@p.iam.gserviceaccount.com"
      member    = "serviceAccount:my-server@p.iam.gserviceaccount.com"
      unique_id = "109876543210987654321"
    }
  }
  mock_resource "google_cloud_run_v2_service" {
    defaults = {
      uri = "https://my-abc-an.a.run.app"
    }
  }
}

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
  mock_data "aws_iam_policy_document" {
    defaults = {
      json          = "{}"
      minified_json = "{}"
    }
  }
  mock_data "aws_region" {
    defaults = {
      region = "ap-northeast-1"
    }
  }
  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::111122223333:role/my-server"
    }
  }
  mock_resource "aws_cloudwatch_log_group" {
    defaults = {
      arn = "arn:aws:logs:ap-northeast-1:111122223333:log-group:/aws/lambda/my"
    }
  }
  mock_resource "aws_lambda_function_url" {
    defaults = {
      function_url = "https://abc.lambda-url.ap-northeast-1.on.aws/"
    }
  }
}

run "both_on_cloud_run" {
  command = apply

  module {
    source = "../../tests/billing-on-cloud-run"
  }

  assert {
    condition     = output.env["MY_BILLING_SOURCES"] == "aws,gcp"
    error_message = "both modules must sit in backends without naming the setting twice"
  }

  assert {
    condition     = output.env["MY_BILLING_AWS_ROLE_ARN"] == "arn:aws:iam::111122223333:role/my-billing-import"
    error_message = "the server on Cloud Run must assume the role of the AWS module"
  }

  assert {
    condition     = output.env["MY_BILLING_GCP_METHOD"] == "bigquery" && output.env["MY_BILLING_GCP_TABLE"] == "p.billing_export.gcp_billing_export_v1_012345_6789AB_CDEF01"
    error_message = "the Google Cloud module's settings must reach the server"
  }
}

run "the_aws_module_alone_on_cloud_run" {
  command = apply

  module {
    source = "../../tests/billing-on-cloud-run"
  }

  variables {
    sources = ["aws"]
  }

  assert {
    condition     = output.env["MY_BILLING_SOURCES"] == "aws" && !contains(keys(output.env), "MY_BILLING_GCP_METHOD")
    error_message = "one module must name its own source alone"
  }
}

run "the_budget_method_beside_aws_on_cloud_run" {
  command = apply

  module {
    source = "../../tests/billing-on-cloud-run"
  }

  variables {
    method = "budget"
  }

  assert {
    condition     = output.env["MY_BILLING_SOURCES"] == "aws,gcp" && output.env["MY_BILLING_GCP_BUDGET_SUBSCRIPTION"] == "projects/p/subscriptions/my-billing-budget"
    error_message = "the method must not change what the app does with the sources"
  }
}

run "on_lambda" {
  command = plan

  module {
    source = "../../tests/billing-on-lambda"
  }

  assert {
    condition     = output.env["MY_BILLING_SOURCES"] == "aws" && !contains(keys(output.env), "MY_BILLING_AWS_ROLE_ARN")
    error_message = "on AWS the module sits in backends, and the execution role needs no role to assume"
  }

  assert {
    condition     = output.policy_actions[0].permission == "ce:GetCostAndUsage"
    error_message = "the execution role must get the Cost Explorer action"
  }
}
