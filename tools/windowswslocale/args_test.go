package main

import (
	"reflect"
	"testing"
)

func TestWSLLocaleArgs(t *testing.T) {
	tests := []struct {
		name      string
		locale    string
		localeSet bool
		args      []string
		want      []string
	}{
		{
			name: "available locales do not invent LC_ALL",
			args: []string{"-a"},
			want: []string{"-d", "Ubuntu", "--", "env", "locale", "-a"},
		},
		{
			name:      "charmap preserves LC_ALL",
			locale:    "zh_HK.big5hkscs",
			localeSet: true,
			args:      []string{"charmap"},
			want:      []string{"-d", "Ubuntu", "--", "env", "LC_ALL=zh_HK.big5hkscs", "locale", "charmap"},
		},
		{
			name:      "category query is passed through",
			locale:    "ja_JP.SJIS",
			localeSet: true,
			args:      []string{"-k", "LC_CTYPE"},
			want:      []string{"-d", "Ubuntu", "--", "env", "LC_ALL=ja_JP.SJIS", "locale", "-k", "LC_CTYPE"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wslLocaleArgs("Ubuntu", tt.locale, tt.localeSet, tt.args); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("wslLocaleArgs() = %q, want %q", got, tt.want)
			}
		})
	}
}
