package db

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"

	"todoapp/migrations"
)

func TestNormalizeDSNForcesSettings(t *testing.T) {
	out, err := NormalizeDSN("u:p@tcp(db:3306)/todoapp?parseTime=false&interpolateParams=true")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := mysql.ParseDSN(out)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ParseTime || cfg.InterpolateParams || cfg.Loc.String() != "UTC" {
		t.Fatalf("settings not forced: %s", out)
	}
	if cfg.Params["time_zone"] != "'+00:00'" {
		t.Fatalf("time_zone not forced: %s", out)
	}
	if !strings.Contains(out, "utf8mb4") {
		t.Fatalf("collation not set: %s", out)
	}
}

func TestEmbeddedMigrationsLoad(t *testing.T) {
	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no embedded migrations found")
	}
	for _, f := range files {
		b, err := fs.ReadFile(migrations.FS, f)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if !strings.Contains(s, "-- +goose Up") || !strings.Contains(s, "-- +goose Down") {
			t.Errorf("%s: missing goose Up/Down annotations", f)
		}
	}
}
