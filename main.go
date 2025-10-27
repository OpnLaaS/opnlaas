package main

import (
	"github.com/opnlaas/laas/config"
	"github.com/opnlaas/laas/hosts"
	"github.com/z46-dev/go-logger"
)

var log *logger.Logger

func init() {
	log = logger.NewLogger().SetPrefix("[MAIN]", logger.BoldPurple)

	var err error
	if err = config.InitEnv(".env"); err != nil {
		log.Errorf("Failed to initialize environment: %v\n", err)
		panic(err)
	}
}

func main() {
	var (
		err  error
		host *hosts.Host = &hosts.Host{
			ManagementIP:   "10.0.2.4",
			Vendor:         hosts.VendorDELL,
			FormFactor:     hosts.FormFactorRackmount,
			ManagementType: hosts.ManagementTypeRedfish,
			Model:          "PowerEdge R740xd",
		}
	)

	if host.Management, err = hosts.NewHostManagementClient(host); err != nil {
		log.Errorf("Failed to create host management client: %v\n", err)
		return
	}

	defer host.Management.Close()

	var value hosts.PowerState
	if value, err = host.Management.PowerState(); err != nil {
		log.Errorf("Failed to get power state: %v\n", err)
	} else {
		log.Statusf("Power State: %s\n", value)
	}

	if err = host.Management.UpdateSystemInfo(); err != nil {
		log.Errorf("Failed to update system info: %v\n", err)
	}

	log.Statusf("Host Specs: %+v\n", host.Specs)
}
