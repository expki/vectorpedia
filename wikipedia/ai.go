package wikipedia

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/expki/vectorpedia/ai"
	"github.com/expki/vectorpedia/compute"
)

// GenerateSummary generates a summary for a Wikipedia article using AI
func (w *Wikipedia) GenerateSummary(ctx context.Context, title, content string) (string, error) {
	systemMsg := "You are a Wikipedia article summarizer. Output only the summary text itself - no introductory phrases, no prefixes, no 'Here's a summary' or similar text. Write a clear, informative summary in 2-3 sentences."
	userPrefix := fmt.Sprintf("Article: %s\n\n", title)
	
	// Tokenize the system message and user prefix to calculate remaining tokens
	tokenStart := time.Now()
	systemTokens, err := w.client.TokenizeChat(ctx, &ai.TokenizeRequest{Content: systemMsg})
	tokenDuration := time.Since(tokenStart).Nanoseconds()
	w.updateMetrics(tokenDuration, MetricType_TokenizeChat)
	if err != nil {
		return "", fmt.Errorf("failed to tokenize system message: %w", err)
	}
	
	tokenStart = time.Now()
	prefixTokens, err := w.client.TokenizeChat(ctx, &ai.TokenizeRequest{Content: userPrefix})
	tokenDuration = time.Since(tokenStart).Nanoseconds()
	w.updateMetrics(tokenDuration, MetricType_TokenizeChat)
	if err != nil {
		return "", fmt.Errorf("failed to tokenize user prefix: %w", err)
	}
	
	// Calculate allowance for content
	allowance := int(w.contextSizeChat) - len(systemTokens.Tokens) - len(prefixTokens.Tokens) - 100 // Reserve 100 tokens for safety
	
	// Tokenize content and truncate if needed
	tokenStart = time.Now()
	contentTokens, err := w.client.TokenizeChat(ctx, &ai.TokenizeRequest{Content: content})
	tokenDuration = time.Since(tokenStart).Nanoseconds()
	w.updateMetrics(tokenDuration, MetricType_TokenizeChat)
	if err != nil {
		return "", fmt.Errorf("failed to tokenize content: %w", err)
	}
	
	truncatedContent := content
	if len(contentTokens.Tokens) > allowance && allowance > 0 {
		// Truncate tokens and detokenize
		truncatedTokens := contentTokens.Tokens[:allowance]
		detokenStart := time.Now()
		detokenized, err := w.client.DetokenizeChat(ctx, &ai.DetokenizeRequest{Tokens: truncatedTokens})
		detokenDuration := time.Since(detokenStart).Nanoseconds()
		w.updateMetrics(detokenDuration, MetricType_DetokenizeChat)
		if err != nil {
			return "", fmt.Errorf("failed to detokenize content: %w", err)
		}
		truncatedContent = detokenized.Content
	}
	
	req := &ai.ChatRequest{
		Messages: []ai.ChatMessage{
			{
				Role:    "system",
				Content: systemMsg,
			},
			{
				Role:    "user",
				Content: userPrefix + truncatedContent,
			},
		},
		MaxTokens:   int(w.contextSizeEmbed),
		Temperature: 0.3,
	}

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
		go func(idx int, text string) {
			defer wg.Done()
			
			// Tokenize and truncate if needed
			tokenStart := time.Now()
			tokenResp, err := w.client.TokenizeEmbed(ctx, &ai.TokenizeRequest{Content: text})
			tokenDuration := time.Since(tokenStart).Nanoseconds()
			w.updateMetrics(tokenDuration, MetricType_TokenizeEmbed)
			if err != nil {
				errors[idx] = fmt.Errorf("failed to tokenize: %w", err)
				return
			}
			
			safeText := text
			// Leave some buffer space (use 95% of context size to be safe)
			maxTokens := int(w.contextSizeEmbed * 95 / 100)
			if len(tokenResp.Tokens) > maxTokens {
				// Truncate tokens and detokenize
				truncatedTokens := tokenResp.Tokens[:maxTokens]
				detokenStart := time.Now()
				detokenized, err := w.client.DetokenizeEmbed(ctx, &ai.DetokenizeRequest{Tokens: truncatedTokens})
				detokenDuration := time.Since(detokenStart).Nanoseconds()
				w.updateMetrics(detokenDuration, MetricType_DetokenizeEmbed)
				if err != nil {
					errors[idx] = fmt.Errorf("failed to detokenize: %w", err)
					return
				}
				safeText = detokenized.Content
			}

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
		}(idx, text)
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
