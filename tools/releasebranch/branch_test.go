package main

import "testing"

// queryOutput is the start of a real `bazel query --output=xml` response,
// including the xml 1.1 declaration that Go's decoder rejects.
var queryOutput = `<?xml version="1.1" encoding="UTF-8" standalone="no"?>
<query version="2">
    <rule class="image_push" name="//cmd/kontakted:push">
        <string name="name" value="push"/>
        <string name="registry" value="ghcr.io"/>
        <string name="repository" value="teapotovh/kontakted"/>
    </rule>
    <rule class="image_push" name="//cmd/netd:push_ttl">
        <string name="name" value="push_ttl"/>
        <string name="registry" value="ocihub.com"/>
        <string name="repository" value="teapotovh_netd"/>
    </rule>
</query>
`

func TestParseQuery(t *testing.T) {
	t.Parallel()

	var query queryResult

	if err := parseQuery([]byte(queryOutput), &query); err != nil {
		t.Fatal(err)
	}

	if len(query.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(query.Rules))
	}

	repo, ok := pushImage(query.Rules[0])
	if !ok {
		t.Fatal("expected ghcr.io rule to be accepted")
	}

	if repo != "ghcr.io/teapotovh/kontakted" {
		t.Fatalf("expected ghcr.io/teapotovh/kontakted, got %s", repo)
	}

	if _, ok := pushImage(query.Rules[1]); ok {
		t.Fatal("expected mirror push to be rejected")
	}
}
