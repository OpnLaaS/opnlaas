package tests

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/iso"
)

func TestISO(t *testing.T) {
	setup(t)
	defer cleanup(t)

	// for all /home/egp1042/Documents/ISOS/*.iso files, extract them to ./<name>
	if things, err := os.ReadDir("/home/egp1042/Documents/ISOS/"); err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	} else {
		var wg sync.WaitGroup
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
				isoPath := "/home/egp1042/Documents/ISOS/" + name
				destPath := config.Config.ISOs.SearchDir + "/" + name[:len(name)-4]
				if k, err := iso.ExtractISO(isoPath, destPath); err != nil {
					t.Errorf("FAIL %s: %v", isoPath, err)
				} else {
					fmt.Printf("%s: %+v\n", isoPath, k)
				}
			}(name)
		}

		wg.Wait()
	}
}
