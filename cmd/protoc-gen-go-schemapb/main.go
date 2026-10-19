// Command protoc-gen-go-schemapb generates schemapb validators for Go
// messages produced by protoc-gen-go.
package main

import (
	"fmt"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/pluginpb"

	"github.com/gopherex/protoc-gen-schemapb/internal/gogen"
)

func main() {
	protogen.Options{}.Run(func(plugin *protogen.Plugin) error {
		plugin.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)

		for _, f := range plugin.Files {
			if !f.Generate {
				continue
			}

			if err := gogen.GenerateFile(plugin, f); err != nil {
				return fmt.Errorf("%s: %w", f.Desc.Path(), err)
			}
		}

		return nil
	})
}
