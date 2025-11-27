package tests

// func TestBookingPeople(t *testing.T) {
// 	setup(t)
// 	defer cleanup(t)

// 	// Booking 1:
// 	// 	 Alice: Owner
// 	//   Bob: Operator
// 	//   Charlie: Viewer

// 	// Booking 2:
// 	//   Bob: Owner,
// 	//   Alice: Viewer

// 	var (
// 		err    error
// 		people []*db.BookingPerson = []*db.BookingPerson{
// 			{BookingID: 1, Username: "alice", PermissionLevel: db.BookingPermissionLevelOwner},
// 			{BookingID: 1, Username: "bob", PermissionLevel: db.BookingPermissionLevelOperator},
// 			{BookingID: 1, Username: "charlie", PermissionLevel: db.BookingPermissionLevelViewer},
// 			{BookingID: 2, Username: "bob", PermissionLevel: db.BookingPermissionLevelOwner},
// 			{BookingID: 2, Username: "alice", PermissionLevel: db.BookingPermissionLevelViewer},
// 		}
// 	)

// 	for _, person := range people {
// 		if err = db.InsertBookingPerson(person); err != nil {
// 			t.Fatalf("Failed to set booking person: %v", err)
// 		}
// 	}

// 	var records []*db.BookingPerson
// 	if records, err = db.BookingPersonsFor("alice"); err != nil {
// 		t.Fatalf("Failed to get bookings for person: %v", err)
// 	} else {
// 		assert.Len(t, records, 2, "Expected 2 booking records for Alice")
// 		assert.Equal(t, db.BookingPermissionLevelOwner, records[0].PermissionLevel, "Expected Alice to be Owner in Booking 1")
// 		assert.Equal(t, db.BookingPermissionLevelViewer, records[1].PermissionLevel, "Expected Alice to be Viewer in Booking 2")
// 	}

// 	if records, err = db.BookingPersonsFor("bob"); err != nil {
// 		t.Fatalf("Failed to get bookings for person: %v", err)
// 	} else {
// 		assert.Len(t, records, 2, "Expected 2 booking records for Bob")
// 		assert.Equal(t, db.BookingPermissionLevelOperator, records[0].PermissionLevel, "Expected Bob to be Operator in Booking 1")
// 		assert.Equal(t, db.BookingPermissionLevelOwner, records[1].PermissionLevel, "Expected Bob to be Owner in Booking 2")
// 	}

// 	if records, err = db.BookingPersonsFor("charlie"); err != nil {
// 		t.Fatalf("Failed to get bookings for person: %v", err)
// 	} else {
// 		assert.Len(t, records, 1, "Expected 1 booking record for Charlie")
// 		assert.Equal(t, db.BookingPermissionLevelViewer, records[0].PermissionLevel, "Expected Charlie to be Viewer in Booking 1")
// 	}
// }
