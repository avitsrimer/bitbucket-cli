package user

type Author struct {
	Type string `json:"type"`
	Raw  string `json:"raw,omitempty"`
	User User   `json:"user"`
}

// IsEmpty checks if this Author is empty
func (author Author) IsEmpty() bool {
	return author.Type == "" && author.Raw == "" && author.User.IsEmpty()
}
