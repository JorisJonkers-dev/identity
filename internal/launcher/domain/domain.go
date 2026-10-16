// Package domain holds the Launcher's types: the apps a User can open, and their icons.
package domain

// Client is a Client a person can open: a native or gated one the cluster declares now.
type Client struct {
	ID          string
	Label       string
	Description string
	Host        string
	LaunchURL   string
	IconURL     string
}

// Launch is where the Launcher sends a person: the Client's launch URL, or its host's root.
func (c Client) Launch() string {
	if c.LaunchURL != "" {
		return c.LaunchURL
	}
	return "https://" + c.Host + "/"
}

// App is a Client a User holds an App Role on, with those roles.
type App struct {
	Client
	Roles []string
}

// Icon is a Client's icon as identity serves it.
type Icon struct {
	ContentType string
	Body        []byte
}
