package common

import (
	"strings"
	"time"
)

type LocalTime struct {
	time.Time
}

func (lt *LocalTime) UnmarshalJSON(b []byte) error {
	s := string(b)
	s = strings.Trim(s, "\"")
	t, err := time.Parse("2006-01-02T15:04:05", s)
	if err != nil {
		return err
	}
	lt.Time = t
	return nil
}
