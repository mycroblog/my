# SPDX-License-Identifier: AGPL-3.0-only

# What a deployment reads after an apply.

output "secrets_to_fill" {
  description = "The secrets this module created empty, which the server needs before it starts: secret ID to what goes in it and the command that adds it."
  value = merge(
    {
      for key, what in local.to_fill : local.secret_ids[key] => {
        what    = what
        command = "printf '%s' '<value>' | gcloud secrets versions add ${local.secret_ids[key]} --project=${local.project_id} --data-file=-"
      }
    },
    {
      for key, s in module.settings.secrets : s.secret => {
        what    = "The value of the setting ${key}."
        command = "printf '%s' '<value>' | gcloud secrets versions add ${s.secret} --project=${local.project_id} --data-file=-"
      }
    },
  )
}

output "dns_records" {
  description = "The records to add at the DNS provider for the ingress and the media host. Empty where Cloudflare creates them."
  value = concat(
    var.ingress.kind == "load-balancer" ? module.ingress_load_balancer.dns_records : [],
    var.ingress.kind == "domain-mapping" ? module.ingress_domain_mapping.dns_records : [],
  )
}

output "kek" {
  description = "Where the KEK is. With kek.kind = secret-manager, copy it once with the command in copy and keep the copy away from this project: losing it loses every secret in the database."
  value = var.kek.kind == "secret-manager" ? {
    kind   = "secret-manager"
    secret = local.secret_ids.kek
    copy   = "gcloud secrets versions access latest --secret=${local.secret_ids.kek} --project=${local.project_id}"
    } : {
    kind   = "kms"
    secret = null
    copy   = null
  }
}

output "service_uri" {
  description = "URL Cloud Run gave the service. With ingress.kind = load-balancer it answers only through the load balancer."
  value       = module.app.service_uri
}

output "console" {
  description = "Pipe this to console.sh: tofu output -json console | .terraform/modules/mycroblog/deploy/opentofu/console.sh <my command>. Null while console_enabled is off."
  value       = module.app.console
}

output "upgrade" {
  description = "What upgrade.sh takes: --project, --region, and --name, and the services it moves after the migration job."
  value = {
    project         = local.project_id
    region          = local.region
    name            = var.name
    migration_job   = module.app.migration_job
    service_name    = module.app.service_name
    worker_services = module.app.worker_services
  }
}
