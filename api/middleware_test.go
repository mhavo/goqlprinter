package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTokenAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		token      string
		authHeader string
		wantCode   int
	}{
		{"valid token", "secret123", "Bearer secret123", http.StatusOK},
		{"missing header", "secret123", "", http.StatusUnauthorized},
		{"wrong token", "secret123", "Bearer wrong", http.StatusUnauthorized},
		{"no bearer prefix", "secret123", "secret123", http.StatusUnauthorized},
		{"empty bearer", "secret123", "Bearer ", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(TokenAuth(tt.token))
			r.GET("/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("got status %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}
