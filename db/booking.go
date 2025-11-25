package db

import (
	"time"

	"github.com/z46-dev/gomysql"
)

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

func CreateNewBooking(name, description string, durationDays int, dnsName string) (record *Booking, err error) {
	record = &Booking{
		Name:        name,
		Description: description,
		StartTime:   time.Now(),
		EndTime:     time.Now().AddDate(0, 0, durationDays),
		Status:      BookingStatusActiveWithRequestPending,
		DNSName:     dnsName,
	}

	err = bookings.Insert(record)
	return
}

func LoadBooking(id int) (record *Booking, err error) {
	return
}
