package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub redis connection", err)
	}
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	return mr, client
}

func TestSaveAnalysis_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewTokenRepository(db, nil)
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

	repo := NewTokenRepository(db, nil)
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

	repo := NewTokenRepository(db, nil)
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

func TestGetAnalysis_CacheHit(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	// Nil DB because it should exclusively use the cache
	repo := NewTokenRepository(nil, rdb)
	ctx := context.Background()

	// Seed cache
	err := mr.Set("token_verdict:mint123", `{"score":85,"verdict":"SAFE"}`)
	assert.NoError(t, err)

	resp, err := repo.GetAnalysis(ctx, "mint123")
	assert.NoError(t, err)
	assert.Equal(t, int32(85), resp.Score)
	assert.Equal(t, "SAFE", resp.Verdict)
}

func TestGetAnalysis_CacheMiss_DBHit(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	repo := NewTokenRepository(db, rdb)
	ctx := context.Background()

	mock.ExpectQuery(`SELECT verdict, rug_score FROM token_analysis WHERE mint_address = \$1`).
		WithArgs("mint_miss").
		WillReturnRows(sqlmock.NewRows([]string{"verdict", "rug_score"}).AddRow("RUG", 20))

	resp, err := repo.GetAnalysis(ctx, "mint_miss")
	assert.NoError(t, err)
	assert.Equal(t, int32(20), resp.Score)
	assert.Equal(t, "RUG", resp.Verdict)

	// Verify it was cached in Redis after the miss
	val, err := mr.Get("token_verdict:mint_miss")
	assert.NoError(t, err)
	assert.Contains(t, val, "RUG")
}

func TestGetAnalysis_DBNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	repo := NewTokenRepository(db, rdb)
	ctx := context.Background()

	mock.ExpectQuery(`SELECT verdict, rug_score FROM token_analysis WHERE mint_address = \$1`).
		WithArgs("mint_not_found").
		WillReturnError(sql.ErrNoRows)

	resp, err := repo.GetAnalysis(ctx, "mint_not_found")
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "analysis not found")
}

func TestGetAnalysis_CacheUnmarshalError(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	repo := NewTokenRepository(db, rdb)
	ctx := context.Background()

	// Seed cache with invalid JSON format
	err = mr.Set("token_verdict:mint_unmarshal_err", `not-a-json`)
	assert.NoError(t, err)

	mock.ExpectQuery(`SELECT verdict, rug_score FROM token_analysis WHERE mint_address = \$1`).
		WithArgs("mint_unmarshal_err").
		WillReturnRows(sqlmock.NewRows([]string{"verdict", "rug_score"}).AddRow("SAFE", 99))

	resp, err := repo.GetAnalysis(ctx, "mint_unmarshal_err")
	assert.NoError(t, err)
	assert.Equal(t, int32(99), resp.Score)
}

func TestGetAnalysis_RedisGetAndSetError(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	mr, rdb := setupTestRedis(t)

	repo := NewTokenRepository(db, rdb)
	ctx := context.Background()

	// Close miniredis immediately to force Redis GET and SET network errors
	mr.Close()

	mock.ExpectQuery(`SELECT verdict, rug_score FROM token_analysis WHERE mint_address = \$1`).
		WithArgs("mint_redis_down").
		WillReturnRows(sqlmock.NewRows([]string{"verdict", "rug_score"}).AddRow("RUG", 20))

	resp, err := repo.GetAnalysis(ctx, "mint_redis_down")
	assert.NoError(t, err)
	assert.Equal(t, int32(20), resp.Score)
	assert.Equal(t, "RUG", resp.Verdict)
}

func TestGetAnalysis_DBQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	repo := NewTokenRepository(db, rdb)
	ctx := context.Background()

	mock.ExpectQuery(`SELECT verdict, rug_score FROM token_analysis WHERE mint_address = \$1`).
		WithArgs("mint_db_err").
		WillReturnError(errors.New("db connection dropped"))

	resp, err := repo.GetAnalysis(ctx, "mint_db_err")
	assert.Error(t, err)
	assert.Nil(t, resp)
}
