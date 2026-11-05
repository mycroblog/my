# SPDX-License-Identifier: AGPL-3.0-only

# Both billing modules beside the app of AWS, for the tests of billing-aws. Each block says which provider it needs.

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.0, < 7.0"
    }
    google = {
      source  = "hashicorp/google"
      version = ">= 7.0, < 8.0"
    }
  }
}

# Needs aws in name only: on this app it creates nothing, and returns the permission.
module "billing_aws" {
  source = "../../modules/billing-aws"
}

# Needs google. The app of AWS refuses a Google Cloud role, so this module grants the roles itself.
module "billing_gcp" {
  source = "../../modules/billing-gcp"

  project_id         = "p"
  billing_account_id = "012345-6789AB-CDEF01"
  external_member    = "principalSet://iam.googleapis.com/projects/1/locations/global/workloadIdentityPools/my/attribute.aws_role/my-server"
}

# Needs aws.
module "app" {
  source = "../../modules/app-aws-lambda"

  image = "111122223333.dkr.ecr.ap-northeast-1.amazonaws.com/my:0.1.0"
  host  = "social.example.org"
  backends = [
    { secret_env = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek-AbCdEf" } },
    module.billing_aws,
    module.billing_gcp,
  ]
}

output "env" {
  value = module.app.env
}

output "policy_actions" {
  value = module.billing_aws.permissions
}
