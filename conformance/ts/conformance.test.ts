// Runs the shared cases through the generated TypeScript validators and
// compares with the goldens the Go harness writes: both backends embed the
// same schema, and both runtimes must produce the same ValidationResult.
import { readFileSync } from "node:fs";
import { join } from "node:path";
import {
  createRegistry,
  type DescMessage,
  equals,
  fromJson,
  type JsonValue,
  toJson,
} from "@bufbuild/protobuf";
import {
  SchemaSchema,
  ValidationFailure,
  ValidationResultSchema,
  type Validator,
} from "@gopherex/schemapb";
import { describe, expect, it } from "vitest";
import { DocValidator } from "./gen/conformance/edition/v1/edition_schemapb.js";
import { RefValidator } from "./gen/conformance/other/v1/other_schemapb.js";
import { AddressValidator, NodeValidator, User_SettingsValidator, UserValidator } from "./gen/conformance/v1/conformance_schemapb.js";
import { file_conformance_other_v1_other } from "./gen/conformance/other/v1/other_pb.js";
import { file_conformance_v1_conformance } from "./gen/conformance/v1/conformance_pb.js";
import { EnvelopeValidator } from "./gen/conformance/v1/envelope_schemapb.js";
import { KindsValidator } from "./gen/conformance/v1/kinds_schemapb.js";
import { FailureValidator, ReservedValidator } from "./gen/conformance/v1/reserved_schemapb.js";

const root = join(import.meta.dirname, "..");

interface Case {
  name: string;
  message: string;
  input: JsonValue;
  resolve?: boolean;
}

// protoJSON of Any needs the packed types.
const registry = createRegistry(file_conformance_other_v1_other, file_conformance_v1_conformance);
const json = { registry };

const validators: Record<string, Validator<DescMessage>> = {
  "conformance.v1.User": UserValidator,
  "conformance.v1.Address": AddressValidator,
  "conformance.v1.Node": NodeValidator,
  "conformance.v1.User.Settings": User_SettingsValidator,
  "conformance.v1.Kinds": KindsValidator,
  "conformance.other.v1.Ref": RefValidator,
  "conformance.edition.v1.Doc": DocValidator,
  "conformance.v1.Envelope": EnvelopeValidator,
  "conformance.v1.Reserved": ReservedValidator,
  "conformance.v1.Failure": FailureValidator,
};

const cases = JSON.parse(readFileSync(join(root, "cases.json"), "utf8")) as Case[];

function golden(name: string): JsonValue {
  return JSON.parse(readFileSync(join(root, "golden", name), "utf8")) as JsonValue;
}

describe("conformance", () => {
  for (const tc of cases) {
    it(tc.name, () => {
      const v = validators[tc.message];
      expect(v, tc.message).toBeDefined();
      if (v === undefined) return;

      const msg = fromJson(v.desc, tc.input, json);
      const res = v.check(msg);
      const want = fromJson(ValidationResultSchema, golden(`${tc.name}.json`));
      // Compare as protoJSON first for a readable diff, then exactly.
      expect(toJson(ValidationResultSchema, res)).toEqual(toJson(ValidationResultSchema, want));
      expect(equals(ValidationResultSchema, res, want)).toBe(true);

      const blocking = res.errors.some((e) => e.severity !== 2);
      if (blocking) {
        expect(() => v.validate(msg)).toThrow(ValidationFailure);
      } else {
        expect(v.validate(msg).errors).toEqual(res.errors);
      }

      if (tc.resolve) {
        const resolved = v.resolve(msg);
        expect(equals(ValidationResultSchema, resolved, res)).toBe(true);
        const after = fromJson(v.desc, golden(`${tc.name}.resolved.json`), json);
        expect(toJson(v.desc, msg, json)).toEqual(toJson(v.desc, after, json));
        expect(equals(v.desc, msg, after, json)).toBe(true);
      }
    });
  }

  for (const [name, v] of Object.entries(validators)) {
    it(`schema of ${name} matches the Go golden`, () => {
      const want = fromJson(SchemaSchema, golden(`schema.${name}.json`));
      expect(equals(SchemaSchema, v.schema, want)).toBe(true);
    });
  }
});
