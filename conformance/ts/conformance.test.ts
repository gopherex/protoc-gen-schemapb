// Runs the shared cases through the generated TypeScript validators and
// compares with the goldens the Go harness writes: both backends embed the
// same schema, and both runtimes must produce the same ValidationResult.
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { type DescMessage, equals, fromJson, type JsonValue, toJson } from "@bufbuild/protobuf";
import {
  SchemaSchema,
  ValidationFailure,
  ValidationResultSchema,
  type Validator,
} from "@gopherex/schemapb";
import { describe, expect, it } from "vitest";
import { AddressValidator, NodeValidator, User_SettingsValidator, UserValidator } from "./gen/conformance/v1/conformance_schemapb.js";

const root = join(import.meta.dirname, "..");

interface Case {
  name: string;
  message: string;
  input: JsonValue;
}

const validators: Record<string, Validator<DescMessage>> = {
  "conformance.v1.User": UserValidator,
  "conformance.v1.Address": AddressValidator,
  "conformance.v1.Node": NodeValidator,
  "conformance.v1.User.Settings": User_SettingsValidator,
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

      const msg = fromJson(v.desc, tc.input);
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
    });
  }

  for (const [name, v] of Object.entries(validators)) {
    it(`schema of ${name} matches the Go golden`, () => {
      const want = fromJson(SchemaSchema, golden(`schema.${name}.json`));
      expect(equals(SchemaSchema, v.schema, want)).toBe(true);
    });
  }
});
