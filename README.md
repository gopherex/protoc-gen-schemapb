# protoc-gen-schemapb

Validators generated from `.proto` options, on the
[schemapb](https://github.com/gopherex/schemapb) runtime. Go and TypeScript
for now.

- **No vendor lock-in.** The options file and the plugins live here and are
  fetched straight from git (easyp deps, `go install`). No registry, no
  proxy, no protoc required. At run time a service needs the schemapb
  runtime (`github.com/gopherex/schemapb/go`, `@gopherex/schemapb`) and the
  generated code of the imported protos: in Go the options package of this
  module (protoc-gen-go output imports every imported file's package; it
  pulls in only `schemapb/go` and `google.golang.org/protobuf`), in
  TypeScript the npm packages `@gopherex/protoc-gen-schemapb` (the
  `options_pb` module) and `@gopherex/schemapb` (the schemapb protos and the
  runtime), both on GitHub Packages.
- **Defects fail generation, not production.** The plugin builds a
  `schemapb.Schema` per message, checks it and compiles every CEL
  expression. A kind that contradicts the proto type or a broken rule stops
  the `protoc`/`easyp` run with the field's name. A field named with a CEL
  reserved word (`in`, `namespace`, `const`, ...) is fine: expressions reach
  it by index, `this["in"]`, and test presence with `"in" in this`.
- **One semantics, two languages.** Both plugins share one frontend and embed
  the same schema; both runtimes reproduce the same goldens byte for byte
  (`conformance/`).
- **Errors are data.** A failure is a `schemapb.ValidationResult` proto:
  stable code, path with proto field numbers, typed expected/actual, schema
  identity. A client holding only the proto files can read it, and translate
  it (see [Errors](#errors)).

## Install

```yaml
# easyp.yaml of your project
deps:
  - github.com/gopherex/schemapb@vX.Y.Z            # schemapb/schema.proto, ...
  - github.com/gopherex/protoc-gen-schemapb@vX.Y.Z # schemapb/options/v1/options.proto
generate:
  plugins:
    - name: go
      out: .
      opts: {paths: source_relative}
    - name: go-schemapb          # go install github.com/gopherex/protoc-gen-schemapb/cmd/protoc-gen-go-schemapb@vX.Y.Z
      out: .
      opts: {paths: source_relative}
    - name: es                   # protoc-gen-es (protobuf-es v2)
      out: web/gen
      opts:
        target: ts
        import_extension: js
        rewrite_imports:         # import the imported protos from the packages
          - ./schemapb/options/**/*_pb.js:@gopherex/protoc-gen-schemapb
          - ./schemapb/*_pb.js:@gopherex/schemapb
    - name: ts-schemapb          # go install github.com/gopherex/protoc-gen-schemapb/cmd/protoc-gen-ts-schemapb@vX.Y.Z
      out: web/gen
      opts: {import_extension: js}
```

```sh
# .npmrc / .yarnrc.yml: @gopherex resolves from https://npm.pkg.github.com
npm install @gopherex/schemapb @gopherex/protoc-gen-schemapb @bufbuild/protobuf
```

Do not generate the imported protos yourself (`with_imports`): a second
copy of the schemapb descriptors is a second set of types, and messages of
one are not the messages of the other. The packages export each module at
its proto path (`@gopherex/schemapb/schemapb/errors_pb.js`,
`@gopherex/protoc-gen-schemapb/schemapb/options/v1/options_pb.js`), and
`@gopherex/protoc-gen-schemapb` takes `@gopherex/schemapb` as a peer, so one
instance serves the whole application. Package versions follow the git tags
of their repositories.

`protoc-gen-ts-schemapb` parameters, as `protoc-gen-es`: `target=ts|js|dts`
or a `+` combination (default `ts`), `import_extension=none|js|ts` (default
`none`); plus `runtime_module=<module>` (default `@gopherex/schemapb`).

## Write rules

```proto
import "schemapb/options/v1/options.proto";

option (schemapb.options.v1.file) = {version: "v1.0.0"};

message User {
  option (schemapb.options.v1.message) = {
    rules: [{id: "adult-gold", expr: "this.tier != 'TIER_GOLD' || this.age >= 18", message: "gold users must be adults"}]
  };

  // Display name.
  string name = 1 [(schemapb.options.v1.field).string = {min_len: 2, max_len: 32}];
  int32 age = 2 [(schemapb.options.v1.field).int32 = {gte: 0, lte: 150}];
  optional string email = 3 [(schemapb.options.v1.field) = {required: true, string: {format: "email"}}];
  Tier tier = 4;
  repeated string tags = 5 [(schemapb.options.v1.field).list = {max_items: 3, unique: true, items: {string: {min_len: 1}}}];
  map<string, int64> quotas = 6 [(schemapb.options.v1.field).map = {value_field: {int64: {gt: 0}}}];
  string password = 7 [(schemapb.options.v1.field) = {secret: true, string: {min_len: 8}}];
  string coupon = 8 [(schemapb.options.v1.field) = {when: "root.tier == 'TIER_GOLD'", string: {len: 6}}];
  map<string, int32> scores = 11 [(schemapb.options.v1.field).map = {
    key_field: {string: {pattern: "^[a-z]+$"}}   // keys, in the key's kind
    value_field: {int32: {gte: 0}}
  }];
  google.protobuf.Any payload = 12 [(schemapb.options.v1.any) = {types: ["acme.v1.Card"]}];
  int32 tag_count = 13 [(schemapb.options.v1.field).computed = {expr: "size(root.tags)"}];
  repeated string tags = 14;
  oneof contact {
    option (schemapb.options.v1.oneof).required = true;
    string phone = 9;
    string telegram = 10;
  }
}

enum Tier {
  TIER_UNSPECIFIED = 0 [(schemapb.options.v1.enum_value).exclude = true]; // => tier is required
  TIER_FREE = 1 [(schemapb.options.v1.enum_value).label = "Free"];
  TIER_GOLD = 2;
}
```

The field option **is** a `schemapb.Schema.Field`, so everything the runtime
supports is available: constraints of every kind, `rules`, `when`, `secret`,
`immutable`, `normalize`, severity, titles, units, annotations. The plugin
derives the name and the kind from the proto field and merges the option in.
Full contract: [`schemapb/options/v1/options.proto`](schemapb/options/v1/options.proto).

### How a message is validated

| proto | schemapb kind | value seen by the engine |
|---|---|---|
| `int32` `sint32` `sfixed32` | int32 | integer |
| `int64` `sint64` `sfixed64` | int64 | integer |
| `uint32` `fixed32` / `uint64` `fixed64` | uint32 / uint64 | unsigned integer |
| `float` / `double` | float / double | double |
| `bool` `string` `bytes` | same | same |
| enum | choice of value names (`exclude` drops one) | value name; unknown number stays an integer |
| message | ref into the root `defs` (recursion works) | object of its fields |
| `repeated T` / `map<K, V>` | list / map | list / object keyed by the key as string |
| `Timestamp` / `Duration` | timestamp / duration | same |
| wrappers | the wrapped kind | the scalar; absent when unset |
| `Struct` `Value` `ListValue` | json | JSON value |
| `Empty` / `FieldMask` | object / list of string | `{}` / paths |
| `Any` with `(any).types` | union on `@type` over those types | `@type` + the packed message's fields |
| `Any` without types | json | `@type` only |

`Resolve` writes the resolved values (defaults, normalize, `computed`) back
into the message only when nothing blocks, all or nothing; an `Any` of a
listed type is unpacked, written and packed again, so what was validated is
what is stored. A computed field's result type defaults to its proto type.

Presence follows proto: implicit-presence fields are always present (their
zero value is checked against the constraints); `optional`, message and
oneof members are present only when set, so `required` is accepted only on
them. Expressions are CEL; in message rules `this` is the message, `root` is
the message being validated.

## Use

Go (`<name>_schemapb.pb.go`, next to `protoc-gen-go` output):

```go
if err := user.Validate(); err != nil {          // nil when nothing blocks
	var res *schemapb.ValidationResult            // the error IS the proto
	errors.As(err, &res)
	st, _ := protomsg.Status(err)                  // google.rpc.Status, INVALID_ARGUMENT, result in details
	return status.ErrorProto(st)                   // gRPC
}
res, err := user.ValidateResult()                  // every failure, warnings included
res, err = user.Resolve()                          // validate, then write defaults/normalize/computed back
schema := user.SchemaPB()                          // e.g. ship it to a form renderer
```

TypeScript (`<name>_schemapb.ts`, next to `protoc-gen-es` output):

```ts
import { UserValidator } from "./gen/user_schemapb.js";

const res = UserValidator.check(user);   // ValidationResult, warnings included
UserValidator.validate(user);            // throws ValidationFailure when something blocks
UserValidator.resolve(user);             // validate, then write defaults/normalize/computed back
UserValidator.schema;                    // the embedded schemapb.Schema
```

On a Connect client the server's result arrives in the error details:
`ConnectError.from(e).findDetails(ValidationResultSchema)` (from
`@gopherex/schemapb`).

## Errors

Every `ValidationError` carries:

- `code` — stable `ErrorCode`. Its `error_spec` option in `schemapb/errors.proto`
  documents it: summary, message template, constraint names, and what
  `expected`/`actual` hold;
- `path` and `path_segments` — proto field names; each segment that names a
  proto field also carries its **field number**, stable under renames;
- `expected` / `actual` — typed values (`actual` is absent on secret fields);
- `constraint`, `rule_id`, `expr`, `severity`, and an English `message`.

`ValidationResult.schema` is the identity of the schema (proto package,
message name, file version).

**Translation.** Render messages on the client from a per-locale catalog:
keys are the `ErrorCode` name (`ERROR_CODE_GTE_VIOLATED`) or, for a violated
rule, its `id`; templates use `{expected}` and `{actual}`. The English
catalog is `conformance/golden/messages.json` in schemapb.

```ts
import { displayValue, formatMessage } from "@gopherex/schemapb";

const ru = {
  ERROR_CODE_GTE_VIOLATED: "должно быть не меньше {expected}",
  "adult-gold": "золотой тариф только для совершеннолетних",
};
formatMessage(err, ru, (v) => new Intl.NumberFormat("ru").format(Number(displayValue(v))));
```

Go has the same: `schemapb.Catalog(ru).Message(err, display)`. Give every rule
an `id` to make it translatable; generated oneof rules use `oneof:<name>`.

## Development

```sh
make configure   # pinned tools into ./bin, TS harness deps
make gen         # descriptors (easyp) -> options Go code + conformance code
make golden      # rewrite goldens from the Go backend (the reference)
make test        # Go tests + TS conformance against the built runtime
make lint
```

schemapb is consumed as released: the Go module in `go.mod`, the contract
protos as an easyp dep, and `@gopherex/schemapb` from GitHub Packages for the
TypeScript harness (`GITHUB_TOKEN` with `read:packages` in the environment).

Layout:

```
schemapb/options/v1/   options.proto + Go code
internal/frontend/     descriptor + options -> schemapb.Schema (shared IR)
internal/gogen/        Go backend
internal/tsgen/        TypeScript backend (protobuf-es naming)
cmd/                   protoc-gen-go-schemapb, protoc-gen-ts-schemapb
internal/tools/protocgen/  runs a plugin over a FileDescriptorSet
conformance/           cases.json, goldens, Go and TS harnesses
```
