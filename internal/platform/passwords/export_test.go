package passwords

// Hold takes one of a's slots, as a running hash would, until the returned func is called.
func Hold(a Argon2id) func() {
	a.slots <- struct{}{}
	return func() { <-a.slots }
}
