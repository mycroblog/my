# SPDX-License-Identifier: AGPL-3.0-only

# The inputs of mycroblog-gcp. Each section picks a backend by kind, and every section defaults to the recommended setup, so
# that a deployment names only host, media.bucket_name, mail, and monitoring. HCL has no union types: the fields of one kind sit
# beside the others in the section's object, and the validations below refuse what does not fit together before the plan.

# ---------------------------------------------------------------------------------------------------------------------------------
# Where and what

variable "project_id" {
  description = "Google Cloud project. Null takes the project of the google provider."
  type        = string
  default     = null
}

variable "region" {
  description = "Region of the service, the jobs, the queue, and every resource that has one. Null takes the region of the google provider."
  type        = string
  default     = null
}

variable "name" {
  description = "Start of every name this module creates (the services, the secrets, the queue, the network), so that two deployments can share a project."
  type        = string
  default     = "my"
  nullable    = false

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,18}[a-z0-9]$", var.name))
    error_message = "name must be 2 to 20 lowercase letters, digits, and hyphens, start with a letter, and not end with a hyphen."
  }
}

variable "host" {
  description = "Domain of the server. It cannot change once the server has started."
  type        = string
  nullable    = false
}

variable "image" {
  description = "The server image: repository, version, the image tag (0.2.0, or nightly), and digest, sha256:<hex>, which pins the image by its digest. version defaults to the release of this module, so that raising the ref of the source upgrades too. Another image is an upgrade: the apply runs the migration with it, then moves the workers, then the service."
  type = object({
    repository = optional(string, "ghcr.io/mycroblog/my")
    version    = optional(string)
    digest     = optional(string)
  })
  default  = {}
  nullable = false

  validation {
    condition     = var.image.version == null || can(regex("^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$", var.image.version))
    error_message = "image.version must be an image tag, such as 0.2.0."
  }

  validation {
    condition     = var.image.version != "latest"
    error_message = "image.version must name a version. There is no latest tag."
  }

  validation {
    condition     = var.image.version == null || !can(regex("^v[0-9]", var.image.version))
    error_message = "image.version is the image tag, without the v of the release: 0.2.0, not v0.2.0."
  }

  validation {
    condition     = var.image.digest == null || can(regex("^sha256:[0-9a-f]{64}$", var.image.digest))
    error_message = "image.digest must be sha256: and 64 hexadecimal digits."
  }
}

variable "upgrade" {
  description = "How an upgrade runs. allow_destructive_for names the one version, such as 0.3.0, whose migration may drop a table or a column, as the release notes of that version call for: my upgrade gets -allow-destructive only on the apply of that version, so a value left here lets no later release drop data."
  type = object({
    allow_destructive_for = optional(string)
  })
  default  = {}
  nullable = false
}

# ---------------------------------------------------------------------------------------------------------------------------------
# Network and traffic

variable "network" {
  description = "The VPC the service and the jobs attach to. none (recommended while every backend has a public endpoint) attaches nothing; vpc creates one; existing attaches to a VPC of yours, described by network, subnetwork, private_services_range, and memorystore_connection_policy. kv.kind = memorystore needs a VPC, and database.kind = cloud-sql takes a private address on one. nat, with vpc alone, sends all egress through Cloud NAT, for a static source address."
  type = object({
    kind = optional(string, "none")
    nat  = optional(bool, false)

    # vpc
    subnet_cidr           = optional(string, "10.20.0.0/24")
    service_connect_cidr  = optional(string, "10.20.1.0/24")
    private_services_cidr = optional(string, "10.21.0.0/20")

    # existing
    network                       = optional(string)
    subnetwork                    = optional(string)
    private_services_range        = optional(string)
    memorystore_connection_policy = optional(bool, false)
  })
  default  = {}
  nullable = false

  validation {
    condition     = contains(["none", "vpc", "existing"], var.network.kind)
    error_message = "network.kind must be none, vpc, or existing."
  }

  validation {
    condition     = var.network.kind != "existing" || (var.network.network != null && var.network.subnetwork != null)
    error_message = "network.kind = existing needs network.network and network.subnetwork."
  }

  validation {
    condition     = var.network.kind == "vpc" || !var.network.nat
    error_message = "network.nat needs network.kind = vpc. On a VPC of yours, set up Cloud NAT there."
  }
}

variable "ingress" {
  description = "How traffic reaches the service. load-balancer (recommended) closes the run.app URL, at a monthly fee; domain-mapping costs nothing but is a Preview feature and leaves the run.app URL open; cloudflare puts the zone of cloudflare in front and closes the origin with a secret this module generates. media_host adds a cached media host on Cloudflare beside any of them."
  type = object({
    kind       = optional(string, "load-balancer")
    media_host = optional(string)
  })
  default  = {}
  nullable = false

  validation {
    condition     = contains(["load-balancer", "domain-mapping", "cloudflare"], var.ingress.kind)
    error_message = "ingress.kind must be load-balancer, domain-mapping, or cloudflare."
  }

  validation {
    condition     = (var.ingress.kind != "cloudflare" && var.ingress.media_host == null) || try(var.cloudflare.zone_id, null) != null
    error_message = "ingress.kind = cloudflare and ingress.media_host need cloudflare.zone_id."
  }

  validation {
    condition     = var.ingress.media_host != var.host
    error_message = "ingress.media_host must be another host than host."
  }
}

variable "cloudflare" {
  description = "The Cloudflare account and zone, for every Cloudflare kind: ingress.kind = cloudflare, ingress.media_host, and media.kind = r2. Null, the default, when none is chosen; the provider then needs no CLOUDFLARE_API_TOKEN. host_header_override, on the Enterprise plan alone, lets Cloudflare connect to the run.app URL; otherwise a domain mapping of each host is the origin. proxied = false leaves the records unproxied, which a domain mapping may need until its certificate is issued."
  type = object({
    zone_id              = optional(string)
    account_id           = optional(string)
    host_header_override = optional(bool, false)
    proxied              = optional(bool, true)
  })
  default = null
}

# ---------------------------------------------------------------------------------------------------------------------------------
# Backends

variable "database" {
  description = "The PostgreSQL database. external (recommended: Neon, or any PostgreSQL you run) reads its URL from a secret you fill; pooled adds a second secret for a pooled URL, which the service then uses. cloud-sql creates an instance and its password, which bills while idle: through the connector's socket, or on a private address with a VPC."
  type = object({
    kind   = optional(string, "external")
    pooled = optional(bool, false)

    # cloud-sql
    tier                = optional(string, "db-f1-micro")
    disk_size_gb        = optional(number, 10)
    retained_backups    = optional(number, 7)
    deletion_protection = optional(bool, true)
  })
  default  = {}
  nullable = false

  validation {
    condition     = contains(["external", "cloud-sql"], var.database.kind)
    error_message = "database.kind must be external or cloud-sql."
  }

  validation {
    condition     = var.database.kind == "external" || !var.database.pooled
    error_message = "database.pooled needs database.kind = external: Cloud SQL has no pooler here."
  }
}

variable "queue" {
  description = "The job queue. cloud-tasks (recommended) pushes jobs to the service, which can then scale to zero; db polls the database and needs a process that stays up (service.min_instances >= 1 and service.cpu_always_allocated). tick_schedule, every minute by default, wakes the server and the database: a longer one lets a database that scales to zero stop more often."
  type = object({
    kind                      = optional(string, "cloud-tasks")
    tick_schedule             = optional(string, "* * * * *")
    max_concurrent_dispatches = optional(number, 20)
  })
  default  = {}
  nullable = false

  validation {
    condition     = contains(["cloud-tasks", "db"], var.queue.kind)
    error_message = "queue.kind must be cloud-tasks or db."
  }

  validation {
    condition     = var.queue.kind != "db" || (var.service.min_instances >= 1 && var.service.cpu_always_allocated)
    error_message = "queue.kind = db polls the database from a process that stays up: it needs service.min_instances >= 1 and service.cpu_always_allocated = true, which bill an instance all the time. Keep queue.kind = cloud-tasks to scale to zero."
  }
}

variable "media" {
  description = "The media store. gcs (recommended) creates a bucket; r2 creates an R2 bucket in cloudflare.account_id; s3 uses any S3-compatible store at endpoint. r2 and s3 read their access key from secrets you fill."
  type = object({
    kind        = optional(string, "gcs")
    bucket_name = string

    # gcs. A null location takes the region.
    location                = optional(string)
    versioning              = optional(bool, false)
    noncurrent_version_days = optional(number, 35)
    # r2
    jurisdiction = optional(string, "default")
    # s3
    endpoint   = optional(string)
    s3_region  = optional(string)
    path_style = optional(bool, false)
  })
  nullable = false

  validation {
    condition     = contains(["gcs", "r2", "s3"], var.media.kind)
    error_message = "media.kind must be gcs, r2, or s3."
  }

  validation {
    condition     = var.media.kind != "s3" || var.media.endpoint != null
    error_message = "media.kind = s3 needs media.endpoint."
  }

  validation {
    condition     = var.media.kind != "r2" || try(var.cloudflare.account_id, null) != null
    error_message = "media.kind = r2 needs cloudflare.account_id."
  }
}

variable "kv" {
  description = "The store of caches and rate limits. db (recommended) keeps them in the database; memorystore creates Memorystore for Valkey, which bills while idle and needs a VPC; external reads a Valkey URL from a secret you fill, and with tls_ca the CA of its certificate from another."
  type = object({
    kind = optional(string, "db")

    # memorystore
    node_type     = optional(string, "SHARED_CORE_NANO")
    replica_count = optional(number, 0)
    # external
    tls_ca     = optional(bool, false)
    key_prefix = optional(string)
  })
  default  = {}
  nullable = false

  validation {
    condition     = contains(["db", "memorystore", "external"], var.kv.kind)
    error_message = "kv.kind must be db, memorystore, or external."
  }

  validation {
    condition     = var.kv.kind != "memorystore" || var.network.kind != "none"
    error_message = "kv.kind = memorystore needs network.kind = vpc or existing: Memorystore has only private addresses."
  }

  validation {
    condition     = var.kv.kind == "external" || (!var.kv.tls_ca && var.kv.key_prefix == null)
    error_message = "kv.tls_ca and kv.key_prefix need kv.kind = external."
  }
}

variable "stream" {
  description = "The broker of real-time updates. pubsub (recommended) reaches every instance; postgres uses LISTEN / NOTIFY of the database over its direct connection; valkey uses the Valkey of kv."
  type = object({
    kind = optional(string, "pubsub")
  })
  default  = {}
  nullable = false

  validation {
    condition     = contains(["pubsub", "postgres", "valkey"], var.stream.kind)
    error_message = "stream.kind must be pubsub, postgres, or valkey."
  }

  validation {
    condition     = var.stream.kind != "valkey" || var.kv.kind != "db"
    error_message = "stream.kind = valkey uses the Valkey of kv: it needs kv.kind = memorystore or external."
  }
}

variable "mail" {
  description = "Email, through an SMTP service: from, and smtp.host, port, and username. With a username, the password goes in a secret you fill."
  type = object({
    from = string
    smtp = object({
      host     = string
      port     = optional(number, 587)
      username = optional(string)
    })
  })
  nullable = false
}

variable "kek" {
  description = "The key that encrypts the secrets in the database. secret-manager (recommended) generates it as a secret, which you copy once (the output kek says how); kms creates a key in Cloud KMS, which never leaves it. Changing it on an instance with data is a move: read the guide first."
  type = object({
    kind = optional(string, "secret-manager")
  })
  default  = {}
  nullable = false

  validation {
    condition     = contains(["secret-manager", "kms"], var.kek.kind)
    error_message = "kek.kind must be secret-manager or kms."
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# The server

variable "service" {
  description = "Size of the Cloud Run service that serves traffic and, without workers, runs every job. min_instances = 0 scales to zero."
  type = object({
    cpu                     = optional(string, "1")
    memory_mib              = optional(number, 512)
    min_instances           = optional(number, 0)
    max_instances           = optional(number, 3)
    cpu_always_allocated    = optional(bool, false)
    concurrency             = optional(number, 80)
    request_timeout_seconds = optional(number, 900)
  })
  default  = {}
  nullable = false
}

variable "jobs" {
  description = "Size of the one-off jobs: the migration and the console."
  type = object({
    memory_mib      = optional(number, 1024)
    timeout_seconds = optional(number, 3600)
  })
  default  = {}
  nullable = false
}

variable "workers" {
  description = "Services that run some job types apart from the default service, by a key that names them: job_types, and min_instances, max_instances, cpu, memory_mib, concurrency, max_concurrent_dispatches, and max_dispatches_per_second. The service, the queue, and the monitoring take the same value. Empty, the default, runs every job type on the default service."
  type        = any
  default     = {}
  nullable    = false
}

variable "console_enabled" {
  description = "Creates the console job, which console.sh runs. Turn it on for as long as you need it."
  type        = bool
  default     = false
  nullable    = false
}

variable "deletion_protection" {
  description = "Refuses to destroy the service."
  type        = bool
  default     = true
  nullable    = false
}

variable "settings" {
  description = "Other settings, by configuration key (translator_deepl_url, not MY_TRANSLATOR_DEEPL_URL), for what no section covers. A key a section sets fails the plan."
  type        = map(string)
  default     = {}
  nullable    = false
}

variable "secrets" {
  description = "Other secret settings, by configuration key. Each gets an empty secret <name>-<key>, with hyphens for underscores, which the output secrets_to_fill lists."
  type        = set(string)
  default     = []
  nullable    = false
}

# ---------------------------------------------------------------------------------------------------------------------------------
# Operations

variable "monitoring" {
  description = "Uptime check, log-based metrics, and alert policies. Required, with no default: name at least one of notification_emails and notification_channels, since an alert that notifies nobody is not one, or set null to create none. Each uptime check wakes an instance, and the database too when it reaches it: uptime_period is the trade-off."
  type = object({
    notification_emails        = optional(list(string), [])
    notification_channels      = optional(list(string), [])
    page_notification_channels = optional(list(string), [])
    metrics_exporter           = optional(string, "none")
    uptime_period              = optional(string, "900s")
    disabled_alerts            = optional(set(string), [])

    database_size_limit_bytes      = optional(number)
    media_size_limit_bytes         = optional(number)
    remote_media_cache_limit_bytes = optional(number)
  })
}

variable "billing" {
  description = "The import of the usage cost into the server's dashboard. Null, the default, imports nothing."
  type = object({
    billing_account_id = string
    method             = optional(string, "bigquery")
    dataset_location   = optional(string, "US")
  })
  default = null
}
