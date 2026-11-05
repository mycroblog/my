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

output "service_account_unique_id" {
  description = "Numeric ID of that service account, which another cloud's trust policy names."
  value       = google_service_account.server.unique_id
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
  description = "Merged settings of the service, without GOMEMLIMIT. The jobs and the workers take the same, without what splits the job types."
  value       = local.service_env
}

output "secret_env" {
  description = "Merged secret settings of the service: variable name to secret ID."
  value       = local.secret_env
}

output "one_off_secret_env" {
  description = "Merged secret settings of the migration and console jobs, which take direct_secret_env over secret_env."
  value       = local.one_off_secret_env
}

output "subnetwork" {
  description = "The subnet that the service and the jobs attach to, or null with no network."
  value       = var.network == null ? null : var.network.subnetwork
}

output "worker_services" {
  description = "The Cloud Run service of each worker, by its key."
  value       = { for key, w in local.workers : key => w.name }
}

output "worker_env" {
  description = "The settings of each worker's service, without GOMEMLIMIT."
  value       = { for key, w in local.workers : key => w.env }
}
