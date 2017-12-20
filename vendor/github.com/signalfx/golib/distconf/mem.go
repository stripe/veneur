package distconf

import "sync"

type memConfig struct {
	vals    map[string][]byte
	watches map[string][]backingCallbackFunction
	mu      sync.Mutex
}

func (m *memConfig) Get(key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, exists := m.vals[key]
	if !exists {
		return nil, nil
	}
	return b, nil
}

func (m *memConfig) Write(key string, value []byte) error {
	m.mu.Lock()
	if value == nil {
		delete(m.vals, key)
	} else {
		m.vals[key] = value
	}
	m.mu.Unlock()

	for _, calls := range m.watches[key] {
		calls(key)
	}
	return nil
}

func (m *memConfig) Watch(key string, callback backingCallbackFunction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, existing := m.watches[key]
	if !existing {
		m.watches[key] = []backingCallbackFunction{}
	}
	m.watches[key] = append(m.watches[key], callback)
	return nil
}

func (m *memConfig) Close() {
}

// Mem creates a memory config
func Mem() ReaderWriter {
	return &memConfig{
		vals:    make(map[string][]byte),
		watches: make(map[string][]backingCallbackFunction),
	}
}

// MemLoader is a helper for loading a memory conf
func MemLoader() BackingLoader {
	return BackingLoaderFunc(func() (Reader, error) {
		return Mem(), nil
	})
}
