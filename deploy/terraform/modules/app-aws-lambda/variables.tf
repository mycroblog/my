# SPDX-License-Identifier: AGPL-3.0-only

variable "image" {
  description = "Image at creation, in Amazon ECR in the function's region: a single-architecture image with the Lambda Web Adapter, built from the released one. The README has how. After creation the upgrade script changes it."
  type        = string
  nullable    = false

  validation {
    condition     = can(regex("^[0-9]{12}\\.dkr\\.ecr\\.[a-z0-9-]+\\.amazonaws\\.com(\\.cn)?/[^/].*$", var.image))
    error_message = "image must be in Amazon ECR, such as 111122223333.dkr.ecr.ap-northeast-1.amazonaws.com/my:0.1.0. Lambda runs no image from ghcr.io, and not the released one as it is: read the README."
  }
}

variable "architecture" {
  description = "Architecture of the image: x86_64 or arm64. Lambda runs no multi-architecture image."
  type        = string
  default     = "x86_64"
  nullable    = false

  validation {
    condition     = contains(["x86_64", "arm64"], var.architecture)
    error_message = "architecture must be x86_64 or arm64."
  }
}

variable "host" {
  description = "Public domain of the server. It cannot change once the server has started."
  type        = string
  nullable    = false
}

variable "name" {
  description = "Start of the names of the functions (<name>-web, -worker, -migrate, -console) and the role."
  type        = string
  default     = "my"
  nullable    = false
}

variable "profile" {
  description = "profile of the server."
  type        = string
  default     = "production"
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
      conditions = optional(list(object({
        test     = string
        variable = string
        values   = list(string)
      })), [])
    })), [])
    # Queues that Lambda polls to invoke the worker.
    event_sources = optional(map(object({
      arn                        = string
      visibility_timeout_seconds = number
    })), {})
    # A module of another platform's value, which this module cannot honor.
    service_ingress = optional(string)
    # What an ingress puts in front of the function. The module adds the Web Adapter.
    trusted_proxies = optional(list(string))
    # Accepted so that a backend of no platform fits, and refused below: nothing calls the functions.
    invokers = optional(list(string), [])
  }))
  default  = []
  nullable = false

  validation {
    condition = alltrue(flatten([
      for b in var.backends : [for r in b.roles : r.scope == "policy" && r.resource != null]
    ]))
    error_message = "Each role has a scope of policy and a resource: the role is an IAM action, and the resource an ARN. Another scope is a backend for Google Cloud."
  }

  validation {
    condition     = alltrue([for b in var.backends : b.service_ingress == null])
    error_message = "service_ingress belongs to Cloud Run: an ingress-gcp-* module does not work here. Nothing can limit a Function URL to what is in front of it: read the README."
  }

  validation {
    condition     = alltrue([for b in var.backends : length(b.invokers) == 0])
    error_message = "invokers belongs to a platform that calls the app. Here Lambda polls the queue, which a backend names in event_sources."
  }

  validation {
    condition = alltrue(flatten([
      for b in var.backends : [for name in keys(b.secret_files) : endswith(name, "_FILE")]
    ]))
    error_message = "The names of secret_files end in _FILE."
  }
}

variable "env" {
  description = "Other settings by variable name, for everything no backend covers."
  type        = map(string)
  default     = {}
  nullable    = false
}

variable "secret_env" {
  description = "Other secret settings: variable name to the ARN of a secret in Secrets Manager. The role gets access to each."
  type        = map(string)
  default     = {}
  nullable    = false
}

variable "secret_files" {
  description = "The same as secret_env under the name ending in _FILE, since Lambda mounts no secret: variable name to the ARN of a secret."
  type        = map(string)
  default     = {}
  nullable    = false

  validation {
    condition     = alltrue([for name in keys(var.secret_files) : endswith(name, "_FILE")])
    error_message = "The names of secret_files end in _FILE."
  }
}

variable "web_memory_mib" {
  description = "Memory of the web function in MiB, from which Lambda also derives its CPU. GOMEMLIMIT is set to 90% of it."
  type        = number
  default     = 512
  nullable    = false
}

variable "web_timeout_seconds" {
  description = "Timeout of the web function, at most 900. It is also the longest a stream of real-time updates stays open."
  type        = number
  default     = 900
  nullable    = false

  validation {
    condition     = var.web_timeout_seconds >= 1 && var.web_timeout_seconds <= 900
    error_message = "A Lambda function runs for 900 seconds at most."
  }
}

variable "web_reserved_concurrency" {
  description = "Most instances of the web function at once, each serving one request, which bounds the connections to the database. Null takes from the account's unreserved concurrency."
  type        = number
  default     = null
}

variable "worker_memory_mib" {
  description = "Memory of the worker function in MiB."
  type        = number
  default     = 1024
  nullable    = false
}

variable "worker_timeout_seconds" {
  description = "Timeout of the worker function. A job's record starts only while its 5-minute lease and 30 seconds remain, so at least 330, and 600 to 900 to run several records of a batch."
  type        = number
  default     = 900
  nullable    = false

  validation {
    condition     = var.worker_timeout_seconds >= 330 && var.worker_timeout_seconds <= 900
    error_message = "worker_timeout_seconds must be from 330, a job's lease and 30 seconds, to 900, the most a Lambda function runs."
  }
}

variable "worker_batch_size" {
  description = "Records of the queue in one invocation. The server runs them one after another."
  type        = number
  default     = 10
  nullable    = false
}

variable "worker_max_concurrency" {
  description = "Most invocations of the worker at once from one queue, from 2 to 1,000, which bounds the connections to the database."
  type        = number
  default     = 5
  nullable    = false

  validation {
    condition     = var.worker_max_concurrency >= 2 && var.worker_max_concurrency <= 1000
    error_message = "worker_max_concurrency must be from 2 to 1,000."
  }
}

variable "job_memory_mib" {
  description = "Memory of the migration and console functions in MiB."
  type        = number
  default     = 1024
  nullable    = false
}

variable "job_timeout_seconds" {
  description = "How long one command of the migration and console functions may run, at most 900. Cloud Run's jobs have an hour by default: a data upgrade step longer than this cannot run here."
  type        = number
  default     = 900
  nullable    = false

  validation {
    condition     = var.job_timeout_seconds >= 1 && var.job_timeout_seconds <= 900
    error_message = "A Lambda function runs for 900 seconds at most."
  }
}

variable "one_off_command" {
  description = "Arguments of the image for the migration and console functions, a command that takes one my command per invocation. No released image has it yet; the name is what this module assumes."
  type        = list(string)
  default     = ["lambda-run"]
  nullable    = false
}

variable "console_enabled" {
  description = "Creates the console function. Turn it on for as long as you need it."
  type        = bool
  default     = false
  nullable    = false
}

variable "log_retention_days" {
  description = "How long CloudWatch Logs keeps the logs of the functions."
  type        = number
  default     = 30
  nullable    = false
}
