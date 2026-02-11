package issuedetector

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func schedulingRules() []Rule {
	return []Rule{
		&ruleSCHED001{baseRule: baseRule{id: "SCHED001", name: "Stale CronJob", description: "CronJob has not been scheduled within the expected interval", severity: SeverityWarning, category: CategoryWorkloads, requires: []string{"cronjobs"}}},
		&ruleSCHED002{baseRule: baseRule{id: "SCHED002", name: "Failed CronJob", description: "CronJob's most recent Jobs have all failed", severity: SeverityWarning, category: CategoryWorkloads, requires: []string{"cronjobs", "jobs"}}},
	}
}

// SCHED001: CronJob not scheduled within expected interval
type ruleSCHED001 struct{ baseRule }

func (r *ruleSCHED001) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	cronjobs := cache.CronJobs()
	now := time.Now()
	var findings []Finding

	for _, cj := range cronjobs {
		// Skip suspended CronJobs
		if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
			continue
		}

		interval := parseCronInterval(cj.Spec.Schedule)
		threshold := interval * 2

		if cj.Status.LastScheduleTime != nil && !cj.Status.LastScheduleTime.IsZero() {
			elapsed := now.Sub(cj.Status.LastScheduleTime.Time)
			if elapsed > threshold {
				findings = append(findings, makeFinding(r,
					ResourceRef{Kind: "CronJob", Name: cj.Name, Namespace: cj.Namespace},
					fmt.Sprintf("CronJob '%s' last scheduled %s ago (schedule: %s, expected interval: %s)",
						cj.Name, elapsed.Round(time.Second), cj.Spec.Schedule, interval),
					"Check CronJob events, ensure the schedule is correct, and verify the cluster can schedule jobs",
					map[string]string{
						"schedule":         cj.Spec.Schedule,
						"lastScheduleTime": cj.Status.LastScheduleTime.Time.Format(time.RFC3339),
						"expectedInterval": interval.String(),
					},
				))
			}
		} else {
			// No lastScheduleTime — flag if CronJob has been around for more than 1 hour
			if cj.CreationTimestamp.Time.Before(now.Add(-1 * time.Hour)) {
				findings = append(findings, makeFinding(r,
					ResourceRef{Kind: "CronJob", Name: cj.Name, Namespace: cj.Namespace},
					fmt.Sprintf("CronJob '%s' has never been scheduled (created %s ago, schedule: %s)",
						cj.Name, now.Sub(cj.CreationTimestamp.Time).Round(time.Second), cj.Spec.Schedule),
					"Check CronJob events and ensure the schedule expression is valid",
					map[string]string{
						"schedule":         cj.Spec.Schedule,
						"expectedInterval": interval.String(),
					},
				))
			}
		}
	}
	return findings, nil
}

// SCHED002: CronJob with consecutively failing Jobs
type ruleSCHED002 struct{ baseRule }

func (r *ruleSCHED002) Evaluate(_ context.Context, cache *ResourceCache) ([]Finding, error) {
	cronjobs := cache.CronJobs()
	jobs := cache.Jobs()
	var findings []Finding

	for _, cj := range cronjobs {
		// Find Jobs owned by this CronJob
		var ownedJobs []struct {
			name      string
			failed    int32
			succeeded int32
			startTime time.Time
		}

		for _, job := range jobs {
			if job.Namespace != cj.Namespace {
				continue
			}
			for _, ref := range job.OwnerReferences {
				if ref.Kind == "CronJob" && ref.Name == cj.Name {
					st := time.Time{}
					if job.Status.StartTime != nil {
						st = job.Status.StartTime.Time
					}
					ownedJobs = append(ownedJobs, struct {
						name      string
						failed    int32
						succeeded int32
						startTime time.Time
					}{
						name:      job.Name,
						failed:    job.Status.Failed,
						succeeded: job.Status.Succeeded,
						startTime: st,
					})
					break
				}
			}
		}

		if len(ownedJobs) < 3 {
			continue
		}

		// Sort by start time descending (most recent first)
		sort.Slice(ownedJobs, func(i, j int) bool {
			return ownedJobs[i].startTime.After(ownedJobs[j].startTime)
		})

		// Check if the most recent 3 Jobs all failed
		allFailed := true
		for i := 0; i < 3; i++ {
			if ownedJobs[i].failed == 0 || ownedJobs[i].succeeded > 0 {
				allFailed = false
				break
			}
		}

		if allFailed {
			findings = append(findings, makeFinding(r,
				ResourceRef{Kind: "CronJob", Name: cj.Name, Namespace: cj.Namespace},
				fmt.Sprintf("CronJob '%s' has 3 consecutively failed Jobs", cj.Name),
				"Check Job logs and events for failure reasons; review the CronJob pod template",
				map[string]string{
					"lastFailedJob": ownedJobs[0].name,
				},
			))
		}
	}
	return findings, nil
}

// parseCronInterval attempts to determine the expected interval from a cron schedule expression.
// It handles: predefined (@hourly, @daily, @weekly), */N patterns, specific day-of-week,
// specific day-of-month, specific hour, and comma-separated values.
func parseCronInterval(schedule string) time.Duration {
	schedule = strings.TrimSpace(schedule)

	// Handle predefined schedules
	switch schedule {
	case "@hourly":
		return time.Hour
	case "@daily", "@midnight":
		return 24 * time.Hour
	case "@weekly":
		return 7 * 24 * time.Hour
	case "@monthly", "@annually", "@yearly":
		return 30 * 24 * time.Hour
	}

	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		return 24 * time.Hour // unparseable, conservative default
	}

	minute, hour, dayOfMonth, _, dayOfWeek := fields[0], fields[1], fields[2], fields[3], fields[4]

	// Day-of-week is specific (not *) → weekly or multi-weekly schedule
	if dayOfWeek != "*" {
		count := countCronValues(dayOfWeek)
		if count > 0 && count < 7 {
			// e.g. "0 9 * * 1" = weekly, "0 9 * * 1,4" = ~3.5 days
			return time.Duration(7/count) * 24 * time.Hour
		}
	}

	// Day-of-month is specific (not * and not */N) → monthly schedule
	if dayOfMonth != "*" && !strings.HasPrefix(dayOfMonth, "*/") {
		count := countCronValues(dayOfMonth)
		if count > 0 {
			return time.Duration(30/count) * 24 * time.Hour
		}
	}

	// Day-of-month */N pattern
	if m := regexp.MustCompile(`^\*/(\d+)$`).FindStringSubmatch(dayOfMonth); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			return time.Duration(n) * 24 * time.Hour
		}
	}

	// Hour */N pattern
	if m := regexp.MustCompile(`^\*/(\d+)$`).FindStringSubmatch(hour); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			return time.Duration(n) * time.Hour
		}
	}

	// Hour is specific (not *) → daily or multi-daily schedule
	if hour != "*" && !strings.HasPrefix(hour, "*/") {
		count := countCronValues(hour)
		if count > 0 {
			return time.Duration(24/count) * time.Hour
		}
	}

	// Minute */N pattern
	if m := regexp.MustCompile(`^\*/(\d+)$`).FindStringSubmatch(minute); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			return time.Duration(n) * time.Minute
		}
	}

	// Default: daily
	return 24 * time.Hour
}

// countCronValues returns how many distinct values a cron field represents.
// Handles: single value ("5"), comma-separated ("1,3,5"), ranges ("1-5").
func countCronValues(field string) int {
	if field == "*" {
		return 0
	}
	count := 0
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, err1 := strconv.Atoi(bounds[0])
			hi, err2 := strconv.Atoi(bounds[1])
			if err1 == nil && err2 == nil && hi >= lo {
				count += hi - lo + 1
			} else {
				count++
			}
		} else {
			count++
		}
	}
	return count
}
