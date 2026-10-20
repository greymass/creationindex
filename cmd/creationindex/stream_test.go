package main

import "testing"

func TestStreamAddress(t *testing.T) {
	cases := []struct {
		source string
		want   string
		ok     bool
	}{
		{"stream://localhost:9081", "localhost:9081", true},
		{"stream:///ramdisk/vaulta/actionstream.sock", "/ramdisk/vaulta/actionstream.sock", true},
		{"stream://", "", false},
		{"http://localhost:9081", "", false},
	}
	for _, c := range cases {
		got, err := streamAddress(c.source)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("streamAddress(%q) = %q, %v", c.source, got, err)
		}
	}
}
