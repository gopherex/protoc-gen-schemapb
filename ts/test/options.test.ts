import { create, getExtension, setExtension } from "@bufbuild/protobuf";
import { FieldOptionsSchema } from "@bufbuild/protobuf/wkt";
import { file_schemapb_schema, Schema_FieldSchema } from "@gopherex/schemapb";
import { describe, expect, it } from "vitest";
import { field, file_schemapb_options_v1_options } from "../src/index.js";

describe("options_pb", () => {
  it("depends on the schemapb protos of @gopherex/schemapb", () => {
    expect(file_schemapb_options_v1_options.dependencies).toContain(file_schemapb_schema);
  });

  it("reads the field option as a runtime Schema_Field", () => {
    const opts = create(FieldOptionsSchema);
    setExtension(opts, field, create(Schema_FieldSchema, { required: true }));
    const got = getExtension(opts, field);
    expect(got.$typeName).toBe("schemapb.Schema.Field");
    expect(got.required).toBe(true);
  });
});
