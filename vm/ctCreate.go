package vm

import (
	"fmt"
	"sync"
	"time"

	"github.com/luthermonson/go-proxmox"
)

func (api *ProxmoxAPI) CreateContainer(node *proxmox.Node, conf *ContainerCreateOptions) (result *ProxmoxAPICreateResult, err error) {
	api.createLock.Lock()
	defer api.createLock.Unlock()

	result = &ProxmoxAPICreateResult{}

	if result.CTID, err = api.Cluster.NextID(api.bg); err != nil {
		err = fmt.Errorf("failed to get next container ID: %w", err)
		return
	}

	var task *proxmox.Task
	if task, err = node.NewContainer(api.bg, result.CTID, conf.GoProxmoxOptions()...); err != nil {
		err = fmt.Errorf("failed to create container: %w", err)
		return
	}

	if err = task.Wait(api.bg, time.Second, time.Minute*5); err != nil {
		err = fmt.Errorf("failed to wait for container creation task: %w", err)
		return
	}

	if result.Container, err = node.Container(api.bg, result.CTID); err != nil {
		err = fmt.Errorf("failed to get created container info: %w", err)
		return
	}

	return
}

func (api *ProxmoxAPI) BulkCreateContainers(nodes []*proxmox.Node, confs []*ContainerCreateOptions) (results []*ProxmoxAPIBulkCreateResult) {
	api.createLock.Lock()
	defer api.createLock.Unlock()

	results = make([]*ProxmoxAPIBulkCreateResult, len(confs))

	for i := range confs {
		results[i] = &ProxmoxAPIBulkCreateResult{}
		results[i].Result = &ProxmoxAPICreateResult{}

		if results[i].Result.CTID, results[i].Error = api.Cluster.NextID(api.bg); results[i].Error != nil {
			results[i].Error = fmt.Errorf("failed to get next container ID: %w", results[i].Error)
			continue
		}

		var task *proxmox.Task
		if task, results[i].Error = nodes[i%len(nodes)].NewContainer(api.bg, results[i].Result.CTID, confs[i].GoProxmoxOptions()...); results[i].Error != nil {
			results[i].Error = fmt.Errorf("failed to create container: %w", results[i].Error)
			continue
		}

		if results[i].Error = task.Wait(api.bg, time.Second, time.Minute*5); results[i].Error != nil {
			results[i].Error = fmt.Errorf("failed to wait for container creation task: %w", results[i].Error)
			continue
		}

		if results[i].Result.Container, results[i].Error = nodes[i%len(nodes)].Container(api.bg, results[i].Result.CTID); results[i].Error != nil {
			results[i].Error = fmt.Errorf("failed to get created container info: %w", results[i].Error)
			continue
		}
	}

	return
}

func (api *ProxmoxAPI) BulkCreateContainersConcurrent(nodes []*proxmox.Node, confs []*ContainerCreateOptions, numCreateRetries int) (results []*ProxmoxAPIBulkCreateResult, err error) {
	api.createLock.Lock()
	defer api.createLock.Unlock()

	results = make([]*ProxmoxAPIBulkCreateResult, len(confs))

	var (
		potentialIDs []int = make([]int, len(confs))
		bumpID       int
	)

	if bumpID, err = api.Cluster.NextID(api.bg); err != nil {
		// err = fmt.Errorf("failed to get next container ID: %w", err)
		return
	}

	for i := range confs {
		for {
			var idIsFree bool
			if idIsFree, err = api.Cluster.CheckID(api.bg, bumpID); err != nil {
				// err = fmt.Errorf("failed to check container ID %d: %w", bumpID, err)
				return
			}

			if idIsFree {
				potentialIDs[i] = bumpID
				bumpID++
				break
			}

			bumpID++
		}
	}

	for i := range confs {
		results[i] = &ProxmoxAPIBulkCreateResult{}
		results[i].Result = &ProxmoxAPICreateResult{}
		results[i].Result.CTID = potentialIDs[i]
	}

	var (
		wg sync.WaitGroup
	)

	for i := range confs {
		wg.Add(1)

		go func(index int) {
			defer wg.Done()

			for i := range numCreateRetries + 1 {
				if i > 0 {
					// It's a retry, sleep a bit and then retry with a new CTID
					time.Sleep(time.Second * (time.Duration(i) + 1))

					// fmt.Printf("Retrying creation of container (attempt %d/%d)\n", i, numCreateRetries)

					var (
						bumpID   int
						idIsFree bool
					)

					if bumpID, results[index].Error = api.Cluster.NextID(api.bg); results[index].Error != nil {
						// results[index].Error = fmt.Errorf("failed to get next container ID: %w", results[index].Error)
						return
					}

					for {
						if idIsFree, results[index].Error = api.Cluster.CheckID(api.bg, bumpID); results[index].Error != nil {
							// results[index].Error = fmt.Errorf("failed to check container ID %d: %w", bumpID, results[index].Error)
							return
						}

						if idIsFree {
							results[index].Result.CTID = bumpID
							break
						}

						bumpID++
					}
				}

				var task *proxmox.Task
				if task, results[index].Error = nodes[index%len(nodes)].NewContainer(api.bg, results[index].Result.CTID, confs[index].GoProxmoxOptions()...); results[index].Error != nil {
					// results[index].Error = fmt.Errorf("failed to create container: %w", results[index].Error)
					continue
				}

				if results[index].Error = task.Wait(api.bg, time.Second, time.Minute*5); results[index].Error != nil {
					// results[index].Error = fmt.Errorf("failed to wait for container creation task: %w", results[index].Error)
					continue
				}

				if results[index].Result.Container, results[index].Error = nodes[index%len(nodes)].Container(api.bg, results[index].Result.CTID); results[index].Error != nil {
					// results[index].Error = fmt.Errorf("failed to get created container info: %w", results[index].Error)
					continue
				}

				results[index].Error = nil
				break
			}
		}(i)
	}

	wg.Wait()

	return
}
