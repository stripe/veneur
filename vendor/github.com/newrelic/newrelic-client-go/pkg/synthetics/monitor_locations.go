package synthetics

// MonitorLocation represents a valid location for a New Relic Synthetics monitor.
type MonitorLocation struct {
	HighSecurityMode bool   `json:"highSecurityMode"`
	Private          bool   `json:"private"`
	Name             string `json:"name"`
	Label            string `json:"label"`
	Description      string `json:"description"`
}

// GetMonitorLocations is used to retrieve all valid locations for Synthetics monitors.
func (s *Synthetics) GetMonitorLocations() ([]*MonitorLocation, error) {
	url := "/v1/locations"

	resp := []*MonitorLocation{}

	_, err := s.client.Get(s.config.Region().SyntheticsURL(url), nil, &resp)
	if err != nil {
		return resp, err
	}

	return resp, nil
}
