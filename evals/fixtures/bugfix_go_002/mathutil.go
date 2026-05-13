package main

// SumFirstN returns the sum of the first n natural numbers (1 to n).
func SumFirstN(n int) int {
	sum := 0
	for i := 1; i < n; i++ {
		sum += i
	}
	return sum
}

// Factorial returns n! (n factorial).
func Factorial(n int) int {
	if n <= 1 {
		return 1
	}
	result := 1
	for i := 2; i <= n; i++ {
		result *= i
	}
	return result
}
