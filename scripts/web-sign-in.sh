# shellcheck shell=bash
# SPDX-License-Identifier: AGPL-3.0-only
#
# Sourced by scripts/docker-smoke.sh and scripts/upgrade-test.sh, which define fail(). Needs curl, jq, openssl.
#
# web_sign_in API_ORIGIN WEB_ORIGIN EMAIL PASSWORD prints an access token of the account, got without a browser as
# the web client gets one: the OAuth 2.0 authorization code flow with PKCE through the authorization endpoint, the
# sign-in form, the redirect back with a code and the issuer, and the token endpoint. Over plain HTTP on localhost,
# every step stays on http://.

# URL-safe base64 without padding.
b64url() { openssl base64 -A | tr '+/' '-_' | tr -d '='; }

# The value of a query parameter of a URL, percent-decoded.
url_param() {
	local v
	v="$(printf '%s' "${1#*\?}" | tr '&' '\n' | sed -n "s/^$2=//p" | head -n 1)"
	v="${v//+/ }"
	printf '%b' "${v//%/\\x}"
}

# A hidden field of an HTML form.
form_field() { echo "$2" | grep -o "name=\"$1\" value=\"[^\"]*\"" | head -n 1 | sed 's/.*value="//; s/"$//; s/&amp;/\&/g'; }

web_sign_in() {
	local api_origin="$1" web_origin="$2" email="$3" password="$4"
	local verifier challenge state callback jar
	verifier="$(openssl rand 32 | b64url)"
	challenge="$(printf '%s' "$verifier" | openssl dgst -sha256 -binary | b64url)"
	state="$(openssl rand 16 | b64url)"
	callback="$web_origin/login/callback"
	jar="$(mktemp)"
	# The cookie jar keeps the session and CSRF cookies: curl, like browsers, takes Secure cookies from http://localhost.
	local browser=(curl -fsS -b "$jar" -c "$jar")
	local redirect=("${browser[@]}" -o /dev/null -w '%{redirect_url}')

	local sign_in form csrf return_to authorize back code tokens access
	sign_in="$("${redirect[@]}" -G --data-urlencode response_type=code --data-urlencode client_id=my-web \
		--data-urlencode "redirect_uri=$callback" --data-urlencode "code_challenge=$challenge" \
		--data-urlencode code_challenge_method=S256 --data-urlencode "state=$state" "$api_origin/oauth/authorize")"
	[[ "$sign_in" == "$api_origin/auth/sign_in?"* ]] || fail "the authorization endpoint redirects to '$sign_in', want the sign-in page"
	form="$("${browser[@]}" "$sign_in")"
	csrf="$(form_field csrf_token "$form")"
	return_to="$(form_field return_to "$form")"
	[[ -n "$csrf" && -n "$return_to" ]] || fail "the sign-in page has no csrf_token or return_to: $form"
	authorize="$("${redirect[@]}" -H "Origin: $api_origin" --data-urlencode "csrf_token=$csrf" --data-urlencode "return_to=$return_to" \
		--data-urlencode "email=$email" --data-urlencode "password=$password" "$api_origin/auth/sign_in")"
	[[ "$authorize" == "$api_origin/oauth/authorize?"* ]] || fail "signing in redirects to '$authorize', want the authorization endpoint"
	back="$("${redirect[@]}" "$authorize")"
	[[ "$back" == "$callback?"* ]] || fail "the authorization endpoint redirects to '$back', want $callback"
	[[ "$(url_param "$back" state)" == "$state" ]] || fail "the redirect does not carry the state: $back"
	[[ "$(url_param "$back" iss)" == "$api_origin" ]] || fail "the redirect's issuer is not $api_origin: $back"
	code="$(url_param "$back" code)"
	[[ -n "$code" ]] || fail "the redirect has no code: $back"

	tokens="$("${browser[@]}" -H "Origin: $web_origin" --data-urlencode grant_type=authorization_code \
		--data-urlencode "code=$code" --data-urlencode "redirect_uri=$callback" --data-urlencode client_id=my-web \
		--data-urlencode "code_verifier=$verifier" "$api_origin/oauth/token")"
	access="$(echo "$tokens" | jq -er '.access_token')" || fail "unexpected token response: $tokens"
	grep -q '__Host-my_rt' "$jar" || fail "the token endpoint set no refresh token cookie"
	rm -f "$jar"
	printf '%s\n' "$access"
}
