package tsgen

import (
	"regexp"
	"strconv"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// descNames reproduces protobuf-es (protoplugin names.ts) so the generated
// code imports exactly the identifiers protoc-gen-es emits: identifiers drop
// the package and join nesting with "_", messages and enums get "Schema",
// clashes take a "$", "$1", ... salt, reserved words a "$" suffix.
func descNames(file protoreflect.FileDescriptor) map[protoreflect.FullName]string {
	types := nestedTypes(file)
	taken := map[string]bool{}

	// First pass: shape names (messages and enums) claim their identifiers.
	for _, d := range types {
		if isShape(d) {
			claim(taken, func(salt string) string { return safeIdentifier(identifier(file, d) + salt) })
		}
	}

	claim(taken, func(salt string) string {
		return safeIdentifier("file_" + nonIdent.ReplaceAllString(strings.TrimSuffix(file.Path(), ".proto"), "_") + salt)
	})

	names := map[protoreflect.FullName]string{}

	for _, d := range types {
		suffix := ""
		if isShape(d) {
			suffix = "Schema"
		}

		names[d.FullName()] = claim(taken, func(salt string) string {
			return safeIdentifier(identifier(file, d) + suffix + salt)
		})
	}

	return names
}

func claim(taken map[string]bool, ideal func(salt string) string) string {
	for i := 0; ; i++ {
		salt := ""

		switch {
		case i == 1:
			salt = "$"
		case i > 1:
			salt = "$" + strconv.Itoa(i-1)
		}

		if name := ideal(salt); !taken[name] {
			taken[name] = true

			return name
		}
	}
}

func isShape(d protoreflect.Descriptor) bool {
	switch d.(type) {
	case protoreflect.MessageDescriptor, protoreflect.EnumDescriptor:
		return true
	default:
		return false
	}
}

// nestedTypes is protobuf-es reflect nestedTypes(): messages depth-first
// (each followed by its nested types), then enums, services, extensions.
func nestedTypes(file protoreflect.FileDescriptor) []protoreflect.Descriptor {
	out := make([]protoreflect.Descriptor, 0, file.Messages().Len()+file.Enums().Len())

	var message func(m protoreflect.MessageDescriptor)

	message = func(m protoreflect.MessageDescriptor) {
		for i := range m.Messages().Len() {
			out = append(out, m.Messages().Get(i))
			message(m.Messages().Get(i))
		}

		for i := range m.Enums().Len() {
			out = append(out, m.Enums().Get(i))
		}

		for i := range m.Extensions().Len() {
			out = append(out, m.Extensions().Get(i))
		}
	}

	for i := range file.Messages().Len() {
		out = append(out, file.Messages().Get(i))
		message(file.Messages().Get(i))
	}

	for i := range file.Enums().Len() {
		out = append(out, file.Enums().Get(i))
	}

	for i := range file.Services().Len() {
		out = append(out, file.Services().Get(i))
	}

	for i := range file.Extensions().Len() {
		out = append(out, file.Extensions().Get(i))
	}

	return out
}

// identifier is the type name without the package, dots joined by "_". An
// extension's type name is its scope plus its own name.
func identifier(file protoreflect.FileDescriptor, d protoreflect.Descriptor) string {
	name := string(d.FullName())
	if pkg := string(file.Package()); pkg != "" {
		name = strings.TrimPrefix(name, pkg+".")
	}

	return strings.ReplaceAll(name, ".", "_")
}

var nonIdent = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

//nolint:gochecknoglobals // the protobuf-es reserved identifier list
var reserved = map[string]bool{
	"break": true, "case": true, "catch": true, "class": true, "const": true, "continue": true,
	"debugger": true, "default": true, "delete": true, "do": true, "else": true, "export": true,
	"extends": true, "false": true, "finally": true, "for": true, "function": true, "if": true,
	"import": true, "in": true, "instanceof": true, "new": true, "null": true, "return": true,
	"super": true, "switch": true, "this": true, "throw": true, "true": true, "try": true,
	"typeof": true, "var": true, "void": true, "while": true, "with": true, "yield": true,
	"enum": true, "implements": true, "interface": true, "let": true, "package": true,
	"private": true, "protected": true, "public": true, "static": true,
	"Object": true, "bigint": true, "number": true, "boolean": true, "string": true, "object": true,
	"globalThis": true, "Uint8Array": true, "Partial": true,
}

func safeIdentifier(name string) string {
	if reserved[name] {
		return name + "$"
	}

	return name
}
