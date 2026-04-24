package runtime

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newTestStmt returns a fresh *gorm.DB in DryRun mode backed by sqlite
// in-memory. DryRun lets us call Where/Find/Count without actually hitting
// the database, so each call produces an inspectable Statement without side
// effects between tests. We need a real Dialector for GORM to place
// identifiers correctly; pure-Go glebarez/sqlite is already in go.mod.
func newTestStmt(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	return db.Session(&gorm.Session{DryRun: true, NewDB: true})
}

// explainTestStmt compiles the current Where/Order clauses to a SQL string
// by running a dummy Find. Callers inspect the result with strings.Contains
// so column quoting and operator placement are asserted without pinning the
// exact SELECT shape (which varies across GORM versions).
func explainTestStmt(tx *gorm.DB) string {
	type row struct{ ID int }
	stmt := tx.Session(&gorm.Session{DryRun: true}).Find(&[]row{}).Statement
	return stmt.SQL.String()
}
