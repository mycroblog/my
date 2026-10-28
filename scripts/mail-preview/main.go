// SPDX-License-Identifier: AGPL-3.0-only

// Command mail-preview writes every email that the server sends, rendered from sample inputs in
// en, ja, en-XA, and ar-XB, to a directory with an index.html that shows them, and prints the
// path of index.html. It needs no database, server, or mail server. `scripts/task.sh mail:preview`
// runs it:
//
//	go run ./scripts/mail-preview -out .tmp/mail/preview
package main

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"github.com/mycroblog/my/internal/platform/mailpreview"
)

func main() {
	out := flag.String("out", ".tmp/mail/preview", "the directory to write the preview to (replaced)")
	flag.Parse()
	index, err := mailpreview.Write(*out, mailpreview.Locales)
	if err != nil {
		log.Fatal(err)
	}
	if abs, err := filepath.Abs(index); err == nil {
		index = abs
	}
	fmt.Println(index)
}
