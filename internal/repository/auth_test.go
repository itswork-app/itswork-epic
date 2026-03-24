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

	t.Run("SaveUserRole", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO users").
			WithArgs("user1", "trader").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.SaveUserRole(ctx, "user1", "trader")
		assert.NoError(t, err)
	})

	t.Run("SyncUser", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO users").
			WithArgs("user1").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.SyncUser(ctx, "user1")
		assert.NoError(t, err)
	})

	t.Run("GetUserRole_Found", func(t *testing.T) {
		mock.ExpectQuery("SELECT role FROM users").
			WithArgs("user1").
			WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("trader"))

		role, err := repo.GetUserRole(ctx, "user1")
		assert.NoError(t, err)
		assert.Equal(t, "trader", role)
	})

	t.Run("SaveUserRole_Error", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO users").
			WillReturnError(sql.ErrConnDone)

		err := repo.SaveUserRole(ctx, "user-err", "trader")
		assert.Error(t, err)
	})

	t.Run("SyncUser_Error", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO users").
			WillReturnError(sql.ErrConnDone)

		err := repo.SyncUser(ctx, "user-err")
		assert.Error(t, err)
	})

	t.Run("GetUserRole_Error", func(t *testing.T) {
		mock.ExpectQuery("SELECT role FROM users").
			WillReturnError(sql.ErrConnDone)

		role, err := repo.GetUserRole(ctx, "user-err")
		assert.Error(t, err)
		assert.Empty(t, role)
	})
}
