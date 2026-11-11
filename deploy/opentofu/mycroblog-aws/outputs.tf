# SPDX-License-Identifier: AGPL-3.0-only

# What a deployment reads after an apply.

output "secrets_to_fill" {
  description = "The secrets this module created empty, which the server needs before it starts: secret name to what goes in it and the command that adds it."
  value = merge(
    {
      for key, what in local.to_fill : local.secret_ids[key] => {
        what    = what
        command = "aws secretsmanager put-secret-value --region ${local.region} --secret-id ${local.secret_ids[key]} --secret-string '<value>'"
      }
    },
    {
      for key, s in module.settings.secrets : s.secret => {
        what    = "The value of the setting ${key}."
        command = "aws secretsmanager put-secret-value --region ${local.region} --secret-id ${s.secret} --secret-string '<value>'"
      }
    },
  )
}

output "dns_records" {
  description = "The records to add at the DNS provider: host, unless Route 53 or Cloudflare creates it, and with mail.kind = ses the DKIM records that verify the domain of mail.from in SES. Empty where Route 53 or Cloudflare creates them."
  value = concat(
    var.ingress.kind == "load-balancer" ? module.ingress_alb.dns_records : [],
    var.ingress.kind == "cloudfront" ? module.ingress_cloudfront.dns_records : [],
    var.mail.kind == "ses" ? module.mail_ses.dns_records : [],
  )
}

output "dns_note" {
  description = "What dns_records cannot say, or null: at a zone apex without Route 53, the record of host needs an ALIAS or ANAME."
  value = (
    var.ingress.kind == "load-balancer" ? module.ingress_alb.dns_note :
    var.ingress.kind == "cloudfront" ? module.ingress_cloudfront.dns_note : null
  )
}

output "kek" {
  description = "Where the KEK is. With kek.kind = secrets-manager, copy it once with the command in copy and keep the copy away from this account: losing it loses every secret in the database."
  value = var.kek.kind == "secrets-manager" ? {
    kind   = "secrets-manager"
    secret = local.secret_ids.kek
    copy   = "aws secretsmanager get-secret-value --region ${local.region} --secret-id ${local.secret_ids.kek} --query SecretString --output text"
    } : {
    kind   = "kms"
    secret = null
    copy   = null
  }
}

output "domain_name" {
  description = "Domain name of what host points at: the distribution with ingress.kind = cloudfront, the load balancer otherwise, which Cloudflare connects to with ingress.kind = cloudflare."
  value       = var.ingress.kind == "cloudfront" ? module.ingress_cloudfront.domain_name : module.ingress_alb.domain_name
}

output "console" {
  description = "Pipe this to console.sh: tofu output -json console | .terraform/modules/mycroblog/deploy/opentofu/console.sh <my command>. Null while console_enabled is off."
  value       = module.app.console
}

output "upgrade" {
  description = "What upgrade.sh takes: --platform aws-ecs, --region, and --name, and the services it moves after the migration task."
  value = {
    platform                  = "aws-ecs"
    region                    = local.region
    name                      = var.name
    cluster_name              = module.app.cluster_name
    migration_task_definition = module.app.migration_task_definition
    service_name              = module.app.service_name
    worker_services           = module.app.worker_services
  }
}
