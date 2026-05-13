package main

import "testing"

func TestToFahrenheit1(t *testing.T) {
	if v := ToFahrenheit1(0); v != 32 {
		t.Errorf("expected 32, got %v", v)
	}
	if v := ToFahrenheit1(100); v != 212 {
		t.Errorf("expected 212, got %v", v)
	}
}

func TestToFahrenheit2(t *testing.T) {
	if v := ToFahrenheit2(0); v != 32 {
		t.Errorf("expected 32, got %v", v)
	}
}

func TestToFahrenheit3(t *testing.T) {
	if v := ToFahrenheit3(37); v != 98.6 {
		t.Errorf("expected 98.6, got %v", v)
	}
}
