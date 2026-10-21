// Package migrations embeds the SQL migration files so the server binary is
// self-contained and can migrate the database on startup.
package migrations

import "embed"

// FS holds every *.sql migration in this directory, in goose format.
//
//go:embed *.sql
var FS embed.FS
