package providers

import "context"

type GenerateRequest struct {
	Model  string
	Prompt string
}

type GenerateResponse struct {
	Output string
	Meta   map[string]any
}

type Provider interface {
	Name() string
	Generate(context.Context, GenerateRequest) (GenerateResponse, error)
}
