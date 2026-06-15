package github

import "testing"

func TestParseRepoFullName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		repoURL string
		owner   string
		repo    string
		wantErr bool
	}{
		{
			name:    "https repo",
			repoURL: "https://github.com/acme/widget",
			owner:   "acme",
			repo:    "widget",
		},
		{
			name:    "git suffix",
			repoURL: "https://github.com/acme/widget.git",
			owner:   "acme",
			repo:    "widget",
		},
		{
			name:    "extra path segments",
			repoURL: "https://github.com/acme/widget/tree/main",
			owner:   "acme",
			repo:    "widget",
		},
		{
			name:    "invalid host",
			repoURL: "https://gitlab.com/acme/widget",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			owner, repo, err := ParseRepoFullName(tt.repoURL)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.repoURL)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRepoFullName(%q) error = %v", tt.repoURL, err)
			}
			if owner != tt.owner || repo != tt.repo {
				t.Fatalf("ParseRepoFullName(%q) = %q/%q, want %q/%q", tt.repoURL, owner, repo, tt.owner, tt.repo)
			}
		})
	}
}
