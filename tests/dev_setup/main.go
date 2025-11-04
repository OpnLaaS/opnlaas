package main

import (
	"fmt"
	"math/rand/v2"

	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/db"
)

func main() {
	if err := config.InitEnv(".env"); err != nil {
		panic(err)
	}

	if err := db.InitDB(); err != nil {
		panic(err)
	}

	type ModelType struct {
		Model      string
		Vendor     db.VendorID
		FormFactor db.FormFactor
	}

	var modelTypes = []ModelType{
		{
			Model:      "PowerEdge R640",
			Vendor:     db.VendorDELL,
			FormFactor: db.FormFactorRackmount,
		},
		{
			Model:      "ProLiant DL360 Gen10",
			Vendor:     db.VendorHPE,
			FormFactor: db.FormFactorRackmount,
		},
		{
			Model:      "ThinkSystem SR650",
			Vendor:     db.VendorLenovo,
			FormFactor: db.FormFactorRackmount,
		},
		{
			Model:      "PowerEdge M640 (VRTX)",
			Vendor:     db.VendorDELL,
			FormFactor: db.FormFactorBlade,
		},
		{
			Model:      "PowerEdge R730",
			Vendor:     db.VendorDELL,
			FormFactor: db.FormFactorRackmount,
		},
		{
			Model:      "ProLiant DL380 Gen9",
			Vendor:     db.VendorHPE,
			FormFactor: db.FormFactorRackmount,
		},
		{
			Model:      "ThinkSystem SR630",
			Vendor:     db.VendorLenovo,
			FormFactor: db.FormFactorRackmount,
		},
	}

	var processorChoices = []db.HostCPUSpecs{
		{
			Manufacturer: "Intel",
			Count:        2,
			Sku:          "Intel Xeon Silver 4214",
			Cores:        12,
			Threads:      24,
			BaseSpeedMHz: 2100,
			MaxSpeedMHz:  3000,
		},
		{
			Manufacturer: "Intel",
			Count:        2,
			Sku:          "Intel Xeon Gold 6230",
			Cores:        20,
			Threads:      40,
			BaseSpeedMHz: 2000,
			MaxSpeedMHz:  2700,
		},
		{
			Manufacturer: "AMD",
			Count:        2,
			Sku:          "AMD EPYC 7302P",
			Cores:        16,
			Threads:      32,
			BaseSpeedMHz: 3200,
			MaxSpeedMHz:  3500,
		},
		{
			Manufacturer: "AMD",
			Count:        1,
			Sku:          "AMD EPYC 7502P",
			Cores:        32,
			Threads:      64,
			BaseSpeedMHz: 2000,
			MaxSpeedMHz:  3500,
		},
	}

	var memoryChoices = []db.HostMemorySpecs{
		{
			SizeGB:   64,
			NumDIMMs: 4,
			SpeedMHz: 2666,
		},
		{
			SizeGB:   128,
			NumDIMMs: 8,
			SpeedMHz: 2933,
		},
		{
			SizeGB:   256,
			NumDIMMs: 16,
			SpeedMHz: 3200,
		},
		{
			SizeGB:   512,
			NumDIMMs: 32,
			SpeedMHz: 2933,
		},
	}

	var storageChoices = []db.HostStorageSpecs{
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
		// db.db.Insert(&db.Host{
		// 	ManagementIP:   fmt.Sprintf("10.0.1.%d", i+1),
		// 	Vendor:         db.VendorID(rand.IntN(int(db.VendorIntel)) + 1),
		// 	FormFactor:     db.FormFactor(rand.IntN(int(db.FormFactorMicroserver)) + 1),
		// 	ManagementType: db.ManagementType(rand.IntN(int(db.ManagementTypeRedfish)) + 1),
		// })

		var choice = modelTypes[rand.IntN(len(modelTypes))]

		if err := db.Hosts.Insert(&db.Host{
			ManagementIP:   fmt.Sprintf("10.0.1.%d", i+1),
			Model:          choice.Model,
			Vendor:         choice.Vendor,
			FormFactor:     choice.FormFactor,
			ManagementType: db.ManagementTypeRedfish,
			Specs: db.HostSpecs{
				Processor: processorChoices[rand.IntN(len(processorChoices))],
				Memory:    memoryChoices[rand.IntN(len(memoryChoices))],
				Storage: func() []db.HostStorageSpecs {
					var numDrives = rand.IntN(3) + 1
					var drives = make([]db.HostStorageSpecs, numDrives)
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
