package main

import "testing"

func TestAgentResponseSkipsProgressText(t *testing.T) {
	data := []byte(`{"type":"text","part":{"type":"text","text":"Scanning your latest commit for bugs and regressions."}}
{"type":"text","part":{"type":"text","text":"{\"body\":\"summary\",\"comments\":[]}"}}`)

	got := AgentResponse(data)
	review, err := parseReviewResponse(got)
	if err != nil {
		t.Fatalf("parseReviewResponse(AgentResponse()) error = %v; response=%q", err, got)
	}
	if review.Body != "summary" {
		t.Fatalf("review body = %q, want summary", review.Body)
	}
}

func TestParseReviewResponse(t *testing.T) {
	review, err := parseReviewResponse(`{"body":"summary","comments":[{"body":"finding","path":"cmd/main.go","line":12}]}`)
	if err != nil {
		t.Fatalf("parseReviewResponse() error = %v", err)
	}
	if review.Body != "summary" || len(review.Comments) != 1 || review.Comments[0].Line != 12 {
		t.Fatalf("unexpected review: %+v", review)
	}
}

func TestParseReviewResponseRejectsInvalidComment(t *testing.T) {
	_, err := parseReviewResponse(`{"body":"summary","comments":[{"body":"finding","path":"cmd/main.go","line":0}]}`)
	if err == nil {
		t.Fatal("parseReviewResponse() accepted invalid line")
	}
}

func TestWithReviewMarker(t *testing.T) {
	if got, want := withReviewMarker("finding"), "finding\n\nAI review by op-reviewer"; got != want {
		t.Fatalf("withReviewMarker() = %q, want %q", got, want)
	}
	if got, want := withReviewMarker("finding\n\nAI review by op-reviewer"), "finding\n\nAI review by op-reviewer"; got != want {
		t.Fatalf("withReviewMarker() duplicated marker: %q", got)
	}
}
