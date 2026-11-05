# SPDX-License-Identifier: AGPL-3.0-only

variables {
  from               = "noreply@social.example"
  host               = "smtp.example.org"
  username           = "apikey"
  password_secret_id = "my-smtp-password"
}

run "carries_every_setting_and_the_password_by_id" {
  command = plan

  assert {
    condition = tomap(output.env) == tomap({
      MY_MAIL               = "smtp"
      MY_MAIL_FROM          = "noreply@social.example"
      MY_MAIL_SMTP_HOST     = "smtp.example.org"
      MY_MAIL_SMTP_PORT     = "587"
      MY_MAIL_SMTP_USERNAME = "apikey"
    })
    error_message = "env does not carry the settings of the mailer"
  }

  assert {
    condition     = tomap(output.secret_env) == tomap({ MY_MAIL_SMTP_PASSWORD = "my-smtp-password" })
    error_message = "the password must arrive as a secret ID"
  }

  assert {
    condition     = length(output.permissions) == 0
    error_message = "the mailer needs no permission"
  }
}

run "a_server_without_authentication" {
  command = plan

  variables {
    username           = null
    password_secret_id = null
    port               = 2525
  }

  assert {
    condition     = !contains(keys(output.env), "MY_MAIL_SMTP_USERNAME") && length(output.secret_env) == 0 && output.env["MY_MAIL_SMTP_PORT"] == "2525"
    error_message = "no authentication means no username and no password"
  }
}

run "a_username_without_a_password_fails" {
  command = plan

  variables {
    password_secret_id = null
  }

  expect_failures = [var.password_secret_id]
}

run "a_named_sender_fails" {
  command = plan

  variables {
    from = "My <noreply@social.example>"
  }

  expect_failures = [var.from]
}
