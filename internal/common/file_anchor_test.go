package common_test

import (
	"encoding/json"
	"testing"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
)

func TestFileAnchorString(t *testing.T) {
	tests := []struct {
		name   string
		anchor common.FileAnchor
		want   string
	}{
		{
			name:   "new side single",
			anchor: common.FileAnchor{Path: "path/to/file.go", To: 1040},
			want:   "path/to/file.go:1040",
		},
		{
			name:   "old side single",
			anchor: common.FileAnchor{Path: "path/to/file.go", From: 990},
			want:   "path/to/file.go:990(old)",
		},
		{
			name:   "both sides set - new side wins",
			anchor: common.FileAnchor{Path: "path/to/file.go", From: 45, To: 1040},
			want:   "path/to/file.go:1040",
		},
		{
			name:   "path only",
			anchor: common.FileAnchor{Path: "path/to/file.go"},
			want:   "path/to/file.go",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.anchor.String(); got != test.want {
				t.Errorf("String() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFileAnchorMarshalJSON(t *testing.T) {
	tests := []struct {
		name   string
		anchor common.FileAnchor
		want   string
	}{
		{
			name:   "new side only",
			anchor: common.FileAnchor{Path: "file.go", To: 1040},
			want:   `{"to":1040,"path":"file.go"}`,
		},
		{
			name:   "old side only",
			anchor: common.FileAnchor{Path: "file.go", From: 990},
			want:   `{"from":990,"path":"file.go"}`,
		},
		{
			name:   "path only - from/to omitted",
			anchor: common.FileAnchor{Path: "file.go"},
			want:   `{"path":"file.go"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(test.anchor)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if string(data) != test.want {
				t.Errorf("Marshal() = %s, want %s", data, test.want)
			}
		})
	}
}
