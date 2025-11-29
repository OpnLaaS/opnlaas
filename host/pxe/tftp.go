package pxe

import (
	"fmt"
	"net"
	"os"
	"strings"
)

// handleTFTPRequest processes an incoming TFTP request for the given filename.
func (s *Service) handleTFTPRequest(filename string, remote *net.UDPAddr) (response []byte, err error) {
	if filename = strings.TrimLeft(filename, "/"); filename == "" {
		err = fmt.Errorf("invalid filename")
		return
	}

	if strings.Contains(filename, "..") {
		err = fmt.Errorf("invalid path")
		return
	}

	var (
		ctx *tftpRequestContext = &tftpRequestContext{
			filename:   filename,
			remoteAddr: remote,
		}

		handled bool
	)

	if response, handled, err = s.handlePXELinux(filename, ctx); handled {
		return
	}

	if strings.HasPrefix(strings.ToLower(filename), "profiles/") {
		response, err = s.serveProfileFile(filename)
		return
	}

	var full string = fmt.Sprintf("%s/%s", s.tftpRoot, filename)
	if response, err = os.ReadFile(full); err != nil {
		s.log.Errorf("TFTP static miss %s (full=%s): %v\n", filename, full, err)
		return
	}

	s.log.Basicf("TFTP static served %s (%d bytes)\n", filename, len(response))
	return
}
