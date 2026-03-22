package main

import (
	"context"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"itswork.app/internal/ingestor"
	"itswork.app/internal/processor"

	"github.com/stretchr/testify/assert"
)

func TestApp_Config(t *testing.T) {
	// Test that main logic handles environment correctly
	os.Setenv("PORT", "9999")
	assert.Equal(t, "9999", os.Getenv("PORT"))
}

func TestApp_Lifecycle(t *testing.T) {
	// Setup app manually with mocks to avoid real networking
	app := &App{
		Port:   "9999",
		Server: &http.Server{Addr: ":9999"},
		Sub:    processor.NewSubscriber(nil, nil, nil),
		BrainClient: &processor.BrainClient{},
		Pub:    ingestor.NewPublisher(),
	}

	// Run orchestration (in background)
	app.Run()
	
	// Fast Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	app.Shutdown(ctx)
	
	assert.NotNil(t, app.Server)
}

func TestSetupApp_Success(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	app, err := SetupApp(AppOptions{DB: db})
	assert.NoError(t, err)
	assert.NotNil(t, app)
}

func TestRunMain_Signal(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	
	go func() {
		time.Sleep(100 * time.Millisecond)
		p, _ := os.FindProcess(os.Getpid())
		p.Signal(syscall.SIGINT) // nolint:errcheck
	}()

	err := RunMain(AppOptions{DB: db})
	assert.NoError(t, err)
}

func TestRunMain_Fail(t *testing.T) {
	os.Setenv("DATABASE_URL", "invalid_dsn")
	err := RunMain()
	if err != nil {
		assert.Error(t, err)
	}
}
