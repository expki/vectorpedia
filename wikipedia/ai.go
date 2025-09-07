package wikipedia

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/expki/vectorpedia/ai"
	"github.com/expki/vectorpedia/compute"
)

// GenerateSummary generates a summary for a Wikipedia article using AI
func (w *Wikipedia) GenerateSummary(ctx context.Context, title, content string) (string, error) {

	req := &ai.ChatRequest{
		Messages: []ai.ChatMessage{
			{
				Role:    "system",
				Content: "You are a helpful assistant that creates concise summaries of Wikipedia articles. Provide a clear, informative summary in 2-3 sentences.",
			},
			{
				Role:    "user",
				Content: fmt.Sprintf("Please provide a concise summary of this Wikipedia article titled '%s':\n\n", title),
			},
		},
		MaxTokens:   int(w.contextSizeEmbed),
		Temperature: 0.3,
	}
	allowance := int(w.contextSizeChat)
	for _, message := range req.Messages {
		allowance -= estimateTokensConservative(message.Role)
		allowance -= estimateTokensConservative(message.Content)
	}
	req.Messages[len(req.Messages)-1].Content += truncate(content, allowance)

	resp, err := w.client.Chat(ctx, req)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from AI")
	}

	return resp.Choices[0].Message.Content, nil
}

// GenerateEmbedding generates an embedding for the given text
func (w *Wikipedia) GenerateEmbedding(ctx context.Context, texts []string) ([][]byte, error) {
	output := make([][]byte, len(texts))
	errors := make([]error, len(texts))

	var wg sync.WaitGroup
	wg.Add(len(texts))

	for idx, text := range texts {
		go func(idx int, safeText string) {
			defer wg.Done()

			req := &ai.EmbedRequest{
				Input: []string{safeText},
			}
			resp, err := w.client.Embed(ctx, req)
			if err != nil {
				errors[idx] = err
				return
			}
			if len(resp.Data) == 0 {
				errors[idx] = fmt.Errorf("no embedding returned")
				return
			}
			output[idx] = compute.QuantizeVectorFloat32(resp.Data[0].Embedding)
		}(idx, truncate(text, int(w.contextSizeEmbed)))
	}
	wg.Wait()

	// Check for any errors
	for idx, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("failed to embed text %d: %w", idx, err)
		}
	}

	return output, nil
}

func estimateTokensConservative(text string) int {
	if len(text) == 0 {
		return 0
	}

	// assume 2.5 chars per token
	charCount := len(text)
	estimatedTokens := int(math.Ceil(float64(charCount) / 2.5))

	if estimatedTokens == 0 {
		return 1
	}

	return estimatedTokens
}

func estimateContentLength(tokens int) int {
	// assume 2.5 chars per token
	return int(math.Floor(float64(tokens) * 2.5))
}

func truncate(text string, maxTokens int) string {
	chars := int(math.Floor(float64(maxTokens) * 2.5))
	if len(text) > chars {
		return text[:chars]
	}
	return text
}
