package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestPaymentRepository_CommitUsage_ErrorPaths(t *testing.T) {
	t.Run("CreditDeductError", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		r := NewPaymentRepository(db, nil)

		mock.ExpectBegin()
		mock.ExpectQuery("UPDATE user_credits").
			WithArgs("user_credit_fail").
			WillReturnError(errors.New("tx error"))
		mock.ExpectRollback()

		r.CommitUsage(context.Background(), "user_credit_fail", "credit", "mint123")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("NoUserID", func(t *testing.T) {
		r := NewPaymentRepository(nil, nil)
		r.CommitUsage(context.Background(), "", "subscription", "mint123")
	})
}

func TestPaymentRepository_CheckAccess_EdgeCases(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()

	t.Run("FreeUsageExhausted_NoSubscription", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		r := NewPaymentRepository(db, rdb)

		mock.ExpectExec("INSERT INTO user_credits").WillReturnResult(sqlmock.NewResult(1, 1))

		// Exhaust free tier for ALL free kinds
		rdb.Set(ctx, "free:user:user_exhausted:ui", "10", 0)
		rdb.Set(ctx, "free:user:user_exhausted:api", "10", 0)

		// Mock IsProSubscriber (COUNT(*) = 0)
		mock.ExpectQuery("SELECT COUNT.*FROM user_subscriptions WHERE user_id = \\$1 AND status = 'active'").
			WithArgs("user_exhausted").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		granted, kind, err := r.CheckAccess(ctx, "user_exhausted", "mint1", false)
		assert.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, "", kind)
	})

	t.Run("CheckAccess_Pro_Success", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		r := NewPaymentRepository(db, rdb)

		// Exhaust free tier
		rdb.Set(ctx, "free:user:user_pro:ui", "10", 0)

		// Mock IsProSubscriber (COUNT(*) = 1)
		mock.ExpectQuery("SELECT COUNT.*FROM user_subscriptions WHERE user_id = \\$1 AND status = 'active'").
			WithArgs("user_pro").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		// Mock GetQuotaRemaining (quota_limit = 100)
		mock.ExpectQuery("SELECT quota_limit FROM user_subscriptions WHERE user_id = \\$1 AND status = 'active'").
			WithArgs("user_pro").
			WillReturnRows(sqlmock.NewRows([]string{"quota_limit"}).AddRow(int64(100)))

		granted, kind, err := r.CheckAccess(ctx, "user_pro", "mint1", false)
		assert.NoError(t, err)
		assert.True(t, granted)
		assert.Equal(t, "subscription", kind)
	})

	t.Run("CheckAccess_Pro_RedisHit", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		r := NewPaymentRepository(db, rdb)

		// Set Redis free:user:user_hit:ui to 10 (exceed limit)
		rdb.Set(ctx, "free:user:user_hit:ui", "10", 0)
		// Set Redis sub_active:true and sub_limit:100
		rdb.Set(ctx, "sub_active:user_hit", "true", 0)
		rdb.Set(ctx, "sub_limit:user_hit", "100", 0)
		rdb.Set(ctx, "usage:user:user_hit", "0", 0)

		granted, kind, err := r.CheckAccess(ctx, "user_hit", "mint1", false)
		assert.NoError(t, err)
		assert.True(t, granted)
		assert.Equal(t, "subscription", kind)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("AddUserCredits", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		r := NewPaymentRepository(db, nil)

		mock.ExpectExec("INSERT INTO user_credits").
			WithArgs("user_add_credit", 50).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := r.AddUserCredits(ctx, "user_add_credit", 50)
		assert.NoError(t, err)
	})

	t.Run("AddUserCredits_Error", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		r := NewPaymentRepository(db, nil)

		mock.ExpectExec("INSERT INTO user_credits").WillReturnError(errors.New("conn error"))
		err := r.AddUserCredits(ctx, "user_err", 10)
		assert.Error(t, err)
	})
}

func TestPaymentRepository_UpdatePaymentStatus_Coverage(t *testing.T) {
	t.Run("Error", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		r := NewPaymentRepository(db, nil)

		mock.ExpectQuery("UPDATE payments").
			WillReturnError(errors.New("db error"))

		err := r.UpdatePaymentStatus(context.Background(), "tx123", "failed")
		assert.Error(t, err)
	})

	t.Run("Success", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		r := NewPaymentRepository(db, nil)

		mock.ExpectQuery("UPDATE payments").
			WithArgs("completed", "tx_ok").
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "mint_address", "amount_sol"}).
				AddRow("user1", "mint1", 0.1))

		err := r.UpdatePaymentStatus(context.Background(), "tx_ok", "completed")
		assert.NoError(t, err)
	})
}

func TestPaymentRepository_ActivateSubscription_Coverage(t *testing.T) {
	t.Run("Error_Insert", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		r := NewPaymentRepository(db, nil)

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT plan_type,.*FROM user_subscriptions WHERE user_id = \\$1").
			WithArgs("user1").
			WillReturnError(sql.ErrNoRows)

		mock.ExpectExec("INSERT INTO user_subscriptions").WillReturnError(errors.New("deadlock"))
		mock.ExpectRollback()

		err := r.ActivateSubscription(context.Background(), "user1", "SUB_PRO", 30, 100)
		assert.Error(t, err)
	})

	t.Run("Success_Update", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		r := NewPaymentRepository(db, nil)

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT plan_type,.*FROM user_subscriptions WHERE user_id = \\$1").
			WithArgs("user_up").
			WillReturnRows(sqlmock.NewRows([]string{"plan_type", "plan_tier", "status", "quota_limit", "current_usage", "expires_at"}).
				AddRow("SUB_FREE", 0, "active", 0, 0, time.Now()))

		mock.ExpectExec("UPDATE user_subscriptions SET").
			WithArgs("SUB_MONTHLY_PRO", 2, 30, 500, "user_up").
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectCommit()

		err := r.ActivateSubscription(context.Background(), "user_up", "SUB_MONTHLY_PRO", 30, 500)
		assert.NoError(t, err)
	})
}

func TestAuthRepository_GetAPIKey_Coverage(t *testing.T) {
	t.Run("NotFound", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		r := NewAuthRepository(db, nil)

		mock.ExpectQuery(`SELECT.*FROM api_keys WHERE api_key_hash = \$1`).
			WithArgs("missing-key").
			WillReturnError(sql.ErrNoRows)

		userID, err := r.GetUserIDByAPIKey(context.Background(), "missing-key")
		assert.NoError(t, err)
		assert.Equal(t, "", userID)
	})

	t.Run("Inactive", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		r := NewAuthRepository(db, nil)

		mock.ExpectQuery(`SELECT.*FROM api_keys WHERE api_key_hash = \$1`).
			WithArgs("inactive-key").
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "status"}).AddRow("user1", "revoked"))

		userID, err := r.GetUserIDByAPIKey(context.Background(), "inactive-key")
		assert.NoError(t, err)
		assert.Equal(t, "", userID)
	})
}

func TestRepository_Getters(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	r1 := NewPaymentRepository(db, nil)
	assert.NotNil(t, r1.GetDB())
	assert.Nil(t, r1.GetRedis())

	r2 := NewTokenRepository(db, nil)
	assert.NotNil(t, r2.GetDB())
	assert.Nil(t, r2.GetRedis())
}

func TestTokenRepository_SaveAnalysis_Error(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	r := NewTokenRepository(db, nil)

	mock.ExpectExec("INSERT INTO token_analysis").WillReturnError(errors.New("db fail"))
	err := r.SaveAnalysis(context.Background(), "mint", "creator", "v", "r", "rep", "risk", 10)
	assert.Error(t, err)
}
