package llm

import (
	"context"
	"fmt"
	"strings"

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

func (c *OpenAIClient) GenerateText(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
	if maxTokens == 0 {
		maxTokens = c.config.MaxTokens
	}
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
		{Role: openai.ChatMessageRoleUser, Content: userPrompt},
	}

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       c.config.Model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: 0.2,
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("응답이 비어있습니다")
	}
	return resp.Choices[0].Message.Content, nil
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

func (c *OpenAIClient) ClassifyCategory(ctx context.Context, content string) (string, error) {
	systemPrompt := `당신은 문서를 간단한 카테고리로 분류하는 어시스턴트입니다.
- 결과는 10자 이내의 한 단어 또는 짧은 구로만 답하세요.
- 설명이나 추가 문장은 포함하지 마세요.
- 적절한 카테고리가 떠오르지 않으면 "기타"라고 답하세요.
`

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.config.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: content},
		},
		MaxTokens:   16,
		Temperature: 0,
	})
	if err != nil {
		return "", fmt.Errorf("카테고리 분류 실패: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("카테고리 응답이 비어있습니다")
	}

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

// ExtractKeywords returns a small set of comma-separated keywords using the LLM.
func (c *OpenAIClient) ExtractKeywords(ctx context.Context, text string, maxKeywords int) ([]string, error) {
	if maxKeywords <= 0 {
		maxKeywords = 8
	}

	systemPrompt := fmt.Sprintf(`당신은 키워드 추출기입니다.
- 입력 문장에서 의미 있는 핵심 키워드 %d개 이내를 쉼표로 구분해 출력하세요.
- 조사/어미/구두점은 제거하고 명사/핵심 구만 남기세요.
- 추가 설명 없이 키워드만 출력하세요.`, maxKeywords)

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.config.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: text},
		},
		MaxTokens:   64,
		Temperature: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("키워드 추출 실패: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("키워드 추출 응답이 비어있습니다")
	}

	raw := resp.Choices[0].Message.Content
	parts := strings.Split(raw, ",")
	var keywords []string
	for _, p := range parts {
		k := strings.TrimSpace(p)
		if k != "" {
			keywords = append(keywords, k)
		}
	}
	return keywords, nil
}
