package strategy

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var usernamePattern = regexp.MustCompile(`^([A-Za-z]{2})-([0-9]{1,4})$`)

type Strategy struct {
	Region        string `json:"region"`
	RotateMinutes int    `json:"rotate_minutes"`
}

func Parse(username string) (Strategy, error) {
	matches := usernamePattern.FindStringSubmatch(strings.TrimSpace(username))
	if matches == nil {
		return Strategy{}, fmt.Errorf("strategy username must match <region>-<minutes>")
	}

	minutes, err := strconv.Atoi(matches[2])
	if err != nil {
		return Strategy{}, fmt.Errorf("invalid rotation minutes: %w", err)
	}
	if minutes > 1440 {
		return Strategy{}, fmt.Errorf("rotation minutes must be <= 1440")
	}

	return Strategy{
		Region:        strings.ToLower(matches[1]),
		RotateMinutes: minutes,
	}, nil
}

func (s Strategy) Key() string {
	return fmt.Sprintf("%s-%d", strings.ToLower(s.Region), s.RotateMinutes)
}

func (s Strategy) Fixed() bool {
	return s.RotateMinutes == 0
}
