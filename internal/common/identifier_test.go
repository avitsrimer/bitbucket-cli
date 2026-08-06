package common

import "testing"

func TestValidatePathIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "valid numeric id", value: "42", wantErr: false},
		{name: "valid uuid-shaped", value: "{11111111-1111-1111-1111-111111111111}", wantErr: false},
		{name: "valid name", value: "Build and Test", wantErr: false},
		{name: "empty", value: "", wantErr: true},
		{name: "dot", value: ".", wantErr: true},
		{name: "dotdot", value: "..", wantErr: true},
		{name: "dotdot traversal", value: "../..", wantErr: true},
		{name: "dotdot traversal deep", value: "../../..", wantErr: true},
		{name: "leading slash", value: "/etc/passwd", wantErr: true},
		{name: "embedded slash", value: "foo/bar", wantErr: true},
		{name: "trailing slash", value: "foo/", wantErr: true},
		{name: "embedded backslash", value: `foo\bar`, wantErr: true},
		{name: "percent-encoded slash lowercase", value: "foo%2fbar", wantErr: true},
		{name: "percent-encoded slash uppercase", value: "foo%2Fbar", wantErr: true},
		{name: "percent-encoded backslash lowercase", value: "foo%5cbar", wantErr: true},
		{name: "percent-encoded backslash uppercase", value: "foo%5Cbar", wantErr: true},
		{name: "percent-encoded dot-dot-slash", value: "%2e%2e%2f", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePathIdentifier("argument", tt.value)
			if tt.wantErr && err == nil {
				t.Errorf("ValidatePathIdentifier(%q) = nil, want an error", tt.value)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidatePathIdentifier(%q) = %v, want nil", tt.value, err)
			}
		})
	}
}
