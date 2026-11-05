# SPDX-License-Identifier: AGPL-3.0-only

# The server on Amazon ECS with Fargate: one service whose process stays up, and one-off tasks for the migration and the
# console. ECS resolves each secret into a variable when a task starts, so no secret value reaches the plan or the state.
# This module has never been applied: read the README.

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
  container_name = "my"
  container_port = 8080

  named_settings = concat(
    flatten([for b in var.backends : concat(keys(b.env), keys(b.secret_env), keys(b.secret_files))]),
    keys(var.env),
    keys(var.secret_env),
    keys(var.secret_files),
  )

  # Nothing of the platform stands in front of a task, so only an ingress names proxies.
  ingress_trust = [for b in var.backends : b.trusted_proxies if b.trusted_proxies != null]
  base_env = merge(
    {
      MY_HOST    = var.host
      MY_PROFILE = var.profile
    },
    length(local.ingress_trust) == 0 ? {} : { MY_TRUSTED_PROXIES = join(", ", flatten(local.ingress_trust)) },
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

  # Fargate mounts no secret, so secret_files reach the server as the variable of the name without _FILE.
  secret_files = merge(var.secret_files, [for b in var.backends : b.secret_files]...)
  secret_env   = merge(var.secret_env, [for b in var.backends : b.secret_env]...)
  secret_arns  = merge(local.secret_env, { for name, arn in local.secret_files : trimsuffix(name, "_FILE") => arn })

  # The migration and the console take the service's secrets, with what a backend names for one-off work in their place.
  direct_secret_env = merge([for b in var.backends : b.direct_secret_env]...)
  one_off_arns      = merge(local.secret_arns, local.direct_secret_env)
  all_secret_arns   = concat(values(local.secret_arns), values(local.direct_secret_env))

  # The server refuses to start without these, but only once it runs: say it at plan time.
  key_manager = lookup(local.env, "MY_KEY_MANAGER", "local")
  kek_missing = (
    local.key_manager == "local" ? !contains(local.setting_names, "MY_KEK") : !contains(local.setting_names, "MY_KMS_KEY")
  )

  # What only fails once the server runs, or later: no process would run the jobs, or a restart would lose the data.
  queue       = lookup(local.env, "MY_QUEUE", "db")
  stream      = lookup(local.env, "MY_STREAM", "hub")
  on_disk     = [for name, value in { MY_DATABASE = lookup(local.env, "MY_DATABASE", "sqlite"), MY_BLOB = lookup(local.env, "MY_BLOB", "fs") } : name if contains(["sqlite", "fs"], value)]
  permissions = flatten([for b in var.backends : b.permissions])

  load_balancers    = [for b in var.backends : b.load_balancer if b.load_balancer != null]
  has_load_balancer = length(local.load_balancers) > 0

  # upgrade.sh and console.sh read this to start a one-off task, the first time before the service exists.
  network = {
    awsvpcConfiguration = {
      subnets        = var.subnet_ids
      securityGroups = [aws_security_group.task.id]
      assignPublicIp = var.assign_public_ip ? "ENABLED" : "DISABLED"
    }
  }

  # The same image with other settings: one definition makes every task definition, as one block makes the jobs on Cloud Run.
  # The family of each is what upgrade.sh and console.sh name, so do not change one here without them. A one-off task always
  # gets its command from them; "version" is there so that a task started by hand does nothing.
  long_running = {
    server = {
      family  = var.name
      cpu     = var.cpu
      memory  = var.memory_mib
      secrets = local.secret_arns
      command = null
      ports   = [{ containerPort = local.container_port, hostPort = local.container_port, protocol = "tcp" }]
    }
  }
  one_off = {
    for key in concat(["migrate"], var.console_enabled ? ["console"] : []) : key => {
      family  = "${var.name}-${key}"
      cpu     = var.job_cpu
      memory  = var.job_memory_mib
      secrets = local.one_off_arns
      command = ["version"]
      ports   = []
    }
  }
  tasks = merge(local.long_running, local.one_off)
}

resource "aws_iam_role" "execution" {
  name = "${var.name}-execution"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Action    = "sts:AssumeRole"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
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
      error_message = "More than one backend says what is in front of the service with trusted_proxies. Use one ingress-* module for that; a media host returns none."
    }
    precondition {
      condition     = length(local.load_balancers) <= 1
      error_message = "More than one backend names a load balancer. This module attaches the service to one."
    }
    precondition {
      condition     = !local.kek_missing
      error_message = "No KEK: with key_manager = ${local.key_manager} the server needs ${local.key_manager == "local" ? "kek" : "kms_key"}. Add a kek-* module to backends."
    }
    precondition {
      condition     = alltrue([for arn in local.all_secret_arns : startswith(arn, "arn:")])
      error_message = "A secret on AWS is named by its full ARN, which is what the execution role is granted and what ECS resolves."
    }
    precondition {
      condition     = contains(["db", "inline"], local.queue)
      error_message = "queue = ${local.queue} runs jobs only when the platform calls the server, which nothing does here. Use queue-db: a process here stays up."
    }
    precondition {
      condition     = var.desired_count == 1 || local.stream != "hub"
      error_message = "With desired_count above 1, stream = hub would give each stream only the changes of its own task. Add a stream-* module, or set MY_STREAM to postgres."
    }
    precondition {
      condition     = length(local.on_disk) == 0
      error_message = "A task loses its disk when it stops, and these settings would keep their data there: ${join(", ", local.on_disk)}. Add a db-* and a blob-* module."
    }
  }
}

# Starts the tasks: it pulls the image, writes the logs, and reads the secrets, which the server itself gets as variables and
# has no permission to read. One document, whatever the backends ask.
data "aws_iam_policy_document" "execution" {
  statement {
    sid = "Image"
    actions = [
      "ecr:GetAuthorizationToken",
      "ecr:BatchCheckLayerAvailability",
      "ecr:GetDownloadUrlForLayer",
      "ecr:BatchGetImage",
    ]
    resources = ["*"]
  }

  statement {
    sid       = "Logs"
    actions   = ["logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["${aws_cloudwatch_log_group.tasks.arn}:*"]
  }

  dynamic "statement" {
    for_each = length(local.all_secret_arns) > 0 ? [1] : []
    content {
      sid       = "Secrets"
      actions   = ["secretsmanager:GetSecretValue"]
      resources = local.all_secret_arns
    }
  }
}

resource "aws_iam_role_policy" "execution" {
  name   = "${var.name}-execution"
  role   = aws_iam_role.execution.id
  policy = data.aws_iam_policy_document.execution.json

  lifecycle {
    precondition {
      condition     = length(data.aws_iam_policy_document.execution.minified_json) <= 10240
      error_message = "The execution role's policy is above the 10,240 characters that IAM allows for a role's inline policies."
    }
  }
}

# What the server may do: the identity of the service and of its one-off tasks alike.
resource "aws_iam_role" "task" {
  name = "${var.name}-task"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Action    = "sts:AssumeRole"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
    }]
  })
}

data "aws_iam_policy_document" "task" {
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

# IAM refuses a policy with no statement, which a deployment without a backend that asks for anything would be.
resource "aws_iam_role_policy" "task" {
  name   = "${var.name}-task"
  role   = aws_iam_role.task.id
  policy = data.aws_iam_policy_document.task.json

  lifecycle {
    enabled = length(local.permissions) > 0

    precondition {
      condition     = length(data.aws_iam_policy_document.task.minified_json) <= 10240
      error_message = "The task role's policy is above the 10,240 characters that IAM allows for a role's inline policies."
    }
  }
}

resource "aws_cloudwatch_log_group" "tasks" {
  name              = "/ecs/${var.name}"
  retention_in_days = var.log_retention_days
}

# Closed to everything but the load balancer: nothing else can connect to a task, whether it has a public address or not.
resource "aws_security_group" "task" {
  name        = "${var.name}-task"
  description = "Tasks of ${var.name}"
  vpc_id      = var.vpc_id
}

resource "aws_vpc_security_group_ingress_rule" "from_load_balancer" {
  security_group_id            = aws_security_group.task.id
  description                  = "From the load balancer"
  referenced_security_group_id = try(local.load_balancers[0].security_group_id, null)
  ip_protocol                  = "tcp"
  from_port                    = local.container_port
  to_port                      = local.container_port

  lifecycle {
    enabled = local.has_load_balancer
  }
}

resource "aws_vpc_security_group_egress_rule" "out" {
  for_each = { ipv4 = { cidr_ipv4 = "0.0.0.0/0", cidr_ipv6 = null }, ipv6 = { cidr_ipv4 = null, cidr_ipv6 = "::/0" } }

  security_group_id = aws_security_group.task.id
  description       = "The image, the database, the cloud's APIs, and federation"
  ip_protocol       = "-1"
  cidr_ipv4         = each.value.cidr_ipv4
  cidr_ipv6         = each.value.cidr_ipv6
}

# Where the running image lives, since upgrade.sh changes it and apply must not move it back. An ECS task definition is an
# immutable revision that holds the image and every setting together, so ignore_changes there would also stop settings from
# reaching the service. Instead upgrade.sh writes this parameter, a refresh reads it, and the task definitions below take it.
resource "aws_ssm_parameter" "image" {
  name           = "/my/${var.name}/image"
  description    = "Image of the service, which upgrade.sh writes."
  type           = "String"
  insecure_value = var.image

  lifecycle {
    ignore_changes = [insecure_value]
  }
}

resource "aws_ssm_parameter" "network" {
  name           = "/my/${var.name}/network"
  description    = "Network of the one-off tasks, which upgrade.sh reads."
  type           = "String"
  insecure_value = jsonencode(local.network)
}

resource "aws_ecs_cluster" "this" {
  name = var.name
}

resource "aws_ecs_task_definition" "task" {
  for_each = local.tasks

  family                   = each.value.family
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = tostring(each.value.cpu)
  memory                   = tostring(each.value.memory)
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn

  # upgrade.sh and console.sh register revisions of their own. With this, a refresh reads the latest revision of the family
  # rather than the one apply made, so one that only took the image of the parameter above shows no change.
  track_latest = true
  # A deregistered revision cannot be rolled back to. The console's stay out: nothing extra stays around while it is off.
  skip_destroy = each.key == "server"

  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = upper(var.architecture)
  }

  container_definitions = jsonencode([merge(
    {
      name      = local.container_name
      image     = aws_ssm_parameter.image.insecure_value
      essential = true
      # The server's own shutdown takes under 30 seconds; a task gets 120 at most.
      stopTimeout = 60
      environment = [
        for name, value in merge(local.env, {
          GOMEMLIMIT = "${floor(each.value.memory * 0.9)}MiB"
          MY_ROLE    = "all"
        }) : { name = name, value = value }
      ]
      secrets      = [for name, arn in each.value.secrets : { name = name, valueFrom = arn }]
      portMappings = each.value.ports
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.tasks.name
          awslogs-region        = data.aws_region.current.region
          awslogs-stream-prefix = each.key
        }
      }
    },
    each.value.command == null ? {} : { command = each.value.command },
  )])

  # Everything a one-off task needs, so that the first deployment can target the migration's definition alone.
  depends_on = [
    aws_ecs_cluster.this,
    aws_iam_role_policy.execution,
    aws_iam_role_policy.task,
    aws_ssm_parameter.network,
    aws_vpc_security_group_egress_rule.out,
  ]
}

resource "aws_ecs_service" "server" {
  name            = var.name
  cluster         = aws_ecs_cluster.this.arn
  task_definition = aws_ecs_task_definition.task["server"].arn
  desired_count   = var.desired_count
  launch_type     = "FARGATE"

  # A new task must pass before an old one goes, so an upgrade runs one more for a while and a stream of hub splits for that time.
  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200
  health_check_grace_period_seconds  = local.has_load_balancer ? var.health_check_grace_seconds : null

  # What holds a bad image back: a deployment whose tasks do not start, or fail the load balancer's health check, goes back.
  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  network_configuration {
    subnets          = var.subnet_ids
    security_groups  = [aws_security_group.task.id]
    assign_public_ip = var.assign_public_ip
  }

  dynamic "load_balancer" {
    for_each = local.load_balancers
    content {
      target_group_arn = load_balancer.value.target_group_arn
      container_name   = local.container_name
      container_port   = local.container_port
    }
  }

  depends_on = [aws_vpc_security_group_ingress_rule.from_load_balancer, aws_vpc_security_group_egress_rule.out]
}
