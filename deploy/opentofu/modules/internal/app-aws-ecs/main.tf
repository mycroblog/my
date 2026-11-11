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

module "workers" {
  source = "../workers"

  workers = var.workers
  fields  = ["instances", "cpu", "memory_mib", "concurrency"]
}

locals {
  container_name = "my"
  container_port = 8080

  # One setting, which several backends add to. A source named twice is listed below as a setting named twice.
  billing_sources = flatten([for b in var.backends : b.billing_sources])
  named_settings = concat(
    flatten([for b in var.backends : concat(keys(b.env), keys(b.secret_env), keys(b.secret_files))]),
    keys(var.env),
    keys(var.secret_env),
    keys(var.secret_files),
    [for source in local.billing_sources : "billing_sources: ${source}"],
  )

  # Nothing of the platform stands in front of a task, so only an ingress names proxies.
  ingress_trust = [for b in var.backends : b.trusted_proxies if b.trusted_proxies != null]
  base_env = merge(
    {
      MY_HOST    = var.host
      MY_PROFILE = var.profile
    },
    length(local.ingress_trust) == 0 ? {} : { MY_TRUSTED_PROXIES = join(", ", flatten(local.ingress_trust)) },
    length(local.billing_sources) == 0 ? {} : {
      MY_BILLING_SOURCES = join(",", sort(distinct(local.billing_sources)))
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

  # Fargate mounts no secret, so secret_files reach the server as the variable of the name without _FILE.
  secret_files = merge(var.secret_files, [for b in var.backends : b.secret_files]...)
  secret_env   = merge(var.secret_env, [for b in var.backends : b.secret_env]...)
  secret_arns  = merge(local.secret_env, { for name, arn in local.secret_files : trimsuffix(name, "_FILE") => arn })

  # The one-off tasks differ from the service here: my schema apply holds a session-level lock, which a pooler in
  # transaction mode does not keep, so they take what a backend names in direct_secret_env.
  direct_secret_env      = merge([for b in var.backends : b.direct_secret_env]...)
  direct_names           = flatten([for b in var.backends : keys(b.direct_secret_env)])
  direct_without_setting = setsubtract(local.direct_names, keys(local.secret_arns))
  one_off_arns           = merge(local.secret_arns, local.direct_secret_env)
  all_secret_arns        = distinct(concat(values(local.secret_arns), values(local.direct_secret_env)))

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

  # The task's own group, which admits the load balancer, and one for each backend in the VPC, which that backend admits.
  # The service and every one-off task join them all: a migration needs the database as much as the service does.
  task_security_group_ids = distinct(concat([aws_security_group.task.id], flatten([for b in var.backends : b.security_group_ids])))

  load_balancers    = [for b in var.backends : b.load_balancer if b.load_balancer != null]
  has_load_balancer = length(local.load_balancers) > 0

  # A public address is of use only in a public subnet, and a private subnet is what the rest of the network is for.
  task_subnet_ids  = var.assign_public_ip ? var.network.public_subnet_ids : var.network.private_subnet_ids
  private_networks = distinct([for b in var.backends : b.private_network if b.private_network != null])

  # upgrade.sh and console.sh read this to start a one-off task, the first time before the service exists.
  network = {
    awsvpcConfiguration = {
      subnets        = local.task_subnet_ids
      securityGroups = local.task_security_group_ids
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
      role    = "all"
      env     = local.server_env
    }
  }
  # A worker differs from the default service in these alone, and in having no load balancer and no way in.
  has_workers         = length(var.workers) > 0
  server_env          = local.has_workers ? { MY_WORKER_JOB_TYPES_EXCEPT = join(",", module.workers.all_patterns) } : {}
  workers_of_backends = merge([for b in var.backends : b.workers]...)
  worker_settings     = ["MY_WORKER_JOB_TYPES", "MY_WORKER_JOB_TYPES_EXCEPT", "MY_WORKER_JOB_CONCURRENCY"]
  workers = {
    for key, w in module.workers.workers : key => {
      name      = "${var.name}-${key}"
      instances = coalesce(w.instances, 1)
      cpu       = coalesce(w.cpu, var.cpu)
      memory    = coalesce(w.memory_mib, var.memory_mib)
      job_types = w.job_types
      env = merge(
        { MY_WORKER_JOB_TYPES = join(",", w.job_types) },
        w.concurrency == null ? {} : { MY_WORKER_JOB_CONCURRENCY = tostring(w.concurrency) },
      )
      concurrency = w.concurrency
    }
  }
  worker_tasks = {
    for key, w in local.workers : "worker-${key}" => {
      family  = w.name
      cpu     = w.cpu
      memory  = w.memory
      secrets = local.secret_arns
      command = null
      ports   = []
      role    = "worker"
      env     = w.env
    }
  }
  worker_way_out = local.has_workers ? { ipv4 = { cidr_ipv4 = "0.0.0.0/0", cidr_ipv6 = null }, ipv6 = { cidr_ipv4 = null, cidr_ipv6 = "::/0" } } : {}

  # What a worker's concurrency must stay below: a slot needs a connection to claim and to record. The default of the pool is the
  # greater of 4 and the CPU count, so it is known only up to 4 CPUs, and a backend that cannot say leaves it to the server's start.
  pools = [for b in var.backends : b.pool_max_conns if b.pool_max_conns != null]
  workers_over_the_pool = [
    for key, w in local.workers : key
    if w.concurrency != null && length(local.pools) > 0 && (
      w.concurrency >= (local.pools[0] > 0 ? local.pools[0] : (w.cpu <= 4096 ? 4 : w.concurrency + 1))
    )
  ]
  one_off = {
    for key in concat(["migrate"], var.console_enabled ? ["console"] : []) : key => {
      family  = "${var.name}-${key}"
      cpu     = var.job_cpu
      memory  = var.job_memory_mib
      secrets = local.one_off_arns
      command = ["version"]
      ports   = []
      role    = "all"
      env     = {}
    }
  }
  tasks = merge(local.long_running, local.worker_tasks, local.one_off)
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
      condition     = alltrue([for n in local.private_networks : n == var.network.vpc_id])
      error_message = "A backend has private addresses in another VPC than the tasks. Give the app the same network as the backend."
    }
    precondition {
      condition     = length(module.workers.problems) == 0
      error_message = "workers: ${join(" ", module.workers.problems)}"
    }
    precondition {
      condition     = jsonencode(module.workers.job_types) == jsonencode(local.workers_of_backends)
      error_message = "The app was given the workers ${jsonencode(module.workers.job_types)}, and its queue module ${jsonencode(local.workers_of_backends)}. Give queue-db the same value."
    }
    precondition {
      condition     = !local.has_workers || local.queue == "db"
      error_message = "queue = ${local.queue} cannot split its jobs by worker_job_types. Use queue-db."
    }
    precondition {
      condition     = !local.has_workers || local.stream != "hub"
      error_message = "With workers, stream = hub would keep the updates of a job in the task that ran it, and away from the streams. Add a stream-* module, or set MY_STREAM to postgres."
    }
    precondition {
      condition     = !local.has_workers || length(setintersection(local.setting_names, local.worker_settings)) == 0
      error_message = "These are what workers sets, so they cannot be named beside it: ${join(", ", local.worker_settings)}."
    }
    precondition {
      condition     = length(local.pools) <= 1
      error_message = "More than one backend says what the pool of the database is."
    }
    precondition {
      condition     = length(local.workers_over_the_pool) == 0
      error_message = "The concurrency of ${join(", ", local.workers_over_the_pool)} is not below the pool of the database, so the server would not start. Raise pool_max_conns in url_parameters of the database, or lower concurrency."
    }
    precondition {
      condition     = var.network == null || lookup(local.env, "MY_HTTP_ALLOW_PRIVATE_NETWORKS", "false") != "true"
      error_message = "http_allow_private_networks would let what a remote server names reach this network. Leave it off on a network."
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
      condition     = length(local.direct_names) == length(distinct(local.direct_names)) && length(local.direct_without_setting) == 0
      error_message = "direct_secret_env overrides a setting of secret_env for the one-off tasks, once: each name must be in secret_env and in one backend's direct_secret_env. Check: ${join(", ", local.direct_names)}."
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
  vpc_id      = var.network.vpc_id
}

# What a worker's tasks join in place of the group above: it admits nothing at all.
resource "aws_security_group" "worker" {
  name        = "${var.name}-worker"
  description = "Workers of ${var.name}"
  vpc_id      = var.network.vpc_id

  lifecycle {
    enabled = local.has_workers
  }
}

resource "aws_vpc_security_group_egress_rule" "worker_out" {
  for_each = local.worker_way_out

  security_group_id = aws_security_group.worker.id
  description       = "The image, the database, the cloud's APIs, and federation"
  ip_protocol       = "-1"
  cidr_ipv4         = each.value.cidr_ipv4
  cidr_ipv6         = each.value.cidr_ipv6
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
  skip_destroy = contains(keys(local.one_off), each.key) == false

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
        for name, value in merge(local.env, each.value.env, {
          GOMEMLIMIT = "${floor(each.value.memory * 0.9)}MiB"
          MY_ROLE    = each.value.role
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
    subnets          = local.task_subnet_ids
    security_groups  = local.task_security_group_ids
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

# A service for each worker, with everything of the default service but what local.workers says, no load balancer, and a group
# that admits nothing. upgrade.sh finds these in the cluster, and moves them with the default after the migration.
resource "aws_ecs_service" "worker" {
  for_each = local.workers

  name            = each.value.name
  cluster         = aws_ecs_cluster.this.arn
  task_definition = aws_ecs_task_definition.task["worker-${each.key}"].arn
  desired_count   = each.value.instances
  launch_type     = "FARGATE"

  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200

  # With no load balancer, what holds a bad image back is only that its tasks do not start.
  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  network_configuration {
    subnets          = local.task_subnet_ids
    security_groups  = distinct(concat([aws_security_group.worker.id], flatten([for b in var.backends : b.security_group_ids])))
    assign_public_ip = var.assign_public_ip
  }

  depends_on = [aws_vpc_security_group_egress_rule.worker_out]
}
