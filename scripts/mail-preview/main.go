// SPDX-License-Identifier: AGPL-3.0-only

// Command mail-preview writes every email that the server sends, rendered from sample inputs in
// en, ja, zh-CN, zh-TW, es, pt-BR, de, fr, en-XA, and ar-XB, to a directory with an index.html that shows them, and prints the
// path of index.html. It needs no database, server, or mail server. `scripts/task.sh mail:preview`
// runs it:
//
//	go run ./scripts/mail-preview -out .tmp/mail/preview
//
// With -smtp, it sends them instead, light and dark, to the development Mailpit, which requires
// STARTTLS with a certificate of the CA in the file -ca, and prints how many it sent.
// `scripts/task.sh mail:mailpit` runs it, in en and the right-to-left ar-XB:
//
//	go run ./scripts/mail-preview -smtp 127.0.0.1:1025 -ca .tmp/mail/mailpit-ca.pem -locales en,ar-XB
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mycroblog/my/internal/adapter/mail/smtp"
	"github.com/mycroblog/my/internal/platform/mailpreview"
)

func main() {
	out := flag.String("out", ".tmp/mail/preview", "the directory to write the preview to (replaced)")
	server := flag.String("smtp", "", "send the emails to this SMTP server (host:port) instead of writing them")
	ca := flag.String("ca", "", "with -smtp, the PEM file of the CA that the server's certificate is of")
	locales := flag.String("locales", strings.Join(mailpreview.Locales, ","), "the locales, separated by commas")
	flag.Parse()
	list := strings.Split(*locales, ",")
	if *server != "" {
		n, err := send(*server, *ca, list)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("sent %d emails to %s\n", n, *server)
		return
	}
	index, err := mailpreview.Write(*out, list)
	if err != nil {
		log.Fatal(err)
	}
	if abs, err := filepath.Abs(index); err == nil {
		index = abs
	}
	fmt.Println(index)
}

// send sends every email in locales to the SMTP server at addr, which it verifies against the CA
// of the file ca.
func send(addr, ca string, locales []string) (int, error) {
	host, p, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, err
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return 0, fmt.Errorf("the port of %s: %w", addr, err)
	}
	pem, err := os.ReadFile(ca) //nolint:gosec // G304: the file the developer names
	if err != nil {
		return 0, fmt.Errorf("the CA of the SMTP server: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return 0, fmt.Errorf("%s holds no certificate", ca)
	}
	m, err := smtp.New(smtp.Options{
		Host: host, Port: port, From: "noreply@social.example",
		TLSConfig: &tls.Config{ServerName: host, RootCAs: roots, MinVersion: tls.VersionTLS12},
	})
	if err != nil {
		return 0, err
	}
	return mailpreview.Send(context.Background(), m, locales)
}
