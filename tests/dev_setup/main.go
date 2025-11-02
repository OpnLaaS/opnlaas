package main

import (
	"fmt"
	"math/rand/v2"

	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/hosts"
)

func main() {
	if err := config.InitEnv(".env"); err != nil {
		panic(err)
	}

	if err := hosts.InitDB(); err != nil {
		panic(err)
	}

	type ModelType struct {
		Model      string
		Vendor     hosts.VendorID
		FormFactor hosts.FormFactor
	}

	var modelTypes = []ModelType{
		{
			Model:      "PowerEdge R640",
			Vendor:     hosts.VendorDELL,
			FormFactor: hosts.FormFactorRackmount,
		},
		{
			Model:      "ProLiant DL360 Gen10",
			Vendor:     hosts.VendorHPE,
			FormFactor: hosts.FormFactorRackmount,
		},
		{
			Model:      "ThinkSystem SR650",
			Vendor:     hosts.VendorLenovo,
			FormFactor: hosts.FormFactorRackmount,
		},
		{
			Model:      "PowerEdge M640 (VRTX)",
			Vendor:     hosts.VendorDELL,
			FormFactor: hosts.FormFactorBlade,
		},
		{
			Model:      "PowerEdge R730",
			Vendor:     hosts.VendorDELL,
			FormFactor: hosts.FormFactorRackmount,
		},
		{
			Model:      "ProLiant DL380 Gen9",
			Vendor:     hosts.VendorHPE,
			FormFactor: hosts.FormFactorRackmount,
		},
		{
			Model:      "ThinkSystem SR630",
			Vendor:     hosts.VendorLenovo,
			FormFactor: hosts.FormFactorRackmount,
		},
	}

	var processorChoices = []hosts.HostCPUSpecs{
		{
			Count:   2,
			Sku:     "Intel Xeon Silver 4214",
			Cores:   12,
			Threads: 24,
		},
		{
			Count:   2,
			Sku:     "Intel Xeon Gold 6230",
			Cores:   20,
			Threads: 40,
		},
		{
			Count:   2,
			Sku:     "AMD EPYC 7302P",
			Cores:   16,
			Threads: 32,
		},
		{
			Count:   1,
			Sku:     "AMD EPYC 7502P",
			Cores:   32,
			Threads: 64,
		},
	}

	var memoryChoices = []hosts.HostMemorySpecs{
		{
			SizeGB:   64,
			NumDIMMs: 4,
		},
		{
			SizeGB:   128,
			NumDIMMs: 8,
		},
		{
			SizeGB:   256,
			NumDIMMs: 16,
		},
		{
			SizeGB:   512,
			NumDIMMs: 32,
		},
	}

	var storageChoices = []hosts.HostStorageSpecs{
		{
			CapacityGB: 1024,
			MediaType:  "SAS",
		},
		{
			CapacityGB: 2048,
			MediaType:  "SATA",
		},
		{
			CapacityGB: 1024,
			MediaType:  "NVMe",
		},
		{
			CapacityGB: 4096,
			MediaType:  "SAS",
		},
	}

	// 1. Create a bunch of plausible hosts
	for i := range 16 {
		// hosts.Hosts.Insert(&hosts.Host{
		// 	ManagementIP:   fmt.Sprintf("10.0.1.%d", i+1),
		// 	Vendor:         hosts.VendorID(rand.IntN(int(hosts.VendorIntel)) + 1),
		// 	FormFactor:     hosts.FormFactor(rand.IntN(int(hosts.FormFactorMicroserver)) + 1),
		// 	ManagementType: hosts.ManagementType(rand.IntN(int(hosts.ManagementTypeRedfish)) + 1),
		// })

		var choice = modelTypes[rand.IntN(len(modelTypes))]

		if err := hosts.Hosts.Insert(&hosts.Host{
			ManagementIP:   fmt.Sprintf("10.0.1.%d", i+1),
			Model:          choice.Model,
			Vendor:         choice.Vendor,
			FormFactor:     choice.FormFactor,
			ManagementType: hosts.ManagementTypeRedfish,
			Specs: hosts.HostSpecs{
				Processor: processorChoices[rand.IntN(len(processorChoices))],
				Memory:    memoryChoices[rand.IntN(len(memoryChoices))],
				Storage: func() []hosts.HostStorageSpecs {
					var numDrives = rand.IntN(3) + 1
					var drives = make([]hosts.HostStorageSpecs, numDrives)
					for j := 0; j < numDrives; j++ {
						drives[j] = storageChoices[rand.IntN(len(storageChoices))]
					}
					return drives
				}(),
			},
		}); err != nil {
			panic(err)
		}
	}
}
