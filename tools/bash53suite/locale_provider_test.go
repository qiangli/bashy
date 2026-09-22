package main

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConfigureHostLocaleProvider(t *testing.T) {
	t.Run("windows discovers host before private PATH takes over", func(t *testing.T) {
		env := map[string]string{}
		provider, err := configureHostLocaleProvider("windows", func(k string) string { return env[k] }, func(k, v string) error {
			env[k] = v
			return nil
		}, func(name string) (string, error) {
			if name != "locale" {
				t.Fatalf("lookup = %q, want locale", name)
			}
			return filepath.Join("host", "usr", "bin", "locale.exe"), nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if provider == "" || !filepath.IsAbs(provider) || env[hostLocalePathEnv] != provider {
			t.Fatalf("provider=%q env=%q, want one absolute discovered path", provider, env[hostLocalePathEnv])
		}
		if got := strings.Split(env[hostLocaleNamesEnv], ";"); !reflect.DeepEqual(got, corpusLocaleNames) {
			t.Fatalf("candidates=%q, want complete corpus set %q", got, corpusLocaleNames)
		}
	})

	t.Run("explicit package provider and candidate set win", func(t *testing.T) {
		env := map[string]string{
			hostLocalePathEnv:  `C:\private\cygwin\bin\locale.exe`,
			hostLocaleNamesEnv: "custom_LOCALE",
		}
		looked := false
		provider, err := configureHostLocaleProvider("windows", func(k string) string { return env[k] }, func(k, v string) error {
			env[k] = v
			return nil
		}, func(string) (string, error) {
			looked = true
			return "", errors.New("must not run")
		})
		if err != nil || looked || provider != env[hostLocalePathEnv] || env[hostLocaleNamesEnv] != "custom_LOCALE" {
			t.Fatalf("provider=%q looked=%v env=%v err=%v", provider, looked, env, err)
		}
	})

	t.Run("missing provider stays an honest absence", func(t *testing.T) {
		env := map[string]string{}
		provider, err := configureHostLocaleProvider("windows", func(k string) string { return env[k] }, func(k, v string) error {
			env[k] = v
			return nil
		}, func(string) (string, error) { return "", errors.New("not found") })
		if err != nil || provider != "" || len(env) != 0 {
			t.Fatalf("provider=%q env=%v err=%v, want unchanged absence", provider, env, err)
		}
	})

	t.Run("other hosts do not probe", func(t *testing.T) {
		looked := false
		provider, err := configureHostLocaleProvider("linux", func(string) string { return "" }, func(string, string) error { return nil }, func(string) (string, error) {
			looked = true
			return "", nil
		})
		if err != nil || provider != "" || looked {
			t.Fatalf("provider=%q looked=%v err=%v", provider, looked, err)
		}
	})
}
