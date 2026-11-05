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

  referenced = { database_url = "my-database-url", smtp_password = "my-smtp-password" }
}

module "kek_secrets_manager" {
  count  = var.kek == "secrets-manager" ? 1 : 0
  source = "../../modules/kek-aws-secrets-manager"
}

module "kek_kms" {
  count  = var.kek == "kms" ? 1 : 0
  source = "../../modules/kek-aws-kms"
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
  count  = var.mail == "smtp" ? 1 : 0
  source = "../../modules/mail-smtp"

  from               = "noreply@social.example.org"
  host               = "smtp.example.org"
  username           = "u"
  password_secret_id = module.secrets.ids["smtp_password"]
}

module "mail_ses" {
  count  = var.mail == "ses" ? 1 : 0
  source = "../../modules/mail-aws-ses"

  from = "noreply@social.example.org"
}

module "ingress_cloudfront" {
  count  = var.ingress == "cloudfront" ? 1 : 0
  source = "../../modules/ingress-aws-cloudfront"

  host            = "social.example.org"
  origin_host     = module.app.origin_host
  certificate_arn = "arn:aws:acm:us-east-1:111122223333:certificate/abc"
}

module "ingress_cloudflare" {
  count  = var.ingress == "cloudflare" ? 1 : 0
  source = "../../modules/ingress-cloudflare-app"

  zone_id             = "023e105f4ecef8ad9ca31a8372d0c353"
  host                = "social.example.org"
  origin_host         = module.app.origin_host
  rewrite_host_header = true
}

module "app" {
  source = "../../modules/app-aws-lambda"

  image = "111122223333.dkr.ecr.ap-northeast-1.amazonaws.com/my:0.1.0"
  host  = "social.example.org"

  backends = concat(
    [module.secrets, module.db, module.queue, module.blob, module.stream],
    module.kek_secrets_manager,
    module.kek_kms,
    module.mail_smtp,
    module.mail_ses,
    # Named output by output, as in the example: a reference to the whole module would be a cycle.
    var.ingress == "cloudfront" ? [{ env = module.ingress_cloudfront[0].env, service_ingress = module.ingress_cloudfront[0].service_ingress, trusted_proxies = module.ingress_cloudfront[0].trusted_proxies }] : [],
    var.ingress == "cloudflare" ? [{ env = module.ingress_cloudflare[0].env, service_ingress = module.ingress_cloudflare[0].service_ingress, trusted_proxies = module.ingress_cloudflare[0].trusted_proxies }] : [],
  )
}

output "env" {
  value = module.app.env
}

output "secret_env" {
  value = module.app.secret_env
}
