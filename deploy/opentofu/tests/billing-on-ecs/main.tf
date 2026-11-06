# SPDX-License-Identifier: AGPL-3.0-only

# billing-aws beside the app of AWS, for the tests of billing-aws.

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.0, < 7.0"
    }
  }
}

# Needs aws in name only: on this app it creates nothing, and returns the permission.
module "billing_aws" {
  source = "../../modules/billing-aws"
}

module "app" {
  source = "../../modules/app-aws-ecs"

  image = "ghcr.io/mycroblog/my:0.1.0"
  host  = "social.example.org"
  network = {
    platform            = "aws"
    vpc_id              = "vpc-1"
    vpc_cidr            = "10.0.0.0/16"
    public_subnet_ids   = ["subnet-1", "subnet-2"]
    public_subnet_cidrs = ["10.0.0.0/24", "10.0.1.0/24"]
    private_subnet_ids  = ["subnet-a", "subnet-b"]
    nat                 = true
  }
  assign_public_ip = true
  backends = [
    { platform = "aws", env = { MY_DATABASE = "postgres", MY_BLOB = "s3" }, secret_env = { MY_DATABASE_URL = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:db-AbCdEf" } },
    { platform = "aws", secret_env = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" } },
    module.billing_aws,
  ]
}

output "env" {
  value = module.app.env
}

output "policy_actions" {
  value = module.billing_aws.permissions
}
