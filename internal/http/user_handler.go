package http

import (
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

type createUserRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	Role     string `json:"role"`
}

type updateUserRequest struct {
	Email string `json:"email,omitempty" binding:"omitempty,email"`
	Role  string `json:"role,omitempty"`
}

// List returns users from the auth manager (in-memory) with basic metadata.
func (h *UserHandler) List(c *gin.Context) {
	if h.manager == nil {
		InternalServerErrorResponse(c, "인증 관리자가 설정되지 않았습니다")
		return
	}

	users := h.manager.AllUsers()
	var resp []userResponse

	for _, u := range users {
		created := u.CreatedAt
		if created.IsZero() {
			created = time.Now().UTC()
		}
		resp = append(resp, userResponse{
			ID:         u.ID,
			Name:       u.Email, // 이름 데이터가 없어 이메일을 이름으로 사용
			Email:      u.Email,
			Role:       u.Role,
			Status:     "active",
			LastActive: "방금 전",
			CreatedAt:  created.Format(time.RFC3339),
		})
	}

	SuccessResponse(c, gin.H{
		"users": resp,
	})
}

func (h *UserHandler) Create(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequestResponse(c, "잘못된 요청입니다")
		return
	}

	_, user, err := h.manager.Signup(req.Email, req.Password, req.Role)
	if err != nil {
		InternalServerErrorResponse(c, err.Error())
		return
	}

	SuccessResponse(c, gin.H{
		"id":      user.ID,
		"email":   user.Email,
		"role":    user.Role,
		"message": "사용자가 생성되었습니다",
	})
}

func (h *UserHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		BadRequestResponse(c, "사용자 ID가 필요합니다")
		return
	}

	if err := h.manager.DeleteUser(id); err != nil {
		InternalServerErrorResponse(c, err.Error())
		return
	}

	SuccessResponse(c, gin.H{
		"message": "사용자가 삭제되었습니다",
	})
}
