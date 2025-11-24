package db

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2/log"
)

func periodicHostPowerRefresh() (err error) {
	var hosts []*Host

	if hosts, err = Hosts.SelectAll(); err != nil {
		return
	}

	var wg sync.WaitGroup
	for _, host := range hosts {
		wg.Add(1)
		go func(h *Host) {
			defer wg.Done()
			var err error

			if h.Management, err = NewHostManagementClient(h); err != nil {
				log.Errorf("failed to create management client for host %s: %v", h.ManagementIP, err)
				return
			}

			defer h.Management.Close()

			if h.LastKnownPowerState, err = h.Management.PowerState(true); err != nil {
				log.Errorf("failed to get power state for host %s: %v", h.ManagementIP, err)
				return
			}

			h.LastKnownPowerStateTime = time.Now()

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
		for {
			if err = periodicHostPowerRefresh(); err != nil {
				log.Errorf("error during periodic host power refresh: %v", err)
			}

			time.Sleep(5 * time.Minute)
		}
	}()

	return
}
