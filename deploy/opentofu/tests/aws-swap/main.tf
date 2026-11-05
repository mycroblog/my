# SPDX-License-Identifier: AGPL-3.0-only

# The blocks of examples/aws-lambda-neon with the alternatives its comments name, for the example's tests. It shows that a
# swap changes one block and nothing else, and that db-external, mail-smtp, and ingress-cloudflare-app need no change on AWS.

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
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 5.0"
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

variable "ingress" {
  type    = string
  default = "cloudfront"
}

module "secrets" {
  source = "../../modules/secrets-aws-secrets-manager"

  referenced = { database_url = "my-database-url", smtp_password = "my-smtp-password", origin_secret = "my-origin-secret" }
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
  source = "../../modules/queue-aws-sqs"
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

module "ingress_cloudfront" {
  source = "../../modules/ingress-aws-cloudfront"

  host             = "social.example.org"
  origin_host      = module.app.origin_host
  certificate_arn  = "arn:aws:acm:us-east-1:111122223333:certificate/abc"
  origin_secret    = "0123456789abcdef0123456789abcdef"
  origin_secret_id = module.secrets.ids["origin_secret"]

  lifecycle {
    enabled = var.ingress == "cloudfront"
  }
}

module "ingress_cloudflare" {
  source = "../../modules/ingress-cloudflare-app"

  zone_id             = "023e105f4ecef8ad9ca31a8372d0c353"
  host                = "social.example.org"
  origin_host         = module.app.origin_host
  rewrite_host_header = true
  origin_secret       = "0123456789abcdef0123456789abcdef"
  origin_secret_id    = module.secrets.ids["origin_secret"]

  lifecycle {
    enabled = var.ingress == "cloudflare"
  }
}

module "app" {
  source = "../../modules/app-aws-lambda"

  image = "111122223333.dkr.ecr.ap-northeast-1.amazonaws.com/my:0.1.0"
  host  = "social.example.org"

  backends = concat(
    [module.secrets, module.db, module.queue, module.blob, module.stream],
    var.kek == "secrets-manager" ? [module.kek_secrets_manager] : [],
    var.kek == "kms" ? [module.kek_kms] : [],
    var.mail == "smtp" ? [module.mail_smtp] : [],
    var.mail == "ses" ? [module.mail_ses] : [],
    # Named output by output, as in the example: a reference to the whole module would be a cycle.
    var.ingress == "cloudfront" ? [{ env = module.ingress_cloudfront.env, secret_env = module.ingress_cloudfront.secret_env, service_ingress = module.ingress_cloudfront.service_ingress, trusted_proxies = module.ingress_cloudfront.trusted_proxies }] : [],
    var.ingress == "cloudflare" ? [{ env = module.ingress_cloudflare.env, secret_env = module.ingress_cloudflare.secret_env, service_ingress = module.ingress_cloudflare.service_ingress, trusted_proxies = module.ingress_cloudflare.trusted_proxies }] : [],
  )
}

output "env" {
  value = module.app.env
}

output "secret_env" {
  value = module.app.secret_env
}
