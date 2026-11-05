# SPDX-License-Identifier: AGPL-3.0-only

variable "project_id" {
  description = "Project of the service, the jobs, and the service account."
  type        = string
  nullable    = false
}

variable "region" {
  description = "Region of the service and the jobs."
  type        = string
  nullable    = false
}

variable "name" {
  description = "Name of the service, and start of the names of the jobs and the service account."
  type        = string
  default     = "my"
  nullable    = false
}

variable "image" {
  description = "Server image at creation, as ghcr.io/mycroblog/my:<version>, optionally with @sha256:<digest>. After that only the upgrade script changes the image: apply keeps what runs."
  type        = string
  nullable    = false

  validation {
    condition     = can(regex("(:[^/:@]+|@sha256:[0-9a-f]{64})$", var.image)) && !can(regex(":latest(@|$)", var.image))
    error_message = "image must name a version or a digest. There is no latest tag."
  }
}

variable "host" {
  description = "Domain of the server (host). It cannot change once the server has started."
  type        = string
  nullable    = false
}

variable "role" {
  description = "role of the service. Cloud Tasks and Cloud Scheduler call it, so it must run jobs: all or worker."
  type        = string
  default     = "all"
  nullable    = false

  validation {
    condition     = contains(["all", "worker"], var.role)
    error_message = "role must be all or worker: the endpoints that Cloud Tasks and Cloud Scheduler call are not served by web."
  }
}

variable "profile" {
  description = "profile of the server."
  type        = string
  default     = "production"
  nullable    = false
}

variable "trusted_proxies" {
  description = "trusted_proxies: the addresses Cloud Run's front end connects from. Add a load balancer's address if one is in front."
  type        = string
  default     = "169.254.0.0/16"
  nullable    = false
}

variable "backends" {
  description = "What each backend module returns. A setting named twice fails the plan."
  type = list(object({
    env          = optional(map(string), {})
    secret_env   = optional(map(string), {})
    secret_files = optional(map(string), {})
    roles = optional(list(object({
      role     = string
      scope    = string
      resource = optional(string)
    })), [])
    invokers = optional(list(string), [])
  }))
  default  = []
  nullable = false

  validation {
    condition = alltrue(flatten([
      for b in var.backends : [
        for r in b.roles : (
          contains(["project", "bucket", "topic", "queue", "service_account", "crypto_key"], r.scope) &&
          (r.scope == "project") == (r.resource == null)
        )
      ]
    ]))
    error_message = "Each role has a scope of project (without a resource), or bucket, topic, queue, service_account, or crypto_key (with its resource)."
  }

  validation {
    condition = alltrue(flatten([
      for b in var.backends : [for name in keys(b.secret_files) : endswith(name, "_FILE")]
    ]))
    error_message = "The names of secret_files end in _FILE: the variable holds the path of the mounted file."
  }
}

variable "env" {
  description = "Other settings by variable name, for everything no backend covers."
  type        = map(string)
  default     = {}
  nullable    = false
}

variable "secret_env" {
  description = "Other secret settings: variable name to Secret Manager secret ID. The service account gets access to each."
  type        = map(string)
  default     = {}
  nullable    = false
}

variable "secret_files" {
  description = "Other secret settings to mount as files: variable name, ending in _FILE, to Secret Manager secret ID."
  type        = map(string)
  default     = {}
  nullable    = false

  validation {
    condition     = alltrue([for name in keys(var.secret_files) : endswith(name, "_FILE")])
    error_message = "The names of secret_files end in _FILE: the variable holds the path of the mounted file."
  }
}

variable "cpu" {
  description = "CPUs of an instance. Below 1, Cloud Run takes one request at a time."
  type        = string
  default     = "1"
  nullable    = false
}

variable "memory_mib" {
  description = "Memory of an instance in MiB. GOMEMLIMIT is set to 90% of it."
  type        = number
  default     = 512
  nullable    = false
}

variable "max_instances" {
  description = "Most instances at once. With pool_max_conns in the database URL, it bounds the connections to the database."
  type        = number
  default     = 3
  nullable    = false
}

variable "concurrency" {
  description = "Requests per instance. An open stream of real-time updates counts as one."
  type        = number
  default     = 80
  nullable    = false
}

variable "request_timeout_seconds" {
  description = "Request timeout of the service. It must exceed queue_cloudtasks_dispatch_deadline, 10 minutes by default."
  type        = number
  default     = 900
  nullable    = false
}

variable "job_memory_mib" {
  description = "Memory of the migration and console jobs in MiB."
  type        = number
  default     = 1024
  nullable    = false
}

variable "job_timeout_seconds" {
  description = "How long one job execution may run."
  type        = number
  default     = 3600
  nullable    = false
}

variable "console_enabled" {
  description = "Creates the console job. Turn it on for as long as you need it."
  type        = bool
  default     = false
  nullable    = false
}

variable "domain_mapping" {
  description = "Maps host to the service with a Cloud Run domain mapping (a Preview feature, in some regions only). Leave it off when a load balancer or another front serves host."
  type        = bool
  default     = false
  nullable    = false
}

variable "deletion_protection" {
  description = "Refuses to delete the service and the jobs."
  type        = bool
  default     = true
  nullable    = false
}
