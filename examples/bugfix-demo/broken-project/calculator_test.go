package main

import "testing"

func TestCalculateAverage(t *testing.T) {
	result := CalculateAverage([]float64{1, 2, 3, 4, 5})
	if result != 3.0 {
		t.Errorf("expected 3.0, got %v", result)
	}
}

func TestCalculateAverageEmpty(t *testing.T) {
	result := CalculateAverage([]float64{})
	if result != 0 {
		t.Errorf("expected 0 for empty slice, got %v", result)
	}
}

func TestFindMax(t *testing.T) {
	result := FindMax([]float64{3, 7, 2, 9, 5})
	if result != 9 {
		t.Errorf("expected 9, got %v", result)
	}
}
