# SPDX-License-Identifier: AGPL-3.0-only

# The modules that take a network on AWS, sharing one, for modules/network-aws-vpc/tests/compose.tftest.hcl: first from
# network-aws-vpc, then from an object of the same shape for a VPC that exists already, with no network module at all.

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.7"
    }
  }
}

locals {
  kek = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek" }

  existing_vpc = {
    platform            = "aws"
    vpc_id              = "vpc-mine"
    vpc_cidr            = "172.16.0.0/16"
    public_subnet_ids   = ["subnet-pub-1", "subnet-pub-2"]
    public_subnet_cidrs = ["172.16.0.0/24", "172.16.1.0/24"]
    private_subnet_ids  = ["subnet-priv-1", "subnet-priv-2"]
    nat                 = true
  }
}

module "network" {
  source = "../../modules/network-aws-vpc"

  availability_zones = ["ap-northeast-1a", "ap-northeast-1c"]
  nat                = true
}

module "db" {
  source = "../../modules/db-aws-rds"

  network = module.network
}

module "kv" {
  source = "../../modules/kv-aws-elasticache"

  network = module.network
}

module "ingress" {
  source = "../../modules/ingress-aws-alb"

  host            = "social.example.org"
  certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc"
  network         = module.network
}

module "ecs" {
  source = "../../modules/app-aws-ecs"

  name             = "ecs"
  image            = "ghcr.io/mycroblog/my:0.1.0"
  host             = "social.example.org"
  network          = module.network
  assign_public_ip = false
  env              = { MY_BLOB = "s3" }
  secret_env       = local.kek
  backends         = [module.db, module.kv, module.ingress]
}

module "lambda" {
  source = "../../modules/app-aws-lambda"

  name       = "fn"
  image      = "111122223333.dkr.ecr.ap-northeast-1.amazonaws.com/my:0.1.0"
  host       = "social.example.org"
  network    = module.network
  env        = { MY_QUEUE = "sqs" }
  secret_env = local.kek
  backends   = [module.db, module.kv]
}

module "existing_db" {
  source = "../../modules/db-aws-rds"

  name            = "mine"
  url_secret_name = "mine-database-url"
  network         = local.existing_vpc
}

module "existing_kv" {
  source = "../../modules/kv-aws-elasticache"

  name    = "mine-valkey"
  network = local.existing_vpc
}

module "existing_ingress" {
  source = "../../modules/ingress-aws-alb"

  name            = "mine"
  host            = "social.example.org"
  certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc"
  network         = local.existing_vpc
}

module "existing_ecs" {
  source = "../../modules/app-aws-ecs"

  name             = "mine"
  image            = "ghcr.io/mycroblog/my:0.1.0"
  host             = "social.example.org"
  network          = local.existing_vpc
  assign_public_ip = true
  env              = { MY_BLOB = "s3" }
  secret_env       = local.kek
  backends         = [module.existing_db, module.existing_kv, module.existing_ingress]
}

module "existing_lambda" {
  source = "../../modules/app-aws-lambda"

  name       = "mine-fn"
  image      = "111122223333.dkr.ecr.ap-northeast-1.amazonaws.com/my:0.1.0"
  host       = "social.example.org"
  network    = local.existing_vpc
  env        = { MY_QUEUE = "sqs" }
  secret_env = local.kek
  backends   = [module.existing_db, module.existing_kv]
}

output "ecs_env" {
  value = module.ecs.env
}

output "existing_ecs_env" {
  value = module.existing_ecs.env
}

output "existing_db_private_network" {
  value = module.existing_db.private_network
}

output "existing_kv_private_network" {
  value = module.existing_kv.private_network
}

output "existing_ecs_subnets" {
  value = module.existing_ecs.task_subnet_ids
}

output "existing_lambda_subnets" {
  value = module.existing_lambda.vpc_subnet_ids
}
