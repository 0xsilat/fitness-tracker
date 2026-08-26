package web

import (
	"fmt"
	"strings"
	"time"

	"github.com/local/fitness-tracker/internal/domain"
)

func number(value float64) string {
	return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", value), "0"), ".")
}
func date(value time.Time) string      { return value.Format("02 Jan 2006") }
func inputDate(value time.Time) string { return value.Format("2006-01-02") }
func formatName(value string) string {
	return map[string]string{"sets_reps": "Sets × reps", "emom": "EMOM", "mixed": "Mixed formats"}[value]
}
func routineClass(r domain.Routine) string {
	if r.Active {
		return "routine-card active"
	}
	return "routine-card"
}
func routineStatus(r domain.Routine) string {
	if r.Active {
		return "Active"
	}
	if r.Archived {
		return "Archived"
	}
	return "Routine"
}
func routineLogLabel(r domain.Routine) string {
	if r.Archived {
		return r.Name + " (Archived)"
	}
	if r.Active {
		return r.Name + " (Active)"
	}
	return r.Name
}
func skippedClass(skipped bool) string {
	if skipped {
		return "set-row skipped"
	}
	return "set-row"
}
func sessionRowClass(mode string, skipped bool) string {
	value := "set-row"
	if mode == "bodyweight" {
		value += " bodyweight"
	}
	if skipped {
		value += " skipped"
	}
	return value
}
func sessionHeaderClass(mode string) string {
	if mode == "bodyweight" {
		return "set-row header bodyweight"
	}
	return "set-row header"
}
func rpeValue(value *int) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%d", *value)
}
func actualRepsValue(value int) string {
	if value <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", value)
}
func setMarker(format string, set domain.SessionSet) string {
	if format == "emom" {
		if set.Minute > 0 {
			return fmt.Sprintf("%d", set.Minute)
		}
		return "Extra"
	}
	return fmt.Sprintf("%d", set.Position)
}
func chartPoints(points []domain.ChartPoint) string {
	if len(points) == 0 {
		return ""
	}
	max := 0.0
	for _, p := range points {
		if p.Value > max {
			max = p.Value
		}
	}
	if max == 0 {
		max = 1
	}
	var out []string
	for i, p := range points {
		x := 20.0
		if len(points) > 1 {
			x += float64(i) * 560 / float64(len(points)-1)
		}
		y := 180 - (p.Value/max)*150
		out = append(out, fmt.Sprintf("%.1f,%.1f", x, y))
	}
	return strings.Join(out, " ")
}
func chartX(index, count int) string {
	if count <= 1 {
		return "20"
	}
	return fmt.Sprintf("%.1f", 20+float64(index)*560/float64(count-1))
}
func chartY(points []domain.ChartPoint, value float64) string {
	max := 0.0
	for _, p := range points {
		if p.Value > max {
			max = p.Value
		}
	}
	if max == 0 {
		max = 1
	}
	return fmt.Sprintf("%.1f", 180-(value/max)*150)
}
func exerciseMetric(mode string) string {
	if mode == "weighted" {
		return "Volume"
	}
	return "Reps"
}
