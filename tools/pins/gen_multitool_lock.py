import json
import sys

G = "https://github.com"
# (tool, file-in-archive, [(os, cpu, url, sha256)])
T = [
    ("gitleaks", "gitleaks", [
        ("linux", "x86_64", f"{G}/gitleaks/gitleaks/releases/download/v8.30.1/gitleaks_8.30.1_linux_x64.tar.gz", "551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb"),
        ("linux", "arm64", f"{G}/gitleaks/gitleaks/releases/download/v8.30.1/gitleaks_8.30.1_linux_arm64.tar.gz", "e4a487ee7ccd7d3a7f7ec08657610aa3606637dab924210b3aee62570fb4b080"),
        ("macos", "arm64", f"{G}/gitleaks/gitleaks/releases/download/v8.30.1/gitleaks_8.30.1_darwin_arm64.tar.gz", "b40ab0ae55c505963e365f271a8d3846efbc170aa17f2607f13df610a9aeb6a5"),
    ]),
    ("typos", "typos", [
        ("linux", "x86_64", f"{G}/crate-ci/typos/releases/download/v1.51.1/typos-v1.51.1-x86_64-unknown-linux-musl.tar.gz", "93c301fd4120ff076e2cc2a6d6a61cab4a4bdb81899287c0938b452e56d1f454"),
        ("linux", "arm64", f"{G}/crate-ci/typos/releases/download/v1.51.1/typos-v1.51.1-aarch64-unknown-linux-musl.tar.gz", "2ac5e37b3236b3c07dec5cdf1527acf143aec4674a6089b0bf424d021b35e099"),
        ("macos", "arm64", f"{G}/crate-ci/typos/releases/download/v1.51.1/typos-v1.51.1-aarch64-apple-darwin.tar.gz", "316d50a32c3726a80ab39c6d12141b70080498973c5e8a9b068b12cde78b10c1"),
    ]),
    ("shellcheck", "shellcheck-v0.11.0/shellcheck", [
        ("linux", "x86_64", f"{G}/koalaman/shellcheck/releases/download/v0.11.0/shellcheck-v0.11.0.linux.x86_64.tar.gz", "b7af85e41cc99489dcc21d66c6d5f3685138f06d34651e6d34b42ec6d54fe6f6"),
        ("linux", "arm64", f"{G}/koalaman/shellcheck/releases/download/v0.11.0/shellcheck-v0.11.0.linux.aarch64.tar.gz", "68a8133197a50beb8803f8d42f9908d1af1c5540d4bb05fdfca8c1fa47decefc"),
        ("macos", "arm64", f"{G}/koalaman/shellcheck/releases/download/v0.11.0/shellcheck-v0.11.0.darwin.aarch64.tar.gz", "339b930feb1ea764467013cc1f72d09cd6b869ebf1013296ba9055ab2ffbd26f"),
    ]),
    ("actionlint", "actionlint", [
        ("linux", "x86_64", f"{G}/rhysd/actionlint/releases/download/v1.7.12/actionlint_1.7.12_linux_amd64.tar.gz", "8aca8db96f1b94770f1b0d72b6dddcb1ebb8123cb3712530b08cc387b349a3d8"),
        ("linux", "arm64", f"{G}/rhysd/actionlint/releases/download/v1.7.12/actionlint_1.7.12_linux_arm64.tar.gz", "325e971b6ba9bfa504672e29be93c24981eeb1c07576d730e9f7c8805afff0c6"),
        ("macos", "arm64", f"{G}/rhysd/actionlint/releases/download/v1.7.12/actionlint_1.7.12_darwin_arm64.tar.gz", "aba9ced2dee8d27fecca3dc7feb1a7f9a52caefa1eb46f3271ea66b6e0e6953f"),
    ]),
    ("zizmor", "zizmor", [
        ("linux", "x86_64", f"{G}/zizmorcore/zizmor/releases/download/v1.30.1/zizmor-x86_64-unknown-linux-gnu.tar.gz", "e65324f4430c2717591937edcec90ccbefaf14c174f8ec9415e03ca875b46e1a"),
        ("linux", "arm64", f"{G}/zizmorcore/zizmor/releases/download/v1.30.1/zizmor-aarch64-unknown-linux-gnu.tar.gz", "7ff1dce33bdd18fd2a4affe63bdd47efcccca97b2cec1c1863ec26e9e2647540"),
        ("macos", "arm64", f"{G}/zizmorcore/zizmor/releases/download/v1.30.1/zizmor-aarch64-apple-darwin.tar.gz", "e28d22b087f9ebb8d99da6e740d348c930f559961c7c3f12badda54f882195a2"),
    ]),
    ("yq", None, [
        ("linux", "x86_64", f"{G}/mikefarah/yq/releases/download/v4.54.1/yq_linux_amd64", "8e34fc298390875de416e6a4afcb8cabeceb25d9aa8506c1a2f9353cf702ea5f"),
        ("linux", "arm64", f"{G}/mikefarah/yq/releases/download/v4.54.1/yq_linux_arm64", "189088da0c6429ec5178dfaab1a114805f6cab0b61b165ab236efedf1d57a71b"),
        ("macos", "arm64", f"{G}/mikefarah/yq/releases/download/v4.54.1/yq_darwin_arm64", "fee511a181bd8b3e6b7da98842b41973bfac8cd3bcd341ed29a584620b5b2844"),
    ]),
    ("uv", "*/uv", [
        ("linux", "x86_64", f"{G}/astral-sh/uv/releases/download/0.13.1/uv-x86_64-unknown-linux-gnu.tar.gz", "eadd9d27c134ab5ac1ffd4473b97a741407f3ea238288f03121fc4deddf90220"),
        ("linux", "arm64", f"{G}/astral-sh/uv/releases/download/0.13.1/uv-aarch64-unknown-linux-gnu.tar.gz", "9041c73c99fdc5892cf128140beb354eca9231d502068cc36e6cb53a8e59ad14"),
        ("macos", "arm64", f"{G}/astral-sh/uv/releases/download/0.13.1/uv-aarch64-apple-darwin.tar.gz", "cfe454573412ff7380d15fc7dae5997ab53ffce72fce0a78e91ddbc84001acd2"),
    ]),
    ("buf", None, [
        ("linux", "x86_64", f"{G}/bufbuild/buf/releases/download/v1.74.0/buf-Linux-x86_64", "92326d957d613712669fb55f9e4340946a8142a92120e09008459118778273d2"),
        ("linux", "arm64", f"{G}/bufbuild/buf/releases/download/v1.74.0/buf-Linux-aarch64", "d42b471501a85dbadb40062960492dd81ce5ca4e413f4133d514d3ae3c0af4df"),
        ("macos", "arm64", f"{G}/bufbuild/buf/releases/download/v1.74.0/buf-Darwin-arm64", "602d73e39a8835efd29a02ba6db6060ae86e4a0df19b8ac4403032b5deae3569"),
    ]),
    ("prek", "*/prek", [
        ("linux", "x86_64", f"{G}/j178/prek/releases/download/v0.5.5/prek-x86_64-unknown-linux-musl.tar.gz", "57948d4772f4eb58387ce4a6d619ecd2d8d7e9d765eed6a316f5b3f92c86d951"),
        ("linux", "arm64", f"{G}/j178/prek/releases/download/v0.5.5/prek-aarch64-unknown-linux-musl.tar.gz", "e7208e5d42daaa8d9167971e7d2d3388d80cce241d1cecea745a6c88d6b6368e"),
        ("macos", "arm64", f"{G}/j178/prek/releases/download/v0.5.5/prek-aarch64-apple-darwin.tar.gz", "e4bc3132023b50020e62b073bc3147082828bf250d8b9282850c6f2bed5471b6"),
    ]),
    ("buildifier", None, [
        ("linux", "x86_64", f"{G}/bazelbuild/buildtools/releases/download/v10.1.0/buildifier-linux-amd64", "31b6a8aa1e5c746696788f428729701770ad91925873d8256cb885c60e12c77e"),
        ("linux", "arm64", f"{G}/bazelbuild/buildtools/releases/download/v10.1.0/buildifier-linux-arm64", "38d2ed845f560b4a16ddee41de906508a95f8dc85b04e0851b0a71e3a70d3890"),
        ("macos", "arm64", f"{G}/bazelbuild/buildtools/releases/download/v10.1.0/buildifier-darwin-arm64", "e9804864c407f920f5ecbf03a5e056a8145e11a6ae6b90d2438a3fd106d34473"),
    ]),
]
out = {"$schema": "https://raw.githubusercontent.com/theoremlp/rules_multitool/main/lockfile.schema.json"}
for name, inner, bins in T:
    out[name] = {"binaries": [
        ({"kind": "archive", "url": u, "sha256": s, "file": inner.replace("*", u.rsplit("/", 1)[1][: -len(".tar.gz")]), "os": o, "cpu": c}
         if inner else {"kind": "file", "url": u, "sha256": s, "os": o, "cpu": c})
        for o, c, u, s in bins
    ]}
json.dump(out, open(sys.argv[1], "w"), indent=2)
open(sys.argv[1], "a").write("\n")
