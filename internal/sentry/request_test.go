package sentry

import "testing"

func TestParseRequest(t *testing.T) {
	object := ParseRequest([]byte(`{"method": "GET", "url": 5, "headers": {"X-Count": 3, "Accept": "*/*"}}`))

	if object == nil || object.Method != "GET" || object.URL != "5" || object.Headers["X-Count"] != "3" || object.Headers["Accept"] != "*/*" {
		t.Fatalf("object headers: %+v", object)
	}

	pairs := ParseRequest([]byte(`{"method": "GET", "headers": [["Accept", "*/*"], ["X-Count", 3], ["broken"]]}`))

	if pairs == nil || len(pairs.Headers) != 2 || pairs.Headers["X-Count"] != "3" {
		t.Fatalf("header pairs: %+v", pairs)
	}

	if ParseRequest([]byte(`"GET /"`)) != nil || ParseRequest([]byte(`null`)) != nil {
		t.Fatal("a request that is not an object is not shown")
	}
}
