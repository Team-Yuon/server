package rag

type Document struct {
	ID       string                 `json:"id"`
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata"`
	Score    float64                `json:"score,omitempty"`
	FileKey  string                 `json:"fileKey,omitempty"`
	FileURL  string                 `json:"fileUrl,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"` // user, assistant, system
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

type DocumentListParams struct {
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	Query    string `json:"query,omitempty"`
	Category string `json:"category,omitempty"`
}

type DocumentListResult struct {
	Documents []Document `json:"documents"`
	Total     int64      `json:"total"`
	Page      int        `json:"page"`
	PageSize  int        `json:"pageSize"`
	HasNext   bool       `json:"hasNext"`
}

type DocumentStats struct {
	TotalDocuments int64  `json:"totalDocuments"`
	Index          string `json:"index"`
	LastUpdatedAt  string `json:"lastUpdatedAt,omitempty"`
}

type DashboardStats struct {
	TotalDocuments     int64   `json:"total_documents"`
	TotalConversations int64   `json:"total_conversations"`
	ActiveUsers        int64   `json:"active_users"`
	AvgResponseTime    float64 `json:"avg_response_time,omitempty"`
	// Trends (compared to previous period)
	DocumentsTrend     float64 `json:"documents_trend,omitempty"`
	ConversationsTrend float64 `json:"conversations_trend,omitempty"`
	ActiveUsersTrend   float64 `json:"active_users_trend,omitempty"`
	ResponseTimeTrend  float64 `json:"response_time_trend,omitempty"`
}

type ReindexRequest struct {
	DocumentIDs []string `json:"documentIds"`
}

type ReindexResult struct {
	Requested int      `json:"requested"`
	Reindexed int      `json:"reindexed"`
	Failed    []string `json:"failed,omitempty"`
}

type DocumentVector struct {
	ID       string                 `json:"id"`
	Vector   []float32              `json:"vector"`
	Content  string                 `json:"content,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type VectorQueryRequest struct {
	DocumentIDs []string `json:"documentIds,omitempty"`
	Limit       int      `json:"limit,omitempty"`
	WithPayload bool     `json:"withPayload"`
	Offset      string   `json:"offset,omitempty"`
}

type VectorQueryResponse struct {
	Vectors    []DocumentVector `json:"vectors"`
	Count      int              `json:"count"`
	HasMore    bool             `json:"hasMore"`
	NextOffset string           `json:"nextOffset,omitempty"`
}

type VectorProjectionRequest struct {
	Limit       int    `json:"limit,omitempty"`
	Offset      string `json:"offset,omitempty"`
	WithPayload bool   `json:"withPayload"`
}

type ProjectedVector struct {
	ID        string                 `json:"id"`
	X         float64                `json:"x"`
	Y         float64                `json:"y"`
	Content   string                 `json:"content,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Magnitude float64                `json:"magnitude,omitempty"`
}

type VectorProjectionResponse struct {
	Vectors    []ProjectedVector `json:"vectors"`
	Count      int               `json:"count"`
	HasMore    bool              `json:"hasMore"`
	NextOffset string            `json:"nextOffset,omitempty"`
}
