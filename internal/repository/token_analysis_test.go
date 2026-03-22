package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func TestSaveAnalysis_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewTokenRepository(db)
	ctx := context.Background()

	mint := "mint123"
	creator := "creator456"
	verdict := "SAFE"
	score := 85
	creatorUUID := "uuid-123"

	mock.ExpectBegin()
	
	// Stage 1: Wallet UPSERT
	mock.ExpectQuery("INSERT INTO wallets").
		WithArgs(creator).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(creatorUUID))

	// Stage 2: Token Analysis UPSERT
	mock.ExpectExec("INSERT INTO token_analysis").
		WithArgs(mint, creatorUUID, verdict, score, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	err = repo.SaveAnalysis(ctx, mint, creator, verdict, score)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveAnalysis_WalletFailure_Rollback(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewTokenRepository(db)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO wallets").
		WithArgs("creator_fail").
		WillReturnError(errors.New("db error"))
	mock.ExpectRollback()

	err = repo.SaveAnalysis(ctx, "mint", "creator_fail", "SAFE", 80)
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveAnalysis_AnalysisFailure_Rollback(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewTokenRepository(db)
	ctx := context.Background()
	creatorUUID := "uuid-123"

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO wallets").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(creatorUUID))
	mock.ExpectExec("INSERT INTO token_analysis").
		WillReturnError(errors.New("deadlock or something"))
	mock.ExpectRollback()

	err = repo.SaveAnalysis(ctx, "mint", "creator", "SAFE", 80)
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
