package wikipedia

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/expki/vectorpedia/ai"
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
func (w *Wikipedia) GenerateEmbedding(ctx context.Context, text string) ([]byte, error) {
	req := &ai.EmbedRequest{
		Input: []string{truncate(text, int(w.contextSizeEmbed))},
	}

	resp, err := w.client.Embed(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	// Convert float32 slice to byte slice for storage
	embedding := resp.Data[0].Embedding
	bytes := make([]byte, len(embedding)*4)
	for i, f := range embedding {
		// Convert float32 to bytes (little-endian)
		bits := math.Float32bits(f)
		binary.LittleEndian.PutUint32(bytes[i*4:], bits)
	}

	return bytes, nil
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

func truncate(text string, maxTokens int) string {
	chars := int(math.Floor(float64(maxTokens) * 2.5))
	if len(text) > chars {
		return text[:chars]
	}
	return text
}
