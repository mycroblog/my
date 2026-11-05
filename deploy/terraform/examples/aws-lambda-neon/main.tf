# SPDX-License-Identifier: AGPL-3.0-only

# One deployment on AWS: Lambda, SQS, S3, and SNS, with a database created by hand on Neon and email through an SMTP service.
# NOTHING HERE HAS BEEN APPLIED, AND IT CANNOT RUN YET: deploy/terraform/modules/app-aws-lambda/README.md says what is missing.
# It is here to show that the blocks swap, as in the example for Google Cloud.

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
    # Used only by the Cloudflare module in the comment below; it then reads CLOUDFLARE_API_TOKEN.
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.region
}

# The first apply generates the VAPID key. Add the others with aws secretsmanager put-secret-value before the app.
module "secrets" {
  source = "git::https://github.com/mycroblog/my.git//deploy/terraform/modules/secrets-aws-secrets-manager?ref=v0.1.0&depth=1"

  secrets            = { MY_VAPID_PRIVATE_KEY = var.vapid_secret_name }
  generate_p256_keys = ["MY_VAPID_PRIVATE_KEY"]
  referenced = {
    database_url  = var.database_url_secret_name
    smtp_password = var.mail_smtp_password_secret_name
  }
}

# To keep the KEK in AWS KMS instead, replace this block:
#
#   module "kek" {
#     source = "git::https://github.com/mycroblog/my.git//deploy/terraform/modules/kek-aws-kms?ref=v0.1.0&depth=1"
#   }
module "kek" {
  source = "git::https://github.com/mycroblog/my.git//deploy/terraform/modules/kek-aws-secrets-manager?ref=v0.1.0&depth=1"

  secret_name = var.kek_secret_name
}

# The same block as on Google Cloud, with the ARN of a secret in the place of its ID.
module "db" {
  source = "git::https://github.com/mycroblog/my.git//deploy/terraform/modules/db-external?ref=v0.1.0&depth=1"

  url_secret_id = module.secrets.ids["database_url"]
}

module "queue" {
  source = "git::https://github.com/mycroblog/my.git//deploy/terraform/modules/queue-aws-sqs?ref=v0.1.0&depth=1"

  tick_schedule = var.tick_schedule
}

module "blob" {
  source = "git::https://github.com/mycroblog/my.git//deploy/terraform/modules/blob-aws-s3?ref=v0.1.0&depth=1"

  bucket_name = var.bucket_name
}

module "stream" {
  source = "git::https://github.com/mycroblog/my.git//deploy/terraform/modules/stream-aws-sns?ref=v0.1.0&depth=1"
}

# To send through Amazon SES instead, replace this block, and the secret above is not needed:
#
#   module "mail" {
#     source = "git::https://github.com/mycroblog/my.git//deploy/terraform/modules/mail-aws-ses?ref=v0.1.0&depth=1"
#
#     from = var.mail_from
#   }
#
# The same block as on Google Cloud, with the ARN of a secret in the place of its ID.
module "mail" {
  source = "git::https://github.com/mycroblog/my.git//deploy/terraform/modules/mail-smtp?ref=v0.1.0&depth=1"

  from               = var.mail_from
  host               = var.mail_smtp_host
  port               = var.mail_smtp_port
  username           = var.mail_smtp_username
  password_secret_id = module.secrets.ids["smtp_password"]
}

# A Function URL takes no custom domain, so something in front must answer at https://{host}. Or Cloudflare, the same block as
# on Google Cloud, which needs its Enterprise plan to send the origin's Host:
#
#   module "ingress" {
#     source = "git::https://github.com/mycroblog/my.git//deploy/terraform/modules/ingress-cloudflare-app?ref=v0.1.0&depth=1"
#
#     zone_id             = "<the zone's ID>"
#     host                = var.host
#     origin_host         = module.app.origin_host
#     rewrite_host_header = true
#   }
module "ingress" {
  source = "git::https://github.com/mycroblog/my.git//deploy/terraform/modules/ingress-aws-cloudfront?ref=v0.1.0&depth=1"

  host            = var.host
  origin_host     = module.app.origin_host
  certificate_arn = var.certificate_arn
}

module "app" {
  source = "git::https://github.com/mycroblog/my.git//deploy/terraform/modules/app-aws-lambda?ref=v0.1.0&depth=1"

  image           = var.image
  architecture    = var.architecture
  host            = var.host
  console_enabled = var.console_enabled

  backends = [
    module.secrets, module.kek, module.db, module.queue, module.blob, module.stream, module.mail,
    # Named output by output, as on Google Cloud: the ingress reads the app's origin, so its other outputs depend on the app.
    { env = module.ingress.env, service_ingress = module.ingress.service_ingress, trusted_proxies = module.ingress.trusted_proxies },
  ]

  env = merge(
    # A request to Lambda is at most 6 MB with a binary body in base64, about 4.5 MB of file. var.env can set other values.
    { MY_MEDIA_MAX_IMAGE_BYTES = "4194304", MY_MEDIA_MAX_AV_BYTES = "4194304" },
    var.env,
  )
  secret_env = var.secret_env
}
