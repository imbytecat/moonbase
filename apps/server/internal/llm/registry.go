package llm

import (
	llmint "github.com/imbytecat/moonbase/integrations/llm"
	"github.com/imbytecat/moonbase/integrations/llm/anthropic"
	"github.com/imbytecat/moonbase/integrations/llm/openai"
)

func NewRegistry() llmint.Registry { return llmint.MustRegistry(openai.New(), anthropic.New()) }
