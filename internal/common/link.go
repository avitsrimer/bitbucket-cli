package common

import (
	"encoding/json"
	"fmt"
	"net/url"
)

type Link struct {
	Name   string  `json:"name,omitempty"`
	HREF   url.URL `json:"-"`
	GitRef string  `json:"-"`
}

// MarshalJSON implements the json.Marshaler interface.
func (link Link) MarshalJSON() (data []byte, err error) {
	type surrogate Link

	if link.GitRef != "" {
		data, err = json.Marshal(struct {
			surrogate
			GitRef string `json:"href"`
		}{
			surrogate: surrogate(link),
			GitRef:    link.GitRef,
		})
	} else {
		data, err = json.Marshal(struct {
			surrogate
			HREF URL `json:"href"`
		}{
			surrogate: surrogate(link),
			HREF:      URL(link.HREF),
		})
	}
	if err != nil {
		return nil, fmt.Errorf("cannot marshal link to json: %w", err)
	}
	return data, nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (link *Link) UnmarshalJSON(data []byte) (err error) {
	type surrogate Link

	var header struct {
		Name string `json:"name"`
	}
	if err = json.Unmarshal(data, &header); err != nil {
		return fmt.Errorf("cannot unmarshal link: %w", err)
	}
	switch header.Name {
	case "ssh":
		var inner struct {
			surrogate
			GitRef string `json:"href"`
		}
		if err = json.Unmarshal(data, &inner); err != nil {
			return fmt.Errorf("cannot unmarshal link: %w", err)
		}
		*link = Link(inner.surrogate)
		link.GitRef = inner.GitRef
	default:
		var inner struct {
			surrogate
			HREF URL `json:"href"`
		}

		if err = json.Unmarshal(data, &inner); err != nil {
			return fmt.Errorf("cannot unmarshal link: %w", err)
		}
		*link = Link(inner.surrogate)
		link.HREF = inner.HREF.AsURL()
	}
	return nil
}
