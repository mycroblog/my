# SPDX-License-Identifier: AGPL-3.0-only

# One deployment on AWS: ECS with Fargate behind an Application Load Balancer, the jobs in the database, S3, and SNS, with a
# database created by hand on Neon and email through an SMTP service, on a VPC of its own. It runs the released image as it is.
# NOTHING HERE HAS BEEN APPLIED: deploy/opentofu/modules/app-aws-ecs/README.md says what stays unverified and what it costs.

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
  region = var.region
}

# With public addresses on the tasks there is no NAT gateway to pay for, and without them there must be one, or federation stops.
#
# For a VPC that exists already, remove this block and pass network = var.network to the blocks below, an object of the shape in
# deploy/opentofu/README.md: platform, vpc_id, vpc_cidr, public_subnet_ids, public_subnet_cidrs, private_subnet_ids, and nat.
module "network" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/network-aws-vpc?ref=v0.1.0&depth=1"

  availability_zones = var.availability_zones
  nat                = !var.assign_public_ip
}

# The first apply generates the VAPID key. Add the others with aws secretsmanager put-secret-value before the app.
module "secrets" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/secrets-aws-secrets-manager?ref=v0.1.0&depth=1"

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
#     source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/kek-aws-kms?ref=v0.1.0&depth=1"
#   }
module "kek" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/kek-aws-secrets-manager?ref=v0.1.0&depth=1"

  secret_name = var.kek_secret_name
}

# The same block as on Google Cloud, with the ARN of a secret in the place of its ID.
module "db" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/db-external?ref=v0.1.0&depth=1"

  url_secret_id = module.secrets.ids["database_url"]
}

# The process stays up, so it polls the queue and runs the periodic jobs itself. The database never gets to rest: read the README
# of this module before you choose a Neon plan.
module "queue" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/queue-db?ref=v0.1.0&depth=1"
}

module "blob" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/blob-aws-s3?ref=v0.1.0&depth=1"

  bucket_name = var.bucket_name
}

# Above one task, and for the moments of an upgrade when two run, streams need a broker that reaches every task.
module "stream" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/stream-aws-sns?ref=v0.1.0&depth=1"
}

# To send through Amazon SES instead, replace this block, and the secret above is not needed:
#
#   module "mail" {
#     source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/mail-aws-ses?ref=v0.1.0&depth=1"
#
#     from = var.mail_from
#   }
#
# The same block as on Google Cloud, with the ARN of a secret in the place of its ID.
module "mail" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/mail-smtp?ref=v0.1.0&depth=1"

  from               = var.mail_from
  host               = var.mail_smtp_host
  port               = var.mail_smtp_port
  username           = var.mail_smtp_username
  password_secret_id = module.secrets.ids["smtp_password"]
}

# The only way in: the tasks admit nothing else, so no origin secret is involved.
module "ingress" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/ingress-aws-alb?ref=v0.1.0&depth=1"

  host            = var.host
  network         = module.network
  certificate_arn = var.certificate_arn
  # With a hosted zone in Route 53 the module creates the records, at an apex too. Without one, dns_records says what to create.
  route53_zone_id = var.route53_zone_id
  host_is_apex    = var.host_is_apex
}

# To give a kind of job its own service, set the same value on the queue above and on this block, which derive every name from the
# key. The default takes whatever no worker names. Read "Workers" in deploy/opentofu/README.md.
#
#   workers = {
#     delivery = {
#       job_types   = ["federation.fanout", "federation.deliver"]
#       instances   = 2
#       concurrency = 3
#     }
#   }
module "app" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/app-aws-ecs?ref=v0.1.0&depth=1"

  image            = var.image
  architecture     = var.architecture
  host             = var.host
  network          = module.network
  assign_public_ip = var.assign_public_ip
  cpu              = var.cpu
  memory_mib       = var.memory_mib
  desired_count    = var.desired_count
  console_enabled  = var.console_enabled

  # The whole of the ingress, unlike on Lambda: it reads nothing of the app, so there is no cycle.
  backends = [module.secrets, module.kek, module.db, module.queue, module.blob, module.stream, module.mail, module.ingress]

  env        = var.env
  secret_env = var.secret_env
}
