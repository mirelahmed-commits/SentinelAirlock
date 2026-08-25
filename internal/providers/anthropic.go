package providers

import (
	"context"
	"fmt"
)

type AnthropicProvider struct{}

func NewAnthropicProvider() Provider { return AnthropicProvider{} }

func (AnthropicProvider) Name() string { return "anthropic" }

func (AnthropicProvider) Generate(_ context.Context, _ GenerateRequest) (GenerateResponse, error) {
	return GenerateResponse{}, fmt.Errorf("anthropic provider not configured in v1.5 (interface scaffold only)")
}
