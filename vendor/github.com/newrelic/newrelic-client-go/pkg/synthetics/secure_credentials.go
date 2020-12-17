package synthetics

// SecureCredential represents a Synthetics secure credential.
type SecureCredential struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Value       string `json:"value"`
	CreatedAt   *Time  `json:"createdAt"`
	LastUpdated *Time  `json:"lastUpdated"`
}

// GetSecureCredentials is used to retrieve all secure credentials from your New Relic account.
func (s *Synthetics) GetSecureCredentials() ([]*SecureCredential, error) {
	resp := getSecureCredentialsResponse{}

	_, err := s.client.Get(s.config.Region().SyntheticsURL("/v1/secure-credentials"), nil, &resp)
	if err != nil {
		return nil, err
	}

	return resp.SecureCredentials, nil
}

// GetSecureCredential is used to retrieve a specific secure credential from your New Relic account.
func (s *Synthetics) GetSecureCredential(key string) (*SecureCredential, error) {
	var sc SecureCredential

	_, err := s.client.Get(s.config.Region().SyntheticsURL("/v1/secure-credentials", key), nil, &sc)
	if err != nil {
		return nil, err
	}

	return &sc, nil
}

// AddSecureCredential is used to add a secure credential to your New Relic account.
func (s *Synthetics) AddSecureCredential(key, value, description string) (*SecureCredential, error) {
	sc := &SecureCredential{
		Key:         key,
		Value:       value,
		Description: description,
	}

	_, err := s.client.Post(s.config.Region().SyntheticsURL("/v1/secure-credentials"), nil, sc, nil)
	if err != nil {
		return nil, err
	}

	return sc, nil
}

// UpdateSecureCredential is used to update a secure credential in your New Relic account.
func (s *Synthetics) UpdateSecureCredential(key, value, description string) (*SecureCredential, error) {
	sc := &SecureCredential{
		Key:         key,
		Value:       value,
		Description: description,
	}

	_, err := s.client.Put(s.config.Region().SyntheticsURL("/v1/secure-credentials", key), nil, sc, nil)

	if err != nil {
		return nil, err
	}

	return sc, nil
}

// DeleteSecureCredential deletes a secure credential from your New Relic account.
func (s *Synthetics) DeleteSecureCredential(key string) error {
	_, err := s.client.Delete(s.config.Region().SyntheticsURL("/v1/secure-credentials", key), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

type getSecureCredentialsResponse struct {
	SecureCredentials []*SecureCredential `json:"secureCredentials"`
	Count             int                 `json:"count"`
}
