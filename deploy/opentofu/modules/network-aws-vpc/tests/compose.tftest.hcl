# SPDX-License-Identifier: AGPL-3.0-only

# Every module that takes a network sharing one, on mocked providers: nothing here reaches AWS. It shows that the blocks fit and
# that nothing is circular, not that they run. By plan alone: prevent_destroy refuses the destroy after an apply.

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
  mock_data "aws_region" {
    defaults = {
      region = "ap-northeast-1"
    }
  }
  mock_data "aws_iam_policy_document" {
    defaults = {
      json          = "{}"
      minified_json = "{}"
    }
  }
  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::111122223333:role/my-server"
    }
  }
  mock_resource "aws_cloudwatch_log_group" {
    defaults = {
      arn = "arn:aws:logs:ap-northeast-1:111122223333:log-group:/ecs/my"
    }
  }
  mock_resource "aws_lb" {
    defaults = {
      arn      = "arn:aws:elasticloadbalancing:ap-northeast-1:111122223333:loadbalancer/app/my/abc"
      dns_name = "my-123.ap-northeast-1.elb.amazonaws.com"
      zone_id  = "Z14GRHDCWA56QT"
    }
  }
  mock_resource "aws_lb_target_group" {
    defaults = {
      arn = "arn:aws:elasticloadbalancing:ap-northeast-1:111122223333:targetgroup/my/abc"
    }
  }
  mock_resource "aws_elasticache_serverless_cache" {
    defaults = {
      endpoint = [{ address = "my-valkey-abc.serverless.apne1.cache.amazonaws.com", port = 6379 }]
    }
  }
  mock_resource "aws_secretsmanager_secret" {
    defaults = {
      arn = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:s-AbCdEf"
    }
  }
}

mock_provider "random" {}

run "the_modules_share_the_network_of_the_module" {
  command = plan

  module {
    source = "../../tests/network-aws"
  }

  assert {
    condition     = output.ecs_env["MY_TRUSTED_PROXIES"] == "10.0.0.0/20, 10.0.16.0/20"
    error_message = "the ranges of the public subnets, known at plan time, are what the server trusts"
  }
}

run "the_modules_share_an_existing_vpc_by_the_same_input" {
  command = plan

  module {
    source = "../../tests/network-aws"
  }

  assert {
    condition     = output.existing_ecs_env["MY_TRUSTED_PROXIES"] == "172.16.0.0/24, 172.16.1.0/24"
    error_message = "the ranges of the object are what the server trusts"
  }

  assert {
    condition     = output.existing_db_private_network == "vpc-mine" && output.existing_kv_private_network == "vpc-mine"
    error_message = "the object of the shape must work with no network module"
  }

  assert {
    condition     = output.existing_ecs_subnets == tolist(["subnet-pub-1", "subnet-pub-2"])
    error_message = "the tasks must take the public subnets of the object with a public address"
  }

  assert {
    condition     = output.existing_lambda_subnets == tolist(["subnet-priv-1", "subnet-priv-2"])
    error_message = "the functions must take the private subnets of the object"
  }
}
