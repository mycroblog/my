# SPDX-License-Identifier: AGPL-3.0-only

# One deployment on AWS: ECS on Fargate behind an Application Load Balancer, the jobs in the database, S3, SNS, and email through
# Amazon SES, with a database created by hand on Neon, on a VPC of its own. Each section of mycroblog-aws picks another backend
# by its kind: deploy/opentofu/README.md lists them.
# NOTHING ON AWS HAS BEEN APPLIED: read "AWS" in deploy/opentofu/README.md for what stays unverified and what it costs.

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}

provider "aws" {
  region = var.region
}

module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-aws?ref=v0.1.0&depth=1"

  host  = var.host
  image = var.image
  # How the tasks reach the internet has no default: both ways bill by the hour.
  network = { egress = var.egress, availability_zones = var.availability_zones }
  ingress = { certificate_arn = var.certificate_arn, route53_zone_id = var.route53_zone_id }
  media   = { bucket_name = var.bucket_name }
  # SES sends only to verified addresses until AWS grants production access: read "Email through Amazon SES" in
  # deploy/opentofu/README.md.
  mail = { from = var.mail_from }
  # Who the alarms notify. null creates no canary and no alarms.
  monitoring = { notification_emails = var.alert_emails }

  console_enabled = var.console_enabled

  # The recommended setup is the default of every section. To put CloudFront in front of the load balancer, with /media/ cached
  # at its edges, with a certificate for host in us-east-1:
  #
  #   ingress = {
  #     kind                       = "cloudfront"
  #     certificate_arn            = var.certificate_arn
  #     cloudfront_certificate_arn = "arn:aws:acm:us-east-1:111122223333:certificate/00000000-0000-0000-0000-000000000000"
  #   }
  #
  # To give the delivery of federation its own service, which scales apart from the default service:
  #
  #   workers = { delivery = { job_types = ["federation.fanout", "federation.deliver"], instances = 2, concurrency = 3 } }
}
