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

	userID := GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	payURL, reference, amountStr := payService.GeneratePaymentURL(c.Request.Context(), mint)

	price, _ := strconv.ParseFloat(amountStr, 64)

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
		"amount":      amountStr,
		"currency":    "SOL",
	})
}

// CreateBundlePaymentHandler is removed in Nexus V1 Final Spec.
// Bundle credits are deprecated. Use CreateSubscriptionPaymentHandler instead.

// CreateSubscriptionPaymentHandler initiates a payment for Pro subscription
func CreateSubscriptionPaymentHandler(c *gin.Context, payService *pay.PayService, payRepo *repository.PaymentRepository) {
	planType := c.DefaultQuery("plan", "SUB_MONTHLY_PRO")

	userID := GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	payURL, reference, amountStr := payService.GenerateSubscriptionPaymentURL(c.Request.Context(), userID, planType)

	// Save pending payment record
	amount, _ := strconv.ParseFloat(amountStr, 64)
	payment := &repository.Payment{
		UserID:      userID,
		MintAddress: planType,
		Reference:   reference,
		AmountSol:   amount,
	}
	if err := payRepo.SavePayment(c.Request.Context(), payment); err != nil {
		log.Error().Err(err).Str("user", userID).Str("plan", planType).Msg("Failed to save pending sub record")
	}
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
