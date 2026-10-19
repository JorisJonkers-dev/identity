// Package pgtest gives each test its own freshly migrated Postgres database, cloned from a
// template database that liquibase-runner migrated once, in containers shared by the test binary.
package pgtest

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/JorisJonkers-dev/identity/db"
)

// Tag is the revision tag the runner writes after the changelog; 12 hex digits, as in the cluster.
const Tag = "000000000000"

const (
	image = "postgres:16-alpine"
	// The migration image's base, pinned by digest like Dockerfile.migration's FROM.
	runnerImage = "ghcr.io/jorisjonkers-dev/liquibase-runner:v0.2.0@sha256:5e0f07bea045f35170f9d66ececa2de5ba198bf8ba51a2d11234e3c1b44ff7ea"
	template    = "pgtest_template"
	user        = "pgtest"
	password    = "pgtest"
	alias       = "postgres"
)

var (
	once     sync.Once
	nw       *testcontainers.DockerNetwork
	adminURL string
	errStart error
)

// URL returns the connection string of a new database migrated to the latest schema. It skips
// the test under -short, which is how to run the suite without Docker.
func URL(t testing.TB) string {
	t.Helper()
	if testing.Short() {
		t.Skip("needs Docker; skipped with -short")
	}
	started(t)
	name := "t_" + randomSuffix()
	if err := execSQL(context.Background(), adminURL, fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", name, template)); err != nil {
		t.Fatalf("pgtest: create database: %v", err)
	}
	return withDatabase(adminURL, name)
}

// Reversible runs the runner's `ci reversibility` on a new, empty database: every changeset is
// applied, rolled back and applied again, as CI's Migration Proof does before a release.
func Reversible(t testing.TB) error {
	t.Helper()
	if testing.Short() {
		t.Skip("needs Docker; skipped with -short")
	}
	started(t)
	name := "r_" + randomSuffix()
	if err := execSQL(t.Context(), adminURL, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("pgtest: create database: %v", err)
	}
	return runRunner(t.Context(), nw, name, "ci", "reversibility")
}

func started(t testing.TB) {
	t.Helper()
	once.Do(func() { _, nw, adminURL, errStart = start(context.Background()) })
	if errStart != nil {
		t.Fatalf("pgtest: %v", errStart)
	}
}

// Schema migrates a fresh database and returns its schema as plain DDL, without Liquibase's own
// tables and without the session settings pg_dump writes; `task gen:schema` feeds it to sqlc.
func Schema(ctx context.Context) (string, error) {
	pgc, _, _, err := start(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = testcontainers.TerminateContainer(pgc) }()
	code, out, err := pgc.Exec(ctx, []string{
		"pg_dump", "--schema-only", "--no-owner", "--no-privileges", "--no-comments",
		"--exclude-table=databasechangelog*", "--username", user, template,
	}, exec.Multiplexed())
	if err != nil {
		return "", fmt.Errorf("pg_dump: %w", err)
	}
	dump, err := io.ReadAll(out)
	if err != nil {
		return "", fmt.Errorf("pg_dump output: %w", err)
	}
	if code != 0 {
		return "", fmt.Errorf("pg_dump exited %d: %s", code, dump)
	}
	return cleanDump(string(dump)), nil
}

// The containers are left to testcontainers' reaper, which removes them when the process exits.
func start(ctx context.Context) (*postgres.PostgresContainer, *testcontainers.DockerNetwork, string, error) {
	nw, err := network.New(ctx)
	if err != nil {
		return nil, nil, "", fmt.Errorf("network: %w", err)
	}
	pgc, err := postgres.Run(ctx, image,
		postgres.WithDatabase("postgres"),
		postgres.WithUsername(user),
		postgres.WithPassword(password),
		network.WithNetwork([]string{alias}, nw),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		return nil, nil, "", err
	}
	admin, err := pgc.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, nil, "", err
	}
	if err := execSQL(ctx, admin, "CREATE DATABASE "+template); err != nil {
		return nil, nil, "", err
	}
	if err := runRunner(ctx, nw, template, "ci", "up", Tag); err != nil {
		return nil, nil, "", err
	}
	return pgc, nw, admin, nil
}

// runRunner runs one liquibase-runner command against database, signed in the way CI signs in.
func runRunner(ctx context.Context, nw *testcontainers.DockerNetwork, database string, args ...string) error {
	files, err := changelogFiles()
	if err != nil {
		return err
	}
	c, err := testcontainers.Run(ctx, runnerImage,
		network.WithNetwork(nil, nw),
		testcontainers.WithEnv(map[string]string{
			"DATABASE_HOST":     alias,
			"DATABASE_PORT":     "5432",
			"DATABASE_NAME":     database,
			"DATABASE_USERNAME": user,
			"DATABASE_PASSWORD": password,
		}),
		testcontainers.WithFiles(files...),
		testcontainers.WithCmd(args...),
		testcontainers.WithWaitStrategy(wait.ForExit().WithExitTimeout(3*time.Minute)),
	)
	if err != nil {
		return fmt.Errorf("liquibase-runner: %w", err)
	}
	state, err := c.State(ctx)
	if err != nil {
		return fmt.Errorf("liquibase-runner state: %w", err)
	}
	if state.ExitCode != 0 {
		return fmt.Errorf("liquibase-runner %s exited %d:\n%s", strings.Join(args, " "), state.ExitCode, logs(ctx, c))
	}
	return nil
}

func changelogFiles() ([]testcontainers.ContainerFile, error) {
	var files []testcontainers.ContainerFile
	err := fs.WalkDir(db.Changelog, "changelog", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(db.Changelog, path)
		if err != nil {
			return err
		}
		files = append(files, testcontainers.ContainerFile{
			Reader:            bytes.NewReader(data),
			ContainerFilePath: "/liquibase/" + path,
			FileMode:          0o644,
		})
		return nil
	})
	if err == nil && len(files) == 0 {
		err = errors.New("db/changelog is empty")
	}
	return files, err
}

func logs(ctx context.Context, c testcontainers.Container) string {
	r, err := c.Logs(ctx)
	if err != nil {
		return err.Error()
	}
	defer func() { _ = r.Close() }()
	b, _ := io.ReadAll(r)
	return string(b)
}

// cleanDump keeps the DDL: no comments, session settings, search_path calls or psql meta-commands.
func cleanDump(dump string) string {
	var b strings.Builder
	for line := range strings.SplitSeq(dump, "\n") {
		switch {
		case strings.HasPrefix(line, "--"),
			strings.HasPrefix(line, "SET "),
			strings.HasPrefix(line, "SELECT pg_catalog."),
			strings.HasPrefix(line, `\`):
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(collapseBlankLines(b.String())) + "\n"
}

func collapseBlankLines(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}

func execSQL(ctx context.Context, dsn, sql string) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	_, err = conn.Exec(ctx, sql)
	return err
}

func withDatabase(dsn, name string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		panic(err) // testcontainers built dsn; it always parses
	}
	u.Path = "/" + name
	return u.String()
}

func randomSuffix() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
