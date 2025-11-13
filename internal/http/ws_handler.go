package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"yuon/internal/rag"
	"yuon/internal/rag/service"
)

type WebSocketHandler struct {
	service *service.ChatbotService
}

func NewWebSocketHandler(service *service.ChatbotService) *WebSocketHandler {
	return &WebSocketHandler{service: service}
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type wsEnvelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type startConversationPayload struct {
	ConversationID string `json:"conversation_id,omitempty"`
}

type appendMessagePayload struct {
	ConversationID  string            `json:"conversation_id,omitempty"`
	MessageID       string            `json:"message_id,omitempty"`
	Message         string            `json:"message"`
	UseVectorSearch *bool             `json:"use_vector_search,omitempty"`
	UseFullText     *bool             `json:"use_full_text,omitempty"`
	TopK            int               `json:"top_k,omitempty"`
	History         []rag.ChatMessage `json:"history,omitempty"`
}

type wsErrorPayload struct {
	Message string `json:"message"`
}

type messageAckPayload struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
}

type streamChunkPayload struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	Chunk          string `json:"chunk"`
	Index          int    `json:"index"`
}

type streamEndPayload struct {
	ConversationID string         `json:"conversation_id"`
	MessageID      string         `json:"message_id"`
	Answer         string         `json:"answer"`
	Sources        []rag.Document `json:"sources,omitempty"`
	TokensUsed     int            `json:"tokens_used,omitempty"`
}

func (h *WebSocketHandler) Handle(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		slog.Error("웹소켓 업그레이드 실패", "error", err)
		return
	}
	defer conn.Close()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			slog.Warn("웹소켓 연결 종료", "error", err)
			break
		}

		var envelope wsEnvelope
		if err := json.Unmarshal(data, &envelope); err != nil {
			h.sendError(conn, "잘못된 메시지 형식입니다")
			continue
		}

		switch envelope.Type {
		case "start_conversation":
			h.handleStartConversation(conn, envelope.Payload)
		case "append_message":
			h.handleAppendMessage(conn, envelope.Payload)
		case "typing":
			h.handleTyping(conn, envelope.Payload)
		case "end_conversation":
			h.handleEndConversation(conn, envelope.Payload)
		default:
			h.sendError(conn, "알 수 없는 이벤트 타입입니다")
		}
	}
}

func (h *WebSocketHandler) handleStartConversation(conn *websocket.Conn, payload json.RawMessage) {
	req := startConversationPayload{}
	_ = json.Unmarshal(payload, &req)

	if req.ConversationID == "" {
		req.ConversationID = uuid.New().String()
	}

	h.sendSystemNotice(conn, req.ConversationID, "conversation_started")
}

func (h *WebSocketHandler) handleAppendMessage(conn *websocket.Conn, payload json.RawMessage) {
	var req appendMessagePayload
	if err := json.Unmarshal(payload, &req); err != nil {
		h.sendError(conn, "잘못된 요청 데이터입니다")
		return
	}

	if req.Message == "" {
		h.sendError(conn, "message 필드는 필수입니다")
		return
	}

	if req.ConversationID == "" {
		req.ConversationID = uuid.New().String()
	}
	if req.MessageID == "" {
		req.MessageID = uuid.New().String()
	}

	h.write(conn, wsEnvelope{
		Type:    "message_ack",
		Payload: mustMarshal(messageAckPayload{ConversationID: req.ConversationID, MessageID: req.MessageID}),
	})

	useVector := true
	useFullText := true

	if req.UseVectorSearch != nil {
		useVector = *req.UseVectorSearch
	}
	if req.UseFullText != nil {
		useFullText = *req.UseFullText
	}

	if !useVector && !useFullText {
		useVector = true
		useFullText = true
	}

	existingHistory := h.service.ConversationHistory(req.ConversationID)
	if len(req.History) > 0 {
		existingHistory = append(existingHistory, req.History...)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	resp, err := h.service.Chat(ctx, &rag.ChatRequest{
		Message:         req.Message,
		ConversationID:  req.ConversationID,
		UseVectorSearch: useVector,
		UseFullText:     useFullText,
		TopK:            req.TopK,
		History:         existingHistory,
	})
	if err != nil {
		slog.Error("웹소켓 챗 처리 실패", "error", err)
		h.sendError(conn, "응답 생성에 실패했습니다")
		return
	}

	h.service.AppendConversationMessage(req.ConversationID, rag.ChatMessage{
		Role:    "user",
		Content: req.Message,
	})

	chunks := splitString(resp.Answer, 200)
	for idx, chunk := range chunks {
		h.write(conn, wsEnvelope{
			Type: "stream_chunk",
			Payload: mustMarshal(streamChunkPayload{
				ConversationID: resp.ConversationID,
				MessageID:      req.MessageID,
				Chunk:          chunk,
				Index:          idx,
			}),
		})
	}

	h.write(conn, wsEnvelope{
		Type: "stream_end",
		Payload: mustMarshal(streamEndPayload{
			ConversationID: resp.ConversationID,
			MessageID:      req.MessageID,
			Answer:         resp.Answer,
			Sources:        resp.Sources,
			TokensUsed:     resp.TokensUsed,
		}),
	})
	h.service.AppendConversationMessage(req.ConversationID, rag.ChatMessage{
		Role:    "assistant",
		Content: resp.Answer,
	})
}

func (h *WebSocketHandler) sendError(conn *websocket.Conn, msg string) {
	response := wsEnvelope{
		Type:    "error",
		Payload: mustMarshal(wsErrorPayload{Message: msg}),
	}
	h.write(conn, response)
}

func (h *WebSocketHandler) handleTyping(conn *websocket.Conn, payload json.RawMessage) {
	var req struct {
		ConversationID string `json:"conversation_id,omitempty"`
	}
	_ = json.Unmarshal(payload, &req)
	h.sendSystemNotice(conn, req.ConversationID, "typing 이벤트가 수신되었습니다")
}

func (h *WebSocketHandler) handleEndConversation(conn *websocket.Conn, payload json.RawMessage) {
	var req struct {
		ConversationID string `json:"conversation_id,omitempty"`
	}
	_ = json.Unmarshal(payload, &req)
	h.service.CloseConversation(req.ConversationID)
	h.sendSystemNotice(conn, req.ConversationID, "conversation_closed")
}

func (h *WebSocketHandler) sendSystemNotice(conn *websocket.Conn, conversationID, message string) {
	payload := map[string]string{
		"message": message,
	}
	if conversationID != "" {
		payload["conversation_id"] = conversationID
	}
	h.write(conn, wsEnvelope{
		Type:    "system_notice",
		Payload: mustMarshal(payload),
	})
}

func (h *WebSocketHandler) write(conn *websocket.Conn, envelope wsEnvelope) {
	if err := conn.WriteJSON(envelope); err != nil {
		slog.Error("웹소켓 전송 실패", "error", err)
	}
}

func mustMarshal(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

func splitString(text string, size int) []string {
	if size <= 0 {
		size = 200
	}

	runes := []rune(text)
	if len(runes) == 0 {
		return []string{""}
	}

	if len(runes) <= size {
		return []string{text}
	}

	var chunks []string
	for start := 0; start < len(runes); start += size {
		end := start + size
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}
