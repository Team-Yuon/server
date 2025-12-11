package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"yuon/internal/auth"
)

type UserHandler struct {
	manager *auth.Manager
}

func NewUserHandler(manager *auth.Manager) *UserHandler {
	return &UserHandler{manager: manager}
}

type userResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	Role       string `json:"role"`
	Status     string `json:"status"`
	LastActive string `json:"lastActive"`
	CreatedAt  string `json:"createdAt"`
}

// List returns users from the auth manager (in-memory) with basic metadata.
func (h *UserHandler) List(c *gin.Context) {
	if h.manager == nil {
		InternalServerErrorResponse(c, "인증 관리자가 설정되지 않았습니다")
		return
	}

	users := h.manager.AllUsers()
	var resp []userResponse

	now := time.Now().UTC().Format("2006-01-02")
	for _, u := range users {
		resp = append(resp, userResponse{
			ID:         u.ID,
			Name:       u.Email, // 이름 데이터가 없어 이메일을 이름으로 사용
			Email:      u.Email,
			Role:       u.Role,
			Status:     "active",
			LastActive: "방금 전",
			CreatedAt:  now,
		})
	}

	SuccessResponse(c, gin.H{
		"users": resp,
	})
}
