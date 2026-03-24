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
		WithArgs("user123", "active").
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

	paid := repo.IsPaid(ctx, "user123", "mint456")
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
	mock.ExpectQuery("SELECT COUNT(.*) FROM user_subscriptions").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// 2. Credit check fails (no credits)
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE user_credits").
		WithArgs("user123").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	// 3. Eceran check success
	mock.ExpectQuery("SELECT COUNT(.*) FROM payments").
		WithArgs("user123", "mint456").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	paid := repo.IsPaid(ctx, "user123", "mint456")
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

	mock.ExpectQuery("SELECT COUNT(.*) FROM user_subscriptions").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	paid := repo.IsPaid(ctx, "user123", "mint456")
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
	mock.ExpectQuery("SELECT COUNT(.*) FROM user_subscriptions").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// 2. Credit deduction success
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE user_credits").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(9))
	mock.ExpectCommit()

	paid := repo.IsPaid(ctx, "user123", "mint456")
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

	// 1. Subscription check fails
	mock.ExpectQuery("SELECT COUNT(.*) FROM user_subscriptions").
		WithArgs("user123").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// 2. Credit check fails (no credits)
	mock.ExpectBegin()
	mock.ExpectQuery("UPDATE user_credits").
		WithArgs("user123").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	// 3. Eceran check fails
	mock.ExpectQuery("SELECT COUNT(.*) FROM payments").
		WithArgs("user123", "mint456").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	paid := repo.IsPaid(ctx, "user123", "mint456")
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

	mock.ExpectQuery("SELECT COUNT(.*) FROM user_subscriptions").
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
		WithArgs("user123", "active", 30).
		WillReturnError(sql.ErrConnDone)

	err = repo.ActivateSubscription(ctx, "user123", "active", 30)
	assert.Error(t, err)
}
