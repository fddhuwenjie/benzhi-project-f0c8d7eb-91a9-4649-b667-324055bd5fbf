package main

import "testing"

func TestResolveAddress(t *testing.T) {
	t.Setenv("PORT", "")
	value, err := resolveAddress("")
	if err != nil || value != "127.0.0.1:19081" {
		t.Fatalf("default: %q %v", value, err)
	}
	t.Setenv("PORT", "19123")
	value, err = resolveAddress("")
	if err != nil || value != "127.0.0.1:19123" {
		t.Fatalf("PORT: %q %v", value, err)
	}
	value, err = resolveAddress("19234")
	if err != nil || value != "127.0.0.1:19234" {
		t.Fatalf("numeric flag: %q %v", value, err)
	}
	value, err = resolveAddress("127.0.0.1:19345")
	if err != nil || value != "127.0.0.1:19345" {
		t.Fatalf("explicit flag: %q %v", value, err)
	}
	if _, err := resolveAddress("0"); err == nil {
		t.Fatal("invalid port accepted")
	}
}
