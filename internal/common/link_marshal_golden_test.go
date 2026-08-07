package common_test

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
)

// golden byte fixtures captured from the go-core-based code (master, pre-port, see
// docs/plans/20260807-dep-slimming-and-create-flags.md Task 3), pinned here so Link.MarshalJSON's
// HTTP and ssh/GitRef branches keep producing byte-identical output on the locally-ported
// common.URL.
const (
	goldenLinkHTTP     = `{"name":"self","href":"https://api.bitbucket.org/2.0/repositories/ws/repo"}`
	goldenLinkSSH      = `{"name":"ssh","href":"git@bitbucket.org:ws/repo.git"}`
	goldenLinkZeroHREF = `{"name":"empty","href":""}`
)

func TestLinkMarshalJSONGoldenHTTP(t *testing.T) {
	href, err := url.Parse("https://api.bitbucket.org/2.0/repositories/ws/repo")
	if err != nil {
		t.Fatal(err)
	}
	link := common.Link{Name: "self", HREF: *href}

	data, err := json.Marshal(link)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), goldenLinkHTTP; got != want {
		t.Errorf("MarshalJSON() = %s, want %s", got, want)
	}
}

func TestLinkMarshalJSONGoldenSSH(t *testing.T) {
	link := common.Link{Name: "ssh", GitRef: "git@bitbucket.org:ws/repo.git"}

	data, err := json.Marshal(link)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), goldenLinkSSH; got != want {
		t.Errorf("MarshalJSON() = %s, want %s", got, want)
	}
}

func TestLinkMarshalJSONGoldenZeroHREF(t *testing.T) {
	link := common.Link{Name: "empty"}

	data, err := json.Marshal(link)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), goldenLinkZeroHREF; got != want {
		t.Errorf("MarshalJSON() on zero HREF = %s, want %s (must marshal as \"\", not be omitted)", got, want)
	}
}

func TestLinkUnmarshalJSONGoldenRoundTrip(t *testing.T) {
	for name, golden := range map[string]string{
		"http": goldenLinkHTTP,
		"ssh":  goldenLinkSSH,
	} {
		t.Run(name, func(t *testing.T) {
			var link common.Link
			if err := json.Unmarshal([]byte(golden), &link); err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(link)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := string(data), golden; got != want {
				t.Errorf("round-trip = %s, want %s", got, want)
			}
		})
	}
}
