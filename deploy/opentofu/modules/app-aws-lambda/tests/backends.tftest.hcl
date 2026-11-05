# SPDX-License-Identifier: AGPL-3.0-only

# The contract of the backends, on a mocked provider: nothing here reaches AWS.

mock_provider "aws" {
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

variables {
  image = "111122223333.dkr.ecr.ap-northeast-1.amazonaws.com/my:0.1.0"
  host  = "social.example.org"
  backends = [
    {
      env        = { MY_DATABASE = "postgres" }
      secret_env = { MY_DATABASE_URL = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:db-AbCdEf" }
    },
    {
      secret_env = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" }
    },
    {
      env = { MY_QUEUE = "sqs" }
      permissions = [
        { permission = "sqs:SendMessage", scope = "policy", resource = "arn:aws:sqs:ap-northeast-1:111122223333:my-jobs" },
        {
          permission = "sns:Subscribe"
          scope      = "policy"
          resource   = "arn:aws:sns:ap-northeast-1:111122223333:my-stream"
          conditions = [
            { test = "StringEquals", variable = "sns:Protocol", values = ["sqs"] },
          ]
        },
      ]
      event_sources = {
        jobs = { arn = "arn:aws:sqs:ap-northeast-1:111122223333:my-jobs", visibility_timeout_seconds = 5400 }
      }
    },
  ]
}

run "merges_every_source" {
  command = plan

  variables {
    env        = { MY_MAIL = "smtp" }
    secret_env = { MY_VAPID_PRIVATE_KEY = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:vapid-AbCdEf" }
  }

  assert {
    condition = output.env == {
      MY_DATABASE        = "postgres"
      MY_HOST            = "social.example.org"
      MY_MAIL            = "smtp"
      MY_PROFILE         = "production"
      MY_QUEUE           = "sqs"
      MY_TRUSTED_PROXIES = "127.0.0.1/32"
    }
    error_message = "the settings must merge, and the adapter is what connects to the server"
  }

  assert {
    condition     = aws_lambda_function.server["web"].environment[0].variables["SECRET_ARN_MY_VAPID_PRIVATE_KEY"] == "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:vapid-AbCdEf"
    error_message = "a secret reaches the function as the variable that names it"
  }

  assert {
    condition     = !contains(keys(aws_lambda_function.server["web"].environment[0].variables), "MY_DATABASE_URL")
    error_message = "the value of a secret never reaches the function's variables"
  }
}

run "the_two_functions_differ_by_role_and_invoke_mode" {
  command = plan

  assert {
    condition = (
      aws_lambda_function.server["web"].environment[0].variables["MY_ROLE"] == "web" &&
      aws_lambda_function.server["web"].environment[0].variables["AWS_LWA_INVOKE_MODE"] == "response_stream" &&
      aws_lambda_function_url.web.invoke_mode == "RESPONSE_STREAM" &&
      aws_lambda_function.server["worker"].environment[0].variables["MY_ROLE"] == "worker" &&
      aws_lambda_function.server["worker"].environment[0].variables["AWS_LWA_INVOKE_MODE"] == "buffered"
    )
    error_message = "streams need response_stream, and the queue's events buffered"
  }

  assert {
    condition     = aws_lambda_function.server["web"].environment[0].variables["GOMEMLIMIT"] == "460MiB" && aws_lambda_function.server["worker"].environment[0].variables["GOMEMLIMIT"] == "921MiB"
    error_message = "GOMEMLIMIT follows each function's memory"
  }

  assert {
    condition     = aws_lambda_function_url.web.authorization_type == "NONE" && aws_lambda_permission.url.function_url_auth_type == "NONE" && aws_lambda_permission.url_invoke.invoked_via_function_url
    error_message = "the URL must be public, by both permissions a new function URL requires"
  }

  assert {
    condition     = output.console == null && !contains(keys(aws_lambda_function.server), "console")
    error_message = "the console must not exist while it is off"
  }
}

run "the_queue_invokes_the_worker" {
  command = plan

  assert {
    condition     = aws_lambda_event_source_mapping.worker["jobs"].event_source_arn == "arn:aws:sqs:ap-northeast-1:111122223333:my-jobs" && contains(aws_lambda_event_source_mapping.worker["jobs"].function_response_types, "ReportBatchItemFailures")
    error_message = "without ReportBatchItemFailures the whole batch counts as done"
  }
}

run "the_policy_holds_every_grant" {
  command = plan

  assert {
    condition = (
      contains(flatten(data.aws_iam_policy_document.server.statement[*].actions), "sqs:SendMessage") &&
      contains(flatten(data.aws_iam_policy_document.server.statement[*].actions), "secretsmanager:GetSecretValue") &&
      contains(flatten(data.aws_iam_policy_document.server.statement[*].actions), "sqs:ReceiveMessage")
    )
    error_message = "the role must hold what the backends ask, read each secret, and poll the queue"
  }

  assert {
    condition     = length(flatten(data.aws_iam_policy_document.server.statement[*].condition)) == 1
    error_message = "a permission's conditions must reach its statement"
  }
}

run "a_secret_file_is_a_secret_env" {
  command = plan

  variables {
    secret_files = { MY_VAPID_PRIVATE_KEY_FILE = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:hook-AbCdEf" }
  }

  assert {
    condition     = output.secret_env["MY_VAPID_PRIVATE_KEY"] == "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:hook-AbCdEf"
    error_message = "Lambda mounts nothing, so a file is the same as a variable"
  }
}

run "the_console_names_its_platform" {
  command = apply

  variables {
    console_enabled = true
  }

  assert {
    condition     = output.console.platform == "aws-lambda" && output.console.region == "ap-northeast-1"
    error_message = "console.sh chooses by platform, and needs the region"
  }

  assert {
    condition     = aws_lambda_function.server["console"].image_config[0].command[0] == "lambda-run" && aws_lambda_function.server["migrate"].image_config[0].command[0] == "lambda-run"
    error_message = "the one-off functions must run the one-off command"
  }

  assert {
    condition     = output.origin_host == "abc.lambda-url.ap-northeast-1.on.aws"
    error_message = "origin_host is the bare host name of the URL"
  }
}

run "an_ingress_adds_to_the_adapter" {
  command = plan

  variables {
    backends = [
      { secret_env = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" } },
      { trusted_proxies = ["130.176.0.0/18"], env = { MY_TRUSTED_PROXY_HEADER = "x-forwarded-for" } },
    ]
  }

  assert {
    condition     = output.env["MY_TRUSTED_PROXIES"] == "127.0.0.1/32, 130.176.0.0/18"
    error_message = "the module adds the adapter to what the ingress says"
  }
}

run "settings_above_lambdas_limit_fail" {
  command = plan

  variables {
    env = { MY_TRUSTED_PROXIES = join(", ", [for i in range(300) : "130.176.0.0/18"]) }
  }

  expect_failures = [aws_lambda_function.server]
}

run "a_setting_named_twice_fails" {
  command = plan

  variables {
    env = { MY_DATABASE = "sqlite" }
  }

  expect_failures = [aws_iam_role.server]
}

run "a_setting_and_its_file_form_fail" {
  command = plan

  variables {
    secret_files = { MY_KEK_FILE = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" }
  }

  expect_failures = [aws_iam_role.server]
}

run "no_kek_fails" {
  command = plan

  variables {
    backends = [{ env = { MY_DATABASE = "postgres" } }]
  }

  expect_failures = [aws_iam_role.server]
}

run "a_secret_named_other_than_by_arn_fails" {
  command = plan

  variables {
    backends = [{ secret_env = { MY_KEK = "my-kek" } }]
  }

  expect_failures = [aws_iam_role.server]
}

run "a_backend_for_google_cloud_fails" {
  command = plan

  variables {
    backends = [{ permissions = [{ permission = "roles/storage.objectUser", scope = "bucket", resource = "b" }] }]
  }

  expect_failures = [var.backends]
}

run "an_ingress_for_cloud_run_fails" {
  command = plan

  variables {
    backends = [{ service_ingress = "internal-and-load-balancer" }]
  }

  expect_failures = [var.backends]
}

run "invokers_fail" {
  command = plan

  variables {
    backends = [{ invokers = ["invoker@p.iam.gserviceaccount.com"] }]
  }

  expect_failures = [var.backends]
}

run "an_image_outside_ecr_fails" {
  command = plan

  variables {
    image = "ghcr.io/mycroblog/my:0.1.0"
  }

  expect_failures = [var.image]
}

run "a_worker_shorter_than_a_lease_fails" {
  command = plan

  variables {
    worker_timeout_seconds = 300
  }

  expect_failures = [var.worker_timeout_seconds]
}

run "a_queue_that_hides_messages_for_less_than_the_timeout_fails" {
  command = plan

  variables {
    backends = [
      { secret_env = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" } },
      {
        event_sources = {
          jobs = { arn = "arn:aws:sqs:ap-northeast-1:111122223333:my-jobs", visibility_timeout_seconds = 30 }
        }
      },
    ]
  }

  expect_failures = [aws_lambda_event_source_mapping.worker]
}

# Several backends add to the one setting, which is absent while none does.
run "billing_sources_are_absent_without_a_billing_backend" {
  command = plan

  assert {
    condition     = !contains(keys(output.env), "MY_BILLING_SOURCES")
    error_message = "a deployment without a billing backend must not name the setting"
  }
}

run "two_backends_add_to_billing_sources" {
  command = plan

  variables {
    backends = [
      { env = { MY_DATABASE = "postgres" }, secret_env = { MY_DATABASE_URL = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:db-AbCdEf", MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" } },
      { billing_sources = ["gcp"], env = { MY_BILLING_GCP_METHOD = "budget" } },
      { billing_sources = ["aws"] },
    ]
  }

  assert {
    condition     = output.env["MY_BILLING_SOURCES"] == "aws,gcp"
    error_message = "the setting must list each source of the backends"
  }
}

run "a_billing_source_named_twice_fails" {
  command = plan

  variables {
    backends = [
      { env = { MY_DATABASE = "postgres" }, secret_env = { MY_DATABASE_URL = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:db-AbCdEf", MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" } },
      { billing_sources = ["aws"] },
      { billing_sources = ["aws"] },
    ]
  }

  expect_failures = [aws_iam_role.server]
}

run "billing_sources_beside_the_setting_in_env_fail" {
  command = plan

  variables {
    env = { MY_BILLING_SOURCES = "aws" }
    backends = [
      { env = { MY_DATABASE = "postgres" }, secret_env = { MY_DATABASE_URL = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:db-AbCdEf", MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" } },
      { billing_sources = ["gcp"] },
    ]
  }

  expect_failures = [aws_iam_role.server]
}
