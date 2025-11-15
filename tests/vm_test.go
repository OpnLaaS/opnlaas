package tests

import (
	"testing"

	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/vm"
)

func TestContainerLifecycle(t *testing.T) {
	setup(t)
	defer cleanup(t)

	if !config.Config.Proxmox.Enabled {
		t.Skip("Proxmox integration not enabled; skipping test")
	}

	if !config.Config.Proxmox.Testing.Enabled {
		t.Skip("Proxmox testing not enabled; skipping test")
	}

	var err error

	var api *vm.ProxmoxAPI
	if api, err = vm.InitProxmox(); err != nil {
		t.Fatalf("failed to initialize Proxmox API: %v\n", err)
		return
	}

	if len(api.Nodes) == 0 {
		t.Fatalf("no online Proxmox nodes found")
		return
	}

	var node = api.Nodes[0]

	var conf = &vm.ContainerCreateOptions{
		TemplatePath:     config.Config.Proxmox.Testing.UbuntuTemplate,
		StoragePool:      config.Config.Proxmox.Testing.Storage,
		Hostname:         "opnlaas-test-ct",
		RootPassword:     "P@ssw0rd!",
		RootSSHPublicKey: "",
		StorageSizeGB:    8,
		MemoryMB:         512,
		Cores:            1,
		GatewayIPv4:      "10.0.0.1",
		IPv4Address:      "10.255.255.88",
		CIDRBlock:        8,
		NameServer:       "10.0.0.2",
		SearchDomain:     "cyber.lab",
	}

	var result *vm.ProxmoxAPICreateResult

	if result, err = api.CreateContainer(node, conf); err != nil {
		t.Fatalf("failed to create container: %v\n", err)
		return
	}

	t.Logf("created container with ID %d\n", result.CTID)

	if err = api.StartContainer(result.Container); err != nil {
		t.Errorf("failed to start container: %v\n", err)
	} else {
		t.Logf("started container with ID %d\n", result.CTID)
	}

	if err = api.StopContainer(result.Container); err != nil {
		t.Errorf("failed to stop container: %v\n", err)
	} else {
		t.Logf("stopped container with ID %d\n", result.CTID)
	}

	if err = api.DeleteContainer(result.Container); err != nil {
		t.Errorf("failed to delete container: %v\n", err)
	} else {
		t.Logf("deleted container with ID %d\n", result.CTID)
	}
}
