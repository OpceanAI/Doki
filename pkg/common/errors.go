package common

import "fmt"

// ErrNotFound is returned when a resource is not found.
type ErrNotFound struct {
	Resource string
	ID       string
}

func (e *ErrNotFound) Error() string {
	return fmt.Sprintf("%s %s not found", e.Resource, e.ID)
}

func (e *ErrNotFound) Is(target error) bool {
	_, ok := target.(*ErrNotFound)
	return ok
}

// ErrConflict is returned when a resource already exists.
type ErrConflict struct {
	Resource string
	ID       string
}

func (e *ErrConflict) Error() string {
	return fmt.Sprintf("%s %s already exists", e.Resource, e.ID)
}

func (e *ErrConflict) Is(target error) bool {
	_, ok := target.(*ErrConflict)
	return ok
}

// ErrInvalidParam is returned when a parameter is invalid.
type ErrInvalidParam struct {
	Param   string
	Message string
}

func (e *ErrInvalidParam) Error() string {
	return fmt.Sprintf("invalid parameter %s: %s", e.Param, e.Message)
}

// ErrNotImplemented is returned when a feature is not yet implemented.
type ErrNotImplemented struct {
	Feature string
}

func (e *ErrNotImplemented) Error() string {
	return fmt.Sprintf("feature not implemented: %s", e.Feature)
}

// ErrContainerStopped is returned when trying to operate on a stopped container.
type ErrContainerStopped struct {
	ID string
}

func (e *ErrContainerStopped) Error() string {
	return fmt.Sprintf("container %s is not running", e.ID)
}

// ErrContainerPaused is returned when a container is paused.
type ErrContainerPaused struct {
	ID string
}

func (e *ErrContainerPaused) Error() string {
	return fmt.Sprintf("container %s is paused", e.ID)
}

// ErrPermissionDenied is returned when permission is denied.
type ErrPermissionDenied struct {
	Message string
}

func (e *ErrPermissionDenied) Error() string {
	msg := "permission denied"
	if e.Message != "" {
		msg += ": " + e.Message
	}
	return msg
}

// NewErrNotFound creates a new ErrNotFound.
func NewErrNotFound(resource, id string) error {
	return &ErrNotFound{Resource: resource, ID: id}
}

// NewErrConflict creates a new ErrConflict.
func NewErrConflict(resource, id string) error {
	return &ErrConflict{Resource: resource, ID: id}
}

// NewErrInvalidParam creates a new ErrInvalidParam.
func NewErrInvalidParam(param, message string) error {
	return &ErrInvalidParam{Param: param, Message: message}
}
