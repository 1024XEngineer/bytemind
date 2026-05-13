package main

import "strings"

// Reverse returns the reverse of a string.
func Reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// Capitalize capitalizes the first character of each word.
func Capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.Title(s)
}

// Truncate truncates a string to n characters, appending "..." if truncated.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
