// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// exampleEnvPath is the template of the env file of the Compose deployment, relative to the repository root.
const exampleEnvPath = "deploy/docker/example.env"

const exampleEnvHeader = `# SPDX-License-Identifier: AGPL-3.0-only
#
# The template of the settings of the Docker Compose deployment. Copy it to .env (which Git ignores) in this directory:
#
#   cp example.env .env
#   mkdir -m 700 secrets
#   (umask 022; openssl rand -base64 32 > secrets/kek)
#
# Back up secrets/kek apart from the database: without it the data cannot be decrypted.
#
# Compose reads .env twice: to fill in the ${...} of the compose files, and, through env_file, to hand every line to the
# containers of the server as environment variables. So every setting below works by being set here, and:
#
# - The key-encryption key is not in this file. A line MY_KEK=... would reach the containers as an environment variable,
#   so the key lives in the file secrets/kek and reaches them as the Docker secret /run/secrets/my_kek (MY_KEK_FILE). The
#   server runs as the user 65532, so the file must be readable by others: the directory's mode 700 protects it. If this
#   file sets MY_KEK anyway, the server refuses to start, since it takes only one of the two.
# - Other secrets (the database URL, API keys) are passed as environment variables. To keep one out of them too, mount a
#   file and set its NAME_FILE in compose.override.yaml, as compose.yaml does for the KEK. Keep this file private:
#   chmod 600 .env.
# - Lines are taken literally except for $: write a value with a $ in single quotes.
# - The lines of a setting that is commented out show its default where it has one. Uncomment what you change.
#
# The settings are described in full in docs/reference/configuration.md.

# ------------------------------------------------------------------------------------------------------------------
# Compose itself. The server does not read these.
# ------------------------------------------------------------------------------------------------------------------

# The version of the image: 0.1.0 (for good), 0.1 (the latest patch of v0.1, on the next docker compose pull), or
# 0.1.0@sha256:... (the digest in the release's notes). There is no latest.
MY_VERSION=

# The port of the server on the loopback address of this machine.
# MY_HTTP_PORT=8080

# The layers, so that every docker compose command takes them without -f options (: is ; on Windows). Any of
# compose.postgres.yaml, compose.s3.yaml, and compose.caddy.yaml; see the header of each.
# COMPOSE_FILE=compose.yaml:compose.postgres.yaml:compose.s3.yaml:compose.caddy.yaml

# compose.postgres.yaml: the password of the database, letters and digits only (openssl rand -hex 32).
# POSTGRES_PASSWORD=

# compose.s3.yaml with the bundled Garage: the profile, and two secrets of Garage (each from openssl rand -hex 32). Under
# "Media storage" below, also set these settings:
#
#   MY_BLOB_S3_ENDPOINT=http://garage:3900
#   MY_BLOB_S3_BUCKET=my-media
#   MY_BLOB_S3_PATH_STYLE=true
#   MY_BLOB_S3_ACCESS_KEY_ID=      the output of: echo "GK$(openssl rand -hex 12)"
#   MY_BLOB_S3_SECRET_ACCESS_KEY=  the output of: openssl rand -hex 32
#
# COMPOSE_PROFILES=garage
# GARAGE_RPC_SECRET=
# GARAGE_ADMIN_TOKEN=
`

// exampleEnvSections names the group of settings that starts at each key, in the order of the schema.
var exampleEnvSections = map[string]string{
	"host":                          "The instance",
	"maintenance":                   "Maintenance mode",
	"web_origin":                    "Origins",
	"role":                          "Processes",
	"listen":                        "HTTP server",
	"database":                      "Database",
	"queue":                         "Job queue",
	"blob":                          "Media storage",
	"media_origin":                  "Media",
	"kv":                            "Key-value store",
	"stream":                        "Real-time updates",
	"key_manager":                   "Encryption keys",
	"mail":                          "Email",
	"captcha":                       "Captcha",
	"vapid_private_key":             "Web Push",
	"profile":                       "Profile and outbound destinations",
	"relay_max_activities_per_hour": "Relays",
	"billing_sources":               "Cost records",
	"payments":                      "Payments",
	"voice":                         "Voice chat",
	"plugin_translator_address":     "Plugins",
	"dead_job_retention":            "Retention",
	"metrics_exporter":              "Metrics, traces, and error reports",
}

// exampleEnvValues are the settings that the template sets or whose default the deployment replaces.
var exampleEnvValues = map[string]string{
	"host":         "MY_HOST=localhost:8080",
	"web_origin":   "MY_WEB_ORIGIN=http://localhost:8080",
	"database_url": "# MY_DATABASE_URL=/data/my.db",
	"blob_fs_dir":  "# MY_BLOB_FS_DIR=/data/blobs",
}

// firstSentence ends at the first period that a new sentence follows.
var firstSentence = regexp.MustCompile(`^.*?[.:](\s|$)`)

// exampleEnv returns the template from the schema, as Reference does the reference: only the first sentence of each
// description, since the reference has the rest.
func exampleEnv() string {
	var b strings.Builder
	b.WriteString(exampleEnvHeader)
	for i := range schema {
		f := &schema[i]
		if section, ok := exampleEnvSections[f.key]; ok {
			fmt.Fprintf(&b, "\n# %s\n# %s\n# %s\n", strings.Repeat("-", 114), section, strings.Repeat("-", 114))
		}
		b.WriteString("\n")
		for _, line := range wrapComment(exampleEnvNote(f)) {
			b.WriteString(line + "\n")
		}
		if f.key == "kek" {
			continue
		}
		line, ok := exampleEnvValues[f.key]
		if !ok {
			line = "# " + f.env() + "=" + f.def
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// exampleEnvNote is the comment of one setting, on one line.
func exampleEnvNote(f *field) string {
	if f.key == "kek" {
		return "MY_KEK is not set here: the file secrets/kek holds the key-encryption key (see the top of this file)."
	}
	note := strings.TrimRight(strings.TrimSpace(firstSentence.FindString(f.doc)), ":")
	if !strings.HasSuffix(note, ".") {
		note += "."
	}
	if len(f.choices) > 0 {
		note += " Values: " + strings.Join(allowedValues(f), ", ") + "."
	}
	if f.def == "" && f.defDoc != "" {
		note += " Default: " + f.defDoc + "."
	}
	if r := requiredCell(f); r != "no" {
		note += " Required: " + r + "."
	}
	if f.secret {
		note += " A secret."
	}
	return note
}

// wrapComment breaks a note into comment lines of at most 118 characters.
func wrapComment(note string) []string {
	var lines []string
	line := "#"
	for _, word := range strings.Fields(note) {
		if len(line)+1+len(word) > 118 && line != "#" {
			lines = append(lines, line)
			line = "#"
		}
		line += " " + word
	}
	return append(lines, line)
}

// TestExampleEnvUpToDate fails when the committed template differs from the schema.
// Regenerate it with `scripts/task.sh go:config-docs`.
func TestExampleEnvUpToDate(t *testing.T) {
	path := filepath.Join("..", "..", "..", filepath.FromSlash(exampleEnvPath))
	want := exampleEnv()
	if *update {
		if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path) //nolint:gosec // fixed path inside the repository
	if err != nil {
		t.Fatalf("%v; run `scripts/task.sh go:config-docs`", err)
	}
	if string(got) != want {
		t.Errorf("%s is stale; run `scripts/task.sh go:config-docs`", exampleEnvPath)
	}
}

// TestExampleEnvCoversEverySetting checks what the deployment depends on: each setting has its line, except the KEK,
// which no line may set since it would reach the containers as an environment variable.
func TestExampleEnvCoversEverySetting(t *testing.T) {
	text := exampleEnv()
	for i := range schema {
		env := schema[i].env()
		line := regexp.MustCompile(`(?m)^(# )?` + env + `=`)
		if env == EnvPrefix+"KEK" {
			if line.MatchString(text) {
				t.Errorf("the template sets %s", env)
			}
			continue
		}
		if !line.MatchString(text) {
			t.Errorf("the template has no line for %s", env)
		}
	}
	for i, line := range strings.Split(text, "\n") {
		if m := designCitation.FindString(line); m != "" {
			t.Errorf("%s:%d cites %q", exampleEnvPath, i+1, m)
		}
	}
}
