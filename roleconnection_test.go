package starlings

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"strings"
	"testing"
)

func TestBearerToken(t *testing.T) {
	const raw = "mfa.abcdef123456"
	cases := map[string]string{
		raw:              "Bearer " + raw, // bare token
		"Bearer " + raw:  "Bearer " + raw, // already prefixed
		"bearer " + raw:  "Bearer " + raw, // wrong case, normalised
		"":               "Bearer ",
		"Bearertoken123": "Bearer Bearertoken123", // no space, so not a prefix
	}
	for in, want := range cases {
		if got := BearerToken(in); got != want {
			t.Errorf("BearerToken(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRegisterRoleConnectionMetadataLimit checks the guard fires before any
// network call, so exceeding the limit is a clear Go error rather than a 400.
func TestRegisterRoleConnectionMetadataLimit(t *testing.T) {
	c := testClient()
	records := make([]RoleConnectionMetadata, MaxRoleConnectionMetadata+1)

	_, err := c.RegisterRoleConnectionMetadata(context.Background(), 1234, records)
	if !errors.Is(err, ErrTooManyMetadataRecords) {
		t.Fatalf("err = %v, want ErrTooManyMetadataRecords", err)
	}
}

// TestRoleConnectionJSON pins the wire format: metadata values are strings
// even when they represent numbers, and empty fields are omitted rather than
// sent as null.
func TestRoleConnectionJSON(t *testing.T) {
	conn := RoleConnection{
		PlatformName:     "Wuthering Waves",
		PlatformUsername: "Server: SEA",
		Metadata: map[string]string{
			"union_level": "72",
			"echoes":      "608",
		},
	}

	b, err := json.Marshal(conn)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"platform_name":"Wuthering Waves"`,
		`"platform_username":"Server: SEA"`,
		`"union_level":"72"`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("marshalled body %s\nis missing %s", b, want)
		}
	}

	// An empty connection must not emit null fields, which Discord rejects.
	b, err = json.Marshal(RoleConnection{})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "{}" {
		t.Errorf("empty RoleConnection marshalled to %s, want {}", b)
	}
}

// TestMetadataTypeValues transcribes the spec's MetadataItemTypes enum. A
// wrong number here silently changes what a linked-role rule compares, which
// nothing else would catch.
func TestMetadataTypeValues(t *testing.T) {
	want := map[MetadataType]int{
		MetadataIntegerLessThanOrEqual:     1,
		MetadataIntegerGreaterThanOrEqual:  2,
		MetadataIntegerEqual:               3,
		MetadataIntegerNotEqual:            4,
		MetadataDateTimeLessThanOrEqual:    5,
		MetadataDateTimeGreaterThanOrEqual: 6,
		MetadataBooleanEqual:               7,
		MetadataBooleanNotEqual:            8,
	}
	for got, expect := range want {
		if int(got) != expect {
			t.Errorf("metadata type = %d, want %d", got, expect)
		}
	}
}
