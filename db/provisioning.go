package db

func UpsertBookingProvisioningStatus(record *BookingProvisioningStatus) (err error) {
	if record == nil {
		return nil
	}

	err = withBookingLock(record.BookingID, func() error {
		existing, selectErr := bookingProvisioningStatuses.Select(record.BookingID)
		if selectErr != nil {
			return selectErr
		}

		if existing == nil {
			return bookingProvisioningStatuses.Insert(record)
		}

		return bookingProvisioningStatuses.Update(record)
	})
	return
}

func BookingProvisioningStatusByBookingID(bookingID int) (record *BookingProvisioningStatus, err error) {
	record, err = bookingProvisioningStatuses.Select(bookingID)
	return
}

func DeleteBookingProvisioningStatus(bookingID int) (err error) {
	err = withBookingLock(bookingID, func() error {
		existing, selectErr := bookingProvisioningStatuses.Select(bookingID)
		if selectErr != nil {
			return selectErr
		}

		if existing == nil {
			return nil
		}

		return bookingProvisioningStatuses.Delete(bookingID)
	})
	return
}
