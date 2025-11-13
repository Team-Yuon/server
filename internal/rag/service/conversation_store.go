package service

import (
	"sync"

	"yuon/internal/rag"
)

type ConversationStore struct {
	mu        sync.RWMutex
	histories map[string][]rag.ChatMessage
}

func NewConversationStore() *ConversationStore {
	return &ConversationStore{
		histories: make(map[string][]rag.ChatMessage),
	}
}

func (s *ConversationStore) Append(conversationID string, msg rag.ChatMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.histories[conversationID] = append(s.histories[conversationID], msg)
}

func (s *ConversationStore) History(conversationID string) []rag.ChatMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	history := s.histories[conversationID]
	if len(history) == 0 {
		return nil
	}

	clone := make([]rag.ChatMessage, len(history))
	copy(clone, history)
	return clone
}

func (s *ConversationStore) End(conversationID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.histories, conversationID)
}
