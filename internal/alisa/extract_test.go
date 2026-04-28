package alisa

import "testing"

func TestExtractResponseText(t *testing.T) {
	t.Parallel()

	makeAlt := func(text string) ChatCompletionResponse {
		var resp ChatCompletionResponse
		resp.Result.Alternatives = append(resp.Result.Alternatives, struct {
			Message ChatMessage `json:"message"`
		}{Message: ChatMessage{Role: "assistant", Text: text}})
		return resp
	}
	makeChoice := func(text string) ChatCompletionResponse {
		return ChatCompletionResponse{Choices: []struct {
			Message ChatMessage `json:"message"`
		}{{Message: ChatMessage{Role: "assistant", Text: text}}}}
	}
	makeErr := func(msg string) ChatCompletionResponse {
		return ChatCompletionResponse{Error: &struct {
			Message string `json:"message"`
		}{Message: msg}}
	}

	cases := []struct {
		name string
		in   ChatCompletionResponse
		want string
	}{
		{name: "alternatives first", in: makeAlt("from alt"), want: "from alt"},
		{name: "openai-style choices", in: makeChoice("from choice"), want: "from choice"},
		{name: "error payload", in: makeErr("quota"), want: "quota"},
		{name: "empty", in: ChatCompletionResponse{}, want: ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractResponseText(tc.in)
			if got != tc.want {
				t.Fatalf("extractResponseText = %q, want %q", got, tc.want)
			}
		})
	}
}
