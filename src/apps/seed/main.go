// Command seed adds a large demo workspace to otter.db, to check the tree and
// search stay fast (10 000 requests, 6 levels deep by default). Close Otter
// first.
//
//	cd src && go run ./apps/seed
//	cd src && go run ./apps/seed -requests 50000 -db /tmp/otter.db
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/acoradd/otter/src/internal/store"
)

func main() {
	defaultDB := ""
	if dir, err := os.UserConfigDir(); err == nil {
		defaultDB = filepath.Join(dir, "Otter", store.FileName)
	}
	path := flag.String("db", defaultDB, "path of otter.db")
	name := flag.String("name", "Large demo", "name of the demo workspace")
	size := store.LargeDemo
	flag.IntVar(&size.Requests, "requests", size.Requests, "number of requests")
	flag.IntVar(&size.Depth, "depth", size.Depth, "levels of folders")
	flag.IntVar(&size.Branching, "branching", size.Branching, "folders per folder")
	flag.Parse()

	ctx := context.Background()
	db, err := store.Open(ctx, *path)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	start := time.Now()
	if _, err := db.SeedDemo(ctx, *name, size); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%q: %d requests, %d levels, in %s (%s)\n", *name, size.Requests, size.Depth, time.Since(start).Round(time.Millisecond), *path)
}
