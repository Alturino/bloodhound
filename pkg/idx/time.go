package idx

import (
	"strings"
	"time"
)

var jakarta *time.Location

func init() {
	jakarta, _ = time.LoadLocation("Asia/Jakarta")
}

type JsonTime time.Time

func (jt *JsonTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" {
		return nil
	}
	t, err := time.ParseInLocation("2006-01-02T15:04:05", s, jakarta)
	if err != nil {
		return err
	}
	*jt = JsonTime(t)
	return nil
}

func (jt JsonTime) Time() time.Time {
	return time.Time(jt)
}
