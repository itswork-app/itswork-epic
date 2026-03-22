package database

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitDB_NoURL(t *testing.T) {
	// Set empty URL to test warning path
	os.Setenv("DATABASE_URL", "")
	db, err := InitDB()
	assert.Error(t, err) // Should fail to ping
	if db != nil {
		db.Close()
	}
}
