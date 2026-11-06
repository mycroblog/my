# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches AWS, so none of the expressions has been evaluated.

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
  mock_resource "aws_sns_topic" {
    defaults = {
      arn = "arn:aws:sns:ap-northeast-1:111122223333:my-alerts"
    }
  }
  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::111122223333:role/my-canary"
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
  mock_resource "aws_s3_bucket" {
    defaults = {
      arn    = "arn:aws:s3:::my-canary-abc"
      bucket = "my-canary-abc"
    }
  }
}

variables {
  host                           = "social.example"
  load_balancer_arn_suffix       = "app/my/50dc6c495c0c9188"
  notification_emails            = ["ops@social.example"]
  database_size_limit_bytes      = 8589934592
  media_size_limit_bytes         = 53687091200
  remote_media_cache_limit_bytes = 1288490188
}

run "every_alert_of_the_runbook_is_an_alarm_or_listed_as_not_expressible" {
  command = plan

  assert {
    condition = (
      toset(concat(output.alerts.alarms, output.alerts.not_expressible))
      == toset(flatten(regexall("(?m)- alert: (My[A-Za-z]+)$", file("${path.module}/../../../monitoring/prometheus/alerts.yml"))))
    )
    error_message = "the names must be those of deploy/monitoring/prometheus/alerts.yml, each of them once"
  }

  assert {
    condition = alltrue([
      for name in concat(output.alerts.alarms, output.alerts.not_expressible) :
      strcontains(file("${path.module}/README.md"), "`${name}`")
    ])
    error_message = "the README must list every alert"
  }

  assert {
    condition     = length(setintersection(toset(output.alerts.alarms), toset(output.alerts.not_expressible))) == 0
    error_message = "an alert is one or the other"
  }
}

run "the_server_writes_what_the_alarms_read" {
  command = plan

  assert {
    condition     = output.env["MY_METRICS_EXPORTER"] == "emf"
    error_message = "CloudWatch Logs takes the metrics from the lines of emf"
  }

  assert {
    condition = alltrue(flatten([
      for alarm in aws_cloudwatch_metric_alarm.alert : [
        for query in alarm.metric_query : [
          for metric in query.metric : alltrue([
            for dimension in keys(metric.dimensions) :
            metric.namespace != "my" || contains(split(",", output.env["MY_METRICS_EMF_DIMENSIONS"]), dimension)
          ])
        ]
      ]
    ]))
    error_message = "an alarm on a dimension the server does not write sees nothing"
  }

  assert {
    condition = alltrue([
      for alarm in aws_cloudwatch_metric_alarm.alert : length(alarm.metric_query) <= 11
    ])
    error_message = "an alarm reads at most 10 metrics, and its expression"
  }
}

run "the_canary_calls_readyz_at_the_input_period" {
  command = plan

  variables {
    uptime_period_minutes = 5
  }

  assert {
    condition     = aws_synthetics_canary.ready.schedule[0].expression == "rate(5 minutes)"
    error_message = "the period comes from the input"
  }

  assert {
    condition     = aws_synthetics_canary.ready.run_config[0].environment_variables["READY_URL"] == "https://social.example/readyz"
    error_message = "the canary calls /readyz on the host"
  }

  assert {
    condition     = one(flatten([for query in aws_cloudwatch_metric_alarm.alert["MyTargetDown"].metric_query : [for metric in query.metric : metric.period]])) == 300
    error_message = "the alarm's period is the canary's, or a sample without a run would count as a failure"
  }
}

run "one_minute_is_singular_and_takes_five_samples" {
  command = plan

  variables {
    uptime_period_minutes = 1
  }

  assert {
    condition     = aws_synthetics_canary.ready.schedule[0].expression == "rate(1 minute)"
    error_message = "a rate of 1 is singular"
  }

  assert {
    condition     = aws_cloudwatch_metric_alarm.alert["MyTargetDown"].evaluation_periods == 5
    error_message = "5 minutes, as in the runbook"
  }
}

run "a_period_synthetics_has_not_fails" {
  command = plan

  variables {
    uptime_period_minutes = 90
  }

  expect_failures = [var.uptime_period_minutes]
}

run "only_the_canary_and_the_snapshot_count_missing_data_as_a_failure" {
  command = plan

  assert {
    condition = toset([
      for name, alarm in aws_cloudwatch_metric_alarm.alert : name if alarm.treat_missing_data == "breaching"
    ]) == toset(["MyTargetDown", "MyMetricsSnapshotStale"])
    error_message = "a request or job metric has gaps on a function that scales to zero, so absence must not fire"
  }
}

run "the_topics_get_the_alarms_and_a_pager_gets_the_pages" {
  command = plan

  variables {
    notification_topic_arns      = ["arn:aws:sns:ap-northeast-1:111122223333:other"]
    page_notification_topic_arns = ["arn:aws:sns:ap-northeast-1:111122223333:pager"]
  }

  assert {
    condition     = contains(aws_cloudwatch_metric_alarm.alert["MyQueueStalled"].alarm_actions, "arn:aws:sns:ap-northeast-1:111122223333:pager")
    error_message = "a page goes to the pager"
  }

  assert {
    condition     = !contains(aws_cloudwatch_metric_alarm.alert["MyQueueBacklog"].alarm_actions, "arn:aws:sns:ap-northeast-1:111122223333:pager")
    error_message = "a ticket does not"
  }

  assert {
    condition     = contains(aws_cloudwatch_metric_alarm.alert["MyQueueBacklog"].alarm_actions, "arn:aws:sns:ap-northeast-1:111122223333:other")
    error_message = "every alarm goes to the topics"
  }
}

run "an_alarm_that_notifies_nobody_fails" {
  command = plan

  variables {
    notification_emails = []
  }

  expect_failures = [var.notification_emails]
}

run "an_unset_limit_and_a_disabled_alert_are_left_out" {
  command = plan

  variables {
    database_size_limit_bytes      = null
    media_size_limit_bytes         = null
    remote_media_cache_limit_bytes = null
    disabled_alerts                = ["MyAdapterErrors"]
  }

  assert {
    condition     = length(setintersection(toset(output.alerts.alarms), toset(["MyDatabaseStorageLarge", "MyMediaStorageLarge", "MyRemoteMediaCacheOverLimit", "MyAdapterErrors"]))) == 0
    error_message = "all four are left out"
  }
}

run "the_outputs_fit_the_app" {
  command = plan

  module {
    source = "../app-aws-ecs"
  }

  variables {
    image = "ghcr.io/mycroblog/my:0.1.0"
    network = {
      platform            = "aws"
      vpc_id              = "vpc-1"
      vpc_cidr            = "10.0.0.0/16"
      public_subnet_ids   = ["subnet-1", "subnet-2"]
      public_subnet_cidrs = ["10.0.0.0/24", "10.0.1.0/24"]
    }
    assign_public_ip = true
    backends = [
      # What this module returns: run blocks of a plan cannot pass outputs to another module.
      { platform = "aws", env = { MY_METRICS_EXPORTER = "emf", MY_METRICS_EMF_NAMESPACE = "my", MY_METRICS_EMF_DIMENSIONS = "my.outcome" }, secret_env = {}, permissions = [] },
      { platform = "aws", env = { MY_DATABASE = "postgres", MY_BLOB = "s3" }, secret_env = { MY_DATABASE_URL = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:db-AbCdEf", MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" } },
    ]
  }

  assert {
    condition     = output.env["MY_METRICS_EXPORTER"] == "emf"
    error_message = "the app takes the setting"
  }
}
