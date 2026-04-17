package alisa

import (
	"bytes"
	"embed"
	"encoding/json"
	"strings"
	"sync"
	"text/template"

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
	Scenario     string
	AstroProfile AstroProfile
	Context      map[string]any
}

func BuildPrompt(scenario string, astroProfile AstroProfile, context map[string]any) string {
	if context == nil {
		context = map[string]any{}
	}

	tmpl := loadPromptTemplates()
	if tmpl == nil {
		zap.L().Error("prompt templates are not loaded")
		return ""
	}

	templateName := templateNameByScenario(scenario)
	if templateName == "" {
		zap.L().Warn("unknown AlisaAI prompt scenario", zap.String("scenario", scenario))
		return ""
	}

	var result bytes.Buffer
	err := tmpl.ExecuteTemplate(&result, templateName, promptData{
		Scenario:     scenario,
		AstroProfile: astroProfile,
		Context:      context,
	})
	if err != nil {
		zap.L().Error("failed to build AlisaAI prompt", zap.String("scenario", scenario), zap.Error(err))
		return ""
	}

	prompt := strings.TrimSpace(result.String())
	zap.L().Debug("AlisaAI prompt built", zap.String("scenario", scenario), zap.String("prompt", prompt))

	return prompt
}

func loadPromptTemplates() *template.Template {
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
			zap.L().Error("failed to parse AlisaAI prompt templates", zap.Error(err))
			promptTemplates = nil
		} else {
			promptTemplates = parsedTemplates
		}
	})

	return promptTemplates
}

func templateNameByScenario(scenario string) string {
	switch scenario {
	case scenarioPersonalStyle:
		return "personal_style.tmpl"
	case scenarioPerfectGift:
		return "perfect_gift.tmpl"
	default:
		return ""
	}
}
