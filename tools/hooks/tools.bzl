"""The pinned tools the runner receives. The registry names them; this says where they come from."""

load("@bff_template_quality//:defs.bzl", "MODULE_TOOLS")

QUALITY_TOOLS = MODULE_TOOLS | {
    "BASELINE": "@bff_template_quality//proto/cmd:baseline",
    "BUF": "@bff_template_quality//tools:buf",
    "BUF_CONFIG_FILE": "@bff_template_quality//proto:buf.yaml",
    "GOFMT": "@go_sdk//:bin/gofmt",
    "GOVULNCHECK": "@bff_template_quality//tools:govulncheck",
    "PREK": "@bff_template_quality//tools:prek",
    "PRETTIER": "//:prettier",
    "RELEASE": "@bff_template_quality//release:release_bin",
    "UV": "@bff_template_quality//tools:uv",
    "VALIDATE_PROTO_FILE": "@protovalidate_proto//file",
    "YQ": "@bff_template_quality//tools:yq",
}
