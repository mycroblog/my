# SPDX-License-Identifier: AGPL-3.0-only

output "service_name" {
  description = "Name of the web function."
  value       = aws_lambda_function.server["web"].function_name
}

output "function_url" {
  description = "URL Lambda gave the web function. The public origin stays https://{host}."
  value       = aws_lambda_function_url.web.function_url
}

output "origin_host" {
  description = "Host name of the Function URL, for an ingress that takes its origin as an input."
  value       = trimsuffix(trimprefix(aws_lambda_function_url.web.function_url, "https://"), "/")
}

output "role_arn" {
  description = "Execution role of every function."
  value       = aws_iam_role.server.arn
}

output "worker_function" {
  description = "Name of the function the queue invokes."
  value       = aws_lambda_function.server["worker"].function_name
}

output "migration_function" {
  description = "Name of the function the upgrade script invokes."
  value       = aws_lambda_function.server["migrate"].function_name
}

output "console" {
  description = "What console.sh needs to run a my command, or null while console_enabled is off. Every field but platform belongs to the platform."
  value = var.console_enabled ? {
    platform = "aws-lambda"
    region   = data.aws_region.current.region
    function = aws_lambda_function.server["console"].function_name
    service  = aws_lambda_function.server["web"].function_name
  } : null
}

output "env" {
  description = "Merged settings of every function, without GOMEMLIMIT, MY_ROLE, and the settings of the Web Adapter."
  value       = local.env
}

output "secret_env" {
  description = "Merged secret settings of the web and worker functions: the name the server reads to the secret's ARN."
  value       = local.secret_arns
}

output "one_off_secret_env" {
  description = "Merged secret settings of the migration and console functions, which take direct_secret_env over secret_env."
  value       = local.one_off_secret_arns
}

output "secret_references" {
  description = "The variables that carry them, which the server has yet to read."
  value       = local.secret_refs
}
