# SPDX-License-Identifier: AGPL-3.0-only

# The inputs of mycroblog-aws. Each section picks a backend by kind, and every section defaults to the recommended setup, so
# that a deployment names only host, network.egress, ingress.certificate_arn, media.bucket_name, mail.from, and
# monitoring. HCL has no union types: the fields of one kind sit beside the others in the section's object, and the
# validations below refuse what does not fit together before the plan. NOTHING ON AWS HAS BEEN APPLIED.

# ---------------------------------------------------------------------------------------------------------------------------------
# Where and what

variable "name" {
  description = "Start of every name this module creates (the cluster, the services, the secrets, the load balancer, the VPC), so that two deployments can share an account and a region."
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

variable "architecture" {
  description = "Architecture of the tasks: x86_64 or arm64, which costs less on Fargate. The released image has both."
  type        = string
  default     = "x86_64"
  nullable    = false

  validation {
    condition     = contains(["x86_64", "arm64"], var.architecture)
    error_message = "architecture must be x86_64 or arm64."
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# Network and traffic

variable "network" {
  description = "The VPC, which ECS always needs. vpc (recommended) creates one over availability_zones, at least two; existing takes a VPC of yours: vpc_id, vpc_cidr, public_subnet_ids, public_subnet_cidrs, and private_subnet_ids. egress is required, with no default, since both ways out bill by the hour: public-ip gives each task a public IPv4 address in a public subnet; nat puts the tasks in private subnets behind a NAT gateway, which vpc creates and a VPC of yours must have."
  type = object({
    kind   = optional(string, "vpc")
    egress = string

    # vpc
    availability_zones = optional(list(string))
    cidr_block         = optional(string, "10.0.0.0/16")

    # existing
    vpc_id              = optional(string)
    vpc_cidr            = optional(string)
    public_subnet_ids   = optional(list(string))
    public_subnet_cidrs = optional(list(string))
    private_subnet_ids  = optional(list(string))
  })
  nullable = false

  validation {
    condition     = contains(["vpc", "existing"], var.network.kind)
    error_message = "network.kind must be vpc or existing: ECS needs a VPC."
  }

  validation {
    condition     = contains(["public-ip", "nat"], var.network.egress)
    error_message = "network.egress must be public-ip or nat. Both bill by the hour: read the costs in the README of deploy/opentofu."
  }

  validation {
    condition     = var.network.kind != "vpc" || length(coalesce(var.network.availability_zones, [])) >= 2
    error_message = "network.kind = vpc needs at least two network.availability_zones: the load balancer and RDS need two."
  }

  validation {
    condition = var.network.kind != "existing" || alltrue([
      for v in [var.network.vpc_id, var.network.vpc_cidr, var.network.public_subnet_ids, var.network.public_subnet_cidrs, var.network.private_subnet_ids] : v != null
    ])
    error_message = "network.kind = existing needs network.vpc_id, vpc_cidr, public_subnet_ids, public_subnet_cidrs, and private_subnet_ids."
  }

  validation {
    condition     = var.network.kind == "vpc" || var.network.availability_zones == null
    error_message = "network.availability_zones needs network.kind = vpc: a VPC of yours has its subnets already."
  }
}

variable "ingress" {
  description = "How traffic reaches the tasks: an Application Load Balancer always, with a certificate for host in this region (certificate_arn, required). load-balancer (recommended) serves host from it, with the records in route53_zone_id or listed in dns_records; cloudfront puts CloudFront in front, with /media/ cached, and the load balancer admits CloudFront alone: it needs cloudfront_certificate_arn, a certificate in us-east-1; cloudflare puts the zone of cloudflare in front, and the server refuses what lacks a secret this module generates. media_host adds a cached media host on Cloudflare beside load-balancer or cloudflare, which the certificate must cover too."
  type = object({
    kind = optional(string, "load-balancer")

    certificate_arn = string
    route53_zone_id = optional(string)
    host_is_apex    = optional(bool, false)

    # cloudfront
    cloudfront_certificate_arn = optional(string)
    cloudfront_price_class     = optional(string, "PriceClass_All")

    media_host = optional(string)
  })
  nullable = false

  validation {
    condition     = contains(["load-balancer", "cloudfront", "cloudflare"], var.ingress.kind)
    error_message = "ingress.kind must be load-balancer, cloudfront, or cloudflare."
  }

  validation {
    condition     = (var.ingress.kind == "cloudfront") == (var.ingress.cloudfront_certificate_arn != null)
    error_message = "ingress.kind = cloudfront needs ingress.cloudfront_certificate_arn, a certificate for host in us-east-1, the only region CloudFront takes one from; no other kind uses it."
  }

  validation {
    condition     = (var.ingress.kind != "cloudflare" && var.ingress.media_host == null) || try(var.cloudflare.zone_id, null) != null
    error_message = "ingress.kind = cloudflare and ingress.media_host need cloudflare.zone_id."
  }

  validation {
    condition     = var.ingress.kind != "cloudflare" || var.ingress.route53_zone_id == null
    error_message = "ingress.kind = cloudflare creates the record of host in the Cloudflare zone: leave ingress.route53_zone_id out."
  }

  validation {
    condition     = var.ingress.kind != "cloudfront" || var.ingress.media_host == null
    error_message = "ingress.media_host cannot sit beside ingress.kind = cloudfront: the load balancer then admits CloudFront alone, and CloudFront caches /media/ at host itself."
  }

  validation {
    condition     = var.ingress.media_host != var.host
    error_message = "ingress.media_host must be another host than host."
  }
}

variable "cloudflare" {
  description = "The Cloudflare account and zone, for every Cloudflare kind: ingress.kind = cloudflare, ingress.media_host, and media.kind = r2. Null, the default, when none is chosen; the provider then needs no CLOUDFLARE_API_TOKEN."
  type = object({
    zone_id    = optional(string)
    account_id = optional(string)
  })
  default = null
}

# ---------------------------------------------------------------------------------------------------------------------------------
# Backends

variable "database" {
  description = "The PostgreSQL database. external (recommended: Neon, or any PostgreSQL you run) reads its URL from a secret you fill; pooled adds a second secret for a pooled URL, which the service then uses. rds creates an instance and its password in the private subnets, which bills while idle."
  type = object({
    kind   = optional(string, "external")
    pooled = optional(bool, false)

    # rds
    instance_class           = optional(string, "db.t4g.micro")
    allocated_storage_gb     = optional(number, 20)
    max_allocated_storage_gb = optional(number, 100)
    backup_retention_days    = optional(number, 7)
    multi_az                 = optional(bool, false)
    deletion_protection      = optional(bool, true)
  })
  default  = {}
  nullable = false

  validation {
    condition     = contains(["external", "rds"], var.database.kind)
    error_message = "database.kind must be external or rds."
  }

  validation {
    condition     = var.database.kind == "external" || !var.database.pooled
    error_message = "database.pooled needs database.kind = external: RDS has no pooler here."
  }
}

# No queue section: the tasks stay up, so they poll the database for the jobs and run the periodic ones themselves.

variable "media" {
  description = "The media store. s3 (recommended) creates a bucket; r2 creates an R2 bucket in cloudflare.account_id; s3-compatible uses any S3-compatible store at endpoint. r2 and s3-compatible read their access key from secrets you fill."
  type = object({
    kind        = optional(string, "s3")
    bucket_name = string

    # s3
    versioning              = optional(bool, false)
    noncurrent_version_days = optional(number, 35)
    # r2
    jurisdiction = optional(string, "default")
    # s3-compatible
    endpoint   = optional(string)
    s3_region  = optional(string)
    path_style = optional(bool, false)
  })
  nullable = false

  validation {
    condition     = contains(["s3", "r2", "s3-compatible"], var.media.kind)
    error_message = "media.kind must be s3, r2, or s3-compatible."
  }

  validation {
    condition     = var.media.kind != "s3-compatible" || var.media.endpoint != null
    error_message = "media.kind = s3-compatible needs media.endpoint."
  }

  validation {
    condition     = var.media.kind != "r2" || try(var.cloudflare.account_id, null) != null
    error_message = "media.kind = r2 needs cloudflare.account_id."
  }
}

variable "kv" {
  description = "The store of caches and rate limits. db (recommended) keeps them in the database; elasticache creates ElastiCache Serverless for Valkey in the private subnets, which bills while idle; external reads a Valkey URL from a secret you fill, and with tls_ca the CA of its certificate from another."
  type = object({
    kind = optional(string, "db")

    # external
    tls_ca     = optional(bool, false)
    key_prefix = optional(string)
  })
  default  = {}
  nullable = false

  validation {
    condition     = contains(["db", "elasticache", "external"], var.kv.kind)
    error_message = "kv.kind must be db, elasticache, or external."
  }

  validation {
    condition     = var.kv.kind == "external" || (!var.kv.tls_ca && var.kv.key_prefix == null)
    error_message = "kv.tls_ca and kv.key_prefix need kv.kind = external."
  }
}

variable "stream" {
  description = "The broker of real-time updates. sns (recommended) reaches every task; postgres uses LISTEN / NOTIFY of the database over its direct connection; valkey uses the Valkey of kv."
  type = object({
    kind = optional(string, "sns")
  })
  default  = {}
  nullable = false

  validation {
    condition     = contains(["sns", "postgres", "valkey"], var.stream.kind)
    error_message = "stream.kind must be sns, postgres, or valkey."
  }

  validation {
    condition     = var.stream.kind != "valkey" || var.kv.kind != "db"
    error_message = "stream.kind = valkey uses the Valkey of kv: it needs kv.kind = elasticache or external."
  }
}

variable "mail" {
  description = "Email from the bare address from. ses (recommended) sends through Amazon SES in this region from the domain of from, whose records the output dns_records lists; a new account sends only to verified addresses until AWS grants production access. configuration_set, with ses alone, creates one and applies it to every message. smtp sends through an SMTP service: smtp.host, port, and username, whose password goes in a secret you fill."
  type = object({
    kind = optional(string, "ses")
    from = string

    # ses
    configuration_set = optional(string)
    # smtp
    smtp = optional(object({
      host     = string
      port     = optional(number, 587)
      username = optional(string)
    }))
  })
  nullable = false

  validation {
    condition     = contains(["ses", "smtp"], var.mail.kind)
    error_message = "mail.kind must be ses or smtp."
  }

  validation {
    condition     = (var.mail.kind == "smtp") == (var.mail.smtp != null)
    error_message = "mail.kind = smtp needs mail.smtp, and no other kind uses it."
  }

  validation {
    condition     = var.mail.kind == "ses" || var.mail.configuration_set == null
    error_message = "mail.configuration_set needs mail.kind = ses."
  }
}

variable "kek" {
  description = "The key that encrypts the secrets in the database. secrets-manager (recommended) generates it as a secret, which you copy once (the output kek says how); kms creates a key in AWS KMS, which never leaves it. Changing it on an instance with data is a move: read the guide first."
  type = object({
    kind = optional(string, "secrets-manager")
  })
  default  = {}
  nullable = false

  validation {
    condition     = contains(["secrets-manager", "kms"], var.kek.kind)
    error_message = "kek.kind must be secrets-manager or kms."
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# The server

variable "service" {
  description = "Size and count of the tasks of the ECS service, which serves traffic and, without workers, runs every job. AWS bills a task for as long as it runs, idle or not: nothing here scales to zero."
  type = object({
    cpu                        = optional(number, 256)
    memory_mib                 = optional(number, 1024)
    desired_count              = optional(number, 1)
    health_check_grace_seconds = optional(number, 120)
  })
  default  = {}
  nullable = false
}

variable "jobs" {
  description = "Size of the one-off tasks: the migration and the console, billed only while one runs."
  type = object({
    cpu        = optional(number, 512)
    memory_mib = optional(number, 1024)
  })
  default  = {}
  nullable = false
}

variable "workers" {
  description = "Services that run some job types apart from the default service, by a key that names them: job_types, and instances, cpu, memory_mib, and concurrency. The service and the queue take the same value. Empty, the default, runs every job type on the default service."
  type        = any
  default     = {}
  nullable    = false
}

variable "console_enabled" {
  description = "Creates the console's task definition, which console.sh runs. Turn it on for as long as you need it."
  type        = bool
  default     = false
  nullable    = false
}

variable "log_retention_days" {
  description = "How long CloudWatch Logs keeps the logs of the tasks."
  type        = number
  default     = 30
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
  description = "Uptime canary, metrics in CloudWatch, and alarms. Required, with no default: name at least one of notification_emails and notification_topic_arns, since an alarm that notifies nobody is not an alert, or set null to create none. Every run of the canary is billed: uptime_period_minutes is the trade-off."
  type = object({
    notification_emails          = optional(list(string), [])
    notification_topic_arns      = optional(list(string), [])
    page_notification_topic_arns = optional(list(string), [])
    uptime_period_minutes        = optional(number, 15)
    disabled_alerts              = optional(set(string), [])

    database_size_limit_bytes      = optional(number)
    media_size_limit_bytes         = optional(number)
    remote_media_cache_limit_bytes = optional(number)
  })

  validation {
    condition     = var.monitoring == null || length(try(var.monitoring.notification_emails, [])) + length(try(var.monitoring.notification_topic_arns, [])) > 0
    error_message = "monitoring must name at least one of notification_emails and notification_topic_arns, since an alarm that notifies nobody is not an alert, or be null to create none."
  }

  validation {
    condition     = var.monitoring == null || var.observability.metrics == "cloudwatch"
    error_message = "monitoring alarms on the metrics in CloudWatch: it needs observability.metrics = cloudwatch."
  }
}

variable "observability" {
  description = "Where the metrics, the traces, and the error reports go. Everything rides on the log lines, which the tasks' log driver sends to CloudWatch Logs, so nothing needs a grant: metrics = cloudwatch writes them in Embedded Metric Format, in the namespace name, which CloudWatch extracts; errors = cloudwatch keeps the error report line, with its stack, in CloudWatch Logs; traces = none records no spans, since X-Ray takes them only through a collector beside the server, which the tasks do not run. settings leaves a signal to the settings input: errors = settings with errors_exporter = sentry in settings and errors_sentry_dsn in secrets, for one."
  type = object({
    metrics = optional(string, "cloudwatch")
    traces  = optional(string, "none")
    errors  = optional(string, "cloudwatch")
  })
  default  = {}
  nullable = false

  validation {
    condition = (
      contains(["cloudwatch", "settings"], var.observability.metrics) &&
      contains(["none", "settings"], var.observability.traces) &&
      contains(["cloudwatch", "settings"], var.observability.errors)
    )
    error_message = "observability.metrics must be cloudwatch or settings, traces none or settings, and errors cloudwatch or settings."
  }
}

variable "translation" {
  description = "Amazon Translate, the translation provider aws: the module allows the tasks' role translate:TranslateText, and comprehend:DetectDominantLanguage for a post whose language is unknown, and sets the provider's region, the deployment's by default. The instance setting translation_providers enables the provider. Null, the default, does none of it; DeepL and LibreTranslate go through settings and secrets."
  type = object({
    region = optional(string)
  })
  default = null
}

variable "search" {
  description = "Search on a domain of Amazon OpenSearch Service that you created, in the region, with an access policy that lets the tasks' role in: domain_name. The module reads the domain, allows the role the HTTP methods the server sends to it, and sets search = opensearch with the requests signed. index names the alias (my-statuses). For a domain in the VPC, security_group_ids are groups the tasks join, which its security group admits. Null, the default, leaves search to the settings: the database, or Elasticsearch, Meilisearch, or OpenSearch with a user and password."
  type = object({
    domain_name        = string
    index              = optional(string)
    security_group_ids = optional(list(string), [])
  })
  default = null
}

variable "billing" {
  description = "The import of the usage cost into the server's dashboard, through Cost Explorer with the tasks' own role. Null, the default, imports nothing; {} imports it."
  type        = object({})
  default     = null
}
