package sample

import "testing"

func TestGreeting(t *testing.T) {
	if Greeting() != "hello, forge" {
		t.Fatalf("unexpected greeting: %s", Greeting())
	}
}
