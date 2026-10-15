package v1

type errWrapper struct {
	Err *APIErr `json:"error"`
}

type APIErr struct {
	Code int    `json:"-"`
	Msg  string `json:"message"`

	raw []byte
}

func (e *APIErr) Raw() string { return string(e.raw) }
func (e *APIErr) Error() string {
	if e.Msg == "" {
		return e.Raw()
	}

	return e.Msg
}

type ClientErr struct {
	parent error
}

func NewClientErr(err error) *ClientErr { return &ClientErr{parent: err} }

func (e *ClientErr) Error() string { return e.parent.Error() }
func (e *ClientErr) Unwrap() error { return e.parent }

type TransportErr struct {
	parent error
}

func NewTransportErr(err error) *TransportErr { return &TransportErr{parent: err} }

func (e *TransportErr) Error() string { return e.parent.Error() }
func (e *TransportErr) Unwrap() error { return e.parent }
