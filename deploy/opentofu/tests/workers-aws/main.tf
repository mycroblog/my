# SPDX-License-Identifier: AGPL-3.0-only

# One value of workers given to a queue and an app, for modules/workers/tests/compose.tftest.hcl: on ECS with the queue in the
# database, and on Lambda with SQS.

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}

locals {
  network = {
    platform            = "aws"
    vpc_id              = "vpc-1"
    vpc_cidr            = "10.0.0.0/16"
    public_subnet_ids   = ["subnet-1", "subnet-2"]
    public_subnet_cidrs = ["10.0.0.0/24", "10.0.1.0/24"]
  }
  kek = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek" }

  workers = {
    delivery = { job_types = ["federation.fanout", "federation.deliver"], concurrency = 3 }
  }
}

module "queue" {
  source = "../../modules/queue-db"

  workers = local.workers
}

module "ecs" {
  source = "../../modules/app-aws-ecs"

  image            = "ghcr.io/mycroblog/my:0.1.0"
  host             = "social.example.org"
  network          = local.network
  assign_public_ip = true
  env              = { MY_STREAM = "postgres", MY_DATABASE = "postgres", MY_BLOB = "s3" }
  secret_env       = local.kek
  workers          = local.workers
  backends         = [module.queue]
}

module "sqs" {
  source = "../../modules/queue-aws-sqs"

  workers = local.workers
}

module "lambda" {
  source = "../../modules/app-aws-lambda"

  name       = "fn"
  image      = "111122223333.dkr.ecr.ap-northeast-1.amazonaws.com/my:0.1.0"
  host       = "social.example.org"
  secret_env = local.kek
  workers    = local.workers
  backends   = [module.sqs]
}

output "ecs_worker_env" {
  value = module.ecs.worker_env["delivery"]
}

output "ecs_env" {
  value = module.ecs.env
}

output "lambda_worker" {
  value = module.lambda.worker_functions["delivery"]
}
