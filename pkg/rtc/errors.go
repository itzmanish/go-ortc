package rtc

import "fmt"

type SFUError func(e error) error

var (
	errNotImplemented = fmt.Errorf("not implemented")

	errReceiverNotCreated = func(e error) error {
		return fmt.Errorf("receiver creation failed: %s", e)
	}

	errSettingReceiverParameters = func(e error) error {
		return fmt.Errorf("setting receiver parameters failed: %s", e)
	}

	errProducerNotFound = func(id uint) error {
		return fmt.Errorf("producer not found: %v", id)
	}

	errFailedToCreateDownTrack = func(e error) error {
		return fmt.Errorf("failed to create down track: %s", e)
	}

	errFailedToCreateConsumer = func(e error) error {
		return fmt.Errorf("failed to create consumer: %s", e)
	}
)
