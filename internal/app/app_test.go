package app

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func TestSetupApp(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	opts := AppOptions{DB: db}
	application, err := SetupApp(opts)
	assert.NoError(t, err)
	assert.NotNil(t, application)
	assert.NotNil(t, application.Pub)
	assert.NotNil(t, application.Repo)
}

func TestTelemetryInit(t *testing.T) {
	// Test Sentry logic indirectly via env
	os.Setenv("SENTRY_DSN", "http://public@sentry.io/123")
	os.Setenv("ENV", "test")
	defer os.Unsetenv("SENTRY_DSN")
	defer os.Unsetenv("ENV")

	initTelemetry()
	// No panic = Success for this mockable layer
}

func TestApp_Lifecycle(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	application, err := SetupApp(AppOptions{DB: db})
	assert.NoError(t, err)

	// Run orchestration
	application.Run()

	// Shutdown orchestration
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	application.Shutdown(ctx)
}

func TestSetupApp_DBFail(t *testing.T) {
	os.Setenv("DATABASE_URL", "invalid_dsn")
	defer os.Unsetenv("DATABASE_URL")

	application, err := SetupApp()
	assert.Error(t, err)
	assert.Nil(t, application)
}

func TestSentry_InitFail(t *testing.T) {
	// Invalid DSN usually doesn't panic but returns error
	os.Setenv("SENTRY_DSN", "invalid-dsn")
	defer os.Unsetenv("SENTRY_DSN")

	initTelemetry() // Should log error and continue
}

func TestRunMain_Signal(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	// Signal trigger in background
	go func() {
		time.Sleep(100 * time.Millisecond)
		p, _ := os.FindProcess(os.Getpid())
		_ = p.Signal(syscall.SIGINT)
	}()

	err := RunMain(AppOptions{DB: db})
	assert.NoError(t, err)
}

func TestSetupApp_DefaultPort(t *testing.T) {
	os.Unsetenv("PORT")
	db, _, _ := sqlmock.New()
	defer db.Close()

	application, err := SetupApp(AppOptions{DB: db})
	assert.NoError(t, err)
	assert.Equal(t, "8080", application.Port)
}
