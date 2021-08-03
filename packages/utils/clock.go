
// Clock represents interface of clock
type Clock interface {
	Now() time.Time
}

// ClockWrapper represents wrapper of clock
type ClockWrapper struct {
}

// Now returns current time
func (cw *ClockWrapper) Now() time.Time { return time.Now() }
