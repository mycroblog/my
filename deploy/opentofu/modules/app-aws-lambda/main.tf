# SPDX-License-Identifier: AGPL-3.0-only

# The server on AWS Lambda: a web function behind a Function URL and a worker function that an event source mapping
# invokes. They are two because the Web Adapter has one invoke mode per function: streams need response_stream, and the
# events of the queue need buffered. This module has never been applied, and no released image runs in it: read the README.

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.0, < 7.0"
    }
  }
}

data "aws_region" "current" {}

locals {
  named_settings = concat(
    flatten([for b in var.backends : concat(keys(b.env), keys(b.secret_env), keys(b.secret_files))]),
    keys(var.env),
    keys(var.secret_env),
    keys(var.secret_files),
  )

  # The Web Adapter connects to the server from loopback, and an ingress says what connects to the Function URL.
  # MY_TRUSTED_PROXIES in env replaces the value only without an ingress: with one, the setting is named twice and fails.
  ingress_trust = [for b in var.backends : b.trusted_proxies if b.trusted_proxies != null]
  base_env = merge(
    {
      MY_HOST    = var.host
      MY_PROFILE = var.profile
    },
    contains(local.named_settings, "MY_TRUSTED_PROXIES") && length(local.ingress_trust) == 0 ? {} : {
      MY_TRUSTED_PROXIES = join(", ", concat(["127.0.0.1/32"], flatten(local.ingress_trust)))
    },
  )

  # Every setting by the name the server reads: MY_KEK and MY_KEK_FILE are one setting.
  setting_names = [
    for name in concat(["GOMEMLIMIT", "MY_ROLE"], keys(local.base_env), local.named_settings) : trimsuffix(name, "_FILE")
  ]
  duplicate_settings = [
    for name in distinct(local.setting_names) : name
    if length([for other in local.setting_names : other if other == name]) > 1
  ]

  # merge() alone would let the last one win without a word; the precondition below refuses that first.
  env = merge(local.base_env, var.env, [for b in var.backends : b.env]...)

  # Lambda neither resolves a secret into a variable nor mounts one, so secret_env and secret_files reach the server the
  # same way: a variable that names the secret, which the server reads at start. That reading does not exist yet.
  secret_files = merge(var.secret_files, [for b in var.backends : b.secret_files]...)
  secret_env   = merge(var.secret_env, [for b in var.backends : b.secret_env]...)
  secret_arns  = merge(local.secret_env, { for name, arn in local.secret_files : trimsuffix(name, "_FILE") => arn })
  secret_refs  = { for name, arn in local.secret_arns : "SECRET_ARN_${name}" => arn }

  # The one-off functions differ from the others here: my schema apply holds a session-level lock, which a pooler in
  # transaction mode does not keep, so they take what a backend names in direct_secret_env.
  direct_secret_env      = merge([for b in var.backends : b.direct_secret_env]...)
  direct_names           = flatten([for b in var.backends : keys(b.direct_secret_env)])
  direct_without_setting = setsubtract(local.direct_names, keys(local.secret_arns))
  one_off_secret_arns    = merge(local.secret_arns, local.direct_secret_env)
  one_off_secret_refs    = { for name, arn in local.one_off_secret_arns : "SECRET_ARN_${name}" => arn }
  all_secret_arns        = distinct(concat(values(local.secret_arns), values(local.direct_secret_env)))

  # The server refuses to start without these, but only once it runs: say it at plan time.
  key_manager = lookup(local.env, "MY_KEY_MANAGER", "local")
  kek_missing = (
    local.key_manager == "local" ? !contains(local.setting_names, "MY_KEK") : !contains(local.setting_names, "MY_KMS_KEY")
  )

  permissions = flatten([for b in var.backends : b.permissions])
  # Attached to a VPC when a backend lives in one. What the function may reach is the sum of these groups.
  security_group_ids = distinct(concat(var.vpc_security_group_ids, flatten([for b in var.backends : b.security_group_ids])))
  vpc_attached       = length(var.vpc_subnet_ids) > 0
  event_sources      = merge([for b in var.backends : b.event_sources]...)

  # The same image with other settings: one definition makes every function, as one block makes the jobs on Cloud Run.
  # The adapter's defaults are the server's: port 8080, and /events for the events of the queue.
  long_running = {
    web = {
      role    = "web"
      memory  = var.web_memory_mib
      timeout = var.web_timeout_seconds
      command = null
      adapter = { AWS_LWA_INVOKE_MODE = "response_stream", AWS_LWA_READINESS_CHECK_PATH = "/readyz" }
    }
    worker = {
      role    = "worker"
      memory  = var.worker_memory_mib
      timeout = var.worker_timeout_seconds
      command = null
      adapter = { AWS_LWA_INVOKE_MODE = "buffered", AWS_LWA_READINESS_CHECK_PATH = "/readyz" }
    }
  }
  one_off = {
    for key in concat(["migrate"], var.console_enabled ? ["console"] : []) : key => {
      role    = "all"
      memory  = var.job_memory_mib
      timeout = var.job_timeout_seconds
      command = var.one_off_command
      adapter = {}
    }
  }
  functions = merge(local.long_running, local.one_off)

  function_env = {
    for key, f in local.functions : key => merge(local.env, contains(keys(local.one_off), key) ? local.one_off_secret_refs : local.secret_refs, f.adapter, {
      GOMEMLIMIT = "${floor(f.memory * 0.9)}MiB"
      MY_ROLE    = f.role
    })
  }
}

resource "aws_iam_role" "server" {
  name = "${var.name}-server"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Action    = "sts:AssumeRole"
      Principal = { Service = "lambda.amazonaws.com" }
    }]
  })

  # Here because this resource always exists, and the plan fails before anything is created.
  lifecycle {
    precondition {
      condition     = length(local.duplicate_settings) == 0
      error_message = "These settings are named more than once across the backends, env, secret_env, secret_files, and what this module sets itself: ${join(", ", local.duplicate_settings)}."
    }
    precondition {
      condition     = length(local.ingress_trust) <= 1
      error_message = "More than one backend says what is in front of the function with trusted_proxies. Use one ingress-* module for that; a media host returns none."
    }
    precondition {
      condition     = local.vpc_attached || length(local.security_group_ids) == 0
      error_message = "A backend lives in a VPC, or vpc_security_group_ids is set, but vpc_subnet_ids is empty, so the functions cannot reach it. Name the private subnets."
    }
    precondition {
      condition     = !local.vpc_attached || length(local.security_group_ids) > 0
      error_message = "vpc_subnet_ids needs at least one security group. A function in a VPC reaches only what its groups allow, so give vpc_security_group_ids a group that allows the way out."
    }
    precondition {
      condition     = !local.kek_missing
      error_message = "No KEK: with key_manager = ${local.key_manager} the server needs ${local.key_manager == "local" ? "kek" : "kms_key"}. Add a kek-* module to backends."
    }
    precondition {
      condition     = length(local.direct_names) == length(distinct(local.direct_names)) && length(local.direct_without_setting) == 0
      error_message = "direct_secret_env overrides a setting of secret_env for the one-off functions, once: each name must be in secret_env and in one backend's direct_secret_env. Check: ${join(", ", local.direct_names)}."
    }
    precondition {
      condition     = alltrue([for arn in local.all_secret_arns : startswith(arn, "arn:")])
      error_message = "A secret on AWS is named by its full ARN, which is what the role is granted and what the server reads."
    }
  }
}

resource "aws_cloudwatch_log_group" "function" {
  for_each = local.functions

  name              = "/aws/lambda/${var.name}-${each.key}"
  retention_in_days = var.log_retention_days
}

# One document, whatever the backends ask: AWS has one kind of grant, so nothing here is keyed on a name, and a backend may
# name an attribute of its resources. A role's inline policies share 10,240 characters.
data "aws_iam_policy_document" "server" {
  statement {
    sid       = "Logs"
    actions   = ["logs:CreateLogStream", "logs:PutLogEvents"]
    resources = [for group in aws_cloudwatch_log_group.function : "${group.arn}:*"]
  }

  dynamic "statement" {
    for_each = length(local.all_secret_arns) > 0 ? [1] : []
    content {
      sid       = "Secrets"
      actions   = ["secretsmanager:GetSecretValue"]
      resources = local.all_secret_arns
    }
  }

  # What Lambda needs of the function's role to create the network interfaces in the VPC.
  dynamic "statement" {
    for_each = local.vpc_attached ? [1] : []
    content {
      sid = "VpcNetworkInterfaces"
      actions = [
        "ec2:CreateNetworkInterface",
        "ec2:DescribeNetworkInterfaces",
        "ec2:DescribeSubnets",
        "ec2:DeleteNetworkInterface",
        "ec2:AssignPrivateIpAddresses",
        "ec2:UnassignPrivateIpAddresses",
      ]
      resources = ["*"]
    }
  }

  # What an event source mapping needs of the function's role to poll the queue.
  dynamic "statement" {
    for_each = length(local.event_sources) > 0 ? [1] : []
    content {
      sid       = "EventSources"
      actions   = ["sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:GetQueueAttributes"]
      resources = [for source in values(local.event_sources) : source.arn]
    }
  }

  dynamic "statement" {
    for_each = local.permissions
    content {
      actions   = [statement.value.permission]
      resources = [statement.value.resource]

      dynamic "condition" {
        for_each = statement.value.conditions
        content {
          test     = condition.value.test
          variable = condition.value.variable
          values   = condition.value.values
        }
      }
    }
  }
}

resource "aws_iam_role_policy" "server" {
  name   = "${var.name}-server"
  role   = aws_iam_role.server.id
  policy = data.aws_iam_policy_document.server.json

  lifecycle {
    precondition {
      condition     = length(data.aws_iam_policy_document.server.minified_json) <= 10240
      error_message = "The role's policy is above the 10,240 characters that IAM allows for a role's inline policies."
    }
  }
}

resource "aws_lambda_function" "server" {
  for_each = local.functions

  function_name = "${var.name}-${each.key}"
  role          = aws_iam_role.server.arn
  package_type  = "Image"
  image_uri     = var.image
  architectures = [var.architecture]
  memory_size   = each.value.memory
  timeout       = each.value.timeout

  dynamic "image_config" {
    for_each = each.value.command == null ? [] : [1]
    content {
      command = each.value.command
    }
  }

  environment {
    variables = local.function_env[each.key]
  }

  reserved_concurrent_executions = each.key == "web" ? var.web_reserved_concurrency : null

  dynamic "vpc_config" {
    for_each = local.vpc_attached ? [1] : []
    content {
      subnet_ids         = var.vpc_subnet_ids
      security_group_ids = local.security_group_ids
    }
  }

  lifecycle {
    # upgrade.sh changes the image, as on the other platforms, and apply must not move it back.
    ignore_changes = [image_uri]

    precondition {
      condition     = sum([for name, value in local.function_env[each.key] : length(name) + length(value)]) <= 4096
      error_message = "The settings of the ${each.key} function are above the 4 KB that Lambda allows for all its environment variables together. MY_TRUSTED_PROXIES with a long list of ranges is the usual cause."
    }
  }

  depends_on = [aws_iam_role_policy.server, aws_cloudwatch_log_group.function]
}

# Open: the server checks what it must itself, and its paths are public, since federation needs them. An origin access control
# would close the URL to all but CloudFront, but then a client must send the SHA-256 of every POST body, which a peer does not.
resource "aws_lambda_function_url" "web" {
  function_name      = aws_lambda_function.server["web"].function_name
  authorization_type = "NONE"
  invoke_mode        = "RESPONSE_STREAM"
}

resource "aws_lambda_permission" "url" {
  statement_id           = "FunctionURLAllowPublicAccess"
  function_name          = aws_lambda_function.server["web"].function_name
  action                 = "lambda:InvokeFunctionUrl"
  principal              = "*"
  function_url_auth_type = "NONE"
}

resource "aws_lambda_permission" "url_invoke" {
  statement_id             = "FunctionURLInvokeAllowPublicAccess"
  function_name            = aws_lambda_function.server["web"].function_name
  action                   = "lambda:InvokeFunction"
  principal                = "*"
  invoked_via_function_url = true
}

# Nothing calls the worker for its queue: Lambda polls the queue and invokes the function.
resource "aws_lambda_event_source_mapping" "worker" {
  for_each = local.event_sources

  function_name    = aws_lambda_function.server["worker"].function_name
  event_source_arn = each.value.arn
  batch_size       = var.worker_batch_size
  # Without it the whole batch counts as done, whatever the server answers.
  function_response_types = ["ReportBatchItemFailures"]

  scaling_config {
    maximum_concurrency = var.worker_max_concurrency
  }

  lifecycle {
    precondition {
      condition     = var.worker_timeout_seconds <= each.value.visibility_timeout_seconds
      error_message = "The visibility timeout of the queue ${each.key} is below the worker's timeout, which Lambda refuses when it creates the mapping."
    }
  }

  depends_on = [aws_iam_role_policy.server]
}
