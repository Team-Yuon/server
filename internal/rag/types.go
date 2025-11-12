package rag

type Document struct {
	ID       string                 `json:"id"`
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata"`
	Score    float64                `json:"score,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`    // user, assistant, system
	Content string `json:"content"`
}

type ChatRequest struct {
	Message         string        `json:"message" binding:"required"`
	ConversationID  string        `json:"conversationId,omitempty"`
	UseVectorSearch bool          `json:"useVectorSearch"`
	UseFullText     bool          `json:"useFullText"`
	TopK            int           `json:"topK,omitempty"`
	History         []ChatMessage `json:"history,omitempty"`
}

type ChatResponse struct {
	Answer         string     `json:"answer"`
	ConversationID string     `json:"conversationId"`
	Sources        []Document `json:"sources,omitempty"`
	TokensUsed     int        `json:"tokensUsed,omitempty"`
}
