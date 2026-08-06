package user

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParticipantUnmarshalsFromTestdata(t *testing.T) {
	data, err := os.ReadFile("../../testdata/participant.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}

	var participant Participant
	if err := json.Unmarshal(data, &participant); err != nil {
		t.Fatalf("cannot unmarshal participant: %v", err)
	}

	if participant.User.Name != "John Doe" {
		t.Errorf("participant.User.Name = %q, want %q", participant.User.Name, "John Doe")
	}
	if participant.Role != "PARTICIPANT" {
		t.Errorf("participant.Role = %q, want %q", participant.Role, "PARTICIPANT")
	}
	if !participant.Approved {
		t.Error("participant.Approved = false, want true")
	}
	if participant.State != "approved" {
		t.Errorf("participant.State = %q, want %q", participant.State, "approved")
	}
}

func TestParticipantGetRow(t *testing.T) {
	var participant Participant
	data, err := os.ReadFile("../../testdata/participant.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}
	if err := json.Unmarshal(data, &participant); err != nil {
		t.Fatalf("cannot unmarshal participant: %v", err)
	}

	headers := participant.GetHeaders(nil)
	if want := []string{"ID", "Name", "participated on", "approved", "state"}; !slices.Equal(headers, want) {
		t.Errorf("GetHeaders() = %v, want %v", headers, want)
	}

	row := participant.GetRow([]string{"name", "approved", "state"})
	if want := []string{"John Doe", "true", "approved"}; !slices.Equal(row, want) {
		t.Errorf("GetRow() = %v, want %v", row, want)
	}
}

// TestParticipantMarshalJSONOmitsZeroParticipatedOn proves a pending reviewer (no
// participated_on in the source payload, decoding as the zero time.Time) marshals back to json
// WITHOUT a "0001-01-01T00:00:00Z" participated_on field.
func TestParticipantMarshalJSONOmitsZeroParticipatedOn(t *testing.T) {
	participant := Participant{Role: "REVIEWER", State: ""}

	data, err := json.Marshal(participant)
	if err != nil {
		t.Fatalf("cannot marshal participant: %v", err)
	}
	if strings.Contains(string(data), "participated_on") {
		t.Errorf("marshaled json = %s, want no participated_on field for a zero time", data)
	}
	if strings.Contains(string(data), "0001-01-01") {
		t.Errorf("marshaled json = %s, leaked the year-1 zero-time value", data)
	}
}

// TestParticipantMarshalJSONKeepsNonZeroParticipatedOn proves a reviewer who has actually
// participated still carries their participated_on timestamp in json output.
func TestParticipantMarshalJSONKeepsNonZeroParticipatedOn(t *testing.T) {
	data, err := os.ReadFile("../../testdata/participant.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}
	var participant Participant
	if unmarshalErr := json.Unmarshal(data, &participant); unmarshalErr != nil {
		t.Fatalf("cannot unmarshal participant: %v", unmarshalErr)
	}

	marshaled, err := json.Marshal(participant)
	if err != nil {
		t.Fatalf("cannot marshal participant: %v", err)
	}
	if !strings.Contains(string(marshaled), "2023-12-01") {
		t.Errorf("marshaled json = %s, want the real participated_on timestamp preserved", marshaled)
	}
}

// TestParticipantMarshalYAMLOmitsZeroParticipatedOn is
// TestParticipantMarshalJSONOmitsZeroParticipatedOn for -o yaml: FR-9 requires the zero-timestamp
// omission on both formats, but yaml.Marshal never consults a type's json.Marshaler, so this needs
// its own Participant.MarshalYAML to not regress independently of the json one.
//
// This only asserts on the "participatedon" key itself, not a blanket absence of "0001-01-01"
// anywhere in the document: the embedded zero-value User also yaml-encodes its own unrelated
// zero-time CreatedOn field (User has no MarshalYAML, only MarshalJSON), which is a pre-existing
// gap outside FR-9's scope (Participant.ParticipatedOn specifically).
func TestParticipantMarshalYAMLOmitsZeroParticipatedOn(t *testing.T) {
	participant := Participant{Role: "REVIEWER", State: ""}

	data, err := yaml.Marshal(participant)
	if err != nil {
		t.Fatalf("cannot marshal participant: %v", err)
	}
	if strings.Contains(string(data), "participatedon") {
		t.Errorf("marshaled yaml = %s, want no participatedon field for a zero time", data)
	}
}

// TestParticipantMarshalYAMLKeepsNonZeroParticipatedOn is
// TestParticipantMarshalJSONKeepsNonZeroParticipatedOn for -o yaml: a reviewer who has actually
// participated still carries their participated_on timestamp in yaml output.
func TestParticipantMarshalYAMLKeepsNonZeroParticipatedOn(t *testing.T) {
	data, err := os.ReadFile("../../testdata/participant.json")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}
	var participant Participant
	if unmarshalErr := json.Unmarshal(data, &participant); unmarshalErr != nil {
		t.Fatalf("cannot unmarshal participant: %v", unmarshalErr)
	}

	marshaled, err := yaml.Marshal(participant)
	if err != nil {
		t.Fatalf("cannot marshal participant: %v", err)
	}
	if !strings.Contains(string(marshaled), "2023-12-01") {
		t.Errorf("marshaled yaml = %s, want the real participated_on timestamp preserved", marshaled)
	}
}
