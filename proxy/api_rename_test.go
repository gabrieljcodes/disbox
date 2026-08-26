package proxy

import (
	"net/url"
	"strings"
	"testing"
)

func TestFormatCustomDownloadFilename(t *testing.T) {
	tests := []struct {
		name             string
		customName       string
		upstreamFilename string
		origName         string
		expected         string
	}{
		{
			name:             "No custom name uses upstream filename",
			customName:       "",
			upstreamFilename: "Ubuntu-24.04-desktop-amd64.iso",
			origName:         "Ubuntu-24.04-desktop-amd64.iso",
			expected:         "Ubuntu-24.04-desktop-amd64.iso",
		},
		{
			name:             "Custom name without extension inherits upstream extension",
			customName:       "Batata",
			upstreamFilename: "Super.Movie.2026.1080p.mkv",
			origName:         "Super.Movie.2026.1080p.mkv",
			expected:         "Batata.mkv",
		},
		{
			name:             "Custom name with explicit extension preserves user extension",
			customName:       "Cenoura.zip",
			upstreamFilename: "Super.Movie.2026.1080p.mkv",
			origName:         "Super.Movie.2026.1080p.mkv",
			expected:         "Cenoura.zip",
		},
		{
			name:             "Custom name inherits extension from origName if upstream is empty",
			customName:       "MyLinux",
			upstreamFilename: "",
			origName:         "archlinux-2026.01.iso",
			expected:         "MyLinux.iso",
		},
		{
			name:             "Custom name with spaces and special chars without extension",
			customName:       "Batata & Amigos",
			upstreamFilename: "bundle.tar.gz",
			origName:         "bundle.tar.gz",
			expected:         "Batata & Amigos.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatCustomDownloadFilename(tt.customName, tt.upstreamFilename, tt.origName)
			if got != tt.expected {
				t.Errorf("formatCustomDownloadFilename(%q, %q, %q) = %q; expected %q",
					tt.customName, tt.upstreamFilename, tt.origName, got, tt.expected)
			}
		})
	}
}

func TestFormatContentDisposition(t *testing.T) {
	filename := "Batata & Cenoura (2026).mkv"
	header := formatContentDisposition(filename)

	if !strings.HasPrefix(header, "attachment; filename=\"") {
		t.Errorf("expected attachment prefix in %q", header)
	}
	if !strings.Contains(header, "filename*=UTF-8''") {
		t.Errorf("expected UTF-8 filename* parameter in %q", header)
	}
	if !strings.Contains(header, url.PathEscape(filename)) {
		t.Errorf("expected URL-encoded UTF-8 filename in %q", header)
	}
}

func TestSanitizeCustomFilename(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"Normal Name.mkv", "Normal Name.mkv"},
		{"Path/Traversal\\Attempt.iso", "Path_Traversal_Attempt.iso"},
		{"With\x00Null\r\nBytes", "WithNullBytes"},
		{"   Trimmed   ", "Trimmed"},
	}

	for _, c := range cases {
		got := sanitizeCustomFilename(c.input)
		if got != c.expected {
			t.Errorf("sanitizeCustomFilename(%q) = %q; expected %q", c.input, got, c.expected)
		}
	}
}
