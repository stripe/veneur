// Package nrdb provides a programmatic API for interacting with NRDB, New Relic's Datastore
package nrdb

// Query facilitates making a NRQL query.
func (n *Nrdb) Query(accountID int, query Nrql) (*NrdbResultContainer, error) {
	respBody := gqlNrglQueryResponse{}

	vars := map[string]interface{}{
		"accountId": accountID,
		"query":     query,
	}

	if err := n.client.NerdGraphQuery(gqlNrqlQuery, vars, &respBody); err != nil {
		return nil, err
	}

	return &respBody.Actor.Account.Nrql, nil
}

func (n *Nrdb) QueryHistory() (*[]NrqlHistoricalQuery, error) {
	respBody := gqlNrglQueryHistoryResponse{}
	vars := map[string]interface{}{}

	if err := n.client.NerdGraphQuery(gqlNrqlQueryHistoryQuery, vars, &respBody); err != nil {
		return nil, err
	}

	return &respBody.Actor.NrqlQueryHistory, nil
}

const (
	gqlNrqlQueryHistoryQuery = `{ actor { nrqlQueryHistory { accountId nrql timestamp } } }`

	gqlNrqlQuery = `query($query: Nrql!, $accountId: Int!) { actor { account(id: $accountId) { nrql(query: $query) {
    currentResults otherResult previousResults results totalResult
    metadata { eventTypes facets messages timeWindow { begin compareWith end since until } }
  } } } }`
)

type gqlNrglQueryResponse struct {
	Actor struct {
		Account struct {
			Nrql NrdbResultContainer
		}
	}
}

type gqlNrglQueryHistoryResponse struct {
	Actor struct {
		NrqlQueryHistory []NrqlHistoricalQuery
	}
}
