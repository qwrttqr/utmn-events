package domain

import (
	"errors"
	"time"
)

var ErrEventNotFound = errors.New("event not found")
var ErrCapacityBelowRegistrations = errors.New("capacity is below current registration count")
var ErrEventFull = errors.New("event is full")
var ErrEventClosed = errors.New("event registration is closed")
var ErrUserNotRegistered = errors.New("user is not registered")
var ErrEventStartInPast = errors.New("event start must be in the future")
var ErrInvalidEvent = errors.New("invalid event")

type EventFilter struct {
	TagIDs         []int64
	UserTelegramID int64
	UpcomingOnly   bool
}

type EventField string

const (
	EventTitle       EventField = "title"
	EventDescription EventField = "description"
	EventPhoto       EventField = "photo"
	EventLocation    EventField = "location"
	EventDates       EventField = "dates"
	EventCapacity    EventField = "capacity"
	EventTags        EventField = "tags"
)

type EventSummary struct {
	ID    int64
	Title string
}

type Event struct {
	ID                int64
	Title             string
	Description       string
	PhotoFileID       string
	Location          string
	StartsAt          time.Time
	EndsAt            time.Time
	Capacity          *int32 // nil means unlimited.
	Tags              []string
	RegistrationCount int64
}
