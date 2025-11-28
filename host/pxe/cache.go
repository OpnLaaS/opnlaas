package pxe

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/opnlaas/opnlaas/db"
)

type hostCache struct {
	ttl     time.Duration
	expires time.Time
	mu      sync.RWMutex

	byIP   map[string]*db.Host
	bySlug map[string]*db.Host
	byMAC  map[string]*db.Host
}

func newHostCache(ttl time.Duration) *hostCache {
	return &hostCache{ttl: ttl}
}

func (c *hostCache) ensureFresh() error {
	c.mu.RLock()
	ready := c.byIP != nil && time.Now().Before(c.expires)
	c.mu.RUnlock()
	if ready {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byIP != nil && time.Now().Before(c.expires) {
		return nil
	}

	hosts, err := db.Hosts.SelectAll()
	if err != nil {
		return err
	}

	byIP := make(map[string]*db.Host, len(hosts))
	bySlug := make(map[string]*db.Host, len(hosts))
	byMAC := make(map[string]*db.Host)

	for _, host := range hosts {
		if host == nil {
			continue
		}
		if host.ManagementIP != "" {
			byIP[host.ManagementIP] = host
			if slug := makeHostSlug(host.ManagementIP); slug != "" {
				bySlug[slug] = host
			}
		}
		for _, nic := range host.NetworkInterfaces {
			if norm, err := normalizeMAC(nic.MACAddress); err == nil && norm != "" {
				byMAC[norm] = host
			}
		}
	}

	c.byIP = byIP
	c.bySlug = bySlug
	c.byMAC = byMAC
	c.expires = time.Now().Add(c.ttl)
	return nil
}

func (c *hostCache) ByMAC(mac string) (*db.Host, error) {
	norm, err := normalizeMAC(mac)
	if err != nil || norm == "" {
		return nil, err
	}
	if err := c.ensureFresh(); err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.byMAC[norm], nil
}

func (c *hostCache) ByIP(ip string) (*db.Host, error) {
	if err := c.ensureFresh(); err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.byIP[ip], nil
}

func (c *hostCache) BySlug(slug string) (*db.Host, error) {
	if err := c.ensureFresh(); err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.bySlug[strings.ToLower(slug)], nil
}

type profileCache struct {
	ttl     time.Duration
	expires time.Time
	mu      sync.RWMutex

	byIP  map[string]*db.HostPXEProfile
	byMAC map[string]*db.HostPXEProfile
}

func newProfileCache(ttl time.Duration) *profileCache {
	return &profileCache{ttl: ttl}
}

func (c *profileCache) ensureFresh() error {
	c.mu.RLock()
	ready := c.byIP != nil && time.Now().Before(c.expires)
	c.mu.RUnlock()
	if ready {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byIP != nil && time.Now().Before(c.expires) {
		return nil
	}

	records, err := db.PXEProfilesAll()
	if err != nil {
		return err
	}

	byIP := make(map[string]*db.HostPXEProfile, len(records))
	byMAC := make(map[string]*db.HostPXEProfile)
	for _, prof := range records {
		if prof == nil {
			continue
		}
		if prof.ManagementIP != "" {
			byIP[prof.ManagementIP] = prof
		}
		if norm, err := normalizeMAC(prof.BootMACAddress); err == nil && norm != "" {
			byMAC[norm] = prof
		}
	}

	c.byIP = byIP
	c.byMAC = byMAC
	c.expires = time.Now().Add(c.ttl)
	return nil
}

func (c *profileCache) ByIP(ip string) (*db.HostPXEProfile, error) {
	if err := c.ensureFresh(); err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.byIP[ip], nil
}

func (c *profileCache) ByMAC(mac string) (*db.HostPXEProfile, error) {
	norm, err := normalizeMAC(mac)
	if err != nil || norm == "" {
		return nil, err
	}
	if err := c.ensureFresh(); err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.byMAC[norm], nil
}

type leaseStore struct {
	mu     sync.RWMutex
	leases map[string]net.IP
}

func newLeaseStore() *leaseStore {
	return &leaseStore{leases: make(map[string]net.IP)}
}

func (l *leaseStore) Set(mac string, ip net.IP) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.leases[strings.ToLower(mac)] = ip
}

func (l *leaseStore) Get(mac string) net.IP {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.leases[strings.ToLower(mac)]
}
