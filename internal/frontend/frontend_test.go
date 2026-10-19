package frontend_test

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/pluginpb"

	"github.com/gopherex/protoc-gen-schemapb/internal/frontend"
	optionsv1 "github.com/gopherex/protoc-gen-schemapb/schemapb/options/v1"
)

// file wraps one message body (FieldDescriptorProto text) into a proto3
// file that imports the option and well-known protos.
func file(t *testing.T, body string) *protogen.Message {
	t.Helper()

	src := `
name: "t/v1/t.proto" package: "t.v1" syntax: "proto3"
dependency: ["schemapb/options/v1/options.proto", "google/protobuf/timestamp.proto", "google/protobuf/struct.proto"]
options { go_package: "example.com/t/v1;tv1" }
enum_type { name: "E" value { name: "E_UNSPECIFIED" number: 0 } value { name: "E_A" number: 1 } }
message_type { name: "Inner" field { name: "x" number: 1 label: LABEL_OPTIONAL type: TYPE_STRING } }
message_type { name: "M" ` + body + ` }
`

	fdp := &descriptorpb.FileDescriptorProto{}
	if err := prototext.Unmarshal([]byte(src), fdp); err != nil {
		t.Fatal(err)
	}

	deps := []*descriptorpb.FileDescriptorProto{
		protodesc.ToFileDescriptorProto(descriptorpb.File_google_protobuf_descriptor_proto),
		protodesc.ToFileDescriptorProto(timestamppb.File_google_protobuf_timestamp_proto),
		protodesc.ToFileDescriptorProto(structpb.File_google_protobuf_struct_proto),
	}
	deps = append(deps, schemapbFiles()...)
	deps = append(deps, protodesc.ToFileDescriptorProto(optionsv1.File_schemapb_options_v1_options_proto), fdp)

	plugin, err := protogen.Options{}.New(&pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{"t/v1/t.proto"},
		ProtoFile:      deps,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range plugin.Files {
		if f.Desc.Path() == "t/v1/t.proto" {
			return f.Messages[1]
		}
	}

	t.Fatal("test file not found")

	return nil
}

func TestSchemaRejects(t *testing.T) {
	t.Parallel()

	const opt = ` options { [schemapb.options.v1.field] { %s } }`

	field := func(spec, option string) string {
		if option != "" {
			spec += strings.Replace(opt, "%s", option, 1)
		}

		return "field { " + spec + " }"
	}

	cases := []struct {
		name, body, want string
	}{
		{
			"name set", field(`name: "a" number: 1 label: LABEL_OPTIONAL type: TYPE_STRING`, `name: "x"`),
			"name is derived",
		},
		{
			"computed", field(`name: "a" number: 1 label: LABEL_OPTIONAL type: TYPE_STRING`, `computed { expr: "1" }`),
			"computed is not allowed",
		},
		{
			"required implicit", field(`name: "a" number: 1 label: LABEL_OPTIONAL type: TYPE_STRING`, `required: true`),
			"required needs field presence",
		},
		{
			"required repeated", field(`name: "a" number: 1 label: LABEL_REPEATED type: TYPE_STRING`, `required: true`),
			"required needs field presence",
		},
		{
			"kind mismatch", field(`name: "a" number: 1 label: LABEL_OPTIONAL type: TYPE_STRING`, `int32 { gt: 1 }`),
			"kind int32 does not match the proto type (string)",
		},
		{
			"sint32 is int32 not int64", field(`name: "a" number: 1 label: LABEL_OPTIONAL type: TYPE_SINT32`, `int64 {}`),
			"kind int64 does not match the proto type (int32)",
		},
		{
			"list on scalar", field(`name: "a" number: 1 label: LABEL_REPEATED type: TYPE_STRING`, `string {}`),
			"a repeated field takes the list kind",
		},
		{"two items", field(`name: "a" number: 1 label: LABEL_REPEATED type: TYPE_STRING`,
			`list { items { string {} } items { string {} } }`), "at most one item"},
		{"item kind", field(`name: "a" number: 1 label: LABEL_REPEATED type: TYPE_STRING`,
			`list { items { bool {} } }`), "kind bool does not match"},
		{"enum options", field(`name: "a" number: 1 label: LABEL_OPTIONAL type: TYPE_ENUM type_name: ".t.v1.E"`,
			`choice { options { value { string_value: "x" } } }`), "choice options of an enum field are its values"},
		{"message kind", field(`name: "a" number: 1 label: LABEL_OPTIONAL type: TYPE_MESSAGE type_name: ".t.v1.Inner"`,
			`object {}`), "a message field takes no kind"},
		{"wkt mismatch", field(`name: "a" number: 1 label: LABEL_OPTIONAL type: TYPE_MESSAGE type_name: ".google.protobuf.Timestamp"`,
			`duration {}`), "kind duration does not match the proto type (timestamp)"},
		{
			"bad cel", field(`name: "a" number: 1 label: LABEL_OPTIONAL type: TYPE_STRING`, `rules { expr: "this +" message: "m" }`),
			"Syntax error",
		},
		{
			"reserved word", field(`name: "in" number: 1 label: LABEL_OPTIONAL type: TYPE_STRING`, ``),
			"CEL reserved word",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := frontend.Schema(file(t, tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestSchemaAccepts(t *testing.T) {
	t.Parallel()

	cases := []struct{ name, body string }{
		{"choice over string", `field { name: "a" number: 1 label: LABEL_OPTIONAL type: TYPE_STRING
			options { [schemapb.options.v1.field] { choice { options { value { string_value: "x" } } } } } }`},
		{"enum open", `field { name: "a" number: 1 label: LABEL_OPTIONAL type: TYPE_ENUM type_name: ".t.v1.E"
			options { [schemapb.options.v1.field] { choice { open: true } } } }`},
		{"required message", `field { name: "a" number: 1 label: LABEL_OPTIONAL type: TYPE_MESSAGE type_name: ".t.v1.Inner"
			options { [schemapb.options.v1.field] { required: true } } }`},
		{"json struct", `field { name: "a" number: 1 label: LABEL_OPTIONAL type: TYPE_MESSAGE type_name: ".google.protobuf.Struct"
			options { [schemapb.options.v1.field] { json {} } } }`},
		{"list without kind", `field { name: "a" number: 1 label: LABEL_REPEATED type: TYPE_MESSAGE type_name: ".t.v1.Inner" }`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := frontend.Schema(file(t, tc.body)); err != nil {
				t.Fatal(err)
			}
		})
	}
}
