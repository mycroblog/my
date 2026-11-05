# SPDX-License-Identifier: AGPL-3.0-only

# Traffic reaches the app through an Application Load Balancer with TLS. The security group of the tasks admits only this load
# balancer, so the origin is closed on the platform and needs no secret header. This module has never been applied: read the README.

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

variable "vpc_id" {
  description = "VPC of the load balancer and the tasks."
  type        = string
  nullable    = false
}

variable "subnet_ids" {
  description = "Public subnets of the load balancer, each with a free /27 at least, in two Availability Zones or more. A task in a zone that none of these covers gets no traffic."
  type        = list(string)
  nullable    = false

  validation {
    condition     = length(var.subnet_ids) >= 2
    error_message = "An Application Load Balancer needs subnets in at least two Availability Zones."
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
  ingress_rules = {
    for pair in setproduct(keys(local.listener_ports), keys(local.open_to)) : "${pair[0]}-${pair[1]}" => {
      port      = local.listener_ports[pair[0]]
      cidr_ipv4 = local.open_to[pair[1]].cidr_ipv4
      cidr_ipv6 = local.open_to[pair[1]].cidr_ipv6
    }
  }
}

data "aws_region" "current" {}

data "aws_vpc" "this" {
  id = var.vpc_id
}

# By position rather than by ID: an ID may be unknown until the apply that creates the subnet.
data "aws_subnet" "load_balancer" {
  count = length(var.subnet_ids)

  id = var.subnet_ids[count.index]
}

resource "aws_security_group" "load_balancer" {
  name        = "${var.name}-alb"
  description = "Load balancer of ${var.host}"
  vpc_id      = var.vpc_id
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

# The tasks are found by address. The VPC's primary range, since the tasks' subnets are the app's input.
resource "aws_vpc_security_group_egress_rule" "to_tasks" {
  security_group_id = aws_security_group.load_balancer.id
  description       = "To the tasks"
  ip_protocol       = "tcp"
  from_port         = local.target_port
  to_port           = local.target_port
  cidr_ipv4         = data.aws_vpc.this.cidr_block
}

resource "aws_lb" "this" {
  name               = var.name
  load_balancer_type = "application"
  internal           = false
  ip_address_type    = var.ip_address_type
  subnets            = var.subnet_ids
  security_groups    = [aws_security_group.load_balancer.id]
  idle_timeout       = var.idle_timeout

  # append and no port are what the server reads: the rightmost address that is not a proxy is the client's, with no port on it.
  xff_header_processing_mode = "append"
  enable_xff_client_port     = false
  drop_invalid_header_fields = true
}

resource "aws_lb_target_group" "this" {
  name                 = var.name
  vpc_id               = var.vpc_id
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

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.this.arn
  }

  lifecycle {
    precondition {
      condition     = strcontains(var.certificate_arn, ":acm:${data.aws_region.current.region}:")
      error_message = "certificate_arn must be in ${data.aws_region.current.region}, the region of the load balancer."
    }
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
  description = "What connects to the tasks: the load balancer's nodes, from the ranges of its subnets."
  value       = [for subnet in data.aws_subnet.load_balancer : subnet.cidr_block]
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
  depends_on = [aws_lb_listener.https, aws_lb_listener.http]
}

output "dns_records" {
  description = "What to add at the DNS provider: nothing with route53_zone_id, a CNAME for a subdomain, and for an apex an ALIAS or ANAME, the name depending on the provider, since DNS allows no CNAME there."
  value = var.route53_zone_id != null ? [] : [
    { name = var.host, type = var.host_is_apex ? "ALIAS" : "CNAME", rrdata = aws_lb.this.dns_name },
  ]
}

output "dns_note" {
  description = "What dns_records cannot say, or null."
  value       = var.route53_zone_id == null && var.host_is_apex ? "A plain CNAME cannot be used at a zone apex. Create an ALIAS or ANAME record, or a flattened CNAME, at your DNS provider. Not every provider has one: without one, move the zone to a provider that does, such as Route 53 with route53_zone_id." : null
}

output "domain_name" {
  description = "Domain name of the load balancer, for records made elsewhere."
  value       = aws_lb.this.dns_name
}

output "hosted_zone_id" {
  description = "Route 53 hosted zone of the load balancer, for an alias record made elsewhere."
  value       = aws_lb.this.zone_id
}
