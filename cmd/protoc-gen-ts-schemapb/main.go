// Command protoc-gen-ts-schemapb generates schemapb validators for
// TypeScript messages produced by protoc-gen-es (protobuf-es v2).
//
// Parameters: import_extension=none|js|ts (as protoc-gen-es, default none),
// runtime_module=<module> (default @gopherex/schemapb),
// target=ts|js|dts or a + combination (as protoc-gen-es, default ts).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"

	"github.com/gopherex/protoc-gen-schemapb/internal/tsgen"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "protoc-gen-ts-schemapb:", err)
		os.Exit(1)
	}
}

func run() error {
	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read request: %w", err)
	}

	req := &pluginpb.CodeGeneratorRequest{}
	if err := proto.Unmarshal(in, req); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}

	// protogen needs a Go import path for every file; TypeScript protos
	// rarely declare one, and it plays no part in the TypeScript output.
	for _, f := range req.GetProtoFile() {
		if f.GetOptions().GetGoPackage() == "" {
			if f.Options == nil {
				f.Options = &descriptorpb.FileOptions{}
			}

			f.Options.GoPackage = proto.String("schemapb.invalid/" + path.Dir(f.GetName()))
		}
	}

	var opts tsgen.Options

	var flags flag.FlagSet

	flags.Func("import_extension", "none, js or ts", func(v string) error { return opts.Set("import_extension", v) })
	flags.Func("runtime_module", "runtime module", func(v string) error { return opts.Set("runtime_module", v) })
	flags.Func("target", "ts", func(v string) error { return opts.Set("target", v) })

	plugin, err := protogen.Options{ParamFunc: flags.Set}.New(req)
	if err != nil {
		return fmt.Errorf("parse request: %w", err)
	}

	plugin.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL |
		pluginpb.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS)
	plugin.SupportedEditionsMinimum = descriptorpb.Edition_EDITION_PROTO2
	plugin.SupportedEditionsMaximum = descriptorpb.Edition_EDITION_2023

	for _, f := range plugin.Files {
		if !f.Generate {
			continue
		}

		if err := tsgen.GenerateFile(plugin, f, opts); err != nil {
			plugin.Error(fmt.Errorf("%s: %w", f.Desc.Path(), err))

			break
		}
	}

	out, err := proto.Marshal(plugin.Response())
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}

	if _, err := os.Stdout.Write(out); err != nil {
		return fmt.Errorf("write response: %w", err)
	}

	return nil
}
