package vm

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"

	"github.com/opnlaas/opnlaas/config"

	"github.com/luthermonson/go-proxmox"
)

type ProxmoxAPI struct {
	createLock  sync.Mutex
	client      *proxmox.Client
	bg          context.Context
	Nodes       []*proxmox.Node
	Cluster     *proxmox.Cluster
	nodeRotator int
}

type ProxmoxAPICreateResult struct {
	Container *proxmox.Container
	CTID      int
}

type ProxmoxAPIBulkCreateResult struct {
	Result *ProxmoxAPICreateResult
	Error  error
}

func InitProxmox() (api *ProxmoxAPI, err error) {
	api = &ProxmoxAPI{
		client: proxmox.NewClient(
			fmt.Sprintf("https://%s:%s/api2/json", config.Config.Proxmox.Host, config.Config.Proxmox.Port),
			proxmox.WithHTTPClient(&http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
					},
				},
			}),
			proxmox.WithAPIToken(config.Config.Proxmox.TokenID, config.Config.Proxmox.Secret),
		),
		bg:    context.Background(),
		Nodes: make([]*proxmox.Node, 0),
	}

	if api.Cluster, err = api.client.Cluster(api.bg); err != nil {
		err = fmt.Errorf("failed to get cluster info: %w", err)
		return
	}

	var nodeStatuses []*proxmox.NodeStatus
	if nodeStatuses, err = api.client.Nodes(api.bg); err != nil {
		err = fmt.Errorf("failed to get node statuses: %w", err)
		return
	}

	for _, ns := range nodeStatuses {
		if ns.Status == "online" {
			var node *proxmox.Node
			if node, err = api.client.Node(api.bg, ns.Node); err != nil {
				err = fmt.Errorf("failed to get node info for node %s: %w", ns.Node, err)
				return
			}

			api.Nodes = append(api.Nodes, node)
		}
	}

	return
}

func (api *ProxmoxAPI) NextNode() *proxmox.Node {
	if len(api.Nodes) == 0 {
		return nil
	}

	api.nodeRotator = (api.nodeRotator + 1) % len(api.Nodes)
	return api.Nodes[api.nodeRotator]
}
