package iso

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/z46-dev/go-logger"
)

type distroType int

const (
	UNKNOWN distroType = iota
	UBUNTU
	REDHAT
	SUSE
)

var omitDirectory string = ""

var mainLog *logger.Logger = logger.NewLogger().SetPrefix("[ISO PARSER]", logger.BoldWhite).IncludeTimestamp()

type PXEConfig struct {
	label, menuLabel, kernelPath, initrdPath                                   string
	useDHCP                                                                    bool
	boot, netboot, url, instStage2, instRepo, cloudInit, suseInstall, autoYaST *string
}

func (c *PXEConfig) String() string {
	c.kernelPath = strings.Replace(c.kernelPath, omitDirectory, "", 1)
	c.initrdPath = strings.Replace(c.initrdPath, omitDirectory, "", 1)

	var output string = fmt.Sprintf("LABEL %s\n", c.label)
	output += fmt.Sprintf("  MENU LABEL %s\n", c.menuLabel)
	output += fmt.Sprintf("  KERNEL %s\n", c.kernelPath)

	output += fmt.Sprintf("  APPEND initrd=%s", c.initrdPath)

	if c.useDHCP {
		output += " ip=dhcp"
	}

	// Ubuntu "specific"
	if c.boot != nil {
		output += fmt.Sprintf(" boot=%s", *c.boot)
	}

	if c.netboot != nil {
		output += fmt.Sprintf(" netboot=%s", *c.netboot)
	}

	if c.url != nil {
		*c.url = strings.Replace(*c.url, omitDirectory, "", 1)
		output += fmt.Sprintf(" url=%s", *c.url)
	}

	if c.cloudInit != nil {
		*c.cloudInit = strings.Replace(*c.cloudInit, omitDirectory, "", 1)
		output += fmt.Sprintf(" ds=nocloud-net;s=%s autoinstall", *c.cloudInit)
	}

	// Red Hat "specific"
	if c.instRepo != nil {
		*c.instRepo = strings.Replace(*c.instRepo, omitDirectory, "", 1)
		output += fmt.Sprintf(" inst.repo=%s", *c.instRepo)
	}

	if c.instStage2 != nil {
		*c.instStage2 = strings.Replace(*c.instStage2, omitDirectory, "", 1)
		output += fmt.Sprintf(" inst.stage2=%s", *c.instStage2)
	}

	// SUSE "specific"
	if c.suseInstall != nil {
		*c.suseInstall = strings.Replace(*c.suseInstall, omitDirectory, "", 1)
		output += fmt.Sprintf(" splash=silent vga=0x314 showopts install=%s", *c.suseInstall)
	}

	if c.autoYaST != nil {
		*c.autoYaST = strings.Replace(*c.autoYaST, omitDirectory, "", 1)
		output += fmt.Sprintf(" autoyast=%s", *c.autoYaST)
	}

	return output
}

// EX: originalISOPath=/data/isos/ubuntu.iso, isoCopyPath=/data/public/ubuntu.iso, outputDirectory=/data/public/ubuntu/
type ISOParser struct {
	distroType                   distroType
	originalISOPath              string
	isoCopyPath, outputDirectory string
	pxeConfig                    *PXEConfig
	logger                       *logger.Logger
}

func NewISOParser(originalISOPath, isoCopyPath, outputDirectory string) *ISOParser {
	var originalName string = strings.TrimSuffix(path.Base(originalISOPath), path.Ext(originalISOPath))

	return &ISOParser{
		distroType:      distroTypeBasedOnName(originalName),
		originalISOPath: path.Clean(originalISOPath),
		isoCopyPath:     path.Clean(isoCopyPath),
		outputDirectory: path.Clean(outputDirectory),
		pxeConfig: &PXEConfig{
			label:     originalName,
			menuLabel: originalName,
			useDHCP:   true,
		},
		logger: logger.NewLogger().SetPrefix(fmt.Sprintf("[%s]", originalName), logColorBasedOnName(originalName)).IncludeTimestamp(),
	}
}

func (p *ISOParser) setUpMounts() (err error) {
	p.logger.Status("Setting up mounts...")

	cleanUpMount()

	if _, err = runCommand("mkdir", "-p", "/mnt/iso"); err != nil {
		return fmt.Errorf("failed to create /mnt/iso: %s", err)
	}

	if _, err = runCommand("mount", "-o", "loop", p.originalISOPath, "/mnt/iso"); err != nil {
		return fmt.Errorf("failed to mount ISO: %s", err)
	}

	return nil
}

func (p *ISOParser) copyBaseFiles() (err error) {
	p.logger.Status("Copying ISO file...")

	if _, err = runCommand("cp", p.originalISOPath, p.isoCopyPath); err != nil {
		return fmt.Errorf("failed to copy ISO file: %s", err)
	}

	p.logger.Status("Copying base files...")

	if _, err = os.Stat(p.outputDirectory); err == nil {
		p.logger.Warningf("Output directory %s already exists, removing...\n", p.outputDirectory)

		if _, err = runCommand("rm", "-rf", p.outputDirectory); err != nil {
			return fmt.Errorf("failed to remove existing output directory: %s", err)
		}
	}

	if err = os.MkdirAll(p.outputDirectory, 0777); err != nil {
		return fmt.Errorf("failed to create output directory: %s", err)
	}

	if _, err = runCommand("sh", "-c", fmt.Sprintf("cp -r /mnt/iso/* %s/", p.outputDirectory)); err != nil {
		return fmt.Errorf("failed to copy files: %s", err)
	}

	return nil
}

func (p *ISOParser) copyKernelAndInitrd() (err error) {
	p.logger.Status("Copying kernel and initrd...")

	var vmzlinuz, initrd string

	if vmzlinuz, err = runCommand("find", "/mnt/iso", "-name", "vmlinuz*"); err != nil {
		return fmt.Errorf("failed to find vmlinuz: %s", err)
	}

	if vmzlinuz == "" {
		p.logger.Important("Could not find vmlinuz, trying for linux...")

		if vmzlinuz, err = runCommand("find", "/mnt/iso", "-name", "linux*"); err != nil {
			return fmt.Errorf("failed to find linux: %s", err)
		}

		if vmzlinuz == "" {
			return fmt.Errorf("could not find vmlinuz or linux")
		}

		p.logger.Warning("Found linux instead of vmlinuz, issue is resolved")
	}

	if initrd, err = runCommand("find", "/mnt/iso", "-name", "initrd*"); err != nil {
		return fmt.Errorf("failed to find initrd: %s", err)
	}

	p.pxeConfig.kernelPath = path.Join(p.outputDirectory, "vmlinuz")
	p.pxeConfig.initrdPath = path.Join(p.outputDirectory, "initrd")

	if _, err = runCommand("cp", vmzlinuz, p.pxeConfig.kernelPath); err != nil {
		return fmt.Errorf("failed to copy vmlinuz: %s", err)
	}

	if _, err = runCommand("cp", initrd, p.pxeConfig.initrdPath); err != nil {
		return fmt.Errorf("failed to copy initrd: %s", err)
	}

	return nil
}

func (p *ISOParser) detectPXEConfigs() (err error) {
	p.logger.Status("Detecting PXE configs...")

	var serverIP string

	serverIP, err = myLocalIP()

	if err != nil {
		return err
	}

	switch p.distroType {
	case UBUNTU:
		p.pxeConfig.boot = stringPtr("casper")
		p.pxeConfig.netboot = stringPtr("nfs")
		p.pxeConfig.url = stringPtr(fmt.Sprintf("http://%s:8069%s", serverIP, p.isoCopyPath))
		p.pxeConfig.cloudInit = stringPtr(fmt.Sprintf("http://%s:8069/cloud-init/", serverIP)) // TODO: Make this configurable
	case REDHAT:
		p.pxeConfig.instRepo = stringPtr(fmt.Sprintf("http://%s:8069%s", serverIP, p.outputDirectory))
		p.pxeConfig.instStage2 = stringPtr(fmt.Sprintf("http://%s:8069%s", serverIP, p.outputDirectory))
	case SUSE:
		p.pxeConfig.suseInstall = stringPtr(fmt.Sprintf("http://%s:8069%s", serverIP, p.outputDirectory))
		p.pxeConfig.autoYaST = stringPtr(fmt.Sprintf("http://%s:8069/AutoYaST.xml", serverIP))
	}

	return nil
}

func (p *ISOParser) Run() (err error) {
	p.logger.Status("Starting ISO parsing...")

	defer cleanUpMount()

	if err = p.setUpMounts(); err != nil {
		return err
	}

	if err = p.copyBaseFiles(); err != nil {
		return err
	}

	if err = p.copyKernelAndInitrd(); err != nil {
		return err
	}

	if err = p.detectPXEConfigs(); err != nil {
		return err
	}

	p.logger.Success("Successfully parsed ISO!")
	return nil
}

func parseISOsDirectory(directory string, outputDirectory string) (configs []*PXEConfig, err error) {
	var files []os.DirEntry

	if files, err = os.ReadDir(directory); err != nil {
		return nil, fmt.Errorf("failed to read directory: %s", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		parser := NewISOParser(path.Join(directory, file.Name()), path.Join(outputDirectory, file.Name()), path.Join(outputDirectory, strings.TrimSuffix(file.Name(), path.Ext(file.Name()))))

		if err := parser.Run(); err != nil {
			return nil, fmt.Errorf("failed to parse ISO: %s", err)
		}

		configs = append(configs, parser.pxeConfig)
	}

	return configs, nil
}

func generateTFTPConfig(filePath string, configs []*PXEConfig) (err error) {
	var config string = `DEFAULT menu.c32
PROMPT 0
TIMEOUT 600
PROMPT 0

MENU TITLE OpnLaaS PXE Boot Menu

`
	for _, conf := range configs {
		config += conf.String() + "\n\n"
	}

	if err = os.WriteFile(filePath, []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to write TFTP config: %s", err)
	}

	mainLog.Successf("Successfully generated TFTP config at %s:\n\n%s\n", filePath, config)

	return nil
}

func Run(inputDirectory, outputDirectory string) (err error) {
	omitDirectory = outputDirectory

	var configs []*PXEConfig

	mainLog.Status("Starting ISO parsing...")

	if configs, err = parseISOsDirectory(inputDirectory, outputDirectory); err != nil {
		mainLog.Error(err.Error())
		return err
	}

	if err = generateTFTPConfig(path.Join(outputDirectory, "pxelinux.cfg/default"), configs); err != nil {
		mainLog.Error(err.Error())
		return err
	}

	mainLog.Success("Successfully parsed all ISOs!")

	return nil
}
