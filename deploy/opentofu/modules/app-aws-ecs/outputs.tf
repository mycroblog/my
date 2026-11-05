# SPDX-License-Identifier: AGPL-3.0-only

output "service_name" {
  description = "Name of the service, which is also the name of its cluster and its task definition family."
  value       = aws_ecs_service.server.name
}

output "cluster_name" {
  description = "Name of the cluster."
  value       = aws_ecs_cluster.this.name
}

output "origin_host" {
  description = "Always null: a task's address changes with every task, so this service has no host of its own, and an ingress that takes one as its origin fails the plan. Use ingress-aws-alb."
  value       = null
}

output "migration_task_definition" {
  description = "Family of the task definition the upgrade script runs."
  value       = aws_ecs_task_definition.task["migrate"].family
}

output "security_group_id" {
  description = "Security group of the tasks, for a rule of your own when something else stands in front of them."
  value       = aws_security_group.task.id
}

output "task_security_group_ids" {
  description = "Every security group a task joins: its own and those the backends name."
  value       = local.task_security_group_ids
}

output "execution_role_arn" {
  description = "Role that starts the tasks and reads their secrets."
  value       = aws_iam_role.execution.arn
}

output "task_role_arn" {
  description = "Role the server runs as, in the service and the one-off tasks."
  value       = aws_iam_role.task.arn
}

output "log_group" {
  description = "CloudWatch Logs group of every task."
  value       = aws_cloudwatch_log_group.tasks.name
}

output "console" {
  description = "What console.sh needs to run a my command, or null while console_enabled is off. Every field but platform belongs to the platform."
  value = var.console_enabled ? {
    platform        = "aws-ecs"
    region          = data.aws_region.current.region
    cluster         = aws_ecs_cluster.this.name
    task_definition = aws_ecs_task_definition.task["console"].family
    container       = local.container_name
    service         = aws_ecs_service.server.name
    network         = jsonencode(local.network)
    log_group       = aws_cloudwatch_log_group.tasks.name
    log_prefix      = "console"
  } : null
}

output "env" {
  description = "Merged settings of every task, without GOMEMLIMIT and MY_ROLE."
  value       = local.env
}

output "secret_env" {
  description = "Merged secret settings of the service: the name the server reads to the secret's ARN."
  value       = local.secret_arns
}

output "one_off_secret_env" {
  description = "The same for the migration and console tasks, which take what a backend names for one-off work in the place of the service's."
  value       = local.one_off_arns
}
