package parent

import "errors"

var (
	ErrMissStarters           = errors.New("miss starters")
	ErrNoStarter              = errors.New("no starter")
	ErrNoStarterSet           = errors.New("no starter set")
	ErrUnknownStarterName     = errors.New("unknown starterName")
	ErrSomeStarterNoSetting   = errors.New("some starter has no setting")
	ErrStopAllTimeout         = errors.New("stop the module exceeding the maximum wait time")
	ErrStarterNotStarted      = errors.New("not started")
	ErrStarterRestartDisabled = errors.New("starter restart disabled")
)
