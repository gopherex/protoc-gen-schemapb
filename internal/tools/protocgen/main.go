// Command protocgen runs a protoc plugin over a compiled FileDescriptorSet:
// the build of this repository compiles protos once (easyp
// --descriptor_set_out --include_imports) and drives every plugin through
// this command, so no protoc and no registry are involved.
//
//	protocgen -set descriptors.binpb -plugin bin/protoc-gen-go -out . \
//	    -opt paths=source_relative schemapb/options/v1/options.proto
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

var (
	errUsage  = errors.New("usage: protocgen -set FILE -plugin EXE [-out DIR] [-opt PARAM] FILE.proto")
	errPlugin = errors.New("plugin failed")
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "protocgen:", err)
		os.Exit(1)
	}
}

func run() error {
	set := flag.String("set", "", "FileDescriptorSet with all imports")
	plugin := flag.String("plugin", "", "plugin executable")
	out := flag.String("out", ".", "output directory")
	opt := flag.String("opt", "", "plugin parameter")

	flag.Parse()

	if *set == "" || *plugin == "" || flag.NArg() == 0 {
		return errUsage
	}

	raw, err := os.ReadFile(*set)
	if err != nil {
		return fmt.Errorf("read descriptor set: %w", err)
	}

	fds := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(raw, fds); err != nil {
		return fmt.Errorf("decode %s: %w", *set, err)
	}

	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: flag.Args(),
		ProtoFile:      fds.GetFile(),
	}
	if *opt != "" {
		req.Parameter = opt
	}

	in, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(context.Background(), *plugin) //nolint:gosec // the plugin is the caller's choice
	cmd.Stdin = bytes.NewReader(in)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w: %s", *plugin, err, strings.TrimSpace(stderr.String()))
	}

	resp := &pluginpb.CodeGeneratorResponse{}
	if err := proto.Unmarshal(stdout.Bytes(), resp); err != nil {
		return fmt.Errorf("decode %s response: %w", *plugin, err)
	}

	if resp.Error != nil {
		return fmt.Errorf("%w: %s: %s", errPlugin, *plugin, resp.GetError())
	}

	for _, f := range resp.GetFile() {
		path := filepath.Join(*out, f.GetName())
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil { //nolint:mnd // directory mode
			return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
		}

		//nolint:gosec,mnd // generated sources are world-readable
		if err := os.WriteFile(path, []byte(f.GetContent()), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}

	return nil
}
