package store

import (
	"context"
	"errors"
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
