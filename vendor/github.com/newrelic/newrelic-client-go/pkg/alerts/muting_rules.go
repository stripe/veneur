package alerts

// MutingRule represents the alert suppression mechanism in the Alerts API.
type MutingRule struct {
	ID            int                      `json:"id,string,omitempty"`
	AccountID     int                      `json:"accountId,omitempty"`
	Condition     MutingRuleConditionGroup `json:"condition,omitempty"`
	CreatedAt     string                   `json:"createdAt,omitempty"`
	CreatedByUser ByUser                   `json:"createdByUser,omitempty"`
	Description   string                   `json:"description,omitempty"`
	Enabled       bool                     `json:"enabled,omitempty"`
	Name          string                   `json:"name,omitempty"`
	UpdatedAt     string                   `json:"updatedAt,omitempty"`
	UpdatedByUser ByUser                   `json:"updatedByUser,omitempty"`
}

// ByUser is a collection of the user information that created or updated the muting rule.
type ByUser struct {
	Email    string `json:"email"`
	Gravatar string `json:"gravatar"`
	ID       int    `json:"id"`
	Name     string `json:"name"`
}

// MutingRuleConditionGroup is a collection of conditions for muting.
type MutingRuleConditionGroup struct {
	Conditions []MutingRuleCondition `json:"conditions"`
	Operator   string                `json:"operator"`
}

// MutingRuleCondition is a single muting rule condition.
type MutingRuleCondition struct {
	Attribute string   `json:"attribute"`
	Operator  string   `json:"operator"`
	Values    []string `json:"values"`
}

// MutingRuleCreateInput is the input for creating muting rules.
type MutingRuleCreateInput struct {
	Condition   MutingRuleConditionGroup `json:"condition"`
	Description string                   `json:"description"`
	Enabled     bool                     `json:"enabled"`
	Name        string                   `json:"name"`
}

// MutingRuleUpdateInput is the input for updating a rule.
type MutingRuleUpdateInput struct {
	// Condition is is available from the API, but the json needs to be handled
	// properly.

	Condition   *MutingRuleConditionGroup `json:"condition,omitempty"`
	Description string                    `json:"description,omitempty"`
	Enabled     bool                      `json:"enabled,omitempty"`
	Name        string                    `json:"name,omitempty"`
}

// ListMutingRules queries for all muting rules in a given account.
func (a *Alerts) ListMutingRules(accountID int) ([]MutingRule, error) {
	vars := map[string]interface{}{
		"accountID": accountID,
	}

	resp := alertMutingRuleListResponse{}

	if err := a.client.NerdGraphQuery(alertsMutingRulesQuery, vars, &resp); err != nil {
		return nil, err
	}

	return resp.Actor.Account.Alerts.MutingRules, nil
}

// GetMutingRule queries for a single muting rule matching the given ID.
func (a *Alerts) GetMutingRule(accountID, ruleID int) (*MutingRule, error) {
	vars := map[string]interface{}{
		"accountID": accountID,
		"ruleID":    ruleID,
	}

	resp := alertMutingRulesGetResponse{}

	if err := a.client.NerdGraphQuery(alertsMutingRulesGet, vars, &resp); err != nil {
		return nil, err
	}

	return &resp.Actor.Account.Alerts.MutingRule, nil
}

// CreateMutingRule is the mutation to create a muting rule for the given account and input.
func (a *Alerts) CreateMutingRule(accountID int, rule MutingRuleCreateInput) (*MutingRule, error) {
	vars := map[string]interface{}{
		"accountID": accountID,
		"rule":      rule,
	}

	resp := alertMutingRuleCreateResponse{}

	if err := a.client.NerdGraphQuery(alertsMutingRulesCreate, vars, &resp); err != nil {
		return nil, err
	}

	return &resp.AlertsMutingRuleCreate, nil
}

// UpdateMutingRule is the mutation to update an existing muting rule.
func (a *Alerts) UpdateMutingRule(accountID int, ruleID int, rule MutingRuleUpdateInput) (*MutingRule, error) {
	vars := map[string]interface{}{
		"accountID": accountID,
		"ruleID":    ruleID,
		"rule":      rule,
	}

	resp := alertMutingRuleUpdateResponse{}

	if err := a.client.NerdGraphQuery(alertsMutingRulesUpdate, vars, &resp); err != nil {
		return nil, err
	}

	return &resp.AlertsMutingRuleUpdate, nil
}

// DeleteMutingRule is the mutation.
func (a *Alerts) DeleteMutingRule(accountID int, ruleID int) error {
	vars := map[string]interface{}{
		"accountID": accountID,
		"ruleID":    ruleID,
	}

	resp := alertMutingRuleDeleteResponse{}

	if err := a.client.NerdGraphQuery(alertsMutingRuleDelete, vars, &resp); err != nil {
		return err
	}

	return nil
}

type alertMutingRuleCreateResponse struct {
	AlertsMutingRuleCreate MutingRule `json:"alertsMutingRuleCreate"`
}

type alertMutingRuleUpdateResponse struct {
	AlertsMutingRuleUpdate MutingRule `json:"alertsMutingRuleUpdate"`
}

type alertMutingRuleDeleteResponse struct {
	AlertsMutingRuleDelete struct {
		ID string `json:"id"`
	} `json:"alertsMutingRuleDelete"`
}

type alertMutingRuleListResponse struct {
	Actor struct {
		Account struct {
			Alerts struct {
				MutingRules []MutingRule `json:"mutingRules"`
			} `json:"alerts"`
		} `json:"account"`
	} `json:"actor"`
}

type alertMutingRulesGetResponse struct {
	Actor struct {
		Account struct {
			Alerts struct {
				MutingRule MutingRule `json:"mutingRule"`
			} `json:"alerts"`
		} `json:"account"`
	} `json:"actor"`
}

const (
	alertsMutingRulesQuery = `query($accountID: Int!) {
		actor {
			account(id: $accountID) {
				alerts {
					mutingRules {
						name
						description
						enabled
						condition {
							operator
							conditions {
								attribute
								operator
								values
							}
						}
					}
				}
			}
		}
	}`

	alertsMutingRulesGet = `query($accountID: Int!, $ruleID: ID!) {
		actor {
			account(id: $accountID) {
				alerts {
					mutingRule(id: $ruleID) {` +
		alertsMutingRuleFields +
		`}}}}}`

	alertsMutingRuleFields = ` 
		accountId
		condition {
			conditions {
				attribute
				operator
				values
			}
			operator
		}
		id
		name
		enabled
		description
		createdAt
		createdByUser {
			email
			gravatar
			id
			name
		}
		updatedAt
		updatedByUser {
			email
			gravatar
			id
			name
		}
	`

	alertsMutingRulesCreate = `mutation CreateRule($accountID: Int!, $rule: AlertsMutingRuleInput!) {
		alertsMutingRuleCreate(accountId: $accountID, rule: $rule) {` +
		alertsMutingRuleFields +

		`}
	}`

	alertsMutingRulesUpdate = `mutation UpdateRule($accountID: Int!, $ruleID: ID!, $rule: AlertsMutingRuleUpdateInput!) {
		alertsMutingRuleUpdate(accountId: $accountID, id: $ruleID, rule: $rule) {` +
		alertsMutingRuleFields +
		`}
	}`

	alertsMutingRuleDelete = `mutation DeleteRule($accountID: Int!, $ruleID: ID!) {
		alertsMutingRuleDelete(accountId: $accountID, id: $ruleID) {
			id
		}
	}`
)
