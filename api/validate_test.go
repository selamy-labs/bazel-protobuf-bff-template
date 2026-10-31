package api_test

import (
	"errors"
	"strings"
	"testing"

	"buf.build/go/protovalidate"
	"google.golang.org/protobuf/proto"

	notesv1 "example.com/bfftemplate/api/template/notes/v1"
)

// The boundary values come from contracts/notes-api.md in specs/001.
func TestSchemaValidationBoundaries(t *testing.T) {
	note := func(title, body string) *notesv1.CreateNoteRequest {
		return &notesv1.CreateNoteRequest{Note: &notesv1.Note{Title: title, Body: body}}
	}
	cases := []struct {
		name    string
		message proto.Message
		rule    string // empty means the message is valid
	}{
		{"shortest title", note("a", ""), ""},
		{"longest title", note(strings.Repeat("a", 120), ""), ""},
		{"longest title in multi-byte characters", note(strings.Repeat("é", 120), ""), ""},
		{"surrounding spaces are not counted", note("  "+strings.Repeat("a", 120)+"  ", ""), ""},
		{"longest body", note("a", strings.Repeat("b", 4000)), ""},
		{"empty title", note("", ""), "note.title.required"},
		{"whitespace title", note(" \t\n", ""), "note.title.required"},
		{"title too long", note(strings.Repeat("a", 121), ""), "note.title.max_len"},
		{"body too long", note("a", strings.Repeat("b", 4001)), "string.max_len"},
		{"missing note", &notesv1.CreateNoteRequest{}, "required"},
		{"shortest name", &notesv1.GetNoteRequest{Name: "notes/a"}, ""},
		{"longest name", &notesv1.GetNoteRequest{Name: "notes/" + strings.Repeat("a", 64)}, ""},
		{"name with allowed punctuation", &notesv1.GetNoteRequest{Name: "notes/a-1"}, ""},
		{"empty name", &notesv1.GetNoteRequest{}, "string.pattern"},
		{"id too long", &notesv1.GetNoteRequest{Name: "notes/" + strings.Repeat("a", 65)}, "string.pattern"},
		{"uppercase id", &notesv1.GetNoteRequest{Name: "notes/A"}, "string.pattern"},
		{"wrong collection", &notesv1.GetNoteRequest{Name: "tasks/a"}, "string.pattern"},
		{"trailing newline", &notesv1.GetNoteRequest{Name: "notes/a\n"}, "string.pattern"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := protovalidate.Validate(c.message)
			if c.rule == "" {
				if err != nil {
					t.Fatalf("want valid, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want a violation of %s, got none", c.rule)
			}
			var invalid *protovalidate.ValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("want a validation error, got %v", err)
			}
			if len(invalid.Violations) != 1 || invalid.Violations[0].Proto.GetRuleId() != c.rule {
				t.Fatalf("want exactly a violation of %s, got %v", c.rule, err)
			}
		})
	}
}
