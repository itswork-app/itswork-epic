package ingestor

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"itswork.app/internal/pay"
	"itswork.app/internal/repository"
)

// CreatePaymentHandler initiates a Solana Pay session
func CreatePaymentHandler(c *gin.Context, payService *pay.PayService, payRepo *repository.PaymentRepository) {
	mint := c.Query("mint")
	if mint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing mint parameter"})
		return
	}

	userID := c.GetHeader("X-User-Id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required (X-User-Id missing)"})
		return
	}

	payURL, reference := payService.GeneratePaymentURL(mint)

	price, _ := strconv.ParseFloat(payService.ScanPrice, 64)

	payment := &repository.Payment{
		UserID:      userID,
		MintAddress: mint,
		Reference:   reference,
		AmountSol:   price,
	}

	if err := payRepo.SavePayment(c.Request.Context(), payment); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create payment session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"payment_url": payURL,
		"reference":   reference,
		"amount":      payService.ScanPrice,
		"currency":    "SOL",
	})
}

// CreateBundlePaymentHandler initiates a payment for credit bundles
func CreateBundlePaymentHandler(c *gin.Context, payService *pay.PayService, payRepo *repository.PaymentRepository) {
	bundleType := c.Query("type") // BUNDLE_50, BUNDLE_100
	if bundleType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing bundle type parameter"})
		return
	}

	userID := c.GetHeader("X-User-Id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	payURL, reference := payService.GenerateBundlePaymentURL(userID, bundleType)

	// Note: We don't save to 'payments' table yet if it's not a single-scan payment,
	// or we can use a different table 'pending_purchases'.
	// For simplicity, we'll use the reference on-chain to verify later.

	c.JSON(http.StatusOK, gin.H{
		"payment_url": payURL,
		"reference":   reference,
		"type":        bundleType,
	})
}

// CreateSubscriptionPaymentHandler initiates a payment for Pro subscription
func CreateSubscriptionPaymentHandler(c *gin.Context, payService *pay.PayService, payRepo *repository.PaymentRepository) {
	planType := c.DefaultQuery("plan", "SUB_MONTHLY_PRO")

	userID := c.GetHeader("X-User-Id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	payURL, reference := payService.GenerateSubscriptionPaymentURL(userID, planType)

	c.JSON(http.StatusOK, gin.H{
		"payment_url": payURL,
		"reference":   reference,
		"plan":        planType,
	})
}

// VerifyPaymentHandler checks if a transaction is finalized on-chain
func VerifyPaymentHandler(c *gin.Context, payService *pay.PayService, payRepo *repository.PaymentRepository) {
	reference := c.Param("reference")
	if reference == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing reference parameter"})
		return
	}

	success, err := payService.VerifyTransaction(c.Request.Context(), reference)
	if err != nil {
		log.Error().Err(err).Str("ref", reference).Msg("On-chain verification failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Verification service unavailable"})
		return
	}

	if success {
		if updateErr := payRepo.UpdatePaymentStatus(c.Request.Context(), reference, "success"); updateErr != nil {
			log.Error().Err(updateErr).Str("ref", reference).Msg("Failed to update payment status in DB")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update payment records"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Payment verified and analysis unlocked"})
	} else {
		c.JSON(http.StatusAccepted, gin.H{"status": "pending", "message": "Transaction not found or not finalized yet"})
	}
}
