// The compile gate (specs/003-notes-contract FR-009). The real compiler checks a valid caller of
// the generated client, then one defect at a time. A defect earns credit only if the valid caller
// has no diagnostics, the source really changed, and the single diagnostic is the expected one on
// the changed line.
import { readFileSync } from "node:fs";
import path from "node:path";

import ts from "typescript";

const caller = path.resolve("api/testdata/negative/caller.ts");
const original = readFileSync(caller, "utf8");
const config = ts.parseJsonConfigFileContent(
  ts.readConfigFile(path.resolve("tsconfig.json"), ts.sys.readFile).config,
  ts.sys,
  process.cwd(),
);

function diagnostics(text) {
  const options = { ...config.options, noEmit: true, types: [] };
  const host = ts.createCompilerHost(options);
  const read = host.getSourceFile.bind(host);
  host.getSourceFile = (file, languageVersion) =>
    path.resolve(file) === caller
      ? ts.createSourceFile(file, text, languageVersion, true)
      : read(file, languageVersion);
  const program = ts.createProgram([caller], options, host);
  return ts.getPreEmitDiagnostics(program).map((d) => ({
    code: d.code,
    file: d.file?.fileName,
    line: d.file ? d.file.getLineAndCharacterOfPosition(d.start).line + 1 : 0,
    message: ts.flattenDiagnosticMessageText(d.messageText, " "),
  }));
}

const defects = [
  ["wrong field type", 'title: "A title"', "title: 1", 2322],
  ["unknown field", 'body: "text"', 'text: "text"', 2353],
  [
    "fields of another request",
    'getNote({ name: note?.name ?? "" })',
    "getNote({ note: {} })",
    2353,
  ],
  ["method not in the contract", "client.getNote", "client.deleteNote", 2339],
];

let failures = 0;
function fail(message) {
  console.error(`FAIL ${message}`);
  failures += 1;
}

const control = diagnostics(original);
if (control.length !== 0) fail(`control: ${JSON.stringify(control)}`);

for (const [name, from, to, code] of defects) {
  const text = original.replace(from, to);
  if (text === original) {
    fail(`${name}: the defect changed nothing`);
    continue;
  }
  const line = original.slice(0, original.indexOf(from)).split("\n").length;
  const found = diagnostics(text);
  const match = found.length === 1 && found[0].code === code && found[0].line === line;
  if (!match) fail(`${name}: want TS${code} on line ${line}, got ${JSON.stringify(found)}`);
}

if (failures > 0) process.exit(1);
console.log("compile gate passed");
