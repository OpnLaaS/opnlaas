package db

import (
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/z46-dev/gomysql"
)

var (
	bookingCreationLock sync.Mutex
	bookingLocks        sync.Map
	bookingCartLock     sync.Mutex

	bookingCarts   = map[string]*BookingCart{}
	hostCartOwners = map[string]string{}
)

var (
	ErrBookingNotFound   = errors.New("booking not found")
	ErrHostAlreadyBooked = errors.New("host already booked or reserved")
	ErrCartNotFound      = errors.New("cart not found")
)

func cleanupStaleCartReservationsLocked() {
	for ip, holder := range hostCartOwners {
		cart, ok := bookingCarts[holder]
		if !ok || cart == nil {
			delete(hostCartOwners, ip)
			continue
		}

		if _, exists := cart.Hosts[ip]; !exists {
			delete(hostCartOwners, ip)
		}
	}
}

func normalizeHostBookingState(host *Host) (isBooked bool, err error) {
	if host == nil {
		return false, nil
	}

	if !host.IsBooked {
		return false, nil
	}

	if host.ActiveBookingID <= 0 {
		host.IsBooked = false
		host.ActiveBookingID = 0
		host.AssignedIPv4 = ""
		host.AssignedCIDR = ""
		err = Hosts.Update(host)
		return false, err
	}

	var booking *Booking
	if booking, err = BookingByID(host.ActiveBookingID); err != nil {
		return true, err
	}

	if booking == nil {
		host.IsBooked = false
		host.ActiveBookingID = 0
		host.AssignedIPv4 = ""
		host.AssignedCIDR = ""
		err = Hosts.Update(host)
		return false, err
	}

	return true, nil
}

type BookingCart struct {
	Owner       string                        `json:"owner"`
	Hosts       map[string]BookingRequestHost `json:"hosts"`
	NetworkCIDR string                        `json:"network_cidr,omitempty"`
	GatewayIPv4 string                        `json:"gateway_ipv4,omitempty"`
	DNSServers  []string                      `json:"dns_servers,omitempty"`
	UpdatedAt   time.Time                     `json:"updated_at"`
}

// newBookingCart allocates a fresh cart for an owner.
func newBookingCart(owner string) *BookingCart {
	return &BookingCart{
		Owner:     owner,
		Hosts:     map[string]BookingRequestHost{},
		UpdatedAt: time.Now(),
	}
}

// clone returns a deep copy of the cart contents.
func (c *BookingCart) clone() *BookingCart {
	if c == nil {
		return nil
	}

	cloned := newBookingCart(c.Owner)
	maps.Copy(cloned.Hosts, c.Hosts)
	cloned.NetworkCIDR = c.NetworkCIDR
	cloned.GatewayIPv4 = c.GatewayIPv4
	cloned.DNSServers = append([]string(nil), c.DNSServers...)
	cloned.UpdatedAt = c.UpdatedAt

	return cloned
}

// bookingMutex returns a per-booking mutex to coordinate concurrent updates.
func bookingMutex(id int) *sync.Mutex {
	m, _ := bookingLocks.LoadOrStore(id, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// withBookingLock executes fn under a per-booking mutex.
func withBookingLock(bookingID int, fn func() error) error {
	mutex := bookingMutex(bookingID)
	mutex.Lock()
	defer mutex.Unlock()
	return fn()
}

func appendUniqueInt(values []int, v int) []int {
	if slices.Contains(values, v) {
		return values
	}

	return append(values, v)
}

func appendUniqueString(values []string, v string) []string {
	if slices.Contains(values, v) {
		return values
	}

	return append(values, v)
}

func cartNetworkPrefix(c *BookingCart) (prefix netip.Prefix, ok bool) {
	if c == nil || c.NetworkCIDR == "" {
		return netip.Prefix{}, false
	}

	parsed, err := netip.ParsePrefix(c.NetworkCIDR)
	if err != nil || !parsed.Addr().Is4() {
		return netip.Prefix{}, false
	}

	return parsed.Masked(), true
}

func ipv4NetworkAndBroadcast(prefix netip.Prefix) (network netip.Addr, broadcast netip.Addr, err error) {
	if !prefix.Addr().Is4() {
		return network, broadcast, fmt.Errorf("prefix %s is not ipv4", prefix.String())
	}

	base := prefix.Masked().Addr().As4()
	baseUint := (uint32(base[0]) << 24) | (uint32(base[1]) << 16) | (uint32(base[2]) << 8) | uint32(base[3])
	hostBits := uint32(32 - prefix.Bits())
	size := uint32(1) << hostBits
	lastUint := baseUint + size - 1

	network = netip.AddrFrom4(base)
	broadcast = netip.AddrFrom4([4]byte{
		byte(lastUint >> 24),
		byte(lastUint >> 16),
		byte(lastUint >> 8),
		byte(lastUint),
	})
	return
}

func validateCartHostAssignedIPLocked(cart *BookingCart, host BookingRequestHost) (err error) {
	if cart == nil {
		return nil
	}

	var assigned string = host.AssignedIPv4
	if assigned == "" {
		return nil
	}

	prefix, ok := cartNetworkPrefix(cart)
	if !ok {
		return fmt.Errorf("set booking network before assigning per-host IPs")
	}

	addr, addrErr := netip.ParseAddr(assigned)
	if addrErr != nil || !addr.Is4() {
		return fmt.Errorf("assigned_ipv4 %s is invalid", assigned)
	}

	if !prefix.Contains(addr) {
		return fmt.Errorf("assigned_ipv4 %s is outside cart network %s", assigned, prefix.String())
	}

	network, broadcast, nbErr := ipv4NetworkAndBroadcast(prefix)
	if nbErr != nil {
		return nbErr
	}

	if addr == network || addr == broadcast {
		return fmt.Errorf("assigned_ipv4 %s cannot be network or broadcast address", assigned)
	}

	if gateway := cart.GatewayIPv4; gateway != "" && strings.TrimSpace(gateway) == assigned {
		return fmt.Errorf("assigned_ipv4 %s conflicts with gateway", assigned)
	}

	for managementIP, existing := range cart.Hosts {
		if managementIP == host.ManagementIP {
			continue
		}

		if existing.AssignedIPv4 != "" && strings.EqualFold(existing.AssignedIPv4, assigned) {
			return fmt.Errorf("assigned_ipv4 %s already used by host %s", assigned, managementIP)
		}
	}

	return nil
}

func removeInt(values []int, target int) []int {
	filtered := values[:0]
	for _, v := range values {
		if v != target {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

func removeString(values []string, target string) []string {
	filtered := values[:0]
	for _, v := range values {
		if v != target {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

func bookingPersonsFor(username string) (records []*BookingPerson, err error) {
	records, err = bookingPeople.SelectAllWithFilter(gomysql.NewFilter().KeyCmp(bookingPeople.FieldBySQLName("username"), gomysql.OpEqual, username))
	return
}

func insertBookingPerson(record *BookingPerson) (err error) {
	err = bookingPeople.Insert(record)
	return
}

func deleteBookingPerson(record *BookingPerson) (err error) {
	err = bookingPeople.Delete(record.ID)
	return
}

// Booking lifecycle helpers

// CreateBooking inserts a new booking record, defaulting the start time when missing.
func CreateBooking(record *Booking) (err error) {
	bookingCreationLock.Lock()
	defer bookingCreationLock.Unlock()

	if record.StartTime.IsZero() {
		record.StartTime = time.Now()
	}

	err = bookings.Insert(record)
	return
}

// BookingByID fetches a booking by its ID.
func BookingByID(bookingID int) (record *Booking, err error) {
	record, err = bookings.Select(bookingID)
	return
}

// BookingList returns all bookings.
func BookingList() (records []*Booking, err error) {
	records, err = bookings.SelectAll()
	return
}

// UpdateBooking updates a booking using its per-booking lock for safety.
func UpdateBooking(record *Booking) (err error) {
	err = withBookingLock(record.ID, func() error {
		return bookings.Update(record)
	})
	return
}

// DeleteBooking deletes a booking using its per-booking lock.
func DeleteBooking(bookingID int) (err error) {
	err = withBookingLock(bookingID, func() error {
		return bookings.Delete(bookingID)
	})
	return
}

// DeleteBookingCascade removes a booking and its linked records, then clears the per-booking lock entry.
func DeleteBookingCascade(bookingID int) (err error) {
	err = withBookingLock(bookingID, func() error {
		booking, err := bookings.Select(bookingID)
		if err != nil {
			return err
		}

		if booking == nil {
			return ErrBookingNotFound
		}

		for _, managementIP := range booking.OwnedHostManagementIPs {
			host, hostErr := Hosts.Select(managementIP)
			if hostErr != nil {
				return hostErr
			}

			if host == nil || host.ActiveBookingID != bookingID {
				continue
			}

			host.IsBooked = false
			host.ActiveBookingID = 0
			host.AssignedIPv4 = ""
			host.AssignedCIDR = ""
			if updateErr := Hosts.Update(host); updateErr != nil {
				return updateErr
			}
		}

		people, err := bookingPeople.SelectAllWithFilter(gomysql.NewFilter().KeyCmp(bookingPeople.FieldBySQLName("booking_id"), gomysql.OpEqual, bookingID))
		if err != nil {
			return err
		}
		for _, person := range people {
			if person == nil {
				continue
			}

			if err = bookingPeople.Delete(person.ID); err != nil {
				return err
			}
		}

		requests, err := bookingRequests.SelectAllWithFilter(gomysql.NewFilter().KeyCmp(bookingRequests.FieldBySQLName("booking_id"), gomysql.OpEqual, bookingID))
		if err != nil {
			return err
		}
		for _, request := range requests {
			if request == nil {
				continue
			}

			if err = bookingRequests.Delete(request.ID); err != nil {
				return err
			}
		}

		containers, err := bookingContainers.SelectAllWithFilter(gomysql.NewFilter().KeyCmp(bookingContainers.FieldBySQLName("booking_id"), gomysql.OpEqual, bookingID))
		if err != nil {
			return err
		}
		for _, container := range containers {
			if container == nil {
				continue
			}

			if err = bookingContainers.Delete(container.ProxmoxID); err != nil {
				return err
			}
		}

		vms, err := bookingVMs.SelectAllWithFilter(gomysql.NewFilter().KeyCmp(bookingVMs.FieldBySQLName("booking_id"), gomysql.OpEqual, bookingID))
		if err != nil {
			return err
		}
		for _, vm := range vms {
			if vm == nil {
				continue
			}

			if err = bookingVMs.Delete(vm.ProxmoxID); err != nil {
				return err
			}
		}

		if err = bookings.Delete(bookingID); err != nil {
			return err
		}

		bookingLocks.Delete(bookingID)
		return nil
	})
	return
}

// Booking people helpers

// BookingPersonsFor returns all booking roles for a given username.
func BookingPersonsFor(username string) (records []*BookingPerson, err error) {
	records, err = bookingPersonsFor(username)
	return
}

// BookingPeopleForBooking returns all people tied to a booking.
func BookingPeopleForBooking(bookingID int) (records []*BookingPerson, err error) {
	records, err = bookingPeople.SelectAllWithFilter(gomysql.NewFilter().KeyCmp(bookingPeople.FieldBySQLName("booking_id"), gomysql.OpEqual, bookingID))
	return
}

// AddBookingPerson adds a booking-person relation and mirrors it on the booking.
func AddBookingPerson(record *BookingPerson) (err error) {
	err = withBookingLock(record.BookingID, func() error {
		booking, err := bookings.Select(record.BookingID)
		if err != nil {
			return err
		}

		if booking == nil {
			return ErrBookingNotFound
		}

		if err = insertBookingPerson(record); err != nil {
			return err
		}

		booking.People = appendUniqueInt(booking.People, record.ID)
		return bookings.Update(booking)
	})
	return
}

// RemoveBookingPerson deletes a booking-person relation and updates the booking record.
func RemoveBookingPerson(bookingPersonID int) (err error) {
	var record *BookingPerson

	if record, err = bookingPeople.Select(bookingPersonID); err != nil || record == nil {
		return
	}

	err = withBookingLock(record.BookingID, func() error {
		if err := deleteBookingPerson(record); err != nil {
			return err
		}

		booking, err := bookings.Select(record.BookingID)
		if err != nil {
			return err
		}

		if booking == nil {
			return ErrBookingNotFound
		}

		booking.People = removeInt(booking.People, record.ID)
		return bookings.Update(booking)
	})
	return
}

// Booking request helpers

// BookingRequestsForBooking lists all requests for a booking.
func BookingRequestsForBooking(bookingID int) (records []*BookingRequest, err error) {
	records, err = bookingRequests.SelectAllWithFilter(gomysql.NewFilter().KeyCmp(bookingRequests.FieldBySQLName("booking_id"), gomysql.OpEqual, bookingID))
	return
}

// AddBookingRequest inserts a request and marks the booking pending if needed.
func AddBookingRequest(record *BookingRequest) (err error) {
	if record.RequestedAt.IsZero() {
		record.RequestedAt = time.Now()
	}

	if record.Status == 0 {
		record.Status = BookingRequestStatusPending
	}

	err = withBookingLock(record.BookingID, func() error {
		booking, err := bookings.Select(record.BookingID)
		if err != nil {
			return err
		}

		if booking == nil {
			return ErrBookingNotFound
		}

		if err := bookingRequests.Insert(record); err != nil {
			return err
		}

		booking.Requests = appendUniqueInt(booking.Requests, record.ID)
		if booking.Status != BookingStatusActiveWithRequestPending {
			booking.Status = BookingStatusActiveWithRequestPending
		}

		return bookings.Update(booking)
	})
	return
}

// UpdateBookingRequest updates an existing booking request.
func UpdateBookingRequest(record *BookingRequest) (err error) {
	err = withBookingLock(record.BookingID, func() error {
		return bookingRequests.Update(record)
	})
	return
}

// DeleteBookingRequest removes a request and detaches it from the booking.
func DeleteBookingRequest(requestID int) (err error) {
	var record *BookingRequest

	if record, err = bookingRequests.Select(requestID); err != nil || record == nil {
		return
	}

	err = withBookingLock(record.BookingID, func() error {
		if err := bookingRequests.Delete(requestID); err != nil {
			return err
		}

		booking, err := bookings.Select(record.BookingID)
		if err != nil {
			return err
		}

		if booking == nil {
			return ErrBookingNotFound
		}

		booking.Requests = removeInt(booking.Requests, requestID)
		return bookings.Update(booking)
	})
	return
}

// Booking resource helpers

// BookingContainersForBooking lists containers attached to a booking.
func BookingContainersForBooking(bookingID int) (records []*BookingContainer, err error) {
	records, err = bookingContainers.SelectAllWithFilter(gomysql.NewFilter().KeyCmp(bookingContainers.FieldBySQLName("booking_id"), gomysql.OpEqual, bookingID))
	return
}

// AddBookingContainer inserts a container record and tracks ownership on the booking.
func AddBookingContainer(record *BookingContainer) (err error) {
	err = withBookingLock(record.BookingID, func() error {
		booking, err := bookings.Select(record.BookingID)
		if err != nil {
			return err
		}

		if booking == nil {
			return ErrBookingNotFound
		}

		if err := bookingContainers.Insert(record); err != nil {
			return err
		}

		booking.OwnedBookingCTIDs = appendUniqueInt(booking.OwnedBookingCTIDs, record.ProxmoxID)
		return bookings.Update(booking)
	})
	return
}

// RemoveBookingContainer deletes a container and removes it from the booking owner list.
func RemoveBookingContainer(proxmoxID int) (err error) {
	var record *BookingContainer

	if record, err = bookingContainers.Select(proxmoxID); err != nil || record == nil {
		return
	}

	err = withBookingLock(record.BookingID, func() error {
		if err := bookingContainers.Delete(proxmoxID); err != nil {
			return err
		}

		booking, err := bookings.Select(record.BookingID)
		if err != nil {
			return err
		}

		if booking == nil {
			return ErrBookingNotFound
		}

		booking.OwnedBookingCTIDs = removeInt(booking.OwnedBookingCTIDs, proxmoxID)
		return bookings.Update(booking)
	})
	return
}

// BookingVMsForBooking lists VMs attached to a booking.
func BookingVMsForBooking(bookingID int) (records []*BookingVM, err error) {
	records, err = bookingVMs.SelectAllWithFilter(gomysql.NewFilter().KeyCmp(bookingVMs.FieldBySQLName("booking_id"), gomysql.OpEqual, bookingID))
	return
}

// AddBookingVM inserts a VM record and tracks ownership on the booking.
func AddBookingVM(record *BookingVM) (err error) {
	err = withBookingLock(record.BookingID, func() error {
		booking, err := bookings.Select(record.BookingID)
		if err != nil {
			return err
		}

		if booking == nil {
			return ErrBookingNotFound
		}

		if err := bookingVMs.Insert(record); err != nil {
			return err
		}

		booking.OwnedBookingVMIDs = appendUniqueInt(booking.OwnedBookingVMIDs, record.ProxmoxID)
		return bookings.Update(booking)
	})
	return
}

// RemoveBookingVM deletes a VM and removes it from the booking owner list.
func RemoveBookingVM(proxmoxID int) (err error) {
	var record *BookingVM

	if record, err = bookingVMs.Select(proxmoxID); err != nil || record == nil {
		return
	}

	err = withBookingLock(record.BookingID, func() error {
		if err := bookingVMs.Delete(proxmoxID); err != nil {
			return err
		}

		booking, err := bookings.Select(record.BookingID)
		if err != nil {
			return err
		}

		if booking == nil {
			return ErrBookingNotFound
		}

		booking.OwnedBookingVMIDs = removeInt(booking.OwnedBookingVMIDs, proxmoxID)
		return bookings.Update(booking)
	})
	return
}

// AssignHostToBooking reserves a host for the booking and updates host state.
func AssignHostToBooking(bookingID int, managementIP string) (err error) {
	err = withBookingLock(bookingID, func() error {
		booking, err := bookings.Select(bookingID)
		if err != nil {
			return err
		}

		if booking == nil {
			return ErrBookingNotFound
		}

		host, err := Hosts.Select(managementIP)
		if err != nil {
			return err
		}

		if host == nil {
			return fmt.Errorf("host %s not found", managementIP)
		}

		if host.IsBooked && host.ActiveBookingID != bookingID {
			return ErrHostAlreadyBooked
		}

		host.IsBooked = true
		host.ActiveBookingID = bookingID

		if err := Hosts.Update(host); err != nil {
			return err
		}

		booking.OwnedHostManagementIPs = appendUniqueString(booking.OwnedHostManagementIPs, managementIP)
		return bookings.Update(booking)
	})
	return
}

// ReleaseHostFromBooking frees a host and removes it from the booking.
func ReleaseHostFromBooking(bookingID int, managementIP string) (err error) {
	err = withBookingLock(bookingID, func() error {
		booking, err := bookings.Select(bookingID)
		if err != nil {
			return err
		}

		if booking == nil {
			return ErrBookingNotFound
		}

		host, err := Hosts.Select(managementIP)
		if err != nil {
			return err
		}

		if host != nil {
			host.IsBooked = false
			host.ActiveBookingID = 0
			host.AssignedIPv4 = ""
			host.AssignedCIDR = ""
			if err := Hosts.Update(host); err != nil {
				return err
			}
		}

		booking.OwnedHostManagementIPs = removeString(booking.OwnedHostManagementIPs, managementIP)
		return bookings.Update(booking)
	})
	return
}

// Booking cart helpers

// availableHostsForCart filters hosts that are free or reserved by the owner.
func availableHostsForCart(owner string) (records []*Host, err error) {
	bookingCartLock.Lock()
	defer bookingCartLock.Unlock()
	cleanupStaleCartReservationsLocked()

	var hosts []*Host
	if hosts, err = Hosts.SelectAll(); err != nil {
		return
	}

	for _, h := range hosts {
		var booked bool
		if booked, err = normalizeHostBookingState(h); err != nil {
			return
		}

		if booked {
			continue
		}

		if holder, reserved := hostCartOwners[h.ManagementIP]; reserved && holder != owner {
			continue
		}

		records = append(records, h)
	}

	sort.Slice(records, func(i, j int) bool { return records[i].ManagementIP < records[j].ManagementIP })
	return
}

// AvailableHostsForCart is the exported helper for cart host discovery.
func AvailableHostsForCart(owner string) (records []*Host, err error) {
	records, err = availableHostsForCart(owner)
	return
}

// AvailableISOImages lists stored ISOs for host installation selection.
func AvailableISOImages() (records []*StoredISOImage, err error) {
	records, err = StoredISOImages.SelectAll()
	return
}

// getOrCreateCart fetches the owner's cart or initializes a new one.
func getOrCreateCart(owner string) *BookingCart {
	cart, ok := bookingCarts[owner]
	if !ok || cart == nil {
		cart = newBookingCart(owner)
		bookingCarts[owner] = cart
	}

	return cart
}

// BookingCartSnapshot returns a safe copy of the owner's cart.
func BookingCartSnapshot(owner string) (cart *BookingCart, err error) {
	bookingCartLock.Lock()
	defer bookingCartLock.Unlock()

	cart, ok := bookingCarts[owner]
	if !ok || cart == nil {
		err = ErrCartNotFound
		return
	}

	cart = cart.clone()
	return
}

// ResetBookingCart clears the cart and host reservations for an owner.
func ResetBookingCart(owner string) {
	bookingCartLock.Lock()
	defer bookingCartLock.Unlock()

	if cart, ok := bookingCarts[owner]; ok && cart != nil {
		for ip, holder := range hostCartOwners {
			if holder == owner {
				delete(hostCartOwners, ip)
			}
		}

		delete(bookingCarts, owner)
	}
}

// SetCartNetwork configures booking-network metadata for a cart.
func SetCartNetwork(owner string, networkCIDR string, gatewayIPv4 string, dnsServers []string) (err error) {
	bookingCartLock.Lock()
	defer bookingCartLock.Unlock()
	cleanupStaleCartReservationsLocked()

	cart := getOrCreateCart(owner)

	networkCIDR = strings.TrimSpace(networkCIDR)
	if networkCIDR == "" {
		return fmt.Errorf("network_cidr is required")
	}

	parsedPrefix, parseErr := netip.ParsePrefix(networkCIDR)
	if parseErr != nil || !parsedPrefix.Addr().Is4() {
		return fmt.Errorf("network_cidr must be valid IPv4 CIDR")
	}
	parsedPrefix = parsedPrefix.Masked()

	gatewayIPv4 = strings.TrimSpace(gatewayIPv4)
	if gatewayIPv4 != "" {
		gatewayAddr, gwErr := netip.ParseAddr(gatewayIPv4)
		if gwErr != nil || !gatewayAddr.Is4() {
			return fmt.Errorf("gateway_ipv4 must be valid IPv4 address")
		}
		gatewayIPv4 = gatewayAddr.String()
	}

	cart.NetworkCIDR = parsedPrefix.String()
	cart.GatewayIPv4 = gatewayIPv4
	cart.DNSServers = append([]string(nil), dnsServers...)

	for _, host := range cart.Hosts {
		if err = validateCartHostAssignedIPLocked(cart, host); err != nil {
			return
		}
	}

	cart.UpdatedAt = time.Now()
	return nil
}

// AddHostToCart validates and reserves a host in the owner's cart.
func AddHostToCart(owner string, host BookingRequestHost) (err error) {
	bookingCartLock.Lock()
	defer bookingCartLock.Unlock()
	cleanupStaleCartReservationsLocked()

	var dbHost *Host
	if dbHost, err = Hosts.Select(host.ManagementIP); err != nil {
		return
	}

	if dbHost == nil {
		err = fmt.Errorf("host %s not found", host.ManagementIP)
		return
	}

	var booked bool
	if booked, err = normalizeHostBookingState(dbHost); err != nil {
		return
	}

	if booked {
		err = ErrHostAlreadyBooked
		return
	}

	if holder, reserved := hostCartOwners[host.ManagementIP]; reserved && holder != owner {
		err = ErrHostAlreadyBooked
		return
	}

	if host.ISOSelection != "" {
		if iso, errIso := StoredISOImages.Select(host.ISOSelection); errIso != nil {
			err = errIso
			return
		} else if iso == nil {
			err = fmt.Errorf("iso %s not found", host.ISOSelection)
			return
		}
	}

	host.AssignedIPv4 = strings.TrimSpace(host.AssignedIPv4)
	if host.AssignedIPv4 != "" {
		if parsed, parseErr := netip.ParseAddr(host.AssignedIPv4); parseErr != nil || !parsed.Is4() {
			return fmt.Errorf("assigned_ipv4 %s is invalid", host.AssignedIPv4)
		} else {
			host.AssignedIPv4 = parsed.String()
		}
	}

	cart := getOrCreateCart(owner)
	if err = validateCartHostAssignedIPLocked(cart, host); err != nil {
		return
	}

	cart.Hosts[host.ManagementIP] = host
	cart.UpdatedAt = time.Now()
	hostCartOwners[host.ManagementIP] = owner
	return
}

// RemoveHostFromCart releases a host reservation from the owner's cart.
func RemoveHostFromCart(owner string, managementIP string) {
	bookingCartLock.Lock()
	defer bookingCartLock.Unlock()
	cleanupStaleCartReservationsLocked()

	if cart, ok := bookingCarts[owner]; ok && cart != nil {
		delete(cart.Hosts, managementIP)
		if holder, reserved := hostCartOwners[managementIP]; reserved && holder == owner {
			delete(hostCartOwners, managementIP)
		}
		cart.UpdatedAt = time.Now()
	}
}

// CartCounts returns the number of hosts in the cart (virtual always zero).
func CartCounts(owner string) (hostCount int, virtualCount int, err error) {
	bookingCartLock.Lock()
	cleanupStaleCartReservationsLocked()
	bookingCartLock.Unlock()

	var cart *BookingCart
	if cart, err = BookingCartSnapshot(owner); err != nil {
		return
	}

	hostCount = len(cart.Hosts)
	virtualCount = 0
	return
}

// BuildBookingRequestFromCart builds a request using cart hosts plus provided virtual resources.
func BuildBookingRequestFromCart(owner string, bookingID int, justification string, requestedBy string, containers []BookingRequestCT, vms []BookingRequestVM) (request *BookingRequest, err error) {
	var cart *BookingCart
	if cart, err = BookingCartSnapshot(owner); err != nil {
		return
	}

	request = &BookingRequest{
		BookingID:     bookingID,
		RequestedAt:   time.Now(),
		RequestedBy:   requestedBy,
		Justification: justification,
		Status:        BookingRequestStatusPending,
	}

	for _, host := range cart.Hosts {
		request.Hosts = append(request.Hosts, host)
	}

	request.Containers = append(request.Containers, containers...)
	request.VMs = append(request.VMs, vms...)

	return
}
