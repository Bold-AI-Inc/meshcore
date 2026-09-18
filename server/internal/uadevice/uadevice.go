package uadevice

import "github.com/mssola/user_agent"

type Info struct {
	Browser        string
	BrowserVersion string
	OS             string
	OSVersion      string
	DeviceType     string
}

func Parse(ua string) Info {
	if ua == "" {
		return Info{}
	}

	parsed := user_agent.New(ua)

	name, version := parsed.Browser()
	os := parsed.OSInfo()

	return Info{
		Browser:        name,
		BrowserVersion: version,
		OS:             os.Name,
		OSVersion:      os.Version,
		DeviceType:     deviceType(parsed),
	}
}

func deviceType(parsed *user_agent.UserAgent) string {
	switch {
	case parsed.Bot():
		return "bot"
	case parsed.Mobile():
		return "mobile"
	case parsed.Platform() == "iPad":
		return "tablet"
	case parsed.OS() == "":
		return "other"
	default:
		return "desktop"
	}
}
