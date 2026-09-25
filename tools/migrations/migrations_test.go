package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/not-for-prod/authorization/internal/testutil/pgfixture"
	"github.com/not-for-prod/authorization/tools/migrations"
)

func TestMigrationRollbackAndReapply(t *testing.T) {
	pool := pgfixture.New(t)
	ctx := context.Background()
	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for i := len(entries) - 1; i >= 0; i-- {
		data, err := migrations.Files.ReadFile(entries[i].Name())
		if err != nil {
			t.Fatal(err)
		}
		_, down, ok := strings.Cut(string(data), "-- +goose Down")
		if !ok {
			t.Fatal("missing down migration")
		}
		if _, err = pool.Exec(ctx, down); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema()").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rollback left %d tables", count)
	}
	for _, entry := range entries {
		data, err := migrations.Files.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		up, _, _ := strings.Cut(string(data), "-- +goose Down")
		if _, err = pool.Exec(ctx, up); err != nil {
			t.Fatal(err)
		}
	}
}
