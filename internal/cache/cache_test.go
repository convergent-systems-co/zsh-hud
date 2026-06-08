package cache

import (
	"errors"
	"testing"
	"time"
)

func TestGetOrRefreshFetchesThenCaches(t *testing.T) {
	c := New()
	calls := 0
	fetch := func() (string, error) { calls++; return "v1", nil }

	if got := c.GetOrRefresh("k", time.Minute, fetch); got != "v1" {
		t.Fatalf("first = %q, want v1", got)
	}
	if got := c.GetOrRefresh("k", time.Minute, fetch); got != "v1" {
		t.Fatalf("second = %q, want v1 (cached)", got)
	}
	if calls != 1 {
		t.Fatalf("fetch called %d times, want 1", calls)
	}
}

func TestGetOrRefreshRefetchesAfterTTL(t *testing.T) {
	c := New()
	vals := []string{"a", "b"}
	i := 0
	fetch := func() (string, error) { v := vals[i]; i++; return v, nil }

	if got := c.GetOrRefresh("k", time.Nanosecond, fetch); got != "a" {
		t.Fatalf("first = %q, want a", got)
	}
	time.Sleep(time.Millisecond)
	if got := c.GetOrRefresh("k", time.Nanosecond, fetch); got != "b" {
		t.Fatalf("after TTL = %q, want b", got)
	}
}

func TestGetOrRefreshReturnsStaleOnError(t *testing.T) {
	c := New()
	c.GetOrRefresh("k", time.Nanosecond, func() (string, error) { return "good", nil })
	time.Sleep(time.Millisecond)
	got := c.GetOrRefresh("k", time.Nanosecond, func() (string, error) { return "", errors.New("net down") })
	if got != "good" {
		t.Fatalf("on error = %q, want stale 'good'", got)
	}
}

func TestGetOrRefreshEmptyOnErrorWithNoPriorValue(t *testing.T) {
	c := New()
	got := c.GetOrRefresh("k", time.Minute, func() (string, error) { return "", errors.New("boom") })
	if got != "" {
		t.Fatalf("error with no prior value = %q, want empty", got)
	}
}
