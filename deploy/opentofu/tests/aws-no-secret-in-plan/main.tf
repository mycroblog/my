# SPDX-License-Identifier: AGPL-3.0-only

# The modules that generate a value, for scripts/tofu-aws-plan-test.sh. The provider is given values that are no
# credentials and told not to check them: a plan of resources to create calls nothing.

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

provider "aws" {
  region                      = "ap-northeast-1"
  access_key                  = "not-a-credential"
  secret_key                  = "not-a-credential"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_region_validation      = true
  skip_requesting_account_id  = true
}

module "kek" {
  source = "../../modules/kek-aws-secrets-manager"
}

module "secrets" {
  source = "../../modules/secrets-aws-secrets-manager"

  secrets            = { MY_VAPID_PRIVATE_KEY = "my-vapid-key" }
  generate_p256_keys = ["MY_VAPID_PRIVATE_KEY"]
}
