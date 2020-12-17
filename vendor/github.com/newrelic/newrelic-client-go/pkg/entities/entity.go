package entities

import (
	"github.com/newrelic/newrelic-client-go/pkg/errors"
)

// Entity represents a New Relic One entity.
type Entity struct {
	AccountID  int              `json:"accountId,omitempty"`
	Domain     EntityDomainType `json:"domain,omitempty"`
	EntityType EntityType       `json:"entityType,omitempty"` // Full Type (ie APM_APPLICATION_ENTITY)
	GUID       string           `json:"guid,omitempty"`
	Name       string           `json:"name,omitempty"`
	Permalink  string           `json:"permalink,omitempty"`
	Reporting  bool             `json:"reporting,omitempty"`
	Type       Type             `json:"type,omitempty"`

	// ApmApplicationEntity, BrowserApplicationEntity, MobileApplicationEntity
	AlertSeverity *EntityAlertSeverityType `json:"alertSeverity,omitempty"`
	ApplicationID *int                     `json:"applicationId,omitempty"`

	// Stitch in other structs
	ApmApplicationEntity
	BrowserApplicationEntity
}

// ApmApplicationEntity represents the unique fields returned on the ApmApplicationEntity interface
type ApmApplicationEntity struct {
	Language             *string                                          `json:"language,omitempty"`
	RunningAgentVersions *ApmApplicationEntityOutlineRunningAgentVersions `json:"runningAgentVersions,omitempty"`
	Settings             *ApmApplicationEntityOutlineSettings             `json:"settings,omitempty"`
}

type ApmApplicationEntityOutlineSettings struct {
	ApdexTarget      *float64 `json:"apdexTarget,omitempty"`
	ServerSideConfig *bool    `json:"serverSideConfig"`
}

type ApmApplicationEntityOutlineRunningAgentVersions struct {
	MaxVersion *string `json:"maxVersion,omitempty"`
	MinVersion *string `json:"minVersion,omitempty"`
}

// BrowserApplicationEntity represents the unique fields returned on the BrowserApplicationEntity interface
type BrowserApplicationEntity struct {
	ServingApmApplicationID *int `json:"servingApmApplicationId,omitempty"`
}

// EntityType represents a New Relic One entity type (full)
type EntityType string

// Type represents a New Relic One entity type (short)
type Type string

var (
	// Types specifies the possible types for a New Relic One entity.
	Types = struct {
		Application Type
		Dashboard   Type
		Host        Type
		Monitor     Type
		Workload    Type
	}{
		Application: "APPLICATION",
		Dashboard:   "DASHBOARD",
		Host:        "HOST",
		Monitor:     "MONITOR",
		Workload:    "WORKLOAD",
	}
)

var (
	// EntityTypes specifies the possible types for a New Relic One entity.
	EntityTypes = struct {
		Application EntityType
		Browser     EntityType
		Dashboard   EntityType
		Host        EntityType
		Mobile      EntityType
		Monitor     EntityType
		Workload    EntityType
	}{
		Application: "APM_APPLICATION_ENTITY",
		Browser:     "BROWSER_APPLICATION_ENTITY",
		Dashboard:   "DASHBOARD_ENTITY",
		Host:        "INFRASTRUCTURE_HOST_ENTITY",
		Mobile:      "MOBILE_APPLICATION_ENTITY",
		Monitor:     "SYNTHETIC_MONITOR_ENTITY",
		Workload:    "WORKLOAD_ENTITY",
	}
)

// EntityDomainType represents a New Relic One entity domain.
type EntityDomainType string

var (
	// EntityDomains specifies the possible domains for a New Relic One entity.
	EntityDomains = struct {
		APM            EntityDomainType
		Browser        EntityDomainType
		Infrastructure EntityDomainType
		Mobile         EntityDomainType
		Nr1            EntityDomainType
		Synthetics     EntityDomainType
		Visualization  EntityDomainType
	}{
		APM:            "APM",
		Browser:        "BROWSER",
		Infrastructure: "INFRA",
		Mobile:         "MOBILE",
		Nr1:            "NR1",
		Synthetics:     "SYNTH",
		Visualization:  "VIZ",
	}
)

// EntityAlertSeverityType represents a New Relic One entity alert severity.
type EntityAlertSeverityType string

var (
	// EntityAlertSeverities specifies the possible alert severities for a New Relic One entity.
	EntityAlertSeverities = struct {
		Critical      EntityAlertSeverityType
		NotAlerting   EntityAlertSeverityType
		NotConfigured EntityAlertSeverityType
		Warning       EntityAlertSeverityType
	}{
		Critical:      "CRITICAL",
		NotAlerting:   "NOT_ALERTING",
		NotConfigured: "NOT_CONFIGURED",
		Warning:       "WARNING",
	}
)

// SearchEntitiesParams represents a set of search parameters for retrieving New Relic One entities.
type SearchEntitiesParams struct {
	AlertSeverity                 EntityAlertSeverityType `json:"alertSeverity,omitempty"`
	Domain                        EntityDomainType        `json:"domain,omitempty"`
	InfrastructureIntegrationType string                  `json:"infrastructureIntegrationType,omitempty"`
	Name                          string                  `json:"name,omitempty"`
	Reporting                     *bool                   `json:"reporting,omitempty"`
	Tags                          *TagValue               `json:"tags,omitempty"`
	Type                          EntityType              `json:"type,omitempty"`
}

// SearchEntities searches New Relic One entities based on the provided search parameters.
func (e *Entities) SearchEntities(params SearchEntitiesParams) ([]*Entity, error) {
	entities := []*Entity{}
	var nextCursor *string

	for ok := true; ok; ok = nextCursor != nil {
		resp := searchEntitiesResponse{}
		vars := map[string]interface{}{
			"queryBuilder": params,
			"cursor":       nextCursor,
		}

		if err := e.client.NerdGraphQuery(searchEntitiesQuery, vars, &resp); err != nil {
			return nil, err
		}

		entities = append(entities, resp.Actor.EntitySearch.Results.Entities...)

		nextCursor = resp.Actor.EntitySearch.Results.NextCursor
	}

	return entities, nil
}

// GetEntities retrieves a set of New Relic One entities by their entity guids.
func (e *Entities) GetEntities(guids []string) ([]*Entity, error) {
	resp := getEntitiesResponse{}
	vars := map[string]interface{}{
		"guids": guids,
	}

	if err := e.client.NerdGraphQuery(getEntitiesQuery, vars, &resp); err != nil {
		return nil, err
	}

	if len(resp.Actor.Entities) == 0 {
		return nil, errors.NewNotFound("")
	}

	return resp.Actor.Entities, nil
}

// GetEntity retrieve a set of New Relic One entities by their entity guids.
func (e *Entities) GetEntity(guid string) (*Entity, error) {
	resp := getEntityResponse{}
	vars := map[string]interface{}{
		"guid": guid,
	}

	if err := e.client.NerdGraphQuery(getEntityQuery, vars, &resp); err != nil {
		return nil, err
	}

	if resp.Actor.Entity == nil {
		return nil, errors.NewNotFound("")
	}

	return resp.Actor.Entity, nil
}

const (
	// graphqlEntityStructFields is the set of fields that we want returned on entity queries,
	// and should map back directly to the Entity struct
	graphqlEntityStructFields = `
					accountId
					domain
					entityType
					guid
					name
					permalink
					reporting
					type
`

	graphqlApmApplicationEntityFields = `
					... on ApmApplicationEntity {
						applicationId
						alertSeverity
						language
						runningAgentVersions {
							maxVersion
							minVersion
						}
						settings {
							apdexTarget
							serverSideConfig
						}
					}`

	graphqlApmApplicationEntityOutlineFields = `
					... on ApmApplicationEntityOutline {
						applicationId
						alertSeverity
						language
						runningAgentVersions {
							maxVersion
							minVersion
						}
						settings {
							apdexTarget
							serverSideConfig
						}
					}`

	graphqlBrowserApplicationEntityFields = `
		... on BrowserApplicationEntity {
			alertSeverity
			applicationId
			servingApmApplicationId
	}`

	graphqlBrowserApplicationEntityOutlineFields = `
		... on BrowserApplicationEntityOutline {
			alertSeverity
			applicationId
			servingApmApplicationId
	}`

	graphqlMobileApplicationEntityFields = `
		... on MobileApplicationEntity {
			alertSeverity
			applicationId
	}`

	graphqlMobileApplicationEntityOutlineFields = `
		... on MobileApplicationEntityOutline {
			alertSeverity
			applicationId
	}`

	getEntitiesQuery = `query($guids: [String!]!) { actor { entities(guids: $guids)  {` +
		graphqlEntityStructFields +
		graphqlApmApplicationEntityFields +
		graphqlBrowserApplicationEntityFields +
		graphqlMobileApplicationEntityFields +
		` } } }`

	getEntityQuery = `query($guid: String!) { actor { entity(guid: $guid)  {` +
		graphqlEntityStructFields +
		graphqlApmApplicationEntityFields +
		graphqlBrowserApplicationEntityFields +
		graphqlMobileApplicationEntityFields +
		` } } }`

	searchEntitiesQuery = `
		query($queryBuilder: EntitySearchQueryBuilder, $cursor: String) {
			actor {
				entitySearch(queryBuilder: $queryBuilder)  {
					results(cursor: $cursor) {
						nextCursor
						entities {` +
		graphqlEntityStructFields +
		graphqlApmApplicationEntityOutlineFields +
		graphqlBrowserApplicationEntityOutlineFields +
		graphqlMobileApplicationEntityOutlineFields +
		` } } } } }`
)

type searchEntitiesResponse struct {
	Actor struct {
		EntitySearch struct {
			Results struct {
				NextCursor *string
				Entities   []*Entity
			}
		}
	}
}

type getEntitiesResponse struct {
	Actor struct {
		Entities []*Entity
	}
}

type getEntityResponse struct {
	Actor struct {
		Entity *Entity
	}
}
