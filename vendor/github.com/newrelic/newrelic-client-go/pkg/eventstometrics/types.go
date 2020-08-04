// Code generated by tutone: DO NOT EDIT
package eventstometrics

// EventsToMetricsErrorReason - General error categories.
type EventsToMetricsErrorReason string

var EventsToMetricsErrorReasonTypes = struct {
	// Other errors.
	GENERAL EventsToMetricsErrorReason
	// Indicates some part of your submission was invalid.
	INVALID_INPUT EventsToMetricsErrorReason
	// The user attempting to submit this rule is not authorized to do so.
	USER_NOT_AUTHORIZED EventsToMetricsErrorReason
}{
	// Other errors.
	GENERAL: "GENERAL",
	// Indicates some part of your submission was invalid.
	INVALID_INPUT: "INVALID_INPUT",
	// The user attempting to submit this rule is not authorized to do so.
	USER_NOT_AUTHORIZED: "USER_NOT_AUTHORIZED",
}

// EventsToMetricsAccountStitchedFields - Account stitched fields to enable autostitching in NerdGraph
type EventsToMetricsAccountStitchedFields struct {
	// List all rules for your account.
	AllRules EventsToMetricsListRuleResult `json:"allRules"`
	// List rules for your account by id.
	RulesById EventsToMetricsListRuleResult `json:"rulesById"`
}

// EventsToMetricsCreateRuleFailure - Error details about the events to metrics rule that failed to be created and why.
type EventsToMetricsCreateRuleFailure struct {
	// Information about why the create failed.
	Errors []EventsToMetricsError `json:"errors"`
	// Input information about a submitted rule that was unable to be created.
	Submitted EventsToMetricsCreateRuleSubmission `json:"submitted"`
}

// EventsToMetricsCreateRuleInput - Details needed to create an events to metrics conversion rule.
type EventsToMetricsCreateRuleInput struct {
	// The account where the events exist and the metrics will be put.
	AccountID int `json:"accountId"`
	// Provides additional information about the rule.
	Description string `json:"description"`
	// The name of the rule. This must be unique within a given account.
	Name string `json:"name"`
	// Explains how to create one or more metrics from events.
	Nrql string `json:"nrql"`
}

// EventsToMetricsCreateRuleResult - The result of which submitted events to metrics rules were successfully and unsuccessfully created
type EventsToMetricsCreateRuleResult struct {
	// Rules that were not created and why.
	Failures []EventsToMetricsCreateRuleFailure `json:"failures"`
	// Rules that were successfully created.
	Successes []EventsToMetricsRule `json:"successes"`
}

// EventsToMetricsCreateRuleSubmission - The details that were submitted when creating an events to metrics conversion rule.
type EventsToMetricsCreateRuleSubmission struct {
	// The account where the events exist and the metrics will be put.
	AccountID int `json:"accountId"`
	// Provides additional information about the rule.
	Description string `json:"description"`
	// The name of the rule. This must be unique within a given account.
	Name string `json:"name"`
	// Explains how to create one or more metrics from events.
	Nrql string `json:"nrql"`
}

// EventsToMetricsDeleteRuleFailure - Error details about the events to metrics rule that failed to be deleted and why.
type EventsToMetricsDeleteRuleFailure struct {
	// Information about why the delete failed.
	Errors []EventsToMetricsError `json:"errors"`
	// Input information about a submitted rule that was unable to be deleted.
	Submitted EventsToMetricsDeleteRuleSubmission `json:"submitted"`
}

// EventsToMetricsDeleteRuleInput - Identifying information about the events to metrics rule you want to delete.
type EventsToMetricsDeleteRuleInput struct {
	// A submitted account id.
	AccountID int `json:"accountId"`
	// A submitted rule id.
	RuleId string `json:"ruleId"`
}

// EventsToMetricsDeleteRuleResult - The result of which submitted events to metrics rules were successfully and unsuccessfully deleted.
type EventsToMetricsDeleteRuleResult struct {
	// Information about the rules that could not be deleted.
	Failures []EventsToMetricsDeleteRuleFailure `json:"failures"`
	// Rules that were successfully deleted.
	Successes []EventsToMetricsRule `json:"successes"`
}

// EventsToMetricsDeleteRuleSubmission - The details that were submitted when deleteing an events to metrics conversion rule.
type EventsToMetricsDeleteRuleSubmission struct {
	// A submitted account id.
	AccountID int `json:"accountId"`
	// A submitted rule id.
	RuleId string `json:"ruleId"`
}

// EventsToMetricsError - Error details when processing events to metrics rule requests.
type EventsToMetricsError struct {
	// A detailed error message.
	Description string `json:"description"`
	// The category of error that occurred.
	Reason EventsToMetricsErrorReason `json:"reason"`
}

// EventsToMetricsListRuleResult - A list of rule details to be returned.
type EventsToMetricsListRuleResult struct {
	// Event-to-metric rules to be returned.
	Rules []EventsToMetricsRule `json:"rules"`
}

// EventsToMetricsRule - Information about an event-to-metric rule which creates metrics from events.
type EventsToMetricsRule struct {
	// Account with the event and where the metrics will be placed.
	AccountID int `json:"accountId"`
	// The time at which the rule was created
	CreatedAt DateTime `json:"createdAt"`
	// Additional information about the rule.
	Description string `json:"description"`
	// True means this rule is enabled. False means the rule is currently not creating metrics.
	Enabled bool `json:"enabled"`
	// The id, uniquely identifying the rule.
	ID string `json:"id"`
	// The name of the rule. This must be unique within an account.
	Name string `json:"name"`
	// Explains how to create metrics from events.
	Nrql string `json:"nrql"`
	// The time at which the rule was updated
	UpdatedAt DateTime `json:"updatedAt"`
}

// EventsToMetricsUpdateRuleFailure - Error details about the events to metrics rule that failed to be updated and why.
type EventsToMetricsUpdateRuleFailure struct {
	// Information about why the update failed.
	Errors []EventsToMetricsError `json:"errors"`
	// Input information about a failed update.
	Submitted EventsToMetricsUpdateRuleSubmission `json:"submitted"`
}

// EventsToMetricsUpdateRuleInput - Identifying information about the events to metrics rule you want to update.
type EventsToMetricsUpdateRuleInput struct {
	// A submitted account id.
	AccountID int `json:"accountId"`
	// Changes the state of the rule as being enabled or disabled.
	Enabled bool `json:"enabled"`
	// A submitted rule id.
	RuleId string `json:"ruleId"`
}

// EventsToMetricsUpdateRuleResult - The result of which submitted events to metrics rules were successfully and unsuccessfully update.
type EventsToMetricsUpdateRuleResult struct {
	// Rules that failed to get updated.
	Failures []EventsToMetricsUpdateRuleFailure `json:"failures"`
	// Rules that were successfully enabled or disabled.
	Successes []EventsToMetricsRule `json:"successes"`
}

// EventsToMetricsUpdateRuleSubmission - The details that were submitted when updating an events to metrics conversion rule.
type EventsToMetricsUpdateRuleSubmission struct {
	// A submitted account id.
	AccountID int `json:"accountId"`
	// Changes the state of the rule as being enabled or disabled.
	Enabled bool `json:"enabled"`
	// A submitted rule id.
	RuleId string `json:"ruleId"`
}

// DateTime - The `DateTime` scalar represents a date and time. The `DateTime` appears as an ISO8601 formatted string.
type DateTime string

// ID - The `ID` scalar type represents a unique identifier, often used to
// refetch an object or as key for a cache. The ID type appears in a JSON
// response as a String; however, it is not intended to be human-readable.
// When expected as an input type, any string (such as `"4"`) or integer
// (such as `4`) input value will be accepted as an ID.
type ID string
