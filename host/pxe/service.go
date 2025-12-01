package pxe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/db"
	isoextract "github.com/opnlaas/opnlaas/host/iso"
	ptftp "github.com/pin/tftp/v3"
	"github.com/z46-dev/go-logger"
)

var (
	serviceOnce sync.Once
	serviceErr  error
	instance    *Service
)

// Service wires together the DHCP, TFTP, and HTTP helpers that make up PXE boot.
type Service struct {
	log              *logger.Logger
	httpBaseURL      string
	tftpRoot         string
	httpRoot         string
	defaultProfile   defaultProfileConfig
	templateDefaults TemplateDefaults
	hostCache        *hostCache
	profileCache     *profileCache
	leases           *leaseStore
	leaseMu          sync.Mutex
	overrideMu       sync.RWMutex
	overrideProfiles map[string]*db.HostPXEProfile
	ipRangeStart     net.IP
	ipRangeEnd       net.IP
	leaseCursor      net.IP
	proxyDHCP        bool
	tftpServer       *ptftp.Server
	httpServer       *http.Server
	dhcpServer       *server4.Server
	quit             chan struct{}
}

// defaultProfileConfig mirrors the `.env` PXE defaults.
type defaultProfileConfig struct {
	ISOName      string
	BootFilename string
	KernelParams []string
	InitrdParams []string
	TemplateData map[string]string
	IPv4Address  string
	SubnetMask   string
	Gateway      string
	DNSServers   []string
	DomainName   string
	NextServer   string
}

// InitPXE instantiates and starts the PXE helper if enabled in configuration.
func InitPXE() error {
	if !config.Config.PXE.Enabled {
		return nil
	}

	serviceOnce.Do(func() {
		var err error
		if instance, err = newService(); err != nil {
			serviceErr = err
			return
		}

		serviceErr = instance.Start()
	})

	return serviceErr
}

// Shutdown stops the PXE helper when running.
func Shutdown() {
	if instance != nil {
		instance.Stop()
	}
}

// newService creates a new PXE service based on configuration.
func newService() (svc *Service, err error) {
	svc = &Service{}

	if strings.TrimSpace(config.Config.PXE.TFTPServer.Address) == "" && strings.TrimSpace(config.Config.PXE.HTTPServer.Address) == "" && strings.TrimSpace(config.Config.PXE.DHCPServer.Address) == "" {
		err = fmt.Errorf("pxe: nothing configured (TFTP/HTTP/DHCP all disabled)")
		return
	}

	if strings.TrimSpace(config.Config.PXE.TFTPServer.Directory) == "" {
		err = fmt.Errorf("pxe: TFTP root directory must be configured")
		return
	}

	if strings.TrimSpace(config.Config.PXE.HTTPServer.Directory) == "" {
		config.Config.PXE.HTTPServer.Directory = config.Config.PXE.TFTPServer.Directory
	}

	var (
		ipRangeStart, ipRangeEnd net.IP
		start, end               string
	)
	if !config.Config.PXE.DHCPServer.ProxyMode {
		if start = strings.TrimSpace(config.Config.PXE.DHCPServer.IPRangeStart); start != "" {
			if ipRangeStart = net.ParseIP(start).To4(); ipRangeStart == nil {
				err = fmt.Errorf("pxe: invalid DHCP ip_range_start %q", start)
				return
			}
		}

		if end = strings.TrimSpace(config.Config.PXE.DHCPServer.IPRangeEnd); end != "" {
			if ipRangeEnd = net.ParseIP(end).To4(); ipRangeEnd == nil {
				err = fmt.Errorf("pxe: invalid DHCP ip_range_end %q", end)
				return
			}
		}

		if (ipRangeStart == nil) != (ipRangeEnd == nil) {
			err = fmt.Errorf("pxe: both DHCP ip_range_start and ip_range_end must be set together")
			return
		}

		if ipRangeStart != nil && compareIPv4(ipRangeStart, ipRangeEnd) > 0 {
			err = fmt.Errorf("pxe: DHCP ip_range_start must be <= ip_range_end")
			return
		}
	}

	svc.log = logger.NewLogger().SetPrefix("[PXE]", logger.BoldBlue).IncludeTimestamp()

	if svc.templateDefaults, err = loadTemplateDefaults(); err != nil {
		err = fmt.Errorf("pxe: %w", err)
		return
	}

	var ipURL string
	svc.httpBaseURL = strings.TrimSuffix(strings.TrimSpace(config.Config.PXE.HTTPServer.PublicURL), "/")
	if ipURL = buildPublicURLFromIP(config.Config.PXE.DHCPServer.ServerPublicAddress, config.Config.PXE.HTTPServer.Address); ipURL != "" {
		if svc.httpBaseURL != "" && svc.httpBaseURL != ipURL {
			svc.log.Warningf("PXE HTTP public URL overridden by DHCP server public address (%s -> %s)\n", svc.httpBaseURL, ipURL)
		}

		svc.httpBaseURL = ipURL
	}

	svc.tftpRoot = filepath.Clean(config.Config.PXE.TFTPServer.Directory)
	svc.httpRoot = filepath.Clean(config.Config.PXE.HTTPServer.Directory)
	svc.quit = make(chan struct{})
	svc.hostCache = newHostCache(30 * time.Second)
	svc.profileCache = newProfileCache(15 * time.Second)
	svc.leases = newLeaseStore()
	svc.overrideProfiles = make(map[string]*db.HostPXEProfile)
	svc.ipRangeStart = cloneIPv4(ipRangeStart)
	svc.ipRangeEnd = cloneIPv4(ipRangeEnd)
	if ipRangeStart != nil {
		svc.leaseCursor = cloneIPv4(ipRangeStart)
	}

	svc.proxyDHCP = config.Config.PXE.DHCPServer.ProxyMode
	svc.defaultProfile = defaultProfileConfig{
		SubnetMask: config.Config.PXE.DHCPServer.SubnetMask,
		Gateway:    config.Config.PXE.DHCPServer.Router,
		DNSServers: cloneStringSlice(config.Config.PXE.DHCPServer.DNSServers),
		NextServer: config.Config.PXE.DHCPServer.ServerPublicAddress,
	}

	if svc.defaultProfile.BootFilename == "" {
		svc.defaultProfile.BootFilename = "pxelinux.0"
	}

	if svc.defaultProfile.ISOName == "" {
		var isoName string
		if isoName, err = pickDefaultISOName(); err != nil {
			err = fmt.Errorf("pxe: determine default ISO: %w", err)
			return
		} else if isoName != "" {
			svc.defaultProfile.ISOName = isoName
			svc.log.Warningf("PXE default ISO not configured; falling back to %s\n", isoName)
		} else {
			svc.log.Warning("PXE default ISO not configured and no stored ISOs available; PXE profiles must be defined explicitly\n")
		}
	}

	svc.validateSyslinuxAssets()
	svc.ensureArtifactAliases()
	svc.ensureStage2Artifacts()
	return
}

// Start brings up the DHCP, TFTP, and HTTP listeners.
func (s *Service) Start() (err error) {
	if err = s.startTFTP(); err != nil {
		err = fmt.Errorf("start tftp: %w", err)
		return
	}

	if err = s.startHTTP(); err != nil {
		err = fmt.Errorf("start http: %w", err)
		return
	}

	if err = s.startDHCP(); err != nil {
		err = fmt.Errorf("start dhcp: %w", err)
		return
	}

	s.log.Statusf("PXE helper ready (TFTP=%s HTTP=%s DHCP=%s)\n", config.Config.PXE.TFTPServer.Address, config.Config.PXE.HTTPServer.Address, config.Config.PXE.DHCPServer.Address)
	return
}

// Stop shuts down listeners.
func (s *Service) Stop() {
	select {
	case <-s.quit:
	default:
		close(s.quit)
	}

	if s.dhcpServer != nil {
		_ = s.dhcpServer.Close()
	}

	if s.httpServer != nil {
		var (
			ctx    context.Context
			cancel context.CancelFunc
		)

		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(ctx)
	}

	if s.tftpServer != nil {
		s.tftpServer.Shutdown()
	}
}

// startTFTP starts the TFTP server if configured.
func (s *Service) startTFTP() (err error) {
	if strings.TrimSpace(config.Config.PXE.TFTPServer.Address) == "" {
		return
	}

	var handler func(string, io.ReaderFrom) error = func(filename string, rf io.ReaderFrom) (err error) {
		var (
			data       []byte
			reader     *bytes.Reader
			remoteAddr *net.UDPAddr
		)

		if transfer, ok := rf.(ptftp.OutgoingTransfer); ok {
			addr := transfer.RemoteAddr()
			remoteAddr = &addr
		}

		if data, err = s.handleTFTPRequest(filename, remoteAddr); err != nil {
			return
		}

		reader = bytes.NewReader(data)
		_, err = rf.ReadFrom(reader)
		return
	}

	var (
		srv     *ptftp.Server = ptftp.NewServer(handler, nil)
		udpAddr *net.UDPAddr
		conn    *net.UDPConn
	)

	srv.SetTimeout(5 * time.Second)
	if udpAddr, err = net.ResolveUDPAddr("udp4", config.Config.PXE.TFTPServer.Address); err != nil {
		return
	}

	if conn, err = net.ListenUDP("udp4", udpAddr); err != nil {
		return
	}

	go func() {
		var err error
		if err = srv.Serve(conn); err != nil && !errors.Is(err, net.ErrClosed) {
			s.log.Errorf("TFTP server error: %v\n", err)
		}
	}()

	s.tftpServer = srv
	return
}

// startHTTP starts the HTTP server if configured.
func (s *Service) startHTTP() (err error) {
	if strings.TrimSpace(config.Config.PXE.HTTPServer.Address) == "" {
		return
	}

	var (
		mux *http.ServeMux = http.NewServeMux()
		ln  net.Listener
	)

	mux.HandleFunc("/", s.httpHandler)
	if ln, err = listenHTTP(config.Config.PXE.HTTPServer.Address); err != nil {
		return
	}

	s.httpServer = &http.Server{
		Addr:    config.Config.PXE.HTTPServer.Address,
		Handler: mux,
	}

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Errorf("HTTP server error: %v\n", err)
		}
	}()

	return
}

// startDHCP starts the DHCP server if configured.
func (s *Service) startDHCP() (err error) {
	if strings.TrimSpace(config.Config.PXE.DHCPServer.Address) == "" {
		return
	}

	var (
		handler func(net.PacketConn, net.Addr, *dhcpv4.DHCPv4) = func(conn net.PacketConn, peer net.Addr, req *dhcpv4.DHCPv4) {
			var err error
			if err = s.handleDHCP(conn, peer, req); err != nil {
				s.log.Errorf("DHCP handler error: %v\n", err)
			}
		}
		addr   *net.UDPAddr
		server *server4.Server
	)

	if addr, err = net.ResolveUDPAddr("udp4", config.Config.PXE.DHCPServer.Address); err != nil {
		return
	}

	if server, err = server4.NewServer("", addr, handler); err != nil {
		return
	}

	s.dhcpServer = server
	go func() {
		if err := server.Serve(); err != nil && !errors.Is(err, net.ErrClosed) {
			s.log.Errorf("DHCP server error: %v\n", err)
		}
	}()

	return
}

// absoluteURL builds an absolute URL based on the configured HTTP base URL.
func (s *Service) absoluteURL(rel string) (abs string) {
	if abs = pathClean(rel); s.httpBaseURL != "" {
		abs = strings.TrimRight(s.httpBaseURL, "/") + abs
	}

	return
}

// pathClean normalizes a relative path to ensure it starts with a single '/'.
func pathClean(rel string) (clean string) {
	clean = path.Clean(fmt.Sprintf("/%s", strings.TrimPrefix(rel, "/")))
	return
}

// listenHTTP creates a net.Listener for the given address, preferring IPv4.
func listenHTTP(addr string) (listener net.Listener, err error) {
	var (
		host    string
		network string = "tcp"
		ip      net.IP
	)

	if host, _, err = net.SplitHostPort(addr); err != nil {
		listener, err = net.Listen("tcp4", addr)
		return
	}

	if host == "" {
		network = "tcp4"
	} else if ip = net.ParseIP(host); ip != nil && ip.To4() != nil {
		network = "tcp4"
	}

	listener, err = net.Listen(network, addr)
	return
}

// serveProfileFile serves a PXE profile-related file over TFTP or HTTP.
func (s *Service) serveProfileFile(rel string) (data []byte, err error) {
	// parts := strings.SplitN(strings.TrimPrefix(rel, "/"), "/", 4)
	// if len(parts) < 3 {
	// 	return nil, fmt.Errorf("invalid profile path %s", rel)
	// }

	// slug := strings.ToLower(parts[1])
	// category := strings.ToLower(parts[2])
	// var remainder string
	// if len(parts) == 4 {
	// 	remainder = parts[3]
	// }

	// host, _ := s.hostCache.BySlug(slug)
	// var profile *db.HostPXEProfile
	// var err error
	// if host != nil {
	// 	profile, err = s.profileCache.ByIP(host.ManagementIP)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// }
	// if profile == nil {
	// 	profile = s.buildDefaultProfile(host, "")
	// 	if profile == nil {
	// 		return nil, fmt.Errorf("no profile available for slug %s", slug)
	// 	}
	// }

	// iso, err := db.StoredISOImages.Select(profile.ISOName)
	// if err != nil {
	// 	return nil, err
	// }
	// if iso == nil {
	// 	return nil, fmt.Errorf("iso %s not found", profile.ISOName)
	// }

	// ctx := s.buildTemplateContext(host, profile, iso)
	// switch category {
	// case "cloud-init":
	// 	switch strings.ToLower(remainder) {
	// 	case "user-data":
	// 		return s.renderTemplate(templateKeyCloudInitUserData, ctx)
	// 	case "meta-data":
	// 		return s.renderTemplate(templateKeyCloudInitMetaData, ctx)
	// 	case "vendor-data":
	// 		return []byte("#cloud-config\n{}"), nil
	// 	default:
	// 		return nil, fmt.Errorf("unknown cloud-init artifact %s", remainder)
	// 	}
	// case "user-data":
	// 	return s.renderTemplate(templateKeyCloudInitUserData, ctx)
	// case "meta-data":
	// 	return s.renderTemplate(templateKeyCloudInitMetaData, ctx)
	// case "vendor-data":
	// 	return []byte("#cloud-config\n{}"), nil
	// case "kickstart":
	// 	if remainder != "" && strings.ToLower(remainder) != "ks.cfg" {
	// 		return nil, fmt.Errorf("unknown kickstart artifact %s", remainder)
	// 	}
	// 	return s.renderTemplate(templateKeyKickstart, ctx)
	// default:
	// 	return nil, fmt.Errorf("unknown profile artifact %s", rel)
	// }

	var parts []string = strings.SplitN(strings.TrimPrefix(rel, "/"), "/", 4)
	if len(parts) < 3 {
		err = fmt.Errorf("invalid profile path %s", rel)
		return
	}

	var (
		slug, category string = strings.ToLower(parts[1]), strings.ToLower(parts[2])
		remainder      string
		host           *db.Host
		profile        *db.HostPXEProfile
	)

	if len(parts) == 4 {
		remainder = parts[3]
	}

	if host, _ = s.hostCache.BySlug(slug); host != nil {
		if profile, err = s.profileCache.ByIP(host.ManagementIP); err != nil {
			return
		}

		if profile == nil {
			profile = s.overrideProfileForHost(host)
		}
	}

	if profile == nil {
		if profile = s.buildDefaultProfile(host, ""); profile == nil {
			err = fmt.Errorf("no profile available for slug %s", slug)
			return
		}
	}

	var iso *db.StoredISOImage
	if iso, err = db.StoredISOImages.Select(profile.ISOName); err != nil {
		return
	} else if iso == nil {
		err = fmt.Errorf("iso %s not found", profile.ISOName)
		return
	}

	var ctx *TemplateContext = s.buildTemplateContext(host, profile, iso)
	switch category {
	case "cloud-init":
		switch strings.ToLower(remainder) {
		case "user-data":
			data, err = s.renderTemplate(templateKeyCloudInitUserData, ctx)
			return
		case "meta-data":
			data, err = s.renderTemplate(templateKeyCloudInitMetaData, ctx)
			return
		case "vendor-data":
			data = []byte("#cloud-config\n{}")
			return
		default:
			err = fmt.Errorf("unknown cloud-init artifact %s", remainder)
			return
		}
	case "user-data":
		data, err = s.renderTemplate(templateKeyCloudInitUserData, ctx)
		return
	case "meta-data":
		data, err = s.renderTemplate(templateKeyCloudInitMetaData, ctx)
		return
	case "vendor-data":
		data = []byte("#cloud-config\n{}")
		return
	case "kickstart":
		if remainder != "" && strings.ToLower(remainder) != "ks.cfg" {
			err = fmt.Errorf("unknown kickstart artifact %s", remainder)
			return
		}
		data, err = s.renderTemplate(templateKeyKickstart, ctx)
		return
	default:
		err = fmt.Errorf("unknown profile artifact %s", rel)
		return
	}
}

// validateSyslinuxAssets checks for the presence of required Syslinux files.
func (s *Service) validateSyslinuxAssets() {
	var err error
	for _, name := range []string{"pxelinux.0", "ldlinux.c32"} {
		var path string = filepath.Join(s.tftpRoot, name)
		if _, err = os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				s.log.Warningf("Syslinux asset missing: %s; run scripting/setup_host.sh to install\n", path)
			} else {
				s.log.Errorf("Unable to stat %s: %v\n", path, err)
			}
		}
	}

	var cfgDir string = filepath.Join(s.tftpRoot, "pxelinux.cfg")
	if _, err = os.Stat(cfgDir); os.IsNotExist(err) {
		s.log.Warningf("pxelinux.cfg directory missing at %s; pxelinux may fail before we can serve configs\n", cfgDir)
	}
}

// httpHandler handles incoming HTTP requests for PXE artifacts.
func (s *Service) httpHandler(w http.ResponseWriter, r *http.Request) {
	var (
		p   string = pathClean(r.URL.Path)
		err error
	)

	if p == "/" || p == "" {
		http.NotFound(w, r)
		return
	}

	var logPrefix string = fmt.Sprintf("%s %s", r.Method, p)
	if strings.HasPrefix(strings.ToLower(p), "/profiles/") {
		var data []byte

		if data, err = s.serveProfileFile(p); err == nil {
			s.log.Basicf("HTTP served profile %s from %s\n", p, r.RemoteAddr)
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(data)
			return
		} else {
			s.log.Errorf("HTTP profile error %s: %v\n", logPrefix, err)
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	}

	var (
		full string = filepath.Join(s.httpRoot, filepath.FromSlash(strings.TrimPrefix(p, "/")))
		file *os.File
		info os.FileInfo
	)

	if file, err = os.Open(full); err != nil {
		s.log.Errorf("HTTP static miss %s: %v\n", logPrefix, err)
		http.NotFound(w, r)
		return
	}

	defer file.Close()

	if info, err = file.Stat(); err != nil {
		s.log.Errorf("HTTP stat error %s: %v\n", logPrefix, err)
		http.NotFound(w, r)
		return
	}

	s.log.Basicf("HTTP served %s (%d bytes)\n", p, info.Size())
	http.ServeContent(w, r, path.Base(p), info.ModTime(), file)
}

// buildDefaultProfile constructs a PXE profile based on default settings.
func (s *Service) buildDefaultProfile(host *db.Host, mac string) (profile *db.HostPXEProfile) {
	if s.defaultProfile.ISOName == "" {
		return
	}

	profile = &db.HostPXEProfile{
		ManagementIP: func() string {
			if host != nil {
				return host.ManagementIP
			}
			return ""
		}(),
		ISOName:      s.defaultProfile.ISOName,
		BootFilename: s.defaultProfile.BootFilename,
		KernelParams: cloneStringSlice(s.defaultProfile.KernelParams),
		InitrdParams: cloneStringSlice(s.defaultProfile.InitrdParams),
		TemplateData: cloneMap(s.defaultProfile.TemplateData),
		IPv4Address:  s.defaultProfile.IPv4Address,
		SubnetMask:   s.defaultProfile.SubnetMask,
		Gateway:      s.defaultProfile.Gateway,
		DNSServers:   cloneStringSlice(s.defaultProfile.DNSServers),
		DomainName:   s.defaultProfile.DomainName,
		NextServer:   s.defaultProfile.NextServer,
		BootMACAddress: func() string {
			if mac == "" && host != nil && len(host.NetworkInterfaces) > 0 {
				return host.NetworkInterfaces[0].MACAddress
			}
			return mac
		}(),
	}

	if profile.TemplateData == nil {
		profile.TemplateData = map[string]string{}
	}

	return
}

// pickDefaultISOName selects a default ISO name from stored ISOs.
func pickDefaultISOName() (name string, err error) {
	var records []*db.StoredISOImage
	if records, err = db.StoredISOImages.SelectAll(); err != nil || len(records) == 0 {
		return
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].Name < records[j].Name
	})

	name = records[0].Name
	return
}

// ensureArtifactAliases creates symlinks for artifact directories with slugified names.
func (s *Service) ensureArtifactAliases() {
	var err error
	if err = s.ensureAliasForRoot(s.tftpRoot); err != nil {
		s.log.Warningf("PXE artifact alias error (tftp): %v\n", err)
	}

	if s.httpRoot != s.tftpRoot {
		if err = s.ensureAliasForRoot(s.httpRoot); err != nil {
			s.log.Warningf("PXE artifact alias error (http): %v\n", err)
		}
	}
}

// ensureAliasForRoot creates symlinks for artifact directories with slugified names under the given root.
func (s *Service) ensureAliasForRoot(root string) (err error) {
	if root = strings.TrimSpace(root); root == "" {
		return
	}

	var records []*db.StoredISOImage
	if records, err = db.StoredISOImages.SelectAll(); err != nil {
		return
	}

	for _, iso := range records {
		if iso == nil || strings.TrimSpace(iso.Name) == "" {
			continue
		}

		var dirName string = makeArtifactDirName(iso.Name)
		if dirName == strings.TrimSpace(iso.Name) {
			continue
		}

		var (
			actual string = filepath.Join(root, filepath.FromSlash(path.Join("artifacts", iso.Name)))
			alias  string = filepath.Join(root, filepath.FromSlash(path.Join("artifacts", dirName)))
		)

		if _, err = os.Stat(actual); err != nil {
			continue
		}

		if _, err = os.Lstat(alias); err == nil {
			continue
		}

		if err = os.MkdirAll(filepath.Dir(alias), 0755); err != nil {
			return err
		}

		if err = os.Symlink(actual, alias); err != nil && !os.IsExist(err) {
			return err
		}

		s.log.Basicf("PXE artifact alias created %s -> %s\n", alias, actual)
	}

	return
}

// buildPublicURLFromIP constructs a public URL from the given IP and HTTP address.
func buildPublicURLFromIP(ip string, httpAddr string) (addr string) {
	if ip = strings.TrimSpace(ip); ip == "" {
		return
	}

	var port string = "80"
	if addr = strings.TrimSpace(httpAddr); addr != "" {
		if strings.HasPrefix(addr, ":") {
			port = strings.TrimPrefix(addr, ":")
		} else if _, p, err := net.SplitHostPort(addr); err == nil && strings.TrimSpace(p) != "" {
			port = p
		}
	}

	if port == "" || port == "80" {
		addr = fmt.Sprintf("http://%s", ip)
	} else {
		addr = fmt.Sprintf("http://%s:%s", ip, port)
	}

	return
}

// serverIdentifierIP returns the IP address the PXE server identifies as.
func (s *Service) serverIdentifierIP() (ip net.IP) {
	if ip = net.ParseIP(config.Config.PXE.DHCPServer.ServerPublicAddress); ip != nil {
		return
	}

	var (
		host string
		err  error
	)

	if host, _, err = net.SplitHostPort(config.Config.PXE.DHCPServer.Address); err == nil && strings.TrimSpace(host) != "" {
		if ip = net.ParseIP(host); ip != nil {
			return
		}
	}

	return
}

// ensureStage2Artifacts prepares stage2 artifacts for stored ISOs.
func (s *Service) ensureStage2Artifacts() {
	var (
		root    string
		records []*db.StoredISOImage
		err     error
	)

	if root = strings.TrimSpace(s.httpRoot); root == "" {
		return
	}

	if records, err = db.StoredISOImages.SelectAll(); err != nil {
		s.log.Warningf("PXE stage2 listing error: %v\n", err)
		return
	}

	for _, rec := range records {
		if rec == nil || strings.TrimSpace(rec.FullISOPath) == "" {
			continue
		}

		var dest string = filepath.Join(root, filepath.FromSlash(path.Join("artifacts", rec.Name, "stage2")))
		if _, err = os.Stat(filepath.Join(dest, ".treeinfo")); err == nil {
			continue
		}

		if err = isoextract.EnsureStage2Artifacts(rec.FullISOPath, dest); err != nil {
			s.log.Warningf("PXE stage2 prepare error for %s: %v\n", rec.Name, err)
		} else {
			s.log.Basicf("PXE stage2 artifacts prepared for %s\n", rec.Name)
		}
	}
}
