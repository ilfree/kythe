// Binary gotool extracts Kythe compilation information for Go packages named
// by import path on the command line.  The output compilations are written
// into an index pack directory.
package main

import (
	"flag"
	"fmt"
	"go/build"
	"log"
	"os"
	"path/filepath"
	"strings"

	"kythe/go/extractors/golang"
	"kythe/go/platform/indexpack"

	"golang.org/x/net/context"

	apb "kythe/proto/analysis_proto"
)

var (
	bc = build.Default // A shallow copy of the default build settings

	corpus      = flag.String("corpus", "", "Default corpus name to use")
	installPath = flag.String("install_path", "", "Directory where the build system stores outputs")
	localPath   = flag.String("local_path", "", "Directory where relative imports are resolved")
	outputDir   = flag.String("output_dir", "", "Directory where output should be written")
	byDir       = flag.Bool("bydir", false, "Import by directory rather than import path")
	keepGoing   = flag.Bool("continue", false, "Continue past errors")
	verbose     = flag.Bool("v", false, "Enable verbose logging")
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <import-path>...\n", filepath.Base(os.Args[0]))
		fmt.Fprintln(os.Stderr, `
Extract Kythe compilation records from Go import paths specified on the command line.
Outputs are written to an index pack directory.

Options:`)
		flag.PrintDefaults()
	}

	// Attach flags to the various parts of the go/build context we are using.
	// These will override the system defaults from the environment.
	flag.StringVar(&bc.GOARCH, "goarch", bc.GOARCH, "Go system architecture tag")
	flag.StringVar(&bc.GOOS, "goos", bc.GOOS, "Go operating system tag")
	flag.StringVar(&bc.GOPATH, "gopath", bc.GOPATH, "Go library path")
	flag.StringVar(&bc.GOROOT, "goroot", bc.GOROOT, "Go system root")
	flag.BoolVar(&bc.CgoEnabled, "gocgo", bc.CgoEnabled, "Whether to allow cgo")
	flag.StringVar(&bc.Compiler, "gocompiler", bc.Compiler, "Which Go compiler to use")

	// TODO(fromberger): Attach flags to the build and release tags (maybe).
}

func maybeFatal(msg string, args ...interface{}) {
	log.Printf(msg, args...)
	if !*keepGoing {
		os.Exit(1)
	}
}

func maybeLog(msg string, args ...interface{}) {
	if *verbose {
		log.Printf(msg, args...)
	}
}

func main() {
	flag.Parse()

	if *outputDir == "" {
		log.Fatal("You must provide a non-empty --output_dir")
	}

	ext := golang.Extractor{
		BuildContext:   bc,
		Corpus:         *corpus,
		LocalPath:      *localPath,
		AltInstallPath: *installPath,
	}
	locate := ext.Locate
	if *byDir {
		locate = ext.ImportDir
	}
	for _, path := range flag.Args() {
		pkg, err := locate(path)
		if err == nil {
			maybeLog("Found %q in %s", pkg.Path, pkg.BuildPackage.Dir)
		} else {
			maybeFatal("Error locating %q: %v", path, err)
		}
	}

	if err := ext.Extract(); err != nil {
		maybeFatal("Error in extraction: %v", err)
	}

	pack, err := indexpack.CreateOrOpen(context.Background(), *outputDir,
		indexpack.UnitType((*apb.CompilationUnit)(nil)))
	if err != nil {
		log.Fatalf("Unable to open %q: %v", *outputDir, err)
	}

	maybeLog("Writing %d package(s) to %q", len(ext.Packages), pack.Root())
	for _, pkg := range ext.Packages {
		maybeLog("Package %q:\n\t// %s", pkg.Path, pkg.BuildPackage.Doc)
		uf, err := pkg.Store(context.Background(), pack)
		if err != nil {
			maybeFatal("Error writing %q: %v", pkg.Path, err)
		} else {
			maybeLog("Output: %v\n", strings.Join(uf, ", "))
		}
	}
}
