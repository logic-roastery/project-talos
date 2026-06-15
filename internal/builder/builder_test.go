package builder

import "testing"

func TestShortCommitSHA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "full sha", in: "1234567890abcdef", want: "1234567"},
		{name: "short sha", in: "12345", want: "12345"},
		{name: "empty sha", in: "", want: "manual"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := shortCommitSHA(tt.in); got != tt.want {
				t.Fatalf("shortCommitSHA(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
