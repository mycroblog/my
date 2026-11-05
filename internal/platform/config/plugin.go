// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/internal/domain/identity"
)

// pluginsLeftOutTag is the build tag that left the plugins out of this binary, "" in a build that has them.
var pluginsLeftOutTag string

func init() { schema = append(schema, pluginFields()...) }

// pluginFields returns the settings of the destination of every delegated kind (ADR-0146 §5). They exist in the
// configuration file and the environment only, which is what keeps the destination out of the admin UI and the client
// API: an administrator's stolen account cannot change where posts and keys go.
func pluginFields() []field {
	var fields []field
	for _, k := range kind.Delegated() {
		fields = append(fields, pluginFieldsOf(k)...)
		if k.Name == kind.Billing {
			fields = append(fields, pluginBillingFields()...)
		}
		if k.Name == kind.Translator {
			fields = append(fields, pluginTranslatorName())
		}
	}
	return fields
}

// pluginBillingFields returns what the billing source of a plugin says with no call, so that the start depends on none
// (ADR-0146 §11).
func pluginBillingFields() []field {
	return []field{
		{
			key:          "plugin_billing_location",
			doc:          "IANA time zone that cuts the billing months of `billing_sources = plugin`, such as `UTC` or `Asia/Tokyo`. The imports ask the plugin for the months of this zone.",
			requiredWhen: "when `billing_sources` includes `plugin`",
			check:        identity.ValidateTimeZone,
			set:          func(c *Config, v string) { c.PluginBillingLocation = v },
		},
		{
			key:          "plugin_billing_provider",
			doc:          "The provider's name that the totals, the costs page, and the audit log show for `billing_sources = plugin`, such as `Hetzner`, at most 100 characters. All the plugin's costs are under this one name, and the services of its lines tell them apart.",
			requiredWhen: "when `billing_sources` includes `plugin`",
			check:        finance.ValidateProvider,
			set:          func(c *Config, v string) { c.PluginBillingProvider = strings.TrimSpace(v) },
		},
	}
}

// maxPluginTranslatorName bounds plugin_translator_name, which a client shows next to each translation.
const maxPluginTranslatorName = 100

// pluginTranslatorName returns the setting of the name that readers see on a translation by the plugin. The server cannot
// tell what the plugin calls, so only the operator can name it (ADR-0148 §14).
func pluginTranslatorName() field {
	return field{
		key: "plugin_translator_name",
		doc: "The name that readers see next to a translation by the `translator` plugin and that the admin screen lists, such as the service behind your plugin, at most " +
			strconv.Itoa(maxPluginTranslatorName) + " characters. Clients get this name and never the address.",
		requiredWhen: "when `plugin_translator_address` is set",
		check: func(v string) error {
			name := strings.TrimSpace(v)
			if name == "" || utf8.RuneCountInString(name) > maxPluginTranslatorName || !utf8.ValidString(name) || strings.ContainsFunc(name, unicode.IsControl) {
				return fmt.Errorf("must be one line of 1 to %d characters", maxPluginTranslatorName)
			}
			return nil
		},
		set: func(c *Config, v string) { c.PluginTranslatorName = strings.TrimSpace(v) },
	}
}

// pluginAddressRequiredWhen documents the kinds whose address another setting requires, which pluginChecks checks.
var pluginAddressRequiredWhen = map[string]string{
	kind.Billing: "when `billing_sources` includes `plugin`",
	kind.Mailer:  "when `mail = plugin`",
}

func pluginFieldsOf(k kind.Kind) []field {
	store := func(c *Config, change func(*kind.Destination)) {
		if c.Plugins == nil {
			c.Plugins = map[string]kind.Destination{}
		}
		d := c.Plugins[k.Name]
		change(&d)
		c.Plugins[k.Name] = d
	}
	forms := "`unix:///path/to.sock`, `https://host[:port]`, or `http://host[:port]`, which is plain TCP: nothing encrypts or authenticates it, and the server warns at start"
	if !k.PlainTCP {
		forms = "`unix:///path/to.sock` or `https://host[:port]`; `http://` is refused, on loopback too"
	}
	protocols := []choice{
		{value: kind.ProtocolJSON, doc: "The Connect protocol with JSON, which a plugin of an HTTP server and a JSON library serves with no Connect library."},
		{value: kind.ProtocolProto, doc: "The Connect protocol with binary protobuf, for a server on a Connect library."},
		{value: kind.ProtocolGRPC, doc: "gRPC, for a server that speaks only that."},
	}
	if k.Streaming {
		protocols = protocols[1:]
	}
	return []field{
		{
			key: k.Key(kind.SuffixAddress),
			doc: "Where the `" + k.Name + "` plugin listens: " + forms +
				". Empty delegates nothing, and the server opens no connection. Host names are resolved per connection. Restart to change it; no instance setting or API can.",
			requiredWhen: pluginAddressRequiredWhen[k.Name],
			check: func(v string) error {
				_, err := kind.ParseAddress(v)
				return err
			},
			set: func(c *Config, v string) { store(c, func(d *kind.Destination) { d.Address = v }) },
		},
		{
			key:     k.Key(kind.SuffixProtocol),
			doc:     "What the server sends to the `" + k.Name + "` plugin; it must be what the plugin accepts.",
			defDoc:  "`" + k.Protocol + "`",
			choices: protocols,
			set:     func(c *Config, v string) { store(c, func(d *kind.Destination) { d.Protocol = v }) },
		},
		{
			key:    k.Key(kind.SuffixToken),
			doc:    "Bearer token that the server sends to the `" + k.Name + "` plugin, over `https://` or `unix://` only.",
			secret: true,
			check:  checkNotBlank,
			set:    func(c *Config, v string) { store(c, func(d *kind.Destination) { d.Token = v }) },
		},
		{
			key:    k.Key(kind.SuffixTLSCAFile),
			doc:    "File of the CA that verifies the `" + k.Name + "` plugin at an `https://` address.",
			defDoc: "the system's roots",
			set:    func(c *Config, v string) { store(c, func(d *kind.Destination) { d.TLSCAFile = v }) },
		},
		{
			key: k.Key(kind.SuffixTLSCertFile),
			doc: "File of the client certificate that the server shows the `" + k.Name + "` plugin at an `https://` address (mTLS), with its key file.",
			set: func(c *Config, v string) { store(c, func(d *kind.Destination) { d.TLSCertFile = v }) },
		},
		{
			key: k.Key(kind.SuffixTLSKeyFile),
			doc: "File of the key of that client certificate of the `" + k.Name + "` plugin.",
			set: func(c *Config, v string) { store(c, func(d *kind.Destination) { d.TLSKeyFile = v }) },
		},
		{
			key:    k.Key(kind.SuffixTimeout),
			doc:    "Deadline of one call of the `" + k.Name + "` plugin, less than `" + kind.MaxTimeout.String() + "`.",
			defDoc: "`" + k.Timeout.String() + "`",
			check: func(v string) error {
				d, err := time.ParseDuration(v)
				if err != nil || d <= 0 {
					return fmt.Errorf("must be a duration above zero, such as %s", k.Timeout)
				}
				return nil
			},
			set: func(c *Config, v string) {
				timeout, err := time.ParseDuration(v)
				if err == nil {
					store(c, func(d *kind.Destination) { d.Timeout = timeout })
				}
			},
		},
	}
}

// pluginChecks applies the rules of a destination to the settings of every kind (ADR-0146 §5, §6).
func pluginChecks(vals map[string]string, present map[string]bool) []Issue {
	var issues []Issue
	if pluginsLeftOutTag == "" && slices.Contains(splitList(vals["billing_sources"]), "plugin") {
		for _, key := range []string{"plugin_billing_address", "plugin_billing_location", "plugin_billing_provider"} {
			if !present[key] {
				issues = append(issues, Issue{Key: key, Missing: true, Message: "required when billing_sources includes plugin"})
			}
		}
	}
	if pluginsLeftOutTag == "" && vals["mail"] == "plugin" && !present["plugin_mailer_address"] {
		issues = append(issues, Issue{Key: "plugin_mailer_address", Missing: true, Message: "required when mail is plugin"})
	}
	if pluginsLeftOutTag == "" && present["plugin_translator_address"] && !present["plugin_translator_name"] {
		issues = append(issues, Issue{Key: "plugin_translator_name", Missing: true, Message: "required when plugin_translator_address is set"})
	}
	for _, k := range kind.Delegated() {
		address := k.Key(kind.SuffixAddress)
		if !present[address] {
			issues = append(issues, pluginWithoutAddress(k, present)...)
			continue
		}
		if pluginsLeftOutTag != "" {
			issues = append(issues, Issue{Key: address, Message: fmt.Sprintf("is not in this build: the build tag %s left the plugins out (docs/reference/build-tags.md)", pluginsLeftOutTag)})
			continue
		}
		if vals[address] == "" {
			continue // Its own validation reported it.
		}
		timeout, err := time.ParseDuration(vals[k.Key(kind.SuffixTimeout)])
		if err != nil {
			timeout = 0
		}
		d := kind.Destination{
			Address:     vals[address],
			Protocol:    vals[k.Key(kind.SuffixProtocol)],
			Token:       vals[k.Key(kind.SuffixToken)],
			TLSCAFile:   vals[k.Key(kind.SuffixTLSCAFile)],
			TLSCertFile: vals[k.Key(kind.SuffixTLSCertFile)],
			TLSKeyFile:  vals[k.Key(kind.SuffixTLSKeyFile)],
			Timeout:     timeout,
		}
		for _, p := range k.Check(d) {
			issues = append(issues, Issue{Key: k.Key(p.Suffix), Message: p.Message})
		}
	}
	return issues
}

// pluginWithoutAddress rejects the credentials of a kind that has no address, which are most likely a mistyped key away
// from one. The protocol and the timeout have defaults, so their presence says nothing.
func pluginWithoutAddress(k kind.Kind, present map[string]bool) []Issue {
	var issues []Issue
	for _, suffix := range []string{kind.SuffixToken, kind.SuffixTLSCAFile, kind.SuffixTLSCertFile, kind.SuffixTLSKeyFile} {
		if present[k.Key(suffix)] {
			issues = append(issues, Issue{Key: k.Key(suffix), Message: "has no use without " + k.Key(kind.SuffixAddress)})
		}
	}
	return issues
}

// PluginDestination returns the destination of the kind's plugin. Its Enabled says whether one is configured.
func (c *Config) PluginDestination(name string) kind.Destination { return c.Plugins[name] }

// PluginWarnings returns the destinations that work but that the operator should know about, for `my config check` and
// the log at start (ADR-0146 §6).
func PluginWarnings(c *Config) []Finding {
	var out []Finding
	for _, k := range kind.Delegated() {
		d := c.Plugins[k.Name]
		if !d.Enabled() {
			continue
		}
		for _, p := range k.Warnings(d) {
			out = append(out, Finding{Severity: SeverityWarning, Key: k.Key(p.Suffix), Message: p.Message})
		}
	}
	return out
}
