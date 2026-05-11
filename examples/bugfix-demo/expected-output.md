# Bugfix Demo: Expected Output

## The Bug

`CalculateAverage` panics on an empty slice because `len(nums)` is 0 and dividing by zero produces `NaN`.
The test `TestCalculateAverageEmpty` expects `0` for empty input.

## Expected Fix

```go
func CalculateAverage(nums []float64) float64 {
	if len(nums) == 0 {
		return 0
	}
	total := 0.0
	for _, n := range nums {
		total += n
	}
	return total / float64(len(nums))
}
```

## Expected Tool Trace

1. `read_file` calculator.go → understands the code
2. `read_file` calculator_test.go → understands the test expectation
3. `run_tests` → confirms the failure: `panic: runtime error: floating point NaN`
4. `replace_in_file` or `apply_patch` → adds empty guard clause
5. `run_tests` → confirms all tests pass
6. `git_diff` → shows the 3-line change
