package server

type Error string

func (e Error) Error() string {
	return string(e)
}

const (
	ErrNoLoginData Error = "no login data"
)
