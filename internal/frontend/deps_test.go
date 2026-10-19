package frontend_test

import (
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/gopherex/schemapb/go/schemapb"
)

// schemapbFiles are the contract protos the option file imports, in
// dependency order (timestamp comes earlier in the request).
func schemapbFiles() []*descriptorpb.FileDescriptorProto {
	return []*descriptorpb.FileDescriptorProto{
		protodesc.ToFileDescriptorProto(durationpb.File_google_protobuf_duration_proto),
		protodesc.ToFileDescriptorProto(schemapb.File_schemapb_value_proto),
		protodesc.ToFileDescriptorProto(schemapb.File_schemapb_schema_proto),
	}
}
