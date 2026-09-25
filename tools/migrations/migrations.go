// Package migrations exposes the ordered Goose SQL migrations for host deployment.
package migrations

import "embed"

// Files contains the authentication and canonical Vibox migrations.
//
//go:embed *.sql
var Files embed.FS
