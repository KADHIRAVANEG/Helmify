package yaml

import (
	"testing"
)

func TestSplitImage(t *testing.T) {
	tests := []struct {
		name         string
		image        string
		expectedImg  string
		expectedTag  string
	}{
		{"image with tag", "nginx:1.19", "nginx", "1.19"},
		{"image without tag", "nginx", "nginx", "latest"},
		{"image with port and tag", "localhost:5000/app:v1", "localhost:5000/app", "v1"},
		{"image with port no tag", "localhost:5000/app", "localhost:5000/app", "latest"},
		{"complex repo with tag", "gcr.io/my-project/my-app:1.2.3", "gcr.io/my-project/my-app", "1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, tag := splitImage(tt.image)
			if img != tt.expectedImg {
				t.Errorf("expected img %q, got %q", tt.expectedImg, img)
			}
			if tag != tt.expectedTag {
				t.Errorf("expected tag %q, got %q", tt.expectedTag, tag)
			}
		})
	}
}

func TestToInt32(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int32
		ok       bool
	}{
		{"int", int(42), 42, true},
		{"int64", int64(42), 42, true},
		{"float64", float64(42.0), 42, true},
		{"string", "42", 0, false},
		{"nil", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := toInt32(tt.input)
			if ok != tt.ok {
				t.Errorf("expected ok=%v, got %v", tt.ok, ok)
			}
			if val != tt.expected {
				t.Errorf("expected val=%v, got %v", tt.expected, val)
			}
		})
	}
}
