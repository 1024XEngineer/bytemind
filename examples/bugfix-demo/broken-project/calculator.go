package main

// CalculateAverage returns the average of a slice of numbers.
func CalculateAverage(nums []float64) float64 {
	total := 0.0
	for _, n := range nums {
		total += n
	}
	return total / float64(len(nums))
}

// FindMax returns the maximum value in a slice.
func FindMax(nums []float64) float64 {
	if len(nums) == 0 {
		return 0
	}
	max := nums[0]
	for _, n := range nums[1:] {
		if n > max {
			max = n
		}
	}
	return max
}
