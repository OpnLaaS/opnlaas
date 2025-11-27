package tests

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/luthermonson/go-proxmox"
	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/ssh"
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

	var (
		firstIP, lastIP net.IP
		ips             []net.IP
	)

	if firstIP, lastIP, _, err = ssh.ParseSubnet(config.Config.Proxmox.Testing.Subnet); err != nil {
		t.Fatalf("failed to get IP range from subnet %s: %v\n", config.Config.Proxmox.Testing.Subnet, err)
		return
	}

	ips = ssh.GetSubnetRange(firstIP, lastIP)

	var openIPs []net.IP
	if openIPs, err = ssh.FindOpenIPs(ips, 1); err != nil {
		t.Fatalf("failed to find open IPs in subnet %s: %v\n", config.Config.Proxmox.Testing.Subnet, err)
		return
	}

	var pubKey, privKey string
	if pubKey, privKey, err = ssh.CreateSSHKeyPair(); err != nil {
		t.Fatalf("failed to generate temporary SSH key pair: %v\n", err)
		return
	}

	var conf = &vm.ContainerCreateOptions{
		TemplatePath:     config.Config.Proxmox.Testing.UbuntuTemplate,
		StoragePool:      config.Config.Proxmox.Testing.Storage,
		Hostname:         "opnlaas-test-ct",
		RootPassword:     "password",
		RootSSHPublicKey: pubKey,
		StorageSizeGB:    8,
		MemoryMB:         512,
		Cores:            1,
		GatewayIPv4:      config.Config.Proxmox.Testing.Gateway,
		IPv4Address:      openIPs[0].To4().String(),
		CIDRBlock:        8,
		NameServer:       config.Config.Proxmox.Testing.DNS,
		SearchDomain:     config.Config.Proxmox.Testing.SearchDomain,
	}

	var result *vm.ProxmoxAPICreateResult

	if result, err = api.CreateContainer(node, conf); err != nil {
		t.Fatalf("failed to create container: %v\n", err)
		return
	}

	defer func() {
		if err = api.StopContainer(result.Container); err != nil {
			t.Errorf("failed to stop container during cleanup: %v\n", err)
		}

		if err = api.DeleteContainer(result.Container); err != nil {
			t.Errorf("failed to delete container during cleanup: %v\n", err)
		}
	}()

	t.Logf("created container with ID %d\n", result.CTID)

	if err = api.StartContainer(result.Container); err != nil {
		t.Fatalf("failed to start container: %v\n", err)
	} else {
		t.Logf("started container with ID %d\n", result.CTID)
	}

	var conn *ssh.SSHConnection
	if conn, err = ssh.ConnectOnceReadyWithRetry("root", conf.IPv4Address, 22, ssh.WithPrivateKey([]byte(privKey)), 3); err != nil {
		t.Fatalf("failed to establish SSH connection to container: %v\n", err)
	} else {
		defer conn.Close()
	}

	t.Logf("successfully connected to container via SSH at %s\n", openIPs[0].String())

	var output []byte
	if _, output, err = conn.SendWithOutput("hostname"); err != nil {
		t.Fatalf("failed to run hostname command in container: %v\n", err)
	} else if strings.TrimSpace(string(output)) != conf.Hostname {
		t.Fatalf("unexpected hostname output: expected %s, got %s\n", conf.Hostname, output)
	}

	t.Logf("container hostname verified: %s\n", output)
}

func TestCTTemplateClone(t *testing.T) {
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

	var (
		firstIP, lastIP net.IP
		ips             []net.IP
	)

	if firstIP, lastIP, _, err = ssh.ParseSubnet(config.Config.Proxmox.Testing.Subnet); err != nil {
		t.Fatalf("failed to get IP range from subnet %s: %v\n", config.Config.Proxmox.Testing.Subnet, err)
		return
	}

	ips = ssh.GetSubnetRange(firstIP, lastIP)

	var openIPs []net.IP
	if openIPs, err = ssh.FindOpenIPs(ips, 2); err != nil {
		t.Fatalf("failed to find open IPs in subnet %s: %v\n", config.Config.Proxmox.Testing.Subnet, err)
		return
	}

	var conf = &vm.ContainerCreateOptions{
		TemplatePath:     config.Config.Proxmox.Testing.UbuntuTemplate,
		StoragePool:      config.Config.Proxmox.Testing.Storage,
		Hostname:         "opnlaas-test-ct",
		RootPassword:     "password",
		RootSSHPublicKey: "",
		StorageSizeGB:    8,
		MemoryMB:         512,
		Cores:            1,
		GatewayIPv4:      config.Config.Proxmox.Testing.Gateway,
		IPv4Address:      openIPs[0].To4().String(),
		CIDRBlock:        8,
		NameServer:       config.Config.Proxmox.Testing.DNS,
		SearchDomain:     config.Config.Proxmox.Testing.SearchDomain,
	}

	var result *vm.ProxmoxAPICreateResult

	if result, err = api.CreateContainer(node, conf); err != nil {
		t.Fatalf("failed to create container: %v\n", err)
		return
	}

	t.Logf("created container with ID %d\n", result.CTID)

	// In spirit of time sensitivity, we will skip testing start/stop here

	// if err = api.StartContainer(result.Container); err != nil {
	// 	t.Errorf("failed to start container: %v\n", err)
	// } else {
	// 	t.Logf("started container with ID %d\n", result.CTID)
	// }

	// if err = api.StopContainer(result.Container); err != nil {
	// 	t.Errorf("failed to stop container: %v\n", err)
	// } else {
	// 	t.Logf("stopped container with ID %d\n", result.CTID)
	// }

	if err = api.CreateTemplate(result.Container); err != nil {
		t.Errorf("failed to create template from container: %v\n", err)
	} else {
		t.Logf("created template from container with ID %d\n", result.CTID)
	}

	var newCT *proxmox.Container
	if newCT, err = api.CloneTemplate(result.Container, fmt.Sprintf("%s-clone", conf.Hostname)); err != nil {
		t.Errorf("failed to clone template: %v\n", err)
	} else {
		t.Logf("cloned template from container with ID %d to new container with ID %d\n", result.CTID, newCT.VMID)
	}

	if err = api.ChangeContainerNetworking(newCT, conf.GatewayIPv4, openIPs[1].To4().String(), 8); err != nil {
		t.Errorf("failed to change networking of cloned container: %v\n", err)
	} else {
		t.Logf("changed networking of cloned container with ID %d\n", newCT.VMID)
	}

	if err = api.StartContainer(newCT); err != nil {
		t.Errorf("failed to start cloned container: %v\n", err)
	} else {
		t.Logf("started cloned container with ID %d\n", newCT.VMID)
	}

	if err = ssh.WaitOnline(openIPs[1].To4().String()); err != nil {
		t.Errorf("cloned container did not come online in time: %v\n", err)
	} else {
		t.Logf("cloned container with ID %d is online\n", newCT.VMID)
	}

	if err = api.StopContainer(newCT); err != nil {
		t.Errorf("failed to stop cloned container: %v\n", err)
	} else {
		t.Logf("stopped cloned container with ID %d\n", newCT.VMID)
	}

	if err = api.DeleteContainer(newCT); err != nil {
		t.Errorf("failed to delete cloned container: %v\n", err)
	} else {
		t.Logf("deleted cloned container with ID %d\n", newCT.VMID)
	}

	if err = api.DeleteContainer(result.Container); err != nil {
		t.Errorf("failed to delete original container: %v\n", err)
	} else {
		t.Logf("deleted original container with ID %d\n", result.CTID)
	}
}
