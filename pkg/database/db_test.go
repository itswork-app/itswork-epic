package database

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/DATA-DOG/go-sqlmock"
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

func TestInitDBWithDriver_Mock(t *testing.T) {
	db, mock, _ := sqlmock.New()
	mock.ExpectPing()
	db.Close()
	_, _ = InitDBWithDriver("sqlmock", "mock_dsn")
}
