package pxe

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/opnlaas/opnlaas/db"
)

// hostCache provides a caching layer for Host records.
type hostCache struct {
	ttl     time.Duration
	expires time.Time
	mu      sync.RWMutex
	byIP    map[string]*db.Host
	bySlug  map[string]*db.Host
	byMAC   map[string]*db.Host
}

// newHostCache creates a new hostCache with the specified TTL.
func newHostCache(ttl time.Duration) (cache *hostCache) {
	cache = &hostCache{
		ttl: ttl,
	}

	return
}

// ensureFresh refreshes the cache if it is stale.
func (c *hostCache) ensureFresh() (err error) {
	c.mu.RLock()
	var ready bool = c.byIP != nil && time.Now().Before(c.expires)
	c.mu.RUnlock()

	if ready {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.byIP != nil && time.Now().Before(c.expires) {
		return
	}

	var hosts []*db.Host
	if hosts, err = db.Hosts.SelectAll(); err != nil {
		return
	}

	var byIP map[string]*db.Host = make(map[string]*db.Host, len(hosts))
	var bySlug map[string]*db.Host = make(map[string]*db.Host, len(hosts))
	var byMAC map[string]*db.Host = make(map[string]*db.Host)

	for _, host := range hosts {
		if host == nil {
			continue
		}

		if host.ManagementIP != "" {
			byIP[host.ManagementIP] = host
			var slug string
			if slug = makeHostSlug(host.ManagementIP); slug != "" {
				bySlug[slug] = host
			}
		}

		for _, nic := range host.NetworkInterfaces {
			var norm string
			if norm, err = normalizeMAC(nic.MACAddress); err == nil && norm != "" {
				byMAC[norm] = host
			}
		}
	}

	c.byIP = byIP
	c.bySlug = bySlug
	c.byMAC = byMAC
	c.expires = time.Now().Add(c.ttl)
	return
}

// ByMAC looks up a Host by its MAC address.
func (c *hostCache) ByMAC(mac string) (host *db.Host, err error) {
	var norm string
	if norm, err = normalizeMAC(mac); err != nil || norm == "" {
		return
	}

	if err = c.ensureFresh(); err != nil {
		return
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	host = c.byMAC[norm]
	return
}

// ByIP looks up a Host by its Management IP address.
func (c *hostCache) ByIP(ip string) (host *db.Host, err error) {
	if err = c.ensureFresh(); err != nil {
		return
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	host = c.byIP[ip]
	return
}

// BySlug looks up a Host by its slug.
func (c *hostCache) BySlug(slug string) (host *db.Host, err error) {
	if err = c.ensureFresh(); err != nil {
		return
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	host = c.bySlug[slug]
	return
}

// profileCache provides a caching layer for HostPXEProfile records.
type profileCache struct {
	ttl     time.Duration
	expires time.Time
	mu      sync.RWMutex

	byIP  map[string]*db.HostPXEProfile
	byMAC map[string]*db.HostPXEProfile
}

// newProfileCache creates a new profileCache with the specified TTL.
func newProfileCache(ttl time.Duration) (cache *profileCache) {
	cache = &profileCache{
		ttl: ttl,
	}
	return
}

// ensureFresh refreshes the cache if it is stale.
func (c *profileCache) ensureFresh() (err error) {
	c.mu.RLock()
	var ready bool = c.byIP != nil && time.Now().Before(c.expires)
	c.mu.RUnlock()

	if ready {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.byIP != nil && time.Now().Before(c.expires) {
		return
	}

	var records []*db.HostPXEProfile
	if records, err = db.PXEProfilesAll(); err != nil {
		return
	}

	var byIP map[string]*db.HostPXEProfile = make(map[string]*db.HostPXEProfile, len(records))
	var byMAC map[string]*db.HostPXEProfile = make(map[string]*db.HostPXEProfile)

	for _, prof := range records {
		if prof == nil {
			continue
		}

		if prof.ManagementIP != "" {
			byIP[prof.ManagementIP] = prof
		}

		var norm string
		if norm, err = normalizeMAC(prof.BootMACAddress); err == nil && norm != "" {
			byMAC[norm] = prof
		}
	}

	c.byIP = byIP
	c.byMAC = byMAC
	c.expires = time.Now().Add(c.ttl)
	return
}

// ByIP looks up a HostPXEProfile by its Management IP address.
func (c *profileCache) ByIP(ip string) (profile *db.HostPXEProfile, err error) {
	if err = c.ensureFresh(); err != nil {
		return
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	profile = c.byIP[ip]
	return
}

func (c *profileCache) ByMAC(mac string) (profile *db.HostPXEProfile, err error) {
	var norm string
	if norm, err = normalizeMAC(mac); err != nil || norm == "" {
		return
	}

	if err = c.ensureFresh(); err != nil {
		return
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	profile = c.byMAC[norm]
	return
}

// leaseStore manages DHCP leases mapping MAC addresses to IP addresses.
type leaseStore struct {
	mu      sync.RWMutex
	macToIP map[string]net.IP
	ipToMAC map[string]string
}

// newLeaseStore creates a new leaseStore.
func newLeaseStore() (store *leaseStore) {
	store = &leaseStore{
		macToIP: make(map[string]net.IP),
		ipToMAC: make(map[string]string),
	}
	return
}

// Set assigns the given IP address to the specified MAC address in the lease store.
func (l *leaseStore) Set(mac string, ip net.IP) {
	mac = strings.ToLower(strings.TrimSpace(mac))
	if mac == "" {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	var (
		prev   net.IP
		ok     bool
		copyIP net.IP
	)

	if prev, ok = l.macToIP[mac]; ok && prev != nil {
		delete(l.ipToMAC, prev.String())
	}

	if ip == nil {
		delete(l.macToIP, mac)
		return
	}

	if copyIP = cloneIPv4(ip); copyIP == nil {
		copyIP = append(net.IP(nil), ip...)
	}

	l.macToIP[mac] = copyIP
	l.ipToMAC[copyIP.String()] = mac
}

// Get retrieves the IP address assigned to the specified MAC address.
func (l *leaseStore) Get(mac string) (ip net.IP) {
	if mac = strings.ToLower(strings.TrimSpace(mac)); mac == "" {
		return
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	var ok bool
	if ip, ok = l.macToIP[mac]; ok && ip != nil {
		ip = cloneIPv4(ip)
	}

	return
}

// InUse checks if the given IP address is currently leased to any MAC address.
func (l *leaseStore) InUse(ip net.IP) (inUse bool) {
	if ip == nil {
		inUse = false
		return
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	_, inUse = l.ipToMAC[ip.String()]
	return
}
