package main

import (
	"context"
	"fmt"

	"github.com/Origens-Dev/go-ai/packages/ai"
	"github.com/Origens-Dev/go-ai/packages/google"
)

func main() {
	provider := google.New(google.Settings{})
	result, err := ai.GenerateText(context.Background(), ai.GenerateTextOptions{
		Model:  provider.LanguageModel("gemini-2.5-flash"),
		Prompt: "Say hello from Go.",
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Text)
}
