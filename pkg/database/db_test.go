package database

import (
	"os"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
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

func TestInitDBWithDriver_Mock(t *testing.T) {
	db, mock, _ := sqlmock.New()
	mock.ExpectPing()
	db.Close()
	_, _ = InitDBWithDriver("sqlmock", "mock_dsn")
}

func TestInitDBWithDriver_OpenError(t *testing.T) {
	_, err := InitDBWithDriver("invalid_driver_xyz", "dsn")
	assert.Error(t, err)
}
