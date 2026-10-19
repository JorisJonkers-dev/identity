package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
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
		"VAULT_ADDR":   "http://127.0.0.1:1",
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

func TestAnUnknownOrIncompleteCommandPrintsTheUsage(t *testing.T) {
	for _, args := range [][]string{{"admin"}, {"admin", "demote", "a@b.c"}, {"serve", "promote", "a@b.c"}, {"admin", "promote"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, errs strings.Builder
			if code := command(t.Context(), env(nil), args, &out, &errs); code != 2 || !strings.Contains(errs.String(), "usage:") {
				t.Fatalf("command = %d, %q", code, errs.String())
			}
		})
	}
}

func TestPromotingWithoutADatabaseSaysWhatIsMissing(t *testing.T) {
	var out, errs strings.Builder
	if code := command(t.Context(), env(nil), []string{"admin", "promote", "a@b.c"}, &out, &errs); code != 1 || !strings.Contains(errs.String(), "DATABASE_URL is not set") {
		t.Fatalf("command = %d, %q", code, errs.String())
	}
}

func TestPromotingAgainstAnUnreachableDatabaseFails(t *testing.T) {
	var out, errs strings.Builder
	if code := command(t.Context(), env(map[string]string{"DATABASE_URL": unreachable}), []string{"admin", "promote", "--force", "a@b.c"}, &out, &errs); code != 1 || errs.Len() == 0 {
		t.Fatalf("command = %d, %q", code, errs.String())
	}
}

func TestVaultIsRequired(t *testing.T) {
	vars := complete()
	delete(vars, "VAULT_ADDR")
	if _, err := readConfig(env(vars)); err == nil || !strings.Contains(err.Error(), "VAULT_ADDR") {
		t.Fatalf("readConfig = %v", err)
	}
}

func TestVaultSignsInAsTheServiceAccountWithTheTransitKeysNamed(t *testing.T) {
	cfg, err := readConfig(env(complete()))
	if err != nil {
		t.Fatal(err)
	}
	if v := cfg.vault; v.role != "identity" || v.authMount != "kubernetes" || v.transitMount != "transit" || v.totpKey != "identity-totp" ||
		v.signingKey != "identity-signing" || v.jwtPath != "/var/run/secrets/kubernetes.io/serviceaccount/token" {
		t.Fatalf("Vault defaults = %+v", v)
	}
}

func TestVaultIsReachedWithATokenOrTheServiceAccount(t *testing.T) {
	for _, token := range []string{"", "dev-root"} {
		t.Run("token "+token, func(t *testing.T) {
			vars := complete()
			vars["VAULT_TOKEN"], vars["VAULT_JWT_PATH"] = token, "/no/such/token"
			cfg, _ := readConfig(env(vars))
			// Neither reaches a Vault: the token fails to connect, the service account fails to read.
			if _, err := cfg.vault.vault().Transit("transit").Key("identity-signing").Seal(t.Context(), []byte("x")); err == nil {
				t.Errorf("sealed against nothing with token %q", token)
			}
		})
	}
}

func TestPruningLogsHowManyAuditEventsItRemoved(t *testing.T) {
	var logs bytes.Buffer
	job := prune(func(context.Context) (int64, error) { return 4, nil }, slog.New(slog.NewJSONHandler(&logs, nil)), "audit events")
	if err := job(context.Background()); err != nil || !strings.Contains(logs.String(), `"msg":"audit events pruned","count":4`) {
		t.Fatalf("prune = %v, logged %s", err, logs.String())
	}
}

func TestPruningNothingLogsNothing(t *testing.T) {
	var logs bytes.Buffer
	if err := prune(func(context.Context) (int64, error) { return 0, nil }, slog.New(slog.NewJSONHandler(&logs, nil)), "audit events")(context.Background()); err != nil || logs.Len() != 0 {
		t.Fatalf("prune = %v, logged %s", err, logs.String())
	}
}

func TestAFailedPruneIsPassedOn(t *testing.T) {
	boom := errors.New("boom")
	if err := prune(func(context.Context) (int64, error) { return 0, boom }, slog.New(slog.DiscardHandler), "audit events")(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("prune = %v", err)
	}
}

func TestTheDashboardLinksToTheGrafanaURLGiven(t *testing.T) {
	vars := complete()
	vars["GRAFANA_URL"] = "https://grafana.example.org/d/identity"
	cfg, err := readConfig(env(vars))
	if err != nil || cfg.grafana == nil || cfg.grafana.String() != vars["GRAFANA_URL"] {
		t.Fatalf("grafana = %v, %v", cfg.grafana, err)
	}
}

func TestWithoutAGrafanaURLTheDashboardLinksNowhere(t *testing.T) {
	if cfg, err := readConfig(env(complete())); err != nil || cfg.grafana != nil {
		t.Fatalf("grafana = %v, %v", cfg.grafana, err)
	}
}

func TestAGrafanaURLThatIsNotHTTPSIsRefused(t *testing.T) {
	for _, raw := range []string{"http://grafana.example.org", "https://", "::not a url"} {
		t.Run(raw, func(t *testing.T) {
			vars := complete()
			vars["GRAFANA_URL"] = raw
			if _, err := readConfig(env(vars)); err == nil || !strings.Contains(err.Error(), "GRAFANA_URL") {
				t.Fatalf("readConfig = %v", err)
			}
		})
	}
}

func TestAnUnknownOrIncompleteImportPrintsTheUsage(t *testing.T) {
	for _, args := range [][]string{
		{"import"},
		{"import", "keycloak"},
		{"import", "auth-api", "--force"},
		{"import", "auth-api", "--client"},
		{"import", "auth-api", "--client", "MAIL"},
		{"import", "auth-api", "--client", "=stalwart"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, errs strings.Builder
			if code := command(t.Context(), env(nil), args, &out, &errs); code != 2 || !strings.Contains(errs.String(), "usage: identity import auth-api") {
				t.Fatalf("command = %d, %q", code, errs.String())
			}
		})
	}
}

func TestImportingWithoutTheDatabasesAndVaultSaysWhatIsMissing(t *testing.T) {
	var out, errs strings.Builder
	code := command(t.Context(), env(nil), []string{"import", "auth-api", "--dry-run", "--client", "MAIL=stalwart"}, &out, &errs)
	if code != 1 || !strings.Contains(errs.String(), "DATABASE_URL, LEGACY_DATABASE_URL, VAULT_ADDR not set") {
		t.Fatalf("command = %d, %q", code, errs.String())
	}
}

func TestImportingFromAnUnreachableDatabaseFails(t *testing.T) {
	vars := map[string]string{"DATABASE_URL": unreachable, "LEGACY_DATABASE_URL": unreachable, "VAULT_ADDR": "http://127.0.0.1:1"}
	var out, errs strings.Builder
	if code := command(t.Context(), env(vars), []string{"import", "auth-api"}, &out, &errs); code != 1 || errs.Len() == 0 {
		t.Fatalf("command = %d, %q", code, errs.String())
	}
}
