package main

import (
	"context"
	"fmt"
	"os"

	"github.com/holbrookab/go-ai/community/openrouter"
	"github.com/holbrookab/go-ai/packages/ai"
)

func main() {
	provider := openrouter.New(openrouter.Settings{
		APIKey: os.Getenv("OPENROUTER_API_KEY"),
	})

	result, err := ai.GenerateText(context.Background(), ai.GenerateTextOptions{
		Model:  provider.LanguageModel("openai/gpt-4o-mini"),
		Prompt: "Say hello from Go.",
		ProviderOptions: ai.ProviderOptions{
			"openrouter": map[string]any{
				"usage": map[string]any{"include": true},
			},
		},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Text)
}
