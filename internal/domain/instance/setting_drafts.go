// SPDX-License-Identifier: AGPL-3.0-only

package instance

// The limits of drafts and scheduled posts (ADR-0156 §7): one key per limit.
const (
	SettingMaxDrafts                   SettingKey = "max_drafts"
	SettingMaxScheduledPosts           SettingKey = "max_scheduled_posts"
	SettingMaxScheduledPostsPerDay     SettingKey = "max_scheduled_posts_per_day"
	SettingScheduledPostMinLeadMinutes SettingKey = "scheduled_post_min_lead_minutes"
	SettingScheduledPostMaxLeadDays    SettingKey = "scheduled_post_max_lead_days"
)

// MaxDraftLimit bounds the counts of SettingMaxDrafts, SettingMaxScheduledPosts, and
// SettingMaxScheduledPostsPerDay.
const MaxDraftLimit = 10000

// MaxDrafts returns the value of SettingMaxDrafts.
func (s Settings) MaxDrafts() int { return s.intValue(SettingMaxDrafts) }

// MaxScheduledPosts returns the value of SettingMaxScheduledPosts.
func (s Settings) MaxScheduledPosts() int { return s.intValue(SettingMaxScheduledPosts) }

// MaxScheduledPostsPerDay returns the value of SettingMaxScheduledPostsPerDay.
func (s Settings) MaxScheduledPostsPerDay() int { return s.intValue(SettingMaxScheduledPostsPerDay) }

// ScheduledPostMinLeadMinutes returns the value of SettingScheduledPostMinLeadMinutes.
func (s Settings) ScheduledPostMinLeadMinutes() int {
	return s.intValue(SettingScheduledPostMinLeadMinutes)
}

// ScheduledPostMaxLeadDays returns the value of SettingScheduledPostMaxLeadDays.
func (s Settings) ScheduledPostMaxLeadDays() int { return s.intValue(SettingScheduledPostMaxLeadDays) }
