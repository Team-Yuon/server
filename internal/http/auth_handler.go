package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"yuon/internal/auth"
)

type AuthHandler struct {
	manager *auth.Manager
}

func NewAuthHandler(manager *auth.Manager) *AuthHandler {
	return &AuthHandler{manager: manager}
}

type signupRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Signup(c *gin.Context) {
	if h.manager == nil {
		InternalServerErrorResponse(c, "인증 관리자가 설정되지 않았습니다")
		return
	}

	var req signupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequestResponse(c, "잘못된 요청 형식입니다")
		return
	}

	token, user, err := h.manager.Signup(req.Email, req.Password, req.Role)
	if err != nil {
		ErrorResponse(c, http.StatusBadRequest, "SIGNUP_FAILED", err.Error())
		return
	}

	SuccessResponse(c, gin.H{
		"token": token,
		"user": gin.H{
			"id":    user.ID,
			"email": user.Email,
			"role":  user.Role,
		},
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	if h.manager == nil {
		InternalServerErrorResponse(c, "인증 관리자가 설정되지 않았습니다")
		return
	}

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequestResponse(c, "잘못된 요청 형식입니다")
		return
	}

	jwtToken, user, err := h.manager.Login(req.Email, req.Password)
	if err != nil {
		ErrorResponse(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", err.Error())
		return
	}

	SuccessResponse(c, gin.H{
		"token": jwtToken,
		"user": gin.H{
			"id":    user.ID,
			"email": user.Email,
			"role":  user.Role,
		},
	})
}
