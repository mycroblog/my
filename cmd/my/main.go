// SPDX-License-Identifier: AGPL-3.0-only

// Command my is the single server binary. It dispatches to subcommands; `my version` prints the build version.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	// Embed the IANA time zone database so that time zone preferences validate and convert the
	// same on every host, including images without /usr/share/zoneinfo (about 450 KB).
	_ "time/tzdata"
)

// version is the release version, set at build time with
// -ldflags "-X main.version=<version>". When it is empty, the module version
// recorded by the Go toolchain is used, falling back to "dev".
var version string

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the subcommand named by args[0] and returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("my", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { usage(stderr) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	switch cmd := fs.Arg(0); cmd {
	case "version":
		vfs := flag.NewFlagSet("my version", flag.ContinueOnError)
		vfs.SetOutput(stderr)
		if err := vfs.Parse(fs.Args()[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 2
		}
		if vfs.NArg() > 0 {
			_, _ = fmt.Fprintf(stderr, "my version: unexpected argument %q\n", vfs.Arg(0))
			return 2
		}
		_, _ = fmt.Fprintf(stdout, "my %s\n", resolveVersion(version, debug.ReadBuildInfo))
		return 0
	case "config":
		return runConfig(fs.Args()[1:], stdout, stderr)
	case "admin":
		return runAdmin(fs.Args()[1:], os.Stdin, stdout, stderr)
	case "db":
		return runDB(fs.Args()[1:], stdout, stderr)
	case "instance":
		return runInstance(fs.Args()[1:], os.Stdin, stdout, stderr)
	case "invite":
		return runInvite(fs.Args()[1:], stdout, stderr)
	case "job":
		return runJob(fs.Args()[1:], stdout, stderr)
	case "login-provider":
		return runLoginProvider(fs.Args()[1:], os.Stdin, stdout, stderr)
	case "maintenance":
		return runMaintenance(fs.Args()[1:], stdout, stderr)
	case "projection":
		return runProjection(fs.Args()[1:], stdout, stderr)
	case "registration":
		return runRegistration(fs.Args()[1:], stdout, stderr)
	case "serve":
		return runServe(fs.Args()[1:], stderr)
	case "":
		usage(stderr)
		return 2
	default:
		_, _ = fmt.Fprintf(stderr, "my: unknown command %q\n\n", cmd)
		usage(stderr)
		return 2
	}
}

func usage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: my <command>

Commands:
  admin     Create an administrator, moderate domains, handle reports,
            manage roles, and block sign-ups
            (my admin create|account|domain|report|audit|role|signup-block)
  config    Check the configuration and generate the Web Push key
            (my config check|vapid-key)
  db        Run the data upgrade steps of a release upgrade, or check
            before applying its schema (my db upgrade [-check])
  instance  Read and change the instance settings and rules
            (my instance list|get|set|history|rule)
  invite    Manage invite codes (my invite create|list|revoke)
  job       List, retry, and delete the dead jobs (my job list|retry|delete)
  login-provider
            Set up the external providers that users sign in with
            (my login-provider list|add|set|rotate-secret|enable|disable|delete)
  maintenance
            Write the maintenance page as static HTML (my maintenance export)
  projection
            Rebuild and verify the projections
            (my projection list|rebuild|verify)
  registration
            Review sign-ups (my registration list|approve|reject)
  serve     Run the server (role web, worker, or all)
  version   Print the version
`)
}

// resolveVersion returns the injected version, else the module version from
// the build info (set by `go install module@version`), else "dev".
func resolveVersion(injected string, readBuildInfo func() (*debug.BuildInfo, bool)) string {
	if injected != "" {
		return injected
	}
	if info, ok := readBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
