# SPDX-License-Identifier: AGPL-3.0-only

# An app with a worker and the migration of a new image, for modules/internal/app-aws-ecs/tests/upgrade.tftest.hcl. The
# migration is migrate-stub.sh, which reaches nothing, so that a test can apply one version and then another on mocked
# providers. previous_migration takes the output migration_id of an earlier run, which a test compares with this one's.

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}

variable "version_applied" {
  type = string
}

variable "previous_migration" {
  type    = string
  default = null
}

locals {
  workers = { delivery = { job_types = ["federation.fanout", "federation.deliver"] } }
}

module "queue" {
  source = "../../modules/internal/queue-db"

  workers = local.workers
}

module "app" {
  source = "../../modules/internal/app-aws-ecs"

  image = "ghcr.io/mycroblog/my:${var.version_applied}"
  host  = "social.example.org"
  network = {
    platform            = "aws"
    vpc_id              = "vpc-1"
    vpc_cidr            = "10.0.0.0/16"
    public_subnet_ids   = ["subnet-1", "subnet-2"]
    public_subnet_cidrs = ["10.0.0.0/24", "10.0.1.0/24"]
  }
  assign_public_ip = true
  env              = { MY_STREAM = "postgres", MY_DATABASE = "postgres", MY_BLOB = "s3" }
  secret_env       = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek" }
  workers          = local.workers
  backends         = [module.queue]

  migration = {
    script  = abspath("${path.module}/migrate-stub.sh")
    version = var.version_applied
  }
}

output "migration_id" {
  value = module.app.migration_id
}

output "images" {
  value = module.app.images
}
