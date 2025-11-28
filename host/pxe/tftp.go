package pxe

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func (s *Service) handleTFTPRequest(filename string) ([]byte, error) {
	clean := strings.TrimLeft(filename, "/")
	if clean == "" {
		return nil, fmt.Errorf("invalid filename")
	}
	if strings.Contains(clean, "..") {
		return nil, fmt.Errorf("invalid path")
	}

	var remote *net.UDPAddr
	// pin/tftp does not expose remote info directly; leave nil for now.
	reqCtx := &tftpRequestContext{
		filename:   clean,
		remoteAddr: remote,
	}

	if data, handled, err := s.handlePXELinux(clean, reqCtx); handled {
		return data, err
	}

	if strings.HasPrefix(strings.ToLower(clean), "profiles/") {
		return s.serveProfileFile(clean)
	}

	// Static file fallback.
	data, err := os.ReadFile(filepath.Join(s.tftpRoot, filepath.FromSlash(clean)))
	if err != nil {
		return nil, err
	}
	s.log.Basicf("TFTP static served %s (%d bytes)\n", clean, len(data))
	return data, nil
}
