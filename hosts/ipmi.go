package hosts

import (
	"context"

	"github.com/bougou/go-ipmi"
	"github.com/opnlaas/laas/config"
)

func IPMIStatus(host *Host) (status string, err error) {
	var (
		client *ipmi.Client
		resp   *ipmi.GetChassisStatusResponse
	)

	if client, err = ipmi.NewClient(host.ManagementIP, 623, config.Config.Management.DefaultIPMIUser, config.Config.Management.DefaultIPMIPass); err != nil {
		return
	}

	if err = client.Connect(context.Background()); err != nil {
		return
	}

	if resp, err = client.GetChassisStatus(context.Background()); err != nil {
		return
	}

	status = "Power Off"
	if resp.PowerIsOn {
		status = "Power On"
	}

	return
}
