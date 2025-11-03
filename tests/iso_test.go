package tests

import (
	"fmt"
	"os"
	"strings"
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
				debug = fmt.Sprintf("- %+v\n%s", res.parsed, debug)

				switch {
				case strings.Contains(strings.ToLower(res.path), "debian"), strings.Contains(strings.ToLower(res.path), "ubuntu"), strings.Contains(strings.ToLower(res.path), "kali"), strings.Contains(strings.ToLower(res.path), "mint"):
					if res.parsed.DistroType != db.DistroTypeDebianBased {
						t.Errorf("Expected %s distro type for %s, got %s", db.DistroTypeDebianBased, res.path, res.parsed.DistroType)
					}
				case strings.Contains(strings.ToLower(res.path), "centos"), strings.Contains(strings.ToLower(res.path), "rhel"), strings.Contains(strings.ToLower(res.path), "fedora"), strings.Contains(strings.ToLower(res.path), "rocky"), strings.Contains(strings.ToLower(res.path), "almalinux"):
					if res.parsed.DistroType != db.DistroTypeRedHatBased {
						t.Errorf("Expected %s distro type for %s, got %s", db.DistroTypeRedHatBased, res.path, res.parsed.DistroType)
					}
				case strings.Contains(strings.ToLower(res.path), "archlinux"), strings.Contains(strings.ToLower(res.path), "manjaro"):
					if res.parsed.DistroType != db.DistroTypeArchBased {
						t.Errorf("Expected %s distro type for %s, got %s", db.DistroTypeArchBased, res.path, res.parsed.DistroType)
					}
				case strings.Contains(strings.ToLower(res.path), "alpine"):
					if res.parsed.DistroType != db.DistroTypeAlpineBased {
						t.Errorf("Expected %s distro type for %s, got %s", db.DistroTypeAlpineBased, res.path, res.parsed.DistroType)
					}
				case strings.Contains(strings.ToLower(res.path), "leap"), strings.Contains(strings.ToLower(res.path), "tumbleweed"), strings.Contains(strings.ToLower(res.path), "opensuse"):
				}
			} else {
				failed++
				t.Errorf("ISO extraction failed for %s: %v", res.path, res.err)
			}
		}

		t.Logf("ISO extraction test completed: %d passed, %d failed", passed, failed)
		t.Logf("Details:\n%s", debug)
	}
}
