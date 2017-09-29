package sfxtest

import "sync"

// ErrCheck is a type of function that can be passed in that forces an return of some error
type ErrCheck func(string) error

// ForcedError creates an ErrCheck that returns err for any functions with names
func ForcedError(err error, names ...string) ErrCheck {
	m := make(map[string]error, len(names))
	for _, n := range names {
		m[n] = err
	}
	return func(s string) error {
		return m[s]
	}
}

// ErrChecker is used to force errors for testing error code paths
type ErrChecker struct {
	errCheck ErrCheck
	errMutex sync.Mutex
}

// SetErrorCheck sets the ErrCheck for this instance
func (ec *ErrChecker) SetErrorCheck(errCheck ErrCheck) {
	ec.errMutex.Lock()
	defer ec.errMutex.Unlock()
	ec.errCheck = errCheck
}

// CheckForError check to see if an error should be thrown based
//on the existence ErrCheck and whether it returns an error
func (ec *ErrChecker) CheckForError(s string) error {
	ec.errMutex.Lock()
	defer ec.errMutex.Unlock()
	if ec.errCheck != nil {
		err := ec.errCheck(s)
		return err
	}
	return nil
}
