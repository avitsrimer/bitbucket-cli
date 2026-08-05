package common

type RenderedText struct {
	Type   string `json:"type,omitempty"`
	Raw    string `json:"raw"`
	Markup string `json:"markup,omitempty"` // markdown, creaole, plaintext
	HTML   string `json:"html,omitempty"`
}
