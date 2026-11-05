package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateDocumentationPreservesCuratedResourceOrder(t *testing.T) {
	previousDocsFile := docsFile
	t.Cleanup(func() { docsFile = previousDocsFile })
	docsFile = filepath.Join(t.TempDir(), "overview.mdx")
	coverage := marker + "\n" +
		"| Resource | External name | Where to find | Notes |\n|---|---|---|---|\n" +
		"| [Subaccount](#subaccount) | Subaccount GUID | CLI lookup | Keep this note |\n" +
		"| [Directory](#directory) | Directory GUID | CLI lookup | — |\n" + endMarker
	if err := os.WriteFile(docsFile, []byte(coverage), 0644); err != nil {
		t.Fatal(err)
	}
	configs := []ResourceConfig{
		{Name: "Directory", Content: "- Format: Directory GUID\n- How to find:\n\n  - CLI: `btp list accounts/directory`"},
		{Name: "Subaccount", Content: "- Follows Standard: yes\n- Format: Subaccount GUID\n- Note: Keep A|B\n- How to find:\n\n  - CLI: `btp list accounts/subaccount`"},
	}
	generated, err := generateDocumentation(configs)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		" | — |",
		"| Resource | External name | Where to find | Notes |",
		`<span className="badge badge--success"><span className="external-name-key">GUID</span></span> | **CLI:**<br />` + "`btp list accounts/subaccount` | Keep A&#124;B |",
	} {
		if !strings.Contains(generated, expected) {
			t.Fatalf("curated coverage changed: %s", expected)
		}
	}
	if strings.Contains(generated, "Subaccount GUID") || strings.Contains(generated, "Follows Standard:") || strings.Contains(generated, "Format:") {
		t.Fatal("redundant identifier descriptions must not appear in notes")
	}
	if strings.Contains(generated, "CLI export") {
		t.Fatal("CLI export coverage belongs in the import guide")
	}
	if strings.Index(generated, ">Subaccount</Link>") > strings.Index(generated, ">Directory</Link>") {
		t.Fatal("generation changed the curated resource order")
	}
	if !strings.Contains(generated, "Keep A&#124;B") {
		t.Fatal("a pipe in an API comment would create an extra table column")
	}
	if err := updateDocsFile(generated); err != nil {
		t.Fatal(err)
	}
	again, err := generateDocumentation(configs)
	if err != nil || again != generated {
		t.Fatalf("regeneration changed the coverage table: %v", err)
	}
	configs = append(configs, ResourceConfig{Name: "NewResource", Content: "- Format: GUID"})
	if _, err := generateDocumentation(configs); err == nil {
		t.Fatal("generation silently omitted a resource without a coverage row")
	}
}

func TestExternalNameBadgeUsesIdentifierFormat(t *testing.T) {
	for _, tc := range []struct{ format, color, label string }{
		{"ServiceInstance GUID (UUID format)", "success", `<span className="external-name-key">GUID</span>`},
		{"`<appName>/<planName>`", "warning", `Compound: <span className="external-name-key">appName / planName</span>`},
		{"`<service-instance-id>/<service-binding-id>`; a bare GUID is transient", "warning", `Compound: <span className="external-name-key">service-instance-id / service-binding-id</span>`},
		{"Origin key of the identity provider", "warning", "Name"},
		{"Kyma module name", "warning", "Name"},
		{"Not applicable", "secondary", "N/A"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			want := `<span className="badge badge--` + tc.color + `">` + tc.label + `</span>`
			if got := externalNameBadge(tc.format); got != want {
				t.Fatalf("got %s, want %s", got, want)
			}
		})
	}
}

func TestUpdateDocsFilePreservesSurroundingContent(t *testing.T) {
	previousDocsFile := docsFile
	t.Cleanup(func() { docsFile = previousDocsFile })
	docsFile = filepath.Join(t.TempDir(), "overview.mdx")
	prefix := "# Import landscape\n\n| Resource | CLI |\n|---|---|\n| Subaccount | subaccount |\n\n"
	suffix := "\n\n## More guides\n\nKeep this hand-written section.\n"
	original := prefix + marker + "\n\nOld generated details\n\n" + endMarker + suffix
	if err := os.WriteFile(docsFile, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	generated := "New generated details\n"
	if err := updateDocsFile(generated); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(docsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(first), prefix+marker) || !strings.HasSuffix(string(first), endMarker+suffix) {
		t.Fatal("generation changed hand-written content outside the markers")
	}
	if strings.Contains(string(first), "Old generated details") || !strings.Contains(string(first), generated) {
		t.Fatal("generation did not replace the old details")
	}
	if err := updateDocsFile(generated); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(docsFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("repeated generation changed the document")
	}
}

func TestUpdateDocsFileRefusesIncompleteMarkers(t *testing.T) {
	previousDocsFile := docsFile
	t.Cleanup(func() { docsFile = previousDocsFile })
	for _, content := range []string{
		"Hand-written overview without markers\n",
		"Overview\n" + marker + "\nDetails without an end marker\n",
		endMarker + "\nOverview\n" + marker,
	} {
		docsFile = filepath.Join(t.TempDir(), "overview.mdx")
		if err := os.WriteFile(docsFile, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		if err := updateDocsFile("New details\n"); err == nil {
			t.Fatal("expected an error for missing or incorrectly ordered markers")
		}
		after, err := os.ReadFile(docsFile)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != content {
			t.Fatal("failed generation modified the overview")
		}
	}
}
