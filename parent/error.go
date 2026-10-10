package parent

import "errors"

var (
	// ErrNoStarter reports an empty registration snapshot.
	ErrNoStarter = errors.New("no starter")
	// ErrMissStarters is an alias of ErrNoStarter used by Start.
	ErrMissStarters = ErrNoStarter
	// ErrNoStarterSet is an alias of ErrNoStarter used by StopStarter.
	ErrNoStarterSet = ErrNoStarter
	// ErrUnknownStarterName reports a name absent from the registration snapshot.
	ErrUnknownStarterName = errors.New("unknown starterName")
	// ErrSomeStarterNoSetting reports a registered component returning nil settings.
	ErrSomeStarterNoSetting = errors.New("some starter has no setting")
	// ErrNilStarter reports a nil component, including typed nil interface values.
	ErrNilStarter = errors.New("starter is nil")
	// ErrEmptyStarterName reports a component with an empty registration name.
	ErrEmptyStarterName = errors.New("starter name is empty")
	// ErrDuplicateStarterName reports duplicate names; wrapped errors include the name.
	ErrDuplicateStarterName = errors.New("duplicate starter name")
	// ErrStopAllTimeout reports an expired batch wait; background stops continue.
	ErrStopAllTimeout = errors.New("stop the module exceeding the maximum wait time")
	// ErrStarterNotStarted reports a stop request outside the started state, except stopping.
	ErrStarterNotStarted = errors.New("not started")
	// ErrStarterStopping reports a conflicting lifecycle request during stopping.
	ErrStarterStopping = errors.New("starter is stopping")
	// ErrStarterRestartDisabled reports startup after a completed stop without restart permission.
	ErrStarterRestartDisabled = errors.New("starter restart disabled")
)
