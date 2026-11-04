package inputs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckServiceConfigOnValidServiceFixture(t *testing.T) {
	dir := t.TempDir()
	versionsContent := `
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	mainContent := `
locals {
  inputs = jsondecode(file(var.inputs_file))
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(versionsContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(mainContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err != nil {
		t.Errorf("expected valid fixture to pass CheckServiceConfig, got: %v", err)
	}
}

func TestCheckServiceConfigRejectsForbiddenFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "extra.tofu"), []byte("resource \"b\" {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err == nil {
		t.Error("expected error for .tofu file, got nil")
	}
}

func TestCheckServiceConfigRejectsUnauthorizedProvider(t *testing.T) {
	dir := t.TempDir()
	content := `
provider "aws" {
  region = "us-east-1"
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err == nil {
		t.Error("expected error for unauthorized provider, got nil")
	}
}

func TestCheckServiceConfigRejectsImportBlock(t *testing.T) {
	dir := t.TempDir()
	content := `
import {
  to = google_cloud_run_v2_service.bff
  id = "existing"
}
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err == nil {
		t.Error("expected error for import block, got nil")
	}
}

func TestCheckServiceConfigRejectsModuleBlock(t *testing.T) {
	dir := t.TempDir()
	content := `
module "extra" {
  source = "./extra"
}
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err == nil {
		t.Error("expected error for module block, got nil")
	}
}

func TestCheckServiceConfigRejectsDataBlock(t *testing.T) {
	dir := t.TempDir()
	content := `
data "google_project" "p" {
}
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err == nil {
		t.Error("expected error for data block, got nil")
	}
}

func TestCheckServiceConfigIgnoresHarmlessProviderInHeredoc(t *testing.T) {
	dir := t.TempDir()
	content := `
locals {
  note = <<-EOT
  provider "aws" {
    region = "us-east-1"
  }
  EOT
}

provider "google" {
  project = local.inputs.contract.project_id
  region  = local.inputs.contract.region
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err != nil {
		t.Errorf("expected harmless provider heredoc to pass, got: %v", err)
	}
}

func TestCheckServiceConfigRejectsNestedProviderMask(t *testing.T) {
	dir := t.TempDir()
	content := `
provider "google" {
  default_labels = {
    project = jsondecode(file(var.inputs_file)).contract.project_id
    region  = jsondecode(file(var.inputs_file)).contract.region
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket
  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err == nil {
		t.Error("expected error for nested provider mask with wrong top-level project/region, got nil")
	}
}

func TestCheckServiceConfigAcceptsEquivalentLocalReference(t *testing.T) {
	dir := t.TempDir()
	content := `
provider "google" {
  project = local.inputs.contract.project_id
  region  = local.inputs.contract.region
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err != nil {
		t.Errorf("expected equivalent local provider reference to pass, got: %v", err)
	}
}

func TestCheckServiceConfigRejectsStringBraceNestedMask(t *testing.T) {
	dir := t.TempDir()
	content := `
provider "google" {
  default_labels = {
    mask = "}"
    project = jsondecode(file(var.inputs_file)).contract.project_id
    region  = jsondecode(file(var.inputs_file)).contract.region
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket
  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err == nil {
		t.Error("expected error for string-brace nested mask with wrong top-level binding, got nil")
	}
}

func TestCheckServiceConfigRejectsInterpolationMaskWithWrongBinding(t *testing.T) {
	dir := t.TempDir()
	content := `
provider "google" {
  default_labels = {
    mask    = "${"}"}"
    project = var.project_id
    region  = var.region
  }
  project = local.contract.notes_bucket
  region  = local.contract.notes_bucket
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err == nil {
		t.Error("expected error for interpolation-mask wrong binding, got nil")
	}
}

func TestCheckServiceConfigAcceptsInterpolationHarmless(t *testing.T) {
	dir := t.TempDir()
	content := `
provider "google" {
  default_labels = {
    env = "${"dev"}"
  }
  project = var.project_id
  region  = var.region
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err != nil {
		t.Errorf("expected harmless interpolation to pass, got: %v", err)
	}
}

func TestCheckServiceConfigAcceptsInterpolationWithInnerBrace(t *testing.T) {
	dir := t.TempDir()
	content := `
provider "google" {
  default_labels = {
    literal = "${"}"}"
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err != nil {
		t.Errorf("expected harmless inner brace interpolation to pass, got: %v", err)
	}
}

func TestCheckServiceConfigAcceptsFormatInterpolation(t *testing.T) {
	dir := t.TempDir()
	content := `
provider "google" {
  default_labels = {
    literal = "${format("%s", "}")}"
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err != nil {
		t.Errorf("expected format interpolation to pass, got: %v", err)
	}
}

func TestCheckServiceConfigAcceptsHarmlessBraceInString(t *testing.T) {
	dir := t.TempDir()
	content := `
provider "google" {
  default_labels = {
    note = "{"
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err != nil {
		t.Errorf("expected harmless brace in string to pass, got: %v", err)
	}
}

func TestCheckServiceConfigRejectsNestedHeredocInterpolationWrongBinding(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
	}{
		{
			name: "trimspace with non-indented heredoc",
			content: `
provider "google" {
  default_labels = {
    mask = "${trimspace(<<EOT
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
EOT
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket
  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket
}
`,
		},
		{
			name: "chomp with indented heredoc",
			content: `
provider "google" {
  default_labels = {
    mask = "${chomp(<<-EOT
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
EOT
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket
  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket
}
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := CheckServiceConfig(dir); err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestCheckServiceConfigRejectsNestedHeredocInterpolationUnicodeWrongBinding(t *testing.T) {
	dir := t.TempDir()
	content := `
provider "google" {
  default_labels = {
    mask = "${trimspace(<<ÉOT
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
ÉOT
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket
  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err == nil {
		t.Error("expected error for unicode heredoc wrong binding, got nil")
	}
}

func TestCheckServiceConfigAcceptsNestedHeredocInterpolationHarmless(t *testing.T) {
	dir := t.TempDir()
	content := `
provider "google" {
  default_labels = {
    note = "${chomp(<<-EOT
  }"
EOT
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err != nil {
		t.Errorf("expected harmless nested heredoc in interpolation to pass, got: %v", err)
	}
}

func runValidHeredocCases(t *testing.T, cases []struct{ name, content string }) {
	t.Helper()
	for _, tc := range cases {
		t.Run("valid_"+tc.name, func(t *testing.T) {
			dir := t.TempDir()
			content := fmt.Sprintf(`
provider "google" {
  default_labels = {
    %s
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`, tc.content)
			if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := CheckServiceConfig(dir); err != nil {
				t.Fatalf("expected valid heredoc %s to pass CheckServiceConfig, got: %v", tc.name, err)
			}
		})
	}
}

func runInvalidHeredocCases(t *testing.T, cases []struct{ name, content string }) {
	t.Helper()
	for _, tc := range cases {
		t.Run("invalid_"+tc.name, func(t *testing.T) {
			dir := t.TempDir()
			content := fmt.Sprintf(`
provider "google" {
  default_labels = {
    %s
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`, tc.content)
			if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := CheckServiceConfig(dir); err == nil {
				t.Fatalf("expected invalid heredoc %s to fail CheckServiceConfig, got nil", tc.name)
			}
		})
	}
}

func TestCheckServiceConfigHeredocOpeners(t *testing.T) {
	validCases := []struct {
		name    string
		content string
	}{
		{"EOF", "mask = <<EOF\nhello\nEOF\n"},
		{"indented EOF", "mask = <<-EOF\nhello\nEOF\n"},
		{"indented CRLF EOF", "mask = <<-EOF\r\nhello\r\nEOF\r\n"},
		{"_MY_DOC", "mask = <<_MY_DOC\nhello\n_MY_DOC\n"},
		{"EOF-REAL", "mask = <<EOF-REAL\nhello\nEOF-REAL\n"},
		{"indented EOF-REAL", "mask = <<-EOF-REAL\nhello\nEOF-REAL\n"},
		{"combining mark EOT", "mask = <<E\u0301OT\nhello\nE\u0301OT\n"},
		{"Other_ID_Start \\u2118", "mask = <<\u2118\nhello\n\u2118\n"},
		{"Other_ID_Start \\u2118OT", "mask = <<\u2118OT\nhello\n\u2118OT\n"},
		{"Other_ID_Start A\\u2118OT", "mask = <<A\u2118OT\nhello\nA\u2118OT\n"},
		{"Other_ID_Start \\u2118AOT", "mask = <<\u2118AOT\nhello\n\u2118AOT\n"},
		{"Other_ID_Start AOT\\u2118", "mask = <<AOT\u2118\nhello\nAOT\u2118\n"},
		{"Other_ID_Start A\\u212EOT", "mask = <<A\u212EOT\nhello\nA\u212EOT\n"},
		{"Other_ID_Start \\u212EAOT", "mask = <<\u212EAOT\nhello\n\u212EAOT\n"},
		{"Other_ID_Start AOT\\u212E", "mask = <<AOT\u212E\nhello\nAOT\u212E\n"},
		{"Other_ID_Start A\\u309BOT", "mask = <<A\u309BOT\nhello\nA\u309BOT\n"},
		{"Other_ID_Start \\u309BAOT", "mask = <<\u309BAOT\nhello\n\u309BAOT\n"},
		{"Other_ID_Start AOT\\u309B", "mask = <<AOT\u309B\nhello\nAOT\u309B\n"},
	}
	runValidHeredocCases(t, validCases)

	invalidCases := []struct {
		name    string
		content string
	}{
		{"extra tokens after marker", "mask = <<EOF extra\nhello\nEOF\n"},
		{"number start", "mask = <<123\nhello\n123\n"},
		{"indented number start", "mask = <<-123\nhello\n123\n"},
		{"double hyphen", "mask = <<--EOF\nhello\nEOF\n"},
		{"empty opener", "mask = <<\nhello\n\n"},
		{"indented empty opener", "mask = <<-\nhello\n\n"},
		{"space before marker", "mask = << 5\nhello\n5\n"},
	}
	runInvalidHeredocCases(t, invalidCases)
}

func TestCheckServiceConfigHyphenatedHeredocDelimiter(t *testing.T) {
	wrongBinding := `
provider "google" {
  default_labels = {
    mask = "${trimspace(<<EOF-REAL
EOF
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
EOF-REAL
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket
  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket
}
`
	rightBinding := `
provider "google" {
  default_labels = {
    mask = "${trimspace(<<EOF-REAL
EOF
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
EOF-REAL
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	dirWrong := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirWrong, "versions.tf"), []byte(wrongBinding), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dirWrong); err == nil {
		t.Error("expected hyphenated heredoc delimiter wrong binding to be rejected, got nil")
	}

	dirRight := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirRight, "versions.tf"), []byte(rightBinding), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dirRight); err != nil {
		t.Errorf("expected hyphenated heredoc delimiter right binding to be accepted, got: %v", err)
	}
}

func TestCheckServiceConfigCombiningMarkHeredocDelimiter(t *testing.T) {
	wrongBinding := `
provider "google" {
  default_labels = {
    mask = "${trimspace(<<` + "E\u0301OT" + `
E
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
` + "E\u0301OT" + `
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket
  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket
}
`
	rightBinding := `
provider "google" {
  default_labels = {
    mask = "${trimspace(<<` + "E\u0301OT" + `
E
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
` + "E\u0301OT" + `
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	dirWrong := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirWrong, "versions.tf"), []byte(wrongBinding), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dirWrong); err == nil {
		t.Error("expected combining mark heredoc delimiter wrong binding to be rejected, got nil")
	}

	dirRight := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirRight, "versions.tf"), []byte(rightBinding), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dirRight); err != nil {
		t.Errorf("expected combining mark heredoc delimiter right binding to be accepted, got: %v", err)
	}
}

func TestCheckServiceConfigOtherIDStartHeredocDelimiter(t *testing.T) {
	wrongBinding := `
provider "google" {
  default_labels = {
    mask = "${trimspace(<<` + "\u2118" + `
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
` + "\u2118" + `
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket
  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket
}
`
	rightBinding := `
provider "google" {
  default_labels = {
    mask = "${trimspace(<<` + "\u2118" + `
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
` + "\u2118" + `
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	dirWrong := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirWrong, "versions.tf"), []byte(wrongBinding), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dirWrong); err == nil {
		t.Error("expected other ID start heredoc delimiter wrong binding to be rejected, got nil")
	}

	dirRight := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirRight, "versions.tf"), []byte(rightBinding), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dirRight); err != nil {
		t.Errorf("expected other ID start heredoc delimiter right binding to be accepted, got: %v", err)
	}
}

func TestCheckServiceConfigNestedHeredocTemplate(t *testing.T) {
	wrongBinding := `
provider "google" {
  default_labels = {
    mask = "${trimspace(<<OUTER
${trimspace(<<INNER
OUTER
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
INNER
    )}
OUTER
)}"
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket
  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket
}
`
	rightBinding := `
provider "google" {
  default_labels = {
    mask = "${trimspace(<<OUTER
${trimspace(<<INNER
OUTER
}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
INNER
    )}
OUTER
)}"
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	dirWrong := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirWrong, "versions.tf"), []byte(wrongBinding), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dirWrong); err == nil {
		t.Error("expected nested heredoc template wrong binding to be rejected, got nil")
	}

	dirRight := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirRight, "versions.tf"), []byte(rightBinding), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dirRight); err != nil {
		t.Errorf("expected nested heredoc template right binding to be accepted, got: %v", err)
	}
}

func testOtherIDStartForged(t *testing.T, delim string) {
	t.Helper()
	t.Run("forged_"+delim, func(t *testing.T) {
		wrongBinding := fmt.Sprintf(`
provider "google" {
  default_labels = {
    mask = "${trimspace(<<%s
}
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
%s
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket
  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket
}
`, delim, delim)
		dirWrong := t.TempDir()
		if err := os.WriteFile(filepath.Join(dirWrong, "versions.tf"), []byte(wrongBinding), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := CheckServiceConfig(dirWrong); err == nil {
			t.Errorf("expected Other_ID_Start delimiter %q wrong binding to be rejected, got nil", delim)
		}
	})
}

func testOtherIDStartValid(t *testing.T, delim string) {
	t.Helper()
	t.Run("valid_"+delim, func(t *testing.T) {
		rightBinding := fmt.Sprintf(`
provider "google" {
  default_labels = {
    mask = "${trimspace(<<%s
}
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy="
%s
    )}"
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`, delim, delim)
		dirRight := t.TempDir()
		if err := os.WriteFile(filepath.Join(dirRight, "versions.tf"), []byte(rightBinding), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := CheckServiceConfig(dirRight); err != nil {
			t.Errorf("expected Other_ID_Start delimiter %q right binding to be accepted, got: %v", delim, err)
		}
	})
}

func TestCheckServiceConfigOtherIDStartPositions(t *testing.T) {
	delims := []string{
		"A\u2118OT",
		"\u2118AOT",
		"AOT\u2118",
		"A\u212EOT",
		"\u212EAOT",
		"AOT\u212E",
		"A\u309BOT",
		"\u309BAOT",
		"AOT\u309B",
		"A\u309COT",
		"\u309CAOT",
		"AOT\u309C",
	}

	for _, delim := range delims {
		testOtherIDStartForged(t, delim)
		testOtherIDStartValid(t, delim)
	}
}

func testIndentedHeredocForged(t *testing.T, name, opener, closerLine string) {
	t.Helper()
	t.Run("forged_"+name, func(t *testing.T) {
		wrongBinding := fmt.Sprintf(`
provider "google" {
  default_labels = {
    mask = %s
literal
%s
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket
  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket
}
locals {
  trap = <<FAKE
EOF
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
}
FAKE
}
`, opener, closerLine)
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(wrongBinding), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := CheckServiceConfig(dir); err == nil {
			t.Errorf("expected indented closing marker %s with forged binding to be rejected, got nil", name)
		}
	})
}

func testIndentedHeredocValid(t *testing.T, name, opener, closerLine string) {
	t.Helper()
	t.Run("valid_"+name, func(t *testing.T) {
		rightBinding := fmt.Sprintf(`
provider "google" {
  default_labels = {
    mask = %s
literal
%s
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`, opener, closerLine)
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(rightBinding), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := CheckServiceConfig(dir); err != nil {
			t.Errorf("expected indented closing marker %s with right binding to be accepted, got: %v", name, err)
		}
	})
}

func TestCheckServiceConfigIndentedHeredocClosingMarker(t *testing.T) {
	testCases := []struct {
		name       string
		opener     string
		closerLine string
	}{
		{"plain_space_indented", "<<EOF", "  EOF"},
		{"plain_tab_indented", "<<EOF", "\tEOF"},
		{"indented_space_indented", "<<-EOF", "  EOF"},
		{"indented_tab_indented", "<<-EOF", "\tEOF"},
	}

	for _, tc := range testCases {
		testIndentedHeredocForged(t, tc.name, tc.opener, tc.closerLine)
		testIndentedHeredocValid(t, tc.name, tc.opener, tc.closerLine)
	}
}

type unicodeWhitespaceCase struct {
	caseName   string
	opener     string
	closerLine string
}

func buildUnicodeWhitespaceCases() []unicodeWhitespaceCase {
	unicodeRunes := []struct {
		name string
		r    rune
	}{
		{"U+000B", '\u000B'},
		{"U+000C", '\u000C'},
		{"U+0085", '\u0085'},
		{"U+00A0", '\u00A0'},
		{"U+1680", '\u1680'},
		{"U+2003", '\u2003'},
		{"U+2028", '\u2028'},
		{"U+202F", '\u202F'},
		{"U+3000", '\u3000'},
	}
	forms := []struct {
		formName string
		opener   string
	}{
		{"plain", "<<EOF"},
		{"indented", "<<-EOF"},
	}
	positions := []struct {
		posName string
		closer  func(r rune) string
	}{
		{"prefix", func(r rune) string { return string(r) + "EOF" }},
		{"suffix", func(r rune) string { return "EOF" + string(r) }},
		{"both", func(r rune) string { return string(r) + "EOF" + string(r) }},
	}
	var cases []unicodeWhitespaceCase
	for _, ur := range unicodeRunes {
		for _, form := range forms {
			for _, pos := range positions {
				cases = append(cases, unicodeWhitespaceCase{
					caseName:   fmt.Sprintf("%s_%s_%s", ur.name, form.formName, pos.posName),
					opener:     form.opener,
					closerLine: pos.closer(ur.r),
				})
			}
		}
	}
	return cases
}

func testUnicodeWhitespaceCloser(t *testing.T, tc unicodeWhitespaceCase) {
	t.Helper()
	testIndentedHeredocForged(t, tc.caseName, tc.opener, tc.closerLine)
	testIndentedHeredocValid(t, tc.caseName, tc.opener, tc.closerLine)
}

func TestCheckServiceConfigUnicodeWhitespaceHeredocCloser(t *testing.T) {
	for _, tc := range buildUnicodeWhitespaceCases() {
		testUnicodeWhitespaceCloser(t, tc)
	}
}

func quotedPrefixTF(char string, count int, valid bool) string {
	prefix := strings.Repeat(char, count)
	if count == 1 {
		expr := "${upper(\"literal\")}"
		if char != "$" {
			if valid {
				expr = "%{if true}literal%{endif}"
			} else {
				expr = "%{if true}literal"
			}
		}
		if valid {
			return fmt.Sprintf(`
provider "google" {
  default_labels = {
    mask = "%s"
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`, expr)
		}
		return fmt.Sprintf(`
provider "google" {
  default_labels = {
    mask = "%s
}
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
dummy=\""
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket
  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket
}
`, expr)
	}

	if valid {
		return fmt.Sprintf(`
provider "google" {
  default_labels = {
    mask = "%s{"
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`, prefix)
	}
	return fmt.Sprintf(`
provider "google" {
  default_labels = {
    mask = "%s{"
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket
  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket
}
locals {
  trap = <<FAKE
"}"
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
}
FAKE
}
`, prefix)
}

func heredocPrefixTF(char string, count int, valid bool) string {
	prefix := strings.Repeat(char, count)
	if count == 1 {
		expr := "${upper(\"literal\")}"
		if char != "$" {
			expr = "%{if true}literal%{endif}"
		}
		if valid {
			return fmt.Sprintf(`
provider "google" {
  default_labels = {
    mask = <<EOF
%s
EOF
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`, expr)
		}
		return fmt.Sprintf(`
provider "google" {
  default_labels = {
    mask = <<EOF
%s
EOF
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket
  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket
}
`, expr)
	}

	if valid {
		return fmt.Sprintf(`
provider "google" {
  default_labels = {
    mask = <<EOF
%s{
EOF
  }
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`, prefix)
	}
	return fmt.Sprintf(`
provider "google" {
  default_labels = {
    mask = <<EOF
%s{
EOF
  }
  project = jsondecode(file(var.inputs_file)).contract.notes_bucket
  region  = jsondecode(file(var.inputs_file)).contract.notes_bucket
}
locals {
  trap = <<FAKE
EOF
}
project = jsondecode(file(var.inputs_file)).contract.project_id
region = jsondecode(file(var.inputs_file)).contract.region
}
FAKE
}
`, prefix)
}

func testPrefixCase(t *testing.T, testName, tfContent string, wantErr bool) {
	t.Helper()
	t.Run(testName, func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(tfContent), 0o600); err != nil {
			t.Fatal(err)
		}
		err := CheckServiceConfig(dir)
		if wantErr && err == nil {
			t.Errorf("expected %s to be rejected, got nil", testName)
		} else if !wantErr && err != nil {
			t.Errorf("expected %s to be accepted, got: %v", testName, err)
		}
	})
}

func TestCheckServiceConfigPrefixEscapeSemantics(t *testing.T) {
	for _, char := range []string{"$", "%"} {
		for count := 1; count <= 5; count++ {
			testPrefixCase(t, fmt.Sprintf("quoted_forged_%s_%d", char, count), quotedPrefixTF(char, count, false), true)
			testPrefixCase(t, fmt.Sprintf("quoted_valid_%s_%d", char, count), quotedPrefixTF(char, count, true), false)
			testPrefixCase(t, fmt.Sprintf("heredoc_forged_%s_%d", char, count), heredocPrefixTF(char, count, false), true)
			testPrefixCase(t, fmt.Sprintf("heredoc_valid_%s_%d", char, count), heredocPrefixTF(char, count, true), false)
		}
	}
}

func TestValidateApprovedPlanDigest(t *testing.T) {
	valid := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := ValidateApprovedPlanDigest(valid); err != nil {
		t.Fatalf("expected valid 64-hex to pass, got: %v", err)
	}

	invalidCases := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"leading space", " " + valid},
		{"trailing space", valid + " "},
		{"uppercase", "0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef"},
		{"newline injection", valid + "  /dev/null\n#"},
		{"short", "0123456789abcdef"},
		{"long", valid + "00"},
		{"non-hex", "g123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateApprovedPlanDigest(tc.value); err == nil {
				t.Errorf("expected refusal for %s (%q), got nil", tc.name, tc.value)
			}
		})
	}
}

func TestVerifyServingOutputKeys(t *testing.T) {
	validMap := map[string]any{
		"allowed_origin": "https://example.invalid",
		"image":          "example-image",
		"max_instances":  float64(2),
		"port":           float64(8080),
		"revision":       "0000000000000000000000000000000000000000",
	}
	if err := verifyServingOutputKeys(validMap); err != nil {
		t.Fatalf("expected valid serving keys to pass, got: %v", err)
	}

	extraMap := map[string]any{
		"allowed_origin": "https://example.invalid",
		"image":          "example-image",
		"max_instances":  float64(2),
		"port":           float64(8080),
		"revision":       "0000000000000000000000000000000000000000",
		"owner_budget":   float64(50),
	}
	if err := verifyServingOutputKeys(extraMap); err == nil {
		t.Error("expected error for smuggled owner_budget key in serving output, got nil")
	}

	missingKeyMap := map[string]any{
		"allowed_origin": "https://example.invalid",
		"image":          "example-image",
		"max_instances":  float64(2),
		"port":           float64(8080),
	}
	if err := verifyServingOutputKeys(missingKeyMap); err == nil {
		t.Error("expected error for missing required key in serving output, got nil")
	}
}

func TestCheckServiceConfigAcceptsCommentsWithUnmatchedQuotes(t *testing.T) {
	dir := t.TempDir()
	versionsContent := `
# An operator wrote " here as documentation.
// Documentation with another " unmatched quote.
/* Block comment with " quote and { braces } inside */
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	mainContent := `
locals {
  inputs = jsondecode(file(var.inputs_file))
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(versionsContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(mainContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err != nil {
		t.Fatalf("expected comments with unmatched quotes to pass CheckServiceConfig, got: %v", err)
	}
}

func TestValidateApplyStepScript(t *testing.T) {
	validScript := `
set -o pipefail
[[ ! -e /workspace/cancelled ]] || exit 0
printf '%s  /workspace/plan.tfplan\n' "${_APPROVED_PLAN_SHA256}" | sha256sum --strict -c -
if [[ -f /workspace/plan.tfplan.sha256 ]]; then
  sha256sum --strict -c /workspace/plan.tfplan.sha256
  grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256 || exit 1
fi
tofu init -input=false -lockfile=readonly
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`
	if err := ValidateApplyStepScript(validScript); err != nil {
		t.Fatalf("expected valid apply script to pass, got: %v", err)
	}

	testCases := []struct {
		name   string
		script string
	}{
		{
			name: "planted tofu plan in apply step",
			script: `
tofu plan -input=false -out=/workspace/plan.tfplan
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "planted tofu plan with global chdir flag",
			script: `
tofu -chdir=. plan -input=false -out=/workspace/./plan.tfplan
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "planted tofu plan with separate chdir flag",
			script: `
tofu -chdir . plan -input=false -out=/workspace/plan.tfplan
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "planted tofu plan with no-color flag",
			script: `
tofu -no-color plan -input=false -out=/workspace/plan.tfplan
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "tofu apply with global chdir flag",
			script: `
tofu -chdir=. apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "tofu apply with unnormalized path",
			script: `
tofu apply -input=false -auto-approve /workspace/./plan.tfplan
`,
		},
		{
			name: "plan overwrite via unnormalized redirection",
			script: `
echo "fake-plan" > /workspace/./plan.tfplan
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "plan overwrite via cp command",
			script: `
cp /tmp/evil.plan /workspace/./plan.tfplan
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "tofu apply with extra -var flag",
			script: `
tofu apply -var="env=prod" -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "tofu apply with -target flag",
			script: `
tofu apply -target=google_cloud_run_v2_service.bff -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "tofu apply with unapproved plan file",
			script: `
tofu apply -input=false -auto-approve /workspace/plan.tfplan.unapproved
`,
		},
		{
			name: "tofu apply missing plan file",
			script: `
tofu apply -input=false -auto-approve
`,
		},
		{
			name: "plan overwrite in apply step",
			script: `
echo "fake-plan" > /workspace/plan.tfplan
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "tofu destroy before tofu apply",
			script: `
tofu init -input=false -lockfile=readonly
tofu destroy -auto-approve -input=false
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "tofu state rm before tofu apply",
			script: `
tofu init -input=false -lockfile=readonly
tofu state rm google_cloud_run_v2_service.bff
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "tofu import before tofu apply",
			script: `
tofu init -input=false -lockfile=readonly
tofu import google_cloud_run_v2_service.bff x
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "duplicate tofu apply",
			script: `
tofu init -input=false -lockfile=readonly
tofu apply -input=false -auto-approve /workspace/plan.tfplan
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "export TF_CLI_ARGS_apply before tofu apply",
			script: `
export TF_CLI_ARGS_apply=-lock=false
tofu init -input=false -lockfile=readonly
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "inline TF_CLI_ARGS_apply before tofu apply",
			script: `
tofu init -input=false -lockfile=readonly
TF_CLI_ARGS_apply=-lock=false tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "tofu apply before tofu init",
			script: `
tofu apply -input=false -auto-approve /workspace/plan.tfplan
tofu init -input=false -lockfile=readonly
`,
		},
		{
			name: "duplicate tofu init",
			script: `
tofu init -input=false -lockfile=readonly
tofu init -input=false -lockfile=readonly
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "tofu init with global flag",
			script: `
tofu -chdir=. init -input=false -lockfile=readonly
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "tofu init with backend false",
			script: `
tofu init -backend=false
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "tofu init with migrate-state",
			script: `
tofu init -input=false -lockfile=readonly -migrate-state
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "tofu init with backend-config",
			script: `
tofu init -input=false -lockfile=readonly -backend-config=foo.tfbackend
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "tofu init with reconfigure",
			script: `
tofu init -input=false -lockfile=readonly -reconfigure
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "tofu init with upgrade",
			script: `
tofu init -input=false -upgrade
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "tofu init missing lockfile readonly",
			script: `
tofu init -input=false
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "tofu init missing input false",
			script: `
tofu init -lockfile=readonly
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "export TF_WORKSPACE before tofu apply",
			script: `
export TF_WORKSPACE=default
tofu init -input=false -lockfile=readonly
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "export TF_CLI_CONFIG_FILE before tofu apply",
			script: `
export TF_CLI_CONFIG_FILE=/tmp/override.tfrc
tofu init -input=false -lockfile=readonly
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "export TF_DATA_DIR before tofu apply",
			script: `
export TF_DATA_DIR=/tmp/data
tofu init -input=false -lockfile=readonly
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "export OPENTOFU_LOG before tofu apply",
			script: `
export OPENTOFU_LOG=DEBUG
tofu init -input=false -lockfile=readonly
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "inline TF_WORKSPACE before tofu apply",
			script: `
tofu init -input=false -lockfile=readonly
TF_WORKSPACE=default tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "no tofu apply at all",
			script: `
echo "skipped apply"
`,
		},
		{
			name: "approved digest check masked with || true",
			script: `
set -o pipefail
printf '%s  /workspace/plan.tfplan\n' "${_APPROVED_PLAN_SHA256}" | sha256sum --strict -c - || true
if [[ -f /workspace/plan.tfplan.sha256 ]]; then
  sha256sum --strict -c /workspace/plan.tfplan.sha256
  grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256 || exit 1
fi
tofu init -input=false -lockfile=readonly
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "sha256sum without --strict flag",
			script: `
set -o pipefail
printf '%s  /workspace/plan.tfplan\n' "${_APPROVED_PLAN_SHA256}" | sha256sum -c -
if [[ -f /workspace/plan.tfplan.sha256 ]]; then
  sha256sum --strict -c /workspace/plan.tfplan.sha256
  grep -Fq "${_APPROVED_PLAN_SHA256}" /workspace/plan.tfplan.sha256 || exit 1
fi
tofu init -input=false -lockfile=readonly
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
		{
			name: "sidecar checksum grep removed",
			script: `
set -o pipefail
printf '%s  /workspace/plan.tfplan\n' "${_APPROVED_PLAN_SHA256}" | sha256sum --strict -c -
if [[ -f /workspace/plan.tfplan.sha256 ]]; then
  sha256sum --strict -c /workspace/plan.tfplan.sha256
fi
tofu init -input=false -lockfile=readonly
tofu apply -input=false -auto-approve /workspace/plan.tfplan
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateApplyStepScript(tc.script); err == nil {
				t.Errorf("expected failure for %s, got nil", tc.name)
			}
		})
	}
}

func findCloudBuildYAML(t *testing.T) string {
	t.Helper()
	if srcdir := os.Getenv("TEST_SRCDIR"); srcdir != "" {
		if rel := os.Getenv("CLOUDBUILD_YAML"); rel != "" {
			p := filepath.Join(srcdir, rel)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	candidates := []string{
		filepath.Join("..", "cloudbuild", "cloudbuild.yaml"),
		filepath.Join("deploy", "gcp", "cloudbuild", "cloudbuild.yaml"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Fatal("could not locate cloudbuild.yaml for test")
	return ""
}

func testPipelineMutation(t *testing.T, baseContent []byte, name, target, replacement string) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		mutated := strings.Replace(string(baseContent), target, replacement, 1)
		tmpFile := filepath.Join(t.TempDir(), "cloudbuild.yaml")
		if err := os.WriteFile(tmpFile, []byte(mutated), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := CheckPipelineConfig(tmpFile); err == nil {
			t.Errorf("expected mutation %s to fail CheckPipelineConfig, got nil", name)
		}
	})
}

func testPermittedNeutralEnv(t *testing.T, baseContent []byte) {
	t.Helper()
	t.Run("apply step env with permitted neutral variable passes", func(t *testing.T) {
		target := "- id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\""
		replacement := target + "\n      - \"MY_VAR=allowed\""
		neutral := strings.Replace(string(baseContent), target, replacement, 1)
		tmpFile := filepath.Join(t.TempDir(), "cloudbuild.yaml")
		if err := os.WriteFile(tmpFile, []byte(neutral), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := CheckPipelineConfig(tmpFile); err != nil {
			t.Fatalf("expected neutral env variable in apply step to pass CheckPipelineConfig, got: %v", err)
		}
	})
}

func TestCheckPipelineConfig(t *testing.T) {
	validPath := findCloudBuildYAML(t)
	if err := CheckPipelineConfig(validPath); err != nil {
		t.Fatalf("expected valid cloudbuild.yaml to pass CheckPipelineConfig, got: %v", err)
	}

	baseContent, err := os.ReadFile(validPath)
	if err != nil {
		t.Fatal(err)
	}

	mutations := []struct {
		name        string
		target      string
		replacement string
	}{
		{
			name:        "replan planted in apply step",
			target:      "tofu apply -input=false -auto-approve /workspace/plan.tfplan",
			replacement: "tofu plan -input=false -out=/workspace/plan.tfplan\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
		},
		{
			name:        "global option replan planted in apply step",
			target:      "tofu apply -input=false -auto-approve /workspace/plan.tfplan",
			replacement: "tofu -chdir=. plan -input=false -out=/workspace/./plan.tfplan\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
		},
		{
			name:        "plan overwrite via unnormalized redirection in apply step",
			target:      "tofu apply -input=false -auto-approve /workspace/plan.tfplan",
			replacement: "echo bad > /workspace/./plan.tfplan\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
		},
		{
			name:        "plan overwrite via cp in apply step",
			target:      "tofu apply -input=false -auto-approve /workspace/plan.tfplan",
			replacement: "cp /tmp/bad /workspace/./plan.tfplan\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
		},
		{
			name:        "extra flag in tofu apply",
			target:      "tofu apply -input=false -auto-approve /workspace/plan.tfplan",
			replacement: "tofu apply -var \"env=prod\" -input=false -auto-approve /workspace/plan.tfplan",
		},
		{
			name:        "tofu destroy planted in apply step",
			target:      "tofu apply -input=false -auto-approve /workspace/plan.tfplan",
			replacement: "tofu destroy -auto-approve -input=false\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
		},
		{
			name:        "tofu state rm planted in apply step",
			target:      "tofu apply -input=false -auto-approve /workspace/plan.tfplan",
			replacement: "tofu state rm google_cloud_run_v2_service.bff\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
		},
		{
			name:        "tofu import planted in apply step",
			target:      "tofu apply -input=false -auto-approve /workspace/plan.tfplan",
			replacement: "tofu import google_cloud_run_v2_service.bff x\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
		},
		{
			name:        "duplicate tofu apply in apply step",
			target:      "tofu apply -input=false -auto-approve /workspace/plan.tfplan",
			replacement: "tofu apply -input=false -auto-approve /workspace/plan.tfplan\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
		},
		{
			name:        "export TF_CLI_ARGS_apply in apply step",
			target:      "tofu apply -input=false -auto-approve /workspace/plan.tfplan",
			replacement: "export TF_CLI_ARGS_apply=-lock=false\n        tofu apply -input=false -auto-approve /workspace/plan.tfplan",
		},
		{
			name:        "crane digest recheck removed",
			target:      "test \"$(crane digest \"${_ARTIFACT_REGISTRY}@${_SERVING_DIGEST}\")\" = \"${_SERVING_DIGEST}\"",
			replacement: "# crane digest omitted",
		},
		{
			name:        "cancel self filter dropped",
			target:      "AND id!=${BUILD_ID}",
			replacement: "",
		},
		{
			name:        "record describe dropped",
			target:      "gcloud run services describe bff --region \"${_REGION}\" --format=json > /workspace/service.after.json",
			replacement: "# describe omitted",
		},
		{
			name:        "step env with TF_WORKSPACE",
			target:      "- id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"",
			replacement: "- id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n      - \"TF_WORKSPACE=unreviewed\"",
		},
		{
			name:        "step env with TF_CLI_CONFIG_FILE",
			target:      "- id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"",
			replacement: "- id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n      - \"TF_CLI_CONFIG_FILE=/workspace/unreviewed.tfrc\"",
		},
		{
			name:        "step env with OPENTOFU_LOG",
			target:      "- id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"",
			replacement: "- id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n      - \"OPENTOFU_LOG=trace\"",
		},
		{
			name:        "step env with GOOGLE_APPLICATION_CREDENTIALS",
			target:      "- id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"",
			replacement: "- id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n      - \"GOOGLE_APPLICATION_CREDENTIALS=/workspace/sa.json\"",
		},
		{
			name:        "step env with GCLOUD_PROJECT",
			target:      "- id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"",
			replacement: "- id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n      - \"GCLOUD_PROJECT=unreviewed\"",
		},
		{
			name:        "step env with CLOUDSDK_CORE_PROJECT",
			target:      "- id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"",
			replacement: "- id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"\n      - \"CLOUDSDK_CORE_PROJECT=unreviewed\"",
		},
		{
			name:        "plan step missing approved plan sha env",
			target:      "- id: plan\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"",
			replacement: "- id: plan\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"",
		},
		{
			name:        "plan step missing storage url env",
			target:      "- id: plan\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"",
			replacement: "- id: plan\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"",
		},
		{
			name:        "apply step missing approved plan sha env",
			target:      "- id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"",
			replacement: "- id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"",
		},
		{
			name:        "apply step missing storage url env",
			target:      "- id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"\n      - \"_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}\"",
			replacement: "- id: apply-behind-the-gate\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"",
		},
		{
			name:        "record step missing approved plan sha env",
			target:      "- id: record\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash\n    env:\n      - \"_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}\"",
			replacement: "- id: record\n    name: \"${_TOOLS_IMAGE}\"\n    entrypoint: bash",
		},
		{
			name:        "options missing automapSubstitutions",
			target:      "automapSubstitutions: true",
			replacement: "automapSubstitutions: false",
		},
		{
			name:        "options env with TF_WORKSPACE",
			target:      "options:\n  logging: CLOUD_LOGGING_ONLY",
			replacement: "options:\n  logging: CLOUD_LOGGING_ONLY\n  env: [\"TF_WORKSPACE=unreviewed\"]",
		},
		{
			name:        "approved digest check masked with || true",
			target:      "printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c -",
			replacement: "printf '%s  /workspace/plan.tfplan\\n' \"${_APPROVED_PLAN_SHA256}\" | sha256sum --strict -c - || true",
		},
		{
			name:        "sha256sum without --strict flag in pipeline",
			target:      "sha256sum --strict -c -",
			replacement: "sha256sum -c -",
		},
		{
			name:        "sidecar checksum grep removed in pipeline",
			target:      "grep -Fq \"${_APPROVED_PLAN_SHA256}\" /workspace/plan.tfplan.sha256 || {",
			replacement: "true || {",
		},
	}

	for _, m := range mutations {
		testPipelineMutation(t, baseContent, m.name, m.target, m.replacement)
	}
	testPermittedNeutralEnv(t, baseContent)
}

func TestCheckServiceConfigAcceptsInlineHCLComments(t *testing.T) {
	dir := t.TempDir()
	versionsContent := `
provider "google" {
  project = jsondecode(/* harmless */ file(var.inputs_file)).contract.project_id
  region  = jsondecode(/* harmless */ file(var.inputs_file)).contract.region
}
`
	mainContent := `
resource "google_cloud_run_v2_service" "bff" {
  provider = google /* comment */ . /* comment */ funnel
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(versionsContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(mainContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err != nil {
		t.Errorf("expected inline comments in project, region, and provider to pass CheckServiceConfig, got: %v", err)
	}
}

func TestCheckServiceConfigRejectsDisguisedOrIgnoredTFFiles(t *testing.T) {
	cases := []struct {
		name     string
		filename string
	}{
		{"dot-prefixed tf", ".providers.tf"},
		{"emacs autosave tf", "#providers.tf#"},
		{"tilde-suffixed tf", "providers.tf~"},
		{"dot-prefixed tofu", ".main.tofu"},
		{"hash-prefixed tofu", "#main.tofu"},
		{"emacs autosave tofu", "#main.tofu#"},
		{"tilde-suffixed tofu", "main.tofu~"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			versionsContent := `
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
			if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(versionsContent), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, tc.filename), []byte("resource \"a\" {}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := CheckServiceConfig(dir); err == nil {
				t.Errorf("expected error for disguised/ignored file %s, got nil", tc.filename)
			}
		})
	}
}

func testSubdirectoryTFFile(t *testing.T, name, subdir, file string) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		dir := t.TempDir()
		versionsContent := `
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
		if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(versionsContent), 0o600); err != nil {
			t.Fatal(err)
		}
		subDirPath := filepath.Join(dir, subdir)
		if err := os.MkdirAll(subDirPath, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(subDirPath, file), []byte("resource \"a\" {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := CheckServiceConfig(dir); err == nil {
			t.Errorf("expected error for file in subdirectory %s/%s, got nil", subdir, file)
		}
	})
}

func TestCheckServiceConfigRejectsSubdirectoryTFFiles(t *testing.T) {
	subcases := []struct {
		name   string
		subdir string
		file   string
	}{
		{"tf in sub directory", "sub", "providers.tf"},
		{"tf in hidden directory", ".hidden", "providers.tf"},
		{"tofu in sub directory", "sub", "main.tofu"},
		{"tf in tests directory", "tests", "providers.tf"},
		{"disguised tftest in tests directory", "tests", ".service.tftest.hcl"},
	}

	for _, sc := range subcases {
		testSubdirectoryTFFile(t, sc.name, sc.subdir, sc.file)
	}
}

func TestCheckServiceConfigAcceptsHarmlessTildeFile(t *testing.T) {
	dir := t.TempDir()
	versionsContent := `
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(versionsContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md~"), []byte("# Notes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err != nil {
		t.Errorf("expected harmless README.md~ to pass CheckServiceConfig, got: %v", err)
	}
}

func TestCheckServiceConfigAcceptsValidTestsDirectory(t *testing.T) {
	dir := t.TempDir()
	versionsContent := `
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(versionsContent), 0o600); err != nil {
		t.Fatal(err)
	}
	testsDir := filepath.Join(dir, "tests")
	if err := os.MkdirAll(testsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testsDir, "service.tftest.hcl"), []byte("# valid test file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err != nil {
		t.Fatalf("expected valid tests directory to pass CheckServiceConfig, got: %v", err)
	}
}

func makeValidTestPlan(inputsFile string) map[string]any {
	return map[string]any{
		"variables": map[string]any{
			"inputs_file": map[string]any{"value": inputsFile},
		},
		"configuration": map[string]any{
			"provider_config": map[string]any{
				"google": map[string]any{
					"expressions": map[string]any{
						"project": map[string]any{"references": []any{"local.inputs.contract.project_id"}},
						"region":  map[string]any{"references": []any{"local.inputs.contract.region"}},
					},
				},
			},
			"root_module": map[string]any{
				"resources": []any{
					map[string]any{
						"mode":    "managed",
						"address": "google_cloud_run_v2_service.bff",
						"expressions": map[string]any{
							"template": []any{
								map[string]any{
									"max_instance_request_concurrency": map[string]any{"references": []any{"local.inputs.bounds.concurrency"}},
									"timeout":                          map[string]any{"references": []any{"local.inputs.bounds.request_timeout_seconds"}},
									"scaling": []any{
										map[string]any{
											"max_instance_count": map[string]any{"references": []any{"local.inputs.bounds.max_instances"}},
										},
									},
								},
							},
						},
					},
					map[string]any{
						"mode":    "managed",
						"address": "google_cloud_run_v2_service_iam_member.public_invoker",
					},
				},
				"outputs": map[string]any{
					"serving": map[string]any{},
				},
			},
		},
		"planned_values": map[string]any{
			"root_module": map[string]any{
				"resources": []any{
					map[string]any{
						"address": "google_cloud_run_v2_service.bff",
						"values": map[string]any{
							"template": []any{
								map[string]any{
									"scaling": []any{
										map[string]any{"max_instance_count": 2},
									},
								},
							},
						},
					},
					map[string]any{"address": "google_cloud_run_v2_service_iam_member.public_invoker"},
				},
			},
			"outputs": map[string]any{
				"serving": map[string]any{
					"value": map[string]any{
						"allowed_origin": "https://example.com",
						"image":          "gcr.io/proj/bff@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
						"max_instances":  10,
						"port":           8080,
						"revision":       "v1.0.0",
					},
				},
			},
		},
	}
}

func TestCheckTestStreamValid(t *testing.T) {
	dir := t.TempDir()
	streamPath := filepath.Join(dir, "test.jsonl")

	summaryLine := `{"type": "test_summary", "test_summary": {"status": "pass", "passed": 9, "failed": 0, "errored": 0}}`
	p1, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": makeValidTestPlan("../inputs/example.inputs.json")})
	p2, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": makeValidTestPlan("../inputs/budget.inputs.json")})

	content := strings.Join([]string{summaryLine, string(p1), string(p2)}, "\n") + "\n"
	if err := os.WriteFile(streamPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := CheckTestStream(streamPath); err != nil {
		t.Fatalf("expected valid test stream to pass CheckTestStream, got: %v", err)
	}
}

func writeTestStream(dir, name string, lines []string) string {
	p := filepath.Join(dir, name)
	_ = os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
	return p
}

func testStreamSummaryFailures(t *testing.T, dir string, validP1, validP2 []byte) {
	t.Helper()
	// Missing file
	if err := CheckTestStream(filepath.Join(dir, "nonexistent.jsonl")); err == nil {
		t.Error("expected error for non-existent file, got nil")
	}

	// Malformed JSON line is ignored by parseStreamLine, but summary will be nil
	malformedPath := filepath.Join(dir, "malformed.jsonl")
	_ = os.WriteFile(malformedPath, []byte("not-json\n"), 0o600)
	if err := CheckTestStream(malformedPath); err == nil {
		t.Error("expected error when summary is missing, got nil")
	}

	// Summary failure: status != pass
	p := writeTestStream(dir, "status_fail.jsonl", []string{
		`{"type": "test_summary", "test_summary": {"status": "fail", "passed": 9, "failed": 1, "errored": 0}}`,
		string(validP1), string(validP2),
	})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for failed test summary status, got nil")
	}

	// Summary failure: passed < 9
	p = writeTestStream(dir, "passed_few.jsonl", []string{
		`{"type": "test_summary", "test_summary": {"status": "pass", "passed": 8, "failed": 0, "errored": 0}}`,
		string(validP1), string(validP2),
	})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for passed < 9, got nil")
	}

	// Summary failure: errored > 0
	p = writeTestStream(dir, "errored.jsonl", []string{
		`{"type": "test_summary", "test_summary": {"status": "pass", "passed": 9, "failed": 0, "errored": 1}}`,
		string(validP1), string(validP2),
	})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for errored > 0, got nil")
	}
}

func testStreamResourceFailures(t *testing.T, dir string, validSummary string, validP2 []byte) {
	t.Helper()
	// Plan failure: module_calls present
	planWithModule := makeValidTestPlan("../inputs/example.inputs.json")
	planWithModule["configuration"].(map[string]any)["root_module"].(map[string]any)["module_calls"] = map[string]any{"sub": map[string]any{}}
	pwModule, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planWithModule})
	p := writeTestStream(dir, "module_call.jsonl", []string{validSummary, string(pwModule), string(validP2)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for module_calls present, got nil")
	}

	// Plan failure: unmanaged resource mode
	planUnmanaged := makeValidTestPlan("../inputs/example.inputs.json")
	planUnmanaged["configuration"].(map[string]any)["root_module"].(map[string]any)["resources"].([]any)[0].(map[string]any)["mode"] = "data"
	pwUnmanaged, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planUnmanaged})
	p = writeTestStream(dir, "unmanaged.jsonl", []string{validSummary, string(pwUnmanaged), string(validP2)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for data resource mode, got nil")
	}

	// Plan failure: unauthorized resource address
	planBadAddr := makeValidTestPlan("../inputs/example.inputs.json")
	planBadAddr["configuration"].(map[string]any)["root_module"].(map[string]any)["resources"].([]any)[0].(map[string]any)["address"] = "google_storage_bucket.bad"
	pwBadAddr, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planBadAddr})
	p = writeTestStream(dir, "bad_addr.jsonl", []string{validSummary, string(pwBadAddr), string(validP2)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for unexpected resource address, got nil")
	}

	// Plan failure: planned_values resource address mismatch
	planPVBadAddr := makeValidTestPlan("../inputs/example.inputs.json")
	planPVBadAddr["planned_values"].(map[string]any)["root_module"].(map[string]any)["resources"].([]any)[0].(map[string]any)["address"] = "google_storage_bucket.bad"
	pwPVBadAddr, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planPVBadAddr})
	p = writeTestStream(dir, "pv_bad_addr.jsonl", []string{validSummary, string(pwPVBadAddr), string(validP2)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for planned_values resource mismatch, got nil")
	}

	// Plan failure: service missing template
	planNoTmpl := makeValidTestPlan("../inputs/example.inputs.json")
	delete(planNoTmpl["configuration"].(map[string]any)["root_module"].(map[string]any)["resources"].([]any)[0].(map[string]any)["expressions"].(map[string]any), "template")
	pwNoTmpl, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planNoTmpl})
	p = writeTestStream(dir, "no_tmpl.jsonl", []string{validSummary, string(pwNoTmpl), string(validP2)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for missing template expressions, got nil")
	}

	// Plan failure: service missing scaling
	planNoScaling := makeValidTestPlan("../inputs/example.inputs.json")
	planNoScaling["configuration"].(map[string]any)["root_module"].(map[string]any)["resources"].([]any)[0].(map[string]any)["expressions"].(map[string]any)["template"].([]any)[0].(map[string]any)["scaling"] = []any{}
	pwNoScaling, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planNoScaling})
	p = writeTestStream(dir, "no_scaling.jsonl", []string{validSummary, string(pwNoScaling), string(validP2)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for missing scaling, got nil")
	}

	// Plan failure: wrong bounds references
	planWrongBounds := makeValidTestPlan("../inputs/example.inputs.json")
	planWrongBounds["configuration"].(map[string]any)["root_module"].(map[string]any)["resources"].([]any)[0].(map[string]any)["expressions"].(map[string]any)["template"].([]any)[0].(map[string]any)["scaling"].([]any)[0].(map[string]any)["max_instance_count"] = map[string]any{"references": []any{"wrong"}}
	pwWrongBounds, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planWrongBounds})
	p = writeTestStream(dir, "wrong_bounds.jsonl", []string{validSummary, string(pwWrongBounds), string(validP2)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for wrong bounds references, got nil")
	}

	// Plan failure: wrong concurrency references
	planWrongConc := makeValidTestPlan("../inputs/example.inputs.json")
	planWrongConc["configuration"].(map[string]any)["root_module"].(map[string]any)["resources"].([]any)[0].(map[string]any)["expressions"].(map[string]any)["template"].([]any)[0].(map[string]any)["max_instance_request_concurrency"] = map[string]any{"references": []any{"wrong"}}
	pwWrongConc, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planWrongConc})
	p = writeTestStream(dir, "wrong_conc.jsonl", []string{validSummary, string(pwWrongConc), string(validP2)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for wrong concurrency references, got nil")
	}

	// Plan failure: wrong timeout references
	planWrongTimeout := makeValidTestPlan("../inputs/example.inputs.json")
	planWrongTimeout["configuration"].(map[string]any)["root_module"].(map[string]any)["resources"].([]any)[0].(map[string]any)["expressions"].(map[string]any)["template"].([]any)[0].(map[string]any)["timeout"] = map[string]any{"references": []any{"wrong"}}
	pwWrongTimeout, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planWrongTimeout})
	p = writeTestStream(dir, "wrong_timeout.jsonl", []string{validSummary, string(pwWrongTimeout), string(validP2)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for wrong timeout references, got nil")
	}
}

func testStreamServingOutputFailures(t *testing.T, dir string, validSummary string, validP2 []byte) {
	t.Helper()
	// Plan failure: unauthorized output name
	planBadOutput := makeValidTestPlan("../inputs/example.inputs.json")
	planBadOutput["configuration"].(map[string]any)["root_module"].(map[string]any)["outputs"] = map[string]any{"unauthorized": map[string]any{}}
	pwBadOutput, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planBadOutput})
	p := writeTestStream(dir, "bad_output.jsonl", []string{validSummary, string(pwBadOutput), string(validP2)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for unauthorized output name, got nil")
	}

	// Plan failure: planned_values unauthorized output name
	planPVBadOutput := makeValidTestPlan("../inputs/example.inputs.json")
	planPVBadOutput["planned_values"].(map[string]any)["outputs"] = map[string]any{"unauthorized": map[string]any{}}
	pwPVBadOutput, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planPVBadOutput})
	p = writeTestStream(dir, "pv_bad_output.jsonl", []string{validSummary, string(pwPVBadOutput), string(validP2)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for planned_values unauthorized output name, got nil")
	}

	// Plan failure: serving missing from planned_values
	planNoServing := makeValidTestPlan("../inputs/example.inputs.json")
	planNoServing["planned_values"].(map[string]any)["outputs"] = map[string]any{}
	pwNoServing, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planNoServing})
	p = writeTestStream(dir, "no_serving.jsonl", []string{validSummary, string(pwNoServing), string(validP2)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error when serving is missing from planned_values, got nil")
	}

	// Plan failure: serving value is not a map
	planServingNotMap := makeValidTestPlan("../inputs/example.inputs.json")
	planServingNotMap["planned_values"].(map[string]any)["outputs"].(map[string]any)["serving"] = map[string]any{"value": "not-a-map"}
	pwServingNotMap, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planServingNotMap})
	p = writeTestStream(dir, "serving_not_map.jsonl", []string{validSummary, string(pwServingNotMap), string(validP2)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error when serving value is not a map, got nil")
	}

	// Plan failure: serving output has unauthorized key
	planExtraKey := makeValidTestPlan("../inputs/example.inputs.json")
	planExtraKey["planned_values"].(map[string]any)["outputs"].(map[string]any)["serving"].(map[string]any)["value"].(map[string]any)["extra"] = "bad"
	pwExtraKey, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planExtraKey})
	p = writeTestStream(dir, "extra_key.jsonl", []string{validSummary, string(pwExtraKey), string(validP2)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for extra serving key, got nil")
	}

	// Plan failure: serving output missing required key
	planMissingKey := makeValidTestPlan("../inputs/example.inputs.json")
	delete(planMissingKey["planned_values"].(map[string]any)["outputs"].(map[string]any)["serving"].(map[string]any)["value"].(map[string]any), "revision")
	pwMissingKey, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planMissingKey})
	p = writeTestStream(dir, "missing_key.jsonl", []string{validSummary, string(pwMissingKey), string(validP2)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for missing serving key, got nil")
	}
}

func testStreamProviderAndBudgetFailures(t *testing.T, dir string, validSummary string, validP1, validP2 []byte) {
	t.Helper()
	// Plan failure: budget alert strings leaked
	planBudgetLeak := makeValidTestPlan("../inputs/example.inputs.json")
	planBudgetLeak["planned_values"].(map[string]any)["outputs"].(map[string]any)["serving"].(map[string]any)["value"].(map[string]any)["revision"] = "budget_alert_amount_leaked"
	pwBudgetLeak, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planBudgetLeak})
	p := writeTestStream(dir, "budget_leak.jsonl", []string{validSummary, string(pwBudgetLeak), string(validP2)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for budget alert string in serving output, got nil")
	}

	// Plan failure: provider project not bound
	planBadProj := makeValidTestPlan("../inputs/example.inputs.json")
	planBadProj["configuration"].(map[string]any)["provider_config"].(map[string]any)["google"].(map[string]any)["expressions"].(map[string]any)["project"] = map[string]any{"references": []any{"unbound"}}
	pwBadProj, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planBadProj})
	p = writeTestStream(dir, "bad_proj.jsonl", []string{validSummary, string(pwBadProj), string(validP2)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for unbound provider project, got nil")
	}

	// Plan failure: provider region not bound
	planBadReg := makeValidTestPlan("../inputs/example.inputs.json")
	planBadReg["configuration"].(map[string]any)["provider_config"].(map[string]any)["google"].(map[string]any)["expressions"].(map[string]any)["region"] = map[string]any{"references": []any{"unbound"}}
	pwBadReg, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planBadReg})
	p = writeTestStream(dir, "bad_reg.jsonl", []string{validSummary, string(pwBadReg), string(validP2)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error for unbound provider region, got nil")
	}

	// Plan failure: serving outputs vary between example and budget plans
	planBudgetDiff := makeValidTestPlan("../inputs/budget.inputs.json")
	planBudgetDiff["planned_values"].(map[string]any)["outputs"].(map[string]any)["serving"].(map[string]any)["value"].(map[string]any)["port"] = 9090
	pwBudgetDiff, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planBudgetDiff})
	p = writeTestStream(dir, "variance.jsonl", []string{validSummary, string(validP1), string(pwBudgetDiff)})
	if err := CheckTestStream(p); err == nil {
		t.Error("expected error when serving outputs vary between plans, got nil")
	}
}

func testStreamComparisonFailures(t *testing.T, dir string, validSummary string, validP1, validP2 []byte) {
	t.Helper()
	// Plan failure: missing budget comparison plan
	pMissingBudget := writeTestStream(dir, "missing_budget.jsonl", []string{validSummary, string(validP1)})
	if err := CheckTestStream(pMissingBudget); err == nil {
		t.Error("expected error when budget plan is missing, got nil")
	}

	// Plan failure: missing example comparison plan
	pMissingExample := writeTestStream(dir, "missing_example.jsonl", []string{validSummary, string(validP2)})
	if err := CheckTestStream(pMissingExample); err == nil {
		t.Error("expected error when example plan is missing, got nil")
	}

	// Plan failure: planned resource bounds vary between example and budget plans
	planResDiff := makeValidTestPlan("../inputs/budget.inputs.json")
	planResDiff["planned_values"].(map[string]any)["root_module"].(map[string]any)["resources"].([]any)[0].(map[string]any)["values"].(map[string]any)["template"].([]any)[0].(map[string]any)["scaling"].([]any)[0].(map[string]any)["max_instance_count"] = 10
	pwResDiff, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": planResDiff})
	pResDiff := writeTestStream(dir, "res_diff.jsonl", []string{validSummary, string(validP1), string(pwResDiff)})
	if err := CheckTestStream(pResDiff); err == nil {
		t.Error("expected error when planned resource bounds vary between plans, got nil")
	}
}

func testStreamServingFailures(t *testing.T, dir string, validSummary string, validP1, validP2 []byte) {
	t.Helper()
	testStreamServingOutputFailures(t, dir, validSummary, validP2)
	testStreamProviderAndBudgetFailures(t, dir, validSummary, validP1, validP2)
	testStreamComparisonFailures(t, dir, validSummary, validP1, validP2)
}

func TestCheckTestStreamFailures(t *testing.T) {
	dir := t.TempDir()
	validSummary := `{"type": "test_summary", "test_summary": {"status": "pass", "passed": 9, "failed": 0, "errored": 0}}`
	validP1, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": makeValidTestPlan("../inputs/example.inputs.json")})
	validP2, _ := json.Marshal(map[string]any{"type": "test_plan", "test_plan": makeValidTestPlan("../inputs/budget.inputs.json")})

	testStreamSummaryFailures(t, dir, validP1, validP2)
	testStreamResourceFailures(t, dir, validSummary, validP2)
	testStreamServingFailures(t, dir, validSummary, validP1, validP2)
}

func TestCheckServiceConfigEdgeBranches(t *testing.T) {
	dir := t.TempDir()

	// 1. Missing versions.tf -> 0 provider blocks
	_ = os.WriteFile(filepath.Join(dir, "main.tf"), []byte("resource \"google_cloud_run_v2_service\" \"bff\" {}\n"), 0o600)
	if err := CheckServiceConfig(dir); err == nil {
		t.Error("expected error when 0 google provider blocks, got nil")
	}

	// 2. Multiple google provider blocks
	dir2 := t.TempDir()
	content2 := `
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
provider "google" {
  alias   = "second"
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	_ = os.WriteFile(filepath.Join(dir2, "versions.tf"), []byte(content2), 0o600)
	if err := CheckServiceConfig(dir2); err == nil {
		t.Error("expected error for duplicate google provider blocks, got nil")
	}

	// 3. Provider with nested block
	dir3 := t.TempDir()
	content3 := `
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
  nested_block {
    key = "value"
  }
}
`
	_ = os.WriteFile(filepath.Join(dir3, "versions.tf"), []byte(content3), 0o600)
	if err := CheckServiceConfig(dir3); err == nil {
		t.Error("expected error for nested block in provider, got nil")
	}

	// 4. Provider missing project
	dir4 := t.TempDir()
	content4 := `
provider "google" {
  region = jsondecode(file(var.inputs_file)).contract.region
}
`
	_ = os.WriteFile(filepath.Join(dir4, "versions.tf"), []byte(content4), 0o600)
	if err := CheckServiceConfig(dir4); err == nil {
		t.Error("expected error for provider missing project, got nil")
	}

	// 5. Provider missing region
	dir5 := t.TempDir()
	content5 := `
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
}
`
	_ = os.WriteFile(filepath.Join(dir5, "versions.tf"), []byte(content5), 0o600)
	if err := CheckServiceConfig(dir5); err == nil {
		t.Error("expected error for provider missing region, got nil")
	}

	// 6. Unlabeled provider block
	dir6 := t.TempDir()
	content6 := `
provider {
}
`
	_ = os.WriteFile(filepath.Join(dir6, "versions.tf"), []byte(content6), 0o600)
	if err := CheckServiceConfig(dir6); err == nil {
		t.Error("expected error for unlabeled provider, got nil")
	}

	// 7. Non-existent directory
	if err := CheckServiceConfig(filepath.Join(dir, "nonexistent")); err == nil {
		t.Error("expected error for non-existent directory, got nil")
	}
}

func testTofuFlagsAndCommands(t *testing.T) {
	t.Helper()
	// isGlobalTofuFlag
	flags := []string{"-chdir=/path", "-no-color", "-help", "--help", "-h", "-version", "-v", "--version", "-other", "not-a-flag"}
	for _, f := range flags {
		_ = isGlobalTofuFlag(f)
	}

	// isTofuSeparator
	seps := []string{";", "&&", "||", "|", "&", "other"}
	for _, s := range seps {
		_ = isTofuSeparator(s)
	}

	// parseTofuCommands with comment and separators
	cmds := parseTofuCommands("tofu -chdir=/foo init -input=false && tofu apply -auto-approve /path/plan.tfplan # trailing comment")
	if len(cmds) != 2 {
		t.Errorf("expected 2 tofu commands, got %d", len(cmds))
	}

	// parseTofuCommands comment-only line
	if cmdsNone := parseTofuCommands("# only a comment"); len(cmdsNone) != 0 {
		t.Errorf("expected 0 commands for comment line, got %d", len(cmdsNone))
	}
}

func testTofuInitAndApply(t *testing.T) {
	t.Helper()
	// checkTofuInitCmd edge cases
	if err := checkTofuInitCmd(tofuInvocation{subcommand: "init", globalFlags: []string{"-chdir=/unreviewed"}}); err == nil {
		t.Error("expected error for unreviewed chdir in init, got nil")
	}
	if err := checkTofuInitCmd(tofuInvocation{subcommand: "init", args: []string{"-backend=false"}}); err == nil {
		t.Error("expected error for missing -input=false in init, got nil")
	}

	// checkTofuApplyCmd edge cases
	if err := checkTofuApplyCmd(tofuInvocation{subcommand: "apply", globalFlags: []string{"-chdir=/unreviewed"}}); err == nil {
		t.Error("expected error for unreviewed chdir in apply, got nil")
	}
	if err := checkTofuApplyCmd(tofuInvocation{subcommand: "apply", args: []string{"-auto-approve"}}); err == nil {
		t.Error("expected error for missing -input=false in apply, got nil")
	}
	if err := checkTofuApplyCmd(tofuInvocation{subcommand: "apply", args: []string{"-input=false"}}); err == nil {
		t.Error("expected error for missing -auto-approve in apply, got nil")
	}
	if err := checkTofuApplyCmd(tofuInvocation{subcommand: "apply", args: []string{"-input=false", "-auto-approve"}}); err == nil {
		t.Error("expected error for missing plan argument in apply, got nil")
	}
}

func testTofuCopyAndEnvAndScript(t *testing.T) {
	t.Helper()
	// checkCopyOverwrite edge cases
	if !checkCopyOverwrite([]string{"cp", "/foo", "/workspace/plan.tfplan"}, 0) {
		t.Error("expected true for cp overwriting plan")
	}
	if !checkCopyOverwrite([]string{"mv", "/foo", "/workspace/plan.tfplan"}, 0) {
		t.Error("expected true for mv overwriting plan")
	}
	if checkCopyOverwrite([]string{"echo", "/foo"}, 0) {
		t.Error("expected false for echo")
	}

	// checkNoEnvOverride edge cases
	if err := checkNoEnvOverride("export TF_CLI_ARGS_apply=-lock=false"); err == nil {
		t.Error("expected error for TF_CLI_ARGS_apply, got nil")
	}
	if err := checkNoEnvOverride("export TF_VAR_foo=bar"); err == nil {
		t.Error("expected error for TF_VAR_foo, got nil")
	}

	// ValidateApplyStepScript with forbidden commands
	badScripts := []string{
		"unauthorized_command /workspace/plan.tfplan",
		"tofu plan",
		"curl http://malicious",
	}
	for _, bs := range badScripts {
		if err := ValidateApplyStepScript(bs); err == nil {
			t.Errorf("expected error for bad script %q, got nil", bs)
		}
	}
}

func TestTofuHelperFunctionsCoverage(t *testing.T) {
	testTofuFlagsAndCommands(t)
	testTofuInitAndApply(t)
	testTofuCopyAndEnvAndScript(t)
}

func TestCheckServiceConfigAcceptsHashPrefixedTfFile(t *testing.T) {
	dir := t.TempDir()
	versionsContent := `
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(versionsContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "#resources.tf"), []byte("resource \"google_cloud_run_v2_service\" \"bff\" {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err != nil {
		t.Errorf("expected #resources.tf to be accepted, got error: %v", err)
	}
}

func TestCheckServiceConfigAcceptsTestsReadme(t *testing.T) {
	dir := t.TempDir()
	testsDir := filepath.Join(dir, "tests")
	if err := os.MkdirAll(testsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	versionsContent := `
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(versionsContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testsDir, "service.tftest.hcl"), []byte("# test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testsDir, "README.md"), []byte("# Tests documentation\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err != nil {
		t.Errorf("expected tests/README.md to be accepted, got error: %v", err)
	}
}

func TestCheckServiceConfigAcceptsNestedTestsDir(t *testing.T) {
	dir := t.TempDir()
	nestedDir := filepath.Join(dir, "tests", "unit")
	if err := os.MkdirAll(nestedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	versionsContent := `
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(versionsContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "unit.tftest.hcl"), []byte("# test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "vars.tfvars"), []byte("project_id = \"p\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "payload.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "doc.md"), []byte("# doc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err != nil {
		t.Errorf("expected nested tests files to be accepted, got error: %v", err)
	}
}

func testRejectTfInTests(t *testing.T, name, filename string) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		dir := t.TempDir()
		nested := filepath.Join(dir, "tests", "nested")
		if err := os.MkdirAll(nested, 0o700); err != nil {
			t.Fatal(err)
		}
		versionsContent := `
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
		if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(versionsContent), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(nested, filename), []byte("resource \"a\" {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := CheckServiceConfig(dir); err == nil {
			t.Errorf("expected error for %s in tests/, got nil", filename)
		}
	})
}

func TestCheckServiceConfigRejectsTfInTestsSubdir(t *testing.T) {
	tests := []struct {
		name     string
		filename string
	}{
		{"tf in nested tests", "bad.tf"},
		{"tofu in nested tests", "bad.tofu"},
		{"tf.json in nested tests", "bad.tf.json"},
		{"txt in nested tests", "bad.txt"},
	}
	for _, tc := range tests {
		testRejectTfInTests(t, tc.name, tc.filename)
	}
}

func TestCheckServiceConfigAcceptsNonConfigInSubdir(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0o700); err != nil {
		t.Fatal(err)
	}
	versionsContent := `
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(versionsContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "notes.txt"), []byte("harmless note\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err != nil {
		t.Errorf("expected sub/notes.txt to be accepted, got error: %v", err)
	}
}

func TestCheckServiceConfigSkipsTerraformDir(t *testing.T) {
	dir := t.TempDir()
	tfDir := filepath.Join(dir, ".terraform")
	if err := os.MkdirAll(tfDir, 0o700); err != nil {
		t.Fatal(err)
	}
	versionsContent := `
provider "google" {
  project = jsondecode(file(var.inputs_file)).contract.project_id
  region  = jsondecode(file(var.inputs_file)).contract.region
}
`
	if err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(versionsContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tfDir, "plugin.tf"), []byte("invalid content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckServiceConfig(dir); err != nil {
		t.Errorf("expected .terraform dir to be skipped, got error: %v", err)
	}
}

func TestParseAndInspectTFFileNonExistent(t *testing.T) {
	var googleBlocks []providerBlockEntry
	if err := parseAndInspectTFFile("/nonexistent-file.tf", &googleBlocks); err == nil {
		t.Error("expected error for non-existent file, got nil")
	}
}

func TestVerifyServiceBoundsWiringNoBFF(t *testing.T) {
	resources := []any{
		map[string]any{"address": "other_service"},
	}
	if err := verifyServiceBoundsWiring(resources); err != nil {
		t.Errorf("expected nil when bff service not in list, got: %v", err)
	}
}

func testPlanInputsAndServingEdgeCases(t *testing.T) {
	t.Helper()
	if val := getPlanInputsFile(nil); val != "" {
		t.Errorf("expected empty string for nil plan, got %q", val)
	}
	if val := getPlanInputsFile(map[string]any{"variables": map[string]any{}}); val != "" {
		t.Errorf("expected empty string for missing inputs_file, got %q", val)
	}

	if val := getPlanServingValue(nil); val != nil {
		t.Errorf("expected nil for nil plan, got %v", val)
	}
	if val := getPlanServingValue(map[string]any{"planned_values": map[string]any{}}); val != nil {
		t.Errorf("expected nil for missing outputs, got %v", val)
	}
	if val := getPlanServingValue(map[string]any{"planned_values": map[string]any{"outputs": map[string]any{}}}); val != nil {
		t.Errorf("expected nil for missing serving output, got %v", val)
	}
}

func testPlanServiceResourceEdgeCases(t *testing.T) {
	t.Helper()
	if val := getPlanServiceResource(nil); val != nil {
		t.Errorf("expected nil for nil plan, got %v", val)
	}
	if val := getPlanServiceResource(map[string]any{"planned_values": "not-map"}); val != nil {
		t.Errorf("expected nil for non-map planned_values, got %v", val)
	}
	if val := getPlanServiceResource(map[string]any{"planned_values": map[string]any{}}); val != nil {
		t.Errorf("expected nil for missing root_module, got %v", val)
	}
	if val := getPlanServiceResource(map[string]any{"planned_values": map[string]any{"root_module": map[string]any{}}}); val != nil {
		t.Errorf("expected nil for missing resources, got %v", val)
	}
	if val := getPlanServiceResource(map[string]any{"planned_values": map[string]any{"root_module": map[string]any{"resources": []any{"not-map", map[string]any{"address": "other"}}}}}); val != nil {
		t.Errorf("expected nil for non-matching resources, got %v", val)
	}
}

func testPlanBoundsWiringEdgeCases(t *testing.T) {
	t.Helper()
	badScalingExpr := map[string]any{
		"scaling": []any{
			map[string]any{
				"max_instance_count": map[string]any{
					"references": []any{"local.inputs.bounds.max_instances", "local.inputs.bounds.budget_alert_amount"},
				},
			},
		},
	}
	if err := checkScalingBounds(badScalingExpr); err == nil {
		t.Error("expected error when scaling references budget_alert, got nil")
	}

	badWiring := map[string]any{
		"address": "google_cloud_run_v2_service.bff",
		"expressions": map[string]any{
			"template": []any{
				map[string]any{
					"max_instance_request_concurrency": map[string]any{
						"references": []any{"local.inputs.bounds.concurrency", "local.inputs.bounds.budget_alert_amount"},
					},
					"timeout": map[string]any{
						"references": []any{"local.inputs.bounds.request_timeout_seconds"},
					},
					"scaling": []any{
						map[string]any{
							"max_instance_count": map[string]any{
								"references": []any{"local.inputs.bounds.max_instances"},
							},
						},
					},
				},
			},
		},
	}
	if err := checkBFFServiceWiring(badWiring); err == nil {
		t.Error("expected error when wiring references budget_alert, got nil")
	}

	if err := verifyBudgetWitnessInvariance(nil); err == nil {
		t.Errorf("expected error when budget witness missing, got nil")
	}
}

func testNestedReferencesEdgeCases(t *testing.T) {
	t.Helper()
	if refs := getNestedReferences(nil, "missing"); refs != nil {
		t.Errorf("expected nil refs for nil map, got %v", refs)
	}
	if refs := getNestedReferences(map[string]any{"key": map[string]any{}}, "key"); refs != nil {
		t.Errorf("expected nil refs for missing references, got %v", refs)
	}
}

func TestPlanHelperCoverageEdgeCases(t *testing.T) {
	testPlanInputsAndServingEdgeCases(t)
	testPlanServiceResourceEdgeCases(t)
	testPlanBoundsWiringEdgeCases(t)
	testNestedReferencesEdgeCases(t)
}

func TestCheckTestStreamEmptyPlans(t *testing.T) {
	f, err := os.CreateTemp("", "empty-plans-stream-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	summary := `{"type":"test_summary","test_summary":{"status":"pass","failed":0,"errored":0,"passed":9}}` + "\n"
	if _, err := f.WriteString(summary); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := CheckTestStream(f.Name()); err == nil {
		t.Error("expected error for empty plans, got nil")
	}
}

func TestParseTofuFieldSeparateChdir(t *testing.T) {
	cmds := parseTofuCommands("tofu -chdir /custom/dir init -input=false -lockfile=readonly")
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if len(cmds[0].globalFlags) != 2 || cmds[0].globalFlags[0] != "-chdir" || cmds[0].globalFlags[1] != "/custom/dir" {
		t.Errorf("unexpected global flags: %v", cmds[0].globalFlags)
	}
}

func TestInlineCommentStrippingAndAnyFieldPlan(t *testing.T) {
	if err := checkNoEnvOverride("export FOO=bar # comment"); err != nil {
		t.Errorf("unexpected error for harmless env with comment: %v", err)
	}
	if err := checkNoPlanOverwriteInApply("echo harmless # comment"); err != nil {
		t.Errorf("unexpected error for harmless echo with comment: %v", err)
	}
	if anyFieldIsPlanPath([]string{"other", "file"}) {
		t.Error("expected false for anyFieldIsPlanPath without plan path")
	}
}

func TestApplyScriptValidatorEdgeCases(t *testing.T) {
	v := &applyScriptValidator{}
	if err := v.validateInvocation(tofuInvocation{subcommand: ""}); err == nil {
		t.Error("expected error for missing subcommand, got nil")
	}
	if err := v.validateApply(tofuInvocation{subcommand: "apply"}); err == nil {
		t.Error("expected error for apply before init, got nil")
	}
	initInv := tofuInvocation{subcommand: "init", args: []string{"-input=false", "-lockfile=readonly"}}
	if err := v.validateInit(initInv); err != nil {
		t.Errorf("unexpected error for valid init: %v", err)
	}
	if err := v.validateInit(initInv); err == nil {
		t.Error("expected error for duplicate init, got nil")
	}
	vBad := &applyScriptValidator{}
	if err := vBad.validateInit(tofuInvocation{subcommand: "init", args: []string{"bad"}}); err == nil {
		t.Error("expected error for bad init args, got nil")
	}
	vApplyFirst := &applyScriptValidator{hasInit: false, hasApply: true}
	if err := vApplyFirst.validateInit(initInv); err == nil {
		t.Error("expected error for init after apply, got nil")
	}
	if err := checkSha256Strict("sha256sum -c plan.sha256"); err == nil {
		t.Error("expected error for sha256sum missing --strict, got nil")
	}
	badShaScript := "sha256sum --strict -c -\n" +
		"grep -Fq \"${_APPROVED_PLAN_SHA256}\" /workspace/plan.tfplan.sha256\n" +
		"sha256sum -c other.sha256\n"
	if err := checkApprovalDigestVerification(badShaScript); err == nil {
		t.Error("expected error for non-strict sha256sum in checkApprovalDigestVerification, got nil")
	}
	scriptNoApply := "sha256sum --strict -c -\n" +
		"grep -Fq \"${_APPROVED_PLAN_SHA256}\" /workspace/plan.tfplan.sha256\n" +
		"tofu init -input=false -lockfile=readonly\n"
	if err := ValidateApplyStepScript(scriptNoApply); err == nil {
		t.Error("expected error for script without tofu apply, got nil")
	}
}

func TestCheckPipelineConfigEdgeCases(t *testing.T) {
	if err := CheckPipelineConfig("/nonexistent-pipeline.yaml"); err == nil {
		t.Error("expected error for non-existent pipeline, got nil")
	}
	f, err := os.CreateTemp("", "bad-yaml-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(":\n- invalid yaml : [")
	f.Close()
	if err := CheckPipelineConfig(f.Name()); err == nil {
		t.Error("expected error for invalid yaml, got nil")
	}
	f2, err := os.CreateTemp("", "no-apply-yaml-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f2.Name())
	noApplyYaml := `
options:
  automapSubstitutions: true
steps:
  - id: plan
    env:
      - "_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}"
      - "_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}"
  - id: apply-behind-the-gate
    env:
      - "_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}"
      - "_PLAN_STORAGE_URL=${_PLAN_STORAGE_URL}"
  - id: record
    env:
      - "_APPROVED_PLAN_SHA256=${_APPROVED_PLAN_SHA256}"
`
	f2.WriteString(noApplyYaml)
	f2.Close()
	if err := CheckPipelineConfig(f2.Name()); err == nil {
		t.Error("expected error for missing apply-behind-the-gate script, got nil")
	}
}
