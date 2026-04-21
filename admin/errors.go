package admin

type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

func NewError(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

var (
	ErrNotFound      = NewError(404, "NOT_FOUND", "resource not found")
	ErrBadRequest    = NewError(400, "BAD_REQUEST", "invalid request")
	ErrInvalidAction = NewError(404, "INVALID_ACTION", "unknown action")
	ErrInvalidEntity = NewError(404, "INVALID_ENTITY", "unknown entity")
	ErrInternal      = NewError(500, "INTERNAL", "internal error")
)

func validationError(err error) *Error {
	return NewError(400, "VALIDATION_FAILED", err.Error())
}

func badRequest(message string) *Error {
	return NewError(400, "BAD_REQUEST", message)
}
