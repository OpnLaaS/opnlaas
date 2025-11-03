package iso

import (
	"fmt"
	"os"

	"github.com/kdomanski/iso9660"
)

func extractISO9660(sourceImage string) (err error) {
	f, err := os.Open(sourceImage)
	if err != nil {
		return err
	}
	defer f.Close()

	image, err := iso9660.OpenImage(f)
	if err != nil {
		return err
	}

	art, err := ScanISOForBootArtifactsISO9660(image)
	if err != nil {
		return err
	}

	fmt.Printf("[ISO] Distro=%s Arch=%s\n", art.DistroGuess, art.ArchGuess)
	fmt.Printf("[ISO] Kernel=%s\n", art.KernelPath)
	fmt.Printf("[ISO] Initrd=%s\n", art.InitrdPath)
	if len(art.ConfigPaths) > 0 {
		fmt.Printf("[ISO] Boot configs:\n")
		for _, c := range art.ConfigPaths {
			fmt.Printf("  - %s\n", c)
		}
	}
	if len(art.KernelArgs) > 0 {
		fmt.Printf("[ISO] Kernel args:\n")
		for _, arg := range art.KernelArgs {
			fmt.Printf("  - %s\n", arg)
		}
	}
	fmt.Printf("[ISO] Preconfigure: %s\n", art.Preconfigure)
	return nil
}
