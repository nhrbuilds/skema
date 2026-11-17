package skema_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	sk "github.com/nhrbuilds/skema"
)

// stressNode is a self-recursive schema exercised concurrently; it forces the
// sync.Once-guarded Lazy path plus nested struct/slice parsing.
type stressNode struct {
	Value    int          `json:"value"`
	Children []stressNode `json:"children,omitempty"`
}

var stressNodeSchema sk.Validator[stressNode]

func init() {
	stressNodeSchema = sk.Lazy("StressNode", func() sk.Validator[stressNode] {
		return sk.Struct(func(s *sk.ObjectBuilder[stressNode]) {
			sk.Field(s, &s.Target.Value, sk.Int())
			sk.Field(s, &s.Target.Children, sk.Slice(stressNodeSchema).Optional())
		})
	})
}

func deepStressPayload(depth int) string {
	if depth == 0 {
		return `{"value":0}`
	}
	return fmt.Sprintf(`{"value":%d,"children":[%s]}`, depth, deepStressPayload(depth-1))
}

// TestStressConcurrentParse hammers shared schemas from many goroutines while
// interleaving schema derivation and JSON Schema emission. Run with -race.
func TestStressConcurrentParse(t *testing.T) {
	payload := []byte(`{"email":"user@example.com","age":30,"address":{"city":"NYC","country":"US"}}`)
	deep := deepStressPayload(32)

	const workers = 32
	const iterations = 2000

	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				u, err := SignupSchema.ParseBytes(payload)
				if err != nil {
					errs <- fmt.Errorf("parse: %w", err)
					return
				}
				if u.Email != "user@example.com" || u.Age != 30 || u.Plan != "free" {
					errs <- fmt.Errorf("bad result: %+v", u)
					return
				}
				if i%100 == 0 {
					// Derived schemas and JSON Schema emission race the parser.
					strict := SignupSchema.Strict()
					if _, err := strict.ParseBytes(payload); err != nil {
						errs <- fmt.Errorf("strict parse: %w", err)
						return
					}
					patch := SignupSchema.Partial()
					if _, err := patch.ParseString(`{"email":"a@b.com"}`); err != nil {
						errs <- fmt.Errorf("partial parse: %w", err)
						return
					}
					if _, err := SignupSchema.JSONSchemaMap(); err != nil {
						errs <- fmt.Errorf("schema emission: %w", err)
						return
					}
					n, err := stressNodeSchema.ParseString(deep)
					if err != nil || n.Value != 32 {
						errs <- fmt.Errorf("lazy parse: %+v %w", n, err)
						return
					}
					if _, err := stressNodeSchema.JSONSchemaMap(); err != nil {
						errs <- fmt.Errorf("lazy schema emission: %w", err)
						return
					}
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestStressDeepNesting(t *testing.T) {
	const depth = 200
	n, err := stressNodeSchema.ParseString(deepStressPayload(depth))
	if err != nil {
		t.Fatalf("depth %d: %v", depth, err)
	}
	if n.Value != depth || len(n.Children) != 1 {
		t.Fatalf("bad root: value=%d children=%d", n.Value, len(n.Children))
	}
}

func largeAddressPayload(items int) []byte {
	var sb strings.Builder
	sb.WriteString(`{"name":"acme","addresses":[`)
	for i := 0; i < items; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"city":"Dhaka","country":"BD"}`)
	}
	sb.WriteString(`]}`)
	return []byte(sb.String())
}

type orgWithAddresses struct {
	Name      string    `json:"name"`
	Addresses []Address `json:"addresses"`
}

var orgSchema = sk.Struct(func(s *sk.ObjectBuilder[orgWithAddresses]) {
	sk.Field(s, &s.Target.Name, sk.String().NonEmpty())
	sk.Field(s, &s.Target.Addresses, sk.Slice(AddressSchema).Max(1000))
})

func BenchmarkStructLargeArray(b *testing.B) {
	payload := largeAddressPayload(200)
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := orgSchema.ParseBytes(payload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDeepNesting(b *testing.B) {
	payload := []byte(deepStressPayload(64))
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := stressNodeSchema.ParseBytes(payload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConcurrentParse(b *testing.B) {
	payload := []byte(`{"email":"u@example.com","age":30,"plan":"pro","address":{"city":"NYC","country":"US"}}`)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := SignupSchema.ParseBytes(payload); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkJSONSchemaEmission(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := SignupSchema.JSONSchemaMap(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFormatValidation(b *testing.B) {
	emailPayload := []byte(`"user@example.com"`)
	uuidPayload := []byte(`"3d1f1e5c-7a2e-4a7b-9c1d-8e2f3a4b5c6d"`)
	emailSchema := sk.String().Email()
	uuidSchema := sk.String().UUID()
	b.Run("Email", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := emailSchema.ParseBytes(emailPayload); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("UUID", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := uuidSchema.ParseBytes(uuidPayload); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkCollectAllErrors(b *testing.B) {
	// Invalid payload with several issues; measures collect-all cost.
	payload := []byte(`{"email":"not-an-email","age":5,"plan":"nope","tags":[1],"address":{"city":"","country":"BAN"}}`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := SignupSchema.ParseBytes(payload); err == nil {
			b.Fatal("expected validation error")
		}
	}
}
