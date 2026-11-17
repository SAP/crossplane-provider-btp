// Reproducer for the daemon.Image() / docker save bug seen in upgrade tests on ubuntu-26.04 / Docker 29.4.2.
// Replicates xp-testing's extractPackageYamlFromImage path exactly.
package main

import (
	"archive/tar"
	"fmt"
	"io"
	"os"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: main.go <image-ref>")
		os.Exit(1)
	}
	ref, err := name.ParseReference(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse ref: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("calling daemon.Image()...")
	img, err := daemon.Image(ref)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon.Image() FAILED: %v\n", err)
		os.Exit(1)
	}

	sz, err := img.Size()
	if err != nil {
		fmt.Fprintf(os.Stderr, "img.Size() FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("img.Size() = %d\n", sz)

	tarc := mutate.Extract(img)
	t := tar.NewReader(tarc)
	var read int
	for {
		h, err := t.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "tar.Next() FAILED after %d entries: %v\n", read, err)
			os.Exit(1)
		}
		if h.Name == "package.yaml" {
			fmt.Printf("found package.yaml after %d entries — SUCCESS\n", read)
			os.Exit(0)
		}
		read++
	}
	fmt.Fprintf(os.Stderr, "package.yaml not found after %d entries\n", read)
	os.Exit(1)
}
