package ai

import "testing"

func TestParseReview(t *testing.T) {
	raw := "```json\n{\"passed\":true,\"score\":0,\"bugs\":null,\"style_notes\":[\"ок\"],\"follow_up_question\":\"почему?\"}\n```"
	r, err := ParseReview(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Passed || r.Score != 1 || len(r.Bugs) != 0 || r.StyleNotes[0] != "ок" {
		t.Fatalf("%+v", r)
	}
	r, err = ParseReview(`{"passed":false,"score":9,"bugs":["race"],"style_notes":[],"follow_up_question":""}`)
	if err != nil {
		t.Fatal(err)
	}
	if r.Passed || r.Score != 5 || r.Bugs[0] != "race" {
		t.Fatalf("%+v", r)
	}
}
