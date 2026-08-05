package user

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
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
