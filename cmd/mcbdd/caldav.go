package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
	"github.com/google/uuid"
)

const (
	ConstCalendarName = "Birthdays"
)

func (d *Daemon) ensureBirthdayCal(ctx context.Context, httpClient webdav.HTTPClient, user string) error {
	endpoint, err := url.JoinPath(d.baseURL, "SOGo/dav", user, "Calendar/")
	if err != nil {
		return err
	}
	cl, err := caldav.NewClient(httpClient, endpoint)
	if err != nil {
		return err
	}
	cc, err := cl.FindCalendars(ctx, "")
	if err != nil {
		return err
	}
	for _, c := range cc {
		if strings.HasSuffix(c.Path, fmt.Sprintf("/%s", ConstCalendarName)) {
			return nil
		}
	}
	if err := cl.Mkdir(ctx, ConstCalendarName); err != nil {
		return err
	}
	slog.InfoContext(ctx, "created birthday calendar", "user", user)
	return nil
}

func (d *Daemon) syncBirthdaysToCal(ctx context.Context, httpClient webdav.HTTPClient, user string, birthdays []BirthdayContact) error {
	endpoint, err := url.JoinPath(d.baseURL, "SOGo/dav", user, "Calendar/")
	if err != nil {
		return err
	}
	cl, err := caldav.NewClient(httpClient, endpoint)
	if err != nil {
		return err
	}
	calendarPath := fmt.Sprintf("/SOGo/dav/%s/Calendar/%s", user, ConstCalendarName)
	events, err := cl.QueryCalendar(ctx, calendarPath, &caldav.CalendarQuery{
		CompRequest: caldav.CalendarCompRequest{
			Name: "VCALENDAR",
		},
		CompFilter: caldav.CompFilter{
			Name: "VCALENDAR",
			Comps: []caldav.CompFilter{{
				Name: "VEVENT",
			}},
			Start: time.Now().Add(time.Hour * 24 * 365 * -1).UTC(),
			End:   time.Now().Add(time.Hour * 24 * 365 * 100).UTC(),
		},
	})
	if err != nil {
		return err
	}
	bevs := generateBirthdayEvents(birthdays)
	bevsInSync := make([]int, 0)
	driftedEvents := make([]string, 0)
	for _, ev := range events {
		matchedBev := false
		for _, v := range ev.Data.Children {
			for i, bev := range bevs {
				if icalMatchesBev(v, bev) {
					bevsInSync = append(bevsInSync, i)
					matchedBev = true
				}
			}
		}
		if !matchedBev {
			driftedEvents = append(driftedEvents, ev.Path)
		}
	}
	counterDelete, counterAdded := 0, 0
	for _, v := range driftedEvents {
		if err := cl.RemoveAll(ctx, v); err != nil {
			return err
		}
		counterDelete++
	}
	for i, v := range bevs {
		if slices.Contains(bevsInSync, i) {
			continue
		}
		p, ic := v.generateICAL(calendarPath)
		_, err := cl.PutCalendarObject(ctx, p, ic)
		if err != nil {
			return err
		}
		counterAdded++
	}
	if (counterAdded + counterDelete) > 0 {
		slog.InfoContext(ctx, "synchronized birthday events", "user", user, "added", counterAdded, "removed", counterDelete)
	}
	return nil
}

type birthdayEvent struct {
	Summary       string
	DateTimeStart string
	DateTimeEnd   string
}

func generateBirthdayEvents(birthdays []BirthdayContact) []birthdayEvent {
	cyear := time.Now().Year()
	bb := make([]birthdayEvent, 0)
	for _, v := range birthdays {
		for year := cyear; year <= 10+cyear; year++ {
			yearshift := year - v.Date.Year()
			ev := birthdayEvent{
				Summary:       fmt.Sprintf("%s %s", v.GivenName, v.FamilyName),
				DateTimeStart: v.Date.AddDate(yearshift, 0, 0).Format("20060102"),
				DateTimeEnd:   v.Date.AddDate(yearshift, 0, 1).Format("20060102"),
			}
			if v.YearKnown {
				ev.Summary = fmt.Sprintf("%s (%d)", ev.Summary, yearshift)
			}
			bb = append(bb, ev)
		}
	}
	return bb
}

func icalMatchesBev(ic *ical.Component, bev birthdayEvent) bool {
	if ic.Props.Get(ical.PropSummary) == nil || ic.Props.Get(ical.PropSummary).Value != bev.Summary {
		return false
	}
	if ic.Props.Get(ical.PropDateTimeStart) == nil || ic.Props.Get(ical.PropDateTimeStart).Value != bev.DateTimeStart {
		return false
	}
	if ic.Props.Get(ical.PropDateTimeEnd) == nil || ic.Props.Get(ical.PropDateTimeEnd).Value != bev.DateTimeEnd {
		return false
	}
	hasAlarm := false
	for _, child := range ic.Children {
		if child.Name == ical.CompAlarm {
			hasAlarm = true
			break
		}
	}
	return hasAlarm
}

func (bev birthdayEvent) generateICAL(calendar string) (string, *ical.Calendar) {
	id := uuid.New().String()
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropProductID, "-//Marco98//MailcowBirthdayDaemon//EN")
	cal.Props.SetText(ical.PropVersion, "2.0")
	event := ical.NewComponent(ical.CompEvent)
	event.Props.SetText(ical.PropUID, id)
	event.Props.SetText(ical.PropSummary, bev.Summary)
	event.Props.SetDateTime(ical.PropDateTimeStamp, time.Now())
	start := ical.NewProp(ical.PropDateTimeStart)
	start.Value = bev.DateTimeStart
	end := ical.NewProp(ical.PropDateTimeEnd)
	end.Value = bev.DateTimeEnd
	event.Props.Set(start)
	event.Props.Set(end)
	alarm := ical.NewComponent(ical.CompAlarm)
	alarm.Props.SetText(ical.PropAction, "DISPLAY")
	alarm.Props.SetText(ical.PropDescription, bev.Summary)
	trigger := ical.NewProp(ical.PropTrigger)
	trigger.Params.Set(ical.ParamValue, string(ical.ValueDuration))
	trigger.Value = "PT0S"
	alarm.Props.Set(trigger)
	event.Children = append(event.Children, alarm)
	cal.Children = append(cal.Children, event)
	return fmt.Sprintf("%s/%s.ics", calendar, id), cal
}
