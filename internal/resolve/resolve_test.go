package resolve

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct{ in, kind, val string }{
		{"@durov", "username", "durov"},
		{"durov", "name", "durov"},
		{"123456789", "id", "123456789"},
		{"-1001234567890", "id", "-1001234567890"},
		{"+79991234567", "phone", "79991234567"},
		{"Alice Smith", "name", "Alice Smith"},
	}
	for _, c := range cases {
		kind, val := Classify(c.in)
		if kind != c.kind || val != c.val {
			t.Errorf("Classify(%q) = %q,%q want %q,%q", c.in, kind, val, c.kind, c.val)
		}
	}
}

func TestFuzzyMatch(t *testing.T) {
	peers := []Peer{
		{ID: 1, Title: "Alice Smith", Username: "alice"},
		{ID: 2, Title: "Alice Jones"},
		{ID: 3, Title: "Work Chat"},
	}
	got := FuzzyMatch(peers, "work")
	if len(got) != 1 || got[0].ID != 3 {
		t.Fatalf("want peer 3, got %+v", got)
	}
	got = FuzzyMatch(peers, "alice")
	if len(got) != 2 {
		t.Fatalf("want 2 candidates, got %d", len(got))
	}
	got = FuzzyMatch(peers, "ALICE")
	if len(got) == 0 {
		t.Fatal("username match must be case-insensitive")
	}
	cyr := []Peer{{ID: 4, Title: "\u0412\u0430\u0441\u044f"}}
	if len(FuzzyMatch(cyr, "\u0432\u0430\u0441\u044f")) != 1 {
		t.Fatal("cyrillic titles must match case-insensitively")
	}
}

func TestDialogCacheTTL(t *testing.T) {
	dir := t.TempDir()
	peers := []Peer{{ID: 1, Title: "A", Type: "user"}}
	if err := saveDialogCache(dir, peers); err != nil {
		t.Fatal(err)
	}
	got, ok := loadDialogCache(dir, 300)
	if !ok || len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("fresh cache must load: ok=%v got=%+v", ok, got)
	}
	if _, ok := loadDialogCache(dir, 0); ok {
		t.Fatal("expired cache (ttl=0) must miss")
	}
}
