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
  source = "../../modules/app-aws-lambda"

  image = "111122223333.dkr.ecr.ap-northeast-1.amazonaws.com/my:0.1.0"
  host  = "social.example.org"
  env   = { MY_QUEUE = "sqs" }
  backends = [
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
