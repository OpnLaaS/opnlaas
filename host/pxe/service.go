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
	log         *logger.Logger
	cfg         config.Configuration
	httpBaseURL string

	tftpRoot string
	httpRoot string

	defaultProfile   defaultProfileConfig
	templateDefaults TemplateDefaults

	hostCache    *hostCache
	profileCache *profileCache
	leases       *leaseStore
	leaseMu      sync.Mutex

	ipRangeStart net.IP
	ipRangeEnd   net.IP
	leaseCursor  net.IP
	proxyDHCP    bool

	tftpServer *ptftp.Server
	httpServer *http.Server
	dhcpServer *server4.Server

	quit chan struct{}
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

func newService() (*Service, error) {
	cfg := config.Config

	if strings.TrimSpace(cfg.PXE.TFTPServer.Address) == "" && strings.TrimSpace(cfg.PXE.HTTPServer.Address) == "" && strings.TrimSpace(cfg.PXE.DHCPServer.Address) == "" {
		return nil, fmt.Errorf("pxe: nothing configured (TFTP/HTTP/DHCP all disabled)")
	}

	if strings.TrimSpace(cfg.PXE.TFTPServer.Directory) == "" {
		return nil, fmt.Errorf("pxe: TFTP root directory must be configured")
	}

	if strings.TrimSpace(cfg.PXE.HTTPServer.Directory) == "" {
		cfg.PXE.HTTPServer.Directory = cfg.PXE.TFTPServer.Directory
	}

	var ipRangeStart, ipRangeEnd net.IP
	if !cfg.PXE.DHCPServer.ProxyMode {
		if start := strings.TrimSpace(cfg.PXE.DHCPServer.IPRangeStart); start != "" {
			ipRangeStart = net.ParseIP(start).To4()
			if ipRangeStart == nil {
				return nil, fmt.Errorf("pxe: invalid DHCP ip_range_start %q", start)
			}
		}
		if end := strings.TrimSpace(cfg.PXE.DHCPServer.IPRangeEnd); end != "" {
			ipRangeEnd = net.ParseIP(end).To4()
			if ipRangeEnd == nil {
				return nil, fmt.Errorf("pxe: invalid DHCP ip_range_end %q", end)
			}
		}
		if (ipRangeStart == nil) != (ipRangeEnd == nil) {
			return nil, fmt.Errorf("pxe: both DHCP ip_range_start and ip_range_end must be set together")
		}
		if ipRangeStart != nil && compareIPv4(ipRangeStart, ipRangeEnd) > 0 {
			return nil, fmt.Errorf("pxe: DHCP ip_range_start must be <= ip_range_end")
		}
	}

	log := logger.NewLogger().SetPrefix("[PXE]", logger.BoldBlue).IncludeTimestamp()

	templateDefaults, err := loadTemplateDefaults()
	if err != nil {
		return nil, fmt.Errorf("pxe: %w", err)
	}

	httpBaseURL := strings.TrimSuffix(strings.TrimSpace(cfg.PXE.HTTPServer.PublicURL), "/")
	if ipURL := buildPublicURLFromIP(cfg.PXE.DHCPServer.ServerPublicAddress, cfg.PXE.HTTPServer.Address); ipURL != "" {
		if httpBaseURL != "" && httpBaseURL != ipURL {
			log.Warningf("PXE HTTP public URL overridden by DHCP server public address (%s -> %s)\n", httpBaseURL, ipURL)
		}
		httpBaseURL = ipURL
	}

	svc := &Service{
		log:              log,
		cfg:              cfg,
		tftpRoot:         filepath.Clean(cfg.PXE.TFTPServer.Directory),
		httpRoot:         filepath.Clean(cfg.PXE.HTTPServer.Directory),
		httpBaseURL:      httpBaseURL,
		quit:             make(chan struct{}),
		hostCache:        newHostCache(30 * time.Second),
		profileCache:     newProfileCache(15 * time.Second),
		leases:           newLeaseStore(),
		templateDefaults: templateDefaults,
		ipRangeStart:     cloneIPv4(ipRangeStart),
		ipRangeEnd:       cloneIPv4(ipRangeEnd),
		leaseCursor: func() net.IP {
			if ipRangeStart == nil {
				return nil
			}
			return cloneIPv4(ipRangeStart)
		}(),
		proxyDHCP: cfg.PXE.DHCPServer.ProxyMode,
		defaultProfile: defaultProfileConfig{
			SubnetMask: cfg.PXE.DHCPServer.SubnetMask,
			Gateway:    cfg.PXE.DHCPServer.Router,
			DNSServers: cloneStringSlice(cfg.PXE.DHCPServer.DNSServers),
			NextServer: cfg.PXE.DHCPServer.ServerPublicAddress,
		},
	}

	if svc.defaultProfile.BootFilename == "" {
		svc.defaultProfile.BootFilename = "pxelinux.0"
	}
	if svc.defaultProfile.ISOName == "" {
		if isoName, err := pickDefaultISOName(); err != nil {
			return nil, fmt.Errorf("pxe: determine default ISO: %w", err)
		} else if isoName != "" {
			svc.defaultProfile.ISOName = isoName
			log.Warningf("PXE default ISO not configured; falling back to %s\n", isoName)
		} else {
			log.Warning("PXE default ISO not configured and no stored ISOs available; PXE profiles must be defined explicitly\n")
		}
	}

	svc.validateSyslinuxAssets()
	svc.ensureArtifactAliases()
	svc.ensureStage2Artifacts()
	return svc, nil
}

// Start brings up the DHCP, TFTP, and HTTP listeners.
func (s *Service) Start() error {
	if err := s.startTFTP(); err != nil {
		return fmt.Errorf("start tftp: %w", err)
	}
	if err := s.startHTTP(); err != nil {
		return fmt.Errorf("start http: %w", err)
	}
	if err := s.startDHCP(); err != nil {
		return fmt.Errorf("start dhcp: %w", err)
	}
	s.log.Statusf("PXE helper ready (TFTP=%s HTTP=%s DHCP=%s)\n",
		s.cfg.PXE.TFTPServer.Address, s.cfg.PXE.HTTPServer.Address, s.cfg.PXE.DHCPServer.Address)
	return nil
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(ctx)
	}
	if s.tftpServer != nil {
		s.tftpServer.Shutdown()
	}
}

func (s *Service) startTFTP() error {
	if strings.TrimSpace(s.cfg.PXE.TFTPServer.Address) == "" {
		return nil
	}

	handler := func(filename string, rf io.ReaderFrom) error {
		data, err := s.handleTFTPRequest(filename)
		if err != nil {
			return err
		}
		reader := bytes.NewReader(data)
		_, err = rf.ReadFrom(reader)
		return err
	}

	srv := ptftp.NewServer(handler, nil)
	srv.SetTimeout(5 * time.Second)

	udpAddr, err := net.ResolveUDPAddr("udp4", s.cfg.PXE.TFTPServer.Address)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		return err
	}

	go func() {
		if err := srv.Serve(conn); err != nil && !errors.Is(err, net.ErrClosed) {
			s.log.Errorf("TFTP server error: %v\n", err)
		}
	}()

	s.tftpServer = srv
	return nil
}

func (s *Service) startHTTP() error {
	if strings.TrimSpace(s.cfg.PXE.HTTPServer.Address) == "" {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.httpHandler)

	ln, err := listenHTTP(s.cfg.PXE.HTTPServer.Address)
	if err != nil {
		return err
	}

	s.httpServer = &http.Server{
		Addr:    s.cfg.PXE.HTTPServer.Address,
		Handler: mux,
	}

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Errorf("HTTP server error: %v\n", err)
		}
	}()

	return nil
}

func (s *Service) startDHCP() error {
	if strings.TrimSpace(s.cfg.PXE.DHCPServer.Address) == "" {
		return nil
	}

	handler := func(conn net.PacketConn, peer net.Addr, req *dhcpv4.DHCPv4) {
		if err := s.handleDHCP(conn, peer, req); err != nil {
			s.log.Errorf("DHCP handler error: %v\n", err)
		}
	}

	addr, err := net.ResolveUDPAddr("udp4", s.cfg.PXE.DHCPServer.Address)
	if err != nil {
		return err
	}

	server, err := server4.NewServer("", addr, handler)
	if err != nil {
		return err
	}
	s.dhcpServer = server
	go func() {
		if err := server.Serve(); err != nil && !errors.Is(err, net.ErrClosed) {
			s.log.Errorf("DHCP server error: %v\n", err)
		}
	}()
	return nil
}

func (s *Service) absoluteURL(rel string) string {
	clean := pathClean(rel)
	if s.httpBaseURL == "" {
		return clean
	}
	return strings.TrimRight(s.httpBaseURL, "/") + clean
}

func (s *Service) serveStatic(root, rel string) ([]byte, error) {
	if root == "" {
		return nil, fmt.Errorf("no root configured")
	}
	full := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(rel, "/")))
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func pathClean(rel string) string {
	normalized := "/" + strings.TrimPrefix(rel, "/")
	return path.Clean(normalized)
}

func listenHTTP(addr string) (net.Listener, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return net.Listen("tcp4", addr)
	}
	network := "tcp"
	if host == "" {
		network = "tcp4"
	} else if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
		network = "tcp4"
	}
	return net.Listen(network, addr)
}

func (s *Service) serveProfileFile(rel string) ([]byte, error) {
	parts := strings.SplitN(strings.TrimPrefix(rel, "/"), "/", 4)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid profile path %s", rel)
	}

	slug := strings.ToLower(parts[1])
	category := strings.ToLower(parts[2])
	var remainder string
	if len(parts) == 4 {
		remainder = parts[3]
	}

	host, _ := s.hostCache.BySlug(slug)
	var profile *db.HostPXEProfile
	var err error
	if host != nil {
		profile, err = s.profileCache.ByIP(host.ManagementIP)
		if err != nil {
			return nil, err
		}
	}
	if profile == nil {
		profile = s.buildDefaultProfile(host, "")
		if profile == nil {
			return nil, fmt.Errorf("no profile available for slug %s", slug)
		}
	}

	iso, err := db.StoredISOImages.Select(profile.ISOName)
	if err != nil {
		return nil, err
	}
	if iso == nil {
		return nil, fmt.Errorf("iso %s not found", profile.ISOName)
	}

	ctx := s.buildTemplateContext(host, profile, iso)
	switch category {
	case "cloud-init":
		switch strings.ToLower(remainder) {
		case "user-data":
			return s.renderTemplate(templateKeyCloudInitUserData, ctx)
		case "meta-data":
			return s.renderTemplate(templateKeyCloudInitMetaData, ctx)
		case "vendor-data":
			return []byte("#cloud-config\n{}"), nil
		default:
			return nil, fmt.Errorf("unknown cloud-init artifact %s", remainder)
		}
	case "user-data":
		return s.renderTemplate(templateKeyCloudInitUserData, ctx)
	case "meta-data":
		return s.renderTemplate(templateKeyCloudInitMetaData, ctx)
	case "vendor-data":
		return []byte("#cloud-config\n{}"), nil
	case "kickstart":
		if remainder != "" && strings.ToLower(remainder) != "ks.cfg" {
			return nil, fmt.Errorf("unknown kickstart artifact %s", remainder)
		}
		return s.renderTemplate(templateKeyKickstart, ctx)
	default:
		return nil, fmt.Errorf("unknown profile artifact %s", rel)
	}
}

func (s *Service) validateSyslinuxAssets() {
	required := []string{"pxelinux.0", "ldlinux.c32"}
	for _, name := range required {
		p := filepath.Join(s.tftpRoot, name)
		if _, err := os.Stat(p); err != nil {
			if os.IsNotExist(err) {
				s.log.Warningf("Syslinux asset missing: %s; run scripting/setup_host.sh to install\n", p)
			} else {
				s.log.Errorf("Unable to stat %s: %v\n", p, err)
			}
		}
	}
	cfgDir := filepath.Join(s.tftpRoot, "pxelinux.cfg")
	if _, err := os.Stat(cfgDir); os.IsNotExist(err) {
		s.log.Warningf("pxelinux.cfg directory missing at %s; pxelinux may fail before we can serve configs\n", cfgDir)
	}
}

func (s *Service) httpHandler(w http.ResponseWriter, r *http.Request) {
	p := pathClean(r.URL.Path)
	if p == "/" || p == "" {
		http.NotFound(w, r)
		return
	}

	logPrefix := fmt.Sprintf("%s %s", r.Method, p)

	if strings.HasPrefix(strings.ToLower(p), "/profiles/") {
		if data, err := s.serveProfileFile(p); err == nil {
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

	full := filepath.Join(s.httpRoot, filepath.FromSlash(strings.TrimPrefix(p, "/")))
	f, err := os.Open(full)
	if err != nil {
		s.log.Errorf("HTTP static miss %s: %v\n", logPrefix, err)
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		s.log.Errorf("HTTP stat error %s: %v\n", logPrefix, err)
		http.NotFound(w, r)
		return
	}

	s.log.Basicf("HTTP served %s (%d bytes)\n", p, info.Size())
	http.ServeContent(w, r, path.Base(p), info.ModTime(), f)
}

func (s *Service) buildDefaultProfile(host *db.Host, mac string) *db.HostPXEProfile {
	if s.defaultProfile.ISOName == "" {
		return nil
	}
	profile := &db.HostPXEProfile{
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
	return profile
}

func pickDefaultISOName() (string, error) {
	records, err := db.StoredISOImages.SelectAll()
	if err != nil {
		return "", err
	}
	if len(records) == 0 {
		return "", nil
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Name < records[j].Name
	})
	return records[0].Name, nil
}

func (s *Service) ensureArtifactAliases() {
	if err := s.ensureAliasForRoot(s.tftpRoot); err != nil {
		s.log.Warningf("PXE artifact alias error (tftp): %v\n", err)
	}
	if s.httpRoot != s.tftpRoot {
		if err := s.ensureAliasForRoot(s.httpRoot); err != nil {
			s.log.Warningf("PXE artifact alias error (http): %v\n", err)
		}
	}
}

func (s *Service) ensureAliasForRoot(root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	records, err := db.StoredISOImages.SelectAll()
	if err != nil {
		return err
	}
	for _, iso := range records {
		if iso == nil || strings.TrimSpace(iso.Name) == "" {
			continue
		}
		dirName := makeArtifactDirName(iso.Name)
		if dirName == strings.TrimSpace(iso.Name) {
			continue
		}
		actual := filepath.Join(root, filepath.FromSlash(path.Join("artifacts", iso.Name)))
		if _, err := os.Stat(actual); err != nil {
			continue
		}
		alias := filepath.Join(root, filepath.FromSlash(path.Join("artifacts", dirName)))
		if _, err := os.Lstat(alias); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(alias), 0755); err != nil {
			return err
		}
		if err := os.Symlink(actual, alias); err != nil && !os.IsExist(err) {
			return err
		}
		s.log.Basicf("PXE artifact alias created %s -> %s\n", alias, actual)
	}
	return nil
}

func buildPublicURLFromIP(ip string, httpAddr string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	port := "80"
	addr := strings.TrimSpace(httpAddr)
	if addr != "" {
		if strings.HasPrefix(addr, ":") {
			port = strings.TrimPrefix(addr, ":")
		} else if _, p, err := net.SplitHostPort(addr); err == nil && strings.TrimSpace(p) != "" {
			port = p
		}
	}
	if port == "" || port == "80" {
		return fmt.Sprintf("http://%s", ip)
	}
	return fmt.Sprintf("http://%s:%s", ip, port)
}

func (s *Service) serverIdentifierIP() net.IP {
	if ip := parseIP(s.cfg.PXE.DHCPServer.ServerPublicAddress); ip != nil {
		return ip
	}
	host, _, err := net.SplitHostPort(s.cfg.PXE.DHCPServer.Address)
	if err == nil && strings.TrimSpace(host) != "" {
		if ip := net.ParseIP(host); ip != nil {
			return ip
		}
	}
	return nil
}

func (s *Service) ensureStage2Artifacts() {
	root := strings.TrimSpace(s.httpRoot)
	if root == "" {
		return
	}
	records, err := db.StoredISOImages.SelectAll()
	if err != nil {
		s.log.Warningf("PXE stage2 listing error: %v\n", err)
		return
	}
	for _, rec := range records {
		if rec == nil || strings.TrimSpace(rec.FullISOPath) == "" {
			continue
		}
		dest := filepath.Join(root, filepath.FromSlash(path.Join("artifacts", rec.Name, "stage2")))
		if _, err := os.Stat(filepath.Join(dest, ".treeinfo")); err == nil {
			continue
		}
		if err := isoextract.EnsureStage2Artifacts(rec.FullISOPath, dest); err != nil {
			s.log.Warningf("PXE stage2 prepare error for %s: %v\n", rec.Name, err)
		} else {
			s.log.Basicf("PXE stage2 artifacts prepared for %s\n", rec.Name)
		}
	}
}
