package yaml

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/sap/crossplane-provider-btp/internal/exporttool/erratt"
	"github.com/charmbracelet/glamour"
	"sigs.k8s.io/yaml"
)

// CommentedYAML interface specifies a Comment method. When a resource
// implements this interface, the output YAML will be commented out as
// long as the Comment method returns `true` as the second result. The
// first (string) result wil be put over the YAML in a comment.
type CommentedYAML interface {
	Comment() (string, bool)
}

// Marshal returns the YAML representation of a Kubernetes resource,
// indented and wrapped with "---" and "..." markers.
func Marshal(resource any) (string, error) {
	b, err := yaml.Marshal(resource)
	if err != nil {
		return "", err
	}
	return wrapResource(resource, string(b)), nil
}

// MarshalPretty returns a syntax-highlighted YAML string suitable for
// terminal display.
func MarshalPretty(resource any) (string, error) {
	b, err := yaml.Marshal(resource)
	if err != nil {
		jsonBytes, err2 := json.Marshal(resource)
		if err2 == nil {
			return "", erratt.Errorf("error marshalling to YAML: %w", err).With("json", string(jsonBytes))
		}
		return "", err

	}
	return glamour.Render(fmt.Sprintf("```yaml\n%s```",
		wrapResource(resource, string(b))),
		"dracula")
}

func wrapResource(resource any, y string) string {
	return commentResource(resource,
		fmt.Sprintf("---\n%s...\n", y))
}

func commentResource(resource any, y string) string {
	if r, ok := resource.(CommentedYAML); ok {
		if prepend, ok := r.Comment(); ok {
			return commentYaml(prepend, y)
		}
	}
	return y
}

func commentYaml(prepend, y string) string {
	out := bytes.NewBuffer(nil)
	if len(prepend) > 0 {
		fmt.Fprintln(out, "#")
		scanner := bufio.NewScanner(bytes.NewBufferString(prepend))
		for scanner.Scan() {
			fmt.Fprintf(out, "# %s\n", scanner.Text())
		}
		fmt.Fprintln(out, "#")
	}
	scanner := bufio.NewScanner(bytes.NewBufferString(y))
	for scanner.Scan() {
		fmt.Fprintf(out, "# %s\n", scanner.Text())
	}
	return out.String()
}
