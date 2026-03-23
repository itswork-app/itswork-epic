package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func TestSavePayment_Success(t *testing.T) {
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
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "token_mint"}).AddRow("user123", "mint456"))

	err = repo.UpdatePaymentStatus(ctx, reference, status)
	assert.NoError(t, err)

	// Verify Redis cache
	val, err := rdb.Get(ctx, "payment_verified:user123:mint456").Result()
	assert.NoError(t, err)
	assert.Equal(t, "true", val)
}

func TestIsPaid_CacheHit(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer mr.Close()

	repo := NewPaymentRepository(nil, rdb)
	ctx := context.Background()

	mr.Set("payment_verified:user123:mint456", "true")

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

	mock.ExpectQuery("SELECT COUNT").
		WithArgs("user123", "mint456").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	paid := repo.IsPaid(ctx, "user123", "mint456")
	assert.True(t, paid)

	// Verify it was cached
	val, _ := rdb.Get(ctx, "payment_verified:user123:mint456").Result()
	assert.Equal(t, "true", val)
}
