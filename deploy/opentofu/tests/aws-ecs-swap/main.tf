# SPDX-License-Identifier: AGPL-3.0-only

# The blocks of examples/aws-ecs-neon with the alternatives its comments name, for the example's tests. It shows that a swap changes
# one block and nothing else.

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

variable "kek" {
  type    = string
  default = "secrets-manager"
}

variable "mail" {
  type    = string
  default = "smtp"
}

module "secrets" {
  source = "../../modules/secrets-aws-secrets-manager"

  referenced = { database_url = "my-database-url", smtp_password = "my-smtp-password" }
}

module "kek_secrets_manager" {
  source = "../../modules/kek-aws-secrets-manager"

  lifecycle {
    enabled = var.kek == "secrets-manager"
  }
}

module "kek_kms" {
  source = "../../modules/kek-aws-kms"

  lifecycle {
    enabled = var.kek == "kms"
  }
}

module "db" {
  source = "../../modules/db-external"

  url_secret_id = module.secrets.ids["database_url"]
}

module "queue" {
  source = "../../modules/queue-db"
}

module "blob" {
  source = "../../modules/blob-aws-s3"

  bucket_name = "b"
}

module "stream" {
  source = "../../modules/stream-aws-sns"
}

module "mail_smtp" {
  source = "../../modules/mail-smtp"

  from               = "noreply@social.example.org"
  host               = "smtp.example.org"
  username           = "u"
  password_secret_id = module.secrets.ids["smtp_password"]

  lifecycle {
    enabled = var.mail == "smtp"
  }
}

module "mail_ses" {
  source = "../../modules/mail-aws-ses"

  from = "noreply@social.example.org"

  lifecycle {
    enabled = var.mail == "ses"
  }
}

locals {
  network = {
    platform            = "aws"
    vpc_id              = "vpc-1"
    vpc_cidr            = "10.0.0.0/16"
    public_subnet_ids   = ["subnet-1", "subnet-2"]
    public_subnet_cidrs = ["10.0.0.0/24", "10.0.1.0/24"]
    private_subnet_ids  = ["subnet-3", "subnet-4"]
  }
}

module "ingress" {
  source = "../../modules/ingress-aws-alb"

  host            = "social.example.org"
  network         = local.network
  certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc"
}

module "app" {
  source = "../../modules/app-aws-ecs"

  image            = "ghcr.io/mycroblog/my:0.1.0"
  host             = "social.example.org"
  network          = local.network
  assign_public_ip = true

  backends = concat(
    [module.secrets, module.db, module.queue, module.blob, module.stream, module.ingress],
    var.kek == "secrets-manager" ? [module.kek_secrets_manager] : [],
    var.kek == "kms" ? [module.kek_kms] : [],
    var.mail == "smtp" ? [module.mail_smtp] : [],
    var.mail == "ses" ? [module.mail_ses] : [],
  )
}

output "env" {
  value = module.app.env
}

output "secret_env" {
  value = module.app.secret_env
}
