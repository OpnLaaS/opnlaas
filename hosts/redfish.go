package hosts

import (
	"github.com/opnlaas/laas/config"
	"github.com/stmcginnis/gofish"
)

func RedfishStatus(host *Host) (status string, err error) {
	var (
		client *gofish.APIClient
	)

	if client, err = gofish.Connect(gofish.ClientConfig{
		Endpoint: "https://" + host.ManagementIP,
		Username: config.Config.Management.DefaultIPMIUser,
		Password: config.Config.Management.DefaultIPMIPass,
		Insecure: true,
	}); err != nil {
		return
	}

	defer client.Logout()

	service := client.Service
	chassis, err := service.Chassis()
	if err != nil {
		panic(err)
	}

	status = string(chassis[0].PowerState)
	return
}
