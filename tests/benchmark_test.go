package skema_test

import (
	"bytes"
	"encoding/json"
	"testing"
)

var benchPayload = []byte(`{"email":"u@example.com","age":30,"plan":"pro","address":{"city":"NYC","country":"US"}}`)

func BenchmarkSkemaStruct(b *testing.B) {
	schema := SignupSchema
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := schema.ParseBytes(benchPayload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSkemaStructParseAny(b *testing.B) {
	schema := SignupSchema
	dec := json.NewDecoder(bytes.NewReader(benchPayload))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := schema.ParseAny(tree); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodingJSON(b *testing.B) {
	var v SignupInput
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := json.Unmarshal(benchPayload, &v); err != nil {
			b.Fatal(err)
		}
	}
}
