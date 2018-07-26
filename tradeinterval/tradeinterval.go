package tradeinterval

import (
	"regexp"
	"strconv"
	"strings"
)

// Unit represents the time unit of an interval
type Unit string

// Units for different time intervals
const (
	Minute Unit = ""
	Hour        = "H"
	Day         = "D"
	Week        = "W"
	Month       = "M"
)

// Interval represents a time interval
type Interval struct {
	Num  int
	Unit Unit
}

// Seconds returns the length of the interval in seconds
func (i Interval) Seconds() int {
	var factor int

	switch i.Unit {
	case Minute:
		factor = 60
	case Day:
		factor = 60 * 60 * 24
	case Week:
		factor = 60 * 60 * 24 * 7
	case Month:
		factor = 60 * 60 * 24 * 30
	}

	return i.Num * factor
}

// MinHourDay returns the interval represented by minutes/hours/days
func (i Interval) MinHourDay() Interval {
	var res Interval

	switch i.Unit {
	case Minute:
		if i.Num < 60 {
			res.Unit = Minute
			res.Num = i.Num
		} else {
			res.Unit = Hour
			res.Num = i.Num / 60
		}
	case Hour:
		res.Unit = Hour
		res.Num = i.Num
	case Day:
		res.Unit = Day
		res.Num = i.Num
	case Week:
		res.Unit = Day
		res.Num = i.Num * 7
	case Month:
		res.Unit = Day
		res.Num = i.Num * 30
	}

	return res
}

// Parse converts a string to an interval
func Parse(s string) Interval {
	units := map[string]Unit{
		"":  Minute,
		"D": Day,
		"W": Week,
		"M": Month,
	}

	i := Interval{Num: 1, Unit: Minute}

	reg := regexp.MustCompile("(\\d+)?\\s*([DWM])?")
	results := reg.FindStringSubmatch(strings.ToUpper(s))

	num, err := strconv.Atoi(results[1])
	if err == nil {
		i.Num = num
	}

	i.Unit = units[results[2]]

	return i
}
