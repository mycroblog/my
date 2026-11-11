# SPDX-License-Identifier: AGPL-3.0-only

# Traffic reaches the app through an Application Load Balancer with TLS. The security group of the tasks admits only this load
# balancer, so the tasks are closed on the platform and need no secret header. With behind_cloudfront the load balancer itself
# admits only CloudFront, and forwards only what carries CloudFront's header. This module has never been applied: read the README.

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.0, < 7.0"
    }
  }
}

variable "host" {
  description = "Domain of the server."
  type        = string
  nullable    = false
}

variable "name" {
  description = "Name of the load balancer and its target group, and the start of its security group's."
  type        = string
  default     = "my"
  nullable    = false

  validation {
    condition     = can(regex("^[a-zA-Z0-9]([a-zA-Z0-9-]{0,30}[a-zA-Z0-9])?$", var.name)) && !startswith(var.name, "internal-")
    error_message = "name must be 1 to 32 letters, digits, and inner hyphens, and must not start with internal-."
  }
}

variable "network" {
  description = "The network of the load balancer, from network-aws-vpc or an object of the same shape for a VPC of yours: its public subnets, each with a free /27 at least, in two Availability Zones or more. A task in a zone that none of these covers gets no traffic."
  type = object({
    platform            = string
    vpc_id              = optional(string)
    vpc_cidr            = optional(string)
    public_subnet_ids   = optional(list(string), [])
    public_subnet_cidrs = optional(list(string), [])
    private_subnet_ids  = optional(list(string), [])
    nat                 = optional(bool, false)
  })
  nullable = false

  validation {
    condition     = var.network.platform == "aws"
    error_message = "network is of another platform: on AWS use network-aws-vpc, or an object of the shape in the README of deploy/opentofu."
  }

  validation {
    condition     = var.network.vpc_id != null
    error_message = "network.vpc_id is required."
  }

  validation {
    condition     = length(var.network.public_subnet_ids) >= 2
    error_message = "An Application Load Balancer needs network.public_subnet_ids in at least two Availability Zones."
  }

  validation {
    condition     = length(var.network.public_subnet_cidrs) == length(var.network.public_subnet_ids)
    error_message = "network.public_subnet_cidrs must hold the range of each of network.public_subnet_ids: they are what the server trusts."
  }

  validation {
    condition     = var.network.vpc_cidr != null
    error_message = "network.vpc_cidr is required: it is what the load balancer's group allows out to reach the tasks."
  }
}

variable "certificate_arn" {
  description = "ARN of a validated certificate for host in AWS Certificate Manager, in the region of the load balancer. Create it by hand first: validating one here would hold the apply until the records it prints are in DNS."
  type        = string
  nullable    = false

  validation {
    condition     = can(regex("^arn:[a-z-]+:acm:[a-z0-9-]+:[0-9]{12}:certificate/.+$", var.certificate_arn))
    error_message = "certificate_arn must be a certificate in AWS Certificate Manager."
  }
}

variable "ssl_policy" {
  description = "TLS security policy of the listener. The default is the one AWS recommends."
  type        = string
  default     = "ELBSecurityPolicy-TLS13-1-2-Res-PQ-2025-09"
  nullable    = false
}

variable "ip_address_type" {
  description = "ipv4, or dualstack for AAAA too, which needs IPv6 on the subnets. The load balancer reaches the tasks over IPv4 either way."
  type        = string
  default     = "ipv4"
  nullable    = false

  validation {
    condition     = contains(["ipv4", "dualstack"], var.ip_address_type)
    error_message = "ip_address_type must be ipv4 or dualstack."
  }

  validation {
    condition     = !var.behind_cloudfront || var.ip_address_type == "ipv4"
    error_message = "Behind CloudFront, viewers reach CloudFront over IPv6 too, and CloudFront reaches the load balancer over IPv4: leave ip_address_type at ipv4."
  }
}

variable "behind_cloudfront" {
  description = "Puts ingress-aws-cloudfront in front: the load balancer admits CloudFront's origin-facing addresses alone, forwards only a request that carries origin_secret in origin_secret_header, answers 403 to the rest, and creates no DNS record, since host points at CloudFront."
  type        = bool
  default     = false
  nullable    = false
}

variable "origin_secret" {
  description = "With behind_cloudfront, the value of the header that CloudFront sends and the listener checks. THE PROVIDER HAS NO WRITE-ONLY FORM OF EITHER, SO IT LANDS IN THE STATE: keep the state private. Pass it with TF_VAR_origin_secret."
  type        = string
  default     = null
  sensitive   = true

  validation {
    condition     = (var.origin_secret == null) == !var.behind_cloudfront
    error_message = "origin_secret is required with behind_cloudfront, and has no use without it."
  }

  # A listener matches a header's value up to 128 characters, without case, and reads * and ? as wildcards.
  validation {
    condition     = var.origin_secret == null || can(regex("^[A-Za-z0-9_-]{32,128}$", var.origin_secret))
    error_message = "origin_secret must be 32 to 128 letters, digits, hyphens, and underscores: openssl rand -hex 32."
  }
}

variable "origin_secret_previous" {
  description = "A second value that the listener accepts, only while the value rotates. Read the README."
  type        = string
  default     = null
  sensitive   = true

  validation {
    condition     = var.origin_secret_previous == null || (var.behind_cloudfront && can(regex("^[A-Za-z0-9_-]{32,128}$", var.origin_secret_previous)))
    error_message = "origin_secret_previous needs behind_cloudfront, and must be 32 to 128 letters, digits, hyphens, and underscores."
  }
}

variable "origin_secret_header" {
  description = "Name of the header that carries origin_secret, which nothing else uses."
  type        = string
  default     = "x-my-origin-secret"
  nullable    = false

  validation {
    condition     = length(var.origin_secret_header) <= 40 && can(regex("^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$", var.origin_secret_header)) && !can(regex("(?i)^(cf-|x-cf-|x-forwarded-|x-amz|x-amzn-|x-edge-|sec-|proxy-|if-|access-control-|host$)", var.origin_secret_header))
    error_message = "origin_secret_header must be up to 40 letters, digits, and inner hyphens, and must not be a name that CloudFront, the load balancer, or a browser sets, such as x-forwarded-*."
  }
}

variable "idle_timeout" {
  description = "Seconds a connection may stay silent. Above the 30 seconds between a stream's heartbeats, and below the 120 for which the server keeps an idle connection, so that the server never closes one the load balancer is about to use."
  type        = number
  default     = 60
  nullable    = false

  validation {
    condition     = var.idle_timeout > 30 && var.idle_timeout < 120
    error_message = "idle_timeout must be above 30, a stream's heartbeat, and below 120, the server's own idle timeout."
  }
}

variable "deregistration_delay" {
  description = "Seconds a leaving task keeps its connections. An open stream lasts until the end of this, so it is also how long an upgrade waits per task; clients reconnect."
  type        = number
  default     = 30
  nullable    = false
}

variable "health_check_path" {
  description = "What the load balancer asks. /readyz answers 200 once the database is reachable and has its schema, which is what holds a bad image back. A load balancer has one check, so a database that stays down also fails it: read the README."
  type        = string
  default     = "/readyz"
  nullable    = false
}

variable "route53_zone_id" {
  description = "Public Route 53 hosted zone of host. Set, the module creates alias records, which work at a zone apex and on a subdomain. Null, you create the records that dns_records lists."
  type        = string
  default     = null

  validation {
    condition     = var.route53_zone_id == null || !var.behind_cloudfront
    error_message = "Behind CloudFront, host points at the distribution: give route53_zone_id to ingress-aws-cloudfront instead."
  }
}

variable "host_is_apex" {
  description = "Whether host is the apex of its DNS zone, which decides what dns_records asks for while route53_zone_id is null. An input, since the labels of a name cannot tell: social.example.org and example.co.uk both have three."
  type        = bool
  default     = false
  nullable    = false
}

locals {
  # The server's port. The app sets its container to the same.
  target_port = 8080

  open_to = merge(
    { ipv4 = { cidr_ipv4 = "0.0.0.0/0", cidr_ipv6 = null } },
    var.ip_address_type == "dualstack" ? { ipv6 = { cidr_ipv4 = null, cidr_ipv6 = "::/0" } } : {},
  )
  listener_ports = { http = 80, https = 443 }
  ingress_rules = var.behind_cloudfront ? {} : {
    for pair in setproduct(keys(local.listener_ports), keys(local.open_to)) : "${pair[0]}-${pair[1]}" => {
      port      = local.listener_ports[pair[0]]
      cidr_ipv4 = local.open_to[pair[1]].cidr_ipv4
      cidr_ipv6 = local.open_to[pair[1]].cidr_ipv6
    }
  }

  # What connects to the load balancer from CloudFront, which appends the viewer's address to X-Forwarded-For before the
  # load balancer appends CloudFront's.
  cloudfront_ranges = var.behind_cloudfront ? sort([for e in data.aws_ec2_managed_prefix_list.cloudfront[0].entries : e.cidr]) : []
}

data "aws_region" "current" {}

# AWS's list of the addresses that CloudFront connects to an origin from, which a security group can name as one rule.
data "aws_ec2_managed_prefix_list" "cloudfront" {
  count = var.behind_cloudfront ? 1 : 0

  name = "com.amazonaws.global.cloudfront.origin-facing"
}

resource "aws_security_group" "load_balancer" {
  name        = "${var.name}-alb"
  description = "Load balancer of ${var.host}"
  vpc_id      = var.network.vpc_id
}

resource "aws_vpc_security_group_ingress_rule" "internet" {
  for_each = local.ingress_rules

  security_group_id = aws_security_group.load_balancer.id
  description       = "From the internet"
  ip_protocol       = "tcp"
  from_port         = each.value.port
  to_port           = each.value.port
  cidr_ipv4         = each.value.cidr_ipv4
  cidr_ipv6         = each.value.cidr_ipv6
}

# CloudFront reaches the origin over HTTPS alone, so port 80 stays closed.
resource "aws_vpc_security_group_ingress_rule" "cloudfront" {
  security_group_id = aws_security_group.load_balancer.id
  description       = "From CloudFront"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  prefix_list_id    = data.aws_ec2_managed_prefix_list.cloudfront[0].id

  lifecycle {
    enabled = var.behind_cloudfront
  }
}

# The tasks are found by address. The VPC's primary range, since which subnets the tasks take is the app's choice.
resource "aws_vpc_security_group_egress_rule" "to_tasks" {
  security_group_id = aws_security_group.load_balancer.id
  description       = "To the tasks"
  ip_protocol       = "tcp"
  from_port         = local.target_port
  to_port           = local.target_port
  cidr_ipv4         = var.network.vpc_cidr
}

resource "aws_lb" "this" {
  name               = var.name
  load_balancer_type = "application"
  internal           = false
  ip_address_type    = var.ip_address_type
  subnets            = var.network.public_subnet_ids
  security_groups    = [aws_security_group.load_balancer.id]
  idle_timeout       = var.idle_timeout

  # append and no port are what the server reads: the rightmost address that is not a proxy is the client's, with no port on it.
  xff_header_processing_mode = "append"
  enable_xff_client_port     = false
  drop_invalid_header_fields = true
}

resource "aws_lb_target_group" "this" {
  name                 = var.name
  vpc_id               = var.network.vpc_id
  target_type          = "ip"
  protocol             = "HTTP"
  protocol_version     = "HTTP1"
  port                 = local.target_port
  deregistration_delay = var.deregistration_delay

  # 2.5 minutes of failures before ECS replaces a task, so that a short outage of the database restarts nothing. 5 seconds,
  # since the server gives up on the database after 3.
  health_check {
    path                = var.health_check_path
    protocol            = "HTTP"
    matcher             = "200"
    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 5
  }
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.this.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = var.ssl_policy
  certificate_arn   = var.certificate_arn

  # Behind CloudFront, only the rule below forwards: another distribution, of another account, connects from the same addresses.
  default_action {
    type             = var.behind_cloudfront ? "fixed-response" : "forward"
    target_group_arn = var.behind_cloudfront ? null : aws_lb_target_group.this.arn

    dynamic "fixed_response" {
      for_each = var.behind_cloudfront ? [1] : []
      content {
        content_type = "text/plain"
        message_body = "Forbidden"
        status_code  = "403"
      }
    }
  }

  lifecycle {
    precondition {
      condition     = strcontains(var.certificate_arn, ":acm:${data.aws_region.current.region}:")
      error_message = "certificate_arn must be in ${data.aws_region.current.region}, the region of the load balancer."
    }
  }
}

# A request with the header CloudFront sends, which replaces whatever a viewer sent under that name.
resource "aws_lb_listener_rule" "cloudfront" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 1

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.this.arn
  }

  condition {
    http_header {
      http_header_name = var.origin_secret_header
      values           = var.origin_secret_previous == null ? [var.origin_secret] : [var.origin_secret, var.origin_secret_previous]
    }
  }

  lifecycle {
    enabled = var.behind_cloudfront
  }
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.this.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"

    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}

# Alias records answer with the load balancer's addresses, so no CNAME is involved and an apex is fine.
resource "aws_route53_record" "alias" {
  for_each = var.route53_zone_id == null ? toset([]) : toset(var.ip_address_type == "dualstack" ? ["A", "AAAA"] : ["A"])

  zone_id = var.route53_zone_id
  name    = var.host
  type    = each.value

  alias {
    name                   = aws_lb.this.dns_name
    zone_id                = aws_lb.this.zone_id
    evaluate_target_health = false
  }
}

output "platform" {
  description = "The platform of this module: it works only with an app of AWS."
  value       = "aws"
}

output "env" {
  description = "The header that the load balancer writes."
  value = {
    MY_TRUSTED_PROXY_HEADER = "x-forwarded-for"
  }
}

output "secret_env" {
  description = "Nothing: the security group of the tasks closes the origin, and a load balancer cannot add a header to a request anyway. Read the README."
  value       = {}
}

output "permissions" {
  description = "Nothing."
  value       = []
}

output "trusted_proxies" {
  description = "What connects to the tasks: the load balancer's nodes, from the ranges of its subnets, and behind CloudFront what connects to the load balancer."
  value       = tolist(concat(var.network.public_subnet_cidrs, local.cloudfront_ranges))
}

output "service_ingress" {
  description = "load-balancer: traffic reaches the service only through the load balancer it is attached to."
  value       = "load-balancer"
}

output "load_balancer" {
  description = "What the service attaches to. After the listener, since ECS refuses a target group that no listener uses."
  value = {
    target_group_arn  = aws_lb_target_group.this.arn
    security_group_id = aws_security_group.load_balancer.id
  }
  depends_on = [aws_lb_listener.https, aws_lb_listener.http, aws_lb_listener_rule.cloudfront]
}

output "cloudfront_origin" {
  description = "What ingress-aws-cloudfront takes as origin, or null without behind_cloudfront."
  value = var.behind_cloudfront ? {
    domain_name          = aws_lb.this.dns_name
    origin_secret_header = var.origin_secret_header
    idle_timeout         = var.idle_timeout
  } : null
}

output "origin_secret" {
  description = "The value that ingress-aws-cloudfront sends, so that it is given once."
  value       = var.origin_secret
  sensitive   = true
}

output "dns_records" {
  description = "What to add at the DNS provider: nothing with route53_zone_id or behind CloudFront, a CNAME for a subdomain, and for an apex an ALIAS or ANAME, the name depending on the provider, since DNS allows no CNAME there."
  value = var.route53_zone_id != null || var.behind_cloudfront ? [] : [
    { name = var.host, type = var.host_is_apex ? "ALIAS" : "CNAME", rrdata = aws_lb.this.dns_name },
  ]
}

output "dns_note" {
  description = "What dns_records cannot say, or null."
  value       = var.route53_zone_id == null && var.host_is_apex && !var.behind_cloudfront ? "A plain CNAME cannot be used at a zone apex. Create an ALIAS or ANAME record, or a flattened CNAME, at your DNS provider. Not every provider has one: without one, move the zone to a provider that does, such as Route 53 with route53_zone_id." : null
}

output "domain_name" {
  description = "Domain name of the load balancer, for records made elsewhere."
  value       = aws_lb.this.dns_name
}

output "hosted_zone_id" {
  description = "Route 53 hosted zone of the load balancer, for an alias record made elsewhere."
  value       = aws_lb.this.zone_id
}

output "arn_suffix" {
  description = "The load balancer's dimension in CloudWatch, for the HTTP alarms of monitoring-aws."
  value       = aws_lb.this.arn_suffix
}
