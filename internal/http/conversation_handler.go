package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type ConversationHandler struct {
}

func NewConversationHandler() *ConversationHandler {
	return &ConversationHandler{}
}

type conversationSummary struct {
	ID           string `json:"id"`
	Preview      string `json:"preview"`
	MessageCount int    `json:"messageCount"`
	CreatedAt    string `json:"createdAt"`
	TokenUsage   int    `json:"tokenUsage"`
}

type conversationDetail struct {
	ID       string                  `json:"id"`
	Messages []conversationMessage   `json:"messages"`
	Metadata map[string]interface{}  `json:"metadata,omitempty"`
}

type conversationMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

// List returns static conversation summaries for now.
func (h *ConversationHandler) List(c *gin.Context) {
	data := []conversationSummary{
		{ID: "conv-1", Preview: "API 인증 방법에 대해 알려주세요", MessageCount: 8, CreatedAt: "2025-12-09T14:32:00Z", TokenUsage: 1200},
		{ID: "conv-2", Preview: "문서 업로드가 안 됩니다", MessageCount: 5, CreatedAt: "2025-12-09T13:15:00Z", TokenUsage: 850},
		{ID: "conv-3", Preview: "벡터 검색 결과가 부정확해요", MessageCount: 12, CreatedAt: "2025-12-09T11:48:00Z", TokenUsage: 2100},
		{ID: "conv-4", Preview: "JWT 토큰 갱신은 어떻게 하나요?", MessageCount: 6, CreatedAt: "2025-12-09T10:22:00Z", TokenUsage: 980},
		{ID: "conv-5", Preview: "파일 형식 제한이 있나요?", MessageCount: 4, CreatedAt: "2025-12-08T16:45:00Z", TokenUsage: 620},
	}

	SuccessResponse(c, gin.H{
		"conversations": data,
	})
}

// Detail returns static messages for a given conversation id.
func (h *ConversationHandler) Detail(c *gin.Context) {
	id := c.Param("id")
	now := time.Now().UTC()

	messages := []conversationMessage{
		{Role: "user", Content: "API 인증 방법에 대해 알려주세요", Timestamp: now.Add(-10 * time.Minute).Format(time.RFC3339)},
		{Role: "assistant", Content: "API 인증은 JWT를 사용합니다. /auth/login 엔드포인트로 이메일과 비밀번호를 보내 토큰을 발급받으세요.", Timestamp: now.Add(-9 * time.Minute).Format(time.RFC3339)},
		{Role: "user", Content: "토큰은 어디에 포함시켜야 하나요?", Timestamp: now.Add(-8 * time.Minute).Format(time.RFC3339)},
		{Role: "assistant", Content: "Authorization 헤더에 Bearer {token} 형식으로 포함하세요.", Timestamp: now.Add(-7 * time.Minute).Format(time.RFC3339)},
	}

	SuccessResponse(c, conversationDetail{
		ID:       id,
		Messages: messages,
	})
}
