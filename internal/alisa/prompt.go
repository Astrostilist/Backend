package alisa

import (
	"bytes"
	"embed"
	"encoding/json"
	"strings"
	"sync"
	"text/template"

	"astroapi/internal/astro"

	"go.uber.org/zap"
)

const (
	scenarioPersonalStyle = "personal_style"
	scenarioPerfectGift   = "perfect_gift"
)

//go:embed prompts/*.tmpl
var promptFS embed.FS

var (
	promptTemplates *template.Template
	promptOnce      sync.Once
)

type promptData struct {
	Scenario  string
	NatalData astro.NatalData
	Context   map[string]any
}

func BuildPrompt(scenario string, natalData astro.NatalData, context map[string]any, logger *zap.Logger) string {
	if context == nil {
		context = map[string]any{}
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	tmpl := loadPromptTemplates(logger)
	if tmpl == nil {
		zap.L().Error("prompt templates are not loaded")
		return ""
	}

	templateName := templateNameByScenario(scenario)
	if templateName == "" {
		logger.Error("unknown AlisaAI prompt scenario", zap.String("scenario", scenario))
		return ""
	}

	var result bytes.Buffer
	err := tmpl.ExecuteTemplate(&result, templateName, promptData{
		Scenario:  scenario,
		NatalData: natalData,
		Context:   context,
	})
	if err != nil {
		logger.Error("failed to build AlisaAI prompt", zap.String("scenario", scenario), zap.Error(err))
		return ""
	}

	prompt := strings.TrimSpace(result.String())
	logger.Debug("AlisaAI prompt built", zap.String("scenario", scenario), zap.String("prompt", prompt))

	return prompt
}

func loadPromptTemplates(logger *zap.Logger) *template.Template {
	promptOnce.Do(func() {
		funcMap := template.FuncMap{
			"toJSON": func(v any) string {
				data, err := json.MarshalIndent(v, "", "  ")
				if err != nil {
					return "{}"
				}
				return string(data)
			},
		}

		promptTemplates = template.New("prompts").Funcs(funcMap)
		parsedTemplates, err := promptTemplates.ParseFS(promptFS, "prompts/*.tmpl")
		if err != nil {
			logger.Error("failed to parse AlisaAI prompt templates", zap.Error(err))
			promptTemplates = nil
		} else {
			promptTemplates = parsedTemplates
		}
	})

	return promptTemplates
}
