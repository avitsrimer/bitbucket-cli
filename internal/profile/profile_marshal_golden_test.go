package profile_test

import (
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/profile"
)

// golden byte fixtures captured from the go-core-based code (master, pre-port, see
// docs/plans/20260807-dep-slimming-and-create-flags.md Task 3), pinned here so swapping
// Profile.APIRoot/Token.ExpiresOn onto the locally-ported common.URL/common.Timestamp cannot
// silently change the wire format.
const (
	goldenProfileNoAPIRoot   = `{"name":"p1","default":false,"user":"alice"}`
	goldenProfileWithAPIRoot = `{"name":"p2","default":false,"user":"bob","apiRoot":"https://api.bitbucket.org/2.0"}`
	goldenToken              = `{"token_type":"bearer","access_token":"abc123","refresh_token":"ref456","expires_on":1767225600000,"scope":"repository"}`
	goldenTokenQuotedInput   = `{"token_type":"bearer","access_token":"abc123","refresh_token":"ref456","expires_on":"1767225600000","scope":"repository"}`
)

func TestProfileMarshalJSONGoldenWithoutAPIRoot(t *testing.T) {
	p := profile.Profile{Name: "p1", User: "alice"}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), goldenProfileNoAPIRoot; got != want {
		t.Errorf("MarshalJSON() = %s, want %s", got, want)
	}
}

func TestProfileMarshalJSONGoldenWithAPIRoot(t *testing.T) {
	apiRoot, err := url.Parse("https://api.bitbucket.org/2.0")
	if err != nil {
		t.Fatal(err)
	}
	p := profile.Profile{Name: "p2", User: "bob", APIRoot: apiRoot}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), goldenProfileWithAPIRoot; got != want {
		t.Errorf("MarshalJSON() = %s, want %s", got, want)
	}
}

func TestProfileUnmarshalJSONGoldenRoundTrip(t *testing.T) {
	for _, golden := range []string{goldenProfileNoAPIRoot, goldenProfileWithAPIRoot} {
		var p profile.Profile
		if err := json.Unmarshal([]byte(golden), &p); err != nil {
			t.Fatalf("Unmarshal(%s): %v", golden, err)
		}
		if _, err := json.Marshal(p); err != nil {
			t.Fatalf("re-Marshal after Unmarshal(%s): %v", golden, err)
		}
	}
}

func TestTokenMarshalJSONGolden(t *testing.T) {
	expiresOn := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	token := profile.Token{
		TokenType:    "bearer",
		AccessToken:  "abc123",
		RefreshToken: "ref456",
		Scope:        "repository",
	}
	// ExpiresOn is set through UnmarshalJSON against the same golden bytes below, to avoid
	// reaching into the unexported common.Timestamp conversion from this external test package.
	if err := json.Unmarshal([]byte(goldenToken), &token); err != nil {
		t.Fatal(err)
	}
	if token.GetExpiresOn().Sub(expiresOn) != 0 {
		t.Fatalf("GetExpiresOn() = %v, want %v", token.GetExpiresOn(), expiresOn)
	}

	data, err := json.Marshal(token)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), goldenToken; got != want {
		t.Errorf("MarshalJSON() = %s, want %s", got, want)
	}
}

func TestTokenUnmarshalJSONGoldenAcceptsQuotedExpiresOn(t *testing.T) {
	var token profile.Token
	if err := json.Unmarshal([]byte(goldenTokenQuotedInput), &token); err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(token)
	if err != nil {
		t.Fatal(err)
	}
	// re-marshaling always emits the bare-number form, regardless of whether the input was
	// quoted or bare.
	if got, want := string(data), goldenToken; got != want {
		t.Errorf("MarshalJSON() after unmarshaling a quoted expires_on = %s, want %s", got, want)
	}
}
