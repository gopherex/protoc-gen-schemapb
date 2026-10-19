// Package frontend translates proto messages and their schemapb options into
// schemapb schemas. It is shared by every language backend: the schema is
// the intermediate representation, and the backends only embed it.
//
// The translation adds no semantics of its own. It derives field names and
// kinds from the proto types, merges the options in, and leaves validation
// to the schemapb runtime; Schema compiles the result, so a defect (a kind
// that contradicts the proto type, an invalid CEL expression, a reserved
// field name) fails generation instead of surfacing at run time.
package frontend

import (
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/gopherex/schemapb/go/schemapb"

	optionsv1 "github.com/gopherex/protoc-gen-schemapb/schemapb/options/v1"
)

// ErrInvalid reports options that contradict the proto definition.
var ErrInvalid = errors.New("schemapb options")

// Skipped reports whether msg opts out of a generated validator. Map entry
// messages never get one.
func Skipped(msg *protogen.Message) bool {
	return msg.Desc.IsMapEntry() || messageOptions(msg).GetSkip()
}

// Schema returns the compiled-and-checked schema of msg.
func Schema(msg *protogen.Message) (*schemapb.Schema, error) {
	b := &builder{defs: map[string]*schemapb.Schema{}}

	root, err := b.message(msg)
	if err != nil {
		return nil, err
	}

	if len(b.defs) > 0 {
		root.Defs = b.defs
	}

	if _, err := schemapb.Compile(root); err != nil {
		return nil, fmt.Errorf("%s: %w", msg.Desc.FullName(), err)
	}

	return root, nil
}

type builder struct {
	defs map[string]*schemapb.Schema
}

func (b *builder) message(msg *protogen.Message) (*schemapb.Schema, error) {
	id, err := identity(msg)
	if err != nil {
		return nil, err
	}

	opts := messageOptions(msg)
	schema := &schemapb.Schema{
		Id:            id,
		Description:   opts.Description,
		Rules:         opts.GetRules(),
		MinProperties: opts.MinProperties,
		MaxProperties: opts.MaxProperties,
		Templates:     opts.GetTemplates(),
	}

	if schema.Description == nil {
		schema.Description = comment(msg.Comments.Leading)
	}

	for _, field := range msg.Fields {
		f, err := b.field(field)
		if err != nil {
			return nil, err
		}

		schema.Fields = append(schema.Fields, f)
	}

	for _, oneof := range msg.Oneofs {
		if rule := oneofRule(oneof); rule != nil {
			schema.Rules = append(schema.Rules, rule)
		}
	}

	return schema, nil
}

// oneofRule enforces a required oneof: one member must be set.
func oneofRule(oneof *protogen.Oneof) *schemapb.Schema_Field_Rule {
	if oneof.Desc.IsSynthetic() {
		return nil
	}

	opts, _ := proto.GetExtension(oneof.Desc.Options(), optionsv1.E_Oneof).(*optionsv1.OneofOptions)
	if !opts.GetRequired() {
		return nil
	}

	names := make([]string, len(oneof.Fields))
	checks := make([]string, len(oneof.Fields))

	for i, f := range oneof.Fields {
		names[i] = string(f.Desc.Name())
		checks[i] = "has(this." + names[i] + ")"
	}

	id := "oneof:" + string(oneof.Desc.Name())

	return &schemapb.Schema_Field_Rule{
		Expr:    strings.Join(checks, " || "),
		Message: "one of " + strings.Join(names, ", ") + " is required",
		Id:      &id,
	}
}

func (b *builder) field(field *protogen.Field) (*schemapb.Schema_Field, error) {
	desc := field.Desc
	out := &schemapb.Schema_Field{}

	if opt, ok := proto.GetExtension(desc.Options(), optionsv1.E_Field).(*schemapb.Schema_Field); ok && opt != nil {
		out = proto.CloneOf(opt)
	}

	if out.GetName() != "" {
		return nil, invalid(desc, "name is derived from the proto field and must not be set")
	}

	if out.GetComputed() != nil {
		return nil, invalid(desc, "computed is not allowed on a proto field")
	}

	if out.GetRequired() && !desc.HasPresence() {
		return nil, invalid(desc, "required needs field presence (optional, message or oneof member); "+
			"an implicit-presence, repeated or map field is always present — use a constraint instead")
	}

	out.Name = string(desc.Name())

	var err error

	switch {
	case desc.IsMap():
		err = b.mapKind(field, out)
	case desc.IsList():
		err = b.listKind(field, out)
	default:
		err = b.singular(field, out)
	}

	if err != nil {
		return nil, err
	}

	if out.Description == nil {
		out.Description = comment(field.Comments.Leading)
	}

	if isDeprecated(desc) {
		out.Deprecated = true
	}

	return out, nil
}

func (b *builder) listKind(field *protogen.Field, out *schemapb.Schema_Field) error {
	return listOf(field.Desc, out, func(item *schemapb.Schema_Field) error { return b.singular(field, item) })
}

// listOf merges a list kind whose single item is derived by elem; the
// option's item, if any, carries the element constraints.
func listOf(
	desc protoreflect.FieldDescriptor, out *schemapb.Schema_Field, elem func(*schemapb.Schema_Field) error,
) error {
	list := &schemapb.Schema_Field_List{}

	switch k := out.GetKind().(type) {
	case nil:
	case *schemapb.Schema_Field_List_:
		list = k.List
	default:
		return invalid(desc, "a repeated field takes the list kind, not "+kindName(out))
	}

	if len(list.GetItems()) > 1 {
		return invalid(desc, "a repeated field takes at most one item definition")
	}

	item := &schemapb.Schema_Field{}
	if len(list.GetItems()) == 1 {
		item = list.GetItems()[0]
	}

	item.Name = "item"
	if err := elem(item); err != nil {
		return err
	}

	list.Items = []*schemapb.Schema_Field{item}
	out.Kind = &schemapb.Schema_Field_List_{List: list}

	return nil
}

func (b *builder) mapKind(field *protogen.Field, out *schemapb.Schema_Field) error {
	mp := &schemapb.Schema_Field_Map{}

	switch k := out.GetKind().(type) {
	case nil:
	case *schemapb.Schema_Field_Map_:
		mp = k.Map
	default:
		return invalid(field.Desc, "a map field takes the map kind, not "+kindName(out))
	}

	if mp.GetValueSchema() != nil {
		return invalid(field.Desc, "map value_schema is derived from the proto value type; use value_field")
	}

	value := mp.GetValueField()
	if value == nil {
		value = &schemapb.Schema_Field{}
	}

	value.Name = "value"
	if err := b.singular(field.Message.Fields[1], value); err != nil { // [0] is the key
		return err
	}

	mp.ValueField = value
	out.Kind = &schemapb.Schema_Field_Map_{Map: mp}

	return nil
}

// singular sets the kind of a single (non-repeated) value of field's type
// on out, merging the kind out already carries from the options.
func (b *builder) singular(field *protogen.Field, out *schemapb.Schema_Field) error {
	desc := field.Desc

	switch desc.Kind() {
	case protoreflect.EnumKind:
		return enumKind(field, out)
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return b.messageKind(field, out)
	default:
		return scalarKind(desc, desc.Kind(), out)
	}
}

func scalarKind(desc protoreflect.FieldDescriptor, kind protoreflect.Kind, out *schemapb.Schema_Field) error {
	derived := scalarOf(kind)
	if derived == nil {
		return invalid(desc, fmt.Sprintf("unsupported proto kind %s", kind))
	}

	return merge(desc, out, derived)
}

func scalarOf(kind protoreflect.Kind) *schemapb.Schema_Field {
	f := &schemapb.Schema_Field{}

	switch kind {
	case protoreflect.BoolKind:
		f.Kind = &schemapb.Schema_Field_Bool_{Bool: &schemapb.Schema_Field_Bool{}}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		f.Kind = &schemapb.Schema_Field_Int32_{Int32: &schemapb.Schema_Field_Int32{}}
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		f.Kind = &schemapb.Schema_Field_Int64_{Int64: &schemapb.Schema_Field_Int64{}}
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		f.Kind = &schemapb.Schema_Field_Uint32{Uint32: &schemapb.Schema_Field_UInt32{}}
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		f.Kind = &schemapb.Schema_Field_Uint64{Uint64: &schemapb.Schema_Field_UInt64{}}
	case protoreflect.FloatKind:
		f.Kind = &schemapb.Schema_Field_Float_{Float: &schemapb.Schema_Field_Float{}}
	case protoreflect.DoubleKind:
		f.Kind = &schemapb.Schema_Field_Double_{Double: &schemapb.Schema_Field_Double{}}
	case protoreflect.StringKind:
		f.Kind = &schemapb.Schema_Field_String_{String_: &schemapb.Schema_Field_String{}}
	case protoreflect.BytesKind:
		f.Kind = &schemapb.Schema_Field_Bytes_{Bytes: &schemapb.Schema_Field_Bytes{}}
	default:
		return nil
	}

	return f
}

// merge combines the kind derived from the proto type with the kind the
// options set: none => derived; the same kind => the option's (it carries
// the constraints); a choice over a scalar => the option's; else an error.
func merge(desc protoreflect.FieldDescriptor, out, derived *schemapb.Schema_Field) error {
	switch {
	case out.GetKind() == nil:
		out.Kind = derived.GetKind()
	case kindName(out) == kindName(derived):
	case out.GetChoice() != nil && isScalar(derived):
	default:
		return invalid(desc, fmt.Sprintf("kind %s does not match the proto type (%s)", kindName(out), kindName(derived)))
	}

	return nil
}

func isScalar(f *schemapb.Schema_Field) bool {
	switch f.GetKind().(type) {
	case *schemapb.Schema_Field_Bool_, *schemapb.Schema_Field_Int32_, *schemapb.Schema_Field_Int64_,
		*schemapb.Schema_Field_Uint32, *schemapb.Schema_Field_Uint64,
		*schemapb.Schema_Field_Float_, *schemapb.Schema_Field_Double_,
		*schemapb.Schema_Field_String_, *schemapb.Schema_Field_Bytes_:
		return true
	default:
		return false
	}
}

func enumKind(field *protogen.Field, out *schemapb.Schema_Field) error {
	choice := &schemapb.Schema_Field_Choice{}

	switch k := out.GetKind().(type) {
	case nil:
	case *schemapb.Schema_Field_Choice_:
		choice = k.Choice
	default:
		return invalid(field.Desc, "an enum field takes the choice kind, not "+kindName(out))
	}

	if len(choice.GetOptions()) > 0 {
		return invalid(field.Desc, "choice options of an enum field are its values; use enum_value options")
	}

	for _, value := range field.Enum.Values {
		opts, _ := proto.GetExtension(value.Desc.Options(), optionsv1.E_EnumValue).(*optionsv1.EnumValueOptions)
		if opts.GetExclude() {
			continue
		}

		description := opts.GetDescription()
		if description == "" {
			if c := comment(value.Comments.Leading); c != nil {
				description = *c
			}
		}

		choice.Options = append(choice.Options, &schemapb.Schema_Field_Choice_Option{
			Value:       schemapb.StrV(string(value.Desc.Name())),
			Label:       opts.GetLabel(),
			Description: description,
			Deprecated:  isDeprecated(value.Desc),
		})
	}

	out.Kind = &schemapb.Schema_Field_Choice_{Choice: choice}

	return nil
}

func (b *builder) messageKind(field *protogen.Field, out *schemapb.Schema_Field) error {
	msg := field.Message
	derived := &schemapb.Schema_Field{}

	switch msg.Desc.FullName() {
	case "google.protobuf.Timestamp":
		derived.Kind = &schemapb.Schema_Field_Timestamp_{Timestamp: &schemapb.Schema_Field_Timestamp{}}
	case "google.protobuf.Duration":
		derived.Kind = &schemapb.Schema_Field_Duration_{Duration: &schemapb.Schema_Field_Duration{}}
	case "google.protobuf.DoubleValue", "google.protobuf.FloatValue",
		"google.protobuf.Int64Value", "google.protobuf.UInt64Value",
		"google.protobuf.Int32Value", "google.protobuf.UInt32Value",
		"google.protobuf.BoolValue", "google.protobuf.StringValue", "google.protobuf.BytesValue":
		return scalarKind(field.Desc, msg.Fields[0].Desc.Kind(), out)
	case "google.protobuf.Struct", "google.protobuf.Value", "google.protobuf.ListValue":
		derived.Kind = &schemapb.Schema_Field_Json_{Json: &schemapb.Schema_Field_Json{}}
	case "google.protobuf.Empty":
		derived.Kind = &schemapb.Schema_Field_Object_{Object: &schemapb.Schema_Field_Object{Schema: &schemapb.Schema{}}}
	case "google.protobuf.FieldMask":
		return listOf(field.Desc, out, func(item *schemapb.Schema_Field) error {
			return scalarKind(field.Desc, protoreflect.StringKind, item)
		})
	case "google.protobuf.Any":
		return invalid(field.Desc, "google.protobuf.Any is not supported")
	default:
		if out.GetKind() != nil {
			return invalid(field.Desc, "a message field takes no kind (it is a ref to "+string(msg.Desc.FullName())+")")
		}

		if err := b.def(msg); err != nil {
			return err
		}

		out.Kind = &schemapb.Schema_Field_Ref_{Ref: &schemapb.Schema_Field_Ref{
			Target: &schemapb.Schema_Field_Ref_Name{Name: string(msg.Desc.FullName())},
		}}

		return nil
	}

	return merge(field.Desc, out, derived)
}

// def registers the schema of msg in the root defs, once. The placeholder
// is stored before the fields are built, so recursive messages terminate.
func (b *builder) def(msg *protogen.Message) error {
	key := string(msg.Desc.FullName())
	if _, ok := b.defs[key]; ok {
		return nil
	}

	placeholder := &schemapb.Schema{}
	b.defs[key] = placeholder

	schema, err := b.message(msg)
	if err != nil {
		return err
	}

	proto.Merge(placeholder, schema)

	return nil
}

// identity is namespace = the file namespace option or the proto package,
// name = the message name within the package ("Outer.Inner"), version =
// the file version option.
func identity(msg *protogen.Message) (*schemapb.SchemaIdentity, error) {
	file := msg.Desc.ParentFile()
	opts, _ := proto.GetExtension(file.Options(), optionsv1.E_File).(*optionsv1.FileOptions)

	version, err := schemapb.ParseVersion(opts.GetVersion())
	if err != nil {
		return nil, fmt.Errorf("%w: %s: file version: %w", ErrInvalid, file.Path(), err)
	}

	namespace := string(file.Package())
	if opts != nil && opts.Namespace != nil {
		namespace = opts.GetNamespace()
	}

	name := strings.TrimPrefix(string(msg.Desc.FullName()), string(file.Package())+".")

	return &schemapb.SchemaIdentity{Namespace: namespace, Name: name, Version: version.String()}, nil
}

func messageOptions(msg *protogen.Message) *optionsv1.MessageOptions {
	opts, ok := proto.GetExtension(msg.Desc.Options(), optionsv1.E_Message).(*optionsv1.MessageOptions)
	if ok && opts != nil {
		return opts
	}

	return &optionsv1.MessageOptions{}
}

// kindName is the schemapb kind oneof case ("int32", "list", "ref", ...).
func kindName(f *schemapb.Schema_Field) string {
	m := f.ProtoReflect()
	if fd := m.WhichOneof(m.Descriptor().Oneofs().ByName("kind")); fd != nil {
		return string(fd.Name())
	}

	return "none"
}

type deprecatable interface {
	Options() protoreflect.ProtoMessage
}

func isDeprecated(desc deprecatable) bool {
	type deprecated interface{ GetDeprecated() bool }

	d, ok := desc.Options().(deprecated)

	return ok && d.GetDeprecated()
}

// comment turns a leading comment into a description; empty => nil.
func comment(c protogen.Comments) *string {
	lines := strings.Split(strings.TrimSpace(string(c)), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}

	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if text == "" {
		return nil
	}

	return &text
}

func invalid(desc protoreflect.FieldDescriptor, msg string) error {
	return fmt.Errorf("%w: %s: %s", ErrInvalid, desc.FullName(), msg)
}
