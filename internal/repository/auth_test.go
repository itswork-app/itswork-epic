package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestAuthRepository(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	repo := NewAuthRepository(db, rdb)
	ctx := context.Background()
	apiKey := "test-secret-key"
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(apiKey)))

	t.Run("SaveAPIKey", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO api_keys").
			WithArgs("user1", hash, "test-label").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.SaveAPIKey(ctx, "user1", hash, "test-label")
		assert.NoError(t, err)
	})

	t.Run("GetUserIDByAPIKey_CacheMiss_DBHit", func(t *testing.T) {
		mock.ExpectQuery("SELECT user_id, status FROM api_keys").
			WithArgs(hash).
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "status"}).AddRow("user1", "active"))

		userID, err := repo.GetUserIDByAPIKey(ctx, hash)
		assert.NoError(t, err)
		assert.Equal(t, "user1", userID)

		// Verify it's cached in Redis
		cachedUserID, _ := rdb.Get(ctx, "api_key_user:"+hash).Result()
		assert.Equal(t, "user1", cachedUserID)
	})

	t.Run("GetUserIDByAPIKey_CacheHit", func(t *testing.T) {
		// Redis already populated from previous test
		userID, err := repo.GetUserIDByAPIKey(ctx, hash)
		assert.NoError(t, err)
		assert.Equal(t, "user1", userID)
	})

	t.Run("GetUserIDByAPIKey_Revoked", func(t *testing.T) {
		revokedKey := "revoked-key"
		revHash := fmt.Sprintf("%x", sha256.Sum256([]byte(revokedKey)))

		mock.ExpectQuery("SELECT user_id, status FROM api_keys").
			WithArgs(revHash).
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "status"}).AddRow("user1", "revoked"))

		userID, err := repo.GetUserIDByAPIKey(ctx, revHash)
		assert.NoError(t, err)
		assert.Empty(t, userID)
	})

	t.Run("GetUserIDByAPIKey_NotFound", func(t *testing.T) {
		mock.ExpectQuery("SELECT user_id, status FROM api_keys").
			WillReturnError(sql.ErrNoRows)

		userID, err := repo.GetUserIDByAPIKey(ctx, "non-existent")
		assert.NoError(t, err)
		assert.Empty(t, userID)
	})

	t.Run("SaveAPIKey_Error", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO api_keys").
			WithArgs("user2", "hash2", "test-label2").
			WillReturnError(sql.ErrConnDone)

		err := repo.SaveAPIKey(ctx, "user2", "hash2", "test-label2")
		assert.Error(t, err)
	})
}
