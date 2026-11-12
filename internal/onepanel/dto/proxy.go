package dto

// SystemProxy is the upstream transport contract; the standalone program uses the environment proxy.
type SystemProxy struct{ Type, URL, Port, User, Password string }
