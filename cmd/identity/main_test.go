package main

import (
	"strings"
	"testing"
)

func env(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

const unreachable = "postgres://nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=1"

// complete is every required variable, for tests to change one at a time.
func complete() map[string]string {
	return map[string]string{
		"DATABASE_URL": unreachable,
		"PUBLIC_URL":   "https://auth.test/",
		"SMTP_ADDR":    "127.0.0.1:1",
		"MAIL_FROM":    "identity@auth.test",
	}
}

func TestStartRefusesABadEnvironment(t *testing.T) {
	badTLS := complete()
	badTLS["SMTP_TLS"] = "maybe"
	cases := map[string]map[string]string{
		"nothing set":          {},
		"bad SMTP_TLS":         badTLS,
		"unreachable database": complete(),
	}
	for name, vars := range cases {
		t.Run(name, func(t *testing.T) {
			if code := start(t.Context(), env(vars)); code != 1 {
				t.Fatalf("start = %d, want 1", code)
			}
		})
	}
}

func TestReadConfigNamesEveryMissingVariable(t *testing.T) {
	_, err := readConfig(env(map[string]string{}))
	for _, name := range []string{"DATABASE_URL", "PUBLIC_URL", "SMTP_ADDR", "MAIL_FROM"} {
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("err = %v, want it to name %s", err, name)
		}
	}
}

func TestReadConfigDefaults(t *testing.T) {
	cfg, err := readConfig(env(complete()))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.addr != ":8080" || cfg.metricsAddr != ":9090" || cfg.publicURL != "https://auth.test" || !cfg.mailer.RequireTLS {
		t.Fatalf("defaults: %+v", cfg)
	}
	local := complete()
	local["SMTP_TLS"] = "none"
	local["ADDR"] = "127.0.0.1:9000"
	if cfg, err := readConfig(env(local)); err != nil || cfg.mailer.RequireTLS || cfg.addr != "127.0.0.1:9000" {
		t.Fatalf("SMTP_TLS=none: %+v, %v", cfg, err)
	}
	required := complete()
	required["SMTP_TLS"] = "required"
	if cfg, err := readConfig(env(required)); err != nil || !cfg.mailer.RequireTLS {
		t.Fatalf("SMTP_TLS=required: %+v, %v", cfg, err)
	}
}

func TestCommandUsage(t *testing.T) {
	for _, args := range [][]string{{"admin"}, {"admin", "demote", "a@b.c"}, {"serve", "promote", "a@b.c"}, {"admin", "promote"}} {
		var out, errs strings.Builder
		if code := command(t.Context(), env(nil), args, &out, &errs); code != 2 || !strings.Contains(errs.String(), "usage:") {
			t.Errorf("command(%v) = %d, %q", args, code, errs.String())
		}
	}
	var out, errs strings.Builder
	if code := command(t.Context(), env(nil), []string{"admin", "promote", "a@b.c"}, &out, &errs); code != 1 || !strings.Contains(errs.String(), "DATABASE_URL is not set") {
		t.Fatalf("without a database = %d, %q", code, errs.String())
	}
	errs.Reset()
	if code := command(t.Context(), env(map[string]string{"DATABASE_URL": unreachable}), []string{"admin", "promote", "--force", "a@b.c"}, &out, &errs); code != 1 || errs.Len() == 0 {
		t.Fatalf("an unreachable database = %d, %q", code, errs.String())
	}
}

func TestVaultIsOptionalAndSignsInAsConfigured(t *testing.T) {
	cfg, err := readConfig(env(complete()))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.vault.sealer(); ok {
		t.Fatal("a sealer without VAULT_ADDR")
	}
	if v := cfg.vault; v.role != "identity" || v.authMount != "kubernetes" || v.transitMount != "transit" || v.totpKey != "identity-totp" ||
		v.jwtPath != "/var/run/secrets/kubernetes.io/serviceaccount/token" {
		t.Fatalf("Vault defaults = %+v", v)
	}
	for _, token := range []string{"", "dev-root"} {
		vars := complete()
		vars["VAULT_ADDR"], vars["VAULT_TOKEN"], vars["VAULT_JWT_PATH"] = "http://127.0.0.1:1", token, "/no/such/token"
		cfg, _ := readConfig(env(vars))
		sealer, ok := cfg.vault.sealer()
		if !ok {
			t.Fatal("no sealer with VAULT_ADDR")
		}
		// Neither reaches a Vault: the token fails to connect, the service account fails to read.
		if _, err := sealer.Seal(t.Context(), []byte("x")); err == nil {
			t.Errorf("sealed against nothing with token %q", token)
		}
	}
}
