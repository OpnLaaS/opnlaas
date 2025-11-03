package tests

import (
	"encoding/json"
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

	// for all /home/egp1042/Documents/ISOS/*.iso files, extract them to ./<name>
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
		for _, res := range results {
			if !res.pass {
				t.Errorf("ISO extraction failed for %s: %v", res.path, res.err)
			} else if res.parsed == nil {
				t.Errorf("ISO extraction returned no parsed data for %s", res.path)
			} else {
				var marshalled []byte
				if marshalled, err = json.Marshal(res.parsed); err != nil {
					t.Errorf("ISO extraction returned unparsable data for %s: %v", res.path, err)
				} else {
					t.Logf("ISO extraction succeeded for %s: %s", res.path, string(marshalled))
				}
			}
		}
	}
}
