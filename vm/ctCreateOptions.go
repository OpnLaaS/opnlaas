package vm

import (
	"fmt"

	"github.com/luthermonson/go-proxmox"
)

type ContainerCreateOptions struct {
	TemplatePath     string
	StoragePool      string
	Hostname         string
	RootPassword     string
	RootSSHPublicKey string
	StorageSizeGB    int
	MemoryMB         int
	Cores            int
	GatewayIPv4      string
	IPv4Address      string
	CIDRBlock        int
	NameServer       string
	SearchDomain     string
}

func (c *ContainerCreateOptions) GoProxmoxOptions() (opts []proxmox.ContainerOption) {
	opts = append(opts, proxmox.ContainerOption{
		Name:  "ostemplate",
		Value: c.TemplatePath,
	})

	opts = append(opts, proxmox.ContainerOption{
		Name:  "storage",
		Value: c.StoragePool,
	})

	opts = append(opts, proxmox.ContainerOption{
		Name:  "hostname",
		Value: c.Hostname,
	})

	opts = append(opts, proxmox.ContainerOption{
		Name:  "password",
		Value: c.RootPassword,
	})

	if c.RootSSHPublicKey != "" {
		opts = append(opts, proxmox.ContainerOption{
			Name:  "ssh-public-keys",
			Value: c.RootSSHPublicKey,
		})
	}

	opts = append(opts, proxmox.ContainerOption{
		Name:  "rootfs",
		Value: fmt.Sprintf("volume=%s:%d", c.StoragePool, c.StorageSizeGB),
	})

	opts = append(opts, proxmox.ContainerOption{
		Name:  "memory",
		Value: c.MemoryMB,
	})

	opts = append(opts, proxmox.ContainerOption{
		Name:  "cores",
		Value: c.Cores,
	})

	opts = append(opts, proxmox.ContainerOption{
		Name:  "net0",
		Value: fmt.Sprintf("name=eth0,bridge=vmbr0,firewall=1,gw=%s,ip=%s/%d", c.GatewayIPv4, c.IPv4Address, c.CIDRBlock),
	})

	opts = append(opts, proxmox.ContainerOption{
		Name:  "nameserver",
		Value: c.NameServer,
	})

	if c.SearchDomain != "" {
		opts = append(opts, proxmox.ContainerOption{
			Name:  "searchdomain",
			Value: c.SearchDomain,
		})
	}

	opts = append(opts, proxmox.ContainerOption{
		Name:  "unprivileged",
		Value: true,
	})

	opts = append(opts, proxmox.ContainerOption{
		Name:  "features",
		Value: "nesting=1",
	})

	return
}
