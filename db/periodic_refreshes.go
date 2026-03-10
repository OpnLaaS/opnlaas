package db

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2/log"
)

const (
	hostPowerRefreshInterval          = 5 * time.Minute
	hostSystemInfoRefreshEveryNCycles = 36 // 36 * 5m = 3 hours
)

func periodicHostPowerRefresh(cycle uint64) (err error) {
	var hosts []*Host

	if hosts, err = Hosts.SelectAll(); err != nil {
		return
	}

	shouldRefreshSystemInfo := cycle%hostSystemInfoRefreshEveryNCycles == 0

	var wg sync.WaitGroup
	for _, host := range hosts {
		wg.Add(1)
		go func(h *Host) {
			defer wg.Done()
			var (
				err     error
				changed bool
			)

			if h.Management, err = NewHostManagementClient(h); err != nil {
				log.Errorf("failed to create management client for host %s: %v", h.ManagementIP, err)
				return
			}

			defer h.Management.Close()

			if shouldRefreshSystemInfo {
				if err = h.Management.UpdateSystemInfo(); err != nil {
					log.Warnf("failed to refresh system info for host %s: %v", h.ManagementIP, err)
				} else {
					changed = true
				}
			}

			if h.LastKnownPowerState, err = h.Management.PowerState(true); err != nil {
				log.Errorf("failed to get power state for host %s: %v", h.ManagementIP, err)
			} else {
				h.LastKnownPowerStateTime = time.Now()
				changed = true
			}

			if !changed {
				return
			}
			if err = Hosts.Update(h); err != nil {
				log.Errorf("failed to update host %s in database: %v", h.ManagementIP, err)
				return
			}
		}(host)
	}

	wg.Wait()

	return
}

func BeginPeriodicRefreshes() (err error) {
	go func() {
		var cycle uint64 = 0
		for {
			if err = periodicHostPowerRefresh(cycle); err != nil {
				log.Errorf("error during periodic host power refresh: %v", err)
			}

			cycle += 1
			time.Sleep(hostPowerRefreshInterval)
		}
	}()

	return
}
