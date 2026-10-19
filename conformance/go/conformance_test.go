// Package conformance_test runs the shared cases through the Go generated
// code and writes (with -update) or compares the goldens every backend must
// reproduce.
package conformance_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/gopherex/schemapb/go/schemapb"

	conformancev1 "github.com/gopherex/protoc-gen-schemapb/conformance/gen/go/conformance/v1"
)

var update = flag.Bool("update", false, "rewrite golden files")

type validated interface {
	proto.Message
	Validate() error
	ValidateResult() (*schemapb.ValidationResult, error)
	SchemaPB() *schemapb.Schema
}

type testCase struct {
	Name    string          `json:"name"`
	Message string          `json:"message"`
	Input   json.RawMessage `json:"input"`
}

func loadCases(t *testing.T) []testCase {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}

	var cases []testCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}

	return cases
}

//nolint:ireturn // the generated types are only known through the registry
func newMessage(t *testing.T, name string) validated {
	t.Helper()

	_ = conformancev1.File_conformance_v1_conformance_proto // link the generated types

	mt, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(name))
	if err != nil {
		t.Fatal(err)
	}

	msg, ok := mt.New().Interface().(validated)
	if !ok {
		t.Fatalf("%s has no generated validator", name)
	}

	return msg
}

func TestConformance(t *testing.T) {
	t.Parallel()

	for _, tc := range loadCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			msg := newMessage(t, tc.Message)
			if err := protojson.Unmarshal(tc.Input, msg); err != nil {
				t.Fatal(err)
			}

			res, err := msg.ValidateResult()
			if err != nil {
				t.Fatal(err)
			}

			verr := msg.Validate()
			if res.Blocking() {
				var got *schemapb.ValidationResult
				if !errors.As(verr, &got) || !proto.Equal(got, res) {
					t.Fatalf("Validate() = %v, want the blocking result", verr)
				}
			} else if verr != nil {
				t.Fatalf("Validate() = %v, want nil", verr)
			}

			checkGolden(t, filepath.Join("..", "golden", tc.Name+".json"), res)
		})
	}
}

// TestSchemas pins the generated schema of every message: it is the IR
// every backend embeds, so a frontend change shows up here first.
func TestSchemas(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"conformance.v1.User", "conformance.v1.User.Settings", "conformance.v1.Address", "conformance.v1.Node",
	} {
		msg := newMessage(t, name)
		checkGolden(t, filepath.Join("..", "golden", "schema."+name+".json"), msg.SchemaPB())
	}
}

func checkGolden(t *testing.T, path string, m proto.Message) {
	t.Helper()

	got := stableJSON(t, m)
	if *update {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}

		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s missing (run: go test ./conformance/go -update): %v", path, err)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("%s differs:\n got: %s\nwant: %s", path, got, want)
	}
}

// stableJSON reformats protojson output deterministically.
func stableJSON(t *testing.T, m proto.Message) []byte {
	t.Helper()

	raw, err := protojson.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}

	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}

	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	return append(out, '\n')
}
