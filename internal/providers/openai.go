package providers

import (
	"context"
	"fmt"
)

type OpenAIProvider struct{}

func NewOpenAIProvider() Provider { return OpenAIProvider{} }

func (OpenAIProvider) Name() string { return "openai" }

func (OpenAIProvider) Generate(_ context.Context, _ GenerateRequest) (GenerateResponse, error) {
	return GenerateResponse{}, fmt.Errorf("openai provider not configured in v1.5 (interface scaffold only)")
}
