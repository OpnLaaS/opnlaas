package db

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
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

type BookingCart struct {
	Owner     string                        `json:"owner"`
	Hosts     map[string]BookingRequestHost `json:"hosts"`
	UpdatedAt time.Time                     `json:"updated_at"`
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
	var hosts []*Host
	if hosts, err = Hosts.SelectAll(); err != nil {
		return
	}

	for _, h := range hosts {
		if h.IsBooked && h.ActiveBookingID != 0 {
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

// AddHostToCart validates and reserves a host in the owner's cart.
func AddHostToCart(owner string, host BookingRequestHost) (err error) {
	bookingCartLock.Lock()
	defer bookingCartLock.Unlock()

	var dbHost *Host
	if dbHost, err = Hosts.Select(host.ManagementIP); err != nil {
		return
	}

	if dbHost == nil {
		err = fmt.Errorf("host %s not found", host.ManagementIP)
		return
	}

	if dbHost.IsBooked {
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

	cart := getOrCreateCart(owner)
	cart.Hosts[host.ManagementIP] = host
	cart.UpdatedAt = time.Now()
	hostCartOwners[host.ManagementIP] = owner
	return
}

// RemoveHostFromCart releases a host reservation from the owner's cart.
func RemoveHostFromCart(owner string, managementIP string) {
	bookingCartLock.Lock()
	defer bookingCartLock.Unlock()

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
