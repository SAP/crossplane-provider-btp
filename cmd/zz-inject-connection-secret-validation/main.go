// Command zz-inject-connection-secret-validation patches the generated
// SubaccountApiCredential controller to validate its connection Secret after
// external observation.
//
// Upjet does not provide a public controller-template override hook. The
// resource-specific connector wrapper therefore has to be inserted after
// Upjet generates the controller. This command is invoked from apis/generate.go
// and is idempotent.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	controllerFileName = "zz_controller.go"
	resourcePackage    = "subaccountapicredential"
	connectorPrefix    = "managed.WithExternalConnecter("
	connectorCall      = "tjcontroller.NewConnector("
	validatorCall      = "NewConnectionSecretValidatingConnector("
)

func main() {
	root := flag.String("root", "internal/controller", "directory to scan for zz_controller.go files")
	flag.Parse()

	var patched, skipped int
	err := filepath.WalkDir(*root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Base(path) != controllerFileName || filepath.Base(filepath.Dir(path)) != resourcePackage {
			return nil
		}
		changed, perr := patchFile(path)
		if perr != nil {
			return fmt.Errorf("%s: %w", path, perr)
		}
		if changed {
			patched++
			fmt.Printf("patched %s\n", path)
		} else {
			skipped++
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "walk failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("connection-secret validation patcher: %d patched, %d already-patched\n", patched, skipped)
}

// patchFile wraps the generated Upjet connector in the resource-specific
// validator. It assumes the generated call has the form
// managed.WithExternalConnecter(tjcontroller.NewConnector(...)).
func patchFile(path string) (bool, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // path is filtered by WalkDir
	if err != nil {
		return false, err
	}
	if bytes.Contains(raw, []byte(validatorCall)) {
		return false, nil
	}

	outer := bytes.Index(raw, []byte(connectorPrefix+connectorCall))
	if outer == -1 {
		return false, nil
	}
	open := outer + len(connectorPrefix) + len("tjcontroller.NewConnector")
	close, err := matchingParen(raw, open)
	if err != nil {
		return false, err
	}

	var out bytes.Buffer
	out.Write(raw[:outer])
	out.WriteString(connectorPrefix)
	out.WriteString(validatorCall)
	out.WriteString("\n\t\t\t")
	out.Write(raw[outer+len(connectorPrefix) : close+1])
	out.WriteString(",\n\t\t\tmgr.GetClient(),\n\t\t)")
	out.Write(raw[close+1:])

	info, err := os.Stat(path)
	mode := os.FileMode(0o644)
	if err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(path, out.Bytes(), mode); err != nil {
		return false, err
	}
	return true, nil
}

func matchingParen(raw []byte, open int) (int, error) {
	if open < 0 || open >= len(raw) || raw[open] != '(' {
		return 0, fmt.Errorf("expected opening parenthesis at byte %d", open)
	}
	depth := 0
	for index := open; index < len(raw); index++ {
		switch raw[index] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index, nil
			}
		}
	}
	return 0, fmt.Errorf("unterminated connector call")
}
