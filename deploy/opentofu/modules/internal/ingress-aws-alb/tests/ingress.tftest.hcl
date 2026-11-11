# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches AWS.

mock_provider "aws" {
  mock_data "aws_region" {
    defaults = {
      region = "ap-northeast-1"
    }
  }
  mock_data "aws_ec2_managed_prefix_list" {
    defaults = {
      id = "pl-cloudfront"
      entries = [
        { cidr = "15.158.0.0/16", description = "" },
        { cidr = "130.176.0.0/18", description = "" },
      ]
    }
  }
  mock_resource "aws_lb" {
    defaults = {
      arn      = "arn:aws:elasticloadbalancing:ap-northeast-1:111122223333:loadbalancer/app/my/abc"
      dns_name = "my-123.ap-northeast-1.elb.amazonaws.com"
      zone_id  = "Z14GRHDCWA56QT"
    }
  }
  mock_resource "aws_lb_listener" {
    defaults = {
      arn = "arn:aws:elasticloadbalancing:ap-northeast-1:111122223333:listener/app/my/abc/def"
    }
  }
  mock_resource "aws_lb_target_group" {
    defaults = {
      arn = "arn:aws:elasticloadbalancing:ap-northeast-1:111122223333:targetgroup/my/abc"
    }
  }
}

variables {
  host = "social.example.org"
  network = {
    platform            = "aws"
    vpc_id              = "vpc-1"
    vpc_cidr            = "10.0.0.0/16"
    public_subnet_ids   = ["subnet-1", "subnet-2"]
    public_subnet_cidrs = ["10.0.0.0/24", "10.0.1.0/24"]
    private_subnet_ids  = ["subnet-a", "subnet-b"]
    nat                 = true
  }
  certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc"
}

run "it_terminates_tls_and_forwards_to_the_tasks" {
  command = plan

  assert {
    condition     = aws_lb_listener.https.protocol == "HTTPS" && aws_lb_listener.https.ssl_policy == "ELBSecurityPolicy-TLS13-1-2-Res-PQ-2025-09" && aws_lb_listener.https.default_action[0].type == "forward"
    error_message = "the listener must serve TLS with the policy AWS recommends, not the older default of the API"
  }

  assert {
    condition     = aws_lb_listener.http.default_action[0].redirect[0].status_code == "HTTP_301" && aws_lb_listener.http.default_action[0].redirect[0].protocol == "HTTPS"
    error_message = "port 80 only redirects"
  }

  assert {
    condition     = aws_lb_target_group.this.target_type == "ip" && aws_lb_target_group.this.port == 8080 && aws_lb_target_group.this.health_check[0].path == "/readyz"
    error_message = "a Fargate task registers by address, on the server's port, and is checked on readiness"
  }

  assert {
    condition     = aws_lb_target_group.this.health_check[0].timeout > 3
    error_message = "the check must outlast the server's own 3 seconds on the database"
  }
}

run "it_writes_what_the_server_trusts" {
  command = apply

  assert {
    condition     = aws_lb.this.xff_header_processing_mode == "append" && !aws_lb.this.enable_xff_client_port
    error_message = "the server reads the rightmost address that is not a proxy, with no port on it"
  }

  assert {
    condition     = output.trusted_proxies == tolist(["10.0.0.0/24", "10.0.1.0/24"]) && output.env["MY_TRUSTED_PROXY_HEADER"] == "x-forwarded-for"
    error_message = "the ranges of the load balancer's subnets are what connects to the tasks"
  }

  assert {
    condition     = output.service_ingress == "load-balancer"
    error_message = "it names what an app must attach to, which no ingress of another kind does"
  }

  assert {
    condition     = output.load_balancer.target_group_arn == aws_lb_target_group.this.arn && output.load_balancer.security_group_id == aws_security_group.load_balancer.id
    error_message = "the app needs both to attach and to admit it"
  }
}

# The decision of this module: the security group of the tasks closes the origin, so nothing secret exists here.
run "it_has_no_origin_secret" {
  command = plan

  assert {
    condition     = output.secret_env == {} && !contains(keys(output.env), "MY_ORIGIN_SECRET_HEADER")
    error_message = "the security group closes the origin, and a load balancer cannot add a header anyway"
  }

  assert {
    condition     = output.permissions == []
    error_message = "the app needs no permission for it"
  }
}

run "only_the_listeners_are_open" {
  command = plan

  assert {
    condition     = toset(keys(aws_vpc_security_group_ingress_rule.internet)) == toset(["http-ipv4", "https-ipv4"])
    error_message = "ipv4 only, unless dualstack"
  }

  assert {
    condition     = aws_vpc_security_group_egress_rule.to_tasks.from_port == 8080 && aws_vpc_security_group_egress_rule.to_tasks.to_port == 8080
    error_message = "the load balancer reaches only the server's port"
  }
}

run "dualstack_opens_ipv6" {
  command = plan

  variables {
    ip_address_type = "dualstack"
    route53_zone_id = "Z0123456789"
  }

  assert {
    condition     = length(aws_vpc_security_group_ingress_rule.internet) == 4 && aws_vpc_security_group_ingress_rule.internet["https-ipv6"].cidr_ipv6 == "::/0"
    error_message = "IPv6 must be open on both listeners"
  }

  assert {
    condition     = toset(keys(aws_route53_record.alias)) == toset(["A", "AAAA"])
    error_message = "both families need a record"
  }
}

run "a_zone_creates_the_record" {
  command = plan

  variables {
    route53_zone_id = "Z0123456789"
  }

  assert {
    condition     = toset(keys(aws_route53_record.alias)) == toset(["A"]) && length(output.dns_records) == 0 && output.dns_note == null
    error_message = "with a zone the module creates the records, and asks for nothing"
  }
}

run "without_a_zone_it_asks_for_a_cname" {
  command = apply

  assert {
    condition     = output.dns_records[0].type == "CNAME" && output.dns_records[0].rrdata == "my-123.ap-northeast-1.elb.amazonaws.com" && output.dns_note == null
    error_message = "a subdomain takes a CNAME"
  }
}

run "an_apex_without_a_zone_says_so" {
  command = apply

  variables {
    host_is_apex = true
  }

  assert {
    condition     = output.dns_records[0].type == "ALIAS" && output.dns_note != null
    error_message = "an apex allows no CNAME"
  }
}

run "one_subnet_fails" {
  command = plan

  variables {
    network = {
      platform            = "aws"
      vpc_id              = "vpc-1"
      vpc_cidr            = "10.0.0.0/16"
      public_subnet_ids   = ["subnet-1"]
      public_subnet_cidrs = ["10.0.0.0/24"]
    }
  }

  expect_failures = [var.network]
}

run "a_certificate_of_another_region_fails" {
  command = plan

  variables {
    certificate_arn = "arn:aws:acm:us-east-1:111122223333:certificate/abc"
  }

  expect_failures = [aws_lb_listener.https]
}

run "an_idle_timeout_beyond_the_servers_fails" {
  command = plan

  variables {
    idle_timeout = 120
  }

  expect_failures = [var.idle_timeout]
}

# Behind CloudFront: the load balancer is the origin that must stay closed to all but CloudFront.
run "behind_cloudfront_admits_cloudfront_alone" {
  command = plan

  variables {
    behind_cloudfront = true
    origin_secret     = "0123456789abcdef0123456789abcdef"
  }

  assert {
    condition     = length(aws_vpc_security_group_ingress_rule.internet) == 0 && aws_vpc_security_group_ingress_rule.cloudfront.prefix_list_id == "pl-cloudfront" && aws_vpc_security_group_ingress_rule.cloudfront.from_port == 443
    error_message = "nothing but CloudFront's origin-facing addresses may connect, and only on 443"
  }

  assert {
    condition     = aws_lb_listener.https.default_action[0].type == "fixed-response" && aws_lb_listener.https.default_action[0].fixed_response[0].status_code == "403"
    error_message = "another distribution connects from the same addresses, so the listener must refuse what lacks the header"
  }

  assert {
    condition     = nonsensitive(one(one(aws_lb_listener_rule.cloudfront.condition).http_header).http_header_name) == "x-my-origin-secret" && aws_lb_listener_rule.cloudfront.action[0].type == "forward"
    error_message = "only a request with CloudFront's header is forwarded"
  }

  assert {
    condition     = output.trusted_proxies == tolist(["10.0.0.0/24", "10.0.1.0/24", "130.176.0.0/18", "15.158.0.0/16"])
    error_message = "CloudFront appends the viewer's address and the load balancer appends CloudFront's, so both are trusted"
  }

  assert {
    condition     = output.cloudfront_origin.domain_name == "my-123.ap-northeast-1.elb.amazonaws.com" && output.cloudfront_origin.origin_secret_header == "x-my-origin-secret" && output.cloudfront_origin.idle_timeout == 60
    error_message = "ingress-aws-cloudfront takes the load balancer from here"
  }

  assert {
    condition     = length(output.dns_records) == 0 && output.dns_note == null && length(aws_route53_record.alias) == 0
    error_message = "host points at CloudFront, not at the load balancer"
  }
}

run "behind_cloudfront_a_rotation_accepts_both_values" {
  command = plan

  variables {
    behind_cloudfront      = true
    origin_secret          = "0123456789abcdef0123456789abcdef"
    origin_secret_previous = "fedcba9876543210fedcba9876543210"
  }

  assert {
    condition     = length(nonsensitive(one(one(aws_lb_listener_rule.cloudfront.condition).http_header).values)) == 2
    error_message = "while the value rotates, the listener accepts the old and the new"
  }
}

run "the_internet_mode_has_no_listener_rule" {
  command = plan

  assert {
    condition     = aws_lb_listener_rule.cloudfront == null && output.cloudfront_origin == null
    error_message = "without CloudFront the listener forwards everything, and the security group of the tasks closes the origin"
  }
}

run "behind_cloudfront_without_a_secret_fails" {
  command = plan

  variables {
    behind_cloudfront = true
  }

  expect_failures = [var.origin_secret]
}

run "a_secret_with_a_wildcard_fails" {
  command = plan

  variables {
    behind_cloudfront = true
    origin_secret     = "0123456789abcdef0123456789abcde*"
  }

  expect_failures = [var.origin_secret]
}

run "behind_cloudfront_a_zone_fails" {
  command = plan

  variables {
    behind_cloudfront = true
    origin_secret     = "0123456789abcdef0123456789abcdef"
    route53_zone_id   = "Z0123456789"
  }

  expect_failures = [var.route53_zone_id]
}

run "behind_cloudfront_dualstack_fails" {
  command = plan

  variables {
    behind_cloudfront = true
    origin_secret     = "0123456789abcdef0123456789abcdef"
    ip_address_type   = "dualstack"
  }

  expect_failures = [var.ip_address_type]
}

run "a_network_of_another_platform_fails" {
  command = plan

  variables {
    network = { platform = "gcp", network = "projects/p/global/networks/n", subnetwork = "projects/p/regions/r/subnetworks/s" }
  }

  expect_failures = [var.network]
}
