package llm

import (
	"context"
	"fmt"

	"yuon/configuration"
	"yuon/internal/rag"

	"github.com/sashabaranov/go-openai"
)

type OpenAIClient struct {
	client *openai.Client
	config *configuration.OpenAIConfig
}

func NewOpenAIClient(cfg *configuration.OpenAIConfig) *OpenAIClient {
	return &OpenAIClient{
		client: openai.NewClient(cfg.APIKey),
		config: cfg,
	}
}

func (c *OpenAIClient) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	resp, err := c.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(c.config.EmbeddingModel),
		Input: []string{text},
	})
	if err != nil {
		return nil, fmt.Errorf("임베딩 생성 실패: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("임베딩 결과가 비어있습니다")
	}

	return resp.Data[0].Embedding, nil
}

func (c *OpenAIClient) Chat(ctx context.Context, messages []rag.ChatMessage, documents []rag.Document) (string, int, error) {
	systemPrompt := c.buildSystemPrompt(documents)

	openaiMessages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		},
	}

	for _, msg := range messages {
		openaiMessages = append(openaiMessages, openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       c.config.Model,
		Messages:    openaiMessages,
		MaxTokens:   c.config.MaxTokens,
		Temperature: c.config.Temperature,
	})
	if err != nil {
		return "", 0, fmt.Errorf("채팅 생성 실패: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", 0, fmt.Errorf("응답이 비어있습니다")
	}

	return resp.Choices[0].Message.Content, resp.Usage.TotalTokens, nil
}

func (c *OpenAIClient) buildSystemPrompt(documents []rag.Document) string {
	if len(documents) == 0 {
		return `당신은 친절하고 도움이 되는 AI 어시스턴트입니다.
				사용자의 질문에 정확하고 유용한 답변을 제공하세요.`
	}

	prompt := `당신은 제공된 문서를 기반으로 답변하는 AI 어시스턴트입니다.

				다음 규칙을 따르세요:
				1. 제공된 문서의 내용을 바탕으로 답변하세요
				2. 답변할 수 없다면 솔직하게 "제공된 정보로는 답변하기 어렵습니다"라고 말하세요
				3. 가능한 한 구체적이고 명확하게 답변하세요

				참고 문서:
`

	for i, doc := range documents {
		prompt += fmt.Sprintf("\n[문서 %d]\n%s\n", i+1, doc.Content)
	}

	return prompt
}
