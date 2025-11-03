package tests

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/db"
	"github.com/opnlaas/opnlaas/iso"
)

func TestISO(t *testing.T) {

	
	setup(t)
	defer cleanup(t)

	// for all ISOs in the test ISO directory, attempt to extract them and verify expected results
	if things, err := os.ReadDir(config.Config.ISOs.SearchDir); err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	} else {
		type result struct {
			pass   bool
			err    error
			parsed *db.StoredISOImage
			path   string
		}

		var (
			wg      sync.WaitGroup
			results []*result
		)

		for _, thing := range things {
			if thing.IsDir() {
				continue
			}
			name := thing.Name()
			if len(name) < 5 || name[len(name)-4:] != ".iso" {
				continue
			}
			wg.Add(1)
			go func(name string) {
				defer wg.Done()
				isoPath := fmt.Sprintf("%s/%s", config.Config.ISOs.SearchDir, name)
				destPath := fmt.Sprintf("%s/%s", config.Config.ISOs.StorageDir, name[:len(name)-4])
				if k, err := iso.ExtractISO(isoPath, destPath); err != nil {
					results = append(results, &result{pass: false, err: err, path: isoPath})
				} else {
					results = append(results, &result{pass: true, parsed: k, path: isoPath})
				}
			}(name)
		}

		wg.Wait()
		var (
			passed, failed int
			debug          string
		)

		for _, res := range results {
			if res.pass {
				passed++
				debug = fmt.Sprintf("- %s\n%s", fmt.Sprintf(
					"Name=%s Distro=%s Version=%s DistroType=%s Arch=%s PreConfigure=%s Full ISO Path=%s Kernel Path=%s Initrd Path=%s",
					res.parsed.Name, res.parsed.DistroName, res.parsed.Version, res.parsed.DistroType, res.parsed.Architecture, res.parsed.PreConfigure,
					res.parsed.FullISOPath, res.parsed.KernelPath, res.parsed.InitrdPath,
				), debug)
			} else {
				failed++
				t.Errorf("ISO extraction failed for %s: %v", res.path, res.err)
			}
		}

		t.Logf("ISO extraction test completed: %d passed, %d failed", passed, failed)
		t.Logf("Details:\n%s", debug)
	}
}
