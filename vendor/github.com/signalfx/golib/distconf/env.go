package distconf

import "os"

type envDisco struct {
	noopCloser
}

func (p *envDisco) Get(key string) ([]byte, error) {
	val := os.Getenv(key)
	if val == "" {
		return nil, nil
	}
	return []byte(val), nil
}

// Env creates a backing config reader that reads properties from environment variables
func Env() Reader {
	return &envDisco{}
}

// EnvLoader is a loading helper for Env{} config variables
func EnvLoader() BackingLoader {
	return BackingLoaderFunc(func() (Reader, error) {
		return Env(), nil
	})
}
