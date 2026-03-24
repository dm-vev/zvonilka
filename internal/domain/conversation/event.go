package conversation

func eventID() string {
	id, err := newID("evt")
	if err != nil {
		return ""
	}

	return id
}
