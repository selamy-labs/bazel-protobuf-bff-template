"""The pinned tools the runner receives. The registry names them; this says where they come from."""

QUALITY_TOOLS = {
    "ACTIONLINT": "@bff_template_quality//tools:actionlint",
    "BASELINE": "@bff_template_quality//proto/cmd:baseline",
    "BUF": "@bff_template_quality//tools:buf",
    "BUF_CONFIG_FILE": "@bff_template_quality//proto:buf.yaml",
    "BUILDIFIER": "@bff_template_quality//tools:buildifier",
    "FLOORS": "@bff_template_quality//checks/coverage:floors",
    "GITLEAKS": "@bff_template_quality//tools:gitleaks",
    "GOFMT": "@go_sdk//:bin/gofmt",
    "PREK": "@bff_template_quality//tools:prek",
    "PRETTIER": "//:prettier",
    "SHELLCHECK": "@bff_template_quality//tools:shellcheck",
    "SUPPRESSION": "@bff_template_quality//checks/suppression:suppression-ban",
    "TYPOS": "@bff_template_quality//tools:typos",
    "UV": "@bff_template_quality//tools:uv",
    "VALIDATE_PROTO_FILE": "@protovalidate_proto//file",
    "YQ": "@bff_template_quality//tools:yq",
    "ZIZMOR": "@bff_template_quality//tools:zizmor",
}
