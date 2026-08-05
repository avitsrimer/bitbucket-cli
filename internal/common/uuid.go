package common

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type UUID uuid.UUID

func ParseUUID(s string) (UUID, error) {
	u, err := uuid.Parse(s)
	return UUID(u), err
}

func (u UUID) IsNil() bool {
	return uuid.UUID(u) == uuid.Nil
}

func (u UUID) String() string {
	return "{" + uuid.UUID(u).String() + "}"
}

func (u UUID) MarshalJSON() ([]byte, error) {
	return []byte(`"` + u.String() + `"`), nil
}

func (u *UUID) UnmarshalJSON(payload []byte) error {
	if len(payload) < 2 {
		return errors.New("cannot unmarshal uuid: unexpected end of JSON input")
	}
	value := string(payload[1 : len(payload)-1])
	if value == "" {
		*u = UUID(uuid.Nil)
		return nil
	}
	parsed, err := ParseUUID(value)
	if err != nil {
		return fmt.Errorf("cannot unmarshal uuid: %w", err)
	}
	*u = parsed
	return nil
}
