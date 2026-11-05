# SPDX-License-Identifier: AGPL-3.0-only

# Each managed database in front of each app it serves, for modules/db-neon/tests/compose.tftest.hcl. It shows that the blocks
# fit together with no cycle: db-gcp-cloud-sql and db-aws-rds alone, and db-neon with the secrets module that writes its URLs.

terraform {
  required_version = ">= 1.11"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.7"
    }
    neon = {
      source  = "kislerdm/neon"
      version = "= 0.18.0"
    }
  }
}

module "cloud_sql" {
  source = "../../modules/db-gcp-cloud-sql"

  project_id = "p"
  region     = "asia-northeast1"
}

module "app_on_cloud_sql" {
  source = "../../modules/app-gcp-cloud-run"

  project_id      = "p"
  region          = "asia-northeast1"
  name            = "cloud-sql"
  image           = "ghcr.io/mycroblog/my:0.1.0"
  host            = "social.example.org"
  service_ingress = "all"
  secret_files    = { MY_KEK_FILE = "kek" }
  backends        = [module.cloud_sql]
}

module "neon_on_gcp" {
  source = "../../modules/db-neon"

  org_id               = "org-1"
  region_id            = "aws-ap-southeast-1"
  url_secret_id        = module.secrets_on_gcp.ids["database_url"]
  pooled_url_secret_id = module.secrets_on_gcp.ids["database_url_pooled"]
}

module "secrets_on_gcp" {
  source = "../../modules/secrets-gcp-secret-manager"

  project_id = "p"
  referenced = { database_url = "my-database-url", database_url_pooled = "my-database-url-pooled" }
  write_keys = ["database_url", "database_url_pooled"]
  write_values = {
    database_url        = module.neon_on_gcp.connection_uri
    database_url_pooled = module.neon_on_gcp.connection_uri_pooler
  }
}

module "app_on_neon_gcp" {
  source = "../../modules/app-gcp-cloud-run"

  project_id      = "p"
  region          = "asia-northeast1"
  name            = "neon"
  image           = "ghcr.io/mycroblog/my:0.1.0"
  host            = "social.example.org"
  service_ingress = "all"
  secret_files    = { MY_KEK_FILE = "kek" }
  backends        = [module.secrets_on_gcp, module.neon_on_gcp]
}

module "rds" {
  source = "../../modules/db-aws-rds"

  vpc_id     = "vpc-1"
  subnet_ids = ["subnet-a", "subnet-b"]
}

module "app_on_rds" {
  source = "../../modules/app-aws-lambda"

  name                   = "rds"
  image                  = "111122223333.dkr.ecr.ap-northeast-1.amazonaws.com/my:0.1.0"
  host                   = "social.example.org"
  secret_env             = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek" }
  vpc_subnet_ids         = ["subnet-c", "subnet-d"]
  vpc_security_group_ids = ["sg-out"]
  backends               = [module.rds]
}

module "neon_on_aws" {
  source = "../../modules/db-neon"

  org_id        = "org-1"
  region_id     = "aws-ap-southeast-1"
  url_secret_id = module.secrets_on_aws.ids["database_url"]
}

module "secrets_on_aws" {
  source = "../../modules/secrets-aws-secrets-manager"

  referenced = { database_url = "my-database-url" }
  write_keys = ["database_url"]
  write_values = {
    database_url = module.neon_on_aws.connection_uri
  }
}

module "app_on_neon_aws" {
  source = "../../modules/app-aws-lambda"

  name       = "neon"
  image      = "111122223333.dkr.ecr.ap-northeast-1.amazonaws.com/my:0.1.0"
  host       = "social.example.org"
  secret_env = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek" }
  backends   = [module.secrets_on_aws, module.neon_on_aws]
}

output "neon_gcp_service_url" {
  value = module.app_on_neon_gcp.secret_env["MY_DATABASE_URL"]
}

output "neon_gcp_job_url" {
  value = module.app_on_neon_gcp.one_off_secret_env["MY_DATABASE_URL"]
}
