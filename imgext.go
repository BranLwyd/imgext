package main

import (
	"flag"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	// The below blank includes are to allow support for various image file formats.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

var (
	dryRun      = flag.Bool("dry_run", false, "If set, do not rename files, just print what renames would occur.")
	concurrency = flag.Int("concurrency", 0, "The number of files to process at once. If unset, a reasonable value will be chosen automatically.")

	typeMap = map[string]string{
		"jpeg": "jpg",
	}
)

func main() {
	// Parse & validate flags.
	flag.Parse()
	if len(flag.Args()) == 0 {
		die("Usage: imgext [--dry_run] [--concurrency=N] globs")
	}
	switch {
	case *concurrency == 0:
		*concurrency = runtime.GOMAXPROCS(0)
	case *concurrency < 0:
		die("The --concurrency flag must be non-negative.")
	}

	// Start per-file workers.
	var wg sync.WaitGroup
	var errCount int64
	ch := make(chan string)
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for fn := range ch {
				if err := func() error {
					f, err := os.Open(fn)
					if err != nil {
						return fmt.Errorf("couldn't open: %w", err)
					}
					defer f.Close()
					_, typ, err := image.DecodeConfig(f)
					if err != nil {
						return fmt.Errorf("couldn't classify: %w", err)
					}
					if translatedTyp, ok := typeMap[typ]; ok {
						typ = translatedTyp
					}
					if err := f.Close(); err != nil {
						return fmt.Errorf("couldn't close: %w", err)
					}
					newFN := fmt.Sprintf("%s.%s", fn[:len(fn)-len(filepath.Ext(fn))], typ)
					if fn != newFN {
						fmt.Printf("%s -> %s\n", fn, newFN)
						if !*dryRun {
							if err := os.Rename(fn, newFN); err != nil {
								return fmt.Errorf("couldn't rename: %w", err)
							}
						}
					}
					return nil
				}(); err != nil {
					atomic.AddInt64(&errCount, 1)
					fmt.Fprintf(os.Stderr, "Couldn't handle %q: %v\n", fn, err)
				}
			}
		}()
	}

	// Find files to rename. (find all files before renaming anything to ensure we handle each file only once)
	files := map[string]struct{}{}
	for _, glob := range flag.Args() {
		fns, err := filepath.Glob(glob)
		if err != nil {
			die("Bad glob %q: %v", glob, err)
		}
		for _, fn := range fns {
			files[fn] = struct{}{}
		}
	}
	fmt.Printf("Renaming %d file(s)\n", len(files))
	for fn := range files {
		ch <- fn
	}
	close(ch)
	wg.Wait()
	if errCount > 0 {
		die("Encountered %d errors", errCount)
	}
}

func die(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
