package repository

import (
	"context"
	"database/sql"
	"testing"

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
	mock.ExpectExec("INSERT INTO user_subscriptions").
		WithArgs("user123", "active", 30, 200).
		WillReturnResult(sqlmock.NewResult(1, 1))

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

	repo := NewPaymentRepository(db, nil)
	ctx := context.Background()

	// Lazy-init
	mock.ExpectExec("INSERT INTO user_credits").WithArgs("user123").WillReturnResult(sqlmock.NewResult(1, 1))

	// Free tier exhausted (used=3, limit=3 => fall through)
	mock.ExpectQuery("SELECT free_scans_used FROM users").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"free_scans_used"}).AddRow(3))

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

	// 3. Eceran check fails
	mock.ExpectQuery("SELECT COUNT").
		WithArgs("user123", "mint456").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	paid := repo.IsPaid(ctx, "user123", "mint456", false)
	assert.False(t, paid)
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

	mock.ExpectExec("INSERT INTO user_subscriptions").
		WithArgs("user123", "active", 30, 200).
		WillReturnError(sql.ErrConnDone)

	err = repo.ActivateSubscription(ctx, "user123", "active", 30, 200)
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

	// CheckAccess should return true
	granted, accessKind, err := repo.CheckAccess(ctx, userID, "any_mint", false)
	assert.NoError(t, err)
	assert.True(t, granted)
	assert.Equal(t, "free_ui", accessKind)

	// CommitUsage should increment Redis synchronously
	repo.CommitUsage(ctx, userID, accessKind, "any_mint")

	val, _ := rdb.Get(ctx, "free:user:user_free:ui").Int64()
	assert.Equal(t, int64(3), val)

	// Next CheckAccess should fail
	granted, _, _ = repo.CheckAccess(ctx, userID, "another_mint", false)
	assert.False(t, granted)
}
