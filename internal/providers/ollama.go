package providers

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

type OllamaProvider struct{}

func NewOllamaProvider() Provider { return OllamaProvider{} }

func (OllamaProvider) Name() string { return "ollama" }

func (OllamaProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	if req.Model == "" {
		req.Model = "llama3"
	}
	cmd := exec.CommandContext(ctx, "ollama", "run", req.Model, req.Prompt)
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return GenerateResponse{}, fmt.Errorf("ollama run failed: %v | %s", err, errBuf.String())
	}
	return GenerateResponse{Output: out.String(), Meta: map[string]any{"model": req.Model, "provider": "ollama"}}, nil
}
