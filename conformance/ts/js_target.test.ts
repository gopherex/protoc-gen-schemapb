// The js+dts outputs, consumed as a JavaScript project would: the .d.ts
// files typecheck this test, the .js files run it.
import { fromJson } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { EnvelopeValidator } from "./genjs/conformance/v1/envelope_schemapb.js";

describe("js+dts targets", () => {
  it("validates and resolves like the ts target", () => {
    const msg = fromJson(EnvelopeValidator.desc, { slug: "Hello-World", tags: ["a"] });
    const res = EnvelopeValidator.resolve(msg);
    expect(res.errors).toEqual([]);
    expect(msg.slug).toBe("hello-world");
    expect(msg.tagCount).toBe(1);
    expect(msg.region).toBe("eu");
  });
});
