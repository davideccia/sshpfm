package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func tagLabels(tags []Tag) []string {
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = t.Label
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCreateWithNewTags(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	c := sample()
	c.NewTags = []Tag{{Label: " prod ", Color: "#38BDF8"}, {Label: "db", Color: "#818cf8"}}
	id, err := s.Create(ctx, c)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if want := []string{"db", "prod"}; !equalStrings(tagLabels(got.Tags), want) {
		t.Errorf("tags = %v, want %v", tagLabels(got.Tags), want)
	}
	for _, tag := range got.Tags {
		if tag.Label == "prod" && tag.Color != "#38bdf8" {
			t.Errorf("prod colour = %q, want lower-cased #38bdf8", tag.Color)
		}
	}
}

func TestNewTagReusesExistingLabelCaseInsensitively(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	a := sample()
	a.NewTags = []Tag{{Label: "Prod", Color: "#38bdf8"}}
	if _, err := s.Create(ctx, a); err != nil {
		t.Fatalf("Create a: %v", err)
	}

	b := sample()
	b.NewTags = []Tag{{Label: "prod", Color: "#f472b6"}}
	if _, err := s.Create(ctx, b); err != nil {
		t.Fatalf("Create b: %v", err)
	}

	tags, err := s.ListTags(ctx)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("ListTags = %d tags, want 1", len(tags))
	}
	if tags[0].Label != "Prod" || tags[0].Color != "#38bdf8" || tags[0].ConnectionCount != 2 {
		t.Errorf("tag = %+v, want Prod #38bdf8 used by 2", tags[0])
	}
}

func TestUpdateReplacesTags(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	c := sample()
	c.NewTags = []Tag{{Label: "a", Color: "#000000"}, {Label: "b", Color: "#000000"}}
	id, err := s.Create(ctx, c)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := s.Get(ctx, id)

	var keep int64
	for _, tag := range got.Tags {
		if tag.Label == "b" {
			keep = tag.ID
		}
	}

	got.TagIDs = []int64{keep}
	got.NewTags = []Tag{{Label: "c", Color: "#ffffff"}}
	if err := s.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = s.Get(ctx, id)
	if want := []string{"b", "c"}; !equalStrings(tagLabels(got.Tags), want) {
		t.Errorf("tags = %v, want %v", tagLabels(got.Tags), want)
	}

	// Empty write fields mean "no tags", not "leave them alone".
	got.TagIDs, got.NewTags = nil, nil
	if err := s.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = s.Get(ctx, id)
	if len(got.Tags) != 0 {
		t.Errorf("tags = %v, want none", tagLabels(got.Tags))
	}

	// The unused Tags survive.
	tags, _ := s.ListTags(ctx)
	if len(tags) != 3 {
		t.Errorf("ListTags = %d, want 3 orphan tags kept", len(tags))
	}
}

func TestBadTagRollsBackConnection(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	tests := []struct {
		name string
		c    func() Connection
	}{
		{"unknown tag id", func() Connection { c := sample(); c.TagIDs = []int64{999}; return c }},
		{"bad colour", func() Connection { c := sample(); c.NewTags = []Tag{{Label: "x", Color: "red"}}; return c }},
		{"empty label", func() Connection { c := sample(); c.NewTags = []Tag{{Label: "  ", Color: "#000000"}}; return c }},
		{"label too long", func() Connection {
			c := sample()
			c.NewTags = []Tag{{Label: "abcdefghijklmnopqrstuvwxy", Color: "#000000"}}
			return c
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.Create(ctx, tt.c()); err == nil {
				t.Fatal("Create = nil error, want failure")
			}
			conns, _ := s.List(ctx)
			if len(conns) != 0 {
				t.Errorf("List = %d connections, want the failed create rolled back", len(conns))
			}
		})
	}
}

func TestDeleteTagCascades(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	c := sample()
	c.NewTags = []Tag{{Label: "gone", Color: "#000000"}}
	id, err := s.Create(ctx, c)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := s.Get(ctx, id)

	if err := s.DeleteTag(ctx, got.Tags[0].ID); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	got, err = s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get after DeleteTag: %v", err)
	}
	if len(got.Tags) != 0 {
		t.Errorf("tags = %v, want none", tagLabels(got.Tags))
	}
	if err := s.DeleteTag(ctx, 999); !errors.Is(err, ErrTagNotFound) {
		t.Errorf("DeleteTag(999) = %v, want ErrTagNotFound", err)
	}
}

func TestDeleteConnectionKeepsTag(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	c := sample()
	c.NewTags = []Tag{{Label: "kept", Color: "#000000"}}
	id, _ := s.Create(ctx, c)
	if err := s.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	tags, _ := s.ListTags(ctx)
	if len(tags) != 1 || tags[0].ConnectionCount != 0 {
		t.Errorf("ListTags = %+v, want one unused tag", tags)
	}
}

func TestUpdateTag(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	c := sample()
	c.NewTags = []Tag{{Label: "one", Color: "#000000"}, {Label: "two", Color: "#000000"}}
	if _, err := s.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	tags, _ := s.ListTags(ctx)
	one := tags[0]

	one.Label, one.Color = "One", "#ABCDEF"
	if err := s.UpdateTag(ctx, one); err != nil {
		t.Fatalf("UpdateTag own label recased: %v", err)
	}
	one.Label = "TWO"
	if err := s.UpdateTag(ctx, one); err == nil {
		t.Error("UpdateTag onto another tag's label = nil error, want clash")
	}
	tags, _ = s.ListTags(ctx)
	if tags[0].Label != "One" || tags[0].Color != "#abcdef" {
		t.Errorf("tag = %+v, want One #abcdef", tags[0])
	}
}

func TestImportMatchesTagsByLabel(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	local := sample()
	local.NewTags = []Tag{{Label: "prod", Color: "#000000"}}
	if _, err := s.Create(ctx, local); err != nil {
		t.Fatalf("Create: %v", err)
	}

	imported := sample()
	imported.Tags = []Tag{{ID: 42, Label: "PROD", Color: "#ffffff"}, {ID: 43, Label: "new", Color: "#ffffff"}}
	if err := s.ImportData(ctx, []Connection{imported}); err != nil {
		t.Fatalf("ImportData: %v", err)
	}
	tags, _ := s.ListTags(ctx)
	if want := []string{"new", "prod"}; !equalStrings(tagLabels(tags), want) {
		t.Errorf("tags = %v, want %v", tagLabels(tags), want)
	}
}

func TestCreateTag(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	id, err := s.CreateTag(ctx, Tag{Label: " Infra ", Color: "#2DD4BF"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if _, err := s.CreateTag(ctx, Tag{Label: "infra", Color: "#000000"}); err == nil {
		t.Error("CreateTag duplicate label = nil error, want clash")
	}
	if _, err := s.CreateTag(ctx, Tag{Label: "x", Color: "teal"}); err == nil {
		t.Error("CreateTag bad colour = nil error, want failure")
	}

	tags, _ := s.ListTags(ctx)
	if len(tags) != 1 || tags[0].ID != id || tags[0].Label != "Infra" || tags[0].Color != "#2dd4bf" || tags[0].ConnectionCount != 0 {
		t.Errorf("ListTags = %+v, want one unused Infra #2dd4bf", tags)
	}
}

func TestBadTagRollsBackUpdate(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	c := sample()
	c.NewTags = []Tag{{Label: "old", Color: "#000000"}}
	id, err := s.Create(ctx, c)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	before, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	edit := before
	edit.Label = "renamed"
	edit.TagIDs = nil
	edit.NewTags = []Tag{{Label: "fresh", Color: "#ffffff"}, {Label: "broken", Color: "red"}}
	if err := s.Update(ctx, edit); err == nil {
		t.Fatal("Update = nil error, want failure")
	}

	after, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get after failed Update: %v", err)
	}
	if after.Label != before.Label {
		t.Errorf("label = %q, want %q kept by the rollback", after.Label, before.Label)
	}
	if want := []string{"old"}; !equalStrings(tagLabels(after.Tags), want) {
		t.Errorf("tags = %v, want %v kept by the rollback", tagLabels(after.Tags), want)
	}
	tags, err := s.ListTags(ctx)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if want := []string{"old"}; !equalStrings(tagLabels(tags), want) {
		t.Errorf("ListTags = %v, want %v: the valid new tag must roll back too", tagLabels(tags), want)
	}
}

func TestUpdateMissingConnection(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	c := sample()
	c.ID = 999
	c.NewTags = []Tag{{Label: "orphan", Color: "#000000"}}
	if err := s.Update(ctx, c); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update(999) = %v, want ErrNotFound", err)
	}
	tags, err := s.ListTags(ctx)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("ListTags = %v, want no tag created for a missing connection", tagLabels(tags))
	}
}

func TestFailedCreateLeavesNoNewTag(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	c := sample()
	c.NewTags = []Tag{{Label: "valid", Color: "#000000"}, {Label: "invalid", Color: "nope"}}
	if _, err := s.Create(ctx, c); err == nil {
		t.Fatal("Create = nil error, want failure")
	}
	tags, err := s.ListTags(ctx)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("ListTags = %v, want none left behind by the failed save", tagLabels(tags))
	}
}

func TestTagLabelLengthBoundary(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	// Runes, not bytes: 24 three-byte characters must still fit.
	fits := strings.Repeat("é", MaxTagLabel-12) + strings.Repeat("日", 12)
	if _, err := s.CreateTag(ctx, Tag{Label: fits, Color: "#000000"}); err != nil {
		t.Errorf("CreateTag with %d characters: %v", MaxTagLabel, err)
	}
	tooLong := strings.Repeat("a", MaxTagLabel+1)
	if _, err := s.CreateTag(ctx, Tag{Label: tooLong, Color: "#000000"}); err == nil {
		t.Errorf("CreateTag with %d characters = nil error, want failure", MaxTagLabel+1)
	}
}

func TestOverlappingTagSelectionsCollapse(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	id, err := s.CreateTag(ctx, Tag{Label: "prod", Color: "#000000"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	c := sample()
	c.TagIDs = []int64{id, id}
	c.NewTags = []Tag{
		{Label: "PROD", Color: "#ffffff"},
		{Label: "db", Color: "#ffffff"},
		{Label: "DB", Color: "#ffffff"},
	}
	connID, err := s.Create(ctx, c)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.Get(ctx, connID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if want := []string{"db", "prod"}; !equalStrings(tagLabels(got.Tags), want) {
		t.Errorf("tags = %v, want %v", tagLabels(got.Tags), want)
	}
}

func TestListAttachesTagsToTheRightConnection(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	a := sample()
	a.Label = "a"
	a.NewTags = []Tag{{Label: "zeta", Color: "#000000"}, {Label: "Alpha", Color: "#000000"}}
	if _, err := s.Create(ctx, a); err != nil {
		t.Fatalf("Create a: %v", err)
	}
	b := sample()
	b.Label = "b"
	b.NewTags = []Tag{{Label: "beta", Color: "#000000"}}
	if _, err := s.Create(ctx, b); err != nil {
		t.Fatalf("Create b: %v", err)
	}
	none := sample()
	none.Label = "none"
	if _, err := s.Create(ctx, none); err != nil {
		t.Fatalf("Create none: %v", err)
	}

	conns, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := map[string][]string{"a": {"Alpha", "zeta"}, "b": {"beta"}, "none": {}}
	if len(conns) != len(want) {
		t.Fatalf("List = %d connections, want %d", len(conns), len(want))
	}
	for _, c := range conns {
		if c.Tags == nil {
			t.Errorf("%s: Tags is nil, want an empty slice so the bridge sends []", c.Label)
		}
		if !equalStrings(tagLabels(c.Tags), want[c.Label]) {
			t.Errorf("%s: tags = %v, want %v", c.Label, tagLabels(c.Tags), want[c.Label])
		}
	}
}

func TestImportAttachesTagsAndKeepsLocalColour(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	local := sample()
	local.Label = "local"
	local.NewTags = []Tag{{Label: "prod", Color: "#000000"}}
	if _, err := s.Create(ctx, local); err != nil {
		t.Fatalf("Create: %v", err)
	}

	imported := sample()
	imported.Label = "imported"
	imported.TagIDs = []int64{999} // ids from another database must be ignored
	imported.Tags = []Tag{{ID: 42, Label: "PROD", Color: "#ffffff"}, {ID: 43, Label: "new", Color: "#ffffff"}}
	if err := s.ImportData(ctx, []Connection{imported}); err != nil {
		t.Fatalf("ImportData: %v", err)
	}

	conns, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var got *Connection
	for i := range conns {
		if conns[i].Label == "imported" {
			got = &conns[i]
		}
	}
	if got == nil {
		t.Fatal("imported connection not found")
	}
	if want := []string{"new", "prod"}; !equalStrings(tagLabels(got.Tags), want) {
		t.Errorf("imported tags = %v, want %v", tagLabels(got.Tags), want)
	}
	for _, tag := range got.Tags {
		if tag.Label == "prod" && tag.Color != "#000000" {
			t.Errorf("prod colour = %q, want the local #000000 kept", tag.Color)
		}
	}
}

func TestUpdateTagErrors(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	c := sample()
	c.NewTags = []Tag{{Label: "linked", Color: "#000000"}}
	id, err := s.Create(ctx, c)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	tag := got.Tags[0]

	if err := s.UpdateTag(ctx, Tag{ID: 999, Label: "x", Color: "#000000"}); !errors.Is(err, ErrTagNotFound) {
		t.Errorf("UpdateTag(999) = %v, want ErrTagNotFound", err)
	}
	if err := s.UpdateTag(ctx, Tag{Label: "x", Color: "#000000"}); err == nil {
		t.Error("UpdateTag without id = nil error, want failure")
	}
	if err := s.UpdateTag(ctx, Tag{ID: tag.ID, Label: "x", Color: "blue"}); err == nil {
		t.Error("UpdateTag bad colour = nil error, want failure")
	}

	// A rename keeps the link to the connection.
	tag.Label = "renamed"
	if err := s.UpdateTag(ctx, tag); err != nil {
		t.Fatalf("UpdateTag rename: %v", err)
	}
	got, err = s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get after rename: %v", err)
	}
	if want := []string{"renamed"}; !equalStrings(tagLabels(got.Tags), want) {
		t.Errorf("tags = %v, want %v", tagLabels(got.Tags), want)
	}
}
