package main

import "testing"

func TestSumFirstN(t *testing.T) {
	if v := SumFirstN(5); v != 15 {
		t.Errorf("expected 15, got %v", v)
	}
	if v := SumFirstN(0); v != 0 {
		t.Errorf("expected 0, got %v", v)
	}
	if v := SumFirstN(1); v != 1 {
		t.Errorf("expected 1, got %v", v)
	}
}

func TestFactorial(t *testing.T) {
	if v := Factorial(5); v != 120 {
		t.Errorf("expected 120, got %v", v)
	}
	if v := Factorial(0); v != 1 {
		t.Errorf("expected 1, got %v", v)
	}
}
