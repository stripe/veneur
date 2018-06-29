package samplers

import (
	"sort"
	"strings"
	"time"

	"github.com/segmentio/fasthash/fnv1a"
	"github.com/stripe/veneur/ssf"
)

// CheckStates holds the statuses of all status checks that the veneur
// has received, if a retention time for these checks is configured.
type CheckStates struct {
	// statuses maps from one (UDPMetric-like) digest value for a
	// status check to its success status.
	statuses map[uint32]float64
	resetAt  time.Time
	validFor time.Duration
}

// NewCheckStatusTracker constructs and returns a CheckStates object
// that can be used to make decisions on whether a status check should
// be flushed at the current time or not.
func NewCheckStatusTracker(validFor time.Duration) *CheckStates {
	return &CheckStates{
		statuses: make(map[uint32]float64),
		resetAt:  time.Now().Add(validFor),
		validFor: validFor,
	}
}

// ShouldRecord updates the CheckStates state, makes a decision on
// whether a status check should be flushed and returns that decision
// as a boolean - a true value indicates that the check should be
// flushed.  ShouldRecord is not safe to call from more than a single
// goroutine.
//
// Criteria For Flushing
//
// If the current value is not OK, flush the check.
//
// If the current value is OK and the previous value of the check was
// not OK, flush the current check.
//
// If the current value is OK and the previous value is both known and
// OK, don't flush the current check.
func (c *CheckStates) ShouldRecord(status *StatusCheck) bool {
	if c == nil {
		return true
	}

	// Construct a MetricKey to use for computing the digest:
	tags := make([]string, len(status.Tags))
	for i, t := range status.Tags {
		tags[i] = t
	}
	sort.Strings(tags)
	digest := fnv1a.Init32
	digest = fnv1a.AddString32(digest, status.Name)
	digest = fnv1a.AddString32(digest, "status")
	digest = fnv1a.AddString32(digest, strings.Join(tags, ","))

	// Decide what to do with the check result, depending on its
	// current and previous value. We always report a non-positive
	// current result, or one that differs from the previous
	// result.
	last, ok := c.statuses[digest]
	c.statuses[digest] = status.Value
	if !ok || last != status.Value || status.Value != float64(ssf.SSFSample_OK) {
		return true
	}
	return false
}

// MaybeReset resets the check status if it has become invalid. It is
// not concurrency-safe.
func (c *CheckStates) MaybeReset(now time.Time) {
	if c == nil {
		return
	}
	if c.resetAt.Before(now) {
		c.statuses = make(map[uint32]float64)
		c.resetAt = now.Add(c.validFor)
	}
}
