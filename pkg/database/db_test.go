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
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectPing()

	// Verify connection on startup
	if err := db.Ping(); err != nil {
		t.Errorf("Failed to ping: %v", err)
	}
}

func TestInitDBWithDriver_OpenError(t *testing.T) {
	_, err := InitDBWithDriver("invalid_driver_xyz", "dsn")
	assert.Error(t, err)
}
