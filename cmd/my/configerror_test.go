// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/mycroblog/my/internal/platform/config"
)

func TestPrintLoadError(t *testing.T) {
	err := &config.ValidationError{Issues: []config.Issue{
		{Key: "host", Missing: true},
		{Key: "database", Message: `unknown value "x"`},
		{Message: `config file my.toml: unknown key "x"`},
	}}
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"en", map[string]string{"LANG": "C.UTF-8"}, `Found 3 configuration errors.
  - The required setting MY_HOST is not set.
  - The setting MY_DATABASE has an invalid value.
    database: unknown value "x"
  - config file my.toml: unknown key "x"
`},
		{"ja", map[string]string{"LANG": "ja_JP.UTF-8"}, `設定エラーが3件あります。
  - 必須の設定 MY_HOST が設定されていません。
  - 設定 MY_DATABASE の値が正しくありません。
    database: unknown value "x"
  - config file my.toml: unknown key "x"
`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printLoadErrorEnv(&buf, err, func(k string) (string, bool) {
				v, ok := tt.env[k]
				return v, ok
			})
			if buf.String() != tt.want {
				t.Errorf("output =\n%s\nwant\n%s", buf.String(), tt.want)
			}
		})
	}

	var buf bytes.Buffer
	printLoadErrorEnv(&buf, errors.New("boom"), func(string) (string, bool) { return "", false })
	if buf.String() != "boom\n" {
		t.Errorf("other error = %q, want %q", buf.String(), "boom\n")
	}
}
