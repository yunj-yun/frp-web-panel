package platform

import "testing"

func TestDockerImageTag(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "release", version: "v1.2.3", want: "v1.2.3"},
		{name: "development fallback", version: "dev-build", want: "alpha"},
		{name: "empty fallback", version: "", want: "alpha"},
		{name: "invalid tag fallback", version: "feature/test", want: "alpha"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dockerImageTag(tt.version); got != tt.want {
				t.Fatalf("dockerImageTag(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}
