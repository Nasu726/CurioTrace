package session

// IsDurable reports whether this authority is backed by a durable snapshot
// repository for the current helper instance.
func (a *Authority) IsDurable() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.repository != nil
}
