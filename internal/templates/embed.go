package templates

import "embed"

//go:embed composes/*.yml
var ComposeFS embed.FS

func GetComposeTemplate(slug string) (string, error) {
	data, err := ComposeFS.ReadFile("composes/" + slug + ".yml")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
