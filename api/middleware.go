package api

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

// TokenAuth returns a Gin middleware that requires a valid Bearer token
// on all requests. Uses constant-time comparison to prevent timing attacks.
func TokenAuth(token string) gin.HandlerFunc {
	expected := []byte("Bearer " + token)
	return func(c *gin.Context) {
		got := []byte(c.GetHeader("Authorization"))
		if subtle.ConstantTimeCompare(got, expected) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}
