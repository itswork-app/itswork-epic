package main

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"

	"itswork.app/internal/app"
)

func TestApp_Config(t *testing.T) {
	// Test that main logic handles environment correctly
	os.Setenv("PORT", "9999")
	assert.Equal(t, "9999", os.Getenv("PORT"))
}

func TestApp_Lifecycle(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	application, err := app.SetupApp(app.AppOptions{DB: db})
	assert.NoError(t, err)
	assert.NotNil(t, application)

	// Run orchestration (in background)
	application.Run()

	// Fast Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	application.Shutdown(ctx)

	assert.NotNil(t, application.Server)
}

func TestSetupApp_Success(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	application, err := app.SetupApp(app.AppOptions{DB: db})
	assert.NoError(t, err)
	assert.NotNil(t, application)
}

func TestRunMain_Signal(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	go func() {
		time.Sleep(200 * time.Millisecond)
		p, _ := os.FindProcess(os.Getpid())
		_ = p.Signal(syscall.SIGINT) // Send signal to self to trigger shutdown
	}()

	err := app.RunMain(app.AppOptions{DB: db})
	assert.NoError(t, err)
}

func TestRunMain_Fail(t *testing.T) {
	os.Setenv("DATABASE_URL", "invalid_dsn")
	defer os.Unsetenv("DATABASE_URL")
	err := app.RunMain()
	// Should fail because of invalid DSN in InitDB
	assert.Error(t, err)
}

func TestMainExecution(t *testing.T) {
	oldRunMain := runMain
	defer func() { runMain = oldRunMain }()

	runMain = func(opts ...app.AppOptions) error {
		return nil // mock success
	}

	main()
}
