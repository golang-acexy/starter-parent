package parent

import "errors"

var (
	ErrNoStarter              = errors.New("no starter")
	ErrMissStarters           = ErrNoStarter
	ErrNoStarterSet           = ErrNoStarter
	ErrUnknownStarterName     = errors.New("unknown starterName")
	ErrSomeStarterNoSetting   = errors.New("some starter has no setting")
	ErrNilStarter             = errors.New("starter is nil")
	ErrEmptyStarterName       = errors.New("starter name is empty")
	ErrDuplicateStarterName   = errors.New("duplicate starter name")
	ErrStopAllTimeout         = errors.New("stop the module exceeding the maximum wait time")
	ErrStarterNotStarted      = errors.New("not started")
	ErrStarterStopping        = errors.New("starter is stopping")
	ErrStarterRestartDisabled = errors.New("starter restart disabled")
)
