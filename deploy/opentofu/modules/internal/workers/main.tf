# SPDX-License-Identifier: AGPL-3.0-only

# What one value of the workers input means, for the app and queue modules that take it. It is no block of a deployment: it makes
# nothing and returns no settings, and the modules read it by a relative source. It is the one place that holds the job types and
# what must stay together, so that the app and its queue can only read the value the same way.

terraform {
  required_version = ">= 1.11"
}

variable "workers" {
  description = "The workers, by a key that becomes part of their names. Any type, so that a field nobody reads is an error here rather than dropped by a conversion."
  type        = any
  default     = {}
  nullable    = false
}

variable "fields" {
  description = "The fields that the module asking takes beside job_types."
  type        = list(string)
  nullable    = false
}

locals {
  # scripts/tofu-workers-check.sh keeps these in step with internal/platform/job_types.go and docs/operations/running.md.
  all_job_types = jsondecode(file("${path.module}/job_types.json"))
  keep_together = jsondecode(file("${path.module}/keep_together.json"))

  # The names of what a deployment already has, and what would be confused with them.
  reserved_keys = ["all", "console", "default", "jobs", "migrate", "server", "web", "worker"]

  known_fields = ["job_types", "min_instances", "max_instances", "instances", "cpu", "memory_mib", "concurrency", "max_concurrent_dispatches", "max_dispatches_per_second"]

  workers = {
    for key, w in var.workers : key => {
      job_types                 = sort(distinct(try(tolist(w.job_types), [])))
      min_instances             = try(w.min_instances, null)
      max_instances             = try(w.max_instances, null)
      instances                 = try(w.instances, null)
      cpu                       = try(w.cpu, null)
      memory_mib                = try(w.memory_mib, null)
      concurrency               = try(w.concurrency, null)
      max_concurrent_dispatches = try(w.max_concurrent_dispatches, null)
      max_dispatches_per_second = try(w.max_dispatches_per_second, null)
    }
  }

  # What a pattern covers, as the server reads it: a name, or a prefix ending in *.
  covered = {
    for key, w in local.workers : key => distinct(flatten([
      for pattern in w.job_types : [
        for type in local.all_job_types : type
        if type == pattern || (endswith(pattern, "*") && startswith(type, trimsuffix(pattern, "*")))
      ]
    ]))
  }
  # Who runs a type: the workers that cover it. Nobody means the default service.
  holders = {
    for type in local.all_job_types : type => [for key, types in local.covered : key if contains(types, type)]
  }

  bad_keys = [
    for key in keys(local.workers) : "\"${key}\" is not a usable key of workers: it becomes part of names, so it takes 1 to 20 lower-case letters, digits, and inner hyphens, starting with a letter, and is none of ${join(", ", local.reserved_keys)}."
    if !can(regex("^[a-z]([a-z0-9-]{0,18}[a-z0-9])?$", key)) || contains(local.reserved_keys, key)
  ]
  bad_fields = flatten([
    for key, w in var.workers : [
      for field in keys(w) : "worker \"${key}\" has the field ${field}, which ${contains(local.known_fields, field) ? "this platform and queue do not have" : "is no field of workers"}: it takes job_types, ${join(", ", var.fields)}."
      if field != "job_types" && !contains(var.fields, field)
    ]
  ])
  empty = [
    for key, w in local.workers : "worker \"${key}\" has no job_types."
    if length(w.job_types) == 0
  ]
  unknown_patterns = flatten([
    for key, w in local.workers : [
      for pattern in w.job_types : "worker \"${key}\" names ${pattern}, which matches no job type. my job types prints them."
      if length([for type in local.all_job_types : type if type == pattern || (endswith(pattern, "*") && startswith(type, trimsuffix(pattern, "*")))]) == 0
    ]
  ])
  twice = [
    for type, keys in local.holders : "${type} is on two workers, ${join(" and ", keys)}: a type has one home, or the split means nothing."
    if length(keys) > 1
  ]
  split = [
    for group in local.keep_together : "${join(" and ", group)} must stay on one worker, since jobs of one ordering key run in order whatever their type, and they are split: ${join(", ", [for type in group : "${type} on ${length(local.holders[type]) == 0 ? "the default" : local.holders[type][0]}"])}."
    if length(distinct([for type in group : join(",", local.holders[type])])) > 1
  ]
  bad_concurrency = [
    for key, w in local.workers : "worker \"${key}\" has concurrency ${w.concurrency}, and it must be a whole number from 1 to 128."
    if w.concurrency != null && !try(w.concurrency >= 1 && w.concurrency <= 128 && floor(w.concurrency) == w.concurrency, false)
  ]
}

output "workers" {
  description = "Every worker with every field, null where the value does not name it, and its job_types sorted."
  value       = local.workers
}

output "job_types" {
  description = "The job types of each worker, which an app compares with what its queue module was given."
  value       = { for key, w in local.workers : key => w.job_types }
}

output "all_patterns" {
  description = "Every pattern of every worker, sorted, for worker_job_types_except of the default."
  value       = sort(distinct(flatten([for w in local.workers : w.job_types])))
}

output "problems" {
  description = "What is wrong with the value, each with what to do. The modules that take it fail the plan on these."
  value = concat(
    local.bad_keys,
    local.bad_fields,
    local.empty,
    local.unknown_patterns,
    local.twice,
    local.split,
    local.bad_concurrency,
  )
}
