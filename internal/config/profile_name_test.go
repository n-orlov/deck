package config

import "testing"

// TestValidateProfileName pins SPEC §3.4's one validator: the exact message
// text for each rejection reason, and the boundary cases explicitly called
// out there (default, and the 16/17-character length boundary).
func TestValidateProfileName(t *testing.T) {
	sixteen := "abcdefghijklmnop"    // 16 chars, valid
	seventeen := "abcdefghijklmnopq" // 17 chars

	cases := []struct {
		name    string
		input   string
		wantErr string // "" means accepted (nil error)
	}{
		{
			name:    "16 characters is accepted at the limit",
			input:   sixteen,
			wantErr: "",
		},
		{
			name:    "17 characters is rejected naming the length and the limit",
			input:   seventeen,
			wantErr: `error: profile name "abcdefghijklmnopq" is 17 characters; the limit is 16`,
		},
		{
			name:    "uppercase is rejected with a lowercase suggestion",
			input:   "Work",
			wantErr: `error: profile names are lowercase; did you mean "work"?`,
		},
		{
			name:    "a dot is rejected naming the character and the allowed set",
			input:   "acme.prod",
			wantErr: `error: profile name "acme.prod" contains "."; allowed: a-z 0-9 - _`,
		},
		{
			name:    "a leading underscore is rejected as a first-character violation",
			input:   "_abc",
			wantErr: `error: profile name "_abc" starts with "_"; must start with a-z or 0-9`,
		},
		{
			name:    "a leading hyphen is rejected as a first-character violation",
			input:   "-abc",
			wantErr: `error: profile name "-abc" starts with "-"; must start with a-z or 0-9`,
		},
		{
			name:    "default is always accepted",
			input:   "default",
			wantErr: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateProfileName(tc.input)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateProfileName(%q) = %v, want accepted", tc.input, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateProfileName(%q) = nil, want error %q", tc.input, tc.wantErr)
			}
			if got := err.Error(); got != tc.wantErr {
				t.Fatalf("ValidateProfileName(%q) error = %q, want %q", tc.input, got, tc.wantErr)
			}
		})
	}
}

// TestValidateProfileNameEnv pins the DECK_PROFILE-prefixed variant: the
// same rejection, wrapped with SPEC §3.4's `DECK_PROFILE="…": ` prefix.
func TestValidateProfileNameEnv(t *testing.T) {
	const input = "Work"
	want := `DECK_PROFILE="Work": error: profile names are lowercase; did you mean "work"?`

	err := ValidateProfileNameEnv(input)
	if err == nil {
		t.Fatalf("ValidateProfileNameEnv(%q) = nil, want error %q", input, want)
	}
	if got := err.Error(); got != want {
		t.Fatalf("ValidateProfileNameEnv(%q) error = %q, want %q", input, got, want)
	}
}

func TestValidateProfileNameEnvAccepts(t *testing.T) {
	if err := ValidateProfileNameEnv("default"); err != nil {
		t.Fatalf("ValidateProfileNameEnv(%q) = %v, want accepted", "default", err)
	}
}
