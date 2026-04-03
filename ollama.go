package main

import (
	"context"
	"time"

	"github.com/ollama/ollama/api"
)

type OllamaClient struct {
	client *api.Client
}

func NewOllamaClient() (*OllamaClient, error) {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return nil, err
	}
	return &OllamaClient{client: client}, nil
}

func (c *OllamaClient) List() ([]api.ListModelResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.client.List(ctx)
	if err != nil {
		return nil, err
	}
	return resp.Models, nil
}

func (c *OllamaClient) Delete(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := &api.DeleteRequest{Name: name}
	return c.client.Delete(ctx, req)
}

func (c *OllamaClient) Load(name string) error {
	// A generate request with empty prompt and keep_alive forces the model to load into memory.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // loading might take a bit
	defer cancel()

	keepAlive := &api.Duration{Duration: 24 * time.Hour}
	req := &api.GenerateRequest{
		Model:     name,
		KeepAlive: keepAlive,
	}

	fn := func(res api.GenerateResponse) error {
		return nil
	}

	return c.client.Generate(ctx, req, fn)
}

func (c *OllamaClient) Unload(name string) error {
	// A generate request with empty prompt and 0 keep_alive forces the model to unload from memory.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	keepAlive := &api.Duration{Duration: 0}
	req := &api.GenerateRequest{
		Model:     name,
		KeepAlive: keepAlive,
	}

	fn := func(res api.GenerateResponse) error {
		return nil
	}

	return c.client.Generate(ctx, req, fn)
}

func (c *OllamaClient) Pull(ctx context.Context, name string, insecure bool, progressFn func(api.ProgressResponse) error) error {
	req := &api.PullRequest{
		Name:     name,
		Insecure: insecure,
	}
	return c.client.Pull(ctx, req, progressFn)
}
