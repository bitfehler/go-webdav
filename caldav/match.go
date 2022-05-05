package caldav

import (
	"time"

	"github.com/emersion/go-ical"
)

// Filter returns the filtered list of calendar objects matching the provided query.
// A nil query will return the full list of calendar objects.
func Filter(query *CalendarQuery, cos []CalendarObject) ([]CalendarObject, error) {
	if query == nil {
		// FIXME: should we always return a copy of the provided slice?
		return cos, nil
	}

	out := make([]CalendarObject, 0)
	for _, co := range cos {
		ok, err := Match(query.CompFilter, &co)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		// TODO properties are not currently filtered even if requested
		out = append(out, co)
	}
	return out, nil
}

// Match reports whether the provided CalendarObject matches the query.
func Match(query CompFilter, co *CalendarObject) (matched bool, err error) {
	if co.Data == nil || co.Data.Component == nil {
		// TODO is this a thing? Should this match if comp.IsNotDefined?
		return false, nil
	}
	comp := co.Data.Component

	if query.Name != comp.Name {
		return query.IsNotDefined, nil
	}

	for _, compFilter := range query.Comps {
		match, err := matchCompFilter(compFilter, comp)
		if err != nil {
			return false, err
		}
		if !match {
			return false, nil
		}
	}
	for _, propFilter := range query.Props {
		match, err := matchPropFilter(propFilter, comp)
		if err != nil {
			return false, err
		}
		if !match {
			return false, nil
		}
	}
	return true, nil
}

func matchCompFilter(filter CompFilter, comp *ical.Component) (bool, error) {
	var matches []*ical.Component

	for _, child := range comp.Children {
		match, err := matchCompFilterChild(filter, child)
		if err != nil {
			return false, err
		} else if match {
			matches = append(matches, child)
		}
	}
	if len(matches) == 0 {
		return filter.IsNotDefined, nil
	}
	return true, nil
}

func matchCompFilterChild(filter CompFilter, comp *ical.Component) (bool, error) {
	if comp.Name != filter.Name {
		return false, nil
	}

	var zeroDate time.Time
	if filter.Start != zeroDate {
		match, err := matchCompTimeRange(filter.Start, filter.End, comp)
		if err != nil {
			return false, err
		}
		if !match {
			return false, nil
		}
	}
	for _, compFilter := range filter.Comps {
		match, err := matchCompFilter(compFilter, comp)
		if err != nil {
			return false, err
		}
		if !match {
			return false, nil
		}
	}
	for _, propFilter := range filter.Props {
		match, err := matchPropFilter(propFilter, comp)
		if err != nil {
			return false, err
		}
		if !match {
			return false, nil
		}
	}
	return true, nil
}

func matchPropFilter(filter PropFilter, comp *ical.Component) (bool, error) {
	// TODO: this only matches first field, can there be multiple like for CardDAV?
	field := comp.Props.Get(filter.Name)
	if field == nil {
		return filter.IsNotDefined, nil
	}

	var zeroDate time.Time
	if filter.Start != zeroDate {
		match, err := matchPropTimeRange(filter.Start, filter.End, field)
		if err != nil {
			return false, err
		}
		if !match {
			return false, nil
		}
		for _, paramFilter := range filter.ParamFilter {
			if !matchParamFilter(paramFilter, field) {
				return false, nil
			}
		}
	} else if filter.TextMatch != nil {
		if !matchTextMatch(*filter.TextMatch, field.Value) {
			return false, nil
		}
		for _, paramFilter := range filter.ParamFilter {
			if !matchParamFilter(paramFilter, field) {
				return false, nil
			}
		}
		return true, nil
	}
	// empty prop-filter, property exists
	return true, nil
}

func matchCompTimeRange(start, end time.Time, comp *ical.Component) (bool, error) {
	// TODO what about other types of components?
	if comp.Name != ical.CompEvent {
		return false, nil
	}
	event := ical.Event{comp}

	eventStart, err := event.DateTimeStart(start.Location())
	if err != nil {
		return false, err
	}
	eventEnd, err := event.DateTimeEnd(end.Location())
	if err != nil {
		return false, err
	}

	// Event starts in time range
	if eventStart.After(start) && eventStart.Before(end) {
		return true, nil
	}
	// Event ends in time range
	if eventEnd.After(start) && eventEnd.Before(end) {
		return true, nil
	}
	// Event covers entire time range plus some
	if eventStart.Before(start) && eventEnd.After(end) {
		return true, nil
	}
	return false, nil
}

func matchPropTimeRange(start, end time.Time, field *ical.Prop) (bool, error) {
	// The RFC says: "The CALDAV:prop-filter XML element contains a
	// CALDAV:time-range XML element and the property value overlaps the
	// specified time range".
	// Not entirely sure how a property can "overlap" a time range, but I
	// assume it means "the property is Date/Time that falls into the given
	// time range.
	ptime, err := field.DateTime(start.Location())
	if err != nil {
		return false, err
	}
	if ptime.After(start) && ptime.Before(end) {
		return true, nil
	}
	return false, nil
}

func matchParamFilter(filter ParamFilter, field *ical.Prop) bool {
	// TODO there can be multiple values
	value := field.Params.Get(filter.Name)
	if value == "" {
		return filter.IsNotDefined
	} else if filter.IsNotDefined {
		return false
	}
	if filter.TextMatch != nil {
		return matchTextMatch(*filter.TextMatch, value)
	}
	return true
}

func matchTextMatch(txt TextMatch, value string) bool {
	// TODO: handle text-match collation attribute
	match := value == txt.Text
	if txt.NegateCondition {
		match = !match
	}
	return match
}
