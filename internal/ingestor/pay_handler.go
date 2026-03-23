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
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Payment verified and analysis unlocked"})
	} else {
		c.JSON(http.StatusAccepted, gin.H{"status": "pending", "message": "Transaction not found or not finalized yet"})
	}
}
