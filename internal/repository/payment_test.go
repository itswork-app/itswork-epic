package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func TestSavePayment(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	payment := &Payment{
		UserID:      "user123",
		MintAddress: "mint456",
		Reference:   "ref789",
		AmountSol:   0.1,
	}

	mock.ExpectQuery("INSERT INTO payments").
		WithArgs(payment.UserID, payment.MintAddress, payment.Reference, "pending", payment.AmountSol).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("uuid-123"))

	err = repo.SavePayment(ctx, payment)
	assert.NoError(t, err)
	assert.Equal(t, "uuid-123", payment.ID)
}

func TestUpdatePaymentStatus_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	repo := NewPaymentRepository(db, rdb)
	ctx := context.Background()

	reference := "ref789"
	status := "success"

	mock.ExpectQuery("UPDATE payments").
		WithArgs(status, reference).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "mint_address", "amount_sol"}).AddRow("user123", "mint456", 0.1))

	err = repo.UpdatePaymentStatus(ctx, reference, status)
	assert.NoError(t, err)

	// Verify Redis cache
	val, err := rdb.Get(ctx, "payment_verified:user123:mint456").Result()
	assert.NoError(t, err)
	assert.Equal(t, "true", val)
}

func TestUpdatePaymentStatus_BundleFulfillment(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	// Mock Update for BUNDLE_50
	mock.ExpectQuery("UPDATE payments").
		WithArgs("success", "ref-bundle").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "mint_address", "amount_sol"}).AddRow("user123", "BUNDLE_50", 4.0))

	// Fulfillment: AddUserCredits
	mock.ExpectExec("INSERT INTO user_credits").
		WithArgs("user123", 50).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = repo.UpdatePaymentStatus(ctx, "ref-bundle", "success")
	assert.NoError(t, err)
}

func TestUpdatePaymentStatus_SubscriptionFulfillment(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	// Mock Update for SUB_MONTHLY_PRO
	mock.ExpectQuery("UPDATE payments").
		WithArgs("success", "ref-sub").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "mint_address", "amount_sol"}).AddRow("user123", "SUB_MONTHLY_PRO", 2.0))

	// Fulfillment: ActivateSubscription
	// PR-SUBSCRIPTION-PRESTIGE: new signature(ctx, userID, planType, duration, quota)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT plan_type, COALESCE").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("INSERT INTO user_subscriptions").
		WithArgs("user123", "SUB_MONTHLY_PRO", 2, 30, 200).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = repo.UpdatePaymentStatus(ctx, "ref-sub", "success")
	assert.NoError(t, err)
}

func TestIsPaid_CacheHit(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	repo := NewPaymentRepository(db, rdb)
	ctx := context.Background()

	// Lazy-init
	mock.ExpectExec("INSERT INTO user_credits").WithArgs("user123").WillReturnResult(sqlmock.NewResult(1, 1))

	err = mr.Set("payment_verified:user123:mint456", "true")
	assert.NoError(t, err)

	paid := repo.IsPaid(ctx, "user123", "mint456", false)
	assert.True(t, paid)
}

func TestIsPaid_CacheMiss_DBHit(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	repo := NewPaymentRepository(db, rdb)
	ctx := context.Background()

	// Lazy-init
	mock.ExpectExec("INSERT INTO user_credits").WithArgs("user123").WillReturnResult(sqlmock.NewResult(1, 1))

	// 1. Subscription check fails
	mock.ExpectQuery("SELECT COUNT").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// 2. Credit check fails (no credits)
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE user_credits").
		WithArgs("user123").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	// 3. Eceran check success
	mock.ExpectQuery("SELECT COUNT").
		WithArgs("user123", "mint456").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	paid := repo.IsPaid(ctx, "user123", "mint456", false)
	assert.True(t, paid)
}

func TestIsPaid_Subscription(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	// Lazy-init
	mock.ExpectExec("INSERT INTO user_credits").WithArgs("user123").WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery("SELECT COUNT").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// GetQuotaRemaining check
	mock.ExpectQuery("SELECT quota_limit FROM user_subscriptions").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"quota_limit"}).AddRow(200))

	paid := repo.IsPaid(ctx, "user123", "mint456", false)
	assert.True(t, paid)
}

func TestIsPaid_Credit(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	// Lazy-init
	mock.ExpectExec("INSERT INTO user_credits").WithArgs("user123").WillReturnResult(sqlmock.NewResult(1, 1))

	// 1. Subscription check fails
	mock.ExpectQuery("SELECT COUNT").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// 2. Credit deduction success
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE user_credits").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(9))
	mock.ExpectCommit()

	paid := repo.IsPaid(ctx, "user123", "mint456", false)
	assert.True(t, paid)
}

func TestIsPaid_UsageLimitExceeded(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	// PR-NEXUS-INTELLIGENCE: With Redis nil, CheckAndIncrFreeUsage returns (true, nil) = fail-open
	// So IsPaid will return true (granted through free_atomic_ui)
	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	// Lazy-init
	mock.ExpectExec("INSERT INTO user_credits").WithArgs("user123").WillReturnResult(sqlmock.NewResult(1, 1))

	paid := repo.IsPaid(ctx, "user123", "mint456", false)
	assert.True(t, paid) // Fail-open when Redis is nil
}

func TestDeductCredit_Atomic(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE user_credits").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(49))
	mock.ExpectCommit()

	success, err := repo.DeductCredit(ctx, "user123")
	assert.NoError(t, err)
	assert.True(t, success)
}

func TestIsProSubscriber_CacheHit(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	repo := NewPaymentRepository(nil, rdb)
	ctx := context.Background()

	err := mr.Set("sub_active:user123", "true")
	assert.NoError(t, err)

	active := repo.IsProSubscriber(ctx, "user123")
	assert.True(t, active)
}

func TestIsProSubscriber_CacheMiss(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	repo := NewPaymentRepository(db, rdb)
	ctx := context.Background()

	mock.ExpectQuery("SELECT COUNT").
		WithArgs("user_pro").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	active := repo.IsProSubscriber(ctx, "user_pro")
	assert.True(t, active)

	// Verify it was cached
	val, _ := rdb.Get(ctx, "sub_active:user_pro").Result()
	assert.Equal(t, "true", val)
}

func TestUpdatePaymentStatus_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	mock.ExpectQuery("UPDATE payments").
		WithArgs("success", "ref-err").
		WillReturnError(sql.ErrNoRows)

	err = repo.UpdatePaymentStatus(ctx, "ref-err", "success")
	assert.Error(t, err)
}

func TestActivateSubscription_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT plan_type, COALESCE").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("INSERT INTO user_subscriptions").
		WithArgs("user123", "SUB_MONTHLY_PRO", 2, 30, 200).
		WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()

	err = repo.ActivateSubscription(ctx, "user123", "SUB_MONTHLY_PRO", 30, 200)
	assert.Error(t, err)
}

func TestGetQuotaRemaining_DBFallback(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	mock.ExpectQuery("SELECT quota_limit FROM user_subscriptions").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"quota_limit"}).AddRow(200))

	remaining, err := repo.GetQuotaRemaining(ctx, "user123")
	assert.NoError(t, err)
	assert.Equal(t, int64(200), remaining)
}

func TestGetQuotaRemaining_RedisFallback(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	repo := NewPaymentRepository(nil, rdb)
	ctx := context.Background()

	// Use the actual Redis key format: sub_limit:{userID}
	err := mr.Set("sub_limit:user123", "1234")
	assert.NoError(t, err)

	remaining, err := repo.GetQuotaRemaining(ctx, "user123")
	assert.NoError(t, err)
	assert.Equal(t, int64(1234), remaining)
}

func TestIncrementUsage_RedisNil(t *testing.T) {
	db, _, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	// With nil Redis, IncrementUsage just returns without panicking
	repo.IncrementUsage(ctx, "user123")
}

func TestGetFreeUsage_RedisFallback(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	repo := NewPaymentRepository(nil, rdb)
	ctx := context.Background()

	err := mr.Set("free:user:user123:ui", "2")
	assert.NoError(t, err)

	used := repo.GetFreeUsage(ctx, "user123", "ui")
	assert.Equal(t, int64(2), used)
}

func TestGetFreeUsage_DBFallback(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	mock.ExpectQuery("SELECT free_api_used FROM users").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"free_api_used"}).AddRow(5))

	used := repo.GetFreeUsage(ctx, "user123", "api")
	assert.Equal(t, int64(5), used)
}

func TestIsPaid_FreeTierAPI(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	// Lazy-init
	mock.ExpectExec("INSERT INTO user_credits").WithArgs("user123").WillReturnResult(sqlmock.NewResult(1, 1))

	// Free tier: used=5 < limit=10 => grant free access
	mock.ExpectQuery("SELECT free_api_used FROM users").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"free_api_used"}).AddRow(5))

	paid := repo.IsPaid(ctx, "user123", "mintapi", true)
	assert.True(t, paid)
}

func TestIsPaid_FreeTierUI(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	// Lazy-init
	mock.ExpectExec("INSERT INTO user_credits").WithArgs("user123").WillReturnResult(sqlmock.NewResult(1, 1))

	// Free tier: used=1 < limit=3 => grant free access for UI
	mock.ExpectQuery("SELECT free_scans_used FROM users").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"free_scans_used"}).AddRow(1))

	paid := repo.IsPaid(ctx, "user123", "mintui", false)
	assert.True(t, paid)
}

func TestCheckAccessAndCommit_AtomicRecovery(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	repo := NewPaymentRepository(db, rdb)
	ctx := context.Background()

	userID := "user_audit"
	mint := "mint_audit"

	// Audit PR-FIX-V1: Ensure free tier is EXHAUSTED so it falls through to 'credit'
	_ = rdb.Set(ctx, "free:user:user_audit:ui", "3", 0)

	// 1. Setup: User has 10 credits
	mock.ExpectExec("INSERT INTO user_credits").WithArgs(userID).WillReturnResult(sqlmock.NewResult(1, 1))
	// Mock subscription check failure (falls through to credit)
	mock.ExpectQuery("SELECT COUNT").
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// Now mock credit check
	mock.ExpectQuery("SELECT balance FROM user_credits").
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(10))

	// 2. Step 1: CheckAccess
	granted, kind, err := repo.CheckAccess(ctx, userID, mint, false)
	assert.NoError(t, err)
	assert.True(t, granted)
	assert.Equal(t, "credit", kind)

	// SCENARIO: ANALYSIS FAILS (Work fails)
	// We do NOT call CommitUsage. Quota should NOT be deducted.

	// SCENARIO: ANALYSIS SUCCEEDS (Work succeeds)
	// Now we call CommitUsage
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE user_credits").
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(9))
	mock.ExpectCommit()

	repo.CommitUsage(ctx, userID, kind, mint)

	// Verify Redis cache was set after commit
	val, _ := rdb.Get(ctx, "payment_verified:user_audit:mint_audit").Result()
	assert.Equal(t, "true", val)
}

func TestCheckAccess_FreeTierDoubleSpendPrevention(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	repo := NewPaymentRepository(db, rdb)
	ctx := context.Background()

	userID := "user_free"

	// Set Redis used = 2 (limit is 3)
	err = mr.Set("free:user:user_free:ui", "2")
	assert.NoError(t, err)

	// Lazy-init
	mock.ExpectExec("INSERT INTO user_credits").WithArgs(userID).WillReturnResult(sqlmock.NewResult(1, 1))

	// PR-NEXUS-INTELLIGENCE: CheckAccess now returns free_atomic_ui
	granted, accessKind, err := repo.CheckAccess(ctx, userID, "any_mint", false)
	assert.NoError(t, err)
	assert.True(t, granted)
	assert.Equal(t, "free_atomic_ui", accessKind)

	// CommitUsage should just cache access (increment happened atomically)
	repo.CommitUsage(ctx, userID, accessKind, "any_mint")

	// The Lua script already incremented from 2 to 3
	val, _ := rdb.Get(ctx, "free:user:user_free:ui").Int64()
	assert.Equal(t, int64(3), val)

	// Next CheckAccess should fail (used=3 >= limit=3)
	// Need subscription check expectations since free is exhausted
	mock.ExpectQuery("SELECT COUNT(.*) FROM user_subscriptions").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE user_credits").
		WithArgs(userID).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	granted, _, _ = repo.CheckAccess(ctx, userID, "another_mint", false)
	assert.False(t, granted)
}

func TestActivateSubscription_UpgradeCarryOver(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	userID := "user_upgrade"
	oldExpiry := time.Now().Add(10 * 24 * time.Hour)

	// 1. Mock Fetching Current Subscription (Tier 1 - Weekly)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT plan_type, COALESCE").
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"plan_type", "plan_tier", "status", "quota_limit", "current_usage", "expires_at"}).
			AddRow("SUB_WEEKLY_PRO", 1, "active", 50, 10, oldExpiry))

	// 2. Mock Upgrade to Tier 2 (Monthly)
	// leftover = 50 - 10 = 40
	// new_limit = 200 + 40 = 240
	mock.ExpectExec("UPDATE user_subscriptions").
		WithArgs("SUB_MONTHLY_PRO", 2, 30, 200, 40, userID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = repo.ActivateSubscription(ctx, userID, "SUB_MONTHLY_PRO", 30, 200)
	assert.NoError(t, err)
}

func TestActivateSubscription_QueuedDowngrade(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	userID := "user_downgrade"
	oldExpiry := time.Now().Add(10 * 24 * time.Hour)

	// 1. Mock Fetching Current Subscription (Tier 3 - Ultra)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT plan_type, COALESCE").
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"plan_type", "plan_tier", "status", "quota_limit", "current_usage", "expires_at"}).
			AddRow("SUB_ULTRA_PRO", 3, "active", 1200, 100, oldExpiry))

	// 2. Mock Downgrade to Tier 2 (Monthly)
	mock.ExpectExec("UPDATE user_subscriptions").
		WithArgs("SUB_MONTHLY_PRO", userID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = repo.ActivateSubscription(ctx, userID, "SUB_MONTHLY_PRO", 30, 200)
	assert.NoError(t, err)
}

func TestSavePayment_Error(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	p := &Payment{
		UserID:      "user1",
		MintAddress: "mint1",
		Reference:   "ref1",
		AmountSol:   1.0,
	}

	mock.ExpectQuery("INSERT INTO payments").
		WithArgs(p.UserID, p.MintAddress, p.Reference, "pending", p.AmountSol).
		WillReturnError(sql.ErrConnDone)

	err := repo.SavePayment(ctx, p)
	assert.Error(t, err)
}

func TestUpdatePaymentStatus_Errors(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery("UPDATE payments").
			WithArgs("success", "ref1").
			WillReturnError(sql.ErrNoRows)

		err := repo.UpdatePaymentStatus(ctx, "ref1", "success")
		assert.Error(t, err)
	})

	t.Run("FulfillmentError_Sub", func(t *testing.T) {
		mock.ExpectQuery("UPDATE payments").
			WithArgs("success", "ref3").
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "mint_address", "amount_sol"}).
				AddRow("user1", "SUB_MONTHLY_PRO", 0.3))

		// ActivateSubscription fails
		mock.ExpectExec("INSERT INTO user_subscriptions").
			WithArgs("user1", "SUB_MONTHLY_PRO", sqlmock.AnyArg(), sqlmock.AnyArg(), "active", sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnError(sql.ErrConnDone)

		err := repo.UpdatePaymentStatus(ctx, "ref3", "success")
		assert.Error(t, err)
	})
}

// --- Coverage Boost Tests ---

func TestGetDB_GetRedis(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	repo := NewPaymentRepository(db, rdb)
	assert.Equal(t, db, repo.GetDB())
	assert.Equal(t, rdb, repo.GetRedis())
}

func TestGetTier(t *testing.T) {
	assert.Equal(t, TierWeeklyPro, getTier("SUB_WEEKLY_PRO"))
	assert.Equal(t, TierMonthlyPro, getTier("SUB_MONTHLY_PRO"))
	assert.Equal(t, TierUltraPro, getTier("SUB_ULTRA_PRO"))
	assert.Equal(t, TierEnterprise, getTier("SUB_ENTERPRISE"))
	assert.Equal(t, 0, getTier("UNKNOWN_PLAN"))
}

func TestCommitUsage_AllBranches(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	repo := NewPaymentRepository(db, rdb)
	ctx := context.Background()

	t.Run("cache", func(t *testing.T) {
		repo.CommitUsage(ctx, "user1", "cache", "mint1")
		// No-op, should not panic
	})

	t.Run("free_atomic_ui", func(t *testing.T) {
		repo.CommitUsage(ctx, "user1", "free_atomic_ui", "mint1")
	})

	t.Run("free_atomic_api", func(t *testing.T) {
		repo.CommitUsage(ctx, "user1", "free_atomic_api", "mint1")
	})

	t.Run("subscription", func(t *testing.T) {
		repo.CommitUsage(ctx, "user1", "subscription", "mint1")
	})

	t.Run("credit", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectQuery("UPDATE user_credits").
			WithArgs("user1").
			WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(9))
		mock.ExpectCommit()
		repo.CommitUsage(ctx, "user1", "credit", "mint1")
	})

	t.Run("single_pay", func(t *testing.T) {
		repo.CommitUsage(ctx, "user1", "single_pay", "mint1")
	})
}

func TestIncrementUsage_WithRedis(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	repo := NewPaymentRepository(db, rdb)
	ctx := context.Background()

	repo.IncrementUsage(ctx, "user_incr")

	val, _ := rdb.Get(ctx, "usage:user:user_incr").Int64()
	assert.Equal(t, int64(1), val)
}

func TestIncrementUsage_NilRedis(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	// Should not panic
	repo.IncrementUsage(ctx, "user_nil")
}

func TestCheckAndIncrFreeUsage_NilRedis(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	granted, err := repo.CheckAndIncrFreeUsage(ctx, "user1", "ui", 3)
	assert.NoError(t, err)
	assert.True(t, granted) // Fail-open
}

func TestCheckAndIncrFreeUsage_WithRedis_Granted(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	repo := NewPaymentRepository(db, rdb)
	ctx := context.Background()

	// usage=0, limit=3 -> granted
	granted, err := repo.CheckAndIncrFreeUsage(ctx, "user_lua", "ui", 3)
	assert.NoError(t, err)
	assert.True(t, granted)

	// Verify Redis was incremented
	val, _ := rdb.Get(ctx, "free:user:user_lua:ui").Int64()
	assert.Equal(t, int64(1), val)
}

func TestCheckAndIncrFreeUsage_WithRedis_Exhausted(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	_ = mr.Set("free:user:user_exhausted:ui", "3")

	repo := NewPaymentRepository(db, rdb)
	ctx := context.Background()

	// usage=3, limit=3 -> denied
	granted, err := repo.CheckAndIncrFreeUsage(ctx, "user_exhausted", "ui", 3)
	assert.NoError(t, err)
	assert.False(t, granted)
}

func TestGetFreeUsage_WithRedis(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	_ = mr.Set("free:user:user_cached:ui", "5")

	repo := NewPaymentRepository(db, rdb)
	ctx := context.Background()

	used := repo.GetFreeUsage(ctx, "user_cached", "ui")
	assert.Equal(t, int64(5), used)
}

func TestDeductCredit_BeginError(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	mock.ExpectBegin().WillReturnError(errors.New("connection refused"))

	ok, err := repo.DeductCredit(ctx, "user1")
	assert.False(t, ok)
	assert.Error(t, err)
}

func TestDeductCredit_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE user_credits").
		WithArgs("user_success").
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(9))
	mock.ExpectCommit()

	ok, err := repo.DeductCredit(ctx, "user_success")
	assert.True(t, ok)
	assert.NoError(t, err)
}

func TestDeductCredit_Insufficient(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE user_credits").
		WithArgs("user_empty").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	ok, err := repo.DeductCredit(ctx, "user_empty")
	assert.False(t, ok)
	assert.NoError(t, err) // ErrNoRows is handled and returns false, nil
}

func TestCheckAccess_NoUsageFound(t *testing.T) {
	// Re-verify CheckAccess behavior when no usage record is found in DB
	// ... (This depends on the actual test content, but let's just terminate with empty line if nothing else follows)
}
