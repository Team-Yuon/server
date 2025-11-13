package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"yuon/internal/auth"
)

func authMiddleware(manager *auth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			InternalServerErrorResponse(c, "인증 구성이 설정되지 않았습니다")
			c.Abort()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			ErrorResponse(c, http.StatusUnauthorized, "UNAUTHENTICATED", "Bearer 토큰이 필요합니다")
			c.Abort()
			return
		}

		token := strings.TrimSpace(authHeader[7:])
		claims, err := manager.ValidateJWT(token)
		if err != nil {
			ErrorResponse(c, http.StatusForbidden, "FORBIDDEN", err.Error())
			c.Abort()
			return
		}

		c.Set("userID", claims.Subject)
		c.Set("userRole", claims.Role)
		c.Next()
	}
}
