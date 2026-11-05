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
      arn = "arn:aws:iam::111122223333:role/my"
    }
  }
  mock_resource "aws_cloudwatch_log_group" {
    defaults = {
      arn = "arn:aws:logs:ap-northeast-1:111122223333:log-group:/ecs/my"
    }
  }
  mock_resource "aws_ecs_cluster" {
    defaults = {
      arn = "arn:aws:ecs:ap-northeast-1:111122223333:cluster/my"
    }
  }
  mock_resource "aws_ecs_task_definition" {
    defaults = {
      arn = "arn:aws:ecs:ap-northeast-1:111122223333:task-definition/my:1"
    }
  }
}

variables {
  image            = "ghcr.io/mycroblog/my:0.1.0"
  host             = "social.example.org"
  vpc_id           = "vpc-1"
  subnet_ids       = ["subnet-1", "subnet-2"]
  assign_public_ip = true
  backends = [
    {
      env        = { MY_DATABASE = "postgres" }
      secret_env = { MY_DATABASE_URL = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:db-AbCdEf" }
    },
    {
      secret_env = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" }
    },
    {
      env = { MY_BLOB = "s3", MY_QUEUE = "db" }
      permissions = [
        {
          permission = "s3:GetObject"
          scope      = "policy"
          resource   = "arn:aws:s3:::media/*"
          conditions = [
            { test = "StringEquals", variable = "s3:ExistingObjectTag/x", values = ["y"] },
          ]
        },
      ]
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
      MY_BLOB     = "s3"
      MY_DATABASE = "postgres"
      MY_HOST     = "social.example.org"
      MY_MAIL     = "smtp"
      MY_PROFILE  = "production"
      MY_QUEUE    = "db"
    }
    error_message = "the settings must merge, and nothing of the platform stands in front of a task"
  }

  assert {
    condition     = [for s in jsondecode(aws_ecs_task_definition.task["server"].container_definitions)[0].secrets : s if s.name == "MY_VAPID_PRIVATE_KEY"][0].valueFrom == "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:vapid-AbCdEf"
    error_message = "a secret reaches the task through ECS's own secrets, by its ARN"
  }

  assert {
    condition     = length([for e in jsondecode(aws_ecs_task_definition.task["server"].container_definitions)[0].environment : e if e.name == "MY_DATABASE_URL" || startswith(e.name, "SECRET_ARN_")]) == 0
    error_message = "a secret must not reach the plain settings, in its value or as a reference that the server would have to read"
  }
}

run "the_service_is_one_process_that_stays_up" {
  command = plan

  assert {
    condition = (
      contains(jsondecode(aws_ecs_task_definition.task["server"].container_definitions)[0].environment, { name = "MY_ROLE", value = "all" }) &&
      contains(jsondecode(aws_ecs_task_definition.task["server"].container_definitions)[0].environment, { name = "GOMEMLIMIT", value = "921MiB" })
    )
    error_message = "the process runs the web and the jobs, and GOMEMLIMIT follows its memory"
  }

  assert {
    condition     = aws_ecs_service.server.launch_type == "FARGATE" && aws_ecs_service.server.deployment_circuit_breaker[0].enable && aws_ecs_service.server.deployment_circuit_breaker[0].rollback
    error_message = "a deployment that fails must go back on its own"
  }

  assert {
    condition     = length(aws_ecs_service.server.load_balancer) == 0 && aws_ecs_service.server.health_check_grace_period_seconds == null
    error_message = "without an ingress nothing is attached, and AWS refuses a grace period without a load balancer"
  }

  assert {
    condition     = output.console == null && !contains(keys(aws_ecs_task_definition.task), "console")
    error_message = "the console must not exist while it is off"
  }

  assert {
    condition     = jsondecode(aws_ssm_parameter.network.insecure_value).awsvpcConfiguration.assignPublicIp == "ENABLED" && aws_ecs_service.server.network_configuration[0].assign_public_ip
    error_message = "the one-off tasks must get the service's network"
  }

  assert {
    condition     = aws_ecs_task_definition.task["server"].track_latest && aws_ecs_task_definition.task["migrate"].track_latest
    error_message = "a revision that upgrade.sh registered must not show as a change"
  }

  assert {
    condition     = aws_ecs_task_definition.task["server"].skip_destroy && !aws_ecs_task_definition.task["migrate"].skip_destroy
    error_message = "a revision must stay for a rollback, on the service alone"
  }

  assert {
    condition     = jsondecode(aws_ecs_task_definition.task["server"].container_definitions)[0].image == "ghcr.io/mycroblog/my:0.1.0" && aws_ssm_parameter.image.insecure_value == "ghcr.io/mycroblog/my:0.1.0"
    error_message = "the image goes through the parameter that upgrade.sh writes"
  }

  assert {
    condition     = length(aws_vpc_security_group_ingress_rule.from_load_balancer) == 0 || aws_vpc_security_group_ingress_rule.from_load_balancer == null
    error_message = "without a load balancer no rule opens the tasks"
  }

  assert {
    condition     = output.origin_host == null
    error_message = "a task has no host of its own"
  }
}

# The seam for a database whose service goes through a pooler.
run "one_off_tasks_take_the_direct_settings_in_the_place_of_the_services" {
  command = plan

  variables {
    backends = [
      {
        env               = { MY_DATABASE = "postgres", MY_BLOB = "s3" }
        secret_env        = { MY_DATABASE_URL = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:pooled-AbCdEf" }
        direct_secret_env = { MY_DATABASE_URL = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:direct-AbCdEf" }
      },
      { secret_env = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" } },
    ]
    console_enabled = true
  }

  assert {
    condition     = output.secret_env["MY_DATABASE_URL"] == "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:pooled-AbCdEf"
    error_message = "the service keeps the pooled connection"
  }

  assert {
    condition = (
      [for s in jsondecode(aws_ecs_task_definition.task["server"].container_definitions)[0].secrets : s.valueFrom if s.name == "MY_DATABASE_URL"][0] == "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:pooled-AbCdEf" &&
      [for s in jsondecode(aws_ecs_task_definition.task["migrate"].container_definitions)[0].secrets : s.valueFrom if s.name == "MY_DATABASE_URL"][0] == "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:direct-AbCdEf" &&
      [for s in jsondecode(aws_ecs_task_definition.task["console"].container_definitions)[0].secrets : s.valueFrom if s.name == "MY_DATABASE_URL"][0] == "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:direct-AbCdEf"
    )
    error_message = "the migration and the console take the direct connection, and the service does not"
  }

  assert {
    condition     = output.one_off_secret_env["MY_KEK"] == output.secret_env["MY_KEK"]
    error_message = "the one-off tasks keep every other secret"
  }
}

run "the_roles_are_two" {
  command = plan

  assert {
    condition = (
      contains(flatten(data.aws_iam_policy_document.execution.statement[*].actions), "secretsmanager:GetSecretValue") &&
      !contains(flatten(data.aws_iam_policy_document.task.statement[*].actions), "secretsmanager:GetSecretValue")
    )
    error_message = "the role that starts a task reads the secrets, and the server's own role does not"
  }

  assert {
    condition     = contains(flatten(data.aws_iam_policy_document.task.statement[*].actions), "s3:GetObject") && length(flatten(data.aws_iam_policy_document.task.statement[*].condition)) == 1
    error_message = "the server's role holds what the backends ask, with their conditions"
  }
}

run "no_permission_makes_no_policy" {
  command = plan

  variables {
    backends = [
      { env = { MY_DATABASE = "postgres", MY_BLOB = "s3" } },
      { secret_env = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" } },
    ]
  }

  assert {
    condition     = aws_iam_role_policy.task == null
    error_message = "IAM refuses a policy with no statement"
  }
}

run "a_secret_file_is_a_secret_env" {
  command = plan

  variables {
    secret_files = { MY_VAPID_PRIVATE_KEY_FILE = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:hook-AbCdEf" }
  }

  assert {
    condition     = output.secret_env["MY_VAPID_PRIVATE_KEY"] == "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:hook-AbCdEf"
    error_message = "Fargate mounts nothing, so a file is the same as a variable"
  }
}

run "the_console_names_its_platform" {
  command = apply

  variables {
    console_enabled = true
  }

  assert {
    condition     = output.console.platform == "aws-ecs" && output.console.region == "ap-northeast-1" && output.console.task_definition == "my-console" && output.console.service == "my" && output.console.log_prefix == "console"
    error_message = "console.sh chooses by platform, and needs where to start the task"
  }

  assert {
    condition     = jsondecode(aws_ecs_task_definition.task["console"].container_definitions)[0].command == ["version"] && jsondecode(aws_ecs_task_definition.task["console"].container_definitions)[0].logConfiguration.options.awslogs-stream-prefix == "console"
    error_message = "a one-off task started by hand must do nothing, and its logs must be where console.sh reads them"
  }

  assert {
    condition     = output.migration_task_definition == "my-migrate" && output.service_name == "my"
    error_message = "the families are the names upgrade.sh takes"
  }
}

run "an_ingress_attaches_the_service" {
  command = plan

  variables {
    backends = [
      { env = { MY_DATABASE = "postgres", MY_BLOB = "s3" } },
      { secret_env = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" } },
      {
        env             = { MY_TRUSTED_PROXY_HEADER = "x-forwarded-for" }
        service_ingress = "load-balancer"
        trusted_proxies = ["10.0.0.0/24", "10.0.1.0/24"]
        load_balancer   = { target_group_arn = "arn:aws:elasticloadbalancing:ap-northeast-1:111122223333:targetgroup/my/abc", security_group_id = "sg-lb" }
      },
    ]
  }

  assert {
    condition     = output.env["MY_TRUSTED_PROXIES"] == "10.0.0.0/24, 10.0.1.0/24"
    error_message = "the service trusts what the ingress says, with nothing of the platform to add"
  }

  assert {
    condition     = one(aws_ecs_service.server.load_balancer).container_port == 8080 && one(aws_ecs_service.server.load_balancer).container_name == "my" && aws_ecs_service.server.health_check_grace_period_seconds == 120
    error_message = "the service must attach to the target group"
  }

  assert {
    condition     = aws_vpc_security_group_ingress_rule.from_load_balancer.referenced_security_group_id == "sg-lb" && aws_vpc_security_group_ingress_rule.from_load_balancer.from_port == 8080
    error_message = "the tasks must admit the load balancer, and only by its group"
  }
}

run "a_setting_named_twice_fails" {
  command = plan

  variables {
    env = { MY_DATABASE = "sqlite" }
  }

  expect_failures = [aws_iam_role.execution]
}

run "a_setting_and_its_file_form_fail" {
  command = plan

  variables {
    secret_files = { MY_KEK_FILE = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" }
  }

  expect_failures = [aws_iam_role.execution]
}

run "trust_in_env_beside_an_ingress_fails" {
  command = plan

  variables {
    env = { MY_TRUSTED_PROXIES = "10.0.0.0/8" }
    backends = [
      { env = { MY_DATABASE = "postgres", MY_BLOB = "s3" } },
      { secret_env = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" } },
      { service_ingress = "load-balancer", trusted_proxies = ["10.0.0.0/24"] },
    ]
  }

  expect_failures = [aws_iam_role.execution]
}

run "no_kek_fails" {
  command = plan

  variables {
    backends = [{ env = { MY_DATABASE = "postgres", MY_BLOB = "s3" } }]
  }

  expect_failures = [aws_iam_role.execution]
}

run "a_secret_named_other_than_by_arn_fails" {
  command = plan

  variables {
    backends = [
      { env = { MY_DATABASE = "postgres", MY_BLOB = "s3" } },
      { secret_env = { MY_KEK = "my-kek" } },
    ]
  }

  expect_failures = [aws_iam_role.execution]
}

run "a_direct_secret_named_other_than_by_arn_fails" {
  command = plan

  variables {
    backends = [
      { env = { MY_DATABASE = "postgres", MY_BLOB = "s3" }, direct_secret_env = { MY_DATABASE_URL = "direct" } },
      { secret_env = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" } },
    ]
  }

  expect_failures = [aws_iam_role.execution]
}

run "a_queue_that_needs_a_caller_fails" {
  command = plan

  variables {
    backends = [
      { env = { MY_DATABASE = "postgres", MY_BLOB = "s3", MY_QUEUE = "sqs" } },
      { secret_env = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" } },
    ]
  }

  expect_failures = [aws_iam_role.execution]
}

run "two_tasks_on_the_stream_of_one_process_fail" {
  command = plan

  variables {
    desired_count = 2
  }

  expect_failures = [aws_iam_role.execution]
}

run "two_tasks_with_a_broker_pass" {
  command = plan

  variables {
    desired_count = 2
    env           = { MY_STREAM = "postgres" }
  }

  assert {
    condition     = aws_ecs_service.server.desired_count == 2
    error_message = "a broker reaches every task"
  }
}

run "data_on_the_disk_of_a_task_fails" {
  command = plan

  variables {
    backends = [{ secret_env = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" } }]
  }

  expect_failures = [aws_iam_role.execution]
}

run "two_load_balancers_fail" {
  command = plan

  variables {
    backends = [
      { env = { MY_DATABASE = "postgres", MY_BLOB = "s3" } },
      { secret_env = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" } },
      { load_balancer = { target_group_arn = "a", security_group_id = "b" } },
      { load_balancer = { target_group_arn = "c", security_group_id = "d" } },
    ]
  }

  expect_failures = [aws_iam_role.execution]
}

run "a_backend_for_google_cloud_fails" {
  command = plan

  variables {
    backends = [{ permissions = [{ permission = "roles/storage.objectUser", scope = "bucket", resource = "b" }] }]
  }

  expect_failures = [var.backends]
}

run "an_ingress_that_needs_a_host_fails" {
  command = plan

  variables {
    backends = [{ service_ingress = "all" }]
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

run "a_queue_that_lambda_polls_fails" {
  command = plan

  variables {
    backends = [{ event_sources = { jobs = { arn = "arn:aws:sqs:ap-northeast-1:111122223333:my-jobs", visibility_timeout_seconds = 5400 } } }]
  }

  expect_failures = [var.backends]
}
