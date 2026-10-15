// Package pgtest gives each test its own freshly migrated Postgres database, cloned from a
// template database in one container shared by the whole test binary.
package pgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/pg"
)

const (
	image    = "postgres:16-alpine"
	template = "pgtest_template"
)

var (
	once     sync.Once
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
	once.Do(start)
	if errStart != nil {
		t.Fatalf("pgtest: %v", errStart)
	}
	name := "t_" + randomSuffix()
	if err := exec(context.Background(), adminURL, fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", name, template)); err != nil {
		t.Fatalf("pgtest: create database: %v", err)
	}
	return withDatabase(adminURL, name)
}

// The container is left to testcontainers' reaper, which removes it when the test binary exits.
func start() {
	ctx := context.Background()
	c, err := postgres.Run(ctx, image,
		postgres.WithDatabase("postgres"),
		postgres.WithUsername("pgtest"),
		postgres.WithPassword("pgtest"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		errStart = err
		return
	}
	if adminURL, errStart = c.ConnectionString(ctx, "sslmode=disable"); errStart != nil {
		return
	}
	if errStart = exec(ctx, adminURL, "CREATE DATABASE "+template); errStart != nil {
		return
	}
	errStart = pg.Migrate(ctx, withDatabase(adminURL, template))
}

func exec(ctx context.Context, dsn, sql string) error {
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
