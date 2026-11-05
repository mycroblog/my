# SPDX-License-Identifier: AGPL-3.0-only

output "service_name" {
  description = "Name of the Cloud Run service."
  value       = google_cloud_run_v2_service.server.name
}

output "service_uri" {
  description = "URL Cloud Run gave the service. The public origin stays https://{host}."
  value       = google_cloud_run_v2_service.server.uri
}

output "origin_host" {
  description = "Host name of the URL Cloud Run gave the service, for an ingress that takes its origin as an input."
  value       = trimprefix(google_cloud_run_v2_service.server.uri, "https://")
}

output "service_account_email" {
  description = "Service account of the service and the jobs."
  value       = google_service_account.server.email
}

output "migration_job" {
  description = "Name of the job the upgrade script runs."
  value       = google_cloud_run_v2_job.one_off["migrate"].name
}

output "console" {
  description = "What console.sh needs to run a my command, or null while console_enabled is off. Every field but platform belongs to the platform."
  value = var.console_enabled ? {
    platform = "gcp-cloud-run"
    project  = var.project_id
    region   = var.region
    job      = google_cloud_run_v2_job.one_off["console"].name
    service  = google_cloud_run_v2_service.server.name
  } : null
}

output "service_ingress" {
  description = "Who may reach the service, as service_ingress says."
  value       = var.service_ingress
}

output "env" {
  description = "Merged settings of the service and the jobs, without GOMEMLIMIT."
  value       = local.env
}

output "secret_env" {
  description = "Merged secret settings of the service: variable name to secret ID."
  value       = local.secret_env
}

output "one_off_secret_env" {
  description = "Merged secret settings of the migration and console jobs, which take direct_secret_env over secret_env."
  value       = local.one_off_secret_env
}
